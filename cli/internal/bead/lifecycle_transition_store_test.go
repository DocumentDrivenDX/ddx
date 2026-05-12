package bead

import (
	"encoding/json"
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

func TestLifecycleProposedToOpenAppendsTriaged(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{Title: "operator accepted bead", Status: StatusProposed}
	require.NoError(t, s.Create(b))

	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{
		Kind:      "intake.blocked",
		Summary:   "operator_required",
		Body:      `{"fingerprint":"finding-123","prompt_fingerprint":"prompt-abc"}`,
		Actor:     "worker",
		Source:    "ddx work",
		CreatedAt: b.CreatedAt,
	}))

	require.NoError(t, s.SetLifecycleStatus(b.ID, StatusOpen, LifecycleTransitionOptions{
		Reason: "operator accepted readiness decision",
		Actor:  "reviewer",
		Source: "test",
	}))

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)

	events, err := s.EventsByKind(b.ID, "triaged")
	require.NoError(t, err)
	require.Len(t, events, 1)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(events[0].Body), &body))
	assert.Equal(t, "proposed", body["from_status"])
	assert.Equal(t, "open", body["to_status"])
	assert.Equal(t, "operator accepted readiness decision", body["reason"])
	assert.Equal(t, "reviewer", body["actor"])
	assert.Equal(t, true, body["operator_required"])
	assert.Equal(t, "finding-123", body["accepted_fingerprint"])
	assert.Equal(t, "prompt-abc", body["accepted_prompt_fingerprint"])
	assert.NotEmpty(t, body["prompt_fingerprint"])
}
