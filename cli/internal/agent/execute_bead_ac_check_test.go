package agent

import (
	"context"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Reviewer ratification of ac-check results
// ---------------------------------------------------------------------------

func TestReviewer_RatifiesACCheck_AllPass_ApproveImplicit(t *testing.T) {
	b := &bead.Bead{
		ID:         "ddx-accheck-allpass",
		Title:      "All-pass AC-check bead",
		Acceptance: "1. TestFoo passes\n2. lefthook run pre-commit passes",
	}
	acCheckJSON := `{"schema_version":1,"bead_id":"ddx-accheck-allpass","summary":{"pass":2,"fail":0,"needs_judgment":0,"error":0},"items":[{"ac":1,"kind":"test-name","name":"TestFoo","result":"pass","evidence":"TestFoo found in diff"},{"ac":2,"kind":"build-gate","result":"pass","evidence":"pre-commit gate green"}]}`
	built := BuildReviewPromptBounded(b, 1, "rev-abc", "diff content", t.TempDir(), nil, BuildReviewPromptOptions{ACCheckJSON: acCheckJSON})
	assert.Contains(t, built.Prompt, "<ac-check>")
	assert.Contains(t, built.Prompt, `"result":"pass"`)
	assert.Contains(t, built.Prompt, "TestFoo")
	// Reviewer instructions must mention ratification of pass results.
	assert.Contains(t, built.Prompt, "ratif")
}

func TestReviewer_RatifiesACCheck_Fail_BlocksByRatification(t *testing.T) {
	b := &bead.Bead{
		ID:         "ddx-accheck-fail",
		Title:      "Failing AC-check bead",
		Acceptance: "1. TestBar passes",
	}
	acCheckJSON := `{"schema_version":1,"bead_id":"ddx-accheck-fail","summary":{"pass":0,"fail":1,"needs_judgment":0,"error":0},"items":[{"ac":1,"kind":"test-name","name":"TestBar","result":"fail","evidence":"TestBar not found in diff"}]}`
	built := BuildReviewPromptBounded(b, 1, "rev-fail", "diff content", t.TempDir(), nil, BuildReviewPromptOptions{ACCheckJSON: acCheckJSON})
	assert.Contains(t, built.Prompt, "<ac-check>")
	assert.Contains(t, built.Prompt, `"result":"fail"`)
	// Reviewer instructions must state that fail results require BLOCK.
	assert.Contains(t, built.Prompt, "BLOCK")
	// The instructions must mention AC-Waive as the only bypass.
	assert.Contains(t, built.Prompt, "AC-Waive")
}

func TestReviewer_NeedsJudgment_AdjudicatedOrClarification(t *testing.T) {
	b := &bead.Bead{
		ID:         "ddx-accheck-nj",
		Title:      "Needs-judgment AC-check bead",
		Acceptance: "1. The PR description is clear",
	}
	acCheckJSON := `{"schema_version":1,"bead_id":"ddx-accheck-nj","summary":{"pass":0,"fail":0,"needs_judgment":1,"error":0},"items":[{"ac":1,"kind":"prose","result":"needs_judgment","evidence":"prose AC requires reviewer adjudication"}]}`
	built := BuildReviewPromptBounded(b, 1, "rev-nj", "diff content", t.TempDir(), nil, BuildReviewPromptOptions{ACCheckJSON: acCheckJSON})
	assert.Contains(t, built.Prompt, "<ac-check>")
	assert.Contains(t, built.Prompt, `"result":"needs_judgment"`)
	// The instructions must mention REQUEST_CLARIFICATION for undecidable needs_judgment items.
	assert.Contains(t, built.Prompt, "REQUEST_CLARIFICATION")
}

// ---------------------------------------------------------------------------
// REQUEST_CLARIFICATION routing — does not block the queue
// ---------------------------------------------------------------------------

func TestReviewer_RequestClarification_NotBlockingQueue(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict:         VerdictRequestClarification,
			Rationale:       "AC#1 is a prose item requiring operator input",
			RawOutput:       `{"schema_version":1,"verdict":"REQUEST_CLARIFICATION","summary":"prose AC requires operator input"}`,
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
			Findings: []Finding{
				{Severity: "info", Summary: "prose AC cannot be adjudicated from diff alone"},
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
			SessionID: "sess-clarif",
			ResultRev: "aabbcc02",
			BaseRev:   "aabbcc00",
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Now:         time.Now,
		Assignee:    "worker",
	})

	assert.False(t, out.Approved, "REQUEST_CLARIFICATION must not approve the attempt")
	assert.Equal(t, ExecuteBeadStatusReviewRequestClarification, out.Report.Status)

	// Bead must be parked to proposed (operator lane), not closed.
	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status, "REQUEST_CLARIFICATION must park to proposed")
	assert.NotEqual(t, bead.StatusClosed, got.Status, "REQUEST_CLARIFICATION must not close the bead")

	// A review-request-clarification event must be recorded.
	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var found bool
	for _, ev := range events {
		if ev.Kind == ReviewRequestClarificationEventKind {
			found = true
			break
		}
	}
	assert.True(t, found, "expected review-request-clarification event")
}

