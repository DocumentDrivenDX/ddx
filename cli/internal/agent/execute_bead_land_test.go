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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/gitlock"
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

func (r *landTestRepo) syncWorkTreeFrom(fromRev string) {
	r.t.Helper()
	if err := syncWorkTreeToHeadExcludingPaths(r.dir, fromRev, nil); err != nil {
		r.t.Fatalf("sync worktree from %s: %v", fromRev, err)
	}
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

func writeExecuteBeadBundle(t *testing.T, root string, res *ExecuteBeadResult, extraFiles map[string]string) {
	t.Helper()
	evidenceDir := filepath.Join(root, filepath.FromSlash(res.ExecutionDir))
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeArtifactJSON(filepath.Join(evidenceDir, "manifest.json"), map[string]string{
		"attempt_id": res.AttemptID,
		"bead_id":    res.BeadID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeArtifactJSON(filepath.Join(evidenceDir, "result.json"), res); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evidenceDir, "prompt.md"), []byte("# Prompt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, content := range extraFiles {
		full := filepath.Join(evidenceDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func (r *landTestRepo) commitExecuteBeadEvidence(baseSHA string, res *ExecuteBeadResult, extraFiles map[string]string) string {
	r.t.Helper()
	wt, err := os.MkdirTemp("", "land-test-evidence-wt-*")
	if err != nil {
		r.t.Fatal(err)
	}
	_ = os.RemoveAll(wt)
	r.runGit("worktree", "add", "--detach", wt, baseSHA)
	defer func() {
		r.runGit("worktree", "remove", "--force", wt)
		_ = os.RemoveAll(wt)
	}()

	writeExecuteBeadBundle(r.t, wt, res, extraFiles)

	cmd := exec.Command("git", "-C", wt, "add", "--", filepath.FromSlash(res.ExecutionDir))
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git add evidence: %s: %v", string(out), err)
	}
	cmd = exec.Command("git", "-C", wt,
		"-c", "user.name=ddx-land-coordinator",
		"-c", "user.email=coordinator@ddx.local",
		"commit", "--no-verify", "-m", "chore: add execution evidence ["+res.AttemptID[:16]+"]",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git commit evidence: %s: %v", string(out), err)
	}
	out, err := exec.Command("git", "-C", wt, "rev-parse", "HEAD").Output()
	if err != nil {
		r.t.Fatalf("git rev-parse HEAD: %v", err)
	}
	sha := strings.TrimSpace(string(out))
	ref := "refs/ddx/test-pins/" + sha[:12]
	r.runGit("update-ref", ref, sha)
	return sha
}

func (r *landTestRepo) changedFiles(sha string) []string {
	r.t.Helper()
	out := r.runGit("show", "--pretty=format:", "--name-only", sha)
	if strings.TrimSpace(out) == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
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

type localOnlyGitOps struct {
	real RealLandingGitOps
}

var _ LandingGitOps = (*localOnlyGitOps)(nil)

func (g *localOnlyGitOps) CurrentBranch(dir string) (string, error) {
	return g.real.CurrentBranch(dir)
}

func (g *localOnlyGitOps) ResolveRef(dir, ref string) (string, error) {
	return g.real.ResolveRef(dir, ref)
}

func (g *localOnlyGitOps) UpdateRefTo(dir, ref, sha, oldSHA string) error {
	return g.real.UpdateRefTo(dir, ref, sha, oldSHA)
}

func (g *localOnlyGitOps) SyncWorkTreeToHead(dir, fromRev string) error {
	return g.real.SyncWorkTreeToHead(dir, fromRev)
}

func (g *localOnlyGitOps) AddWorktree(dir, path, rev string) error {
	return g.real.AddWorktree(dir, path, rev)
}

func (g *localOnlyGitOps) AddBranchWorktree(dir, path, branch string) error {
	return g.real.AddBranchWorktree(dir, path, branch)
}

func (g *localOnlyGitOps) RemoveWorktree(dir, path string) error {
	return g.real.RemoveWorktree(dir, path)
}

func (g *localOnlyGitOps) MergeInto(wtDir, srcRev, msg string) error {
	return g.real.MergeInto(wtDir, srcRev, msg)
}

func (g *localOnlyGitOps) HeadRevAt(dir string) (string, error) {
	return g.real.HeadRevAt(dir)
}

func (g *localOnlyGitOps) CountCommits(dir, base, tip string) int {
	return g.real.CountCommits(dir, base, tip)
}

func (g *localOnlyGitOps) StageDir(dir, relPath string) error {
	return g.real.StageDir(dir, relPath)
}

func (g *localOnlyGitOps) CommitStaged(dir, msg string) (string, error) {
	return g.real.CommitStaged(dir, msg)
}

func (g *localOnlyGitOps) DiffNumstat(dir, base, tip string) (string, error) {
	return g.real.DiffNumstat(dir, base, tip)
}

func (g *localOnlyGitOps) DiffNameOnly(dir, base, tip string) ([]string, error) {
	return g.real.DiffNameOnly(dir, base, tip)
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

func TestLand_EvidenceCommitUsesCleanLandingWorktree(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	attemptID := "20260507T000000-cleanwt"
	evidenceDir := filepath.Join(ddxroot.DirName, "executions", attemptID)
	fullDir := filepath.Join(r.dir, evidenceDir)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fullDir, "manifest.json"), []byte(`{"attempt_id":"`+attemptID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature\n", "feat: feature")

	hookMarker := filepath.Join(t.TempDir(), "pre-commit-pwd.txt")
	hookPath := filepath.Join(r.dir, ".git", "hooks", "pre-commit")
	hook := fmt.Sprintf("#!/bin/sh\npwd > %s\n", hookMarker)
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookPath, []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-clean-evidence",
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  filepath.ToSlash(evidenceDir),
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.EvidenceCommitSHA == "" {
		t.Fatalf("expected evidence commit")
	}
	hookPWDBytes, err := os.ReadFile(hookMarker)
	if err != nil {
		t.Fatalf("reading hook marker: %v", err)
	}
	hookPWD := strings.TrimSpace(string(hookPWDBytes))
	if hookPWD == "" {
		t.Fatal("pre-commit hook did not record its working directory")
	}
	if hookPWD == r.dir {
		t.Fatalf("evidence commit ran in operator checkout %s; want clean landing worktree", hookPWD)
	}
}

func TestLand_StagedOperatorFilesBlockLanding(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	attemptID := "20260507T000001-staged"
	evidenceDir := filepath.Join(ddxroot.DirName, "executions", attemptID)
	fullDir := filepath.Join(r.dir, evidenceDir)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fullDir, "manifest.json"), []byte(`{"attempt_id":"`+attemptID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r.writeFile("operator.txt", "operator staged content\n")
	r.runGit("add", "operator.txt")

	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature\n", "feat: feature")
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-staged-operator",
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  filepath.ToSlash(evidenceDir),
	}
	land, err := Land(r.dir, req, ops)
	if err == nil {
		t.Fatalf("Land succeeded with staged operator file; result=%+v", land)
	}
	if !strings.Contains(err.Error(), "staged changes") {
		t.Fatalf("Land error = %v, want staged changes error", err)
	}
	if got := r.resolveRef("refs/heads/main"); got != r.baseSHA {
		t.Fatalf("main advanced despite staged operator file: got %s want %s", got, r.baseSHA)
	}
	cached := r.runGit("diff", "--cached", "--name-only")
	if strings.TrimSpace(cached) != "operator.txt" {
		t.Fatalf("staged operator file was not preserved in index: %q", cached)
	}
}

func TestLand_EvidenceCommitFailurePreservesAndRestoresTarget(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	attemptID := "20260507T000003-noevidence"
	evidenceDir := filepath.Join(ddxroot.DirName, "executions", attemptID)
	fullDir := filepath.Join(r.dir, evidenceDir, "embedded")
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fullDir, "agent.jsonl"), []byte(`{"event":"ignored"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature\n", "feat: feature")
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-evidence-failure",
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  filepath.ToSlash(evidenceDir),
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land == nil || land.Status != "preserved" {
		t.Fatalf("expected preserved land result, got %+v", land)
	}
	if !strings.Contains(land.Reason, "evidence commit failed") {
		t.Fatalf("preserve reason = %q, want evidence failure", land.Reason)
	}
	if land.PreserveRef == "" {
		t.Fatalf("expected preserve ref")
	}
	if got := r.resolveRef("refs/heads/main"); got != r.baseSHA {
		t.Fatalf("main was not restored after evidence failure: got %s want %s", got, r.baseSHA)
	}
	if got := r.resolveRef(land.PreserveRef); got != workerSHA {
		t.Fatalf("preserve ref = %s, want worker %s", got, workerSHA)
	}
}

func TestLand_DirtyProjectRootDoesNotBlockSuccessfulLand(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	r.writeFile("README.md", "# operator edit\n")
	r.writeFile("operator-scratch.txt", "scratch\n")
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature\n", "feat: feature")

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-dirty-root",
		AttemptID:    "20260507T000002-dirty",
		TargetBranch: "main",
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land with dirty operator checkout: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected landed, got %q reason=%q", land.Status, land.Reason)
	}
	if got := r.resolveRef("refs/heads/main"); got != workerSHA {
		t.Fatalf("main tip = %s, want worker %s", got, workerSHA)
	}
	if content, err := os.ReadFile(filepath.Join(r.dir, "README.md")); err != nil || string(content) != "# operator edit\n" {
		t.Fatalf("operator tracked edit was not preserved, content=%q err=%v", string(content), err)
	}
	if content, err := os.ReadFile(filepath.Join(r.dir, "operator-scratch.txt")); err != nil || string(content) != "scratch\n" {
		t.Fatalf("operator untracked scratch was not preserved, content=%q err=%v", string(content), err)
	}
}

func TestLand_SyncDeferredWhenOperatorCheckoutDirtyOverlap(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	r.writeFile("README.md", "# operator edit\n")
	workerSHA := r.commitOn(r.baseSHA, "README.md", "# worker edit\n", "feat: worker readme")

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-dirty-overlap",
		AttemptID:    "20260507T000003-overlap",
		TargetBranch: "main",
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected landed, got %q reason=%q", land.Status, land.Reason)
	}
	if !land.CheckoutSyncDeferred {
		t.Fatalf("expected checkout sync to be deferred for dirty overlap")
	}
	if len(land.CheckoutSyncDeferredPaths) != 1 || land.CheckoutSyncDeferredPaths[0] != "README.md" {
		t.Fatalf("deferred paths = %v, want [README.md]", land.CheckoutSyncDeferredPaths)
	}
	content, err := os.ReadFile(filepath.Join(r.dir, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	if string(content) != "# operator edit\n" {
		t.Fatalf("dirty operator README was overwritten: %q", string(content))
	}

	res := &ExecuteBeadResult{Outcome: ExecuteBeadOutcomeTaskSucceeded}
	ApplyLandResultToExecuteBeadResult(res, land)
	if !strings.Contains(res.Reason, "checkout_sync_deferred: README.md") {
		t.Fatalf("ExecuteBeadResult reason %q does not expose checkout sync deferral", res.Reason)
	}
}

func TestLand_SyncCleanCheckoutStillUpdatesFiles(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature\n", "feat: feature")
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-clean-sync",
		AttemptID:    "20260507T000004-cleansync",
		TargetBranch: "main",
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.CheckoutSyncDeferred {
		t.Fatalf("clean checkout unexpectedly deferred sync: %v", land.CheckoutSyncDeferredPaths)
	}
	content, err := os.ReadFile(filepath.Join(r.dir, "feature.txt"))
	if err != nil {
		t.Fatalf("feature.txt not synced into clean checkout: %v", err)
	}
	if string(content) != "feature\n" {
		t.Fatalf("feature.txt content = %q, want feature", string(content))
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
	r.syncWorkTreeFrom(r.baseSHA)

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
	r.syncWorkTreeFrom(r.baseSHA)

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

func TestChaos_PostLandCommandDoesNotHoldMainGitLock(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature\n", "feat: feature")
	markerDir := t.TempDir()
	startedMarker := filepath.Join(markerDir, "started")
	releaseMarker := filepath.Join(markerDir, "release")
	req := LandRequest{
		WorktreeDir:     r.dir,
		BaseRev:         r.baseSHA,
		ResultRev:       workerSHA,
		BeadID:          "ddx-land-post-gate-lock-surface",
		AttemptID:       "20260430T120202-post-gate-lock-surface",
		TargetBranch:    "main",
		PostLandCommand: []string{"sh", "-c", "echo started > \"$1\"; while [ ! -f \"$2\" ]; do sleep 0.05; done", "sh", startedMarker, releaseMarker},
	}

	type landOutcome struct {
		land *LandResult
		err  error
	}

	landCh := make(chan landOutcome, 1)
	go func() {
		land, err := Land(r.dir, req, ops)
		landCh <- landOutcome{land: land, err: err}
	}()

	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(startedMarker); err == nil {
			break
		} else if !os.IsNotExist(err) {
			t.Fatalf("reading post-land start marker: %v", err)
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for post-land command to start")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	acquired := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		if err := withMainGitLock(r.dir, "test", func() error { return nil }); err != nil {
			t.Errorf("withMainGitLock during blocked post-land command: %v", err)
			return
		}
		acquired <- time.Since(start)
	}()

	select {
	case waited := <-acquired:
		if waited > time.Second {
			t.Fatalf("main-git lock was held for %s while post-land command was blocked", waited)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for concurrent main-git lock acquisition")
	}

	select {
	case out := <-landCh:
		t.Fatalf("Land finished before the blocking post-land command was released: %+v", out)
	default:
	}

	if err := os.WriteFile(releaseMarker, []byte("release\n"), 0o644); err != nil {
		t.Fatalf("releasing post-land command: %v", err)
	}

	var out landOutcome
	select {
	case out = <-landCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Land to finish after release")
	}
	if out.err != nil {
		t.Fatalf("Land: %v", out.err)
	}
	if out.land == nil || out.land.Status != "landed" {
		t.Fatalf("expected landed outcome after blocked command release, got %+v", out.land)
	}
	if out.land.NewTip != workerSHA {
		t.Fatalf("new tip = %s, want %s", out.land.NewTip, workerSHA)
	}
	if got := r.resolveRef("refs/heads/main"); got != workerSHA {
		t.Fatalf("main tip = %s, want %s", got, workerSHA)
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

// TestLand_IsNetworkFree verifies that Land() no longer performs remote fetch
// or push work and that it releases the main-git lock as soon as it returns.
func TestLand_IsNetworkFree(t *testing.T) {
	r := newLandTestRepo(t)
	ops := &localOnlyGitOps{}

	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature\n", "feat: feature")
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-network-free",
		AttemptID:    "20260511T000000-network-free",
		TargetBranch: "main",
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed, got %q (reason=%q)", land.Status, land.Reason)
	}
	if land.NewTip != workerSHA {
		t.Fatalf("expected new tip %s, got %s", workerSHA, land.NewTip)
	}

	acquired := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		if err := withMainGitLock(r.dir, "test", func() error { return nil }); err != nil {
			t.Errorf("withMainGitLock after Land: %v", err)
			return
		}
		acquired <- time.Since(start)
	}()

	// Give the goroutine time to attempt lock acquisition. The actual
	// sleep duration does not affect correctness — the lock is released
	// by Land() before this goroutine even starts trying.
	time.Sleep(100 * time.Millisecond)

	select {
	case waited := <-acquired:
		if waited > time.Second {
			t.Fatalf("main-git lock was held for %s after Land returned; want < 1s", waited)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for main-git lock acquisition after Land returned")
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

func TestLand_PreserveRecoveryDoesNotSyncDirtyProjectRoot(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	iterSHA := r.commitOn(r.baseSHA, "README.md", "# recovered iteration\n", "iter: update readme")
	preserveRef := "refs/ddx/iterations/ddx-land-recover/20260507T000006-" + r.baseSHA[:12]
	r.runGit("update-ref", preserveRef, iterSHA)

	r.writeFile("README.md", "# operator edit\n")
	newTip, err := LandConflictAutoRecover(r.dir, preserveRef, ops)
	if err != nil {
		t.Fatalf("LandConflictAutoRecover: %v", err)
	}
	if newTip == "" {
		t.Fatal("expected recovered tip")
	}
	content, err := os.ReadFile(filepath.Join(r.dir, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	if string(content) != "# operator edit\n" {
		t.Fatalf("dirty operator README was overwritten during preserved recovery: %q", string(content))
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

// TestLand_MergeRequired_IndexCleanAfterMerge is a regression test for
// ddx-7e659c95. After a successful merge landing (the target branch advanced
// since the worker started), the main worktree's git INDEX must be clean —
// i.e. `git diff --cached --name-only` must return nothing for files the
// bead changed. Before the fix, UpdateRefTo advanced the main branch ref
// without updating the index, leaving staged reverts of the bead's changes.
// When the operator subsequently ran `git commit` (intending only
// beads.jsonl), those phantom staged entries were swept in, undoing the
// bead's work.
//
// The test exercises the index-lock contention scenario: a goroutine holds
// `.git/index.lock` for 2 s while Land() runs. Before the fix,
// SyncWorkTreeToHead silently discarded the lock-contention error and
// returned, leaving the index dirty. After the fix, SyncWorkTreeToHead
// retries until the lock is released and the index is clean.
func TestLand_MergeRequired_IndexCleanAfterMerge(t *testing.T) {
	// Reduce lsof timeout: on Linux lsof can consume up to LsofTimeout (2s)
	// per RecoverGitIndexLock call when no process holds the file. 100ms is
	// sufficient to determine there is no owner.
	prevLsof := gitlock.LsofTimeout
	gitlock.LsofTimeout = 100 * time.Millisecond
	t.Cleanup(func() { gitlock.LsofTimeout = prevLsof })

	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	// Pre-seed a tracked file in the initial commit so the worker has
	// something to modify (regression requires a modification, not just a
	// new file, so that UpdateRefTo dirtying the index is observable).
	r.writeFile("tracked.txt", "original content\n")
	r.runGit("add", "-A")
	r.runGit("commit", "-m", "test: seed tracked file")
	r.baseSHA = r.resolveRef("refs/heads/main")

	// Worker modifies tracked.txt (branching off baseSHA).
	workerSHA := r.commitOn(r.baseSHA, "tracked.txt", "bead changes\n", "feat: bead updates tracked")

	// Meanwhile, a sibling lands a commit on main (different file, no conflict).
	siblingSHA := r.commitOn(r.baseSHA, "sibling.txt", "sibling content\n", "feat: sibling")
	r.runGit("update-ref", "refs/heads/main", siblingSHA)
	r.syncWorkTreeFrom(r.baseSHA)

	// Create an evidence dir so the full merge+evidence path is exercised.
	// (Evidence triggers landingFinalizationWorktree and landEvidence, which
	// is the code path that existed when ddx-7e659c95 was filed.)
	attemptID := "20260508T000000-7e659c95"
	evidenceDir := filepath.Join(ddxroot.DirName, "executions", attemptID)
	fullEvidenceDir := filepath.Join(r.dir, evidenceDir)
	if err := os.MkdirAll(fullEvidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fullEvidenceDir, "manifest.json"),
		[]byte(`{"attempt_id":"`+attemptID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate operator index-lock contention: hold .git/index.lock so that
	// the first git read-tree HEAD call inside SyncWorkTreeToHead fails with
	// "Unable to create index.lock: File exists". Before the fix,
	// SyncWorkTreeToHead returned silently on this error and the index stayed
	// dirty. After the fix it retries until the lock clears.
	//
	// Timing: Land() takes ~800ms. The syncWorkTreeToHeadGuarded call happens
	// near the end (after merge + evidence worktree ops). Hold the lock for
	// 2s so it is still present when git read-tree HEAD is attempted; the
	// goroutine releases it so that the retry eventually succeeds.
	indexLockPath := filepath.Join(r.dir, ".git", "index.lock")
	lockFile, err := os.OpenFile(indexLockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("could not pre-create index.lock for contention test: %v", err)
	}
	_ = lockFile.Close()
	go func() {
		// Hold the lock for 2 s then release it so SyncWorkTreeToHead can retry.
		time.Sleep(2 * time.Second)
		_ = os.Remove(indexLockPath)
	}()

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-7e659c95",
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  filepath.ToSlash(evidenceDir),
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed, got %q (reason=%q)", land.Status, land.Reason)
	}
	if !land.Merged {
		t.Fatalf("expected Merged=true on merge path")
	}

	// AC2: after Land() returns, the main worktree index must be clean.
	// Specifically, `git diff --cached --name-only` must be empty: no
	// staged changes (including no phantom revert of tracked.txt).
	cached := r.runGit("diff", "--cached", "--name-only")
	if strings.TrimSpace(cached) != "" {
		t.Errorf("ddx-7e659c95 regression: main worktree index is dirty after merge landing.\n"+
			"Staged changes:\n%s\n"+
			"These phantom staged entries cause subsequent operator commits to "+
			"undo the bead's work.", cached)
	}

	// AC1: no pre-merge revert for tracked.txt in git status.
	statusOut := r.runGit("status", "--porcelain", "tracked.txt")
	if strings.TrimSpace(statusOut) != "" {
		t.Errorf("ddx-7e659c95 regression: tracked.txt has unexpected status after merge landing: %q", statusOut)
	}

	// Sanity: main tip must equal the evidence commit (or the merge commit if
	// no evidence was committed, but evidence is set here).
	if land.NewTip == "" {
		t.Fatalf("NewTip must not be empty after successful land")
	}
	if got := r.resolveRef("refs/heads/main"); got != land.NewTip {
		t.Errorf("main tip = %s, want land.NewTip %s", got, land.NewTip)
	}

	// Sanity: tracked.txt on disk must reflect the bead's change.
	content, readErr := os.ReadFile(filepath.Join(r.dir, "tracked.txt"))
	if readErr != nil {
		t.Errorf("tracked.txt not present in working tree after merge landing: %v", readErr)
	} else if string(content) != "bead changes\n" {
		t.Errorf("tracked.txt content = %q after merge landing, want bead changes", string(content))
	}
}

func TestExecuteBeadLandingCommitsFinalResultArtifact(t *testing.T) {
	r := newLandTestRepo(t)
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature content\n", "feat: worker change [ddx-final-result]")

	attemptID := "20260515T170000-finalresult"
	evidenceDir := filepath.ToSlash(filepath.Join(ddxroot.DirName, "executions", attemptID))
	prelim := &ExecuteBeadResult{
		BeadID:            "ddx-final-result",
		AttemptID:         attemptID,
		BaseRev:           r.baseSHA,
		ResultRev:         workerSHA,
		ImplementationRev: workerSHA,
		ExecutionDir:      evidenceDir,
		PromptFile:        filepath.ToSlash(filepath.Join(evidenceDir, "prompt.md")),
		ManifestFile:      filepath.ToSlash(filepath.Join(evidenceDir, "manifest.json")),
		ResultFile:        filepath.ToSlash(filepath.Join(evidenceDir, "result.json")),
		Outcome:           ExecuteBeadOutcomeTaskSucceeded,
		Status:            ExecuteBeadStatusSuccess,
		ExitCode:          0,
	}
	evidenceSHA := r.commitExecuteBeadEvidence(workerSHA, prelim, nil)

	res := *prelim
	res.ResultRev = evidenceSHA
	res.EvidenceRev = evidenceSHA

	land, err := Land(r.dir, LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    evidenceSHA,
		BeadID:       res.BeadID,
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  evidenceDir,
	}, RealLandingGitOps{})
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed, got %q (reason=%q)", land.Status, land.Reason)
	}

	ApplyLandResultToExecuteBeadResult(&res, land)
	if err := WriteExecuteBeadResultArtifact(r.dir, &res); err != nil {
		t.Fatalf("WriteExecuteBeadResultArtifact: %v", err)
	}

	resultRel := filepath.ToSlash(filepath.Join(evidenceDir, "result.json"))
	if staged := strings.TrimSpace(r.runGit("diff", "--cached", "--name-only", "--", resultRel)); staged != "" {
		t.Fatalf("final result artifact left staged diff: %s", staged)
	}
	if status := strings.TrimSpace(r.runGit("status", "--porcelain", "--", resultRel)); status != "" {
		t.Fatalf("final result artifact left working tree diff: %s", status)
	}

	resultPath := filepath.Join(r.dir, filepath.FromSlash(resultRel))
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read final result.json: %v", err)
	}
	var got ExecuteBeadResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("parse final result.json: %v", err)
	}
	if got.ImplementationRev != workerSHA {
		t.Fatalf("implementation_rev = %q, want %q", got.ImplementationRev, workerSHA)
	}
	if got.LandedRev != land.LandedTip {
		t.Fatalf("landed_rev = %q, want %q", got.LandedRev, land.LandedTip)
	}
	if got.TargetBranch != "main" {
		t.Fatalf("landed_branch = %q, want %q", got.TargetBranch, "main")
	}
	if got.Outcome != "merged" {
		t.Fatalf("outcome = %q, want %q", got.Outcome, "merged")
	}
	if got.OrchestratorStatus != ExecuteBeadStatusSuccess {
		t.Fatalf("orchestrator_status = %q, want %q", got.OrchestratorStatus, ExecuteBeadStatusSuccess)
	}
}

func TestExecuteBeadLandingFinalResultSkipsEmbeddedEvidence(t *testing.T) {
	r := newLandTestRepo(t)
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature content\n", "feat: worker change [ddx-final-embedded]")

	attemptID := "20260515T170100-finalembed"
	evidenceDir := filepath.ToSlash(filepath.Join(ddxroot.DirName, "executions", attemptID))
	prelim := &ExecuteBeadResult{
		BeadID:            "ddx-final-embedded",
		AttemptID:         attemptID,
		BaseRev:           r.baseSHA,
		ResultRev:         workerSHA,
		ImplementationRev: workerSHA,
		ExecutionDir:      evidenceDir,
		ResultFile:        filepath.ToSlash(filepath.Join(evidenceDir, "result.json")),
		Outcome:           ExecuteBeadOutcomeTaskSucceeded,
		Status:            ExecuteBeadStatusSuccess,
		ExitCode:          0,
	}
	writeExecuteBeadBundle(t, r.dir, prelim, map[string]string{
		"embedded/agent-001.jsonl": "{\"type\":\"result\"}\n",
	})

	land, err := Land(r.dir, LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       prelim.BeadID,
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  evidenceDir,
	}, RealLandingGitOps{})
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.EvidenceCommitSHA == "" {
		t.Fatalf("expected evidence commit SHA")
	}

	embeddedPath := filepath.ToSlash(filepath.Join(evidenceDir, "embedded", "agent-001.jsonl"))
	for _, path := range r.changedFiles(land.EvidenceCommitSHA) {
		if path == embeddedPath {
			t.Fatalf("final-result commit staged embedded session log %s", embeddedPath)
		}
	}
}

