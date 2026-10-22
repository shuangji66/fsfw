// Package server 承载 HTTP 层：路由与所有 API handler。
//
// 只监听回环地址上的 TCP 端口（由 Tauri 外壳拉起，端口默认由系统分配），
// 所有路由都挂在 /api 下；界面由 Tauri 自己提供，这里没有静态资源。
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"fsfw/internal/config"
	"fsfw/internal/fnpack"
	"fsfw/internal/logging"
	"fsfw/internal/projects"
	"fsfw/internal/state"
)

// tokenHeader 是 JSON / XHR 请求携带 token 的请求头（下载、EventSource 用查询参数）。
const tokenHeader = "X-FSFW-Token"

// Server 是全部依赖的持有者。
type Server struct {
	cfg   *config.Config
	log   *logging.Logger
	fn    *fnpack.Client
	prj   *projects.Manager
	state *state.Store

	mux       *http.ServeMux
	handler   http.Handler
	startedAt time.Time
	// listenAddr 是实际监听地址（端口是系统分配的时候与配置里的 :0 不同）。
	listenAddr string
	// closing 在关服时关闭：SSE 这类「一直挂着等事件」的 handler 靠它主动退出，
	// 否则 Shutdown 只能等满超时（实测每次 SIGTERM 都要白等 5 秒）。
	closing chan struct{}
}

// New 构造 Server。
func New(cfg *config.Config, log *logging.Logger, fn *fnpack.Client, prj *projects.Manager, st *state.Store) *Server {
	s := &Server{
		cfg:       cfg,
		log:       log,
		fn:        fn,
		prj:       prj,
		state:     st,
		mux:       http.NewServeMux(),
		startedAt: time.Now(),
		closing:   make(chan struct{}),
	}
	s.routes()
	if cfg.Token == "" {
		// 只记一次：纯浏览器开发时每个请求都提醒一遍会把日志淹掉。
		s.log.Warnf("没有配置 API token（--token / %s），鉴权已关闭：只适合本机浏览器开发", config.EnvToken)
	}
	s.handler = s.withMiddleware(s.mux)
	return s
}

// Handler 是唯一对外入口（CORS + token 鉴权 + 路由）。
func (s *Server) Handler() http.Handler { return s.handler }

// ---- 路由 ----

func (s *Server) routes() {
	s.api("/api/system/info", s.handleSystemInfo)
	s.api("/api/system/fnpack", s.handleSystemFnpack)
	s.api("/api/system/reveal", s.handleSystemReveal)

	s.api("/api/projects", s.handleProjects)
	s.api("/api/projects/create", s.handleProjectCreate)
	s.api("/api/projects/delete", s.handleProjectDelete)
	s.api("/api/projects/exists", s.handleProjectExists)
	s.api("/api/projects/platforms", s.handleProjectPlatforms)
	s.api("/api/projects/settings", s.handleProjectSettings)
	s.api("/api/projects/import", s.handleProjectImport)
	s.api("/api/projects/import-path", s.handleProjectImportPath)
	s.api("/api/projects/pack", s.handleProjectPack)
	s.api("/api/projects/artifacts", s.handleProjectArtifacts)
	s.api("/api/projects/artifacts/delete", s.handleProjectArtifactDelete)

	s.api("/api/fs/list", s.handleFsList)
	s.api("/api/fs/read", s.handleFsRead)
	s.api("/api/fs/write", s.handleFsWrite)
	s.api("/api/fs/create", s.handleFsCreate)
	s.api("/api/fs/delete", s.handleFsDelete)
	s.api("/api/fs/rename", s.handleFsRename)
	s.api("/api/fs/copy", s.handleFsCopy)
	s.api("/api/fs/upload", s.handleFsUpload)
	s.api("/api/fs/extract", s.handleFsExtract)
	s.api("/api/fs/download", s.handleFsDownload)

	s.api("/api/logs", s.handleLogs)
	s.api("/api/logs/stream", s.handleLogStream)

	// 设置（目录）。
	s.api("/api/settings", s.handleSettings)

	// 兜底：界面由 Tauri 提供，这里没有静态资源；未知路径回 JSON 404，
	// 免得前端从一个 200 text/html 里猜「接口是不是写错了」。
	s.mux.HandleFunc("/", s.handleUnknown)
}

