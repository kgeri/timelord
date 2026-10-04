#Requires -Version 5.1
<#
.SYNOPSIS
Installs TimeLord as a Windows service that runs as LocalSystem.

.DESCRIPTION
Copies timelord.exe into the install directory, registers it as an automatic
service, opens the metrics port, and starts the service. The script must run
from an elevated session. It is meant to be copied next to timelord.exe, but
-BinaryPath can point elsewhere.

.PARAMETER BinaryPath
The timelord.exe to install. Defaults to timelord.exe next to this script.

.PARAMETER InstallDir
The directory to install into. Defaults to %ProgramFiles%\TimeLord.

.PARAMETER ServiceName
The Windows service name. Defaults to TimeLord.

.PARAMETER ListenPort
The metrics port to open in the firewall. Defaults to 9220.
#>
[CmdletBinding()]
param(
    [string]$BinaryPath = '',
    [string]$InstallDir = (Join-Path $env:ProgramFiles 'TimeLord'),
    [string]$ServiceName = 'TimeLord',
    [int]$ListenPort = 9220
)

$ErrorActionPreference = 'Stop'

# $PSScriptRoot is not reliably populated in the param block, so resolve the
# default binary next to this script after binding.
if (-not $BinaryPath) {
    $scriptDir = ''
    if ($MyInvocation.MyCommand.Path) {
        $scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    }
    if (-not $scriptDir -and $PSScriptRoot) {
        $scriptDir = $PSScriptRoot
    }
    if (-not $scriptDir) {
        $scriptDir = (Get-Location).Path
    }
    $BinaryPath = Join-Path $scriptDir 'timelord.exe'
}

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'TimeLord must be installed from an elevated (administrator) session.'
}

if (-not (Test-Path -LiteralPath $BinaryPath)) {
    throw "Binary not found: $BinaryPath"
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$target = Join-Path $InstallDir 'timelord.exe'

if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    Write-Host "Removing the existing ${ServiceName} service..."
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    sc.exe delete $ServiceName | Out-Null
    # The service manager deletes a service asynchronously.
    for ($i = 0; $i -lt 10 -and (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue); $i++) {
        Start-Sleep -Milliseconds 500
    }
}

Write-Host "Installing to $target..."
Copy-Item -LiteralPath $BinaryPath -Destination $target -Force

Write-Host 'Running the TimeLord self-test...'
& $target -selftest
if ($LASTEXITCODE -ne 0) {
    throw "TimeLord self-test failed with exit code $LASTEXITCODE."
}

Write-Host "Registering the $ServiceName service (LocalSystem, automatic)..."
New-Service -Name $ServiceName `
    -BinaryPathName "`"$target`" -listen 0.0.0.0:$ListenPort" `
    -DisplayName 'TimeLord' `
    -StartupType Automatic | Out-Null
sc.exe description $ServiceName 'TimeLord process monitor' | Out-Null
# Restart after 5s, 10s, and 30s, then at most once a day.
sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/10000/restart/30000 | Out-Null

Write-Host "Opening inbound TCP $ListenPort..."
if (-not (Get-NetFirewallRule -DisplayName 'TimeLord metrics' -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -DisplayName 'TimeLord metrics' `
        -Direction Inbound -Action Allow -Protocol TCP -LocalPort $ListenPort | Out-Null
}

Write-Host "Starting the $ServiceName service..."
Start-Service -Name $ServiceName
Get-Service -Name $ServiceName | Format-Table -AutoSize
