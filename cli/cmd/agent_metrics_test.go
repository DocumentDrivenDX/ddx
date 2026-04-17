package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeExecResult writes a minimal result.json fixture for tier-success tests.
func writeExecResult(t *testing.T, execRoot, attemptID string, res map[string]any) {
	t.Helper()
	dir := filepath.Join(execRoot, attemptID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	raw, err := json.Marshal(res)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "result.json"), raw, 0o644))
}

func TestAgentMetricsTierSuccess(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := t.TempDir()
	execRoot := filepath.Join(dir, ".ddx", "executions")

	// claude/sonnet: 2 attempts, 1 success.
	writeExecResult(t, execRoot, "20260401T100000-aaaa0001", map[string]any{
		"bead_id":     "ddx-1",
		"harness":     "claude",
		"model":       "sonnet",
		"outcome":     "task_succeeded",
		"duration_ms": 100000,
		"cost_usd":    1.0,
	})
	writeExecResult(t, execRoot, "20260401T110000-aaaa0002", map[string]any{
		"bead_id":     "ddx-2",
		"harness":     "claude",
		"model":       "sonnet",
		"outcome":     "task_failed",
		"duration_ms": 200000,
		"cost_usd":    2.0,
	})
	// claude/opus: 1 attempt, 1 success.
	writeExecResult(t, execRoot, "20260401T120000-aaaa0003", map[string]any{
		"bead_id":     "ddx-3",
		"harness":     "claude",
		"model":       "opus",
		"outcome":     "task_succeeded",
		"duration_ms": 300000,
		"cost_usd":    5.0,
	})
	// agent (no model): 1 attempt, 0 successes (error).
	writeExecResult(t, execRoot, "20260401T130000-aaaa0004", map[string]any{
		"bead_id":     "ddx-4",
		"harness":     "agent",
		"outcome":     "error",
		"duration_ms": 1000,
	})

	// Malformed result.json (should be skipped).
	badDir := filepath.Join(execRoot, "20260401T140000-aaaa0005")
	require.NoError(t, os.MkdirAll(badDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(badDir, "result.json"), []byte("not json"), 0o644))

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "metrics", "tier-success", "--json")
	require.NoError(t, err)

	var rows []tierSuccessRow
	require.NoError(t, json.Unmarshal([]byte(output), &rows))

	byTier := map[string]tierSuccessRow{}
	for _, r := range rows {
		byTier[r.Tier] = r
	}

	require.Contains(t, byTier, "claude/sonnet")
	assert.Equal(t, 2, byTier["claude/sonnet"].Attempts)
	assert.Equal(t, 1, byTier["claude/sonnet"].Successes)
	assert.InDelta(t, 0.5, byTier["claude/sonnet"].SuccessRate, 0.0001)
	assert.InDelta(t, 1.5, byTier["claude/sonnet"].AvgCostUSD, 0.0001)
	assert.InDelta(t, 150000.0, byTier["claude/sonnet"].AvgDurationMS, 0.0001)

	require.Contains(t, byTier, "claude/opus")
	assert.Equal(t, 1, byTier["claude/opus"].Attempts)
	assert.Equal(t, 1, byTier["claude/opus"].Successes)
	assert.InDelta(t, 1.0, byTier["claude/opus"].SuccessRate, 0.0001)

	require.Contains(t, byTier, "agent")
	assert.Equal(t, 1, byTier["agent"].Attempts)
	assert.Equal(t, 0, byTier["agent"].Successes)
	assert.InDelta(t, 0.0, byTier["agent"].SuccessRate, 0.0001)

	// Table output shape: has header columns for tier, attempts, successes,
	// success_rate, avg_cost_usd, avg_duration_ms.
	tableCmd := NewCommandFactory(dir).NewRootCommand()
	tableOut, err := executeCommand(tableCmd, "agent", "metrics", "tier-success")
	require.NoError(t, err)
	header := strings.SplitN(tableOut, "\n", 2)[0]
	for _, col := range []string{"TIER", "ATTEMPTS", "SUCCESSES", "SUCCESS_RATE", "AVG_COST_USD", "AVG_DURATION_MS"} {
		assert.Contains(t, header, col)
	}

	// --last 1 keeps only the most recent attempt (agent / error).
	lastCmd := NewCommandFactory(dir).NewRootCommand()
	lastOut, err := executeCommand(lastCmd, "agent", "metrics", "tier-success", "--last", "1", "--json")
	require.NoError(t, err)
	var lastRows []tierSuccessRow
	require.NoError(t, json.Unmarshal([]byte(lastOut), &lastRows))
	require.Len(t, lastRows, 1)
	assert.Equal(t, "agent", lastRows[0].Tier)
	assert.Equal(t, 1, lastRows[0].Attempts)
	assert.Equal(t, 0, lastRows[0].Successes)

	// Empty executions dir returns an empty list, not an error.
	empty := t.TempDir()
	emptyCmd := NewCommandFactory(empty).NewRootCommand()
	emptyOut, err := executeCommand(emptyCmd, "agent", "metrics", "tier-success", "--json")
	require.NoError(t, err)
	var emptyRows []tierSuccessRow
	require.NoError(t, json.Unmarshal([]byte(emptyOut), &emptyRows))
	assert.Empty(t, emptyRows)
}

