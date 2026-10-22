//go:build windows

package server

import (
	"fmt"
	"os/exec"
)

// openPathInFileManager 用资源管理器打开路径：目录直接打开，文件则选中它。
//
// explorer.exe 即使成功也常返回非 0 退出码，所以只检查「拉起来了没有」，
// 不等它退出（Start 之后立刻 Wait 会拿到 1，那不代表失败）。
func openPathInFileManager(path string, isDir bool) error {
	args := []string{path}
	if !isDir {
		// /select,<path> 是 explorer 的参数格式：逗号后面不能有空格。
		args = []string{"/select," + path}
	}
	cmd := exec.Command("explorer.exe", args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 explorer.exe 失败: %w", err)
	}
	// 回收子进程，避免留下僵尸；退出码不参与判断（见上）。
	go func() { _ = cmd.Wait() }()
	return nil
}
