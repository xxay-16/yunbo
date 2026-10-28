param([string]$OutputName = 'yunbo-fyne', [switch]$Console)
$ErrorActionPreference = 'Stop'
if (-not $OutputName.EndsWith('.exe', [StringComparison]::OrdinalIgnoreCase)) { $OutputName += '.exe' }
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) { throw 'Fyne requires MinGW GCC in PATH.' }
    if ($env:CGO_ENABLED -eq '0') { throw 'Fyne requires CGO_ENABLED=1.' }
    New-Item -ItemType Directory -Path 'build/bin' -Force | Out-Null
    $linkFlags = '-s -w'
    if (-not $Console) { $linkFlags += ' -H=windowsgui' }
    go build -trimpath -ldflags $linkFlags -o (Join-Path 'build/bin' $OutputName) .
    if ($LASTEXITCODE -ne 0) { throw 'Fyne build failed.' }
} finally { Pop-Location }