// Package config 负责 FSFW 的全部运行时配置。
//
// 设计原则：所有可变路径都由环境变量统一提供，程序自身不假设任何部署形态。
// 数据目录（projects / logs / run / tmp / state.json）与打包输出目录分开配置，
// 日志、运行信息、临时文件全部落在数据目录下。
//
// 桌面版的进程模型是「Tauri 外壳拉起一个只监听回环地址的子进程」：监听地址、
// 端口、token、runtime 文件都由外壳用命令行参数显式给出（见 main.go），
// 环境变量是同一组配置的备用入口。
package config

import (
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// 环境变量名。所有变量都以 FSFW_ 为前缀。
const (
	EnvDataDir     = "FSFW_DATA_DIR"
	EnvProjectsDir = "FSFW_PROJECTS_DIR"
	EnvOutputDir   = "FSFW_OUTPUT_DIR"
	EnvFnpack      = "FSFW_FNPACK"
	EnvLogLevel    = "FSFW_LOG_LEVEL"
	EnvRingSize    = "FSFW_LOG_RING"
	EnvListenHost  = "FSFW_HOST"
	EnvListenPort  = "FSFW_PORT"
	EnvToken       = "FSFW_TOKEN"
	EnvRuntimeFile = "FSFW_RUNTIME_FILE"
	EnvParentPID   = "FSFW_PARENT_PID"
)

// Config 是解析完成的运行时配置。
type Config struct {
	Version string

	// DataDir 是数据根目录，下面挂 ProjectsDir/LogDir/RunDir/TmpDir。
	DataDir string
	// OutputDir 是打包产物目录，允许在 DataDir 之外（例如挂到共享目录）。
	OutputDir string

	// ProjectsDir 是项目根目录，允许在 DataDir 之外（例如用户自己的文档目录）。
	ProjectsDir string
	LogDir      string
	RunDir      string
	TmpDir      string

	StateFile string

	// ListenHost 是 HTTP 监听地址，只允许回环地址（127.0.0.1 / ::1 / localhost）。
	ListenHost string
	// ListenPort 是监听端口，0 表示由系统分配（Tauri 外壳就是这么用的）。
	ListenPort int
	// Token 是 API 访问令牌；为空表示关闭鉴权（纯浏览器开发时用）。
	Token string
	// RuntimeFile 是外壳轮询的运行时信息文件（JSON，见 server.writeRuntimeFile）。
	RuntimeFile string
	// ParentPID 是父进程（Tauri 外壳）的 pid，非 0 时由看门狗盯着，父进程退出就跟着退。
	ParentPID int

	// FnpackBin 是 fnpack 可执行文件的路径或名字。
	FnpackBin string
	LogLevel  string
	LogRing   int

	// OutputEnvIgnored 为真表示 FSFW_OUTPUT_DIR 被忽略了（它和项目目录指到
	// 同一个路径），OutputDir 已经回退到数据目录下的默认值。界面据此标注。
	OutputEnvIgnored bool
}

// Options 是命令行参数里显式给出的覆盖值。
//
// 用指针区分「没写这个参数」和「写了一个空值」：前者沿用环境变量，后者是用户的
// 明确选择（例如 --token "" 关掉鉴权）。没写的字段保持 nil，配置全部来自环境变量。
type Options struct {
	DataDir     *string
	ListenHost  *string
	ListenPort  *int
	Token       *string
	RuntimeFile *string
	ParentPID   *int
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// Load 读取环境变量、创建目录之后的全部运行时配置。
func Load(version string) (*Config, error) {
	return LoadWith(version, Options{})
}

// LoadWith 同 Load，另外接受命令行参数的覆盖值（参数优先于环境变量）。
func LoadWith(version string, opt Options) (*Config, error) {
	dataDirRaw := env(EnvDataDir, defaultDataDir())
	if opt.DataDir != nil {
		dataDirRaw = strings.TrimSpace(*opt.DataDir)
		if dataDirRaw == "" {
			return nil, fmt.Errorf("数据目录不能为空（--data-dir / %s）", EnvDataDir)
		}
	}
	dataDir, err := filepath.Abs(dataDirRaw)
	if err != nil {
		return nil, fmt.Errorf("解析数据目录失败: %w", err)
	}

	host := env(EnvListenHost, "127.0.0.1")
	if opt.ListenHost != nil {
		host = strings.TrimSpace(*opt.ListenHost)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if !IsLoopbackHost(host) {
		return nil, fmt.Errorf(
			"监听地址必须是回环地址（127.0.0.1 / ::1 / localhost），当前是 %q：本服务只给本机的 Tauri 外壳用",
			host)
	}

	port, err := listenPort(env(EnvListenPort, ""))
	if err != nil {
		return nil, err
	}
	if opt.ListenPort != nil {
		port = *opt.ListenPort
	}
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("监听端口超出范围（0-65535）：%d", port)
	}

	parentPID, err := parsePID(env(EnvParentPID, ""))
	if err != nil {
		return nil, err
	}
	if opt.ParentPID != nil {
		parentPID = *opt.ParentPID
	}
	if parentPID < 0 || parentPID > math.MaxInt32 {
		return nil, fmt.Errorf("父进程 pid 不合法：%d", parentPID)
	}

	c := &Config{
		Version:   version,
		DataDir:   filepath.Clean(dataDir),
		OutputDir: cleanAbs(env(EnvOutputDir, filepath.Join(dataDir, "output"))),
		FnpackBin: env(EnvFnpack, "fnpack"),
		LogLevel:  strings.ToLower(env(EnvLogLevel, "info")),
		LogRing:   atoiDefault(env(EnvRingSize, ""), 5000),

		ListenHost: host,
		ListenPort: port,
		Token:      strings.TrimSpace(env(EnvToken, "")),
		ParentPID:  parentPID,
	}
	if opt.Token != nil {
		c.Token = strings.TrimSpace(*opt.Token)
	}

	c.ProjectsDir = cleanAbs(env(EnvProjectsDir, filepath.Join(c.DataDir, "projects")))
	// 项目目录与输出目录指到同一个路径时保项目目录，输出目录按「没设置」处理、
	// 退回默认值：两个目录混在一起会让产物既算项目产物又算输出产物，列两遍、复制
	// 到自己身上，谁都说不清它到底在哪。
	if samePath(c.ProjectsDir, c.OutputDir) {
		c.OutputEnvIgnored = true
		c.OutputDir = filepath.Join(c.DataDir, "output")
	}

	c.LogDir = filepath.Join(c.DataDir, "logs")
	c.RunDir = filepath.Join(c.DataDir, "run")
	c.TmpDir = filepath.Join(c.DataDir, "tmp")
	c.StateFile = filepath.Join(c.DataDir, "state.json")

	runtimeFile := env(EnvRuntimeFile, filepath.Join(c.RunDir, "server.json"))
	if opt.RuntimeFile != nil {
		runtimeFile = strings.TrimSpace(*opt.RuntimeFile)
		if runtimeFile == "" {
			return nil, fmt.Errorf("runtime 文件路径不能为空（--runtime-file / %s）", EnvRuntimeFile)
		}
	}
	c.RuntimeFile = cleanAbs(runtimeFile)

	for _, d := range []string{c.DataDir, c.OutputDir, c.ProjectsDir, c.LogDir, c.RunDir, c.TmpDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("创建目录 %s 失败: %w", d, err)
		}
		// 有些文件系统（网络盘、SMB 共享）上 mkdir 的 mode 会被忽略，新建项可能落成
		// 一个谁都不认识的权限，所以显式补一次——和 fsx 里的同一条约定。
		if err := os.Chmod(d, 0o755); err != nil {
			return nil, fmt.Errorf("设置目录 %s 的权限失败: %w", d, err)
		}
	}

	return c, nil
}

// LogFile 返回主日志文件路径。
func (c *Config) LogFile() string { return filepath.Join(c.LogDir, "fsfw.log") }

// ListenAddr 返回给日志和 --paths 用的监听地址。
func (c *Config) ListenAddr() string {
	return net.JoinHostPort(c.ListenHost, strconv.Itoa(c.ListenPort))
}

// Env 返回用于注入子进程的环境变量，让 fnpack 把临时文件写在数据目录里。
//
// 这不是洁癖：fnpack 会在 TMPDIR 里复制整个项目再打包，如果 TMPDIR 跨挂载点
// 或者落在受限目录（网络盘、被清理策略盯着的系统临时目录），fnpack 会在
// "Copy pack ... to tmp dir" 这一步直接失败。
func (c *Config) Env() []string {
	env := os.Environ()
	env = append(env,
		"TMPDIR="+c.TmpDir,
		"TMP="+c.TmpDir,
		"TEMP="+c.TmpDir,
	)
	return env
}

// IsLoopbackHost 判断 host 是不是回环地址。
//
// 只认三种写法：127.0.0.0/8 里的 IPv4、::1、以及主机名 localhost。空串按回环处理
// （调用方已经把它替换成默认值）。监听 0.0.0.0 会把一个带 token 的本机 API 暴露到
// 局域网上，所以这里刻意不给「反正有 token」开口子。
func IsLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	// 去掉 IPv6 的方括号（"[::1]" 是 URL 里的写法，参数里也常这么抄）。
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func defaultDataDir() string {
	return defaultDataDirFor(runtime.GOOS)
}

// defaultDataDirFor 按目标系统给出数据目录默认值。
//
// Windows 上放在「文档」下（Tauri 外壳会把 OneDrive 重定向后的真实 Documents
// 目录显式传给 --data-dir，这里只是命令行 / 环境变量都没给时的兜底）；其它系统
// 沿用 XDG 风格，方便在 Linux 上开发与跑测试。
func defaultDataDirFor(goos string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	if goos == "windows" {
		if home == "" {
			return "fpkstudio-data"
		}
		return filepath.Join(home, "Documents", "fpkstudio")
	}
	if x := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); x != "" {
		return filepath.Join(x, "fsfw")
	}
	if home == "" {
		return "fsfw-data"
	}
	return filepath.Join(home, ".local", "share", "fsfw")
}

// samePath 判断两个路径是不是同一个位置（只做清洁后的字面比较，不去解析符号
// 链接：这里判的是「部署时填重了」，不是「文件系统上是不是同一个 inode」）。
func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func cleanAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(a)
}

// listenPort 解析端口配置，空串回落到 0（系统分配）。
func listenPort(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("解析监听端口失败（%s=%q）: %w", EnvListenPort, s, err)
	}
	return n, nil
}

// parsePID 解析父进程 pid，空串或 0 表示「没有父进程要看」。
func parsePID(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("解析父进程 pid 失败（%s=%q）: %w", EnvParentPID, s, err)
	}
	return int(n), nil
}

// atoiDefault 解析一个正整数配置，非法或超范围时回落到默认值。
//
// 以前是手写累加：`FSFW_LOG_RING=99999999999999999999` 会溢出成一个巨大的正数，
// 接着 logging.New 的 `make([]Entry, 0, n)` 直接把进程打死（配置写错不该让服务起不来，
// 更不该以 panic 的形式）。
func atoiDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > maxConfigInt {
		return def
	}
	return n
}

// maxConfigInt 是数字型配置项的上限（防手滑写一个天文数字把内存吃光）。
const maxConfigInt = 1 << 20
