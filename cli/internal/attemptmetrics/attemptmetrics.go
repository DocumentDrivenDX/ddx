// Package attemptmetrics implements the cold-powerClass metrics layer: one row per
// bead-attempt written to .ddx/metrics/attempts.jsonl. Rows are appended with
// O_APPEND so concurrent single-writer access is safe. The schema_version field
// allows future migrations without losing historical rows.
package attemptmetrics

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

const SchemaVersion = 1

// AttemptRow is one row in .ddx/metrics/attempts.jsonl.
type AttemptRow struct {
	SchemaVersion int    `json:"schema_version"`
	AttemptID     string `json:"attempt_id"`
	BeadID        string `json:"bead_id"`
	SessionID     string `json:"session_id,omitempty"`
	TSStart       string `json:"ts_start,omitempty"` // RFC3339
	TSEnd         string `json:"ts_end,omitempty"`   // RFC3339

	Model    string `json:"model,omitempty"`
	Harness  string `json:"harness,omitempty"`
	Profile  string `json:"profile,omitempty"`
	Provider string `json:"provider,omitempty"`

	// PromptTemplateID and PromptTemplateHash identify the prompt variant used.
	PromptTemplateID   string `json:"prompt_template_id,omitempty"`
	PromptTemplateHash string `json:"prompt_template_hash,omitempty"` // hex sha256 of rendered prompt

	SpecID string `json:"spec_id,omitempty"`

	Outcome  string  `json:"outcome"`
	ExitCode int     `json:"exit_code"`
	CostUSD  float64 `json:"cost_usd,omitempty"`

	DurationMS   int `json:"duration_ms"`
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	TotalTokens  int `json:"total_tokens,omitempty"`

	ReviewVerdict                string         `json:"review_verdict,omitempty"`
	ReviewFindingCountBySeverity map[string]int `json:"review_finding_count_by_severity,omitempty"`

	// ReviewerModel is the resolved model used for the reviewer dispatch.
	ReviewerModel string `json:"reviewer_model,omitempty"`
	// ReviewerACMismatchCount is the number of ACs where the reviewer's grade
	// disagreed with the ac-check.json mechanical result.
	ReviewerACMismatchCount int `json:"reviewer_acmismatch_count,omitempty"`
	// ReviewerOverrideReasons lists each AC where reviewer and ac-check diverged.
	ReviewerOverrideReasons []string `json:"reviewer_override_reasons,omitempty"`
	// ReviewerAccuracySignal records the eventual truth after the reviewer's
	// verdict: confirmed (operator agreed), override (operator contradicted),
	// contested, or unknown (not yet determined).
	ReviewerAccuracySignal string `json:"reviewer_accuracy_signal,omitempty"`

	LadderMinPowerInitial int `json:"ladder_min_power_initial,omitempty"`
	LadderMinPowerFinal   int `json:"ladder_min_power_final,omitempty"`
	LadderStepsTaken      int `json:"ladder_steps_taken,omitempty"`

	// ReplayOf, when non-empty, identifies the original attempt_id this row
	// was produced from. Set by `ddx bead replay` so the analyze CLI can
	// group replays with their source attempt.
	ReplayOf string `json:"replay_of,omitempty"`
}

// BeadAttemptEvents is the raw input for BackfillFromEvents: a bead ID, its
// optional spec-id, and its full event stream.
type BeadAttemptEvents struct {
	BeadID string
	SpecID string // from bead Extra["spec-id"], optional
	Events []bead.BeadEvent
}

