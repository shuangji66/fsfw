package fnpack

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"fsfw/internal/config"
	"fsfw/internal/logging"
)

// newShellClient 造一个用本机 sh 当 fnpack 的客户端。这些测试依赖 POSIX shell，
// 在 Windows 上直接跳过（项目里的 Go 测试本来也只在 WSL / Linux 上跑）。
func newShellClient(t *testing.T) *Client {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("依赖 /bin/sh，Windows 上跳过")
	}
	dir := t.TempDir()
	cfg := &config.Config{
		TmpDir:    dir,
		FnpackBin: "/bin/sh",
		LogLevel:  "error",
	}
	log, err := logging.New("", "error", 100)
	if err != nil {
		t.Fatalf("创建日志器失败: %v", err)
	}
	t.Cleanup(func() { log.Close() })
	return New(cfg, log)
}

// 进程立刻退出时也不能丢掉最后一段输出。
//
// 以前用 StdoutPipe + 先 Wait 再等读协程：Wait 会关掉父侧读端，管道里还没读走的
// 行就没了。N 行输出 + 立即退出正是最容易命中的形态。
func TestRunKeepsTailWhenProcessExitsImmediately(t *testing.T) {
	c := newShellClient(t)
	const lines = 5000
	script := fmt.Sprintf(`i=0; while [ $i -lt %d ]; do echo "line-$i"; i=$((i+1)); done; echo "last-line-without-newline"`, lines)

	out, err := c.Run(context.Background(), t.TempDir(), "test", "-c", script)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	got := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(got) != lines+1 {
		t.Fatalf("输出行数 = %d，期望 %d（尾部：%q）", len(got), lines+1, tailOf(out))
	}
	if got[lines-1] != fmt.Sprintf("line-%d", lines-1) {
		t.Errorf("第 %d 行 = %q", lines, got[lines-1])
	}
	if got[lines] != "last-line-without-newline" {
		t.Errorf("没有换行的最后一行没被带出来：%q", got[lines])
	}
}

// 孙进程继承了输出句柄时，Run 不能跟着一起等。
//
// 这是当初选「先 Wait」的唯一理由：`(sleep 20) &` 会把写端一直开着，先等读协程
// 的写法会挂到孙进程退出。现在改成文件之后，Wait 只等直接子进程，读协程也不依赖
// 写端关闭，两边都不该被拖住。
func TestRunDoesNotWaitForGrandchild(t *testing.T) {
	c := newShellClient(t)
	script := `echo before-background; (sleep 10) & echo after-background; exit 0`

	start := time.Now()
	out, err := c.Run(context.Background(), t.TempDir(), "test", "-c", script)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Run 被孙进程拖住了：耗时 %v", elapsed)
	}
	for _, want := range []string{"before-background", "after-background"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出里缺少 %q：%q", want, out)
		}
	}
}

// 临时输出文件不能留在数据目录里：每次打包 / 建项目都会生成一个。
func TestRunRemovesOutputFile(t *testing.T) {
	c := newShellClient(t)
	dir := c.cfg.TmpDir
	if _, err := c.Run(context.Background(), t.TempDir(), "test", "-c", "echo hi"); err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取临时目录失败: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "fnpack-") {
			t.Errorf("临时输出文件没有清理：%s", e.Name())
		}
	}
}

func tailOf(s string) string {
	if len(s) <= 200 {
		return s
	}
	return "…" + s[len(s)-200:]
}
