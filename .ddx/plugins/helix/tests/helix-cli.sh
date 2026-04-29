#!/usr/bin/env bash
set -euo pipefail

export DDX_DISABLE_UPDATE_CHECK=1

# Wrap ddx to filter out the update-available notice that ddx prints to stdout
# (ddx bug: notice goes to stdout instead of stderr, corrupts JSON/arithmetic).
ddx() {
  local _ddx_out _ddx_rc=0
  _ddx_out="$(command ddx "$@")" || _ddx_rc=$?
  printf '%s\n' "$_ddx_out" | grep -v '^💡\|^⬆️' || true
  return "$_ddx_rc"
}
export -f ddx

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
test_count=0

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local expected="$1"
  local actual="$2"
  local message="$3"
  if [[ "$expected" != "$actual" ]]; then
    printf 'expected:\n%s\nactual:\n%s\n' "$expected" "$actual" >&2
    fail "$message"
  fi
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local message="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    printf 'missing substring: %s\nin:\n%s\n' "$needle" "$haystack" >&2
    fail "$message"
  fi
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  local message="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    printf 'unexpected substring: %s\nin:\n%s\n' "$needle" "$haystack" >&2
    fail "$message"
  fi
}

assert_file_exists() {
  local path="$1"
  local message="$2"
  [[ -f "$path" ]] || fail "$message"
}

assert_file_contains() {
  local path="$1"
  local needle="$2"
  local message="$3"
  [[ -f "$path" ]] || fail "$message"
  local content
  content="$(cat "$path")"
  assert_contains "$content" "$needle" "$message"
}

assert_fails() {
  local message="$1"
  shift
  if "$@"; then
    fail "$message"
  fi
}

make_mock_bin() {
  local root="$1"
  mkdir -p "$root/bin" "$root/state"

  cat >"$root/bin/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

state_root="${MOCK_STATE_ROOT:?}"

if [[ "$*" != *"--dangerously-bypass-approvals-and-sandbox"* ]]; then
  echo "mock codex expected --dangerously-bypass-approvals-and-sandbox" >&2
  exit 1
fi


payload="$*"
mode="${MOCK_BACKFILL_MODE:-complete}"

record() {
  printf '%s\n' "$1" >> "$state_root/calls.log"
}

next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then
    echo STOP
    return
  fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}

case "$payload" in
  *"implementation action"*)
    record implement
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    echo "Recommended Command: mock"
    ;;
  *"alignment action"*)
    if [[ "${MOCK_ALIGN_FAIL:-0}" == "1" ]]; then
      echo "mock alignment failure" >&2
      exit 1
    fi
    record align
    echo "alignment complete"
    ;;
  *"backfill action"*)
    record backfill
    case "$mode" in
      complete)
        mkdir -p docs/helix/06-iterate/backfill-reports
        report="docs/helix/06-iterate/backfill-reports/BF-2099-01-01-repo.md"
        printf '# mock backfill report\n' > "$report"
        echo "Backfill Metadata"
        echo "BACKFILL_STATUS: COMPLETE"
        echo "BACKFILL_REPORT: $report"
        echo "RESEARCH_EPIC: hx-mock-backfill"
        ;;
      guidance)
        mkdir -p docs/helix/06-iterate/backfill-reports
        report="docs/helix/06-iterate/backfill-reports/BF-2099-01-01-guidance.md"
        printf '# mock guidance report\n' > "$report"
        echo "Backfill Metadata"
        echo "BACKFILL_STATUS: GUIDANCE_NEEDED"
        echo "BACKFILL_REPORT: $report"
        echo "RESEARCH_EPIC: hx-mock-backfill"
        ;;
      missing-report)
        echo "Backfill Metadata"
        echo "BACKFILL_STATUS: COMPLETE"
        echo "RESEARCH_EPIC: hx-mock-backfill"
        ;;
      blocked)
        mkdir -p docs/helix/06-iterate/backfill-reports
        report="docs/helix/06-iterate/backfill-reports/BF-2099-01-01-blocked.md"
        printf '# mock blocked report\n' > "$report"
        echo "Backfill Metadata"
        echo "BACKFILL_STATUS: BLOCKED"
        echo "BACKFILL_REPORT: $report"
        echo "RESEARCH_EPIC: hx-mock-backfill"
        ;;
      *)
        echo "unsupported mock backfill mode: $mode" >&2
        exit 1
        ;;
    esac
    ;;
  *"plan action"*)
    record plan
    echo "PLAN_STATUS: CONVERGED"
    echo "PLAN_DOCUMENT: docs/helix/02-design/plan-2099-01-01-repo.md"
    echo "PLAN_ROUNDS: 5"
    ;;
  *"polish action"*)
    record polish
    echo "POLISH_STATUS: CONVERGED"
    echo "POLISH_ROUNDS: 4"
    echo "ISSUES_MODIFIED: 3"
    echo "ISSUES_CREATED: 1"
    echo "ISSUES_MERGED: 2"
    ;;
  *"fresh-eyes review"*)
    record review
    echo "REVIEW_STATUS: CLEAN"
    echo "ISSUES_COUNT: 0"
    ;;
  *"experiment action"*)
    record experiment
    echo "EXPERIMENT_STATUS: ITERATION_COMPLETE"
    echo "EXPERIMENT_ITERATIONS: 1"
    echo "EXPERIMENT_KEPT: 0"
    echo "EXPERIMENT_BEST: test_runtime=4.2s (-0.0% vs baseline)"
    echo "EXPERIMENT_CONFIDENCE: 0.0"
    ;;
  *)
    record other
    echo "mock codex"
    ;;
esac
EOF

  cat >"$root/bin/claude" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

state_root="${MOCK_STATE_ROOT:?}"

if [[ "$*" != *"--dangerously-skip-permissions"* ]]; then
  echo "mock claude expected --dangerously-skip-permissions" >&2
  exit 1
fi

if [[ "$*" != *"--no-session-persistence"* ]]; then
  echo "mock claude expected --no-session-persistence" >&2
  exit 1
fi

if [[ "${MOCK_CLAUDE_SLEEP:-0}" -gt 0 ]]; then
  sleep "$MOCK_CLAUDE_SLEEP" &
  wait $!
  exit 0
fi

stdin_payload=""
if ! [[ -t 0 ]]; then
  stdin_payload="$(cat)"
fi
payload="$* $stdin_payload"
mode="${MOCK_BACKFILL_MODE:-complete}"

record() {
  printf '%s\n' "$1" >> "$state_root/calls.log"
}

next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then
    echo STOP
    return
  fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}

case "$payload" in
  *"implementation action"*)
    record implement
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    echo "Recommended Command: mock"
    ;;
  *"alignment action"*)
    if [[ "${MOCK_ALIGN_FAIL:-0}" == "1" ]]; then
      echo "mock alignment failure" >&2
      exit 1
    fi
    record align
    echo "alignment complete"
    ;;
  *"backfill action"*)
    record backfill
    case "$mode" in
      complete)
        mkdir -p docs/helix/06-iterate/backfill-reports
        report="docs/helix/06-iterate/backfill-reports/BF-2099-01-01-repo.md"
        printf '# mock backfill report\n' > "$report"
        echo "Backfill Metadata"
        echo "BACKFILL_STATUS: COMPLETE"
        echo "BACKFILL_REPORT: $report"
        echo "RESEARCH_EPIC: hx-mock-backfill"
        ;;
      *)
        echo "unsupported mock backfill mode: $mode" >&2
        exit 1
        ;;
    esac
    ;;
  *"plan action"*)
    record plan
    echo "PLAN_STATUS: CONVERGED"
    echo "PLAN_DOCUMENT: docs/helix/02-design/plan-2099-01-01-repo.md"
    echo "PLAN_ROUNDS: 5"
    ;;
  *"polish action"*)
    record polish
    echo "POLISH_STATUS: CONVERGED"
    echo "POLISH_ROUNDS: 4"
    echo "ISSUES_MODIFIED: 3"
    echo "ISSUES_CREATED: 1"
    echo "ISSUES_MERGED: 2"
    ;;
  *"fresh-eyes review"*)
    record review
    echo "REVIEW_STATUS: CLEAN"
    echo "ISSUES_COUNT: 0"
    ;;
  *"experiment action"*)
    record experiment
    echo "EXPERIMENT_STATUS: ITERATION_COMPLETE"
    echo "EXPERIMENT_ITERATIONS: 1"
    echo "EXPERIMENT_KEPT: 0"
    echo "EXPERIMENT_BEST: test_runtime=4.2s (-0.0% vs baseline)"
    echo "EXPERIMENT_CONFIDENCE: 0.0"
    ;;
  *)
    record other
    echo "mock claude"
    ;;
esac
EOF

  cat >"$root/bin/bd" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" != "list" ]] || [[ "${2:-}" != "--json" ]]; then
  echo "mock bd only supports: bd list --json" >&2
  exit 1
fi

if [[ "${MOCK_BD_FAIL:-0}" == "1" ]]; then
  echo "mock bd failure" >&2
  exit 1
fi

printf '%s\n' "${MOCK_BD_LIST_JSON:-[]}"
EOF

  cat >"$root/bin/br" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" != "list" ]] || [[ "${2:-}" != "--json" ]]; then
  echo "mock br only supports: br list --json" >&2
  exit 1
fi

if [[ "${MOCK_BR_FAIL:-0}" == "1" ]]; then
  echo "mock br failure" >&2
  exit 1
fi

printf '%s\n' "${MOCK_BR_LIST_JSON:-[]}"
EOF

  chmod +x "$root/bin/codex" "$root/bin/claude" "$root/bin/bd" "$root/bin/br"
}

make_workspace() {
  cd "$repo_root" 2>/dev/null || true
  local root
  root="$(mktemp -d)"
  mkdir -p "$root/work"
  make_mock_bin "$root"
  (
    cd "$root/work"
    git init -q
  )
  echo "$root"
}

# Seed the built-in tracker with ready issues for loop tests
seed_tracker() {
  local root="$1"
  local count="${2:-0}"
  local work_dir="$root/work"
  mkdir -p "$work_dir/.ddx"
  local i
  for ((i = 0; i < count; i++)); do
    printf '{"id":"hx-mock-%d","title":"mock issue %d","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"","spec-id":"","description":"","design":"","acceptance":"","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n' "$i" "$i"
  done > "$work_dir/.ddx/beads.jsonl"
}

# Close all tracker issues (simulates agent completing work)
close_all_issues() {
  local work_dir="$1/work"
  if [[ -f "$work_dir/.ddx/beads.jsonl" ]]; then
    local tmp
    tmp="$(ddx jq -c '.status = "closed"' "$work_dir/.ddx/beads.jsonl")"
    printf '%s\n' "$tmp" > "$work_dir/.ddx/beads.jsonl"
  fi
}

run_helix() {
  local root="$1"
  shift
  local cmd="${1:-help}"
  shift || true
  (
    cd "$root/work"
    HOME="$root/home" \
    PATH="$root/bin:$PATH" \
    MOCK_STATE_ROOT="$root/state" \
    HELIX_LIBRARY_ROOT="$repo_root/workflows" \
    HELIX_FORCE_EPHEMERAL=1 \
    HELIX_REVIEW_AGENT="codex" \
    HELIX_ALT_AGENT="none" \
    HELIX_DIRECT_AGENT=1 \
    HELIX_AUTO_ALIGN="${HELIX_AUTO_ALIGN:-0}" \
    DDX_BEAD_DIR="$root/work/.ddx" \
    DDX_DISABLE_UPDATE_CHECK=1 \
    bash "$repo_root/scripts/helix" "$cmd" --quiet "$@"
  )
}

run_bead() {
  local root="$1"
  shift
  (
    cd "$root/work"
    HOME="$root/home" \
    PATH="$root/bin:$PATH" \
    MOCK_STATE_ROOT="$root/state" \
    HELIX_LIBRARY_ROOT="$repo_root/workflows" \
    HELIX_FORCE_EPHEMERAL=1 \
    DDX_BEAD_DIR="$root/work/.ddx" \
    DDX_DISABLE_UPDATE_CHECK=1 \
    ddx bead "$@"
  )
}

