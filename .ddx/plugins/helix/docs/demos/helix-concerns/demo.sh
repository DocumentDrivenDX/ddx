#!/usr/bin/env bash
# HELIX Concerns Demo — scripted asciinema recording
#
# Demonstrates how HELIX concerns prevent technology drift:
#   1. Setup: create a Bun/TypeScript project with concerns declared
#   2. Frame: agent sees concerns and uses Bun-native tooling
#   3. Build: agent implements using bun:test, Biome, Bun.serve()
#   4. Drift: introduce a deliberate Node.js drift (vitest import)
#   5. Review: agent catches the drift via concern-aware review
#
# Uses `ddx agent run` as the agent harness.
#
set -euo pipefail

RECORDING_FILE="/recordings/helix-concerns-$(date +%Y%m%d-%H%M%S).cast"
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

# Ensure ddx is available
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
  if [[ -d "$HELIX_ROOT/docs/demos/helix-concerns" ]]; then
    echo "$HELIX_ROOT/docs/demos/helix-concerns"
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
# Usage: apply_fixtures docs tests src
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
  for d in docs src tests; do
    [[ -d "$d" ]] && cp -rf "$d" "$src/fixtures/"
  done
  # drift.test.ts is created by the script, not by the agent — exclude it
  rm -f "$src/fixtures/tests/drift.test.ts" 2>/dev/null || true
  [[ -f package.json ]] && cp -f package.json "$src/fixtures/"
  [[ -f biome.json ]] && cp -f biome.json "$src/fixtures/"
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
  # ── ACT 1: Setup with Concerns ─────────────────────────────
  narrate "ACT 1: Create a Bun Project with Concerns"

  echo "Installing HELIX..."
  export HELIX_LIBRARY_ROOT="$HELIX_ROOT/workflows"
  export PATH="$HELIX_ROOT/scripts:$HELIX_ROOT/bin:$PATH"
  mkdir -p ~/.ddx/plugins && ln -sfn "$HELIX_ROOT" ~/.ddx/plugins/helix && bash "$HELIX_ROOT/scripts/helix" doctor --fix 2>&1
  mkdir -p ~/.agents/skills ~/.claude/skills && for s in "$HELIX_ROOT"/.agents/skills/helix-*; do ln -sfn "$s" ~/.agents/skills/$(basename "$s"); ln -sfn "$s" ~/.claude/skills/$(basename "$s"); done
  echo ""

  run git init hello-bun
  cd hello-bun
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

This is hello-bun — a Bun HTTP server.

## Quick Reference

```bash
bun test              # Run tests
bun run src/index.ts  # Start the server
```
AGENTS

  # Create the concerns file — this is the key artifact
  mkdir -p docs/helix/01-frame
  cat > docs/helix/01-frame/concerns.md <<'CONCERNS'
# Project Concerns

## Active Concerns
- typescript-bun (tech-stack)

## Area Labels

| Label | Applies to |
|-------|-----------|
| `all` | Every bead |
| `api` | HTTP server, endpoints |

## Project Overrides

### typescript-bun
- **HTTP framework**: raw `Bun.serve()` — no Express, no Fastify
- **Test framework**: `bun:test` — do not use Vitest or Jest
- **Linter/formatter**: Biome — do not use ESLint or Prettier
CONCERNS

  echo ""
  echo "The concerns file declares: use Bun, Biome, bun:test."
  echo "NOT Node.js, NOT ESLint, NOT Vitest."
  echo ""
  show_file docs/helix/01-frame/concerns.md

  git add -A && git commit -m "init: hello-bun with typescript-bun concern" --quiet
  sleep 2

  # ── ACT 2: Frame with Concern Context ──────────────────────
  narrate "ACT 2: Frame — Agent Sees Concerns"

  agent_run <<'FRAME_PROMPT'
Read docs/helix/01-frame/concerns.md first. Then create Frame-phase artifacts:

1. Write docs/helix/01-frame/prd.md:
   - A simple HTTP server that returns JSON health checks
   - GET /health returns {"status":"ok","uptime":<seconds>}
   - Must use the tools declared in concerns.md
   - ~30 lines

2. Create a tracker issue:
   ddx bead create "Implement /health endpoint per PRD" \
     --type task --priority 1 --labels helix,phase:build,area:api \
     --spec-id prd --acceptance "GET /health returns JSON with status and uptime, using Bun.serve and bun:test"
FRAME_PROMPT
  apply_fixtures docs

  require_file docs/helix/01-frame/prd.md "PRD"
  show_file docs/helix/01-frame/prd.md

  echo "Tracker:"
  run ddx bead list
  git add -A && git commit -m "frame: PRD with concern-aware requirements" --quiet
  sleep 2

  # ── ACT 3: Build with Correct Tools ────────────────────────
  narrate "ACT 3: Build — Agent Uses Bun-Native Tools"

  agent_run <<'BUILD_PROMPT'
