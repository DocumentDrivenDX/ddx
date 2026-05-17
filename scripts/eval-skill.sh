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
#   - SKILL.md exists at library/skills/ddx/SKILL.md
#   - Frontmatter contains only `name` + `description` (portable agentskills.io minimum)
#   - Body is under 500 lines
#   - All reference/*.md files linked from SKILL.md exist
#
# Routing eval (skipped in --validate mode):
#   - For each row in library/skills/ddx/evals/routing.jsonl, invoke
#     `ddx agent run --harness <h> --text <phrase>` and check that the
#     response mentions the expected reference file OR the expected CLI command.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKILL_DIR="$REPO_ROOT/library/skills/ddx"
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

# --- Fixture schema validation ---

info "==> Validating routing fixture schema ($EVAL_FILE)"

schema_fail=0
row_count=0
while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  row_count=$((row_count + 1))
  for field in phrase mode references queue_commands tracker_mutation_allowed code_edits_allowed expected_next_action; do
    if ! echo "$line" | jq -e "has(\"$field\")" >/dev/null 2>&1; then
      echo "  SCHEMA FAIL row $row_count: missing field '$field'" >&2
      schema_fail=1
    fi
  done
  # references and queue_commands must be arrays
  for arr_field in references queue_commands; do
    arr_type="$(echo "$line" | jq -r ".${arr_field} | type" 2>/dev/null)"
    if [[ "$arr_type" != "array" ]]; then
      echo "  SCHEMA FAIL row $row_count: '$arr_field' must be an array (got $arr_type)" >&2
      schema_fail=1
    fi
  done
  # tracker_mutation_allowed and code_edits_allowed must be booleans
  for bool_field in tracker_mutation_allowed code_edits_allowed; do
    bool_type="$(echo "$line" | jq -r ".${bool_field} | type" 2>/dev/null)"
    if [[ "$bool_type" != "boolean" ]]; then
      echo "  SCHEMA FAIL row $row_count: '$bool_field' must be a boolean (got $bool_type)" >&2
      schema_fail=1
    fi
  done
done < "$EVAL_FILE"

[[ "$schema_fail" -eq 0 ]] || fail "routing.jsonl schema validation failed"
[[ "$row_count" -ge 15 ]] || fail "routing.jsonl must have at least 15 rows (got $row_count)"
info "✓ Fixture schema valid ($row_count rows)"

# --- Harness routing eval ---

info "==> Running routing evals against harnesses: $HARNESSES"

fail_count=0
pass_count=0
IFS=',' read -ra harness_list <<< "$HARNESSES"

while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  phrase="$(echo "$line" | jq -r '.phrase')"
  mode="$(echo "$line" | jq -r '.mode')"
  expected_next_action="$(echo "$line" | jq -r '.expected_next_action')"

  for h in "${harness_list[@]}"; do
    # Skip harnesses that aren't available (avoid spurious failures in CI)
    if ! ddx agent list 2>/dev/null | grep -q "^$h"; then
      info "  (skip: harness $h not available)"
      continue
    fi

    response="$(ddx agent run --harness "$h" --text "$phrase" 2>&1 || true)"

    # Check if response mentions any reference file from the fixture
    matched=0
    while IFS= read -r ref; do
      [[ -z "$ref" ]] && continue
      if echo "$response" | grep -qF "$ref"; then
        matched=1
        break
      fi
    done < <(echo "$line" | jq -r '.references[]' 2>/dev/null)

    # If no ref matched, check queue_commands
    if [[ "$matched" -eq 0 ]]; then
      while IFS= read -r cmd; do
        [[ -z "$cmd" ]] && continue
        if echo "$response" | grep -qF "$cmd"; then
          matched=1
          break
        fi
      done < <(echo "$line" | jq -r '.queue_commands[]' 2>/dev/null)
    fi

    if [[ "$matched" -eq 1 ]]; then
      pass_count=$((pass_count+1))
    else
      fail_count=$((fail_count+1))
      refs_str="$(echo "$line" | jq -r '[.references[]] | join(", ")' 2>/dev/null)"
      cmds_str="$(echo "$line" | jq -r '[.queue_commands[]] | join(", ")' 2>/dev/null)"
      echo "  ✗ [$h] '$phrase' (mode=$mode, next=$expected_next_action)" >&2
      echo "      expected ref in: ${refs_str:-none}" >&2
      echo "      expected cmd in: ${cmds_str:-none}" >&2
    fi
  done
done < "$EVAL_FILE"

info "==> Routing eval: $pass_count passed, $fail_count failed"

if [[ "$fail_count" -gt 0 ]]; then
  exit 1
fi
