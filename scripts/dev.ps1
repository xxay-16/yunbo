$ErrorActionPreference = 'Stop'
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    wails dev -skipembedcreate
    if ($LASTEXITCODE -ne 0) { throw 'Wails development process failed.' }
} finally {
    Pop-Location
}