// costEventBody matches the JSON shape written by appendBeadCostEvidence.
type costEventBody struct {
	AttemptID    string  `json:"attempt_id"`
	Harness      string  `json:"harness,omitempty"`
	Provider     string  `json:"provider,omitempty"`
	Model        string  `json:"model,omitempty"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	DurationMS   int     `json:"duration_ms"`
	ExitCode     int     `json:"exit_code"`
}

// AttemptsPath returns the absolute path to metrics/attempts.jsonl under the
// project's DDx state root.
func AttemptsPath(projectRoot string) string {
	return ddxroot.JoinProject(projectRoot, "metrics", "attempts.jsonl")
}

// AppendRow appends row to .ddx/metrics/attempts.jsonl under projectRoot using
// O_APPEND so concurrent single-writer appends are safe. Creates parent
// directories if needed. Best-effort callers may discard the returned error.
func AppendRow(projectRoot string, row AttemptRow) error {
	path := AttemptsPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	data, err := json.Marshal(row)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

// LoadAttemptIDs reads .ddx/metrics/attempts.jsonl and returns the set of
// attempt_ids already present. Used to deduplicate BackfillFromEvents runs.
// Returns an empty map when the file does not exist.
func LoadAttemptIDs(projectRoot string) (map[string]struct{}, error) {
	path := AttemptsPath(projectRoot)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]struct{}{}, nil
		}
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	seen := map[string]struct{}{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024) // 1 MiB lines max
	for sc.Scan() {
		var row struct {
			AttemptID string `json:"attempt_id"`
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil || row.AttemptID == "" {
			continue
		}
		seen[row.AttemptID] = struct{}{}
	}
	return seen, sc.Err()
}

// LoadRows reads all rows from .ddx/metrics/attempts.jsonl. Malformed lines
// are skipped. Returns an empty slice when the file does not exist.
func LoadRows(projectRoot string) ([]AttemptRow, error) {
	path := AttemptsPath(projectRoot)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []AttemptRow{}, nil
		}
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var rows []AttemptRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		var row AttemptRow
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			continue
		}
		rows = append(rows, row)
	}
	return rows, sc.Err()
}

// BackfillFromEvents processes beads' event streams, appending one AttemptRow
// per attempt found (identified by kind:cost events). Deduplicates by
// attempt_id: rows already in attempts.jsonl are skipped. Returns the count of
// new rows added.
//
// The matching algorithm: for each kind:cost event, scan forward to find the
// next kind:execute-bead event (which carries the finalization outcome). This
// pair gives the attempt's cost/token/timing data and its loop-level outcome.
func BackfillFromEvents(projectRoot string, beads []BeadAttemptEvents) (int, error) {
	seen, err := LoadAttemptIDs(projectRoot)
	if err != nil {
		return 0, err
	}

	var added int
	for _, b := range beads {
		rows := extractRowsFromEvents(b)
		for _, row := range rows {
			if _, exists := seen[row.AttemptID]; exists {
				continue
			}
			if err := AppendRow(projectRoot, row); err != nil {
				return added, err
			}
			seen[row.AttemptID] = struct{}{}
			added++
		}
	}
	return added, nil
}

// extractRowsFromEvents derives one AttemptRow per kind:cost event found in
// b.Events. Each cost event is paired with the immediately following
// kind:execute-bead event to obtain the outcome.
func extractRowsFromEvents(b BeadAttemptEvents) []AttemptRow {
	var rows []AttemptRow
	for i, ev := range b.Events {
		if ev.Kind != "cost" {
			continue
		}
		var cost costEventBody
		if err := json.Unmarshal([]byte(ev.Body), &cost); err != nil || cost.AttemptID == "" {
			continue
		}

		// Scan forward for the next execute-bead event to get outcome.
		outcome := ""
		for _, next := range b.Events[i+1:] {
			if next.Kind == "execute-bead" {
				outcome = next.Summary
				break
			}
		}

		tsEnd := ""
		if !ev.CreatedAt.IsZero() {
			tsEnd = ev.CreatedAt.UTC().Format(time.RFC3339)
		}
		// Approximate ts_start from ts_end and duration.
		tsStart := ""
		if !ev.CreatedAt.IsZero() && cost.DurationMS > 0 {
			start := ev.CreatedAt.UTC().Add(-time.Duration(cost.DurationMS) * time.Millisecond)
			tsStart = start.Format(time.RFC3339)
		}

		total := cost.TotalTokens
		if total == 0 {
			total = cost.InputTokens + cost.OutputTokens
		}

		row := AttemptRow{
			SchemaVersion: SchemaVersion,
			AttemptID:     cost.AttemptID,
			BeadID:        b.BeadID,
			SpecID:        b.SpecID,
			TSStart:       tsStart,
			TSEnd:         tsEnd,
			Model:         cost.Model,
			Harness:       cost.Harness,
			Provider:      cost.Provider,
			Outcome:       outcome,
			ExitCode:      cost.ExitCode,
			CostUSD:       cost.CostUSD,
			DurationMS:    cost.DurationMS,
			InputTokens:   cost.InputTokens,
			OutputTokens:  cost.OutputTokens,
			TotalTokens:   total,
		}
		rows = append(rows, row)
	}
	return rows
}

// Rfc3339 formats a time.Time as RFC3339 UTC; returns "" for the zero value.
func Rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
