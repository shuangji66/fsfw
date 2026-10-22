# Build the Go backend and stage the Tauri external binaries.
#
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-backend.ps1
#
# Only the amd64 target is supported. The two binaries staged here are bundled next to
# fsfw.exe:
#   binaries/fsfw-backend-x86_64-pc-windows-msvc.exe  -> fsfw-backend.exe
#   binaries/fnpack-x86_64-pc-windows-msvc.exe        -> fnpack.exe
# Tauri strips the target-triple suffix when bundling.
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
$backendOut = Join-Path $binDir "fsfw-backend-$target.exe"
$fnpackSrc = Join-Path $root "vendor\fnpack.exe"
$fnpackOut = Join-Path $binDir "fnpack-$target.exe"

New-Item -ItemType Directory -Force -Path $binDir | Out-Null

# Fail early with a readable message instead of letting the bundler reject the file.
function Assert-PeAmd64([string]$Path, [string]$Label) {
    if (-not (Test-Path $Path)) { throw "$Label not found: $Path" }
    $stream = [IO.File]::OpenRead($Path)
    try {
        $reader = New-Object IO.BinaryReader($stream)
        if ($reader.ReadUInt16() -ne 0x5A4D) { throw "$Label is not a PE executable: $Path" }
        $stream.Position = 0x3C
        $peOffset = $reader.ReadInt32()
        $stream.Position = $peOffset
        if ($reader.ReadUInt32() -ne 0x00004550) { throw "$Label has no PE header: $Path" }
        $machine = $reader.ReadUInt16()
        if ($machine -ne 0x8664) {
            throw ("{0} is not an amd64 binary (PE machine 0x{1:X4}): {2}" -f $Label, $machine, $Path)
        }
    }
    finally {
        $stream.Dispose()
    }
}

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
# A git worktree / exported source tree may not have readable VCS metadata.
$env:GOFLAGS = "-buildvcs=false"

Push-Location (Join-Path $root "backend")
try {
    Write-Host "==> go build $target (version $Version)"
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $backendOut .
    if ($LASTEXITCODE -ne 0) { throw "go build failed (exit code $LASTEXITCODE)" }
}
finally {
    Pop-Location
}
Assert-PeAmd64 $backendOut "backend"

Write-Host "==> staging vendor fnpack.exe"
Copy-Item -Force $fnpackSrc $fnpackOut
Assert-PeAmd64 $fnpackOut "fnpack"

$backendSize = [math]::Round((Get-Item $backendOut).Length / 1MB, 1)
$fnpackSize = [math]::Round((Get-Item $fnpackOut).Length / 1MB, 1)
Write-Host "==> backend: $backendOut ($backendSize MB)"
Write-Host "==> fnpack : $fnpackOut ($fnpackSize MB)"