// TestAgentMetricsTierSuccessFailureModes verifies the failure_mode
// breakdown is aggregated per tier and surfaced in both JSON and table
// output. Each recorded failure_mode contributes to a count under its
// tier's FailureModes map; successes do not contribute. This is the
// measurement surface for FEAT-routing-visibility / failure taxonomy.
func TestAgentMetricsTierSuccessFailureModes(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := t.TempDir()
	execRoot := filepath.Join(dir, ".ddx", "executions")

	// claude/sonnet tier: mixed outcomes with distinct failure modes.
	writeExecResult(t, execRoot, "20260401T100000-bbbb0001", map[string]any{
		"bead_id": "ddx-1", "harness": "claude", "model": "sonnet",
		"outcome": "task_succeeded",
	})
	writeExecResult(t, execRoot, "20260401T100001-bbbb0002", map[string]any{
		"bead_id": "ddx-2", "harness": "claude", "model": "sonnet",
		"outcome": "task_failed", "failure_mode": "context_overflow",
	})
	writeExecResult(t, execRoot, "20260401T100002-bbbb0003", map[string]any{
		"bead_id": "ddx-3", "harness": "claude", "model": "sonnet",
		"outcome": "task_failed", "failure_mode": "context_overflow",
	})
	writeExecResult(t, execRoot, "20260401T100003-bbbb0004", map[string]any{
		"bead_id": "ddx-4", "harness": "claude", "model": "sonnet",
		"outcome": "preserved", "failure_mode": "merge_conflict",
	})
	// agent tier: a single no_changes failure mode.
	writeExecResult(t, execRoot, "20260401T100004-bbbb0005", map[string]any{
		"bead_id": "ddx-5", "harness": "agent",
		"outcome": "task_no_changes", "failure_mode": "no_changes",
	})

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "metrics", "tier-success", "--json")
	require.NoError(t, err)

	var rows []tierSuccessRow
	require.NoError(t, json.Unmarshal([]byte(output), &rows))

	byTier := map[string]tierSuccessRow{}
	for _, r := range rows {
		byTier[r.Tier] = r
	}

	require.Contains(t, byTier, "claude/sonnet")
	sonnet := byTier["claude/sonnet"]
	assert.Equal(t, 4, sonnet.Attempts)
	assert.Equal(t, 1, sonnet.Successes)
	require.NotNil(t, sonnet.FailureModes)
	assert.Equal(t, 2, sonnet.FailureModes["context_overflow"])
	assert.Equal(t, 1, sonnet.FailureModes["merge_conflict"])
	// The one success contributes no failure_mode entry.
	assert.NotContains(t, sonnet.FailureModes, "")

	require.Contains(t, byTier, "agent")
	agentRow := byTier["agent"]
	assert.Equal(t, 1, agentRow.Attempts)
	require.NotNil(t, agentRow.FailureModes)
	assert.Equal(t, 1, agentRow.FailureModes["no_changes"])

	// Table output includes a FAILURE_MODES column and the per-tier
	// mode=count breakdown is rendered stably (sorted by mode name).
	tableCmd := NewCommandFactory(dir).NewRootCommand()
	tableOut, err := executeCommand(tableCmd, "agent", "metrics", "tier-success")
	require.NoError(t, err)
	header := strings.SplitN(tableOut, "\n", 2)[0]
	assert.Contains(t, header, "FAILURE_MODES")
	assert.Contains(t, tableOut, "context_overflow=2")
	assert.Contains(t, tableOut, "merge_conflict=1")
	assert.Contains(t, tableOut, "no_changes=1")
}

