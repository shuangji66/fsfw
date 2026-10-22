package server

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type fnpackInfo struct {
	Bin     string `json:"bin"`
	Version string `json:"version"`
	Message string `json:"message"`
	OK      bool   `json:"ok"`
}

type systemInfo struct {
	Version   string     `json:"version"`
	Platform  string     `json:"platform"`
	PID       int        `json:"pid"`
	DataDir   string     `json:"dataDir"`
	Projects  string     `json:"projectsDir"`
	OutputDir string     `json:"outputDir"`
	LogDir    string     `json:"logDir"`
	TmpDir    string     `json:"tmpDir"`
	LogFile   string     `json:"logFile"`
	Fnpack    fnpackInfo `json:"fnpack"`
	LastSeq   int64      `json:"lastSeq"`
	StartedAt string     `json:"startedAt"`
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) error {
	bin, ver, msg := s.fn.Probe(r.Context())
	return writeJSON(w, systemInfo{
		Version:   s.cfg.Version,
		Platform:  runtime.GOOS,
		PID:       os.Getpid(),
		DataDir:   s.cfg.DataDir,
		Projects:  s.prj.ProjectRoot(),
		OutputDir: s.prj.OutputDir(),
		LogDir:    s.cfg.LogDir,
		TmpDir:    s.cfg.TmpDir,
		LogFile:   s.cfg.LogFile(),
		Fnpack: fnpackInfo{
			Bin:     bin,
			Version: ver,
			Message: msg,
			OK:      msg == "",
		},
		LastSeq:   s.log.LastSeq(),
		StartedAt: s.startedAt.Format("2006-01-02 15:04:05"),
	})
}

func (s *Server) handleSystemFnpack(w http.ResponseWriter, r *http.Request) error {
	bin, ver, msg := s.fn.Probe(r.Context())
	return writeJSON(w, fnpackInfo{Bin: bin, Version: ver, Message: msg, OK: msg == ""})
}

// revealInFileManager 在系统文件管理器里打开一个路径。
//
// 变量而不是直接调用：测试里替换成空实现，只验参数校验，不真的弹窗口。
var revealInFileManager = openPathInFileManager

// handleSystemReveal 在文件管理器中打开某个路径（界面上「打开所在文件夹」）。
//
// 路径来自前端（可能是用户手填的），所以这里要自己校验：必须是绝对路径、必须存在。
// 不做「必须在项目目录里」的限制——这个接口的用途就是打开数据目录之外的位置，
// 而进程本身只有当前用户的权限，能打开的东西也就是用户自己能打开的东西。
func (s *Server) handleSystemReveal(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	p := strings.TrimSpace(req.Path)
	if p == "" {
		return badRequest("缺少 path")
	}
	if !filepath.IsAbs(p) {
		return badRequest("需要绝对路径：%s", p)
	}
	p = filepath.Clean(p)

	// Stat（不是 Lstat）：指向目录的链接也应该按目录打开，断链则按「不存在」处理。
	fi, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return notFound("路径不存在：%s", p)
		}
		return badRequest("无法访问 %s：%v", p, err)
	}

	if err := revealInFileManager(p, fi.IsDir()); err != nil {
		return internal("打开文件管理器失败: %v", err)
	}
	s.log.Infof("在文件管理器中打开 %s", p)
	return writeJSON(w, map[string]any{"ok": true})
}
