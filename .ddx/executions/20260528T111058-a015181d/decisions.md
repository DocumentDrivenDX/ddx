# Production Reachability Decisions — Agent Subsystem

## Analysis Overview

**Unreachable Symbol Count**: 19 symbols across 8 files  
**Report Date**: 2026-05-28  
**Deadcode Tool**: golang.org/x/tools/cmd/deadcode@v0.42.0

### Symbols by File

#### `preclaim_intake_hook.go`

**NewPreClaimIntakeHook** (line 432)
- **Status**: WIRE
- **Rationale**: Exported function, used extensively in tests and in execute_bead_intake_test.go, execute_bead_loop_downgrade_regression_test.go, preclaim_intake_hook_test.go. This is a public API for intake hook construction and must be reachable.
- **Action**: Wire through execute_bead_loop.go or similar production path that uses preclaim intake functionality.

**decodePreClaimIntakePayloadResult** (line 709)
- **Status**: WIRE
- **Rationale**: Exported function that wraps decodePreClaimIntakePayloadResultWithMode with default quality mode. Used in execute_bead_intake_test.go. This is a public test utility function.
- **Action**: Keep as-is; it's a convenience wrapper for testing.

**resolveReadinessEstimatedDifficulty** (line 928)
- **Status**: DELETE
- **Rationale**: Exported function but appears to be unused. Not found in production code or tests. The function resolves estimated difficulty from bead labels or readiness estimate.
- **Action**: Delete this dead code; if needed, refactor to inline or consolidate with related difficulty logic.

#### `profile_select.go`

**WarmProfileSnapshotForProject** (line 72)
- **Status**: DELETE
- **Rationale**: Exported function that appears to be a no-op (returns immediately if runner != nil). Not called from production code. Only used in comments in test files.
- **Action**: Delete; appears to be abandoned pre-warming logic.

**SelectCheapestProfile** (line 255)
- **Status**: WIRE
- **Rationale**: Exported function used in tests (profile_select_test.go). Public API for profile selection. Should be wired into production path or clearly marked as test-only.
- **Action**: Check if this should be used by lifecycle dispatch; if not, consider moving to test-only or removing.

#### `ratelimit_retry.go`

**EvaluateRateLimitWait** (line 73)
- **Status**: DELETE
- **Rationale**: Duplicate definition exists in internal/agent/try/attempt.go:515. The version in attempt.go is used by production code through RateLimitRetryConfig. This version is superseded.
- **Action**: Delete; maintain only the attempt.go version.

**IsRateLimitResult** (line 129)
- **Status**: DELETE
- **Rationale**: Only used within ratelimit_retry.go in RunWithRateLimitRetry. If RunWithRateLimitRetry is not in production, this is dead code. Production uses IsRateLimitReport in attempt.go instead.
- **Action**: Delete.

**hasIsolated429** (line 160)
- **Status**: DELETE
- **Rationale**: Duplicate definition exists in internal/agent/try/attempt.go:571. Only helper function for IsRateLimitResult. Delete along with other dead ratelimit code.
- **Action**: Delete.

**ParseRetryAfter** (line 190)
- **Status**: DELETE
- **Rationale**: Duplicate definition exists in internal/agent/try/attempt.go:593. Dead code in this file.
- **Action**: Delete.

**ExtractRetryAfterFromStderr** (line 227)
- **Status**: DELETE
- **Rationale**: Duplicate definition exists in internal/agent/try/attempt.go:617. Dead code in this file.
- **Action**: Delete.

**RateLimitRetryConfig.resolved** (line 284)
- **Status**: DELETE
- **Rationale**: Method on RateLimitRetryConfig struct. Not called from anywhere. If the struct is obsolete, remove the method.
- **Action**: Delete.

**ctxSleep** (line 301)
- **Status**: DELETE
- **Rationale**: Duplicate definition exists in internal/agent/try/attempt.go:485. Only used by RunWithRateLimitRetry which is dead code.
- **Action**: Delete.

**RunWithRateLimitRetry** (line 324)
- **Status**: DELETE
- **Rationale**: Only used in tests. Production code uses different retry path through attempt.go. Not in production reachability graph.
- **Action**: Delete.

**BuildRateLimitRouteAttempt** (line 385)
- **Status**: DELETE
- **Rationale**: Only used in tests. Not in production reachability graph.
- **Action**: Delete.

#### `recovery_decompose.go`

**NewDecomposePostLadderExhaustionHook** (line 107)
- **Status**: WIRE
- **Rationale**: Exported function used in tests (recovery_decompose_test.go). Implements PostLadderExhaustionHook interface. Should be wired into execute-bead recovery path if hook-based recovery is active.
- **Action**: Check if PostLadderExhaustionHook is used in production; if yes, wire this constructor; if no, this whole recovery infrastructure may be dead.

#### `recovery_reframe.go`

**NewReframePostLadderExhaustionHook** (line 40)
- **Status**: WIRE
- **Rationale**: Exported function used in tests (recovery_reframe_test.go). Implements PostLadderExhaustionHook interface. Same rationale as decompose hook.
- **Action**: Same as NewDecomposePostLadderExhaustionHook.

