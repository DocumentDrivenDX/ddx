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
	makeStore := func(t *testing.T) *Store {
		t.Helper()
		switch tc.backend {
		case BackendJSONL:
			return newJSONLStore(t)
		case BackendAxon:
			return newAxonStore(t)
		default:
			t.Fatalf("unsupported backend %q", tc.backend)
			return nil
		}
	}

	t.Run("create-get", func(t *testing.T) {
		t.Parallel()
		s := makeStore(t)

		b := &Bead{Title: "first", Description: "round-trip"}
		require.NoError(t, s.Create(b))
		require.NotEmpty(t, b.ID)

		got, err := s.Get(b.ID)
		require.NoError(t, err)
		assert.Equal(t, "first", got.Title)
		assert.Equal(t, "round-trip", got.Description)
		assert.Equal(t, StatusOpen, got.Status)
	})

	t.Run("update", func(t *testing.T) {
		t.Parallel()
		s := makeStore(t)

		b := &Bead{Title: "to-update"}
		require.NoError(t, s.Create(b))

		require.NoError(t, s.Update(b.ID, func(bb *Bead) {
			bb.Notes = "added by update"
			bb.Priority = 3
		}))

		got, err := s.Get(b.ID)
		require.NoError(t, err)
		assert.Equal(t, "added by update", got.Notes)
		assert.Equal(t, 3, got.Priority)
	})

	t.Run("claim", func(t *testing.T) {
		t.Parallel()
		s := makeStore(t)

		b := &Bead{Title: "claimable"}
		require.NoError(t, s.Create(b))

		require.NoError(t, s.Claim(b.ID, "alice"))
		got, err := s.Get(b.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusInProgress, got.Status)
		assert.Equal(t, "alice", got.Owner)
		require.NotNil(t, got.Extra)
		assert.NotEmpty(t, got.Extra["claimed-at"])

		err = s.Claim(b.ID, "bob")
		require.Error(t, err)

		require.NoError(t, s.Unclaim(b.ID))
		got, err = s.Get(b.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusOpen, got.Status)
		assert.Empty(t, got.Owner)
	})

	t.Run("deps", func(t *testing.T) {
		t.Parallel()
		s := makeStore(t)

		root := &Bead{Title: "root"}
		child := &Bead{Title: "child"}
		require.NoError(t, s.Create(root))
		require.NoError(t, s.Create(child))

		require.NoError(t, s.DepAdd(child.ID, root.ID))
		got, err := s.Get(child.ID)
		require.NoError(t, err)
		require.Len(t, got.Dependencies, 1)
		assert.Equal(t, root.ID, got.Dependencies[0].DependsOnID)

		tree, err := s.DepTree(root.ID)
		require.NoError(t, err)
		assert.Contains(t, tree, root.ID)

		require.NoError(t, s.DepRemove(child.ID, root.ID))
		got, err = s.Get(child.ID)
		require.NoError(t, err)
		assert.Empty(t, got.Dependencies)
	})

	t.Run("events-close", func(t *testing.T) {
		t.Parallel()
		s := makeStore(t)

		b := &Bead{Title: "with-events"}
		require.NoError(t, s.Create(b))

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

		require.NoError(t, s.Close(b.ID))
		got, err := s.Get(b.ID)
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
