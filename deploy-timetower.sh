#!/bin/bash
#
# Build and deploy Timetower to the k3s cluster.
#
# Usage:
#   ./deploy-timetower.sh [user@]host
#
# The default host is srvu. The live hostPath config is seeded there so the
# hostPath-backed Deployment always sees the same configuration.
#
# The image tag is the current git commit. The script refuses to deploy from a
# dirty tree, reads TIMELORD_KEY from secrets.env, updates the Timetower
# Kubernetes Secret, seeds /data/hosted/timetower/config.json from
# deploy/timetower/config-default.json when it is missing, runs the Go checks,
# builds and pushes the image, and applies the Kubernetes manifests.
#
# The script creates /data/hosted/timetower on the target host and copies the
# default config there over SSH/SCP when it is missing. Set
# TIMETOWER_FORCE_CONFIG=1 to replace an existing config with the default.
# TIMETOWER_HOST can replace the host argument.
#
# Set TIMETOWER_NODE to override the Kubernetes node name (default srvu).
# Set TIMETOWER_DIR to override /data/hosted/timetower.

set -euxo pipefail

CWD=$(cd "$(dirname "$0")" && pwd)
NAMESPACE="${NAMESPACE:-default}"
TIMETOWER_HOST="${1:-${TIMETOWER_HOST:-srvu}}"
TIMETOWER_NODE="${TIMETOWER_NODE:-srvu}"
TIMETOWER_DIR="${TIMETOWER_DIR:-/data/hosted/timetower}"
DEFAULT_CONFIG="$CWD/deploy/timetower/config-default.json"
CONFIG_PATH="$TIMETOWER_DIR/config.json"
SECRETS_FILE="${TIMELORD_SECRETS:-$CWD/secrets.env}"
SECRET_NAME="timetower-secrets"
IMAGE_REPOSITORY="docker.ogreg.org/timetower"

if [ -n "$(git -C "$CWD" status --porcelain)" ]; then
	echo "ERROR: working tree has uncommitted changes; commit before deploying" >&2
	exit 1
fi

if [ ! -f "$DEFAULT_CONFIG" ]; then
	echo "ERROR: default config file not found: $DEFAULT_CONFIG" >&2
	exit 1
fi

if [ ! -f "$SECRETS_FILE" ]; then
	echo "ERROR: secrets file not found: $SECRETS_FILE" >&2
	exit 1
fi
secret_mode=$(stat -c '%a' "$SECRETS_FILE" 2>/dev/null || true)
if [ "$secret_mode" != "600" ] && [ "$secret_mode" != "400" ]; then
	echo "ERROR: $SECRETS_FILE must be mode 0600 or 0400 (currently ${secret_mode:-unknown})" >&2
	exit 1
fi

set +x
# shellcheck disable=SC1090
. "$SECRETS_FILE"
if [ -z "${TIMELORD_KEY:-}" ]; then
	echo "ERROR: TIMELORD_KEY is empty or missing in $SECRETS_FILE" >&2
	exit 1
fi
secret_file=$(mktemp)
trap 'rm -f "$secret_file"' EXIT
chmod 600 "$secret_file"
printf 'TIMELORD_KEY=%s\n' "$TIMELORD_KEY" > "$secret_file"
kubectl create secret generic "$SECRET_NAME" \
	--namespace "$NAMESPACE" \
	--from-env-file="$secret_file" \
	--dry-run=client -o yaml | kubectl apply -f -
set -x

# Seed the live config once. An existing host config is preserved unless
# TIMETOWER_FORCE_CONFIG=1 is set.
ssh "$TIMETOWER_HOST" "mkdir -p '$TIMETOWER_DIR'"
if [ "${TIMETOWER_FORCE_CONFIG:-0}" = "1" ] || ! ssh "$TIMETOWER_HOST" "test -e '$CONFIG_PATH'"; then
	scp "$DEFAULT_CONFIG" "$TIMETOWER_HOST:$CONFIG_PATH"
fi

# Run the container as the owner of the hostPath directory so it can write the
# state file without running as root.
TIMETOWER_UID=$(ssh "$TIMETOWER_HOST" "stat -c '%u' '$TIMETOWER_DIR'")
TIMETOWER_GID=$(ssh "$TIMETOWER_HOST" "stat -c '%g' '$TIMETOWER_DIR'")
case "$TIMETOWER_UID" in
	''|*[!0-9]*) echo "ERROR: invalid hostPath owner UID: $TIMETOWER_UID" >&2; exit 1 ;;
esac
case "$TIMETOWER_GID" in
	''|*[!0-9]*) echo "ERROR: invalid hostPath owner GID: $TIMETOWER_GID" >&2; exit 1 ;;
esac
if [ "$TIMETOWER_UID" = "0" ]; then
	echo "ERROR: $TIMETOWER_DIR is owned by root; chown it to the deploy user" >&2
	exit 1
fi

# Tests
(cd "$CWD" && go test ./...)
(cd "$CWD" && go vet ./...)

# Docker cleanup
docker container prune -f
docker image prune -f

# Build
TAG=$(git -C "$CWD" rev-parse --short HEAD)
docker build \
	-f "$CWD/deploy/timetower/Dockerfile" \
	-t "timetower:$TAG" \
	"$CWD"
docker tag "timetower:$TAG" "${IMAGE_REPOSITORY}:$TAG"
docker push "${IMAGE_REPOSITORY}:$TAG"

# Deploy
NAMESPACE="$NAMESPACE" TIMETOWER_NODE="$TIMETOWER_NODE" TIMETOWER_DIR="$TIMETOWER_DIR" TIMETOWER_UID="$TIMETOWER_UID" TIMETOWER_GID="$TIMETOWER_GID" IMAGE_TAG="$TAG" envsubst < "$CWD/deploy/timetower/deployment.yaml" | kubectl apply -f -
kubectl rollout restart deployment/timetower --namespace "$NAMESPACE"
kubectl rollout status deployment/timetower --namespace "$NAMESPACE" --timeout=120s

echo "Deployment completed"
