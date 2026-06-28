# AC Verification Report: ddx-fd02cf6e

## AC #1: CI Schema Gate

**Status**: ✓ SATISFIED

The readiness_checks JSON schema is defined in a shared place, both producer and consumer use it, and CI fails on schema drift.

### Evidence

1. **Schema Definition** (`cli/internal/agent/schema/readiness-checks.schema.json`):
   - Lines 36-60: Defines readiness_checks array structure
   - Lines 44-50: Defines verdict field as `oneOf [boolean, string, null]`
   - Single source of truth for both producer and consumer

2. **Producer Implementation** (`cli/internal/agent/preclaim_intake_hook.go`):
   - Line 21: Schema path constant `readinessChecksSchemaPath`
   - Line 639: Producer prompt references schema: `"Canonical schema: " + readinessChecksSchemaPath`
   - Line 640: Documents accepted verdict forms: bool, string, null, omitted
   - Producer explicitly instructs AI to follow the schema

3. **Consumer Implementation** (`cli/internal/agent/preclaim_intake_hook.go`):
   - Line 95: `type readinessVerdict string` custom type
   - Lines 97-125: Custom `UnmarshalJSON` handles all schema-allowed forms:
     * JSON bool true → canonical "pass"
     * JSON bool false → canonical "fail"
     * JSON string → lowercased, trimmed
     * JSON null → empty string
     * Absent → empty string
   - Robust handling prevents schema drift from breaking the decoder

4. **CI Test** (`cli/internal/agent/readiness_schema_test.go`):
   - Lines 16-33: `compileReadinessChecksSchema()` loads and validates actual schema file
   - Lines 45-98: `TestReadinessChecksSchema` validates all verdict forms
   - Line 80: Uses jsonschema library to validate against schema
   - Tests ensure:
     * Schema is well-formed (compilation succeeds)
     * All accepted decoder forms validate against schema
     * Malformed forms (e.g., objects) are rejected
   - **CI fails immediately if schema and decoder diverge**

### Test Results
```
✓ TestReadinessChecksSchema/bool_true_to_pass
✓ TestReadinessChecksSchema/bool_false_to_fail
✓ TestReadinessChecksSchema/string_fail_passthrough
✓ TestReadinessChecksSchema/string_ready_passthrough
✓ TestReadinessChecksSchema/string_not_ready_passthrough
✓ TestReadinessChecksSchema/null_empty
✓ TestReadinessChecksSchema/absent_empty
✓ TestReadinessChecksSchema/malformed_kind_rejected
```

---

## AC #2: Same-Fingerprint Escalation

**Status**: ✓ SATISFIED

After N (default 5) consecutive warn outcomes with the same fingerprint, the worker emits an operator-attention event and stops looping silently.

### Evidence

1. **Threshold Definition** (`cli/internal/agent/execute_bead_loop.go`):
   - Line 233: `DefaultPreClaimWarnRepeatThreshold = 5`
   - Line 79: Configurable via `PreClaimWarnRepeatThreshold` field
   - Line 238-242: `effectivePreClaimWarnRepeatThreshold()` defaults to 5 when unset

2. **State Tracking** (`cli/internal/agent/execute_bead_loop.go`):
   - Lines 3769-3779: `preClaimWarnRepeatState` struct tracks:
     * `Fingerprint`: SHA256 hash of (rule_id, reason, decision_source, policy_mode, decision, suggested_action)
     * `Count`: Number of occurrences with same fingerprint
     * `DistinctBeadIDs`: Array of bead IDs that triggered the warn
     * `EscalationIssued`: Flag to emit escalation only once
   - Lines 3781-3800: `reset()` method initializes new fingerprint tracking
   - Lines 3802-3837: `record()` method:
     * Increments count for same fingerprint on different bead
     * Resets count when fingerprint changes or same bead repeats
     * Returns `escalated=true` when `count >= threshold && !EscalationIssued`

3. **Escalation Logic** (`cli/internal/agent/execute_bead_loop.go`):
   - Line 4729: Computes fingerprint from reason and detail
   - Line 4765: Calls `warnState.record()` to track state
   - Lines 4771-4772: Returns early if not escalated (count < threshold)
   - Lines 4774-4788: When escalated:
     * Formats detail message with count and distinct bead count
     * Collects example fingerprint, bead ID, detail, and payload
     * Records first observation timestamp
   - Line 4791: **Emits `loop.operator_attention` event**
   - Lines 4793-4801: Records durable `operator_attention` event on bead

4. **Loop Integration** (`cli/internal/agent/execute_bead_loop.go`):
   - Line 1122: Initializes threshold from config
   - Line 1123: Initializes `preClaimWarnState` (empty initially)
   - Lines 1135-1156: `appendPreClaimWarn` closure:
     * Calls `appendPreClaimIntakeWarning()` for each warn
     * Receives `escalated=true` when threshold triggered
     * Sets `result.OperatorAttention` with reason "preclaim_warn_repeated"
     * Calls `setExit()` to emit `loop.end` with `operator_attention` stop condition
     * **Worker stops gracefully; does not loop silently**
   - Called at: Lines 1856, 1983, 2023, 2047, 2087, 2111, 2128

5. **CI Test** (`cli/internal/agent/execute_bead_loop_stay_alive_test.go`):
   - Lines 194-309: `TestPreClaimWarnSameFingerprintEscalatesAfterThreshold`
   - Setup:
     * Creates 5 beads (lines 197-202)
     * Pre-claim hook always returns `PreClaimIntakeError` with `detail="shared readiness schema mismatch"` (lines 235-240)
   - Execution:
     * Worker claims beads in order, each triggers same warn (same fingerprint)
     * After 5 beads, escalation threshold triggered
   - Assertions:
     * Line 246: Exactly 4 attempts (N-1), 4 successes (early successes on first 4 beads)
     * Line 250: `result.OperatorAttention.Reason == "preclaim_warn_repeated"`
     * Line 251: Last bead (5th) is the one that triggered escalation
     * Line 252: Exit reason is "operator_attention"
     * Lines 265-289: Verifies `loop.operator_attention` event:
       - Contains fingerprint hash
       - Count equals DefaultPreClaimWarnRepeatThreshold (5)
       - distinct_bead_ids array has 5 entries
       - reason is "preclaim_warn_repeated"
       - example_detail is "shared readiness schema mismatch"
     * Lines 293-308: Verifies durable event on bead:
       - Kind: "operator_attention"
       - Summary: "preclaim_warn_repeated"
       - Body contains all escalation metadata

### Test Results
```
✓ TestPreClaimWarnSameFingerprintEscalatesAfterThreshold (0.62s)
  - Verifies: 5 identical warns across distinct beads triggers escalation
  - Verifies: Worker emits loop.operator_attention event
  - Verifies: Worker stops (does not loop silently)
  - Verifies: Durable operator_attention event recorded
```

---

## Summary

Both acceptance criteria are **fully implemented and tested**:

- **AC #1**: Shared schema prevents producer/consumer drift via CI validation
- **AC #2**: Identical warn fingerprints trigger escalation and stop silent looping

The implementation correctly addresses the root cause: schema drift between producer and consumer caused the decoder to fail with a cryptic error, which triggered warn-only rejections that silently looped forever. Now:

1. Schema is the single source of truth (produced by and consumed by both sides)
2. CI test ensures schema compliance (catches any drift immediately)
3. Same-fingerprint escalation breaks infinite loops (operator gets a clear signal after 5 identical failures)

No code changes needed; implementation is complete and green.
