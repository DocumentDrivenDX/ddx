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
	mu     sync.Mutex
	calls  []RunArgs
	result *Result
	err    error
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
	return r.result, r.err
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
