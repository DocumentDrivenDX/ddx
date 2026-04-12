---
ddx:
  id: TP-010
  depends_on:
    - FEAT-010
    - ADR-004
    - TD-010
---
# Test Plan: Execution Definitions and Runs

## Scope

Validate the generic execution substrate: bead-backed collection storage,
attachment-backed payloads, deterministic inspection, and migration-safe reads
from legacy `.ddx/exec/` data.

## Test Cases

### Definition Storage and Inspection

- `TestExecCommandsValidateRunHistoryAndResult`
- `TestDefinitionRoundTrips`
- `TestExecListFiltersDefinitionsByArtifactID`
- `TestExecShowReturnsLatestDefinitionRecord`

### Validation

- `TestValidateRunHistoryAndBundle`
- `TestExecValidateRejectsMissingArtifact`
- `TestExecValidateRejectsMissingCommandExecutorConfig`
- `TestExecValidateRejectsUnknownExecutorKind`

### Run Persistence

- `TestExecRunWritesCollectionBackedRunRecord`
- `TestExecRunPublishesResultStdoutAndStderrAttachments`
- `TestExecResultReadsAttachmentBackedPayload`
- `TestExecHistoryFiltersByArtifactAndDefinition`

### Compatibility and Migration

- `TestListDefinitionsFallsBackToLegacyExecDirectory`
- `TestHistoryFallsBackToLegacyExecBundle`
- `TestCollectionRecordsTakePrecedenceOverLegacyExecData`

### Concurrency and Atomicity

- `TestConcurrentRunBundleWrites`
- `TestExecRunRecordWriteIsAtomicUnderConcurrentWriters`
- `TestExecReaderDoesNotObservePartialRunRecord`

## Fixtures

- `.ddx/exec-definitions.jsonl`
- `.ddx/exec-runs.jsonl`
- `.ddx/exec-runs.d/<run-id>/result.json`
- `.ddx/exec/definitions/*.json` legacy fixtures
- `.ddx/exec/runs/<run-id>/` legacy fixtures

## Exit Criteria

- New authoritative writes land in `exec-definitions` and `exec-runs`
  bead-backed collections.
- Attachment publication is deterministic and published rows never reference
  missing files.
- Legacy `.ddx/exec/` definitions and run bundles remain readable during the
  migration window.
- CLI inspection commands and the underlying readers agree on definition/run
  identity, ordering, and attachment resolution.
