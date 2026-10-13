#requires -Version 5.1
param([switch]$SkipTap, [string]$OutDir = 'bin')
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$previousOS, $previousArch = $env:GOOS, $env:GOARCH
$env:GOOS, $env:GOARCH = 'windows', 'amd64'
Push-Location $root
try {
    $releaseDir = Join-Path $OutDir 'release'
    $tap = Join-Path $releaseDir 'tap.dll'
    $agent = Join-Path $releaseDir 'wintaskbar.exe'
    $installer = Join-Path $OutDir 'wintaskbar-setup.exe'
    $cli = Join-Path $OutDir 'wintaskbar-cli.exe'
    $archive = Join-Path $OutDir 'wintaskbar-portable.zip'
    New-Item -ItemType Directory -Force -Path $releaseDir,'cmd\setup\payload' | Out-Null
    if (-not $SkipTap) { & "$PSScriptRoot\build-tap.ps1" -Out $tap }
    if (-not (Test-Path -LiteralPath $tap)) { throw 'Build the TAP first.' }
    go build -trimpath -ldflags '-H=windowsgui -s -w' -o $agent .\cmd\wintaskbar
    if ($LASTEXITCODE -ne 0) { throw 'Agent build failed.' }
    Copy-Item -LiteralPath $agent,$tap -Destination 'cmd\setup\payload'
    go build -trimpath -ldflags '-H=windowsgui -s -w' -o $installer .\cmd\setup
    if ($LASTEXITCODE -ne 0) { throw 'Installer build failed.' }
    # Um build com console continua útil para desenvolvedores e para verificações automatizadas.
    go build -trimpath -o $cli .\cmd\wintaskbar
    if ($LASTEXITCODE -ne 0) { throw 'CLI build failed.' }
    Copy-Item -LiteralPath 'README.md','LICENSE','THIRD_PARTY_NOTICES.md' -Destination $releaseDir
    Compress-Archive -Path (Join-Path $releaseDir '*') -DestinationPath $archive -Force
    Write-Host "Ready: $installer and $archive"
} finally {
    $env:GOOS, $env:GOARCH = $previousOS, $previousArch
    Pop-Location
}
