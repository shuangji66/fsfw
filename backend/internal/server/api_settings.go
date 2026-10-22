package server

import (
	"net/http"
	"strings"
)

// ---- 设置 ----

type dirsPayload struct {
	// ProjectRoot / OutputDir 是当前生效的绝对路径（可能来自用户设置，也可能是默认值）。
	ProjectRoot string `json:"projectRoot"`
	OutputDir   string `json:"outputDir"`
	// 两个 From* 表示该路径是不是用户设置过的——为假时前端显示「默认」。
	ProjectRootFromUser bool `json:"projectRootFromUser"`
	OutputDirFromUser   bool `json:"outputDirFromUser"`
	// 默认值，用于「恢复默认」时给用户看会回到哪里。
	DefaultProjectRoot string `json:"defaultProjectRoot"`
	DefaultOutputDir   string `json:"defaultOutputDir"`
	// OutputEnvIgnored 为真表示 FSFW_OUTPUT_DIR 与项目目录撞了、已被忽略，
	// 生效的是默认输出目录。界面要说明这一点，否则用户会以为环境变量没生效。
	OutputEnvIgnored bool `json:"outputEnvIgnored"`
}

type settingsResponse struct {
	Dirs    dirsPayload `json:"dirs"`
	DataDir string      `json:"dataDir"`
}

func (s *Server) settingsSnapshot() settingsResponse {
	saved := s.state.Dirs()
	return settingsResponse{
		Dirs: dirsPayload{
			ProjectRoot:         s.prj.ProjectRoot(),
			OutputDir:           s.prj.OutputDir(),
			ProjectRootFromUser: saved.ProjectRoot != "",
			OutputDirFromUser:   saved.OutputDir != "",
			DefaultProjectRoot:  s.cfg.ProjectsDir,
			DefaultOutputDir:    s.cfg.OutputDir,
			OutputEnvIgnored:    s.cfg.OutputEnvIgnored,
		},
		DataDir: s.cfg.DataDir,
	}
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case http.MethodGet:
		return writeJSON(w, s.settingsSnapshot())
	case http.MethodPut, http.MethodPost:
		var req struct {
			ProjectRoot *string `json:"projectRoot"`
			OutputDir   *string `json:"outputDir"`
		}
		if err := decodeJSON(r, &req); err != nil {
			return err
		}

		// 未提到的字段沿用当前保存的值：前端每次只改一个槽位也没问题。
		saved := s.state.Dirs()
		next := saved
		if req.ProjectRoot != nil {
			next.ProjectRoot = strings.TrimSpace(*req.ProjectRoot)
		}
		if req.OutputDir != nil {
			next.OutputDir = strings.TrimSpace(*req.OutputDir)
		}

		// 先真正换一遍（校验 + 打开沙箱），成功了再落盘——反过来的话，一次
		// 打不开的路径会被持久化，下次启动还得靠人去删 state.json。
		if err := s.prj.SetRoots(next.ProjectRoot, next.OutputDir); err != nil {
			return badRequest("%v", err)
		}
		if err := s.state.Save(next); err != nil {
			return internal("保存目录设置失败: %v", err)
		}

		s.log.Infof("目录设置已更新：项目根目录 %s，输出目录 %s", s.prj.ProjectRoot(), s.prj.OutputDir())
		return writeJSON(w, s.settingsSnapshot())
	default:
		return fail(http.StatusMethodNotAllowed, "不支持的方法")
	}
}

// handleProjectImportPath 导入本机某个路径上的 fpk（Tauri 文件选择器选的）。
//
// 桌面端没有「目录授权」这一层：进程本来就是当前用户，选到的路径就是它有权访问的
// 路径。路径本身仍然要校验（绝对路径、存在、是文件、不是符号链接），那是
// projects.ImportPath 的事，不在这里重复。
func (s *Server) handleProjectImportPath(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return badRequest("缺少 path")
	}

	info, err := s.prj.ImportPath(r.Context(), path)
	if err != nil {
		// 路径不合法 / 包太大都是用户能自己处理的，回 400；其余才是服务端故障。
		return projectError(err)
	}
	s.log.Infof("从 %s 导入项目 %s", path, info.Name)
	return writeJSON(w, map[string]any{"ok": true, "project": info})
}
