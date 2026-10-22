// Package fnpack 包装飞牛官方的 fnpack 命令行工具。
//
// 只做三件事：定位、执行、把输出逐行转给日志器。
//
// 成败判断以退出码为准，`build` 是这样的。`create` 不是：项目名非法时（太短、
// 含非法字符）它照样以退出码 0 结束，只把错误打在 stdout 上。所以调用 create
// 的一方除了退出码，还得回读输出——见 FailureReason。
package fnpack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"fsfw/internal/config"
	"fsfw/internal/logging"
	"fsfw/internal/winproc"
)

var versionRe = regexp.MustCompile(`(?i)version\s+v?([0-9][0-9A-Za-z.\-]*)`)

// create 失败时的输出形如：
//
//	Create application name ab error : appname ilegal,app name (ab) length must be between 3 and 32
var failureRe = regexp.MustCompile(`(?i)\berror\b\s*:`)

// FailureReason 从命令输出里取出 fnpack 自己给出的失败原因，没有则返回空串。
//
// 存在的原因只有一个：fnpack create 失败时退出码还是 0，判断成败只能看输出。
func FailureReason(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" && failureRe.MatchString(line) {
			return line
		}
	}
	return ""
}

// Client 是 fnpack 的调用入口。
type Client struct {
	cfg *config.Config
	log *logging.Logger

	mu      sync.Mutex
	binPath string
	version string
	// probeMessage 是探测失败的原因（成功时为空）。probed 单独一个标志：
	// 以前用「消息非空」当「探测过了」的判据，结果是**失败被永久缓存、成功每次都重探**
	// （成功时消息为空）——每次打开界面都白起一个 fnpack 进程。
	probeMessage string
	probeAt      time.Time
	probed       bool
}

// probeFailureTTL 是「探测失败」的缓存时长。
//
// 成功可以一直缓存（fnpack 不会自己换版本），失败不行：用户装好 fnpack 之后应当自己
// 恢复，而不是必须重启应用。也不能不缓存——那样每次 /api/system/info 都会起一个进程。
var probeFailureTTL = 30 * time.Second

// New 创建客户端。
func New(cfg *config.Config, log *logging.Logger) *Client {
	return &Client{cfg: cfg, log: log}
}

// Bin 返回实际可执行文件路径（探测失败时返回原始配置值）。
func (c *Client) Bin() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.binPath != "" {
		return c.binPath
	}
	return c.cfg.FnpackBin
}

