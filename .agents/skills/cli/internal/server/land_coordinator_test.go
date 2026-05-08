package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLandingGitOps is an in-memory stub of agent.LandingGitOps that returns
// canned outcomes based on testOutcomes. It implements the full interface so
// the compiler enforces completeness; only the methods Land() actually calls
// are given meaningful behaviour.
//
// Per concerns.md: stubs of your own domain interfaces are fine in unit tests
// when you're testing the coordinator's own counter logic in isolation.
type fakeLandingGitOps struct {
	// outcomes controls what Land() returns for each call (indexed by call order).
	// Each entry is one of: "landed", "preserved", "push_failed", "error".
	outcomes []string
	callIdx  int
}

func (f *fakeLandingGitOps) HasRemote(_, _ string) bool               { return false }
func (f *fakeLandingGitOps) FetchBranch(_, _, _ string) error         { return nil }
func (f *fakeLandingGitOps) SyncWorkTreeToHead(_, _ string) error     { return nil }
func (f *fakeLandingGitOps) RemoveWorktree(_, _ string) error         { return nil }
func (f *fakeLandingGitOps) PushFFOnly(_, _, _, _ string) error       { return nil }
func (f *fakeLandingGitOps) CountCommits(_, _, _ string) int          { return 1 }
func (f *fakeLandingGitOps) StageDir(_, _ string) error               { return nil }
func (f *fakeLandingGitOps) CommitStaged(_, _ string) (string, error) { return "", nil }

func (f *fakeLandingGitOps) CurrentBranch(_ string) (string, error) {
	return "main", nil
}

func (f *fakeLandingGitOps) ResolveRef(_, ref string) (string, error) {
	// For simplicity always claim the target ref is at "base000" so fast-path
	// is taken unless we want a merge. Since we drive outcomes by intercepting
	// UpdateRefTo, returning "base000" matches the base we send in LandRequest.
	return "base000", nil
}

func (f *fakeLandingGitOps) UpdateRefTo(_, ref, sha, _ string) error {
	// If the ref is a preserve ref (starts with "refs/ddx/"), we allow it.
	// Otherwise we allow the ff update.
	return nil
}

func (f *fakeLandingGitOps) AddWorktree(_, _, _ string) error   { return nil }
func (f *fakeLandingGitOps) HeadRevAt(_ string) (string, error) { return "newTip123", nil }

func (f *fakeLandingGitOps) MergeInto(_, _, _ string) error {
	// Decide based on current pending outcome whether the merge should fail.
	idx := f.callIdx
	if idx < len(f.outcomes) && f.outcomes[idx] == "preserved" {
		return assert.AnError // triggers preserved path
	}
	return nil
}

func (f *fakeLandingGitOps) FetchOriginAncestryCheck(_, _ string) (agent.PreClaimResult, error) {
	return agent.PreClaimResult{Action: "no-origin"}, nil
}

// outcomeGitOps drives Land() outcomes deterministically from a test script.
//
// Land() takes the merge path only when currentTip != req.BaseRev, so for
// "preserved" outcomes we return a different currentTip from ResolveRef and
// make MergeInto return an error.
type outcomeGitOps struct {
	outcomes []string
	callIdx  int
}

func (o *outcomeGitOps) HasRemote(_, _ string) bool             { return false }
func (o *outcomeGitOps) FetchBranch(_, _, _ string) error       { return nil }
func (o *outcomeGitOps) SyncWorkTreeToHead(_, _ string) error   { return nil }
func (o *outcomeGitOps) RemoveWorktree(_, _ string) error       { return nil }
func (o *outcomeGitOps) CountCommits(_, _, _ string) int        { return 2 }
func (o *outcomeGitOps) CurrentBranch(_ string) (string, error) { return "main", nil }
func (o *outcomeGitOps) AddWorktree(_, _, _ string) error       { return nil }
func (o *outcomeGitOps) HeadRevAt(_ string) (string, error)     { return "mergedTip", nil }
func (o *outcomeGitOps) FetchOriginAncestryCheck(_, _ string) (agent.PreClaimResult, error) {
	return agent.PreClaimResult{Action: "no-origin"}, nil
}
func (o *outcomeGitOps) PushFFOnly(_, _, _, _ string) error       { return nil }
func (o *outcomeGitOps) StageDir(_, _ string) error               { return nil }
func (o *outcomeGitOps) CommitStaged(_, _ string) (string, error) { return "", nil }

