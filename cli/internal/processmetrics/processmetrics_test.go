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

func writeZeroCostFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(filepath.Join(ddxDir, "agent-logs"), 0o755))

	beads := []string{
		`{"id":"bx-010","title":"Zero-cost feature","status":"closed","priority":1,"issue_type":"task","created_at":"2026-02-01T00:00:00Z","updated_at":"2026-02-01T01:00:00Z","spec-id":"FEAT-010","session_id":"as-010","events":[{"kind":"status","summary":"closed","created_at":"2026-02-01T01:00:00Z","source":"test"}]}`,
		`{"id":"bx-011","title":"Unknown-cost feature","status":"closed","priority":1,"issue_type":"task","created_at":"2026-02-02T00:00:00Z","updated_at":"2026-02-02T01:00:00Z","spec-id":"FEAT-011","session_id":"as-011","events":[{"kind":"status","summary":"closed","created_at":"2026-02-02T01:00:00Z","source":"test"}]}`,
	}
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(beads[0]+"\n"+beads[1]+"\n"), 0o644))

	sessions := []string{
		`{"id":"as-010","timestamp":"2026-02-01T00:30:00Z","harness":"codex","model":"gpt-5.4","prompt_len":100,"input_tokens":100,"output_tokens":50,"total_tokens":150,"cost_usd":0,"duration_ms":1000,"exit_code":0,"correlation":{"bead_id":"bx-010"}}`,
		`{"id":"as-011","timestamp":"2026-02-02T00:30:00Z","harness":"codex","model":"qwen/qwen3-coder-30b","prompt_len":100,"input_tokens":100,"output_tokens":50,"total_tokens":150,"cost_usd":-1,"duration_ms":1000,"exit_code":0,"correlation":{"bead_id":"bx-011"}}`,
	}
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "agent-logs", "sessions.jsonl"), []byte(sessions[0]+"\n"+sessions[1]+"\n"), 0o644))

	return dir
}

func writeWindowedMetricsFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(filepath.Join(ddxDir, "agent-logs"), 0o755))

	beads := []string{
		`{"id":"bx-201","title":"Pre-cutoff feature","status":"closed","priority":1,"issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T03:30:00Z","labels":["helix"],"spec-id":"FEAT-201","session_id":"as-201","events":[{"kind":"status","summary":"closed","created_at":"2026-01-01T01:00:00Z","source":"test"}]}`,
		`{"id":"bx-202","title":"Post-cutoff feature","status":"closed","priority":1,"issue_type":"task","created_at":"2026-03-01T00:00:00Z","updated_at":"2026-03-01T01:30:00Z","spec-id":"FEAT-202","session_id":"as-202","events":[{"kind":"status","summary":"closed","created_at":"2026-03-01T01:30:00Z","source":"test"}]}`,
	}
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(beads[0]+"\n"+beads[1]+"\n"), 0o644))

	sessions := []string{
		`{"id":"as-201","timestamp":"2026-01-01T00:30:00Z","harness":"codex","model":"gpt-5.4","prompt_len":100,"input_tokens":100,"output_tokens":50,"total_tokens":150,"cost_usd":2.5,"duration_ms":1000,"exit_code":0,"correlation":{"bead_id":"bx-201"}}`,
		`{"id":"as-202","timestamp":"2026-03-01T00:45:00Z","harness":"claude","model":"claude-sonnet-4-6","prompt_len":120,"input_tokens":1000,"output_tokens":1000,"total_tokens":2000,"cost_usd":1.0,"duration_ms":2000,"exit_code":0,"correlation":{"bead_id":"bx-202"}}`,
	}
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "agent-logs", "sessions.jsonl"), []byte(sessions[0]+"\n"+sessions[1]+"\n"), 0o644))

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