func TestExecuteBeadLandingFinalResultSkipsRunStateFiles(t *testing.T) {
	r := newLandTestRepo(t)
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature content\n", "feat: worker change [ddx-final-runstate]")

	attemptID := "20260515T170200-finalstate"
	evidenceDir := filepath.ToSlash(filepath.Join(ddxroot.DirName, "executions", attemptID))
	prelim := &ExecuteBeadResult{
		BeadID:            "ddx-final-runstate",
		AttemptID:         attemptID,
		BaseRev:           r.baseSHA,
		ResultRev:         workerSHA,
		ImplementationRev: workerSHA,
		ExecutionDir:      evidenceDir,
		ResultFile:        filepath.ToSlash(filepath.Join(evidenceDir, "result.json")),
		Outcome:           ExecuteBeadOutcomeTaskSucceeded,
		Status:            ExecuteBeadStatusSuccess,
		ExitCode:          0,
	}
	writeExecuteBeadBundle(t, r.dir, prelim, nil)
	runStatePath := filepath.Join(r.dir, ddxroot.DirName, "run-state", attemptID+".json")
	if err := os.MkdirAll(filepath.Dir(runStatePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runStatePath, []byte(`{"attempt_id":"`+attemptID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	land, err := Land(r.dir, LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       prelim.BeadID,
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  evidenceDir,
	}, RealLandingGitOps{})
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.EvidenceCommitSHA == "" {
		t.Fatalf("expected evidence commit SHA")
	}

	runStateRel := filepath.ToSlash(filepath.Join(ddxroot.DirName, "run-state", attemptID+".json"))
	for _, path := range r.changedFiles(land.EvidenceCommitSHA) {
		if path == runStateRel {
			t.Fatalf("final-result commit staged run-state file %s", runStateRel)
		}
	}
}

// Deterministic test clock helper — avoids unused time import when no test
// overrides NowFunc.
var _ = time.Now

// ---------------------------------------------------------------------------
// Revision-triplet tests (ddx-2599ce10)
// ---------------------------------------------------------------------------

// TestApplyLandResult_PreservesImplementationRevAndSetsLandedRev verifies that
// a successful land preserves the worker's own commit as ImplementationRev and
// records the target branch tip as LandedRev, without losing the original rev.
func TestApplyLandResult_PreservesImplementationRevAndSetsLandedRev(t *testing.T) {
	const workerSHA = "aaaa1111bbbb2222cccc3333dddd4444eeee5555"
	const branchTip = "ffff6666aaaa7777bbbb8888cccc9999dddd0000"

	res := &ExecuteBeadResult{
		BeadID:    "ddx-test",
		BaseRev:   "0000000000000000000000000000000000000000",
		ResultRev: workerSHA,
	}
	land := &LandResult{
		Status: "landed",
		NewTip: branchTip,
		Merged: false,
	}

	ApplyLandResultToExecuteBeadResult(res, land)

	if res.ImplementationRev != workerSHA {
		t.Errorf("ImplementationRev: want %q, got %q", workerSHA, res.ImplementationRev)
	}
	if res.LandedRev != branchTip {
		t.Errorf("LandedRev: want %q, got %q", branchTip, res.LandedRev)
	}
	// ResultRev is the backwards-compat alias that mirrors LandedRev.
	if res.ResultRev != branchTip {
		t.Errorf("ResultRev (compat): want %q, got %q", branchTip, res.ResultRev)
	}
	// Outcome must be "merged".
	if res.Outcome != "merged" {
		t.Errorf("Outcome: want %q, got %q", "merged", res.Outcome)
	}
}

// TestApplyLandResult_PreservesImplementationRevAndSetsLandedRev_MergeCommit
// confirms that a merge-commit landing also preserves ImplementationRev.
func TestApplyLandResult_PreservesImplementationRevAndSetsLandedRev_MergeCommit(t *testing.T) {
	const workerSHA = "1111aaaa2222bbbb3333cccc4444dddd5555eeee"
	const mergeCommit = "9999ffff8888eeee7777dddd6666cccc5555bbbb"

	res := &ExecuteBeadResult{
		BeadID:    "ddx-test",
		BaseRev:   "0000000000000000000000000000000000000000",
		ResultRev: workerSHA,
	}
	land := &LandResult{
		Status: "landed",
		NewTip: mergeCommit,
		Merged: true,
	}

	ApplyLandResultToExecuteBeadResult(res, land)

	if res.ImplementationRev != workerSHA {
		t.Errorf("ImplementationRev: want %q, got %q (merge-commit path)", workerSHA, res.ImplementationRev)
	}
	if res.LandedRev != mergeCommit {
		t.Errorf("LandedRev: want %q, got %q (merge-commit path)", mergeCommit, res.LandedRev)
	}
}

func TestLandResult_CarriesResolvedTargetBranch(t *testing.T) {
	r := newLandTestRepo(t)
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "hello\n", "feat: worker change")

	land, err := Land(r.dir, LandRequest{
		BaseRev:   r.baseSHA,
		ResultRev: workerSHA,
		BeadID:    "ddx-test",
		AttemptID: "20260515T091251-13788c66",
	}, RealLandingGitOps{})
	if err != nil {
		t.Fatalf("Land() error: %v", err)
	}
	if land.TargetBranch != "main" {
		t.Fatalf("TargetBranch: want %q, got %q", "main", land.TargetBranch)
	}
}

func TestApplyLandResult_PropagatesTargetBranchToReport(t *testing.T) {
	const workerSHA = "aaaa1111bbbb2222cccc3333dddd4444eeee5555"
	const landedSHA = "ffff6666aaaa7777bbbb8888cccc9999dddd0000"
	const landedBranch = "ddx/a54e0299-burndown-232516"
	const projectRoot = "/tmp/fizeau-a54e0299-rescue.235101"

	res := &ExecuteBeadResult{
		BeadID:      "ddx-test",
		BaseRev:     "0000000000000000000000000000000000000000",
		ResultRev:   workerSHA,
		ProjectRoot: projectRoot,
	}
	land := &LandResult{
		Status:       "landed",
		NewTip:       landedSHA,
		TargetBranch: landedBranch,
	}

	ApplyLandResultToExecuteBeadResult(res, land)
	report := ReportFromExecuteBeadResult(res, "standard")
	if report.TargetBranch != landedBranch {
		t.Fatalf("ExecuteBeadReport.TargetBranch: want %q, got %q", landedBranch, report.TargetBranch)
	}
	if report.ProjectRoot != projectRoot {
		t.Fatalf("ExecuteBeadReport.ProjectRoot: want %q, got %q", projectRoot, report.ProjectRoot)
	}
	legacy := toTryReport(report)
	if legacy.TargetBranch != landedBranch {
		t.Fatalf("try.Report.TargetBranch: want %q, got %q", landedBranch, legacy.TargetBranch)
	}
	if legacy.ProjectRoot != projectRoot {
		t.Fatalf("try.Report.ProjectRoot: want %q, got %q", projectRoot, legacy.ProjectRoot)
	}
}

// TestBuildLandRequest_UsesImplementationRevNotEvidenceRev proves that
// BuildLandRequestFromResult uses the pre-landing implementation revision even
// when EvidenceRev and LandedRev are also set (e.g. after a first land
// already rewrote ResultRev to the branch tip).
func TestBuildLandRequest_UsesImplementationRevNotEvidenceRev(t *testing.T) {
	const implSHA = "impl1111impl2222impl3333impl4444impl5555"
	const landedSHA = "land1111land2222land3333land4444land5555"
	const evidenceSHA = "evid1111evid2222evid3333evid4444evid5555"

	res := &ExecuteBeadResult{
		BeadID:            "ddx-test",
		AttemptID:         "20260101T000000-deadbeef",
		BaseRev:           "base1111base2222base3333base4444base5555",
		ResultRev:         landedSHA, // already rewritten to branch tip
		ImplementationRev: implSHA,   // original worker commit
		LandedRev:         landedSHA,
		EvidenceRev:       evidenceSHA,
		ExecutionDir:      ".ddx/executions/20260101T000000-deadbeef",
	}

	req := BuildLandRequestFromResult("/some/project/root", res)

	if req.ResultRev != implSHA {
		t.Errorf("LandRequest.ResultRev: want implementation rev %q, got %q", implSHA, req.ResultRev)
	}
	// Sanity: base rev is passed through unchanged.
	if req.BaseRev != res.BaseRev {
		t.Errorf("LandRequest.BaseRev: want %q, got %q", res.BaseRev, req.BaseRev)
	}
}

// TestBuildLandRequest_UsesResultRevBeforeFirstLand ensures the first landing
// still submits the evidence-bundle commit when ResultRev already points at
// that trailing audit commit but LandedRev has not been set yet.
func TestBuildLandRequest_UsesResultRevBeforeFirstLand(t *testing.T) {
	const implSHA = "impl1111impl2222impl3333impl4444impl5555"
	const evidenceSHA = "evid1111evid2222evid3333evid4444evid5555"

	res := &ExecuteBeadResult{
		BeadID:            "ddx-test",
		AttemptID:         "20260101T000000-deadbeef",
		BaseRev:           "base1111base2222base3333base4444base5555",
		ResultRev:         evidenceSHA,
		ImplementationRev: implSHA,
		EvidenceRev:       evidenceSHA,
		ExecutionDir:      ".ddx/executions/20260101T000000-deadbeef",
	}

	req := BuildLandRequestFromResult("/some/project/root", res)

	if req.ResultRev != evidenceSHA {
		t.Errorf("LandRequest.ResultRev: want first-land evidence rev %q, got %q", evidenceSHA, req.ResultRev)
	}
}

// TestBuildLandRequest_FallsBackToResultRevWhenImplementationRevEmpty confirms
// the backwards-compat path: when ImplementationRev is not yet set (pre-landing
// state) BuildLandRequestFromResult uses ResultRev.
func TestBuildLandRequest_FallsBackToResultRevWhenImplementationRevEmpty(t *testing.T) {
	const workerSHA = "work1111work2222work3333work4444work5555"

	res := &ExecuteBeadResult{
		BeadID:            "ddx-test",
		BaseRev:           "base1111",
		ResultRev:         workerSHA,
		ImplementationRev: "", // not yet set
	}

	req := BuildLandRequestFromResult("/project", res)

	if req.ResultRev != workerSHA {
		t.Errorf("LandRequest.ResultRev: want ResultRev fallback %q, got %q", workerSHA, req.ResultRev)
	}
}

// TestNoChangesRevisionSemantics_PreservesBaseRev verifies that a no-changes
// landing outcome does not fabricate ImplementationRev, LandedRev, or
// EvidenceRev, and leaves ResultRev and BaseRev intact.
func TestNoChangesRevisionSemantics_PreservesBaseRev(t *testing.T) {
	const baseSHA = "base1111base2222base3333base4444base5555"

	res := &ExecuteBeadResult{
		BeadID:    "ddx-test",
		BaseRev:   baseSHA,
		ResultRev: baseSHA, // worker produced no commit
	}
	land := &LandResult{
		Status: "no-changes",
		Reason: "worker reported no changes",
	}

	ApplyLandResultToExecuteBeadResult(res, land)

	if res.ImplementationRev != "" {
		t.Errorf("ImplementationRev: want empty for no-changes, got %q", res.ImplementationRev)
	}
	if res.LandedRev != "" {
		t.Errorf("LandedRev: want empty for no-changes, got %q", res.LandedRev)
	}
	if res.EvidenceRev != "" {
		t.Errorf("EvidenceRev: want empty for no-changes, got %q", res.EvidenceRev)
	}
	if res.ResultRev != baseSHA {
		t.Errorf("ResultRev: want base SHA %q unchanged, got %q", baseSHA, res.ResultRev)
	}
	if res.BaseRev != baseSHA {
		t.Errorf("BaseRev: want %q unchanged, got %q", baseSHA, res.BaseRev)
	}
	if res.Outcome != "no-changes" {
		t.Errorf("Outcome: want %q, got %q", "no-changes", res.Outcome)
	}
}

// TestSyncWorkTreeToHead_DoesNotClobberBeadsJSONL verifies that
// RealLandingGitOps.SyncWorkTreeToHead never overwrites .ddx/beads.jsonl in
// the main worktree, even when the landed commit modified it.
func TestSyncWorkTreeToHead_DoesNotClobberBeadsJSONL(t *testing.T) {
	r := newLandTestRepo(t)

	// Base commit includes .ddx/beads.jsonl with open state.
	r.writeFile(".ddx/beads.jsonl", `{"id":"ddx-test","status":"open"}`+"\n")
	r.runGit("add", ".ddx/beads.jsonl")
	r.runGit("commit", "-m", "add beads.jsonl")
	fromRev := r.resolveRef("refs/heads/main")

	// Landing commit: agent snapshot has in_progress, no queue-rank.
	r.writeFile(".ddx/beads.jsonl", `{"id":"ddx-test","status":"in_progress"}`+"\n")
	r.writeFile("feature.txt", "feature\n")
	r.runGit("add", ".ddx/beads.jsonl", "feature.txt")
	r.runGit("commit", "-m", "feat: add feature [ddx-test]")

	// Operator's live state written after claim: queue-rank preserved.
	liveContent := `{"id":"ddx-test","status":"open","extra":{"queue-rank":5}}` + "\n"
	r.writeFile(".ddx/beads.jsonl", liveContent)

	ops := RealLandingGitOps{}
	if err := ops.SyncWorkTreeToHead(r.dir, fromRev); err != nil {
		t.Fatalf("SyncWorkTreeToHead: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(r.dir, ddxroot.DirName, "beads.jsonl"))
	if err != nil {
		t.Fatalf("reading beads.jsonl: %v", err)
	}
	if string(got) != liveContent {
		t.Errorf("beads.jsonl was clobbered by SyncWorkTreeToHead\ngot:  %q\nwant: %q", string(got), liveContent)
	}
}

// TestStageDirForcesGitIgnored verifies that StageDir uses --force so that
// files under a gitignored directory are staged successfully. Regression for
// ddx-723bd318: projects whose .gitignore covers .ddx/executions/ had every
// bead silently preserved because StageDir staged nothing.
func TestStageDirForcesGitIgnored(t *testing.T) {
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s: %v", strings.Join(args, " "), string(out), err)
		}
	}
	runGit("init", "-b", "main")
	runGit("config", "user.name", "Test")
	runGit("config", "user.email", "test@test.local")

	// Commit .gitignore covering .ddx/executions/ before staging evidence.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".ddx/executions/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".gitignore")
	runGit("commit", "-m", "init")

	// Create evidence file under the gitignored directory.
	attemptID := "20260510T000000-gitignore-stagetest"
	evidenceDir := filepath.Join(ddxroot.DirName, "executions", attemptID)
	fullDir := filepath.Join(dir, evidenceDir)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fullDir, "manifest.json"), []byte(`{"attempt_id":"`+attemptID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	ops := RealLandingGitOps{}
	if err := ops.StageDir(dir, filepath.ToSlash(evidenceDir)); err != nil {
		t.Fatalf("StageDir on gitignored path: %v", err)
	}

	cmd := exec.Command("git", "-C", dir, "diff", "--cached", "--name-only")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git diff --cached: %v", err)
	}
	staged := strings.TrimSpace(string(out))
	wantFile := filepath.ToSlash(filepath.Join(evidenceDir, "manifest.json"))
	if !strings.Contains(staged, wantFile) {
		t.Fatalf("gitignored manifest.json not in staged index; staged=%q want %q", staged, wantFile)
	}
}

