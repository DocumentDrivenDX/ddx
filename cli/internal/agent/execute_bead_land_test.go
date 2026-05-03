package agent

// execute_bead_land_test.go — Land() coordinator-pattern unit tests.
//
// These tests run against a real temp git repo so they exercise the actual
// git plumbing (update-ref, merge --no-ff, worktree add, etc.) rather than a
// mock. Each scenario asserts that the target tip advances correctly and —
// crucially for replay fidelity — that the worker's own commit is never
// rewritten. Its parent always stays BaseRev so replay sees the same inputs
// the worker saw at execution time.

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

func (r *landTestRepo) commitDeleteOn(baseSHA, path, msg string) string {
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

	if err := os.Remove(filepath.Join(wt, path)); err != nil {
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
	ref := fmt.Sprintf("refs/ddx/test-pins/%s", sha[:12])
	r.runGit("update-ref", ref, sha)
	return sha
}

func largeFileLines(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "line-%03d\n", i)
	}
	return b.String()
}

// mergeCommitCount returns the number of merge commits (commits with >1
// parent) reachable from ref. Used to assert merge-commit semantics on the
// merge path.
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

// commitParents returns the parent SHAs of sha.
func (r *landTestRepo) commitParents(sha string) []string {
	r.t.Helper()
	out := r.runGit("rev-list", "--parents", "-n", "1", sha)
	fields := strings.Fields(out)
	if len(fields) <= 1 {
		return nil
	}
	return fields[1:]
}

// shaReachable returns true when sha is reachable from ref via any commit
// path (including through merge commit parents).
func (r *landTestRepo) shaReachable(ref, sha string) bool {
	r.t.Helper()
	cmd := exec.Command("git", "-C", r.dir, "merge-base", "--is-ancestor", sha, ref)
	return cmd.Run() == nil
}

// ----------------------------------------------------------------------------
// Tests
// ----------------------------------------------------------------------------

// TestLand_HappyPath_FastForward verifies the fast-forward path: currentTip
// == BaseRev → target branch is advanced directly to ResultRev with no
// merge commit. The worker's commit becomes the new tip unchanged.
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
	if land.Merged {
		t.Errorf("expected Merged=false on fast path, got true")
	}
	if land.NewTip != resultSHA {
		t.Errorf("expected NewTip=%s, got %s", resultSHA, land.NewTip)
	}
	if got := r.resolveRef("refs/heads/main"); got != resultSHA {
		t.Errorf("main tip = %s, want %s", got, resultSHA)
	}
	if n := r.mergeCommitCount("refs/heads/main"); n != 0 {
		t.Errorf("expected 0 merge commits on main on ff path, got %d", n)
	}
	// Replay fidelity: the worker commit's parent must still be BaseRev.
	parents := r.commitParents(resultSHA)
	if len(parents) != 1 || parents[0] != r.baseSHA {
		t.Errorf("worker commit parent = %v, want [%s]", parents, r.baseSHA)
	}
	// Worktree sync: feature.txt must exist on disk in the main worktree
	// after Land() (bug ddx-eaebaffb regression test — pre-fix, the file
	// was in the index but missing from disk because update-ref bypassed
	// the working tree).
	featurePath := filepath.Join(r.dir, "feature.txt")
	content, readErr := os.ReadFile(featurePath)
	if readErr != nil {
		t.Errorf("feature.txt not materialized in working tree after Land(): %v", readErr)
	} else if string(content) != "hello\n" {
		t.Errorf("feature.txt content = %q, want %q", string(content), "hello\n")
	}
	// git status should show NO phantom deleted/modified entries for feature.txt.
	statusOut := r.runGit("status", "--porcelain", "feature.txt")
	if strings.TrimSpace(statusOut) != "" {
		t.Errorf("git status after Land() shows unexpected entry for feature.txt: %q", statusOut)
	}
}

