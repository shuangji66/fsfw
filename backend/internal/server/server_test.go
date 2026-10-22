package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"fsfw/internal/config"
)

// 这一组钉住桌面版的进程契约：token 鉴权、CORS、runtime 文件、只监听回环地址。

func serve(s *Server, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// token 为空时关闭鉴权（纯浏览器开发），配置了之后缺 / 错 token 一律 401。
func TestTokenAuth(t *testing.T) {
	s, _ := newTestServerWith(t, func(c *config.Config) { c.Token = "s3cret" })

	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"没有 token", nil, http.StatusUnauthorized},
		{"错误的 token", map[string]string{tokenHeader: "nope"}, http.StatusUnauthorized},
		{"正确的 token", map[string]string{tokenHeader: "s3cret"}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(s, http.MethodGet, "/api/projects", tc.headers)
			if rec.Code != tc.want {
				t.Fatalf("状态码 = %d，期望 %d（%s）", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusUnauthorized && !strings.Contains(rec.Body.String(), "token") {
				t.Errorf("401 应当说明是 token 的问题：%s", rec.Body.String())
			}
		})
	}

	// 下载 / EventSource 走查询参数。
	if rec := serve(s, http.MethodGet, "/api/projects?token=s3cret", nil); rec.Code != http.StatusOK {
		t.Errorf("查询参数 token：状态码 = %d（%s）", rec.Code, rec.Body.String())
	}
	if rec := serve(s, http.MethodGet, "/api/projects?token=wrong", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("错误的查询参数 token 应当 401，实际 %d", rec.Code)
	}
}

// token 为空时不能把本机 API 拦死（浏览器开发没有 Tauri 注入 token）。
func TestTokenDisabledWhenEmpty(t *testing.T) {
	s, _ := newTestServer(t)
	if rec := serve(s, http.MethodGet, "/api/projects", nil); rec.Code != http.StatusOK {
		t.Fatalf("token 为空时不该鉴权：%d（%s）", rec.Code, rec.Body.String())
	}
}

// CORS：只回显白名单里的 Origin，绝不用 `*`。
func TestCORSReflectsAllowedOrigin(t *testing.T) {
	s, _ := newTestServer(t)

	for _, origin := range []string{
		"http://tauri.localhost",
		"https://tauri.localhost",
		"tauri://localhost",
		"http://localhost:5173",
		"http://127.0.0.1:1420",
	} {
		rec := serve(s, http.MethodGet, "/api/projects", map[string]string{"Origin": origin})
		if rec.Code != http.StatusOK {
			t.Fatalf("%s：状态码 = %d", origin, rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("%s：Allow-Origin = %q，期望原样回显", origin, got)
		}
		if got := rec.Header().Values("Vary"); len(got) == 0 || !strings.Contains(strings.Join(got, ","), "Origin") {
			t.Errorf("%s：缺少 Vary: Origin（%v）", origin, got)
		}
		if got := rec.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, "Content-Disposition") {
			t.Errorf("%s：Expose-Headers = %q", origin, got)
		}
	}

	// 不在白名单里的站点：一个 CORS 头都不给，浏览器自己会挡住响应。
	rec := serve(s, http.MethodGet, "/api/projects", map[string]string{"Origin": "https://evil.example"})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("非白名单 Origin 不该被回显：%q", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("非白名单 Origin 的普通请求仍然照常处理：%d", rec.Code)
	}
}

