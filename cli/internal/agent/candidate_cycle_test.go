package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
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

type repairPassFunc func(ctx context.Context, candidate CandidateResult, prompt string) (CandidateResult, error)

func (f repairPassFunc) Repair(ctx context.Context, candidate CandidateResult, prompt string) (CandidateResult, error) {
	return f(ctx, candidate, prompt)
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

func TestCandidateCycleState_WritesMetadataAndRunState(t *testing.T) {
	projectRoot := t.TempDir()
	dir := t.TempDir()
	attemptID := "attempt-cycle-state"
	require.NoError(t, WriteExecutionCleanupMetadata(dir, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-cycle-state",
		AttemptID:    attemptID,
		WorktreePath: dir,
		Registered:   true,
	}))
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-cycle-state",
		AttemptID:    attemptID,
		StartedAt:    time.Now().UTC(),
		WorktreePath: dir,
	}))

	state := CandidateCycleState{
		Active:       true,
		Phase:        "repair",
		CandidateRef: "refs/ddx/iterations/attempt-cycle-state/1",
		CandidateRev: "cafed00d",
		CycleIndex:   1,
		RepairActive: true,
	}
	require.NoError(t, WriteWorktreeCandidateCycleState(projectRoot, dir, attemptID, state))

	meta, err := ReadExecutionCleanupMetadata(dir)
	require.NoError(t, err)
	assert.True(t, meta.ActiveCandidateCycle)
	assert.Equal(t, "repair", meta.CandidateCyclePhase)
	assert.Equal(t, state.CandidateRef, meta.CandidateRef)
	assert.Equal(t, state.CandidateRev, meta.CandidateRev)
	assert.Equal(t, 1, meta.CycleIndex)
	assert.True(t, meta.RepairActive)

	runState, err := ReadRunState(projectRoot)
	require.NoError(t, err)
	require.NotNil(t, runState)
	assert.Equal(t, "repair", runState.CandidateCyclePhase)
	assert.Equal(t, state.CandidateRef, runState.CandidateRef)
	assert.Equal(t, state.CandidateRev, runState.CandidateRev)
	assert.Equal(t, 1, runState.CycleIndex)
	assert.True(t, runState.RepairActive)
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

func TestPreLandReview_ApprovesThenLands(t *testing.T) {
	refStore := &inMemoryCandidateRefStore{}
	reviewerCalled := false
	landerCalled := false
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:    beadID,
					AttemptID: "attempt-review-001",
					Status:    ExecuteBeadStatusSuccess,
					BaseRev:   "base-rev",
					ResultRev: "candidate-rev",
				},
				WorktreePath: "/attempt/worktree",
			}, nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, projectRoot string, candidate CandidateResult) (CandidateReviewResult, error) {
			reviewerCalled = true
			assert.Equal(t, "/project", projectRoot)
			assert.Equal(t, "/attempt/worktree", candidate.WorktreePath)
			return CandidateReviewResult{Verdict: "APPROVE", Rationale: "ready to land"}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			landerCalled = true
			assert.Equal(t, "APPROVE", candidate.Report.ReviewVerdict)
			assert.Equal(t, "ready to land", candidate.Report.ReviewRationale)
			return candidate.Report, nil
		}),
		RefStore:    refStore,
		ProjectRoot: "/project",
	}

	result, err := coord.Run(context.Background(), "ddx-review-bead")
	require.NoError(t, err)
	assert.True(t, reviewerCalled)
	assert.True(t, landerCalled)
	assert.True(t, result.Landed)
	assert.Equal(t, ExecuteBeadStatusSuccess, result.Report.Status)
	assert.Equal(t, []string{"refs/ddx/iterations/attempt-review-001/0"}, refStore.unpinned)
}

func TestPreLandReview_RequestChangesPreventsLand(t *testing.T) {
	tests := []struct {
		name       string
		review     CandidateReviewResult
		reviewErr  error
		wantStatus string
		wantDetail string
	}{
		{
			name:       "request changes",
			review:     CandidateReviewResult{Verdict: "REQUEST_CHANGES", Rationale: "missing AC evidence"},
			wantStatus: ExecuteBeadStatusReviewRequestChanges,
			wantDetail: "pre-land review: REQUEST_CHANGES",
		},
		{
			name:       "block",
			review:     CandidateReviewResult{Verdict: "BLOCK", Rationale: "unsafe scope", Findings: []Finding{{Severity: "block", Summary: "unsafe scope", Location: "bead:AC1"}}},
			wantStatus: ExecuteBeadStatusReviewBlock,
			wantDetail: "pre-land review: BLOCK",
		},
		{
			name:       "review error",
			reviewErr:  errors.New("review provider empty"),
			wantStatus: ExecuteBeadStatusReviewMalfunction,
			wantDetail: "pre-land review: review provider empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			refStore := &inMemoryCandidateRefStore{}
			landerCalled := false
			coord := &AttemptCycleCoordinator{
				Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
					return CandidateResult{
						Report: ExecuteBeadReport{
							BeadID:    beadID,
							AttemptID: "attempt-review-002",
							Status:    ExecuteBeadStatusSuccess,
							BaseRev:   "base-rev",
							ResultRev: "candidate-rev",
						},
					}, nil
				}),
				Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, _ CandidateResult) (CandidateReviewResult, error) {
					return tc.review, tc.reviewErr
				}),
				Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
					landerCalled = true
					return candidate.Report, nil
				}),
				RefStore:    refStore,
				ProjectRoot: "/project",
			}

			result, err := coord.Run(context.Background(), "ddx-review-bead")
			require.NoError(t, err)
			assert.False(t, result.Landed)
			assert.False(t, landerCalled)
			assert.Equal(t, tc.wantStatus, result.Report.Status)
			assert.Equal(t, tc.wantDetail, result.Report.Detail)
			assert.Empty(t, refStore.unpinned, "rejected review candidates must retain their ref")
		})
	}
}

