#!/usr/bin/env bash
#
# Build the TimeLord Enforcer for Windows and deploy it over SSH.
#
# Usage:
#   ./deploy-enforcer-windows.sh [--uninstall] [user@]host
#
# The script cross-compiles a Windows amd64 binary, copies the binary and a
# PowerShell script to the host, and runs the script. The host must run an
# OpenSSH server. Installing or removing the service requires an elevated
# session: install.ps1 and uninstall.ps1 refuse to run otherwise. Use the
# built-in Administrator account, or set LocalAccountTokenFilterPolicy if a
# local administrator login is not elevated.

set -euo pipefail

BINARY_NAME="timelord.exe"
INSTALLER_SRC="deploy/windows/install.ps1"
UNINSTALLER_SRC="deploy/windows/uninstall.ps1"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="${SCRIPT_DIR}/dist"
TIMETOWER_URL="${TIMETOWER_URL:-https://timetower.ogreg.org}"
TIMELORD_USERS="${TIMELORD_USERS:-kissd=dani kissr=reka}"
SECRETS_FILE="${TIMELORD_SECRETS:-${SCRIPT_DIR}/secrets.env}"
key_build=""
trap 'rm -f "$key_build"' EXIT

# Windows OpenSSH may not provide a usable scp when the SSH shell is cmd.exe.
# SFTP is part of the Windows OpenSSH server, so use it for uploads.
sftp_put() {
	local host="$1"
	local source="$2"
	local destination="$3"
	printf 'put "%s" "%s"\n' "$source" "$destination" | sftp -q -b - "$host"
}

# Remove the old service and binary before uploading the new build. This also
# clears a stale service registration that points at an incomplete command line.
wipe_remote() {
	local host="$1"
	ssh "$host" 'powershell -NoProfile -ExecutionPolicy Bypass -Command -' <<'POWERSHELL'
$ErrorActionPreference = 'Stop'
$serviceName = 'TimeLord'
$binary = Join-Path (Join-Path $env:ProgramFiles 'TimeLord') 'timelord.exe'

$serviceKey = "HKLM:\SYSTEM\CurrentControlSet\Services\$serviceName"
$service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
if ($null -ne $service) {
    if ($service.Status -ne 'Stopped') {
        Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
        try { $service.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(20)) } catch {}
    }
    sc.exe delete $serviceName | Out-Null
}

# sc.exe delete is asynchronous. Wait for both the service and its registry key
# to disappear before the installer recreates the same name.
$deadline = (Get-Date).AddSeconds(60)
while (((Get-Service -Name $serviceName -ErrorAction SilentlyContinue) -or (Test-Path -LiteralPath $serviceKey)) -and (Get-Date) -lt $deadline) {
    Start-Sleep -Milliseconds 250
}
if ((Get-Service -Name $serviceName -ErrorAction SilentlyContinue) -or (Test-Path -LiteralPath $serviceKey)) {
    throw "service $serviceName still exists after delete"
}

Get-Process -Name timelord -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 250

if (Test-Path -LiteralPath $binary) {
    Remove-Item -LiteralPath $binary -Force
}
POWERSHELL
}

usage() {
	echo "usage: $(basename "$0") [--uninstall] [user@]host" >&2
}

uninstall=false
host=""

while [ "$#" -gt 0 ]; do
	case "$1" in
		--uninstall)
			uninstall=true
			;;
		-h | --help)
			usage
			exit 0
			;;
		-*)
			echo "error: unknown option: $1" >&2
			usage
			exit 1
			;;
		*)
			if [ -n "$host" ]; then
				echo "error: only one host is allowed" >&2
				usage
				exit 1
			fi
			host="$1"
			;;
	esac
	shift
done

if [ -z "$host" ]; then
	usage
	exit 1
fi

if [ ! -f "${SCRIPT_DIR}/${INSTALLER_SRC}" ]; then
	echo "error: installer not found: ${SCRIPT_DIR}/${INSTALLER_SRC}" >&2
	exit 1
fi

