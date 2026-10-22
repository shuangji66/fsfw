// Command fsfw-backend 是 FSFW（fpkstudio for Windows）的本地 HTTP API。
//
// 特点：
//   - 只监听回环地址（127.0.0.1），端口默认由系统分配
//   - 由 Tauri 外壳拉起；启动后把端口 / token 写进 runtime 文件供外壳读取
//   - 不再内嵌、也不再转发前端静态资源：界面由 Tauri 自己提供
//   - 带 --parent-pid 时盯着父进程，外壳退出就跟着退出
//
// 配置入口（命令行参数优先于环境变量，完整列表见 docs/porting-contract.md）：
//
//	FSFW_DATA_DIR        数据目录（projects / logs / run / tmp / state.json）
//	FSFW_OUTPUT_DIR      打包输出目录
//	FSFW_FNPACK          fnpack 可执行文件
//	FSFW_LOG_LEVEL       日志级别
//	FSFW_LOG_RING        日志环大小
//	FSFW_HOST            监听地址（只允许回环地址）
//	FSFW_PORT            监听端口
//	FSFW_TOKEN           API 访问令牌
//	FSFW_RUNTIME_FILE    运行时信息文件路径
//	FSFW_PARENT_PID      看门狗盯着的父进程 pid
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fsfw/internal/config"
	"fsfw/internal/fnpack"
	"fsfw/internal/logging"
	"fsfw/internal/projects"
	"fsfw/internal/server"
	"fsfw/internal/state"
)

// version 由构建脚本通过 -ldflags "-X main.version=..." 注入。
var version = "dev"

// watchParentInterval 是父进程存活检查的间隔。
const watchParentInterval = 2 * time.Second

func main() {
	var (
		flagVersion   = flag.Bool("version", false, "打印版本号并退出")
		flagPaths     = flag.Bool("paths", false, "打印解析后的路径配置与监听地址并退出")
		flagHost      = flag.String("host", "", "监听地址（只允许回环地址，默认 127.0.0.1）")
		flagPort      = flag.Int("port", 0, "监听端口（0 = 由系统分配）")
		flagToken     = flag.String("token", "", "API 访问令牌（空 = 关闭鉴权）")
		flagRuntime   = flag.String("runtime-file", "", "运行时信息文件路径（默认 <data>/run/server.json）")
		flagDataDir   = flag.String("data-dir", "", "数据目录（默认 Windows 下为 Documents\\fpkstudio）")
		flagParentPID = flag.Int("parent-pid", 0, "父进程 pid，非 0 时父进程退出后本进程跟着退出")
	)
	flag.Parse()

	if *flagVersion {
		fmt.Printf("fsfw-backend %s\n", version)
		return
	}

	// 只有显式写出来的参数才覆盖环境变量：--port 0 与「没写 --port」是两回事。
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
	opt := config.Options{}
	if set["host"] {
		opt.ListenHost = flagHost
	}
	if set["port"] {
		opt.ListenPort = flagPort
	}
	if set["token"] {
		opt.Token = flagToken
	}
	if set["runtime-file"] {
		opt.RuntimeFile = flagRuntime
	}
	if set["data-dir"] {
		opt.DataDir = flagDataDir
	}
	if set["parent-pid"] {
		opt.ParentPID = flagParentPID
	}

	cfg, err := config.LoadWith(version, opt)
	if err != nil {
		fatal(err)
	}

	if *flagPaths {
		fmt.Printf("数据目录      : %s\n", cfg.DataDir)
		fmt.Printf("项目目录      : %s\n", cfg.ProjectsDir)
		fmt.Printf("输出目录      : %s\n", cfg.OutputDir)
		fmt.Printf("临时目录      : %s\n", cfg.TmpDir)
		fmt.Printf("日志文件      : %s\n", cfg.LogFile())
		fmt.Printf("runtime 文件  : %s\n", cfg.RuntimeFile)
		fmt.Printf("监听地址      : %s\n", cfg.ListenAddr())
		fmt.Printf("鉴权          : %s\n", tokenState(cfg.Token))
		fmt.Printf("fnpack        : %s\n", cfg.FnpackBin)
		fmt.Printf("状态文件      : %s\n", cfg.StateFile)
		if cfg.ParentPID > 0 {
			fmt.Printf("父进程 pid    : %d\n", cfg.ParentPID)
		}
		return
	}

	log, err := logging.New(cfg.LogFile(), cfg.LogLevel, cfg.LogRing)
	if err != nil {
		fatal(fmt.Errorf("初始化日志失败: %w", err))
	}
	defer log.Close()

	fn := fnpack.New(cfg, log)

	prj, err := projects.New(cfg, log, fn)
	if err != nil {
		fatal(err)
	}
	defer prj.Close()

	// 目录设置存在数据目录下的 state.json，启动时应用一次：项目根目录与打包
	// 输出目录都可以被用户指向自己的目录。打不开就沿用默认目录，不能让一个过期
	// 的路径把整个服务拖住起不来。
	st, err := state.Open(cfg.StateFile)
	if err != nil {
		log.Warnf("读取 %s 失败，目录设置按默认处理: %v", cfg.StateFile, err)
		st = state.Empty(cfg.StateFile)
	}
	if dirs := st.Dirs(); !dirs.Empty() {
		if err := prj.SetRoots(dirs.ProjectRoot, dirs.OutputDir); err != nil {
			log.Warnf("应用保存的目录设置失败，沿用默认目录: %v", err)
		} else {
			log.Infof("已应用保存的目录设置：项目根目录 %s，输出目录 %s", prj.ProjectRoot(), prj.OutputDir())
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 父进程看门狗：Tauri 外壳被强杀（升级、任务管理器结束进程）时子进程不会收到
	// 任何信号，只能自己盯着。父进程没了就取消 ctx，走和 Ctrl+C 一样的优雅退出。
	if cfg.ParentPID > 0 {
		log.Infof("看门狗已启动：父进程 pid=%d", cfg.ParentPID)
		go func() {
			if err := watchParent(ctx, cfg.ParentPID); err != nil {
				log.Warnf("%v，正在退出", err)
				stop()
			}
		}()
	}

	srv := server.New(cfg, log, fn, prj, st)
	if err := srv.Run(ctx); err != nil {
		log.Errorf("服务异常退出: %v", err)
		fatal(err)
	}
}

// tokenState 把 token 的有无写成一句人话（--paths 里用，别把 token 本身打出来）。
func tokenState(token string) string {
	if token == "" {
		return "关闭（没有 token，仅供本机浏览器开发）"
	}
	return "已启用"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "fsfw:", err)
	os.Exit(1)
}
