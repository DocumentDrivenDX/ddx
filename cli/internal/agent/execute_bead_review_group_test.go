package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reviewGroupRunnerStub struct {
	mu      sync.Mutex
	calls   []RunArgs
	result  *Result
	results []*Result
	err     error
	errs    []error
}

func (r *reviewGroupRunnerStub) Run(opts RunArgs) (*Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	copied := opts
	if opts.Correlation != nil {
		copied.Correlation = make(map[string]string, len(opts.Correlation))
		for k, v := range opts.Correlation {
			copied.Correlation[k] = v
		}
	}
	r.calls = append(r.calls, copied)
	idx := len(r.calls) - 1
	if idx < len(r.results) {
		var err error
		if idx < len(r.errs) {
			err = r.errs[idx]
		}
		return r.results[idx], err
	}
	return r.result, r.err
}

const reviewerOutputRequestChanges = "```json\n{\"schema_version\":1,\"verdict\":\"REQUEST_CHANGES\",\"summary\":\"missing regression test\",\"findings\":[{\"severity\":\"warn\",\"summary\":\"missing regression test\",\"location\":\"cli/internal/agent/foo_test.go:42\"}]}\n```"

func TestDefaultCloseGateSingleReviewer(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	runner := &reviewGroupRunnerStub{result: &Result{Output: reviewerOutputApprove}}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      runner,
	}

	group, err := reviewer.ReviewGroup(context.Background(), "ddx-pairing", head, ImplementerRouting{})
	require.NoError(t, err)
	require.Len(t, group.Slots, 1)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, 0, group.Slots[0].ReviewerIndex)
	assert.Equal(t, lifecycleStrongMinPower, group.Slots[0].Runtime.MinPowerOverride)
}

func TestElevatedCloseGateOnlyForRiskLabelsOrReviewTierOverride(t *testing.T) {
	tests := []struct {
		name       string
		labels     []string
		reviewTier string
		impl       ImplementerRouting
		wantSlots  int
	}{
		{name: "kind safety", labels: []string{"kind:safety"}, wantSlots: 2},
		{name: "area storage", labels: []string{"area:storage"}, wantSlots: 2},
		{name: "kind migration", labels: []string{"kind:migration"}, wantSlots: 2},
		{name: "explicit elevated tier", reviewTier: "elevated", wantSlots: 2},
		{name: "ordinary bead", wantSlots: 1},
		{
			name:      "concrete primary route pin",
			impl:      ImplementerRouting{Harness: "operator-harness", Provider: "operator-provider", Model: "operator-model", ActualPower: 70},
			wantSlots: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot, head, store := reviewPairingTestSetup(t)
			require.NoError(t, store.Update(context.Background(), "ddx-pairing", func(b *bead.Bead) {
				b.Labels = append([]string(nil), tc.labels...)
			}))
			runner := &reviewGroupRunnerStub{result: &Result{Output: reviewerOutputApprove}}
			reviewer := &DefaultBeadReviewer{
				ProjectRoot: projectRoot,
				BeadStore:   store,
				Runner:      runner,
				ReviewTier:  tc.reviewTier,
			}

			group, err := reviewer.ReviewGroup(context.Background(), "ddx-pairing", head, tc.impl)
			require.NoError(t, err)
			require.Len(t, group.Slots, tc.wantSlots)
			require.Len(t, runner.calls, tc.wantSlots)
			for i, call := range runner.calls {
				assert.Empty(t, call.Harness)
				assert.Empty(t, call.Provider)
				assert.Empty(t, call.Model)
				assert.Empty(t, group.Slots[i].Runtime.ProfileOverride)
			}
		})
	}
}

