//go:build windows

package fsx

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// TestCrossDeviceWindows 是「数据目录在 C:、项目根在 D: 时导入必失败」的回归。
//
// Go 在 Windows 上的 syscall.EXDEV 是 APPLICATION_ERROR(1<<29) 起的自造值（不是 POSIX
// 的 18），而跨卷 rename 实际报的是 Win32 的 ERROR_NOT_SAME_DEVICE(17)：只比 EXDEV
// 永远不成立，复制兜底就成了死代码。
func TestCrossDeviceWindows(t *testing.T) {
	crossVolume := &os.LinkError{
		Op:  "rename",
		Old: `C:\Users\a\Documents\fpkstudio\tmp\import-1`,
		New: `D:\projects\demo`,
		Err: windows.ERROR_NOT_SAME_DEVICE,
	}
	if !CrossDevice(crossVolume) {
		t.Errorf("%v 应当算跨设备", crossVolume)
	}

	// 这两个都不能算跨设备：目标已存在 / 没权限时退回复制会把另一个项目覆盖掉。
	for _, err := range []error{windows.ERROR_ACCESS_DENIED, windows.ERROR_ALREADY_EXISTS, windows.ERROR_FILE_NOT_FOUND} {
		if CrossDevice(&os.LinkError{Op: "rename", Err: err}) {
			t.Errorf("%v 不该算跨设备", err)
		}
	}
}
