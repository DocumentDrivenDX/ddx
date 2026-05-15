# Candidate Cycle Residual Decisions

The `internal/agent/candidate_cycle.go` residual cluster is wired through the live `ExecuteBeadWithConfig` worker path via `applyWorkerCandidateCycle`, which runs `AttemptCycleCoordinator.Run` against the completed worker candidate before `result.json` is written.

| Symbol | Decision | Evidence |
| --- | --- | --- |
| `candidateIterationRef` | `WIRE` | Reached from `GitCandidateRefStore.PinCandidateRef` during worker candidate pinning. |
| `GitCandidateRefStore.PinCandidateRef` | `WIRE` | Used by `applyWorkerCandidateCycle` via `AttemptCycleCoordinator.RefStore`. |
| `GitCandidateRefStore.UnpinCandidateRef` | `WIRE` | Reachable from the landed-cleanup branch of `AttemptCycleCoordinator.Run`. |
| `ShouldRetainCandidateRef` | `WIRE` | Reachable from the landed-cleanup branch of `AttemptCycleCoordinator.Run`. |
| `AttemptCycleCoordinator.Run` | `WIRE` | Called by `applyWorkerCandidateCycle` from `ExecuteBeadWithConfig`. |
| `AttemptCycleCoordinator.recordCandidateCycleState` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run`. |
| `candidateChecksFailedDetail` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run` check-failure handling. |
| `appendUniqueString` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run` pinned-ref tracking. |
| `AttemptCycleCoordinator.pinCandidateRef` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run`. |
| `AttemptCycleCoordinator.appendCandidateChecksFailedEvent` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run` check-failure handling. |
| `AttemptCycleCoordinator.maxRepairCycles` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run` repair eligibility handling. |
| `AttemptCycleCoordinator.classifyCandidateReview` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run` review classification handling. |
| `repairPromptInput` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run` repair prompt construction. |
| `normalizeRepairedCandidate` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run` repair result normalization. |
| `AttemptCycleCoordinator.appendCandidateReviewClassifiedEvent` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run` review-event handling. |
| `AttemptCycleCoordinator.appendRepairCycleStartedEvent` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run` repair-event handling. |
| `appendExecutionCycleTrace` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run` cycle trace recording. |
| `executionCycleTraceFor` | `WIRE` | Reached inside `AttemptCycleCoordinator.Run` cycle trace recording. |
| `WriteWorktreeCandidateCycleState` | `WIRE` | Reached from `AttemptCycleCoordinator.recordCandidateCycleState`. |
| `MarkWorktreeActiveCycle` | `WIRE` | Called by `applyWorkerCandidateCycle` before running the coordinator. |
| `ClearWorktreeActiveCycle` | `WIRE` | Called by `applyWorkerCandidateCycle` on coordinator exit. |
| `applyCandidateCycleStateToMetadata` | `WIRE` | Reached from `WriteWorktreeCandidateCycleState`. |
| `applyCandidateCycleStateToRunState` | `WIRE` | Reached from `WriteWorktreeCandidateCycleState`. |
