//go:build !windows

package fsx

import (
	"errors"
	"syscall"
)

// CrossDevice 报告 err 是不是「rename 跨了文件系统边界」（POSIX 上就是 EXDEV）。
func CrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
