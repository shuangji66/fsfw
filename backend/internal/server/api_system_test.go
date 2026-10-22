package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// /api/system/info 不再带 socket / baseURL / pidFile，新增 platform。
func TestSystemInfoShape(t *testing.T) {
	s, _ := newTestServer(t)

	rec := serve(s, http.MethodGet, "/api/system/info", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d（%s）", rec.Code, rec.Body.String())
	}
	info := keysOf(t, rec.Body.Bytes())
	assertKeys(t, "system/info", info,
		"version", "platform", "pid", "dataDir", "projectsDir", "outputDir",
		"logDir", "tmpDir", "logFile", "fnpack", "lastSeq", "startedAt")
	for _, gone := range []string{"socket", "baseURL", "pidFile", "trim", "user"} {
		if _, ok := info[gone]; ok {
			t.Errorf("system/info 里不该再有 %q", gone)
		}
	}

	var body struct {
		Platform string `json:"platform"`
		PID      int    `json:"pid"`
		DataDir  string `json:"dataDir"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Platform != runtime.GOOS {
		t.Errorf("platform = %q，期望 %q", body.Platform, runtime.GOOS)
	}
	if body.PID != os.Getpid() {
		t.Errorf("pid = %d，期望 %d", body.PID, os.Getpid())
	}
}

// reveal 只测参数校验：把真正拉起文件管理器的函数换掉，不弹窗口。
func TestRevealValidation(t *testing.T) {
	s, _ := newTestServer(t)

	old := revealInFileManager
	calls := 0
	var gotPath string
	var gotDir bool
	revealInFileManager = func(p string, isDir bool) error {
		calls++
		gotPath, gotDir = p, isDir
		return nil
	}
	t.Cleanup(func() { revealInFileManager = old })

	dir := t.TempDir()
	file := filepath.Join(dir, "a.fpk")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if code, body := doJSON(t, s, "/api/system/reveal", map[string]any{"path": "relative/path"}); code != http.StatusBadRequest {
		t.Errorf("相对路径：状态码 = %d（%v），期望 400", code, body)
	}
	if code, _ := doJSON(t, s, "/api/system/reveal", map[string]any{"path": "  "}); code != http.StatusBadRequest {
		t.Errorf("空路径：状态码 = %d，期望 400", code)
	}
	if code, _ := doJSON(t, s, "/api/system/reveal", map[string]any{"path": filepath.Join(dir, "missing")}); code != http.StatusNotFound {
		t.Errorf("不存在的路径：状态码 = %d，期望 404", code)
	}
	if calls != 0 {
		t.Fatalf("校验失败时不该调用文件管理器（调用了 %d 次）", calls)
	}

	if code, body := doJSON(t, s, "/api/system/reveal", map[string]any{"path": dir}); code != http.StatusOK {
		t.Fatalf("打开目录：状态码 = %d（%v）", code, body)
	}
	if calls != 1 || gotPath != dir || !gotDir {
		t.Errorf("目录参数不对：calls=%d path=%q isDir=%v", calls, gotPath, gotDir)
	}

	if code, _ := doJSON(t, s, "/api/system/reveal", map[string]any{"path": file}); code != http.StatusOK {
		t.Errorf("打开文件：状态码 = %d", code)
	}
	if calls != 2 || gotPath != file || gotDir {
		t.Errorf("文件参数不对：calls=%d path=%q isDir=%v", calls, gotPath, gotDir)
	}

	if rec := serve(s, http.MethodGet, "/api/system/reveal", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 状态码 = %d，期望 405", rec.Code)
	}
}

// 拉起失败要回 500 且带上原因（用户至少知道是系统没配合）。
func TestRevealLaunchFailure(t *testing.T) {
	s, _ := newTestServer(t)
	old := revealInFileManager
	revealInFileManager = func(string, bool) error { return errors.New("explorer.exe 起不来") }
	t.Cleanup(func() { revealInFileManager = old })

	code, body := doJSON(t, s, "/api/system/reveal", map[string]any{"path": t.TempDir()})
	if code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d（%v），期望 500", code, body)
	}
}

// 图标导出：把 base64 字节写到保存对话框给的绝对路径。
func TestSystemSaveFile(t *testing.T) {
	s, _ := newTestServer(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "icon.png")
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 1, 2, 3}

	code, body := doJSON(t, s, "/api/system/save-file", map[string]any{
		"path": target,
		// encoding/json 把 []byte 编成 base64，和前端传的字符串对齐。
		"data": png,
	})
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d（%v），期望 200", code, body)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("目标文件不存在: %v", err)
	}
	if string(got) != string(png) {
		t.Errorf("写出的内容 = %v，期望 %v", got, png)
	}

	// 覆盖已有文件
	next := []byte("second")
	if code, body = doJSON(t, s, "/api/system/save-file", map[string]any{"path": target, "data": next}); code != http.StatusOK {
		t.Fatalf("覆盖保存：状态码 = %d（%v），期望 200", code, body)
	}
	if got, _ := os.ReadFile(target); string(got) != string(next) {
		t.Errorf("覆盖后内容 = %q，期望 %q", got, next)
	}

	// 保存失败不能把旧文件截断：把目标换成一个不存在的目录。
	code, _ = doJSON(t, s, "/api/system/save-file", map[string]any{
		"path": filepath.Join(dir, "missing", "icon.png"),
		"data": png,
	})
	if code != http.StatusBadRequest {
		t.Errorf("不存在的目录：状态码 = %d，期望 400", code)
	}
	if got, _ := os.ReadFile(target); string(got) != string(next) {
		t.Errorf("失败路径不该动到旧文件，现在是 %q", got)
	}
}

func TestSystemSaveFileValidation(t *testing.T) {
	s, _ := newTestServer(t)

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"相对路径", map[string]any{"path": "icon.png", "data": []byte("x")}, http.StatusBadRequest},
		{"空路径", map[string]any{"path": "  ", "data": []byte("x")}, http.StatusBadRequest},
		{"空内容", map[string]any{"path": filepath.Join(t.TempDir(), "x.png")}, http.StatusBadRequest},
		{"目标是目录", map[string]any{"path": t.TempDir(), "data": []byte("x")}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code, body := doJSON(t, s, "/api/system/save-file", tc.body); code != tc.want {
				t.Errorf("状态码 = %d（%v），期望 %d", code, body, tc.want)
			}
		})
	}

	if rec := serve(s, http.MethodGet, "/api/system/save-file", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 状态码 = %d，期望 405", rec.Code)
	}
}
