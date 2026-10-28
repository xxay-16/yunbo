$ErrorActionPreference = 'Stop'
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    if (-not (Get-Command makensis -ErrorAction SilentlyContinue)) {
        throw 'NSIS is required: install NSIS and add its directory to PATH.'
    }
    $version = (Get-Content -LiteralPath 'wails.json' -Raw | ConvertFrom-Json).info.productVersion
    if ($version -notmatch '^\d+\.\d+\.\d+$') { throw 'Invalid productVersion in wails.json.' }
    $stem = "yunbo-$version-windows-amd64"
    wails build -tags wails -skipembedcreate -trimpath -platform windows/amd64 -o yunbo.exe -nsis
    if ($LASTEXITCODE -ne 0) { throw 'Wails Windows installer build failed.' }
    $installer = 'build\bin\yunbo-amd64-installer.exe'
    if (-not (Test-Path -LiteralPath $installer -PathType Leaf)) { throw 'NSIS did not generate an installer.' }
    New-Item -ItemType Directory -Path 'dist' -Force | Out-Null
    Copy-Item -LiteralPath $installer -Destination "dist\$stem-setup.exe"
    Compress-Archive -LiteralPath 'build\bin\yunbo.exe' -DestinationPath "dist\$stem-portable.zip" -Force
    Get-ChildItem -LiteralPath 'dist' -File | Select-Object Name,Length
} finally {
    Pop-Location
}
