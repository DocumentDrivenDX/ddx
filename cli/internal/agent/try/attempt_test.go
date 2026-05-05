package try

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttempt_WrapsLegacyExecutor(t *testing.T) {
	out, err := Attempt(context.Background(), nil, "ddx-test", AttemptOpts{
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{BeadID: beadID, Status: StatusSuccess, ResultRev: "abc123"}, nil
		}),
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Equal(t, "ddx-test", out.Report.BeadID)
	assert.Equal(t, StatusSuccess, out.Report.Status)
	assert.Equal(t, "abc123", out.Report.ResultRev)
}

func TestAttempt_NoChanges_StaysCanonical_NotNeedsInvestigation(t *testing.T) {
	store := &attemptStore{}

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:             beadID,
				Status:             StatusNoChanges,
				NoChangesRationale: "status: needs_investigation\nreason: provider quota unknown",
			}, nil
		}),
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Equal(t, StatusNoChanges, out.Report.Status)
	require.NotNil(t, out.NoChanges)
	assert.False(t, out.NoChanges.Satisfied)
	assert.Equal(t, NoChangesEventNeedsInvestigation, out.NoChanges.EventKind)
	assert.Equal(t, NoChangesLabelNeedsInvestigation, out.NoChanges.Label)
	assert.Contains(t, out.NoChanges.EventBody, "provider quota unknown")
	assert.Equal(t, 1, store.noChangesCountCalls)
}

func TestAttempt_NoChanges_WiresAdjudicateNoChangesContract(t *testing.T) {
	store := &attemptStore{}
	runnerCalls := 0

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:             beadID,
				Status:             StatusNoChanges,
				NoChangesRationale: "verification_command: true\noutput: passes",
			}, nil
		}),
		VerificationRunner: func(ctx context.Context, projectRoot, command string) (int, string, error) {
			runnerCalls++
			assert.Equal(t, "true", command)
			return 0, "ok", nil
		},
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Equal(t, StatusAlreadySatisfied, out.Report.Status)
	require.NotNil(t, out.NoChanges)
	assert.True(t, out.NoChanges.Satisfied)
	assert.Equal(t, NoChangesEventVerified, out.NoChanges.EventKind)
	assert.Empty(t, out.NoChanges.Label)
	assert.Equal(t, 1, runnerCalls)
}

func TestAttempt_ConflictRecovered_ReturnsMergedOutcome(t *testing.T) {
	store := &attemptStore{}
	target := bead.Bead{ID: "ddx-test"}

	out, err := Attempt(context.Background(), store, target.ID, AttemptOpts{
		Bead: target,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:      beadID,
				Status:      StatusLandConflict,
				Detail:      "merge conflict",
				PreserveRef: "refs/ddx/iterations/ddx-test/attempt",
				BaseRev:     "base",
				ResultRev:   "result",
				SessionID:   "session",
			}, nil
		}),
		Store:       store,
		ProjectRoot: t.TempDir(),
		AutoRecover: func(wd, preserveRef string) (string, error) {
			return "merged-tip", nil
		},
		Assignee: "worker",
		Now:      func() time.Time { return time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC) },
		Cooldown: time.Minute,
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeSuccess, out.Disposition)
	assert.Equal(t, StatusSuccess, out.Report.Status)
	assert.Equal(t, "merged-tip", out.Report.ResultRev)
	assert.Equal(t, "merged-tip", store.closedSHA)
	require.Len(t, store.events, 1)
	assert.Equal(t, "land-conflict-auto-recovered", store.events[0].Kind)
}

func TestAttempt_ConflictUnresolvable_ReturnsPark(t *testing.T) {
	store := &attemptStore{}
	target := bead.Bead{ID: "ddx-test"}

	out, err := Attempt(context.Background(), store, target.ID, AttemptOpts{
		Bead: target,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:      beadID,
				Status:      StatusLandConflict,
				Detail:      "merge conflict",
				PreserveRef: "refs/ddx/iterations/ddx-test/attempt",
				BaseRev:     "base",
				ResultRev:   "result",
				SessionID:   "session",
			}, nil
		}),
		Store:       store,
		ProjectRoot: t.TempDir(),
		AutoRecover: func(wd, preserveRef string) (string, error) {
			return "", fmt.Errorf("cannot auto-merge")
		},
		Assignee: "worker",
		Now:      func() time.Time { return time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC) },
		Cooldown: 15 * time.Minute,
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomePark, out.Disposition)
	assert.Equal(t, StatusLandConflictUnresolvable, out.Report.Status)
	assert.True(t, store.unclaimed)
	assert.Equal(t, StatusLandConflictUnresolvable, store.cooldownStatus)
	assert.NotEmpty(t, out.Report.RetryAfter)
	require.Len(t, store.events, 1)
	assert.Equal(t, "land-conflict-unresolvable", store.events[0].Kind)
}

type attemptStore struct {
	events              []bead.BeadEvent
	closedSHA           string
	unclaimed           bool
	cooldownStatus      string
	noChangesCountCalls int
}

func (s *attemptStore) AppendEvent(beadID string, ev bead.BeadEvent) error {
	s.events = append(s.events, ev)
	return nil
}

func (s *attemptStore) CloseWithEvidence(beadID, sessionID, sha string) error {
	s.closedSHA = sha
	return nil
}

func (s *attemptStore) Unclaim(beadID string) error {
	s.unclaimed = true
	return nil
}

func (s *attemptStore) SetExecutionCooldown(beadID string, until time.Time, status, detail string) error {
	s.cooldownStatus = status
	return nil
}

func (s *attemptStore) IncrNoChangesCount(beadID string) (int, error) {
	s.noChangesCountCalls++
	return s.noChangesCountCalls, nil
}
