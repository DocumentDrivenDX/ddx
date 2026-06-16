#!/usr/bin/env bash
# server-managed-workers-docker.sh
#
# Docker-backed integration proof that server-managed worker cleanup leaves no
# process leaks. It builds `ddx` from the CURRENT source tree, bakes it plus the
# in-container scenario into a throwaway image, and runs the scenario in an
# isolated container (so the no-leak assertions see only this test's processes,
# never host tmux / host agent sessions).
#
# Cleanup paths proven by the scenario: explicit stop, double stop (no-op),
# watchdog reap, and graceful server shutdown. No Claude/Codex credentials are
# required — fake claude/codex binaries are placed first on PATH inside the
# container.
#
# Exit codes:
#   0  scenario passed (no leaks) OR Docker is unavailable (skipped — printed)
#   1  scenario failed (a leak was detected, or the build/run errored)
#
# When Docker is unavailable this script SKIPS with exit 0 so it is safe in
# environments without a daemon; CI/release gates run it on a Docker-enabled
# host where the scenario actually executes.
set -euo pipefail

SKIP_MARKER="SKIP: docker unavailable"

if ! command -v docker >/dev/null 2>&1; then
	echo "$SKIP_MARKER (docker CLI not found)"
	exit 0
fi
if ! docker info >/dev/null 2>&1; then
	echo "$SKIP_MARKER (docker daemon not reachable)"
	exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CLI_DIR="$REPO_ROOT/cli"
SCENARIO="$SCRIPT_DIR/server-managed-workers-scenario.sh"

if [[ ! -f "$SCENARIO" ]]; then
	echo "FAIL: scenario script not found at $SCENARIO" >&2
	exit 1
fi

# Match the container architecture so the prebuilt binary runs natively.
GOARCH="$(go env GOARCH 2>/dev/null || echo "")"
BASE_IMAGE="${DDX_LEAKTEST_BASE_IMAGE:-alpine/git:latest}"

BUILD_CTX="$(mktemp -d)"
IMAGE_TAG="ddx-server-managed-worker-leaktest:$$"
cleanup() {
	docker image rm -f "$IMAGE_TAG" >/dev/null 2>&1 || true
	rm -rf "$BUILD_CTX"
}
trap cleanup EXIT

echo "==> Building ddx from current source (CGO disabled, static)"
( cd "$CLI_DIR" && CGO_ENABLED=0 GOARCH="${GOARCH:-$(go env GOARCH)}" \
	go build -o "$BUILD_CTX/ddx" ./ )

cp "$SCENARIO" "$BUILD_CTX/scenario.sh"
chmod +x "$BUILD_CTX/scenario.sh"

cat > "$BUILD_CTX/Dockerfile" <<EOF
FROM $BASE_IMAGE
ENTRYPOINT []
COPY ddx /usr/local/bin/ddx
COPY scenario.sh /scenario.sh
RUN chmod +x /usr/local/bin/ddx /scenario.sh
EOF

echo "==> Building scenario image $IMAGE_TAG"
docker build -q -t "$IMAGE_TAG" "$BUILD_CTX" >/dev/null

echo "==> Running isolated scenario container"
set +e
docker run --rm --entrypoint /bin/sh "$IMAGE_TAG" /scenario.sh
rc=$?
set -e

if [[ "$rc" -eq 0 ]]; then
	echo "==> PASS: server-managed worker cleanup left no process leaks"
else
	echo "==> FAIL: scenario reported a process leak (exit $rc)" >&2
fi
exit "$rc"
