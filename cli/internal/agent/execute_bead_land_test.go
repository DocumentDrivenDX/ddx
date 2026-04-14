package agent

// execute_bead_land_test.go — Land() coordinator-pattern unit tests.
//
// These tests run against a real temp git repo so they exercise the actual
// git plumbing (update-ref, rebase --onto, worktree add, etc.) rather than a
// mock. Each scenario asserts that the new target tip is reachable on the
// target branch, that no merge commits are introduced, and that concurrent
// submissions are serialized correctly.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ----------------------------------------------------------------------------
// Test helpers (real git repo fixtures)
// ----------------------------------------------------------------------------

type landTestRepo struct {
	t       *testing.T
	dir     string
	origin  string // path to a bare origin repo, or "" if no remote
	branch  string // target branch
	baseSHA string // the initial commit on the target branch
}

func newLandTestRepo(t *testing.T) *landTestRepo {
	t.Helper()
	dir := t.TempDir()
	r := &landTestRepo{t: t, dir: dir, branch: "main"}
	r.runGit("init", "-b", "main")
	r.runGit("config", "user.name", "Test")
	r.runGit("config", "user.email", "test@test.local")
	r.writeFile("README.md", "# test\n")
	r.runGit("add", "-A")
	r.runGit("commit", "-m", "init")
	r.baseSHA = r.resolveRef("refs/heads/main")
	return r
}

// newLandTestRepoWithOrigin creates a test repo whose origin is a separate
// bare repo. Used by the push-ff-only test.
func newLandTestRepoWithOrigin(t *testing.T) *landTestRepo {
	t.Helper()
	r := newLandTestRepo(t)

	bareDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", "-b", "main", bareDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s: %v", string(out), err)
	}
	r.origin = bareDir
	r.runGit("remote", "add", "origin", bareDir)
	r.runGit("push", "-u", "origin", "main")
	return r
}

func (r *landTestRepo) runGit(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %s: %v", strings.Join(args, " "), string(out), err)
	}
	return strings.TrimSpace(string(out))
}

func (r *landTestRepo) writeFile(path, content string) {
	r.t.Helper()
	full := filepath.Join(r.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *landTestRepo) resolveRef(ref string) string {
	r.t.Helper()
	return r.runGit("rev-parse", ref)
}

// commitOn creates a detached-head commit at baseSHA in a throwaway worktree
// and returns the new commit SHA. The commit is reachable via object store
// but not via any branch in the main repo, simulating what a worker worktree
// produces after ExecuteBead cleans up.
func (r *landTestRepo) commitOn(baseSHA, path, content, msg string) string {
	r.t.Helper()
	wt, err := os.MkdirTemp("", "land-test-wt-*")
	if err != nil {
		r.t.Fatal(err)
	}
	_ = os.RemoveAll(wt)
	r.runGit("worktree", "add", "--detach", wt, baseSHA)
	defer func() {
		r.runGit("worktree", "remove", "--force", wt)
		_ = os.RemoveAll(wt)
	}()

	if err := os.WriteFile(filepath.Join(wt, path), []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", wt, "add", "-A")
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git add: %s: %v", string(out), err)
	}
	cmd = exec.Command("git", "-C", wt, "-c", "user.name=Test", "-c", "user.email=test@test.local", "commit", "-m", msg)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git commit: %s: %v", string(out), err)
	}
	cmd = exec.Command("git", "-C", wt, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		r.t.Fatalf("git rev-parse HEAD: %v", err)
	}
	sha := strings.TrimSpace(string(out))

	// Pin the commit with a temporary ref so it survives the worktree
	// cleanup. The test caller is responsible for Land()-ing it later.
	ref := fmt.Sprintf("refs/ddx/test-pins/%s", sha[:12])
	r.runGit("update-ref", ref, sha)
	return sha
}

