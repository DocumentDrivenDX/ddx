package docprose

// Finding is the machine-readable output shape the prose-quality tests
// lock down. It intentionally mirrors the TD-036 core fields only.
type Finding struct {
	File          string `json:"file"`
	Line          int    `json:"line"`
	RuleID        string `json:"rule_id"`
	Severity      string `json:"severity"`
	Rationale     string `json:"rationale"`
	SuggestedEdit string `json:"suggested_edit"`
}