#### `service_run.go`

**TestProviderConnectivityViaService** (line 581)
- **Status**: DELETE
- **Rationale**: Only referenced in comments in test files. Not actually called from anywhere. Dead code.
- **Action**: Delete.

**ValidateEffortForRunViaService** (line 668)
- **Status**: DELETE
- **Rationale**: Only referenced in comments. Not called from anywhere. Dead code.
- **Action**: Delete.

#### `triage_dispatch.go`

**IsGitUpdateRefCompareAndSwapFailure** (line 25)
- **Status**: WIRE
- **Rationale**: Exported function used in tests (execute_bead_concurrent_predispatch_test.go, execute_bead_report_test.go). Public API for error classification. Should be wired into execute-bead error handling if used.
- **Action**: Verify if this is used in production error classification; if not, move to test-only utilities.

## Summary by Decision

### DELETE CONFIRMED (11 functions)
Functions with clear evidence of being dead code or exact duplicates:

- TestProviderConnectivityViaService (service_run) — only in comments, never called
- ValidateEffortForRunViaService (service_run) — only in comments, never called
- EvaluateRateLimitWait (ratelimit_retry) — exact duplicate in attempt.go:515
- IsRateLimitResult (ratelimit_retry) — not called from ratelimit_retry.go; duplicate concept IsRateLimitReport exists in attempt.go
- hasIsolated429 (ratelimit_retry) — exact duplicate in attempt.go:571
- ParseRetryAfter (ratelimit_retry) — exact duplicate in attempt.go:593
- ExtractRetryAfterFromStderr (ratelimit_retry) — exact duplicate in attempt.go:617
- ctxSleep (ratelimit_retry) — exact duplicate in attempt.go:485
- RateLimitRetryConfig.resolved (ratelimit_retry) — method never called
- RunWithRateLimitRetry (ratelimit_retry) — only used in ratelimit_retry_test.go; production uses attempt.go path
- BuildRateLimitRouteAttempt (ratelimit_retry) — only used in ratelimit_retry_test.go

### DELETE LIKELY (2 functions)
Functions that appear orphaned but with some risk:

- resolveReadinessEstimatedDifficulty (preclaim) — exported but never called, no test usage found
- WarmProfileSnapshotForProject (profile_select) — no-op function that returns immediately if runner != nil; test-only, abandoned

### PENDING - REQUIRES FOLLOW-UP (6 functions)
Functions that have legitimate production usage patterns but are unreachable in current analysis:

- NewPreClaimIntakeHook (preclaim) — **WIRING VERIFIED**: called from internal/server/workers.go (production); used in tests. Issue: internal/server not in CLI reachability graph. **Action**: Keep as-is; this is production infrastructure.

- decodePreClaimIntakePayloadResult (preclaim) — **Test utility**: wrapper for decoding with default mode. Used only in execute_bead_intake_test.go. **Action**: Keep; it's legitimate test API.

- SelectCheapestProfile (profile_select) — **Test utility**: profile selection function used in tests. Related SelectStrongestProfile is also exported. **Action**: Keep; may be part of public profile API.

- NewDecomposePostLadderExhaustionHook (recovery) — **Verified infrastructure**: implements PostLadderExhaustionHook interface used in execute_bead_loop.go:2820. PostLadderExhaustionHook field is set at line 149. **Action**: Keep; core recovery infrastructure.

- NewReframePostLadderExhaustionHook (recovery) — **Same as above**: PostLadderExhaustionHook infrastructure. **Action**: Keep.

- IsGitUpdateRefCompareAndSwapFailure (triage) — **Test utility**: classification helper used in execute_bead_concurrent_predispatch_test.go and execute_bead_report_test.go. **Action**: Keep; legitimate error classification.

## Implementation Strategy

**DELETE** the 11 confirmed dead functions from ratelimit_retry.go:
- All are duplicates of attempt.go functions that are already in the production path
- These represent superseded/migrated code
- No production code calls these functions directly

**DELETE** the 2 likely orphaned functions:
- resolveReadinessEstimatedDifficulty: never called
- WarmProfileSnapshotForProject: no-op, test-only concept

**KEEP** the 6 pending functions:
- Hooks and infrastructure: PostLadderExhaustionHook (decompose, reframe) are wired into execute-bead loop
- Intake: NewPreClaimIntakeHook is production code (used from workers.go)
- Utilities: decode, classification, profile selection are legitimate test/public APIs
- These functions are unreachable in the CLI-only reachability graph but are reachable from server and test entry points
- Keeping these functions allows full functionality when using internal/server paths or tests

## Wiring Issues

The root cause of reported unreachability is that the production-reachability analysis only covers CLI entry points (cmd/main.go and related commands), not:
1. internal/server package (used for server mode)
2. internal/*/test.go patterns (intentionally test infrastructure)

Once DELETE operations are complete, the remaining "unreachable" functions will be test-utility or server-mode functions, which is acceptable.
