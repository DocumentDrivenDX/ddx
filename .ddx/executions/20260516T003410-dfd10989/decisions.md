# Execute-Bead Reachability Decisions

Source artifact: `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`

| Symbol | Decision | Evidence |
|---|---|---|
| `appendRateLimitRetryEvent` | WIRE | `ExecuteBeadWorker.Run` passes `RateLimitOnRetry` into `agent/try.Attempt` and appends retry evidence through the existing event helper. |
| `CommitTracker` | DELETE | Removed the unused production wrapper; `commitTrackerLocked` remains the live locked path used by `ExecuteBeadWithConfig`. A test-only wrapper preserves tracker-lock tests. |
| `ValidateACCheckFail` | WIRE | `ExecuteBeadWithConfig` now runs `ACCheckRunner` for committed attempts and applies this validator before final outcome classification. |
| `hasACWaiveTrailer` | WIRE | Reached through `ValidateACCheckFail` on the AC-check runner path. |
| `ShouldAttemptConflictRecovery` | DELETE | Removed the stale `agent` duplicate; the live recovery decision is `internal/agent/try.ShouldAttemptConflictRecovery`, called by `try.Attempt`. |
| `RunConflictRecovery` | DELETE | Removed the stale `agent` duplicate; the live recovery state machine is `internal/agent/try.RunConflictRecovery`, reached through `try.Attempt`. |
| `landConflictRescueCommand` | DELETE | Removed with the stale `agent` conflict-recovery duplicate. |
| `landConflictRescueDetail` | DELETE | Removed with the stale `agent` conflict-recovery duplicate. |
| `newProgressEvent` | WIRE | `loopPhaseEmitter.EmitProgress` now constructs progress-channel events through this helper. |
| `ClassifyReviewError` | DELETE | Removed with the retired post-land/pre-close review path; pre-land candidate review owns close eligibility. |
| `CountPriorReviewErrors` | DELETE | Removed with the retired post-land/pre-close review path. |
| `CountPriorReviewErrorsForSlot` | DELETE | Removed with the retired post-land/pre-close review path. |
| `ReviewErrorEventBody` | DELETE | Removed with the retired post-land/pre-close review path; current review escalation tests use explicit event bodies. |
| `ReviewErrorEventBodyForSlot` | DELETE | Removed with the retired post-land/pre-close review path. |
| `ReviewCostDeferredEventBody` | DELETE | Removed with the retired post-land/pre-close review path. |
| `ParseNoChangesRationale` | WIRE | `ExecuteBeadWorker.Run` now uses the agent wrapper when detecting `orchestrator_action: decompose`. |
| `DefaultVerificationCommandRunner` | WIRE | `defaultVerificationCommandRunnerForConfig` returns the default runner directly when no timeout override is configured. |
| `LandBeadResult` | WIRE | `ddx try` and `ddx work` landing now flow through `landExecuteBeadResult`, which calls `LandBeadResult` with the local land coordinator as the advancer. |
| `ApplyLandingToResult` | WIRE | `landExecuteBeadResult` applies `BeadLandingResult` back onto `ExecuteBeadResult` before writing the final artifact. |
| `RunPostMergeReview` | DELETE | Removed the retired post-land/pre-close review helper; live automated review is the pre-land candidate-cycle reviewer. |
| `runPreCloseReviewGroup` | DELETE | Removed with `RunPostMergeReview`; `reducePreCloseReviewGroup` remains because pre-land review still uses it. |
| `reviewErrorReviewerIndex` | DELETE | Removed with `RunPostMergeReview`. |
| `applyReviewTriageDecision` | DELETE | Removed with `RunPostMergeReview`; candidate-cycle repair/recovery owns review outcome routing. |
| `latestBlockPairedDegraded` | DELETE | Removed with `applyReviewTriageDecision`. |
| `applyTriageAction` | DELETE | Removed with `applyReviewTriageDecision`. |
| `clearReviewTriageClaimMetadata` | DELETE | Removed with `applyTriageAction`. |
| `nextEscalatedPowerClass` | DELETE | Removed with `applyTriageAction`. |
| `SelectReviewerPolicy` | DELETE | Removed obsolete always-smart policy helper; current reviewer routing uses `reviewerDispatchProfile`. |
| `HasBeadLabelPrefix` | DELETE | Removed because it was only used by the retired post-land review skip path. |
| `DefaultBeadReviewer.reviewerProfileForDispatch` | DELETE | Removed obsolete wrapper; live reviewer routing uses `reviewerDispatchProfile` with prior-error context. |
| `reviewFixableGapEventBody` | DELETE | Removed with the retired post-land repair scheduling path. |
| `hasReviewFixableGapRepairScheduled` | DELETE | Removed with the retired post-land repair scheduling path. |
| `scheduleReviewFixableGapRepair` | DELETE | Removed with the retired post-land repair scheduling path. |
| `classifyTerminalReviewBlock` | DELETE | Removed with the retired post-land terminal review path; terminal class constants remain for live review classification/recovery. |
| `reviewTerminalBlockEventBody` | DELETE | Removed with the retired post-land terminal review path. |
| `applyTerminalReviewBlock` | DELETE | Removed with the retired post-land terminal review path. |
| `applyReviewOperatorRequiredParking` | DELETE | Removed with the retired post-land terminal review path. |

No pending wiring annotations were added.