func TestCandidateCycle_RetriesProviderEmptyReviewThenSucceeds(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	reviewCalls := 0
	reviewRoute := ExecutionCycleRouteFacts{
		Harness:     "claude",
		Provider:    "anthropic",
		Model:       "claude-sonnet-4-5",
		ActualPower: 71,
	}

	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:      beadID,
					AttemptID:   "attempt-review-retry-001",
					Status:      ExecuteBeadStatusSuccess,
					BaseRev:     "base-rev",
					ResultRev:   "candidate-rev",
					Harness:     "codex",
					Provider:    "openai",
					Model:       "gpt-5",
					ActualPower: 70,
				},
			}, nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateReviewResult, error) {
			reviewCalls++
			if reviewCalls <= 2 {
				return CandidateReviewResult{
					ReviewerRoute: reviewRoute,
				}, fmt.Errorf("reviewer: %s: %w", evidence.OutcomeReviewProviderEmpty, errors.New("provider returned zero bytes"))
			}
			assert.Equal(t, "candidate-rev", candidate.Report.ResultRev)
			return CandidateReviewResult{
				Verdict:       "APPROVE",
				Rationale:     "looks good",
				ReviewerRoute: reviewRoute,
			}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			return candidate.Report, nil
		}),
		BeadEvents: store,
	}

	result, err := coord.Run(context.Background(), first.ID)
	require.NoError(t, err)
	require.True(t, result.Landed)
	assert.Equal(t, ExecuteBeadStatusSuccess, result.Report.Status)
	assert.Equal(t, 3, reviewCalls, "two provider_empty retries plus the successful approval")
	assert.Equal(t, 2, result.Report.EscalationCount, "report must record the two retry attempts")
	require.Len(t, result.Report.CycleTrace, 1)
	assert.Equal(t, 2, result.Report.CycleTrace[0].EscalationCount, "cycle trace must preserve the review retry count")

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var retryEvents []string
	for _, event := range events {
		if event.Kind == "review-error" {
			retryEvents = append(retryEvents, event.Body)
		}
	}
	require.Len(t, retryEvents, 2)
	assert.Contains(t, retryEvents[0], "attempt_count=1")
	assert.Contains(t, retryEvents[1], "attempt_count=2")
	assert.Contains(t, retryEvents[0], "review route harness=claude model=claude-sonnet-4-5 provider=anthropic")
	assert.Contains(t, retryEvents[1], "review route harness=claude model=claude-sonnet-4-5 provider=anthropic")
}

