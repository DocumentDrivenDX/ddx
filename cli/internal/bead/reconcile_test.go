package bead

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeadReconcile_LatestSuccessClearsStaleNoChangesMetadata(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{
		ID:     "ddx-stale-success",
		Title:  "success after no_changes",
		Labels: []string{LabelNoChangesUnjustified},
		Extra: map[string]any{
			ExtraRetryAfter:     "2026-01-01T00:00:00Z",
			ExtraLastStatus:     "no_changes",
			ExtraLastDetail:     "agent exited without a commit",
			ExtraNoChangesCount: 2,
			"events": []any{
				map[string]any{"kind": "execute-bead", "summary": "no_changes", "created_at": "2026-01-01T00:00:00Z"},
				map[string]any{"kind": "execute-bead", "summary": "success", "created_at": "2026-01-01T01:00:00Z"},
			},
		},
	}
	require.NoError(t, s.Create(testCtx(), b))

	plans, err := s.ReconcileLifecycleMetadata(ReconcileOptions{})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, b.ID, plans[0].BeadID)
	assert.Contains(t, plans[0].ClearFields, ExtraRetryAfter)
	assert.Contains(t, plans[0].RemoveLabels, LabelNoChangesUnjustified)

	plans, err = s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.NotContains(t, got.Extra, ExtraRetryAfter)
	assert.NotContains(t, got.Extra, ExtraLastStatus)
	assert.NotContains(t, got.Extra, ExtraLastDetail)
	assert.NotContains(t, got.Extra, ExtraNoChangesCount)
	assert.NotContains(t, got.Labels, LabelNoChangesUnjustified)
}

func TestBeadReconcile_VerifiedNoChangesCanCloseAlreadySatisfied(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{
		ID:     "ddx-verified",
		Title:  "verified no changes",
		Labels: []string{LabelNoChangesUnverified},
		Extra: map[string]any{
			ExtraLastStatus: "no_changes",
			"events": []any{
				map[string]any{"kind": "no_changes_verified", "summary": "no_changes_verified", "body": "verification_command=true\nexit_code=0", "created_at": "2026-01-01T00:00:00Z"},
			},
		},
	}
	require.NoError(t, s.Create(testCtx(), b))

	plans, err := s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.True(t, plans[0].CloseSatisfied)

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status)
	assert.NotContains(t, got.Extra, ExtraLastStatus)
	assert.NotContains(t, got.Labels, LabelNoChangesUnverified)
	events, err := s.Events(b.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, events)
}

func TestBeadReconcile_ParentEpicMarksNotExecutableOnlyWithChildEvidence(t *testing.T) {
	s := newTestStore(t)
	parent := &Bead{
		ID:        "ddx-parent",
		Title:     "EPIC: parent owns no implementation",
		IssueType: "epic",
		Extra: map[string]any{
			ExtraNoChangesCount: 2,
		},
	}
	child := &Bead{ID: "ddx-child", Title: "child", Parent: parent.ID}
	require.NoError(t, s.Create(testCtx(), parent))
	require.NoError(t, s.Create(testCtx(), child))

	plans, err := s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, parent.ID, plans[0].BeadID)
	assert.Equal(t, false, plans[0].SetFields[ExtraExecutionElig])

	got, err := s.Get(testCtx(), parent.ID)
	require.NoError(t, err)
	assert.Equal(t, false, got.Extra[ExtraExecutionElig])
	assert.Equal(t, "parent/epic container; execute child beads first", got.Extra[ExtraExecutionReason])
}

func TestLifecycle_ReconcileLatestSuccessClearsStaleNoChanges(t *testing.T) {
	TestBeadReconcile_LatestSuccessClearsStaleNoChangesMetadata(t)
}

func TestLifecycle_NoChangesVerified_ClosesAndLeavesQueue(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{
		ID:    "ddx-verified-lifecycle",
		Title: "verified already done",
		Extra: map[string]any{
			ExtraLastStatus: "no_changes",
			"events": []any{
				map[string]any{"kind": "no_changes_verified", "summary": "no_changes_verified", "body": "verification_command=true\nexit_code=0", "created_at": "2026-01-01T00:00:00Z"},
			},
		},
	}
	require.NoError(t, s.Create(testCtx(), b))
	_, err := s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status)
	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	assert.Empty(t, ready)
}