// TestLand_MergeRequired verifies the merge path: the target has advanced
// since the worker started. Land() creates a merge commit whose parents are
// [currentTip, workerSHA], and critically the worker's own commit is NOT
// rewritten — its parent is still baseSHA so replay sees the original inputs.
func TestLand_MergeRequired(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	// Worker branches off baseSHA.
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature-content\n", "feat: worker")

	// Meanwhile, a sibling lands a commit on main directly.
	siblingSHA := r.commitOn(r.baseSHA, "sibling.txt", "sibling-content\n", "feat: sibling")
	r.runGit("update-ref", "refs/heads/main", siblingSHA)

	// Now land the worker's result. currentTip = siblingSHA != baseSHA → merge.
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-merge",
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
	if !land.Merged {
		t.Errorf("expected Merged=true on sibling-advanced tip, got false")
	}
	if land.NewTip == workerSHA {
		t.Errorf("expected NewTip to be the merge commit (different from worker %s), got same SHA", workerSHA)
	}
	if got := r.resolveRef("refs/heads/main"); got != land.NewTip {
		t.Errorf("main tip = %s, want %s", got, land.NewTip)
	}
	// Exactly one merge commit was created.
	if n := r.mergeCommitCount("refs/heads/main"); n != 1 {
		t.Errorf("expected 1 merge commit on main after merge path, got %d", n)
	}
	// The merge commit's parents must be [siblingSHA, workerSHA] (in that order:
	// `git merge --no-ff` from a worktree at currentTip produces [currentTip, incoming]).
	parents := r.commitParents(land.NewTip)
	if len(parents) != 2 {
		t.Fatalf("merge commit should have 2 parents, got %v", parents)
	}
	if parents[0] != siblingSHA {
		t.Errorf("merge commit parent[0] = %s, want currentTip %s", parents[0], siblingSHA)
	}
	if parents[1] != workerSHA {
		t.Errorf("merge commit parent[1] = %s, want workerSHA %s", parents[1], workerSHA)
	}
	// Replay fidelity: the worker's commit is NOT rewritten — its parent is still baseSHA.
	workerParents := r.commitParents(workerSHA)
	if len(workerParents) != 1 || workerParents[0] != r.baseSHA {
		t.Errorf("worker commit parent = %v, want [%s] (replay fidelity)", workerParents, r.baseSHA)
	}
	// main should have baseSHA + siblingSHA + workerSHA + merge commit = 4 commits.
	if n := r.commitCount("refs/heads/main"); n != 4 {
		t.Errorf("expected 4 commits on main (base+sibling+worker+merge), got %d", n)
	}
}

// TestLand_MergeConflict verifies that a merge conflict is handled cleanly:
// the target branch is untouched, the original ResultRev is preserved under
// refs/ddx/iterations/, and no stale worktree is left behind.
func TestLand_MergeConflict(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	// Worker edits feature.txt starting from baseSHA.
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "worker-version\n", "feat: worker")

	// Sibling edits the SAME file (feature.txt) on main. Merging the worker
	// commit into this tip will conflict.
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
	// No stale ddx-land-wt-* worktrees (the merge ran in a temp worktree
	// which must have been cleaned up on abort).
	wtList := r.runGit("worktree", "list", "--porcelain")
	for _, line := range strings.Split(wtList, "\n") {
		if strings.HasPrefix(line, "worktree ") && strings.Contains(line, "ddx-land-wt-") {
			t.Errorf("stale land worktree left behind: %q", line)
		}
	}
}

