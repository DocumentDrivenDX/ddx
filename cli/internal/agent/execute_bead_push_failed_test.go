package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassifyExecuteBeadStatusPushFailed pins the contract that a merged
// outcome whose reason carries the push-failure marker classifies as
// push_failed (not success), so the work refuses to close the bead.
func TestClassifyExecuteBeadStatusPushFailed(t *testing.T) {
	got := ClassifyExecuteBeadStatus("merged", 0, PushFailedReasonPrefix+" remote rejected: pre-receive hook declined")
	assert.Equal(t, ExecuteBeadStatusPushFailed, got,
		"merged outcome with push-failed reason must classify as push_failed, not success")

	gotMerged := ClassifyExecuteBeadStatus("merged", 0, "merged onto current tip")
	assert.Equal(t, ExecuteBeadStatusSuccess, gotMerged,
		"merged outcome without push-failed reason must still classify as success")
}

func TestClassifyExecuteBeadStatusPreservedNeedsReview(t *testing.T) {
	for _, reason := range []string{
		"large-deletion gate: huge.txt deleted 250 lines (threshold 200)",
		"syntax sanity gate: config.json: invalid JSON",
		"post-land gate failed: make test: exit status 2",
	} {
		got := ClassifyExecuteBeadStatus("preserved", 0, reason)
		assert.Equal(t, ExecuteBeadStatusPreservedNeedsReview, got, reason)
	}
}

// TestExecuteBeadWorkerPushFailedStaysOpen asserts that push_failed:
//   - does NOT close the bead (status remains open)
//   - releases the claim (owner is empty)
//   - does NOT set work-retry-after (no cooldown)
//   - surfaces the push stderr in a bead event
func TestExecuteBeadWorkerPushFailedStaysOpen(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	pushStderr := "remote: error: GH001: large files detected; aborting"
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusPushFailed,
				Detail:    PushFailedReasonPrefix + " " + pushStderr,
				SessionID: "sess-push-failed",
				BaseRev:   "aaaa1111",
				ResultRev: "bbbb2222",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.GreaterOrEqual(t, result.Failures, 1)
	assert.Equal(t, ExecuteBeadStatusPushFailed, result.LastFailureStatus)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "push_failed must NOT close the bead")
	assert.Empty(t, got.Owner, "push_failed must release the claim")

	assert.Empty(t, got.Extra["work-retry-after"],
		"push_failed must NOT park the bead — no cooldown should be set")

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	var sawPushStderr bool
	for _, ev := range events {
		if ev.Summary == ExecuteBeadStatusPushFailed && strings.Contains(ev.Body, pushStderr) {
			sawPushStderr = true
			break
		}
	}
	assert.True(t, sawPushStderr,
		"push stderr must appear in a bead event so operators can see why the push failed")
}

// TestExecuteBeadWorkerPushConflictParksAndEmitsEvent pins AC #3 + #5 of
// ddx-a458af7c: push_conflict (auto-merge during push recovery failed)
// emits a structured `push-conflict` event, parks the bead under the 24h
// cap, and is distinct from generic execution_failed.
func TestExecuteBeadWorkerPushConflictParksAndEmitsEvent(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	conflictDetail := PushConflictReasonPrefix + " auto-recovery retry push: CONFLICT (content)"
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusPushConflict,
				Detail:    conflictDetail,
				SessionID: "sess-pconf",
				BaseRev:   "aaaa1111",
				ResultRev: "cccc3333",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	startedAt := time.Now().UTC()
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ExecuteBeadStatusPushConflict, result.LastFailureStatus)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "push_conflict must NOT close the bead")
	require.NotNil(t, got.Extra)
	assert.Equal(t, "push_conflict", got.Extra["work-last-status"])

	// Cooldown capped at 24h.
	retryAfter, _ := got.Extra["work-retry-after"].(string)
	require.NotEmpty(t, retryAfter)
	parsed, perr := time.Parse(time.RFC3339, retryAfter)
	require.NoError(t, perr)
	delta := parsed.Sub(startedAt)
	assert.LessOrEqual(t, delta, MaxLoopCooldown+time.Minute,
		"push_conflict cooldown must cap at MaxLoopCooldown (24h), got %s", delta)

	// Structured push-conflict event with the conflict context.
	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var sawPushConflict bool
	for _, ev := range events {
		if ev.Kind == "push-conflict" {
			sawPushConflict = true
			assert.Contains(t, ev.Body, conflictDetail,
				"push-conflict event body must record the conflict detail")
			break
		}
	}
	assert.True(t, sawPushConflict,
		"loop must emit a kind:push-conflict event so operators can see the conflict context")
}
