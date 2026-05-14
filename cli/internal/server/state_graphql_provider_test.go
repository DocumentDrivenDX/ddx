package server

import (
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

func TestAgentSessionsGraphQLFiltersByProvider(t *testing.T) {
	workDir := t.TempDir()
	writeConfig(t, workDir, `version: "1.0"`+"\n")
	state := stateWithProject(workDir)
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	appendSummarySession(t, workDir, agent.SessionIndexEntry{ID: "claude-1", Harness: "claude", Provider: "claude", StartedAt: now}, now)
	appendSummarySession(t, workDir, agent.SessionIndexEntry{ID: "codex-1", Harness: "codex", Provider: "codex", StartedAt: now.Add(-time.Minute)}, now.Add(-time.Minute))

	sessions := state.GetAgentSessionsGraphQL("claude", nil, nil)
	if len(sessions) != 1 {
		t.Fatalf("sessions=%d, want 1", len(sessions))
	}
	if sessions[0].Provider == nil || *sessions[0].Provider != "claude" {
		t.Fatalf("provider=%v, want claude", sessions[0].Provider)
	}
	if sessions[0].ID != "claude-1" {
		t.Fatalf("session ID=%q, want claude-1", sessions[0].ID)
	}
}

func TestWorkerResultFromRecordIncludesPower(t *testing.T) {
	gqlResult := workerResultFromRecord(&WorkerExecutionResult{
		AttemptID:  "attempt-1",
		PowerClass: "cheap",
		Provider:   "claude",
	})
	if gqlResult.PowerClass == nil || *gqlResult.PowerClass != "cheap" {
		t.Fatalf("powerClass=%v, want cheap", gqlResult.PowerClass)
	}
	if gqlResult.Provider == nil || *gqlResult.Provider != "claude" {
		t.Fatalf("provider=%v, want claude", gqlResult.Provider)
	}
}
