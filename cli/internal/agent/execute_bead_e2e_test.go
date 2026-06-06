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
)

// gateTestGitOps is a minimal GitOps mock for execute-bead gate enforcement tests.
// WorktreeAdd calls wtSetupFn to let the test populate the worktree directory.
type gateTestGitOps struct {
	projectRoot string
	baseRev     string
	resultRev   string
	wtSetupFn   func(wtPath string)
}

func (m *gateTestGitOps) HeadRev(dir string) (string, error) {
	if filepath.Clean(dir) == filepath.Clean(m.projectRoot) {
		return m.baseRev, nil
	}
	return m.resultRev, nil
}

func (m *gateTestGitOps) ResolveRev(dir, rev string) (string, error) {
	return m.baseRev, nil
}

func (m *gateTestGitOps) WorktreeAdd(dir, wtPath, rev string) error {
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		return err
	}
	if m.wtSetupFn != nil {
		m.wtSetupFn(wtPath)
	}
	return nil
}

func (m *gateTestGitOps) WorktreeRemove(dir, wtPath string) error { return nil }

func (m *gateTestGitOps) WorktreeList(dir string) ([]string, error) { return nil, nil }

func (m *gateTestGitOps) WorktreePrune(dir string) error { return nil }

func (m *gateTestGitOps) IsDirty(dir string) (bool, error) { return false, nil }

func (m *gateTestGitOps) SynthesizeCommit(dir, msg string) (bool, error) { return false, nil }

func (m *gateTestGitOps) UpdateRef(dir, ref, sha string) error { return nil }

func (m *gateTestGitOps) DeleteRef(dir, ref string) error { return nil }

// gateTestOrchestratorGitOps is an OrchestratorGitOps mock for landing tests.
// After the land-coordinator redesign, OrchestratorGitOps only needs UpdateRef;
// the old Merge path has been replaced by LandingAdvancer (see execute_bead_land.go).
type gateTestOrchestratorGitOps struct {
	preserveRef string
	preserveSHA string
}

func (m *gateTestOrchestratorGitOps) UpdateRef(dir, ref, sha string) error {
	m.preserveRef = ref
	m.preserveSHA = sha
	return nil
}

// gateTestLandingAdvancer is a minimal LandingAdvancer stub for gate tests.
// It always returns a landed result so the gate tests can assert the landing
// path without spinning up a real coordinator.
type gateTestLandingAdvancer struct {
	called    bool
	returnErr error
}

func (a *gateTestLandingAdvancer) advance(res *ExecuteBeadResult) (*LandResult, error) {
	a.called = true
	if a.returnErr != nil {
		return nil, a.returnErr
	}
	return &LandResult{Status: "landed", NewTip: res.ResultRev}, nil
}

// gateTestAgentRunner is a minimal AgentRunner mock that always succeeds.
type gateTestAgentRunner struct {
	exitCode int
}

func (r *gateTestAgentRunner) Run(opts RunArgs) (*Result, error) {
	return &Result{ExitCode: r.exitCode}, nil
}

type gateTestResultRunner struct {
	result *Result
	err    error
}

func (r *gateTestResultRunner) Run(opts RunArgs) (*Result, error) {
	return r.result, r.err
}

