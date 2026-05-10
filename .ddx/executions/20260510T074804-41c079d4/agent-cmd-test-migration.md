# agent-cmd test migration audit

AC 11 deliverable for ddx-d8474e0e. Each row names the test function and its
disposition: (a) cobra-flag-parsing-only → delete, (b) engine-via-harness →
migrate to call `ExecuteBeadWithConfig` or `try.Attempt` directly, (c) unique
behavior → re-target to `ddx try` / `ddx work` cobra surface with same
assertions.

## Disposition key

- **(a)** cobra-flag-parsing-only: test exercises only Cobra flag registration,
  help text, or arg parsing — no engine behavior. Delete with the file.
- **(b)** engine-via-harness: test exercises engine behavior (orchestrator,
  landing, git, prompt synthesis) but enters through the Cobra wrapper. Migrate
  to call `agent.ExecuteBeadWithConfig` or `try.Attempt` directly; the test
  body and assertions carry over unchanged.
- **(c)** unique behavior: test asserts behavior that is specific to the Cobra
  surface (flag name, cobra-level validation, exit-code propagation at the
  cobra layer). Keep the test but re-target entry point to `ddx try` / `ddx
  work` cobra surface.

---

## cli/cmd/agent_execute_bead_test.go (3024 lines, 30 tests)

| Test function | Disposition | Rationale |
|---|---|---|
| TestExecuteBeadMerge | **(b)** engine-via-harness | Tests full merge outcome (worktree add, script harness commit, Land). Entered via cobra `agent execute-bead`; migrate to call `ExecuteBeadWithConfig` with same real-git repo setup. |
| TestExecuteBeadPreserveOnMergeFailure | **(b)** engine-via-harness | Tests preserve path on merge conflict. Real git + script harness; entry via cobra. Migrate to direct engine call. |
| TestExecuteBeadNoMerge | **(c)** unique behavior | Asserts `--no-merge` flag suppresses merge and sets `Reason == "--no-merge specified"`. Flag name is cobra-surface-specific; re-target to `ddx try --no-merge`. |
| TestExecuteBeadHiddenRefUniqueness | **(b)** engine-via-harness | Tests that two concurrent runs produce distinct preserve refs. Real git; entry via cobra. Migrate to `ExecuteBeadWithConfig`. |
| TestExecuteBeadNoChanges | **(b)** engine-via-harness | Tests `no-changes` outcome when agent writes rationale but no commit. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadNoEvidenceProducedWithoutCommitOrRationale | **(b)** engine-via-harness | Tests `no-evidence` outcome when agent does nothing. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadDirtyWorktreeWithoutCommits | **(b)** engine-via-harness | Tests SynthesizeCommit path for dirty-but-uncommitted worktrees. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadMergePreservesContext | **(b)** engine-via-harness | Tests multi-commit worker branch survives merge onto main. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadSynthesizesPromptAndArtifacts | **(b)** engine-via-harness | Tests prompt synthesis (bead title, description, governing refs). Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadResolvesPathStyleSpecID | **(b)** engine-via-harness | Tests path-style spec-id resolution in prompt synthesis. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadWritesResultArtifactBundle | **(b)** engine-via-harness | Tests manifest.json / result.json artifact bundle. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadFromRevFlag | **(c)** unique behavior | Asserts `--from` flag resolves a named revision as BaseRev. Flag name is cobra-surface-specific; re-target to `ddx try --from`. |
| TestExecuteBeadOrphanRecovery | **(b)** engine-via-harness | Tests `RecoverOrphans` sweeps stale worktrees before a new attempt. This is the behavior TestTry_RecoversOrphanedWorktreesBeforeSpawn (AC 10) must assert after the ddx-ddacd4ff lift. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadHarnessNoiseNotSynthesized | **(b)** engine-via-harness | Tests noise-only worktrees are not synthesized into a commit. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadAgentErrorNoCommits | **(b)** engine-via-harness | Tests `execution_failed` outcome when agent errors with no commits. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadTimeoutNoCommitsReportsExecutionFailure | **(b)** engine-via-harness | Tests ExitCode=-1 shape maps to `execution_failed`. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadAgentErrorWithCommitsPreservesBeforeLand | **(b)** engine-via-harness | Tests error-with-commits preserves before attempting merge. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadAgentErrorWithCommitsPreserves | **(b)** engine-via-harness | Tests preserve ref is set on error-with-commits path. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadAgentErrorMessageInOutput | **(b)** engine-via-harness | Tests error message appears in JSON `Error` field. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadHeadRevFailure | **(b)** engine-via-harness | Tests `HeadRev` failure after agent run is classified as error outcome. Real git + script harness (deletes `.git` file). Migrate to direct engine call. |
| TestExecuteBeadCompoundErrorAgentAndHeadRevFailure | **(c)** unique behavior | Asserts `cmdErr != nil` from cobra (cobra-level exit-code propagation) alongside JSON fields. The `require.Error(t, cmdErr)` assertion is cobra-layer-specific; re-target to `ddx try` cobra surface. |
| TestExecuteBeadInvalidBeadID | **(c)** unique behavior | Tests cobra-level bead-ID regex validation returns a cobra error before any engine work. The `validBeadID` check lives in `runAgentExecuteBead`; re-target to `ddx try` cobra handler. |
| TestExecuteBeadEvidenceFields | **(b)** engine-via-harness | Tests runtime evidence fields (Harness, Model, SessionID, timestamps). Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadModelFlagPassthrough | **(c)** unique behavior | Asserts `--model` flag value is forwarded verbatim to runner and empty stays empty. Flag name and passthrough contract are cobra-surface-specific; re-target to `ddx try --model`. |
| TestExecuteBeadStatusMapping | **(b)** engine-via-harness | Tests all status enum values (success, no_changes, execution_failed, land_conflict) via real git scenarios. Migrate to direct engine call. |
| TestExecuteBeadGatePass | **(b)** engine-via-harness | Tests required gate pass → merge. This is the behavior TestTry_AppliesGateContextToLanding (AC 10) must assert after the ddx-ddacd4ff lift of BuildLandingGateContext. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadGateBlocksLanding | **(b)** engine-via-harness | Tests required gate fail → preserve. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadNoGatesWhenNoChanges | **(b)** engine-via-harness | Tests gates are not evaluated when resultRev == baseRev. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadEmbeddedAgentStateRedirected | **(b)** engine-via-harness | Tests SessionLogDir wiring redirects harness state into execution bundle. Real git + script harness. Migrate to direct engine call. |
| TestExecuteBeadPromptIsXMLTagged | **(b)** engine-via-harness | Tests synthesized prompt is well-formed XML with required tags. Real git + script harness. Migrate to direct engine call. |

