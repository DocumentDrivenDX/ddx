package agent

// TriagePowerHintKey is the transient work-loop retry floor key. It is not a
// bead-authored difficulty hint; authored task difficulty lives at
// triage.estimated_difficulty.
const TriagePowerHintKey = "work-next-min-power"

// TriageNeedsHumanLabel is the legacy manual-review label retained for
// compatibility paths outside review-block triage. Review-block triage now
// moves beads to status=proposed instead of appending this label.
const TriageNeedsHumanLabel = "needs_human"
