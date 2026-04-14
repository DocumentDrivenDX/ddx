package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// gateTestGitOps is a minimal GitOps mock for execute-bead gate enforcement tests.
// WorktreeAdd calls wtSetupFn to let the test populate the worktree directory.
type gateTestGitOps struct {
	projectRoot string
	baseRev     string
	resultRev   string
	mergeErr    error
	wtSetupFn   func(wtPath string)
	// recorded outputs
	mergeCalled  bool
	mergedRev    string
	preserveRef  string
	preservedSHA string
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

func (m *gateTestGitOps) Merge(dir, rev string) error {
	m.mergeCalled = true
	m.mergedRev = rev
	return m.mergeErr
}

func (m *gateTestGitOps) UpdateRef(dir, ref, sha string) error {
	m.preserveRef = ref
	m.preservedSHA = sha
	return nil
}

func (m *gateTestGitOps) IsDirty(dir string) (bool, error) { return false, nil }

func (m *gateTestGitOps) SynthesizeCommit(dir string) (bool, error) { return false, nil }

// gateTestAgentRunner is a minimal AgentRunner mock that always succeeds.
type gateTestAgentRunner struct {
	exitCode int
}

func (r *gateTestAgentRunner) Run(opts RunOptions) (*Result, error) {
	return &Result{ExitCode: r.exitCode}, nil
}

// setupGateTestProjectRoot creates projectRoot with the minimal .ddx/ structure
// needed for the lock and artifact bundle creation.
func setupGateTestProjectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	ddxDir := filepath.Join(root, ".ddx")
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
	ddxDir := filepath.Join(wtPath, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := bead.NewStore(ddxDir)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	b := &bead.Bead{
		ID:    beadID,
		Title: "Gate test bead",
		Extra: map[string]any{"spec-id": specID},
	}
	if err := store.Create(b); err != nil {
		t.Fatal(err)
	}

	// Governing spec document so ResolveGoverningRefs resolves the ID.
	writeArtifactDoc(t, wtPath, specID)

	if withGate {
		cmd := fmt.Sprintf("exit %d", gateExitCode)
		writeGateDoc(t, wtPath, "exec."+specID+".smoke", specID, true, []string{"sh", "-c", cmd})
	}
}

// TestExecuteBead_MergeDefault_NoGates verifies the merge-by-default contract:
// when the agent succeeds and produces changes with no required gates defined,
// the result is merged rather than preserved.
func TestExecuteBead_MergeDefault_NoGates(t *testing.T) {
	const beadID = "ddx-gate-test-01"
	const specID = "FEAT-GATETEST"

	projectRoot := setupGateTestProjectRoot(t)

	gitOps := &gateTestGitOps{
		projectRoot: projectRoot,
		baseRev:     "aaa0000000000001",
		resultRev:   "bbb0000000000001", // different from baseRev → agent made commits
		wtSetupFn: func(wtPath string) {
			setupGateTestWorktree(t, wtPath, beadID, specID, false, 0)
		},
	}
	runner := &gateTestAgentRunner{exitCode: 0}

	res, err := ExecuteBead(projectRoot, beadID, ExecuteBeadOptions{}, gitOps, runner)
	if err != nil {
		t.Fatalf("ExecuteBead returned error: %v", err)
	}

	if res.Outcome != "merged" {
		t.Errorf("expected outcome=merged, got %q (reason=%q)", res.Outcome, res.Reason)
	}
	if !gitOps.mergeCalled {
		t.Error("expected Merge to be called for merge-by-default path")
	}
	if gitOps.preserveRef != "" {
		t.Errorf("expected no preserve ref, got %q", gitOps.preserveRef)
	}
	if res.RequiredExecSummary != "skipped" {
		t.Errorf("expected required_exec_summary=skipped (no gates), got %q", res.RequiredExecSummary)
	}
}

// TestExecuteBead_RequiredGateFails_Preserves verifies that when a required
// execution gate fails after the agent run, the result is preserved instead of
// merged and the gate outcome is recorded.
func TestExecuteBead_RequiredGateFails_Preserves(t *testing.T) {
	const beadID = "ddx-gate-test-02"
	const specID = "FEAT-GATEFAIL"

	projectRoot := setupGateTestProjectRoot(t)

	gitOps := &gateTestGitOps{
		projectRoot: projectRoot,
		baseRev:     "aaa0000000000002",
		resultRev:   "bbb0000000000002",
		wtSetupFn: func(wtPath string) {
			setupGateTestWorktree(t, wtPath, beadID, specID, true, 1) // gate exits 1
		},
	}
	runner := &gateTestAgentRunner{exitCode: 0}

	res, err := ExecuteBead(projectRoot, beadID, ExecuteBeadOptions{}, gitOps, runner)
	if err != nil {
		t.Fatalf("ExecuteBead returned error: %v", err)
	}

	if res.Outcome != "preserved" {
		t.Errorf("expected outcome=preserved when required gate fails, got %q", res.Outcome)
	}
	if gitOps.mergeCalled {
		t.Error("Merge must not be called when required gate fails")
	}
	if gitOps.preserveRef == "" {
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
// gate passes after the agent run, the result is merged (gate does not block landing).
func TestExecuteBead_RequiredGatePasses_Merges(t *testing.T) {
	const beadID = "ddx-gate-test-03"
	const specID = "FEAT-GATEPASS"

	projectRoot := setupGateTestProjectRoot(t)

	gitOps := &gateTestGitOps{
		projectRoot: projectRoot,
		baseRev:     "aaa0000000000003",
		resultRev:   "bbb0000000000003",
		wtSetupFn: func(wtPath string) {
			setupGateTestWorktree(t, wtPath, beadID, specID, true, 0) // gate exits 0
		},
	}
	runner := &gateTestAgentRunner{exitCode: 0}

	res, err := ExecuteBead(projectRoot, beadID, ExecuteBeadOptions{}, gitOps, runner)
	if err != nil {
		t.Fatalf("ExecuteBead returned error: %v", err)
	}

	if res.Outcome != "merged" {
		t.Errorf("expected outcome=merged when required gate passes, got %q (reason=%q)", res.Outcome, res.Reason)
	}
	if !gitOps.mergeCalled {
		t.Error("Merge must be called when required gate passes")
	}
	if gitOps.preserveRef != "" {
		t.Errorf("expected no preserve ref when gate passes, got %q", gitOps.preserveRef)
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