func TestCandidateCycle_ProviderEmptyRetryDoesNotRerunChecks(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	validateCalls := 0
	importCalls := 0
	releaseCalls := 0
	checksCalls := 0
	reviewCalls := 0
	reviewRoute := ExecutionCycleRouteFacts{
		Harness:     "claude",
		Provider:    "anthropic",
		Model:       "claude-sonnet-4-5",
		ActualPower: 71,
	}

	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:      beadID,
					AttemptID:   "attempt-review-retry-003",
					Status:      ExecuteBeadStatusSuccess,
					BaseRev:     "base-rev",
					ResultRev:   "candidate-rev",
					Harness:     "codex",
					Provider:    "openai",
					Model:       "gpt-5",
					ActualPower: 70,
				},
			}, nil
		}),
		ValidateCandidate: func(candidate CandidateResult) error {
			validateCalls++
			assert.Equal(t, "candidate-rev", candidate.Report.ResultRev)
			return nil
		},
		ImportCandidate: func(candidate CandidateResult) error {
			importCalls++
			assert.Equal(t, "candidate-rev", candidate.Report.ResultRev)
			return nil
		},
		ReleaseCandidateImport: func(candidate CandidateResult) error {
			releaseCalls++
			assert.Equal(t, "candidate-rev", candidate.Report.ResultRev)
			return nil
		},
		Checks: candidateCheckRunnerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateCheckResult, error) {
			checksCalls++
			assert.Equal(t, "candidate-rev", candidate.Report.ResultRev)
			return CandidateCheckResult{Passed: true, Detail: "checks passed"}, nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateReviewResult, error) {
			reviewCalls++
			assert.Equal(t, "candidate-rev", candidate.Report.ResultRev)
			if reviewCalls == 1 {
				return CandidateReviewResult{
					ReviewerRoute: reviewRoute,
				}, fmt.Errorf("reviewer: %s: %w", evidence.OutcomeReviewProviderEmpty, errors.New("provider returned zero bytes"))
			}
			return CandidateReviewResult{
				Verdict:       "APPROVE",
				Rationale:     "looks good",
				ReviewerRoute: reviewRoute,
			}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			return candidate.Report, nil
		}),
		BeadEvents: store,
	}

	result, err := coord.Run(context.Background(), first.ID)
	require.NoError(t, err)
	require.True(t, result.Landed)
	assert.Equal(t, ExecuteBeadStatusSuccess, result.Report.Status)
	assert.Equal(t, 2, reviewCalls, "the reviewer should retry once after provider_empty")
	assert.Equal(t, 1, validateCalls, "validation must not rerun for an unchanged candidate revision")
	assert.Equal(t, 1, importCalls, "import must not rerun for an unchanged candidate revision")
	assert.Equal(t, 1, releaseCalls, "import release must not rerun for an unchanged candidate revision")
	assert.Equal(t, 1, checksCalls, "RunChecks must run once for an unchanged candidate revision")
	assert.Equal(t, 1, result.Report.EscalationCount, "one provider_empty retry should be recorded")
	require.Len(t, result.Report.CycleTrace, 1)
	assert.Equal(t, 1, result.Report.CycleTrace[0].EscalationCount, "cycle trace must preserve the retry count")

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var retryEvents []string
	for _, event := range events {
		if event.Kind == "review-error" {
			retryEvents = append(retryEvents, event.Body)
		}
	}
	require.Len(t, retryEvents, 1)
	assert.Contains(t, retryEvents[0], "attempt_count=1")
	assert.Contains(t, retryEvents[0], "review route harness=claude model=claude-sonnet-4-5 provider=anthropic")
}

func TestCandidateCycle_ProviderEmptyExhaustsRetriesParksWithRoute(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	reviewCalls := 0
	reviewRoute := ExecutionCycleRouteFacts{
		Harness:     "claude",
		Provider:    "anthropic",
		Model:       "claude-sonnet-4-5",
		ActualPower: 71,
	}

	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:      beadID,
					AttemptID:   "attempt-review-retry-002",
					Status:      ExecuteBeadStatusSuccess,
					BaseRev:     "base-rev",
					ResultRev:   "candidate-rev",
					Harness:     "codex",
					Provider:    "openai",
					Model:       "gpt-5",
					ActualPower: 70,
				},
			}, nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, _ CandidateResult) (CandidateReviewResult, error) {
			reviewCalls++
			return CandidateReviewResult{
				ReviewerRoute: reviewRoute,
			}, fmt.Errorf("reviewer: %s: %w", evidence.OutcomeReviewProviderEmpty, errors.New("provider returned zero bytes"))
		}),
		BeadEvents: store,
	}

	result, err := coord.Run(context.Background(), first.ID)
	require.NoError(t, err)
	require.False(t, result.Landed)
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, result.Report.Status)
	assert.Equal(t, 3, reviewCalls, "the coordinator must stop after the third provider_empty failure")
	assert.Equal(t, 2, result.Report.EscalationCount, "the report must carry the retry count")
	assert.Contains(t, result.Report.Detail, "harness=claude")
	assert.Contains(t, result.Report.Detail, "model=claude-sonnet-4-5")
	assert.Contains(t, result.Report.Detail, "retries=2")

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var retryEvents []string
	for _, event := range events {
		if event.Kind == "review-error" {
			retryEvents = append(retryEvents, event.Body)
		}
	}
	require.Len(t, retryEvents, 3)
	assert.Contains(t, retryEvents[0], "attempt_count=1")
	assert.Contains(t, retryEvents[1], "attempt_count=2")
	assert.Contains(t, retryEvents[2], "attempt_count=3")
	assert.Contains(t, retryEvents[2], "review route harness=claude model=claude-sonnet-4-5 provider=anthropic")
}

