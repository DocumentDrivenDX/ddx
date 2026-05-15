package agent

// TriagePowerHintKey is the legacy transient work-loop retry floor key retained
// for compatibility with non-review retry paths. Review-block triage must not
// write it; operator-required parking only clears stale legacy values.
const TriagePowerHintKey = "work-next-min-power"

// TriageNeedsHumanLabel is the legacy manual-review label retained for
// compatibility paths outside review-block triage. Review-block triage now
// moves beads to status=proposed instead of appending this label.
const TriageNeedsHumanLabel = "needs_human"
