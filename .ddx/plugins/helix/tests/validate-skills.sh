#!/usr/bin/env bash
set -euo pipefail

repo_root="${HELIX_VALIDATE_SKILLS_REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
skills_dir="$repo_root/skills"
agents_package_dir="$repo_root/.agents/skills"
claude_package_dir="$repo_root/.claude/skills"

declare -A skills_requiring_argument_hint=(
  [helix-align]=1
  [helix-backfill]=1
  [helix-check]=1
  [helix-experiment]=1
  [helix-build]=1
  [helix-design]=1
  [helix-evolve]=1
  [helix-implement]=1
  [helix-plan]=1
  [helix-polish]=1
  [helix-review]=1
  [helix-run]=1
  [helix-triage]=1
)

fail() {
  printf 'skill validation failed: %s\n' "$*" >&2
  exit 1
}

assert_file_contains() {
  local path="$1"
  local needle="$2"
  local message="$3"

  [[ -f "$path" ]] || fail "missing file for validation: $path"
  if ! grep -Fq "$needle" "$path"; then
    fail "$message"
  fi
}

assert_file_not_contains() {
  local path="$1"
  local needle="$2"
  local message="$3"

  [[ -f "$path" ]] || fail "missing file for validation: $path"
  if grep -Fq "$needle" "$path"; then
    fail "$message"
  fi
}

assert_command_fails() {
  local output_file="$1"
  local message="$2"
  shift 2

  if "$@" >"$output_file" 2>&1; then
    fail "$message"
  fi
}

assert_output_contains() {
  local haystack="$1"
  local needle="$2"
  local message="$3"

  if [[ "$haystack" != *"$needle"* ]]; then
    printf 'expected substring: %s\nin:\n%s\n' "$needle" "$haystack" >&2
    fail "$message"
  fi
}

validate_helix_triage_intro() {
  local path="$1"
  local intro normalized
  local blanket_execution_ready_pattern

  [[ -f "$path" ]] || fail "missing file for validation: $path"
  intro="$(
    awk '
      /^# Triage: Shape Execution-Ready And Planning Issues$/ { in_intro = 1; next }
      in_intro && /^## / { exit }
      in_intro { print }
    ' "$path"
  )"
  [[ -n "$intro" ]] || fail "helix-triage intro block is missing"

  normalized="$(
    printf '%s\n' "$intro" \
      | tr '[:upper:]' '[:lower:]' \
      | tr '\n' ' ' \
      | tr -s '[:space:]' ' '
  )"

  blanket_execution_ready_pattern='(every|all|each)([[:space:]]+[[:alpha:]][[:alpha:]-]*)*[[:space:]]+(issue|issues|bead|beads|task|tasks|work[[:space:]]+item|work[[:space:]]+items)[[:space:]]+should[[:space:]]+enter[[:space:]]+the[[:space:]]+tracker[[:space:]]+ready[[:space:]]+(to[[:space:]]+execute|for[[:space:]]+execution)'
  if [[ "$normalized" =~ $blanket_execution_ready_pattern ]]; then
    fail "helix-triage intro must not prime every issue as execution-ready"
  fi
}

