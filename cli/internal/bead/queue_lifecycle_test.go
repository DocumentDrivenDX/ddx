package bead

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// helpers shared across queue lifecycle tests

func createBeadWithQueueRank(t *testing.T, s *Store, rank int) *Bead {
	t.Helper()
	b := &Bead{Title: "queue-rank test bead"}
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.Update(testCtx(), b.ID, func(bead *Bead) {
		if bead.Extra == nil {
			bead.Extra = make(map[string]any)
		}
		bead.Extra[queueRankKey] = rank
	}))
	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	return got
}

func assertQueueRank(t *testing.T, s *Store, id string, want int) {
	t.Helper()
	got, err := s.Get(testCtx(), id)
	require.NoError(t, err)
	rank, ok := parseQueueRank(got.Extra[queueRankKey])
	if !ok {
		t.Errorf("parseQueueRank: no queue-rank found on bead %s (Extra=%v)", id, got.Extra)
		return
	}
	if rank != want {
		t.Errorf("queue-rank on bead %s: got %d, want %d", id, rank, want)
	}
}

func assertQueueRankInArchive(t *testing.T, s *Store, id string, want int) {
	t.Helper()
	got, err := s.GetWithArchive(testCtx(), id)
	require.NoError(t, err)
	rank, ok := parseQueueRank(got.Extra[queueRankKey])
	if !ok {
		t.Errorf("parseQueueRank: no queue-rank found in archive for bead %s (Extra=%v)", id, got.Extra)
		return
	}
	if rank != want {
		t.Errorf("queue-rank in archive for bead %s: got %d, want %d", id, rank, want)
	}
}

// TestQueueRank_PreservedAcrossClaim verifies that Claim does not delete queue-rank from Extra.
func TestQueueRank_PreservedAcrossClaim(t *testing.T) {
	s := newTestStore(t)
	b := createBeadWithQueueRank(t, s, 5)
	require.NoError(t, s.Claim(b.ID, "worker"))
	assertQueueRank(t, s, b.ID, 5)
}

// TestQueueRank_PreservedAcrossUnclaim verifies that Unclaim does not delete queue-rank from Extra.
func TestQueueRank_PreservedAcrossUnclaim(t *testing.T) {
	s := newTestStore(t)
	b := createBeadWithQueueRank(t, s, 5)
	require.NoError(t, s.Claim(b.ID, "worker"))
	require.NoError(t, s.Unclaim(b.ID))
	assertQueueRank(t, s, b.ID, 5)
}

// TestQueueRank_PreservedAcrossClose verifies that Close does not delete queue-rank from Extra.
func TestQueueRank_PreservedAcrossClose(t *testing.T) {
	s := newTestStore(t)
	b := createBeadWithQueueRank(t, s, 5)
	require.NoError(t, s.Close(testCtx(), b.ID))
	assertQueueRank(t, s, b.ID, 5)
}

// TestQueueRank_PreservedAcrossReopen verifies that Reopen does not delete queue-rank from Extra.
func TestQueueRank_PreservedAcrossReopen(t *testing.T) {
	s := newTestStore(t)
	b := createBeadWithQueueRank(t, s, 5)
	require.NoError(t, s.Close(testCtx(), b.ID))
	require.NoError(t, s.Reopen(b.ID, "reopen reason", ""))
	assertQueueRank(t, s, b.ID, 5)
}

// TestQueueRank_PreservedAcrossReconcile verifies that ReconcileLifecycleMetadata
// does not delete queue-rank from Extra.
func TestQueueRank_PreservedAcrossReconcile(t *testing.T) {
	s := newTestStore(t)
	b := createBeadWithQueueRank(t, s, 5)
	_, err := s.ReconcileLifecycleMetadata(ReconcileOptions{Apply: true, Now: time.Now().UTC()})
	require.NoError(t, err)
	assertQueueRank(t, s, b.ID, 5)
}

// TestQueueRank_PreservedAcrossMigrateLifecycle verifies that MigrateLifecycle
// does not delete queue-rank from Extra.
func TestQueueRank_PreservedAcrossMigrateLifecycle(t *testing.T) {
	s := newTestStore(t)
	b := createBeadWithQueueRank(t, s, 5)
	_, err := s.MigrateLifecycle()
	require.NoError(t, err)
	assertQueueRank(t, s, b.ID, 5)
}

// TestQueueRank_PreservedAcrossArchive verifies that ArchiveWithEvents does not
// delete queue-rank; the archived bead must carry the same rank.
func TestQueueRank_PreservedAcrossArchive(t *testing.T) {
	s := newTestStore(t)
	b := createBeadWithQueueRank(t, s, 5)
	require.NoError(t, s.Close(testCtx(), b.ID))

	policy := migratePolicy() // statuses=[closed], MinAge=0, MinActiveCount=0
	_, err := s.ArchiveWithEvents(policy)
	require.NoError(t, err)

	assertQueueRankInArchive(t, s, b.ID, 5)
}

// TestQueueRank_RoundTripViaJSONL verifies that queue-rank written as an int
// survives a JSONL write+read round-trip and parseQueueRank succeeds.
func TestQueueRank_RoundTripViaJSONL(t *testing.T) {
	s := newTestStore(t)
	b := createBeadWithQueueRank(t, s, 7)
	// Read back from disk (forces JSONL decode; int 7 will come back as float64(7)).
	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	rank, ok := parseQueueRank(got.Extra[queueRankKey])
	if !ok {
		t.Fatalf("parseQueueRank after JSONL round-trip: not ok (Extra=%v)", got.Extra)
	}
	if rank != 7 {
		t.Errorf("queue-rank after JSONL round-trip: got %d, want 7", rank)
	}
}

// TestQueueRank_SetFlagStringRoundTrip simulates `ddx bead update --set
// queue-rank=5` which stores the value as a string. The string must survive
// JSONL round-trip and parseQueueRank must return the numeric value.
func TestQueueRank_SetFlagStringRoundTrip(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{Title: "string queue-rank test"}
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.Update(testCtx(), b.ID, func(bead *Bead) {
		if bead.Extra == nil {
			bead.Extra = make(map[string]any)
		}
		bead.Extra[queueRankKey] = "5" // string, as produced by --set flag
	}))
	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	rank, ok := parseQueueRank(got.Extra[queueRankKey])
	if !ok {
		t.Fatalf("parseQueueRank with string input: not ok (Extra=%v)", got.Extra)
	}
	if rank != 5 {
		t.Errorf("parseQueueRank with string input: got %d, want 5", rank)
	}
}
