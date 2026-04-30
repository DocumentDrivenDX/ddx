# DDx Work: Operator Guide

This is the short operator reference for the `ddx run` / `ddx try` / `ddx work`
layers. For full flags, see `ddx run --help`, `ddx try --help`, and
`ddx work --help`.

## Which command do I run?

- **`ddx work`** — the queue-drain surface. It claims ready beads, invokes
  `ddx try`, records outcomes, and applies DDx-owned retry policy.
- **`ddx try <id>`** — one bead attempt in an isolated worktree. It wraps
  prompt construction, evidence capture, and merge/preserve finalization around
  one or more `ddx run` invocations.
- **`ddx run`** — one agent invocation atom. It calls upstream agent `Execute`
  exactly once with prompt/config, `MinPower`/`MaxPower`, and optional
  passthrough constraints.

DDx owns bead orchestration, evidence, success classification, and retry policy.
The upstream agent owns harness/provider/model routing and execution.

## Power and Passthrough

Use power bounds for normal dispatch:

```bash
ddx run --min-power 10 --prompt task.md
ddx work --top-power
```

`--harness`, `--provider`, and `--model` are passthrough constraints only. DDx
sends them unchanged to the agent and does not validate, fallback, rewrite, or
branch on them:

```bash
ddx run --min-power 10 --provider openrouter --model qwen3.6-27b --prompt task.md
```

If hard pins make the requested power bounds unsatisfiable, DDx records the
agent's typed error and stops with operator action required. It does not remove
pins or call route preflight to choose a substitute.

## Result Statuses

Every `ddx try` attempt reports a status in the attempt record. `ddx work`
uses these statuses plus DDx-owned evidence to decide close, preserve, stop, or
eligible retry.

| Status | Meaning | Work action |
|---|---|---|
| `success` | Agent produced changes and finalization succeeded | Close bead with evidence |
| `already_satisfied` | Acceptance was already met | Close bead with evidence |
| `no_changes` | Agent ran but produced no diff | Leave open; retry only if policy allows |
| `land_conflict` | Merge/finalization failed | Stop as operator-action or cooldown; do not power-retry |
| `post_run_check_failed` | Post-run checks failed after a valid attempt | Retry may raise `MinPower` if policy allows |
| `execution_failed` | Agent or environment errored | Classify before retry; deterministic setup failures do not power-retry |
| `structural_validation_failed` | Bead or prompt inputs invalid | Stop; fix tracker/spec input |

## Common Operations

```bash
# Drain the current ready queue
ddx work

# Process at most one ready bead and stop
ddx work --once

# Run as a long-lived worker
ddx work --poll-interval 30s

# Debug a specific bead
ddx try <bead-id>

# Preserve the result instead of merging it back
ddx try <bead-id> --no-merge

# Run one direct prompt
ddx run --min-power 10 --prompt task.md
```

## Retry Boundary

DDx may raise `MinPower` only when DDx-owned evidence shows a stronger model
could plausibly help after a valid attempt started. It must not power-retry
dirty worktrees, merge conflicts, invalid bead metadata, unresolved
dependencies, config parse errors, missing binaries, auth failures, toolchain
setup failures, or passthrough exhaustion.

`ResolveRoute` and route candidate traces are status/debug-only. Normal
`run`/`try`/`work` execution does not call route preflight and never feeds a
route decision back into `Execute`.
