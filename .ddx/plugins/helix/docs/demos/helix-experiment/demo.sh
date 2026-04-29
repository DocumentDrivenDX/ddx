#!/usr/bin/env bash
# HELIX Experiment Demo — scripted asciinema recording
#
# Demonstrates metric-driven optimization with helix experiment:
#   1. Setup: create a project with a slow string-processing function
#   2. Baseline: measure current performance
#   3. Iterate: agent hypothesizes, edits, tests, benchmarks, keeps/discards
#   4. Close: squash-merge kept changes, report improvement
#
# Uses `ddx agent run` as the agent harness.
#
set -euo pipefail

RECORDING_FILE="/recordings/helix-experiment-$(date +%Y%m%d-%H%M%S).cast"
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
  if [[ -d "$HELIX_ROOT/docs/demos/helix-experiment" ]]; then
    echo "$HELIX_ROOT/docs/demos/helix-experiment"
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
# Usage: apply_fixtures src tests
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
  for d in src tests; do
    [[ -d "$d" ]] && cp -rf "$d" "$src/fixtures/"
  done
  [[ -f package.json ]] && cp -f package.json "$src/fixtures/"
  [[ -f benchmark.js ]] && cp -f benchmark.js "$src/fixtures/"
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

# ── Demo body ─────────────────────────────────────────────────

