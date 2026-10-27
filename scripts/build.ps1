$ErrorActionPreference = 'Stop'
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    # Assets already exist. Skipping directory discovery also avoids the old
    # x/tools export-data scanner in Wails 2.11 with newer Go toolchains.
    wails build -clean -skipembedcreate
    if ($LASTEXITCODE -ne 0) { throw 'Wails build failed.' }
} finally {
    Pop-Location
}