run_helix_with_env() {
  local root="$1"
  local env_name="$2"
  local env_value="$3"
  shift 3
  local cmd="${1:-help}"
  shift || true
  (
    cd "$root/work"
    env \
      HOME="$root/home" \
      PATH="$root/bin:$PATH" \
      MOCK_STATE_ROOT="$root/state" \
      HELIX_LIBRARY_ROOT="$repo_root/workflows" \
      "DDX_BEAD_DIR=$root/work/.ddx" \
      "DDX_DISABLE_UPDATE_CHECK=1" \
      "HELIX_DIRECT_AGENT=1" \
      "$env_name=$env_value" \
      bash "$repo_root/scripts/helix" "$cmd" --quiet "$@"
  )
}

run_helix_with_envs() {
  local root="$1"
  shift
  local env_args=()
  while [[ $# -gt 0 ]] && [[ "$1" != "--" ]]; do
    env_args+=("$1")
    shift
  done
  if [[ "${1:-}" != "--" ]]; then
    fail "run_helix_with_envs requires -- before command arguments"
  fi
  shift
  local cmd="${1:-help}"
  shift || true
  (
    cd "$root/work"
    env \
      HOME="$root/home" \
      PATH="$root/bin:$PATH" \
      MOCK_STATE_ROOT="$root/state" \
      HELIX_LIBRARY_ROOT="$repo_root/workflows" \
      "DDX_BEAD_DIR=$root/work/.ddx" \
      "DDX_DISABLE_UPDATE_CHECK=1" \
      "HELIX_DIRECT_AGENT=1" \
      "${env_args[@]}" \
      bash "$repo_root/scripts/helix" "$cmd" --quiet "$@"
  )
}

# ── Tracker unit tests ──────────────────────────────────────────────





# ── CLI integration tests ───────────────────────────────────────────

test_help() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" help)"
  assert_contains "$output" "helix run" "help should list run command"
  assert_contains "$output" "--review-every" "help should list review option"
  assert_not_contains "$output" $'\n  tracker ' "help should not list tracker command"
  rm -rf "$root"
}

test_bead_help() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_bead "$root" --help)"
  assert_contains "$output" "Manage beads" "bead help should come from ddx bead"
  assert_contains "$output" "ddx bead create" "bead help should list create"
  assert_contains "$output" "ddx bead import" "bead help should list import"
  assert_contains "$output" "Export beads as JSONL" "bead help should list export"
  rm -rf "$root"
}

test_check_dry_run() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" check --dry-run repo)"
  assert_contains "$output" "codex --dangerously-bypass-approvals-and-sandbox exec -C" "dry-run should print codex command"
  assert_contains "$output" "actions/check.md" "dry-run should reference check action"
  assert_contains "$output" "BUILD, DESIGN, POLISH, ALIGN, BACKFILL, WAIT, GUIDANCE, or STOP" "check dry-run should advertise the full NEXT_ACTION contract"
  rm -rf "$root"
}

test_backfill_dry_run() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" backfill --dry-run repo)"
  assert_contains "$output" "actions/backfill-helix-docs.md" "backfill dry-run should reference backfill action"
  assert_contains "$output" "This is a writable live session" "backfill dry-run should assert writable live execution"
  assert_contains "$output" "BACKFILL_STATUS" "backfill dry-run should require machine-readable trailer"
  rm -rf "$root"
}

test_quickstart_demo_script_exists() {
  local demo_script="$repo_root/docs/demos/helix-quickstart/demo.sh"
  [[ -f "$demo_script" ]] || fail "demo script should exist"
  [[ -x "$demo_script" ]] || [[ "$(head -1 "$demo_script")" == "#!/usr/bin/env bash" ]] || fail "demo script should be executable or have bash shebang"
  local dockerfile="$repo_root/docs/demos/helix-quickstart/Dockerfile"
  [[ -f "$dockerfile" ]] || fail "Dockerfile should exist"
  assert_file_contains "$demo_script" "ddx bead init" "demo should init the tracker"
  assert_file_contains "$demo_script" "ddx bead create" "demo should create tracker issues"
  assert_file_contains "$demo_script" "product-vision" "demo should create a vision doc"
  assert_file_contains "$demo_script" "prd.md" "demo should create a PRD"
}

test_run_stops_after_queue_drains() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 2
  printf 'STOP\n' > "$root/state/next-actions"

  # The mock agent's implement call doesn't close issues, so we need to
  # close them after each implement call. We'll seed with 1 issue and
  # have the mock close it by manipulating the tracker file.
  seed_tracker "$root" 1

  # Override codex to also close the issue after implementing
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    # Close all open issues to simulate completing work
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *"alignment action"*)
    if [[ "${MOCK_ALIGN_FAIL:-0}" == "1" ]]; then
      echo "mock alignment failure" >&2; exit 1
    fi
    record align
    echo "alignment complete"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --no-auto-review --no-auto-align 2>&1)"

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'implement\ncheck' "$calls" "run should implement until drained, then check"
  assert_contains "$output" "helix: stopping after check returned STOP" "run should report why it stopped"
  rm -rf "$root"
}

test_run_stops_on_queue_drain_check_failure() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf '1\n' > "$root/state/check-fails"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    if [[ -f "$state_root/check-fails" ]]; then
      echo "mock check failure" >&2
      exit 1
    fi
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output rc=0
  output="$(run_helix "$root" run --no-auto-review --no-auto-align 2>&1)" || rc=$?
  [[ "$rc" -ne 0 ]] || fail "run should fail when queue-drain check fails"
  assert_contains "$output" "helix: check failed after queue drain" \
    "run should surface the queue-drain check failure"
  assert_not_contains "$output" "unbound variable" \
    "run should not trip a set -u unbound-variable error"

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'implement\ncheck' "$calls" "run should implement then fail on queue-drain check"
  rm -rf "$root"
}

test_run_periodic_alignment() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 2
  printf 'STOP\n' > "$root/state/next-actions"

  # Mock agent that closes one issue per implement call
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    # Close first open issue
    if [[ -f .ddx/beads.jsonl ]]; then
      first_open="$(ddx jq -r 'select(.status == "open") | .id' .ddx/beads.jsonl | head -1)"
      if [[ -n "$first_open" ]]; then
        tmp="$(ddx jq -c "if .id == \"$first_open\" then .status = \"closed\" else . end" .ddx/beads.jsonl)"
        printf '%s\n' "$tmp" > .ddx/beads.jsonl
      fi
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *"alignment action"*)
    if [[ "${MOCK_ALIGN_FAIL:-0}" == "1" ]]; then
      echo "mock alignment failure" >&2; exit 1
    fi
    record align
    echo "alignment complete"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  run_helix "$root" run --review-every 2 --no-auto-review --no-auto-align >/dev/null

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'implement\nimplement\nalign\ncheck' "$calls" "periodic alignment should run after configured cycles"
  rm -rf "$root"
}

test_run_auto_aligns_once() {
  local root
  root="$(make_workspace)"
  # No issues — queue is empty from the start
  printf 'ALIGN\nSTOP\n' > "$root/state/next-actions"

  HELIX_AUTO_ALIGN=1 run_helix "$root" run --no-auto-review >/dev/null

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'check\nalign\ncheck' "$calls" "run should auto-align once when check returns ALIGN"
  rm -rf "$root"
}

test_run_dispatches_plan_after_queue_drain() {
  local root
  root="$(make_workspace)"
  printf 'DESIGN\nBUILD\nSTOP\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *"plan action"*)
    record plan
    mkdir -p .helix
    cat > .ddx/beads.jsonl <<'EOF'
{"id":"hx-planned","title":"planned issue","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"","spec-id":"SD-001","description":"","design":"","acceptance":"","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}
EOF
    echo "PLAN_STATUS: CONVERGED"
    echo "PLAN_DOCUMENT: docs/helix/02-design/plan-2099-01-01-repo.md"
    ;;
  *"implementation action"*"Implementation target: hx-planned"*)
    record implement
    tmp="$(ddx jq -c 'if .id == "hx-planned" then .status = "closed" else . end' .ddx/beads.jsonl)"
    printf '%s\n' "$tmp" > .ddx/beads.jsonl
    echo "implementation complete"
    ;;
  *"implementation action"*)
    echo "implementation targeted wrong issue after plan" >&2
    exit 1
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --no-auto-review 2>&1)"

  local calls
  calls="$(cat "$root/state/calls.log")"
  case "$calls" in
    check$'\n'plan$'\n'implement$'\n'check*|check$'\n'plan$'\n'check$'\n'implement$'\n'check*)
      ;;
    *)
      printf 'unexpected calls:\n%s\n' "$calls" >&2
      fail "run should dispatch plan before bounded implementation resumes"
      ;;
  esac
  assert_contains "$output" "queue drained, running design" "run should report design dispatch"
  rm -rf "$root"
}

test_run_dispatches_polish_after_queue_drain() {
  local root
  root="$(make_workspace)"
  mkdir -p "$root/work/.ddx"
  cat >"$root/work/.ddx/beads.jsonl" <<'EOF'
{"id":"hx-refine","title":"refine queue","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:design"],"parent":"","spec-id":"SD-001","description":"","design":"","acceptance":"","dependencies":[],"owner":"","notes":"","execution-eligible":false,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}
EOF
  printf 'POLISH\nBUILD\nSTOP\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *"polish action"*)
    record polish
    cat > .ddx/beads.jsonl <<'EOF'
{"id":"hx-polished","title":"polished issue","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"","spec-id":"SD-001","description":"","design":"","acceptance":"","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}
EOF
    echo "POLISH_STATUS: CONVERGED"
    echo "POLISH_ROUNDS: 4"
    ;;
  *"implementation action"*"Implementation target: hx-polished"*)
    record implement
    tmp="$(ddx jq -c 'if .id == "hx-polished" then .status = "closed" else . end' .ddx/beads.jsonl)"
    printf '%s\n' "$tmp" > .ddx/beads.jsonl
    echo "implementation complete"
    ;;
  *"implementation action"*)
    echo "implementation targeted wrong issue after polish" >&2
    exit 1
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --no-auto-review 2>&1)"

  local calls
  calls="$(cat "$root/state/calls.log")"
  case "$calls" in
    check$'\n'polish$'\n'implement$'\n'check*|check$'\n'polish$'\n'check$'\n'implement$'\n'check*)
      ;;
    *)
      printf 'unexpected calls:\n%s\n' "$calls" >&2
      fail "run should dispatch polish before bounded implementation resumes"
      ;;
  esac
  assert_contains "$output" "queue drained, running polish" "run should report polish dispatch"
  rm -rf "$root"
}

test_run_reports_periodic_alignment_failure() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  # Mock that closes issue on implement
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"alignment action"*)
    echo "mock alignment failure" >&2; exit 1
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  if output="$(HELIX_AUTO_ALIGN=1 run_helix "$root" run --review-every 1 --no-auto-review 2>&1)"; then
    fail "run should fail when periodic alignment fails"
  fi

  assert_contains "$output" "mock alignment failure" "run should surface the alignment failure"
  assert_contains "$output" "helix: periodic alignment failed after 1 cycles" "run should report why the loop exited"
  rm -rf "$root"
}

test_run_stops_on_queue_drift_before_close() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.["spec-id"] = "TP-DRIFT" | .status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    printf 'NEXT_ACTION: STOP\n'
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --no-auto-review 2>&1)"
  assert_contains "$output" "queue drift detected" "run should report queue drift"
  assert_contains "$output" "after queue drift" "run should skip drifted issue and continue"

  local status
  status="$(run_bead "$root" show hx-mock-0 --json | ddx jq -r '.status')"
  assert_eq "closed" "$status" "run should leave a closed bead closed when drift is non-supersession governance change"

  assert_contains "$output" "BLOCKERS" "run should report blockers at the end"
  assert_contains "$output" "hx-mock-0" "blocker report should name the drifted issue"
  rm -rf "$root"
}

test_run_stops_on_queue_drift_before_close_without_closing() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.["spec-id"] = "TP-DRIFT"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    printf 'NEXT_ACTION: STOP\n'
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --no-auto-review 2>&1)"
  assert_contains "$output" "queue drift detected" "run should report drift even if the issue was not closed"
  assert_contains "$output" "after queue drift" "run should skip drifted issue and continue"

  local status
  status="$(run_bead "$root" show hx-mock-0 --json | ddx jq -r '.status')"
  assert_eq "open" "$status" "run should leave the issue open for re-check"
  rm -rf "$root"
}

