#requires -Version 5.1
# Apaga só produtos conhecidos; preserva caches, toolchains e outros arquivos.
# Binários em uso causam uma falha visível em vez de serem ignorados em silêncio.
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))
$prefix = $root.TrimEnd('\') + '\'
$products = @(
    'bin\release\wintaskbar.exe',
    'bin\release\tap.dll',
    'bin\release\README.md',
    'bin\release\LICENSE',
    'bin\wintaskbar-cli.exe',
    'bin\wintaskbar-setup.exe',
    'bin\wintaskbar-portable.zip',
    'bin\tests\theme_file_test.exe',
    'bin\check\release\wintaskbar.exe',
    'bin\check\release\tap.dll',
    'bin\check\release\README.md',
    'bin\check\release\LICENSE',
    'bin\check\wintaskbar-cli.exe',
    'bin\check\wintaskbar-setup.exe',
    'bin\check\wintaskbar-portable.zip',
    'tap\build\tap.dll',
    'cmd\setup\payload\wintaskbar.exe',
    'cmd\setup\payload\tap.dll'
)
foreach ($product in $products) {
    $target = [IO.Path]::GetFullPath((Join-Path $root $product))
    if (-not $target.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Build product is outside the project: $target"
    }
    if (Test-Path -LiteralPath $target) {
        Remove-Item -LiteralPath $target -Force
        Write-Host "Removed $product"
    }
}