func (o *outcomeGitOps) ResolveRef(_, ref string) (string, error) {
	// For "preserved" and merge-path "landed" we return a tip that differs
	// from the base so Land() takes the merge code path.
	idx := o.callIdx
	if idx < len(o.outcomes) {
		switch o.outcomes[idx] {
		case "preserved", "landed_merge":
			return "advancedTip", nil // != "base000" → merge path
		}
	}
	return "base000", nil // fast-forward path
}

func (o *outcomeGitOps) UpdateRefTo(_, _, _, _ string) error { return nil }

func (o *outcomeGitOps) MergeInto(_, _, _ string) error {
	idx := o.callIdx
	if idx < len(o.outcomes) && o.outcomes[idx] == "preserved" {
		return assert.AnError
	}
	return nil
}

// submit sends one LandRequest to the coordinator using the given base so
// the gitOps stub can decide the fast vs merge path.
func submitOne(t *testing.T, c *LandCoordinator, ops *outcomeGitOps, outcome string, base string) {
	t.Helper()
	ops.callIdx = len(ops.outcomes)
	ops.outcomes = append(ops.outcomes, outcome)
	_, err := c.Submit(agent.LandRequest{
		WorktreeDir: t.TempDir(),
		BaseRev:     base,
		ResultRev:   "result" + base,
		BeadID:      "ddx-test",
		AttemptID:   "atm-test",
	})
	if outcome != "error" {
		require.NoError(t, err)
	}
}

// TestLandCoordinatorMetrics submits 10 simulated submissions
// (3 landed, 2 preserved, 1 landed, 4 landed) and asserts the counters
// reflect the correct outcome counts.
func TestLandCoordinatorMetrics(t *testing.T) {
	ops := &outcomeGitOps{}

	c := newLandCoordinator(t.TempDir(), ops)
	defer c.Stop()

	// 3 landed (fast-forward path)
	for i := 0; i < 3; i++ {
		submitOne(t, c, ops, "landed", "base000")
	}
	// 2 preserved (merge path with conflict)
	submitOne(t, c, ops, "preserved", "base000")
	submitOne(t, c, ops, "preserved", "base000")
	// 1 landed
	submitOne(t, c, ops, "landed", "base000")
	// 4 more landed
	for i := 0; i < 4; i++ {
		submitOne(t, c, ops, "landed", "base000")
	}

	m := c.Metrics()

	// 10 total: 8 landed, 2 preserved
	assert.Equal(t, int64(8), m.Landed, "landed count")
	assert.Equal(t, int64(2), m.Preserved, "preserved count")
	assert.Equal(t, int64(0), m.Failed, "failed count")
	assert.Equal(t, int64(0), m.PushFailed, "push_failed count")

	total := m.Landed + m.Preserved + m.Failed
	assert.Equal(t, int64(10), total, "total submissions")

	// Preserved ratio: 2/10
	assert.InDelta(t, float64(2)/float64(10), m.PreservedRatio, 0.001)

	// Last 10 submissions should have exactly 10 entries (we submitted exactly 10)
	assert.Len(t, m.LastSubmissions, 10)
	// All entries have a valid outcome label
	for _, s := range m.LastSubmissions {
		assert.Contains(t, []string{"landed", "preserved", "failed", "push_failed"}, s.Outcome)
	}
	// CommitCount is non-zero for preserved entries (CountCommits returns 2)
	preservedEntries := 0
	for _, s := range m.LastSubmissions {
		if s.Outcome == "preserved" {
			preservedEntries++
			assert.Equal(t, 2, s.CommitCount, "preserved entry should have commit count from CountCommits")
		}
	}
	assert.Equal(t, 2, preservedEntries, "exactly 2 preserved entries in last submissions")
}

