#!/usr/bin/env bash
# HELIX Evolve Demo — scripted asciinema recording
#
# Demonstrates how `helix evolve` threads a new requirement through
# the entire artifact stack:
#   1. Setup: create a project with existing artifacts (PRD, design, tests, code)
#   2. Evolve: add a new requirement ("add Kelvin support")
#   3. Propagate: watch the agent update PRD, design, and create tracker issues
#   4. Build: implement the new requirement from the generated issues
#   5. Verify: confirm the evolution is complete
#
# Uses `ddx agent run` as the agent harness.
#
set -euo pipefail

RECORDING_FILE="/recordings/helix-evolve-$(date +%Y%m%d-%H%M%S).cast"
MAX_RETRIES=3
COOLDOWN=3
AGENT_PACE="${DEMO_PACE:-5}"

# Auto-detect helix repo
if [[ -d /helix/workflows ]]; then
  HELIX_ROOT="/helix"
elif [[ -d "$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/workflows" ]]; then
  HELIX_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
else
  echo "FAIL: cannot find helix repo" >&2
  exit 1
fi

if ! command -v ddx >/dev/null 2>&1; then
  if [[ -x /ddx/ddx ]]; then
    export PATH="/ddx:$PATH"
  elif [[ -x "$HELIX_ROOT/../ddx/ddx" ]]; then
    export PATH="$HELIX_ROOT/../ddx:$PATH"
  else
    echo "FAIL: ddx not found" >&2
    exit 1
  fi
fi

# ── Display helpers ───────────────────────────────────────────

narrate() {
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  $1"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""
  sleep 2
}

run() {
  echo "$ $*"
  "$@"
  echo ""
  sleep 1
}

show_file() {
  local file="$1"
  local lines="${2:-25}"
  echo "── $file ──"
  head -n "$lines" "$file" 2>/dev/null || echo "(file not found)"
  echo "..."
  echo ""
  sleep 2
}

# Resolve the demo source directory (handles both local and Docker paths).
demo_source_dir() {
  if [[ -d "$HELIX_ROOT/docs/demos/helix-evolve" ]]; then
    echo "$HELIX_ROOT/docs/demos/helix-evolve"
  else
    cd "$(dirname "${BASH_SOURCE[0]}")" && pwd
  fi
}