func TestLand_LargeDeletionPreservedBeforeFastForward(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	r.writeFile("large.txt", largeFileLines(defaultLargeDeletionLineThreshold+25))
	r.runGit("add", "-A")
	r.runGit("commit", "-m", "test: add large file")
	r.baseSHA = r.resolveRef("refs/heads/main")

	workerSHA := r.commitDeleteOn(r.baseSHA, "large.txt", "feat: remove generated content")
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-large-delete",
		AttemptID:    "20260430T120000-large-delete",
		TargetBranch: "main",
	}

	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "preserved" {
		t.Fatalf("expected status=preserved, got %q", land.Status)
	}
	if !strings.Contains(land.Reason, "large-deletion gate") || !strings.Contains(land.Reason, "large.txt") {
		t.Fatalf("expected large-deletion reason naming file, got %q", land.Reason)
	}
	if got := r.resolveRef("refs/heads/main"); got != r.baseSHA {
		t.Fatalf("main tip = %s, want unchanged base %s", got, r.baseSHA)
	}
	if got := r.resolveRef(land.PreserveRef); got != workerSHA {
		t.Fatalf("preserve ref resolves to %s, want %s", got, workerSHA)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "large.txt")); err != nil {
		t.Fatalf("large.txt should remain in main worktree after preserved land: %v", err)
	}
}

func TestLand_LargeDeletionAcknowledgementAllowsLand(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	r.writeFile("large.txt", largeFileLines(defaultLargeDeletionLineThreshold+25))
	r.runGit("add", "-A")
	r.runGit("commit", "-m", "test: add large file")
	r.baseSHA = r.resolveRef("refs/heads/main")

	workerSHA := r.commitDeleteOn(r.baseSHA, "large.txt", "feat: remove obsolete fixture\n\nintentional large deletion: fixture is generated elsewhere")
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-large-delete-ack",
		AttemptID:    "20260430T120001-large-delete-ack",
		TargetBranch: "main",
	}

	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed, got %q (reason=%q)", land.Status, land.Reason)
	}
	if got := r.resolveRef("refs/heads/main"); got != workerSHA {
		t.Fatalf("main tip = %s, want worker %s", got, workerSHA)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "large.txt")); !os.IsNotExist(err) {
		t.Fatalf("large.txt should be removed from main worktree after acknowledged land, stat err=%v", err)
	}
}

func TestLand_LargeDeletionThresholdOverride(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	r.writeFile("small-large.txt", largeFileLines(25))
	r.runGit("add", "-A")
	r.runGit("commit", "-m", "test: add threshold fixture")
	r.baseSHA = r.resolveRef("refs/heads/main")

	workerSHA := r.commitDeleteOn(r.baseSHA, "small-large.txt", "feat: remove fixture")
	req := LandRequest{
		WorktreeDir:                r.dir,
		BaseRev:                    r.baseSHA,
		ResultRev:                  workerSHA,
		BeadID:                     "ddx-land-large-delete-threshold",
		AttemptID:                  "20260430T120002-large-delete-threshold",
		TargetBranch:               "main",
		LargeDeletionLineThreshold: 10,
	}

	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "preserved" {
		t.Fatalf("expected status=preserved, got %q", land.Status)
	}
	if !strings.Contains(land.Reason, "threshold 10") {
		t.Fatalf("expected custom threshold in reason, got %q", land.Reason)
	}
	if got := r.resolveRef("refs/heads/main"); got != r.baseSHA {
		t.Fatalf("main tip = %s, want unchanged base %s", got, r.baseSHA)
	}
}

func TestLand_InvalidJSONSyntaxPreservedBeforeFastForward(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	workerSHA := r.commitOn(r.baseSHA, "config.json", "{broken-json\n", "feat: write config")
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-json-syntax",
		AttemptID:    "20260430T120100-json-syntax",
		TargetBranch: "main",
	}

	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "preserved" {
		t.Fatalf("expected status=preserved, got %q", land.Status)
	}
	if !strings.Contains(land.Reason, "syntax sanity gate") || !strings.Contains(land.Reason, "config.json") {
		t.Fatalf("expected syntax sanity reason naming config.json, got %q", land.Reason)
	}
	if got := r.resolveRef("refs/heads/main"); got != r.baseSHA {
		t.Fatalf("main tip = %s, want unchanged base %s", got, r.baseSHA)
	}
	if got := r.resolveRef(land.PreserveRef); got != workerSHA {
		t.Fatalf("preserve ref resolves to %s, want %s", got, workerSHA)
	}
}

