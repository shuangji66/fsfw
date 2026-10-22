package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shortTempDir 给数据目录用：不把测试名拼进路径，免得路径太长之后报一些和测试
// 内容毫无关系的错误。
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cfg")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// 只允许回环地址：监听 0.0.0.0 会把一个带 token 的本机 API 暴露到局域网上。
func TestIsLoopbackHost(t *testing.T) {
	cases := map[string]bool{
		"":            true,
		"127.0.0.1":   true,
		"127.0.0.5":   true,
		"::1":         true,
		"[::1]":       true,
		"localhost":   true,
		"LOCALHOST":   true,
		"0.0.0.0":     false,
		"::":          false,
		"192.168.1.2": false,
		"example.com": false,
	}
	for host, want := range cases {
		if got := IsLoopbackHost(host); got != want {
			t.Errorf("IsLoopbackHost(%q) = %v，期望 %v", host, got, want)
		}
	}
}

// 数据目录默认值：Windows 放在「文档」下，其它系统沿用 XDG 风格。
func TestDefaultDataDirFor(t *testing.T) {
	home := shortTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if got, want := defaultDataDirFor("windows"), filepath.Join(home, "Documents", "fpkstudio"); got != want {
		t.Errorf("Windows 默认数据目录 = %q，期望 %q", got, want)
	}

	xdg := shortTempDir(t)
	t.Setenv("XDG_DATA_HOME", xdg)
	if got, want := defaultDataDirFor("linux"), filepath.Join(xdg, "fsfw"); got != want {
		t.Errorf("XDG 默认数据目录 = %q，期望 %q", got, want)
	}

	t.Setenv("XDG_DATA_HOME", "")
	if got, want := defaultDataDirFor("linux"), filepath.Join(home, ".local", "share", "fsfw"); got != want {
		t.Errorf("HOME 默认数据目录 = %q，期望 %q", got, want)
	}
}

// 项目目录由环境变量给出：没设置时回退到数据目录下的 projects/。
func TestProjectsDirFromEnv(t *testing.T) {
	dataDir := shortTempDir(t)

	t.Setenv(EnvDataDir, dataDir)
	t.Setenv(EnvProjectsDir, "")
	if c, err := Load("test"); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	} else if c.ProjectsDir != filepath.Join(c.DataDir, "projects") {
		t.Errorf("ProjectsDir = %q，期望回退到 %s", c.ProjectsDir, filepath.Join(c.DataDir, "projects"))
	}

	outside := t.TempDir()
	t.Setenv(EnvProjectsDir, outside)
	if c, err := Load("test"); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	} else if c.ProjectsDir != outside {
		t.Errorf("ProjectsDir = %q，期望 %q", c.ProjectsDir, outside)
	}
}

// 两个目录撞在一起时保项目目录：输出目录退回默认值，并标记「环境变量被忽略」。
func TestOutputDirFallsBackWhenSameAsProjects(t *testing.T) {
	dataDir := shortTempDir(t)
	same := shortTempDir(t)

	t.Setenv(EnvDataDir, dataDir)
	t.Setenv(EnvProjectsDir, same)
	t.Setenv(EnvOutputDir, same)

	c, err := Load("test")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if c.ProjectsDir != same {
		t.Errorf("ProjectsDir = %q，期望保项目目录 %q", c.ProjectsDir, same)
	}
	if want := filepath.Join(dataDir, "output"); c.OutputDir != want {
		t.Errorf("OutputDir = %q，期望回退到 %q", c.OutputDir, want)
	}
	if !c.OutputEnvIgnored {
		t.Error("OutputEnvIgnored 应当为真，界面要据此标注")
	}

	// 只设输出目录、项目目录用默认值时不该误判。
	t.Setenv(EnvProjectsDir, "")
	c, err = Load("test")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if c.OutputEnvIgnored {
		t.Error("两个目录不同时不应当标记忽略")
	}
}

// 桌面端的配置入口：监听地址 / 端口 / token / runtime 文件 / 父进程 pid。
func TestLoadDesktopOptions(t *testing.T) {
	dataDir := shortTempDir(t)
	t.Setenv(EnvDataDir, dataDir)
	t.Setenv(EnvListenHost, "127.0.0.1")
	t.Setenv(EnvListenPort, "45123")
	t.Setenv(EnvToken, "abc123")
	t.Setenv(EnvRuntimeFile, "")
	t.Setenv(EnvParentPID, "4242")

	c, err := Load("1.2.3")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if c.Version != "1.2.3" {
		t.Errorf("Version = %q", c.Version)
	}
	if c.ListenHost != "127.0.0.1" || c.ListenPort != 45123 {
		t.Errorf("监听地址 = %s:%d", c.ListenHost, c.ListenPort)
	}
	if got, want := c.ListenAddr(), "127.0.0.1:45123"; got != want {
		t.Errorf("ListenAddr() = %q，期望 %q", got, want)
	}
	if c.Token != "abc123" {
		t.Errorf("Token = %q", c.Token)
	}
	if c.ParentPID != 4242 {
		t.Errorf("ParentPID = %d", c.ParentPID)
	}
	if got, want := c.RuntimeFile, filepath.Join(dataDir, "run", "server.json"); got != want {
		t.Errorf("RuntimeFile = %q，期望 %q", got, want)
	}
	if got, want := c.LogFile(), filepath.Join(dataDir, "logs", "fsfw.log"); got != want {
		t.Errorf("LogFile() = %q，期望 %q", got, want)
	}
}