assert_helix_triage_blanket_priming_regression() {
  local temp_root output_file regression_output
  local -a blanket_priming_sentences=(
    "All implementation issues should enter the tracker ready to execute when possible."
    "All execution-ready implementation planning and review issues should enter the tracker ready to execute when possible."
  )

  if [[ "${HELIX_VALIDATE_SKILLS_SKIP_REGRESSION:-0}" == "1" ]]; then
    return
  fi

  command -v python3 >/dev/null 2>&1 || fail "python3 is required for execution-ready bead validation"

  temp_root="$(mktemp -d)"
  output_file="$(mktemp)"

  cp -Rf \
    "$repo_root/.agents" \
    "$repo_root/.claude" \
    "$repo_root/.claude-plugin" \
    "$repo_root/bin" \
    "$repo_root/docs" \
    "$repo_root/hooks" \
    "$repo_root/skills" \
    "$repo_root/workflows" \
    "$temp_root/"
  mkdir -p "$temp_root/tests"
  cp -f "$repo_root/tests/validate-skills.sh" "$temp_root/tests/validate-skills.sh"

  for blanket_priming_sentence in "${blanket_priming_sentences[@]}"; do
    python3 - "$temp_root/skills/helix-triage/SKILL.md" "$blanket_priming_sentence" <<'PYEOF'
from pathlib import Path
import sys

path = Path(sys.argv[1])
blanket_priming_sentence = sys.argv[2]
text = path.read_text(encoding="utf-8")
needle = "# Triage: Shape Execution-Ready And Planning Issues\n\n"
replacement = (
    "# Triage: Shape Execution-Ready And Planning Issues\n\n"
    f"{blanket_priming_sentence}\n\n"
)
if needle not in text:
    raise SystemExit("missing helix-triage heading in regression fixture")
path.write_text(text.replace(needle, replacement, 1), encoding="utf-8")
PYEOF

    assert_command_fails \
      "$output_file" \
      "validate-skills must fail when helix-triage regains blanket execution-ready priming" \
      env \
        HELIX_VALIDATE_SKILLS_REPO_ROOT="$temp_root" \
        HELIX_VALIDATE_SKILLS_SKIP_REGRESSION=1 \
        bash "$temp_root/tests/validate-skills.sh"

    regression_output="$(<"$output_file")"
    assert_output_contains \
      "$regression_output" \
      "helix-triage intro must not prime every issue as execution-ready" \
      "validate-skills must report the blanket execution-ready triage regression"

    cp -f "$repo_root/skills/helix-triage/SKILL.md" "$temp_root/skills/helix-triage/SKILL.md"
  done

  rm -rf "$temp_root"
  rm -f "$output_file"
}

# ---------- Plugin layout checks ----------

plugin_manifest="$repo_root/.claude-plugin/plugin.json"
[[ -f "$plugin_manifest" ]] || fail "missing plugin manifest at .claude-plugin/plugin.json"
plugin_bin="$repo_root/bin/helix"
plugin_hooks="$repo_root/hooks/hooks.json"

# Validate plugin.json is parseable JSON with required fields
if command -v python3 &>/dev/null; then
  python3 - "$plugin_manifest" "$plugin_bin" "$plugin_hooks" <<'PYEOF'
import json, sys
path, plugin_bin, plugin_hooks = sys.argv[1:4]
try:
    manifest = json.load(open(path))
except json.JSONDecodeError as e:
    print(f"invalid JSON in {path}: {e}", file=sys.stderr)
    sys.exit(1)
required = ("name", "version", "description", "skills", "hooks")
missing = [k for k in required if not manifest.get(k)]
if missing:
    print(f"plugin.json missing required fields: {', '.join(missing)}", file=sys.stderr)
    sys.exit(1)
if manifest["skills"] != "./skills/":
    print(f"plugin.json skills must be ./skills/ (got {manifest['skills']!r})", file=sys.stderr)
    sys.exit(1)
if manifest["hooks"] != "./hooks/hooks.json":
    print(f"plugin.json hooks must be ./hooks/hooks.json (got {manifest['hooks']!r})", file=sys.stderr)
    sys.exit(1)
try:
    hooks = json.load(open(plugin_hooks))
except json.JSONDecodeError as e:
    print(f"invalid JSON in {plugin_hooks}: {e}", file=sys.stderr)
    sys.exit(1)
if not isinstance(hooks, dict):
    print(f"{plugin_hooks} must contain a JSON object", file=sys.stderr)
    sys.exit(1)
if "version" not in hooks or "hooks" not in hooks:
    print(f"{plugin_hooks} must define version and hooks keys", file=sys.stderr)
    sys.exit(1)
PYEOF
  [[ $? -eq 0 ]] || fail "plugin.json validation failed"
fi

