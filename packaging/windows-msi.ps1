# Build a Windows installer with the WiX toolset, on Windows.
#
#   packaging/windows-msi.ps1 -Stage <dir> -Extra <dir> -Out <file.msi>
#
# <Extra> is what packaging/windows-msi.sh made on Linux: the icon, the dialog
# bitmaps, the installer's note and msi.env. WiX builds the dialogs wixl could
# not - a Destination Folder page with a Change button, and a page that says the
# install finished - and it only builds an MSI on Windows (#508).
param(
  [Parameter(Mandatory)] [string] $Stage,
  [Parameter(Mandatory)] [string] $Extra,
  [Parameter(Mandatory)] [string] $Out
)
$ErrorActionPreference = 'Stop'

$cfg = @{}
Get-Content (Join-Path $Extra 'msi.env') | ForEach-Object {
  $k, $v = $_ -split '=', 2
  if ($k) { $cfg[$k] = $v }
}

$wxs = Join-Path $PSScriptRoot 'meshbench.wxs'
$Stage = (Resolve-Path $Stage).Path
$Extra = (Resolve-Path $Extra).Path
$Out = [System.IO.Path]::GetFullPath($Out)

# One command, and no harvest step: WiX walks the tree itself, in its own
# order, so the same tree built twice is the same installer.
& wix build -arch x64 `
  -d "ProductCode=$($cfg.PRODUCT)" -d "Version=$($cfg.MSIVERSION)" `
  -d "Stage=$Stage" -d "Extra=$Extra" `
  -ext WixToolset.UI.wixext `
  -o $Out $wxs
if ($LASTEXITCODE -ne 0) { throw "wix build failed with $LASTEXITCODE" }

# ALLUSERS back to 2, which is the one thing the toolset will not author.
#
# This package has always been dual-purpose: per-machine by default, and
# per-user with MSIINSTALLPERUSER=1, which is what lets somebody without
# administrator rights install into their own profile. WiX 4 removed the
# option and emits ALLUSERS=1, so the row is corrected here, through Windows
# Installer's own database API, rather than a documented install route being
# dropped to suit a schema. verify-msi.sh checks it.
$installer = New-Object -ComObject WindowsInstaller.Installer
$db = $installer.GetType().InvokeMember('OpenDatabase', 'InvokeMethod', $null, $installer, @($Out, 1))
$view = $db.GetType().InvokeMember('OpenView', 'InvokeMethod', $null, $db,
  @("UPDATE ``Property`` SET ``Value``='2' WHERE ``Property``='ALLUSERS'"))
$view.GetType().InvokeMember('Execute', 'InvokeMethod', $null, $view, $null) | Out-Null
$view.GetType().InvokeMember('Close', 'InvokeMethod', $null, $view, $null) | Out-Null
$db.GetType().InvokeMember('Commit', 'InvokeMethod', $null, $db, $null) | Out-Null
[System.Runtime.InteropServices.Marshal]::FinalReleaseComObject($db) | Out-Null

Get-Item $Out | Format-List Name, Length
