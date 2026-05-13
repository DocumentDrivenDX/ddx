package bead

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type backendConformanceCase struct {
	name    string
	backend string
}

func backendConformanceCases() []backendConformanceCase {
	return []backendConformanceCase{
		{name: "jsonl", backend: BackendJSONL},
		{name: "axon", backend: BackendAxon},
	}
}

// newBackendConformanceStore makes the backend choice explicit for the
// package-wide conformance matrix instead of relying on any default path.
func newBackendConformanceStore(t *testing.T, backend string) *Store {
	t.Helper()
	switch backend {
	case BackendJSONL:
		return newJSONLStore(t)
	case BackendAxon:
		return newAxonStore(t)
	default:
		t.Fatalf("unsupported backend %q", backend)
		return nil
	}
}

func forEachBackendConformanceCase(t *testing.T, fn func(*testing.T, backendConformanceCase)) {
	t.Helper()
	for _, tc := range backendConformanceCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fn(t, tc)
		})
	}
}

func runBackendConformanceSuite(t *testing.T, tc backendConformanceCase) {
	t.Helper()
	makeStore := func(t *testing.T) *Store { return newBackendConformanceStore(t, tc.backend) }

	t.Run("create-get", func(t *testing.T) {
		t.Parallel()
		s := makeStore(t)

		b := &Bead{Title: "first", Description: "round-trip"}
		require.NoError(t, s.Create(testCtx(), b))
		require.NotEmpty(t, b.ID)

		got, err := s.Get(testCtx(), b.ID)
		require.NoError(t, err)
		assert.Equal(t, "first", got.Title)
		assert.Equal(t, "round-trip", got.Description)
		assert.Equal(t, StatusOpen, got.Status)
	})

	t.Run("update", func(t *testing.T) {
		t.Parallel()
		s := makeStore(t)

		b := &Bead{Title: "to-update"}
		require.NoError(t, s.Create(testCtx(), b))

		require.NoError(t, s.Update(testCtx(), b.ID, func(bb *Bead) {
			bb.Notes = "added by update"
			bb.Priority = 3
		}))

		got, err := s.Get(testCtx(), b.ID)
		require.NoError(t, err)
		assert.Equal(t, "added by update", got.Notes)
		assert.Equal(t, 3, got.Priority)
	})

	t.Run("claim", func(t *testing.T) {
		t.Parallel()
		s := makeStore(t)

		b := &Bead{Title: "claimable"}
		require.NoError(t, s.Create(testCtx(), b))

		require.NoError(t, s.Claim(b.ID, "alice"))
		got, err := s.Get(testCtx(), b.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusInProgress, got.Status)
		assert.Equal(t, "alice", got.Owner)
		require.NotNil(t, got.Extra)
		assert.NotEmpty(t, got.Extra["claimed-at"])

		err = s.Claim(b.ID, "bob")
		require.Error(t, err)

		require.NoError(t, s.Unclaim(b.ID))
		got, err = s.Get(testCtx(), b.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusOpen, got.Status)
		assert.Empty(t, got.Owner)
	})

	t.Run("deps", func(t *testing.T) {
		t.Parallel()
		s := makeStore(t)

		root := &Bead{Title: "root"}
		child := &Bead{Title: "child"}
		require.NoError(t, s.Create(testCtx(), root))
		require.NoError(t, s.Create(testCtx(), child))

		require.NoError(t, s.DepAdd(child.ID, root.ID))
		got, err := s.Get(testCtx(), child.ID)
		require.NoError(t, err)
		require.Len(t, got.Dependencies, 1)
		assert.Equal(t, root.ID, got.Dependencies[0].DependsOnID)

		tree, err := s.DepTree(root.ID)
		require.NoError(t, err)
		assert.Contains(t, tree, root.ID)

		require.NoError(t, s.DepRemove(child.ID, root.ID))
		got, err = s.Get(testCtx(), child.ID)
		require.NoError(t, err)
		assert.Empty(t, got.Dependencies)
	})

	t.Run("events-close", func(t *testing.T) {
		t.Parallel()
		s := makeStore(t)

		b := &Bead{Title: "with-events"}
		require.NoError(t, s.Create(testCtx(), b))

		for i := 0; i < 3; i++ {
			require.NoError(t, s.AppendEvent(b.ID, BeadEvent{
				Kind:    "test",
				Summary: fmt.Sprintf("event-%d", i),
				Body:    fmt.Sprintf("body-%d", i),
				Actor:   "tester",
			}))
		}

		events, err := s.Events(b.ID)
		require.NoError(t, err)
		require.Len(t, events, 3)
		for i, e := range events {
			assert.Equal(t, fmt.Sprintf("event-%d", i), e.Summary)
		}

		require.NoError(t, s.Close(testCtx(), b.ID))
		got, err := s.Get(testCtx(), b.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusClosed, got.Status)
		events, err = s.Events(b.ID)
		require.NoError(t, err)
		assert.Len(t, events, 3)
	})
}