func TestLand_InvalidGoSyntaxPreservedBeforeFastForward(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	workerSHA := r.commitOn(r.baseSHA, "broken.go", "package main\nfunc broken( {\n", "feat: write go")
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-go-syntax",
		AttemptID:    "20260430T120101-go-syntax",
		TargetBranch: "main",
	}

	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "preserved" {
		t.Fatalf("expected status=preserved, got %q", land.Status)
	}
	if !strings.Contains(land.Reason, "syntax sanity gate") || !strings.Contains(land.Reason, "broken.go") {
		t.Fatalf("expected syntax sanity reason naming broken.go, got %q", land.Reason)
	}
	if got := r.resolveRef("refs/heads/main"); got != r.baseSHA {
		t.Fatalf("main tip = %s, want unchanged base %s", got, r.baseSHA)
	}
}

func TestLand_TruncatedSvelteSyntaxPreservedBeforeFastForward(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	r.writeFile("Page.svelte", "<script lang=\"ts\">\nlet count = 0;\n</script>\n\n<button>{count}</button>\n")
	r.runGit("add", "-A")
	r.runGit("commit", "-m", "test: add svelte page")
	r.baseSHA = r.resolveRef("refs/heads/main")

	workerSHA := r.commitOn(r.baseSHA, "Page.svelte", "import { onMount } from 'svelte';\nlet count = 0;\n", "feat: rewrite page")
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-svelte-syntax",
		AttemptID:    "20260430T120102-svelte-syntax",
		TargetBranch: "main",
	}

	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "preserved" {
		t.Fatalf("expected status=preserved, got %q", land.Status)
	}
	if !strings.Contains(land.Reason, "syntax sanity gate") || !strings.Contains(land.Reason, "Page.svelte") {
		t.Fatalf("expected syntax sanity reason naming Page.svelte, got %q", land.Reason)
	}
	if got := r.resolveRef("refs/heads/main"); got != r.baseSHA {
		t.Fatalf("main tip = %s, want unchanged base %s", got, r.baseSHA)
	}
}

func TestLand_PostLandGateFailureRestoresTargetAndPreserves(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature\n", "feat: feature")
	req := LandRequest{
		WorktreeDir:     r.dir,
		BaseRev:         r.baseSHA,
		ResultRev:       workerSHA,
		BeadID:          "ddx-land-post-gate-fail",
		AttemptID:       "20260430T120200-post-gate-fail",
		TargetBranch:    "main",
		PostLandCommand: []string{"sh", "-c", "printf 'build failed'; exit 7"},
	}

	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "preserved" {
		t.Fatalf("expected status=preserved, got %q", land.Status)
	}
	if !strings.Contains(land.Reason, "post-land gate failed") || !strings.Contains(land.Reason, "build failed") {
		t.Fatalf("expected post-land gate reason with command output, got %q", land.Reason)
	}
	if got := r.resolveRef("refs/heads/main"); got != r.baseSHA {
		t.Fatalf("main tip = %s, want restored base %s", got, r.baseSHA)
	}
	if got := r.resolveRef(land.PreserveRef); got != workerSHA {
		t.Fatalf("preserve ref resolves to %s, want %s", got, workerSHA)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "feature.txt")); !os.IsNotExist(err) {
		t.Fatalf("feature.txt should not remain in main worktree after post-land rollback, stat err=%v", err)
	}
}

func TestLand_PostLandGatePassLeavesTargetAdvanced(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature\n", "feat: feature")
	req := LandRequest{
		WorktreeDir:     r.dir,
		BaseRev:         r.baseSHA,
		ResultRev:       workerSHA,
		BeadID:          "ddx-land-post-gate-pass",
		AttemptID:       "20260430T120201-post-gate-pass",
		TargetBranch:    "main",
		PostLandCommand: []string{"sh", "-c", "test -f feature.txt"},
	}

	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed, got %q (reason=%q)", land.Status, land.Reason)
	}
	if got := r.resolveRef("refs/heads/main"); got != workerSHA {
		t.Fatalf("main tip = %s, want worker %s", got, workerSHA)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "feature.txt")); err != nil {
		t.Fatalf("feature.txt should remain in main worktree after passing post-land gate: %v", err)
	}
}