// TestLandCoordinatorMetricsRegistry verifies the registry's AllMetrics and
// MetricsFor helpers return the right data.
func TestLandCoordinatorMetricsRegistry(t *testing.T) {
	reg := newCoordinatorRegistry()
	ops := &outcomeGitOps{}
	reg.gitOpsOverride = ops

	root1 := t.TempDir()
	root2 := t.TempDir()

	c1 := reg.Get(root1)
	defer reg.StopAll()

	// No submissions yet for root1, root2 not created
	m1 := reg.MetricsFor(root1)
	require.NotNil(t, m1)
	assert.Equal(t, int64(0), m1.Landed)

	m2 := reg.MetricsFor(root2)
	assert.Nil(t, m2, "coordinator for root2 not created yet")

	// Submit one landed to root1 coordinator
	ops.callIdx = 0
	ops.outcomes = []string{"landed"}
	_, err := c1.Submit(agent.LandRequest{
		WorktreeDir: root1,
		BaseRev:     "base000",
		ResultRev:   "result001",
		BeadID:      "ddx-r",
		AttemptID:   "atm-r",
	})
	require.NoError(t, err)

	all := reg.AllMetrics()
	require.Len(t, all, 1)
	assert.Equal(t, root1, all[0].ProjectRoot)
	assert.Equal(t, int64(1), all[0].Metrics.Landed)
}

// ---------------------------------------------------------------------------
// Integration test — real git repo
// ---------------------------------------------------------------------------

// landIntegRepo is a minimal git repo fixture for land coordinator integration
// tests. Uses real git commands (no mocks).
type landIntegRepo struct {
	t   *testing.T
	dir string
}

func newLandIntegRepo(t *testing.T) *landIntegRepo {
	t.Helper()
	dir := t.TempDir()
	r := &landIntegRepo{t: t, dir: dir}
	r.git("init", "-b", "main")
	r.git("config", "user.name", "Test")
	r.git("config", "user.email", "test@land.local")
	r.writeFile("README.md", "# test\n")
	r.git("add", "-A")
	r.git("commit", "-m", "init")
	return r
}

func (r *landIntegRepo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.dir}, args...)...)
	// Scrub parent GIT_* env so test git calls don't inherit lefthook state.
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	require.NoError(r.t, err, "git %s: %s", strings.Join(args, " "), string(out))
	return strings.TrimSpace(string(out))
}

func (r *landIntegRepo) writeFile(path, content string) {
	r.t.Helper()
	full := filepath.Join(r.dir, path)
	require.NoError(r.t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(r.t, os.WriteFile(full, []byte(content), 0o644))
}

func (r *landIntegRepo) headSHA() string {
	r.t.Helper()
	return r.git("rev-parse", "HEAD")
}

// commitOn creates a commit in a throwaway worktree at baseSHA and returns the
// new commit SHA. The commit is pinned via a temp ref so it survives worktree
// cleanup and is submittable to the land coordinator.
func (r *landIntegRepo) commitOn(baseSHA, file, content, msg string) string {
	r.t.Helper()
	wt, err := os.MkdirTemp("", "land-integ-wt-*")
	require.NoError(r.t, err)
	_ = os.RemoveAll(wt) // git worktree add needs the dir absent
	r.git("worktree", "add", "--detach", wt, baseSHA)
	defer func() {
		r.git("worktree", "remove", "--force", wt)
		_ = os.RemoveAll(wt)
	}()

	require.NoError(r.t, os.WriteFile(filepath.Join(wt, file), []byte(content), 0o644))
	gitWt := func(args ...string) {
		r.t.Helper()
		cmd := exec.Command("git", append([]string{"-C", wt}, args...)...)
		env := make([]string, 0, len(os.Environ()))
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(kv, "GIT_") {
				env = append(env, kv)
			}
		}
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		require.NoError(r.t, err, "git %s: %s", strings.Join(args, " "), string(out))
	}
	gitWt("add", "-A")
	gitWt("-c", "user.name=Test", "-c", "user.email=test@land.local", "commit", "-m", msg)

	cmd := exec.Command("git", "-C", wt, "rev-parse", "HEAD")
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	out, err := cmd.Output()
	require.NoError(r.t, err)
	sha := strings.TrimSpace(string(out))

	// Pin so the SHA survives worktree removal.
	r.git("update-ref", fmt.Sprintf("refs/ddx/test-pins/%s", sha[:12]), sha)
	return sha
}

