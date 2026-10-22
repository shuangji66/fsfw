//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"
)

// watchParent 盯着 pid，进程消失时返回错误；ctx 结束时返回 nil。
//
// 非 Windows 上没有可等待的进程句柄，用 signal 0 轮询：它只做存在性 / 权限检查，
// 不真的发信号。ESRCH 表示进程已经没了；EPERM 说明进程还在、只是不归我们管，
// 不能算退出。
func watchParent(ctx context.Context, pid int) error {
	ticker := time.NewTicker(watchParentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			err := syscall.Kill(pid, 0)
			switch {
			case err == nil, errors.Is(err, syscall.EPERM):
				continue
			case errors.Is(err, syscall.ESRCH):
				return fmt.Errorf("父进程 %d 已退出", pid)
			default:
				return fmt.Errorf("检查父进程 %d 失败: %w", pid, err)
			}
		}
	}
}
