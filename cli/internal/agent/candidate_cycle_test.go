package agent

import (
	"context"
	"encoding/json"
	"errors"
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

type candidateCheckRunnerFunc func(ctx context.Context, projectRoot string, candidate CandidateResult) (CandidateCheckResult, error)

func (f candidateCheckRunnerFunc) RunChecks(ctx context.Context, projectRoot string, candidate CandidateResult) (CandidateCheckResult, error) {
	return f(ctx, projectRoot, candidate)
}

type candidateReviewerFunc func(ctx context.Context, projectRoot string, candidate CandidateResult) (CandidateReviewResult, error)

func (f candidateReviewerFunc) Review(ctx context.Context, projectRoot string, candidate CandidateResult) (CandidateReviewResult, error) {
	return f(ctx, projectRoot, candidate)
}

type inMemoryCandidateRefStore struct {
	pinned   []string
	unpinned []string
}

func (s *inMemoryCandidateRefStore) PinCandidateRef(_ string, attemptID string, cycleIndex int, _ string) (string, error) {
	ref := candidateIterationRef(attemptID, cycleIndex)
	s.pinned = append(s.pinned, ref)
	return ref, nil
}

func (s *inMemoryCandidateRefStore) UnpinCandidateRef(_ string, ref string) error {
	s.unpinned = append(s.unpinned, ref)
	return nil
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
	checksCalled := false
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
		Checks: candidateCheckRunnerFunc(func(_ context.Context, _ string, _ CandidateResult) (CandidateCheckResult, error) {
			checksCalled = true
			return CandidateCheckResult{Passed: true}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			landed = true
			return candidate.Report, nil
		}),
	}

	result, err := coord.Run(context.Background(), "ddx-test-bead")
	require.NoError(t, err)
	assert.False(t, checksCalled, "checks must not run on non-success implementation output")
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

func TestCandidateChecks_RunBeforeReview(t *testing.T) {
	refStore := &inMemoryCandidateRefStore{}
	events := &inMemoryEventAppender{}
	checksCalled := false
	reviewerCalled := false
	landerCalled := false

	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:    beadID,
					AttemptID: "attempt-checks-001",
					Status:    ExecuteBeadStatusSuccess,
					BaseRev:   "base-rev",
					ResultRev: "candidate-rev",
				},
			}, nil
		}),
		Checks: candidateCheckRunnerFunc(func(_ context.Context, projectRoot string, candidate CandidateResult) (CandidateCheckResult, error) {
			checksCalled = true
			assert.Equal(t, "/project", projectRoot)
			assert.Equal(t, "refs/ddx/iterations/attempt-checks-001/0", candidate.Report.CandidateRef)
			return CandidateCheckResult{Passed: false, Detail: "unit suite failed"}, nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, _ CandidateResult) (CandidateReviewResult, error) {
			reviewerCalled = true
			return CandidateReviewResult{Verdict: "APPROVE"}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			landerCalled = true
			return candidate.Report, nil
		}),
		RefStore:    refStore,
		ProjectRoot: "/project",
		BeadEvents:  events,
	}

	result, err := coord.Run(context.Background(), "ddx-checks-bead")
	require.NoError(t, err)
	assert.True(t, checksCalled, "candidate checks must run")
	assert.False(t, reviewerCalled, "reviewer must not run when candidate checks fail")
	assert.False(t, landerCalled, "lander must not run when candidate checks fail")
	assert.False(t, result.Landed)
	assert.Equal(t, ExecuteBeadStatusPostRunCheckFailed, result.Report.Status)
	assert.Equal(t, candidateChecksFailedEventKind, result.Report.OutcomeReason)
	assert.Contains(t, result.Report.Detail, "unit suite failed")
	assert.Equal(t, []string{"refs/ddx/iterations/attempt-checks-001/0"}, refStore.pinned)
	assert.Empty(t, refStore.unpinned, "failed candidates must retain their project-root ref")
	require.Len(t, events.events, 2)
	assert.Equal(t, candidateChecksFailedEventKind, events.events[1].Kind)
}

