# Build the Go backend and name it the way Tauri's externalBin wants it.
#
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-backend.ps1
#
# Tauri requires the target-triple suffix; the bundler strips it when packaging, so
# the installed file is fsfw-backend.exe. Runtime flags (--port 0, --token, ...) are
# passed by src-tauri/src/backend.rs.
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
# A git worktree / exported source tree may not have readable VCS metadata.
$env:GOFLAGS = "-buildvcs=false"

Push-Location (Join-Path $root "backend")
try {
    Write-Host "==> go build $target (version $Version)"
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $out .
    if ($LASTEXITCODE -ne 0) { throw "go build failed (exit code $LASTEXITCODE)" }
}
finally {
    Pop-Location
}

if (-not (Test-Path $out)) { throw "missing build output: $out" }
$size = [math]::Round((Get-Item $out).Length / 1MB, 1)
Write-Host "==> backend: $out ($size MB)"
