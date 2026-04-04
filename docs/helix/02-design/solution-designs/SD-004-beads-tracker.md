---
ddx:
  id: SD-004
  depends_on:
    - FEAT-004
    - ADR-001
    - ADR-003
---
# Solution Design: Beads Tracker

## Overview

This design specifies the runtime behavior of `ddx bead` and the file-backed
tracker used by HELIX and other workflows. The key design constraint is that
`beads.jsonl` must remain safe under concurrent access and recoverable after
partial corruption without requiring a separate database.

## Goals

- Keep the default backend human-readable and portable.
- Preserve unknown fields end-to-end for workflow-specific metadata.
- Make queue operations deterministic from a single parsed snapshot.
- Prevent corruption from concurrent writers.
- Self-heal partially corrupted JSONL when valid beads still exist.
- Expose enough context to diagnose malformed input without losing the whole queue.

## Data Format

### Primary File

- Path: `.ddx/beads.jsonl`
- Format: one JSON object per line
- Ordering: written sorted by `id`
- Semantics: each line is one full bead record; blank lines are ignored

### Repair Artifacts

- Backup path: `.ddx/beads.jsonl.bak`
- Temporary write path: `.ddx/beads.jsonl.tmp`
- Backups are created only when the store auto-repairs a partially corrupted file
- The replacement write is atomic on the same filesystem via `rename`

### Field Preservation

Known fields are parsed into the bead struct. Unknown fields are preserved in
`Extra` and round-trip through read/write and import/export flows. This is the
mechanism that allows HELIX to store fields such as `spec-id`,
`execution-eligible`, `claimed-at`, `claimed-pid`, `superseded-by`, and
`replaces`.

## Read Path

The read path is intentionally best-effort.

1. Read the full file from disk.
2. Scan line-by-line instead of parsing the entire file as one JSON value.
3. Trim whitespace per line.
4. Unmarshal each non-empty line independently.
5. Preserve valid beads in a snapshot.
6. Emit line-numbered warnings for malformed records.
7. If at least one valid bead exists and at least one malformed record was seen,
   trigger repair under the store lock.
8. If no valid bead exists, return a contextual error that names the file and
   malformed-record count.

Why this shape:

- A single malformed record should not break `ready`, `blocked`, `status`, or
  `list`.
- Line numbers are enough to isolate the bad record quickly.
- A full snapshot keeps queue views deterministic within a single command.

## Repair Path

Repair only runs when the file contains a mix of valid and malformed records.

1. Acquire the bead store lock.
2. Re-read the current file under the lock.
3. Reparse it to verify it still needs repair.
4. Copy the current file to `.ddx/beads.jsonl.bak` using a temp file plus rename.
5. Rewrite the cleaned bead snapshot using the normal atomic writer.
6. Leave the backup in place for inspection and rollback.

This prevents concurrent readers from racing to repair the same file after it
has already been fixed by another process.

## Write Path

Mutating commands use the same pattern:

1. Acquire the store lock.
2. Read the current bead snapshot.
3. Apply the mutation in memory.
4. Validate the result.
5. Serialize the full snapshot to `.ddx/beads.jsonl.tmp`.
6. Rename the temp file to `.ddx/beads.jsonl`.

This ensures there is never a partially written main tracker file. The lock
serializes writers, and the temp-file swap prevents readers from seeing a half
written JSONL record.

## Queue Derivation

The tracker queue views are derived from one in-memory snapshot.

### Ready

- Consider only `open` beads.
- A bead is ready when every dependency is `closed`.
- Execution-ready views additionally filter on `execution-eligible` and
  `superseded-by`.
- Results are sorted by priority, then by stable iteration order from the
  parsed snapshot.

### Blocked

- Consider only `open` beads.
- A bead is blocked when at least one dependency is not `closed`.

### Status

- `Total` is the number of parsed beads.
- `Open`, `Closed`, `Ready`, and `Blocked` are derived from the same snapshot.
- Status reporting never reparses the file independently of the queue view.

## Concurrency Model

- Directory locks prevent simultaneous writes.
- Reads are allowed without the lock, but the repair path uses the lock before
  rewriting anything.
- No API writes directly to `beads.jsonl`; all mutation goes through `WriteAll`
  or a higher-level store method that calls it.
- External tools are expected to use `ddx bead`, not edit the JSONL directly.

This model is sufficient for the expected HELIX/DDX usage pattern:
multiple agents can race to claim or update work, but the store will serialize
the resulting file writes and either preserve or repair the tracker snapshot.

## Performance Targets

The feature spec sets the target at under 100 ms for local operations on up to
10,000 beads. The design supports that target by:

- using a single sequential parse of the JSONL file
- avoiding database overhead
- deriving queue views from one snapshot
- keeping writes as a single temp-file pass plus rename

## Validation Matrix

These are the key tests that validate the design:

- `go test ./internal/bead/...`
- `go test ./cmd -run 'TestBead' -count=1`
- `TestConcurrentCreatesSerialized`
- `TestMalformedJSONLSkipsBadRecords`
- `TestMalformedJSONLAllBadReturnsError`

Expected outcomes:

- Concurrent creates complete without corruption.
- A mixed-validity JSONL file is repaired with a `.bak` backup.
- An all-malformed JSONL file fails with a contextual error.
- Queue commands keep working when at least one valid record remains.

## Failure Modes

- If the file is completely malformed, the command fails instead of fabricating
  empty state.
- If the repair backup cannot be written, the read returns an error and leaves
  the original file untouched.
- If a lock cannot be acquired in time, the mutating command fails cleanly.

## Non-Goals

- Real-time sync between multiple bead stores.
- Database-backed mutation logic in the default backend.
- Workflow-specific validation policy in the bead engine itself.

Those concerns belong in higher layers such as HELIX hooks, import/export, or
alternative backends.