func TestCandidateCycle_GenuineVerdictNotRetried(t *testing.T) {
	t.Run("request_changes", func(t *testing.T) {
		_, first, _ := newExecuteLoopTestStore(t)
		reviewCalls := 0
		reviewRoute := ExecutionCycleRouteFacts{
			Harness:     "claude",
			Provider:    "anthropic",
			Model:       "claude-sonnet-4-5",
			ActualPower: 71,
		}

		coord := &AttemptCycleCoordinator{
			Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
				return CandidateResult{
					Report: ExecuteBeadReport{
						BeadID:    beadID,
						AttemptID: "attempt-review-verdict-001",
						Status:    ExecuteBeadStatusSuccess,
						BaseRev:   "base-rev",
						ResultRev: "candidate-rev",
					},
				}, nil
			}),
			Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, _ CandidateResult) (CandidateReviewResult, error) {
				reviewCalls++
				return CandidateReviewResult{
					Verdict:       "REQUEST_CHANGES",
					Rationale:     "missing coverage",
					ReviewerRoute: reviewRoute,
				}, nil
			}),
		}

		result, err := coord.Run(context.Background(), first.ID)
		require.NoError(t, err)
		require.False(t, result.Landed)
		assert.Equal(t, 1, reviewCalls)
		assert.Equal(t, ExecuteBeadStatusReviewRequestChanges, result.Report.Status)
		assert.Equal(t, "pre-land review: REQUEST_CHANGES", result.Report.Detail)
		assert.Equal(t, 0, result.Report.EscalationCount)
	})

	t.Run("unparseable", func(t *testing.T) {
		store, first, _ := newExecuteLoopTestStore(t)
		reviewCalls := 0
		reviewRoute := ExecutionCycleRouteFacts{
			Harness:     "claude",
			Provider:    "anthropic",
			Model:       "claude-sonnet-4-5",
			ActualPower: 71,
		}

		coord := &AttemptCycleCoordinator{
			Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
				return CandidateResult{
					Report: ExecuteBeadReport{
						BeadID:    beadID,
						AttemptID: "attempt-review-verdict-002",
						Status:    ExecuteBeadStatusSuccess,
						BaseRev:   "base-rev",
						ResultRev: "candidate-rev",
					},
				}, nil
			}),
			Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, _ CandidateResult) (CandidateReviewResult, error) {
				reviewCalls++
				return CandidateReviewResult{
					ReviewerRoute: reviewRoute,
				}, fmt.Errorf("reviewer: %s: %w", evidence.OutcomeReviewUnparseable, errors.New("structured verdict missing"))
			}),
			BeadEvents: store,
		}

		result, err := coord.Run(context.Background(), first.ID)
		require.NoError(t, err)
		require.False(t, result.Landed)
		assert.Equal(t, 1, reviewCalls)
		assert.Equal(t, ExecuteBeadStatusReviewMalfunction, result.Report.Status)
		assert.Contains(t, result.Report.Detail, evidence.OutcomeReviewUnparseable)
		assert.Equal(t, 0, result.Report.EscalationCount)

		events, err := store.Events(first.ID)
		require.NoError(t, err)
		for _, event := range events {
			assert.NotEqual(t, "review-error", event.Kind, "unparseable verdicts are not provider-empty retries")
		}
	})
}

func TestReviewClassification_CandidateCycleEventIncludesCandidateRev(t *testing.T) {
	events := &inMemoryEventAppender{}
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:    beadID,
					AttemptID: "attempt-review-class-001",
					Status:    ExecuteBeadStatusSuccess,
					BaseRev:   "base-rev",
					ResultRev: "candidate-rev",
				},
			}, nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, _ CandidateResult) (CandidateReviewResult, error) {
			return CandidateReviewResult{
				Verdict:   "BLOCK",
				Rationale: "scope problem",
				Findings: []Finding{
					{Severity: "block", Summary: "Forbidden out-of-scope file change.", Location: "cli/internal/fizeauadapter/router.go:12"},
				},
			}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			return candidate.Report, nil
		}),
		RefStore:    &inMemoryCandidateRefStore{},
		ProjectRoot: "/project",
		BeadEvents:  events,
	}

	result, err := coord.Run(context.Background(), "ddx-review-bead")
	require.NoError(t, err)
	assert.False(t, result.Landed)
	assert.Equal(t, ExecuteBeadStatusReviewBlock, result.Report.Status)

	var body CandidateReviewClassifiedEventBody
	foundClassified := false
	for _, event := range events.events {
		if event.Kind != candidateReviewClassifiedEventKind {
			continue
		}
		foundClassified = true
		require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
		assert.Equal(t, ReviewTerminalClassUnsafeOrOutScope, event.Summary)
		break
	}
	require.True(t, foundClassified, "candidate review classification event must be emitted")
	assert.Equal(t, "refs/ddx/iterations/attempt-review-class-001/0", body.CandidateRef)
	assert.Equal(t, "attempt-review-class-001", body.AttemptID)
	assert.Equal(t, "base-rev", body.BaseRev)
	assert.Equal(t, "candidate-rev", body.ResultRev)
	assert.Equal(t, "BLOCK", body.Verdict)
	assert.Equal(t, ReviewTerminalClassUnsafeOrOutScope, body.Classification)
}