test_run_skips_execution_ineligible_ready_work() {
  local root
  root="$(make_workspace)"
  mkdir -p "$root/work/.ddx"
  cat >"$root/work/.ddx/beads.jsonl" <<'EOF'
{"id":"hx-refine","title":"refinement","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:design"],"parent":"","spec-id":"","description":"","design":"","acceptance":"","dependencies":[],"owner":"","notes":"","execution-eligible":false,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}
{"id":"hx-build","title":"build","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"","spec-id":"","description":"","design":"","acceptance":"","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}
EOF
  printf 'STOP\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*"Implementation target: hx-build"*)
    record implement
    tmp="$(ddx jq -c 'if .id == "hx-build" then .status = "closed" else . end' .ddx/beads.jsonl)"
    printf '%s\n' "$tmp" > .ddx/beads.jsonl
    echo "implementation complete"
    ;;
  *"implementation action"*)
    echo "implementation targeted wrong issue" >&2
    exit 1
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  run_helix "$root" run --no-auto-review --no-auto-align >/dev/null

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'implement\ncheck' "$calls" "run should implement the eligible issue then check"

  local refine_status build_status
  refine_status="$(run_bead "$root" show hx-refine --json | ddx jq -r '.status')"
  build_status="$(run_bead "$root" show hx-build --json | ddx jq -r '.status')"
  assert_eq "open" "$refine_status" "run should not claim refinement work"
  assert_eq "closed" "$build_status" "run should execute the eligible build issue"
  rm -rf "$root"
}

test_run_stops_when_issue_is_superseded() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  # Override both agent mocks — helix dispatches to claude by default
  cat >"$root/bin/mock-supersession" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
stdin_payload=""
if ! [[ -t 0 ]]; then stdin_payload="$(cat)"; fi
payload="$* $stdin_payload"
case "$payload" in
  *"implementation action"*)
    record implement
    tmp="$(ddx jq -c '.["superseded-by"] = "hx-replacement" | .status = "closed"' .ddx/beads.jsonl)"
    printf '%s\n' "$tmp" > .ddx/beads.jsonl
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    printf 'NEXT_ACTION: STOP\n'
    ;;
  *) record other; echo "mock agent" ;;
esac
MOCK
  cp "$root/bin/mock-supersession" "$root/bin/codex"
  cp "$root/bin/mock-supersession" "$root/bin/claude"
  chmod +x "$root/bin/codex" "$root/bin/claude"

  local output
  output="$(run_helix "$root" run --no-auto-review 2>&1)"
  assert_contains "$output" "queue drift detected" "run should report supersession as drift"
  assert_contains "$output" "after queue drift" "run should skip superseded issue"

  local status superseded_by
  status="$(run_bead "$root" show hx-mock-0 --json | ddx jq -r '.status')"
  superseded_by="$(run_bead "$root" show hx-mock-0 --json | ddx jq -r '.["superseded-by"]')"
  assert_eq "open" "$status" "run should refuse stale close after supersession"
  assert_eq "hx-replacement" "$superseded_by" "supersession metadata should remain visible"
  rm -rf "$root"
}

run_backfill_missing_report() {
  local root="$1"
  MOCK_BACKFILL_MODE=missing-report run_helix "$root" backfill repo >/dev/null
}

test_backfill_requires_report_marker() {
  local root
  root="$(make_workspace)"
  assert_fails "backfill should fail when the trailer omits BACKFILL_REPORT" \
    run_backfill_missing_report "$root"
  rm -rf "$root"
}

test_backfill_creates_report() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" backfill repo)"
  assert_contains "$output" "BACKFILL_STATUS: COMPLETE" "backfill should report completion"
  assert_file_exists "$root/work/docs/helix/06-iterate/backfill-reports/BF-2099-01-01-repo.md" "backfill should create the declared report"
  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'backfill' "$calls" "backfill should invoke the backfill action once"
  rm -rf "$root"
}

test_doctor_reports_status() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" doctor 2>&1)" || true
  assert_contains "$output" "HELIX Doctor" "doctor should print header"
  assert_contains "$output" "workflows" "doctor should check workflows"
  rm -rf "$root"
}

test_skill_package_validation() {
  (
    cd "$repo_root"
    bash tests/validate-skills.sh >/dev/null
  )
}

test_claude_run_stops_after_queue_drains() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  # Mock claude that closes issues
  cat >"$root/bin/claude" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
stdin_payload=""
if ! [[ -t 0 ]]; then stdin_payload="$(cat)"; fi
payload="$* $stdin_payload"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *"alignment action"*)
    record align; echo "alignment complete" ;;
  *) record other; echo "mock claude" ;;
esac
MOCK
  chmod +x "$root/bin/claude"

  local output
  output="$(run_helix "$root" run --claude --no-auto-review --no-auto-align 2>&1)"

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'implement\ncheck' "$calls" "claude run should implement until drained, then check"
  assert_contains "$output" "helix: stopping after check returned STOP" "claude run should report why it stopped"
  rm -rf "$root"
}

test_claude_run_auto_aligns() {
  local root
  root="$(make_workspace)"
  printf 'ALIGN\nSTOP\n' > "$root/state/next-actions"

  HELIX_AUTO_ALIGN=1 run_helix "$root" run --claude --no-auto-review >/dev/null

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'check\nalign\ncheck' "$calls" "claude run should auto-align when check returns ALIGN"
  rm -rf "$root"
}

test_claude_check_dry_run() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" check --claude --dry-run repo)"
  assert_contains "$output" "claude -p --permission-mode bypassPermissions" "claude dry-run should print claude command"
  assert_contains "$output" "check action" "claude dry-run should reference check action"
  rm -rf "$root"
}

test_run_stops_on_wait() {
  local root
  root="$(make_workspace)"
  printf 'WAIT\n' > "$root/state/next-actions"

  local output
  output="$(run_helix "$root" run --no-auto-review 2>&1)"

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'check' "$calls" "run should stop immediately when check returns WAIT"
  assert_contains "$output" "stopping after check returned WAIT" "run should report terminal WAIT"
  rm -rf "$root"
}

test_run_dispatches_backfill() {
  local root
  root="$(make_workspace)"
  printf 'BACKFILL\nSTOP\n' > "$root/state/next-actions"

  local output
  output="$(run_helix "$root" run --no-auto-review 2>&1)"

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'check\nbackfill\ncheck' "$calls" "run should dispatch backfill inline and then re-check"
  assert_contains "$output" "BACKFILL_STATUS: COMPLETE" "run should print backfill completion output"
  assert_contains "$output" "stopping after check returned STOP" "run should continue after backfill and stop on the follow-up check"
  assert_not_contains "$output" "run \`helix backfill <scope>\` to reconstruct missing docs" "run should not print the old backfill handoff"
  rm -rf "$root"
}

test_run_stops_after_repeated_backfill() {
  local root
  root="$(make_workspace)"
  printf 'BACKFILL\nBACKFILL\n' > "$root/state/next-actions"

  local output
  output="$(run_helix "$root" run --no-auto-review 2>&1)"

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'check\nbackfill\ncheck' "$calls" "run should refuse to loop on repeated BACKFILL results"
  assert_contains "$output" "stopping after check returned BACKFILL" "run should stop when backfill is already drained once"
  rm -rf "$root"
}

test_run_max_cycles_counts_completed_cycles_only() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
attempt_file="$state_root/implement-attempts"
attempts=0
if [[ -f "$attempt_file" ]]; then
  attempts="$(cat "$attempt_file")"
fi
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    attempts=$((attempts + 1))
    printf '%s\n' "$attempts" > "$attempt_file"
    if (( attempts >= 2 )) && [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"alignment action"*)
    record align
    echo "alignment complete"
    ;;
  *"check action"*)
    record check
    echo "NEXT_ACTION: STOP"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --max-cycles 1 --no-auto-review 2>&1)"

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'implement\nimplement' "$calls" "max cycles should count only completed issue closures"
  assert_contains "$output" "did not complete a tracked issue" "run should distinguish attempted work from completed work"
  assert_contains "$output" "reached max cycles (1)" "run should stop after one completed cycle"
  rm -rf "$root"
}

test_run_periodic_alignment_ignores_failed_attempts() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
attempt_file="$state_root/implement-attempts"
attempts=0
if [[ -f "$attempt_file" ]]; then
  attempts="$(cat "$attempt_file")"
fi
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    attempts=$((attempts + 1))
    printf '%s\n' "$attempts" > "$attempt_file"
    if (( attempts == 1 )); then
      echo "mock implementation failure" >&2
      exit 1
    fi
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"alignment action"*)
    record align
    echo "alignment complete"
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --review-every 1 --no-auto-review --no-auto-align 2>&1)"

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'implement\nimplement\nalign\ncheck' "$calls" "periodic alignment should wait for a completed cycle, not a failed attempt"
  assert_contains "$output" "implementation failed during cycle 1" "run should report failed attempts without counting them as complete"
  rm -rf "$root"
}

test_run_recovery_preserves_unrelated_dirty_changes() {
  local root
  root="$(make_workspace)"
  local issue_id
  issue_id="$(run_bead "$root" create "Claimed task" --labels helix,phase:build --set spec-id=TEST --acceptance "test passes")"
  run_bead "$root" update "$issue_id" --claim >/dev/null

  (
    cd "$root/work"
    printf 'tracked\n' > keep.txt
    git add keep.txt
    git commit -qm "seed tracked file"
    printf 'tracked but dirty\n' > keep.txt
  )
  printf 'WAIT\n' > "$root/state/next-actions"

  local output
  output="$(run_helix "$root" run --no-auto-review 2>&1)"

  local status
  status="$(run_bead "$root" show "$issue_id" --json | ddx jq -r '.status')"
  assert_eq "in_progress" "$status" "fresh claims should remain claimed"
  assert_file_contains "$root/work/keep.txt" "tracked but dirty" "recovery should not revert unrelated dirty worktree changes"
  assert_contains "$output" "too fresh to reclaim" "run should report that claim is too fresh"
  rm -rf "$root"
}

