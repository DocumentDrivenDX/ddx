package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteBeadWorker_ReviewerFailureModesKeepBeadOpen covers ddx-738edf47
// AC #3: on any reviewer terminal failure — nonzero exit, empty output, or
// unparseable output — the loop must record a failure event and leave the
// bead un-closed so a later attempt can retry. Closing on reviewer failure
// is the silent-false-closure surface that 738edf47 eliminates.
func TestExecuteBeadWorker_ReviewerFailureModesKeepBeadOpen(t *testing.T) {
	tests := []struct {
		name     string
		reviewer BeadReviewerFunc
	}{
		{
			name: "reviewer exits non-zero (returns error)",
			reviewer: BeadReviewerFunc(func(_ context.Context, _, _, _, _ string) (*ReviewResult, error) {
				return nil, errors.New("reviewer harness exited with code 1")
			}),
		},
		{
			name: "reviewer output empty — parse returns unparseable",
			reviewer: BeadReviewerFunc(func(_ context.Context, _, _, _, _ string) (*ReviewResult, error) {
				return &ReviewResult{
					Verdict:   "",
					RawOutput: "",
				}, nil
			}),
		},
		{
			name: "reviewer output unparseable — no recognizable verdict line",
			reviewer: BeadReviewerFunc(func(_ context.Context, _, _, _, _ string) (*ReviewResult, error) {
				return &ReviewResult{
					Verdict:   "",
					RawOutput: "Reviewer crashed mid-stream with no structured verdict.",
				}, nil
			}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, first, _ := newExecuteLoopTestStore(t)
			worker := &ExecuteBeadWorker{
				Store: store,
				Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
					return ExecuteBeadReport{
						BeadID:    beadID,
						Status:    ExecuteBeadStatusSuccess,
						SessionID: "sess-x",
						ResultRev: "c0ffee" + tc.name[:4],
					}, nil
				}),
				Reviewer: tc.reviewer,
			}

			_, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker", Once: true})
			require.NoError(t, err)

			got, err := store.Get(first.ID)
			require.NoError(t, err)
			assert.NotEqual(t, bead.StatusClosed, got.Status,
				"reviewer failure of any kind must not close the bead — the whole point of this invariant is that a broken reviewer cannot silently end work")
		})
	}
}