func TestExecuteBead_CandidateCycleRunsConfiguredReviewer(t *testing.T) {
	reviewCalls := 0
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:      beadID,
					AttemptID:   "attempt-configured-review-001",
					Status:      ExecuteBeadStatusSuccess,
					BaseRev:     "base-rev",
					ResultRev:   "candidate-rev",
					Harness:     "codex",
					Provider:    "openai",
					Model:       "gpt-5",
					ActualPower: 70,
				},
				WorktreePath: "/attempt/worktree",
			}, nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, projectRoot string, candidate CandidateResult) (CandidateReviewResult, error) {
			reviewCalls++
			assert.Equal(t, "/project", projectRoot)
			assert.Equal(t, "candidate-rev", candidate.Report.ResultRev)
			return CandidateReviewResult{
				Verdict:         "APPROVE",
				Rationale:       "ready to land",
				ReviewGroupID:   "rg-configured-review",
				ReviewerIndices: []int{0},
				ReviewerVerdicts: []string{
					"APPROVE",
				},
				ReviewerRoute: ExecutionCycleRouteFacts{
					Harness:     "claude",
					Provider:    "anthropic",
					Model:       "claude-opus-4-7",
					ActualPower: 71,
				},
			}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			return candidate.Report, nil
		}),
		RefStore:    &inMemoryCandidateRefStore{},
		ProjectRoot: "/project",
	}

	result, err := coord.Run(context.Background(), "ddx-configured-review-bead")
	require.NoError(t, err)
	require.True(t, result.Landed)
	require.Len(t, result.Report.CycleTrace, 1)
	assert.Equal(t, 1, reviewCalls, "configured reviewer must be invoked exactly once")

	trace := result.Report.CycleTrace[0]
	assert.Equal(t, "completed", trace.ReviewStatus)
	assert.Equal(t, "APPROVE", trace.ReviewResult.Verdict)
	assert.Equal(t, "rg-configured-review", trace.ReviewGroupID)
	assert.Equal(t, "claude", trace.ReviewerRoute.Harness)
	assert.Equal(t, "anthropic", trace.ReviewerRoute.Provider)
	assert.Equal(t, "claude-opus-4-7", trace.ReviewerRoute.Model)
	assert.Equal(t, 71, trace.ReviewerRoute.ActualPower)
	assert.GreaterOrEqual(t, trace.ReviewerRoute.ActualPower, trace.ImplementerRoute.ActualPower+1)
}

func TestRepairCycle_RequestChangesAppendsRepairCommit(t *testing.T) {
	var repairCalled bool
	var landed CandidateResult
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return repairCycleCandidate(beadID, "attempt-repair-001", "base-rev", "candidate-rev", 0), nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateReviewResult, error) {
			if candidate.Report.ResultRev == "repair-rev" {
				return CandidateReviewResult{Verdict: "APPROVE", Rationale: "repair complete"}, nil
			}
			return repairCycleFixableReview(), nil
		}),
		Repair: repairPassFunc(func(_ context.Context, candidate CandidateResult, prompt string) (CandidateResult, error) {
			repairCalled = true
			assert.Equal(t, "/attempt/worktree", candidate.WorktreePath)
			assert.Contains(t, prompt, "failed_candidate_rev: candidate-rev")
			return repairCycleCandidate(candidate.Report.BeadID, candidate.Report.AttemptID, candidate.Report.BaseRev, "repair-rev", 1), nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			landed = candidate
			return candidate.Report, nil
		}),
		RefStore:    &inMemoryCandidateRefStore{},
		ProjectRoot: "/project",
	}

	result, err := coord.Run(context.Background(), "ddx-repair-bead")
	require.NoError(t, err)
	assert.True(t, repairCalled)
	assert.True(t, result.Landed)
	assert.Equal(t, "repair-rev", landed.Report.ResultRev)
	assert.Equal(t, 1, landed.CycleIndex)
	assert.Equal(t, "APPROVE", landed.Report.ReviewVerdict)
}

func TestRepairCycle_PromptIncludesReviewFindings(t *testing.T) {
	prompt := BuildRepairPrompt(RepairPromptInput{
		BeadID:             "ddx-repair-bead",
		BaseRev:            "base-rev",
		FailedCandidateRev: "failed-rev",
		CycleIndex:         1,
		ReviewRationale:    "AC#2 test missing",
		PerAC: []ReviewAC{
			{Number: 2, Item: "Add regression test", Grade: "REQUEST_CHANGES", Evidence: "no TestRepairCycle coverage"},
		},
		Findings: []Finding{
			{Severity: "warn", Summary: "missing repair regression", Location: "cli/internal/agent/candidate_cycle_test.go:1"},
		},
		VerificationCommands: []string{"cd cli && go test ./internal/agent/... -run TestRepairCycle"},
	})

	for _, want := range []string{
		"base_rev: base-rev",
		"failed_candidate_rev: failed-rev",
		"AC#2 test missing",
		"REQUEST_CHANGES",
		"missing repair regression",
		"cd cli && go test ./internal/agent/... -run TestRepairCycle",
		"Make exactly one append-only repair commit",
		"Do not reset, amend, squash, rebase",
	} {
		assert.Contains(t, prompt, want)
	}
}

