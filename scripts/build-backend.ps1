# 构建 Go 后端，并按 Tauri 的 externalBin 约定命名。
#
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-backend.ps1
#
# Tauri 要求 externalBin 的文件名带目标三元组后缀，打包时会去掉后缀，安装目录里
# 得到的是 fsfw-backend.exe。--port 0 / --token 等参数由 src-tauri/src/backend.rs 传入。
param(
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$configPath = Join-Path $root "src-tauri\tauri.conf.json"
if (-not $Version) {
    $Version = (Get-Content $configPath -Raw | ConvertFrom-Json).version
}

$target = "x86_64-pc-windows-msvc"
$binDir = Join-Path $root "src-tauri\binaries"
$out = Join-Path $binDir "fsfw-backend-$target.exe"

New-Item -ItemType Directory -Force -Path $binDir | Out-Null

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
# 仓库可能是 git worktree / 导出目录，VCS 信息不可读时不该让构建失败。
$env:GOFLAGS = "-buildvcs=false"

Push-Location (Join-Path $root "backend")
try {
    Write-Host "==> go build $target (version $Version)"
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $out .
    if ($LASTEXITCODE -ne 0) { throw "go build 失败（退出码 $LASTEXITCODE）" }
}
finally {
    Pop-Location
}

if (-not (Test-Path $out)) { throw "没有生成 $out" }
$size = [math]::Round((Get-Item $out).Length / 1MB, 1)
Write-Host "==> 后端已生成：$out ($size MB)"