// setupGateTestProjectRoot creates projectRoot with the minimal .ddx/ structure
// needed for the lock and artifact bundle creation.
func setupGateTestProjectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	ddxDir := filepath.Join(root, ddxroot.DirName)
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// setupGateTestWorktree populates wtPath with a bead store (containing a bead with
// spec-id=specID), a governing spec document, and optionally a required execution gate.
func setupGateTestWorktree(t *testing.T, wtPath, beadID, specID string, withGate bool, gateExitCode int) {
	t.Helper()

	// Bead store in the worktree
	ddxDir := filepath.Join(wtPath, ddxroot.DirName)
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := bead.NewStore(ddxDir)
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	b := &bead.Bead{
		ID:    beadID,
		Title: "Gate test bead",
		Extra: map[string]any{"spec-id": specID},
	}
	if err := store.Create(context.Background(), b); err != nil {
		t.Fatal(err)
	}

	// Governing spec document so ResolveGoverningRefs resolves the ID.
	writeArtifactDoc(t, wtPath, specID)

	if withGate {
		cmd := "exit " + itoa(gateExitCode)
		writeGateDoc(t, wtPath, "exec."+specID+".smoke", specID, true, []string{"sh", "-c", cmd})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// TestExecuteBead_MergeDefault_NoGates verifies the merge-by-default contract:
// when the agent succeeds and produces changes with no required gates defined,
// the orchestrator merges the result.
func TestExecuteBead_MergeDefault_NoGates(t *testing.T) {
	const beadID = "ddx-gate-test-01"
	const specID = "FEAT-GATETEST"

	projectRoot := setupGateTestProjectRoot(t)
	wtPath := t.TempDir()
	setupGateTestWorktree(t, wtPath, beadID, specID, false, 0)

	res := &ExecuteBeadResult{
		BeadID:    beadID,
		BaseRev:   "aaa0000000000001",
		ResultRev: "bbb0000000000001", // different → agent made commits
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
	}

	orch := &gateTestOrchestratorGitOps{}
	advancer := &gateTestLandingAdvancer{}
	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		WtPath:          wtPath,
		GovernIDs:       []string{specID},
		LandingAdvancer: advancer.advance,
	})
	if err != nil {
		t.Fatalf("LandBeadResult returned error: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "merged" {
		t.Errorf("expected outcome=merged, got %q (reason=%q)", res.Outcome, res.Reason)
	}
	if !advancer.called {
		t.Error("expected LandingAdvancer to be called for the merge-by-default path")
	}
	if orch.preserveRef != "" {
		t.Errorf("expected no preserve ref, got %q", orch.preserveRef)
	}
	if res.RequiredExecSummary != "skipped" {
		t.Errorf("expected required_exec_summary=skipped (no gates), got %q", res.RequiredExecSummary)
	}
}

// TestExecuteBead_RequiredGateFails_Preserves verifies that when a required
// execution gate fails, the orchestrator preserves the result instead of merging.
func TestExecuteBead_RequiredGateFails_Preserves(t *testing.T) {
	const beadID = "ddx-gate-test-02"
	const specID = "FEAT-GATEFAIL"

	projectRoot := setupGateTestProjectRoot(t)
	wtPath := t.TempDir()
	setupGateTestWorktree(t, wtPath, beadID, specID, true, 1) // gate exits 1

	res := &ExecuteBeadResult{
		BeadID:    beadID,
		BaseRev:   "aaa0000000000002",
		ResultRev: "bbb0000000000002",
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
	}

	orch := &gateTestOrchestratorGitOps{}
	advancer := &gateTestLandingAdvancer{}
	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		WtPath:          wtPath,
		GovernIDs:       []string{specID},
		LandingAdvancer: advancer.advance,
	})
	if err != nil {
		t.Fatalf("LandBeadResult returned error: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "preserved" {
		t.Errorf("expected outcome=preserved when required gate fails, got %q", res.Outcome)
	}
	if advancer.called {
		t.Error("LandingAdvancer must not be called when required gate fails")
	}
	if orch.preserveRef == "" {
		t.Error("expected a preserve ref to be set when required gate fails")
	}
	if res.RequiredExecSummary != "fail" {
		t.Errorf("expected required_exec_summary=fail, got %q", res.RequiredExecSummary)
	}
	if len(res.GateResults) == 0 {
		t.Error("expected gate results to be recorded")
	}
	if res.GateResults[0].Status != "fail" {
		t.Errorf("expected first gate status=fail, got %q", res.GateResults[0].Status)
	}
}

