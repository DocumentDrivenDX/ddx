package agent

// TriagePowerHintKey is the bead Extra key that records the triage policy's
// powerClass-pin hint for the next attempt. Populated when the BLOCK ladder reaches
// the escalate_power rung; consumed by powerClass-aware executors to start above the
// default powerClass on the next drain pass.
const TriagePowerHintKey = "triage.power_hint"

// TriageNeedsHumanLabel is the legacy manual-review label retained for
// compatibility paths outside review-block triage. Review-block triage now
// moves beads to status=proposed instead of appending this label.
const TriageNeedsHumanLabel = "needs_human"