// TestEvidenceCommitSucceedsWhenGitignored verifies end-to-end that Land()
// produces status="landed" with an evidence commit even when the project
// .gitignore covers .ddx/executions/. Regression for ddx-723bd318.
func TestEvidenceCommitSucceedsWhenGitignored(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	// Commit .gitignore covering .ddx/executions/ on top of the initial commit.
	r.writeFile(".gitignore", ".ddx/executions/\n")
	r.runGit("add", ".gitignore")
	r.runGit("commit", "-m", "chore: gitignore executions dir")
	baseSHA := r.resolveRef("refs/heads/main")

	attemptID := "20260510T000000-gitignore-e2e"
	evidenceDir := filepath.Join(ddxroot.DirName, "executions", attemptID)
	fullDir := filepath.Join(r.dir, evidenceDir)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fullDir, "manifest.json"), []byte(`{"attempt_id":"`+attemptID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	workerSHA := r.commitOn(baseSHA, "feature.txt", "feature\n", "feat: feature")
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-gitignore-evidence-e2e",
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  filepath.ToSlash(evidenceDir),
	}
	land, err := Land(r.dir, req, ops)
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed when .ddx/executions/ is gitignored, got %q (reason=%q)", land.Status, land.Reason)
	}
	if land.EvidenceCommitSHA == "" {
		t.Fatalf("expected evidence commit SHA; evidence commit was skipped or failed")
	}
}

