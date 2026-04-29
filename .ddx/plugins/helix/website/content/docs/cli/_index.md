---
title: CLI Reference
weight: 3
prev: /docs/workflow
next: /docs/skills
---

The `helix` CLI is a shell orchestrator that sits on top of DDx primitives.
It delegates work-item storage to `ddx bead` and agent dispatch to
`ddx agent run`, adding supervisory intelligence on top.

## Commands

### Execution

| Command | Purpose |
|---------|---------|
| `helix run` | Supervisory autopilot — select and execute the next best action |
| `helix build [issue]` | One bounded implementation pass |
| `helix check [scope]` | Decide what action should happen next |
| `helix status` | Structured lifecycle snapshot |

### Steering

| Command | Purpose |
|---------|---------|
| `helix frame [scope]` | Create vision, PRD, feature specs |
| `helix design [scope]` | Create or extend design documents |
| `helix evolve "requirement"` | Thread a change through the artifact stack |
| `helix review [scope]` | Fresh-eyes review of recent work |
| `helix align [scope]` | Top-down reconciliation audit |
| `helix polish [scope]` | Iterative issue refinement |
| `helix triage "Title"` | Create well-structured tracker issues |
| `helix experiment [issue]` | Metric-driven optimization |
| `helix next` | Show recommended next issue |
| `helix commit [issue-id]` | Commit with build gate and tracker update |

### Aliases

- `helix implement` is an alias for `helix build`
- `helix plan` is an alias for `helix design`

## helix run

The primary command. Runs the supervisory autopilot loop.

```bash
helix run [options]
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `--agent codex\|claude` | `codex` | Which agent to dispatch |
| `--smart-model MODEL` | auto | Model for reasoning stages |
| `--cheap-model MODEL` | auto | Model for mechanical stages |
| `--max-cycles N` | unlimited | Stop after N completed builds |
| `--review-every N` | 15 | Periodic alignment interval |
| `--review-threshold N` | 100 | Skip review below N changed lines |
| `--review-agent AGENT` | auto | Agent for cross-model review |
| `--no-auto-align` | | Skip periodic alignment |
| `--no-auto-review` | | Skip post-build reviews |
| `--summary, -s` | | Concise output with log pointers |
| `--quiet, -q` | | Suppress progress output |
| `--dry-run` | | Print agent commands, don't execute |

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HELIX_AGENT` | `codex` | Default primary agent |
| `HELIX_ALT_AGENT` | auto-detected | Alternate agent for adversarial rotation |
| `HELIX_AGENT_TIMEOUT` | 2700 | Per-agent-call timeout (seconds) |
| `HELIX_SMART_MODEL` | opus / gpt-5.4 | Model for reasoning stages |
| `HELIX_CHEAP_MODEL` | sonnet / gpt-5.4-mini | Model for mechanical stages |
| `HELIX_LIBRARY_ROOT` | auto | Override workflows/ location |
| `HELIX_BACKOFF_SLEEP` | formula | Override backoff delay |
| `HELIX_ORPHAN_THRESHOLD` | 7200 | Stale claim threshold (seconds) |

### NEXT_ACTION Contract

When the ready queue drains, `helix check` returns one of:

| Action | Behavior |
|--------|----------|
| `BUILD` | Re-enter ready-work selection |
| `DESIGN` | Run one bounded design pass, then re-check |
| `POLISH` | Run one issue-refinement pass, then re-check |
| `ALIGN` | Run alignment once if auto-align enabled |
| `BACKFILL` | Stop and surface the backfill command |
| `WAIT` | Stop — blocked by external dependency |
| `GUIDANCE` | Stop — human decision required |
| `STOP` | Stop — no actionable work remains |

## helix status

Reports the current run-controller state:

```bash
helix status
```

Shows: current claimed issue, focused epic, completed cycles, token usage,
blocked work, and stop reason.

## helix commit

Commit with HELIX conventions:

```bash
helix commit [issue-id]
```

1. Auto-stages modified files if nothing staged
2. Runs the build gate (lefthook, cargo check, npm test)
3. Commits with the issue title as summary
4. Pushes with rebase
5. Closes the tracker issue