func TestBackendConformance(t *testing.T) {
	forEachBackendConformanceCase(t, func(t *testing.T, tc backendConformanceCase) {
		runBackendConformanceSuite(t, tc)
	})
}

func TestBackendConformance_TransitionMatrix(t *testing.T) {
	forEachBackendConformanceCase(t, func(t *testing.T, bcc backendConformanceCase) {
		makeStore := func(t *testing.T) *Store { return newBackendConformanceStore(t, bcc.backend) }

		// Legal transitions: each sub-test creates a bead in the required starting
		// state, applies the transition via SetLifecycleStatus, then asserts both
		// Status persistence and any required Extra side effects.
		t.Run("legal", func(t *testing.T) {
			t.Parallel()

			type legalCase struct {
				name       string
				fromStatus string
				toStatus   string
				opts       LifecycleTransitionOptions
				// wantExtra: non-nil value → stored value must equal it.
				wantExtra map[string]any
			}

			cases := []legalCase{
				// proposed -> {open, cancelled}
				{name: "proposed-to-open", fromStatus: StatusProposed, toStatus: StatusOpen},
				{name: "proposed-to-cancelled", fromStatus: StatusProposed, toStatus: StatusCancelled},
				// open -> {in_progress, blocked, proposed, cancelled}
				{name: "open-to-in_progress", fromStatus: StatusOpen, toStatus: StatusInProgress},
				{name: "open-to-blocked", fromStatus: StatusOpen, toStatus: StatusBlocked,
					opts:      LifecycleTransitionOptions{ExternalBlockerReason: "external issue"},
					wantExtra: map[string]any{ExtraLifecycleExternalBlockerReason: "external issue"},
				},
				{name: "open-to-proposed", fromStatus: StatusOpen, toStatus: StatusProposed,
					opts: LifecycleTransitionOptions{OperatorRequired: true},
				},
				{name: "open-to-cancelled", fromStatus: StatusOpen, toStatus: StatusCancelled},
				// in_progress -> {open, closed, blocked, proposed}
				{name: "in_progress-to-open", fromStatus: StatusInProgress, toStatus: StatusOpen},
				{name: "in_progress-to-closed", fromStatus: StatusInProgress, toStatus: StatusClosed,
					opts: LifecycleTransitionOptions{ManualClose: true},
				},
				{name: "in_progress-to-blocked", fromStatus: StatusInProgress, toStatus: StatusBlocked,
					opts:      LifecycleTransitionOptions{ExternalBlockerReason: "upstream broken"},
					wantExtra: map[string]any{ExtraLifecycleExternalBlockerReason: "upstream broken"},
				},
				{name: "in_progress-to-proposed", fromStatus: StatusInProgress, toStatus: StatusProposed,
					opts: LifecycleTransitionOptions{OperatorRequired: true},
				},
			}

			for _, lc := range cases {
				lc := lc
				t.Run(lc.name, func(t *testing.T) {
					t.Parallel()
					s := makeStore(t)
					b := &Bead{Title: lc.name, Status: lc.fromStatus}
					require.NoError(t, s.Create(testCtx(), b))

					require.NoError(t, s.SetLifecycleStatus(b.ID, lc.toStatus, lc.opts),
						"legal transition %s -> %s must succeed", lc.fromStatus, lc.toStatus)

					got, err := s.Get(testCtx(), b.ID)
					require.NoError(t, err)
					assert.Equal(t, lc.toStatus, got.Status, "status must be persisted after transition")
					for k, want := range lc.wantExtra {
						require.NotNil(t, got.Extra, "Extra must not be nil when expected values are set")
						assert.Equal(t, want, got.Extra[k], "Extra[%q] must match after transition to %s", k, lc.toStatus)
					}
				})
			}
		})

		// blocked -> {open, proposed, cancelled}: set up bead through open->blocked first so
		// ExtraLifecycleExternalBlockerReason is present, then verify it is cleared on exit.
		t.Run("blocked-from", func(t *testing.T) {
			t.Parallel()

			type blockedFromCase struct {
				name     string
				toStatus string
				opts     LifecycleTransitionOptions
			}

			cases := []blockedFromCase{
				{name: "blocked-to-open", toStatus: StatusOpen},
				{name: "blocked-to-proposed", toStatus: StatusProposed,
					opts: LifecycleTransitionOptions{OperatorRequired: true},
				},
				{name: "blocked-to-cancelled", toStatus: StatusCancelled},
			}

			for _, lc := range cases {
				lc := lc
				t.Run(lc.name, func(t *testing.T) {
					t.Parallel()
					s := makeStore(t)

					b := &Bead{Title: lc.name}
					require.NoError(t, s.Create(testCtx(), b))
					require.NoError(t, s.SetLifecycleStatus(b.ID, StatusBlocked, LifecycleTransitionOptions{
						ExternalBlockerReason: "blocking reason",
					}))

					pre, err := s.Get(testCtx(), b.ID)
					require.NoError(t, err)
					require.NotNil(t, pre.Extra)
					require.Equal(t, "blocking reason", pre.Extra[ExtraLifecycleExternalBlockerReason],
						"ExtraLifecycleExternalBlockerReason must be set before blocked-from transition")

					require.NoError(t, s.SetLifecycleStatus(b.ID, lc.toStatus, lc.opts),
						"legal transition blocked -> %s must succeed", lc.toStatus)

					got, err := s.Get(testCtx(), b.ID)
					require.NoError(t, err)
					assert.Equal(t, lc.toStatus, got.Status, "status must be persisted")
					// Extra may be nil after the last extra key is deleted — a nil map means
					// the key is absent, satisfying the "cleared" requirement.
					assert.Empty(t, got.Extra[ExtraLifecycleExternalBlockerReason],
						"ExtraLifecycleExternalBlockerReason must be cleared after leaving blocked")
				})
			}
		})

		// closed -> open via Store.Reopen: status changes and claim Extra is cleared.
		t.Run("closed-to-open-reopen", func(t *testing.T) {
			t.Parallel()
			s := makeStore(t)

			b := &Bead{Title: "reopen-claim-cleanup"}
			require.NoError(t, s.Create(testCtx(), b))
			require.NoError(t, s.Claim(b.ID, "worker"))
			require.NoError(t, s.Close(testCtx(), b.ID))

			pre, err := s.Get(testCtx(), b.ID)
			require.NoError(t, err)
			require.Equal(t, StatusClosed, pre.Status)
			require.NotNil(t, pre.Extra)
			require.NotEmpty(t, pre.Extra["claimed-at"], "claimed-at must be present before reopen")

			require.NoError(t, s.Reopen(b.ID, "reopen for test", ""))

			got, err := s.Get(testCtx(), b.ID)
			require.NoError(t, err)
			assert.Equal(t, StatusOpen, got.Status, "status must be open after Reopen")
			assert.Empty(t, got.Owner, "owner must be cleared on Reopen")
			require.NotNil(t, got.Extra)
			assert.Empty(t, got.Extra["claimed-at"], "claimed-at must be cleared on Reopen")
			assert.Empty(t, got.Extra["claimed-pid"], "claimed-pid must be cleared on Reopen")
		})

		// Forbidden transitions: each must return a ValidateLifecycleTransition error
		// and leave the persisted status unchanged.
		t.Run("forbidden", func(t *testing.T) {
			t.Parallel()

			type forbiddenCase struct {
				name       string
				fromStatus string
				toStatus   string
				opts       LifecycleTransitionOptions
			}

			cases := []forbiddenCase{
				// closed is terminal; every outgoing edge is rejected except ManualReopen.
				{name: "closed-to-in_progress", fromStatus: StatusClosed, toStatus: StatusInProgress},
				{name: "closed-to-blocked", fromStatus: StatusClosed, toStatus: StatusBlocked,
					opts: LifecycleTransitionOptions{ExternalBlockerReason: "reason"},
				},
				{name: "closed-to-proposed", fromStatus: StatusClosed, toStatus: StatusProposed,
					opts: LifecycleTransitionOptions{OperatorRequired: true},
				},
				// cancelled is terminal; every outgoing edge is rejected.
				{name: "cancelled-to-open", fromStatus: StatusCancelled, toStatus: StatusOpen},
				{name: "cancelled-to-in_progress", fromStatus: StatusCancelled, toStatus: StatusInProgress},
				{name: "cancelled-to-blocked", fromStatus: StatusCancelled, toStatus: StatusBlocked,
					opts: LifecycleTransitionOptions{ExternalBlockerReason: "reason"},
				},
				// in_progress -> cancelled is not in the transition matrix.
				{name: "in_progress-to-cancelled", fromStatus: StatusInProgress, toStatus: StatusCancelled},
			}

			for _, fc := range cases {
				fc := fc
				t.Run(fc.name, func(t *testing.T) {
					t.Parallel()
					s := makeStore(t)
					b := &Bead{Title: fc.name, Status: fc.fromStatus}
					require.NoError(t, s.Create(testCtx(), b))

					err := s.SetLifecycleStatus(b.ID, fc.toStatus, fc.opts)
					require.Error(t, err, "forbidden transition %s -> %s must return an error", fc.fromStatus, fc.toStatus)

					got, getErr := s.Get(testCtx(), b.ID)
					require.NoError(t, getErr)
					assert.Equal(t, fc.fromStatus, got.Status,
						"status must not change after rejected transition")
				})
			}
		})
	})
}
