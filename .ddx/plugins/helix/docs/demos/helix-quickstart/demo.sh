#!/usr/bin/env bash
# HELIX Quickstart Demo — scripted asciinema recording
#
# Demonstrates the full HELIX onboarding experience:
#   1. Install: install HELIX as a plugin
#   2. Setup: initialize a new project with DDx tracker
#   3. Frame: create product vision, PRD, and feature spec
#   4. Design: create technical design and tracker issues
#   5. Build: TDD cycle — write failing tests (Red), implement (Green)
#   6. Verify: run tests, check acceptance criteria
#   7. Review: fresh-eyes review of the work
#
# Uses `ddx agent run` as the agent harness.
#
# Usage:
#   docker build -t helix-demo docs/demos/helix-quickstart/
#   docker run --rm \
#     -v ~/.claude.json:/root/.claude.json:ro \
#     -v ~/.claude:/root/.claude \
#     -v $(pwd):/helix:ro \
#     -v $(pwd)/docs/demos/helix-quickstart/recordings:/recordings \
#     helix-demo
#
set -euo pipefail

RECORDING_FILE="/recordings/helix-quickstart-$(date +%Y%m%d-%H%M%S).cast"
MAX_RETRIES=3
COOLDOWN=3
# Simulated agent thinking time for virtual replay recordings.
# Real Claude calls take 15-30s each; this adds pacing so the screencast
# is watchable. Set to 0 for fast validation runs.
AGENT_PACE="${DEMO_PACE:-5}"

# Auto-detect helix repo: Docker mount at /helix, or relative to this script
if [[ -d /helix/workflows ]]; then
  HELIX_ROOT="/helix"
elif [[ -d "$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/workflows" ]]; then
  HELIX_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
else
  echo "FAIL: cannot find helix repo — set HELIX_ROOT or mount at /helix" >&2
  exit 1
fi

# Ensure ddx is available
if ! command -v ddx >/dev/null 2>&1; then
  if [[ -x /ddx/ddx ]]; then
    export PATH="/ddx:$PATH"
  elif [[ -x "$HELIX_ROOT/../ddx/ddx" ]]; then
    export PATH="$HELIX_ROOT/../ddx:$PATH"
  else
    echo "FAIL: ddx not found — mount ddx binary or install it" >&2
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
  local lines="${2:-20}"
  echo "── $file ──"
  head -n "$lines" "$file" 2>/dev/null || echo "(file not found)"
  echo "..."
  echo ""
  sleep 2
}

