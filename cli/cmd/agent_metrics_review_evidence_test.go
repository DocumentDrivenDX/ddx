package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewEvidenceApproveAttributesToTier verifies the full
// execute-bead-loop → review-outcomes pipeline: when the executor returns
// success and the reviewer APPROVEs, the bead carries both a kind:routing
// event (with resolved_provider/resolved_model) and a kind:review event (with
// APPROVE in summary), and computeReviewOutcomes attributes that review to
// the executor's harness/model tier with approvals=1.
func TestReviewEvidenceApproveAttributesToTier(t *testing.T) {
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-rev-approve", Title: "approve target"}))

	worker := &agent.ExecuteBeadWorker{
		Store: store,
		Executor: agent.ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			return agent.ExecuteBeadReport{
				BeadID:    beadID,
				Status:    agent.ExecuteBeadStatusSuccess,
				SessionID: "sess-approve",
				ResultRev: "aabbccdd",
				Harness:   "claude",
				Provider:  "claude",
				Model:     "sonnet",
			}, nil
		}),
		Reviewer: agent.BeadReviewerFunc(func(_ context.Context, _, _ string, _ agent.ImplementerRouting) (*agent.ReviewResult, error) {
			return &agent.ReviewResult{Verdict: agent.VerdictApprove, RawOutput: "### Verdict: APPROVE"}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, agent.ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	events, err := store.Events("ddx-rev-approve")
	require.NoError(t, err)

	routingIdx, reviewIdx := -1, -1
	for i, e := range events {
		switch e.Kind {
		case "routing":
			if routingIdx == -1 {
				routingIdx = i
			}
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(e.Body), &body))
			assert.Equal(t, "claude", body["resolved_provider"])
			assert.Equal(t, "sonnet", body["resolved_model"])
		case "review":
			reviewIdx = i
			assert.Equal(t, "APPROVE", e.Summary)
		}
	}
	require.GreaterOrEqual(t, routingIdx, 0, "loop must append a kind:routing event before review")
	require.GreaterOrEqual(t, reviewIdx, 0, "loop must append a kind:review event after APPROVE")
	assert.Less(t, routingIdx, reviewIdx, "routing must precede review for correct tier attribution")

	rows, err := computeReviewOutcomes(dir)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "claude/sonnet", rows[0].Tier)
	assert.Equal(t, "claude", rows[0].Harness)
	assert.Equal(t, "sonnet", rows[0].Model)
	assert.Equal(t, 1, rows[0].Reviews)
	assert.Equal(t, 1, rows[0].Approvals)
	assert.Equal(t, 0, rows[0].Rejections)
	assert.InDelta(t, 1.0, rows[0].ApprovalRate, 0.0001)
}

// TestReviewEvidenceRequestChangesCountedAsRejection verifies that a
// REQUEST_CHANGES verdict is attributed to the same tier as the executor's
// report and counted as a rejection in review-outcomes.
func TestReviewEvidenceRequestChangesCountedAsRejection(t *testing.T) {
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-rev-reject", Title: "reject target"}))

	worker := &agent.ExecuteBeadWorker{
		Store: store,
		Executor: agent.ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			return agent.ExecuteBeadReport{
				BeadID:    beadID,
				Status:    agent.ExecuteBeadStatusSuccess,
				SessionID: "sess-reject",
				ResultRev: "11223344",
				Harness:   "claude",
				Provider:  "claude",
				Model:     "sonnet",
			}, nil
		}),
		Reviewer: agent.BeadReviewerFunc(func(_ context.Context, _, _ string, _ agent.ImplementerRouting) (*agent.ReviewResult, error) {
			return &agent.ReviewResult{Verdict: agent.VerdictRequestChanges, RawOutput: "### Verdict: REQUEST_CHANGES\n\n- needs tests"}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, agent.ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	events, err := store.Events("ddx-rev-reject")
	require.NoError(t, err)
	var hasRouting, hasReview bool
	for _, e := range events {
		if e.Kind == "routing" {
			hasRouting = true
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(e.Body), &body))
			assert.Equal(t, "claude", body["resolved_provider"])
			assert.Equal(t, "sonnet", body["resolved_model"])
		}
		if e.Kind == "review" {
			hasReview = true
			assert.Equal(t, "REQUEST_CHANGES", e.Summary)
		}
	}
	assert.True(t, hasRouting, "loop must append kind:routing on the reopened-after-review path")
	assert.True(t, hasReview, "loop must append kind:review with REQUEST_CHANGES summary")

	rows, err := computeReviewOutcomes(dir)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "claude/sonnet", rows[0].Tier)
	assert.Equal(t, 1, rows[0].Reviews)
	assert.Equal(t, 0, rows[0].Approvals)
	assert.Equal(t, 1, rows[0].Rejections)
	assert.InDelta(t, 0.0, rows[0].ApprovalRate, 0.0001)
}
