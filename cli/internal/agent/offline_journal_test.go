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

// TestOfflineJournal_AcknowledgedResume verifies durable acknowledged-through
// resume for the offline coordination journal (ADR-022):
//  1. Acknowledged-through state survives close/reopen.
//  2. Resume/list-pending skips acknowledged mutations while preserving later
//     unacknowledged mutations in original sequence order.
//  3. Appending after acknowledgement assigns a higher monotonic sequence and
//     appears after prior pending mutations.
func TestOfflineJournal_AcknowledgedResume(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	ackPath := OfflineJournalAckPath(projectRoot)
	require.Equal(t, filepath.Join(projectRoot, ".ddx", "coordination", "offline-journal.ack"), ackPath)

	j, err := OpenOfflineJournal(projectRoot)
	require.NoError(t, err)
	require.Equal(t, uint64(0), j.AcknowledgedThrough())

	inputs := []OfflineJournalAppend{
		{
			Operation:      "claim",
			IdempotencyKey: "idem-ack-1",
			PayloadHash:    "sha256:payload-ack-1",
			Precondition:   `{"bead_status":"open"}`,
			Outcome:        "applied",
		},
		{
			Operation:      "tracker_transition",
			IdempotencyKey: "idem-ack-2",
			PayloadHash:    "sha256:payload-ack-2",
			Precondition:   `{"bead_status":"in_progress"}`,
			Outcome:        "applied",
		},
		{
			Operation:      "land",
			IdempotencyKey: "idem-ack-3",
			PayloadHash:    "sha256:payload-ack-3",
			Precondition:   `{"base_rev":"aaa"}`,
			Outcome:        "applied",
		},
		{
			Operation:      "claim",
			IdempotencyKey: "idem-ack-4",
			PayloadHash:    "sha256:payload-ack-4",
			Precondition:   `{"bead_status":"open"}`,
			Outcome:        "applied",
		},
	}

	appended := make([]OfflineJournalRecord, 0, len(inputs))
	for i, in := range inputs {
		rec, err := j.Append(in)
		require.NoError(t, err, "append %d", i)
		require.Equal(t, uint64(i+1), rec.Sequence)
		appended = append(appended, rec)
	}

	// Acknowledge contiguous sequences 1 and 2; 3 and 4 remain pending.
	require.NoError(t, j.AcknowledgeThrough(2))
	require.Equal(t, uint64(2), j.AcknowledgedThrough())

	pendingBeforeClose, err := j.ListPending()
	require.NoError(t, err)
	require.Len(t, pendingBeforeClose, 2)
	assert.Equal(t, []uint64{3, 4}, []uint64{
		pendingBeforeClose[0].Sequence,
		pendingBeforeClose[1].Sequence,
	})
	assert.Equal(t, "idem-ack-3", pendingBeforeClose[0].IdempotencyKey)
	assert.Equal(t, "idem-ack-4", pendingBeforeClose[1].IdempotencyKey)

	require.NoError(t, j.Close())

	// AC#1: acknowledged-through state survives close/reopen.
	acked, err := LoadOfflineJournalAcknowledgedThrough(projectRoot)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), acked)

	j2, err := OpenOfflineJournal(projectRoot)
	require.NoError(t, err)
	defer func() { _ = j2.Close() }()
	assert.Equal(t, uint64(2), j2.AcknowledgedThrough())

	// AC#2: resume/list-pending skips acknowledged mutations, preserves order.
	pending, err := j2.ListPending()
	require.NoError(t, err)
	require.Len(t, pending, 2)
	assert.Equal(t, uint64(3), pending[0].Sequence)
	assert.Equal(t, uint64(4), pending[1].Sequence)
	assert.Equal(t, appended[2].IdempotencyKey, pending[0].IdempotencyKey)
	assert.Equal(t, appended[2].PayloadHash, pending[0].PayloadHash)
	assert.Equal(t, appended[2].Precondition, pending[0].Precondition)
	assert.Equal(t, appended[2].Outcome, pending[0].Outcome)
	assert.Equal(t, appended[3].IdempotencyKey, pending[1].IdempotencyKey)
	assert.Equal(t, appended[3].PayloadHash, pending[1].PayloadHash)

	// Package-level pending load matches open-handle ListPending.
	loadedPending, err := LoadOfflineJournalPending(projectRoot)
	require.NoError(t, err)
	require.Len(t, loadedPending, 2)
	assert.Equal(t, pending[0].Sequence, loadedPending[0].Sequence)
	assert.Equal(t, pending[1].Sequence, loadedPending[1].Sequence)

	// Full journal still retains acknowledged rows (no physical compaction).
	all, err := LoadOfflineJournalRecords(projectRoot)
	require.NoError(t, err)
	require.Len(t, all, 4)

	// AC#3: append after acknowledgement gets a higher monotonic sequence and
	// appears after prior pending mutations.
	assert.Equal(t, uint64(5), j2.NextSequence())
	rec5, err := j2.Append(OfflineJournalAppend{
		Operation:      "tracker_transition",
		IdempotencyKey: "idem-ack-5",
		PayloadHash:    "sha256:payload-ack-5",
		Precondition:   `{"bead_status":"in_progress"}`,
		Outcome:        "applied",
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(5), rec5.Sequence)
	assert.Greater(t, rec5.Sequence, pending[1].Sequence)

	pendingAfter, err := j2.ListPending()
	require.NoError(t, err)
	require.Len(t, pendingAfter, 3)
	assert.Equal(t, []uint64{3, 4, 5}, []uint64{
		pendingAfter[0].Sequence,
		pendingAfter[1].Sequence,
		pendingAfter[2].Sequence,
	})
	assert.Equal(t, "idem-ack-3", pendingAfter[0].IdempotencyKey)
	assert.Equal(t, "idem-ack-4", pendingAfter[1].IdempotencyKey)
	assert.Equal(t, "idem-ack-5", pendingAfter[2].IdempotencyKey)
	assert.Equal(t, "sha256:payload-ack-5", pendingAfter[2].PayloadHash)

	// Advancing ack further leaves only the new mutation pending after reopen.
	require.NoError(t, j2.AcknowledgeThrough(4))
	require.NoError(t, j2.Close())

	j3, err := OpenOfflineJournal(projectRoot)
	require.NoError(t, err)
	defer func() { _ = j3.Close() }()
	assert.Equal(t, uint64(4), j3.AcknowledgedThrough())
	finalPending, err := j3.ListPending()
	require.NoError(t, err)
	require.Len(t, finalPending, 1)
	assert.Equal(t, uint64(5), finalPending[0].Sequence)
	assert.Equal(t, "idem-ack-5", finalPending[0].IdempotencyKey)
}
