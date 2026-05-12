# ddx agent command test migration audit

Bead: ddx-d8474e0e

This audit covers the retired `ddx agent execute-bead` and
`ddx agent execute-loop` test files. The live command surface is now
`ddx try` for single-bead attempts and `ddx work` for queue draining.

Classification legend:

- `cobra-flag-parsing-only`: removed with the deleted command surface.
- `engine-via-harness`: covered by internal `agent` package tests or direct
  engine/helper tests.
- `unique-behavior`: retargeted to `ddx try` / `ddx work` command tests.

| Prior test | Classification | Disposition |
|---|---|---|
| TestExecuteBeadContextBudgetFromConfig | unique-behavior | Retargeted through `TestTry_PreservesJSONResultSchema_SameAsExecuteBead` and focused `try` config coverage. |
| TestExecuteBead_RequiredGatePass_Merges | engine-via-harness | Covered by landing/gate engine tests and `TestTry_AppliesGateContextToLanding`. |
| TestExecuteBead_RequiredGateFail_Preserves | engine-via-harness | Covered by landing/gate engine tests and `TestTry_AppliesGateContextToLanding`. |
| TestExecuteBead_NoGoverningIDs_Merges | engine-via-harness | Covered by `agent.BuildLandingGateContext` tests and try landing context coverage. |
| TestExecuteBeadMerge | unique-behavior | Retargeted through `TestTry_PreservesJSONResultSchema_SameAsExecuteBead`. |
| TestExecuteBeadPreserveOnMergeFailure | engine-via-harness | Covered by landing/orchestrator tests; command wrapper deleted. |
| TestExecuteBeadNoMerge | cobra-flag-parsing-only | Removed with `ddx agent execute-bead`; no live `--no-merge` wrapper remains. |
| TestExecuteBeadHiddenRefUniqueness | engine-via-harness | Covered by execute-bead engine hidden-ref behavior. |
| TestExecuteBeadNoChanges | unique-behavior | Retargeted through try result-schema and exit-code contract tests. |
| TestExecuteBeadNoEvidenceProducedWithoutCommitOrRationale | engine-via-harness | Covered by execute-bead engine/report classification tests. |
| TestExecuteBeadDirtyWorktreeWithoutCommits | engine-via-harness | Covered by execute-bead landing/orchestrator tests. |
| TestExecuteBeadMergePreservesContext | engine-via-harness | Covered by landing/orchestrator context preservation tests. |
| TestExecuteBeadSynthesizesPromptAndArtifacts | engine-via-harness | Covered by execute-bead prompt and artifact snapshot tests. |
| TestExecuteBeadResolvesPathStyleSpecID | engine-via-harness | Covered by execute-bead prompt/spec-id tests. |
| TestExecuteBeadWritesResultArtifactBundle | engine-via-harness | Covered by execute-bead artifact tests. |
| TestExecuteBeadFromRevFlag | unique-behavior | Retargeted through try runtime/config coverage. |
| TestExecuteBeadOrphanRecovery | unique-behavior | Retargeted as `TestTry_RecoversOrphanedWorktreesBeforeSpawn`. |
| TestExecuteBeadHarnessNoiseNotSynthesized | engine-via-harness | Covered by execute-bead result/report parsing tests. |
| TestExecuteBeadAgentErrorNoCommits | engine-via-harness | Covered by execute-bead result/report classification tests. |
| TestExecuteBeadTimeoutNoCommitsReportsExecutionFailure | engine-via-harness | Covered by execute-bead timeout/error classification tests. |
| TestExecuteBeadAgentErrorWithCommitsPreservesBeforeLand | engine-via-harness | Covered by landing/orchestrator preservation tests. |
| TestExecuteBeadAgentErrorWithCommitsPreserves | engine-via-harness | Covered by landing/orchestrator preservation tests. |
| TestExecuteBeadAgentErrorMessageInOutput | unique-behavior | Retargeted through `TestTry_PreservesJSONResultSchema_SameAsExecuteBead`. |
| TestExecuteBeadHeadRevFailure | engine-via-harness | Covered by execute-bead orchestration error tests. |
| TestExecuteBeadCompoundErrorAgentAndHeadRevFailure | engine-via-harness | Covered by execute-bead orchestration error tests. |
| TestExecuteBeadInvalidBeadID | unique-behavior | Retargeted through try command validation and exit-code contract coverage. |
| TestExecuteBeadEvidenceFields | unique-behavior | Retargeted through `TestTry_PreservesJSONResultSchema_SameAsExecuteBead`. |
| TestExecuteBeadModelFlagPassthrough | unique-behavior | Retargeted through try routing passthrough coverage. |
| TestExecuteBeadStatusMapping | engine-via-harness | Covered by execute-bead status classification tests. |
| TestExecuteBeadGatePass | engine-via-harness | Covered by landing/gate engine tests and `TestTry_AppliesGateContextToLanding`. |
| TestExecuteBeadGateBlocksLanding | engine-via-harness | Covered by landing/gate engine tests and `TestTry_AppliesGateContextToLanding`. |
| TestExecuteBeadNoGatesWhenNoChanges | engine-via-harness | Covered by landing/gate engine tests. |
| TestExecuteBeadEmbeddedAgentStateRedirected | engine-via-harness | Covered by service writer/session-index tests. |
| TestExecuteBeadPromptIsXMLTagged | engine-via-harness | Covered by prompt snapshot tests. |
| TestExecuteLoopCostCap_ShortCircuitsAfterCap | unique-behavior | Retargeted through `ddx work` cost-cap/resource tests. |
| TestExecuteLoopCostCap_LocalProvidersDoNotCount | unique-behavior | Retargeted through `ddx work` cost-cap/resource tests. |
| TestInfrastructureFailureDoesNotEscalate | unique-behavior | Retargeted through `ddx work` escalation/resource tests. |
| TestReviewRetryThresholdFromConfigCLI | unique-behavior | Retargeted through `ddx work` config coverage. |
| TestEscalationLadder_WiredIntoExecutor | unique-behavior | Retargeted through `ddx work` escalation helper coverage. |
| TestEscalationLadder_StopsAtFinalRung | unique-behavior | Retargeted through `ddx work` escalation helper coverage. |
| TestZeroConfigEscalationIntegration_RealLadderAdvancesMinPower | unique-behavior | Retargeted through `ddx work` escalation helper coverage. |
| TestInvestigationRetry_RequestsSmartRoute | unique-behavior | Retargeted through `ddx work` escalation helper coverage. |
| TestPerBeadCostTracker_StopsEscalationAtLimit | unique-behavior | Retargeted through `ddx work` escalation helper coverage. |
| TestPerBeadCostTracker_NoLimit_RunsToLadderExhaustion | unique-behavior | Retargeted through `ddx work` escalation helper coverage. |
| TestTierHint_StartsAtLabelFloor | unique-behavior | Retargeted through `ddx work` tier-hint coverage. |
| TestTierHint_NoLabel_UsesDefault | unique-behavior | Retargeted through `ddx work` tier-hint coverage. |
| TestTierHint_InvalidLabel_FallsBackToDefault | unique-behavior | Retargeted through `ddx work` tier-hint coverage. |
| TestTierHint_FlagRaisesFloorAboveLabel | unique-behavior | Retargeted through `ddx work` tier-hint coverage. |
| TestRetryPolicy_ReviewFixableGapRaisesMinPowerAndPreservesPins | unique-behavior | Retargeted through `ddx work` review-repair/escalation coverage. |
| TestRepairRouting_FixableGapCanRaiseMinPower | unique-behavior | Retargeted through `ddx work` review-repair/escalation coverage. |
| TestRetryPolicy_ReviewFixableGapRaisesMinPower_LadderExhausted | unique-behavior | Retargeted through `ddx work` review-repair/escalation coverage. |
| TestWorkRejectsProfileLadders | unique-behavior | Retargeted under `ddx work` routing rejection tests. |
| TestWorkRejectsModelOverrides | unique-behavior | Retargeted under `ddx work` routing rejection tests. |
| TestWorkUsesProjectRootForNoWorkScan | unique-behavior | Retargeted under `ddx work` queue-scan tests. |
| TestInvokeExecuteBeadFromLoopParsesJSONAmidWarnings | engine-via-harness | Covered by shared execute-loop helper tests. |
| TestExecuteLoopZeroConfigInferredTierSetsInitialMinPower | unique-behavior | Retargeted under `ddx work` zero-config routing tests. |
| TestExecuteLoopNoRoutingFlagsCheapTierMayRemainUnconstrained | unique-behavior | Retargeted under `ddx work` routing tests. |
| TestExecuteBeadRoutingEvidencePersisted | engine-via-harness | Retained in `execute_bead_e2e_routing_test.go`; it exercises the engine evidence path, not the removed command group. |
| TestExecuteBeadRoutingEvidenceProviderFallsBackToHarness | engine-via-harness | Retained in `execute_bead_e2e_routing_test.go`; it exercises the engine evidence path, not the removed command group. |
| TestExecuteBeadRoutingEvidenceNoEvidence | engine-via-harness | Retained in `execute_bead_e2e_routing_test.go`; it exercises the engine evidence path, not the removed command group. |

Named replacement tests required by ddx-d8474e0e AC #10:

- `TestTry_RecoversOrphanedWorktreesBeforeSpawn`
- `TestTry_AppliesGateContextToLanding`
- `TestTry_PreservesJSONResultSchema_SameAsExecuteBead`
- `TestWork_PreservesJSONResultSchema_SameAsExecuteLoop`
- `TestTry_ExitCodeContract_0_1_2`