func TestRepairCycle_NoHistoryRewrite(t *testing.T) {
	projectRoot, baseRev := initTestGitRepo(t)
	implRev := commitTestFile(t, projectRoot, "impl.txt", "impl\n", "implementation commit")
	var repairRev string

	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:    beadID,
					AttemptID: "attempt-repair-history",
					Status:    ExecuteBeadStatusSuccess,
					BaseRev:   baseRev,
					ResultRev: implRev,
				},
				WorktreePath: projectRoot,
			}, nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateReviewResult, error) {
			if candidate.Report.ResultRev == repairRev && repairRev != "" {
				return CandidateReviewResult{Verdict: "APPROVE", Rationale: "history ok"}, nil
			}
			return repairCycleFixableReview(), nil
		}),
		Repair: repairPassFunc(func(_ context.Context, candidate CandidateResult, _ string) (CandidateResult, error) {
			repairRev = commitTestFile(t, projectRoot, "repair.txt", "repair\n", "repair commit")
			repaired := candidate
			repaired.Report.ResultRev = repairRev
			repaired.CycleIndex = 1
			return repaired, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			return candidate.Report, nil
		}),
		RefStore:    &GitCandidateRefStore{},
		ProjectRoot: projectRoot,
	}

	result, err := coord.Run(context.Background(), "ddx-repair-bead")
	require.NoError(t, err)
	assert.True(t, result.Landed)
	assert.NotEmpty(t, repairRev)

	logOut, err := exec.Command("git", "-C", projectRoot, "log", "--format=%s", baseRev+".."+repairRev).Output()
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(logOut)), "\n")
	assert.Equal(t, []string{"repair commit", "implementation commit"}, lines)
}

func TestRepairCycleEvidenceHistoryRejectedBeforeRePinOrLand(t *testing.T) {
	projectRoot, baseRev := initTestGitRepo(t)
	wtPath := filepath.Join(t.TempDir(), "candidate")
	out, err := exec.Command("git", "-C", projectRoot, "worktree", "add", "--detach", wtPath, baseRev).CombinedOutput()
	require.NoError(t, err, "git worktree add: %s", out)
	t.Cleanup(func() {
		_, _ = exec.Command("git", "-C", projectRoot, "worktree", "remove", "--force", wtPath).CombinedOutput()
	})
	implRev := commitTestFile(t, wtPath, "implementation.txt", "implementation\n", "implementation commit")

	landCalls := 0
	reviewCalls := 0
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:            beadID,
					AttemptID:         "attempt-repair-evidence",
					Status:            ExecuteBeadStatusSuccess,
					BaseRev:           baseRev,
					ResultRev:         implRev,
					ImplementationRev: implRev,
				},
				WorktreePath: wtPath,
			}, nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateReviewResult, error) {
			reviewCalls++
			return repairCycleFixableReview(), nil
		}),
		Repair: repairPassFunc(func(_ context.Context, candidate CandidateResult, _ string) (CandidateResult, error) {
			evidenceRel := filepath.Join(ddxroot.DirName, "executions", "attempt-repair-evidence", "report.md")
			require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(wtPath, evidenceRel)), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(wtPath, evidenceRel), []byte("local evidence\n"), 0o644))
			out, addErr := exec.Command("git", "-C", wtPath, "add", "-f", "--", evidenceRel).CombinedOutput()
			require.NoError(t, addErr, "git add evidence: %s", out)
			out, commitErr := exec.Command("git", "-C", wtPath, "commit", "-m", "repair: force-add local evidence").CombinedOutput()
			require.NoError(t, commitErr, "git commit evidence: %s", out)
			out, rmErr := exec.Command("git", "-C", wtPath, "rm", "--", evidenceRel).CombinedOutput()
			require.NoError(t, rmErr, "git rm evidence: %s", out)
			out, commitErr = exec.Command("git", "-C", wtPath, "commit", "-m", "repair: delete local evidence").CombinedOutput()
			require.NoError(t, commitErr, "git commit evidence deletion: %s", out)
			repairedRev, revErr := exec.Command("git", "-C", wtPath, "rev-parse", "HEAD").Output()
			require.NoError(t, revErr)
			repaired := candidate
			repaired.Report.ResultRev = strings.TrimSpace(string(repairedRev))
			repaired.CycleIndex = 1
			return repaired, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			landCalls++
			return candidate.Report, nil
		}),
		RefStore:    &GitCandidateRefStore{},
		ProjectRoot: projectRoot,
		ValidateCandidate: func(candidate CandidateResult) error {
			return VerifyCandidateHasNoExecutionEvidence(candidate.WorktreePath, candidate.Report.BaseRev, candidate.Report.ResultRev)
		},
	}

	result, err := coord.Run(context.Background(), "ddx-repair-evidence")
	require.ErrorContains(t, err, "candidate history commit")
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.Report.Status)
	assert.Equal(t, FailureModeAttemptIntegrity, result.Report.OutcomeReason)
	assert.Equal(t, 1, reviewCalls, "repaired candidate must be rejected before a second review")
	assert.Zero(t, landCalls, "evidence-bearing repaired candidate must not land")
	refsOut, refsErr := exec.Command("git", "-C", projectRoot, "for-each-ref", "--format=%(refname)", "refs/ddx/iterations/attempt-repair-evidence/").Output()
	require.NoError(t, refsErr)
	assert.Empty(t, strings.TrimSpace(string(refsOut)), "integrity rejection must retain no candidate ref")
	mainRev, mainErr := exec.Command("git", "-C", projectRoot, "rev-parse", "refs/heads/main").Output()
	require.NoError(t, mainErr)
	assert.Equal(t, baseRev, strings.TrimSpace(string(mainRev)), "integrity rejection must not move the target")
}

