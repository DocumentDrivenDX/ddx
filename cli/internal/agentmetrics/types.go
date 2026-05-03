// Package agentmetrics loads attempt outcomes for Story 11 aggregations
// (success rate, cost, duration per harness/provider/model/tier).
//
// Source priority is FEAT-010 run-store first (.ddx/exec/runs/*.json) with
// .ddx/executions/*/result.json as the legacy fallback. Per-attempt records
// are deduped by AttemptID — run-store wins over bundles when both exist —
// matching the multi-source dedupe used by state_runs.go (ddx-65543530).
//
// Per the Story 11 locked decision, the already_satisfied terminal status
// counts as success. See Bucket.Successful.
package agentmetrics

import "time"

// Source identifies where an Attempt was loaded from.
const (
	SourceRunStore = "run-store"
	SourceBundle   = "bundle"
)

// Attempt is one execute-bead try projected to the fields Story 11
// aggregations need. Cost, duration, and exit_code come from the per-attempt
// result (run record or bundle result.json). Harness/provider/model are
// taken from the result when present and enriched from kind:routing /
// kind:escalation-summary bead events otherwise.
type Attempt struct {
	AttemptID    string    `json:"attempt_id"`
	BeadID       string    `json:"bead_id,omitempty"`
	Harness      string    `json:"harness,omitempty"`
	Provider     string    `json:"provider,omitempty"`
	Model        string    `json:"model,omitempty"`
	Tier         string    `json:"tier,omitempty"`
	Status       string    `json:"status,omitempty"`
	Outcome      string    `json:"outcome,omitempty"`
	Bucket       Bucket    `json:"bucket,omitempty"`
	CostUSD      float64   `json:"cost_usd,omitempty"`
	DurationMS   int       `json:"duration_ms,omitempty"`
	ExitCode     int       `json:"exit_code"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	FinishedAt   time.Time `json:"finished_at,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	Source       string    `json:"source,omitempty"`
}
