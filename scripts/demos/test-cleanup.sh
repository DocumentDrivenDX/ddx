#!/usr/bin/env bash
# Test that register_demo_cleanup fires on early exit and signal, leaving no DEMO_DIR on disk
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PASS=0
FAIL=0

test_exit_cleanup() {
  local label="$1"
  local test_dir
  test_dir=$(mktemp -d)

  bash -c "
    DEMO_DIR='$test_dir'
    source '$SCRIPT_DIR/_lib.sh'
    register_demo_cleanup
    exit 1
  " 2>/dev/null || true

  if [[ -d "$test_dir" ]]; then
    echo "FAIL: $label — DEMO_DIR still exists after early exit: $test_dir"
    rm -rf "$test_dir"
    FAIL=$((FAIL + 1))
  else
    echo "PASS: $label"
    PASS=$((PASS + 1))
  fi
}

test_signal_cleanup() {
  local label="$1"
  local sig="$2"
  local test_dir
  test_dir=$(mktemp -d)

  bash -c "
    DEMO_DIR='$test_dir'
    source '$SCRIPT_DIR/_lib.sh'
    register_demo_cleanup
    sleep 30
  " 2>/dev/null &
  local pid=$!
  sleep 0.2
  kill "-$sig" "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true

  if [[ -d "$test_dir" ]]; then
    echo "FAIL: $label — DEMO_DIR still exists after $sig signal: $test_dir"
    rm -rf "$test_dir"
    FAIL=$((FAIL + 1))
  else
    echo "PASS: $label"
    PASS=$((PASS + 1))
  fi
}

# Verify early-exit cleanup for each demo script scenario
test_exit_cleanup "04-project-create early exit"
test_exit_cleanup "05-evolve-feature early exit"
test_exit_cleanup "06-full-journey early exit"
test_exit_cleanup "07-quickstart early exit"

# Verify signal-triggered cleanup
test_signal_cleanup "INT signal cleanup" "INT"
test_signal_cleanup "TERM signal cleanup" "TERM"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