func TestCandidateChecks_FinalLandChecksStillRun(t *testing.T) {
	var order []string
	refStore := &inMemoryCandidateRefStore{}

	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:    beadID,
					AttemptID: "attempt-checks-002",
					Status:    ExecuteBeadStatusSuccess,
					BaseRev:   "base-rev",
					ResultRev: "candidate-rev",
				},
			}, nil
		}),
		Checks: candidateCheckRunnerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateCheckResult, error) {
			order = append(order, "candidate-checks")
			require.NotEmpty(t, candidate.Report.CandidateRef, "candidate checks must see the pinned candidate ref")
			return CandidateCheckResult{Passed: true, Detail: "candidate checks passed"}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			order = append(order, "final-land-checks")
			require.NotEmpty(t, candidate.Report.CandidateRef, "final land gate must still receive candidate metadata")
			return candidate.Report, nil
		}),
		RefStore:    refStore,
		ProjectRoot: "/project",
	}

	result, err := coord.Run(context.Background(), "ddx-checks-bead")
	require.NoError(t, err)
	assert.True(t, result.Landed)
	assert.Equal(t, []string{"candidate-checks", "final-land-checks"}, order)
	assert.Equal(t, []string{"refs/ddx/iterations/attempt-checks-002/0"}, refStore.unpinned)
}

func TestCandidateChecks_ArtifactsRecorded(t *testing.T) {
	events := &inMemoryEventAppender{}
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:    beadID,
					AttemptID: "attempt-checks-003",
					Status:    ExecuteBeadStatusSuccess,
					BaseRev:   "base-rev",
					ResultRev: "candidate-rev",
				},
			}, nil
		}),
		Checks: candidateCheckRunnerFunc(func(_ context.Context, _ string, _ CandidateResult) (CandidateCheckResult, error) {
			return CandidateCheckResult{
				Passed:    false,
				Detail:    "lint blocked candidate",
				Artifacts: []string{".ddx/executions/attempt-checks-003/checks/lint.json"},
			}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			return candidate.Report, nil
		}),
		RefStore:    &inMemoryCandidateRefStore{},
		ProjectRoot: "/project",
		BeadEvents:  events,
	}

	result, err := coord.Run(context.Background(), "ddx-checks-bead")
	require.NoError(t, err)
	assert.False(t, result.Landed)

	var failed beadEventWithBody
	for _, event := range events.events {
		if event.Kind == candidateChecksFailedEventKind {
			failed = beadEventWithBody{kind: event.Kind, body: event.Body}
			break
		}
	}
	require.Equal(t, candidateChecksFailedEventKind, failed.kind)

	var body CandidateChecksFailedEventBody
	require.NoError(t, json.Unmarshal([]byte(failed.body), &body))
	assert.Equal(t, "refs/ddx/iterations/attempt-checks-003/0", body.CandidateRef)
	assert.Equal(t, "attempt-checks-003", body.AttemptID)
	assert.Equal(t, "base-rev", body.BaseRev)
	assert.Equal(t, "candidate-rev", body.ResultRev)
	assert.Equal(t, "lint blocked candidate", body.Detail)
	assert.Equal(t, []string{".ddx/executions/attempt-checks-003/checks/lint.json"}, body.Artifacts)
}

func TestCandidateChecks_ErrorStopsBeforeLand(t *testing.T) {
	events := &inMemoryEventAppender{}
	landerCalled := false
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:    beadID,
					AttemptID: "attempt-checks-004",
					Status:    ExecuteBeadStatusSuccess,
					BaseRev:   "base-rev",
					ResultRev: "candidate-rev",
				},
			}, nil
		}),
		Checks: candidateCheckRunnerFunc(func(_ context.Context, _ string, _ CandidateResult) (CandidateCheckResult, error) {
			return CandidateCheckResult{}, errors.New("check runner crashed")
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			landerCalled = true
			return candidate.Report, nil
		}),
		RefStore:    &inMemoryCandidateRefStore{},
		ProjectRoot: "/project",
		BeadEvents:  events,
	}

	result, err := coord.Run(context.Background(), "ddx-checks-bead")
	require.NoError(t, err)
	assert.False(t, result.Landed)
	assert.False(t, landerCalled)
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.Report.Status)
	assert.Equal(t, "check runner crashed", result.Report.Error)
	assert.Contains(t, result.Report.Detail, "check runner crashed")

	var body CandidateChecksFailedEventBody
	for _, event := range events.events {
		if event.Kind != candidateChecksFailedEventKind {
			continue
		}
		require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
		break
	}
	assert.Equal(t, "check runner crashed", body.Detail)
}

type beadEventWithBody struct {
	kind string
	body string
}