[[ -f "$plugin_bin" ]] || fail "missing plugin wrapper at bin/helix"
[[ -x "$plugin_bin" ]] || fail "expected bin/helix to be executable"
[[ -f "$plugin_hooks" ]] || fail "missing plugin hooks at hooks/hooks.json"

write_wrapper_probe() {
  local path="$1"
  local label="$2"

  cat >"$path" <<EOF
#!/usr/bin/env bash
set -euo pipefail
printf 'probe=%s\n' '$label'
printf 'HELIX_ROOT=%s\n' "\${HELIX_ROOT:-}"
printf 'ARGS=%s\n' "\$*"
EOF
  chmod +x "$path"
}

plugin_wrapper_root="$(mktemp -d)"
claude_plugin_root="$(mktemp -d)"
mkdir -p \
  "$plugin_wrapper_root/bin" \
  "$plugin_wrapper_root/scripts" \
  "$claude_plugin_root/scripts"
cp -f "$plugin_bin" "$plugin_wrapper_root/bin/helix"
write_wrapper_probe "$plugin_wrapper_root/scripts/helix" "wrapper-path"
write_wrapper_probe "$claude_plugin_root/scripts/helix" "claude-plugin-root"

wrapper_output="$(
  env -u HELIX_ROOT -u CLAUDE_PLUGIN_ROOT \
    "$plugin_wrapper_root/bin/helix" probe 2>&1
)"
assert_output_contains \
  "$wrapper_output" \
  "probe=wrapper-path" \
  "bin/helix should delegate to scripts/helix resolved from its own path"
assert_output_contains \
  "$wrapper_output" \
  "HELIX_ROOT=$plugin_wrapper_root" \
  "bin/helix should export HELIX_ROOT from wrapper path resolution"
assert_output_contains \
  "$wrapper_output" \
  "ARGS=probe" \
  "bin/helix should forward arguments through wrapper-path delegation"

claude_wrapper_output="$(
  env -u HELIX_ROOT \
    CLAUDE_PLUGIN_ROOT="$claude_plugin_root" \
    "$plugin_wrapper_root/bin/helix" probe 2>&1
)"
assert_output_contains \
  "$claude_wrapper_output" \
  "probe=claude-plugin-root" \
  "bin/helix should delegate to CLAUDE_PLUGIN_ROOT/scripts/helix"
assert_output_contains \
  "$claude_wrapper_output" \
  "HELIX_ROOT=$claude_plugin_root" \
  "bin/helix should export HELIX_ROOT from CLAUDE_PLUGIN_ROOT"
assert_output_contains \
  "$claude_wrapper_output" \
  "ARGS=probe" \
  "bin/helix should forward arguments through CLAUDE_PLUGIN_ROOT delegation"
rm -rf "$plugin_wrapper_root" "$claude_plugin_root"

# Verify that workflows/ references in SKILL.md files resolve from plugin root
while IFS= read -r wf_ref; do
  [[ -n "$wf_ref" ]] || continue
  wf_path="$repo_root/$wf_ref"
  [[ -f "$wf_path" ]] || fail "a SKILL.md references $wf_ref which does not exist at $wf_path"
done < <(grep -roh "workflows/[a-zA-Z0-9/_.-]*\.md" "$repo_root/skills"/*/SKILL.md 2>/dev/null | sort -u)

# ------------------------------------------

extract_frontmatter() {
  local skill_file="$1"

  awk '
    NR == 1 && $0 == "---" { in_frontmatter = 1; next }
    in_frontmatter && $0 == "---" { exit }
    in_frontmatter { print }
  ' "$skill_file"
}

extract_field() {
  local frontmatter="$1"
  local key="$2"

  printf '%s\n' "$frontmatter" | sed -n "s/^${key}:[[:space:]]*//p" | head -n1
}

[[ -d "$skills_dir" ]] || fail "missing skills directory at $skills_dir"
[[ -d "$agents_package_dir" ]] || fail "missing package directory at $agents_package_dir"
[[ -d "$claude_package_dir" ]] || fail "missing package directory at $claude_package_dir"

