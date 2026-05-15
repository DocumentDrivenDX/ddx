package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type executeBeadResourcePreflightGitOps struct {
	headRevCalls       int
	resolveRevCalls    int
	worktreeAddCalls   int
	synthesizeCalls    int
	worktreeRemove     int
	worktreeListCalls  int
	worktreePruneCalls int
}

func (g *executeBeadResourcePreflightGitOps) HeadRev(dir string) (string, error) {
	g.headRevCalls++
	return "base123", nil
}

func (g *executeBeadResourcePreflightGitOps) ResolveRev(dir, rev string) (string, error) {
	g.resolveRevCalls++
	return "base123", nil
}

func (g *executeBeadResourcePreflightGitOps) WorktreeAdd(dir, wtPath, rev string) error {
	g.worktreeAddCalls++
	return errors.New("unexpected worktree add")
}

func (g *executeBeadResourcePreflightGitOps) WorktreeRemove(dir, wtPath string) error {
	g.worktreeRemove++
	return nil
}

func (g *executeBeadResourcePreflightGitOps) WorktreeList(dir string) ([]string, error) {
	g.worktreeListCalls++
	return nil, nil
}

func (g *executeBeadResourcePreflightGitOps) WorktreePrune(dir string) error {
	g.worktreePruneCalls++
	return nil
}

func (g *executeBeadResourcePreflightGitOps) IsDirty(dir string) (bool, error) {
	return false, nil
}

func (g *executeBeadResourcePreflightGitOps) SynthesizeCommit(dir, msg string) (bool, error) {
	g.synthesizeCalls++
	return false, nil
}

func (g *executeBeadResourcePreflightGitOps) UpdateRef(dir, ref, sha string) error { return nil }
func (g *executeBeadResourcePreflightGitOps) DeleteRef(dir, ref string) error      { return nil }

type executeBeadFailingResourceChecker struct {
	calls int
}

func (c *executeBeadFailingResourceChecker) Check(ctx context.Context) (ExecutionResourceCheckResult, error) {
	_ = ctx
	c.calls++
	return ExecutionResourceCheckResult{
		ProjectRoot: "project",
		TempRoot:    "temp",
	}, &ResourceExhaustedError{Detail: "temp root is full"}
}

func TestExecuteBeadResourcePreflight_FailsBeforeWorktreeSetup(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ddxroot.DirName, "beads.jsonl"), []byte("{}\n"), 0o644))

	gitOps := &executeBeadResourcePreflightGitOps{}
	checker := &executeBeadFailingResourceChecker{}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-resource-worker", rcfg, ExecuteBeadRuntime{
		ResourceChecker: checker,
	}, gitOps)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, ExecuteBeadStatusResourceExhausted, res.Status)
	assert.Contains(t, res.Error, "resource_exhausted")
	require.NotNil(t, res.ResourceExhausted)
	assert.Equal(t, 1, checker.calls)
	assert.Zero(t, gitOps.headRevCalls)
	assert.Zero(t, gitOps.resolveRevCalls)
	assert.Zero(t, gitOps.synthesizeCalls)
	assert.Zero(t, gitOps.worktreeAddCalls)
}
