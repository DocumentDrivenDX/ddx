# ddx-0c0565f3 — ADR-004 Epic Closure Evidence

This bead is the parent epic. All implementation landed via the six children
listed in the bead notes. This report verifies each acceptance criterion
against the current state of the repository.

## AC1 — All child beads closed

| Child            | Status | Closing commit |
|------------------|--------|----------------|
| ddx-2f453147 (collection abstraction)            | closed | 55838f32 |
| ddx-f7f09b6e (beads-archive read-through)        | closed | c69689ec |
| ddx-cd1f0f7e (attachment sidecar for events)     | closed | a8cf295c |
| ddx-8fcfe2a7 (`ddx bead archive` + size trigger) | closed | cd0b37b5 |
| ddx-cb2eb7e3 (migration tool)                    | closed | df2cea94 |
| ddx-9f7a04f4 (bd/br external-backend support)    | closed | 9ede5414 |

Verified via `ddx bead show <id>` for each child.

## AC2 — beads.jsonl kept under configurable size threshold via active+archive split

```
.ddx/beads.jsonl          115 KB   (active queue)
.ddx/beads-archive.jsonl  2.7 MB   (closed/archived rows, events externalized)
.ddx/attachments/         571 sidecar dirs
```

Active file is well under the default 4 MB threshold (`ddx bead archive
--max-size`, default `4194304`). Threshold is configurable via the
`--max-size`, `--older-than`, `--max-count` flags on `ddx bead archive`.

## AC3 — Existing tooling still works transparently

- `ddx bead list` lists active+archived beads (sample tail showed mixed
  archived rows like `hx-fee1dec1`, `smoke-wt-2fa5cc3b`).
- `ddx bead show ddx-2f453147` resolves an archived bead transparently
  (read-through from ddx-f7f09b6e).
- `ddx bead ready` still operates on the active collection only and
  returned ready P3 beads.
- `ddx work` (alias of `ddx agent execute-loop`) is the very command
  driving this attempt — it is functioning.

## AC4 — bd/br interchange tests still green

`go test ./internal/bead/...` → `ok github.com/DocumentDrivenDX/ddx/internal/bead 4.511s`.
This package contains `schema_compat_test.go` and the bd/br round-trip
tests covered by ddx-9f7a04f4.

## AC5 — Migration tool exists and runs cleanly

- `ddx bead migrate --help` exists ("Split active beads.jsonl into beads +
  beads-archive + attachments").
- `ddx bead migrate --dry-run` on the current repo reports:
  `Externalized events: 0 / Archived beads: 0 / No changes — already migrated.`
  i.e. the migration was already applied and the command is idempotent.

## Conclusion

All five acceptance criteria are satisfied by the merged children plus the
current state of `.ddx/`. No further code change is required at the epic
level; this report is the evidence artifact for the post-merge reviewer.
