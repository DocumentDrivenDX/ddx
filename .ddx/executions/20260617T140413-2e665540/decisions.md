# Decisions: ddx-97375097 — checks: residual production-reachability (internal/agent runtime support)

Run ID: 20260617T140413-2e665540

## Symbols from original artifact (.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json)

Note: Many original symbols were already resolved between 2026-05-15 and this run. The current
deadcode run (2026-06-17) only showed residuals for `profile_select.go`, `ratelimit_retry.go`,
and `service_run.go`.

### preclaim_intake_hook.go (3 symbols in original artifact)
| Symbol | Decision | Rationale |
|--------|----------|-----------|
| `NewPreClaimIntakeHook` (line 368) | ALREADY RESOLVED | Not present in current deadcode output — wired before this run |
| `decodePreClaimIntakePayloadResult` (line 643) | ALREADY RESOLVED | Not present in current deadcode output |
| `resolveReadinessEstimatedDifficulty` (line 862) | ALREADY RESOLVED | Not present in current deadcode output |

### profile_select.go (2 symbols in original artifact, 1 in current run)
| Symbol | Decision | Rationale |
|--------|----------|-----------|
| `WarmProfileSnapshotForProject` (original line 72) | ALREADY RESOLVED | Not present in current deadcode output |
| `SelectCheapestProfile` (original line 255) | ALREADY RESOLVED | Not present in current deadcode output |
| `ResetProfileSnapshotCacheForTesting` (current line 35) | WIRE | Exported test-helper used in `cmd/run_test_helpers_test.go`. Added `init()` that calls it with a no-op nil-reset — same pattern as `SetServiceRunFactory`. Safe: cache is empty at startup. |

### ratelimit_retry.go (9 symbols in current run, all deleted)
| Symbol | Decision | Rationale |
|--------|----------|-----------|
| `EvaluateRateLimitWait` | DELETE | Thin wrapper over `ratelimitpolicy.EvaluateRateLimitWait`. Superseded by `try/attempt.go` which calls `ratelimitpolicy` directly and is wired via `execute_bead_loop.go`. |
| `IsRateLimitResult` | DELETE | Thin wrapper over `ratelimitpolicy.IsRateLimitText`. Superseded by `try.IsRateLimitReport`. |
| `ParseRetryAfter` | DELETE | Thin wrapper over `ratelimitpolicy.ParseRetryAfter`. Not wired in any execution path. |
| `ExtractRetryAfterFromStderr` | DELETE | Thin wrapper over `ratelimitpolicy.ExtractRetryAfterFromStderr`. Superseded by `try/attempt.go`. |
| `RateLimitRetryConfig` (type) + `resolved()` method | DELETE | Only used by `RunWithRateLimitRetry`. The live execution path uses `try.RateLimitRetryConfig` in `execute_bead_loop.go`. |
| `ctxSleep` | DELETE | Only called by `RateLimitRetryConfig.resolved`. |
| `RunWithRateLimitRetry` | DELETE | Superseded by rate-limit retry loop inside `try/attempt.go` (wired in `execute_bead_loop.go:3550`). |
| `BuildRateLimitRouteAttempt` | DELETE | Not called from any production site; transparent routing feedback is achieved via `fromTryRateLimitRetryInfo` + `appendRateLimitRetryEvent`. |
| `hasIsolated429` (internal) | DELETE | Helper for `IsRateLimitResult`. Removed with the parent. |

Kept in `ratelimit_retry.go`: constants (`RateLimitRetryDefaultBudget`, `RateLimitRetryDefaultPerWaitCap`, `RateLimitBudgetExhaustedReason`, `RateLimitRetryEventKind`), type alias (`RateLimitWaitDecision`), and `RateLimitRetryInfo` struct — all used in production by `execute_bead.go`, `execute_bead_try_adapter.go`, `cmd/work.go`, `cmd/agent_execute_loop_escalation.go`.

Tests in `ratelimit_retry_test.go` deleted (all tested deleted functions). The underlying `ratelimitpolicy` behaviors are exercised end-to-end via `try/attempt.go` integration tests.

### recovery_decompose.go (1 symbol in original artifact)
| Symbol | Decision | Rationale |
|--------|----------|-----------|
| `NewDecomposePostLadderExhaustionHook` (line 107) | ALREADY RESOLVED | Not present in current deadcode output |

### recovery_reframe.go (1 symbol in original artifact)
| Symbol | Decision | Rationale |
|--------|----------|-----------|
| `NewReframePostLadderExhaustionHook` (line 40) | ALREADY RESOLVED | Not present in current deadcode output |

### repair_prompt.go (1 symbol in original artifact)
| Symbol | Decision | Rationale |
|--------|----------|-----------|
| `BuildRepairPrompt` (line 24) | ALREADY RESOLVED | Not present in current deadcode output |

### service_run.go (2 symbols in original artifact, 1 in current run)
| Symbol | Decision | Rationale |
|--------|----------|-----------|
| `TestProviderConnectivityViaService` (original line 581) | ALREADY RESOLVED | Not present in current deadcode output |
| `ValidateEffortForRunViaService` (original line 668) | ALREADY RESOLVED | Not present in current deadcode output |
| `ResolveServiceFromWorkDir` (current line 55) | DELETE | Not called from any production or test site. Superseded by `ResolveServiceFromWorkDirCtx` which provides context-scoped lifetime management. |

### triage_dispatch.go (1 symbol in original artifact)
| Symbol | Decision | Rationale |
|--------|----------|-----------|
| `IsGitUpdateRefCompareAndSwapFailure` (line 25) | ALREADY RESOLVED | Not present in current deadcode output |

## No wiring:pending annotations created

All symbols in the current deadcode output were cleanly resolved (WIRE or DELETE). No follow-up beads required.
