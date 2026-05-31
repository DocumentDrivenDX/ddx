# Bead Lifecycle

Beads are portable work items tracked in `.ddx/beads.jsonl`. This document covers
the bead lifecycle and auto-close behaviours. For the full bead state machine and
storage design see `docs/helix/02-design/technical-designs/TD-027-bead-collection-abstraction.md`.

## Status values

| Status | Meaning |
|---|---|
| `open` | Ready for execution or waiting on dependencies |
| `in_progress` | Claimed by a worker |
| `closed` | Done; satisfies downstream dependents |
| `cancelled` | Will not run; does not satisfy dependents |
| `blocked` | Waiting on an external blocker |
| `proposed` | Awaiting operator approval |

## Epic auto-close

When the last non-terminal child of an epic bead transitions to a terminal state
(`closed` or `cancelled`) via `ddx bead close`, the epic is automatically closed.

Rules:
- Both `closed` and `cancelled` children count as terminal.
- The epic must have at least one child (epics with no children are left alone).
- The auto-close fires immediately during `ddx bead close` on the child — no
  separate command is needed.
- An `epic_auto_close` event is appended to the epic's event stream recording the
  reason.

### Backfill with `ddx bead reap`

Epics that existed before this feature landed may still be open with all-terminal
children. Use `ddx bead reap` to find and close them:

```
ddx bead reap           # list candidates (dry run)
ddx bead reap --apply   # close all candidates
```

### Dead-intermediate auto-close (RC-3)

Beads with `execution-eligible=false` (i.e. structural intermediates/containers
that were set non-executable by the decomposer) are also auto-closed when all their
children reach terminal state. This is the RC-3 walk-up closure, which predates the
epic auto-close feature. The event kind for this path is `dead_intermediate_close`.
