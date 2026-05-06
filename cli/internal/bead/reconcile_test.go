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
	require.NoError(t, s.Create(b))

	plans, err := s.ReconcileLifecycleMetadata(ReconcileOptions{})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, b.ID, plans[0].BeadID)
	assert.Contains(t, plans[0].ClearFields, ExtraRetryAfter)
	assert.Contains(t, plans[0].RemoveLabels, LabelNoChangesUnjustified)

	plans, err = s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	got, err := s.Get(b.ID)
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
	require.NoError(t, s.Create(b))

	plans, err := s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.True(t, plans[0].CloseSatisfied)

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status)
	assert.NotContains(t, got.Extra, ExtraLastStatus)
	assert.NotContains(t, got.Labels, LabelNoChangesUnverified)
	events, err := s.Events(b.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, events)
}

func TestBeadReconcile_NonRetryableNeedsInvestigationClearsCooldown(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{
		ID:     "ddx-needs-investigation",
		Title:  "needs investigation",
		Labels: []string{LabelNeedsInvestigation},
		Extra: map[string]any{
			ExtraRetryAfter: "2026-01-01T00:00:00Z",
			ExtraLastStatus: "no_changes",
			ExtraLastDetail: "operator input required",
		},
	}
	require.NoError(t, s.Create(b))

	plans, err := s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, "needs_investigation is non-retryable; clear cooldown metadata", plans[0].Reason)

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)
	assert.Contains(t, got.Labels, LabelNeedsInvestigation)
	assert.NotContains(t, got.Extra, ExtraRetryAfter)
}

func TestReconcile_NoViableProviderClearsStaleNeedsInvestigation(t *testing.T) {
	s := newTestStore(t)
	retryAfter := time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339)
	b := &Bead{
		ID:     "ddx-no-provider",
		Title:  "provider outage",
		Labels: []string{LabelNeedsInvestigation, LabelNoChangesUnverified},
		Extra: map[string]any{
			ExtraRetryAfter: retryAfter,
			ExtraLastStatus: "execution_failed",
			ExtraLastDetail: "execute-loop: all tiers exhausted - no viable provider found",
			"events": []any{
				map[string]any{
					"kind":       "execute-bead",
					"summary":    "execution_failed",
					"body":       "execute-loop: all tiers exhausted - no viable provider found",
					"created_at": "2026-01-01T00:00:00Z",
				},
			},
		},
	}
	require.NoError(t, s.Create(b))

	plans, err := s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, "no_viable_provider is retryable transport state; clear stale needs-investigation label", plans[0].Reason)
	assert.Equal(t, []string{LabelNeedsInvestigation}, plans[0].RemoveLabels)

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.NotContains(t, got.Labels, LabelNeedsInvestigation)
	assert.Contains(t, got.Labels, LabelNoChangesUnverified)
	assert.Equal(t, retryAfter, got.Extra[ExtraRetryAfter])
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
	require.NoError(t, s.Create(parent))
	require.NoError(t, s.Create(child))

	plans, err := s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, parent.ID, plans[0].BeadID)
	assert.Equal(t, false, plans[0].SetFields[ExtraExecutionElig])

	got, err := s.Get(parent.ID)
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
	require.NoError(t, s.Create(b))
	_, err := s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status)
	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	assert.Empty(t, ready)
}

func TestLifecycle_NeedsInvestigation_NoCooldownAndExplainedSkip(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{
		ID:     "ddx-needs-skip",
		Title:  "needs investigation skip",
		Labels: []string{LabelNeedsInvestigation},
		Extra: map[string]any{
			ExtraRetryAfter: time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339),
			ExtraLastStatus: "no_changes",
		},
	}
	require.NoError(t, s.Create(b))
	_, err := s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true})
	require.NoError(t, err)

	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	assert.Empty(t, ready)

	blocked, err := s.BlockedAll()
	require.NoError(t, err)
	require.Len(t, blocked, 1)
	assert.Equal(t, BlockerKindNeedsInvestigation, blocked[0].Blocker.Kind)
	assert.Contains(t, blocked[0].Blocker.Reason, "needs investigation")
	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.NotContains(t, got.Extra, ExtraRetryAfter)
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
	require.NoError(t, s.Create(b))

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)
	assert.Contains(t, got.Labels, LabelNoChangesUnjustified)
	assert.NotContains(t, got.Extra, ExtraRetryAfter)
}

func TestLifecycle_TransientTransport_UsesCooldown(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{ID: "ddx-transient", Title: "transient transport"}
	require.NoError(t, s.Create(b))
	until := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Second)
	require.NoError(t, s.SetExecutionCooldown(b.ID, until, "execution_failed", "transport retryable"))

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
	require.NoError(t, s.Create(parent))
	require.NoError(t, s.Create(child))
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
