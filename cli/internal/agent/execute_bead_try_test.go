package agent

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTry_ReviewVerdict_ApproveRoundTrips verifies that when a ddx try execution
// reaches post-merge review with an APPROVE verdict, the terminal verdict persists
// in both the bead event stream and the .ddx/metrics/attempts.jsonl row.
// This regression test ensures that review verdicts remain queryable for
// cross-bead economics analysis (e.g., "what is the cheapest model that produces
// approved work?") from the durable metrics.
func TestTry_ReviewVerdict_ApproveRoundTrips(t *testing.T) {
	projectRoot := t.TempDir()
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init())

	targetBead := &bead.Bead{
		ID:         "ddx-try-review-approve",
		Title:      "Try bead with approval",
		Acceptance: "1. PASS_MARKER found in diff\n2. lefthook run pre-commit passes",
	}
	require.NoError(t, store.Create(targetBead))

	// Stub executor that returns success.
	executor := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		return ExecuteBeadReport{
			BeadID:     beadID,
			AttemptID:  "attempt-approve-001",
			Status:     ExecuteBeadStatusSuccess,
			ResultRev:  "result123abc",
			BaseRev:    "base123abc",
			SessionID:  "session-approve-001",
			Model:      "test-model",
			Harness:    "test-harness",
			Provider:   "test-provider",
			CostUSD:    1.5,
			DurationMS: 5000,
		}, nil
	})

	// Deterministic reviewer that returns APPROVE with per-AC evidence.
	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict:   VerdictApprove,
			Rationale: "All acceptance criteria met",
			PerAC: []ReviewAC{
				{Number: 1, Item: "TestFoo passes", Grade: "pass", Evidence: "TestFoo found in diff"},
				{Number: 2, Item: "lefthook run pre-commit passes", Grade: "pass", Evidence: "pre-commit gate green"},
			},
			RawOutput:       `{"verdict":"APPROVE","rationale":"All acceptance criteria met"}`,
			ReviewerHarness: "review-harness",
			ReviewerModel:   "review-model",
			InputBytes:      100,
			OutputBytes:     50,
			DurationMS:      1000,
		}, nil
	})

	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: executor,
		Reviewer: reviewer,
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "test-worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// AC 1: Run the worker once through the post-merge review flow.
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Mode:            "once",
		WorkerID:        "test-worker",
		ProjectRoot:     projectRoot,
		SessionID:       "session-approve-001",
		EventSink:       io.Discard,
		PostMergeReview: true,
		FinalizeDurableAudit: func(report ExecuteBeadReport) error {
			return FinalizeDurableAttemptAudit(projectRoot, store, report)
		},
		NoReview: false,
	})

	require.NoError(t, err, "worker.Run must succeed")
	require.NotNil(t, result)
	require.Greater(t, len(result.Results), 0, "expected at least one result")
	report := result.Results[0]

	// AC 1: The attempt report must carry the APPROVE verdict.
	assert.Equal(t, string(VerdictApprove), report.ReviewVerdict,
		"report.ReviewVerdict must be APPROVE")
	assert.Equal(t, "All acceptance criteria met", report.ReviewRationale,
		"report.ReviewRationale must match the reviewer's rationale")

	// AC 2: The bead event stream must contain a review event with APPROVE verdict.
	events, err := store.Events(targetBead.ID)
	require.NoError(t, err, "store.Events must succeed")

	var reviewEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "review" && events[i].Summary == "APPROVE" {
			reviewEvent = &events[i]
			break
		}
	}
	require.NotNil(t, reviewEvent, "expected review event with APPROVE summary in event stream")

	// AC 1, AC 2: The .ddx/metrics/attempts.jsonl must contain the APPROVE verdict.
	metricsPath := attemptmetrics.AttemptsPath(projectRoot)
	f, err := os.Open(metricsPath)
	require.NoError(t, err, "metrics file must exist and be readable")
	defer f.Close()

	var metricsRow attemptmetrics.AttemptRow
	decoder := json.NewDecoder(f)
	for decoder.More() {
		err := decoder.Decode(&metricsRow)
		require.NoError(t, err, "metrics row must be valid JSON")
		if metricsRow.AttemptID == "attempt-approve-001" {
			break
		}
	}

	assert.Equal(t, "attempt-approve-001", metricsRow.AttemptID,
		"metrics row must have matching attempt_id")
	assert.Equal(t, string(VerdictApprove), metricsRow.ReviewVerdict,
		"metrics row.ReviewVerdict must be APPROVE")
	assert.Equal(t, "test-model", metricsRow.Model,
		"metrics row must preserve model from report")
	assert.Equal(t, "test-harness", metricsRow.Harness,
		"metrics row must preserve harness from report")
	assert.Equal(t, 1.5, metricsRow.CostUSD,
		"metrics row must preserve cost from report")
}