func (s *Server) handleUnknown(w http.ResponseWriter, r *http.Request) {
	writeError(w, notFound("没有这个接口：%s", r.URL.Path))
}

func (s *Server) api(pattern string, h handlerFunc) {
	s.mux.HandleFunc(pattern, s.wrap(pattern, h))
}

func (s *Server) wrap(pattern string, h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// 一个 handler 里的 panic 不能把整个进程带走（net/http 自己会 recover，但那样
		// 连接直接断掉、日志里也只有一行 stderr）。这里兜住并记进日志器。
		var err error
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					s.log.Errorf("处理 %s 时 panic: %v\n%s", pattern, rec, debug.Stack())
					err = internal("服务内部错误: %v", rec)
				}
			}()
			err = h(w, r)
		}()
		if err != nil {
			writeError(w, err)
		}
		d := time.Since(start)
		s.log.Debugf("%s %s %s %s", r.Method, pattern, d.Round(time.Millisecond), errText(err))
	}
}

// ---- 中间件 ----

// withMiddleware 把 CORS 包在最外层：预检请求不带 token，必须在鉴权之前回掉。
func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return s.cors(s.auth(next))
}

// auth 校验 API token（请求头 X-FSFW-Token 或查询参数 token）。
//
// token 为空时关闭鉴权：纯浏览器开发（npm run dev）没有 Tauri 注入 token，
// 只能这样跑；启动时已经记过一条警告。
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Token != "" && !s.tokenOK(r) {
			writeError(w, fail(http.StatusUnauthorized,
				"缺少或错误的 API token（请求头 %s，或查询参数 token）", tokenHeader))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) tokenOK(r *http.Request) bool {
	got := r.Header.Get(tokenHeader)
	if got == "" {
		got = r.URL.Query().Get("token")
	}
	// 定长比较：别让响应时间泄露 token 的前缀。
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Token)) == 1
}

// cors 只回显白名单里的 Origin。
//
// 不能用 `*`：带 token 的接口一旦对任意站点开放，用户浏览器里任何一个页面都能
// 调这个 API。白名单是 Tauri 自己的 origin 加本机 Vite 开发服务器。
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := originAllowed(origin)
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, Content-Length")
		}
		if r.Method == http.MethodOptions {
			if allowed {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-FSFW-Token, Range")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originAllowed 判断 Origin 是否在白名单里。
func originAllowed(origin string) bool {
	switch origin {
	case "http://tauri.localhost", "https://tauri.localhost", "tauri://localhost":
		return true
	}
	return isLocalDevOrigin(origin)
}

// isLocalDevOrigin 认 http://localhost:<any> 与 http://127.0.0.1:<any>（Vite 开发）。
func isLocalDevOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1"
}

// ---- Run ----

// Run 启动服务并阻塞直到 ctx 结束。
func (s *Server) Run(ctx context.Context) error {
	// 第二道防线：config.Load 已经拒绝过非回环地址，这里再判一次——Server 也可能
	// 被别的入口直接构造出来。
	if !config.IsLoopbackHost(s.cfg.ListenHost) {
		return fmt.Errorf("监听地址必须是回环地址（127.0.0.1 / ::1 / localhost），当前是 %q", s.cfg.ListenHost)
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(s.cfg.ListenHost, strconv.Itoa(s.cfg.ListenPort)))
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", s.cfg.ListenAddr(), err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("监听地址不是 TCP：%s", ln.Addr())
	}
	s.listenAddr = net.JoinHostPort(s.cfg.ListenHost, strconv.Itoa(addr.Port))

	// 监听成功之后立刻写 runtime 文件：Tauri 外壳就是靠它发现端口和 token 的。
	if err := s.writeRuntimeFile(addr.Port); err != nil {
		return fmt.Errorf("写 runtime 文件失败: %w", err)
	}

	srv := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		// net/http 自己的报错（superfluous WriteHeader、连接被重置…）默认只进 stderr，
		// 而日志弹窗是用户唯一能看到的地方。
		ErrorLog: log.New(logWriter{s.log}, "", 0),
	}

	errc := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	s.banner()

	select {
	case err := <-errc:
		s.cleanup()
		return err
	case <-ctx.Done():
	}

	s.log.Infof("收到退出信号，正在停止…")
	// 先让 SSE 这类长连接 handler 自己退出，再 Shutdown：Shutdown 不会取消请求的
	// context，挂着不动的 handler 只能等满超时。
	close(s.closing)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		s.log.Warnf("优雅停止超时（%v），强制关闭连接", err)
		_ = srv.Close()
	}
	s.cleanup()
	return nil
}

