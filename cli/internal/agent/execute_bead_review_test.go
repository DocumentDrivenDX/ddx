package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reviewRunnerStub struct {
	result   *Result
	err      error
	lastOpts RunArgs
}

func (r *reviewRunnerStub) Run(opts RunArgs) (*Result, error) {
	r.lastOpts = opts
	return r.result, r.err
}

func TestPreLandReview_UsesAttemptWorktree(t *testing.T) {
	projectRoot, baseRev, store := newReviewArtifactsFixture(t)
	resultRev := commitReviewFixtureFile(t, projectRoot, "candidate.txt", "candidate change\n")
	attemptWorktree := t.TempDir()
	runner := &reviewRunnerStub{result: approveReviewResult()}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      runner,
	}

	res, err := reviewer.Review(context.Background(), projectRoot, CandidateResult{
		Report: ExecuteBeadReport{
			BeadID:    "ddx-review-happy",
			AttemptID: "attempt-preland-worktree",
			Status:    ExecuteBeadStatusSuccess,
			BaseRev:   baseRev,
			ResultRev: resultRev,
		},
		WorktreePath: attemptWorktree,
	})
	require.NoError(t, err)
	assert.Equal(t, "APPROVE", res.Verdict)
	assert.Equal(t, attemptWorktree, runner.lastOpts.WorkDir)
}

func TestPreLandReview_ReadOnlyProfileRequired(t *testing.T) {
	projectRoot, baseRev, store := newReviewArtifactsFixture(t)
	resultRev := commitReviewFixtureFile(t, projectRoot, "candidate.txt", "candidate change\n")
	runner := &reviewRunnerStub{result: approveReviewResult()}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      runner,
	}

	_, err := reviewer.Review(context.Background(), projectRoot, CandidateResult{
		Report: ExecuteBeadReport{
			BeadID:    "ddx-review-happy",
			AttemptID: "attempt-preland-readonly",
			Status:    ExecuteBeadStatusSuccess,
			BaseRev:   baseRev,
			ResultRev: resultRev,
		},
		WorktreePath: t.TempDir(),
	})
	require.NoError(t, err)
	assert.Equal(t, PermissionsReadOnlyReviewer, runner.lastOpts.Permissions)
	assert.Equal(t, "reviewer", runner.lastOpts.Role)
}

func TestPreLandReview_DiffCoversBaseToCandidate(t *testing.T) {
	projectRoot, baseRev, store := newReviewArtifactsFixture(t)
	_ = commitReviewFixtureFile(t, projectRoot, "first.txt", "first candidate line\n")
	resultRev := commitReviewFixtureFile(t, projectRoot, "second.txt", "second candidate line\n")
	runner := &reviewRunnerStub{result: approveReviewResult()}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      runner,
	}

	_, err := reviewer.Review(context.Background(), projectRoot, CandidateResult{
		Report: ExecuteBeadReport{
			BeadID:    "ddx-review-happy",
			AttemptID: "attempt-preland-diff",
			Status:    ExecuteBeadStatusSuccess,
			BaseRev:   baseRev,
			ResultRev: resultRev,
		},
		WorktreePath: t.TempDir(),
	})
	require.NoError(t, err)
	assert.Contains(t, runner.lastOpts.Prompt, "first candidate line", "pre-land review must cover the full base..candidate range")
	assert.Contains(t, runner.lastOpts.Prompt, "second candidate line")
	assert.NotContains(t, runner.lastOpts.Prompt, "commit "+resultRev, "candidate review must not use single-commit git show output")
}

func approveReviewResult() *Result {
	return &Result{
		Harness:    "claude",
		Model:      "claude-opus-4-6",
		Output:     `{"schema_version":1,"verdict":"APPROVE","summary":"ok to land","per_ac":[{"number":1,"item":"AC#1","grade":"pass","evidence":"candidate diff reviewed"}]}`,
		DurationMS: 10,
	}
}