demo_body() {
  # ── ACT 1: Setup — A Slow Function ─────────────────────────
  narrate "ACT 1: Setup — A Project with a Slow Function"

  echo "Installing HELIX..."
  export HELIX_LIBRARY_ROOT="$HELIX_ROOT/workflows"
  export PATH="$HELIX_ROOT/scripts:$HELIX_ROOT/bin:$PATH"
  mkdir -p ~/.ddx/plugins && ln -sfn "$HELIX_ROOT" ~/.ddx/plugins/helix && bash "$HELIX_ROOT/scripts/helix" doctor --fix 2>&1
  mkdir -p ~/.agents/skills ~/.claude/skills && for s in "$HELIX_ROOT"/.agents/skills/helix-*; do ln -sfn "$s" ~/.agents/skills/$(basename "$s"); ln -sfn "$s" ~/.claude/skills/$(basename "$s"); done
  echo ""

  run git init perf-lab
  cd perf-lab
  run ddx bead init
  seed_dictionary

  mkdir -p .agents .claude src tests
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

Performance lab — a string processing module being optimized.

## Quick Reference

```bash
npm test                    # Run tests
node benchmark.js           # Run benchmark (outputs METRIC runtime=<ms>)
```
AGENTS

  cat > package.json <<'PKG'
{"name":"perf-lab","version":"1.0.0","scripts":{"test":"node --test"}}
PKG

  # Create a deliberately slow string processor
  cat > src/process.js <<'CODE'
// Deliberately slow: rebuilds the string character by character
function processStrings(items) {
  const results = [];
  for (let i = 0; i < items.length; i++) {
    let result = '';
    const item = items[i];
    for (let j = 0; j < item.length; j++) {
      result = result + item[j].toUpperCase();
    }
    // Unnecessary duplicate check using indexOf on the results array
    if (results.indexOf(result) === -1) {
      results.push(result);
    }
  }
  return results;
}

module.exports = { processStrings };
CODE

  # Tests that verify correctness
  cat > tests/process.test.js <<'TEST'
const { describe, it } = require('node:test');
const assert = require('node:assert');
const { processStrings } = require('../src/process.js');

describe('processStrings', () => {
  it('uppercases all items', () => {
    const result = processStrings(['hello', 'world']);
    assert.deepStrictEqual(result, ['HELLO', 'WORLD']);
  });

  it('deduplicates results', () => {
    const result = processStrings(['hello', 'hello', 'world']);
    assert.deepStrictEqual(result, ['HELLO', 'WORLD']);
  });

  it('handles empty input', () => {
    const result = processStrings([]);
    assert.deepStrictEqual(result, []);
  });

  it('handles single character strings', () => {
    const result = processStrings(['a', 'b', 'a']);
    assert.deepStrictEqual(result, ['A', 'B']);
  });
});
TEST

  # Benchmark script
  cat > benchmark.js <<'BENCH'
const { processStrings } = require('./src/process.js');

// Generate test data: 5000 strings of 100 chars each, ~10% duplicates
const data = [];
const chars = 'abcdefghijklmnopqrstuvwxyz';
for (let i = 0; i < 5000; i++) {
  let s = '';
  const len = 50 + Math.floor(Math.random() * 50);
  for (let j = 0; j < len; j++) {
    s += chars[Math.floor(Math.random() * chars.length)];
  }
  // ~10% duplicates
  if (i > 0 && Math.random() < 0.1) {
    data.push(data[Math.floor(Math.random() * i)]);
  } else {
    data.push(s);
  }
}

const start = performance.now();
const result = processStrings(data);
const elapsed = performance.now() - start;

console.log(`Processed ${data.length} items → ${result.length} unique results`);
console.log(`METRIC runtime=${elapsed.toFixed(1)}`);
BENCH

  echo "Verify tests pass:"
  run npm test
  echo ""

  echo "Baseline benchmark:"
  run node benchmark.js

  git add -A && git commit -m "init: perf-lab with slow string processor" --quiet
  sleep 2

  # Create a tracker issue for the experiment
  ddx bead create "Optimize processStrings runtime" \
    --type task --priority 1 \
    --labels helix,phase:iterate \
    --acceptance "runtime metric improves by at least 30% while all tests still pass" \
    --description "Use helix experiment to optimize src/process.js. Primary metric: runtime (ms, lower is better). Files in scope: src/process.js only. Off-limits: tests/, benchmark.js." 2>/dev/null
  echo ""

  # ── ACT 2: Experiment Iteration 1 ──────────────────────────
  narrate "ACT 2: Experiment — Iteration 1"

  echo "The agent will hypothesize a change, implement it, test, benchmark,"
  echo "and decide whether to keep or discard."
  echo ""

  agent_run <<'EXPERIMENT_PROMPT'
You are running one experiment iteration to optimize src/process.js.

Goal: reduce the runtime metric from benchmark.js (lower is better).
Constraint: all tests in tests/process.test.js must still pass.
Files in scope: src/process.js ONLY.

1. Read src/process.js and identify the performance bottleneck.
2. Hypothesize ONE specific change that will improve performance.
3. Make the change.
4. Run: npm test (must pass — if not, revert and try something else).
5. Run: node benchmark.js (capture the METRIC line).
6. Report: what you changed, the new metric value, and whether to KEEP or DISCARD.

Common optimizations to consider:
- String concatenation in a loop (use array join instead)
- indexOf on an array for dedup (use a Set instead)
- Unnecessary intermediate allocations

Make exactly ONE change per iteration.
EXPERIMENT_PROMPT
  apply_fixtures src

  echo "After iteration 1:"
  run npm test
  run node benchmark.js
  git add -A && git commit -m "experiment: iteration 1" --quiet 2>/dev/null || true
  sleep 2

  # ── ACT 3: Experiment Iteration 2 ──────────────────────────
  narrate "ACT 3: Experiment — Iteration 2"

  agent_run <<'EXPERIMENT2_PROMPT'
Continue optimizing src/process.js. This is iteration 2.

Read the current src/process.js (it may have been improved in iteration 1).
Identify the NEXT performance bottleneck.

1. Hypothesize ONE additional change.
2. Implement it.
3. Run: npm test (must pass).
4. Run: node benchmark.js (capture METRIC).
5. Report: what you changed, new metric, KEEP or DISCARD.

Remember: only modify src/process.js, and make ONE change.
EXPERIMENT2_PROMPT
  apply_fixtures src

  echo "After iteration 2:"
  run npm test
  run node benchmark.js
  git add -A && git commit -m "experiment: iteration 2" --quiet 2>/dev/null || true
  sleep 2

  # ── ACT 4: Results ──────────────────────────────────────────
  narrate "ACT 4: Results"

  echo "Final state:"
  show_file src/process.js

  echo "Final benchmark:"
  run node benchmark.js

  echo "Tests still pass:"
  run npm test

  echo ""
  echo "Tracker:"
  run ddx bead list

  # ── Summary ─────────────────────────────────────────────────
  narrate "Demo Complete!"

  echo "What you just saw:"
  echo "  1. A deliberately slow function with correctness tests"
  echo "  2. Baseline measurement establishing the starting metric"
  echo "  3. Two experiment iterations: hypothesize → edit → test → benchmark → keep/discard"
  echo "  4. Final result: faster code with all tests still passing"
  echo ""
  echo "helix experiment enforces: tests MUST pass. If they don't,"
  echo "the change is discarded regardless of metric improvement."
  echo "Correctness is non-negotiable."
  echo ""

  # Save recorded responses and fixtures back to the demo source directory
  [[ "${DEMO_RECORD:-0}" == "1" ]] && save_recording || true
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  if [[ -d /recordings && "${HELIX_DEMO_RECORDING:-0}" != "1" ]]; then
    echo "Recording to $RECORDING_FILE"
    HELIX_DEMO_RECORDING=1 asciinema rec \
      -c "bash /usr/local/bin/demo.sh" \
      --title "HELIX Experiment: Metric-Driven Optimization" \
      --cols 100 --rows 30 \
      "$RECORDING_FILE"
    echo "Recording saved: $RECORDING_FILE"
  else
    demo_body
  fi
fi
