package fsx

import (
	"os"
	"path/filepath"
)

// Windows 自带 bsdtar（%SystemRoot%\System32\tar.exe）：tar 与 zip 都交给它，
// 而 unzip / xz / zstd / gzip / bzip2 系统里通常没有。
//
// 单个文件的 .gz / .bz2 因此改用 Go 标准库（见 decompressStdlib）；.xz / .zst
// 没有标准库实现，只能报「当前系统没有这个解压工具」。
var (
	TarBin   = system32("tar.exe")
	UnzipBin = TarBin
	GzipBin  = ""
	Bzip2Bin = ""
	XzBin    = ""
	ZstdBin  = ""
)

const (
	// zipViaTar 为真：用 bsdtar 解 zip，不依赖 unzip。
	zipViaTar = true
	// singleDecompressInGo 为真：.gz/.bz2 用标准库解，不依赖外部命令。
	singleDecompressInGo = true
)

// system32 拼出 %SystemRoot%\System32 下的系统工具路径。
//
// 不用 exec.LookPath 直接找 "tar.exe"：这个进程的 PATH 来自 Tauri 外壳，可能被
// 精简过；而 System32 一定在。SystemRoot 理论上总有值，没有时按最常见的安装位置兜底。
func system32(name string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", name)
}
