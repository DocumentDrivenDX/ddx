#!/usr/bin/env bash
# Verify that code snippets from the microsite documentation actually work.
# Run from repo root: bash scripts/test-doc-snippets.sh
set -e

PASS=0
FAIL=0
SKIP=0

check() {
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then
    echo "  ✓ $desc"
    PASS=$((PASS + 1))
  else
    echo "  ✗ $desc (exit $?)"
    FAIL=$((FAIL + 1))
  fi
}

skip() {
  echo "  - $1 (skipped: $2)"
  SKIP=$((SKIP + 1))
}

# Setup: create a temp project
export GIT_TEMPLATE_DIR=""
TESTDIR=$(mktemp -d)
cd "$TESTDIR"
git init -q && git config user.email "t@t" && git config user.name "T"
echo "# T" > README.md && git add . && git commit -q -m "init"

echo "=== Setup commands ==="
check "ddx init"           ddx init
check "ddx doctor"         ddx doctor
check "ddx status"         ddx status
check "ddx list"           ddx list

echo ""
echo "=== Package registry ==="
check "ddx search workflow" ddx search workflow
check "ddx installed"       ddx installed
# ddx install helix requires network — skip in quick mode
if [ "${FULL_TEST:-0}" = "1" ]; then
  check "ddx install helix" ddx install helix
else
  skip "ddx install helix" "set FULL_TEST=1 for network tests"
fi

echo ""
echo "=== Beads ==="
BEAD_ID=$(ddx bead create "Test bead" --type task --labels "helix,phase:build" --acceptance "test" 2>/dev/null | head -1)
check "ddx bead create"     test -n "$BEAD_ID"
check "ddx bead list"       ddx bead list
check "ddx bead show"       ddx bead show "$BEAD_ID"
check "ddx bead ready"      ddx bead ready
check "ddx bead close"      ddx bead close "$BEAD_ID"

echo ""
echo "=== Agent ==="
check "ddx agent list"         ddx agent list
check "ddx agent usage"        ddx agent usage
check "ddx agent usage --help" ddx agent usage --help

echo ""
echo "=== Documents ==="
check "ddx doc --help"         ddx doc --help
check "ddx checkpoint --help"  ddx checkpoint --help

echo ""
echo "=== Config ==="
check "ddx config --help"     ddx config --help

# Cleanup
cd / && rm -rf "$TESTDIR"

echo ""
echo "=== Results ==="
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
echo "  Skipped: $SKIP"
[ $FAIL -eq 0 ] || exit 1
