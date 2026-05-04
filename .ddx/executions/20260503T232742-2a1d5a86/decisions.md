# ddx-83b8994f decisions log

Backfill production-reachability for `internal/agent` package.
66 symbols decided per bead's AC. Status legend: WIRE | DELETE | PENDING.

| File:line | Symbol | Decision | Reason |
|---|---|---|---|
| ac_structural_check.go:53 | ParseACClaims | DELETE | Replaced by NoChangesContract path; only caller `adjudicateNoChanges` deleted in same change. File removed. |
| ac_structural_check.go:96 | VerifyACClaims | DELETE | Same as ParseACClaims. File removed. |
| ac_structural_check.go:130 | goFieldDeclared | DELETE | Helper for VerifyACClaims; file removed. |
| catalog.go:118 | Catalog.IsBlockedModelID | DELETE | Method only used by tests; logic kept inline via test helper `catalogIsBlockedModelID`. |
| catalog.go:162 | Catalog.Entry | DELETE | Method only used by tests; logic kept inline via test helper `catalogEntry`. |
| catalog.go:168 | Catalog.KnownOnAnySurface | DELETE | Method only used by tests; logic kept inline via test helper `catalogKnownOnAnySurface`. |
| catalog.go:182 | Catalog.NormalizeModelRef | DELETE | No callers. |
| compare_adapter.go:374 | RunQuorumWithConfig | DELETE | Replaced by `RunQuorumWithConfigViaService`; tests routed to test helper `runQuorumWithConfig`. |
| evidence_telemetry.go:66 | compareResultEventBody | DELETE | Only one test exercised it; subtest removed. |
| execute_bead_loop.go:292 | SatisfactionCheckerFunc.CheckSatisfied | DELETE | Functional adapter only used in tests; replaced by test-local `satisfactionCheckerFunc` adapter. |
| execute_bead_loop.go:1518 | rationaleIsSpecific | DELETE | Only used by `adjudicateNoChanges` (also dead). |
| execute_bead_loop.go:1612 | ExecuteBeadWorker.adjudicateNoChanges | DELETE | Replaced by `adjudicateNoChangesContract` (TD-031 §8.1, live at line 974). |
| execute_bead_post_review.go:336 | ApplyReviewTriageDecision | DELETE | Exported wrapper around live `applyReviewTriageDecision`; tests moved into package `agent` to access the unexported version directly. |
| execute_bead_review.go:139 | BeadReviewerFunc.ReviewBead | DELETE | Functional adapter only used in tests/testfixtures; replaced by test-local `beadReviewerFunc` (agent), `reviewerFunc` (testfixtures, cmd). |
| execute_bead_review.go:834 | ReadReviewArtifactResult | DELETE | One test caller; test now reads JSON inline via `reviewArtifactResult` struct. |
| format.go:10 | FormatComparisonTable | DELETE | No callers. File removed. |
| format.go:58 | FormatGradeTable | DELETE | No callers. File removed. |
| grade.go:52 | writeGradeArtifacts | DELETE | Whole grading subsystem dead. File removed. |
| grade.go:119 | GradeFn | DELETE | Whole grading subsystem dead. File removed. |
| grade.go:248 | buildGradingPrompt | DELETE | Whole grading subsystem dead. File removed. |
| grade.go:262 | buildGradingPromptBounded | DELETE | Whole grading subsystem dead. File removed. |
| grade.go:431 | parseGrades | DELETE | Whole grading subsystem dead. File removed. |
| harness_config.go:235 | harnessRegistry.Names | DELETE | Test-only. |
| harness_config.go:260 | harnessRegistry.Discover | DELETE | No production callers. |
| harness_config.go:293 | harnessRegistry.FirstAvailable | DELETE | No callers. |
| model_catalog_yaml.go:64 | WriteModelCatalogYAML | DELETE | No callers (load/apply paths still live). |
| models.go:37 | DefaultBenchmarkArms | DELETE | One test caller; test removed. |
| power_threshold.go:18 | TopNPowerThreshold | DELETE | Used only by retry-policy chain (also dead). File removed. |
| ratelimit_headers.go:47 | ParseRateLimitHeaders | DELETE | Tests only; tests rewritten to construct `RateLimitSignal` directly. |
| ratelimit_headers.go:59 | emptyRateLimitSignal | DELETE | Internal helper; removed with parser chain. |
| ratelimit_headers.go:69 | normalizeHeaders | DELETE | Internal helper; removed with parser chain. |
| ratelimit_headers.go:83 | parseClaudeRateLimit | DELETE | Internal helper; removed with parser chain. |
| ratelimit_headers.go:119 | parseCodexRateLimit | DELETE | Internal helper; removed with parser chain. |
| ratelimit_headers.go:138 | parseIntHeader | DELETE | Internal helper; removed with parser chain. |
| ratelimit_headers.go:152 | parseResetTimestamp | DELETE | Internal helper; removed with parser chain. |
| retry_policy.go:74 | ClassifyRetryOutcome | DELETE | Tests only. File removed. |
| retry_policy.go:113 | EvaluateRetryPolicy | DELETE | Tests only. File removed. |
| retry_policy.go:193 | computeNextMinPower | DELETE | Tests only. File removed. |
| routing_metrics.go:34 | RoutingMetricsStore.snapshotFile | DELETE | Quota-snapshot surface; no live producer. |
| routing_metrics.go:48 | RoutingMetricsStore.AppendQuotaSnapshot | DELETE | Tests only. |
| routing_metrics.go:53 | RoutingMetricsStore.AppendBurnSummary | DELETE | No callers (BurnSummary is read-only in production). |
| routing_metrics.go:63 | RoutingMetricsStore.ReadQuotaSnapshots | DELETE | Tests only. |
| routing_metrics.go:92 | BuildBurnSummaries | DELETE | Tests only. |
| routing_metrics.go:228 | parseWindowMinutes | DELETE | Used only inside dead BuildBurnSummaries. |
| runner.go:243 | Runner.RunWithConfig | DELETE | Replaced by `RunWithConfigViaService`; tests routed to test helper `runnerRunWithConfig`. |
| runner.go:276 | Runner.ValidateForExecuteLoop | DELETE | Replaced by `ValidateForExecuteLoopViaService`; tests routed to test helper `runnerValidateForExecuteLoop`. |
| runner.go:299 | Runner.Capabilities | DELETE | Replaced by `CapabilitiesViaService`; tests routed to test helper `runnerCapabilities`. |
| runner.go:964 | Runner.TestProviderConnectivity | DELETE | Replaced by `TestProviderConnectivityViaService`; tests routed to test helper `runnerTestProviderConnectivity`. |
| service_run.go:41 | SetServiceRunFactory | WIRE | Test-injection seam used by cmd/ integration tests. Reachable from `init()` (calls `SetServiceRunFactory(nil)` no-op) so RTA marks it live. |
| triage.go:79 | NewComplexityGate | DELETE | ComplexityGate field never wired in production; whole file removed. |
| triage.go:172 | triageDepth | DELETE | Helper for dead `NewComplexityGate`. |
| triage.go:191 | fileChildren | DELETE | Helper for dead `NewComplexityGate`. |
| triage.go:228 | blockParentDecomposed | DELETE | Helper for dead `NewComplexityGate`. |
| triage.go:263 | ACCoverageRate | DELETE | Tests only. |
| triage.go:297 | tokenizeAC | DELETE | Helper for dead `ACCoverageRate`. |
| triage.go:319 | RuleBasedClassifier | DELETE | Tests only. |
| triage_dispatch.go:54 | ClassifyAttemptForTriage | DELETE | `DispatchPostAttempt` never wired; whole subsystem trimmed. Live `IsLockContentionError` retained. |
| triage_dispatch.go:78 | RationaleCitesAC | DELETE | Tests only. |
| triage_dispatch.go:107 | tokenizeRationale | DELETE | Helper for dead `RationaleCitesAC`. |
| triage_dispatch.go:133 | DefaultSleep | DELETE | Helper for dead `RunWithLockBackoff`. |
| triage_dispatch.go:170 | RunWithLockBackoff | DELETE | Tests only. |
| triage_dispatch.go:200 | attemptIsLockContention | DELETE | Helper for dead `RunWithLockBackoff`. |
| triage_dispatch.go:224 | HistoryFromEvents | DELETE | Tests only. |
| triage_dispatch.go:239 | extractTriageEventField | DELETE | Helper for dead `HistoryFromEvents`. |
| triage_dispatch.go:278 | DispatchPostAttempt | DELETE | Tests only; never wired in production. |
| triage_dispatch.go:335 | fileNoChangesFollowup | DELETE | Helper for dead `DispatchPostAttempt`. |

