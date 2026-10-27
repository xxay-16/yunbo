# Exercise the real installer only on a disposable CI runner.
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true') { throw 'Installer verification requires a GitHub Actions runner.' }
$version = (Get-Content -LiteralPath 'wails.json' -Raw | ConvertFrom-Json).info.productVersion
$installer = (Resolve-Path -LiteralPath "dist\yunbo-$version-windows-amd64-setup.exe").Path
$installRoot = Join-Path $env:RUNNER_TEMP ('Yunbo Install Test ' + [guid]::NewGuid().ToString('N'))
$process = Start-Process -FilePath $installer -ArgumentList @('/S', "/D=$installRoot") -WindowStyle Hidden -Wait -PassThru
if ($process.ExitCode -ne 0) { throw "Installer exited with $($process.ExitCode)." }
$installedApp = Join-Path $installRoot 'yunbo.exe'
if (-not (Test-Path -LiteralPath $installedApp)) { throw 'Installer did not install yunbo.exe.' }
if ((Get-FileHash -LiteralPath $installedApp).Hash -ne (Get-FileHash -LiteralPath 'build\bin\yunbo.exe').Hash) {
    throw 'Installed executable differs from the build.'
}
$sentinel = Join-Path $installRoot 'keep-user-file.txt'
[IO.File]::WriteAllText($sentinel, 'Keep files not owned by Yunbo.')
$uninstaller = Join-Path $installRoot 'uninstall.exe'
$process = Start-Process -FilePath $uninstaller -ArgumentList '/S' -WindowStyle Hidden -Wait -PassThru
if ($process.ExitCode -ne 0) { throw "Uninstaller exited with $($process.ExitCode)." }
if (Test-Path -LiteralPath $installedApp) { throw 'Uninstaller left the application executable.' }
if (-not (Test-Path -LiteralPath $sentinel)) { throw 'Uninstaller deleted an unrelated file.' }
if (Test-Path -LiteralPath 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Yunbo') {
    throw 'Uninstaller left its registration.'
}
Write-Output 'Windows install/uninstall passed; unrelated files were retained.'