// TestLandCoordinatorIntegration wires a real LandCoordinator to a real temp
// git repo. It submits 3 separate LandRequests where 2 are designed to produce
// a merge-conflict (preserved) outcome by advancing the target branch between
// submissions. Asserts that the metrics endpoint reflects at least 1 preserved
// outcome after the submissions.
//
// No mocks are used for git operations — this test uses real git via exec.
func TestLandCoordinatorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	r := newLandIntegRepo(t)

	// Use the real LandingGitOps — no mocks.
	c := NewLocalLandCoordinator(r.dir, agent.RealLandingGitOps{})
	defer c.Stop()

	baseSHA := r.headSHA()

	// Submission 1: lands cleanly (fast-forward from baseSHA).
	result1SHA := r.commitOn(baseSHA, "work1.txt", "work1\n", "feat: work1")
	res1, err := c.Submit(agent.LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      baseSHA,
		ResultRev:    result1SHA,
		BeadID:       "ddx-integ-1",
		AttemptID:    "20260101T000001-a",
		TargetBranch: "main",
	})
	require.NoError(t, err)
	assert.Equal(t, "landed", res1.Status, "submission 1 should land")

	// The target branch has now advanced to result1SHA.
	// Submission 2 was branched off baseSHA (stale), writes to SAME file as
	// submission 3 to guarantee a merge conflict.
	result2SHA := r.commitOn(baseSHA, "conflict.txt", "version-A\n", "feat: conflict-A")

	// Submission 3: advance the target further first so the merge base
	// diverges even more (makes the conflict certain).
	result3SHA := r.commitOn(r.headSHA(), "conflict.txt", "version-B\n", "feat: conflict-B")
	_, err = c.Submit(agent.LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.headSHA(), // current tip after submission 1
		ResultRev:    result3SHA,
		BeadID:       "ddx-integ-3",
		AttemptID:    "20260101T000003-c",
		TargetBranch: "main",
	})
	require.NoError(t, err)
	// submission 3 lands first to advance the target

	// Now submit result2 (branched off baseSHA, writes to same conflict.txt
	// as result3 which is now on main). Merge should conflict.
	res2, err := c.Submit(agent.LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      baseSHA,
		ResultRev:    result2SHA,
		BeadID:       "ddx-integ-2",
		AttemptID:    "20260101T000002-b",
		TargetBranch: "main",
	})
	require.NoError(t, err)
	// This should be preserved (merge conflict) because both result2 and
	// result3 modify conflict.txt from the same base.
	assert.Equal(t, "preserved", res2.Status, "submission 2 should be preserved due to merge conflict")

	// Verify coordinator metrics reflect at least 1 preserved outcome.
	m := c.Metrics()
	assert.GreaterOrEqual(t, m.Preserved, int64(1), "at least 1 preserved outcome in metrics")
	assert.GreaterOrEqual(t, m.Landed, int64(1), "at least 1 landed outcome in metrics")

	// The last submissions slice should contain the preserved entry.
	found := false
	for _, s := range m.LastSubmissions {
		if s.Outcome == "preserved" {
			found = true
			break
		}
	}
	assert.True(t, found, "preserved outcome should appear in last submissions")
}