func TestRepairCycle_RerunsChecksAndReview(t *testing.T) {
	var checked []string
	var reviewed []string
	refStore := &inMemoryCandidateRefStore{}
	events := &inMemoryEventAppender{}
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return repairCycleCandidate(beadID, "attempt-repair-rerun", "base-rev", "candidate-rev", 0), nil
		}),
		Checks: candidateCheckRunnerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateCheckResult, error) {
			checked = append(checked, candidate.Report.ResultRev)
			return CandidateCheckResult{Passed: true}, nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateReviewResult, error) {
			reviewed = append(reviewed, candidate.Report.ResultRev)
			if candidate.Report.ResultRev == "repair-rev" {
				return CandidateReviewResult{Verdict: "APPROVE", Rationale: "repair complete"}, nil
			}
			return repairCycleFixableReview(), nil
		}),
		Repair: repairPassFunc(func(_ context.Context, candidate CandidateResult, _ string) (CandidateResult, error) {
			return repairCycleCandidate(candidate.Report.BeadID, candidate.Report.AttemptID, candidate.Report.BaseRev, "repair-rev", 1), nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			return candidate.Report, nil
		}),
		RefStore:    refStore,
		ProjectRoot: "/project",
		BeadEvents:  events,
	}

	result, err := coord.Run(context.Background(), "ddx-repair-bead")
	require.NoError(t, err)
	assert.True(t, result.Landed)
	assert.Equal(t, []string{"candidate-rev", "repair-rev"}, checked)
	assert.Equal(t, []string{"candidate-rev", "repair-rev"}, reviewed)
	assert.Equal(t, []string{"refs/ddx/iterations/attempt-repair-rerun/0", "refs/ddx/iterations/attempt-repair-rerun/1"}, refStore.pinned)
	assert.Equal(t, refStore.pinned, refStore.unpinned)

	foundRepairStart := false
	for _, event := range events.events {
		if event.Kind == repairCycleStartedEventKind {
			foundRepairStart = true
		}
	}
	assert.True(t, foundRepairStart, "repair cycle start event must be recorded")
}

func TestExecutionTrace_RecordsInitialAndRepairCycles(t *testing.T) {
	var repairSawTraceLen int
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return repairCycleCandidate(beadID, "attempt-trace-001", "base-rev", "candidate-rev", 0), nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateReviewResult, error) {
			switch candidate.Report.ResultRev {
			case "candidate-rev":
				return CandidateReviewResult{
					Verdict:          "REQUEST_CHANGES",
					Rationale:        "missing regression coverage",
					Classification:   ReviewFindingClassFixableGap,
					ReviewGroupID:    "rg-initial",
					ReviewerIndices:  []int{0, 1},
					ReviewerVerdicts: []string{"BLOCK", "BLOCK"},
					PerAC: []ReviewAC{
						{Number: 1, Item: "Add regression", Grade: "REQUEST_CHANGES", Evidence: "TestTrace missing"},
					},
					Findings: []Finding{
						{Severity: "warn", Summary: "missing regression coverage", Location: "cli/internal/agent/candidate_cycle_test.go:1"},
					},
				}, nil
			case "repair-rev":
				return CandidateReviewResult{
					Verdict:          "APPROVE",
					Rationale:        "repair complete",
					ReviewGroupID:    "rg-repair",
					ReviewerIndices:  []int{0, 1},
					ReviewerVerdicts: []string{"APPROVE", "APPROVE"},
				}, nil
			default:
				t.Fatalf("unexpected review rev %q", candidate.Report.ResultRev)
				return CandidateReviewResult{}, nil
			}
		}),
		Repair: repairPassFunc(func(_ context.Context, candidate CandidateResult, _ string) (CandidateResult, error) {
			repairSawTraceLen = len(candidate.Report.CycleTrace)
			if repairSawTraceLen != 1 {
				t.Fatalf("repair must receive the prior trace entry, got %d entries", repairSawTraceLen)
			}
			require.Len(t, candidate.Report.CycleTrace, 1)
			assert.Equal(t, "candidate-rev", candidate.Report.CycleTrace[0].ResultRev)
			return repairCycleCandidate(candidate.Report.BeadID, candidate.Report.AttemptID, candidate.Report.BaseRev, "repair-rev", 1), nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			return candidate.Report, nil
		}),
		RefStore:    &inMemoryCandidateRefStore{},
		ProjectRoot: "/project",
	}

	result, err := coord.Run(context.Background(), "ddx-trace-bead")
	require.NoError(t, err)
	require.True(t, result.Landed)
	require.Len(t, result.Report.CycleTrace, 2)

	initial := result.Report.CycleTrace[0]
	assert.Equal(t, 0, initial.CycleIndex)
	assert.Equal(t, "attempt-trace-001", initial.AttemptID)
	assert.Equal(t, "candidate-rev", initial.ResultRev)
	assert.Equal(t, "rg-initial", initial.ReviewGroupID)
	assert.Equal(t, []int{0, 1}, initial.ReviewerIndices)
	assert.Equal(t, []string{"BLOCK", "BLOCK"}, initial.ReviewVerdicts)
	assert.Equal(t, "REQUEST_CHANGES", initial.ReviewResult.Verdict)
	assert.Equal(t, "review_fixable_gap", initial.FinalDecision)

	repair := result.Report.CycleTrace[1]
	assert.Equal(t, 1, repair.CycleIndex)
	assert.Equal(t, "repair-rev", repair.ResultRev)
	assert.Equal(t, "rg-repair", repair.ReviewGroupID)
	assert.Equal(t, []int{0, 1}, repair.ReviewerIndices)
	assert.Equal(t, []string{"APPROVE", "APPROVE"}, repair.ReviewVerdicts)
	assert.Equal(t, "APPROVE", repair.ReviewResult.Verdict)
	assert.Equal(t, ExecuteBeadStatusSuccess, repair.FinalDecision)
	assert.Equal(t, 1, repairSawTraceLen)
}

