//go:build !windows

package fsx

// 解压用的外部命令。
//
// 默认写死 /usr/bin：应用以普通用户跑在 Linux 桌面上，服务进程的 PATH 不保证包含
// /usr/bin（从桌面环境 / 应用商店起的进程常常只有很窄的一段），而这些工具通常就在
// 那儿。文件不在（别的发行版可能装在别处）才回落到 PATH 查找，不给它们加环境变量
// ——多一个配置项就多一处「没配好就解压不了」。
var (
	TarBin   = "/usr/bin/tar"
	GzipBin  = "/usr/bin/gzip"
	Bzip2Bin = "/usr/bin/bzip2"
	XzBin    = "/usr/bin/xz"
	ZstdBin  = "/usr/bin/zstd"
	UnzipBin = "/usr/bin/unzip"
)

// zipViaTar 为假：Linux 上有 Info-ZIP 的 unzip，zip 仍然走它（见 unzip）。
const zipViaTar = false

// singleDecompressInGo 为假：.gz/.bz2 仍然交给外部工具。
const singleDecompressInGo = false