func commitReviewFixtureFile(t *testing.T, projectRoot, name, body string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, name), []byte(body), 0o644))
	out, err := exec.Command("git", "-C", projectRoot, "add", name).CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("git", "-C", projectRoot, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "candidate "+name).CombinedOutput()
	require.NoError(t, err, string(out))
	headRaw, err := exec.Command("git", "-C", projectRoot, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(headRaw))
}

// ---------------------------------------------------------------------------
// HasBeadLabel
// ---------------------------------------------------------------------------

func TestHasBeadLabel(t *testing.T) {
	assert.True(t, HasBeadLabel([]string{"review:skip", "helix"}, "review:skip"))
	assert.False(t, HasBeadLabel([]string{"helix"}, "review:skip"))
	assert.False(t, HasBeadLabel(nil, "review:skip"))
}

func TestHasBeadLabelPrefix(t *testing.T) {
	assert.True(t, HasBeadLabelPrefix([]string{"review:skip-reason:doc-only"}, "review:skip-reason:"))
	assert.False(t, HasBeadLabelPrefix([]string{"review:skip"}, "review:skip-reason:"))
	assert.False(t, HasBeadLabelPrefix(nil, "review:skip-reason:"))
}

// ---------------------------------------------------------------------------
// ExecuteBeadWorker with reviewer — loop integration tests
// ---------------------------------------------------------------------------

// makeReviewer builds a beadReviewerFunc that always returns the given verdict.
func makeReviewer(verdict Verdict, output string) beadReviewerFunc {
	return beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		res := &ReviewResult{
			Verdict:         verdict,
			RawOutput:       output,
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
		}
		if verdict == VerdictApprove {
			res.PerAC = []ReviewAC{
				{
					Number:   1,
					Item:     "AC#1",
					Evidence: "evidence-backed approval",
				},
			}
		}
		return res, nil
	})
}

func makeReviewerWithCost(verdict Verdict, output string, costUSD float64) beadReviewerFunc {
	return beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		res := &ReviewResult{
			Verdict:         verdict,
			RawOutput:       output,
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
			CostUSD:         costUSD,
		}
		if verdict == VerdictApprove {
			res.PerAC = []ReviewAC{
				{
					Number:   1,
					Item:     "AC#1",
					Evidence: "evidence-backed approval",
				},
			}
		}
		return res, nil
	})
}

func TestPostLandReviewPath_UnreachableInCandidateCycle(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	reviewerCalls := 0
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-review-1",
				ResultRev: "aabbccdd",
			}, nil
		}),
		Reviewer: beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
			reviewerCalls++
			return &ReviewResult{Verdict: VerdictRequestChanges, Rationale: "legacy reviewer should not run"}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)
	assert.Equal(t, 0, reviewerCalls, "legacy post-land reviewer must not run after candidate-cycle land")

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	for _, ev := range events {
		assert.NotEqual(t, "review", ev.Kind, "legacy post-land review event must not be emitted")
	}

	require.Len(t, result.Results, 1)
	assert.Empty(t, result.Results[0].ReviewVerdict)
	assert.Equal(t, ExecuteBeadStatusSuccess, result.Results[0].Status)
}

