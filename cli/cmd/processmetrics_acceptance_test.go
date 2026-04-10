package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/processmetrics"
	"github.com/stretchr/testify/require"
)

func writeProcessMetricsFixture(t *testing.T, workingDir string) {
	t.Helper()

	ddxDir := filepath.Join(workingDir, ".ddx")
	require.NoError(t, os.MkdirAll(filepath.Join(ddxDir, "agent-logs"), 0o755))

	beads := []string{
		`{"id":"bx-001","title":"Feature one","status":"closed","priority":1,"issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T03:30:00Z","labels":["helix"],"spec-id":"FEAT-001","session_id":"as-001","events":[{"kind":"status","summary":"closed","created_at":"2026-01-01T01:00:00Z","source":"test"},{"kind":"status","summary":"open","created_at":"2026-01-01T02:00:00Z","source":"test"},{"kind":"status","summary":"closed","created_at":"2026-01-01T03:00:00Z","source":"test"}]}`,
		`{"id":"bx-002","title":"Feature two","status":"closed","priority":1,"issue_type":"task","created_at":"2026-01-02T00:00:00Z","updated_at":"2026-01-02T01:30:00Z","spec-id":"FEAT-001","session_id":"as-002","events":[{"kind":"status","summary":"closed","created_at":"2026-01-02T01:30:00Z","source":"test"}]}`,
	}
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(beads[0]+"\n"+beads[1]+"\n"), 0o644))

	sessions := []string{
		`{"id":"as-001","timestamp":"2026-01-01T00:30:00Z","harness":"codex","model":"gpt-5.4","prompt_len":100,"input_tokens":100,"output_tokens":50,"total_tokens":150,"cost_usd":2.5,"duration_ms":1000,"exit_code":0,"correlation":{"bead_id":"bx-001"}}`,
		`{"id":"as-002","timestamp":"2026-01-02T00:45:00Z","harness":"claude","model":"claude-sonnet-4-6","prompt_len":120,"input_tokens":1000,"output_tokens":1000,"total_tokens":2000,"duration_ms":2000,"exit_code":0,"correlation":{"bead_id":"bx-002"}}`,
		`{"id":"as-003","timestamp":"2026-01-03T00:00:00Z","harness":"codex","prompt_len":50,"input_tokens":10,"output_tokens":20,"total_tokens":30,"duration_ms":150,"exit_code":0}`,
	}
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "agent-logs", "sessions.jsonl"), []byte(sessions[0]+"\n"+sessions[1]+"\n"+sessions[2]+"\n"), 0o644))
}

func TestMetricsCommandsExposeDerivedProcessMetrics(t *testing.T) {
	workingDir := t.TempDir()
	writeProcessMetricsFixture(t, workingDir)
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	summaryOut, err := runMetricsCommand(t, workingDir, "metrics", "summary", "--json")
	require.NoError(t, err)
	var summary processmetrics.AggregateSummary
	require.NoError(t, json.Unmarshal([]byte(summaryOut), &summary))
	require.Equal(t, 2, summary.Beads.Total)
	require.Equal(t, 2, summary.Beads.Closed)
	require.Equal(t, 1, summary.Beads.Reopened)
	require.Equal(t, 3, summary.Sessions.Total)
	require.Equal(t, 2, summary.Sessions.Correlated)
	require.Equal(t, 1, summary.Sessions.Uncorrelated)

	costOut, err := runMetricsCommand(t, workingDir, "metrics", "cost", "--feature", "FEAT-001", "--json")
	require.NoError(t, err)
	var cost processmetrics.CostReport
	require.NoError(t, json.Unmarshal([]byte(costOut), &cost))
	require.Equal(t, "feature", cost.Scope)
	require.Len(t, cost.Beads, 2)
	require.Len(t, cost.Features, 1)
	require.Equal(t, "FEAT-001", cost.Features[0].SpecID)
	require.Equal(t, "bx-001", cost.Beads[0].BeadID)
	require.Equal(t, "bx-002", cost.Beads[1].BeadID)
	require.Equal(t, processmetrics.State("known"), cost.Beads[0].CostState)
	require.Equal(t, processmetrics.State("estimated"), cost.Beads[1].CostState)

	beadCostOut, err := runMetricsCommand(t, workingDir, "metrics", "cost", "--bead", "bx-002", "--json")
	require.NoError(t, err)
	var beadCost processmetrics.CostReport
	require.NoError(t, json.Unmarshal([]byte(beadCostOut), &beadCost))
	require.Equal(t, "bead", beadCost.Scope)
	require.Equal(t, "bx-002", beadCost.BeadID)
	require.Len(t, beadCost.Beads, 1)
	require.Equal(t, "bx-002", beadCost.Beads[0].BeadID)

	cycleOut, err := runMetricsCommand(t, workingDir, "metrics", "cycle-time", "--json")
	require.NoError(t, err)
	var cycle processmetrics.CycleTimeReport
	require.NoError(t, json.Unmarshal([]byte(cycleOut), &cycle))
	require.Equal(t, 2, cycle.Summary.KnownCount)
	require.Len(t, cycle.Beads, 2)
	require.NotNil(t, cycle.Beads[0].CycleTimeMS)
	require.EqualValues(t, 3600000, *cycle.Beads[0].CycleTimeMS)

	reworkOut, err := runMetricsCommand(t, workingDir, "metrics", "rework", "--json")
	require.NoError(t, err)
	var rework processmetrics.ReworkReport
	require.NoError(t, json.Unmarshal([]byte(reworkOut), &rework))
	require.Equal(t, 2, rework.Summary.KnownClosed)
	require.Equal(t, 1, rework.Summary.KnownReopened)
	require.Equal(t, 1, rework.Summary.RevisionCount)
	require.Len(t, rework.Beads, 2)

	_, err = runMetricsCommand(t, workingDir, "metrics", "cost", "--bead", "bx-001", "--feature", "FEAT-001")
	require.Error(t, err)
}

func runMetricsCommand(t *testing.T, workingDir string, args ...string) (string, error) {
	t.Helper()
	return executeCommand(NewCommandFactory(workingDir).NewRootCommand(), args...)
}
