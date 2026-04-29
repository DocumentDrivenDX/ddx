#!/usr/bin/env bash
# Interactive HELIX demo: shows a Claude Code session with HELIX skills.
#
# Uses tmux + tmux send-keys to automate an interactive Claude session
# while asciinema records the output.
#
# Requirements: tmux, asciinema, claude CLI, ddx, helix
# Credentials: mount ~/.claude.json and ~/.claude/ into the container

set -euo pipefail

# --- Configuration ---
SESSION="helix-demo"
COLS=100
ROWS=30
WORKSPACE="/tmp/helix-demo-project"
RECORDING_DIR="${RECORDING_DIR:-/recordings}"
RECORDING="${RECORDING_DIR}/helix-interactive-$(date +%Y%m%d-%H%M%S).cast"

# --- Helpers ---
narrate() {
  printf '\n\033[1;36m━━━ %s ━━━\033[0m\n\n' "$1"
}

type_prompt() {
  # Send text character-by-character with slight delay for natural feel
  local text="$1"
  local session="${2:-$SESSION}"
  # Send the text (tmux send-keys -l sends literal characters)
  tmux send-keys -t "$session" -l "$text"
  sleep 0.5
  tmux send-keys -t "$session" Enter
}

wait_for_idle() {
  # Wait for Claude to finish responding.
  # We poll the pane content and wait for it to stabilize.
  local session="${1:-$SESSION}"
  local timeout="${2:-120}"
  local poll_interval=3
  local stable_count=0
  local prev_hash=""
  local elapsed=0

  while (( elapsed < timeout )); do
    local current
    current="$(tmux capture-pane -t "$session" -p | md5sum | cut -d' ' -f1)"
    if [[ "$current" == "$prev_hash" ]]; then
      (( stable_count++ ))
      # Consider idle after 3 consecutive identical captures (9 seconds)
      (( stable_count >= 3 )) && return 0
    else
      stable_count=0
      prev_hash="$current"
    fi
    sleep "$poll_interval"
    (( elapsed += poll_interval ))
  done
  echo "WARNING: wait_for_idle timed out after ${timeout}s" >&2
}

# --- Setup ---
narrate "SETUP: Preparing demo workspace"

# Create a fresh project directory
rm -rf "$WORKSPACE"
mkdir -p "$WORKSPACE"
cd "$WORKSPACE"
git init -q
echo '# Demo Project' > README.md
git add . && git commit -q -m "Initial commit"

# Initialize DDx
ddx init 2>/dev/null || true

# Ensure Claude settings allow tool use
mkdir -p .claude
cat > .claude/settings.json << 'SETTINGS'
{
  "permissions": {
    "allow": [
      "Bash(*)",
      "Read(*)",
      "Write(*)",
      "Edit(*)",
      "Glob(*)",
      "Grep(*)"
    ]
  }
}
SETTINGS

# --- Recording ---
narrate "RECORDING: Starting interactive Claude Code session"

# Kill any existing tmux session
tmux kill-session -t "$SESSION" 2>/dev/null || true

# Start tmux session
tmux new-session -d -s "$SESSION" -x "$COLS" -y "$ROWS"
tmux send-keys -t "$SESSION" "cd $WORKSPACE && clear" Enter
sleep 1

# Start asciinema recording — it attaches to the tmux session
if [[ -d "$RECORDING_DIR" ]]; then
  asciinema rec \
    --cols "$COLS" --rows "$ROWS" \
    --command "tmux attach -t $SESSION" \
    "$RECORDING" &
  ASCIINEMA_PID=$!
  sleep 2
else
  # No recording dir — just attach for local testing
  echo "No recording dir at $RECORDING_DIR — running without recording"
  ASCIINEMA_PID=""
fi

# ═══════════════════════════════════════════════════════════════
# ACT 1: Open Claude and frame the project
# ═══════════════════════════════════════════════════════════════

tmux send-keys -t "$SESSION" "claude" Enter
sleep 5  # wait for Claude to initialize

type_prompt "I want to build a Node.js CLI that converts temperatures between Celsius, Fahrenheit, and Kelvin. Use /helix-frame to create the product vision and requirements."
wait_for_idle "$SESSION" 90

# ═══════════════════════════════════════════════════════════════
# ACT 2: Run the autopilot
# ═══════════════════════════════════════════════════════════════

type_prompt "/helix-run"
wait_for_idle "$SESSION" 180

# ═══════════════════════════════════════════════════════════════
# ACT 3: Review what was built
# ═══════════════════════════════════════════════════════════════

type_prompt "Show me the test results. Did everything pass?"
wait_for_idle "$SESSION" 30

# ═══════════════════════════════════════════════════════════════
# ACT 4: Evolve — add a new requirement
# ═══════════════════════════════════════════════════════════════

type_prompt "/helix-evolve \"Add Kelvin support for all conversion functions\""
wait_for_idle "$SESSION" 60

# ═══════════════════════════════════════════════════════════════
# CLOSE
# ═══════════════════════════════════════════════════════════════

sleep 3
tmux send-keys -t "$SESSION" "/exit" Enter
sleep 2

# Clean up
tmux kill-session -t "$SESSION" 2>/dev/null || true
if [[ -n "${ASCIINEMA_PID:-}" ]]; then
  wait "$ASCIINEMA_PID" 2>/dev/null || true
fi

narrate "DONE: Recording saved to $RECORDING"
