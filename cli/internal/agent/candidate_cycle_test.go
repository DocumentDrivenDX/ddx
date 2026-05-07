package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// implementationPassFunc is a test adapter for ImplementationPass.
type implementationPassFunc func(ctx context.Context, beadID string) (CandidateResult, error)

func (f implementationPassFunc) Execute(ctx context.Context, beadID string) (CandidateResult, error) {
	return f(ctx, beadID)
}

// candidateLanderFunc is a test adapter for CandidateLander.
type candidateLanderFunc func(ctx context.Context, candidate CandidateResult) (ExecuteBeadReport, error)

func (f candidateLanderFunc) Land(ctx context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
	return f(ctx, candidate)
}

func TestAttemptCycleCoordinator_SinglePassSuccessStillLands(t *testing.T) {
	landed := false
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusSuccess,
					ResultRev: "abc123",
				},
			}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			landed = true
			return candidate.Report, nil
		}),
	}

	result, err := coord.Run(context.Background(), "ddx-test-bead")
	require.NoError(t, err)
	assert.True(t, landed, "lander must be called on success")
	assert.True(t, result.Landed)
	assert.Equal(t, ExecuteBeadStatusSuccess, result.Report.Status)
}

func TestAttemptCycleCoordinator_NoChangesBehaviorUnchanged(t *testing.T) {
	landed := false
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:             beadID,
					Status:             ExecuteBeadStatusNoChanges,
					NoChangesRationale: "nothing to do",
				},
			}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			landed = true
			return candidate.Report, nil
		}),
	}

	result, err := coord.Run(context.Background(), "ddx-test-bead")
	require.NoError(t, err)
	assert.False(t, landed, "lander must not be called on no_changes")
	assert.False(t, result.Landed)
	assert.Equal(t, ExecuteBeadStatusNoChanges, result.Report.Status)
	assert.Equal(t, "nothing to do", result.Report.NoChangesRationale)
}

func TestAttemptCycleCoordinator_LiveWorktreeProtected(t *testing.T) {
	dir := t.TempDir()
	err := WriteExecutionCleanupMetadata(dir, ExecutionCleanupMetadata{
		ProjectRoot:          "/fake/project",
		BeadID:               "ddx-test-bead",
		AttemptID:            "attempt-001",
		WorktreePath:         dir,
		Registered:           true,
		ActiveCandidateCycle: true,
		CreatedAt:            time.Now().UTC(),
	})
	require.NoError(t, err)

	meta, err := ReadExecutionCleanupMetadata(dir)
	require.NoError(t, err)
	require.True(t, meta.ActiveCandidateCycle)

	probe := defaultExecutionCleanupLivenessProbe{}
	live, reason := probe.IsLive(meta, nil, time.Now())
	assert.True(t, live, "active candidate cycle worktree must be preserved by liveness probe")
	assert.Contains(t, reason, "active candidate cycle")

	// Clearing the flag must make the worktree eligible for cleanup.
	err = ClearWorktreeActiveCycle(dir)
	require.NoError(t, err)

	cleared, err := ReadExecutionCleanupMetadata(dir)
	require.NoError(t, err)
	assert.False(t, cleared.ActiveCandidateCycle)

	live2, _ := probe.IsLive(cleared, nil, time.Now())
	assert.False(t, live2, "cleared worktree must no longer be protected by active-cycle flag")
}
