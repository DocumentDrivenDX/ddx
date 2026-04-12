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
the default active-work collection must remain safe under concurrent access and
recoverable after partial corruption without requiring a separate database.

## Goals

- Keep the default backend human-readable and portable.
- Preserve unknown fields end-to-end for workflow-specific metadata.
- Make queue operations deterministic from a single parsed snapshot.
- Prevent corruption from concurrent writers.
- Self-heal partially corrupted JSONL when valid beads still exist.
- Expose enough context to diagnose malformed input without losing the whole queue.

## Reuse Boundary

The bead store is the reusable DDx record-store substrate, but the current
implementation is too tightly coupled to the primary tracker file.

- The backend interface is bead-shaped, which is useful: it gives DDx one
  portable record schema with unknown-field preservation and interchangeable
  backends.
- The missing boundary is between the storage engine and one specific file such
  as `beads.jsonl`.
- DDx should support multiple named bead-backed collections, each with its own
  retention and indexing behavior.

The reusable pieces are lower-level:

- directory locking
- atomic temp-file swap
- repo-local backend selection conventions
- best-effort repair behavior where a line-oriented store is appropriate
- a stable bead-schema envelope with domain-specific fields in preserved extras

Execution, metric, archive, and agent-session storage should reuse those
mechanics through separate bead-backed collections rather than sharing the
primary active-work file.

## Data Format

### Primary File

- Path: `.ddx/beads.jsonl` for the default active-work collection
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

The design also reserves two workflow-facing shapes:

- `assignee` is the advisory owner recorded by claim operations.
- `events` is an append-only array of execution evidence records stored in
  `Extra["events"]`.

Each evidence record carries `kind`, `summary`, `body`, `actor`,
`created_at`, and `source`. DDx treats the array as ordered history and never
rewrites or compacts it during normal operations.

## Claim Algorithm

Claim/unclaim stays a normal bead mutation, but ownership is now explicit.

1. Acquire the bead store lock.
2. Read the current bead snapshot.
3. Resolve the assignee from the explicit `--assignee` flag, then runtime
   caller identity, then `ddx` as the fallback.
4. For `--claim`, set `status=in_progress`, `assignee`, `claimed-at`, and
   `claimed-pid`.
5. For `--unclaim`, set `status=open` and clear `assignee`, `claimed-at`, and
   `claimed-pid`.
6. Serialize the full snapshot to `.ddx/beads.jsonl.tmp`.
7. Atomically rename the temp file into place.

Claims remain advisory; the store does not introduce a hard reservation lock.
The metadata exists so humans and agents can distinguish who holds the claim
and when it happened.

## Evidence Algorithm

Execution evidence is append-only.

1. Acquire the bead store lock.
2. Read the current bead snapshot.
3. Load the target bead and append one evidence entry to `Extra["events"]`.
4. Populate `kind`, `summary`, `body`, `actor`, `created_at`, and `source`.
5. Serialize the full snapshot to `.ddx/beads.jsonl.tmp`.
6. Atomically rename the temp file into place.

This deliberately preserves full history. Evidence writes never mutate or
remove existing entries, so close summaries and experiment outcomes remain
auditable.

## Read Path

The read path is intentionally best-effort.

1. Read the full file from disk.
2. Scan line-by-line instead of parsing the entire file as one JSON value.
3. Trim whitespace per line.
4. Unmarshal each non-empty line independently.
5. Preserve valid beads in a snapshot.
6. Emit line-numbered warnings for malformed records.
7. Return `events` history and claim metadata exactly as stored.
8. If at least one valid bead exists and at least one malformed record was seen,
   trigger repair under the store lock.
9. If no valid bead exists, return a contextual error that names the file and
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
- Evidence history stored in `Extra["events"]` does not affect queue derivation.
- Claim metadata does not affect queue derivation beyond the bead's status.

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
- `TestBeadClaimUsesExplicitAssignee`
- `TestBeadClaimFallsBackToCallerIdentity`
- `TestBeadUnclaimClearsClaimMetadata`
- `TestBeadEvidenceAppendPreservesOrder`
- `TestBeadEvidenceAppendAtomicWithConcurrentWriters`
- `TestBeadShowJSONIncludesEvidenceHistory`
- `docs/helix/03-test/test-plans/TP-004-beads-claims-evidence.md`

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
- If evidence append fails, the mutation fails atomically and the prior bead
  snapshot remains unchanged.

## Non-Goals

- Real-time sync between multiple bead stores.
- Database-backed mutation logic in the default backend.
- Workflow-specific validation policy in the bead engine itself.

Those concerns belong in higher layers such as HELIX hooks, import/export, or
alternative backends.
