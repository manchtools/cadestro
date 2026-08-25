#!/bin/bash










set -euo pipefail

CONTAINER_NAME="cadestro-sdk-test-$$"
IMAGE_NAME="cadestro-sdk-test"
CONTAINER_LIMITS=(--memory=6g --cpus=4)

cleanup() {
    podman stop -t 2 "$CONTAINER_NAME" 2>/dev/null || true
    podman rm -f "$CONTAINER_NAME" 2>/dev/null || true
}
trap cleanup EXIT

echo "==> Building test image..."
podman build -f sdk/test/Dockerfile.integration -t "$IMAGE_NAME" .

echo "==> Starting systemd container..."
podman run -d --privileged "${CONTAINER_LIMITS[@]}" --name "$CONTAINER_NAME" "$IMAGE_NAME"

echo "==> Waiting for systemd to boot..."
for i in $(seq 1 30); do
    if podman exec "$CONTAINER_NAME" systemctl is-system-running --wait 2>/dev/null; then
        break
    fi
    sleep 0.5
done

echo "==> Running integration tests..."




TEST_LOCALE="${CADESTRO_TEST_LOCALE:-ja_JP.UTF-8}"
echo "    (locale: ${TEST_LOCALE})"







INTEGRATION_PKGS=(
    sdk/sys/exec
    sdk/sys/fs
    sdk/sys/user
    sdk/sys/service
)
for p in "${INTEGRATION_PKGS[@]}"; do
    if [ ! -d "$p" ]; then
        echo "ERROR: integration package path '$p' does not exist (stale after a rename/move?)" >&2
        exit 1
    fi
done

podman exec -w /workspace "$CONTAINER_NAME" \
    runuser -u cadestro -- env "LANG=${TEST_LOCALE}" "LC_ALL=${TEST_LOCALE}" \
        /usr/local/go/bin/go test -p 1 \
        -v -tags=integration -count=1 -timeout=10m \
        "${INTEGRATION_PKGS[@]/#/./}"
