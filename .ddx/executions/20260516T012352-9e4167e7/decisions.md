# Decisions

Run: `20260516T012352-9e4167e7`
Bead: `ddx-97375097`

| Symbol | Decision | Evidence |
|---|---|---|
| `internal/agent/preclaim_intake_hook.go:NewPreClaimIntakeHook` | DELETE | Removed the production-only compatibility wrapper; production uses `NewPreClaimIntakeHookWithLog` / `NewPreClaimIntakeHookWithLogVerbose`, while tests keep a `_test.go` helper. |
| `internal/agent/preclaim_intake_hook.go:decodePreClaimIntakePayloadResult` | DELETE | Removed the production-only decoder shim; runtime uses `decodePreClaimIntakePayloadResultWithMode` with configured quality mode, while tests keep a `_test.go` helper. |
| `internal/agent/preclaim_intake_hook.go:resolveReadinessEstimatedDifficulty` | DELETE | Removed the production helper because readiness difficulty is intentionally transient and must not persist bead metadata; the legacy assertion remains as `_test.go` support only. |
| `internal/agent/profile_select.go:WarmProfileSnapshotForProject` | DELETE | Removed the unused warmer; `selectProfileForDispatch` already performs a bounded cold policy load and schedules background refresh from the live worker path. |
| `internal/agent/profile_select.go:SelectCheapestProfile` | DELETE | Removed the superseded cheap-profile helper; production cheap routing uses `SelectImplementationProfile(..., PowerCheap)`, and tests now cover that path directly. |
| `internal/agent/ratelimit_retry.go:EvaluateRateLimitWait` | DELETE | Removed the duplicate top-level retry policy; the live worker path uses `internal/agent/try.Attempt` and its `EvaluateRateLimitWait`. |
| `internal/agent/ratelimit_retry.go:IsRateLimitResult` | DELETE | Removed the duplicate top-level result detector; the live worker path uses `internal/agent/try.IsRateLimitReport`. |
| `internal/agent/ratelimit_retry.go:hasIsolated429` | DELETE | Removed with the duplicate top-level rate-limit detector; the active implementation remains in `internal/agent/try`. |
| `internal/agent/ratelimit_retry.go:ParseRetryAfter` | DELETE | Removed the duplicate top-level parser; the active implementation remains in `internal/agent/try`. |
| `internal/agent/ratelimit_retry.go:ExtractRetryAfterFromStderr` | DELETE | Removed the duplicate top-level extractor; the active implementation remains in `internal/agent/try`. |
| `internal/agent/ratelimit_retry.go:RateLimitRetryConfig.resolved` | DELETE | Removed with the duplicate top-level retry wrapper. |
| `internal/agent/ratelimit_retry.go:ctxSleep` | DELETE | Removed with the duplicate top-level retry wrapper. |
| `internal/agent/ratelimit_retry.go:RunWithRateLimitRetry` | DELETE | Removed the superseded top-level wrapper; the live worker path retries through `internal/agent/try.Attempt`, which reads per-attempt `RateLimitBudget` from executor reports. |
| `internal/agent/ratelimit_retry.go:BuildRateLimitRouteAttempt` | WIRE | `execute_bead_loop.go` now converts `try.RateLimitRetryInfo` and records `rate_limited` route attempts via this helper on retry. |
| `internal/agent/recovery_decompose.go:NewDecomposePostLadderExhaustionHook` | DELETE | Removed the narrower wrapper; production and tests use `NewAutoRecoveryPostLadderExhaustionHook`, which routes `TooLarge` to decomposition. |
| `internal/agent/recovery_reframe.go:NewReframePostLadderExhaustionHook` | DELETE | Removed the narrower wrapper; production and tests use `NewAutoRecoveryPostLadderExhaustionHook`, which routes `SpecGap` and persistent failures to reframe. |
| `internal/agent/repair_prompt.go:BuildRepairPrompt` | WIRE | Already wired from `candidate_cycle.go` for append-only repair prompts; current-tree deadcode no longer reports it. |
| `internal/agent/service_run.go:TestProviderConnectivityViaService` | DELETE | Removed the obsolete service helper; provider health and route failures are handled through the current service run and route-attempt paths. |
| `internal/agent/service_run.go:ValidateEffortForRunViaService` | DELETE | Removed the unused preflight helper; `ddx run` passes effort as an opaque service constraint. |
| `internal/agent/triage_dispatch.go:IsGitUpdateRefCompareAndSwapFailure` | WIRE | `ClassifyFailureMode` now classifies update-ref compare-and-swap races as lock contention. |

No `// wiring:pending` annotations were added.
