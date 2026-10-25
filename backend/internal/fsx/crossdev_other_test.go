//go:build !windows

package fsx

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

// TestCrossDeviceUnix：POSIX 上跨设备 rename 报 EXDEV，而且被 *os.LinkError 包着。
func TestCrossDeviceUnix(t *testing.T) {
	if !CrossDevice(syscall.EXDEV) {
		t.Error("裸 EXDEV 应当算跨设备")
	}
	wrapped := &os.LinkError{Op: "rename", Old: "/a", New: "/b", Err: syscall.EXDEV}
	if !CrossDevice(wrapped) {
		t.Errorf("%v 应当算跨设备", wrapped)
	}
	if CrossDevice(&os.LinkError{Op: "rename", Old: "/a", New: "/b", Err: syscall.EACCES}) {
		t.Error("EACCES 不该算跨设备：那会把「目标已存在 / 没权限」当成可以合并复制")
	}
	if CrossDevice(nil) {
		t.Error("nil 不该算跨设备")
	}
	if CrossDevice(errors.New("rename /a /b: invalid cross-device link")) {
		t.Error("只认 errno，不认错误文本")
	}
}