// TestLand_ConcurrentSubmissions_Serialized spawns N concurrent Land() calls
// through a single coordinator-like serialization (sync.Mutex) and asserts
// that (a) all N worker commits are reachable from main, (b) each non-first
// submission took the merge path and produced a merge commit, and (c) every
// worker commit's original parent is preserved (replay fidelity).
//
// This is a direct test of the "single-writer" contract the server
// coordinator enforces, plus the replay-fidelity invariant of the
// merge-over-rebase design.
func TestLand_ConcurrentSubmissions_Serialized(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	const N = 5
	// Prepare N worker commits, each branching off the original baseSHA.
	// Each touches a distinct file so merges always complete cleanly.
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

	// Exactly one submission took the ff path (the first one under lock);
	// every subsequent one saw an advanced tip and took the merge path.
	merged := 0
	ff := 0
	for _, res := range results {
		if res == nil {
			continue
		}
		if res.Merged {
			merged++
		} else {
			ff++
		}
	}
	if ff != 1 {
		t.Errorf("expected exactly 1 fast-forward submission, got %d", ff)
	}
	if merged != N-1 {
		t.Errorf("expected %d merged submissions, got %d", N-1, merged)
	}

	// All N worker commits must be reachable from main.
	for i, sha := range workerSHAs {
		if !r.shaReachable("refs/heads/main", sha) {
			t.Errorf("worker %d commit %s not reachable from main", i, sha)
		}
	}

	// Replay fidelity: every worker commit must still have parent == baseSHA.
	for i, sha := range workerSHAs {
		parents := r.commitParents(sha)
		if len(parents) != 1 || parents[0] != r.baseSHA {
			t.Errorf("worker %d commit parent = %v, want [%s]", i, parents, r.baseSHA)
		}
	}

	// Each non-ff submission produced exactly one merge commit on main.
	if n := r.mergeCommitCount("refs/heads/main"); n != N-1 {
		t.Errorf("expected %d merge commits on main, got %d", N-1, n)
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

	// Now the origin has advanced beyond the local r.dir/main. To force a
	// push failure specifically, we need the local main to be advanced
	// ahead of origin AND the push to conflict. Simplest construction:
	// disable the auto-fetch by marking the remote as unreachable, then
	// land a local commit via the ff path and watch the push fail.
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

// TestLand_PushAutoRecovery_RaceWithRemoteResolved verifies the AC for
// ddx-a458af7c (#1, #4): a non-fast-forward push because origin advanced
// while the worker was running is auto-recovered by fetch + merge + retry-push.
// The bead lands cleanly without operator intervention.
func TestLand_PushAutoRecovery_RaceWithRemoteResolved(t *testing.T) {
	r := newLandTestRepoWithOrigin(t)
	ops := RealLandingGitOps{}

	// Side-clone advances origin/main with a commit that touches a different
	// file, so the auto-merge will succeed without conflicts.
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
	runCmd(sideDir, "commit", "-m", "remote: side-clone advance")
	runCmd(sideDir, "push", "origin", "main")

	// Worker branched off baseSHA before the side-clone landed.
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "worker\n", "feat: worker")

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-pushrace",
		AttemptID:    "20260429T000001-pr",
		TargetBranch: "main",
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed, got %q (reason=%q)", land.Status, land.Reason)
	}
	if land.PushFailed {
		t.Errorf("expected PushFailed=false after auto-recovery, got true (err=%q)", land.PushError)
	}
	if land.PushConflict {
		t.Errorf("expected PushConflict=false on clean auto-merge, got true (err=%q)", land.PushError)
	}
	if !land.PushRecovered {
		t.Errorf("expected PushRecovered=true after a successful auto-recovery, got false")
	}
	// Local target ref must point at the merge commit (not the original
	// workerSHA, since auto-recovery merged origin into it).
	localTip := r.resolveRef("refs/heads/main")
	if localTip == workerSHA {
		t.Errorf("expected local main to advance past workerSHA after auto-recovery, still at %s", workerSHA)
	}
	if land.NewTip != localTip {
		t.Errorf("LandResult.NewTip = %s, want local main tip %s", land.NewTip, localTip)
	}
	// Origin must have received the new tip (push retry succeeded).
	originOut, err := exec.Command("git", "-C", r.origin, "rev-parse", "refs/heads/main").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse origin/main: %s: %v", string(originOut), err)
	}
	originTip := strings.TrimSpace(string(originOut))
	if originTip != localTip {
		t.Errorf("origin/main = %s, want local %s — retry push must have landed remotely", originTip, localTip)
	}
	// Replay fidelity: workerSHA's own parent is unchanged (= baseSHA).
	parents := r.commitParents(workerSHA)
	if len(parents) != 1 || parents[0] != r.baseSHA {
		t.Errorf("worker commit parent = %v, want [%s] (auto-recovery must not rewrite worker commits)", parents, r.baseSHA)
	}
}

