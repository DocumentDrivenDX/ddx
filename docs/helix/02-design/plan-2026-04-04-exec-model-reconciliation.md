---
ddx:
  id: PLAN-2026-04-04-EXEC-MODEL-RECONCILIATION
  depends_on:
    - FEAT-004
    - FEAT-010
---
# Design Plan: DDx Execution Model Reconciliation

**Date**: 2026-04-04
**Status**: Converged
**Refinement Rounds**: 4

## Problem Statement

DDx now has a generic execution substrate, but the repository had drifted into two parallel runtime stories:

- `ddx exec` as the shared runtime model for definitions, runs, logs, and structured results
- `ddx metric` as a narrower projection for numeric observations and trends

That split created three risks:

- storage divergence between generic execution records and metric-specific files
- inconsistent migration behavior for existing `.ddx/exec/` and `.ddx/metrics/` data
- duplicated read paths, test surfaces, and failure handling for the same logical runtime evidence

The user impact is concrete: build-time evidence, metric observations, and future agent/session records should all sit on the same substrate so operators and automation can query one model instead of reconciling multiple local stores.

## Requirements

### Functional

- `ddx exec` remains the authoritative runtime API for execution definitions, run history, logs, and structured results.
- Metric behavior is a projection over `ddx exec` records, not a second authoritative store.
- New runtime writes land in bead-backed named collections.
- Legacy `.ddx/exec/definitions/`, `.ddx/exec/runs/`, and `.ddx/metrics/` layouts remain readable during migration.
- Attachment-backed payloads remain separate from metadata rows.
- External backends receive logical collection names only; DDx must not encode backend file layout assumptions.
- Concurrent writers must not publish partial runtime records or mismatched attachment references.

### Non-Functional

- Runtime reads and writes must remain deterministic.
- Writes must be atomic or serialized at the collection boundary.
- Collection repair should preserve a backup before replacement and use an
  atomic swap for the final publish step.
- Migration reads must be explicit and test-covered.
- Metric projections must not require a separate authoritative history file.
- The model must remain portable across `jsonl`, `bd`, and `br`.

### Constraints

- Preserve bd/br-compatible bead envelope fields and unknown-field round-tripping.
- Keep the primary bead tracker collection compatible with `bd`/`br` interchange expectations.
- Avoid introducing top-level bead fields that would break the active tracker contract.
- Keep execution-specific payloads in preserved extras on the existing bead
  envelope rather than adding shared envelope fields.
- Maintain repo-local, file-backed storage for v1.

## Architecture Decisions

### Decision 1: Make `ddx exec` the single runtime authority

- **Question**: Should metrics own their own storage or project over generic execution records?
- **Alternatives**:
  - Keep a separate `.ddx/metrics/` runtime store.
  - Reuse `ddx exec` and layer metric semantics on top.
- **Chosen**: Reuse `ddx exec`.
- **Rationale**: Metrics are just one runtime specialization. Separate storage multiplies write paths, migration rules, and corruption modes.

### Decision 2: Store runtime records in named bead-backed collections

- **Question**: Should execution definitions/runs remain bespoke JSON files or become bead-backed collections?
- **Alternatives**:
  - Bespoke file trees under `.ddx/exec/`
  - Named bead-backed collections with attachment sidecars
- **Chosen**: Named bead-backed collections.
- **Rationale**: The bead envelope already gives DDx the required portability, locking, and compatibility surface.

### Decision 3: Preserve legacy runtime data as read-only fallback

- **Question**: How do we avoid stranding existing repositories?
- **Alternatives**:
  - Hard cutover, breaking old runtime data.
  - Automatic migration with destructive rewrite.
  - Read fallback with new authoritative writes landing in collections.
- **Chosen**: Read fallback only, with new writes targeting the collections.
- **Rationale**: This minimizes migration risk and avoids silent data loss while still converging on one model.

### Decision 4: Keep large payloads in attachment directories

- **Question**: Should stdout, stderr, and structured result bodies be inline or sidecar?
- **Alternatives**:
  - Inline everything in the metadata row.
  - Sidecar all large payloads.
- **Chosen**: Sidecar attachments referenced from the bead row.
- **Rationale**: It keeps collection rewrites small and makes concurrent publication safer.

### Decision 5: Treat metric commands as projections, not owners

- **Question**: Should `ddx metric` own its own lifecycle and storage?
- **Alternatives**:
  - Metric-specific storage and validation
  - Projection over `ddx exec`
- **Chosen**: Projection over `ddx exec`.
- **Rationale**: The command surface stays useful, but the runtime substrate stays singular.

## Interface Contracts

### CLI

- `ddx exec list [--artifact ID]`
- `ddx exec show <definition-id>`
- `ddx exec validate <definition-id>`
- `ddx exec run <definition-id>`
- `ddx exec log <run-id>`
- `ddx exec result <run-id> [--json]`
- `ddx exec history [--artifact ID] [--definition ID]`
- `ddx metric validate|run|compare|history|trend` remains a projection layer over `ddx exec`

### Storage Formats

- `exec-definitions` and `exec-runs` are logical bead-backed collections.
- JSONL backend maps those collections to `.ddx/exec-definitions.jsonl` and `.ddx/exec-runs.jsonl`.
- Attachment publication uses `.ddx/exec-runs.d/<run-id>/`.
- Legacy `.ddx/exec/definitions/` and `.ddx/exec/runs/` remain readable only during migration.
- Collection writes should use the bead store pattern: lock the collection,
  write a temp snapshot, keep a `.bak` on repair, and swap the completed file
  into place atomically.