// 预检请求不带 token，必须在鉴权之前回 204。
func TestCORSPreflight(t *testing.T) {
	// Tauri dev：带 token，但页面来自 Vite（localhost:5273），靠 --dev 放行。
	s, _ := newTestServerWith(t, func(c *config.Config) {
		c.Token = "s3cret"
		c.Dev = true
	})

	rec := serve(s, http.MethodOptions, "/api/fs/upload", map[string]string{
		"Origin":                         "http://localhost:5173",
		"Access-Control-Request-Method":  "POST",
		"Access-Control-Request-Headers": "content-type,x-fsfw-token",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("预检状态码 = %d，期望 204（%s）", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Allow-Origin = %q", got)
	}
	headers := rec.Header().Get("Access-Control-Allow-Headers")
	for _, want := range []string{"Content-Type", tokenHeader, "Range"} {
		if !strings.Contains(headers, want) {
			t.Errorf("Allow-Headers = %q，缺少 %q", headers, want)
		}
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Errorf("Allow-Methods = %q", got)
	}
}

// runtime 文件：内容、权限、原子写、退出时清理。
func TestRuntimeFileLifecycle(t *testing.T) {
	s, dataDir := newTestServer(t)
	if err := s.writeRuntimeFile(51234); err != nil {
		t.Fatalf("写 runtime 文件失败: %v", err)
	}

	path := s.cfg.RuntimeFile
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 runtime 文件失败: %v", err)
	}
	var info runtimeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("runtime 文件不是合法 JSON: %v\n%s", err, data)
	}
	if info.Port != 51234 {
		t.Errorf("port = %d", info.Port)
	}
	if info.PID != os.Getpid() {
		t.Errorf("pid = %d，期望 %d", info.PID, os.Getpid())
	}
	if info.Version != "test" {
		t.Errorf("version = %q", info.Version)
	}
	if info.DataDir != dataDir {
		t.Errorf("dataDir = %q，期望 %q", info.DataDir, dataDir)
	}
	if _, err := time.Parse("2006-01-02 15:04:05", info.StartedAt); err != nil {
		t.Errorf("startedAt = %q 不能按 yyyy-mm-dd hh:mm:ss 解析: %v", info.StartedAt, err)
	}

	// token 在文件里，权限必须是 0600（Windows 上不检查 POSIX 位）。
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("runtime 文件权限 = %04o，期望 0600", fi.Mode().Perm())
	}

	// 临时文件不能留在目录里（外壳会轮询这个目录）。
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".server-") {
			t.Errorf("临时文件没有清掉：%s", e.Name())
		}
	}

	s.removeRuntimeFile()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("退出后 runtime 文件应当被删掉: %v", err)
	}
}

// 退出时只删自己写的那一份：新实例的 runtime 文件不能被旧实例的清理顺手删掉。
func TestRemoveRuntimeFileKeepsForeignFile(t *testing.T) {
	s, _ := newTestServer(t)
	js := `{"port":1234,"pid":1,"version":"other"}` + "\n"
	if err := os.MkdirAll(filepath.Dir(s.cfg.RuntimeFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.cfg.RuntimeFile, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}

	s.removeRuntimeFile()
	if _, err := os.Stat(s.cfg.RuntimeFile); err != nil {
		t.Errorf("别的进程写的 runtime 文件不该被删掉: %v", err)
	}
}

// 非回环地址必须在监听之前就被拒绝（config 已经拦过一次，这里是第二道）。
func TestRunRejectsNonLoopback(t *testing.T) {
	s, _ := newTestServerWith(t, func(c *config.Config) { c.ListenHost = "0.0.0.0" })
	err := s.Run(t.Context())
	if err == nil {
		t.Fatal("监听 0.0.0.0 应当被拒绝")
	}
	if !strings.Contains(err.Error(), "回环地址") {
		t.Errorf("错误不够明确: %v", err)
	}
}

// 未知路径回 JSON 404（界面由 Tauri 提供，这里没有静态资源兜底）。
func TestUnknownPathJSON404(t *testing.T) {
	s, _ := newTestServer(t)
	for _, path := range []string{"/api/nope", "/", "/project/demo"} {
		rec := serve(s, http.MethodGet, path, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s 状态码 = %d，期望 404", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("%s Content-Type = %q，期望 JSON", path, ct)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
			t.Errorf("%s 响应不是带 error 的 JSON：%s", path, rec.Body.String())
		}
	}
}

// 生产构建（带 token、非 dev）不放行任意 localhost 端口：只有 Tauri 自己的源算数。
func TestCORSRejectsLocalDevOriginInProduction(t *testing.T) {
	s, _ := newTestServerWith(t, func(c *config.Config) { c.Token = "s3cret" })

	rec := serve(s, http.MethodGet, "/api/projects", map[string]string{
		"Origin":    "http://localhost:5273",
		tokenHeader: "s3cret",
	})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("生产模式不该回显 localhost 源：%q", got)
	}

	rec = serve(s, http.MethodGet, "/api/projects", map[string]string{
		"Origin":    "http://tauri.localhost",
		tokenHeader: "s3cret",
	})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://tauri.localhost" {
		t.Errorf("Tauri 源应当照常放行：%q", got)
	}
}