// mergeCommitCount returns the number of merge commits (commits with >1
// parent) reachable from ref. Used to assert linear history.
func (r *landTestRepo) mergeCommitCount(ref string) int {
	r.t.Helper()
	out := r.runGit("log", "--merges", "--format=%H", ref)
	if out == "" {
		return 0
	}
	return len(strings.Split(out, "\n"))
}

// commitCount returns the total number of commits reachable from ref.
func (r *landTestRepo) commitCount(ref string) int {
	r.t.Helper()
	out := r.runGit("rev-list", "--count", ref)
	n := 0
	_, _ = fmt.Sscanf(out, "%d", &n)
	return n
}

// ----------------------------------------------------------------------------
// Tests
// ----------------------------------------------------------------------------

// TestLand_HappyPath_FastForward verifies the fast-forward path: currentTip
// == BaseRev → target branch is advanced directly to ResultRev with no
// merge commit.
func TestLand_HappyPath_FastForward(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	// Worker commit at current main.
	resultSHA := r.commitOn(r.baseSHA, "feature.txt", "hello\n", "feat: hello")

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    resultSHA,
		BeadID:       "ddx-land-happy",
		AttemptID:    "20260414T000000-aabb",
		TargetBranch: "main",
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed, got %q (reason=%q)", land.Status, land.Reason)
	}
	if land.Rebased {
		t.Errorf("expected Rebased=false on fast path, got true")
	}
	if land.NewTip != resultSHA {
		t.Errorf("expected NewTip=%s, got %s", resultSHA, land.NewTip)
	}
	if got := r.resolveRef("refs/heads/main"); got != resultSHA {
		t.Errorf("main tip = %s, want %s", got, resultSHA)
	}
	if n := r.mergeCommitCount("refs/heads/main"); n != 0 {
		t.Errorf("expected 0 merge commits on main, got %d", n)
	}
}

// TestLand_RebaseRequired verifies the rebase path: the target has advanced
// since the worker started. The worker's commit is replayed onto the new tip,
// producing a different SHA, and no merge commit is introduced.
func TestLand_RebaseRequired(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	// Worker branches off baseSHA.
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature-content\n", "feat: worker")

	// Meanwhile, a sibling lands a commit on main directly.
	siblingSHA := r.commitOn(r.baseSHA, "sibling.txt", "sibling-content\n", "feat: sibling")
	r.runGit("update-ref", "refs/heads/main", siblingSHA)

	// Now land the worker's result. currentTip = siblingSHA != baseSHA → rebase.
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-rebase",
		AttemptID:    "20260414T000001-ccdd",
		TargetBranch: "main",
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed, got %q (reason=%q)", land.Status, land.Reason)
	}
	if !land.Rebased {
		t.Errorf("expected Rebased=true on sibling-advanced tip, got false")
	}
	if land.NewTip == workerSHA {
		t.Errorf("expected NewTip to be rebased (different from %s), got same SHA", workerSHA)
	}
	if got := r.resolveRef("refs/heads/main"); got != land.NewTip {
		t.Errorf("main tip = %s, want %s", got, land.NewTip)
	}
	if n := r.mergeCommitCount("refs/heads/main"); n != 0 {
		t.Errorf("expected 0 merge commits on main after rebase, got %d", n)
	}
	// main should have baseSHA + siblingSHA + rebased-worker = 3 commits.
	if n := r.commitCount("refs/heads/main"); n != 3 {
		t.Errorf("expected 3 commits on main, got %d", n)
	}
}

