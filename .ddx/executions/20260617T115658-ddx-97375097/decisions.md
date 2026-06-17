# ddx-97375097 decisions

## DELETE

- `cli/internal/agent/ratelimit_retry.go`: removed obsolete agent-level wrappers for `EvaluateRateLimitWait`, `IsRateLimitResult`, `ParseRetryAfter`, `ExtractRetryAfterFromStderr`, `RateLimitRetryConfig.resolved`, `ctxSleep`, `RunWithRateLimitRetry`, and `BuildRateLimitRouteAttempt`. The live retry path is `cli/internal/agent/try/attempt.go`, backed by `cli/internal/ratelimitpolicy`.
- `cli/internal/agent/ratelimit_retry_test.go`: removed tests that exercised only the deleted wrapper layer.
- `cli/internal/agent/service_run.go`: removed unused `ResolveServiceFromWorkDir`; live production callers use `ResolveServiceFromWorkDirCtx`.

## WIRE

- `cli/internal/agent/profile_select.go`: kept `ResetProfileSnapshotCacheForTesting` because cmd integration tests use it across package boundaries; wired it through package initialization so it is reachable in production builds without changing runtime behavior beyond resetting the empty startup cache.

## PENDING

- None.
