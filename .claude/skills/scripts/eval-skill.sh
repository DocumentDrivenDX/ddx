#!/usr/bin/env bash
# eval-skill.sh — validate the ddx skill and optionally drive routing evals
# against live harnesses.
#
# Usage:
#   scripts/eval-skill.sh --validate            # structural spec conformance only
#   scripts/eval-skill.sh                       # validate + routing eval on claude + codex
#   scripts/eval-skill.sh --harnesses=claude,codex
#   HARNESSES=claude scripts/eval-skill.sh      # env var also works
#
# Exit codes:
#   0 — all checks passed
#   1 — validation or routing check failed
#   2 — invocation error (missing files, no ddx binary, etc.)
#
# Validation (always runs):
#   - SKILL.md exists at skills/ddx/SKILL.md
#   - Frontmatter contains only `name` + `description` (portable agentskills.io minimum)
#   - Body is under 500 lines
#   - All reference/*.md files linked from SKILL.md exist
#
# Routing eval (skipped in --validate mode):
#   - For each row in skills/ddx/evals/routing.jsonl, invoke
#     `ddx agent run --harness <h> --text <phrase>` and check that the
#     response mentions the expected reference file OR the expected CLI command.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKILL_DIR="$REPO_ROOT/skills/ddx"
SKILL_MD="$SKILL_DIR/SKILL.md"
EVAL_FILE="$SKILL_DIR/evals/routing.jsonl"

VALIDATE_ONLY=0
HARNESSES="${HARNESSES:-claude,codex}"

for arg in "$@"; do
  case "$arg" in
    --validate) VALIDATE_ONLY=1 ;;
    --harnesses=*) HARNESSES="${arg#*=}" ;;
    -h|--help)
      sed -n '1,30p' "$0"
      exit 0
      ;;
  esac
done

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

info() {
  echo "$*"
}

# --- Validation (always) ---

info "==> Validating $SKILL_MD against agentskills.io spec"

[[ -f "$SKILL_MD" ]] || fail "SKILL.md not found at $SKILL_MD"

# Extract frontmatter (between first two --- lines)
frontmatter="$(awk '/^---$/{c++; if (c==1) next; if (c==2) exit} c==1' "$SKILL_MD")"
[[ -n "$frontmatter" ]] || fail "SKILL.md has no frontmatter"

# Check required fields
echo "$frontmatter" | grep -qE '^name:[[:space:]]*ddx[[:space:]]*$' \
  || fail "SKILL.md frontmatter missing or wrong 'name: ddx'"
echo "$frontmatter" | grep -qE '^description:[[:space:]]*' \
  || fail "SKILL.md frontmatter missing 'description:'"

# Check for Claude-Code-only fields that would break portability
for forbidden in argument-hint when_to_use disable-model-invocation user-invocable allowed-tools context: paths: hooks: model: effort: agent:; do
  if echo "$frontmatter" | grep -qE "^${forbidden}[[:space:]]*"; then
    fail "SKILL.md frontmatter contains non-portable field '$forbidden' (agentskills.io minimum is name + description only)"
  fi
done

# Body under 500 lines (per Anthropic progressive-disclosure guidance)
body_lines="$(awk '/^---$/{c++; if (c==2) { found=1; next }} found' "$SKILL_MD" | wc -l | tr -d '[:space:]')"
if [[ "$body_lines" -gt 500 ]]; then
  fail "SKILL.md body is $body_lines lines (>500). Move detail to reference/*.md files."
fi

# All reference/*.md files linked from SKILL.md must exist
missing_refs=""
while IFS= read -r ref; do
  if [[ ! -f "$SKILL_DIR/$ref" ]]; then
    missing_refs+="$ref "
  fi
done < <(grep -oE 'reference/[a-z0-9_-]+\.md' "$SKILL_MD" | sort -u)
[[ -z "$missing_refs" ]] || fail "SKILL.md links to missing reference files: $missing_refs"

info "✓ Validation passed ($body_lines body lines, portable frontmatter, refs exist)"

if [[ "$VALIDATE_ONLY" == "1" ]]; then
  exit 0
fi

# --- Routing eval ---

[[ -f "$EVAL_FILE" ]] || fail "Eval fixtures missing at $EVAL_FILE"
command -v ddx >/dev/null 2>&1 || fail "ddx binary not on PATH; routing eval needs it"
command -v jq >/dev/null 2>&1 || fail "jq required for reading routing.jsonl"

info "==> Running routing evals against harnesses: $HARNESSES"

fail_count=0
pass_count=0
IFS=',' read -ra harness_list <<< "$HARNESSES"

while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  phrase="$(echo "$line" | jq -r '.phrase')"
  expected_ref="$(echo "$line" | jq -r '.expected_reference')"
  expected_cli="$(echo "$line" | jq -r '.expected_cli')"

  for h in "${harness_list[@]}"; do
    # Skip harnesses that aren't available (avoid spurious failures in CI)
    if ! ddx agent list 2>/dev/null | grep -q "^$h"; then
      info "  (skip: harness $h not available)"
      continue
    fi

    response="$(ddx agent run --harness "$h" --text "$phrase" 2>&1 || true)"
    if echo "$response" | grep -qF "$expected_ref" \
      || echo "$response" | grep -qF "$expected_cli"; then
      pass_count=$((pass_count+1))
    else
      fail_count=$((fail_count+1))
      echo "  ✗ [$h] '$phrase' — expected hint of '$expected_ref' or '$expected_cli'" >&2
    fi
  done
done < "$EVAL_FILE"

info "==> Routing eval: $pass_count passed, $fail_count failed"

if [[ "$fail_count" -gt 0 ]]; then
  exit 1
fi
