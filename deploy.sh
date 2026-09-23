#!/usr/bin/env bash
#
# Build TimeLord and deploy it to a host over SSH.
#
# Usage:
#   sudo ./deploy.sh [--uninstall] [user@]host
#
# The script builds a static Linux binary, runs the tests, copies the binary and
# the systemd unit to the host, and restarts the service. Use --uninstall to
# stop and remove the service from the host.
#
# It must run as root, because it logs in to the host as root.

set -euo pipefail

BINARY_NAME="timelord"
INSTALL_DIR="/usr/local/bin"
INSTALL_PATH="${INSTALL_DIR}/${BINARY_NAME}"
UNIT_NAME="timelord.service"
UNIT_PATH="/etc/systemd/system/${UNIT_NAME}"
REMOTE_TMP="/tmp/${BINARY_NAME}.new"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNIT_SRC="${SCRIPT_DIR}/deploy/${UNIT_NAME}"
DIST_DIR="${SCRIPT_DIR}/dist"

# The user that invoked sudo owns the Go toolchain and its caches. Run the build
# tools as that user, not as root.
build_user="${SUDO_USER:-$(id -un)}"

# run_go runs a command in the repository as the build user.
run_go() {
	if [ "$build_user" = "$(id -un)" ]; then
		"$@"
		return
	fi
	runuser -u "$build_user" -- bash -lc 'cd "$1"; shift; exec "$@"' bash "$SCRIPT_DIR" "$@"
}

usage() {
	echo "usage: $(basename "$0") [--uninstall] [user@]host" >&2
}

if [ "$(id -u)" -ne 0 ]; then
	echo "error: run this script as root" >&2
	exit 1
fi

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

if [ "$uninstall" = true ]; then
	echo "Removing TimeLord from ${host}..."
	ssh "$host" "
		systemctl stop '${UNIT_NAME}' 2>/dev/null || true
		systemctl disable '${UNIT_NAME}' 2>/dev/null || true
		systemctl reset-failed '${UNIT_NAME}' 2>/dev/null || true
		rm -f '${UNIT_PATH}'
		systemctl daemon-reload
		rm -f '${INSTALL_PATH}' '${REMOTE_TMP}'
	"
	echo "Removed TimeLord from ${host}."
	exit 0
fi

if [ ! -f "$UNIT_SRC" ]; then
	echo "error: unit file not found: ${UNIT_SRC}" >&2
	exit 1
fi

# Fail early if the login on the host is not root.
remote_uid="$(ssh "$host" id -u)"
if [ "$remote_uid" != "0" ]; then
	echo "error: ${host} did not log in as root (uid ${remote_uid})" >&2
	exit 1
fi

echo "Running checks..."
cd "$SCRIPT_DIR"
if ! run_go go version >/dev/null 2>&1; then
	echo "error: the go command is not available to ${build_user}" >&2
	exit 1
fi
run_go go vet ./...
run_go go test ./...

echo "Building..."
# Go already builds with optimizations. -trimpath and -ldflags are for a
# reproducible, smaller binary. CGO_ENABLED=0 makes the binary static.
run_go mkdir -p "$DIST_DIR"
run_go env CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "${DIST_DIR}/${BINARY_NAME}" ./cmd/timelord

echo "Copying the binary to ${host}..."
scp "${DIST_DIR}/${BINARY_NAME}" "${host}:${REMOTE_TMP}"

echo "Installing the binary..."
# Replace the file with a rename, so a running service is not disturbed.
ssh "$host" "
	set -e
	install -m 0755 '${REMOTE_TMP}' '${INSTALL_PATH}.new'
	mv -f '${INSTALL_PATH}.new' '${INSTALL_PATH}'
	rm -f '${REMOTE_TMP}'
"

echo "Installing the systemd unit..."
scp "$UNIT_SRC" "${host}:${UNIT_PATH}.new"
ssh "$host" "
	set -e
	mv -f '${UNIT_PATH}.new' '${UNIT_PATH}'
	systemctl daemon-reload
	systemctl enable '${UNIT_NAME}'
	systemctl restart '${UNIT_NAME}'
"

echo "Checking that the service is up..."
for _ in 1 2 3 4 5 6 7 8 9 10; do
	if ssh "$host" "systemctl is-active --quiet '${UNIT_NAME}'"; then
		echo "TimeLord is running on ${host}."
		exit 0
	fi
	sleep 1
done

echo "error: TimeLord did not start on ${host}" >&2
ssh "$host" "systemctl status '${UNIT_NAME}' --no-pager" || true
exit 1
