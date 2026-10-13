#requires -Version 5.1
<# Mede a Taskbar por UI Automation, sem clicar, reiniciar ou alterar o tema. #>
param([switch]$Json)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class TaskbarMeasurement {
    [StructLayout(LayoutKind.Sequential)] public struct Rect { public int Left, Top, Right, Bottom; }
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern IntPtr FindWindow(string cls, string name);
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hwnd, out Rect rect);
    [DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
}
'@

$previousDpi = [TaskbarMeasurement]::SetThreadDpiAwarenessContext([IntPtr](-4))
try {
    $handle = [TaskbarMeasurement]::FindWindow('Shell_TrayWnd', $null)
    if ($handle -eq [IntPtr]::Zero) { throw 'Taskbar not found.' }
    $rect = New-Object TaskbarMeasurement+Rect
    if (-not [TaskbarMeasurement]::GetWindowRect($handle, [ref]$rect)) { throw 'Taskbar bounds unavailable.' }
    $root = [System.Windows.Automation.AutomationElement]::FromHandle($handle)
    $nodes = $root.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
    $rows = @()
    $appCount = 0
    foreach ($node in $nodes) {
        $info = $node.Current
        if ($info.ClassName -like '*TaskListButton*') { $appCount++ }
        $kind = $null
        if ($info.ClassName -like '*TaskbarFrame*') { $kind = 'Frame' }
        elseif ($info.AutomationId -eq 'OverflowButton') { $kind = 'Overflow' }
        elseif ($info.ClassName -eq 'TrayNotifyWnd') { $kind = 'Tray' }
        elseif ($info.ClassName -like '*SystemTray.NormalButton*') { $kind = 'NormalButton' }
        elseif ($info.ClassName -like '*SystemTray.ShowDesktopButton*') { $kind = 'ShowDesktop' }
        if (-not $kind) { continue }
        $bounds = $info.BoundingRectangle
        $rows += [pscustomobject]@{
            Kind = $kind; Class = $info.ClassName; Id = $info.AutomationId; Name = $info.Name
            X = $bounds.Left; Right = $bounds.Right; Width = $bounds.Width
            Offscreen = $info.IsOffscreen
        }
    }
    $frame = $rows | Where-Object Kind -eq 'Frame' | Select-Object -First 1
    $tray = $rows | Where-Object Kind -eq 'Tray' | Select-Object -First 1
    $overflow = $rows | Where-Object Kind -eq 'Overflow' | Select-Object -First 1
    $desktop = $rows | Where-Object Kind -eq 'ShowDesktop' | Select-Object -First 1
    $normal = $rows | Where-Object Kind -eq 'NormalButton' | Sort-Object X | Select-Object -First 1
    $result = [pscustomobject]@{
        Time = (Get-Date).ToString('o')
        TaskbarX = $rect.Left; TaskbarWidth = $rect.Right - $rect.Left
        AppElements = $appCount
        Frame = $(if ($frame) { $frame } else { $null })
        Overflow = $(if ($overflow) { $overflow } else { $null })
        Tray = $(if ($tray) { $tray } else { $null })
        FirstNormalButton = $(if ($normal) { $normal } else { $null })
        ShowDesktop = $(if ($desktop) { $desktop } else { $null })
        OverflowGap = $(if ($overflow -and $tray) { $tray.X - $overflow.Right } else { $null })
        OverflowBeforeTray = $(if ($overflow -and $tray) { -not $overflow.Offscreen -and $overflow.Width -gt 0 -and $overflow.Right -le $tray.X } else { $null })
        FrameCenterOffset = $(if ($frame) { ($frame.X + $frame.Right - $rect.Left - $rect.Right) / 2 } else { $null })
        ShowDesktopVisible = [bool]($desktop -and -not $desktop.Offscreen -and $desktop.Width -gt 0)
    }
    if ($Json) { $result | ConvertTo-Json -Depth 5 }
    else { $result | Format-List }
} finally {
    if ($previousDpi -ne [IntPtr]::Zero) { [void][TaskbarMeasurement]::SetThreadDpiAwarenessContext($previousDpi) }
}
