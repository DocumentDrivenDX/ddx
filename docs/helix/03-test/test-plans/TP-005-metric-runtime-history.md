---
ddx:
  id: TP-005
  depends_on:
    - FEAT-005
    - FEAT-010
    - TD-010
    - SD-005
    - TD-005
---
# Test Plan: Metric Runtime and History

This plan extends [TP-010](/home/erik/Projects/ddx/docs/helix/03-test/test-plans/TP-010-executions.md).
Generic execution storage, attachment, and legacy-compatibility coverage lives
there; this plan adds the metric-specific projection and comparison cases.

## Scope

Validate that metric artifacts remain declarative while metric-linked
`ddx exec` definitions and generic execution runs are file-backed,
deterministic, and safe under concurrent writes.

## Test Cases

### Artifact Boundary

- `TestMetricArtifactValidationDoesNotRequireExecutableCode`
- `TestMetricExecDefinitionLinksToMetricArtifact`
- `TestMetricExecRunLinksToMetricAndDefinition`

### Runtime Validation

- `TestExecValidateRejectsMissingMetricArtifact`
- `TestExecValidateRejectsMissingDefinition`
- `TestExecValidateRejectsInvalidMetricThresholds`
- `TestExecValidateReportsArtifactAndDefinitionPaths`

### Run and History

- `TestExecRunWritesMetricLinkedRunRecord`
- `TestExecRunCapturesExitCodeStdoutAndStderr`
- `TestMetricHistoryProjectionPreservesAppendOrder`
- `TestMetricTrendProjectionFiltersByMetricArtifactId`

### Concurrency and Atomicity

- `TestExecRunRecordWriteAtomicUnderConcurrentWriters`
- `TestExecDefinitionWriteUsesAtomicSwap`
- `TestExecReaderDoesNotObservePartialRunRecord`

## Fixtures

- `.ddx/exec-definitions.jsonl`
- `.ddx/exec-runs.jsonl`
- `.ddx/exec-runs.d/exec-metric-startup-time@2026-04-04T15:01:00Z/result.json`
- `cli/internal/exec/testdata/concurrent-runs/`

## Exit Criteria

- Metric tests pass deterministically under `go test`.
- Concurrent writes do not corrupt generic execution metadata collections or
  attachment references.
- Validation and run errors name the metric artifact, execution definition, and
  concrete collection or attachment path involved.
- Metric history and trend remain projections over generic execution runs and
  do not depend on HELIX stage semantics.