// ---------------------------------------------------------------------------
// Reviewer override: pass → BLOCK requires judgment_override_reason
// ---------------------------------------------------------------------------

func TestReviewer_OverridePassToBlock_RequiresJudgmentReason(t *testing.T) {
	// The reviewer instructions must state that overriding a mechanical grade
	// requires an explicit judgment_override_reason and a diff citation.
	assert.Contains(t, beadReviewInstructions, "judgment_override_reason",
		"override-to-block instruction must name the judgment_override_reason field")
	assert.Contains(t, beadReviewInstructions, "diff citation",
		"override-to-block instruction must require a diff citation")
}

// ---------------------------------------------------------------------------
// Strictness mode routing from bead labels
// ---------------------------------------------------------------------------

func TestReviewer_StrictnessMode_KindFix_RequiresTestNames(t *testing.T) {
	assert.Equal(t, StrictnessStrict, beadStrictnessMode([]string{"kind:fix"}))
	assert.Equal(t, StrictnessStrict, beadStrictnessMode([]string{"kind:feat"}))
	assert.Equal(t, StrictnessStrict, beadStrictnessMode([]string{"phase:6", "kind:fix", "area:agent"}))

	b := &bead.Bead{ID: "ddx-fix", Title: "fix bead", Labels: []string{"kind:fix"}}
	built := BuildReviewPromptBounded(b, 1, "rev", "diff", t.TempDir(), nil, BuildReviewPromptOptions{})
	assert.Contains(t, built.Prompt, `mode="strict"`)
	assert.Contains(t, built.Prompt, "Test*")
}

func TestReviewer_StrictnessMode_KindRefactor_BuildGreenSuffices(t *testing.T) {
	assert.Equal(t, StrictnessBehavior, beadStrictnessMode([]string{"kind:refactor"}))
	assert.Equal(t, StrictnessBehavior, beadStrictnessMode([]string{"kind:chore"}))

	b := &bead.Bead{ID: "ddx-refactor", Title: "refactor bead", Labels: []string{"kind:refactor"}}
	built := BuildReviewPromptBounded(b, 1, "rev", "diff", t.TempDir(), nil, BuildReviewPromptOptions{})
	assert.Contains(t, built.Prompt, `mode="behavior-light"`)
	assert.Contains(t, built.Prompt, "build green")
}

func TestReviewer_StrictnessMode_KindDoc_FilePresenceSuffices(t *testing.T) {
	assert.Equal(t, StrictnessMechanical, beadStrictnessMode([]string{"kind:doc"}))
	assert.Equal(t, StrictnessMechanical, beadStrictnessMode([]string{"kind:mechanical"}))

	b := &bead.Bead{ID: "ddx-doc", Title: "doc bead", Labels: []string{"kind:doc"}}
	built := BuildReviewPromptBounded(b, 1, "rev", "diff", t.TempDir(), nil, BuildReviewPromptOptions{})
	assert.Contains(t, built.Prompt, `mode="mechanical"`)
	assert.Contains(t, built.Prompt, "file presence")
}

// ---------------------------------------------------------------------------
// Regression: ac-check failure blocks even when reviewer would approve
// ---------------------------------------------------------------------------

func TestReviewer_RegressionFakeRetainDays_BlocksByACCheck(t *testing.T) {
	// Regression: before ac-check ratification was wired in, a reviewer could
	// return APPROVE even when the ac-check had mechanically verified failures.
	// This test verifies that ac-check failure JSON is visible in the reviewer
	// prompt so the ratification instructions can enforce the block.
	b := &bead.Bead{
		ID:         "ddx-accheck-reg",
		Title:      "Regression bead: retain_days check",
		Acceptance: "1. retain_days field is removed from schema",
	}
	// Simulate an ac-check that mechanically verified the symbol is still present.
	acCheckJSON := `{"schema_version":1,"bead_id":"ddx-accheck-reg","summary":{"pass":0,"fail":1,"needs_judgment":0,"error":0},"items":[{"ac":1,"kind":"symbol","name":"retain_days","result":"fail","evidence":"retain_days symbol still present in diff"}]}`
	built := BuildReviewPromptBounded(b, 1, "rev-reg", "diff", t.TempDir(), nil, BuildReviewPromptOptions{ACCheckJSON: acCheckJSON})
	// The prompt must contain the ac-check section with the failure.
	assert.Contains(t, built.Prompt, "<ac-check>")
	assert.Contains(t, built.Prompt, "retain_days")
	assert.Contains(t, built.Prompt, `"result":"fail"`)
	// The instructions must prevent approval without waive.
	assert.Contains(t, built.Prompt, "AC-Waive")
}
