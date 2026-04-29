# Tracker Usage

Use the built-in HELIX tracker only. Issues are stored in `.ddx/beads.jsonl`.

## Review Epic

```bash
ddx bead create "HELIX alignment review: <scope>" \
  --type epic \
  --labels helix,phase:review,kind:review \
  --spec-id <closest-governing-artifact>
```

## Review Issue

```bash
ddx bead create "Review <functional area>" \
  --type task \
  --parent <review-epic-id> \
  --labels helix,phase:review,kind:review,area:<area>
```

## Execution Issue

```bash
ddx bead create "<deterministic follow-up work>" \
  --type task \
  --labels helix,phase:<phase>,kind:<kind> \
  --spec-id <closest-governing-artifact>
```

Add blockers with:

```bash
ddx bead dep add <blocked-id> <blocker-id>
```

Use:

- `ddx bead ready` for unblocked work
- `ddx bead blocked` for blocked work
- `ddx bead show <id>` for context
- `ddx bead update <id> --claim` to claim work
- `ddx bead close <id>` when complete
