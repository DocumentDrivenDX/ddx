#!/usr/bin/env bash
# DDx + HELIX Full Journey Demo — scripted asciinema recording
#
# Demonstrates the complete DDx onboarding and HELIX build lifecycle:
#   1. Setup: install DDx, install HELIX plugin, init project
#   2. Frame: create PRD and feature spec using HELIX
#   3. Design: create technical design, wire tracker beads
#   4. Build: write tests (Red), implement (Green), close beads
#   5. Evolve: add a feature, update specs, implement
#   6. Inspect: review beads, agent usage, doc history
#
# Usage:
#   bash scripts/demos/06-full-journey.sh
#   # or with asciinema:
#   asciinema rec --cols 100 --rows 30 -c "bash scripts/demos/06-full-journey.sh" journey.cast
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_lib.sh"

COOLDOWN=3
MAX_RETRIES=2

narrate() {
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  $1"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""
  sleep 2
}

show_file() {
  local file="$1"
  local lines="${2:-15}"
  echo "── $file ──"
  head -n "$lines" "$file" 2>/dev/null || echo "(file not found)"
  echo "..."
  echo ""
  sleep 2
}

require_file() {
  if [[ ! -f "$1" ]]; then
    echo "FAIL: $1 not found"
    exit 1
  fi
  echo "  ✓ $1 exists"
}

agent_run() {
  local prompt_file="$1"
  local attempt
  echo "$ ddx agent run --harness claude --effort high --prompt $prompt_file"
  echo ""
  for attempt in $(seq 1 "$MAX_RETRIES"); do
    ddx agent run --harness claude --effort high --prompt "$prompt_file" 2>&1 && break
    if [[ $attempt -lt $MAX_RETRIES ]]; then
      echo "  (retrying $attempt/$MAX_RETRIES...)"
      sleep $((attempt * 3))
    fi
  done
  echo ""
  sleep "$COOLDOWN"
}

# ── ACT 1: Setup ────────────────────────────────────────────
narrate "ACT 1: Setup — Install DDx and HELIX"

setup_demo_dir

type_command ddx init
type_command ddx install helix
type_command ddx doctor

echo ""
echo "Available HELIX skills:"
ls ~/.agents/skills/ | grep helix | tr '\n' ' '
echo ""
sleep 2

# ── ACT 2: Frame ────────────────────────────────────────────
narrate "ACT 2: Frame — Define What to Build"

agent_run "$SCRIPT_DIR/prompts/frame-tracker.md"

echo "Artifacts created:"
find docs/ -name "*.md" 2>/dev/null | sort
echo ""

echo "Tracker:"
type_command ddx bead list

git add -A && git commit -q -m "frame: PRD, feature spec, and beads"
sleep 2

# ── ACT 3: Build ────────────────────────────────────────────
narrate "ACT 3: Build — Implement Per Specs"

agent_run "$SCRIPT_DIR/prompts/build-tracker.md"

echo "Source files:"
find . -name "*.go" -not -path "./.ddx/*" | sort
echo ""

echo "Tests:"
go test ./... 2>&1 || true
echo ""

echo "Tracker after build:"
type_command ddx bead list
type_command ddx bead ready

sleep 2

# ── ACT 4: Evolve ───────────────────────────────────────────
narrate "ACT 4: Evolve — Add Task Priorities"

agent_run "$SCRIPT_DIR/prompts/evolve-priorities.md"

echo "Tracker after evolution:"
type_command ddx bead list

echo ""
echo "Tests after evolution:"
go test ./... 2>&1 || true
sleep 2

# ── ACT 5: Inspect ──────────────────────────────────────────
narrate "ACT 5: Inspect — Review the Work"

type_command ddx bead list
type_command ddx agent usage --since today
type_command git log --oneline

# ── Summary ──────────────────────────────────────────────────
narrate "Demo Complete!"

echo "What you just saw:"
echo "  1. Setup: ddx init + ddx install helix"
echo "  2. Frame: HELIX created PRD, feature spec, and tracker beads"
echo "  3. Build: Agent implemented a Go task tracker with TDD"
echo "  4. Evolve: Added priorities — spec updated, code extended"
echo "  5. Inspect: Beads tracked every step, agent usage visible"
echo ""
echo "The DDx + HELIX lifecycle:"
echo "  Documents drive the agents. DDx drives the documents."
echo "  The tracker drives. Artifacts govern. Agents execute."
echo ""

cleanup_demo_dir
