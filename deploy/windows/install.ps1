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

.PARAMETER TimetowerURL
The Timetower base URL. Defaults to https://timetower.ogreg.org.

.PARAMETER Users
Comma- or space-separated account names to manage. Defaults to dani.

.PARAMETER EnforcerKeyFile
Path to a file that contains the Enforcer key. The installer copies it to a
protected location and registers the service to read it from there.

.PARAMETER KeyPath
The protected key file location. Defaults to
%ProgramData%\TimeLord\timetower.key.
#>
[CmdletBinding()]
param(
    [string]$BinaryPath = '',
    [string]$InstallDir = (Join-Path $env:ProgramFiles 'TimeLord'),
    [string]$ServiceName = 'TimeLord',
    [int]$ListenPort = 9220,
    [string]$TimetowerURL = 'https://timetower.ogreg.org',
    [string[]]$Users = @('dani'),
    [string]$EnforcerKeyFile = '',
    [string]$KeyPath = (Join-Path $env:ProgramData 'TimeLord\timetower.key')
)

$ErrorActionPreference = 'Stop'

# Accept both "-Users dani,alice" and "-Users 'dani alice'".
$managedUsers = @()
foreach ($entry in $Users) {
    foreach ($name in ($entry -split '[,\s]+')) {
        $name = $name.Trim()
        if ($name -and ($managedUsers -notcontains $name)) {
            $managedUsers += $name
        }
    }
}
if ($managedUsers.Count -eq 0) {
    throw 'At least one managed user is required.'
}
if (-not $EnforcerKeyFile) {
    throw 'EnforcerKeyFile is required.'
}
if (-not (Test-Path -LiteralPath $EnforcerKeyFile)) {
    throw "Enforcer key file not found: $EnforcerKeyFile"
}
if ((Get-Item -LiteralPath $EnforcerKeyFile).Length -eq 0) {
    throw "Enforcer key file is empty: $EnforcerKeyFile"
}
$KeyPath = [IO.Path]::GetFullPath($KeyPath)

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

Write-Host "Installing the Enforcer key to $KeyPath..."
$keyDir = Split-Path -Parent $KeyPath
New-Item -ItemType Directory -Force -Path $keyDir | Out-Null
$keySource = [IO.Path]::GetFullPath($EnforcerKeyFile)
$keyTarget = [IO.Path]::GetFullPath($KeyPath)
if ($keySource -ne $keyTarget) {
    if (Test-Path -LiteralPath $KeyPath) {
        Remove-Item -LiteralPath $KeyPath -Force
    }
    Copy-Item -LiteralPath $EnforcerKeyFile -Destination $KeyPath -Force
}
# Use well-known SIDs so this works on non-English Windows installations.
& icacls.exe $KeyPath /inheritance:r /grant:r '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw "Failed to restrict access to $KeyPath."
}

Write-Host "Registering the $ServiceName service (LocalSystem, automatic)..."
$serviceArgs = @(
    '-listen', "0.0.0.0:$ListenPort",
    '-timetower', "`"$TimetowerURL`"",
    '-key-file', "`"$KeyPath`""
)
foreach ($user in $managedUsers) {
    $serviceArgs += @('-user', "`"$user`"")
}
$binaryPathName = "`"$target`" " + ($serviceArgs -join ' ')
New-Service -Name $ServiceName `
    -BinaryPathName $binaryPathName `
    -DisplayName 'TimeLord' `
    -StartupType Automatic | Out-Null
sc.exe description $ServiceName 'TimeLord kid control enforcer' | Out-Null
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
