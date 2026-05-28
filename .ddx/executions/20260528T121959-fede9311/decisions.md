# Decisions Log for ddx-f71ef348

## Summary

Deleted dead rate-limit helper functions from `cli/internal/agent/ratelimit_retry.go` per production reachability analysis. Kept constants and type definitions needed by active code paths.

## Per-Symbol Analysis

### Deleted Functions

| Symbol | Status | Rationale |
|--------|--------|-----------|
| EvaluateRateLimitWait | DELETE | Unreachable; try/attempt.go provides the implementation |
| IsRateLimitResult | DELETE | Unreachable; functionality moved to try package |
| hasIsolated429 | DELETE | Unreachable; helper for IsRateLimitResult |
| ParseRetryAfter | DELETE | Unreachable; logic duplicated in try/attempt.go |
| ExtractRetryAfterFromStderr | DELETE | Unreachable; helper moved to try package |
| ctxSleep | DELETE | Unreachable; replaced by try package sleep logic |
| RunWithRateLimitRetry | DELETE | Unreachable; try/attempt.go Attempt type provides this |
| BuildRateLimitRouteAttempt | DELETE | Unreachable; no callers in production graph |
| RateLimitRetryConfig.resolved | DELETE | Unreachable method; helper for RunWithRateLimitRetry |

### Deleted Tests

Entire file `cli/internal/agent/ratelimit_retry_test.go` removed (432 lines). All test cases were for dead functions:
- TestEvaluateRateLimitWait_* (8 tests)
- TestParseRetryAfter (1 test)
- TestExtractRetryAfterFromStderr (1 test)
- TestIsRateLimitResult (1 test)
- TestRunWithRateLimitRetry_* (6 tests)
- TestBuildRateLimitRouteAttempt_* (2 tests)

### Retained Constants & Types

| Symbol | Status | Rationale |
|--------|--------|-----------|
| RateLimitRetryDefaultBudget | KEEP | Used by cmd/work_test.go:236 |
| RateLimitRetryDefaultPerWaitCap | KEEP | Configuration constant |
| RateLimitBudgetExhaustedReason | KEEP | Used by execute_bead.go:1409, workers.go:635 |
| RateLimitRetryEventKind | KEEP | Used by execute_bead.go:1412 for bead events |
| RateLimitRetryConfig | KEEP | Type definition (unused fields removed) |
| RateLimitRetryInfo | KEEP | Used by execute_bead.go:1381, execute_bead_try_adapter.go:113 |

## Live Callers

- `execute_bead.go:1381`: appendRateLimitRetryEvent(info RateLimitRetryInfo) — active
- `execute_bead.go:1409`: RateLimitBudgetExhaustedReason constant
- `execute_bead.go:1412`: RateLimitRetryEventKind constant  
- `execute_bead_try_adapter.go:113`: Conversion function fromTryRateLimitRetryInfo
- `cmd/work_test.go:236`: RateLimitRetryDefaultBudget constant
- `internal/server/workers.go:635`: RateLimitBudgetExhaustedReason constant

## Verification

✓ Deadcode checker: no hits for ratelimit_retry symbols  
✓ Try-package tests: TestAttempt_RateLimit_* pass  
✓ Full test suite: go test ./... passes  
✓ Lefthook pre-commit: passes without errors