func TestRunPostMergeReviewChargesReviewCostAndDefersWhenCapTrips(t *testing.T) {
	cases := []struct {
		name         string
		maxCost      float64
		preCost      float64
		reviewCost   float64
		wantDeferred bool
		wantApproved bool
	}{
		{
			name:         "normal fit",
			maxCost:      10.0,
			preCost:      4.0,
			reviewCost:   1.5,
			wantDeferred: false,
			wantApproved: true,
		},
		{
			name:         "review tips over budget",
			maxCost:      5.0,
			preCost:      4.0,
			reviewCost:   1.5,
			wantDeferred: true,
			wantApproved: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, first, _ := newExecuteLoopTestStore(t)
			tracker := escalation.NewCostCapTracker(tc.maxCost, func(string) bool { return true })
			tracker.Add("openrouter", tc.preCost)

			reviewer := makeReviewerWithCost(VerdictApprove, "### Verdict: APPROVE\n\nAll good.", tc.reviewCost)
			out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
				Bead: *first,
				Report: ExecuteBeadReport{
					BeadID:    first.ID,
					Status:    ExecuteBeadStatusSuccess,
					SessionID: "sess-review-cost",
					ResultRev: "cafebabe",
					CostUSD:   tc.preCost,
				},
				Reviewer:      reviewer,
				Store:         store,
				ProjectRoot:   t.TempDir(),
				Rcfg:          config.NewTestConfigForLoop(config.TestLoopConfigOpts{Assignee: "worker"}).Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{Assignee: "worker"})),
				Now:           time.Now,
				Assignee:      "worker",
				ReviewCostCap: tracker,
			})
			assert.Equal(t, tc.wantApproved, out.Approved)
			require.NoError(t, out.StoreErr)
			assert.InDelta(t, tc.preCost+tc.reviewCost, tracker.Spent(), 1e-9)

			events, err := store.Events(first.ID)
			require.NoError(t, err)
			foundDeferred := false
			for _, ev := range events {
				if ev.Kind == "review-cost-deferred" {
					foundDeferred = true
					assert.Contains(t, ev.Body, "result_rev=cafebabe")
				}
			}
			assert.Equal(t, tc.wantDeferred, foundDeferred)
		})
	}
}

func TestRunPostMergeReviewChargesReviewCostOnReviewerError(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	tracker := escalation.NewCostCapTracker(5.0, func(string) bool { return true })
	tracker.Add("openrouter", 4.0)

	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict:         VerdictBlock,
			Rationale:       "transport failed",
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
			CostUSD:         1.5,
		}, context.DeadlineExceeded
	})

	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-review-error",
			ResultRev: "feedface",
			CostUSD:   4.0,
		},
		Reviewer:      reviewer,
		Store:         store,
		ProjectRoot:   t.TempDir(),
		Rcfg:          config.NewTestConfigForLoop(config.TestLoopConfigOpts{Assignee: "worker"}).Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{Assignee: "worker"})),
		Now:           time.Now,
		Assignee:      "worker",
		ReviewCostCap: tracker,
	})

	require.False(t, out.Approved, "review error should not approve the bead")
	assert.InDelta(t, 5.5, tracker.Spent(), 1e-9)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	foundDeferred := false
	for _, ev := range events {
		if ev.Kind == "review-cost-deferred" {
			foundDeferred = true
			assert.Contains(t, ev.Body, "result_rev=feedface")
		}
	}
	assert.True(t, foundDeferred, "expected review cost to be charged and deferred on reviewer error")
}

func TestPostLandReviewPath_RequestChangesIgnoredAfterLand(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	reviewerCalls := 0
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-review-2",
				ResultRev: "11223344",
			}, nil
		}),
		Reviewer: beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
			reviewerCalls++
			return &ReviewResult{Verdict: VerdictRequestChanges, Rationale: "missing tests"}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)
	assert.Equal(t, 0, reviewerCalls)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	for _, ev := range events {
		assert.NotEqual(t, "review", ev.Kind)
		assert.NotEqual(t, "reopen", ev.Kind)
	}

	require.Len(t, result.Results, 1)
	assert.Empty(t, result.Results[0].ReviewVerdict)
	assert.Equal(t, ExecuteBeadStatusSuccess, result.Results[0].Status)
}

func TestPostLandReviewPath_BlockIgnoredAfterLand(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	reviewerCalls := 0
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-review-3",
				ResultRev: "deadbeef",
			}, nil
		}),
		Reviewer: beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
			reviewerCalls++
			return &ReviewResult{
				Verdict:   VerdictBlock,
				Rationale: "AC#3 regression test missing",
				RawOutput: "### Verdict: BLOCK\n\n### Findings\n- AC#3 regression test missing",
				Findings: []Finding{
					{Severity: "block", Summary: "AC#3 regression test missing", Location: "cli/internal/agent/foo_test.go:42"},
				},
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)
	assert.Equal(t, 0, reviewerCalls)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
	events, err := store.Events(first.ID)
	require.NoError(t, err)
	for _, ev := range events {
		assert.NotEqual(t, "review", ev.Kind)
		assert.NotEqual(t, "reopen", ev.Kind)
	}
	require.Len(t, result.Results, 1)
	assert.Empty(t, result.Results[0].ReviewVerdict)
	assert.Equal(t, ExecuteBeadStatusSuccess, result.Results[0].Status)
}

