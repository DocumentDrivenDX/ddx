package server

import (
	"context"
	"errors"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/require"
)

type preClaimHookGitOps struct {
	branch string
	result agent.PreClaimResult
	err    error
}

var _ agent.LandingGitOps = (*preClaimHookGitOps)(nil)

func (g *preClaimHookGitOps) HasRemote(_, _ string) bool                    { return false }
func (g *preClaimHookGitOps) CurrentBranch(_ string) (string, error)        { return g.branch, nil }
func (g *preClaimHookGitOps) FetchBranch(_, _, _ string) error              { return nil }
func (g *preClaimHookGitOps) ResolveRef(_, ref string) (string, error)      { return ref, nil }
func (g *preClaimHookGitOps) UpdateRefTo(_, _, _, _ string) error           { return nil }
func (g *preClaimHookGitOps) SyncWorkTreeToHead(_, _ string) error          { return nil }
func (g *preClaimHookGitOps) AddWorktree(_, _, _ string) error              { return nil }
func (g *preClaimHookGitOps) AddBranchWorktree(_, _, _ string) error        { return nil }
func (g *preClaimHookGitOps) RemoveWorktree(_, _ string) error              { return nil }
func (g *preClaimHookGitOps) MergeInto(_, _, _ string) error                { return nil }
func (g *preClaimHookGitOps) HeadRevAt(_ string) (string, error)            { return "HEAD", nil }
func (g *preClaimHookGitOps) PushFFOnly(_, _, _, _ string) error            { return nil }
func (g *preClaimHookGitOps) CountCommits(_, _, _ string) int               { return 0 }
func (g *preClaimHookGitOps) StageDir(_, _ string) error                    { return nil }
func (g *preClaimHookGitOps) CommitStaged(_, _ string) (string, error)      { return "", nil }
func (g *preClaimHookGitOps) DiffNumstat(_, _, _ string) (string, error)    { return "", nil }
func (g *preClaimHookGitOps) DiffNameOnly(_, _, _ string) ([]string, error) { return nil, nil }
func (g *preClaimHookGitOps) LocalAncestryCheck(_, _ string) (agent.PreClaimResult, error) {
	return g.result, g.err
}

func TestServerPreClaimHook_PropagatesStagedMainWorktreeError(t *testing.T) {
	stagedErr := errors.New("landing worktree has staged changes after waiting 2s:\nM\t.ddx/beads.jsonl")
	hook := buildPreClaimHook(t.TempDir(), &preClaimHookGitOps{
		branch: "main",
		err:    stagedErr,
	})

	err := hook(context.Background())
	require.ErrorIs(t, err, stagedErr)
}

func TestServerPreClaimHook_IgnoresGitFetchOriginFailure(t *testing.T) {
	hook := buildPreClaimHook(t.TempDir(), &preClaimHookGitOps{
		branch: "main",
		err:    errors.New("git fetch origin main: fatal: unable to access origin: exit status 128"),
	})

	require.NoError(t, hook(context.Background()))
}
