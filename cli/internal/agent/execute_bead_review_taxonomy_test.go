package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewErrorTaxonomy covers FEAT-022 §12 / Stage G AC #1: each of the
// four review-error classes is emitted distinctly when its root cause fires,
// and the appended review-error event body carries the literal class
// identifier so operators can triage without re-parsing reviewer text.
func TestReviewErrorTaxonomy(t *testing.T) {
	tests := []struct {
		name      string
		wantClass string
		reviewer  BeadReviewerFunc
	}{
		{
			name:      "context_overflow",
			wantClass: evidence.OutcomeReviewContextOverflow,
			// Stage C1's pre-dispatch short-circuit returns ReviewResult.Error
			// = OutcomeReviewContextOverflow alongside a wrapped error. The
			// loop classifier reads the structured Error first.
			reviewer: BeadReviewerFunc(func(_ context.Context, _, resultRev, _, _ string) (*ReviewResult, error) {
				return &ReviewResult{
						Verdict:   VerdictBlock,
						Error:     evidence.OutcomeReviewContextOverflow,
						ResultRev: resultRev,
					}, fmt.Errorf("reviewer: %s (assembled prompt 200000 bytes exceeds cap 100000)",
						evidence.OutcomeReviewContextOverflow)
			}),
		},
		{
			name:      "provider_empty",
			wantClass: evidence.OutcomeReviewProviderEmpty,
			reviewer: BeadReviewerFunc(func(_ context.Context, _, resultRev, _, _ string) (*ReviewResult, error) {
				return &ReviewResult{
						Verdict:   VerdictBlock,
						Error:     evidence.OutcomeReviewProviderEmpty,
						ResultRev: resultRev,
					}, fmt.Errorf("reviewer: %s: %w", evidence.OutcomeReviewProviderEmpty,
						ErrReviewVerdictUnparseable)
			}),
		},
		{
			name:      "unparseable",
			wantClass: evidence.OutcomeReviewUnparseable,
			reviewer: BeadReviewerFunc(func(_ context.Context, _, resultRev, _, _ string) (*ReviewResult, error) {
				return &ReviewResult{
						Verdict:   VerdictBlock,
						Error:     evidence.OutcomeReviewUnparseable,
						ResultRev: resultRev,
						RawOutput: "Reviewer text without a structured verdict line.",
					}, fmt.Errorf("reviewer: %s: %w", evidence.OutcomeReviewUnparseable,
						ErrReviewVerdictUnparseable)
			}),
		},
		{
			name:      "transport",
			wantClass: evidence.OutcomeReviewTransport,
			reviewer: BeadReviewerFunc(func(_ context.Context, _, resultRev, _, _ string) (*ReviewResult, error) {
				return &ReviewResult{
						Verdict:   VerdictBlock,
						Error:     evidence.OutcomeReviewTransport,
						ResultRev: resultRev,
					}, fmt.Errorf("reviewer: %s: %w", evidence.OutcomeReviewTransport,
						errors.New("dial tcp: connection refused"))
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
						SessionID: "sess-tax-" + tc.name,
						ResultRev: "rev-" + tc.name,
					}, nil
				}),
				Reviewer: tc.reviewer,
			}

			cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
			rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
			_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
			require.NoError(t, err)

			events, err := store.Events(first.ID)
			require.NoError(t, err)
			var got *bead.BeadEvent
			for i := range events {
				if events[i].Kind == "review-error" {
					got = &events[i]
					break
				}
			}
			require.NotNil(t, got, "expected a review-error event for class %s", tc.wantClass)
			// AC #1: event body must contain the literal class identifier.
			assert.Contains(t, got.Body, tc.wantClass,
				"review-error event body must carry the literal class string for operator triage")
			assert.Equal(t, tc.wantClass, got.Summary,
				"review-error event summary should be the class identifier")
			// And the result_rev association must be recorded so the retry
			// counter can be scoped to the originating commit.
			assert.True(t, strings.Contains(got.Body, "result_rev=rev-"+tc.name),
				"event body must include result_rev= for retry-counter scoping")
		})
	}
}
