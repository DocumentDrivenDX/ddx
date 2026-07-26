package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bypassGateCommitRunner simulates a provider that stages code, observes a
// failed pre-commit gate, and commits with --no-verify. ToolCalls carries the
// harness-session evidence the integrity adapter must thread into validation.
type bypassGateCommitRunner struct {
	t *testing.T
}

func (r *bypassGateCommitRunner) Run(opts RunArgs) (*Result, error) {
	r.t.Helper()
	require.NotEmpty(r.t, opts.WorkDir)

	// Stage a code-changing file (final staged mutation).
	src := filepath.Join(opts.WorkDir, "bypass_gate.go")
	require.NoError(r.t, os.WriteFile(src, []byte("package main\n// intentional gate-bypass change\n"), 0o644))
	runGitInteg(r.t, opts.WorkDir, "add", "bypass_gate.go")

	// Implementation commit bypasses hooks after the failed gate.
	// --no-verify is the integrity violation under test; the real git
	// commit still lands so ImplementationRev differs from BaseRev.
	runGitInteg(r.t, opts.WorkDir, "commit", "--no-verify", "-m", "fix: bypass staged gate [ddx-int-0001]")

	failedGateOutput := strings.Join([]string{
		"go-test",
		"FAIL: hook failed",
		"summary: (fail) hook failed",
	}, "\n")

	return &Result{
		ExitCode: 0,
		ToolCalls: []ToolCallEntry{
			{
				Tool:   "Bash",
				Input:  `{"command":"git add bypass_gate.go"}`,
				Output: "staged bypass_gate.go",
			},
			{
				Tool:   "Bash",
				Input:  `{"command":"lefthook run pre-commit"}`,
				Output: failedGateOutput,
				Error:  "exit code: 1",
			},
			{
				Tool:   "Bash",
				Input:  `{"command":"git commit --no-verify -m fix: bypass staged gate [ddx-int-0001]"}`,
				Output: "[main abcdef0] fix: bypass staged gate [ddx-int-0001]",
			},
		},
	}, nil
}

// TestExecuteBead_WiresHarnessGateEvidenceIntoIntegrityValidation proves the
// live execute-bead path threads harness-session gate and implementation-git
// evidence into ValidateAttemptIntegrity, and that a staged-code + failed
// pre-commit + commit --no-verify attempt is preserved with
// failure_mode=attempt_integrity rather than merged.
func TestExecuteBead_WiresHarnessGateEvidenceIntoIntegrityValidation(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	// Acceptance requires lefthook so GateEvidenceRequired is true on the
	// integrity input (parent contract: code-changing beads that require the
	// staged gate).
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Update(context.Background(), beadID, func(b *bead.Bead) {
		b.Acceptance = "1. TestExecuteBead_WiresHarnessGateEvidenceIntoIntegrityValidation\n2. lefthook run pre-commit passes"
	}))
	runGitInteg(t, projectRoot, "add", ".ddx/beads.jsonl")
	runGitInteg(t, projectRoot, "commit", "-m", "chore: require lefthook on integrity bead")
	mainBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	var captured AttemptIntegrityInput
	prevHook := attemptIntegrityInputHook
	attemptIntegrityInputHook = func(in AttemptIntegrityInput) {
		captured = in
	}
	t.Cleanup(func() { attemptIntegrityInputHook = prevHook })

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Harness: "virtual",
	}).Resolve(config.TestBeadOverrides(config.TestBeadConfigOpts{Harness: "virtual"}))

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: &bypassGateCommitRunner{t: t},
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)

	// AC1: attempt is rejected with attempt_integrity, not treated as success.
	require.Equal(t, ExecuteBeadOutcomeTaskFailed, res.Outcome)
	require.Equal(t, FailureModeAttemptIntegrity, res.FailureMode)
	require.Equal(t, AttemptIntegrityPreserveReason, res.Reason)
	require.NotEqual(t, res.BaseRev, res.ResultRev, "implementation commit must exist to preserve")
	require.Contains(t, strings.ToLower(res.Error), "no-verify")

	// AC2: ValidateAttemptIntegrity received full harness-session evidence.
	require.True(t, captured.GateEvidenceRequired, "lefthook acceptance must require gate evidence")
	require.True(t, captured.CodeChanging)
	require.Equal(t, res.ImplementationRev, captured.ImplementationRev)
	require.Len(t, captured.GateRuns, 3, "expected stage + failed gate + no-verify commit")

	// Ordering: final staged mutation → gate → implementation commit.
	assert.Contains(t, captured.GateRuns[0].Command, "git add")
	assert.Contains(t, captured.GateRuns[0].Output, "staged")
	assert.Equal(t, 0, captured.GateRuns[0].ExitCode)

	assert.Contains(t, captured.GateRuns[1].Command, "lefthook run pre-commit")
	assert.Contains(t, captured.GateRuns[1].Output, "FAIL")
	assert.Equal(t, 1, captured.GateRuns[1].ExitCode, "failed gate must carry non-zero exit status")

	assert.Contains(t, captured.GateRuns[2].Command, "git commit")
	assert.Contains(t, captured.GateRuns[2].Command, "--no-verify")
	assert.NotEmpty(t, captured.GateRuns[2].Output)

	// Land path: preserve for review, do not merge into main.
	landing, landErr := LandBeadResult(projectRoot, res, &RealGitOps{}, BeadLandingOptions{})
	require.NoError(t, landErr)
	ApplyLandingToResult(res, landing)
	require.Equal(t, "preserved", res.Outcome)
	require.Equal(t, FailureModeAttemptIntegrity, res.FailureMode)
	require.Equal(t, ExecuteBeadStatusPreservedNeedsReview, res.Status)
	require.NotEmpty(t, res.PreserveRef)
	require.Equal(t, mainBefore, runGitInteg(t, projectRoot, "rev-parse", "HEAD"),
		"main must not advance when integrity rejects the attempt")
}

func TestHarnessGateEvidenceFromToolCalls_FiltersBackgroundAndIrrelevant(t *testing.T) {
	runs := harnessGateEvidenceFromToolCalls([]ToolCallEntry{
		{Tool: "Read", Input: `{"path":"README.md"}`, Output: "ok"},
		{Tool: "Bash", Input: `{"command":"git status"}`, Output: "clean"},
		{Tool: "Bash", Input: `{"command":"lefthook run pre-commit","run_in_background":true}`, Output: "background-only"},
		{Tool: "Bash", Input: `{"command":"git add main.go"}`, Output: "staged"},
		{Tool: "Bash", Input: `{"cmd":"lefthook run pre-commit"}`, Output: "✔ executed in 1s", Error: ""},
		{Tool: "Bash", Input: `{"command":"git commit -m feat"}`, Output: "committed"},
	})
	require.Len(t, runs, 3)
	assert.Contains(t, runs[0].Command, "git add")
	assert.Contains(t, runs[1].Command, "lefthook")
	assert.Contains(t, runs[2].Command, "git commit")
}

func TestAcceptanceRequiresStagedGateEvidence(t *testing.T) {
	assert.True(t, acceptanceRequiresStagedGateEvidence("2. lefthook run pre-commit passes"))
	assert.True(t, acceptanceRequiresStagedGateEvidence("run Pre-Commit hooks"))
	assert.False(t, acceptanceRequiresStagedGateEvidence("1. TestFoo passes"))
	assert.False(t, beadRequiresStagedGateEvidence(nil))
}