func TestRepairCycle_MaxCyclesExhaustedPreservesCandidate(t *testing.T) {
	refStore := &inMemoryCandidateRefStore{}
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return repairCycleCandidate(beadID, "attempt-repair-max", "base-rev", "candidate-rev", 0), nil
		}),
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, _ CandidateResult) (CandidateReviewResult, error) {
			return repairCycleFixableReview(), nil
		}),
		Repair: repairPassFunc(func(_ context.Context, candidate CandidateResult, _ string) (CandidateResult, error) {
			return repairCycleCandidate(candidate.Report.BeadID, candidate.Report.AttemptID, candidate.Report.BaseRev, "repair-rev", 1), nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			t.Fatalf("lander must not run after repair budget exhaustion: %+v", candidate)
			return candidate.Report, nil
		}),
		RefStore:        refStore,
		ProjectRoot:     "/project",
		RepairMaxCycles: 1,
	}

	result, err := coord.Run(context.Background(), "ddx-repair-bead")
	require.NoError(t, err)
	assert.False(t, result.Landed)
	assert.Equal(t, ExecuteBeadStatusRepairCycleExhausted, result.Report.Status)
	assert.Equal(t, "repair-rev", result.Report.ResultRev)
	assert.Equal(t, []string{"refs/ddx/iterations/attempt-repair-max/0", "refs/ddx/iterations/attempt-repair-max/1"}, refStore.pinned)
	assert.Empty(t, refStore.unpinned, "exhausted repair candidates must retain refs")
}

type beadEventWithBody struct {
	kind string
	body string
}

func repairCycleCandidate(beadID, attemptID, baseRev, resultRev string, cycleIndex int) CandidateResult {
	return CandidateResult{
		Report: ExecuteBeadReport{
			BeadID:    beadID,
			AttemptID: attemptID,
			Status:    ExecuteBeadStatusSuccess,
			BaseRev:   baseRev,
			ResultRev: resultRev,
		},
		WorktreePath: "/attempt/worktree",
		CycleIndex:   cycleIndex,
	}
}

func repairCycleFixableReview() CandidateReviewResult {
	return CandidateReviewResult{
		Verdict:   "REQUEST_CHANGES",
		Rationale: "missing regression test",
		PerAC: []ReviewAC{
			{Number: 1, Item: "Add regression", Grade: "REQUEST_CHANGES", Evidence: "TestRepairCycle missing"},
		},
		Findings: []Finding{
			{Severity: "warn", Summary: "missing regression test", Location: "cli/internal/agent/candidate_cycle_test.go:1"},
		},
		VerificationCommands: []string{"cd cli && go test ./internal/agent/... -run TestRepairCycle"},
	}
}

func commitTestFile(t *testing.T, repo, name, content, msg string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644))
	out, err := exec.Command("git", "-C", repo, "add", name).CombinedOutput()
	require.NoError(t, err, "git add: %s", out)
	out, err = exec.Command("git", "-C", repo, "commit", "-m", msg).CombinedOutput()
	require.NoError(t, err, "git commit: %s", out)
	raw, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(raw))
}