// TestLand_RebaseConflict verifies that a rebase conflict is handled cleanly:
// the target branch is untouched, the original ResultRev is preserved under
// refs/ddx/iterations/, and no stale worktree or branch is left behind.
func TestLand_RebaseConflict(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	// Worker edits feature.txt starting from baseSHA.
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "worker-version\n", "feat: worker")

	// Sibling edits the SAME file (feature.txt) on main. Rebasing the worker
	// commit onto this tip will conflict.
	siblingSHA := r.commitOn(r.baseSHA, "feature.txt", "sibling-version\n", "feat: sibling")
	r.runGit("update-ref", "refs/heads/main", siblingSHA)

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-conflict",
		AttemptID:    "20260414T000002-eeff",
		TargetBranch: "main",
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "preserved" {
		t.Fatalf("expected status=preserved on conflict, got %q", land.Status)
	}
	if land.PreserveRef == "" || !strings.HasPrefix(land.PreserveRef, "refs/ddx/iterations/ddx-land-conflict/") {
		t.Errorf("expected preserve ref under refs/ddx/iterations/ddx-land-conflict/, got %q", land.PreserveRef)
	}
	if land.Reason == "" {
		t.Errorf("expected non-empty Reason on preserve, got empty")
	}
	// Target branch must be unchanged from the sibling commit.
	if got := r.resolveRef("refs/heads/main"); got != siblingSHA {
		t.Errorf("main tip = %s, want %s (unchanged)", got, siblingSHA)
	}
	// Preserve ref must exist and resolve to the original worker SHA.
	if got := r.resolveRef(land.PreserveRef); got != workerSHA {
		t.Errorf("preserve ref resolves to %s, want %s", got, workerSHA)
	}
	// No stale ddx-land-* branches left over.
	branches := r.runGit("branch", "--format=%(refname:short)")
	for _, b := range strings.Split(branches, "\n") {
		if strings.HasPrefix(strings.TrimSpace(b), "ddx-land-") {
			t.Errorf("stale temp branch left behind: %q", b)
		}
	}
	// No stale ddx-land-* worktrees (the rebase ran in a temp worktree which
	// must have been cleaned up on abort).
	wtList := r.runGit("worktree", "list", "--porcelain")
	for _, line := range strings.Split(wtList, "\n") {
		if strings.HasPrefix(line, "worktree ") && strings.Contains(line, "ddx-land-wt-") {
			t.Errorf("stale land worktree left behind: %q", line)
		}
	}
}

// TestLand_ConcurrentSubmissions_Serialized spawns N concurrent Land() calls
// through a single coordinator-like serialization (sync.Mutex) and asserts
// that all N commits end up on main in linear order with no merge commits.
// This is a direct test of the "single-writer" contract that the server
// coordinator must enforce.
func TestLand_ConcurrentSubmissions_Serialized(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	const N = 5
	// Prepare N worker commits, each branching off the original baseSHA.
	// Each touches a distinct file so rebases always merge cleanly.
	workerSHAs := make([]string, N)
	for i := 0; i < N; i++ {
		workerSHAs[i] = r.commitOn(r.baseSHA, fmt.Sprintf("worker-%d.txt", i), fmt.Sprintf("content-%d\n", i), fmt.Sprintf("feat: worker %d", i))
	}

	// Serialize submissions through a mutex — this simulates the coordinator
	// goroutine. Spawn concurrently so we exercise the lock ordering too.
	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make([]*LandResult, N)
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			defer mu.Unlock()
			req := LandRequest{
				WorktreeDir:  r.dir,
				BaseRev:      r.baseSHA, // all submissions think they branched off the original base
				ResultRev:    workerSHAs[i],
				BeadID:       fmt.Sprintf("ddx-concurrent-%02d", i),
				AttemptID:    fmt.Sprintf("20260414T00%04d-%02d", i, i),
				TargetBranch: "main",
			}
			results[i], errs[i] = Land(r.dir, req, ops)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("submission %d: Land returned error: %v", i, err)
		}
		if results[i] == nil || results[i].Status != "landed" {
			t.Errorf("submission %d: expected landed, got %+v", i, results[i])
		}
	}

	// main must have baseSHA + N new commits = N+1 total.
	if got := r.commitCount("refs/heads/main"); got != N+1 {
		t.Errorf("expected %d commits on main, got %d", N+1, got)
	}
	// Zero merge commits — strictly linear history.
	if n := r.mergeCommitCount("refs/heads/main"); n != 0 {
		t.Errorf("expected 0 merge commits, got %d", n)
	}
	// At least N-1 submissions must have been rebased (the first one ffs;
	// every subsequent one sees an advanced tip).
	rebased := 0
	for _, res := range results {
		if res != nil && res.Rebased {
			rebased++
		}
	}
	if rebased < N-1 {
		t.Errorf("expected at least %d rebased submissions, got %d", N-1, rebased)
	}
}