// writeBeadJSONL appends one bead JSON line to .ddx/beads.jsonl under dir.
// Each bead is expressed as a raw JSON string so tests can write the full
// event timeline (kind:routing, kind:review) verbatim — matching the
// on-disk shape produced by the executor.
func writeBeadJSONL(t *testing.T, dir string, beadJSON string) {
	t.Helper()
	ddx := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddx, 0o755))
	path := filepath.Join(ddx, "beads.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	defer f.Close()
	_, err = f.WriteString(beadJSON + "\n")
	require.NoError(t, err)
}

// TestAgentMetricsReviewOutcomes verifies the review-outcomes subcommand
// aggregates kind:review verdicts per originating harness/model tier. The
// originating tier is the most recent kind:routing event preceding each
// review on the same bead; rows include reviews/approvals/rejections and an
// approval_rate.
func TestAgentMetricsReviewOutcomes(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := t.TempDir()

	// Bead 1 — claude/sonnet routed → APPROVE.
	writeBeadJSONL(t, dir, `{"id":"bead-1","title":"b1","status":"closed","priority":2,"issue_type":"task","created_at":"2026-04-15T00:00:00Z","updated_at":"2026-04-15T00:00:00Z","events":[`+
		`{"kind":"routing","summary":"r","body":"{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\"}","created_at":"2026-04-15T00:01:00Z"},`+
		`{"kind":"review","summary":"APPROVE","body":"### Verdict: APPROVE","created_at":"2026-04-15T00:02:00Z"}`+
		`]}`)

	// Bead 2 — claude/sonnet routed → REQUEST_CHANGES (approve_with_edits).
	writeBeadJSONL(t, dir, `{"id":"bead-2","title":"b2","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-15T00:00:00Z","updated_at":"2026-04-15T00:00:00Z","events":[`+
		`{"kind":"routing","summary":"r","body":"{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\"}","created_at":"2026-04-15T01:00:00Z"},`+
		`{"kind":"review","summary":"REQUEST_CHANGES","body":"needs fixes","created_at":"2026-04-15T01:30:00Z"}`+
		`]}`)

	// Bead 3 — escalation: first attempt sonnet routed then BLOCK; reopened
	// with opus routing + APPROVE. Each review attributes to the routing
	// that immediately precedes it.
	writeBeadJSONL(t, dir, `{"id":"bead-3","title":"b3","status":"closed","priority":2,"issue_type":"task","created_at":"2026-04-15T00:00:00Z","updated_at":"2026-04-15T00:00:00Z","events":[`+
		`{"kind":"routing","summary":"r","body":"{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\"}","created_at":"2026-04-15T02:00:00Z"},`+
		`{"kind":"review","summary":"BLOCK","body":"reject","created_at":"2026-04-15T02:30:00Z"},`+
		`{"kind":"routing","summary":"r","body":"{\"resolved_provider\":\"claude\",\"resolved_model\":\"opus\"}","created_at":"2026-04-15T03:00:00Z"},`+
		`{"kind":"review","summary":"APPROVE","body":"### Verdict: APPROVE","created_at":"2026-04-15T03:30:00Z"}`+
		`]}`)

	// Bead 4 — review with no preceding routing event → "unknown" tier.
	writeBeadJSONL(t, dir, `{"id":"bead-4","title":"b4","status":"closed","priority":2,"issue_type":"task","created_at":"2026-04-15T00:00:00Z","updated_at":"2026-04-15T00:00:00Z","events":[`+
		`{"kind":"review","summary":"APPROVE","body":"ok","created_at":"2026-04-15T04:00:00Z"}`+
		`]}`)

	// Bead 5 — routing without any review (must not appear in output).
	writeBeadJSONL(t, dir, `{"id":"bead-5","title":"b5","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-15T00:00:00Z","updated_at":"2026-04-15T00:00:00Z","events":[`+
		`{"kind":"routing","summary":"r","body":"{\"resolved_provider\":\"agent\",\"resolved_model\":\"\"}","created_at":"2026-04-15T05:00:00Z"}`+
		`]}`)

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "metrics", "review-outcomes", "--json")
	require.NoError(t, err)

	var rows []reviewOutcomesRow
	require.NoError(t, json.Unmarshal([]byte(output), &rows))

	byTier := map[string]reviewOutcomesRow{}
	for _, r := range rows {
		byTier[r.Tier] = r
	}

	// claude/sonnet: 3 reviews (bead-1 APPROVE, bead-2 REQUEST_CHANGES,
	// bead-3 BLOCK) → 1 approval, 2 rejections.
	require.Contains(t, byTier, "claude/sonnet")
	sonnet := byTier["claude/sonnet"]
	assert.Equal(t, "claude", sonnet.Harness)
	assert.Equal(t, "sonnet", sonnet.Model)
	assert.Equal(t, 3, sonnet.Reviews)
	assert.Equal(t, 1, sonnet.Approvals)
	assert.Equal(t, 2, sonnet.Rejections)
	assert.InDelta(t, 1.0/3.0, sonnet.ApprovalRate, 0.0001)

	// claude/opus: 1 review, 1 approval (bead-3 second review).
	require.Contains(t, byTier, "claude/opus")
	opus := byTier["claude/opus"]
	assert.Equal(t, 1, opus.Reviews)
	assert.Equal(t, 1, opus.Approvals)
	assert.Equal(t, 0, opus.Rejections)
	assert.InDelta(t, 1.0, opus.ApprovalRate, 0.0001)

	// unknown tier: bead-4 review with no preceding routing.
	require.Contains(t, byTier, "unknown")
	unk := byTier["unknown"]
	assert.Equal(t, 1, unk.Reviews)
	assert.Equal(t, 1, unk.Approvals)

	// Bead-5 contributed routing only (no review) so it must not
	// surface as its own tier row.
	_, hasAgent := byTier["agent"]
	assert.False(t, hasAgent, "tiers without any reviews must not appear")

	// Table output exposes the required columns.
	tableCmd := NewCommandFactory(dir).NewRootCommand()
	tableOut, err := executeCommand(tableCmd, "agent", "metrics", "review-outcomes")
	require.NoError(t, err)
	header := strings.SplitN(tableOut, "\n", 2)[0]
	for _, col := range []string{"TIER", "REVIEWS", "APPROVALS", "REJECTIONS", "APPROVAL_RATE"} {
		assert.Contains(t, header, col)
	}
	assert.Contains(t, tableOut, "claude/sonnet")
	assert.Contains(t, tableOut, "claude/opus")

	// Empty .ddx returns no rows, not an error.
	empty := t.TempDir()
	emptyCmd := NewCommandFactory(empty).NewRootCommand()
	emptyOut, err := executeCommand(emptyCmd, "agent", "metrics", "review-outcomes", "--json")
	require.NoError(t, err)
	var emptyRows []reviewOutcomesRow
	require.NoError(t, json.Unmarshal([]byte(emptyOut), &emptyRows))
	assert.Empty(t, emptyRows)
}