func TestReviewGroup_DispatchesTwoSlotsSameEvidence(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	runner := &reviewGroupRunnerStub{result: &Result{
		Harness:     "claude",
		Provider:    "anthropic",
		Model:       "claude-opus-4-7",
		ActualPower: 95,
		Output:      reviewerOutputApprove,
	}}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      runner,
		ReviewTier:  "elevated",
	}

	group, err := reviewer.ReviewGroup(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:     "codex",
		Provider:    "openai",
		Model:       "gpt-5",
		ActualPower: 70,
		Correlation: map[string]string{
			"bead_id":    "ddx-pairing",
			"attempt_id": "att-1",
			"session_id": "sess-1",
			"result_rev": head,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.Len(t, runner.calls, 2)
	require.Len(t, group.Slots, 2)

	assert.Equal(t, runner.calls[0].PromptFile, runner.calls[1].PromptFile,
		"both reviewer slots must receive the same prompt file path")
	assert.Equal(t, group.Bundle.PromptAbs, runner.calls[0].PromptFile)
	assert.Equal(t, group.Bundle.PromptAbs, group.Slots[0].Runtime.PromptFile)
	assert.Equal(t, group.Bundle.PromptAbs, group.Slots[1].Runtime.PromptFile)
	assert.Equal(t, group.Bundle.GroupID, group.Slots[0].Runtime.Correlation["review_group_id"])
	assert.Equal(t, group.Bundle.GroupID, group.Slots[1].Runtime.Correlation["review_group_id"])
}

func TestReviewGroup_CorrelationFields(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	runner := &reviewGroupRunnerStub{result: &Result{
		Harness:     "claude",
		Provider:    "anthropic",
		Model:       "claude-opus-4-7",
		ActualPower: 95,
		Output:      reviewerOutputApprove,
	}}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      runner,
		ReviewTier:  "elevated",
	}

	_, err := reviewer.ReviewGroup(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:     "codex",
		Provider:    "openai",
		Model:       "gpt-5",
		ActualPower: 70,
		Correlation: map[string]string{
			"bead_id":    "ddx-pairing",
			"attempt_id": "att-1",
			"session_id": "sess-1",
			"result_rev": head,
		},
	})
	require.NoError(t, err)
	require.Len(t, runner.calls, 2)

	for i, call := range runner.calls {
		require.NotNil(t, call.Correlation)
		assert.Equal(t, "reviewer", call.Role)
		assert.Equal(t, fmt.Sprintf("ddx-pairing:%s:%d", call.Correlation["review_group_id"], i), call.CorrelationID)
		assert.Equal(t, "ddx-pairing", call.Correlation["bead_id"])
		assert.Equal(t, "att-1", call.Correlation["attempt_id"])
		assert.Equal(t, head, call.Correlation["result_rev"])
		assert.Equal(t, "reviewer", call.Correlation["role"])
		assert.Equal(t, fmt.Sprintf("%d", i), call.Correlation["reviewer_index"])
		assert.NotContains(t, call.Correlation, "impl_harness")
		assert.NotContains(t, call.Correlation, "impl_provider")
		assert.NotContains(t, call.Correlation, "impl_model")
		assert.NotContains(t, call.Correlation, "impl_actual_power")
	}
}

func TestReviewGroup_UsesSharedPromptFileOnDisk(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	runner := &reviewGroupRunnerStub{result: &Result{
		Harness:     "claude",
		Provider:    "anthropic",
		Model:       "claude-opus-4-7",
		ActualPower: 95,
		Output:      reviewerOutputApprove,
	}}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      runner,
		ReviewTier:  "elevated",
	}

	group, err := reviewer.ReviewGroup(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:     "codex",
		Provider:    "openai",
		Model:       "gpt-5",
		ActualPower: 70,
		Correlation: map[string]string{
			"bead_id":    "ddx-pairing",
			"attempt_id": "att-1",
			"session_id": "sess-1",
			"result_rev": head,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.FileExists(t, filepath.Clean(group.Bundle.PromptAbs))
	assert.Contains(t, group.Bundle.PromptRel, filepath.ToSlash(".ddx/executions"))
	assert.Contains(t, group.Bundle.PromptRel, "prompt.md")
	assert.NotEmpty(t, strings.TrimSpace(group.Bundle.GroupID))
}

func TestTwoSlotReview_ElevatedDispatchesTwoReviewers(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	runner := &reviewGroupRunnerStub{result: &Result{
		Harness:     "claude",
		Provider:    "anthropic",
		Model:       "claude-opus-4-7",
		ActualPower: 95,
		Output:      reviewerOutputApprove,
	}}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      runner,
		ReviewTier:  "elevated",
	}

	got, err := reviewer.Review(context.Background(), projectRoot, CandidateResult{
		Report: ExecuteBeadReport{
			BeadID:      "ddx-pairing",
			AttemptID:   "att-1",
			SessionID:   "sess-1",
			Status:      ExecuteBeadStatusSuccess,
			BaseRev:     head,
			ResultRev:   head,
			Harness:     "codex",
			Provider:    "openai",
			Model:       "gpt-5",
			ActualPower: 70,
		},
		WorktreePath: projectRoot,
	})

	require.NoError(t, err)
	assert.Equal(t, string(VerdictApprove), got.Verdict)
	require.Len(t, runner.calls, 2)
	assert.Equal(t, runner.calls[0].PromptFile, runner.calls[1].PromptFile)
}

