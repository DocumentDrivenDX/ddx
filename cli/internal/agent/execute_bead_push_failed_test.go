package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassifyExecuteBeadStatusPushFailed pins the contract that a merged
// outcome whose reason carries the push-failure marker classifies as
// push_failed (not success), so the execute-loop refuses to close the bead.
func TestClassifyExecuteBeadStatusPushFailed(t *testing.T) {
	got := ClassifyExecuteBeadStatus("merged", 0, PushFailedReasonPrefix+" remote rejected: pre-receive hook declined")
	assert.Equal(t, ExecuteBeadStatusPushFailed, got,
		"merged outcome with push-failed reason must classify as push_failed, not success")

	gotMerged := ClassifyExecuteBeadStatus("merged", 0, "merged onto current tip")
	assert.Equal(t, ExecuteBeadStatusSuccess, gotMerged,
		"merged outcome without push-failed reason must still classify as success")
}

// TestExecuteBeadWorkerPushFailedStaysOpenAndParks asserts the AC for
// ddx-af54ebf3:
//   - bead is NOT closed (status remains open)
//   - last_status=push_failed and the push stderr are stored as cooldown metadata
//   - the bead's events surface the push stderr so an operator can see it
func TestExecuteBeadWorkerPushFailedStaysOpenAndParks(t *testing.T) {
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

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "push_failed must NOT close the bead")
	assert.Empty(t, got.Owner, "push_failed must release the claim")

	require.NotNil(t, got.Extra)
	assert.Equal(t, "push_failed", got.Extra["execute-loop-last-status"],
		"loop must record last_status=push_failed so subsequent claim attempts can refuse")
	detail, _ := got.Extra["execute-loop-last-detail"].(string)
	assert.Contains(t, detail, pushStderr,
		"loop must record the push stderr in last_detail so operators can see why")
	assert.NotEmpty(t, got.Extra["execute-loop-retry-after"],
		"push_failed must park the bead via execute-loop-retry-after")

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

// TestClaimRefusesPushFailedBead pins the AC that subsequent claim attempts
// on a push-failed bead fail loudly until the operator clears
// execute-loop-last-status.
func TestClaimRefusesPushFailedBead(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-pf01", Title: "push-failed", Priority: 0}
	require.NoError(t, store.Create(b))

	require.NoError(t, store.Update(b.ID, func(bb *bead.Bead) {
		if bb.Extra == nil {
			bb.Extra = map[string]any{}
		}
		bb.Extra["execute-loop-last-status"] = "push_failed"
		bb.Extra["execute-loop-last-detail"] = PushFailedReasonPrefix + " remote: GH001: large files detected"
	}))

	err := store.Claim(b.ID, "worker")
	require.Error(t, err, "Claim must refuse a bead whose last_status=push_failed")
	assert.Contains(t, err.Error(), "push failed")
	assert.Contains(t, err.Error(), "GH001",
		"the previous push stderr must surface in the claim error so it fails loudly")

	// Clearing the last_status restores claimability.
	require.NoError(t, store.Update(b.ID, func(bb *bead.Bead) {
		delete(bb.Extra, "execute-loop-last-status")
		delete(bb.Extra, "execute-loop-last-detail")
	}))
	require.NoError(t, store.Claim(b.ID, "worker"),
		"Claim must succeed after operator clears execute-loop-last-status")
}