// banner 只留一行「已启动」。
//
// 完整的路径清单不往日志里灌：它们是**每次启动都一样的噪声**，而日志弹窗是给
// 用户看「刚才发生了什么」的地方。要看这些值有两个现成的出口——界面的设置弹窗，
// 以及 `fsfw-backend --paths`。
func (s *Server) banner() {
	s.log.Infof("FSFW %s 已启动，监听 %s", s.cfg.Version, s.listenAddr)
}

// runtimeInfo 是 run/server.json 的内容（外壳轮询它拿到端口和 token）。
type runtimeInfo struct {
	Port      int    `json:"port"`
	Token     string `json:"token"`
	PID       int    `json:"pid"`
	Version   string `json:"version"`
	DataDir   string `json:"dataDir"`
	StartedAt string `json:"startedAt"`
}

// writeRuntimeFile 原子地写出运行信息（临时文件 + rename，权限 0600）。
//
// 外壳会在文件出现后立刻读它，所以不能直接往目标文件里写：读到半份 JSON 时
// 外壳只能报「启动超时」。token 在文件里，权限必须是 0600。
func (s *Server) writeRuntimeFile(port int) error {
	if strings.TrimSpace(s.cfg.RuntimeFile) == "" {
		return fmt.Errorf("未配置 runtime 文件路径（--runtime-file / %s）", config.EnvRuntimeFile)
	}
	info := runtimeInfo{
		Port:      port,
		Token:     s.cfg.Token,
		PID:       os.Getpid(),
		Version:   s.cfg.Version,
		DataDir:   s.cfg.DataDir,
		StartedAt: s.startedAt.Format("2006-01-02 15:04:05"),
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.cfg.RuntimeFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".server-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// 成功 rename 之后这个路径已经不存在了，Remove 只是失败路径上的兜底。
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("设置 %s 的权限失败: %w", tmpName, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Windows 上 rename 不能覆盖已存在的文件；这个路径本来就该由我们独占，
	// 先删掉旧的那一份（比如上一个实例被强杀时留下的）。
	_ = os.Remove(s.cfg.RuntimeFile)
	if err := os.Rename(tmpName, s.cfg.RuntimeFile); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", s.cfg.RuntimeFile, err)
	}
	s.log.Debugf("已写入 runtime 文件 %s（端口 %d）", s.cfg.RuntimeFile, port)
	return nil
}

// removeRuntimeFile 删除运行信息文件，但只删还是自己写的那一份。
//
// 启动新实例时旧实例的清理不能把新实例的文件删掉——外壳正等着读它。
func (s *Server) removeRuntimeFile() {
	if strings.TrimSpace(s.cfg.RuntimeFile) == "" {
		return
	}
	if data, err := os.ReadFile(s.cfg.RuntimeFile); err == nil {
		var info runtimeInfo
		if json.Unmarshal(data, &info) == nil && info.PID != 0 && info.PID != os.Getpid() {
			return
		}
	}
	if err := os.Remove(s.cfg.RuntimeFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.log.Warnf("删除 runtime 文件失败: %v", err)
	}
}

func (s *Server) cleanup() {
	s.removeRuntimeFile()
}

// ---- handler 基础设施 ----

// handlerFunc 返回 error，由 wrap 统一转成 HTTP 响应。
type handlerFunc func(http.ResponseWriter, *http.Request) error

// httpError 携带状态码。
type httpError struct {
	Status int
	Msg    string
}

func (e *httpError) Error() string { return e.Msg }

func fail(status int, format string, a ...any) error {
	return &httpError{Status: status, Msg: fmt.Sprintf(format, a...)}
}

func badRequest(format string, a ...any) error { return fail(http.StatusBadRequest, format, a...) }
func notFound(format string, a ...any) error   { return fail(http.StatusNotFound, format, a...) }
func internal(format string, a ...any) error {
	return fail(http.StatusInternalServerError, format, a...)
}

func writeJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var he *httpError
	if errors.As(err, &he) {
		status = he.Status
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// maxJSONBody 是请求体的上限。写文件接口也走它，所以给得宽一点。
const maxJSONBody = 32 << 20

// maxUploadBytes 是上传 / 导入单个请求的体积上限（用户要求 1G）。
//
// 以前只给了 ParseMultipartForm 一个「内存阈值」，总量不设限：请求体超出的部分会落到
// 进程的 os.TempDir() 里，一直写下去能把盘灌满。这里用 http.MaxBytesReader 卡住总量。
// 变量而不是常量：测试里把它调小，才能在没有 1G 数据的情况下验「超限回 413」。
var maxUploadBytes int64 = 1 << 30

// uploadMemoryLimit 是 multipart 在内存里保留的字节数，超出部分落临时文件。
const uploadMemoryLimit = 32 << 20

// tooLargeErr 判断错误是不是「请求体超过 MaxBytesReader 的上限」。
func tooLargeErr(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// extractTimeout 是一次解压的上限（外部命令可能被畸形归档拖住）。
const extractTimeout = 10 * time.Minute

func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return badRequest("请求体为空")
	}
	// 用 LimitedReader 而不是 io.LimitReader：截断之后要能分辨「超限」和「本来就是
	// 坏 JSON」。以前两者都报「请求体不是合法 JSON: unexpected EOF」，用户存一个
	// 超过 32MB 的文件只会看到这句莫名其妙的话。
	body := &io.LimitedReader{R: r.Body, N: maxJSONBody + 1}
	dec := json.NewDecoder(body)
	if err := dec.Decode(v); err != nil {
		if body.N <= 0 {
			return badRequest("请求体过大（上限 %d MB）", maxJSONBody>>20)
		}
		return badRequest("请求体不是合法 JSON: %v", err)
	}
	if body.N <= 0 {
		return badRequest("请求体过大（上限 %d MB）", maxJSONBody>>20)
	}
	return nil
}

// logWriter 把 net/http 内部的错误转进日志器（见 srv.ErrorLog）。
type logWriter struct{ log *logging.Logger }

func (w logWriter) Write(p []byte) (int, error) {
	if msg := strings.TrimSpace(string(p)); msg != "" {
		w.log.Warnf("http: %s", msg)
	}
	return len(p), nil
}

func errText(err error) string {
	if err == nil {
		return "ok"
	}
	return "ERR: " + err.Error()
}

func queryRel(r *http.Request) (string, error) {
	p, err := pathParam(r)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(p) == "" {
		return ".", nil
	}
	// 路径一律以 projects 根为基准，例如 "myapp/config/privilege"。
	p = strings.TrimPrefix(path.Clean("/"+p), "/")
	if p == "" {
		// 空串是「项目根」：以前 `?path=../../` 会归一成 ""，而前端拿它当目录缓存的键，
		// 于是同一个根目录出现 "" 和 "." 两种写法、缓存对不上。
		p = "."
	}
	return p, nil
}

// pathParam 取出 path 参数：先看查询串，查询串里没有再看 JSON 请求体。
//
// 两种都认，是因为前端把 list / read 的 path 放进了 body（写接口本来就走 body），
// 而 Tauri 的 webview 不保证把查询串原样送到后端；一旦 path 丢了，列目录就退化成
// 列项目根目录或者直接报错，表现为「文件列表永远是空的」。查询串形式留着，方便
// curl 和文档里的例子。
func pathParam(r *http.Request) (string, error) {
	if p := r.URL.Query().Get("path"); p != "" {
		return p, nil
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		return "", nil
	}
	if r.Body == nil {
		return "", nil
	}
	var req struct {
		Path string `json:"path"`
	}
	// 解不出来不算错：查询串本来就允许为空（表示项目根目录）。
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
	return req.Path, nil
}
