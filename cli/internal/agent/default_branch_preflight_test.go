package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDefaultBranchFixture creates a bare origin with the given default branch
// name, seeds it with one commit, clones into a work dir, and returns
// (workDir, originDir). Seeding before clone ensures refs/remotes/origin/HEAD
// is a symbolic ref pointing at the default branch.
func setupDefaultBranchFixture(t *testing.T, defaultBranch string) (workDir, originDir string) {
	t.Helper()

	originDir = t.TempDir()
	runGitInteg(t, originDir, "init", "--bare", "-b", defaultBranch)

	// Seed the remote first so a subsequent clone observes origin/HEAD.
	seedDir := t.TempDir()
	runGitInteg(t, seedDir, "clone", originDir, ".")
	runGitInteg(t, seedDir, "config", "user.email", "test@ddx.test")
	runGitInteg(t, seedDir, "config", "user.name", "DDx Test")
	seed := filepath.Join(seedDir, "seed.txt")
	require.NoError(t, os.WriteFile(seed, []byte("seed\n"), 0o644))
	runGitInteg(t, seedDir, "add", "seed.txt")
	runGitInteg(t, seedDir, "commit", "-m", "chore: initial seed")
	runGitInteg(t, seedDir, "push", "-u", "origin", defaultBranch)

	workDir = t.TempDir()
	runGitInteg(t, workDir, "clone", originDir, ".")
	runGitInteg(t, workDir, "config", "user.email", "test@ddx.test")
	runGitInteg(t, workDir, "config", "user.name", "DDx Test")

	// Fresh non-empty clones observe origin/HEAD -> origin/<defaultBranch>.
	// If the local git is old enough not to set it, pin it explicitly.
	if out, err := runGitIntegOutput(workDir, "symbolic-ref", "refs/remotes/origin/HEAD"); err != nil || out == "" {
		runGitInteg(t, workDir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/"+defaultBranch)
	}
	require.Equal(t, "refs/remotes/origin/"+defaultBranch,
		runGitInteg(t, workDir, "symbolic-ref", "refs/remotes/origin/HEAD"))

	return workDir, originDir
}

// TestDefaultBranchPreflight_MatchingUpstreamPasses proves a current branch
// whose upstream branch name equals origin/HEAD's target reports pass.
func TestDefaultBranchPreflight_MatchingUpstreamPasses(t *testing.T) {
	workDir, _ := setupDefaultBranchFixture(t, "master")

	res := internalgit.CheckDefaultBranchPreflight(workDir)

	assert.True(t, res.Pass(), "matching upstream must pass: %+v", res)
	assert.False(t, res.HardFail(), "matching upstream must not hard-fail")
	assert.Equal(t, internalgit.DefaultBranchOK, res.Kind)
	assert.Equal(t, "master", res.CurrentBranch)
	assert.Equal(t, "master", res.UpstreamBranch)
	assert.Equal(t, "master", res.DefaultBranch)
	assert.Equal(t, "refs/remotes/origin/master", res.OriginHEADRef)
	assert.Contains(t, res.Message, "origin/master")
}

// TestDefaultBranchPreflight_MismatchedUpstreamFailsWithBothRefs proves
// branch.master.merge=refs/heads/main with origin/HEAD -> refs/remotes/origin/master
// reports a hard failure naming both refs.
func TestDefaultBranchPreflight_MismatchedUpstreamFailsWithBothRefs(t *testing.T) {
	workDir, _ := setupDefaultBranchFixture(t, "master")

	// Incident-shaped misconfiguration: local branch is master, but its
	// configured merge target is main while origin/HEAD still points at master.
	runGitInteg(t, workDir, "config", "branch.master.merge", "refs/heads/main")
	runGitInteg(t, workDir, "config", "branch.master.remote", "origin")

	require.Equal(t, "refs/remotes/origin/master",
		runGitInteg(t, workDir, "symbolic-ref", "refs/remotes/origin/HEAD"))
	require.Equal(t, "refs/heads/main",
		runGitInteg(t, workDir, "config", "--get", "branch.master.merge"))

	res := internalgit.CheckDefaultBranchPreflight(workDir)

	assert.True(t, res.HardFail(), "mismatched upstream must hard-fail: %+v", res)
	assert.False(t, res.Pass())
	assert.Equal(t, internalgit.DefaultBranchMismatch, res.Kind)
	assert.Equal(t, "master", res.CurrentBranch)
	assert.Equal(t, "main", res.UpstreamBranch)
	assert.Equal(t, "master", res.DefaultBranch)

	// Must name both refs so the operator can see the exact misconfiguration.
	assert.Contains(t, res.Message, "refs/heads/main", "message must name the configured merge ref")
	assert.Contains(t, res.Message, "refs/remotes/origin/master", "message must name origin/HEAD")
	assert.Equal(t, "refs/heads/main", res.UpstreamRef)
	assert.Equal(t, "refs/remotes/origin/master", res.OriginHEADRef)
}

// TestDefaultBranchPreflight_DetachedHEADFailsWithMessage proves detached HEAD
// reports a hard failure with a remediation-oriented message.
func TestDefaultBranchPreflight_DetachedHEADFailsWithMessage(t *testing.T) {
	workDir, _ := setupDefaultBranchFixture(t, "main")

	sha := runGitInteg(t, workDir, "rev-parse", "HEAD")
	runGitInteg(t, workDir, "checkout", "--detach", sha)

	_, err := runGitIntegOutput(workDir, "symbolic-ref", "-q", "HEAD")
	require.Error(t, err, "fixture must be in detached HEAD")

	res := internalgit.CheckDefaultBranchPreflight(workDir)

	assert.True(t, res.HardFail(), "detached HEAD must hard-fail: %+v", res)
	assert.Equal(t, internalgit.DefaultBranchDetachedHEAD, res.Kind)
	assert.Contains(t, res.Message, "detached", "message must mention detached HEAD")
	// Remediation-oriented: tell the operator how to recover.
	assert.True(t,
		strings.Contains(res.Message, "git switch") ||
			strings.Contains(res.Message, "check out") ||
			strings.Contains(res.Message, "checkout"),
		"message must include remediation guidance, got: %s", res.Message)
}

// TestDefaultBranchPreflight_BehindUpstreamWarns proves a local branch behind
// its configured upstream reports a warning without fetching.
func TestDefaultBranchPreflight_BehindUpstreamWarns(t *testing.T) {
	workDir, originDir := setupDefaultBranchFixture(t, "main")

	// Advance origin via a second clone, then fetch into workDir so the cached
	// remote-tracking ref is ahead of local. After that, make origin unreachable
	// so any accidental fetch would fail — the diagnostic must still report
	// "behind" from already-observed refs only (P9: no fetch).
	secondDir := t.TempDir()
	runGitInteg(t, secondDir, "clone", originDir, ".")
	runGitInteg(t, secondDir, "config", "user.email", "test@ddx.test")
	runGitInteg(t, secondDir, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(secondDir, "remote.txt"), []byte("remote\n"), 0o644))
	runGitInteg(t, secondDir, "add", "remote.txt")
	runGitInteg(t, secondDir, "commit", "-m", "feat: remote advance")
	runGitInteg(t, secondDir, "push", "origin", "main")
	originTip := runGitInteg(t, secondDir, "rev-parse", "HEAD")

	// Refresh only the remote-tracking ref (operator-side sync), then break origin.
	runGitInteg(t, workDir, "fetch", "origin", "main")
	localSHA := runGitInteg(t, workDir, "rev-parse", "HEAD")
	require.NotEqual(t, originTip, localSHA, "local must be behind origin tip")
	require.Equal(t, originTip, runGitInteg(t, workDir, "rev-parse", "refs/remotes/origin/main"))

	// Make origin unreachable: any fetch would fail.
	runGitInteg(t, workDir, "remote", "set-url", "origin", "file://"+filepath.Join(t.TempDir(), "gone"))
	require.NoError(t, os.RemoveAll(originDir))

	res := internalgit.CheckDefaultBranchPreflight(workDir)

	assert.True(t, res.Warn(), "behind upstream must warn: %+v", res)
	assert.False(t, res.HardFail(), "behind upstream must not hard-fail when branch names match")
	assert.Equal(t, internalgit.DefaultBranchBehind, res.Kind)
	assert.Equal(t, "main", res.CurrentBranch)
	assert.Equal(t, "main", res.UpstreamBranch)
	assert.Equal(t, "main", res.DefaultBranch)
	assert.Equal(t, localSHA, res.LocalSHA)
	assert.Equal(t, originTip, res.UpstreamSHA)
	assert.Contains(t, res.Message, "behind")

	// Local branch tip must be unchanged (no fetch/pull side effects).
	assert.Equal(t, localSHA, runGitInteg(t, workDir, "rev-parse", "HEAD"))
}