// TestExecuteBead_RequiredGatePasses_Merges verifies that when a required execution
// gate passes, the orchestrator merges the result (gate does not block landing).
func TestExecuteBead_RequiredGatePasses_Merges(t *testing.T) {
	const beadID = "ddx-gate-test-03"
	const specID = "FEAT-GATEPASS"

	projectRoot := setupGateTestProjectRoot(t)
	wtPath := t.TempDir()
	setupGateTestWorktree(t, wtPath, beadID, specID, true, 0) // gate exits 0

	res := &ExecuteBeadResult{
		BeadID:    beadID,
		BaseRev:   "aaa0000000000003",
		ResultRev: "bbb0000000000003",
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
	}

	orch := &gateTestOrchestratorGitOps{}
	advancer := &gateTestLandingAdvancer{}
	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		WtPath:          wtPath,
		GovernIDs:       []string{specID},
		LandingAdvancer: advancer.advance,
	})
	if err != nil {
		t.Fatalf("LandBeadResult returned error: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "merged" {
		t.Errorf("expected outcome=merged when required gate passes, got %q (reason=%q)", res.Outcome, res.Reason)
	}
	if !advancer.called {
		t.Error("LandingAdvancer must be called when required gate passes")
	}
	if orch.preserveRef != "" {
		t.Errorf("expected no preserve ref when gate passes, got %q", orch.preserveRef)
	}
	if res.RequiredExecSummary != "pass" {
		t.Errorf("expected required_exec_summary=pass, got %q", res.RequiredExecSummary)
	}
	if len(res.GateResults) == 0 {
		t.Error("expected gate results to be recorded")
	}
	if res.GateResults[0].Status != "pass" {
		t.Errorf("expected first gate status=pass, got %q", res.GateResults[0].Status)
	}
}

// rationaleTestRunner is a fake AgentRunner that writes a no_changes_rationale.txt
// file into the worktree's execution bundle directory before returning exit 0.
// The file is excluded from SynthesizeCommit, so the outcome will be task_no_changes.
type rationaleTestRunner struct {
	rationale string
}

func (r *rationaleTestRunner) Run(opts RunArgs) (*Result, error) {
	attemptID := opts.Correlation["attempt_id"]
	if attemptID == "" {
		return &Result{ExitCode: 0}, nil
	}
	dir := filepath.Join(opts.WorkDir, ddxroot.DirName, "executions", attemptID)
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "no_changes_rationale.txt"), []byte(r.rationale), 0o644)
	return &Result{ExitCode: 0}, nil
}

// TestExecuteBead_NoChangesRationalePopulated verifies that when the agent writes
// a no_changes_rationale.txt file into the execution bundle dir inside the worktree,
// ExecuteBead reads it and populates ExecuteBeadResult.NoChangesRationale.
func TestExecuteBead_NoChangesRationalePopulated(t *testing.T) {
	const beadID = "ddx-rationale-01"
	const rationale = "Work already present in commit 1da6495 (cli/internal/bead/store.go). " +
		"TestReadyExecutionExcludesEpics confirms the epic filter passes."

	projectRoot := setupGateTestProjectRoot(t)

	// Both HeadRev calls return the same rev so resultRev == baseRev → task_no_changes.
	const fixedRev = "aaaaaaaabbbbbbbb"
	gitOps := &gateTestGitOps{
		projectRoot: projectRoot,
		baseRev:     fixedRev,
		resultRev:   fixedRev,
		wtSetupFn: func(wtPath string) {
			// Populate a minimal bead store so prepareArtifacts succeeds.
			ddxDir := filepath.Join(wtPath, ddxroot.DirName)
			if err := os.MkdirAll(ddxDir, 0o755); err != nil {
				t.Fatal(err)
			}
			store := bead.NewStore(ddxDir)
			if err := store.Init(context.Background()); err != nil {
				t.Fatal(err)
			}
			b := &bead.Bead{ID: beadID, Title: "Rationale test bead"}
			if err := store.Create(context.Background(), b); err != nil {
				t.Fatal(err)
			}
		},
	}

	runner := &rationaleTestRunner{rationale: rationale}

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{AgentRunner: runner}, gitOps)
	if err != nil {
		t.Fatalf("ExecuteBead returned error: %v", err)
	}
	if res.Outcome != ExecuteBeadOutcomeTaskNoChanges {
		t.Errorf("expected outcome=%s, got %q", ExecuteBeadOutcomeTaskNoChanges, res.Outcome)
	}
	if res.NoChangesRationale != rationale {
		t.Errorf("NoChangesRationale mismatch\n got:  %q\nwant: %q", res.NoChangesRationale, rationale)
	}
}

