package projects

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"fsfw/internal/fsx"
)

// otherDeviceDir 找一个与 base 不在同一个卷上的可写目录。
//
// 判断方式就是「真去 rename 一次」：成功说明同一卷（挪回来删掉、试下一个候选），
// 失败且被 fsx.CrossDevice 认出来才算跨卷。这样不用在各平台上做 stat_t / 卷序列号
// 的平台分支，Windows 上同样成立（那里跨卷 rename 报 ERROR_NOT_SAME_DEVICE）。
func otherDeviceDir(t *testing.T, base string) string {
	t.Helper()

	cands := []string{"/dev/shm", "/run/shm", "/tmp", "/var/tmp", "/", os.TempDir()}
	if runtime.GOOS == "windows" {
		for c := 'C'; c <= 'Z'; c++ {
			cands = append(cands, string(c)+`:\`)
		}
	}
	for _, cand := range cands {
		if cand == "" {
			continue
		}
		if st, err := os.Stat(cand); err != nil || !st.IsDir() {
			continue
		}
		probe, err := os.MkdirTemp(base, ".xdev-probe-")
		if err != nil {
			t.Fatalf("建探测目录失败: %v", err)
		}
		dst := filepath.Join(cand, filepath.Base(probe))
		renameErr := os.Rename(probe, dst)
		if renameErr == nil {
			// 同一卷：改名成功，挪回去删掉。
			_ = os.Rename(dst, probe)
			_ = os.Remove(probe)
			continue
		}
		_ = os.Remove(probe)
		if !fsx.CrossDevice(renameErr) {
			// 没权限 / 只读之类，换下一个候选。
			continue
		}
		dir, err := os.MkdirTemp(cand, "fsfw-xdev-")
		if err != nil {
			continue
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		return dir
	}
	t.Skip("本机找不到第二个可写卷，跳过跨设备导入")
	return ""
}

// TestImportAcrossDevices 是「导入失败: 移动到项目目录失败: rename ... The system
// cannot move the file to a different disk drive」的回归。
//
// 场景：数据目录（暂存区 tmp/import-*）在 C:，用户在设置里把项目根目录改到了 D:。
// rename 跨卷必然失败，必须退回「复制 + 删源」，而不是把错误直接抛给用户。
func TestImportAcrossDevices(t *testing.T) {
	m, cfg := newTestManager(t)
	prjRoot := otherDeviceDir(t, cfg.DataDir)
	if err := m.SetRoots(prjRoot, ""); err != nil {
		t.Fatalf("切换项目根目录失败: %v", err)
	}

	info, err := m.Import(context.Background(), bytes.NewReader(fakeFPKVersion(t, "1.0.0")), "demo.fpk")
	if err != nil {
		t.Fatalf("跨设备导入失败: %v", err)
	}
	if info.Name != "demo" {
		t.Errorf("项目名 = %q，期望 demo", info.Name)
	}

	// 内容必须真的落到新项目根上（不是只建了个空目录）。
	got, err := os.ReadFile(filepath.Join(prjRoot, "demo", "app", "hello.txt"))
	if err != nil {
		t.Fatalf("项目没落到新项目根: %v", err)
	}
	if string(got) != "hello 1.0.0\n" {
		t.Errorf("项目内容 = %q，期望 hello 1.0.0", got)
	}

	// 暂存区要清干净：兜底复制成功后必须删源，否则每导一个包就在数据目录留一份。
	entries, err := os.ReadDir(filepath.Join(cfg.DataDir, "tmp"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	left := make([]string, 0, len(entries))
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if len(left) != 0 {
		t.Errorf("导入暂存目录没清干净：%s", strings.Join(left, ", "))
	}
}
