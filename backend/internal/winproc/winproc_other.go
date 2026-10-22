//go:build !windows

// Package winproc 统一处理 Windows 子进程的控制台窗口。
package winproc

import "os/exec"

// Hide 在非 Windows 上是空操作（没有控制台窗口可藏）。
func Hide(cmd *exec.Cmd) {
	_ = cmd
}
