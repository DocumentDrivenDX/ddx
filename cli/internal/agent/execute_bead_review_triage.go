package agent

// TriageTierHintKey is the bead Extra key that records the triage policy's
// tier-pin hint for the next attempt. Populated when the BLOCK ladder reaches
// the escalate_tier rung; consumed by tier-aware executors to start above the
// default tier on the next drain pass.
const TriageTierHintKey = "triage.tier_hint"

// TriageNeedsHumanLabel is the legacy manual-review label retained for
// compatibility paths outside review-block triage. Review-block triage now
// moves beads to status=proposed instead of appending this label.
const TriageNeedsHumanLabel = "needs_human"
