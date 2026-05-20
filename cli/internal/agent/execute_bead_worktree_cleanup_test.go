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
