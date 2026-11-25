#!/bin/bash
#
# Deploy the Timetower configuration to the k3s host.
#
# Usage:
#   ./deploy-timetower-config.sh [user@]host
#
# The default host is srvu. The repository file
# deploy/timetower/config-srvu.json is copied to
# /data/hosted/timetower/config.json, then the Timetower Deployment is restarted
# so it loads the new configuration.
#
# Set TIMETOWER_HOST to replace the host argument.
# Set TIMETOWER_DIR to override /data/hosted/timetower.
# Set NAMESPACE to override the Kubernetes namespace.

set -euxo pipefail

CWD=$(cd "$(dirname "$0")" && pwd)
NAMESPACE="${NAMESPACE:-default}"
TIMETOWER_HOST="${1:-${TIMETOWER_HOST:-srvu}}"
TIMETOWER_DIR="${TIMETOWER_DIR:-/data/hosted/timetower}"
CONFIG_PATH="$TIMETOWER_DIR/config.json"
SRVU_CONFIG="$CWD/deploy/timetower/config-srvu.json"

if [ -n "$(git -C "$CWD" status --porcelain)" ]; then
	echo "ERROR: working tree has uncommitted changes; commit before deploying" >&2
	exit 1
fi

if [ ! -f "$SRVU_CONFIG" ]; then
	echo "ERROR: srvu config file not found: $SRVU_CONFIG" >&2
	exit 1
fi

# Fail before touching the live config when the repository config is invalid.
(cd "$CWD" && go test ./internal/timetower)

ssh "$TIMETOWER_HOST" "mkdir -p '$TIMETOWER_DIR'"
scp "$SRVU_CONFIG" "$TIMETOWER_HOST:${CONFIG_PATH}.new"
ssh "$TIMETOWER_HOST" "chmod 0644 '${CONFIG_PATH}.new' && mv -f '${CONFIG_PATH}.new' '${CONFIG_PATH}'"

kubectl rollout restart deployment/timetower --namespace "$NAMESPACE"
kubectl rollout status deployment/timetower --namespace "$NAMESPACE" --timeout=120s

echo "Timetower config deployed"
