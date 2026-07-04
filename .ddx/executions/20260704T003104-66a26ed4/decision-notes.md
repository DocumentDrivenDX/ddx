# ddx-0bdbc446 Disposition Notes

Observation 1:
- Existing coverage: `ddx-eea2dfca` and `ddx-69672921`.
- Disposition: use the repeated pre-claim warning escalation bead family already filed for the same class of `system_unready` signal. The duplicate-claim race itself is the same operator-visible claim/claim-release failure mode, so no new bead is needed here.

Observation 2:
- Decision: the dirty-main-worktree-during-`phase=running` state is intentional where it comes from landing-time checkout sync deferral.
- Surface: `ExecuteBeadResult.CheckoutSyncDeferred` and `ExecuteBeadResult.CheckoutSyncDeferredPaths` are populated by `ApplyLandResultToExecuteBeadResult` in `cli/internal/agent/execute_bead_land.go`, and the reason string includes `checkout_sync_deferred`.
- Supporting code: `LandResult` documents the deferral fields in `cli/internal/agent/execute_bead_land.go`, and `TestLand_DirtyOverlapDefersCheckoutSync` / `TestLand_SyncCleanCheckoutStillUpdatesFiles` verify the behavior.

Observation 3:
- Decision: the default execute-bead merge/acceptance gate does not run an unconditional full workspace build/test independent of bead scope. It is intentionally governed by the bead's required execution artifacts.
- Surface: `EvaluateRequiredGatesForResult` in `cli/internal/agent/landing_gate_context.go` only evaluates required gates when governing IDs are present, and `execute_bead_orchestrator.go` wires that helper into the land path.
- Follow-up bead: `ddx-cce0227c` tracks the durable docs/policy note for this decision if maintainers want the rationale spelled out in a first-class bead.

Repo-state summary:
- Observation 1 is already covered by existing beads.
- Observation 2 is documented as intentional and surfaced through the execute-bead result object, not via bare `git status`.
- Observation 3 is documented as an explicit no-unconditional-workspace-build decision; the follow-up bead still needs a real ID if the policy is to be changed later.
