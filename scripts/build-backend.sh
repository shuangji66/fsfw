#!/usr/bin/env bash
# 在非 Windows 机器上构建一份「本机目标」的后端，供 tauri dev / 联调使用。
# Windows 安装包必须用 scripts/build-backend.ps1，产物名带 msvc 三元组。
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="${1:-$(sed -n 's/.*"version": "\([^"]*\)".*/\1/p' "$root/src-tauri/tauri.conf.json" | head -1)}"

os="$(go env GOOS)"
arch="$(go env GOARCH)"
case "$os-$arch" in
  linux-amd64) triple="x86_64-unknown-linux-gnu" ;;
  linux-arm64) triple="aarch64-unknown-linux-gnu" ;;
  darwin-amd64) triple="x86_64-apple-darwin" ;;
  darwin-arm64) triple="aarch64-apple-darwin" ;;
  windows-amd64) triple="x86_64-pc-windows-msvc" ;;
  *) echo "不支持的目标 $os-$arch" >&2; exit 1 ;;
esac

ext=""
[ "$os" = "windows" ] && ext=".exe"

out="$root/src-tauri/binaries/fsfw-backend-$triple$ext"
mkdir -p "$root/src-tauri/binaries"

cd "$root/backend"
CGO_ENABLED=0 GOFLAGS="-buildvcs=false" go build -trimpath \
  -ldflags "-s -w -X main.version=$version" -o "$out" .
echo "==> 后端已生成：$out"
