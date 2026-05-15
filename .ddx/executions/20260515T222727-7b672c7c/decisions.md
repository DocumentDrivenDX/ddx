## Residual production-reachability decisions

All symbols listed for `cli/internal/agent/compare_adapter.go` and `cli/internal/agent/models.go` in `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json` were deleted.

Reason: the current tree already routes compare/quorum/benchmark work through DDx skills and explicitly rejects the old inline dispatch fields in `cli/internal/agent/run_dispatch_spec.go`; these adapter helpers no longer have a production entry point.

| Symbol | Decision | Rationale |
|---|---|---|
| `RunCompareWithConfigViaService` | `DELETE` | Retired core compare path; no live caller remains. |
| `RunCompareWith` | `DELETE` | Helper only served the retired compare adapter flow. |
| `defaultResolvePromptForCompare` | `DELETE` | Prompt-file reader only existed for the retired compare path. |
| `runCompareArmWith` | `DELETE` | Worker helper only served the retired compare adapter flow. |
| `createCompareWorktree` | `DELETE` | Sandbox worktree creation only served the retired compare adapter flow. |
| `resolveGitRoot` | `DELETE` | Utility only used by retired compare sandbox helpers. |
| `captureGitDiff` | `DELETE` | Diff capture only used by retired compare sandbox helpers. |
| `runPostCommand` | `DELETE` | Post-run hook only used by retired compare sandbox helpers. |
| `cleanupCompareWorktrees` | `DELETE` | Cleanup helper only served the retired compare adapter flow. |
| `genCompareID` | `DELETE` | Compare ID generation only served the retired compare adapter flow. |
| `RunQuorumWithConfigViaService` | `DELETE` | Quorum is now handled outside core CLI routing. |
| `RunQuorumWith` | `DELETE` | Helper only served the retired quorum adapter flow. |
| `QuorumMet` | `DELETE` | Voting helper only served the retired quorum adapter flow. |
| `effectiveThreshold` | `DELETE` | Threshold helper only served the retired quorum adapter flow. |
| `CondenseOutput` | `DELETE` | Output-condensing shim only existed for the retired adapter layer. |
| `condenseIsDiffHeader` | `DELETE` | Helper only used by the retired output-condensing shim. |
| `condenseIsAlphaNumUnderscore` | `DELETE` | Helper only used by the retired output-condensing shim. |
| `condenseTrimBlankLines` | `DELETE` | Helper only used by the retired output-condensing shim. |
| `LoadBenchmarkSuite` | `DELETE` | Benchmark suite loader only served the retired benchmark CLI. |
| `RunBenchmarkWith` | `DELETE` | Benchmark runner only served the retired benchmark CLI. |
| `RunBenchmarkWithConfigViaService` | `DELETE` | No live benchmark service entry point remains in core CLI routing. |
| `benchSummarize` | `DELETE` | Summary helper only served the retired benchmark CLI. |
| `BenchmarkArm.ResolveArm` | `DELETE` | Benchmark arm model shim only served the retired benchmark CLI. |
| `BenchmarkArmsToCompare` | `DELETE` | Benchmark arm conversion only served the retired benchmark CLI. |

No `PENDING` annotations were needed.
