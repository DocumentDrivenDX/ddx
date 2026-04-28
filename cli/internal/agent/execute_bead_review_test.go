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

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reviewRunnerStub struct {
	result *Result
	err    error
}

func (r *reviewRunnerStub) Run(opts RunArgs) (*Result, error) {
	return r.result, r.err
}

// ---------------------------------------------------------------------------
// SelectReviewerTier
// ---------------------------------------------------------------------------

func TestSelectReviewerTier_AlwaysSmart(t *testing.T) {
	assert.Equal(t, escalation.TierSmart, SelectReviewerTier(escalation.TierCheap))
	assert.Equal(t, escalation.TierSmart, SelectReviewerTier(escalation.TierStandard))
	assert.Equal(t, escalation.TierSmart, SelectReviewerTier(escalation.TierSmart))
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
func makeReviewer(verdict Verdict, output string) BeadReviewerFunc {
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

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
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

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
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

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
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

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, result.LastFailureStatus)

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	// ddx-e30e60a9 + ddx-738edf47: malformed BLOCK is a reviewer malfunction,
	// not a terminal verdict. The loop refuses to close on a malfunction so
	// a later attempt can retry. Pre-wave-2 behavior closed eagerly before
	// review and left the malformed-BLOCK bead closed — that was the silent
	// false-closure surface these beads eliminate.
	assert.NotEqual(t, bead.StatusClosed, got.Status,
		"malformed BLOCK must not close the bead — closing on reviewer malfunction was the silent-false-closure shape")
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
			Output:         "```json\n{\"schema_version\":1,\"verdict\":\"BLOCK\",\"summary\":\"AC#3 regression test missing\",\"findings\":[{\"severity\":\"block\",\"summary\":\"regression test missing\",\"location\":\"file.go:42\"}]}\n```",
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

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
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

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
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

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
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

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
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
	evidenceDir := filepath.Join(root, ".ddx", "executions", "20260417T000000-testattempt", "embedded")
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
