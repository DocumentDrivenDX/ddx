package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

type worktreeLostGitOps struct {
	projectRoot string
	baseRev     string
	worktree    string
}

func (g *worktreeLostGitOps) HeadRev(dir string) (string, error) {
	if filepath.Clean(dir) == filepath.Clean(g.projectRoot) {
		return g.baseRev, nil
	}
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: chdir %s: no such file or directory", dir)
	}
	return g.baseRev, nil
}

func (g *worktreeLostGitOps) ResolveRev(dir, rev string) (string, error) {
	return g.baseRev, nil
}

func (g *worktreeLostGitOps) WorktreeAdd(dir, wtPath, rev string) error {
	g.worktree = wtPath
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		return err
	}
	ddxDir := filepath.Join(wtPath, ddxroot.DirName)
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		return err
	}
	store := bead.NewStore(ddxDir)
	if err := store.Init(); err != nil {
		return err
	}
	return store.Create(&bead.Bead{ID: "ddx-worktree-lost", Title: "Worktree lost"})
}

func (g *worktreeLostGitOps) WorktreeRemove(dir, wtPath string) error { return nil }

func (g *worktreeLostGitOps) WorktreeList(dir string) ([]string, error) {
	if g.worktree == "" {
		return nil, nil
	}
	return []string{g.worktree}, nil
}

func (g *worktreeLostGitOps) WorktreePrune(dir string) error   { return nil }
func (g *worktreeLostGitOps) IsDirty(dir string) (bool, error) { return false, nil }
func (g *worktreeLostGitOps) SynthesizeCommit(dir, msg string) (bool, error) {
	return false, nil
}
func (g *worktreeLostGitOps) UpdateRef(dir, ref, sha string) error { return nil }
func (g *worktreeLostGitOps) DeleteRef(dir, ref string) error      { return nil }

type removeAttemptWorktreeRunner struct{}

func (removeAttemptWorktreeRunner) Run(opts RunArgs) (*Result, error) {
	if strings.TrimSpace(opts.WorkDir) != "" {
		_ = os.RemoveAll(opts.WorkDir)
	}
	return &Result{ExitCode: 0}, nil
}

func runWorktreeLostExecuteBead(t *testing.T) (*ExecuteBeadResult, string, error) {
	t.Helper()
	setExecutionWorktreeRootForTest(t)
	projectRoot := setupArtifactTestProjectRoot(t)
	gitOps := &worktreeLostGitOps{
		projectRoot: projectRoot,
		baseRev:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{Harness: "test-harness"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-worktree-lost", rcfg, ExecuteBeadRuntime{
		AgentRunner: removeAttemptWorktreeRunner{},
	}, gitOps)
	return res, projectRoot, err
}

func TestExecuteBead_WorktreeLostWritesDiagnosticResult(t *testing.T) {
	res, projectRoot, err := runWorktreeLostExecuteBead(t)
	if err == nil {
		t.Fatalf("ExecuteBeadWithConfig succeeded, want worktree HEAD error")
	}
	if res == nil {
		t.Fatalf("ExecuteBeadWithConfig returned nil result: %v", err)
	}
	if res.FailureMode != FailureModeWorktreeLost {
		t.Fatalf("FailureMode = %q, want %q; err=%v", res.FailureMode, FailureModeWorktreeLost, err)
	}
	if res.WorktreePath == "" {
		t.Fatalf("WorktreePath missing from result")
	}
	if res.AttemptDiagnostics == nil {
		t.Fatalf("AttemptDiagnostics missing from result")
	}
	if res.AttemptDiagnostics.WorktreePath != res.WorktreePath {
		t.Fatalf("diagnostic worktree_path = %q, want %q", res.AttemptDiagnostics.WorktreePath, res.WorktreePath)
	}
	if res.AttemptDiagnostics.WorktreePathExists {
		t.Fatalf("diagnostic says vanished worktree still exists")
	}
	if !res.AttemptDiagnostics.RunStatePresent {
		t.Fatalf("diagnostic missing run-state presence")
	}
	if !res.AttemptDiagnostics.GitWorktreeRegistered {
		t.Fatalf("diagnostic missing git worktree registration")
	}

	var stored ExecuteBeadResult
	raw, readErr := os.ReadFile(filepath.Join(projectRoot, res.ResultFile))
	if readErr != nil {
		t.Fatalf("read result.json: %v", readErr)
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("parse result.json: %v", err)
	}
	if stored.FailureMode != FailureModeWorktreeLost {
		t.Fatalf("stored failure_mode = %q, want %q", stored.FailureMode, FailureModeWorktreeLost)
	}
	if stored.WorktreePath == "" || stored.AttemptDiagnostics == nil {
		t.Fatalf("stored result missing worktree diagnostics: %+v", stored)
	}
}

func TestExecuteBead_WorktreeLostPreservesEvidenceFiles(t *testing.T) {
	res, projectRoot, err := runWorktreeLostExecuteBead(t)
	if err == nil {
		t.Fatalf("ExecuteBeadWithConfig succeeded, want worktree HEAD error")
	}
	if res == nil {
		t.Fatalf("ExecuteBeadWithConfig returned nil result: %v", err)
	}
	for _, rel := range []string{res.PromptFile, res.ManifestFile, res.ResultFile} {
		if rel == "" {
			t.Fatalf("evidence path missing in result: %+v", res)
		}
		if _, statErr := os.Stat(filepath.Join(projectRoot, rel)); statErr != nil {
			t.Fatalf("evidence file %s missing: %v", rel, statErr)
		}
	}
}
