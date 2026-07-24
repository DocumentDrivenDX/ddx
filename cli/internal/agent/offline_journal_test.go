package agent

import (
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOfflineJournal_AppendRecordDurability verifies the offline coordination
// journal append API (ADR-022):
//  1. Appended records survive close/reopen with sequence, idempotency key,
//     payload hash, precondition, and outcome intact.
//  2. Sequence numbers are monotonic across multiple appends before close.
//  3. Reopening continues numbering after the highest persisted sequence.
func TestOfflineJournal_AppendRecordDurability(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	journalPath := OfflineJournalPath(projectRoot)
	require.Equal(t, filepath.Join(projectRoot, ".ddx", "coordination", "offline-journal.jsonl"), journalPath)

	j, err := OpenOfflineJournal(projectRoot)
	require.NoError(t, err)
	require.Equal(t, uint64(1), j.NextSequence())

	inputs := []OfflineJournalAppend{
		{
			Operation:      "claim",
			IdempotencyKey: "idem-claim-1",
			PayloadHash:    "sha256:payload-claim-1",
			Precondition:   `{"bead_status":"open","base_rev":"abc"}`,
			Outcome:        "applied",
		},
		{
			Operation:      "tracker_transition",
			IdempotencyKey: "idem-transition-2",
			PayloadHash:    "sha256:payload-transition-2",
			Precondition:   `{"bead_status":"in_progress"}`,
			Outcome:        "applied",
		},
		{
			Operation:      "land",
			IdempotencyKey: "idem-land-3",
			PayloadHash:    "sha256:payload-land-3",
			Precondition:   `{"base_rev":"abc","result_rev":"def"}`,
			Outcome:        "conflict",
		},
	}

	appended := make([]OfflineJournalRecord, 0, len(inputs))
	for i, in := range inputs {
		rec, err := j.Append(in)
		require.NoError(t, err, "append %d", i)
		appended = append(appended, rec)

		// AC#2: monotonic across multiple appends before close.
		assert.Equal(t, uint64(i+1), rec.Sequence, "sequence for append %d", i)
		if i > 0 {
			assert.Greater(t, rec.Sequence, appended[i-1].Sequence, "monotonic at %d", i)
		}

		assert.Equal(t, in.IdempotencyKey, rec.IdempotencyKey)
		assert.Equal(t, in.PayloadHash, rec.PayloadHash)
		assert.Equal(t, in.Precondition, rec.Precondition)
		assert.Equal(t, in.Outcome, rec.Outcome)
		assert.Equal(t, in.Operation, rec.Operation)
		assert.False(t, rec.RecordedAt.IsZero())
	}

	// Distinct sequences for the pre-close batch.
	require.Len(t, appended, 3)
	assert.Equal(t, []uint64{1, 2, 3}, []uint64{
		appended[0].Sequence,
		appended[1].Sequence,
		appended[2].Sequence,
	})

	require.NoError(t, j.Close())

	// AC#1: records survive close/reopen with fields intact.
	loaded, err := LoadOfflineJournalRecords(projectRoot)
	require.NoError(t, err)
	require.Len(t, loaded, 3)

	for i, want := range appended {
		got := loaded[i]
		assert.Equal(t, want.Sequence, got.Sequence, "loaded sequence[%d]", i)
		assert.Equal(t, want.IdempotencyKey, got.IdempotencyKey, "loaded key[%d]", i)
		assert.Equal(t, want.PayloadHash, got.PayloadHash, "loaded hash[%d]", i)
		assert.Equal(t, want.Precondition, got.Precondition, "loaded precondition[%d]", i)
		assert.Equal(t, want.Outcome, got.Outcome, "loaded outcome[%d]", i)
		assert.Equal(t, want.Operation, got.Operation, "loaded operation[%d]", i)
	}

	// AC#3: reopening continues numbering after the highest persisted sequence.
	j2, err := OpenOfflineJournal(projectRoot)
	require.NoError(t, err)
	defer func() { _ = j2.Close() }()

	assert.Equal(t, uint64(4), j2.NextSequence())

	rec4, err := j2.Append(OfflineJournalAppend{
		Operation:      "claim",
		IdempotencyKey: "idem-claim-4",
		PayloadHash:    "sha256:payload-claim-4",
		Precondition:   `{"bead_status":"open"}`,
		Outcome:        "already_applied",
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(4), rec4.Sequence)
	assert.Equal(t, "idem-claim-4", rec4.IdempotencyKey)
	assert.Equal(t, "sha256:payload-claim-4", rec4.PayloadHash)
	assert.Equal(t, `{"bead_status":"open"}`, rec4.Precondition)
	assert.Equal(t, "already_applied", rec4.Outcome)

	require.NoError(t, j2.Close())

	// Final durability check: all four records present after second close.
	final, err := LoadOfflineJournalRecords(projectRoot)
	require.NoError(t, err)
	require.Len(t, final, 4)
	assert.Equal(t, uint64(1), final[0].Sequence)
	assert.Equal(t, uint64(2), final[1].Sequence)
	assert.Equal(t, uint64(3), final[2].Sequence)
	assert.Equal(t, uint64(4), final[3].Sequence)
	assert.Equal(t, "idem-claim-4", final[3].IdempotencyKey)
	assert.Equal(t, "sha256:payload-claim-4", final[3].PayloadHash)
	assert.Equal(t, `{"bead_status":"open"}`, final[3].Precondition)
	assert.Equal(t, "already_applied", final[3].Outcome)
}
