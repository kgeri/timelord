#!/usr/bin/env bash
#
# Deploy TimeLord Enforcers to the known Linux and Windows hosts.
#
# Usage:
#   ./deploy-enforcers.sh [--uninstall]

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
failures=0

usage() {
	echo "usage: $(basename "$0") [--uninstall]" >&2
}

uninstall=false
case "${1:-}" in
	"")
		;;
	--uninstall)
		uninstall=true
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		usage
		exit 1
		;;
esac

deploy_host() {
	local platform="$1"
	local host="$2"
	local users="$3"
	local deploy_script="${SCRIPT_DIR}/deploy-enforcer-${platform}.sh"

	if [ ! -x "$deploy_script" ]; then
		printf '[FAIL] %s %s: deploy script not found: %s\n' "$platform" "$host" "$deploy_script" >&2
		failures=$((failures + 1))
		return
	fi

	local args=()
	if [ "$uninstall" = true ]; then
		args+=(--uninstall)
	fi
	args+=("$host")

	local -a env_args=(
		"TIMELORD_USERS=$users"
		"TIMETOWER_URL=${TIMETOWER_URL:-}"
		"TIMELORD_SECRETS=${TIMELORD_SECRETS:-}"
	)
	local -a prefix=()
	if [ "$platform" = "linux" ] && [ "$(id -u)" -ne 0 ]; then
		if ! sudo -v; then
			printf '[FAIL] %s %s: sudo authentication failed\n' "$platform" "$host" >&2
			failures=$((failures + 1))
			return
		fi
		# Keep the invoking user's SSH config and keys available to root.
		env_args+=("HOME=${HOME:-}")
		prefix=(sudo)
	fi
	prefix+=(env "${env_args[@]}")

	local output
	output=$(mktemp)
	if "${prefix[@]}" "$deploy_script" "${args[@]}" >"$output" 2>&1; then
		printf '[OK] %s %s\n' "$platform" "$host"
	else
		local status=$?
		local message
		message=$(awk 'NF { line=$0 } END { print line }' "$output")
		message="${message%$'\r'}"
		[ -z "$message" ] && message="exit status ${status}"
		printf '[FAIL] %s %s: %s\n' "$platform" "$host" "$message" >&2
		tail -n 20 "$output" | tr -d '\r' | sed 's/^/  /' >&2
		failures=$((failures + 1))
	fi
	rm -f "$output"
}

deploy_host linux dani dani=dani
deploy_host windows acer "kissd=dani kissr=reka"

exit "$failures"
