package agent

import (
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewClassification_FixableGap(t *testing.T) {
	got := ClassifyReviewFindings(&ReviewResult{
		Verdict:   VerdictRequestChanges,
		Rationale: "missing test coverage",
		PerAC: []ReviewAC{
			{
				Number:   2,
				Item:     "Add regression coverage",
				Grade:    "REQUEST_CHANGES",
				Evidence: "AC#2 regression test is missing in cli/internal/agent/foo_test.go",
			},
		},
		Findings: []Finding{
			{Severity: "warn", Summary: "Implementation is fixable: add the missing test.", Location: "cli/internal/agent/foo_test.go:42"},
		},
	})

	assert.Equal(t, ReviewFindingClassFixableGap, got.Class)
	assert.True(t, got.AutomatedRepairEligible)
	require.NotEmpty(t, got.Evidence)
}

func TestReviewClassification_SpecGap(t *testing.T) {
	got := ClassifyReviewFindings(&ReviewResult{
		Verdict: VerdictBlock,
		Findings: []Finding{
			{Severity: "block", Summary: "Contradictory acceptance criteria make this unverifiable.", Location: "bead:AC3"},
		},
	})

	assert.Contains(t, []string{ReviewTerminalClassSpecGap, ReviewTerminalClassMissingAcceptance}, got.Class)
	assert.False(t, got.AutomatedRepairEligible)
	require.NotEmpty(t, got.Evidence)
}

func TestReviewClassification_UnsafeOutOfScope(t *testing.T) {
	got := ClassifyReviewFindings(&ReviewResult{
		Verdict: VerdictBlock,
		Findings: []Finding{
			{Severity: "block", Summary: "The diff makes forbidden out-of-scope routing changes.", Location: "cli/internal/fizeauadapter/router.go:12"},
		},
	})

	assert.Equal(t, ReviewTerminalClassUnsafeOrOutScope, got.Class)
	assert.False(t, got.AutomatedRepairEligible)
	require.NotEmpty(t, got.Evidence)
}

func TestReviewClassification_FreeformOnlyDoesNotRepair(t *testing.T) {
	got := ClassifyReviewFindings(&ReviewResult{
		Verdict:   VerdictBlock,
		Rationale: "AC#3 is missing a test and can be repaired.",
	})

	assert.Equal(t, ReviewFindingClassMalfunction, got.Class)
	assert.False(t, got.AutomatedRepairEligible)
	assert.Empty(t, got.Evidence)

	store, first, _ := newExecuteLoopTestStore(t)
	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict:         VerdictBlock,
			Rationale:       "AC#3 is missing a test and can be repaired.",
			RawOutput:       "BLOCK: AC#3 is missing a test and can be repaired.",
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
			ResultRev:       "freeform01",
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-freeform",
			ResultRev: "freeform01",
			BaseRev:   "freeform00",
		},
		Reviewer: reviewer,
		Store:    store,
		Rcfg:     rcfg,
		Assignee: "worker",
	})

	assert.False(t, out.Approved)
	assert.Nil(t, out.RepairContext, "freeform-only review rationale must not schedule repair")
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, out.Report.Status)
}
