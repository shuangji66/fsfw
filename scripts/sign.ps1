# Authenticode-sign one or more files (exe / msi).
#
#   $env:FSFW_SIGN_CERT_SHA1 = "ABCDEF..."        # cert in CurrentUser\My
#   powershell -File scripts/sign.ps1 -Path dist\app.exe,dist\app.msi
#
# or with a PFX:
#   $env:FSFW_SIGN_PFX = "C:\keys\codesign.pfx"
#   $env:FSFW_SIGN_PFX_PASSWORD = "..."
#
# With no certificate configured it prints a skip message and exits 0. Build scripts
# call it only when one of the variables is set.
param(
    [Parameter(Mandatory = $true)]
    [string[]]$Path,
    [string]$Thumbprint = $env:FSFW_SIGN_CERT_SHA1,
    [string]$PfxPath = $env:FSFW_SIGN_PFX,
    [string]$PfxPassword = $env:FSFW_SIGN_PFX_PASSWORD,
    [string]$TimestampUrl = "http://timestamp.digicert.com"
)

$ErrorActionPreference = "Stop"

if (-not $Thumbprint -and -not $PfxPath) {
    Write-Host "==> signing skipped: FSFW_SIGN_CERT_SHA1 / FSFW_SIGN_PFX is not set"
    return
}

function Find-SignTool {
    $candidates = @()
    $onPath = Get-Command "signtool.exe" -ErrorAction SilentlyContinue
    if ($onPath) { $candidates += $onPath.Source }

    $kitsRoot = Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"
    if (Test-Path $kitsRoot) {
        $candidates += Get-ChildItem -Path $kitsRoot -Recurse -Filter "signtool.exe" -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -match "\\x64\\signtool\.exe$" } |
            Sort-Object FullName -Descending |
            ForEach-Object { $_.FullName }
    }

    $found = $candidates | Select-Object -First 1
    if (-not $found) { throw "signtool.exe not found (install the Windows SDK)" }
    return $found
}

$signtool = Find-SignTool
$files = @(Get-Item -Path $Path -ErrorAction Stop)
if ($files.Count -eq 0) { throw "no files to sign" }

foreach ($file in $files) {
    if (-not $file.PSIsContainer -and $file.Exists) {
        Write-Host "==> signing $($file.FullName)"
        $arguments = @("sign", "/fd", "SHA256", "/tr", $TimestampUrl, "/td", "SHA256")
        if ($PfxPath) {
            $arguments += @("/f", (Resolve-Path $PfxPath).Path)
            if ($PfxPassword) { $arguments += @("/p", $PfxPassword) }
        }
        else {
            $arguments += @("/sha1", $Thumbprint)
        }
        $arguments += $file.FullName

        & $signtool @arguments
        if ($LASTEXITCODE -ne 0) { throw "signing failed: $($file.FullName) (exit code $LASTEXITCODE)" }

        & $signtool verify /pa /v $file.FullName
        if ($LASTEXITCODE -ne 0) { throw "signature verification failed: $($file.FullName)" }
    }
}

Write-Host "==> signing done"
