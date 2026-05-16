# Execute-Bead Reachability Decisions

Source artifact: `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`

| Symbol | Decision | Evidence |
|---|---|---|
| `appendRateLimitRetryEvent` | WIRE | `ExecuteBeadWorker.Run` now passes `RateLimitOnRetry` into `agent/try.Attempt` and records retry events through this helper. |
| `CommitTracker` | DELETE | Removed the exported compatibility wrapper; production already calls `commitTrackerLocked` while holding `withTrackerLock`. Tests use a test-local wrapper. |
| `ValidateACCheckFail` | DELETE | Removed the stranded syntactic AC-waive helper; production gate evidence is handled by pre-merge checks and reviewer AC-check JSON. |
| `hasACWaiveTrailer` | DELETE | Removed with `ValidateACCheckFail`; no production caller remained. |
| `ShouldAttemptConflictRecovery` | DELETE | Removed root-package shim; production conflict recovery is owned by `internal/agent/try.ShouldAttemptConflictRecovery`. |
| `RunConflictRecovery` | DELETE | Removed root-package shim; production conflict recovery is owned by `internal/agent/try.RunConflictRecovery`. |
| `landConflictRescueCommand` | DELETE | Removed root-package shim; production rescue-command formatting remains in `internal/agent/try`. |
| `landConflictRescueDetail` | DELETE | Removed root-package shim; production rescue detail remains in `internal/agent/try`. |
| `newProgressEvent` | WIRE | `loopPhaseEmitter.EmitProgress` now constructs structured progress events through this helper. |
| `ClassifyReviewError` | WIRE | Reachable through the opt-in `ExecuteBeadLoopRuntime.PostMergeReview` path into `RunPostMergeReview`. |
| `CountPriorReviewErrors` | WIRE | Reachable through the opt-in `PostMergeReview` path. |
| `CountPriorReviewErrorsForSlot` | WIRE | Reachable through the opt-in `PostMergeReview` path. |
| `ReviewErrorEventBody` | WIRE | Reachable through the opt-in `PostMergeReview` path. |
| `ReviewErrorEventBodyForSlot` | WIRE | Reachable through the opt-in `PostMergeReview` path. |
| `ReviewCostDeferredEventBody` | WIRE | Reachable through the opt-in `PostMergeReview` path. |
| `ParseNoChangesRationale` | WIRE | Post-attempt decomposition parsing now uses the root compatibility wrapper. |
| `DefaultVerificationCommandRunner` | WIRE | Default no-changes verification now returns this wrapper when no custom timeout is configured. |
| `LandBeadResult` | WIRE | `SubmitWithPreMergeChecks` now routes successful land submissions through `LandBeadResult` with a coordinator-backed landing advancer. |
| `ApplyLandingToResult` | WIRE | `ApplyLandResultToExecuteBeadResult` now reuses it for landed and no-changes mapping. |
| `RunPostMergeReview` | WIRE | Added explicit `ExecuteBeadLoopRuntime.PostMergeReview` opt-in path before `CloseWithEvidence`. |
| `runPreCloseReviewGroup` | WIRE | Reachable through `RunPostMergeReview`. |
| `reviewErrorReviewerIndex` | WIRE | Reachable through `RunPostMergeReview`. |
| `applyReviewTriageDecision` | WIRE | Reachable through `RunPostMergeReview`. |
| `latestBlockPairedDegraded` | WIRE | Reachable through `applyReviewTriageDecision`. |
| `applyTriageAction` | WIRE | Reachable through `applyReviewTriageDecision`. |
| `clearReviewTriageClaimMetadata` | WIRE | Reachable through review terminal/triage parking paths. |
| `nextEscalatedPowerClass` | WIRE | Reachable through `applyTriageAction`. |
| `SelectReviewerPolicy` | DELETE | Removed obsolete fixed-policy helper; reviewer dispatch now uses Fizeau profile metadata. |
| `HasBeadLabelPrefix` | WIRE | Reachable through `RunPostMergeReview` skip-label evaluation. |
| `DefaultBeadReviewer.reviewerProfileForDispatch` | DELETE | Removed obsolete wrapper around `reviewerDispatchProfile`; no production caller remained. |
| `reviewFixableGapEventBody` | WIRE | Reachable through `RunPostMergeReview` fixable-gap scheduling. |
| `hasReviewFixableGapRepairScheduled` | WIRE | Reachable through `RunPostMergeReview`. |
| `scheduleReviewFixableGapRepair` | WIRE | Reachable through `RunPostMergeReview`. |
| `classifyTerminalReviewBlock` | WIRE | Reachable through `RunPostMergeReview`. |
| `reviewTerminalBlockEventBody` | WIRE | Reachable through terminal review block handling. |
| `applyTerminalReviewBlock` | WIRE | Reachable through `RunPostMergeReview`. |
| `applyReviewOperatorRequiredParking` | WIRE | Reachable through review error and terminal block paths. |

No pending wiring annotations were introduced.
