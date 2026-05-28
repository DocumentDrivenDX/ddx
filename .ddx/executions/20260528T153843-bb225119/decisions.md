# Decisions Log: Delete Duplicate Rate-Limit Helpers

## Summary

Deleted duplicate rate-limit helpers from `cli/internal/agent/ratelimit_retry.go` that were superseded by implementations in `cli/internal/agent/try/attempt.go`. The try package has become the authoritative source for rate-limit logic, rendering the agent package versions dead code.

## Symbols Deleted

### Functions
- `EvaluateRateLimitWait` — **DELETE** — live equivalent in try/attempt.go:515
- `IsRateLimitResult` — **DELETE** — only used within the dead `RunWithRateLimitRetry` function
- `hasIsolated429` — **DELETE** — internal helper, live equivalent in try/attempt.go
- `ParseRetryAfter` — **DELETE** — live equivalent in try/attempt.go:593
- `ExtractRetryAfterFromStderr` — **DELETE** — live equivalent in try/attempt.go:617
- `RateLimitRetryConfig.resolved` (method) — **DELETE** — method on dead type
- `ctxSleep` — **DELETE** — only used within dead `RunWithRateLimitRetry`
- `RunWithRateLimitRetry` — **DELETE** — replaced by integration in try/attempt.go
- `BuildRateLimitRouteAttempt` — **DELETE** — routing integration moved to try package

### Types
- `RateLimitRetryConfig` — **DELETE** — live equivalent in try/attempt.go:418

### Variables & Constants
- `rateLimitBackoffSchedule` — **DELETE** — only used by dead `EvaluateRateLimitWait`
- `RateLimitRetryDefaultBudget` — **KEEP** — referenced by cmd/work.go:107
- `RateLimitRetryDefaultPerWaitCap` — **DELETE** — no external references, schedule now in try package

### Kept in ratelimit_retry.go
- `RateLimitBudgetExhaustedReason` — **KEEP** — used by execute_bead.go:1409
- `RateLimitRetryEventKind` — **KEEP** — used by execute_bead.go:1412
- `RateLimitRetryInfo` (type) — **KEEP** — used by execute_bead.go and adapter in execute_bead_try_adapter.go
- `RateLimitRetryDefaultBudget` — **KEEP** — used by cmd/work.go:107

## Evidence

### Deadcode Verification
- Before deletion: deadcode reported 9 unreachable symbols in ratelimit_retry.go
- After deletion: `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/agent/ratelimit_retry\.go'` returns no hits ✓

### Test Results
- Try package rate-limit tests pass:
  - `TestAttempt_RateLimit_Retry_HonorsRetryAfter` ✓
  - `TestAttempt_RateLimit_Retry_UsesExponentialBackoff` ✓
  - `TestAttempt_RateLimit_WiresEvaluateRateLimitWait` ✓

### Deleted Test File
- Entire `cli/internal/agent/ratelimit_retry_test.go` deleted (all tests were for dead functions)
  - TestEvaluateRateLimitWait_* (5 variants)
  - TestParseRetryAfter
  - TestExtractRetryAfterFromStderr
  - TestIsRateLimitResult (4 sub-cases)
  - TestRunWithRateLimitRetry_* (7 variants)
  - TestBuildRateLimitRouteAttempt_* (2 variants)

## Architecture Impact

The rate-limit retry contract (TD-031 §4 / §8.4, ddx-c6e3db02) is now implemented entirely within the try package:
- Policy decisions: `try/attempt.go:EvaluateRateLimitWait`
- Detection: `try/attempt.go:IsRateLimitReport` (on Report type, not Result)
- Header parsing: `try/attempt.go:ParseRetryAfter`, `ExtractRetryAfterFromStderr`
- Sleep/timing: `try/attempt.go:ctxSleep`
- Integration: `try/attempt.go:AttemptOpts.RateLimitOnRetry` callback

The agent package retains only the minimal types and constants needed for integration between execute_bead and the try package (via the adapter in `execute_bead_try_adapter.go`).
