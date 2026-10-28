$ErrorActionPreference = 'Stop'
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    go run .
    if ($LASTEXITCODE -ne 0) { throw 'Fyne development process failed.' }
} finally { Pop-Location }