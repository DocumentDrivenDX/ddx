package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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
		Harness:     "claude",
		Model:       "claude-opus-4-7",
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
		Harness:     "claude",
		Model:       "claude-opus-4-7",
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
		assert.Equal(t, "codex", call.Correlation["impl_harness"])
		assert.Equal(t, "openai", call.Correlation["impl_provider"])
		assert.Equal(t, "gpt-5", call.Correlation["impl_model"])
		assert.Equal(t, "70", call.Correlation["impl_actual_power"])
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
		Harness:     "claude",
		Model:       "claude-opus-4-7",
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

func TestTwoSlotReview_DefaultDispatchesTwoReviewers(t *testing.T) {
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
		Harness:     "claude",
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
		Harness:     "claude",
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
		Harness:     "claude",
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

func TestTwoSlotReview_PairingDegradedIsEvidenceOnly(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	events := &stubBeadEventAppender{}
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
		BeadEvents:  events,
		Runner:      runner,
		Harness:     "claude",
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
					Harness:     "claude",
					Provider:    "anthropic",
					Model:       "claude-sonnet-4-6",
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
	assert.True(t, landed)
	assert.True(t, got.Landed)
	require.Len(t, runner.calls, 2)
	degraded := 0
	for _, ev := range events.events {
		if ev.Event.Kind == ReviewPairingDegradedEventKind {
			degraded++
		}
	}
	assert.Equal(t, 2, degraded)
}