Read docs/helix/01-frame/concerns.md first — it declares typescript-bun.
Then implement the /health endpoint:

1. Create package.json for Bun (NOT npm). Use bun:test for testing.
2. Create biome.json for linting (NOT ESLint, NOT Prettier).
3. Create src/index.ts using Bun.serve() (NOT Express, NOT Node http).
4. Create tests/health.test.ts using bun:test (NOT Vitest, NOT Jest).
5. Run bun test to verify.
6. Claim and close the tracker issue.

Follow the concern constraints exactly.
BUILD_PROMPT
  apply_fixtures src tests package.json biome.json

  echo "Checking what the agent built..."
  echo ""

  if [[ -f package.json ]]; then
    echo "package.json:"
    cat package.json
    echo ""
  fi

  if [[ -f biome.json ]]; then
    echo "  ✓ biome.json exists (Biome, not ESLint)"
  fi

  if [[ -f src/index.ts ]]; then
    echo "  ✓ src/index.ts exists"
    if grep -q "Bun.serve" src/index.ts; then
      echo "  ✓ Uses Bun.serve() (not Express)"
    fi
  fi

  if [[ -f tests/health.test.ts ]]; then
    echo "  ✓ tests/health.test.ts exists"
    if grep -q "bun:test" tests/health.test.ts; then
      echo "  ✓ Uses bun:test (not Vitest)"
    fi
  fi

  echo ""
  git add -A && git commit -m "build: /health endpoint with Bun-native tooling" --quiet
  sleep 2

  # ── ACT 4: Introduce Drift ─────────────────────────────────
  narrate "ACT 4: Drift — Someone Introduces Node.js Tooling"

  echo "A developer adds a test using Vitest instead of bun:test..."
  echo ""

  mkdir -p tests
  cat > tests/drift.test.ts <<'DRIFT'
// Accidentally using Vitest instead of bun:test
import { describe, it, expect } from 'vitest'

describe('drift example', () => {
  it('should use vitest', () => {
    expect(1 + 1).toBe(2)
  })
})
DRIFT

  show_file tests/drift.test.ts

  echo "This violates the typescript-bun concern:"
  echo "  ✗ imports from 'vitest' — should be 'bun:test'"
  echo ""
  git add -A && git commit -m "test: add vitest test (drift!)" --quiet
  sleep 2

  # ── ACT 5: Review Catches Drift ────────────────────────────
  narrate "ACT 5: Review — Agent Detects Concern Drift"

  agent_run <<'REVIEW_PROMPT'
Read docs/helix/01-frame/concerns.md first — it declares typescript-bun.

Review the last commit (git diff HEAD~1) for concern drift.
The typescript-bun concern says:
- Use bun:test, NOT Vitest or Jest
- Use Biome, NOT ESLint or Prettier
- Use Bun.serve(), NOT Express or Node http

Check every file in the diff. Report any imports, tools, or patterns
that violate the declared concerns. Be specific about which file and
line has the drift.

Create a tracker issue for each drift finding:
ddx bead create "drift: <description>" --type task --labels helix,phase:build,review-finding,area:testing
REVIEW_PROMPT

  echo ""
  echo "The agent should have flagged tests/drift.test.ts"
  echo "for importing from 'vitest' instead of 'bun:test'."
  echo ""

  echo "Tracker (should include drift finding):"
  run ddx bead list
  sleep 2

  # ── Summary ─────────────────────────────────────────────────
  narrate "Demo Complete!"

  echo "What you just saw:"
  echo "  1. Concerns declared: typescript-bun (Bun, Biome, bun:test)"
  echo "  2. Agent reads concerns and uses the correct tools"
  echo "  3. Deliberate drift introduced (vitest import)"
  echo "  4. Review catches the drift via concern-aware review"
  echo ""
  echo "Concerns prevent the technology drift that happens when"
  echo "agents (or developers) reach for familiar tools instead"
  echo "of the project's declared stack."
  echo ""

  # Save recorded responses and fixtures back to the demo source directory
  [[ "${DEMO_RECORD:-0}" == "1" ]] && save_recording || true
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  if [[ -d /recordings && "${HELIX_DEMO_RECORDING:-0}" != "1" ]]; then
    echo "Recording to $RECORDING_FILE"
    HELIX_DEMO_RECORDING=1 asciinema rec \
      -c "bash /usr/local/bin/demo.sh" \
      --title "HELIX Concerns: Preventing Technology Drift" \
      --cols 100 --rows 30 \
      "$RECORDING_FILE"
    echo "Recording saved: $RECORDING_FILE"
  else
    demo_body
  fi
fi
