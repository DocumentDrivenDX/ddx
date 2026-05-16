package agent

// TriageNeedsHumanLabel is the legacy manual-review label retained for
// compatibility paths outside review-block triage. Review-block triage now
// moves beads to status=proposed instead of appending this label.
const TriageNeedsHumanLabel = "needs_human"
