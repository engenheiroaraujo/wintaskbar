#requires -Version 5.1
<#
  Builds tap\tap.cpp into tap\build\tap.dll using a clang/mingw toolchain
  that carries the WinRT and xamlom headers.

  Usage:  powershell -File scripts\build-tap.ps1 [-Out tap\build\tap.dll]
          $env:TWS_CLANG = "C:\path\to\clang++.exe" overrides the compiler.
#>
param(
    [string]$Out = "tap\build\tap.dll"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$candidates = @(
    $env:TWS_CLANG,
    (Join-Path $root ".toolchain\Compiler\bin\clang++.exe")
) | Where-Object { $_ }
$clang = $candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $clang) {
    throw ("clang++ not found. Tried: " + ($candidates -join "; ") +
           ". This project needs an llvm-mingw toolchain " +
           "(MinGW-w64 headers + WinRT and xamlom.h). Restore it to .toolchain\Compiler or set TWS_CLANG.")
}

New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Out) | Out-Null

$args = @(
    "-std=c++23", "-O2", "-shared",
    "-target", "x86_64-w64-mingw32",
    "-DUNICODE", "-D_UNICODE", "-DWINVER=0x0A00", "-D_WIN32_WINNT=0x0A00",
    "-D_WIN32_IE=0x0A00", "-DNTDDI_VERSION=0x0A000008", "-D__USE_MINGW_ANSI_STDIO=0",
    "-Wno-pragma-pack", "-Wno-pragma-system-header-outside-header",
    "-static",
    "tap\tap.cpp",
    "-o", $Out,
    "-lole32", "-loleaut32", "-lruntimeobject"
)

Write-Host "$clang $($args -join ' ')"
& $clang @args
if ($LASTEXITCODE -ne 0) { throw "clang++ failed with exit code $LASTEXITCODE" }
Write-Host "OK -> $Out"