// TestLand_PushAutoRecovery_UnresolvableConflictReportsPushConflict verifies
// AC #3 + #5 of ddx-a458af7c: when origin advanced with overlapping changes
// the auto-merge cannot resolve, Land() reports PushConflict (distinct from
// generic PushFailed) so the loop can park the bead for human review under
// the 24h cap.
func TestLand_PushAutoRecovery_UnresolvableConflictReportsPushConflict(t *testing.T) {
	r := newLandTestRepoWithOrigin(t)
	ops := RealLandingGitOps{}

	// Side-clone edits the SAME file (feature.txt) on origin so the auto-merge
	// against our worker's edits will conflict.
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
	if err := os.WriteFile(filepath.Join(sideDir, "feature.txt"), []byte("origin-version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(sideDir, "add", "-A")
	runCmd(sideDir, "commit", "-m", "remote: conflict")
	runCmd(sideDir, "push", "origin", "main")

	// Worker edits the same file with conflicting content.
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "worker-version\n", "feat: worker")

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-pushconflict",
		AttemptID:    "20260429T000002-pc",
		TargetBranch: "main",
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed (local) even when push auto-recovery conflicts, got %q", land.Status)
	}
	if !land.PushConflict {
		t.Errorf("expected PushConflict=true when auto-merge conflicts, got false (PushFailed=%v PushError=%q)", land.PushFailed, land.PushError)
	}
	if land.PushFailed {
		t.Errorf("expected PushFailed=false (PushConflict is the more specific signal), got true")
	}
	if land.PushRecovered {
		t.Errorf("expected PushRecovered=false on conflict, got true")
	}
	if land.PushError == "" {
		t.Errorf("expected non-empty PushError describing the conflict, got empty")
	}
	// ApplyLandResultToExecuteBeadResult must surface the conflict marker so
	// the loop classifies the report as push_conflict, not push_failed.
	res := &ExecuteBeadResult{ExitCode: 0, BaseRev: r.baseSHA, ResultRev: workerSHA, BeadID: req.BeadID, AttemptID: req.AttemptID}
	ApplyLandResultToExecuteBeadResult(res, land)
	if !strings.HasPrefix(res.Reason, PushConflictReasonPrefix) {
		t.Errorf("expected res.Reason to start with PushConflictReasonPrefix, got %q", res.Reason)
	}
	if res.Status != ExecuteBeadStatusPushConflict {
		t.Errorf("expected res.Status = push_conflict, got %q", res.Status)
	}
}