### Validation Rules

- Definition IDs must be stable and non-empty.
- Execution definitions must link to at least one artifact ID.
- Metric definitions must resolve to exactly one governing `MET-*` artifact.
- Run records must not be published until attachments exist.
- Attachment references must be repo-local paths, not arbitrary external URLs.

## Data Model

### Execution Definition Record

Stored as a bead-backed metadata row with preserved extras for the execution contract.

Key fields:

- `id`
- `issue_type=exec_definition`
- `status`
- `title`
- `labels`
- `artifact_ids`
- `executor`
- `result`
- `evaluation`
- `active`
- `created_at`

### Execution Run Record

Stored as a bead-backed metadata row plus attachment files.

Key fields:

- `id` / `run_id`
- `issue_type=exec_run`
- `status`
- `definition_id`
- `artifact_ids`
- `started_at`
- `finished_at`
- `run.status`
- `exit_code`
- `attachments`
- `provenance`
- `result`

### Metric Projection

Metric history is derived from execution runs by filtering on the governing `MET-*` artifact ID and then reading the structured result payload.

Metric-specific read models:

- comparison against a baseline or a prior run
- trend summaries over observed values
- pass/fail interpretation based on threshold and ratchet policy

### Migration Policy

- New writes target bead-backed collections.
- Legacy `.ddx/exec/` and `.ddx/metrics/` data is readable.
- No automatic destructive migration is required for v1.
- The repo should be able to state the current authoritative path unambiguously in docs and tests.

## Error Handling Strategy

- Missing artifact ID: fail validation with the artifact ID and definition ID.
- Invalid executor kind: fail before invocation.
- Missing command for command-backed definitions: fail validation.
- Missing or corrupt attachments: fail run inspection with the concrete run ID and attachment path.
- Corrupt legacy bundle: report the legacy path and do not rewrite it implicitly.
- Missing `bd`/`br` tool: fall back to JSONL backend; do not fail collection resolution just because an external backend is unavailable.
- Partial write during run publication: do not publish the metadata row until attachments are fully present.

## Security Considerations

- Command execution remains the primary attack surface; definitions must not be treated as trusted code.
- Attachment paths must be normalized to repo-local locations to prevent traversal or path injection.
- External backend shell-out must avoid assuming backend-internal storage layout.
- Metrics and exec payloads may contain sensitive logs; redaction belongs to the caller or executor contract, not to ad hoc storage rewrites.

## Test Strategy

- **Unit**: definition/run conversion, attachment path normalization, legacy fallback readers, metric projection mapping, and validation failures.
- **Integration**: `ddx exec` CLI commands against JSONL collections and migrated legacy data.
- **Concurrency**: concurrent definition/run writers must not corrupt collections or attachment publication.
- **Atomicity**: collection repair and write paths must leave either the old
  snapshot or the new snapshot visible, never a partial rewrite.
- **Compatibility**: adapter-level tests must confirm logical collection names propagate to external backends, and schema validation must continue to accept bd/br-compatible bead records.
- **E2E**: `ddx metric` commands should exercise the same execution store as `ddx exec` and never reintroduce a separate authoritative metric store.

Critical paths before release:

- create definition
- run definition
- read run logs/result
- query history
- project metric compare/trend
- read legacy exec/metric data without corruption

## Implementation Plan

### Dependency Graph

1. Bead collection model and schema guard
2. Generic execution substrate
3. Metric projection over execution substrate
4. Legacy read fallback and migration policy
5. External backend adapter coverage
6. CI wiring for bead schema validation
7. Future specialization surfaces such as agent/session projections

### Issue Breakdown

- The generic exec substrate, metric projection, collection migration, backend adapter tests, and CI guard are now implemented or queued through the tracker.
- No new build issue is required for the current reconciled state.
- The remaining planning epic serves as the umbrella record for future runtime projections and eventual deprecation of legacy exec/metric layouts after migration confidence is established.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Legacy layout fallback masks hidden migration bugs | Medium | High | Keep legacy reads explicit, logged, and test-covered; do not silently mutate old bundles |
| Attachment publication and row publication drift apart under concurrency | Medium | High | Publish attachments before metadata rows, use collection locks, and test concurrent writers |
| External backend collection naming diverges from DDx expectations | Medium | Medium | Adapter tests assert logical collection propagation only, not backend file layout |
| Metric projections drift from exec semantics over time | Medium | High | Keep metric history as a read model over exec runs and avoid metric-owned storage |
| Large logs or results make shared rewrites fragile | Medium | High | Keep logs and result payloads in sidecars |

## Observability

- Log validation failures with definition ID, artifact ID, and collection name.
- Log run publication failures with run ID and attachment path.
- Surface legacy fallback usage so operators know when migration reads are still happening.
- Track execution latency, attachment publication latency, and collection write latency separately.
- Keep command output human-readable, but preserve JSON output for automation.

## Governing Artifacts

- [FEAT-010: Executions](../01-frame/features/FEAT-010-executions.md)
- [SD-005: Metric Runtime and History](solution-designs/SD-005-metric-runtime-history.md)
- [TD-005: Metric Runtime and History](technical-designs/TD-005-metric-runtime-history.md)
- [TD-010: Execution Definitions and Runs](technical-designs/TD-010-executions.md)
- [ADR-004: Bead-Backed Runtime Storage](adr/ADR-004-bead-backed-runtime-storage.md)
- [FEAT-004: Beads](../01-frame/features/FEAT-004-beads.md)
