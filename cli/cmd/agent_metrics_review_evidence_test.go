package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func appendMetricsRoutingReviewEvents(t *testing.T, store *bead.Store, beadID, verdict string, createdAt time.Time) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"resolved_provider": "claude",
		"resolved_model":    "sonnet",
		"fallback_chain":    []string{},
	})
	require.NoError(t, err)
	require.NoError(t, store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "routing",
		Summary:   "provider=claude model=sonnet",
		Body:      string(body),
		Actor:     "ddx",
		Source:    "ddx agent execute-loop",
		CreatedAt: createdAt,
	}))
	require.NoError(t, store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "review",
		Summary:   verdict,
		Body:      "fixture review verdict",
		Actor:     "worker",
		Source:    "ddx agent review",
		CreatedAt: createdAt.Add(time.Second),
	}))
}

// TestReviewEvidenceApproveAttributesToTier verifies the review-outcomes
// attribution pipeline over explicit bead evidence: when a kind:routing event
// precedes an APPROVE kind:review event, computeReviewOutcomes attributes the
// review to that provider/model tier with approvals=1.
func TestReviewEvidenceApproveAttributesToTier(t *testing.T) {
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-rev-approve", Title: "approve target"}))
	appendMetricsRoutingReviewEvents(t, store, "ddx-rev-approve", "APPROVE", time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC))

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
	require.GreaterOrEqual(t, routingIdx, 0, "fixture must append a kind:routing event before review")
	require.GreaterOrEqual(t, reviewIdx, 0, "fixture must append a kind:review event after APPROVE")
	assert.Less(t, routingIdx, reviewIdx, "routing must precede review for correct tier attribution")

	report, err := computeReviewOutcomesReport(dir, 0, time.Time{})
	rows := report.Rows
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
// REQUEST_CHANGES verdict is attributed to the most recent routing tier and
// counted as a rejection in review-outcomes.
func TestReviewEvidenceRequestChangesCountedAsRejection(t *testing.T) {
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-rev-reject", Title: "reject target"}))
	appendMetricsRoutingReviewEvents(t, store, "ddx-rev-reject", "REQUEST_CHANGES", time.Date(2026, 5, 9, 2, 0, 0, 0, time.UTC))

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
	assert.True(t, hasRouting, "fixture must append kind:routing for tier attribution")
	assert.True(t, hasReview, "fixture must append kind:review with REQUEST_CHANGES summary")

	report, err := computeReviewOutcomesReport(dir, 0, time.Time{})
	rows := report.Rows
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "claude/sonnet", rows[0].Tier)
	assert.Equal(t, 1, rows[0].Reviews)
	assert.Equal(t, 0, rows[0].Approvals)
	assert.Equal(t, 1, rows[0].Rejections)
	assert.InDelta(t, 0.0, rows[0].ApprovalRate, 0.0001)
}