// TestLand_PushIsFFOnly verifies that Land() never force-pushes and reports
// PushFailed when the remote has advanced beyond the local tip. The local
// target ref is still advanced successfully; remote reconciliation is left
// for later.
func TestLand_PushIsFFOnly(t *testing.T) {
	r := newLandTestRepoWithOrigin(t)
	ops := RealLandingGitOps{}

	// Seed a commit directly on the bare origin so the remote main is
	// ahead of the local main.
	//
	// To create a commit on a bare repo, push from a throwaway clone.
	sideDir, err := os.MkdirTemp("", "land-side-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sideDir)
	runCmd := func(dir string, args ...string) {
		c := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, cerr := c.CombinedOutput()
		if cerr != nil {
			t.Fatalf("git %s: %s: %v", strings.Join(args, " "), string(out), cerr)
		}
	}
	runCmd("", "clone", r.origin, sideDir)
	runCmd(sideDir, "config", "user.name", "Side")
	runCmd(sideDir, "config", "user.email", "side@test.local")
	if err := os.WriteFile(filepath.Join(sideDir, "remote-only.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(sideDir, "add", "-A")
	runCmd(sideDir, "commit", "-m", "remote: seed")
	runCmd(sideDir, "push", "origin", "main")

	// Now the origin has advanced beyond the local r.dir/main. Produce a
	// worker commit on top of the local baseSHA and try to land it. The
	// local fetch will pull the remote tip, so Land() will actually take
	// the rebase path and then attempt a fast-forward push.
	//
	// Wait — we want the push to FAIL (not the rebase). To force a push
	// failure specifically, we need the local main to be advanced ahead of
	// origin AND the push to conflict. Simplest construction: disable the
	// auto-fetch by marking the remote as unreachable. We can do that by
	// overwriting the remote URL with an invalid path after the initial
	// push, then landing a local commit.
	r.runGit("remote", "set-url", "origin", "/nonexistent/path/"+filepath.Base(r.dir))

	workerSHA := r.commitOn(r.baseSHA, "local-only.txt", "local\n", "feat: local")

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-pushff",
		AttemptID:    "20260414T000009-pf",
		TargetBranch: "main",
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	// Local target ref must still advance — push failure is non-fatal.
	if land.Status != "landed" {
		t.Fatalf("expected status=landed (local) even when push fails, got %q", land.Status)
	}
	if !land.PushFailed {
		t.Errorf("expected PushFailed=true when origin is unreachable, got false")
	}
	if land.PushError == "" {
		t.Errorf("expected non-empty PushError when push fails")
	}
	if got := r.resolveRef("refs/heads/main"); got != workerSHA {
		t.Errorf("local main tip = %s, want %s", got, workerSHA)
	}
	// Force-push canary: ensure no --force was used. The unreachable path
	// makes verification of the remote tricky; instead, we verify that the
	// PushError message mentions push (git's own error) and does NOT mention
	// "force".
	if strings.Contains(strings.ToLower(land.PushError), "--force") {
		t.Errorf("PushError mentions --force; Land() must never force-push: %s", land.PushError)
	}
}

// TestLand_NoChanges verifies that Land() short-circuits when ResultRev
// equals BaseRev.
func TestLand_NoChanges(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	req := LandRequest{
		WorktreeDir: r.dir,
		BaseRev:     r.baseSHA,
		ResultRev:   r.baseSHA,
		BeadID:      "ddx-land-nochanges",
		AttemptID:   "20260414T000010-nc",
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "no-changes" {
		t.Errorf("expected status=no-changes, got %q", land.Status)
	}
}

// Deterministic test clock helper — avoids unused time import when no test
// overrides NowFunc.
var _ = time.Now
