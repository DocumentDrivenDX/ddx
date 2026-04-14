package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ExecuteBeadIterationBlock is the machine-readable JSON payload embedded in
// execute-bead iteration commit messages as the "ddx-iteration:" trailer value.
// All fields are projected from result.json; the struct is the canonical source
// for both commit-message rendering and forward-compatibility testing.
type ExecuteBeadIterationBlock struct {
	BeadID              string  `json:"bead_id"`
	AttemptID           string  `json:"attempt_id"`
	SessionID           string  `json:"session_id,omitempty"`
	Harness             string  `json:"harness,omitempty"`
	Provider            string  `json:"provider,omitempty"`
	Model               string  `json:"model,omitempty"`
	TotalTokens         int     `json:"total_tokens"`
	CostUSD             float64 `json:"cost_usd"`
	BaseRev             string  `json:"base_rev"`
	ResultRev           string  `json:"result_rev,omitempty"`
	RequiredExecSummary string  `json:"required_exec_summary"`
	Outcome             string  `json:"outcome"`
	ExecutionBundle     string  `json:"execution_bundle"`
}

// BuildIterationCommitMessage renders the canonical execute-bead iteration
// commit message from res. All values are projected from the result — the same
// fields that are written to result.json — so the message is re-derivable from
// the tracked artifact file without re-running the agent.
//
// The message includes:
//   - A structured subject line referencing the bead ID
//   - The ddx-iteration JSON block (compact agent/provider metadata)
//   - The canonical Git trailer set (Ddx-Attempt-Id, Ddx-Worker-Id,
//     Ddx-Harness, Ddx-Model, Ddx-Result-Status)
//
// Unknown fields are handled deterministically: ResultRev is omitted when
// empty; RequiredExecSummary defaults to "skipped" when unset; WorkerID falls
// back to SessionID when no distinct worker identity exists.
func BuildIterationCommitMessage(res *ExecuteBeadResult) string {
	block := ExecuteBeadIterationBlock{
		BeadID:              res.BeadID,
		AttemptID:           res.AttemptID,
		SessionID:           res.SessionID,
		Harness:             res.Harness,
		Provider:            res.Provider,
		Model:               res.Model,
		TotalTokens:         res.Tokens,
		CostUSD:             res.CostUSD,
		BaseRev:             res.BaseRev,
		ResultRev:           res.ResultRev,
		RequiredExecSummary: iterationRequiredExecSummary(res),
		Outcome:             res.Outcome,
		ExecutionBundle:     res.ExecutionDir,
	}

	blockJSON, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		// Fallback: minimal inline block so the commit is never trailer-free.
		blockJSON = []byte(fmt.Sprintf(`{"bead_id":%q,"attempt_id":%q}`, res.BeadID, res.AttemptID))
	}

	// Ddx-Worker-Id falls back to SessionID when no distinct worker identity exists.
	workerID := res.WorkerID
	if workerID == "" {
		workerID = res.SessionID
	}

	var b strings.Builder
	b.WriteString("chore: execute-bead iteration ")
	b.WriteString(res.BeadID)
	b.WriteString("\n\nddx-iteration: ")
	b.WriteString(string(blockJSON))
	// Blank line between the JSON block and the canonical trailer set is required
	// by git-interpret-trailers so that the trailers are parsed correctly.
	b.WriteString("\n\nDdx-Attempt-Id: ")
	b.WriteString(res.AttemptID)
	b.WriteString("\nDdx-Worker-Id: ")
	b.WriteString(workerID)
	b.WriteString("\nDdx-Harness: ")
	b.WriteString(res.Harness)
	b.WriteString("\nDdx-Model: ")
	b.WriteString(res.Model)
	b.WriteString("\nDdx-Result-Status: ")
	b.WriteString(res.Status)
	return b.String()
}

// iterationRequiredExecSummary returns the required_exec_summary value for the
// ddx-iteration block. When the orchestrator has not yet evaluated gates (i.e.
// the preliminary result is written before the synthesized commit), the field
// defaults to "skipped" per the documented contract. After ApplyLandingToResult
// the field is populated with the real summary and the final result.json is
// written.
func iterationRequiredExecSummary(res *ExecuteBeadResult) string {
	if res.RequiredExecSummary != "" {
		return res.RequiredExecSummary
	}
	return "skipped"
}

// BuildCommitMessageFromResultFile reads the result.json artifact at resultPath
// and renders the canonical execute-bead iteration commit message from it.
// This is the file-sourced rendering path: callers must write result.json before
// calling this function. The message is deterministic for a given result.json —
// anyone can re-derive it from the tracked bundle without re-running the agent.
func BuildCommitMessageFromResultFile(resultPath string) (string, error) {
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		return "", fmt.Errorf("reading result file %s: %w", resultPath, err)
	}
	var res ExecuteBeadResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("parsing result file %s: %w", resultPath, err)
	}
	return BuildIterationCommitMessage(&res), nil
}
