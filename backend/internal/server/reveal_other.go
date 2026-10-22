//go:build !windows

package server

import (
	"fmt"
	"os/exec"
)

// openPathInFileManager 用 xdg-open 打开路径（Linux 开发机上的等价物）。
func openPathInFileManager(path string, _ bool) error {
	cmd := exec.Command("xdg-open", path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 xdg-open 失败（需要装 xdg-utils）: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
