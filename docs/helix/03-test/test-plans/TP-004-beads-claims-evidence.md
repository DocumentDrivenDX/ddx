---
ddx:
  id: TP-004
  depends_on:
    - FEAT-004
    - SD-004
    - TD-004
---
# Test Plan: Bead Claims and Execution Evidence

## Scope

Validate the assignee-aware claim contract and the append-only execution
evidence trail stored in `Extra["events"]` for `ddx bead`.

## Test Cases

### Claim Behavior

- `TestBeadClaimUsesExplicitAssignee`
- `TestBeadClaimFallsBackToCallerIdentity`
- `TestBeadClaimFallsBackToDefaultIdentity`
- `TestBeadUnclaimClearsClaimMetadata`
- `TestBeadClaimDoesNotDropUnknownFields`

### Evidence Behavior

- `TestBeadEvidenceAppendPreservesOrder`
- `TestBeadEvidenceAppendStoresActorAndTimestamp`
- `TestBeadEvidenceAppendAtomicWithConcurrentWriters`
- `TestBeadShowJSONIncludesEvidenceHistory`
- `TestBeadListIgnoresEvidenceForQueueSemantics`

### Compatibility

- `TestLegacyNotesRoundTripsWithEvents`
- `TestImportPreservesEventsAndUnknownFields`

### Fixtures

- `cli/internal/bead/testdata/legacy-notes.jsonl`
- `cli/internal/bead/testdata/evidence-history.jsonl`
- `cli/internal/bead/testdata/concurrent-append.jsonl`

## Exit Criteria

- All claim and evidence tests pass in `go test ./internal/bead ./cmd`.
- Concurrent append tests leave valid JSONL and preserve ordering.
- `show --json` and the server payloads include `Extra["events"]` without
  flattening or dropping entries.

## Notes

There is no separate automated `docs/helix/03-test` runtime layer in the
repository today, so this plan documents the intended test surface directly in
the artifact stack for traceability.
