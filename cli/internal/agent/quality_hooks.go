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
	Classification         string                `json:"classification,omitempty"`
	RecommendedAction      string                `json:"recommended_action,omitempty"`
	Rationale              string                `json:"rationale,omitempty"`
	SuggestedAmendments    []TriageAmendment     `json:"suggested_amendments,omitempty"`
	SuggestedFollowupBeads []FollowupBead        `json:"suggested_followup_beads,omitempty"`
	DecodeWarnings         []TriageDecodeWarning `json:"decode_warnings,omitempty"`
	Malformed              bool                  `json:"malformed,omitempty"`
}

// TriageAmendment describes a targeted bead amendment suggested by
// post-attempt triage.
type TriageAmendment struct {
	Target    string `json:"target,omitempty"`
	Amendment string `json:"amendment,omitempty"`
}

// TriageDecodeWarning preserves non-fatal model-output contract issues so
// operators can review bad triage responses without blocking queue progress.
type TriageDecodeWarning struct {
	Field      string `json:"field,omitempty"`
	Warning    string `json:"warning,omitempty"`
	RawExcerpt string `json:"raw_excerpt,omitempty"`
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
