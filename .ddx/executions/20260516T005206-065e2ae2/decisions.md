# ddx-97375097 Decisions

Source artifact: `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`

| Symbol | Decision | Evidence |
|---|---|---|
| `NewPreClaimIntakeHook` | DELETE | Removed the unused production no-log wrapper from `cli/internal/agent/preclaim_intake_hook.go`; production continues to construct intake hooks through `NewPreClaimIntakeHookWithLogVerbose` in `cli/cmd/execute_loop_shared.go` and `NewPreClaimIntakeHookWithLog` in `cli/internal/server/workers.go`. |
| `decodePreClaimIntakePayloadResult` | DELETE | Removed the unused production warn-only wrapper; the live hook decodes through `decodePreClaimIntakePayloadResultWithMode` with the resolved bead-quality mode. |
| `resolveReadinessEstimatedDifficulty` | DELETE | Removed the obsolete resolver; readiness payload decoding already carries normalized `EstimatedDifficulty` directly on `PreClaimIntakeResult`. |
| `WarmProfileSnapshotForProject` | DELETE | Removed the unused warmup helper; `selectProfileForDispatch` owns cold loading and async refresh for lifecycle profile selection. |
| `SelectCheapestProfile` | DELETE | Removed the unused cheap selector; production implementation routing uses `SelectImplementationProfile` / `SelectImplementationProfileForMinPower`, while lifecycle hooks use standard or strongest selectors. |
| `EvaluateRateLimitWait` | DELETE | Removed the obsolete top-level helper; live retry behavior is implemented in `cli/internal/agent/try/attempt.go`. |
| `IsRateLimitResult` | DELETE | Removed the obsolete top-level `Result` detector; live retry behavior uses `IsRateLimitReport` in `cli/internal/agent/try/attempt.go`. |
| `hasIsolated429` | DELETE | Removed with the obsolete top-level rate-limit detector. |
| `ParseRetryAfter` | DELETE | Removed the obsolete top-level parser; live retry behavior uses the parser in `cli/internal/agent/try/attempt.go`. |
| `ExtractRetryAfterFromStderr` | DELETE | Removed the obsolete top-level extractor; live retry behavior uses the extractor in `cli/internal/agent/try/attempt.go`. |
| `RateLimitRetryConfig.resolved` | DELETE | Removed with the obsolete top-level retry wrapper. |
| `ctxSleep` | DELETE | Removed with the obsolete top-level retry wrapper. |
| `RunWithRateLimitRetry` | DELETE | Removed the obsolete top-level retry wrapper; the execute loop runs through `agenttry.Attempt`, which owns rate-limit retries. |
| `BuildRateLimitRouteAttempt` | DELETE | Removed the obsolete route-attempt builder; no production caller records route attempts through this helper, and retry decisions live in `cli/internal/agent/try/attempt.go`. |
| `NewDecomposePostLadderExhaustionHook` | DELETE | Removed the old single-purpose wrapper; production auto recovery uses `NewAutoRecoveryPostLadderExhaustionHook`, which routes `TooLarge` to `runDecomposer`. |
| `NewReframePostLadderExhaustionHook` | DELETE | Removed the old single-purpose wrapper; production auto recovery uses `NewAutoRecoveryPostLadderExhaustionHook`, which routes `SpecGap` and persistent execution failures through `runReframer`. |
| `BuildRepairPrompt` | WIRE | Already reachable from production candidate repair flow via `cli/internal/agent/candidate_cycle.go`. No code change required for this symbol. |
| `TestProviderConnectivityViaService` | DELETE | Removed the unused legacy connectivity helper; current service execution flows through `RunWithConfigViaService` and execute-loop route preflight where needed. |
| `ValidateEffortForRunViaService` | DELETE | Removed the unused legacy effort prevalidator; `ddx run` intentionally passes effort through opaquely to Fizeau. |
| `IsGitUpdateRefCompareAndSwapFailure` | DELETE | Removed the test-only CAS recognizer from production; live triage keeps the broader `IsLockContentionError` classifier. |

No `// wiring:pending` annotations were added.
