# DDx Agent Execute: Operator Guide

This is the short operator reference for `ddx agent execute-loop` and
`ddx agent execute-bead`. For the full flag list, see `ddx agent execute-loop
--help` and `ddx agent execute-bead --help`.

## Which command do I run?

- **`ddx agent execute-loop`** — the primary surface. Drains the project's
  execution-ready bead queue. Claims the next ready bead, runs
  `execute-bead` on it, records the result, and moves on until no
  unattempted ready work remains. Reach for this by default.
- **`ddx agent execute-bead <id>`** — the primitive. Runs one agent on one
  bead in an isolated worktree. Use it for debugging, replaying, or
  re-running a specific bead. The loop calls this internally for every
  attempt.

Planning and document-only beads are valid execution targets. Any bead
whose dependencies are met and whose acceptance criteria still require
work is eligible — the agent produces whatever artifacts the acceptance
says to produce (docs, specs, code).

## Result statuses

Every execute-bead attempt reports one of these statuses on its `status:`
line (and in the JSON result). The loop's close/unclaim behavior is
determined entirely by this value.

| Status                         | Meaning                                          | Loop action                                             |
|--------------------------------|--------------------------------------------------|---------------------------------------------------------|
| `success`                      | Agent produced changes; merged (or preserved with `--no-merge`) | Close bead with session + commit evidence |
| `already_satisfied`            | Bead returned `no_changes` on repeated attempts  | Close bead with accumulated no-changes evidence         |
| `no_changes`                   | Agent ran but produced no diff                   | Unclaim; may apply cooldown; close after N attempts     |
| `land_conflict`                | Rebase/merge failed; result preserved under `refs/ddx/iterations/<bead-id>/...` | Unclaim; leave bead open                    |
| `post_run_check_failed`        | Post-run checks failed; result preserved         | Unclaim; leave bead open                                |
| `execution_failed`             | Agent or harness error                           | Unclaim; leave bead open                                |
| `structural_validation_failed` | Bead or prompt inputs invalid                    | Unclaim; leave bead open                                |

**Rule of thumb:** only `success` and `already_satisfied` close the bead.
Every other status leaves the bead open and unclaimed so a later attempt
can try again.

## What the loop records per attempt

Each attempt appends one `execute-bead` event to the bead with:

- `status` — one of the values above
- `detail` — human-readable reason
- `base_rev` — git rev the attempt started from
- `result_rev` — git rev of the resulting commit (when present)
- `preserve_ref` — `refs/ddx/iterations/<bead-id>/<ts>-<shortsha>` (when preserved)
- `retry_after` — RFC3339 time for no-progress cooldowns

The underlying agent session log for the attempt is written via the
execute-bead agent-log path (see `ddx agent log`), so session IDs on
closed beads can be replayed.

## Common operations

```bash
# Drain the current ready queue once and exit (normal surface)
ddx agent execute-loop

# Process at most one ready bead and stop
ddx agent execute-loop --once

# Run as a long-lived worker
ddx agent execute-loop --poll-interval 30s

# Run inline in the current process (no server worker)
ddx agent execute-loop --local --once

# Debug a specific bead (primitive; bypasses the queue)
ddx agent execute-bead <bead-id>

# Preserve the result instead of merging it back
ddx agent execute-bead <bead-id> --no-merge
```

## Related

- `cli/internal/agent/execute_bead_loop.go` — canonical close-semantics source
- `cli/internal/agent/execute_bead_status.go` — status constants
- `ddx agent execute-loop --help`, `ddx agent execute-bead --help`