// TestLandConflictAutoRecover_OrtResolvesCleanly verifies AC #6a at the git
// level: when both the preserved iteration and the current tip edit the same
// file (a content conflict), landConflictAutoRecover uses ort -X ours to
// resolve it cleanly, advancing the local target branch to a merge commit that
// is reachable from both the current tip and the preserved iteration.
func TestLandConflictAutoRecover_OrtResolvesCleanly(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	// Base: shared.txt = "line1\noriginal\nline3\n"
	r.writeFile("shared.txt", "line1\noriginal\nline3\n")
	r.runGit("add", "-A")
	r.runGit("commit", "-m", "base: add shared.txt")
	r.baseSHA = r.resolveRef("refs/heads/main")

	// Preserved iteration: changes the middle line.
	iterSHA := r.commitOn(r.baseSHA, "shared.txt", "line1\nITER-version\nline3\n", "iter: change line2")

	// Current tip: changes the same line — creates a content conflict.
	siblingSHA := r.commitOn(r.baseSHA, "shared.txt", "line1\nMAIN-version\nline3\n", "main: change line2")
	r.runGit("update-ref", "refs/heads/main", siblingSHA)

	// Create the preserve ref.
	preserveRef := "refs/ddx/iterations/ddx-land-recover/20260429T000001-" + siblingSHA[:12]
	r.runGit("update-ref", preserveRef, iterSHA)

	// landConflictAutoRecover must resolve via ort -X ours (no error).
	newTip, err := LandConflictAutoRecover(r.dir, preserveRef, ops)
	if err != nil {
		t.Fatalf("landConflictAutoRecover must succeed on mechanical conflict via ort -X ours: %v", err)
	}
	if newTip == "" {
		t.Fatal("expected non-empty newTip from landConflictAutoRecover")
	}

	// Local main must have advanced to the merge commit.
	mainTip := r.resolveRef("refs/heads/main")
	if mainTip != newTip {
		t.Errorf("expected local main tip = %s (newTip), got %s", newTip, mainTip)
	}
	if mainTip == siblingSHA {
		t.Errorf("main must advance past the current tip %s to the merge commit", siblingSHA)
	}

	// The preserved iteration commit is reachable from the new tip.
	if !r.shaReachable(mainTip, iterSHA) {
		t.Errorf("preserved iteration commit %s must be reachable from recovered tip %s", iterSHA, mainTip)
	}

	// The merge commit has two parents: [currentTip, iterSHA].
	parents := r.commitParents(newTip)
	if len(parents) != 2 {
		t.Fatalf("recovered merge commit should have 2 parents, got %v", parents)
	}
	if parents[0] != siblingSHA {
		t.Errorf("merge parent[0] = %s, want currentTip %s", parents[0], siblingSHA)
	}
	if parents[1] != iterSHA {
		t.Errorf("merge parent[1] = %s, want iterSHA %s", parents[1], iterSHA)
	}
}

// TestLandConflictAutoRecover_NonExistentPreserveRef_ReturnsError verifies that
// landConflictAutoRecover returns a non-nil error when the preserve ref does not
// resolve to a commit (e.g. was never written or was garbage-collected). The
// target branch must remain unchanged.
func TestLandConflictAutoRecover_NonExistentPreserveRef_ReturnsError(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	// preserve ref that was never created.
	preserveRef := "refs/ddx/iterations/ddx-ghost/20260429T000000-000000000000"

	beforeTip := r.resolveRef("refs/heads/main")
	_, err := LandConflictAutoRecover(r.dir, preserveRef, ops)
	if err == nil {
		t.Error("must return error when preserve ref does not exist")
	}

	// Target branch must be unchanged.
	afterTip := r.resolveRef("refs/heads/main")
	if afterTip != beforeTip {
		t.Errorf("target branch must not advance when preserve ref is absent: before=%s after=%s", beforeTip, afterTip)
	}
}

// Deterministic test clock helper — avoids unused time import when no test
// overrides NowFunc.
var _ = time.Now