// TestSyncWorkTreeToHead_DoesNotClobberBeadsArchiveJSONL verifies that
// RealLandingGitOps.SyncWorkTreeToHead never overwrites .ddx/beads-archive.jsonl.
func TestSyncWorkTreeToHead_DoesNotClobberBeadsArchiveJSONL(t *testing.T) {
	r := newLandTestRepo(t)

	// Base commit includes .ddx/beads-archive.jsonl.
	r.writeFile(".ddx/beads-archive.jsonl", `{"id":"ddx-archived","status":"closed"}`+"\n")
	r.runGit("add", ".ddx/beads-archive.jsonl")
	r.runGit("commit", "-m", "add beads-archive.jsonl")
	fromRev := r.resolveRef("refs/heads/main")

	// Landing commit: agent snapshot writes a stale archive (missing a bead).
	r.writeFile(".ddx/beads-archive.jsonl", `{"id":"ddx-archived","status":"closed","extra":{"old":"snapshot"}}`+"\n")
	r.writeFile("feature.txt", "feature\n")
	r.runGit("add", ".ddx/beads-archive.jsonl", "feature.txt")
	r.runGit("commit", "-m", "feat: add feature [ddx-test]")

	// Live archive has additional data the operator added after the claim.
	liveContent := `{"id":"ddx-archived","status":"closed"}` + "\n" +
		`{"id":"ddx-archived-2","status":"closed"}` + "\n"
	r.writeFile(".ddx/beads-archive.jsonl", liveContent)

	ops := RealLandingGitOps{}
	if err := ops.SyncWorkTreeToHead(r.dir, fromRev); err != nil {
		t.Fatalf("SyncWorkTreeToHead: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(r.dir, ddxroot.DirName, "beads-archive.jsonl"))
	if err != nil {
		t.Fatalf("reading beads-archive.jsonl: %v", err)
	}
	if string(got) != liveContent {
		t.Errorf("beads-archive.jsonl was clobbered by SyncWorkTreeToHead\ngot:  %q\nwant: %q", string(got), liveContent)
	}
}

func TestSyncWorkTreeToHead_PreservesSkipWorktreeLocalOverlay(t *testing.T) {
	r := newLandTestRepo(t)
	overlayPath := ".ddx/plugins/helix/README.md"
	r.writeFile("app.txt", "old\n")
	r.writeFile(overlayPath, "committed overlay\n")
	r.runGit("add", "-A")
	r.runGit("commit", "-m", "add tracked overlay")
	fromRev := r.resolveRef("HEAD")

	r.runGit("update-index", "--skip-worktree", "--", overlayPath)
	r.writeFile(overlayPath, "local overlay\n")
	next := r.commitOn(fromRev, "app.txt", "new\n", "change app")
	r.runGit("update-ref", "refs/heads/main", next, fromRev)

	ops := RealLandingGitOps{}
	if err := ops.SyncWorkTreeToHead(r.dir, fromRev); err != nil {
		t.Fatalf("SyncWorkTreeToHead: %v", err)
	}

	if got := r.runGit("ls-files", "-v", "--", overlayPath); !strings.HasPrefix(got, "S ") {
		t.Fatalf("overlay skip-worktree bit not preserved: %q", got)
	}
	status := r.runGit("status", "--short", "--", overlayPath)
	if status != "" {
		t.Fatalf("overlay path became visible in git status: %q", status)
	}
	gotApp, err := os.ReadFile(filepath.Join(r.dir, "app.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotApp) != "new\n" {
		t.Fatalf("non-overlay changed file not materialized: %q", string(gotApp))
	}
	gotOverlay, err := os.ReadFile(filepath.Join(r.dir, overlayPath))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotOverlay) != "local overlay\n" {
		t.Fatalf("local overlay was clobbered: %q", string(gotOverlay))
	}
}
