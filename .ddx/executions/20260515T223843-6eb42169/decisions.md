# Decisions for `ddx-5baa6a15`

The retired compare/quorum/benchmark core routing was deleted rather than rewired.
Current dispatch validation and skill docs mark these surfaces as replaced by
workflow skills, so no production entry point remains that should keep them live.

| Symbol | Decision | Notes |
|---|---|---|
| `RunCompareWithConfigViaService` | DELETE | Retired `legacy agent run --compare` production entry point. |
| `RunCompareWith` | DELETE | Test-only compare orchestration for retired surface. |
| `defaultResolvePromptForCompare` | DELETE | Prompt reader only used by retired compare path. |
| `runCompareArmWith` | DELETE | Retired compare arm dispatcher. |
| `createCompareWorktree` | DELETE | Sandbox worktree helper for retired compare mode. |
| `resolveGitRoot` | DELETE | Only used by retired compare sandbox helpers. |
| `captureGitDiff` | DELETE | Only used by retired compare sandbox helpers. |
| `runPostCommand` | DELETE | Only used by retired compare sandbox helpers. |
| `cleanupCompareWorktrees` | DELETE | Sandbox cleanup for retired compare mode. |
| `genCompareID` | DELETE | Compare-run ID generator with no surviving production caller. |
| `RunQuorumWithConfigViaService` | DELETE | Retired `legacy agent run --quorum` production entry point. |
| `RunQuorumWith` | DELETE | Quorum orchestration only kept for retired surface tests. |
| `QuorumMet` | DELETE | Result aggregator for retired quorum path. |
| `effectiveThreshold` | DELETE | Threshold helper for retired quorum path. |
| `CondenseOutput` | DELETE | Test-only output condenser with no production caller. |
| `condenseIsDiffHeader` | DELETE | Helper for deleted `CondenseOutput`. |
| `condenseIsAlphaNumUnderscore` | DELETE | Helper for deleted `CondenseOutput`. |
| `condenseTrimBlankLines` | DELETE | Helper for deleted `CondenseOutput`. |
| `LoadBenchmarkSuite` | DELETE | Benchmark suite loader for retired benchmark core CLI. |
| `RunBenchmarkWith` | DELETE | Benchmark orchestration for retired benchmark core CLI. |
| `RunBenchmarkWithConfigViaService` | DELETE | Retired benchmark production entry point. |
| `benchSummarize` | DELETE | Summary helper for retired benchmark core CLI. |
| `BenchmarkArm.ResolveArm` | DELETE | No remaining benchmark arm resolution path in production. |
| `BenchmarkArmsToCompare` | DELETE | Adapter from retired benchmark config to retired compare runtime. |

No `PENDING` annotations were needed.