func TestTwoSlotReview_CorrelationFacts(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	runner := &reviewGroupRunnerStub{result: &Result{
		Harness:     "claude",
		Provider:    "anthropic",
		Model:       "claude-opus-4-7",
		ActualPower: 95,
		Output:      reviewerOutputApprove,
	}}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      runner,
		ReviewTier:  "elevated",
	}

	_, err := reviewer.Review(context.Background(), projectRoot, CandidateResult{
		CycleIndex: 3,
		Report: ExecuteBeadReport{
			BeadID:       "ddx-pairing",
			AttemptID:    "att-1",
			SessionID:    "sess-1",
			Status:       ExecuteBeadStatusSuccess,
			BaseRev:      head,
			ResultRev:    head,
			CandidateRef: "refs/ddx/iterations/att-1/3",
			Harness:      "codex",
			Provider:     "openai",
			Model:        "gpt-5",
			ActualPower:  70,
		},
		WorktreePath: projectRoot,
	})

	require.NoError(t, err)
	require.Len(t, runner.calls, 2)
	groupID := runner.calls[0].Correlation["review_group_id"]
	require.NotEmpty(t, groupID)
	for i, call := range runner.calls {
		require.NotNil(t, call.Correlation)
		assert.Equal(t, groupID, call.Correlation["review_group_id"])
		assert.Equal(t, fmt.Sprintf("%d", i), call.Correlation["reviewer_index"])
		assert.Equal(t, "refs/ddx/iterations/att-1/3", call.Correlation["candidate_ref"])
		assert.Equal(t, "3", call.Correlation["cycle_index"])
		assert.Equal(t, fmt.Sprintf("ddx-pairing:%s:%d", groupID, i), call.CorrelationID)
	}
}

func TestTwoSlotReview_UnanimousApproveRequired(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	runner := &reviewGroupRunnerStub{
		results: []*Result{
			{Harness: "claude", Provider: "anthropic", Model: "claude-opus-4-7", ActualPower: 95, Output: reviewerOutputApprove},
			{Harness: "claude", Provider: "anthropic", Model: "claude-opus-4-7", ActualPower: 95, Output: reviewerOutputRequestChanges},
		},
	}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      runner,
		ReviewTier:  "elevated",
	}
	landed := false
	coord := &AttemptCycleCoordinator{
		ProjectRoot: projectRoot,
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:      beadID,
					AttemptID:   "att-1",
					SessionID:   "sess-1",
					Status:      ExecuteBeadStatusSuccess,
					BaseRev:     head,
					ResultRev:   head,
					Harness:     "codex",
					Provider:    "openai",
					Model:       "gpt-5",
					ActualPower: 70,
				},
				WorktreePath: projectRoot,
			}, nil
		}),
		Reviewer: reviewer,
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			landed = true
			return candidate.Report, nil
		}),
	}

	got, err := coord.Run(context.Background(), "ddx-pairing")

	require.NoError(t, err)
	assert.False(t, landed)
	assert.False(t, got.Landed)
	assert.Equal(t, ExecuteBeadStatusReviewRequestChanges, got.Report.Status)
	assert.Equal(t, string(VerdictRequestChanges), got.Report.ReviewVerdict)
	require.Len(t, runner.calls, 2)
}

