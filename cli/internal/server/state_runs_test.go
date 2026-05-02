package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// TestRunsLayering_BundlesAreTryAndSessionsAreRun verifies the canonical
// projection: execute-bead bundles map to layer=try, AgentSession entries
// map to layer=run, and a session whose BundlePath points at a bundle is
// parented to the matching try.
func TestRunsLayering_BundlesAreTryAndSessionsAreRun(t *testing.T) {
	workDir := t.TempDir()
	writeConfig(t, workDir, `version: "1.0"`+"\n")
	state := stateWithProject(workDir)

	bundleID := "20260423T002311-2ebaa669"
	bundleDir := filepath.Join(workDir, agent.ExecuteBeadArtifactDir, bundleID)
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"attempt_id":"` + bundleID + `","bead_id":"ddx-001","base_rev":"abc123","result_rev":"def456","verdict":"success"}`)
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	result := []byte(`{"bead_id":"ddx-001","verdict":"success","exit_code":0,"started_at":"2026-04-23T00:23:11Z","finished_at":"2026-04-23T00:24:11Z","base_rev":"abc123","result_rev":"def456"}`)
	if err := os.WriteFile(filepath.Join(bundleDir, "result.json"), result, 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 4, 23, 0, 23, 11, 0, time.UTC)
	bundleRel := filepath.ToSlash(filepath.Join(agent.ExecuteBeadArtifactDir, bundleID))
	appendSummarySession(t, workDir, agent.SessionIndexEntry{
		ID:           "sess-bundle-001",
		BeadID:       "ddx-001",
		Harness:      "claude",
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-6",
		BillingMode:  "subscription",
		StartedAt:    now,
		DurationMS:   5000,
		InputTokens:  100,
		OutputTokens: 50,
		CostUSD:      0.01,
		CostPresent:  true,
		Outcome:      "success",
		BundlePath:   bundleRel,
	}, now)
	appendSummarySession(t, workDir, agent.SessionIndexEntry{
		ID:          "sess-orphan-002",
		Harness:     "codex",
		BillingMode: "paid",
		StartedAt:   now.Add(time.Minute),
		DurationMS:  2000,
		ExitCode:    1,
		Outcome:     "failure",
	}, now.Add(time.Minute))

	runs := state.GetRunsGraphQL("proj-test", ddxgraphql.RunFilter{})
	if len(runs) < 3 {
		t.Fatalf("expected at least 3 runs (1 try + 2 run), got %d", len(runs))
	}

	var tryRun, runFromBundleSession, orphanSession *ddxgraphql.Run
	for _, r := range runs {
		switch r.ID {
		case "exec-" + bundleID:
			tryRun = r
		case "sess-bundle-001":
			runFromBundleSession = r
		case "sess-orphan-002":
			orphanSession = r
		}
	}

	if tryRun == nil {
		t.Fatal("missing try-layer run for bundle")
	}
	if tryRun.Layer != ddxgraphql.RunLayerTry {
		t.Fatalf("bundle layer = %q, want %q", tryRun.Layer, ddxgraphql.RunLayerTry)
	}
	if tryRun.BaseRevision == nil || *tryRun.BaseRevision != "abc123" {
		t.Fatalf("try.baseRevision = %v, want abc123", tryRun.BaseRevision)
	}
	if tryRun.ResultRevision == nil || *tryRun.ResultRevision != "def456" {
		t.Fatalf("try.resultRevision = %v, want def456", tryRun.ResultRevision)
	}
	if tryRun.MergeOutcome == nil || *tryRun.MergeOutcome != "merged" {
		t.Fatalf("try.mergeOutcome = %v, want merged", tryRun.MergeOutcome)
	}
	if tryRun.Harness != nil {
		t.Fatalf("try should not carry harness; got %v", tryRun.Harness)
	}

	if runFromBundleSession == nil {
		t.Fatal("missing run-layer record from bundle-linked session")
	}
	if runFromBundleSession.Layer != ddxgraphql.RunLayerRun {
		t.Fatalf("session layer = %q, want %q", runFromBundleSession.Layer, ddxgraphql.RunLayerRun)
	}
	if runFromBundleSession.Harness == nil || *runFromBundleSession.Harness != "claude" {
		t.Fatalf("session.harness = %v, want claude", runFromBundleSession.Harness)
	}
	if runFromBundleSession.Provider == nil || *runFromBundleSession.Provider != "anthropic" {
		t.Fatalf("session.provider = %v, want anthropic", runFromBundleSession.Provider)
	}
	if runFromBundleSession.TokensIn == nil || *runFromBundleSession.TokensIn != 100 {
		t.Fatalf("session.tokensIn = %v, want 100", runFromBundleSession.TokensIn)
	}
	if runFromBundleSession.TokensOut == nil || *runFromBundleSession.TokensOut != 50 {
		t.Fatalf("session.tokensOut = %v, want 50", runFromBundleSession.TokensOut)
	}
	if runFromBundleSession.Status != "success" {
		t.Fatalf("session.status = %q, want success", runFromBundleSession.Status)
	}
	if runFromBundleSession.ParentRunID == nil || *runFromBundleSession.ParentRunID != "exec-"+bundleID {
		t.Fatalf("session.parentRunId = %v, want exec-%s", runFromBundleSession.ParentRunID, bundleID)
	}

	if orphanSession == nil {
		t.Fatal("missing run-layer record from orphan session")
	}
	if orphanSession.Layer != ddxgraphql.RunLayerRun {
		t.Fatalf("orphan layer = %q, want %q", orphanSession.Layer, ddxgraphql.RunLayerRun)
	}
	if orphanSession.ParentRunID != nil {
		t.Fatalf("orphan.parentRunId = %v, want nil", orphanSession.ParentRunID)
	}
	if orphanSession.Status != "failure" {
		t.Fatalf("orphan.status = %q, want failure", orphanSession.Status)
	}

	tryLayer := ddxgraphql.RunLayerTry
	tryOnly := state.GetRunsGraphQL("proj-test", ddxgraphql.RunFilter{Layer: &tryLayer})
	for _, r := range tryOnly {
		if r.Layer != ddxgraphql.RunLayerTry {
			t.Fatalf("layer=try filter returned %q", r.Layer)
		}
	}
	runLayer := ddxgraphql.RunLayerRun
	runOnly := state.GetRunsGraphQL("proj-test", ddxgraphql.RunFilter{Layer: &runLayer})
	for _, r := range runOnly {
		if r.Layer != ddxgraphql.RunLayerRun {
			t.Fatalf("layer=run filter returned %q", r.Layer)
		}
	}
}
