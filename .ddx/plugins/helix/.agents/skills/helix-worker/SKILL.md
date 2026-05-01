---
name: helix-worker
description: 'Launch helix run as a background process and monitor progress via summary output and logs.'
argument-hint: '[--agent codex|claude] [--max-cycles N] [helix run options...]'
---

# Worker

Launch `helix run` as a background process and monitor its progress.

Use this skill when the user wants HELIX work to proceed in the background
while they continue working interactively. The worker starts the CLI process,
watches summary output, and reads log ranges on failure.

## When to Use

- The user wants `helix run` in the background
- Long-running execution (multiple cycles) where inline mode wastes tokens
- The user wants to monitor progress without blocking the conversation
- The user says "run helix", "start working", "grind through the queue"

## When NOT to Use

- The user wants to make live adjustments between cycles (use `helix-run`)
- A single issue needs implementing (use `helix-build`)
- The user wants to check status only (use `helix status`)

## Launch

Start the worker with the Bash tool's `run_in_background` parameter:

```bash
cd <target-repo> && helix run --agent claude --summary --max-cycles 10 2>&1
```

Key flags:
- `--summary` (required) — routes verbose output to log file, emits concise
  progress lines
- `--agent codex|claude` — which agent to dispatch
- `--model <model>` — specific model (e.g., `gpt-5.3-codex-spark`)
- `--max-cycles N` — stop after N completed build passes
- `--review-every N` — run fresh-eyes review every N cycles
- `--no-auto-align` — skip periodic alignment checks
- `--no-auto-review` — skip post-build reviews

Environment overrides:
- `HELIX_AGENT_TIMEOUT=1800` — per-agent-call timeout in seconds (default 2700)
- `HELIX_BACKOFF_SLEEP=1` — override backoff delay (useful for testing)

## Monitor

Summary output looks like:

```
helix: [14:24:01] cycle 1: hx-42 (5 ready)
helix: [14:24:35] codex complete (rc=0, 34s, 892 tokens) — log L12–L340 in .helix-logs/helix-...log
helix: [14:24:36] cycle 1: hx-42 → COMPLETE (1/3 done, 892 tokens)
helix: ═══ run complete: 3 attempted, 2 completed, 1 skipped, 2400 tokens ═══
```

When a cycle fails or blocks, read the log range:

```bash
# Summary says: "log L500–L800 in .helix-logs/helix-20260402-142401.log"
sed -n '500,800p' .helix-logs/helix-20260402-142401.log
```

## After Completion

Check results:

```bash
helix status
ddx bead ready --execution
```

If the worker stopped with blockers, the blocker report is in
`.helix-logs/blockers-*.md`.

## Examples

```bash
# Background worker with claude, 10 cycles
helix run --agent claude --summary --max-cycles 10 2>&1

# Background worker with codex spark model
HELIX_AGENT_TIMEOUT=1800 helix run --agent codex --model gpt-5.3-codex-spark --summary 2>&1

# Check progress
helix status

# Read failure logs
cat .helix-logs/helix-*.log | tail -50
```