func TestRunPostMergeReview_UnanimousApproveCloses(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	reviewer := beadReviewGroupFunc(func(_ context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewGroupResult, error) {
		slot := func(index int) ReviewGroupSlotResult {
			return ReviewGroupSlotResult{
				ReviewerIndex: index,
				Result: &ReviewResult{
					Verdict:         VerdictApprove,
					RawOutput:       "```json\n{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"ok\"}\n```",
					ReviewerHarness: "claude",
					ReviewerModel:   "claude-opus-4-6",
					ResultRev:       resultRev,
					PerAC: []ReviewAC{
						{
							Number:   1,
							Item:     "AC#1",
							Evidence: fmt.Sprintf("bead=%s reviewer=%d", beadID, index),
						},
					},
				},
			}
		}
		return &ReviewGroupResult{
			BeadID:    beadID,
			ResultRev: resultRev,
			Slots: []ReviewGroupSlotResult{
				slot(0),
				slot(1),
			},
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-review-preclose-approve",
			ResultRev: "aa11bb22",
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Now:         time.Now,
		Assignee:    "worker",
	})
	require.True(t, out.Approved)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	approveCount := 0
	for _, ev := range events {
		if ev.Kind == "review" && ev.Summary == "APPROVE" {
			approveCount++
		}
		assert.NotEqual(t, "reopen", ev.Kind, "APPROVE should not reopen the bead")
	}
	assert.Equal(t, 1, approveCount, "approved pre-close review should record one terminal review event before closing")
}

func TestRunPostMergeReview_ApproveWithoutEvidenceIsReviewError(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	reviewer := beadReviewGroupFunc(func(_ context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewGroupResult, error) {
		return &ReviewGroupResult{
			BeadID:    beadID,
			ResultRev: resultRev,
			Slots: []ReviewGroupSlotResult{
				{
					ReviewerIndex: 0,
					Result: &ReviewResult{
						Verdict:         VerdictApprove,
						RawOutput:       "```json\n{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"ok\"}\n```",
						ReviewerHarness: "claude",
						ReviewerModel:   "claude-opus-4-6",
						ResultRev:       resultRev,
					},
				},
				{
					ReviewerIndex: 1,
					Result: &ReviewResult{
						Verdict:         VerdictApprove,
						RawOutput:       "```json\n{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"ok\"}\n```",
						ReviewerHarness: "claude",
						ReviewerModel:   "claude-opus-4-6",
						ResultRev:       resultRev,
						PerAC: []ReviewAC{
							{
								Number:   1,
								Item:     "AC#1",
								Evidence: "evidence-backed approval",
							},
						},
					},
				},
			},
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-review-preclose-unparseable",
			ResultRev: "cc33dd44",
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Now:         time.Now,
		Assignee:    "worker",
	})
	require.False(t, out.Approved)
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, out.Report.Status)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.NotEqual(t, bead.StatusClosed, got.Status)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	foundError := false
	for _, ev := range events {
		if ev.Kind == "review-error" {
			foundError = true
			assert.Contains(t, ev.Summary, "unparseable")
			assert.Contains(t, ev.Body, "failure_class="+evidence.OutcomeReviewUnparseable)
		}
		assert.NotEqual(t, "reopen", ev.Kind, "pre-close review-error must not invoke Reopen")
	}
	assert.True(t, foundError, "expected evidence-free APPROVE to record review-error: unparseable")
}

func TestRunPostMergeReview_BlockWithoutRationaleIsMalfunction(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-review-4",
			ResultRev: "cafed00d",
		},
		Reviewer:    makeReviewer(VerdictBlock, "### Verdict: BLOCK"),
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Now:         time.Now,
		Assignee:    "worker",
	})
	require.False(t, out.Approved)
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, out.Report.Status)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	// ddx-e30e60a9 + ddx-738edf47: malformed BLOCK is a reviewer malfunction,
	// not a terminal verdict. The loop refuses to close on a malfunction so
	// a later attempt can retry. Pre-wave-2 behavior closed eagerly before
	// review and left the malformed-BLOCK bead closed — that was the silent
	// false-closure surface these beads eliminate.
	assert.NotEqual(t, bead.StatusClosed, got.Status,
		"malformed BLOCK must not close the bead — closing on reviewer malfunction was the silent-false-closure shape")
	assert.NotContains(t, got.Notes, "REVIEW:BLOCK")
}

