# Production Reachability Decisions — ddx-97375097

Run-id: 20260617T120241-14246f2a

## Files and Symbols Addressed

### Original artifact: `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`

| File | Original Symbol | Current Status | Decision | Rationale |
|------|----------------|----------------|----------|-----------|
| preclaim_intake_hook.go:368 | NewPreClaimIntakeHook | Already live | — | Resolved before this execution |
| preclaim_intake_hook.go:643 | decodePreClaimIntakePayloadResult | Already live | — | Resolved before this execution |
| preclaim_intake_hook.go:862 | resolveReadinessEstimatedDifficulty | Already live | — | Resolved before this execution |
| profile_select.go:72 | WarmProfileSnapshotForProject | Already live | — | Resolved before this execution |
| profile_select.go:255 | SelectCheapestProfile | Already live | — | Resolved before this execution |
| profile_select.go:35 | ResetProfileSnapshotCacheForTesting | WIRE | Called from cross-package tests (cmd/run_test_helpers_test.go); added init() to make reachable from main() per established pattern (same as SetServiceRunFactory) |
| ratelimit_retry.go:73 | EvaluateRateLimitWait | DELETE | Wrapper delegating to ratelimitpolicy; superseded by try package's own EvaluateRateLimitWait; not called from production |
| ratelimit_retry.go:129 | IsRateLimitResult | DELETE | Not called from production; try package uses ratelimitpolicy.IsRateLimitText directly |
| ratelimit_retry.go:160 | hasIsolated429 | DELETE | No longer present in current code (renamed/removed before this execution) |
| ratelimit_retry.go:190 | ParseRetryAfter | DELETE | Wrapper not called from production; try package handles this directly |
| ratelimit_retry.go:227 | ExtractRetryAfterFromStderr | DELETE | Wrapper not called from production |
| ratelimit_retry.go:284 | RateLimitRetryConfig.resolved | DELETE | Only used by RunWithRateLimitRetry which is also deleted |
| ratelimit_retry.go:301 | ctxSleep | DELETE | Only used by resolved() which is also deleted |
| ratelimit_retry.go:324 | RunWithRateLimitRetry | DELETE | Production uses try.Attempt which has its own rate-limit retry loop |
| ratelimit_retry.go:385 | BuildRateLimitRouteAttempt | DELETE | Not called from production; execute-bead loop uses fromTryRateLimitRetryInfo + appendRateLimitRetryEvent |
| recovery_decompose.go:107 | NewDecomposePostLadderExhaustionHook | Already live | — | Resolved before this execution |
| recovery_reframe.go:40 | NewReframePostLadderExhaustionHook | Already live | — | Resolved before this execution |
| repair_prompt.go:24 | BuildRepairPrompt | Already live | — | Resolved before this execution |
| service_run.go:581 | TestProviderConnectivityViaService | Already live | — | Resolved before this execution |
| service_run.go:668 | ValidateEffortForRunViaService | Already live | — | Resolved before this execution |
| service_run.go:55 | ResolveServiceFromWorkDir | DELETE | Non-context variant replaced by ResolveServiceFromWorkDirCtx; only callee was resolveService() which remains used by other callers |
| triage_dispatch.go:25 | IsGitUpdateRefCompareAndSwapFailure | Already live | — | Resolved before this execution |

## Verification

- `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/agent/(preclaim_intake_hook|profile_select|ratelimit_retry|recovery_decompose|recovery_reframe|repair_prompt|service_run|triage_dispatch)\.go'` → no output (AC2 satisfied)
- `go test ./internal/agent/...` → all pass
- Pre-existing test failure: `TestHumanWritingSupportSkillContent` in `internal/skills` — unrelated to this cluster (missing skill file in worktree, present on base revision)