shopt -s nullglob
skill_dirs=("$skills_dir"/helix-*)
[[ "${#skill_dirs[@]}" -gt 0 ]] || fail "no published skills found under $skills_dir"

mapfile -t expected_skills < <(
  for path in "${skill_dirs[@]}"; do
    [[ -d "$path" ]] || continue
    printf '%s\n' "${path##*/}"
  done | sort
)

# Validate .agents/skills/ symlinks
mapfile -t agents_published_skills < <(
  for path in "$agents_package_dir"/*; do
    [[ -e "$path" || -L "$path" ]] || continue
    printf '%s\n' "${path##*/}"
  done | sort
)

# Validate .claude/skills/ symlinks
mapfile -t claude_published_skills < <(
  for path in "$claude_package_dir"/*; do
    [[ -e "$path" || -L "$path" ]] || continue
    printf '%s\n' "${path##*/}"
  done | sort
)

# Check .agents/skills/ matches skills/
expected_list="$(printf '%s\n' "${expected_skills[@]}")"
agents_published_list="$(printf '%s\n' "${agents_published_skills[@]}")"
[[ "$expected_list" == "$agents_published_list" ]] || fail "published skills in .agents/skills do not match skills/"

# Check .claude/skills/ matches skills/
claude_published_list="$(printf '%s\n' "${claude_published_skills[@]}")"
[[ "$expected_list" == "$claude_published_list" ]] || fail "published skills in .claude/skills do not match skills/"