func TestLifecycle_UnjustifiedNoChanges_BadAttemptNotLongCooldown(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{
		ID:     "ddx-unjustified",
		Title:  "unjustified no changes",
		Labels: []string{LabelNoChangesUnjustified},
		Extra: map[string]any{
			ExtraLastStatus:     "no_changes",
			ExtraNoChangesCount: 1,
			"events": []any{
				map[string]any{"kind": "no_changes_unjustified", "summary": "no_changes_unjustified", "body": "(rationale absent)", "created_at": "2026-01-01T00:00:00Z"},
			},
		},
	}
	require.NoError(t, s.Create(testCtx(), b))

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)
	assert.Contains(t, got.Labels, LabelNoChangesUnjustified)
	assert.NotContains(t, got.Extra, ExtraRetryAfter)

	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	require.Len(t, ready, 1)
	assert.Equal(t, b.ID, ready[0].ID)
}

func TestLifecycle_TransientTransport_UsesCooldown(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{ID: "ddx-transient", Title: "transient transport"}
	require.NoError(t, s.Create(testCtx(), b))
	until := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Second)
	require.NoError(t, s.SetExecutionCooldown(b.ID, until, "execution_failed", "transport retryable", ""))

	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	assert.Empty(t, ready)
	blocked, err := s.BlockedAll()
	require.NoError(t, err)
	require.Len(t, blocked, 1)
	assert.Equal(t, BlockerKindRetryCooldown, blocked[0].Blocker.Kind)
	assert.Equal(t, until.Format(time.RFC3339), blocked[0].Blocker.NextEligibleAt)
}

func TestLifecycle_ParentEpicNotOrdinaryExecutionReady(t *testing.T) {
	s := newTestStore(t)
	parent := &Bead{ID: "ddx-epic", Title: "EPIC: rollup", IssueType: "epic", Extra: map[string]any{ExtraNoChangesCount: 1}}
	child := &Bead{ID: "ddx-epic-child", Title: "child", Parent: parent.ID}
	require.NoError(t, s.Create(testCtx(), parent))
	require.NoError(t, s.Create(testCtx(), child))
	_, err := s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)

	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	require.Len(t, ready, 1)
	assert.Equal(t, child.ID, ready[0].ID)

	blocked, err := s.BlockedAll()
	require.NoError(t, err)
	require.Len(t, blocked, 1)
	assert.Equal(t, parent.ID, blocked[0].ID)
	assert.Equal(t, BlockerKindEpicOnly, blocked[0].Blocker.Kind)
}

func TestReconcileCloseSkipsClosureGate(t *testing.T) {
	// TD-031 §5: reconcile-close intentionally bypasses ClosureGate — the bead
	// has no execution session and no closing_commit_sha; transitive deps supply evidence.
	s := newTestStore(t)

	// ClosureGate rejects beads with no execution evidence (the axon-c5cc071a shape).
	require.ErrorIs(t, ClosureGate(&Bead{ID: "ddx-empty", Extra: map[string]any{}}), ErrClosureGateRejected)

	// A bead with no session_id, no closing_commit_sha, and a no_changes_verified event
	// is the canonical reconcile-close shape (all deps closed per TD-031 §5).
	b := &Bead{
		ID:    "ddx-reconcile-bypass",
		Title: "dependency-satisfied bead with no session",
		Extra: map[string]any{
			ExtraLastStatus: "no_changes",
			"events": []any{
				map[string]any{
					"kind":       "no_changes_verified",
					"summary":    "no_changes_verified",
					"body":       "verification_command=true\nexit_code=0",
					"created_at": "2026-01-01T00:00:00Z",
				},
			},
		},
	}
	require.NoError(t, s.Create(testCtx(), b))

	plans, err := s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.True(t, plans[0].CloseSatisfied)

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status)
	assert.Empty(t, got.Extra["session_id"])
	assert.Empty(t, got.Extra["closing_commit_sha"])
}