func TestServiceMarksZeroCostKnownAndSentinelUnknown(t *testing.T) {
	dir := writeZeroCostFixture(t)
	svc := New(dir)

	cost, err := svc.Cost(Query{})
	require.NoError(t, err)
	require.Len(t, cost.Beads, 2)
	require.Len(t, cost.Features, 2)
	require.Equal(t, "all", cost.Scope)

	zero := cost.Beads[0]
	require.Equal(t, "bx-010", zero.BeadID)
	require.Equal(t, State(stateKnown), zero.CostState)
	require.NotNil(t, zero.CostUSD)
	require.InDelta(t, 0, *zero.CostUSD, 1e-9)
	require.Zero(t, zero.UnknownSessions)

	unknown := cost.Beads[1]
	require.Equal(t, "bx-011", unknown.BeadID)
	require.Equal(t, State(stateUnknown), unknown.CostState)
	require.Nil(t, unknown.CostUSD)
	require.Equal(t, 1, unknown.UnknownSessions)

	summary, err := svc.Summary(Query{})
	require.NoError(t, err)
	require.Equal(t, 2, summary.Sessions.Total)
	require.Equal(t, 2, summary.Sessions.Correlated)
	require.Equal(t, 0, summary.Sessions.Uncorrelated)
	require.Equal(t, 1, summary.Sessions.KnownCost)
	require.Equal(t, 0, summary.Sessions.EstimatedCost)
	require.Equal(t, 1, summary.Sessions.UnknownCost)
	require.InDelta(t, 0, summary.Sessions.CostUSD, 1e-9)
	require.Equal(t, 1, summary.Beads.KnownCost)
	require.Equal(t, 0, summary.Beads.EstimatedCost)
	require.Equal(t, 1, summary.Beads.UnknownCost)
	require.InDelta(t, 0, summary.Cost.KnownCostUSD, 1e-9)
	require.InDelta(t, 0, summary.Cost.EstimatedCostUSD, 1e-9)
	require.Equal(t, 1, summary.Cost.UnknownBeads)
}

func TestServiceSummaryHonorsSinceWindow(t *testing.T) {
	dir := writeWindowedMetricsFixture(t)
	svc := New(dir)

	since, err := ParseSince("2026-02-01")
	require.NoError(t, err)

	summary, err := svc.Summary(Query{Since: since, HasSince: true})
	require.NoError(t, err)

	require.Equal(t, 1, summary.Beads.Total)
	require.Equal(t, 0, summary.Beads.Open)
	require.Equal(t, 0, summary.Beads.InProgress)
	require.Equal(t, 1, summary.Beads.Closed)
	require.Equal(t, 0, summary.Beads.Reopened)
	require.Equal(t, 1, summary.Beads.KnownCycleTime)
	require.Equal(t, 0, summary.Beads.UnknownCycleTime)
	require.Equal(t, 1, summary.Beads.KnownCost)
	require.Equal(t, 0, summary.Beads.EstimatedCost)
	require.Equal(t, 0, summary.Beads.UnknownCost)

	require.Equal(t, 1, summary.Sessions.Total)
	require.Equal(t, 1, summary.Sessions.Correlated)
	require.Equal(t, 0, summary.Sessions.Uncorrelated)
	require.Equal(t, 1000, summary.Sessions.InputTokens)
	require.Equal(t, 1000, summary.Sessions.OutputTokens)
	require.Equal(t, 2000, summary.Sessions.TotalTokens)
	require.Equal(t, 1, summary.Sessions.KnownCost)
	require.Equal(t, 0, summary.Sessions.EstimatedCost)
	require.Equal(t, 0, summary.Sessions.UnknownCost)
	require.InDelta(t, 1.0, summary.Sessions.CostUSD, 1e-9)

	require.Equal(t, 1, summary.Cost.Beads)
	require.Equal(t, 1, summary.Cost.Features)
	require.InDelta(t, 1.0, summary.Cost.KnownCostUSD, 1e-9)
	require.InDelta(t, 0.0, summary.Cost.EstimatedCostUSD, 1e-9)
	require.Equal(t, 0, summary.Cost.UnknownBeads)

	require.Equal(t, 1, summary.CycleTime.KnownCount)
	require.Equal(t, 0, summary.CycleTime.UnknownCount)
	require.NotNil(t, summary.CycleTime.AverageMS)
	require.NotNil(t, summary.CycleTime.MinMS)
	require.NotNil(t, summary.CycleTime.MaxMS)
	require.Equal(t, int64(5400000), *summary.CycleTime.AverageMS)
	require.Equal(t, int64(5400000), *summary.CycleTime.MinMS)
	require.Equal(t, int64(5400000), *summary.CycleTime.MaxMS)

	require.Equal(t, 1, summary.Rework.KnownClosed)
	require.Equal(t, 0, summary.Rework.KnownReopened)
	require.Equal(t, 0, summary.Rework.UnknownCount)
	require.InDelta(t, 0.0, summary.Rework.ReopenRate, 1e-9)
	require.Equal(t, 0, summary.Rework.RevisionCount)
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
