#!/usr/bin/env bash
#
# Build TimeLord for Windows and deploy it over SSH.
#
# Usage:
#   ./deploy-windows.sh [--uninstall] [user@]host
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

if [ "$uninstall" = false ]; then
	echo "Running checks..."
	cd "$SCRIPT_DIR"
	go vet ./...
	GOOS=windows GOARCH=amd64 go vet ./...
	go test ./...

	echo "Building ${BINARY_NAME}..."
	mkdir -p "$DIST_DIR"
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" \
		-o "${DIST_DIR}/${BINARY_NAME}" ./cmd/timelord
fi

if [ "$uninstall" = true ]; then
	echo "Removing TimeLord from ${host}..."
	scp "${SCRIPT_DIR}/${UNINSTALLER_SRC}" "${host}:uninstall.ps1"
	ssh "$host" 'powershell -NoProfile -ExecutionPolicy Bypass -File uninstall.ps1'
	ssh "$host" 'del /q uninstall.ps1' 2>/dev/null || true
	echo "Removed TimeLord from ${host}."
	exit 0
fi

echo "Copying files to ${host}..."
scp "${DIST_DIR}/${BINARY_NAME}" "${SCRIPT_DIR}/${INSTALLER_SRC}" "${host}:"

echo "Installing TimeLord..."
if ! ssh "$host" 'powershell -NoProfile -ExecutionPolicy Bypass -File install.ps1'; then
	echo "error: the installer failed on ${host}; see the output above." >&2
	echo "A permissions error means the SSH session was not elevated: use the" >&2
	echo "built-in Administrator account, or set LocalAccountTokenFilterPolicy." >&2
	exit 1
fi

ssh "$host" "del /q install.ps1 ${BINARY_NAME}" 2>/dev/null || true

echo "TimeLord is running on ${host}."
