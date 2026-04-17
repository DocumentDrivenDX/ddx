package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reviewRunnerStub struct {
	result *Result
	err    error
}

func (r *reviewRunnerStub) Run(opts RunOptions) (*Result, error) {
	return r.result, r.err
}

// ---------------------------------------------------------------------------
// ParseReviewVerdict
// ---------------------------------------------------------------------------

func TestParseReviewVerdict_Approve(t *testing.T) {
	output := `
## Review: ddx-1234 iter 1

### Verdict: APPROVE

### AC Grades
| 1 | some item | APPROVE | file.go:10 |
`
	assert.Equal(t, VerdictApprove, ParseReviewVerdict(output))
}

func TestParseReviewVerdict_RequestChanges(t *testing.T) {
	output := `
### Verdict: REQUEST_CHANGES

### AC Grades
| 1 | some item | REQUEST_CHANGES | missing tests |
`
	assert.Equal(t, VerdictRequestChanges, ParseReviewVerdict(output))
}

func TestParseReviewVerdict_Block(t *testing.T) {
	output := `
### Verdict: BLOCK
`
	assert.Equal(t, VerdictBlock, ParseReviewVerdict(output))
}

func TestParseReviewVerdict_UnparsableDefaultsToBlock(t *testing.T) {
	assert.Equal(t, VerdictBlock, ParseReviewVerdict(""))
	assert.Equal(t, VerdictBlock, ParseReviewVerdict("No structured output here"))
	assert.Equal(t, VerdictBlock, ParseReviewVerdict("verdict: APPROVE")) // no leading ##
}

func TestParseReviewVerdict_CaseInsensitive(t *testing.T) {
	assert.Equal(t, VerdictApprove, ParseReviewVerdict("### Verdict: approve"))
	assert.Equal(t, VerdictRequestChanges, ParseReviewVerdict("## Verdict: request_changes"))
}

func TestParseReviewVerdict_ExtraWhitespace(t *testing.T) {
	assert.Equal(t, VerdictApprove, ParseReviewVerdict("### Verdict: APPROVE  "))
}

// ---------------------------------------------------------------------------
// SelectReviewerTier
// ---------------------------------------------------------------------------

func TestSelectReviewerTier_AlwaysSmart(t *testing.T) {
	assert.Equal(t, TierSmart, SelectReviewerTier(TierCheap))
	assert.Equal(t, TierSmart, SelectReviewerTier(TierStandard))
	assert.Equal(t, TierSmart, SelectReviewerTier(TierSmart))
}

// ---------------------------------------------------------------------------
// HasBeadLabel
// ---------------------------------------------------------------------------

func TestHasBeadLabel(t *testing.T) {
	assert.True(t, HasBeadLabel([]string{"review:skip", "helix"}, "review:skip"))
	assert.False(t, HasBeadLabel([]string{"helix"}, "review:skip"))
	assert.False(t, HasBeadLabel(nil, "review:skip"))
}

// ---------------------------------------------------------------------------
// ExecuteBeadWorker with reviewer — loop integration tests
// ---------------------------------------------------------------------------

// makeReviewer builds a BeadReviewerFunc that always returns the given verdict.
func makeReviewer(verdict ReviewVerdict, output string) BeadReviewerFunc {
	return BeadReviewerFunc(func(_ context.Context, _, _, _, _ string) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict:         verdict,
			RawOutput:       output,
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
		}, nil
	})
}

func TestExecuteBeadWorkerReviewApproveClosesBead(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
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
		Reviewer: makeReviewer(VerdictApprove, "### Verdict: APPROVE\n\nAll good."),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker", Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)

	// Bead must remain closed.
	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	// Review event must be appended.
	events, err := store.Events(first.ID)
	require.NoError(t, err)
	found := false
	for _, ev := range events {
		if ev.Kind == "review" && ev.Summary == "APPROVE" {
			found = true
		}
	}
	assert.True(t, found, "expected a review:APPROVE event on the bead")

	// Report must carry the verdict.
	require.Len(t, result.Results, 1)
	assert.Equal(t, "APPROVE", result.Results[0].ReviewVerdict)
	assert.Equal(t, ExecuteBeadStatusSuccess, result.Results[0].Status)
}

func TestExecuteBeadWorkerReviewRequestChangesReopensAndCountsFailure(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
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
		Reviewer: makeReviewer(VerdictRequestChanges, "### Verdict: REQUEST_CHANGES\n\n- Missing tests."),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker", Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusReviewRequestChanges, result.LastFailureStatus)

	// Bead must be re-opened.
	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "bead should be reopened after REQUEST_CHANGES")

	// Review findings must appear in bead notes.
	assert.Contains(t, got.Notes, "REQUEST_CHANGES")

	require.Len(t, result.Results, 1)
	assert.Equal(t, "REQUEST_CHANGES", result.Results[0].ReviewVerdict)
	assert.Equal(t, ExecuteBeadStatusReviewRequestChanges, result.Results[0].Status)
}

