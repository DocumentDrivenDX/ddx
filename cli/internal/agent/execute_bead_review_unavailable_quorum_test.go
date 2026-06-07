package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewerUnavailable_DoesNotParkAfterMaxRetries covers AC 1 of ddx-aa9687f5:
// A reviewer routing failure (OutcomeReviewReviewerUnavailable) after ReviewMaxRetries
// must NOT applyReviewOperatorRequiredParking on a landed-eligible candidate.
// It must hold the bead open (emit review-error only) so the loop can retry.
func TestReviewerUnavailable_DoesNotParkAfterMaxRetries(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	const resultRev = "unavail-rev"

	// ReviewMaxRetries=2: seed 1 prior failure so the next call is the 2nd (at threshold).
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", ReviewMaxRetries: 2}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	seedReviewErrorEvents(t, store, first.ID, resultRev, evidence.OutcomeReviewReviewerUnavailable, 1)

	out := runPostMergeReviewRetryFixture(t, store, first, resultRev,
		failingReviewer(evidence.OutcomeReviewReviewerUnavailable), rcfg)
	require.False(t, out.Approved)

	events, err := store.Events(first.ID)
	require.NoError(t, err)

	for _, ev := range events {
		assert.NotEqual(t, "review-manual-required", ev.Kind,
			"reviewer_unavailable (routing failure) must never emit review-manual-required")
	}

	// Bead must NOT be parked to proposed — it must stay open for retry.
	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.NotEqual(t, bead.StatusProposed, got.Status,
		"reviewer_unavailable must not park the bead to proposed")
	assert.NotEqual(t, bead.StatusClosed, got.Status,
		"reviewer_unavailable must not close the bead")

	// A review-error event must still be appended for observability.
	var reviewErrEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "review-error" && strings.Contains(events[i].Body, "result_rev="+resultRev) {
			reviewErrEvent = &events[i]
			break
		}
	}
	require.NotNil(t, reviewErrEvent,
		"reviewer_unavailable must still emit a review-error event")
	assert.Contains(t, reviewErrEvent.Body, "failure_class="+evidence.OutcomeReviewReviewerUnavailable)
}

// TestReviewGroup_EvidencelessBlockLosesToQuorum covers AC 2a of ddx-aa9687f5:
// An APPROVE paired with a single evidence-less BLOCK (no Findings with location,
// no per-AC evidence) does not auto-block. The quorum rule applies: approvers >= blocks
// → the candidate is approved and closed.
func TestReviewGroup_EvidencelessBlockLosesToQuorum(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	const resultRev = "quorum-rev"

	reviewer := beadReviewGroupFunc(func(_ context.Context, beadID, gotRev string, _ ImplementerRouting) (*ReviewGroupResult, error) {
		return &ReviewGroupResult{
			BeadID:    beadID,
			ResultRev: gotRev,
			Slots: []ReviewGroupSlotResult{
				{
					ReviewerIndex: 0,
					Result: &ReviewResult{
						Verdict:         VerdictApprove,
						ReviewerHarness: "claude",
						ReviewerModel:   "claude-opus-4-7",
						ResultRev:       gotRev,
						PerAC: []ReviewAC{
							{Number: 1, Item: "AC#1", Evidence: "slot 0 approved: test passes"},
						},
					},
				},
				{
					ReviewerIndex: 1,
					Result: &ReviewResult{
						// Evidence-less BLOCK: rationale only, no Findings with Location, no PerAC evidence.
						Verdict:         VerdictBlock,
						Rationale:       "looks wrong to me",
						ReviewerHarness: "codex",
						ReviewerModel:   "gpt-5",
						ResultRev:       gotRev,
					},
				},
			},
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-quorum",
			ResultRev: resultRev,
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Assignee:    "worker",
		Now:         time.Now,
	})

	require.True(t, out.Approved,
		"lone evidence-less BLOCK must not auto-block: quorum rule gives APPROVE the win")

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status,
		"quorum APPROVE must close the bead")
}

// TestReviewGroup_EvidencedBlockWinsOverApprove covers AC 2b of ddx-aa9687f5:
// An APPROVE paired with a BLOCK that cites a concrete location (Finding.Location non-empty)
// does block: the evidenced BLOCK overrides the approver.
func TestReviewGroup_EvidencedBlockWinsOverApprove(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	const resultRev = "evidenced-block-rev"

	reviewer := beadReviewGroupFunc(func(_ context.Context, beadID, gotRev string, _ ImplementerRouting) (*ReviewGroupResult, error) {
		return &ReviewGroupResult{
			BeadID:    beadID,
			ResultRev: gotRev,
			Slots: []ReviewGroupSlotResult{
				{
					ReviewerIndex: 0,
					Result: &ReviewResult{
						Verdict:         VerdictApprove,
						ReviewerHarness: "claude",
						ReviewerModel:   "claude-opus-4-7",
						ResultRev:       gotRev,
						PerAC: []ReviewAC{
							{Number: 1, Item: "AC#1", Evidence: "test passes"},
						},
					},
				},
				{
					ReviewerIndex: 1,
					Result: &ReviewResult{
						Verdict:         VerdictBlock,
						Rationale:       "AC#1 is not actually tested — the assertion is missing",
						ReviewerHarness: "codex",
						ReviewerModel:   "gpt-5",
						ResultRev:       gotRev,
						// Cites a concrete location — this is a strong, evidenced BLOCK.
						Findings: []Finding{
							{
								Severity: "block",
								Summary:  "AC#1 missing assertion",
								Location: "cli/internal/agent/foo_test.go:42",
							},
						},
					},
				},
			},
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-evidenced-block",
			ResultRev: resultRev,
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Assignee:    "worker",
		Now:         time.Now,
	})

	require.False(t, out.Approved,
		"a BLOCK citing a concrete location must override the approving reviewer")

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.NotEqual(t, bead.StatusClosed, got.Status,
		"an evidenced BLOCK must not close the bead")
}