// 命令行参数优先于环境变量；没写的参数沿用环境变量。
func TestLoadFlagOverridesEnv(t *testing.T) {
	envData := shortTempDir(t)
	flagData := shortTempDir(t)
	t.Setenv(EnvDataDir, envData)
	t.Setenv(EnvListenHost, "127.0.0.1")
	t.Setenv(EnvListenPort, "1234")
	t.Setenv(EnvToken, "from-env")
	t.Setenv(EnvParentPID, "111")

	port := 5678
	token := "from-flag"
	data := flagData
	c, err := LoadWith("test", Options{
		DataDir:    &data,
		ListenPort: &port,
		Token:      &token,
	})
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if c.DataDir != filepath.Clean(flagData) {
		t.Errorf("DataDir = %q，期望 %q", c.DataDir, flagData)
	}
	if c.ListenPort != 5678 || c.Token != "from-flag" {
		t.Errorf("参数没有覆盖环境变量：port=%d token=%q", c.ListenPort, c.Token)
	}
	if c.ListenHost != "127.0.0.1" {
		t.Errorf("没写的参数应当沿用环境变量：host=%q", c.ListenHost)
	}
	if c.ParentPID != 111 {
		t.Errorf("没写的 ParentPID 应当沿用环境变量：%d", c.ParentPID)
	}
}

// 非回环地址要在加载配置时就拒绝，而不是等到 Listen 报一个难懂的错误。
func TestLoadRejectsNonLoopbackHost(t *testing.T) {
	dataDir := shortTempDir(t)
	t.Setenv(EnvDataDir, dataDir)

	for _, host := range []string{"0.0.0.0", "192.168.1.10", "::"} {
		t.Setenv(EnvListenHost, host)
		if _, err := Load("test"); err == nil {
			t.Errorf("监听 %q 应当被拒绝", host)
		} else if !strings.Contains(err.Error(), "回环地址") {
			t.Errorf("监听 %q 的错误不够明确: %v", host, err)
		}
	}
}

// 端口 / pid 写错要回一句人话。
func TestLoadRejectsBadNumbers(t *testing.T) {
	dataDir := shortTempDir(t)
	t.Setenv(EnvDataDir, dataDir)

	t.Setenv(EnvListenPort, "not-a-port")
	if _, err := Load("test"); err == nil {
		t.Error("非法端口应当报错")
	}
	t.Setenv(EnvListenPort, "70000")
	if _, err := Load("test"); err == nil {
		t.Error("越界端口应当报错")
	}
	t.Setenv(EnvListenPort, "")
	t.Setenv(EnvParentPID, "abc")
	if _, err := Load("test"); err == nil {
		t.Error("非法父进程 pid 应当报错")
	}
}

// 环境变量前缀统一是 FSFW_：改前缀时最容易漏掉一两个，这里钉住。
func TestEnvPrefix(t *testing.T) {
	names := []string{
		EnvDataDir, EnvProjectsDir, EnvOutputDir, EnvFnpack, EnvLogLevel,
		EnvRingSize, EnvListenHost, EnvListenPort, EnvToken, EnvRuntimeFile, EnvParentPID,
	}
	for _, name := range names {
		if !strings.HasPrefix(name, "FSFW_") {
			t.Errorf("环境变量 %q 没有 FSFW_ 前缀", name)
		}
		if strings.Contains(name, "FPKSTUDIO") {
			t.Errorf("环境变量 %q 还带着旧前缀", name)
		}
	}
}

// 子进程（fnpack）的临时目录必须落在数据目录里。
func TestEnvPointsTmpToDataDir(t *testing.T) {
	dataDir := shortTempDir(t)
	t.Setenv(EnvDataDir, dataDir)
	c, err := Load("test")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	got := map[string]string{}
	for _, kv := range c.Env() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			got[k] = v
		}
	}
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		if got[key] != c.TmpDir {
			t.Errorf("%s = %q，期望 %q", key, got[key], c.TmpDir)
		}
	}
}

// 开发模式：环境变量 / 命令行都能开，默认关。
func TestDevFlag(t *testing.T) {
	dataDir := shortTempDir(t)
	t.Setenv(EnvDataDir, dataDir)
	t.Setenv(EnvDev, "1")

	c, err := Load("test")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if !c.Dev {
		t.Error("FSFW_DEV=1 应当打开开发模式")
	}

	t.Setenv(EnvDev, "")
	dev := true
	c, err = LoadWith("test", Options{Dev: &dev})
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if !c.Dev {
		t.Error("Options.Dev 应当覆盖环境变量")
	}

	if c, err := Load("test"); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	} else if c.Dev {
		t.Error("默认不该是开发模式")
	}
}
