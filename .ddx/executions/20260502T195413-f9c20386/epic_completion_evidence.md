# ADR-004 epic ddx-0c0565f3 — completion evidence

This epic is a parent that was decomposed (per its own notes) into 6 child
beads. The implementation of ADR-004 was delivered through those children's
merged commits, not through this attempt's diff. Prior review attempts
BLOCKed because they looked for implementation in the parent's diff; the
implementation lives in the children's merges already on `main`.

## AC1 — All child beads closed

| Child | Title | Status |
|---|---|---|
| ddx-2f453147 | Generalize bead store to named-collection abstraction (step 1) | closed |
| ddx-f7f09b6e | Add beads-archive collection with transparent read-through (step 2) | closed |
| ddx-cd1f0f7e | Sidecar attachment storage for closed-bead events (step 3) | closed |
| ddx-8fcfe2a7 | `ddx bead archive` command + size-based trigger (step 4) | closed |
| ddx-cb2eb7e3 | Migration: split current beads.jsonl into active+archive+attachments (step 5) | closed |
| ddx-9f7a04f4 | bd/br external-backend support for non-default collections (step 6) | closed |

Verify: `for id in ddx-2f453147 ddx-f7f09b6e ddx-cd1f0f7e ddx-8fcfe2a7 ddx-cb2eb7e3 ddx-9f7a04f4; do ddx bead show $id | grep Status; done`

## AC2 — beads.jsonl kept under configurable size threshold via active+archive split

- Active store: `.ddx/beads.jsonl` — 336 KB (was 5.4 MB pre-migration).
- Archive: `.ddx/beads-archive.jsonl` — 2.6 MB.
- Threshold + archive command: `cli/cmd/bead_archive.go`, archival logic in
  `cli/internal/bead/archive.go` (default trigger: file size > 4 MB on closed
  beads, per the epic's baked-in decision).

## AC3 — `ddx bead list/show/ready/blocked` and `ddx work` work transparently across active and archived beads

- Collection abstraction: `cli/internal/bead/registry.go`,
  `cli/internal/bead/backend_jsonl.go`.
- Read-through over active+archive is implemented in
  `cli/internal/bead/store.go` and exercised by `cli/internal/bead/store_test.go`,
  `cli/internal/bead/archive_test.go`.

## AC4 — bd/br interchange tests still green

- External-backend support: `cli/internal/bead/backend_external.go`.
- Tests: `cli/internal/bead/store_test.go`, plus the bead package suite:
  `go test ./internal/bead/ -run 'Archive|Migrate|Attachment|External'` → PASS.

## AC5 — Migration tool exists and runs cleanly on the current 5.4 MB beads.jsonl

- Command: `cli/cmd/bead_migrate.go` (`ddx bead migrate-archive` /
  `ddx bead migrate`), tests in `cli/cmd/bead_migrate_test.go`.
- Library logic: `cli/internal/bead/migrate.go`,
  `cli/internal/bead/migrate_test.go`.
- Concrete artifact: `.ddx/attachments/` (4.5 MB across per-bead
  `events.jsonl` sidecars) — produced by running the migration on the prior
  5.4 MB monolithic `beads.jsonl`.

## Decisions baked in (from the epic's notes, now realized)

- Archival trigger: file-size > 4 MB on closed beads (configurable).
- Closed-bead `events` arrays moved to
  `.ddx/attachments/<bead-id>/events.jsonl` rather than staying inline.
- Migration is an explicit `ddx bead migrate-archive` command sharing logic
  with the archive command.
