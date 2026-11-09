#!/usr/bin/env bash
#
# Upload a TimeLord schedule to a user's home directory.
#
# Usage:
#   ./deploy-schedule.sh [user@]host [schedule-file]
#
# The default host is dani@dani and the default schedule file is
# timelord-schedule.<user>.json. The file is copied to the remote
# ~<user>/timelord-schedule.json.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
	echo "usage: $(basename "$0") [[user@]host] [schedule-file]" >&2
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
	usage
	exit 0
fi

host="${1:-dani@dani}"
user="${host%%@*}"
local_file="${2:-${SCRIPT_DIR}/timelord-schedule.${user}.json}"

if [ ! -f "$local_file" ]; then
	echo "error: schedule file not found: ${local_file}" >&2
	exit 1
fi

echo "Uploading ${local_file} to ${host}:~${user}/timelord-schedule.json..."
scp "$local_file" "${host}:~${user}/timelord-schedule.json"
echo "Schedule updated on ${host}."
