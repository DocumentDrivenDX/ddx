package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunPostMergeReview_ReviewerFailureModesKeepBeadOpen covers ddx-738edf47
// AC #3 for the retained legacy/manual helper: on any reviewer terminal
// failure — nonzero exit, empty output, or unparseable output — the helper
// records a failure event and leaves the bead un-closed. work no
// longer invokes this helper after a candidate has landed.
func TestRunPostMergeReview_ReviewerFailureModesKeepBeadOpen(t *testing.T) {
	tests := []struct {
		name     string
		reviewer beadReviewerFunc
	}{
		{
			name: "reviewer exits non-zero (returns error)",
			reviewer: beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
				return nil, errors.New("reviewer harness exited with code 1")
			}),
		},
		{
			name: "reviewer output empty — parse returns unparseable",
			reviewer: beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
				return &ReviewResult{
					Verdict:   "",
					Error:     "unparseable",
					RawOutput: "",
				}, ErrReviewVerdictUnparseable
			}),
		},
		{
			name: "reviewer output unparseable — no recognizable verdict line",
			reviewer: beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
				return &ReviewResult{
					Verdict:   "",
					Error:     "unparseable",
					RawOutput: "Reviewer crashed mid-stream with no structured verdict.",
				}, ErrReviewVerdictUnparseable
			}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, first, _ := newExecuteLoopTestStore(t)
			cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
			rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
			out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
				Bead: *first,
				Report: ExecuteBeadReport{
					BeadID:    first.ID,
					Status:    ExecuteBeadStatusSuccess,
					SessionID: "sess-x",
					ResultRev: "c0ffee" + tc.name[:4],
				},
				Reviewer:    tc.reviewer,
				Store:       store,
				ProjectRoot: t.TempDir(),
				Rcfg:        rcfg,
				Now:         time.Now,
				Assignee:    "worker",
			})
			require.False(t, out.Approved)

			got, err := store.Get(first.ID)
			require.NoError(t, err)
			assert.NotEqual(t, bead.StatusClosed, got.Status,
				"reviewer failure of any kind must not close the bead — the whole point of this invariant is that a broken reviewer cannot silently end work")
		})
	}
}

// TestExecuteBeadWorker_ReviewerFailureModesKeepBeadOpen is kept as a
// compatibility anchor for callers that reference the old worker-level test
// name. The covered behavior now belongs to the retained helper, not the
// automated work close path.
func TestExecuteBeadWorker_ReviewerFailureModesKeepBeadOpen(t *testing.T) {
	TestRunPostMergeReview_ReviewerFailureModesKeepBeadOpen(t)
}
