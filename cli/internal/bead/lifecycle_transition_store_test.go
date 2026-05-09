package bead

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleTransitionWriteAPI_AllowsDocumentedTransitions(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		opts LifecycleTransitionOptions
	}{
		{name: "proposed to open", from: StatusProposed, to: StatusOpen},
		{name: "proposed to cancelled", from: StatusProposed, to: StatusCancelled},
		{name: "open to in progress", from: StatusOpen, to: StatusInProgress},
		{name: "in progress to open", from: StatusInProgress, to: StatusOpen},
		{name: "in progress to closed", from: StatusInProgress, to: StatusClosed},
		{name: "in progress to proposed", from: StatusInProgress, to: StatusProposed, opts: LifecycleTransitionOptions{OperatorRequired: true}},
		{name: "open to blocked", from: StatusOpen, to: StatusBlocked, opts: LifecycleTransitionOptions{ExternalBlockerReason: "waiting for upstream"}},
		{name: "blocked to open", from: StatusBlocked, to: StatusOpen},
		{name: "closed to open manual reopen", from: StatusClosed, to: StatusOpen, opts: LifecycleTransitionOptions{ManualReopen: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			b := &Bead{Title: tt.name, Status: tt.from}
			require.NoError(t, s.Create(b))

			require.NoError(t, s.SetLifecycleStatus(b.ID, tt.to, tt.opts))
			got, err := s.Get(b.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.to, got.Status)
			if tt.to == StatusBlocked {
				assert.Equal(t, tt.opts.ExternalBlockerReason, got.Extra[ExtraLifecycleExternalBlockerReason])
			}
		})
	}
}

func TestLifecycleTransitionWriteAPI_RejectsInvalidTransitions(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{Title: "guarded"}
	require.NoError(t, s.Create(b))

	require.Error(t, s.SetLifecycleStatus(b.ID, "needs_human", LifecycleTransitionOptions{}))
	require.Error(t, s.SetLifecycleStatus(b.ID, "needs_investigation", LifecycleTransitionOptions{}))
	require.Error(t, s.SetLifecycleStatus(b.ID, StatusBlocked, LifecycleTransitionOptions{}))

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)
	assert.False(t, LifecycleStatusSatisfiesDependency(StatusCancelled))
}

func TestLifecycleTransitionWriteAPI_RejectsDirectUpdateBypass(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{Title: "direct status write"}
	require.NoError(t, s.Create(b))

	err := s.Update(b.ID, func(b *Bead) {
		b.Status = StatusInProgress
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires Store.TransitionLifecycle")

	got, getErr := s.Get(b.ID)
	require.NoError(t, getErr)
	assert.Equal(t, StatusOpen, got.Status)
}
