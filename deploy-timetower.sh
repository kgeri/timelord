#!/bin/bash
#
# Build and deploy Timetower to the k3s cluster.
#
# Usage:
#   ./deploy-timetower.sh [user@]host
#
# The image tag is the current git commit. The script refuses to deploy from a
# dirty tree, seeds /data/hosted/timetower/config.json from
# deploy/timetower/config-default.json when it is missing, runs the Go checks,
# builds and pushes the image, and applies the Kubernetes manifests.
#
# Without a host argument the script assumes it runs on the host that owns
# /data/hosted/timetower. With a host argument it creates the directory and
# copies the default config over SSH/SCP. TIMETOWER_HOST can replace the host
# argument.
#
# Set TIMETOWER_DIR to override /data/hosted/timetower.

set -euxo pipefail

CWD=$(cd "$(dirname "$0")" && pwd)
NAMESPACE="${NAMESPACE:-default}"
TIMETOWER_HOST="${1:-${TIMETOWER_HOST:-}}"
TIMETOWER_DIR="${TIMETOWER_DIR:-/data/hosted/timetower}"
DEFAULT_CONFIG="$CWD/deploy/timetower/config-default.json"
CONFIG_PATH="$TIMETOWER_DIR/config.json"
IMAGE_REPOSITORY="docker.ogreg.org/timetower"

if [ -n "$(git -C "$CWD" status --porcelain)" ]; then
	echo "ERROR: working tree has uncommitted changes; commit before deploying" >&2
	exit 1
fi

if [ ! -f "$DEFAULT_CONFIG" ]; then
	echo "ERROR: default config file not found: $DEFAULT_CONFIG" >&2
	exit 1
fi

# Seed the live config once. An existing host config is never overwritten.
if [ -n "$TIMETOWER_HOST" ]; then
	ssh "$TIMETOWER_HOST" "mkdir -p '$TIMETOWER_DIR'"
	if ! ssh "$TIMETOWER_HOST" "test -e '$CONFIG_PATH'"; then
		scp "$DEFAULT_CONFIG" "$TIMETOWER_HOST:$CONFIG_PATH"
	fi
else
	if ! mkdir -p "$TIMETOWER_DIR"; then
		echo "ERROR: cannot create $TIMETOWER_DIR; pass a host or set TIMETOWER_HOST" >&2
		exit 1
	fi
	if [ ! -e "$CONFIG_PATH" ]; then
		install -m 0644 "$DEFAULT_CONFIG" "$CONFIG_PATH"
	fi
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
NAMESPACE="$NAMESPACE" TIMETOWER_DIR="$TIMETOWER_DIR" IMAGE_TAG="$TAG" envsubst < "$CWD/deploy/timetower/deployment.yaml" | kubectl apply -f -
kubectl rollout restart deployment/timetower --namespace "$NAMESPACE"
kubectl rollout status deployment/timetower --namespace "$NAMESPACE" --timeout=120s

echo "Deployment completed"
