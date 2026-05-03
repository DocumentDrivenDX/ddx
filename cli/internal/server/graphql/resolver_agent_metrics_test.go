package graphql

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeRun seeds one FEAT-010 run-store record under workingDir.
func writeRun(t *testing.T, workingDir, name, body string) {
	t.Helper()
	dir := filepath.Join(workingDir, ".ddx", "exec", "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// fixtureWorkingDir seeds three try-layer attempts that exercise every
// grouping axis: two attempts at claude/sonnet (one success, one failure),
// one at codex/gpt5 (success). Tokens are populated so the input/output
// token aggregates are non-zero.
func fixtureWorkingDir(t *testing.T) string {
	t.Helper()
	wd := t.TempDir()
	now := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	end := time.Now().UTC().Format(time.RFC3339)
	writeRun(t, wd, "a.json", `{
		"id":"att-1","layer":"try","status":"success","outcome":"merged",
		"bead_id":"ddx-1","harness":"claude","provider":"anthropic","model":"sonnet",
		"cost_usd":1.0,"duration_ms":1000,"exit_code":0,
		"tokens_in":100,"tokens_out":50,
		"started_at":"`+now+`","completed_at":"`+end+`"
	}`)
	writeRun(t, wd, "b.json", `{
		"id":"att-2","layer":"try","status":"execution_failed","outcome":"failed",
		"bead_id":"ddx-2","harness":"claude","provider":"anthropic","model":"sonnet",
		"cost_usd":2.0,"duration_ms":3000,"exit_code":1,
		"tokens_in":200,"tokens_out":80,
		"started_at":"`+now+`","completed_at":"`+end+`"
	}`)
	writeRun(t, wd, "c.json", `{
		"id":"att-3","layer":"try","status":"already_satisfied","outcome":"merged",
		"bead_id":"ddx-3","harness":"codex","provider":"openai","model":"gpt5",
		"cost_usd":0.5,"duration_ms":500,"exit_code":0,
		"tokens_in":40,"tokens_out":20,
		"started_at":"`+now+`","completed_at":"`+end+`"
	}`)
	return wd
}

func newTestResolver(t *testing.T, wd string) *queryResolver {
	t.Helper()
	r, err := NewResolver(emptyStateProvider{}, wd)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return &queryResolver{Resolver: r}
}

// emptyStateProvider satisfies StateProvider with zero-value returns.
// agentMetrics does not touch r.State, so this is sufficient for these
// per-axis tests without depending on a real bead.Store.
type emptyStateProvider struct{}

func (emptyStateProvider) GetNodeSnapshot() NodeStateSnapshot { return NodeStateSnapshot{} }
func (emptyStateProvider) GetProjectSnapshots(bool) []*Project {
	return nil
}
func (emptyStateProvider) GetProjectSnapshotByID(string) (*Project, bool) { return nil, false }
func (emptyStateProvider) GetBeadSnapshots(string, string, string, string) []BeadSnapshot {
	return nil
}
func (emptyStateProvider) GetBeadSnapshotsForProject(string, string, string, string) []BeadSnapshot {
	return nil
}
func (emptyStateProvider) GetBeadSnapshot(string) (*BeadSnapshot, bool) { return nil, false }
func (emptyStateProvider) GetWorkersGraphQL(string) []*Worker           { return nil }
func (emptyStateProvider) GetWorkerGraphQL(string) (*Worker, bool)      { return nil, false }
func (emptyStateProvider) GetWorkerLogGraphQL(string) *WorkerLog        { return nil }
func (emptyStateProvider) GetWorkerProgressGraphQL(string) []*PhaseTransition {
	return nil
}
func (emptyStateProvider) GetWorkerPromptGraphQL(string) string { return "" }
func (emptyStateProvider) GetAgentSessionsGraphQL(*time.Time, *time.Time) []*AgentSession {
	return nil
}
func (emptyStateProvider) GetAgentSessionGraphQL(string) (*AgentSession, bool) {
	return nil, false
}
func (emptyStateProvider) GetSessionsCostSummaryGraphQL(string, *time.Time, *time.Time) *SessionsCostSummary {
	return nil
}
func (emptyStateProvider) GetExecDefinitionsGraphQL(string) []*ExecutionDefinition { return nil }
func (emptyStateProvider) GetExecDefinitionGraphQL(string) (*ExecutionDefinition, bool) {
	return nil, false
}
func (emptyStateProvider) GetExecRunsGraphQL(string, string) []*ExecutionRun { return nil }
func (emptyStateProvider) GetExecRunGraphQL(string) (*ExecutionRun, bool)    { return nil, false }
func (emptyStateProvider) GetExecRunLogGraphQL(string) *ExecutionRunLog      { return nil }
func (emptyStateProvider) GetCoordinatorsGraphQL() []*CoordinatorMetricsEntry {
	return nil
}
func (emptyStateProvider) GetCoordinatorMetricsByProjectGraphQL(string) *CoordinatorMetrics {
	return nil
}

// resetCache clears the process-global cache so each test sees a clean
// slate. Without this, tests that share an axis/window collide on cache
// hits and never rebuild against fresh fixtures.
func resetAgentMetricsCache() {
	agentMetricsCache.Lock()
	agentMetricsCache.byKey = map[string]agentMetricsCacheEntry{}
	agentMetricsCache.Unlock()
}

func TestAgentMetrics_GroupByModel(t *testing.T) {
	resetAgentMetricsCache()
	wd := fixtureWorkingDir(t)
	r := newTestResolver(t, wd)
	res, err := r.AgentMetrics(context.Background(), AgentMetricsWindowW7d, AgentMetricsAxisModel)
	if err != nil {
		t.Fatalf("AgentMetrics: %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("model rows = %d, want 2", len(res.Rows))
	}
	// sonnet bucket: 2 attempts, 1 success.
	row := findRow(t, res.Rows, "sonnet")
	if row.Attempts != 2 || row.Successes != 1 {
		t.Fatalf("sonnet: %+v", row)
	}
	if row.SuccessRate != 0.5 {
		t.Fatalf("sonnet successRate = %f, want 0.5", row.SuccessRate)
	}
	// MeanCost = (1.0 + 2.0)/2 = 1.5; effective = 3.0/1 success = 3.0.
	if row.MeanCostUsd != 1.5 {
		t.Fatalf("sonnet meanCost = %f, want 1.5", row.MeanCostUsd)
	}
	if row.EffectiveCostPerSuccessUsd == nil || *row.EffectiveCostPerSuccessUsd != 3.0 {
		t.Fatalf("sonnet effectiveCost = %v, want 3.0", row.EffectiveCostPerSuccessUsd)
	}
	if row.MeanInputTokens != 150 || row.MeanOutputTokens != 65 {
		t.Fatalf("sonnet tokens: in=%f out=%f", row.MeanInputTokens, row.MeanOutputTokens)
	}
	// gpt5 bucket: 1 attempt, 1 success (already_satisfied counts).
	gpt := findRow(t, res.Rows, "gpt5")
	if gpt.Attempts != 1 || gpt.Successes != 1 {
		t.Fatalf("gpt5: %+v", gpt)
	}
	if res.Window != AgentMetricsWindowW7d || res.GroupBy != AgentMetricsAxisModel {
		t.Fatalf("echo fields: %+v", res)
	}
	if res.Revision == "" {
		t.Fatalf("revision must not be empty")
	}
}

func TestAgentMetrics_GroupByHarness(t *testing.T) {
	resetAgentMetricsCache()
	wd := fixtureWorkingDir(t)
	r := newTestResolver(t, wd)
	res, err := r.AgentMetrics(context.Background(), AgentMetricsWindowW24h, AgentMetricsAxisHarness)
	if err != nil {
		t.Fatalf("AgentMetrics: %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("harness rows = %d, want 2", len(res.Rows))
	}
	claude := findRow(t, res.Rows, "claude")
	if claude.Attempts != 2 {
		t.Fatalf("claude attempts = %d, want 2", claude.Attempts)
	}
	codex := findRow(t, res.Rows, "codex")
	if codex.Attempts != 1 || codex.Successes != 1 {
		t.Fatalf("codex: %+v", codex)
	}
}

func TestAgentMetrics_GroupByProvider(t *testing.T) {
	resetAgentMetricsCache()
	wd := fixtureWorkingDir(t)
	r := newTestResolver(t, wd)
	res, err := r.AgentMetrics(context.Background(), AgentMetricsWindowW7d, AgentMetricsAxisProvider)
	if err != nil {
		t.Fatalf("AgentMetrics: %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("provider rows = %d, want 2", len(res.Rows))
	}
	an := findRow(t, res.Rows, "anthropic")
	if an.Attempts != 2 {
		t.Fatalf("anthropic attempts = %d", an.Attempts)
	}
	op := findRow(t, res.Rows, "openai")
	if op.Attempts != 1 || op.Successes != 1 {
		t.Fatalf("openai: %+v", op)
	}
}

func TestAgentMetrics_GroupByTier(t *testing.T) {
	resetAgentMetricsCache()
	wd := fixtureWorkingDir(t)
	r := newTestResolver(t, wd)
	res, err := r.AgentMetrics(context.Background(), AgentMetricsWindowW30d, AgentMetricsAxisTier)
	if err != nil {
		t.Fatalf("AgentMetrics: %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("tier rows = %d, want 2", len(res.Rows))
	}
	if findRow(t, res.Rows, "claude/sonnet").Attempts != 2 {
		t.Fatalf("claude/sonnet attempts != 2")
	}
	if findRow(t, res.Rows, "codex/gpt5").Attempts != 1 {
		t.Fatalf("codex/gpt5 attempts != 1")
	}
}

func TestAgentMetrics_WindowExcludesOldAttempts(t *testing.T) {
	resetAgentMetricsCache()
	wd := t.TempDir()
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	recent := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	writeRun(t, wd, "old.json", `{
		"id":"old","layer":"try","status":"success",
		"harness":"claude","provider":"anthropic","model":"sonnet",
		"cost_usd":1,"duration_ms":1,"started_at":"`+old+`","completed_at":"`+old+`"
	}`)
	writeRun(t, wd, "new.json", `{
		"id":"new","layer":"try","status":"success",
		"harness":"claude","provider":"anthropic","model":"sonnet",
		"cost_usd":1,"duration_ms":1,"started_at":"`+recent+`","completed_at":"`+recent+`"
	}`)
	r := newTestResolver(t, wd)
	res, err := r.AgentMetrics(context.Background(), AgentMetricsWindowW24h, AgentMetricsAxisModel)
	if err != nil {
		t.Fatalf("AgentMetrics: %v", err)
	}
	row := findRow(t, res.Rows, "sonnet")
	if row.Attempts != 1 {
		t.Fatalf("24h window must exclude 48h-old attempt; got %d attempts", row.Attempts)
	}
}

// TestAgentMetrics_RevisionCache asserts the per-axis cache keys results on
// the run-store fingerprint and invalidates when a new run is added.
func TestAgentMetrics_RevisionCache(t *testing.T) {
	resetAgentMetricsCache()
	wd := fixtureWorkingDir(t)
	r := newTestResolver(t, wd)

	first, err := r.AgentMetrics(context.Background(), AgentMetricsWindowW7d, AgentMetricsAxisModel)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	// Same call returns identical pointer: cache hit.
	cached, err := r.AgentMetrics(context.Background(), AgentMetricsWindowW7d, AgentMetricsAxisModel)
	if err != nil {
		t.Fatalf("cached: %v", err)
	}
	if cached != first {
		t.Fatalf("expected cache hit (same pointer); got distinct results")
	}

	// Add a new attempt — fingerprint must change, cache must invalidate.
	now := time.Now().UTC().Format(time.RFC3339)
	writeRun(t, wd, "d.json", `{
		"id":"att-4","layer":"try","status":"success",
		"harness":"claude","provider":"anthropic","model":"sonnet",
		"cost_usd":1,"duration_ms":1,"started_at":"`+now+`","completed_at":"`+now+`"
	}`)
	updated, err := r.AgentMetrics(context.Background(), AgentMetricsWindowW7d, AgentMetricsAxisModel)
	if err != nil {
		t.Fatalf("updated: %v", err)
	}
	if updated == first {
		t.Fatalf("expected cache invalidation after new run; got same pointer")
	}
	if updated.Revision == first.Revision {
		t.Fatalf("revision should change when run-store changes")
	}
	row := findRow(t, updated.Rows, "sonnet")
	if row.Attempts != 3 {
		t.Fatalf("sonnet attempts after add = %d, want 3", row.Attempts)
	}
}

// TestAgentMetrics_RevisionSurvivesArchiveMove emulates the ADR-004 archive
// flow: closing a bead and moving its events into beads-archive.jsonl. The
// fingerprint must update so callers re-aggregate against the new state.
func TestAgentMetrics_RevisionSurvivesArchiveMove(t *testing.T) {
	resetAgentMetricsCache()
	wd := fixtureWorkingDir(t)
	r := newTestResolver(t, wd)
	first, err := r.AgentMetrics(context.Background(), AgentMetricsWindowW7d, AgentMetricsAxisModel)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	// Materialize an archive file — this is the ADR-004 archive surface.
	if err := os.MkdirAll(filepath.Join(wd, ".ddx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, ".ddx", "beads-archive.jsonl"), []byte(`{"id":"x"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := r.AgentMetrics(context.Background(), AgentMetricsWindowW7d, AgentMetricsAxisModel)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.Revision == second.Revision {
		t.Fatalf("revision should flip when beads-archive appears; got identical %q", first.Revision)
	}
}

func TestAgentMetrics_InvalidEnumsRejected(t *testing.T) {
	resetAgentMetricsCache()
	r := newTestResolver(t, t.TempDir())
	if _, err := r.AgentMetrics(context.Background(), AgentMetricsWindow("BOGUS"), AgentMetricsAxisModel); err == nil {
		t.Fatalf("expected invalid window to error")
	}
	if _, err := r.AgentMetrics(context.Background(), AgentMetricsWindowW7d, AgentMetricsAxis("BOGUS")); err == nil {
		t.Fatalf("expected invalid axis to error")
	}
}

func findRow(t *testing.T, rows []*AgentMetricsRow, key string) *AgentMetricsRow {
	t.Helper()
	for _, r := range rows {
		if r.Key == key {
			return r
		}
	}
	t.Fatalf("row %q not found among %d rows", key, len(rows))
	return nil
}