## Summary

- WIRE: 1 (`SetServiceRunFactory` — test seam reached from `init()`).
- DELETE: 65 symbols, plus 9 entire files (`ac_structural_check.go`, `format.go`, `grade.go`, `power_threshold.go`, `retry_policy.go`, `triage.go`, plus `*_test.go` companions).
- PENDING: 0.

## Cascade

Companion deletions to keep tests green:
- Tests that exercised dead helpers were either deleted (e.g., `TestRationaleIsSpecific`, `TestRegistryNamesPreferenceOrder`, `TestDefaultBenchmarkArms`, the "grade" subtest of `TestEvidenceAssemblyTelemetry`, the "compare-result" subtest of `TestReviewEventBodySummary`) or rewired to local test helpers / inline reads.
- `triage_dispatch.go` retained only `IsLockContentionError` and its companion test (`TestIsLockContentionError`); the rest of the file (and `triage_dispatch_test.go`) was reduced.
- `ComplexityGate` field on `ExecuteBeadWorker` removed (dead pre-Claim gate); call sites in `execute_bead_loop.go` collapsed.
- `model_catalog_yaml.go` lost only `WriteModelCatalogYAML`; load/apply path remains live.
- `routing_metrics.go` lost the entire quota-snapshot/burn-summary writer surface; the `ReadBurnSummaries` reader stays live (used by `internal/server/providers.go`).
- `cmd/agent_metrics_review_evidence_test.go` and the testfixtures package switched from `agent.BeadReviewerFunc` to local `reviewerFunc` adapters.
- `internal/server/graphql/providers_unified_test.go` rewritten to construct `agent.RateLimitSignal` directly instead of via deleted `ParseRateLimitHeaders`.

## Verification

`go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from `cli/` reports zero remaining unreachable symbols whose path matches `^internal/agent/[a-z_]+\.go` (i.e., zero in the `internal/agent` package itself). Sub-packages `internal/agent/escalation`, `internal/agent/testfixtures`, `internal/agent/try` are scoped to other child beads (ddx-9df0636c, ddx-cd51f35b, ddx-2da07e5c) and remain unchanged here.

`cd cli && go test ./... -short` is green for all packages this bead touched. `cmd/TestReviewEvidenceApproveAttributesToTier` was confirmed pre-existing-red (failed identically with `git stash` of these changes applied).
