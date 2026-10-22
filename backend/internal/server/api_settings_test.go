package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// keysOf 解出 JSON 对象的所有字段名（用来钉住「多余的字段不能有」）。
func keysOf(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	out := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("响应不是 JSON 对象: %v（%s）", err, raw)
	}
	return out
}

func assertKeys(t *testing.T, what string, got map[string]json.RawMessage, want ...string) {
	t.Helper()
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("%s 缺少字段 %q（实际 %v）", what, k, keysOfNames(got))
		}
	}
	if len(got) != len(want) {
		t.Errorf("%s 字段数 = %d，期望 %d（实际 %v）", what, len(got), len(want), keysOfNames(got))
	}
}

func keysOfNames(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// 设置响应必须严格是契约里的形状：没有 user、没有 trim。
func TestSettingsResponseShape(t *testing.T) {
	s, dataDir := newTestServer(t)

	rec := serve(s, http.MethodGet, "/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d（%s）", rec.Code, rec.Body.String())
	}
	top := keysOf(t, rec.Body.Bytes())
	assertKeys(t, "GET /api/settings", top, "dirs", "dataDir")

	dirs := keysOf(t, top["dirs"])
	assertKeys(t, "dirs", dirs,
		"projectRoot", "outputDir",
		"projectRootFromUser", "outputDirFromUser",
		"defaultProjectRoot", "defaultOutputDir", "outputEnvIgnored")

	var body settingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.DataDir != dataDir {
		t.Errorf("dataDir = %q，期望 %q", body.DataDir, dataDir)
	}
	if body.Dirs.ProjectRoot != s.prj.ProjectRoot() || body.Dirs.OutputDir != s.prj.OutputDir() {
		t.Errorf("生效目录与 Manager 不一致：%+v", body.Dirs)
	}
	if body.Dirs.DefaultProjectRoot != filepath.Join(dataDir, "projects") {
		t.Errorf("defaultProjectRoot = %q", body.Dirs.DefaultProjectRoot)
	}
	if body.Dirs.ProjectRootFromUser || body.Dirs.OutputDirFromUser {
		t.Errorf("默认状态下两个 FromUser 都应当是 false：%+v", body.Dirs)
	}
	for _, gone := range []string{"user", "trim"} {
		if _, ok := top[gone]; ok {
			t.Errorf("响应里不该再有 %q", gone)
		}
	}
}

// PUT：空字符串恢复默认；未提到的字段沿用保存值。
func TestSettingsPutAndRestoreDefault(t *testing.T) {
	s, dataDir := newTestServer(t)
	root := filepath.Join(t.TempDir(), "newroot")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	code, body := doJSON(t, s, "/api/settings", map[string]any{"projectRoot": root})
	if code != http.StatusOK {
		t.Fatalf("改目录：状态码 = %d（%v）", code, body)
	}
	if got := s.prj.ProjectRoot(); got != root {
		t.Fatalf("项目根目录 = %q，期望 %q", got, root)
	}
	dirs := body["dirs"].(map[string]any)
	if dirs["projectRoot"] != root || dirs["projectRootFromUser"] != true {
		t.Errorf("响应没有反映新的目录设置：%v", dirs)
	}
	if dirs["outputDir"] != filepath.Join(dataDir, "output") {
		t.Errorf("没提到的输出目录应当保持不变：%v", dirs["outputDir"])
	}
	if saved := s.state.Dirs(); saved.ProjectRoot != root {
		t.Errorf("设置没有落盘：%+v", saved)
	}

	code, body = doJSON(t, s, "/api/settings", map[string]any{"projectRoot": ""})
	if code != http.StatusOK {
		t.Fatalf("恢复默认：状态码 = %d（%v）", code, body)
	}
	if got, want := s.prj.ProjectRoot(), filepath.Join(dataDir, "projects"); got != want {
		t.Errorf("恢复默认后项目根目录 = %q，期望 %q", got, want)
	}
	dirs = body["dirs"].(map[string]any)
	if dirs["projectRootFromUser"] != false {
		t.Errorf("恢复默认后 FromUser 应当为 false：%v", dirs)
	}
}

// import-path 路由还在；空路径仍然回 400（ACL 那一层已经没了，但校验还在）。
func TestImportPathRouteKept(t *testing.T) {
	s, _ := newTestServer(t)

	rec := serve(s, http.MethodGet, "/api/projects/import-path", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 状态码 = %d，期望 405", rec.Code)
	}

	code, body := doJSON(t, s, "/api/projects/import-path", map[string]any{"path": "  "})
	if code != http.StatusBadRequest {
		t.Errorf("空路径状态码 = %d（%v），期望 400", code, body)
	}
}

// 旧接口必须真的没了：/api/trim/* 回 404，而不是被别的路由接住。
func TestTrimRoutesRemoved(t *testing.T) {
	s, _ := newTestServer(t)
	for _, path := range []string{"/api/trim/status", "/api/trim/folders", "/api/trim/semantic"} {
		if rec := serve(s, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s 状态码 = %d，期望 404", path, rec.Code)
		}
	}
}
