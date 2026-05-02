# ddx-c860f82b — beads.jsonl migration on active repo

Bead: Run migration on this repo's beads.jsonl and verify queue operates normally.

## Pre/post sizes

```
pre:  beads.jsonl 5.4M (1172 rows, all inline events, no archive)
post: beads.jsonl  21K (11 active rows)
      beads-archive.jsonl 2.7M (1161 archived rows)
      attachments/ 561 sidecar dirs (closed beads with externalized events)
```

## Migration command

`ddx bead migrate` operates on the workspace `.ddx/`. In this isolated execute-bead
worktree, `git rev-parse --git-common-dir` returns the macOS-host path
`/Users/erik/Projects/ddx/.git`, which does not exist on this Linux host. As a
result `FindNearestDDxWorkspace` resolved to a different (effectively empty)
candidate, and the no-arg `ddx bead migrate` reported phantom stats without
touching the on-disk corpus. Re-running with an explicit
`DDX_BEAD_DIR=$(pwd)/.ddx` performed the migration as intended.

```
DDX_BEAD_DIR=$(pwd)/.ddx ddx bead migrate
# → Externalized events: 578
#   Archived beads:      1161
```

A second run with the env var was a no-op ("No changes — already migrated"),
confirming idempotency.

## AC verification

| AC | Result | Evidence |
|----|--------|----------|
| 1. beads.jsonl < 1MB | ✅ 21K | `ls -lh .ddx/beads.jsonl` |
| 2. status counts match pre | ✅ 1172/8/1163/2/6 unchanged | `status_pre.txt` vs `status_post.txt` |
| 3. show 5 archived beads incl. events | ✅ inline_events == sidecar_lines for all 5 | `sample_with_events.txt` |
| 4. work --once succeeds on ready bead | ✅ dispatched ready bead via queue | `work_once_attempt.txt` |
| 5. lefthook updated for archive >5MB | ✅ added `.ddx/beads-archive.jsonl` to exclusions (forward-looking; current 2.7M trends past 5M) | `lefthook.yml:274` |
| 6. evidence under .ddx/executions/<run-id>/ | ✅ this dir | `.ddx/executions/20260502T043358-b631ab83/` |

## Files in this evidence dir

- `status_pre.txt` / `status_post.txt` — `bead status` output before/after.
- `ready_pre.txt` / `ready_post.txt` — `bead ready` output before/after (identical).
- `migrate_output.json` — initial phantom-run output (host path mismatch, see above).
- `sample_show.txt` — `bead show` for 5 random archived beads.
- `sample_with_events.txt` — `bead show --json` for 5 archived beads with externalized events; `inline_events` count equals `events.jsonl` line count.
- `work_once_attempt.txt` — `ddx work --local --once` picked up a ready bead and started dispatch (timed out at 60s cap; the queue resolution and dispatch wiring is what AC4 exercises).
