//go:build windows

// Package winproc 统一处理 Windows 子进程的控制台窗口。
package winproc

import (
	"os/exec"
	"syscall"
)

// createNoWindow 是 CreateProcess 的 CREATE_NO_WINDOW。
const createNoWindow = 0x08000000

// Hide 让 cmd 在 Windows 上不弹控制台窗口。
//
// Tauri 外壳用 CREATE_NO_WINDOW 启动 Go 后端，后端自己再拉起 fnpack.exe / tar.exe
// （都是 console 子系统）时，系统默认会给子进程新建一个控制台——用户每次新建项目、
// 打包、解压都会看到一个黑框一闪而过。这里统一补上 CREATE_NO_WINDOW。
func Hide(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
