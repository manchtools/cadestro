#!/bin/bash















set -euo pipefail

ENGINE="${CONTAINER_ENGINE:-docker}"
DISTRO="${1:-debian}"
STATE="${2:-state-locked-apt}"
TEST_PATH="${3:-./sdk/pkg/}"
IMAGE="cadestro-sdk-container-${DISTRO}-${STATE}"
CONTAINER_LIMITS=(--memory=6g --cpus=4)



ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

echo "==> Building ${DISTRO}:${STATE} test image..."
"$ENGINE" build \
    -f "sdk/test/Dockerfile.${DISTRO}" \
    --target "${STATE}" \
    -t "${IMAGE}" \
    "$ROOT"

echo "==> Running container tests (${TEST_PATH}) inside ${STATE}..."


"$ENGINE" run --rm --shm-size=512m --cap-add NET_ADMIN "${CONTAINER_LIMITS[@]}" "${IMAGE}" \
    go test -p 1 -tags=container -count=1 -v "${TEST_PATH}" -run Container
