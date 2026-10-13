#requires -Version 5.1
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    $candidates = @($env:TWS_CLANG, "$root\.toolchain\Compiler\bin\clang++.exe") | Where-Object { $_ }
    $clang = $candidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
    if (-not $clang) { throw 'clang++ not found; set TWS_CLANG.' }
    New-Item -ItemType Directory -Force -Path 'bin\tests' | Out-Null
    & $clang -std=c++23 -static -target x86_64-w64-mingw32 tap\theme_file_test.cpp -o bin\tests\theme_file_test.exe
    if ($LASTEXITCODE -ne 0) { throw 'TAP test build failed.' }
    & .\bin\tests\theme_file_test.exe internal\modules\theme\repository\testdata\default.compiled
    if ($LASTEXITCODE -ne 0) { throw 'TAP contract tests failed.' }
} finally { Pop-Location }
