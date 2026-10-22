# vendor

随安装包分发的第三方可执行文件。

## fnpack.exe

- 来源：飞牛官方 `fnpack` 命令行工具（Windows amd64 构建）
- 版本：1.2.3
- 架构：PE32+ x86-64（`MZ` / `machine=0x8664`）
- SHA-256：`d7af4bd716b009c58f5bcd931615f39db121e7d4b75dc759e575c4fb2879b6ee`

它不会进 Git LFS，也不会被压缩包二次打包：`scripts/build-backend.ps1` 把它复制成
`src-tauri/binaries/fnpack-x86_64-pc-windows-msvc.exe`，Tauri 的 `externalBin` 会把它
和 `fsfw.exe` 一起放进安装目录。运行时 Tauri 用 `FSFW_FNPACK` 把同目录的
`fnpack.exe` 指给 Go 后端；用户自己设置了 `FSFW_FNPACK` 时优先用用户的。

升级这个文件时：

1. 替换 `vendor/fnpack.exe`；
2. 更新上面的版本与 SHA-256；
3. 重新跑 `npm run build`，并在 Windows 上验证「新建 / 打包」两条链路。
