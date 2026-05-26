package cmd

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

// fetchTrackingGitOps records which ancestry-check method the hook invokes.
// LocalAncestryCheck is the network-free path; FetchOriginAncestryCheck is the
// fetch-then-compare path that performs network I/O and must never be reached
// from the drain-loop pre-claim hook (reliability principle P9).
type fetchTrackingGitOps struct {
	branch             string
	result             agent.PreClaimResult
	localAncestryCalls int
	fetchCalls         int
}

func (g *fetchTrackingGitOps) CurrentBranch(string) (string, error) { return g.branch, nil }

func (g *fetchTrackingGitOps) LocalAncestryCheck(_, _ string) (agent.PreClaimResult, error) {
	g.localAncestryCalls++
	return g.result, nil
}

// FetchOriginAncestryCheck is intentionally not part of preClaimGitOps; it is
// here only so the hook would compile-and-call it if the wiring regressed to
// the fetch variant, letting the assertion below catch the regression.
func (g *fetchTrackingGitOps) FetchOriginAncestryCheck(_, _ string) (agent.PreClaimResult, error) {
	g.fetchCalls++
	return g.result, nil
}

func TestBuildCLIPreClaimHook_DoesNotCallGitFetch(t *testing.T) {
	ops := &fetchTrackingGitOps{
		branch: "main",
		result: agent.PreClaimResult{Action: "unchanged"},
	}
	hook := buildCLIPreClaimHook(t.TempDir(), ops)

	require.NoError(t, hook(context.Background()))
	require.Equal(t, 1, ops.localAncestryCalls, "hook must run the network-free local ancestry check")
	require.Zero(t, ops.fetchCalls, "hook must not invoke the git-fetch ancestry variant")
}

func TestCLIServerPreClaimHook_ErrorPolicyParity(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "staged main worktree",
			err:  errors.New("landing worktree has staged changes after waiting 2s:\nM\t.ddx/beads.jsonl"),
		},
		{
			name: "detached main worktree",
			err:  errors.New(`landing worktree is detached; expected branch "main"`),
		},
		{
			name: "branch mismatch",
			err:  errors.New(`landing branch mismatch: on "feature", want "main"`),
		},
		{
			name: "fetch origin failure",
			err:  errors.New("git fetch origin main: fatal: unable to access origin: exit status 128"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := buildCLIPreClaimHook(t.TempDir(), &preClaimHookGitOps{
				branch: "main",
				err:    tt.err,
			})

			err := hook(context.Background())
			if agent.IsIgnorableFetchOriginError(tt.err) {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.err)
		})
	}
}
