# Production Reachability Wiring Decisions

## Summary
Wired pre-claim intake and recovery hooks into the production reachability graph. All symbols from the ddx-83440482 closure list are now either reachable from production entry points or documented as pending alternative implementations.

## Decisions by Symbol

### NewPreClaimIntakeHook - WIRE ✓
- **Status**: Wired into production
- **Change**: Modified `cli/cmd/execute_loop_shared.go` line 295 to call `NewPreClaimIntakeHook` instead of `NewPreClaimIntakeHookWithLogVerbose`
- **Rationale**: Public API wrapper for the pre-claim intake hook. Changed execution loop to use the public constructor, making the full wrapper chain reachable from production entry points.
- **Verification**: Test `TestPreClaimIntakeHook_DispatchesWithStandardProfileNoStrongPowerTrick` passes

### decodePreClaimIntakePayloadResult - DELETE ✓
- **Status**: Deleted
- **Change**: Removed test-only wrapper function from `cli/internal/agent/preclaim_intake_hook.go`
- **Rationale**: Pure test helper that wrapped `decodePreClaimIntakePayloadResultWithMode` with a default quality mode. No production role.
- **Test Updates**: Updated all test files to call `decodePreClaimIntakePayloadResultWithMode(..., config.BeadQualityModeWarnOnly)` directly:
  - `bead_lifecycle_skill_contract_test.go`
  - `preclaim_intake_hook_test.go`
  - `readiness_classification_test.go`
  - `execute_bead_intake_test.go`
- **Verification**: All updated tests pass

### resolveReadinessEstimatedDifficulty - DELETE ✓
- **Status**: Deleted
- **Change**: Removed test-only helper from `cli/internal/agent/preclaim_intake_hook.go`
- **Rationale**: Pure test utility function used only in `TestReadinessUsesBeadDifficultyPrecedence`. Not called from production code.
- **Test Updates**: Deleted the test `TestReadinessUsesBeadDifficultyPrecedence` which only verified the deleted helper function
- **Verification**: Removed unused `escalation` import from `readiness_classification_test.go`

### NewDecomposePostLadderExhaustionHook - PENDING ⏳
- **Status**: Annotated with `// wiring:pending routing redesign`
- **Reason**: Alternative recovery strategy for TooLarge failure class. Production uses `NewAutoRecoveryPostLadderExhaustionHook` (which calls both reframe and decompose). This individual hook is used in unit tests (`recovery_decompose_test.go`) to verify decompose-only recovery in isolation.
- **Decision**: Keep as public API alternative implementation. Not used in production by design; tests verify single-strategy behavior.
- **Future Work**: Routing redesign (FEAT-020 era) may provide configuration to select individual recovery strategies over auto-recovery.

### NewReframePostLadderExhaustionHook - PENDING ⏳
- **Status**: Annotated with `// wiring:pending routing redesign`
- **Reason**: Alternative recovery strategy for SpecGap and PersistentExecutionFailed failure classes. Production uses `NewAutoRecoveryPostLadderExhaustionHook`. This individual hook is used in unit tests (`recovery_reframe_test.go`) to verify reframe-only recovery in isolation.
- **Decision**: Keep as public API alternative implementation. Not used in production by design; tests verify single-strategy behavior.
- **Future Work**: Routing redesign may provide configuration to select individual recovery strategies.

## Build & Test Status
- ✓ Specific tests pass: `TestPreClaimIntakeHook_DispatchesWithStandardProfileNoStrongPowerTrick`, `TestPostLadderExhaustion_TriggersDecompose_ReviewTooLarge`, `TestPostLadderExhaustion_TriggersReframe`
- ✓ Full test suite passes: `go test ./internal/agent/`
- ✓ Pre-commit checks pass: `lefthook run pre-commit`
- ✓ Deadcode verification: No new unreachable symbols introduced

## Architecture Notes
The two pending hooks represent alternative recovery strategies that could be used if the system design shifts from the current "try reframe, then decompose" fallback approach to configuration-based strategy selection. They're kept as public API because:
1. They're genuinely useful for testing individual strategies in isolation
2. They document the available recovery primitives
3. They're exported functions, part of the package's public contract
3. Future routing redesigns may enable configurable per-bead recovery strategies
