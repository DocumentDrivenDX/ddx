package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

func TestSessionsCostSummaryBucketsAndEmptyWindow(t *testing.T) {
	workDir := t.TempDir()
	writeConfig(t, workDir, `version: "1.0"`+"\n")
	state := stateWithProject(workDir)
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	appendSummarySession(t, workDir, agent.SessionIndexEntry{ID: "cash", Harness: "openrouter", StartedAt: now, CostUSD: 0.25, CostPresent: true}, now)
	appendSummarySession(t, workDir, agent.SessionIndexEntry{ID: "sub", Harness: "codex", StartedAt: now.Add(time.Minute), CostUSD: 0.50, CostPresent: true}, now.Add(time.Minute))
	appendSummarySession(t, workDir, agent.SessionIndexEntry{ID: "local", Harness: "agent", StartedAt: now.Add(2 * time.Minute), Tokens: 1500}, now.Add(2*time.Minute))

	summary := state.GetSessionsCostSummaryGraphQL("proj-test", nil, nil)
	if summary.CashUsd != 0.25 {
		t.Fatalf("cashUsd=%v, want 0.25", summary.CashUsd)
	}
	if summary.SubscriptionEquivUsd != 0.50 {
		t.Fatalf("subscriptionEquivUsd=%v, want 0.50", summary.SubscriptionEquivUsd)
	}
	if summary.LocalSessionCount != 1 {
		t.Fatalf("localSessionCount=%d, want 1", summary.LocalSessionCount)
	}
	if summary.LocalEstimatedUsd != nil {
		t.Fatalf("localEstimatedUsd=%v, want nil when config unset", *summary.LocalEstimatedUsd)
	}

	since := now.Add(24 * time.Hour)
	empty := state.GetSessionsCostSummaryGraphQL("proj-test", &since, nil)
	if empty.CashUsd != 0 || empty.SubscriptionEquivUsd != 0 || empty.LocalSessionCount != 0 || empty.LocalEstimatedUsd != nil {
		t.Fatalf("empty summary = %+v, want zero cash/sub/local and nil estimate", empty)
	}
}

func TestSessionsCostSummaryLocalEstimateUsesConfiguredRate(t *testing.T) {
	workDir := t.TempDir()
	writeConfig(t, workDir, "version: \"1.0\"\ncost:\n  local_per_1k_tokens: 0.002\n")
	state := stateWithProject(workDir)
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	appendSummarySession(t, workDir, agent.SessionIndexEntry{ID: "local-a", Harness: "agent", StartedAt: now, Tokens: 1500}, now)
	appendSummarySession(t, workDir, agent.SessionIndexEntry{ID: "local-b", Harness: "agent", StartedAt: now.AddDate(0, -1, 0), InputTokens: 250, OutputTokens: 250}, now.AddDate(0, -1, 0))
	appendSummarySession(t, workDir, agent.SessionIndexEntry{ID: "cash", Harness: "openrouter", StartedAt: now, CostUSD: 0.25, CostPresent: true}, now)

	summary := state.GetSessionsCostSummaryGraphQL("proj-test", nil, nil)
	if summary.LocalEstimatedUsd == nil {
		t.Fatal("localEstimatedUsd=nil, want configured estimate")
	}
	if got, want := *summary.LocalEstimatedUsd, 0.004; got != want {
		t.Fatalf("localEstimatedUsd=%v, want %v", got, want)
	}

	since := now.Add(-time.Hour)
	filtered := state.GetSessionsCostSummaryGraphQL("proj-test", &since, nil)
	if filtered.LocalEstimatedUsd == nil {
		t.Fatal("filtered localEstimatedUsd=nil, want configured estimate")
	}
	if got, want := *filtered.LocalEstimatedUsd, 0.003; got != want {
		t.Fatalf("filtered localEstimatedUsd=%v, want %v", got, want)
	}
}

func stateWithProject(workDir string) *ServerState {
	return &ServerState{
		Projects: []ProjectEntry{{
			ID:   "proj-test",
			Name: "test",
			Path: workDir,
		}},
	}
}

func writeConfig(t *testing.T, workDir, content string) {
	t.Helper()
	ddxDir := filepath.Join(workDir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendSummarySession(t *testing.T, workDir string, entry agent.SessionIndexEntry, ts time.Time) {
	t.Helper()
	if err := agent.AppendSessionIndex(agent.SessionLogDirForWorkDir(workDir), entry, ts); err != nil {
		t.Fatal(err)
	}
}