func TestExecuteBeadWorkerReviewBlockReopensAndFlagsHuman(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
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
		Reviewer: BeadReviewerFunc(func(_ context.Context, _, _, _, _ string) (*ReviewResult, error) {
			return &ReviewResult{
				Verdict:   VerdictBlock,
				Rationale: "AC#3 regression test missing",
				RawOutput: "### Verdict: BLOCK\n\n### Findings\n- AC#3 regression test missing",
			}, nil
		}),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker", Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusReviewBlock, result.LastFailureStatus)

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Contains(t, got.Notes, "REVIEW:BLOCK")
	assert.Contains(t, got.Notes, "AC#3 regression test missing")

	require.Len(t, result.Results, 1)
	assert.Equal(t, "BLOCK", result.Results[0].ReviewVerdict)
	assert.Equal(t, ExecuteBeadStatusReviewBlock, result.Results[0].Status)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	found := false
	for _, ev := range events {
		if ev.Kind == "execute-bead" && ev.Summary == ExecuteBeadStatusReviewBlock {
			assert.Contains(t, ev.Body, "AC#3 regression test missing")
			found = true
		}
	}
	assert.True(t, found, "expected execute-bead review_block event with rationale")
}

func TestExecuteBeadWorkerReviewBlockWithoutRationaleIsMalfunction(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-review-4",
				ResultRev: "cafed00d",
			}, nil
		}),
		Reviewer: makeReviewer(VerdictBlock, "### Verdict: BLOCK"),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker", Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, result.LastFailureStatus)

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status, "malformed BLOCK should not reopen bead")
	assert.NotContains(t, got.Notes, "REVIEW:BLOCK")

	require.Len(t, result.Results, 1)
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, result.Results[0].Status)
	assert.Empty(t, result.Results[0].ReviewRationale)
}

func TestDefaultBeadReviewerWritesReviewArtifacts(t *testing.T) {
	projectRoot := t.TempDir()
	cmd := exec.Command("git", "init", projectRoot)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
	require.NoError(t, store.Init())
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# review test\n"), 0o644))
	require.NoError(t, store.Create(&bead.Bead{
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
			Output:         "### Verdict: BLOCK\n\n### Findings\n- AC#3 regression test missing",
			DurationMS:     42,
			AgentSessionID: "native-review-1",
		}},
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-review-artifacts", head, "claude", "claude-sonnet")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, VerdictBlock, res.Verdict)
	assert.Equal(t, "AC#3 regression test missing", res.Rationale)
	require.NotEmpty(t, res.ExecutionDir)

	promptPath := filepath.Join(projectRoot, filepath.FromSlash(res.ExecutionDir), "prompt.md")
	manifestPath := filepath.Join(projectRoot, filepath.FromSlash(res.ExecutionDir), "manifest.json")
	resultPath := filepath.Join(projectRoot, filepath.FromSlash(res.ExecutionDir), "result.json")
	for _, path := range []string{promptPath, manifestPath, resultPath} {
		_, err := os.Stat(path)
		require.NoError(t, err, "expected review artifact %s", path)
	}

	artifactResult, err := ReadReviewArtifactResult(resultPath)
	require.NoError(t, err)
	assert.Equal(t, VerdictBlock, artifactResult.Verdict)
	assert.Equal(t, "AC#3 regression test missing", artifactResult.Rationale)
	require.Len(t, artifactResult.PerAC, 1)
	assert.Equal(t, 3, artifactResult.PerAC[0].Number)

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
		Reviewer: BeadReviewerFunc(func(_ context.Context, _, _, _, _ string) (*ReviewResult, error) {
			reviewerCalled = true
			return &ReviewResult{Verdict: VerdictRequestChanges}, nil
		}),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{
		Assignee: "worker",
		Once:     true,
		NoReview: true,
	})
	require.NoError(t, err)
	assert.False(t, reviewerCalled, "reviewer must not be called when NoReview=true")
	assert.Equal(t, 1, result.Successes)

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

func TestExecuteBeadWorkerReviewSkipLabelSkipsReviewer(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	labeled := &bead.Bead{ID: "ddx-skip-1", Title: "Skip review", Labels: []string{"review:skip"}}
	require.NoError(t, store.Create(labeled))

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
		Reviewer: BeadReviewerFunc(func(_ context.Context, _, _, _, _ string) (*ReviewResult, error) {
			reviewerCalled = true
			return &ReviewResult{Verdict: VerdictRequestChanges}, nil
		}),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{
		Assignee: "worker",
		Once:     true,
	})
	require.NoError(t, err)
	assert.False(t, reviewerCalled, "reviewer must not be called when bead has review:skip label")
	assert.Equal(t, 1, result.Successes)
}

func TestExecuteBeadWorkerNilReviewerSkipsReview(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
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
		Reviewer: nil, // no reviewer
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker", Once: true})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Successes)

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

// TestExecuteBeadWorkerReviewBoundedByMaxTier verifies that REQUEST_CHANGES
// reopens the bead for the next attempt but does not cause an infinite loop
// within a single worker.Run call. The bead is visited once, the reviewer
// returns REQUEST_CHANGES, the bead is reopened — but the "attempted" map
// prevents it from being picked up again in the same run.
func TestExecuteBeadWorkerReviewBoundedByMaxTier(t *testing.T) {
	// Use a single-bead store so we can assert "exactly one attempt".
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	only := &bead.Bead{ID: "ddx-bound-1", Title: "Impossible AC"}
	require.NoError(t, store.Create(only))

	executorCalls := 0
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
		Reviewer: makeReviewer(VerdictRequestChanges, "### Verdict: REQUEST_CHANGES\n\nStill failing."),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{
		Assignee: "worker",
		// no Once flag: drain the queue fully within this run
	})
	require.NoError(t, err)

	// The executor should have been called exactly once; the attempted map
	// prevents revisiting even after the bead is reopened by the reviewer.
	assert.Equal(t, 1, executorCalls, "bead should be attempted exactly once within a single worker.Run call")
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures, "reopened bead counts as a failure in this run")

	// Bead is open again — available for the next worker.Run call.
	got, err := store.Get(only.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
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