# Seed the virtual agent dictionary from pre-recorded responses.
seed_dictionary() {
  local src
  src="$(demo_source_dir)"
  if [[ -d "$src/agent-dictionary" ]]; then
    mkdir -p .ddx/agent-dictionary
    cp -f "$src"/agent-dictionary/*.json .ddx/agent-dictionary/ 2>/dev/null || true
  fi
}

# In virtual mode, apply specific fixture files the real agent would have created.
# Usage: apply_fixtures docs tests bin
apply_fixtures() {
  [[ "${DEMO_HARNESS:-claude}" == "virtual" ]] || return 0
  local src
  src="$(demo_source_dir)"
  [[ -d "$src/fixtures" ]] || return 0
  for item in "$@"; do
    if [[ -d "$src/fixtures/$item" ]]; then
      cp -rf "$src/fixtures/$item" .
    elif [[ -f "$src/fixtures/$item" ]]; then
      mkdir -p "$(dirname "$item")"
      cp -f "$src/fixtures/$item" "$item"
    fi
  done
}

# After recording, save dictionary and fixtures back to the demo source.
save_recording() {
  local src
  src="$(demo_source_dir)"
  if [[ -d .ddx/agent-dictionary ]]; then
    mkdir -p "$src/agent-dictionary"
    cp -f .ddx/agent-dictionary/*.json "$src/agent-dictionary/" 2>/dev/null || true
  fi
  # Save generated project files as fixtures for virtual replay
  mkdir -p "$src/fixtures"
  for d in docs tests bin; do
    [[ -d "$d" ]] && cp -rf "$d" "$src/fixtures/"
  done
  [[ -f package.json ]] && cp -f package.json "$src/fixtures/"
  echo "Saved recordings and fixtures to $src/"
}

agent_run() {
  local prompt=""
  if [[ $# -gt 0 ]]; then
    prompt="$*"
  else
    prompt="$(cat)"
  fi

  echo '$ ddx agent run --text "'"${prompt:0:80}"'..."'
  echo ""

  local attempt output
  for attempt in $(seq 1 "$MAX_RETRIES"); do
    local -a agent_flags=(
      --harness "${DEMO_HARNESS:-claude}"
      --model "${DEMO_MODEL:-claude-haiku-4-5-20251001}"
      --text "$prompt"
    )
    [[ "${DEMO_RECORD:-0}" == "1" ]] && agent_flags+=(--record)
    output=$(ddx agent run "${agent_flags[@]}" 2>/dev/null) || true

    if [[ -n "$output" && "$output" != "Execution error" ]]; then
      break
    fi
    if [[ $attempt -lt $MAX_RETRIES ]]; then
      echo "  (retrying $attempt/$MAX_RETRIES...)"
      sleep $((attempt * 3))
    fi
  done

  if [[ -n "$output" && "$output" != "Execution error" ]]; then
    local result
    result=$(printf '%s' "$output" | ddx jq -r '.result // empty' 2>/dev/null) || true
    if [[ -n "$result" ]]; then
      printf '%s\n' "$result"
    else
      printf '%s\n' "$output"
    fi
  fi

  echo ""
  sleep "$AGENT_PACE"
  sleep "$COOLDOWN"
}

require_file() {
  local file="$1"
  local label="${2:-$file}"
  if [[ ! -f "$file" ]]; then
    echo "FAIL: $label not found — aborting"
    exit 1
  fi
  echo "  ✓ $label exists"
}

# ── Demo body ─────────────────────────────────────────────────

demo_body() {
  # ── ACT 1: Setup — Existing Project ────────────────────────
  narrate "ACT 1: Setup — An Existing Project"

  echo "Installing HELIX..."
  export HELIX_LIBRARY_ROOT="$HELIX_ROOT/workflows"
  export PATH="$HELIX_ROOT/scripts:$HELIX_ROOT/bin:$PATH"
  mkdir -p ~/.ddx/plugins && ln -sfn "$HELIX_ROOT" ~/.ddx/plugins/helix && bash "$HELIX_ROOT/scripts/helix" doctor --fix 2>&1
  mkdir -p ~/.agents/skills ~/.claude/skills && for s in "$HELIX_ROOT"/.agents/skills/helix-*; do ln -sfn "$s" ~/.agents/skills/$(basename "$s"); ln -sfn "$s" ~/.claude/skills/$(basename "$s"); done
  echo ""

  run git init temp-converter
  cd temp-converter
  run ddx bead init
  seed_dictionary

  mkdir -p .agents .claude
  cp -rf ~/.agents/skills .agents/
  cat > .claude/settings.json <<'SETTINGS'
{
  "permissions": {
    "allow": ["Bash(*)", "Read(*)", "Write(*)", "Edit(*)"]
  }
}
SETTINGS

  cat > AGENTS.md <<'AGENTS'
# Agent Instructions

Temperature converter CLI. Converts between Fahrenheit and Celsius.

## Quick Reference

```bash
npm test                        # Run tests
node bin/convert.js --to-celsius 212  # 100.0
```
AGENTS

  # Create existing artifacts — the project already has a working v1
  mkdir -p docs/helix/00-discover docs/helix/01-frame/features \
           docs/helix/02-design/technical-designs bin tests

  cat > docs/helix/01-frame/prd.md <<'PRD'
# Product Requirements

## Summary
A CLI tool for temperature conversion between Fahrenheit and Celsius.

## P0 Requirements
- P0-1: `convert --to-celsius <temp>` converts F→C
- P0-2: `convert --to-fahrenheit <temp>` converts C→F
- P0-3: Output with one decimal place

## P1 Requirements
- P1-1: Error message for non-numeric input
PRD

  cat > docs/helix/01-frame/features/FEAT-001-temperature-conversion.md <<'FEAT'
# FEAT-001: Temperature Conversion

## Functional Requirements
- FR-1: toFahrenheit(celsius) returns fahrenheit
- FR-2: toCelsius(fahrenheit) returns celsius
- FR-3: CLI accepts --to-celsius and --to-fahrenheit flags
- FR-4: Output rounded to one decimal place

## Acceptance Criteria
- convert --to-celsius 212 → 100.0
- convert --to-fahrenheit 0 → 32.0
- convert --to-celsius 98.6 → 37.0
FEAT

  cat > docs/helix/02-design/technical-designs/TD-001-temperature-conversion.md <<'TD'
# TD-001: Temperature Conversion

## Architecture
- Single file: bin/convert.js
- Exports: toFahrenheit(c), toCelsius(f)
- CLI: parse process.argv for flags
- Formulas: F = C * 9/5 + 32, C = (F - 32) * 5/9
TD

  cat > package.json <<'PKG'
{"name":"temp-converter","version":"1.0.0","scripts":{"test":"node --test"}}
PKG

  cat > bin/convert.js <<'CODE'
function toFahrenheit(c) { return c * 9/5 + 32; }
function toCelsius(f) { return (f - 32) * 5/9; }

if (require.main === module) {
  const args = process.argv.slice(2);
  const flag = args[0];
  const temp = parseFloat(args[1]);
  if (flag === '--to-celsius') console.log(toCelsius(temp).toFixed(1));
  else if (flag === '--to-fahrenheit') console.log(toFahrenheit(temp).toFixed(1));
  else { console.error('Usage: convert --to-celsius|--to-fahrenheit <temp>'); process.exit(1); }
}

module.exports = { toFahrenheit, toCelsius };
CODE

  cat > tests/convert.test.js <<'TEST'
const { describe, it } = require('node:test');
const assert = require('node:assert');
const { toFahrenheit, toCelsius } = require('../bin/convert.js');

describe('temperature conversion', () => {
  it('converts 0°C to 32°F', () => assert.strictEqual(toFahrenheit(0), 32));
  it('converts 212°F to 100°C', () => assert.strictEqual(toCelsius(212), 100));
  it('converts 98.6°F to 37°C', () => assert.strictEqual(Math.round(toCelsius(98.6) * 10) / 10, 37));
});
TEST

  echo "Existing project with working v1:"
  run npm test
  echo ""
  show_file docs/helix/01-frame/prd.md
  git add -A && git commit -m "v1.0: working temperature converter" --quiet
  sleep 2

  # ── ACT 2: Evolve — New Requirement ────────────────────────
  narrate "ACT 2: Evolve — Add Kelvin Support"

  echo "New requirement: 'Add Kelvin conversion support'"
  echo ""
  echo "helix evolve threads this through the entire artifact stack:"
  echo "  PRD → Feature Spec → Technical Design → Tracker Issues"
  echo ""

  agent_run <<'EVOLVE_PROMPT'
You are performing a HELIX evolve action. A new requirement has arrived:

"Add Kelvin conversion support: --to-kelvin and --from-kelvin flags"

Thread this requirement through the existing artifact stack:

1. UPDATE docs/helix/01-frame/prd.md:
   - Add P0-4: convert --to-kelvin <temp> converts C→K
   - Add P0-5: convert --from-kelvin <temp> converts K→C

2. UPDATE docs/helix/01-frame/features/FEAT-001-temperature-conversion.md:
   - Add FR-5: toKelvin(celsius) returns kelvin
   - Add FR-6: fromKelvin(kelvin) returns celsius
   - Add acceptance criteria: --to-kelvin 100 → 373.1, --from-kelvin 273.15 → 0.0

3. UPDATE docs/helix/02-design/technical-designs/TD-001-temperature-conversion.md:
   - Add toKelvin(c) and fromKelvin(k) functions
   - Add --to-kelvin and --from-kelvin CLI flags
   - Formula: K = C + 273.15

4. CREATE tracker issues:
   ddx bead create "Add failing tests for Kelvin conversion" \
     --type task --priority 1 --labels helix,phase:build \
     --spec-id FEAT-001 --acceptance "tests for toKelvin and fromKelvin exist and FAIL"

   ddx bead create "Implement Kelvin conversion in bin/convert.js" \
     --type task --priority 1 --labels helix,phase:build \
     --spec-id TD-001 --acceptance "npm test passes with Kelvin tests, --to-kelvin and --from-kelvin work"

Make minimal, scoped changes to each artifact. Do not rewrite sections
that are unaffected by the Kelvin requirement.
EVOLVE_PROMPT
  apply_fixtures docs

  echo "Checking what changed..."
  echo ""
  echo "Updated artifacts:"
  git diff --stat HEAD
  echo ""
  show_file docs/helix/01-frame/prd.md
  show_file docs/helix/01-frame/features/FEAT-001-temperature-conversion.md 30

  echo "New tracker issues:"
  run ddx bead list
  git add -A && git commit -m "evolve: thread Kelvin requirement through artifact stack" --quiet
  sleep 2

  # ── ACT 3: Build — Implement from Issues ───────────────────
  narrate "ACT 3: Build — Implement the Evolution"

  echo "▶ Red phase: failing Kelvin tests..."
  agent_run <<'RED_PROMPT'
Add failing tests for Kelvin conversion to tests/convert.test.js:
- toKelvin(0) === 273.15
- toKelvin(100) === 373.15
- fromKelvin(273.15) === 0
- fromKelvin(0) === -273.15

Do NOT modify bin/convert.js yet — tests must FAIL.
Then claim and close the test issue from the tracker.
RED_PROMPT
  apply_fixtures tests

  echo "Red phase — new tests should fail:"
  npm test 2>&1 || true
  echo ""
  git add -A && git commit -m "test: failing Kelvin tests (Red)" --quiet
  sleep 2

  echo ""
  echo "▶ Green phase: implement Kelvin..."
  agent_run <<'GREEN_PROMPT'
Add toKelvin(c) and fromKelvin(k) to bin/convert.js:
- toKelvin(c) = c + 273.15
- fromKelvin(k) = k - 273.15
- Add --to-kelvin and --from-kelvin CLI flag handling
- Run npm test — all tests must pass

Then claim and close the implementation issue from the tracker.
GREEN_PROMPT
  apply_fixtures bin

  # ── ACT 4: Verify ──────────────────────────────────────────
  narrate "ACT 4: Verify"

  echo "Tests:"
  npm test 2>&1 || { echo "FAIL: npm test failed"; exit 1; }
  echo "  ✓ all tests pass"
  echo ""

  echo "New acceptance criteria:"
  local out
  out=$(node bin/convert.js --to-kelvin 100)
  echo "  convert --to-kelvin 100 → $out"

  out=$(node bin/convert.js --from-kelvin 273.15)
  echo "  convert --from-kelvin 273.15 → $out"

  echo ""
  git add -A && git commit -m "build: Kelvin conversion (Green)" --quiet
  sleep 2

  # ── Summary ─────────────────────────────────────────────────
  narrate "Demo Complete!"

  echo "What you just saw:"
  echo "  1. Existing project with PRD, design, tests, and working code"
  echo "  2. New requirement: 'Add Kelvin support'"
  echo "  3. helix evolve updated PRD, feature spec, and tech design"
  echo "  4. Tracker issues created for the new work"
  echo "  5. TDD cycle: failing tests → implementation → green"
  echo ""
  echo "helix evolve threads a requirement through every layer of"
  echo "the artifact stack so nothing falls out of sync."
  echo ""

  echo "Tracker:"
  run ddx bead status
  run ddx bead list

  # Save recorded responses and fixtures back to the demo source directory
  [[ "${DEMO_RECORD:-0}" == "1" ]] && save_recording || true
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  if [[ -d /recordings && "${HELIX_DEMO_RECORDING:-0}" != "1" ]]; then
    echo "Recording to $RECORDING_FILE"
    HELIX_DEMO_RECORDING=1 asciinema rec \
      -c "bash /usr/local/bin/demo.sh" \
      --title "HELIX Evolve: Threading Requirements Through the Stack" \
      --cols 100 --rows 30 \
      "$RECORDING_FILE"
    echo "Recording saved: $RECORDING_FILE"
  else
    demo_body
  fi
fi