func TestDefaultBeadReviewerWritesReviewArtifacts(t *testing.T) {
	projectRoot := t.TempDir()
	cmd := exec.Command("git", "init", projectRoot)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# review test\n"), 0o644))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:          "ddx-review-artifacts",
		Title:       "Review bundle test",
		Description: "Ensure review evidence is persisted.",
		Acceptance:  "1. AC one\n2. AC two\n3. AC three",
	}))
	out, err = exec.Command("git", "-C", projectRoot, "add", "README.md", ".ddx/beads.jsonl").CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("git", "-C", projectRoot, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "init").CombinedOutput()
	require.NoError(t, err, string(out))
	headRaw, err := exec.Command("git", "-C", projectRoot, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headRaw))

	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner: &reviewRunnerStub{result: &Result{
			Harness:        "claude",
			Model:          "claude-opus-4-6",
			Output:         "```json\n{\"schema_version\":1,\"verdict\":\"BLOCK\",\"summary\":\"AC#3 regression test missing\",\"findings\":[{\"severity\":\"block\",\"summary\":\"regression test missing\",\"location\":\"file.go:42\"}]}\n```",
			DurationMS:     42,
			CostUSD:        0.0314,
			AgentSessionID: "native-review-1",
		}},
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-review-artifacts", head, ImplementerRouting{Harness: "claude", Model: "claude-sonnet"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, VerdictBlock, res.Verdict)
	assert.Equal(t, "AC#3 regression test missing", res.Rationale)
	assert.InDelta(t, 0.0314, res.CostUSD, 1e-9)
	require.NotEmpty(t, res.ExecutionDir)

	promptPath := filepath.Join(projectRoot, filepath.FromSlash(res.ExecutionDir), "prompt.md")
	manifestPath := filepath.Join(projectRoot, filepath.FromSlash(res.ExecutionDir), "manifest.json")
	resultPath := filepath.Join(projectRoot, filepath.FromSlash(res.ExecutionDir), "result.json")
	for _, path := range []string{promptPath, manifestPath, resultPath} {
		_, err := os.Stat(path)
		require.NoError(t, err, "expected review artifact %s", path)
	}

	rawResult, err := os.ReadFile(resultPath)
	require.NoError(t, err)
	var artifactResult reviewArtifactResult
	require.NoError(t, json.Unmarshal(rawResult, &artifactResult))
	assert.Equal(t, string(VerdictBlock), artifactResult.Verdict)
	assert.Equal(t, "AC#3 regression test missing", artifactResult.Rationale)

	var manifest reviewArtifactManifest
	rawManifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(rawManifest, &manifest))
	assert.Equal(t, "native-review-1", manifest.SessionID)
	assert.Equal(t, strings.TrimSpace(head), manifest.ResultRev)
}

func TestExecuteBeadWorkerNoReviewSkipsReviewer(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	reviewerCalled := false
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-no-review",
				ResultRev: "cafebabe",
			}, nil
		}),
		Reviewer: beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
			reviewerCalled = true
			return &ReviewResult{Verdict: VerdictRequestChanges}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:     true,
		NoReview: true,
	})
	require.NoError(t, err)
	assert.False(t, reviewerCalled, "reviewer must not be called when NoReview=true")
	assert.Equal(t, 1, result.Successes)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