---

## cli/cmd/agent_execute_bead_e2e_test.go (74 lines, 1 test)

| Test function | Disposition | Rationale |
|---|---|---|
| TestExecuteBeadContextBudgetFromConfig | **(b)** engine-via-harness | Tests `ContextBudget: minimal` flows from `.ddx/config.yaml` through `LoadAndResolve` → `ExecuteBeadWithConfig` → prompt synthesis. Uses `fakeExecuteBeadGit` + `fakeAgentRunner`; entered via `runExecuteBead` (cobra). The config wiring is engine-level; migrate to call `ExecuteBeadWithConfig` directly with the same `fakeAgentRunner.sideEffect` callback. |

---

## cli/cmd/agent_execute_bead_gates_test.go (252 lines, 3 tests)

| Test function | Disposition | Rationale |
|---|---|---|
| TestExecuteBead_RequiredGatePass_Merges | **(b)** engine-via-harness | Tests gate pass → merge → GateResults populated. Real git + script harness; entered via `runExecuteBead` (cobra). Migrate to direct engine call. |
| TestExecuteBead_RequiredGateFail_Preserves | **(b)** engine-via-harness | Tests gate fail → preserve + RequiredExecSummary = "fail". Real git + script harness. Migrate to direct engine call. |
| TestExecuteBead_NoGoverningIDs_Merges | **(b)** engine-via-harness | Tests merge proceeds and gate eval is skipped when bead has no spec-id. Real git + script harness. Migrate to direct engine call. |

---

## cli/cmd/execute_bead_e2e_routing_test.go (150 lines, 3 tests)

