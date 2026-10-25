//go:build windows

package fsx

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// CrossDevice 报告 err 是不是「rename 跨了文件系统边界」。
//
// Windows 上只比 syscall.EXDEV 一定不够：Go 的 Windows syscall 没有对应的 POSIX
// errno，EXDEV 是从 APPLICATION_ERROR(1<<29) 起自造的占位值，而 MoveFileEx 跨卷
// 失败返回的是 Win32 的 ERROR_NOT_SAME_DEVICE(17)，两者永远对不上。少了这一条，
// 「数据目录在 C:、项目根在 D:」时导入会直接失败，根本走不到复制兜底
// （见 projects.moveIntoProjects）。
func CrossDevice(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) || errors.Is(err, syscall.EXDEV)
}
