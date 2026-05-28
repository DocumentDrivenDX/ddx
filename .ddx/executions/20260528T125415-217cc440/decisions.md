# Production Reachability Wiring Decisions

## Summary

Wired the production reachability graph to eliminate dead code in the preclaim intake and recovery hooks cluster. All targeted symbols are now either reachable from production entry points or deleted as test-only helpers.

## Decision Log

### cli/internal/agent/preclaim_intake_hook.go

#### NewPreClaimIntakeHook (line 432) — DELETE

**Decision**: DELETE

**Rationale**: Pure wrapper function that just delegated to `NewPreClaimIntakeHookWithLog`. No callers in production code. All production paths call `NewPreClaimIntakeHookWithLogVerbose` (cmd/execute_loop_shared.go:295, internal/server/workers.go:746).

**Action**: Deleted function definition at lines 422-434.

#### decodePreClaimIntakePayloadResult (line 709) — DELETE

**Decision**: DELETE

**Rationale**: Test-only helper function that wrapped `decodePreClaimIntakePayloadResultWithMode` with a default quality mode. Used only in test files, never in production code. Per bead guidance, pure test wrappers should be deleted and assertions folded into caller tests.

**Action**: Deleted function definition at lines 706-711. Updated all test callers (4 test files: bead_lifecycle_skill_contract_test.go, execute_bead_intake_test.go, preclaim_intake_hook_test.go, readiness_classification_test.go) to call `decodePreClaimIntakePayloadResultWithMode` directly with `config.BeadQualityModeWarnOnly`.

#### resolveReadinessEstimatedDifficulty (line 928) — DELETE

**Decision**: DELETE

**Rationale**: Test-only helper with no production callers. Used only in readiness_classification_test.go:233,235. Per bead guidance, deleted and inlined the logic into the caller test.

**Action**: Deleted function definition at lines 928-941. Updated `TestReadinessUsesBeadDifficultyPrecedence` to call `escalation.BeadEstimatedDifficulty` directly.

### cli/internal/agent/recovery_decompose.go

#### NewDecomposePostLadderExhaustionHook (line 107) — DELETE

**Decision**: DELETE

**Rationale**: Never called in production code. No production paths create individual hook factories; instead, `NewAutoRecoveryPostLadderExhaustionHook` in recovery_hook.go handles the orchestration of both decompose and reframe paths. This function was redundant with the unified auto-recovery hook approach.

**Action**: Deleted function definition at lines 105-128. Updated recovery_decompose_test.go:71 to inline the hook closure directly in the test using `runDecomposer`.

### cli/internal/agent/recovery_reframe.go

#### NewReframePostLadderExhaustionHook (line 40) — DELETE

**Decision**: DELETE

**Rationale**: Never called in production code. Like the decompose hook constructor, this was a single-purpose wrapper that is superseded by `NewAutoRecoveryPostLadderExhaustionHook`. The unified auto-recovery hook is the production path.

**Action**: Deleted function definition at lines 37-63. Updated recovery_reframe_test.go:67 to inline the hook closure directly in the test using `runReframer`.

## Verification

All targeted symbols now pass the reachability check:
```
go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | grep "internal/agent/(preclaim_intake_hook|recovery_decompose|recovery_reframe)"
```
Returns zero hits, confirming all symbols are either reachable or deleted.

## Production Entry Points Verified

- **Preclaim intake**: cmd/execute_loop_shared.go:295 → `NewPreClaimIntakeHookWithLogVerbose` → (production path)
- **Preclaim intake (server)**: internal/server/workers.go:746 → `NewPreClaimIntakeHookWithLog` → (production path)
- **Recovery hooks**: cmd/execute_loop_shared.go:308 → `NewAutoRecoveryPostLadderExhaustionHook` → routes to `runReframer` and `runDecomposer`

The production code does not use the deleted hook constructors; it uses the unified orchestrator approach via `NewAutoRecoveryPostLadderExhaustionHook`.