// TestCostEfficiency verifies the cost-efficiency subcommand aggregates
// per-bead spend across all attempts: single-attempt success, an escalation
// chain (two failures + one success), and an all-failure bead. Wasted cost
// is the sum of cost_usd for attempts where outcome != task_succeeded; the
// final tier reflects the most recent attempt for the bead.
func TestCostEfficiency(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := t.TempDir()
	execRoot := filepath.Join(dir, ".ddx", "executions")

	// bead-success: one attempt, succeeded on claude/sonnet.
	writeExecResult(t, execRoot, "20260401T100000-cccc0001", map[string]any{
		"bead_id":  "bead-success",
		"harness":  "claude",
		"model":    "sonnet",
		"outcome":  "task_succeeded",
		"cost_usd": 1.50,
	})

	// bead-escalated: sonnet failed, sonnet failed again, opus succeeded.
	writeExecResult(t, execRoot, "20260401T110000-cccc0002", map[string]any{
		"bead_id":  "bead-escalated",
		"harness":  "claude",
		"model":    "sonnet",
		"outcome":  "task_failed",
		"cost_usd": 0.75,
	})
	writeExecResult(t, execRoot, "20260401T120000-cccc0003", map[string]any{
		"bead_id":  "bead-escalated",
		"harness":  "claude",
		"model":    "sonnet",
		"outcome":  "preserved",
		"cost_usd": 0.80,
	})
	writeExecResult(t, execRoot, "20260401T130000-cccc0004", map[string]any{
		"bead_id":  "bead-escalated",
		"harness":  "claude",
		"model":    "opus",
		"outcome":  "task_succeeded",
		"cost_usd": 4.00,
	})

	// bead-stuck: every attempt failed.
	writeExecResult(t, execRoot, "20260401T140000-cccc0005", map[string]any{
		"bead_id":  "bead-stuck",
		"harness":  "claude",
		"model":    "sonnet",
		"outcome":  "task_failed",
		"cost_usd": 1.00,
	})
	writeExecResult(t, execRoot, "20260401T150000-cccc0006", map[string]any{
		"bead_id":  "bead-stuck",
		"harness":  "claude",
		"model":    "opus",
		"outcome":  "error",
		"cost_usd": 3.00,
	})

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "metrics", "cost-efficiency", "--json")
	require.NoError(t, err)

	var rows []costEfficiencyRow
	require.NoError(t, json.Unmarshal([]byte(output), &rows))

	byBead := map[string]costEfficiencyRow{}
	for _, r := range rows {
		byBead[r.BeadID] = r
	}

	require.Contains(t, byBead, "bead-success")
	s := byBead["bead-success"]
	assert.Equal(t, 1, s.TotalAttempts)
	assert.InDelta(t, 1.50, s.TotalCostUSD, 0.0001)
	assert.InDelta(t, 1.50, s.SuccessfulCostUSD, 0.0001)
	assert.InDelta(t, 0.0, s.WastedCostUSD, 0.0001)
	assert.Equal(t, "claude/sonnet", s.FinalTier)
	assert.Equal(t, "claude", s.FinalHarness)

	require.Contains(t, byBead, "bead-escalated")
	e := byBead["bead-escalated"]
	assert.Equal(t, 3, e.TotalAttempts)
	assert.InDelta(t, 5.55, e.TotalCostUSD, 0.0001)
	assert.InDelta(t, 4.00, e.SuccessfulCostUSD, 0.0001)
	assert.InDelta(t, 1.55, e.WastedCostUSD, 0.0001)
	assert.Equal(t, "claude/opus", e.FinalTier)
	assert.Equal(t, "claude", e.FinalHarness)

	require.Contains(t, byBead, "bead-stuck")
	x := byBead["bead-stuck"]
	assert.Equal(t, 2, x.TotalAttempts)
	assert.InDelta(t, 4.00, x.TotalCostUSD, 0.0001)
	assert.InDelta(t, 0.0, x.SuccessfulCostUSD, 0.0001)
	assert.InDelta(t, 4.00, x.WastedCostUSD, 0.0001)
	assert.Equal(t, "claude/opus", x.FinalTier)

	// Table output contains the required column headers.
	tableCmd := NewCommandFactory(dir).NewRootCommand()
	tableOut, err := executeCommand(tableCmd, "agent", "metrics", "cost-efficiency")
	require.NoError(t, err)
	header := strings.SplitN(tableOut, "\n", 2)[0]
	for _, col := range []string{"BEAD_ID", "TOTAL_ATTEMPTS", "TOTAL_COST_USD", "SUCCESSFUL_COST_USD", "WASTED_COST_USD", "FINAL_TIER", "FINAL_HARNESS"} {
		assert.Contains(t, header, col)
	}
	assert.Contains(t, tableOut, "bead-success")
	assert.Contains(t, tableOut, "bead-escalated")
	assert.Contains(t, tableOut, "bead-stuck")

	// --last 1: only the most recent attempt's bead (bead-stuck @ opus error)
	// is included; its escalation chain (both attempts) still contributes.
	lastCmd := NewCommandFactory(dir).NewRootCommand()
	lastOut, err := executeCommand(lastCmd, "agent", "metrics", "cost-efficiency", "--last", "1", "--json")
	require.NoError(t, err)
	var lastRows []costEfficiencyRow
	require.NoError(t, json.Unmarshal([]byte(lastOut), &lastRows))
	require.Len(t, lastRows, 1)
	assert.Equal(t, "bead-stuck", lastRows[0].BeadID)
	assert.Equal(t, 2, lastRows[0].TotalAttempts)
	assert.InDelta(t, 4.00, lastRows[0].TotalCostUSD, 0.0001)

	// Empty executions dir returns an empty list, not an error.
	empty := t.TempDir()
	emptyCmd := NewCommandFactory(empty).NewRootCommand()
	emptyOut, err := executeCommand(emptyCmd, "agent", "metrics", "cost-efficiency", "--json")
	require.NoError(t, err)
	var emptyRows []costEfficiencyRow
	require.NoError(t, json.Unmarshal([]byte(emptyOut), &emptyRows))
	assert.Empty(t, emptyRows)
}