// Probe 探测 fnpack 是否可用并取版本号。结果会被缓存。
func (c *Client) Probe(ctx context.Context) (bin, version, message string) {
	c.mu.Lock()
	if c.probed && (c.probeMessage == "" || time.Since(c.probeAt) < probeFailureTTL) {
		bin, version, message = c.binPath, c.version, c.probeMessage
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	bin = c.cfg.FnpackBin
	resolved, err := exec.LookPath(bin)
	if err != nil {
		message = fmt.Sprintf("找不到 fnpack（%s）：%v", bin, err)
	} else {
		bin = resolved
		// fnpack 没有 --version，直接跑会打印用法，版本号在用法里。
		probe := exec.CommandContext(ctx, bin)
		winproc.Hide(probe)
		out, runErr := probe.CombinedOutput()
		if v := versionRe.FindStringSubmatch(string(out)); len(v) == 2 {
			version = v[1]
		}
		if runErr != nil && version == "" {
			message = fmt.Sprintf("执行 fnpack 失败：%v", runErr)
		} else if version == "" {
			message = "fnpack 已找到，但未能解析出版本号"
		}
	}

	c.mu.Lock()
	c.binPath, c.version, c.probeMessage, c.probed, c.probeAt = bin, version, message, true, time.Now()
	c.mu.Unlock()
	return bin, version, message
}

// Run 在 dir 下执行 fnpack 子命令，输出逐行转给日志器。
//
// 输出先落到临时文件，再由一个读协程边读边转给日志器，原因见下面注释：用管道
// 时「先等读协程」会被继承了管道的孙进程挂死，「先等进程」又会丢掉管道里没读走
// 的尾巴。文件没有这两个问题——子进程直接写文件句柄，进程退出后内容一定完整。
func (c *Client) Run(ctx context.Context, dir string, source string, args ...string) (string, error) {
	bin := c.Bin()

	cmd := exec.CommandContext(ctx, bin, args...)
	winproc.Hide(cmd)
	cmd.Dir = dir
	// 让 fnpack 的临时目录落在数据目录内，避免跨挂载点 / 受限 /tmp。
	cmd.Env = c.cfg.Env()

	// TmpDir 正常由 config 建好；这里补一次 mkdir，免得用户在运行期把 tmp 目录
	// 删掉之后连「新建项目」都做不了（CreateTemp 不会自己建父目录）。
	if c.cfg.TmpDir != "" {
		if err := os.MkdirAll(c.cfg.TmpDir, 0o755); err != nil {
			return "", fmt.Errorf("创建临时目录失败: %w", err)
		}
	}
	outFile, err := os.CreateTemp(c.cfg.TmpDir, "fnpack-*.log")
	if err != nil {
		return "", fmt.Errorf("创建 fnpack 输出临时文件失败: %w", err)
	}
	outPath := outFile.Name()
	defer func() {
		_ = outFile.Close()
		_ = os.Remove(outPath)
	}()
	cmd.Stdout = outFile
	cmd.Stderr = outFile

	c.log.Line(source, logging.LevelInfo, fmt.Sprintf("$ fnpack %s", strings.Join(args, " ")))
	c.log.Line(source, logging.LevelDebug, fmt.Sprintf("  工作目录: %s", dir))

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动 fnpack 失败: %w", err)
	}

	var (
		sb   strings.Builder
		done = make(chan struct{})
		stop = make(chan struct{})
	)
	go func() {
		defer close(done)
		reader, err := os.Open(outPath)
		if err != nil {
			c.log.Line(source, logging.LevelWarn, fmt.Sprintf("读取 fnpack 输出失败: %v", err))
			return
		}
		defer reader.Close()

		var pending []byte
		emit := func(line string) {
			sb.WriteString(line)
			sb.WriteByte('\n')
			c.log.Line(source, logging.LevelInfo, line)
		}
		consume := func(chunk []byte) {
			pending = append(pending, chunk...)
			for {
				i := bytes.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				emit(strings.TrimSuffix(string(pending[:i]), "\r"))
				pending = pending[i+1:]
			}
			// 超长行不能无限攒着（以前的 Scanner 也有 1MB 上限）。
			if len(pending) > maxOutputLine {
				emit(string(pending[:maxOutputLine]))
				pending = pending[maxOutputLine:]
			}
		}

		buf := make([]byte, 32<<10)
		for {
			n, readErr := reader.Read(buf)
			if n > 0 {
				consume(buf[:n])
			}
			if readErr == nil {
				continue
			}
			if !errors.Is(readErr, io.EOF) {
				c.log.Line(source, logging.LevelWarn, fmt.Sprintf("读取 fnpack 输出中断: %v", readErr))
				return
			}
			// 读到当前末尾：进程还在跑就过一会儿再看有没有新内容。
			select {
			case <-stop:
				// 进程已经退出：把剩余内容读完，最后一行没有换行也要吐出来。
				if rest, err := io.ReadAll(reader); err == nil {
					consume(rest)
				}
				if len(pending) > 0 {
					emit(strings.TrimSuffix(string(pending), "\r"))
				}
				return
			case <-time.After(outputPollInterval):
			}
		}
	}()

	// 只等进程退出，不等读协程：孙进程可能继承了输出文件句柄继续写（`(sleep 20) &`
	// 这种），等它等于把打包请求挂死。读协程收到 stop 后会做最后一次全量读取，
	// 所以进程退出时的输出不会丢。
	waitErr := cmd.Wait()
	close(stop)
	<-done

	output := sb.String()
	if waitErr != nil {
		c.log.Line(source, logging.LevelError, fmt.Sprintf("fnpack 退出异常: %v", waitErr))
	}
	return output, waitErr
}

const (
	// maxOutputLine 是单行日志的上限，超出按行截断，避免畸形输出吃掉内存。
	maxOutputLine = 1 << 20
	// outputPollInterval 是在输出文件末尾等新内容时的轮询间隔。
	outputPollInterval = 20 * time.Millisecond
)

// Create 创建项目骨架。dir 是父目录，name 是项目名。
//
// 参数校验由 fnpack 负责：项目名必须是 3–32 位、以字母或数字开头结尾、中间可含
// . _ -。校验不过时它不会创建任何东西，也不会改退出码，只打印一行错误。
func (c *Client) Create(ctx context.Context, dir, name string, docker, withUI bool) (string, error) {
	args := []string{"create", name}
	if docker {
		args = append(args, "--template", "docker")
	}
	if !withUI {
		args = append(args, "--without-ui")
	}
	return c.Run(ctx, dir, logging.SourceFnpack, args...)
}

// Build 在项目目录内打包。
func (c *Client) Build(ctx context.Context, projectDir string) (string, error) {
	return c.Run(ctx, projectDir, logging.SourceBuild, "build")
}