// TestExecuteBead_MixedCommitAndBlockedNoChangesRationalePreservesEvidence
// verifies that a commit plus no_changes rationale is rejected as a mixed
// signal while preserving the rationale text for downstream evidence.
func TestExecuteBead_MixedCommitAndBlockedNoChangesRationalePreservesEvidence(t *testing.T) {
	const beadID = "ddx-rationale-03"
	const rationale = "status: blocked\nreason: external blocker"

	projectRoot := setupGateTestProjectRoot(t)

	const baseRev = "ddddddddeeeeeeee"
	const resultRev = "ffffffff00000000"
	gitOps := &gateTestGitOps{
		projectRoot: projectRoot,
		baseRev:     baseRev,
		resultRev:   resultRev,
		wtSetupFn: func(wtPath string) {
			ddxDir := filepath.Join(wtPath, ddxroot.DirName)
			if err := os.MkdirAll(ddxDir, 0o755); err != nil {
				t.Fatal(err)
			}
			store := bead.NewStore(ddxDir)
			if err := store.Init(context.Background()); err != nil {
				t.Fatal(err)
			}
			b := &bead.Bead{ID: beadID, Title: "Mixed rationale bead"}
			if err := store.Create(context.Background(), b); err != nil {
				t.Fatal(err)
			}
		},
	}

	runner := &rationaleTestRunner{rationale: rationale}

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{AgentRunner: runner}, gitOps)
	if err != nil {
		t.Fatalf("ExecuteBead returned error: %v", err)
	}
	if res.Outcome != ExecuteBeadOutcomeTaskFailed {
		t.Fatalf("expected outcome=%s, got %q", ExecuteBeadOutcomeTaskFailed, res.Outcome)
	}
	if res.Status != ExecuteBeadStatusExecutionFailed {
		t.Fatalf("expected status=%s, got %q", ExecuteBeadStatusExecutionFailed, res.Status)
	}
	if res.NoChangesRationale != rationale {
		t.Fatalf("expected rationale preserved in result, got %q", res.NoChangesRationale)
	}
	if !strings.Contains(res.Detail, mixedCommitAndNoChangesRationaleReason) {
		t.Fatalf("expected mixed result detail, got %q", res.Detail)
	}
	if res.ResultRev == baseRev {
		t.Fatalf("expected synthesized result rev to differ from base rev")
	}

	report := ReportFromExecuteBeadResult(res, "standard")
	if report.NoChangesRationale != rationale {
		t.Fatalf("expected rationale preserved in report, got %q", report.NoChangesRationale)
	}
	if report.Detail != res.Detail {
		t.Fatalf("report detail mismatch\n got:  %q\nwant: %q", report.Detail, res.Detail)
	}

	landing, err := LandBeadResult(projectRoot, res, &gateTestOrchestratorGitOps{}, BeadLandingOptions{})
	if err != nil {
		t.Fatalf("LandBeadResult returned error: %v", err)
	}
	if landing.Outcome != "preserved" {
		t.Fatalf("expected mixed attempt to be preserved, got %q", landing.Outcome)
	}
	if landing.PreserveRef == "" {
		t.Fatalf("expected preserve ref for mixed attempt")
	}
}

