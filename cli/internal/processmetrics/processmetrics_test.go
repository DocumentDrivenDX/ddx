package processmetrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeMetricsFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
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

	return dir
}

func TestServiceDerivesCostLifecycleAndRework(t *testing.T) {
	dir := writeMetricsFixture(t)
	svc := New(dir)

	cost, err := svc.Cost(Query{FeatureID: "FEAT-001"})
	require.NoError(t, err)
	require.Len(t, cost.Beads, 2)
	require.Len(t, cost.Features, 1)
	require.Equal(t, "feature", cost.Scope)

	first := cost.Beads[0]
	require.Equal(t, "bx-001", first.BeadID)
	require.Equal(t, State(stateKnown), first.CostState)
	require.NotNil(t, first.CostUSD)
	require.InDelta(t, 2.5, *first.CostUSD, 1e-9)

	second := cost.Beads[1]
	require.Equal(t, "bx-002", second.BeadID)
	require.Equal(t, State(stateEstimated), second.CostState)
	require.NotNil(t, second.CostUSD)
	require.InDelta(t, 0.018, *second.CostUSD, 1e-9)
	require.InDelta(t, 2.518, cost.Features[0].CostUSDValue(), 1e-9)

	cycle, err := svc.CycleTime(Query{})
	require.NoError(t, err)
	require.Len(t, cycle.Beads, 2)
	require.Equal(t, int64(3600000), *cycle.Beads[0].CycleTimeMS)
	require.Equal(t, 1, *cycle.Beads[0].ReopenCount)
	require.Equal(t, int64(5400000), *cycle.Beads[1].CycleTimeMS)
	require.Equal(t, 0, *cycle.Beads[1].ReopenCount)

	rework, err := svc.Rework(Query{})
	require.NoError(t, err)
	require.Len(t, rework.Beads, 2)
	require.Equal(t, 2, rework.Summary.KnownClosed)
	require.Equal(t, 1, rework.Summary.KnownReopened)
	require.InDelta(t, 0.5, rework.Summary.ReopenRate, 1e-9)
	require.Equal(t, 1, rework.Summary.RevisionCount)

	summary, err := svc.Summary(Query{})
	require.NoError(t, err)
	require.Equal(t, 2, summary.Beads.Total)
	require.Equal(t, 2, summary.Beads.Closed)
	require.Equal(t, 1, summary.Beads.Reopened)
	require.Equal(t, 3, summary.Sessions.Total)
	require.Equal(t, 2, summary.Sessions.Correlated)
	require.Equal(t, 1, summary.Sessions.Uncorrelated)
	require.InDelta(t, 2.518, summary.Cost.KnownCostUSD+summary.Cost.EstimatedCostUSD, 1e-9)
	require.Equal(t, 2, summary.CycleTime.KnownCount)
	require.Equal(t, 2, summary.Rework.KnownClosed)
}

func TestParseSince(t *testing.T) {
	got, err := ParseSince("2026-01-02")
	require.NoError(t, err)
	require.Equal(t, 2026, got.Year())
	require.Equal(t, time.January, got.Month())
	require.Equal(t, 2, got.Day())
}

func (r FeatureCostRow) CostUSDValue() float64 {
	if r.CostUSD == nil {
		return 0
	}
	return *r.CostUSD
}