for name in "${expected_skills[@]}"; do
  skill_file="$skills_dir/$name/SKILL.md"
  agents_package_link="$agents_package_dir/$name"
  claude_package_link="$claude_package_dir/$name"

  [[ -f "$skill_file" ]] || fail "missing SKILL.md for $name"

  # Validate .agents/skills/ symlinks
  [[ -L "$agents_package_link" ]] || fail "expected $agents_package_link to be a symlink"
  [[ "$(readlink "$agents_package_link")" == "../../skills/$name" ]] || fail "expected $agents_package_link to target ../../skills/$name"

  # Validate .claude/skills/ symlinks
  [[ -L "$claude_package_link" ]] || fail "expected $claude_package_link to be a symlink"
  [[ "$(readlink "$claude_package_link")" == "../../skills/$name" ]] || fail "expected $claude_package_link to target ../../skills/$name"

  frontmatter="$(extract_frontmatter "$skill_file")"
  [[ -n "$frontmatter" ]] || fail "missing frontmatter in $skill_file"

  skill_name="$(extract_field "$frontmatter" "name")"
  description="$(extract_field "$frontmatter" "description")"
  argument_hint="$(extract_field "$frontmatter" "argument-hint")"

  # YAML syntax check: detect unquoted colons in values that break parsers.
  # Codex's skill loader rejects these with "invalid YAML: mapping values
  # are not allowed in this context".
  while IFS= read -r line; do
    # Skip lines that are properly quoted (single or double quotes after key:)
    if [[ "$line" =~ ^[a-z-]+:\ *[\'\"] ]]; then
      continue
    fi
    # Check for a second colon-space in an unquoted value
    key_removed="${line#*: }"
    if [[ "$key_removed" == *": "* ]]; then
      fail "unquoted colon in $skill_file frontmatter: $line — wrap the value in quotes"
    fi
  done <<< "$frontmatter"

  [[ -n "$skill_name" ]] || fail "missing name field in $skill_file"
  # For symlinked skill directories, the SKILL.md name should match the
  # symlink target (not the link name), since the link is just an alias.
  if [[ -L "$skills_dir/$name" ]]; then
    target_name="$(basename "$(readlink "$skills_dir/$name")")"
    [[ "$skill_name" == "$target_name" ]] || fail "frontmatter name $skill_name does not match symlink target $target_name for alias $name"
  else
    [[ "$skill_name" == "$name" ]] || fail "frontmatter name $skill_name does not match directory $name"
  fi
  [[ -n "$description" ]] || fail "missing description field in $skill_file"

  if [[ -n "${skills_requiring_argument_hint[$name]:-}" && -z "$argument_hint" ]]; then
    fail "missing argument-hint field in $skill_file"
  fi
done

assert_file_contains \
  "$repo_root/skills/helix-triage/SKILL.md" \
  "Execution-ready implementation beads should enter the tracker ready to" \
  "helix-triage must scope execution-ready wording to implementation beads only"
assert_file_contains \
  "$repo_root/skills/helix-triage/SKILL.md" \
  "route it to planning/polish or file it explicitly as a" \
  "helix-triage intro must direct vague requests to planning or explicit not-execution-ready paths"
validate_helix_triage_intro "$repo_root/skills/helix-triage/SKILL.md"
assert_file_not_contains \
  "$repo_root/skills/helix-triage/SKILL.md" \
  "Every issue should enter the tracker ready to execute." \
  "helix-triage must not claim every issue enters the tracker ready to execute"
assert_file_contains \
  "$repo_root/skills/helix-triage/SKILL.md" \
  "deterministic acceptance and success-measurement criteria" \
  "helix-triage must require success-measurement criteria for execution-ready beads"
assert_file_contains \
  "$repo_root/skills/helix-triage/SKILL.md" \
  "Triage must not create execution-ready implementation beads without" \
  "helix-triage must block queue-ready implementation beads that lack measurable success criteria"
assert_file_contains \
  "$repo_root/skills/helix-triage/SKILL.md" \
  "the work back to planning/polish, or file it as a not-execution-ready" \
  "helix-triage must define a planning/polish or not-execution-ready fallback for vague build beads"
assert_file_contains \
  "$repo_root/skills/helix-triage/SKILL.md" \
  "not-execution-ready" \
  "helix-triage fallback must explicitly preserve not-execution-ready status for vague build beads"
assert_file_contains \
  "$repo_root/skills/helix-triage/SKILL.md" \
  "DDx-managed execution to close merged work with evidence" \
  "helix-triage must explain DDx-managed close-with-evidence expectations"
assert_file_contains \
  "$repo_root/skills/helix-polish/SKILL.md" \
  "require execution-ready beads to name exact commands," \
  "helix-polish must require explicit measurable acceptance text for execution-ready beads"
assert_file_contains \
  "$repo_root/skills/helix-polish/SKILL.md" \
  "flag it as not execution-ready" \
  "helix-polish must define a flagging path for non-measurable acceptance text"
assert_file_contains \
  "$repo_root/workflows/actions/polish.md" \
  "Treat \"works\", \"correct\", \"complete\", \"aligned\", or similar adjectives" \
  "polish action must reject vague non-measurable acceptance wording"
assert_file_contains \
  "$repo_root/workflows/actions/polish.md" \
  "flag the bead as **not execution-ready**" \
  "polish action must define a not-execution-ready flagging path"
assert_file_contains \
  "$repo_root/docs/helix/01-frame/features/FEAT-011-slider-autonomy.md" \
  "### AC-09: Automation-Friendly Success Criteria" \
  "FEAT-011 must retain the automation-friendly success-criteria acceptance contract"
assert_file_contains \
  "$repo_root/docs/helix/02-design/contracts/CONTRACT-001-ddx-helix-boundary.md" \
  "HELIX-authored execution beads must make success machine-auditable." \
  "CONTRACT-001 must retain machine-auditable bead success criteria"
assert_file_contains \
  "$repo_root/docs/helix/02-design/technical-designs/TD-011-slider-autonomy-implementation.md" \
  "### Decision 5c: Bead Success-Measurement Contract" \
  "TD-011 must retain the bead success-measurement decision"
assert_helix_triage_blanket_priming_regression

command -v python3 >/dev/null 2>&1 || fail "python3 is required for execution-ready bead validation"
mixed_fixture="$repo_root/tests/fixtures/execution-ready-beads/mixed-ready-semantics.jsonl"
mixed_tracker_dir="$(mktemp -d)"
mkdir -p "$mixed_tracker_dir/.ddx"
cp -f "$mixed_fixture" "$mixed_tracker_dir/.ddx/beads.jsonl"
mixed_expected_ids="$(mktemp)"
mixed_actual_ids="$(mktemp)"
DDX_BEAD_DIR="$mixed_tracker_dir/.ddx" ddx bead ready --execution --json >"$mixed_expected_ids"
python3 - "$repo_root/scripts/validate_execution_ready_beads.py" "$mixed_tracker_dir/.ddx/beads.jsonl" >"$mixed_actual_ids" <<'PYEOF'
import importlib.util
import json
import sys
from pathlib import Path

script_path, tracker_path = sys.argv[1:3]
spec = importlib.util.spec_from_file_location("validate_execution_ready_beads", script_path)
module = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(module)
ready = module.execution_ready_beads(module.load_beads(Path(tracker_path)))
json.dump([bead["id"] for _, bead in ready], sys.stdout)
PYEOF
python3 - "$mixed_expected_ids" "$mixed_actual_ids" <<'PYEOF'
import json
import sys

expected_path, actual_path = sys.argv[1:3]
with open(expected_path, "r", encoding="utf-8") as handle:
    expected = sorted(entry["id"] for entry in json.load(handle))
with open(actual_path, "r", encoding="utf-8") as handle:
    actual = sorted(json.load(handle))
if expected != actual:
    print(f"expected ready ids {expected}, got {actual}", file=sys.stderr)
    sys.exit(1)
PYEOF
mixed_reject_output="$(mktemp)"
assert_command_fails \
  "$mixed_reject_output" \
  "execution-ready validator should reject only the vague bead from the mixed queue fixture" \
  python3 \
  "$repo_root/scripts/validate_execution_ready_beads.py" \
  "$mixed_fixture"
grep -Fq "hx-ready-vague" "$mixed_reject_output" || fail \
  "mixed execution-ready fixture should identify the ready vague bead"
for skipped_id in hx-deferred-build hx-closed-build hx-in-progress-build hx-blocked-build hx-not-execution-eligible hx-superseded-build; do
  if grep -Fq "$skipped_id" "$mixed_reject_output"; then
    fail "mixed execution-ready fixture should skip non-ready bead $skipped_id"
  fi
done
rm -f "$mixed_expected_ids" "$mixed_actual_ids" "$mixed_reject_output"
rm -rf "$mixed_tracker_dir"

reject_output="$(mktemp)"
assert_command_fails \
  "$reject_output" \
  "execution-ready validator should reject vague acceptance fixtures" \
  python3 \
  "$repo_root/scripts/validate_execution_ready_beads.py" \
  "$repo_root/tests/fixtures/execution-ready-beads/vague-acceptance.jsonl"
grep -Fq "hx-vague-ac" "$reject_output" || fail "execution-ready validator should identify the rejected fixture"
rm -f "$reject_output"

flagged_fixture="$repo_root/tests/fixtures/execution-ready-beads/flagged-acceptance.jsonl"
assert_file_contains \
  "$flagged_fixture" \
  "\"execution-eligible\":false" \
  "flagged acceptance fixture must stay marked not execution-ready"
assert_file_contains \
  "$flagged_fixture" \
  "flagged by polish for non-measurable acceptance" \
  "flagged acceptance fixture must record non-measurable acceptance as the reason it is not execution-ready"
flagged_output="$(mktemp)"
python3 "$repo_root/scripts/validate_execution_ready_beads.py" \
  "$flagged_fixture" \
  >"$flagged_output" 2>&1
grep -Fq "validated measurable acceptance on 0 execution-ready bead(s)" "$flagged_output" || fail \
  "flagged acceptance fixture should be skipped once it is marked not execution-ready"
rm -f "$flagged_output"

printf 'validated %d HELIX skills\n' "${#expected_skills[@]}"