# Resolve the demo source directory (handles both local and Docker paths).
demo_source_dir() {
  if [[ -d "$HELIX_ROOT/docs/demos/helix-quickstart" ]]; then
    echo "$HELIX_ROOT/docs/demos/helix-quickstart"
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
# Usage: apply_fixtures docs tests bin/convert.js
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

# Run ddx agent with prompt visible and retries
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
    # Extract the result text from ddx agent JSON output
    local result
    result=$(printf '%s' "$output" | ddx jq -r '.result // empty' 2>/dev/null) || true
    if [[ -n "$result" ]]; then
      printf '%s\n' "$result"
    else
      printf '%s\n' "$output"
    fi
  fi

  # Pace the recording — real agent calls take 15-30s, so add a pause
  # when using the virtual agent so the screencast is watchable.
  sleep "$AGENT_PACE"

  echo ""
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

assert_output() {
  local actual="$1"
  local expected="$2"
  local label="$3"
  if [[ "$actual" == *"$expected"* ]]; then
    echo "  ✓ $label — got $expected"
  else
    echo "FAIL: $label — expected '$expected', got '$actual'"
    exit 1
  fi
}

# ── Demo body ─────────────────────────────────────────────────

demo_body() {
  # ── ACT 1: Install HELIX ──────────────────────────────────
  narrate "ACT 1: Install HELIX"

  echo "HELIX repo: $HELIX_ROOT"
  echo ""

  echo "Installing HELIX skills..."
  export HELIX_LIBRARY_ROOT="$HELIX_ROOT/workflows"
  export PATH="$HELIX_ROOT/scripts:$HELIX_ROOT/bin:$PATH"
  mkdir -p ~/.ddx/plugins && ln -sfn "$HELIX_ROOT" ~/.ddx/plugins/helix && bash "$HELIX_ROOT/scripts/helix" doctor --fix 2>&1
  mkdir -p ~/.agents/skills ~/.claude/skills && for s in "$HELIX_ROOT"/.agents/skills/helix-*; do ln -sfn "$s" ~/.agents/skills/$(basename "$s"); ln -sfn "$s" ~/.claude/skills/$(basename "$s"); done
  echo ""

  echo "Available skills:"
  ls ~/.agents/skills/ 2>/dev/null | tr '\n' ' '
  echo ""
  echo ""

  echo "HELIX CLI:"
  run helix help
  sleep 2

  echo "DDx agent harness:"
  run ddx agent list
  sleep 2

  # ── ACT 2: Start a new project ─────────────────────────────
  narrate "ACT 2: Start a New Project"

  run git init hello-helix
  cd hello-helix
  run ddx bead init
  seed_dictionary

  # Set up project for agent access
  mkdir -p .agents .claude
  cp -rf ~/.agents/skills .agents/
  cat > .claude/settings.json <<'SETTINGS'
{
  "permissions": {
    "allow": ["Bash(*)", "Read(*)", "Write(*)", "Edit(*)"]
  }
}
SETTINGS

  # Create AGENTS.md
  cat > AGENTS.md <<'AGENTS'
# Agent Instructions

This is the hello-helix project — a Node.js CLI temperature converter.

## Quick Reference

```bash
npm test              # Run tests
node bin/convert.js   # Run the CLI
```

## Project Structure

- `bin/convert.js` — CLI entry point
- `tests/` — test files
- `docs/helix/` — HELIX artifacts

## Workflow

This project uses HELIX. Run `helix --help` for commands.
AGENTS
  echo "Created AGENTS.md"
  git add -A && git commit -m "init: hello-helix project" --quiet
  echo ""
  sleep 2

  # ── ACT 3: Frame — Vision and PRD ──────────────────────────
  narrate "ACT 3: Frame — Define What to Build"

  agent_run <<'FRAME_PROMPT'
Create the Frame-phase artifacts for "hello-helix", a Node.js CLI temperature converter.

1. Write docs/helix/00-discover/product-vision.md:
   - Mission: simple, reliable temperature conversion from the command line
   - Target: developers who need quick conversions in scripts and terminals
   - Keep it to ~30 lines

2. Write docs/helix/01-frame/prd.md:
   - Features: convert --to-celsius <temp> and convert --to-fahrenheit <temp>
   - Output with one decimal place
   - P0: basic conversion. P1: error messages for bad input. P2: batch mode.
   - Keep it concise — ~50 lines

3. Write docs/helix/01-frame/features/FEAT-001-temperature-conversion.md:
   - User stories with acceptance criteria:
     - convert --to-celsius 212 prints 100.0
     - convert --to-fahrenheit 0 prints 32.0
     - convert --to-celsius 98.6 prints 37.0
   - ~40 lines

Create the directory structure as needed. Use markdown.
FRAME_PROMPT
  apply_fixtures docs

  require_file docs/helix/00-discover/product-vision.md "vision"
  require_file docs/helix/01-frame/prd.md "PRD"
  show_file docs/helix/00-discover/product-vision.md 15
  show_file docs/helix/01-frame/prd.md 20

  git add -A && git commit -m "frame: vision, PRD, and feature spec" --quiet
  sleep 2

  # ── ACT 4: Design — Technical Design ───────────────────────
  narrate "ACT 4: Design — How to Build It"

  agent_run <<'DESIGN_PROMPT'
Create the Design-phase artifact for hello-helix.

Write docs/helix/02-design/technical-designs/TD-001-temperature-conversion.md:
- Architecture: single bin/convert.js file
- Exports: toFahrenheit(c) and toCelsius(f) functions
- CLI: parse process.argv for --to-celsius and --to-fahrenheit flags
- Output: toFixed(1) for one decimal place
- Error handling: print usage on bad input, exit 1
- ~40 lines

Then create tracker issues for the build phase:

ddx bead create "Write failing tests for temperature conversion" \
  --type task --priority 1 --labels helix,phase:build \
  --spec-id FEAT-001 --acceptance "tests exist and FAIL (Red phase)"

ddx bead create "Implement bin/convert.js per TD-001" \
  --type task --priority 1 --labels helix,phase:build \
  --spec-id TD-001 --acceptance "npm test passes, CLI outputs correct values"
DESIGN_PROMPT
  apply_fixtures docs/helix/02-design

  require_file docs/helix/02-design/technical-designs/TD-001-temperature-conversion.md "tech design"
  show_file docs/helix/02-design/technical-designs/TD-001-temperature-conversion.md

  echo "Work queue:"
  run ddx bead list
  git add -A && git commit -m "design: TD-001 and build issues" --quiet
  sleep 2

  # ── ACT 5: Build — Red then Green ──────────────────────────
  narrate "ACT 5: Build — Tests First, Then Code"

  echo "▶ Writing failing tests (Red phase)..."
  agent_run <<'RED_PROMPT'
Write failing tests for hello-helix per FEAT-001 and TD-001.

1. Create package.json with {"name":"hello-helix","version":"0.1.0","scripts":{"test":"node --test"}}
2. Create tests/convert.test.js requiring ../bin/convert.js
3. Test: toFahrenheit(0) === 32
4. Test: toCelsius(212) === 100
5. Test: toCelsius(98.6) is approximately 37.0
6. Do NOT create bin/convert.js — tests must FAIL

Then claim and close the test issue:
ddx bead list --json | ddx jq -r '.[] | select(.title | contains("failing tests")) | .id' | head -1
Use that ID to: ddx bead update <id> --claim && ddx bead close <id>
RED_PROMPT
  apply_fixtures tests package.json

  require_file tests/convert.test.js "tests"
  [[ -f package.json ]] || echo '{"name":"hello-helix","version":"0.1.0","scripts":{"test":"node --test"}}' > package.json
  show_file tests/convert.test.js 20

  echo "Red phase — tests should fail:"
  npm test 2>&1 || true
  echo ""
  git add -A && git commit -m "test: failing tests for FEAT-001 (Red)" --quiet
  sleep 2

  echo ""
  echo "▶ Implementing to pass tests (Green phase)..."
  agent_run <<'GREEN_PROMPT'
Implement bin/convert.js per TD-001 to make all tests pass.

1. Export toFahrenheit(c) and toCelsius(f)
2. CLI: parse --to-celsius and --to-fahrenheit from process.argv
3. Output with toFixed(1)
4. Run npm test — all tests must pass

Then claim and close the implementation issue:
ddx bead list --json | ddx jq -r '.[] | select(.title | contains("Implement")) | .id' | head -1
Use that ID to: ddx bead update <id> --claim && ddx bead close <id>
GREEN_PROMPT
  apply_fixtures bin

  require_file bin/convert.js "implementation"
  show_file bin/convert.js 20

  # ── ACT 6: Verify ──────────────────────────────────────────
  narrate "ACT 6: Verify"

  echo "Tests:"
  npm test 2>&1 || { echo "FAIL: npm test failed — aborting"; exit 1; }
  echo "  ✓ all tests pass"
  echo ""

  echo "Acceptance criteria:"
  local out
  out=$(node bin/convert.js --to-celsius 212)
  assert_output "$out" "100.0" "212°F → Celsius"

  out=$(node bin/convert.js --to-fahrenheit 0)
  assert_output "$out" "32.0" "0°C → Fahrenheit"

  out=$(node bin/convert.js --to-celsius 98.6)
  assert_output "$out" "37.0" "98.6°F → Celsius"

  echo ""
  git add -A && git commit -m "build: implement temperature conversion (Green)" --quiet
  sleep 2

  # ── ACT 7: Review ──────────────────────────────────────────
  narrate "ACT 7: Review"

  agent_run "Review all artifacts and code in this project for errors, omissions, and quality issues. Check docs/helix/ specs against tests and implementation. Are acceptance criteria covered? What's missing? Be concise — bullet points."
  sleep 2

  # ── Summary ─────────────────────────────────────────────────
  narrate "Demo Complete!"

  echo "Tracker status:"
  run ddx bead status
  echo ""
  run ddx bead list

  echo ""
  echo "What you just saw:"
  echo "  1. Install   — HELIX skills and CLI set up"
  echo "  2. Setup     — git repo, tracker, AGENTS.md"
  echo "  3. Frame     — vision, PRD, feature spec created by agent"
  echo "  4. Design    — technical design and tracker issues"
  echo "  5. Build     — TDD: failing tests (Red) then implementation (Green)"
  echo "  6. Verify    — tests pass, acceptance criteria checked"
  echo "  7. Review    — agent reviews its own work"
  echo ""
  echo "The HELIX lifecycle: Frame → Design → Test → Build → Review"
  echo "The tracker drives. Artifacts govern. Agents execute."
  echo ""

  # Save recorded responses and fixtures back to the demo source directory
  [[ "${DEMO_RECORD:-0}" == "1" ]] && save_recording
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  if [[ -d /recordings && "${HELIX_DEMO_RECORDING:-0}" != "1" ]]; then
    echo "Recording to $RECORDING_FILE"
    HELIX_DEMO_RECORDING=1 asciinema rec \
      -c "bash /usr/local/bin/demo.sh" \
      --title "HELIX Quickstart: Temperature Converter" \
      --cols 100 --rows 30 \
      "$RECORDING_FILE"
    echo "Recording saved: $RECORDING_FILE"
  else
    demo_body
  fi
fi