# Resolve the remote profile directory so SFTP and PowerShell use the same
# absolute paths. The SFTP server uses /C:/Users/... style paths on Windows.
remote_home=$(ssh "$host" 'cmd /c echo %USERPROFILE%' | tr -d '\r"')
remote_home=$(printf '%s' "$remote_home" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')
case "$remote_home" in
	[A-Za-z]:*) ;;
	*)
		echo "error: cannot determine the remote profile directory" >&2
		exit 1
		;;
esac
remote_home_sftp="/$(printf '%s' "$remote_home" | sed 's|\\|/|g')"

if [ "$uninstall" = false ]; then
	if [ ! -f "$SECRETS_FILE" ]; then
		echo "error: secrets file not found: ${SECRETS_FILE}" >&2
		exit 1
	fi
	secret_mode=$(stat -c '%a' "$SECRETS_FILE" 2>/dev/null || true)
	if [ "$secret_mode" != "600" ] && [ "$secret_mode" != "400" ]; then
		echo "error: ${SECRETS_FILE} must be mode 0600 or 0400 (currently ${secret_mode:-unknown})" >&2
		exit 1
	fi
	# shellcheck disable=SC1090
	. "$SECRETS_FILE"
	if [ -z "${TIMELORD_ENFORCER_KEY:-}" ]; then
		echo "error: TIMELORD_ENFORCER_KEY is empty or missing in ${SECRETS_FILE}" >&2
		exit 1
	fi

	echo "Running checks..."
	cd "$SCRIPT_DIR"
	go vet ./...
	GOOS=windows GOARCH=amd64 go vet ./...
	go test ./...

	echo "Building ${BINARY_NAME}..."
	mkdir -p "$DIST_DIR"
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" \
		-o "${DIST_DIR}/${BINARY_NAME}" ./cmd/timelord

	echo "Removing existing TimeLord service and binary..."
	wipe_remote "$host"
fi

remote_uninstaller="${remote_home}\\uninstall.ps1"
remote_installer="${remote_home}\\install.ps1"
remote_binary="${remote_home}\\${BINARY_NAME}"
remote_key="${remote_home}\\timelord.key.new"

if [ "$uninstall" = true ]; then
	echo "Removing TimeLord from ${host}..."
	sftp_put "$host" "${SCRIPT_DIR}/${UNINSTALLER_SRC}" "${remote_home_sftp}/uninstall.ps1"
	ssh "$host" "powershell -NoProfile -ExecutionPolicy Bypass -File \"${remote_uninstaller}\""
	ssh "$host" "cmd /c del /q \"${remote_uninstaller}\"" 2>/dev/null || true
	echo "Removed TimeLord from ${host}."
	exit 0
fi

echo "Copying files to ${host} (${remote_home})..."
sftp_put "$host" "${DIST_DIR}/${BINARY_NAME}" "${remote_home_sftp}/${BINARY_NAME}"
sftp_put "$host" "${SCRIPT_DIR}/${INSTALLER_SRC}" "${remote_home_sftp}/install.ps1"
key_build="$(mktemp)"
chmod 600 "$key_build"
printf '%s\n' "$TIMELORD_ENFORCER_KEY" > "$key_build"
sftp_put "$host" "$key_build" "${remote_home_sftp}/timelord.key.new"

echo "Installing TimeLord..."
if ! ssh "$host" "powershell -NoProfile -ExecutionPolicy Bypass -File \"${remote_installer}\" -TimetowerURL \"$TIMETOWER_URL\" -Users \"$TIMELORD_USERS\" -EnforcerKeyFile \"${remote_key}\""; then
	echo "A permissions error means the SSH session was not elevated: use the" >&2
	echo "built-in Administrator account, or set LocalAccountTokenFilterPolicy." >&2
	echo "error: the installer failed on ${host}; see the output above." >&2
	exit 1
fi

if ! ssh "$host" "sc.exe query TimeLord" >/dev/null; then
	echo "error: the TimeLord service was not registered on ${host}" >&2
	exit 1
fi

ssh "$host" "cmd /c del /q \"${remote_installer}\" \"${remote_binary}\" \"${remote_key}\"" 2>/dev/null || true

echo "TimeLord is running on ${host}."