// TestExecuteBead_NoEvidenceProducedWhenRationaleAbsent verifies that a clean
// agent exit with no commits and no rationale is classified separately from
// legitimate no_changes.
func TestExecuteBead_NoEvidenceProducedWhenRationaleAbsent(t *testing.T) {
	const beadID = "ddx-rationale-02"

	projectRoot := setupGateTestProjectRoot(t)

	const fixedRev = "ccccccccdddddddd"
	gitOps := &gateTestGitOps{
		projectRoot: projectRoot,
		baseRev:     fixedRev,
		resultRev:   fixedRev,
		wtSetupFn: func(wtPath string) {
			ddxDir := filepath.Join(wtPath, ddxroot.DirName)
			if err := os.MkdirAll(ddxDir, 0o755); err != nil {
				t.Fatal(err)
			}
			store := bead.NewStore(ddxDir)
			if err := store.Init(context.Background()); err != nil {
				t.Fatal(err)
			}
			b := &bead.Bead{ID: beadID, Title: "No rationale bead"}
			if err := store.Create(context.Background(), b); err != nil {
				t.Fatal(err)
			}
		},
	}

	// Runner that does NOT write the rationale file.
	runner := &gateTestAgentRunner{exitCode: 0}

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{AgentRunner: runner}, gitOps)
	if err != nil {
		t.Fatalf("ExecuteBead returned error: %v", err)
	}
	if res.Outcome != ExecuteBeadOutcomeTaskNoEvidence {
		t.Errorf("expected outcome=%s, got %q", ExecuteBeadOutcomeTaskNoEvidence, res.Outcome)
	}
	if res.Status != ExecuteBeadStatusNoEvidenceProduced {
		t.Errorf("expected status=%s, got %q", ExecuteBeadStatusNoEvidenceProduced, res.Status)
	}
	if res.FailureMode != FailureModeNoEvidenceProduced {
		t.Errorf("expected failure_mode=%s, got %q", FailureModeNoEvidenceProduced, res.FailureMode)
	}
	if res.NoChangesRationale != "" {
		t.Errorf("expected empty NoChangesRationale when file absent, got %q", res.NoChangesRationale)
	}
}

func TestExecuteBead_ServiceErrorWithZeroExitIsExecutionFailed(t *testing.T) {
	const beadID = "ddx-service-error-01"

	projectRoot := setupGateTestProjectRoot(t)

	const fixedRev = "eeeeeeeeffffffff"
	gitOps := &gateTestGitOps{
		projectRoot: projectRoot,
		baseRev:     fixedRev,
		resultRev:   fixedRev,
		wtSetupFn: func(wtPath string) {
			ddxDir := filepath.Join(wtPath, ddxroot.DirName)
			if err := os.MkdirAll(ddxDir, 0o755); err != nil {
				t.Fatal(err)
			}
			store := bead.NewStore(ddxDir)
			if err := store.Init(context.Background()); err != nil {
				t.Fatal(err)
			}
			b := &bead.Bead{ID: beadID, Title: "Service error bead"}
			if err := store.Create(context.Background(), b); err != nil {
				t.Fatal(err)
			}
		},
	}

	runner := &gateTestResultRunner{result: &Result{
		ExitCode: 0,
		Error:    "ResolveRoute: no viable routing candidate: 3 candidates rejected",
	}}

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{AgentRunner: runner}, gitOps)
	if err != nil {
		t.Fatalf("ExecuteBead returned error: %v", err)
	}
	if res.Outcome != ExecuteBeadOutcomeTaskFailed {
		t.Errorf("expected outcome=%s, got %q", ExecuteBeadOutcomeTaskFailed, res.Outcome)
	}
	if res.Status != ExecuteBeadStatusExecutionFailed {
		t.Errorf("expected status=%s, got %q", ExecuteBeadStatusExecutionFailed, res.Status)
	}
	if res.FailureMode != FailureModeNoViableProvider {
		t.Errorf("expected failure_mode=%s, got %q", FailureModeNoViableProvider, res.FailureMode)
	}
}