func TestExecuteBeadWorkerReviewSkipLabelWithReasonSkipsReviewer(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	labeled := &bead.Bead{ID: "ddx-skip-1", Title: "Skip review", Labels: []string{"review:skip", "review:skip-reason:doc-only"}}
	require.NoError(t, store.Create(context.Background(), labeled))

	reviewerCalled := false
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-label-skip",
				ResultRev: "feedface",
			}, nil
		}),
		Reviewer: beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
			reviewerCalled = true
			return &ReviewResult{Verdict: VerdictRequestChanges}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
	})
	require.NoError(t, err)
	assert.False(t, reviewerCalled, "reviewer must not be called when bead has review:skip label")
	assert.Equal(t, 1, result.Successes)
}

func TestExecuteBeadWorkerReviewSkipLabelWithoutReasonIsIgnored(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	labeled := &bead.Bead{ID: "ddx-skip-2", Title: "Skip review", Labels: []string{"review:skip"}}
	require.NoError(t, store.Create(context.Background(), labeled))

	reviewerCalled := false
	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		reviewerCalled = true
		return &ReviewResult{Verdict: VerdictRequestChanges}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *labeled,
		Report: ExecuteBeadReport{
			BeadID:    labeled.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-label-ignore",
			ResultRev: "feedface",
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Now:         time.Now,
		Assignee:    "worker",
	})
	require.False(t, out.Approved)
	assert.True(t, reviewerCalled, "reviewer must be called when review:skip lacks a review:skip-reason label")
	assert.Equal(t, ExecuteBeadStatusReviewRequestChanges, out.Report.Status)

	got, err := store.Get(context.Background(), labeled.ID)
	require.NoError(t, err)
	assert.NotEqual(t, bead.StatusClosed, got.Status, "label-only skip should be ignored")
}

func TestExecuteBeadWorker_NiflheimEvidence_EmptyReviewResultCannotClose(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-empty-review",
				ResultRev: "badc0ffe",
				CycleTrace: []ExecutionCycleTrace{
					{
						CycleIndex:    0,
						AttemptID:     "attempt-empty-review",
						ResultRev:     "badc0ffe",
						ReviewGroupID: "rg-empty-review",
						ReviewerIndices: []int{
							0,
						},
						ReviewVerdicts: []string{
							"APPROVE",
						},
						ReviewResult:  ExecutionCycleReviewResult{},
						FinalDecision: ExecuteBeadStatusSuccess,
					},
				},
			}, nil
		}),
		Reviewer: beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
			return &ReviewResult{Verdict: VerdictApprove}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, result.LastFailureStatus)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.NotEqual(t, bead.StatusClosed, got.Status)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var reviewError *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "review-error" {
			reviewError = &events[i]
			break
		}
	}
	require.NotNil(t, reviewError, "empty review result must emit review-error evidence")
	assert.NotEmpty(t, reviewError.Body)
	assert.Contains(t, reviewError.Body, "empty review result before close")
}

