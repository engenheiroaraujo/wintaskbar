#requires -Version 5.1
# Gera assets\wintaskbar.ico (a roseta de vidro, desenhada aqui e igual a assets\icon.svg) e compila o
# recurso para os .syso de cmd\wintaskbar e cmd\setup. Precisa do llvm-windres.
param([string]$Windres = 'llvm-windres')
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
$root = Split-Path -Parent $PSScriptRoot
$assets = Join-Path $root 'assets'

function New-Frame([int]$size) {
    $bmp = New-Object Drawing.Bitmap $size, $size, ([Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $g = [Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = 'AntiAlias'
    $g.PixelOffsetMode = 'HighQuality'
    $g.Clear([Drawing.Color]::Transparent)
    $s = $size / 112.0
    $g.ScaleTransform($s, $s)
    # Borda de pelo menos ~0,8 px reais, para as pétalas não sumirem nos tamanhos pequenos.
    $edgeWidth = [Math]::Max(2.0, 0.8 / $s)

    # Oito pétalas de vidro (azul e violeta alternados) girando em torno do centro.
    $sky = New-Object Drawing.SolidBrush ([Drawing.Color]::FromArgb(140, 0x38, 0xBD, 0xF8))
    $violet = New-Object Drawing.SolidBrush ([Drawing.Color]::FromArgb(140, 0xA7, 0x8B, 0xFA))
    $pen = New-Object Drawing.Pen ([Drawing.Color]::FromArgb(230, 255, 255, 255)), $edgeWidth
    $pen.LineJoin = 'Round'
    $base = $g.Transform
    for ($k = 0; $k -lt 8; $k++) {
        $g.Transform = $base
        $g.TranslateTransform(56, 56)
        $g.RotateTransform(45 * $k)
        $g.TranslateTransform(-56, -56)
        $brush = if ($k % 2 -eq 0) { $sky } else { $violet }
        $g.FillEllipse($brush, 43, 10, 26, 42)
        $g.DrawEllipse($pen, 43, 10, 26, 42)
    }
    $g.Transform = $base
    $g.Dispose()
    return $bmp
}

# Tamanhos menores vão como DIB (BMP de 32 bits com máscara AND), que todos os
# carregadores de ícone aceitam; só o de 256 px é PNG, como o Windows espera.
function ConvertTo-IconBytes([Drawing.Bitmap]$bmp, [int]$n) {
    $ms = New-Object IO.MemoryStream
    if ($n -ge 256) {
        $bmp.Save($ms, [Drawing.Imaging.ImageFormat]::Png)
        return $ms.ToArray()
    }
    $bw = New-Object IO.BinaryWriter $ms
    $maskRow = [int]([Math]::Ceiling($n / 32.0) * 4)
    $bw.Write([uint32]40); $bw.Write([int32]$n); $bw.Write([int32](2 * $n))
    $bw.Write([uint16]1); $bw.Write([uint16]32); $bw.Write([uint32]0)
    $bw.Write([uint32]($n * $n * 4 + $maskRow * $n))
    $bw.Write([int32]0); $bw.Write([int32]0); $bw.Write([uint32]0); $bw.Write([uint32]0)
    $rect = New-Object Drawing.Rectangle 0, 0, $n, $n
    $data = $bmp.LockBits($rect, 'ReadOnly', [Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $row = New-Object byte[] ($n * 4)
    for ($y = $n - 1; $y -ge 0; $y--) {
        [Runtime.InteropServices.Marshal]::Copy([IntPtr]($data.Scan0.ToInt64() + $y * $data.Stride), $row, 0, $n * 4)
        $bw.Write($row)
    }
    $bmp.UnlockBits($data)
    $bw.Write((New-Object byte[] ($maskRow * $n)))
    $bw.Flush()
    return $ms.ToArray()
}

$sizes = 16, 20, 24, 32, 48, 64, 128, 256
$images = foreach ($n in $sizes) {
    $bmp = New-Frame $n
    $bytes = ConvertTo-IconBytes $bmp $n
    $bmp.Dispose()
    , @($n, $bytes)
}

$ico = Join-Path $assets 'wintaskbar.ico'
$fs = [IO.File]::Create($ico)
$w = New-Object IO.BinaryWriter $fs
$w.Write([uint16]0); $w.Write([uint16]1); $w.Write([uint16]$images.Count)
$offset = 6 + 16 * $images.Count
foreach ($img in $images) {
    $n = $img[0]; $bytes = $img[1]
    $dim = if ($n -ge 256) { 0 } else { $n }
    $w.Write([byte]$dim); $w.Write([byte]$dim); $w.Write([byte]0); $w.Write([byte]0)
    $w.Write([uint16]1); $w.Write([uint16]32); $w.Write([uint32]$bytes.Length); $w.Write([uint32]$offset)
    $offset += $bytes.Length
}
foreach ($img in $images) { $w.Write([byte[]]$img[1]) }
$w.Close(); $fs.Close()
Write-Host "OK -> $ico"

$rc = Join-Path $assets 'wintaskbar.rc'
foreach ($cmd in 'wintaskbar', 'setup') {
    $out = Join-Path $root "cmd\$cmd\icon_windows_amd64.syso"
    Push-Location $assets
    try { & $Windres --target=pe-x86-64 -i wintaskbar.rc -o $out } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) { throw "windres falhou para $cmd" }
    Write-Host "OK -> $out"
}