| Test function | Disposition | Rationale |
|---|---|---|
| TestExecuteBeadRoutingEvidencePersisted | **(b)** engine-via-harness | Tests routing event (`kind:routing`) is persisted to bead store with provider/model/reason fields. Uses `fakeExecuteBeadGit` + `fakeAgentRunner`; entered via `runExecuteBead` (cobra). Migrate to direct `ExecuteBeadWithConfig` call. **Note:** the event's `Source` field currently says `"ddx agent execute-bead"` (line 59); after migration to `ddx try` it must change to `"ddx try"`. |
| TestExecuteBeadRoutingEvidenceProviderFallsBackToHarness | **(b)** engine-via-harness | Tests empty Provider falls back to Harness in routing event. Uses fakes; cobra entry. Migrate to direct engine call. |
| TestExecuteBeadRoutingEvidenceNoEvidence | **(b)** engine-via-harness | Tests routing event is written even on no-evidence outcome. Uses fakes; cobra entry. Migrate to direct engine call. |

---

## cli/cmd/agent_execute_loop_test.go (75 lines, 2 tests)

| Test function | Disposition | Rationale |
|---|---|---|
| TestWorkUsesProjectRootForNoWorkScan | **keep as-is** | Already tests `ddx work` cobra surface (`executeCommand(root, "work", "--json")`). No migration required; file should be renamed to `work_test.go` or merged into `cli/cmd/work_test.go`. |
| TestInvokeExecuteBeadFromLoopParsesJSONAmidWarnings | **keep as-is** | Already calls `agent.ExecuteBeadWithConfig` directly — not through cobra. No migration required. |

---

## cli/cmd/agent_execute_loop_e2e_test.go (184 lines, 1 test)

| Test function | Disposition | Rationale |
|---|---|---|
| TestReviewRetryThresholdFromConfigCLI | **keep as-is** | Calls `agent.ExecuteBeadWorker.Run` directly with `config.LoadAndResolve`. Already at the engine level; tests that `review_max_retries` flows from config YAML into the resolved loop config and that the legacy post-land reviewer is not invoked. No migration required; file should be renamed to `work_e2e_test.go`. |

---

## Summary

| Category | Count | Action |
|---|---|---|
| (a) cobra-flag-parsing-only | 0 | — |
| (b) engine-via-harness | 35 | Migrate: call `ExecuteBeadWithConfig` or `try.Attempt` directly; remove cobra plumbing from each test |
| (c) unique behavior | 5 | Re-target: change entry from `agent execute-bead` / `agent execute-loop` to `ddx try` / `ddx work` cobra surface |
| keep as-is (already migrated) | 2 | Rename files: `agent_execute_loop_test.go` → `work_test.go`, `agent_execute_loop_e2e_test.go` → `work_e2e_test.go` |
| **Total** | **42** | |

### (c) tests requiring re-targeting to `ddx try` / `ddx work`

1. `TestExecuteBeadNoMerge` → `ddx try --no-merge`
2. `TestExecuteBeadFromRevFlag` → `ddx try --from <rev>`
3. `TestExecuteBeadCompoundErrorAgentAndHeadRevFailure` → `ddx try` (cobra exit-code assertion)
4. `TestExecuteBeadInvalidBeadID` → `ddx try` (cobra-layer ID validation)
5. `TestExecuteBeadModelFlagPassthrough` → `ddx try --model`

### Critical prerequisite dependency

`TestExecuteBeadOrphanRecovery` (classified (b)) is the existing test proving
`RecoverOrphans` runs before a new attempt. The AC 10 test
`TestTry_RecoversOrphanedWorktreesBeforeSpawn` is the same assertion re-targeted
to the `ddx try` cobra surface — it cannot pass until ddx-ddacd4ff lifts
`agent.RecoverOrphans` into `try.Attempt` or `ExecuteBeadWithConfig`.

Similarly, `TestExecuteBeadGatePass` and `TestExecuteBead_RequiredGatePass_Merges`
prove `BuildLandingGateContext` is invoked on the landing side. The AC 10 test
`TestTry_AppliesGateContextToLanding` re-targets that assertion to `ddx try` and
cannot pass until ddx-ddacd4ff lifts `agent.BuildLandingGateContext` into the
`try.Attempt` path.

### Routing event Source field

`TestExecuteBeadRoutingEvidencePersisted` (line 59 of execute_bead_e2e_routing_test.go)
asserts `Source == "ddx agent execute-bead"`. After migration to `ddx try`, the
`appendBeadRoutingEvidence` call site must update this constant to `"ddx try"`.
