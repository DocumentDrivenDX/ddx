package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type worktreeAddFailGitOps struct {
	baseRev     string
	addedPath   string
	removedPath string
}

func (g *worktreeAddFailGitOps) HeadRev(dir string) (string, error) { return g.baseRev, nil }
func (g *worktreeAddFailGitOps) ResolveRev(dir, rev string) (string, error) {
	return g.baseRev, nil
}
func (g *worktreeAddFailGitOps) WorktreeAdd(dir, wtPath, rev string) error {
	g.addedPath = wtPath
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(wtPath, "partial.txt"), []byte("partial\n"), 0o644); err != nil {
		return err
	}
	return fmt.Errorf("git worktree add: No space left on device")
}
func (g *worktreeAddFailGitOps) WorktreeRemove(dir, wtPath string) error {
	g.removedPath = wtPath
	return os.RemoveAll(wtPath)
}
func (g *worktreeAddFailGitOps) WorktreeList(dir string) ([]string, error) { return nil, nil }
func (g *worktreeAddFailGitOps) WorktreePrune(dir string) error            { return nil }
func (g *worktreeAddFailGitOps) IsDirty(dir string) (bool, error)          { return false, nil }
func (g *worktreeAddFailGitOps) SynthesizeCommit(dir, msg string) (bool, error) {
	return false, nil
}
func (g *worktreeAddFailGitOps) UpdateRef(dir, ref, sha string) error { return nil }
func (g *worktreeAddFailGitOps) DeleteRef(dir, ref string) error      { return nil }

type cleanupAttemptWorktreeGitOps struct {
	removedPaths []string
}

func (g *cleanupAttemptWorktreeGitOps) HeadRev(string) (string, error)            { return "", nil }
func (g *cleanupAttemptWorktreeGitOps) ResolveRev(string, string) (string, error) { return "", nil }
func (g *cleanupAttemptWorktreeGitOps) WorktreeAdd(string, string, string) error  { return nil }
func (g *cleanupAttemptWorktreeGitOps) WorktreeRemove(_ string, wtPath string) error {
	g.removedPaths = append(g.removedPaths, wtPath)
	return nil
}
func (g *cleanupAttemptWorktreeGitOps) WorktreeList(string) ([]string, error) { return nil, nil }
func (g *cleanupAttemptWorktreeGitOps) WorktreePrune(string) error            { return nil }
func (g *cleanupAttemptWorktreeGitOps) IsDirty(string) (bool, error)          { return false, nil }
func (g *cleanupAttemptWorktreeGitOps) SynthesizeCommit(string, string) (bool, error) {
	return false, nil
}
func (g *cleanupAttemptWorktreeGitOps) UpdateRef(string, string, string) error { return nil }
func (g *cleanupAttemptWorktreeGitOps) DeleteRef(string, string) error         { return nil }

func TestExecuteBeadWorktreeAddFailure_RemovesPartialDir(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ddxroot.DirName, "beads.jsonl"), []byte("{}\n"), 0o644))

	tempRoot := t.TempDir()
	t.Setenv("DDX_EXEC_WT_DIR", tempRoot)

	gitOps := &worktreeAddFailGitOps{baseRev: "base123"}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-worktree-cleanup", rcfg, ExecuteBeadRuntime{}, gitOps)
	// A disk-exhaustion worktree-add failure now surfaces as a
	// resource_exhausted result (not a raw error) so the execute-loop releases
	// the claim instead of leaving the bead claimed-but-open (ddx-f677a50b).
	// The partial worktree dir must still be cleaned up.
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, ExecuteBeadStatusResourceExhausted, res.Status)
	assert.NotEmpty(t, gitOps.addedPath)
	assert.Empty(t, gitOps.removedPath)
	assert.NoFileExists(t, gitOps.addedPath)
}

func TestCleanupAttemptWorktree_RemovesForNonSuccessOutcomes(t *testing.T) {
	cases := []struct {
		name    string
		outcome string
	}{
		{name: "provider_connectivity", outcome: "provider_connectivity"},
		{name: "no_evidence_produced", outcome: ExecuteBeadOutcomeTaskNoEvidence},
		{name: "execution_failed", outcome: ExecuteBeadOutcomeTaskFailed},
		{name: "structural_validation_failed", outcome: "structural_validation_failed"},
		{name: "land_conflict", outcome: "land_conflict"},
		{name: "post_run_check_failed", outcome: "post_run_check_failed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gitOps := &cleanupAttemptWorktreeGitOps{}
			got := cleanupAttemptWorktree(gitOps, "/project/root", "/project/root/.execute-bead-wt-test", tc.outcome, false)
			require.True(t, got)
			require.Equal(t, []string{"/project/root/.execute-bead-wt-test"}, gitOps.removedPaths)
		})
	}
}

func TestCleanupAttemptWorktree_SuccessSkipsRemoval(t *testing.T) {
	gitOps := &cleanupAttemptWorktreeGitOps{}
	got := cleanupAttemptWorktree(gitOps, "/project/root", "/project/root/.execute-bead-wt-test", ExecuteBeadOutcomeTaskSucceeded, false)
	require.False(t, got)
	require.Empty(t, gitOps.removedPaths)
}

func TestCleanupAttemptWorktree_PreserveFlagSkipsRemoval(t *testing.T) {
	gitOps := &cleanupAttemptWorktreeGitOps{}
	got := cleanupAttemptWorktree(gitOps, "/project/root", "/project/root/.execute-bead-wt-test", ExecuteBeadOutcomeTaskFailed, true)
	require.False(t, got)
	require.Empty(t, gitOps.removedPaths)
}
