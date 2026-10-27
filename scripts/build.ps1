param([string]$OutputName = 'yunbo')

$ErrorActionPreference = 'Stop'
if (-not $OutputName.EndsWith('.exe', [StringComparison]::OrdinalIgnoreCase)) { $OutputName += '.exe' }
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    # Assets already exist. Skipping directory discovery also avoids the old
    # x/tools export-data scanner in Wails 2.11 with newer Go toolchains.
    wails build -skipembedcreate -o $OutputName
    if ($LASTEXITCODE -ne 0) { throw 'Wails build failed.' }
} finally {
    Pop-Location
}
