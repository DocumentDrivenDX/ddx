package work

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingPhaseStore struct {
	calls []string
}

func (s *recordingPhaseStore) EmitProgress(_ context.Context, beadID string, phase Phase, outcome Outcome) error {
	s.calls = append(s.calls, string(phase)+":"+beadID)
	s.calls = append(s.calls,
		"disposition="+outcome.Disposition,
		"park_reason="+outcome.ParkReason,
	)
	return nil
}

func (s *recordingPhaseStore) EmitResult(_ context.Context, beadID string, outcome Outcome) error {
	s.calls = append(s.calls, "result:"+beadID)
	s.calls = append(s.calls,
		"status="+outcome.Status,
		"detail="+outcome.Detail,
	)
	return nil
}

func TestEmitPhase_Running_BeforeTerminal(t *testing.T) {
	store := &recordingPhaseStore{}
	ctx := context.Background()

	require.NoError(t, EmitPhase(ctx, store, "ddx-phase", PhaseQueueing, Outcome{AttemptID: "attempt-1"}))
	require.NoError(t, EmitPhase(ctx, store, "ddx-phase", PhaseRunning, Outcome{AttemptID: "attempt-1"}))
	require.NoError(t, EmitPhase(ctx, store, "ddx-phase", PhaseTerminal, Outcome{
		AttemptID:   "attempt-1",
		Status:      "preserved",
		Detail:      "manual review required",
		Disposition: "park",
		ParkReason:  "needs-human",
	}))

	assert.Equal(t, []string{
		"queueing:ddx-phase",
		"disposition=",
		"park_reason=",
		"running:ddx-phase",
		"disposition=",
		"park_reason=",
		"terminal:ddx-phase",
		"disposition=park",
		"park_reason=needs-human",
		"result:ddx-phase",
		"status=preserved",
		"detail=manual review required",
	}, store.calls)
}

func TestEmitPhase_Terminal_WritesCorrectEvent(t *testing.T) {
	store := &recordingPhaseStore{}
	ctx := context.Background()

	outcome := Outcome{
		AttemptID:          "attempt-terminal",
		Status:             "preserved",
		Detail:             "preserved for operator review",
		SessionID:          "session-terminal",
		ResultRev:          "deadbeef",
		BaseRev:            "basebeef",
		PreserveRef:        "refs/ddx/iterations/ddx-test/attempt-1",
		NoChangesRationale: "verification_command: true",
		Disposition:        "park",
		ParkReason:         "operator_required",
		DurationMS:         1234,
	}

	require.NoError(t, EmitPhase(ctx, store, "ddx-terminal", PhaseTerminal, outcome))

	require.Equal(t, []string{
		"terminal:ddx-terminal",
		"disposition=park",
		"park_reason=operator_required",
		"result:ddx-terminal",
		"status=preserved",
		"detail=preserved for operator review",
	}, store.calls)
}
