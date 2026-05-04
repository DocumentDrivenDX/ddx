package agent

// LintResult is the structured outcome returned by the pre-dispatch bead
// quality lint hook.
type LintResult struct {
	Score          int      `json:"score"`
	Rationale      string   `json:"rationale,omitempty"`
	SuggestedFixes []string `json:"suggested_fixes,omitempty"`
	WaiversApplied []string `json:"waivers_applied,omitempty"`
}

// TriageResult is the structured outcome returned by the post-attempt bead
// lifecycle triage hook.
type TriageResult struct {
	Classification         string         `json:"classification,omitempty"`
	RecommendedAction      string         `json:"recommended_action,omitempty"`
	Rationale              string         `json:"rationale,omitempty"`
	SuggestedAmendments    string         `json:"suggested_amendments,omitempty"`
	SuggestedFollowupBeads []FollowupBead `json:"suggested_followup_beads,omitempty"`
}

// FollowupBead describes an execution-ready child or follow-up bead suggested
// by post-attempt triage.
type FollowupBead struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Acceptance  []string `json:"acceptance,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Parent      string   `json:"parent,omitempty"`
	Deps        []string `json:"deps,omitempty"`
}