test_extract_next_action_from_claude_output() {
  extract_next_action() {
    local stripped
    stripped="$(printf '%s\n' "$1" | sed 's/[*`]//g')"
    local result
    result="$(printf '%s\n' "$stripped" | grep -oE 'NEXT_ACTION: *(BUILD|DESIGN|POLISH|ALIGN|BACKFILL|WAIT|GUIDANCE|STOP)' | head -n1 | sed 's/^NEXT_ACTION: *//')"
    printf '%s' "$result"
  }

  local result
  result="$(extract_next_action "## Queue Health
NEXT_ACTION: BUILD
Target issue: hx-mock-1")"
  assert_eq "BUILD" "$result" "extract plain NEXT_ACTION"

  result="$(extract_next_action "**NEXT_ACTION: WAIT**")"
  assert_eq "WAIT" "$result" "extract bold-wrapped NEXT_ACTION"

  result="$(extract_next_action '`NEXT_ACTION: STOP`')"
  assert_eq "STOP" "$result" "extract backtick-wrapped NEXT_ACTION"

  result="$(extract_next_action '**NEXT_ACTION:** GUIDANCE')"
  assert_eq "GUIDANCE" "$result" "extract bold-key NEXT_ACTION"

  result="$(extract_next_action 'NEXT_ACTION: **ALIGN**')"
  assert_eq "ALIGN" "$result" "extract bold-value NEXT_ACTION"

  result="$(extract_next_action 'Some preamble
\`NEXT_ACTION: BACKFILL\`
Some epilogue')"
  assert_eq "BACKFILL" "$result" "extract code-inline NEXT_ACTION"

  result="$(extract_next_action 'NEXT_ACTION: DESIGN')"
  assert_eq "DESIGN" "$result" "extract DESIGN NEXT_ACTION"

  result="$(extract_next_action '**NEXT_ACTION:** POLISH')"
  assert_eq "POLISH" "$result" "extract POLISH NEXT_ACTION"
}

test_design_dry_run() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" design --dry-run repo)"
  assert_contains "$output" "actions/plan.md" "design dry-run should reference design action"
  assert_contains "$output" "Plan scope: repo" "design dry-run should include scope"
  assert_contains "$output" "Refinement rounds: 5" "design dry-run should include default rounds"
  rm -rf "$root"
}

test_design_custom_rounds() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" design --dry-run --rounds 8 auth)"
  assert_contains "$output" "Refinement rounds: 8" "design should accept custom rounds"
  assert_contains "$output" "Plan scope: auth" "design should accept scope argument"
  rm -rf "$root"
}

test_polish_dry_run() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" polish --dry-run)"
  assert_contains "$output" "actions/polish.md" "polish dry-run should reference polish action"
  assert_contains "$output" "Maximum refinement rounds: 6" "polish dry-run should include default rounds"
  rm -rf "$root"
}

test_review_dry_run() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" review --dry-run)"
  assert_contains "$output" "actions/fresh-eyes-review.md" "review dry-run should reference fresh-eyes action"
  assert_contains "$output" "Review scope: last-commit" "review dry-run should default to last-commit"
  rm -rf "$root"
}

test_review_custom_scope() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" review --dry-run src/auth/)"
  assert_contains "$output" "Review scope: src/auth/" "review should accept custom scope"
  rm -rf "$root"
}

test_next_shows_ready_issue() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  local output
  output="$(run_helix "$root" next 2>&1)"
  assert_contains "$output" "hx-mock-0" "next should show first ready issue ID"
  rm -rf "$root"
}

test_next_no_ready_issues() {
  local root
  root="$(make_workspace)"

  local output
  output="$(run_helix "$root" next 2>&1)"
  assert_contains "$output" "no ready issues" "next should report when no issues are ready"
  rm -rf "$root"
}

test_experiment_dry_run() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" experiment --dry-run)"
  assert_contains "$output" "actions/experiment.md" "experiment dry-run should reference experiment action"
  assert_contains "$output" "Experiment target" "experiment dry-run should include experiment target"
  [[ "$output" != *"CLOSE SESSION"* ]] || fail "experiment dry-run without --close must not include CLOSE SESSION"
  rm -rf "$root"
}

test_experiment_requires_clean_worktree() {
  local root
  root="$(make_workspace)"
  echo dirty > "$root/work/dirty.txt" && cd "$root/work" && git add dirty.txt

  local output
  if output="$(run_helix "$root" experiment 2>&1)"; then
    fail "experiment should fail with uncommitted changes"
  fi

  local found=0
  if [[ "$output" == *"uncommitted changes"* ]] || [[ "$output" == *"please commit"* ]]; then
    found=1
  fi
  [[ "$found" -eq 1 ]] || fail "experiment should mention uncommitted changes or please commit"
  rm -rf "$root"
}

test_experiment_close_dry_run() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" experiment --dry-run --close)"
  assert_contains "$output" "CLOSE SESSION" "experiment close dry-run should include CLOSE SESSION"
  rm -rf "$root"
}

test_claude_agent_timeout_kills_process() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  local output
  if output="$(HELIX_AGENT_TIMEOUT=1 MOCK_CLAUDE_SLEEP=30 run_helix "$root" run --claude 2>&1)"; then
    fail "claude run should fail when agent times out"
  fi

  assert_contains "$output" "agent timeout after" "run should report agent timeout"
  assert_contains "$output" "killing" "run should report killing the agent"
  rm -rf "$root"
}

test_build_prompt_references_tracker() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" build --dry-run)"
  assert_contains "$output" "ddx bead" "build prompt should reference ddx bead"
  assert_contains "$output" "issues.jsonl" "build prompt should reference JSONL file"
  assert_contains "$output" "re-read the selected issue immediately before claim and immediately before close" "build prompt should require pre-claim and pre-close revalidation"
  rm -rf "$root"
}

# ── Auto-review in loop tests ──────────────────────────────────────

test_run_auto_reviews_after_implement() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  # Mock codex that closes issue on implement and handles review
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"fresh-eyes review"*)
    record review
    echo "REVIEW_STATUS: CLEAN"
    echo "ISSUES_COUNT: 0"
    echo "AGENTS_MD_UPDATED: NO"
    echo "LEARNINGS_FILED: 0"
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *"alignment action"*)
    record align
    echo "alignment complete"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --review-threshold 0 2>&1)"

  local calls
  calls="$(cat "$root/state/calls.log")"
  # After implement, review runs (CLEAN). Then check returns STOP.
  # review-threshold=0 forces review even when mock makes no file changes.
  assert_eq $'implement\nreview\ncheck' "$calls" "run should review after each successful implementation"
  assert_contains "$output" "post-implementation review" "run should report review step"
  rm -rf "$root"
}

test_run_no_auto_review_flag() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  # Same mock as above
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  run_helix "$root" run --no-auto-review --no-auto-align >/dev/null

  local calls
  calls="$(cat "$root/state/calls.log")"
  assert_eq $'implement\ncheck' "$calls" "run --no-auto-review should skip review"
  rm -rf "$root"
}

test_run_stops_on_review_findings() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"fresh-eyes review"*)
    record review
    echo "REVIEW_STATUS: ISSUES_FOUND"
    echo "ISSUES_COUNT: 2"
    echo "AGENTS_MD_UPDATED: NO"
    echo "LEARNINGS_FILED: 1"
    ;;
  *"check action"*)
    record check
    echo "NEXT_ACTION: STOP"
    ;;
  *"alignment action"*)
    record align
    echo "alignment complete"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run 2>&1)"

  local calls
  calls="$(cat "$root/state/calls.log")"
  # Review findings are now filed as tracker issues and the loop continues.
  assert_eq $'implement\nreview\ncheck' "$calls" "run should continue after review files findings as tracker issues"
  assert_contains "$output" "review reported 2 issue(s), 0 filed as tracker issues" "run should surface review findings count"
  assert_contains "$output" "review filed actionable findings as tracker issues" "run should note findings were filed"
  rm -rf "$root"
}

test_run_fails_on_unparseable_review_output() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"fresh-eyes review"*)
    record review
    echo "review complete without trailer"
    ;;
  *"check action"*)
    record check
    echo "NEXT_ACTION: STOP"
    ;;
  *"alignment action"*)
    record align
    echo "alignment complete"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run 2>&1)"

  local calls
  calls="$(cat "$root/state/calls.log")"
  # Unparseable review output logs a warning but the loop continues.
  assert_eq $'implement\nreview\ncheck' "$calls" "run should continue after unparseable review output"
  assert_contains "$output" "review output missing REVIEW_STATUS trailer" "run should require REVIEW_STATUS"
  assert_contains "$output" "review output was not safely interpretable" "run should explain the issue"
  rm -rf "$root"
}

# ── Extended tracker tests ─────────────────────────────────────────

test_tracker_create_requires_title() {
  local root
  root="$(make_workspace)"
  assert_fails "create without title should fail" run_bead "$root" create 2>/dev/null
  rm -rf "$root"
}

test_tracker_create_help_no_side_effect() {
  local root output
  root="$(make_workspace)"
  output="$(run_bead "$root" create --help)"
  assert_contains "$output" "Create a new bead" "create --help should show ddx usage"
  # Must not create an issue
  local count=0
  if [[ -f "$root/.ddx/beads.jsonl" ]]; then
    count="$(wc -l < "$root/.ddx/beads.jsonl")"
  fi
  [[ "$count" -eq 0 ]] || fail "create --help must not create an issue (found $count)"
  rm -rf "$root"
}




























# ── Run all tests ──────────────────────────────────────────────────

run_test() {
  local name="$1"
  shift
  cd "$repo_root" 2>/dev/null || true
  "$@"
  test_count=$((test_count + 1))
  echo "ok - $name"
}

# Tracker unit tests
run_test "tracker create requires title" test_tracker_create_requires_title
run_test "tracker create --help no side effect" test_tracker_create_help_no_side_effect

# Beads interop tests

# Auto-review in loop tests
run_test "run auto-reviews after implement" test_run_auto_reviews_after_implement
run_test "run no-auto-review flag" test_run_no_auto_review_flag
run_test "run continues after review findings filed as issues" test_run_stops_on_review_findings
run_test "run continues after unparseable review output" test_run_fails_on_unparseable_review_output

# CLI integration tests
run_test "help" test_help
run_test "bead help" test_bead_help
run_test "check dry-run" test_check_dry_run
run_test "backfill dry-run" test_backfill_dry_run
run_test "quickstart demo script exists" test_quickstart_demo_script_exists
run_test "run stops after drain" test_run_stops_after_queue_drains
run_test "run stops on queue-drain check failure" test_run_stops_on_queue_drain_check_failure
run_test "periodic alignment" test_run_periodic_alignment
run_test "auto-align" test_run_auto_aligns_once
run_test "queue-drain plan dispatch" test_run_dispatches_plan_after_queue_drain
run_test "queue-drain polish dispatch" test_run_dispatches_polish_after_queue_drain
run_test "periodic alignment failure reason" test_run_reports_periodic_alignment_failure
run_test "run stops on queue drift before close" test_run_stops_on_queue_drift_before_close
run_test "run stops on queue drift without closing" test_run_stops_on_queue_drift_before_close_without_closing
run_test "run skips execution-ineligible ready work" test_run_skips_execution_ineligible_ready_work
run_test "run stops when issue is superseded" test_run_stops_when_issue_is_superseded
run_test "backfill requires report marker" test_backfill_requires_report_marker
run_test "backfill creates report" test_backfill_creates_report
run_test "doctor reports status" test_doctor_reports_status
run_test "claude run stops after drain" test_claude_run_stops_after_queue_drains
run_test "claude auto-align" test_claude_run_auto_aligns
run_test "claude check dry-run" test_claude_check_dry_run
run_test "run stops on WAIT" test_run_stops_on_wait
run_test "run dispatches backfill" test_run_dispatches_backfill
run_test "run stops after repeated BACKFILL" test_run_stops_after_repeated_backfill
run_test "max cycles count completed work only" test_run_max_cycles_counts_completed_cycles_only
run_test "periodic alignment ignores failed attempts" test_run_periodic_alignment_ignores_failed_attempts
run_test "extract NEXT_ACTION from claude output" test_extract_next_action_from_claude_output
run_test "design dry-run" test_design_dry_run
run_test "design custom rounds" test_design_custom_rounds
run_test "polish dry-run" test_polish_dry_run
run_test "review dry-run" test_review_dry_run
run_test "review custom scope" test_review_custom_scope
run_test "next shows ready issue" test_next_shows_ready_issue
run_test "next no ready issues" test_next_no_ready_issues
run_test "experiment dry-run" test_experiment_dry_run
run_test "experiment requires clean worktree" test_experiment_requires_clean_worktree
run_test "experiment close dry-run" test_experiment_close_dry_run
run_test "recovery preserves unrelated dirty changes" test_run_recovery_preserves_unrelated_dirty_changes
# TODO: timeout test hangs in CI — the mock claude sleep subprocess isn't
# killed reliably through the stdin pipe subshell. Fix the watchdog to
# kill the process group, then re-enable.
# run_test "claude agent timeout kills process" test_claude_agent_timeout_kills_process
run_test "build prompt references tracker" test_build_prompt_references_tracker

# ── triage passthrough tests ──────────────────────────────────────────

test_triage_passthrough_creates_issue() {
  local root
  root="$(make_workspace)"
  local id
  id="$(run_helix "$root" triage "Loose issue")"
  assert_contains "$id" "hx-" "triage should delegate to ddx bead create"
  rm -rf "$root"
}

run_test "triage passthrough creates issue" test_triage_passthrough_creates_issue

# ── evolve tests ──────────────────────────────────────────────────────

test_evolve_dry_run() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" evolve --dry-run "Add WAL compression")"
  assert_contains "$output" "actions/evolve.md" "evolve dry-run should reference evolve action"
  assert_contains "$output" "Add WAL compression" "evolve dry-run should include the requirement"
  assert_contains "$output" "EVOLUTION_STATUS" "evolve dry-run should require machine-readable trailer"
  rm -rf "$root"
}

test_evolve_dry_run_with_scope() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" evolve --dry-run --scope area:wal "WAL compression")"
  assert_contains "$output" "Scope: area:wal" "evolve dry-run should include scope"
  rm -rf "$root"
}

test_evolve_dry_run_with_artifact() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" evolve --dry-run --artifact SD-017 "Loom support")"
  assert_contains "$output" "Target artifact: SD-017" "evolve dry-run should include artifact"
  rm -rf "$root"
}

test_evolve_requires_description() {
  local root
  root="$(make_workspace)"
  assert_fails "evolve without description should fail" \
    run_helix "$root" evolve
  rm -rf "$root"
}

run_test "evolve dry-run" test_evolve_dry_run
run_test "evolve dry-run with scope" test_evolve_dry_run_with_scope
run_test "evolve dry-run with artifact" test_evolve_dry_run_with_artifact
run_test "evolve requires description" test_evolve_requires_description

# ── summary mode tests ───────────────────────────────────────────────

run_helix_summary() {
  local root="$1"
  shift
  local cmd="${1:-help}"
  shift || true
  (
    cd "$root/work"
    HOME="$root/home" \
    PATH="$root/bin:$PATH" \
    MOCK_STATE_ROOT="$root/state" \
    HELIX_LIBRARY_ROOT="$repo_root/workflows" \
    HELIX_FORCE_EPHEMERAL=1 \
    HELIX_REVIEW_AGENT="codex" \
    HELIX_ALT_AGENT="none" \
    HELIX_DIRECT_AGENT=1 \
    HELIX_AUTO_ALIGN="${HELIX_AUTO_ALIGN:-0}" \
    bash "$repo_root/scripts/helix" "$cmd" --summary "$@"
  )
}

test_summary_flag_accepted() {
  # --summary should parse without error (dry-run doesn't exercise run_loop)
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix_summary "$root" build --dry-run 2>&1)"
  # dry-run should still produce the agent command
  assert_contains "$output" "codex" "summary dry-run should print agent command"
  rm -rf "$root"
}

test_summary_concise_output() {
  # Summary mode should produce concise cycle lines and suppress verbose detail
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  # Override codex to close the issue after implementing
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix_summary "$root" run --no-auto-review --no-auto-align 2>&1)"

  # Summary output should contain concise cycle line with issue ID
  assert_contains "$output" "cycle 1:" "summary should show cycle number"
  assert_contains "$output" "COMPLETE" "summary should show completion status"
  # Summary output should contain the run-complete line
  assert_contains "$output" "run complete" "summary should show run complete"
  # Verbose detail (like ready count format) should NOT appear
  if [[ "$output" == *"ready="* ]]; then
    fail "summary mode should not contain verbose ready= format"
  fi
  rm -rf "$root"
}

test_summary_log_has_verbose_detail() {
  # In summary mode, verbose output should go to the log file
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  run_helix_summary "$root" run --no-auto-review --no-auto-align 2>&1 >/dev/null || true

  # Log file should exist and contain verbose codex output
  local log_file
  log_file="$(ls -t "$root/work/.helix-logs"/helix-*.log 2>/dev/null | head -1)"
  [[ -n "$log_file" ]] || fail "summary mode should create a log file"
  local log_content
  log_content="$(cat "$log_file")"
  assert_contains "$log_content" "codex output" "log file should contain verbose codex output"
  rm -rf "$root"
}

test_summary_log_line_references() {
  # Summary mode should include log line references in agent completion lines
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix_summary "$root" run --no-auto-review --no-auto-align 2>&1)"

  # Should contain log line range reference
  assert_contains "$output" "log L" "summary should include log line references"
  assert_contains "$output" ".helix-logs/helix-" "summary should reference log file path"
  rm -rf "$root"
}

test_summary_help_listed() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" help 2>&1)"
  assert_contains "$output" "--summary" "help should list --summary flag"
  rm -rf "$root"
}

run_test "summary flag accepted" test_summary_flag_accepted
run_test "summary concise output" test_summary_concise_output
run_test "summary log has verbose detail" test_summary_log_has_verbose_detail
run_test "summary log line references" test_summary_log_line_references
run_test "summary help listed" test_summary_help_listed

# ── orphan recovery + BUILD loop tests ────────────────────────────────

# Helper: seed tracker with in-progress issues that have stale claim metadata
seed_stale_claimed() {
  local root="$1"
  local count="${2:-1}"
  local work_dir="$root/work"
  mkdir -p "$work_dir/.ddx"
  local stale_ts="2024-01-01T00:00:00Z"  # very old
  local dead_pid=99999  # unlikely to be alive
  local i
  for ((i = 0; i < count; i++)); do
    printf '{"id":"hx-stale-%d","title":"stale issue %d","issue_type":"task","status":"in_progress","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"","spec-id":"","description":"","design":"","acceptance":"mock acceptance","dependencies":[],"owner":"helix","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z","claimed-at":"%s","claimed-pid":%d}\n' "$i" "$i" "$stale_ts" "$dead_pid"
  done > "$work_dir/.ddx/beads.jsonl"
}

test_orphan_recovery_reclaims_stale() {
  local root
  root="$(make_workspace)"
  # Seed 2 stale in-progress issues with dead PIDs and old timestamps
  seed_stale_claimed "$root" 2

  # Verify they're in_progress before recovery
  local before
  before="$(run_bead "$root" list --status in_progress --json | ddx jq 'length')"
  assert_eq "2" "$before" "should have 2 in-progress issues before recovery"

  # Run helix with a mock that returns STOP immediately — recovery happens at startup
  printf 'STOP\n' > "$root/state/next-actions"
  make_mock_bin "$root"

  local output
  output="$(run_helix "$root" run --no-auto-review --no-auto-align 2>&1)" || true

  # Verify issues were reclaimed (back to open)
  local after
  after="$(run_bead "$root" list --status in_progress --json | ddx jq 'length')"
  assert_eq "0" "$after" "orphan recovery should reclaim stale issues"
  assert_contains "$output" "reclaiming orphaned" "should report reclaiming"
  assert_contains "$output" "recovered 2 orphaned" "should report recovery count"
  rm -rf "$root"
}

test_orphan_recovery_skips_fresh() {
  local root
  root="$(make_workspace)"
  local work_dir="$root/work"
  mkdir -p "$work_dir/.ddx"
  # Issue with recent claimed-at and current PID (will be alive)
  local fresh_ts
  fresh_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf '{"id":"hx-fresh-0","title":"fresh issue","issue_type":"task","status":"in_progress","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"","spec-id":"","description":"","design":"","acceptance":"","dependencies":[],"owner":"helix","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"%s","updated_at":"%s","claimed-at":"%s","claimed-pid":%d}\n' \
    "$fresh_ts" "$fresh_ts" "$fresh_ts" "$$" > "$work_dir/.ddx/beads.jsonl"

  printf 'STOP\n' > "$root/state/next-actions"
  make_mock_bin "$root"

  run_helix "$root" run --no-auto-review --no-auto-align 2>&1 >/dev/null || true

  # Issue should still be in_progress (not reclaimed)
  local status
  status="$(run_bead "$root" show hx-fresh-0 --json | ddx jq -r '.status')"
  assert_eq "in_progress" "$status" "fresh claimed issue should not be reclaimed"
  rm -rf "$root"
}

test_build_loop_stops_after_empty_builds() {
  local root
  root="$(make_workspace)"
  # Seed with non-execution-eligible issues (epics) so ready count is 0
  local work_dir="$root/work"
  mkdir -p "$work_dir/.ddx"
  printf '{"id":"hx-epic-0","title":"umbrella epic","issue_type":"epic","status":"open","priority":2,"labels":["helix","phase:build"],"parent":"","spec-id":"","description":"","design":"","acceptance":"","dependencies":[],"owner":"","notes":"","execution-eligible":false,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n' \
    > "$work_dir/.ddx/beads.jsonl"

  # Mock: check always returns BUILD
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
payload="$*"
case "$payload" in
  *"check action"*)
    record check
    printf 'NEXT_ACTION: BUILD\n'
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --no-auto-review --no-auto-align 2>&1)" || true

  # Should stop after 2 consecutive empty BUILDs, not loop forever
  assert_contains "$output" "no selectable issues after orphan recovery" \
    "should stop after consecutive empty BUILD cycles"

  # Should have called check exactly 2 times (2 consecutive empty BUILDs)
  local check_count
  check_count="$(grep -c '^check$' "$root/state/calls.log" 2>/dev/null || echo 0)"
  assert_eq "2" "$check_count" "should run check exactly 2 times before stopping"
  rm -rf "$root"
}

test_build_loop_recovers_orphans_and_continues() {
  # Test that startup orphan recovery reclaims stale issues and they get implemented
  local root
  root="$(make_workspace)"
  local work_dir="$root/work"
  mkdir -p "$work_dir/.ddx"
  # Start with only a stale in-progress issue (no open execution-eligible work)
  local stale_ts="2024-01-01T00:00:00Z"
  printf '{"id":"hx-stale-0","title":"stale issue","issue_type":"task","status":"in_progress","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"","spec-id":"","description":"","design":"","acceptance":"mock acceptance","dependencies":[],"owner":"helix","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z","claimed-at":"%s","claimed-pid":99999}\n' \
    "$stale_ts" > "$work_dir/.ddx/beads.jsonl"

  printf 'STOP\n' > "$root/state/next-actions"
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"
  tail -n +2 "$file" > "$file.tmp" || true
  mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    action="$(next_action)"
    printf 'NEXT_ACTION: %s\n' "$action"
    ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --no-auto-review --no-auto-align 2>&1)" || true

  # Startup recovery should reclaim the orphan, then the loop should implement it
  assert_contains "$output" "reclaiming orphaned" \
    "should report reclaiming orphaned issue"
  local calls
  calls="$(cat "$root/state/calls.log" 2>/dev/null)"
  assert_contains "$calls" "implement" "should implement after recovering orphan"
  rm -rf "$root"
}

test_tracker_claim_records_metadata() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  # Claim the issue
  run_bead "$root" update hx-mock-0 --claim >/dev/null

  # Check claimed-at and claimed-pid are set
  local issue_json
  issue_json="$(run_bead "$root" show hx-mock-0 --json)"

  local claimed_at claimed_pid status
  claimed_at="$(printf '%s' "$issue_json" | ddx jq -r '.["claimed-at"] // ""')"
  claimed_pid="$(printf '%s' "$issue_json" | ddx jq -r '.["claimed-pid"] // ""')"
  status="$(printf '%s' "$issue_json" | ddx jq -r '.status')"

  assert_eq "in_progress" "$status" "claim should set status to in_progress"
  [[ -n "$claimed_at" && "$claimed_at" != "null" ]] || fail "claim should set claimed-at"
  [[ -n "$claimed_pid" && "$claimed_pid" != "null" ]] || fail "claim should set claimed-pid"
  rm -rf "$root"
}

test_tracker_unclaim_clears_metadata() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  # Claim then unclaim
  run_bead "$root" update hx-mock-0 --claim >/dev/null
  run_bead "$root" update hx-mock-0 --unclaim >/dev/null

  local issue_json
  issue_json="$(run_bead "$root" show hx-mock-0 --json)"

  local claimed_at claimed_pid status assignee
  claimed_at="$(printf '%s' "$issue_json" | ddx jq -r '.["claimed-at"] // "null"')"
  claimed_pid="$(printf '%s' "$issue_json" | ddx jq -r '.["claimed-pid"] // "null"')"
  status="$(printf '%s' "$issue_json" | ddx jq -r '.status')"
  assignee="$(printf '%s' "$issue_json" | ddx jq -r '.owner')"

  assert_eq "open" "$status" "unclaim should set status to open"
  [[ -z "$assignee" || "$assignee" == "null" ]] || fail "unclaim should clear assignee (got: $assignee)"
  assert_eq "null" "$claimed_at" "unclaim should clear claimed-at"
  assert_eq "null" "$claimed_pid" "unclaim should clear claimed-pid"
  rm -rf "$root"
}

test_tracker_unclaim_closed_bead() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  # Claim, close, then unclaim
  run_bead "$root" update hx-mock-0 --claim >/dev/null
  run_bead "$root" close hx-mock-0 >/dev/null

  local before_status
  before_status="$(run_bead "$root" show hx-mock-0 --json | ddx jq -r '.status')"
  assert_eq "closed" "$before_status" "bead should be closed before unclaim"

  # Unclaim a closed bead — current behavior: clears claim metadata but status stays closed.
  # ddx bead update --unclaim does NOT reopen a closed bead; it only clears assignee/claim
  # fields. Use --status open explicitly when reopening a closed bead is intentional.
  run_bead "$root" update hx-mock-0 --unclaim >/dev/null

  local issue_json status claimed_at claimed_pid
  issue_json="$(run_bead "$root" show hx-mock-0 --json)"
  status="$(printf '%s' "$issue_json" | ddx jq -r '.status')"
  claimed_at="$(printf '%s' "$issue_json" | ddx jq -r '.["claimed-at"] // "null"')"
  claimed_pid="$(printf '%s' "$issue_json" | ddx jq -r '.["claimed-pid"] // "null"')"

  assert_eq "closed" "$status" "unclaim on closed bead should not reopen it"
  assert_eq "null" "$claimed_at" "unclaim on closed bead should clear claimed-at"
  assert_eq "null" "$claimed_pid" "unclaim on closed bead should clear claimed-pid"
  rm -rf "$root"
}

run_test "tracker claim records metadata" test_tracker_claim_records_metadata
run_test "tracker unclaim clears metadata" test_tracker_unclaim_clears_metadata
run_test "tracker unclaim on closed bead clears metadata" test_tracker_unclaim_closed_bead
run_test "orphan recovery reclaims stale" test_orphan_recovery_reclaims_stale
run_test "orphan recovery skips fresh" test_orphan_recovery_skips_fresh
run_test "BUILD loop stops after empty builds" test_build_loop_stops_after_empty_builds
run_test "BUILD loop recovers orphans and continues" test_build_loop_recovers_orphans_and_continues

# ── port-safety gap tests ─────────────────────────────────────────────

# --- Context regeneration ---

test_context_generated_at_run_start() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  run_helix "$root" run --no-auto-review --no-auto-align 2>&1 >/dev/null || true

  assert_file_exists "$root/work/.helix/context.md" "context.md should be generated at run start"
  assert_file_contains "$root/work/.helix/context.md" "Issue counts:" "context.md should contain issue counts"
  assert_file_contains "$root/work/.helix/context.md" "HELIX Execution Context" "context.md should have header"
  rm -rf "$root"
}

test_context_contains_issue_counts() {
  # Reuse the context file from the previous test pattern — seed 1 issue,
  # implement and close it, check returns STOP
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*) record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  run_helix "$root" run --no-auto-review --no-auto-align 2>&1 >/dev/null || true

  local ctx
  ctx="$(cat "$root/work/.helix/context.md")"
  assert_contains "$ctx" "ready=" "context should show ready count"
  assert_contains "$ctx" "open=" "context should show open count"
  assert_contains "$ctx" "closed=" "context should show closed count"
  rm -rf "$root"
}

# --- Epic focus mode ---

test_epic_focus_selects_children() {
  local root
  root="$(make_workspace)"
  local work_dir="$root/work"
  mkdir -p "$work_dir/.ddx"
  # Create an epic with one child
  {
    printf '{"id":"hx-epic-1","title":"test epic","issue_type":"epic","status":"open","priority":2,"labels":["helix","phase:build"],"parent":"","spec-id":"SPEC-1","description":"","design":"","acceptance":"all children done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
    printf '{"id":"hx-child-1","title":"child 1","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"hx-epic-1","spec-id":"SPEC-1","description":"","design":"","acceptance":"test passes","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
  } > "$work_dir/.ddx/beads.jsonl"

  printf 'STOP\n' > "$root/state/next-actions"

  # Mock: close all open non-epic issues
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c 'if .issue_type != "epic" then .status = "closed" else . end' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --no-auto-review --no-auto-align 2>&1)" || true

  # Should have entered epic focus (output mentions it)
  assert_contains "$output" "epic" "should mention epic in output"

  # Epic should be closed after child completes
  local epic_status
  epic_status="$(ddx jq -r 'select(.id == "hx-epic-1") | .status' "$work_dir/.ddx/beads.jsonl")"
  assert_eq "closed" "$epic_status" "epic should be closed after all children complete"
  rm -rf "$root"
}

test_task_promotes_to_epic_after_decomposition() {
  local root
  root="$(make_workspace)"
  local work_dir="$root/work"
  mkdir -p "$work_dir/.ddx"
  {
    printf '{"id":"hx-task-r","title":"promotion parent","issue_type":"task","status":"open","priority":1,"labels":["helix","phase:build"],"parent":"","spec-id":"SPEC-R","description":"","design":"","acceptance":"done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
    printf '{"id":"hx-task-child-1","title":"existing child","issue_type":"task","status":"closed","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"hx-task-r","spec-id":"SPEC-R","description":"","design":"","acceptance":"done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
  } > "$work_dir/.ddx/beads.jsonl"

  printf 'STOP\n' > "$root/state/next-actions"
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      if [[ ! -f "$state_root/promotion-seen" ]]; then
        cat >> .ddx/beads.jsonl <<'EOF'
{"id":"hx-task-child-2","title":"new child","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"hx-task-r","spec-id":"SPEC-R","description":"","design":"","acceptance":"done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}
EOF
        : > "$state_root/promotion-seen"
      else
        tmp="$(ddx jq -c 'if .id == "hx-task-child-2" then .status = "closed" else . end' .ddx/beads.jsonl)"
        printf '%s\n' "$tmp" > .ddx/beads.jsonl
      fi
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check
    printf 'NEXT_ACTION: %s\n' "$(next_action)"
    ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --max-cycles 2 --no-auto-review --no-auto-align 2>&1)" || true

  assert_contains "$output" "promoting hx-task-r to epic" \
    "should promote decomposed task to epic"
  assert_contains "$output" "epic focus: hx-task-r → child hx-task-child-2" \
    "should continue execution under epic focus after promotion"
  local parent_type
  parent_type="$(ddx jq -r 'select(.id == "hx-task-r") | .issue_type' "$work_dir/.ddx/beads.jsonl")"
  assert_eq "epic" "$parent_type" "parent task should be promoted to epic"
  local parent_labels parent_spec parent_priority
  parent_labels="$(ddx jq -r 'select(.id == "hx-task-r") | .labels | join(",")' "$work_dir/.ddx/beads.jsonl")"
  parent_spec="$(ddx jq -r 'select(.id == "hx-task-r") | .["spec-id"]' "$work_dir/.ddx/beads.jsonl")"
  parent_priority="$(ddx jq -r 'select(.id == "hx-task-r") | .priority' "$work_dir/.ddx/beads.jsonl")"
  assert_eq "helix,phase:build" "$parent_labels" "parent labels should be preserved"
  assert_eq "SPEC-R" "$parent_spec" "parent spec-id should be preserved"
  assert_eq "1" "$parent_priority" "parent priority should be preserved"
  rm -rf "$root"
}

# --- Area-label batching ---

test_batch_falls_back_to_area_labels() {
  local root
  root="$(make_workspace)"
  local work_dir="$root/work"
  mkdir -p "$work_dir/.ddx"
  # Two issues with same area label but no shared parent or spec-id
  {
    printf '{"id":"hx-area-1","title":"area issue 1","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:build","kind:build","area:auth"],"parent":"","spec-id":"SPEC-A","description":"","design":"","acceptance":"done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
    printf '{"id":"hx-area-2","title":"area issue 2","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:build","kind:build","area:auth"],"parent":"","spec-id":"SPEC-B","description":"","design":"","acceptance":"done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
    printf '{"id":"hx-area-3","title":"unrelated issue","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:build","kind:build","area:storage"],"parent":"","spec-id":"SPEC-C","description":"","design":"","acceptance":"done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
  } > "$work_dir/.ddx/beads.jsonl"

  # Use dry-run — the batch prompt should mention both area:auth siblings
  printf 'STOP\n' > "$root/state/next-actions"
  make_mock_bin "$root"
  local output
  output="$(run_helix "$root" run --dry-run --no-auto-review --no-auto-align 2>&1)" || true

  # The dry-run output should show a batch containing both area:auth issues
  # (batch=2 in the cycle line, or both IDs in the prompt)
  assert_contains "$output" "hx-area-2" "batch should include area:auth sibling"
  rm -rf "$root"
}

# --- Revalidation / queue drift ---

test_drift_on_parent_change_skips_close() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  # Mock: implementation succeeds but changes the issue's parent mid-flight
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    # Simulate governance drift: change the parent field mid-execution
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.parent = "hx-new-parent"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"
  printf 'STOP\n' > "$root/state/next-actions"

  local output
  output="$(run_helix "$root" run --no-auto-review --no-auto-align 2>&1)" || true

  assert_contains "$output" "queue drift" "should detect parent change as queue drift"
  rm -rf "$root"
}

# --- Acceptance check filing ---

test_acceptance_failure_filed_as_issue() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  # Create a failing acceptance check script that only fails once
  mkdir -p "$root/work/scripts"
  cat >"$root/work/scripts/check-acceptance-traceability.sh" <<SCRIPT
#!/usr/bin/env bash
flag="$root/state/ac-ran"
if [[ ! -f "\$flag" ]]; then
  touch "\$flag"
  echo "FAIL: missing traceability for module X"
  exit 1
fi
exit 0
SCRIPT
  chmod +x "$root/work/scripts/check-acceptance-traceability.sh"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c 'if .status == "open" then .status = "closed" else . end' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --max-cycles 1 --no-auto-review --no-auto-align 2>&1)" || true

  # Should have filed an acceptance failure issue
  local ac_issues
  ac_issues="$(run_bead "$root" list --label acceptance-failure --json | ddx jq 'length')"
  (( ac_issues > 0 )) || fail "acceptance failure should be filed as tracker issue"

  # Verify the filed issue has correct metadata
  local ac_title
  ac_title="$(run_bead "$root" list --label acceptance-failure --json | ddx jq -r '.[0].title')"
  assert_contains "$ac_title" "Acceptance failure" "filed issue should have descriptive title"
  rm -rf "$root"
}

# --- Exponential backoff ---

test_backoff_delay_formula() {
  # Test the backoff_delay function directly
  local root
  root="$(make_workspace)"

  local delays
  delays="$(cd "$root/work" && HELIX_LIBRARY_ROOT="$repo_root/workflows" \
    bash -c '
      backoff_delay() {
        local attempts="$1"
        local delay=$(( 5 * (1 << (attempts - 1)) ))
        (( delay > 40 )) && delay=40
        printf "%s" "$delay"
      }
      echo "$(backoff_delay 1) $(backoff_delay 2) $(backoff_delay 3) $(backoff_delay 4) $(backoff_delay 5)"
    ')"

  assert_eq "5 10 20 40 40" "$delays" "backoff should be 5s, 10s, 20s, 40s cap"
  rm -rf "$root"
}

test_backoff_blocks_after_four_attempts() {
  # This test exercises the full backoff loop (4 failures with exponential delays).
  # The backoff sleeps total ~55 seconds so we use HELIX_BACKOFF_OVERRIDE=0 to skip delays.
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  # Mock: implementation always fails
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    echo "fail" >&2
    exit 1
    ;;
  *"check action"*)
    record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix_with_env "$root" HELIX_BACKOFF_SLEEP 0 run --no-auto-review --no-auto-align 2>&1)" || true

  # Should have attempted implementation 4 times then blocked
  local impl_count
  impl_count="$(grep -c '^implement$' "$root/state/calls.log" 2>/dev/null || echo 0)"
  assert_eq "4" "$impl_count" "should attempt implementation exactly 4 times before blocking"
  assert_contains "$output" "BLOCKERS" "should report blockers after exhausting attempts"
  rm -rf "$root"
}

# --- Token capture ---

test_extract_codex_tokens_from_output() {
  # Test the extract_codex_tokens function with stdout+stderr merged output
  local result
  result="$(printf 'some agent output\nCommands run:\ncargo test\ntokens used\n1,234\n' | \
    bash -c '
      extract_codex_tokens() {
        local prev=""
        while IFS= read -r line; do
          if [[ "$prev" == "tokens used" ]] && [[ "$line" =~ ^[0-9,]+$ ]]; then
            printf "%s" "${line//,/}"
            return
          fi
          prev="$line"
        done
        printf "0"
      }
      extract_codex_tokens
    ')"
  assert_eq "1234" "$result" "should extract token count from codex output (stripping commas)"
}

test_extract_codex_tokens_missing() {
  # Test fallback when no token footer present
  local result
  result="$(printf 'some output with no tokens\n' | \
    bash -c '
      extract_codex_tokens() {
        local prev=""
        while IFS= read -r line; do
          if [[ "$prev" == "tokens used" ]] && [[ "$line" =~ ^[0-9,]+$ ]]; then
            printf "%s" "${line//,/}"
            return
          fi
          prev="$line"
        done
        printf "0"
      }
      extract_codex_tokens
    ')"
  assert_eq "0" "$result" "should return 0 when no token footer found"
}

# --- GUIDANCE next-action ---

test_run_stops_on_guidance() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'GUIDANCE\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*)
    record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --no-auto-review --no-auto-align 2>&1)" || true

  assert_contains "$output" "stopping after check returned GUIDANCE" \
    "should stop on GUIDANCE"
  rm -rf "$root"
}

# --- Cross-model review ---

test_review_dry_run_uses_review_agent() {
  local root
  root="$(make_workspace)"

  # Run review with --review-agent set via env
  local output
  output="$(
    cd "$root/work"
    HOME="$root/home" \
    PATH="$root/bin:$PATH" \
    MOCK_STATE_ROOT="$root/state" \
    HELIX_LIBRARY_ROOT="$repo_root/workflows" \
    HELIX_FORCE_EPHEMERAL=1 \
    HELIX_REVIEW_AGENT="codex" \
    HELIX_DIRECT_AGENT=1 \
    bash "$repo_root/scripts/helix" review --agent claude --quiet --dry-run 2>&1
  )"

  # In dry-run, the review command should show codex (the review agent), not claude
  assert_contains "$output" "codex" "review dry-run should use the review agent (codex)"
  rm -rf "$root"
}

# --- Blocker report ---

test_blocker_report_written_to_file() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  # Mock: implementation always fails (triggers blocker)
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement; echo "fail" >&2; exit 1 ;;
  *"check action"*)
    record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  run_helix "$root" run --no-auto-review --no-auto-align 2>&1 >/dev/null || true

  # Blocker report file should exist
  local report
  report="$(ls "$root/work/.helix-logs"/blockers-*.md 2>/dev/null | head -1)"
  [[ -n "$report" ]] || fail "blocker report file should be created"

  # Report should contain structured content
  local report_content
  report_content="$(cat "$report")"
  assert_contains "$report_content" "HELIX Run Blocker Report" "report should have header"
  assert_contains "$report_content" "Blocked Issues" "report should list blocked issues"
  assert_contains "$report_content" "hx-mock-0" "report should contain the blocked issue ID"
  rm -rf "$root"
}

test_blocker_report_marks_tracker() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*) record implement; exit 1 ;;
  *"check action"*) record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  run_helix "$root" run --no-auto-review --no-auto-align 2>&1 >/dev/null || true

  # Blocked issue should have "blocked" label in tracker
  local labels
  labels="$(run_bead "$root" show hx-mock-0 --json | ddx jq -r '.labels | join(",")')"
  assert_contains "$labels" "blocked" "blocked issue should have 'blocked' label in tracker"
  rm -rf "$root"
}

run_test "context generated at run start" test_context_generated_at_run_start
run_test "context contains issue counts" test_context_contains_issue_counts
run_test "epic focus selects children" test_epic_focus_selects_children
run_test "batch falls back to area labels" test_batch_falls_back_to_area_labels
run_test "drift on parent change skips close" test_drift_on_parent_change_skips_close
run_test "acceptance failure filed as issue" test_acceptance_failure_filed_as_issue
run_test "backoff delay formula" test_backoff_delay_formula
run_test "backoff blocks after four attempts" test_backoff_blocks_after_four_attempts
run_test "extract codex tokens from output" test_extract_codex_tokens_from_output
run_test "extract codex tokens missing" test_extract_codex_tokens_missing
run_test "run stops on GUIDANCE" test_run_stops_on_guidance
run_test "review dry-run uses review agent" test_review_dry_run_uses_review_agent
test_blocker_notes_stable_across_retries() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  # Mock: both agents always fail implementation (triggers blocker notes)
  cat >"$root/bin/mock-fail" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
stdin_payload=""
if ! [[ -t 0 ]]; then stdin_payload="$(cat)"; fi
payload="$* $stdin_payload"
case "$payload" in
  *"implementation action"*) record implement; exit 1 ;;
  *"check action"*) record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  cp "$root/bin/mock-fail" "$root/bin/codex"
  cp "$root/bin/mock-fail" "$root/bin/claude"
  chmod +x "$root/bin/codex" "$root/bin/claude"

  # First run: triggers blocker notes
  run_helix_with_env "$root" HELIX_BACKOFF_SLEEP 0 run --no-auto-review --no-auto-align 2>&1 >/dev/null || true

  local notes_after_first
  notes_after_first="$(run_bead "$root" show hx-mock-0 --json | ddx jq -r '.notes // ""')"
  [[ -n "$notes_after_first" ]] || fail "first run should set blocker notes"

  # Reset next-actions for the second run
  printf 'STOP\n' > "$root/state/next-actions"
  : > "$root/state/calls.log"

  # Second run: same failure, same reason — notes should be stable
  run_helix_with_env "$root" HELIX_BACKOFF_SLEEP 0 run --no-auto-review --no-auto-align 2>&1 >/dev/null || true

  local notes_after_second
  notes_after_second="$(run_bead "$root" show hx-mock-0 --json | ddx jq -r '.notes // ""')"
  assert_eq "$notes_after_first" "$notes_after_second" \
    "blocker notes should remain stable across identical retries"
  rm -rf "$root"
}

run_test "blocker report written to file" test_blocker_report_written_to_file
run_test "blocker report marks tracker" test_blocker_report_marks_tracker
run_test "blocker notes stable across retries" test_blocker_notes_stable_across_retries

# --- Context refresh on epic switch and every 5 cycles ---

test_context_refreshed_on_epic_switch() {
  local root
  root="$(make_workspace)"
  local work_dir="$root/work"
  mkdir -p "$work_dir/.ddx"
  # Epic with one child — entering epic focus triggers context regeneration
  {
    printf '{"id":"hx-epic-r","title":"refresh epic","issue_type":"epic","status":"open","priority":2,"labels":["helix","phase:build"],"parent":"","spec-id":"SPEC-R","description":"","design":"","acceptance":"done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
    printf '{"id":"hx-child-r","title":"refresh child","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"hx-epic-r","spec-id":"SPEC-R","description":"","design":"","acceptance":"done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
  } > "$work_dir/.ddx/beads.jsonl"

  printf 'STOP\n' > "$root/state/next-actions"
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c 'if .issue_type != "epic" then .status = "closed" else . end' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*) record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  run_helix "$root" run --no-auto-review --no-auto-align 2>&1 >/dev/null || true

  # Context file should contain epic metadata (written on epic switch)
  assert_file_exists "$work_dir/.helix/context.md" "context.md should exist"
  assert_file_contains "$work_dir/.helix/context.md" "Current Epic" \
    "context.md should contain epic section after epic focus switch"
  assert_file_contains "$work_dir/.helix/context.md" "hx-epic-r" \
    "context.md should reference the focused epic ID"
  rm -rf "$root"
}

test_context_refreshed_every_5_cycles() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 6  # 6 issues so we can complete 5+ cycles
  printf 'STOP\n' > "$root/state/next-actions"

  # Track context.md modification times
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      # Close the first open issue only
      first="$(ddx jq -r 'select(.status == "open") | .id' .ddx/beads.jsonl | head -1)"
      if [[ -n "$first" ]]; then
        tmp="$(ddx jq -c "if .id == \"$first\" then .status = \"closed\" else . end" .ddx/beads.jsonl)"
        printf '%s\n' "$tmp" > .ddx/beads.jsonl
      fi
    fi
    # Record context.md hash after each implementation
    if [[ -f .helix/context.md ]]; then
      md5sum .helix/context.md >> "$state_root/context-hashes.log"
    fi
    echo "implementation complete"
    ;;
  *"check action"*) record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  run_helix "$root" run --no-auto-review --no-auto-align 2>&1 >/dev/null || true

  # Should have completed at least 5 cycles
  local impl_count
  impl_count="$(grep -c '^implement$' "$root/state/calls.log" 2>/dev/null || echo 0)"
  (( impl_count >= 5 )) || fail "should complete at least 5 cycles (got $impl_count)"

  # Context should have been refreshed (hashes should differ between early and late)
  local hash_count
  hash_count="$(wc -l < "$root/state/context-hashes.log" 2>/dev/null || echo 0)"
  local unique_hashes
  unique_hashes="$(awk '{print $1}' "$root/state/context-hashes.log" 2>/dev/null | sort -u | wc -l)"
  # After 5 cycles the context is regenerated, so hashes should change
  (( unique_hashes >= 2 )) || fail "context.md should be refreshed during run (got $unique_hashes unique hashes from $hash_count checks)"
  rm -rf "$root"
}

# --- Pre-claim drift (supersession) ---

test_drift_on_supersession_skips_close() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  # Mock: implementation succeeds but supersedes the issue mid-flight
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    # Simulate governance drift: set superseded-by during execution
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.["superseded-by"] = "hx-replacement"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*) record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"
  printf 'STOP\n' > "$root/state/next-actions"

  local output
  output="$(run_helix "$root" run --no-auto-review --no-auto-align 2>&1)" || true

  assert_contains "$output" "queue drift" "should detect supersession as queue drift"
  rm -rf "$root"
}

test_drift_on_spec_id_change_skips_close() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    # Simulate governance drift: change spec-id during execution
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.["spec-id"] = "CHANGED-SPEC"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*) record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"
  printf 'STOP\n' > "$root/state/next-actions"

  local output
  output="$(run_helix "$root" run --no-auto-review --no-auto-align 2>&1)" || true

  assert_contains "$output" "queue drift" "should detect spec-id change as queue drift"
  rm -rf "$root"
}

# --- Review trailer parsing ---

test_review_clean_status_succeeds() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"review"*)
    record review
    echo "REVIEW_STATUS: CLEAN"
    ;;
  *"check action"*) record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --no-auto-align 2>&1)" || true

  # Should have called review
  local calls
  calls="$(cat "$root/state/calls.log" 2>/dev/null)"
  assert_contains "$calls" "review" "should call review after implementation"
  # Should NOT contain any issues_found messages
  if [[ "$output" == *"issue(s)"* ]]; then
    fail "CLEAN review should not report issues"
  fi
  rm -rf "$root"
}

test_review_issues_found_continues_loop() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"review"*)
    record review
    printf 'REVIEW_STATUS: ISSUES_FOUND\nISSUES_COUNT: 2\nFINDINGS_FILED: 2\n'
    ;;
  *"check action"*) record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --no-auto-align 2>&1)" || true

  assert_contains "$output" "2 issue(s)" "should report issue count from review"
  assert_contains "$output" "2 filed" "should report findings filed count"
  # Loop should continue (check should be called after review)
  local calls
  calls="$(cat "$root/state/calls.log" 2>/dev/null)"
  assert_contains "$calls" "check" "loop should continue to check after review findings"
  rm -rf "$root"
}

# --- Cross-model review in live run ---

test_cross_model_review_switches_agent() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  # Use claude as primary, codex as review agent
  # Mock both — implementation via claude, review via codex
  cat >"$root/bin/claude" <<'MOCK'
#!/usr/bin/env bash
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
# claude is used for implementation
record "claude-call"
if [[ -f .ddx/beads.jsonl ]]; then
  tmp="$(ddx jq -c '.status = "closed"' .ddx/beads.jsonl)"
  printf '%s\n' "$tmp" > .ddx/beads.jsonl
fi
echo "implementation complete"
MOCK
  chmod +x "$root/bin/claude"

  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"review"*)
    record "codex-review"
    echo "REVIEW_STATUS: CLEAN"
    ;;
  *"check action"*)
    record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock codex" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(
    cd "$root/work"
    HOME="$root/home" \
    PATH="$root/bin:$PATH" \
    MOCK_STATE_ROOT="$root/state" \
    HELIX_LIBRARY_ROOT="$repo_root/workflows" \
    HELIX_FORCE_EPHEMERAL=1 \
    HELIX_REVIEW_AGENT="codex" \
    HELIX_DIRECT_AGENT=1 \
    bash "$repo_root/scripts/helix" run --agent claude --quiet --no-auto-align 2>&1
  )" || true

  local calls
  calls="$(cat "$root/state/calls.log" 2>/dev/null)"
  # Review should use codex (the review agent), not claude
  assert_contains "$calls" "codex-review" "review should use the cross-model review agent (codex)"
  rm -rf "$root"
}

# --- Epic child failure blocks parent ---

test_epic_blocked_when_child_intractable() {
  local root
  root="$(make_workspace)"
  local work_dir="$root/work"
  mkdir -p "$work_dir/.ddx"
  # Epic with one child that will fail
  {
    printf '{"id":"hx-epic-b","title":"blocked epic","issue_type":"epic","status":"open","priority":2,"labels":["helix","phase:build"],"parent":"","spec-id":"","description":"","design":"","acceptance":"done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
    printf '{"id":"hx-fail-c","title":"failing child","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"hx-epic-b","spec-id":"","description":"","design":"","acceptance":"done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
  } > "$work_dir/.ddx/beads.jsonl"

  printf 'STOP\n' > "$root/state/next-actions"
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*) record implement; exit 1 ;;
  *"check action"*) record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix_with_env "$root" HELIX_BACKOFF_SLEEP 0 run --no-auto-review --no-auto-align 2>&1)" || true

  # Blocker report should mention both the child AND the parent epic
  assert_contains "$output" "hx-fail-c" "should report blocked child"
  assert_contains "$output" "hx-epic-b" "should report blocked parent epic"
  assert_contains "$output" "BLOCKERS (2 issues)" "should report 2 blocked issues (child + epic)"
  rm -rf "$root"
}

run_test "context refreshed on epic switch" test_context_refreshed_on_epic_switch
run_test "task promotes to epic after decomposition" test_task_promotes_to_epic_after_decomposition
run_test "context refreshed every 5 cycles" test_context_refreshed_every_5_cycles
run_test "drift on supersession skips close" test_drift_on_supersession_skips_close
run_test "drift on spec-id change skips close" test_drift_on_spec_id_change_skips_close
run_test "review CLEAN succeeds" test_review_clean_status_succeeds
run_test "review ISSUES_FOUND continues loop" test_review_issues_found_continues_loop
run_test "cross-model review switches agent" test_cross_model_review_switches_agent
run_test "epic blocked when child intractable" test_epic_blocked_when_child_intractable

# ── frame tests ───────────────────────────────────────────────────────

test_frame_dry_run() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" frame --dry-run)"
  assert_contains "$output" "actions/frame.md" "frame dry-run should reference frame action"
  assert_contains "$output" "phases/00-discover/artifacts" "frame dry-run should reference discover templates"
  assert_contains "$output" "phases/01-frame/artifacts" "frame dry-run should reference frame templates"
  rm -rf "$root"
}

test_frame_dry_run_with_scope() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" frame --dry-run auth)"
  assert_contains "$output" "Frame scope: auth" "frame dry-run should include scope"
  rm -rf "$root"
}

test_frame_help_listed() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" help)"
  assert_contains "$output" "frame" "help should list frame command"
  rm -rf "$root"
}

run_test "frame dry-run" test_frame_dry_run
run_test "frame dry-run with scope" test_frame_dry_run_with_scope
run_test "frame help listed" test_frame_help_listed

# ── skill completeness test ───────────────────────────────────────

test_skills_have_symlinks() {
  # Verify every skill in skills/ has symlinks in .agents/skills/ and .claude/skills/
  local missing=0
  for skill_path in "$repo_root/skills"/helix-*/SKILL.md; do
    [[ -e "$skill_path" ]] || continue
    local name
    name="$(basename "$(dirname "$skill_path")")"
    if [[ ! -L "$repo_root/.agents/skills/$name" ]]; then
      printf 'missing .agents/skills/%s symlink\n' "$name" >&2
      missing=$((missing + 1))
    fi
    if [[ ! -L "$repo_root/.claude/skills/$name" ]]; then
      printf 'missing .claude/skills/%s symlink\n' "$name" >&2
      missing=$((missing + 1))
    fi
  done
  (( missing == 0 )) || fail "missing $missing skill symlink(s) — run helix doctor --fix"
}

run_test "skills have symlinks" test_skills_have_symlinks

# ── commit tests ──────────────────────────────────────────────────────

test_commit_fails_with_nothing_to_commit() {
  local root
  root="$(make_workspace)"
  assert_fails "commit with no changes should fail" \
    run_helix "$root" commit
  rm -rf "$root"
}

test_commit_stages_and_commits() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1

  # Git needs author identity since HOME is overridden
  (cd "$root/work" && git config user.email "test@test.com" && git config user.name "Test")

  printf 'new content\n' > "$root/work/test.txt"
  (cd "$root/work" && git add test.txt)

  local output
  output="$(run_helix "$root" commit hx-mock-0 2>&1)"

  assert_contains "$output" "committed" "should report successful commit"
  local status
  status="$(run_bead "$root" show hx-mock-0 --json | ddx jq -r '.status')"
  assert_eq "closed" "$status" "commit should close the tracker issue"
  rm -rf "$root"
}

test_commit_without_issue_id() {
  local root
  root="$(make_workspace)"
  (cd "$root/work" && git config user.email "test@test.com" && git config user.name "Test")

  printf 'new content\n' > "$root/work/test.txt"
  (cd "$root/work" && git add test.txt)

  local output
  output="$(run_helix "$root" commit 2>&1)"

  assert_contains "$output" "committed" "commit without issue should succeed"
  local msg
  msg="$(cd "$root/work" && git log -1 --format=%s)"
  assert_contains "$msg" "test.txt" "commit message should include changed filename"
  rm -rf "$root"
}

test_commit_auto_stages_unstaged() {
  local root
  root="$(make_workspace)"
  (cd "$root/work" && git config user.email "test@test.com" && git config user.name "Test")

  (cd "$root/work" && printf 'original\n' > test.txt && git add test.txt && git commit -qm "seed")
  printf 'modified\n' > "$root/work/test.txt"

  local output
  output="$(run_helix "$root" commit 2>&1)"

  assert_contains "$output" "staging all modified" "should auto-stage unstaged changes"
  assert_contains "$output" "committed" "should commit after auto-staging"
  rm -rf "$root"
}

run_test "commit fails with nothing to commit" test_commit_fails_with_nothing_to_commit
run_test "commit stages and commits with issue" test_commit_stages_and_commits
run_test "commit without issue id" test_commit_without_issue_id
run_test "commit auto-stages unstaged" test_commit_auto_stages_unstaged

# ── final completeness tests ─────────────────────────────────────────

test_help_includes_all_commands() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" help 2>&1)"
  local missing=0
  for cmd in run build check align backfill design polish next review \
             experiment evolve triage commit tracker frame help status; do
    if [[ "$output" != *"$cmd"* ]]; then
      printf 'missing command in help: %s\n' "$cmd" >&2
      missing=$((missing + 1))
    fi
  done
  (( missing == 0 )) || fail "help is missing $missing command(s)"
  rm -rf "$root"
}

test_run_prefers_tasks_over_epics() {
  # The run loop's helix_select_ready_issue prefers non-epic tasks with
  # execution metadata over epics. Verify via the actual run output.
  local root
  root="$(make_workspace)"
  local work_dir="$root/work"
  mkdir -p "$work_dir/.ddx"
  # Task listed AFTER epic — run loop should still select the task first
  {
    printf '{"id":"hx-epic-p","title":"epic","issue_type":"epic","status":"open","priority":2,"labels":["helix","phase:build"],"parent":"","spec-id":"","description":"","design":"","acceptance":"done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
    printf '{"id":"hx-task-p","title":"task with meta","issue_type":"task","status":"open","priority":2,"labels":["helix","phase:build","kind:build"],"parent":"","spec-id":"SPEC","description":"","design":"","acceptance":"done","dependencies":[],"owner":"","notes":"","execution-eligible":true,"superseded-by":"","replaces":"","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}\n'
  } > "$work_dir/.ddx/beads.jsonl"

  printf 'STOP\n' > "$root/state/next-actions"
  cat >"$root/bin/codex" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
state_root="${MOCK_STATE_ROOT:?}"
record() { printf '%s\n' "$1" >> "$state_root/calls.log"; }
next_action() {
  local file="$state_root/next-actions"
  if [[ ! -s "$file" ]]; then echo STOP; return; fi
  head -n1 "$file"; tail -n +2 "$file" > "$file.tmp" || true; mv "$file.tmp" "$file"
}
payload="$*"
case "$payload" in
  *"implementation action"*)
    record implement
    if [[ -f .ddx/beads.jsonl ]]; then
      tmp="$(ddx jq -c 'if .issue_type != "epic" then .status = "closed" else . end' .ddx/beads.jsonl)"
      printf '%s\n' "$tmp" > .ddx/beads.jsonl
    fi
    echo "implementation complete"
    ;;
  *"check action"*) record check; printf 'NEXT_ACTION: %s\n' "$(next_action)" ;;
  *) record other; echo "mock" ;;
esac
MOCK
  chmod +x "$root/bin/codex"

  local output
  output="$(run_helix "$root" run --max-cycles 1 --no-auto-review --no-auto-align 2>&1)"

  # The first cycle line should show hx-task-p, not hx-epic-p
  assert_contains "$output" "issue=hx-task-p" \
    "run loop should prefer task with metadata over epic"
  rm -rf "$root"
}

test_help_includes_all_commands() {
  local root
  root="$(make_workspace)"
  local output
  output="$(run_helix "$root" help 2>&1)"
  for cmd in run start stop build check align backfill design polish status next review experiment evolve commit triage; do
    assert_contains "$output" "$cmd" "help output should include $cmd"
  done
  rm -rf "$root"
}

test_start_writes_pid_file() {
  local root
  root="$(make_workspace)"
  seed_tracker "$root" 1
  printf 'STOP\n' > "$root/state/next-actions"

  # start launches background run
  run_helix "$root" start --max-cycles 1 --no-auto-review --no-auto-align 2>&1 || true
  # Give background process time to write PID and finish
  sleep 2
  assert_file_exists "$root/work/.ddx/helix.pid" "start should write PID file"
  rm -rf "$root"
}

test_stop_no_active_run() {
  local root
  root="$(make_workspace)"

  # stop should fail with no PID file
  local output rc=0
  output="$(run_helix "$root" stop 2>&1)" || rc=$?
  [[ "$rc" -ne 0 ]] || fail "stop should fail when no run is active"
  assert_contains "$output" "no active run" "stop should report no active run"
  rm -rf "$root"
}

test_stop_stale_pid() {
  local root
  root="$(make_workspace)"
  mkdir -p "$root/work/.ddx"
  # Write a PID that doesn't exist
  printf '99999999\n' > "$root/work/.ddx/helix.pid"

  local output rc=0
  output="$(run_helix "$root" stop 2>&1)" || rc=$?
  [[ "$rc" -ne 0 ]] || fail "stop should fail on stale PID"
  assert_contains "$output" "stale PID" "stop should report stale PID"
  [[ ! -f "$root/work/.ddx/helix.pid" ]] || fail "stop should remove stale PID file"
  rm -rf "$root"
}

test_status_shows_active_run() {
  local root
  root="$(make_workspace)"
  mkdir -p "$root/work/.ddx"
  # Write our own PID (will be alive)
  printf '%s\n' "$$" > "$root/work/.ddx/helix.pid"

  local output
  output="$(run_helix "$root" status 2>&1)"
  assert_contains "$output" "Active Run" "status should show Active Run section"
  assert_contains "$output" "PID:" "status should show PID"

  rm -f "$root/work/.ddx/helix.pid"
  rm -rf "$root"
}

test_status_cleans_stale_pid() {
  local root
  root="$(make_workspace)"
  mkdir -p "$root/work/.ddx"
  printf '99999999\n' > "$root/work/.ddx/helix.pid"

  run_helix "$root" status 2>&1 >/dev/null
  [[ ! -f "$root/work/.ddx/helix.pid" ]] || fail "status should clean stale PID file"
  rm -rf "$root"
}

run_test "help includes all commands" test_help_includes_all_commands
run_test "run prefers tasks over epics" test_run_prefers_tasks_over_epics
run_test "stop with no active run" test_stop_no_active_run
run_test "stop cleans stale PID" test_stop_stale_pid
run_test "status shows active run" test_status_shows_active_run
run_test "status cleans stale PID" test_status_cleans_stale_pid

echo "PASS: ${test_count} helix wrapper tests"