func TestReviewerProviderIdentityIsEvidenceOnly(t *testing.T) {
	type requestSnapshot struct {
		harness          string
		model            string
		profile          string
		minPower         int
		role             string
		clearRoutingPins bool
		clearProfile     bool
	}
	type controlSnapshot struct {
		landed        bool
		status        string
		reviewVerdict string
		nextRequests  []requestSnapshot
		provider      string
	}
	var snapshots []controlSnapshot

	for _, reviewerProvider := range []string{"anthropic", "openai"} {
		reviewerProvider := reviewerProvider
		t.Run(reviewerProvider, func(t *testing.T) {
			projectRoot, head, store := reviewPairingTestSetup(t)
			runner := &reviewGroupRunnerStub{result: &Result{
				Harness:     "claude",
				Provider:    reviewerProvider,
				Model:       "claude-opus-4-7",
				ActualPower: 95,
				Output:      reviewerOutputApprove,
			}}
			reviewer := &DefaultBeadReviewer{
				ProjectRoot: projectRoot,
				BeadStore:   store,
				BeadEvents:  store,
				EventReader: store,
				Runner:      runner,
				ReviewTier:  "elevated",
			}
			impl := ImplementerRouting{
				Harness:     "claude",
				Provider:    "anthropic",
				Model:       "claude-sonnet-4-6",
				ActualPower: 70,
			}

			landed := false
			coord := &AttemptCycleCoordinator{
				ProjectRoot: projectRoot,
				Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
					return CandidateResult{
						Report: ExecuteBeadReport{
							BeadID:      beadID,
							AttemptID:   "att-1",
							SessionID:   "sess-1",
							Status:      ExecuteBeadStatusSuccess,
							BaseRev:     head,
							ResultRev:   head,
							Harness:     impl.Harness,
							Provider:    impl.Provider,
							Model:       impl.Model,
							ActualPower: impl.ActualPower,
						},
						WorktreePath: projectRoot,
					}, nil
				}),
				Reviewer: reviewer,
				Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
					landed = true
					return candidate.Report, nil
				}),
			}

			got, err := coord.Run(context.Background(), "ddx-pairing")
			require.NoError(t, err)
			assert.True(t, landed)
			assert.True(t, got.Landed)

			// Dispatch the same review again. A prior same-provider result must not
			// raise the next MinPower or otherwise alter the request.
			group, err := reviewer.ReviewGroup(context.Background(), "ddx-pairing", head, impl)
			require.NoError(t, err)
			require.Len(t, group.Slots, 2)
			nextRequests := make([]requestSnapshot, 0, len(group.Slots))
			for _, slot := range group.Slots {
				require.NotNil(t, slot.Result)
				assert.Equal(t, reviewerProvider, slot.Result.ReviewerProvider,
					"concrete provider must remain visible as result evidence")
				assert.Equal(t, 95, slot.Result.ActualPower,
					"abstract actual power remains separately available as evidence")
				assert.Equal(t, VerdictApprove, slot.Result.Verdict)
				nextRequests = append(nextRequests, requestSnapshot{
					harness:          slot.Runtime.HarnessOverride,
					model:            slot.Runtime.ModelOverride,
					profile:          slot.Runtime.ProfileOverride,
					minPower:         slot.Runtime.MinPowerOverride,
					role:             slot.Runtime.Role,
					clearRoutingPins: slot.Runtime.ClearRoutingPins,
					clearProfile:     slot.Runtime.ClearProfile,
				})
			}

			beadEvents, err := store.Events("ddx-pairing")
			require.NoError(t, err)
			for _, event := range beadEvents {
				assert.NotEqual(t, legacyReviewPairingDegradedEventKind, event.Kind,
					"provider equality must not create DDx control state")
			}

			snapshots = append(snapshots, controlSnapshot{
				landed:        got.Landed,
				status:        got.Report.Status,
				reviewVerdict: got.Report.ReviewVerdict,
				nextRequests:  nextRequests,
				provider:      reviewerProvider,
			})
		})
	}

	require.Len(t, snapshots, 2)
	assert.NotEqual(t, snapshots[0].provider, snapshots[1].provider,
		"fixture must vary only concrete reviewer provider identity")
	assert.Equal(t, snapshots[0].landed, snapshots[1].landed)
	assert.Equal(t, snapshots[0].status, snapshots[1].status)
	assert.Equal(t, snapshots[0].reviewVerdict, snapshots[1].reviewVerdict)
	assert.Equal(t, snapshots[0].nextRequests, snapshots[1].nextRequests,
		"concrete reviewer provider identity must not steer the next request")
	assert.Equal(t, []requestSnapshot{
		{minPower: 71, role: "reviewer"},
		{minPower: 71, role: "reviewer"},
	}, snapshots[0].nextRequests)
}