func TestExecuteBeadWorker_NiflheimEvidence_NilReviewerRequiresExplicitSkip(t *testing.T) {
	t.Run("missing durable skip fails review gate", func(t *testing.T) {
		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init(context.Background()))
		target := &bead.Bead{ID: "ddx-nil-reviewer", Title: "Nil reviewer"}
		require.NoError(t, store.Create(context.Background(), target))

		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusSuccess,
					SessionID: "sess-nil-reviewer",
					ResultRev: "badc0ffe",
				}, nil
			}),
			Reviewer: nil,
		}

		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true, PostMergeReview: true})
		require.NoError(t, err)
		assert.Equal(t, 0, result.Successes)
		assert.Equal(t, 1, result.Failures)
		assert.Equal(t, ExecuteBeadStatusReviewMalfunction, result.LastFailureStatus)

		got, err := store.Get(context.Background(), target.ID)
		require.NoError(t, err)
		assert.NotEqual(t, bead.StatusClosed, got.Status)

		events, err := store.Events(target.ID)
		require.NoError(t, err)
		var reviewError *bead.BeadEvent
		for i := range events {
			if events[i].Kind == "review-error" {
				reviewError = &events[i]
				break
			}
		}
		require.NotNil(t, reviewError, "nil reviewer without durable skip must emit review-error evidence")
		assert.Contains(t, reviewError.Body, "reviewer is nil")
	})

	t.Run("durable skip closes", func(t *testing.T) {
		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init(context.Background()))
		target := &bead.Bead{
			ID:     "ddx-nil-reviewer-skip",
			Title:  "Nil reviewer durable skip",
			Labels: []string{"review:skip", "review:skip-reason:test-fixture"},
		}
		require.NoError(t, store.Create(context.Background(), target))

		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusSuccess,
					SessionID: "sess-nil-reviewer-skip",
					ResultRev: "feedface",
				}, nil
			}),
			Reviewer: nil,
		}

		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true, PostMergeReview: true})
		require.NoError(t, err)
		assert.Equal(t, 1, result.Successes)

		got, err := store.Get(context.Background(), target.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusClosed, got.Status)
	})
}

