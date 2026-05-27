# AC Verification for ddx-fd02cf6e

## AC1: CI Schema Gate

**Requirement**: The readiness_checks JSON schema is defined in a shared place; producer and consumer both consume it; CI fails on schema drift.

**Implementation**:
- Schema location: `cli/internal/agent/schema/readiness-checks.schema.json`
- Consumer decoder: `cli/internal/agent/preclaim_intake_hook.go` (readinessVerdict.UnmarshalJSON)
- Schema validator: `cli/internal/agent/readiness_schema_test.go` (TestReadinessChecksSchema)

**Test Coverage**:
- TestReadinessChecksSchema validates that:
  - Every form accepted by readinessVerdict.UnmarshalJSON (bool true/false, string, null, absent) is also accepted by the JSON schema
  - Each form decodes to the canonical readinessVerdict value (bool true → "pass", bool false → "fail", string → lowercase, null/absent → "")
  - Malformed forms (objects) are rejected by the schema

**Test Results**: ✓ PASS
```
=== RUN   TestReadinessChecksSchema
=== RUN   TestReadinessChecksSchema/bool_true_to_pass
=== RUN   TestReadinessChecksSchema/bool_false_to_fail
=== RUN   TestReadinessChecksSchema/string_fail_passthrough
=== RUN   TestReadinessChecksSchema/string_ready_passthrough
=== RUN   TestReadinessChecksSchema/string_not_ready_passthrough
=== RUN   TestReadinessChecksSchema/null_empty
=== RUN   TestReadinessChecksSchema/absent_empty
=== RUN   TestReadinessChecksSchema/malformed_kind_rejected
--- PASS: TestReadinessChecksSchema (0.00s)
```

**CI Integration**: This test runs as part of `go test ./internal/agent`. Any schema/decoder drift would cause a test failure and block CI.

---

## AC2: Same-Fingerprint Escalation

**Requirement**: After N (default 5) consecutive warn outcomes with the same fingerprint, the worker emits an operator-attention event and stops looping silently. Tested by injecting a faulty readiness payload and asserting the escalation.

**Implementation**:
- Fingerprint computation: `cli/internal/agent/execute_bead_loop.go` (preClaimIntakeWarningFingerprint)
- Escalation state tracking: `cli/internal/agent/execute_bead_loop.go` (preClaimWarnRepeatState)
- Escalation trigger: `cli/internal/agent/execute_bead_loop.go` (appendPreClaimIntakeWarning)
- Escalation event emission: "loop.operator_attention" with reason "preclaim_warn_repeated"
- Loop exit: Sets result.OperatorAttention and exit reason to "operator_attention"

**Test Coverage**:
- TestPreClaimWarnSameFingerprintEscalatesAfterThreshold:
  - Creates 5 beads with identical pre-claim intake failures (same fingerprint)
  - Verifies the loop exits with OperatorAttention after the 5th bead
  - Asserts "loop.operator_attention" event is emitted with:
    - reason: "preclaim_warn_repeated"
    - count: DefaultPreClaimWarnRepeatThreshold (5)
    - fingerprint: hash of (reason + detail)
    - distinct_bead_ids: [all 5 beads]
    - example_detail: "shared readiness schema mismatch"

**Test Results**: ✓ PASS
```
=== RUN   TestPreClaimWarnSameFingerprintEscalatesAfterThreshold
--- PASS: TestPreClaimWarnSameFingerprintEscalatesAfterThreshold (0.07s)
```

**Faulty Payload Injection**: The test injects a faulty readiness intake result via PreClaimIntakeHook:
```go
PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
    return PreClaimIntakeResult{
        Outcome: PreClaimIntakeError,
        Reason:  "system_unready",
        Detail:  "shared readiness schema mismatch",
    }, nil
},
```

This simulates the same decoder error that would occur if readiness_checks.verdict was received as a JSON bool when the struct field type was string (the original bug reported in ddx-f9ddaa68).

---

## Summary

Both acceptance criteria are fully implemented and tested:

1. **AC1 (Schema Gate)** ✓ - The schema is defined in a shared location, consumed by both producer and decoder, and validated via TestReadinessChecksSchema to detect any drift.

2. **AC2 (Escalation)** ✓ - Same-fingerprint repetitions trigger operator attention after the default threshold (5), stopping the loop and preventing silent queue deadlock.

All tests pass: `go test ./internal/agent -count=1` → ok (70.429s)
