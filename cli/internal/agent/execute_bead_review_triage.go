package agent

// TriageTierHintKey is the bead Extra key that records the triage policy's
// tier-pin hint for the next attempt. Populated when the BLOCK ladder reaches
// the escalate_tier rung; consumed by tier-aware executors to start above the
// default tier on the next drain pass.
const TriageTierHintKey = "triage.tier_hint"

// TriageNeedsHumanLabel is appended to a bead when the BLOCK ladder reaches
// its terminal rung. The execute-loop label-filter and the human triage UI
// both use this label to surface beads that no longer benefit from automated
// re-attempts.
const TriageNeedsHumanLabel = "needs_human"
