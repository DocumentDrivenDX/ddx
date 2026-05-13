#!/usr/bin/env bash
# Test that register_demo_cleanup fires on early exit and signal, leaving no DEMO_DIR on disk
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REAL_GIT="$(command -v git)"

PASS=0
FAIL=0

make_fake_mktemp() {
  local fake_bin="$1"
  local demo_dir="$2"

  cat >"$fake_bin/mktemp" <<EOF
#!/usr/bin/env bash
mkdir -p "$demo_dir"
printf '%s\n' "$demo_dir"
EOF
  chmod +x "$fake_bin/mktemp"
}

make_fake_git() {
  local fake_bin="$1"

  cat >"$fake_bin/git" <<EOF
#!/usr/bin/env bash
if [[ "\${1:-}" == "init" ]]; then
  exit 1
fi
exec "$REAL_GIT" "\$@"
EOF
  chmod +x "$fake_bin/git"
}

test_setup_demo_dir_cleanup() {
  local label="$1"
  local sandbox_dir fake_bin demo_dir
  sandbox_dir=$(mktemp -d)
  fake_bin="$sandbox_dir/bin"
  demo_dir="$sandbox_dir/demo"
  mkdir -p "$fake_bin"
  make_fake_mktemp "$fake_bin" "$demo_dir"
  make_fake_git "$fake_bin"

  bash -c "
    set -euo pipefail
    PATH='$fake_bin:$PATH'
    source '$SCRIPT_DIR/_lib.sh'
    setup_demo_dir
  " 2>/dev/null || true

  if [[ -d "$demo_dir" ]]; then
    echo "FAIL: $label — DEMO_DIR still exists after setup failure: $demo_dir"
    rm -rf "$sandbox_dir"
    FAIL=$((FAIL + 1))
  else
    echo "PASS: $label"
    PASS=$((PASS + 1))
  fi

  rm -rf "$sandbox_dir"
}

test_direct_registration_cleanup() {
  local label="$1"
  local sandbox_dir fake_bin demo_dir
  sandbox_dir=$(mktemp -d)
  fake_bin="$sandbox_dir/bin"
  demo_dir="$sandbox_dir/demo"
  mkdir -p "$fake_bin"
  make_fake_mktemp "$fake_bin" "$demo_dir"
  make_fake_git "$fake_bin"

  bash -c "
    set -euo pipefail
    PATH='$fake_bin:$PATH'
    source '$SCRIPT_DIR/_lib.sh'
    DEMO_DIR=\$(mktemp -d)
    register_demo_cleanup
    cd \"\$DEMO_DIR\"
    git init -q
  " 2>/dev/null || true

  if [[ -d "$demo_dir" ]]; then
    echo "FAIL: $label — DEMO_DIR still exists after early git init failure: $demo_dir"
    rm -rf "$sandbox_dir"
    FAIL=$((FAIL + 1))
  else
    echo "PASS: $label"
    PASS=$((PASS + 1))
  fi

  rm -rf "$sandbox_dir"
}

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

# Verify cleanup during setup for helper-based demos and direct registration for the quickstart demo
test_setup_demo_dir_cleanup "04-project-create setup failure"
test_setup_demo_dir_cleanup "05-evolve-feature setup failure"
test_setup_demo_dir_cleanup "06-full-journey setup failure"
test_direct_registration_cleanup "07-quickstart init failure"

# Verify signal-triggered cleanup
test_signal_cleanup "INT signal cleanup" "INT"
test_signal_cleanup "TERM signal cleanup" "TERM"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
