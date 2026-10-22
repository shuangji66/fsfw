//go:build windows

package main

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

// watchParent 等父进程退出：拿一个 SYNCHRONIZE 句柄挂在 WaitForSingleObject 上。
//
// 比 signal 0 轮询可靠：句柄绑定的是那个进程对象，父进程退出后即使 pid 被复用也
// 不会被骗过去。等待带上 2 秒超时，只是为了能及时响应 ctx 取消。
func watchParent(ctx context.Context, pid int) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("打开父进程 %d 失败: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	interval := uint32(watchParentInterval / time.Millisecond)
	for {
		ev, err := windows.WaitForSingleObject(h, interval)
		if err != nil {
			return fmt.Errorf("等待父进程 %d 失败: %w", pid, err)
		}
		switch ev {
		case windows.WAIT_OBJECT_0:
			return fmt.Errorf("父进程 %d 已退出", pid)
		case uint32(windows.WAIT_TIMEOUT):
			if ctx.Err() != nil {
				return nil
			}
		default:
			return fmt.Errorf("等待父进程 %d 返回未知状态 %d", pid, ev)
		}
	}
}
