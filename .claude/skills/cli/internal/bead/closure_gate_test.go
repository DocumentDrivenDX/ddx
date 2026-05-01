package bead

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClosureGate covers ddx-e30e60a9 AC #3: the gate rejects every
// insufficient-evidence closure. Each case names the real-world failure mode
// it blocks — an invariant that lost meaning would let false closures slip
// through again.
func TestClosureGate(t *testing.T) {
	tests := []struct {
		name        string
		bead        *Bead
		shouldClose bool
		rejectHint  string
	}{
		{
			name:        "nil bead",
			bead:        nil,
			shouldClose: false,
			rejectHint:  "nil bead",
		},
		{
			name: "no events and no closing commit — axon-c5cc071a shape",
			bead: &Bead{
				ID:    "axon-c5cc071a-replay",
				Extra: map[string]any{"events": []any{}},
			},
			shouldClose: false,
			rejectHint:  "no execution evidence",
		},
		{
			name: "APPROVE with empty rationale — the f7ae036f review-malfunction shape",
			bead: &Bead{
				ID: "ddx-approve-empty",
				Extra: map[string]any{
					"closing_commit_sha": "abc123",
					"events": []any{
						map[string]any{"kind": "review", "summary": "APPROVE", "body": ""},
					},
				},
			},
			shouldClose: false,
			rejectHint:  "empty rationale",
		},
		{
			name: "commit SHA and no review — review-skipped path (--no-review / nil reviewer)",
			bead: &Bead{
				ID: "ddx-skip",
				Extra: map[string]any{
					"closing_commit_sha": "abc123",
					"events": []any{
						map[string]any{"kind": "execute-bead", "summary": "success"},
					},
				},
			},
			shouldClose: true,
		},
		{
			name: "APPROVE with rationale — happy reviewer path",
			bead: &Bead{
				ID: "ddx-happy",
				Extra: map[string]any{
					"closing_commit_sha": "abc123",
					"events": []any{
						map[string]any{"kind": "execute-bead", "summary": "success"},
						map[string]any{"kind": "review", "summary": "APPROVE", "body": "All AC met."},
					},
				},
			},
			shouldClose: true,
		},
		{
			name: "BLOCK with rationale — not an automatic close but gate permits (caller reopens)",
			bead: &Bead{
				ID: "ddx-block-with-body",
				Extra: map[string]any{
					"closing_commit_sha": "abc123",
					"events": []any{
						map[string]any{"kind": "review", "summary": "BLOCK", "body": "missing tests"},
					},
				},
			},
			shouldClose: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ClosureGate(tc.bead)
			if tc.shouldClose {
				assert.NoError(t, err, "gate must allow this shape — blocking it would reject legitimate closures")
				return
			}
			require.Error(t, err, "gate must reject this shape — allowing it would reintroduce silent false-closure")
			assert.ErrorIs(t, err, ErrClosureGateRejected)
			if tc.rejectHint != "" {
				assert.Contains(t, err.Error(), tc.rejectHint,
					"rejection message must name the specific missing evidence so operator audit can correlate with the failure mode")
			}
		})
	}
}

// TestCloseWithEvidence_RefusesInsufficientEvidence covers ddx-e30e60a9 AC
// #1 and #4: Store.CloseWithEvidence must actually use the gate, not just
// export it as a library function. A bead lacking both execution evidence
// and a terminal verdict stays open after the call and receives a rejection
// note so operator audit sees why.
func TestCloseWithEvidence_RefusesInsufficientEvidence(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &Bead{ID: "ddx-gate-test", Title: "gate test", Priority: 2}
	require.NoError(t, store.Create(b))

	// Replay the axon-c5cc071a shape: no events, no commit.
	err := store.CloseWithEvidence("ddx-gate-test", "session-123", "")
	require.NoError(t, err, "Store.CloseWithEvidence itself does not error; it records the refusal on the bead so callers can observe it")

	after, err := store.Get("ddx-gate-test")
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, after.Status,
		"bead must remain open when closure gate rejects — the whole point is to prevent silent closure")
	assert.Contains(t, after.Notes, "closure rejected",
		"rejection must surface on the bead where operator audit looks, not just in logs")
}

// TestCloseWithEvidence_AllowsHappyPath asserts the gate doesn't over-reject:
// a bead with proper execution evidence and a rationale-carrying APPROVE
// closes cleanly. If this test fails, automation is broken, not just the
// gate.
func TestCloseWithEvidence_AllowsHappyPath(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &Bead{ID: "ddx-happy-close", Title: "happy", Priority: 2}
	require.NoError(t, store.Create(b))

	require.NoError(t, store.AppendEvent("ddx-happy-close", BeadEvent{
		Kind:      "execute-bead",
		Summary:   "success",
		CreatedAt: time.Now().UTC(),
	}))
	require.NoError(t, store.AppendEvent("ddx-happy-close", BeadEvent{
		Kind:      "review",
		Summary:   "APPROVE",
		Body:      "All AC met; see artifact .ddx/executions/foo",
		CreatedAt: time.Now().UTC(),
	}))

	require.NoError(t, store.CloseWithEvidence("ddx-happy-close", "session-abc", "deadbeef"))

	after, err := store.Get("ddx-happy-close")
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, after.Status)
	assert.Equal(t, "deadbeef", after.Extra["closing_commit_sha"])
}
