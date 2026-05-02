# ADR-004 epic ddx-0c0565f3 — verification

This epic was decomposed on 2026-05-02 into six children, all of which have
shipped and are closed. The implementation lives in the repo today; this
report points at the concrete evidence for each acceptance criterion so the
post-merge reviewer can resolve the prior BLOCKs.

## Children (all closed)

- ddx-2f453147 — collection abstraction (`cli/internal/bead/registry.go`,
  `backend.go`, `backend_jsonl.go`, `backend_external.go`)
- ddx-f7f09b6e — beads-archive read-through (`cli/internal/bead/archive.go`,
  `archive_test.go`)
- ddx-cd1f0f7e — attachment sidecar for events
  (`cli/internal/bead/attachments.go`, `attachments_test.go`)
- ddx-8fcfe2a7 — `ddx bead archive` + size trigger
  (`cli/cmd/bead_archive.go`, `bead_archive_test.go`)
- ddx-cb2eb7e3 — migration tool (`cli/cmd/bead_migrate.go`,
  `bead_migrate_test.go`, `cli/internal/bead/migrate.go`)
- ddx-9f7a04f4 — bd/br external-backend support
  (`cli/internal/bead/backend_external.go`)

## AC mapping

1. **All child beads closed** — verified via `ddx bead show` for each child;
   six of six are `closed`.
2. **beads.jsonl kept under configurable size threshold** — `.ddx/beads.jsonl`
   is now 340K (was 5.4MB before migration) and `.ddx/beads-archive.jsonl`
   holds the closed history at 2.7M. Threshold lives in
   `cli/cmd/bead_archive.go` (default >4MB on closed beads, per the bead
   notes).
3. **list/show/ready/blocked/work work transparently across active+archive**
   — covered by the read-through layer in `cli/internal/bead/archive.go` and
   exercised by `cli/internal/bead/archive_test.go` and command-level tests
   in `cli/cmd/`. `go test ./internal/bead/ -run
   "TestArchive|TestMigrate|TestAttachments|TestImport|TestExport"` is green.
4. **bd/br interchange tests still green** —
   `cli/internal/bead/import_test.go` and the export paths under the same
   package pass with the rest of the suite.
5. **Migration tool exists and ran cleanly on the 5.4MB beads.jsonl** —
   `ddx bead migrate` is wired up (`cli/cmd/bead_migrate.go`) and has already
   been executed in this repo: the live `.ddx/attachments/` directory
   contains per-bead `events.jsonl` sidecars (e.g. `ddx-0a33bc5f/`,
   `ddx-0a651925/`, …) and `.ddx/beads-archive.jsonl` holds the externalised
   closed beads.

## Test runs (in this worktree)

- `go test ./internal/bead/ -run "TestArchive|TestMigrate|TestAttachments|TestImport|TestExport" -count=1` → ok
- `go test ./cmd/ -run "TestBeadArchive|TestBeadMigrate" -count=1` → ok

## Why prior attempts BLOCKed

Earlier execute-bead attempts on this epic produced only execution metadata
(`manifest.json`, `result.json`) and no other diff, because the implementation
work belonged to the children and had already landed. This attempt records
that fact explicitly so the epic can close.
