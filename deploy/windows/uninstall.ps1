#Requires -Version 5.1
<#
.SYNOPSIS
Removes the TimeLord Windows service.

.DESCRIPTION
Stops and deletes the service, removes the firewall rule, and deletes the
install directory. The script must run from an elevated session.

.PARAMETER ServiceName
The Windows service name. Defaults to TimeLord.

.PARAMETER InstallDir
The install directory to delete. Defaults to %ProgramFiles%\TimeLord.

.PARAMETER KeepFiles
Keep the install directory instead of deleting it.
#>
[CmdletBinding()]
param(
    [string]$ServiceName = 'TimeLord',
    [string]$InstallDir = (Join-Path $env:ProgramFiles 'TimeLord'),
    [switch]$KeepFiles
)

$ErrorActionPreference = 'Stop'

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'TimeLord must be removed from an elevated (administrator) session.'
}

if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    Write-Host "Removing the $ServiceName service..."
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    sc.exe delete $ServiceName | Out-Null
}

Remove-NetFirewallRule -DisplayName 'TimeLord metrics' -ErrorAction SilentlyContinue

if (-not $KeepFiles) {
    Write-Host "Deleting $InstallDir..."
    Remove-Item -LiteralPath $InstallDir -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host 'TimeLord removed.'