// TestExecuteBeadWorkerReviewBypassesRetiredPostLandPath verifies the retired post-land
// reviewer is unreachable from work success. Candidate-cycle review
// owns repair/retry now, so a success closes directly within one worker.Run.
func TestExecuteBeadWorkerReviewBypassesRetiredPostLandPath(t *testing.T) {
	// Use a single-bead store so we can assert "exactly one attempt".
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	only := &bead.Bead{ID: "ddx-bound-1", Title: "Impossible AC"}
	require.NoError(t, store.Create(context.Background(), only))

	executorCalls := 0
	reviewerCalls := 0
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			executorCalls++
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-bound",
				ResultRev: "11111111",
			}, nil
		}),
		Reviewer: beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
			reviewerCalls++
			return &ReviewResult{Verdict: VerdictRequestChanges}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		// no Once flag: drain the queue fully within this run
	})
	require.NoError(t, err)

	// The executor should have been called exactly once; the retired
	// post-land review path should not run or trigger an in-loop retry.
	assert.Equal(t, 1, executorCalls, "bead should be attempted exactly once within a single worker.Run call")
	assert.Equal(t, 0, reviewerCalls, "legacy post-land reviewer must not run from work success")
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)

	got, err := store.Get(context.Background(), only.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

// ---------------------------------------------------------------------------
// BuildReviewPrompt
// ---------------------------------------------------------------------------

func TestBuildReviewPrompt_ContainsRequiredSections(t *testing.T) {
	b := &bead.Bead{
		ID:          "ddx-0001",
		Title:       "Test bead",
		Description: "Do the thing.",
		Acceptance:  "- [ ] thing is done",
	}
	diff := "diff --git a/file.go b/file.go\n+func Foo() {}\n"
	prompt := BuildReviewPrompt(b, 1, "abc1234", diff, t.TempDir(), nil)

	assert.Contains(t, prompt, "<bead-review>")
	assert.Contains(t, prompt, `id="ddx-0001"`)
	assert.Contains(t, prompt, "<title>Test bead</title>")
	assert.Contains(t, prompt, "thing is done")
	assert.Contains(t, prompt, `rev="abc1234"`)
	assert.Contains(t, prompt, "Foo()")
	assert.Contains(t, prompt, "<instructions>")
	assert.Contains(t, prompt, "APPROVE")
	assert.Contains(t, prompt, "</bead-review>")
}

// TestGitShowExcludesEvidenceNoiseFromReviewDiff is the regression for
// ddx-39e27896. A prior attempt that tracked a multi-thousand-line
// session log (.ddx/executions/<attempt>/embedded/agent-*.jsonl) in git
// history would cause DefaultBeadReviewer.gitShow to emit a <diff>
// section sized O(session log), pushing retry prompts past 2M tokens
// and crashing every provider with n_keep > n_ctx.
//
// This test creates a synthetic repo matching the failure scenario —
// a commit that adds a 10k-line embedded session log — then runs
// gitShow and asserts the output excludes the embedded file content
// and stays bounded.
func TestGitShowExcludesEvidenceNoiseFromReviewDiff(t *testing.T) {
	root := t.TempDir()
	runGitInteg(t, root, "init", "-b", "main")
	runGitInteg(t, root, "config", "user.email", "test@ddx.test")
	runGitInteg(t, root, "config", "user.name", "DDx Test")

	// Seed commit so we have a base rev.
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	runGitInteg(t, root, "add", "README.md")
	runGitInteg(t, root, "commit", "-m", "seed")

	// Synthetic evidence commit that adds a multi-thousand-line session log
	// PLUS a legitimate implementation change. The fix must exclude the
	// session log from the diff while keeping the implementation change.
	evidenceDir := filepath.Join(root, ddxroot.DirName, "executions", "20260417T000000-testattempt", "embedded")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatalf("mkdir evidence: %v", err)
	}
	var bigLog strings.Builder
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(&bigLog, "{\"seq\":%d,\"event\":\"tool_call\",\"payload\":\"lorem ipsum dolor sit amet consectetur adipiscing elit\"}\n", i)
	}
	sessionLogPath := filepath.Join(evidenceDir, "agent-123.jsonl")
	if err := os.WriteFile(sessionLogPath, []byte(bigLog.String()), 0o644); err != nil {
		t.Fatalf("write session log: %v", err)
	}

	// Legitimate implementation change that MUST survive in the diff.
	if err := os.WriteFile(filepath.Join(root, "implementation.go"), []byte("package main\n\nfunc Added() {}\n"), 0o644); err != nil {
		t.Fatalf("write implementation: %v", err)
	}

	// Force-add the evidence (the pre-fix landEvidence behavior) plus the real change.
	runGitInteg(t, root, "add", "-f", ".ddx/executions/")
	runGitInteg(t, root, "add", "implementation.go")
	runGitInteg(t, root, "commit", "-m", "chore: add execution evidence [testattempt] + impl")

	// Get the HEAD sha (the evidence commit).
	headSha := strings.TrimSpace(runGitInteg(t, root, "rev-parse", "HEAD"))

	// Call the gitShow method with the fix in place.
	reviewer := &DefaultBeadReviewer{ProjectRoot: root}
	out, err := reviewer.gitShow(headSha)
	if err != nil {
		t.Fatalf("gitShow: %v", err)
	}

	// Must NOT include the session log content.
	if strings.Contains(out, "lorem ipsum dolor sit amet") {
		t.Errorf("gitShow output includes embedded session log content (pathspec exclusion not applied)")
	}

	// Must include the legitimate implementation change.
	if !strings.Contains(out, "implementation.go") {
		t.Errorf("gitShow output missing the legitimate implementation file (pathspec too aggressive)")
	}
	if !strings.Contains(out, "func Added()") {
		t.Errorf("gitShow output missing the implementation change body")
	}

	// Must be bounded in size. The raw session log was ~1MB; fixed diff
	// should be under 50KB easily (seed + impl.go + evidence metadata).
	if len(out) > 150_000 {
		t.Errorf("gitShow output size %d exceeds 150KB budget — pathspec exclusion did not bound the diff", len(out))
	}
}

func TestRunPostMergeReviewIgnoresProseFindingsForClosure(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict: VerdictApprove,
			PerAC: []ReviewAC{
				{
					Number:   1,
					Item:     "AC#1",
					Evidence: "correctness evidence",
				},
			},
			ProseFindings: []Finding{
				{
					Severity: "warning",
					Summary:  "tighten the prose",
					Location: "bead.md:3",
				},
			},
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
		}, nil
	})

	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-review-prose",
			ResultRev: "cafef00d",
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        config.NewTestConfigForLoop(config.TestLoopConfigOpts{Assignee: "worker"}).Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{Assignee: "worker"})),
		Now:         time.Now,
		Assignee:    "worker",
	})

	require.True(t, out.Approved, "advisory prose findings must not block closure")
	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}
