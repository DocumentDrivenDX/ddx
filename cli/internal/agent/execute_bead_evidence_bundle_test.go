package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestExecuteBeadArtifacts_BundleCreatedInAttemptWorktree proves that
// createArtifactBundle places DirAbs (and all artifact absolute paths) inside
// the isolated attempt worktree, not under the main project root.
func TestExecuteBeadArtifacts_BundleCreatedInAttemptWorktree(t *testing.T) {
	root := t.TempDir()
	wt := t.TempDir()
	const attemptID = "20260508T000000-aabbccdd"

	arts, err := createArtifactBundle(root, wt, attemptID)
	if err != nil {
		t.Fatalf("createArtifactBundle: %v", err)
	}

	// DirAbs must be inside the worktree, not the project root.
	wtAbs, _ := filepath.Abs(wt)
	dirAbs, _ := filepath.Abs(arts.DirAbs)
	if !strings.HasPrefix(dirAbs, wtAbs+string(filepath.Separator)) && dirAbs != wtAbs {
		t.Errorf("DirAbs %q must be under worktree %q", arts.DirAbs, wt)
	}
	rootAbs, _ := filepath.Abs(root)
	if strings.HasPrefix(dirAbs, rootAbs+string(filepath.Separator)) {
		t.Errorf("DirAbs %q must NOT be under project root %q", arts.DirAbs, root)
	}

	// DirRel is unchanged: relative path from repo root.
	wantDirRel := ".ddx/executions/" + attemptID
	if arts.DirRel != wantDirRel {
		t.Errorf("DirRel = %q, want %q", arts.DirRel, wantDirRel)
	}

	// Bundle directory created at worktree path.
	if fi, err := os.Stat(filepath.Join(wt, ExecuteBeadArtifactDir, attemptID)); err != nil || !fi.IsDir() {
		t.Errorf("bundle dir not created at worktree path: %v", err)
	}
	// No bundle directory created at project root.
	if _, err := os.Stat(filepath.Join(root, ExecuteBeadArtifactDir, attemptID)); err == nil {
		t.Errorf("bundle dir must NOT be created at project root %s", root)
	}
}

// TestExecuteBeadWork_MainTreeHasNoUntrackedExecutionEvidence proves that when
// the evidence bundle is committed in the attempt worktree (normal success
// path), the main project checkout has no untracked .ddx/executions/<attempt>/
// entry — eliminating live-worker noise in the operator's working directory.
func TestExecuteBeadWork_MainTreeHasNoUntrackedExecutionEvidence(t *testing.T) {
	r := newLandTestRepo(t)
	const attemptID = "20260508T000000-worktree1"
	dirRel := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID))

	// Simulate what ExecuteBeadWithConfig does: create a worktree and write
	// evidence files there (without touching the project root).
	wt, err := os.MkdirTemp("", "ddx-test-eb-wt-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.RemoveAll(wt)
	r.runGit("worktree", "add", "--detach", wt, r.baseSHA)
	defer func() { r.runGit("worktree", "remove", "--force", wt) }()

	bundleDir := filepath.Join(wt, ExecuteBeadArtifactDir, attemptID)
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, bundleDir, "manifest.json", `{"attempt_id":"`+attemptID+`"}`)
	writeEvidenceFile(t, bundleDir, "result.json", `{"status":"success"}`)
	writeEvidenceFile(t, bundleDir, "prompt.md", "# Prompt\n")

	// commitEvidenceBundleInWorktree commits evidence inside the worktree.
	newRev := commitEvidenceBundleInWorktree(wt, dirRel, attemptID)
	if newRev == "" {
		t.Fatal("commitEvidenceBundleInWorktree returned empty rev — evidence was not committed")
	}

	// Main checkout must have NO untracked evidence for this attempt.
	statusOut, _, _ := runGitStatus(r.dir, dirRel)
	if strings.TrimSpace(statusOut) != "" {
		t.Errorf("main checkout has untracked evidence after worktree commit:\n%s", statusOut)
	}

	// Evidence commit must be reachable in the repo's object store.
	if out, err := exec.Command("git", "-C", r.dir, "cat-file", "-e", newRev).CombinedOutput(); err != nil {
		t.Errorf("evidence commit %s not reachable in object store: %v\n%s", newRev, err, out)
	}
}

// TestLand_EvidenceCommitUsesWorktreeBundleWithoutProjectCopy proves that when
// evidence is already committed in ResultRev (worktree-origin path), Land()
// records it as the EvidenceCommitSHA and does not need copyEvidenceDirForLanding
// from the project root.
func TestLand_EvidenceCommitUsesWorktreeBundleWithoutProjectCopy(t *testing.T) {
	r := newLandTestRepo(t)
	const attemptID = "20260508T000000-land1234"
	dirRel := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID))

	// Build a two-commit chain that simulates the worktree-origin path:
	//   baseSHA → workerSHA (code change) → evidenceSHA (evidence bundle)
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature content\n", "feat: implement feature [ddx-land-test]")
	evidenceSHA := r.commitWithEvidence(workerSHA, dirRel, attemptID)

	// Verify: evidence NOT present at project root.
	if _, err := os.Stat(filepath.Join(r.dir, ExecuteBeadArtifactDir, attemptID)); err == nil {
		t.Fatal("evidence must NOT be in project root before Land is called")
	}

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    evidenceSHA,
		BeadID:       "ddx-land-test",
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  dirRel,
	}
	land, err := Land(r.dir, req, RealLandingGitOps{})
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed, got %q (reason=%q)", land.Status, land.Reason)
	}

	// Evidence commit SHA is set. On the worktree-origin path, landing now
	// rewrites result.json to the final orchestrator fields before staging the
	// bundle, so a trailing commit may advance HEAD beyond the pre-land
	// evidenceSHA.
	if land.EvidenceCommitSHA == "" {
		t.Error("EvidenceCommitSHA must be set")
	}
	if !r.shaReachable(land.NewTip, evidenceSHA) {
		t.Errorf("evidence SHA %s must remain reachable from landed tip %s", evidenceSHA, land.NewTip)
	}

	// copyEvidenceDirForLanding was NOT called from project root: verify by
	// checking the evidence files are tracked (committed), not untracked.
	// After sync, files should be present and clean (not untracked noise).
	statusOut := r.runGit("status", "--porcelain", "--", dirRel)
	if strings.TrimSpace(statusOut) != "" {
		t.Errorf("evidence files show as dirty after worktree-origin land (want tracked+clean):\n%s", statusOut)
	}
	// Evidence exists in HEAD as a committed file (not a projectRoot copy).
	logOut := r.runGit("log", "--oneline", "--name-only", "HEAD")
	evidenceManifestPath := filepath.ToSlash(filepath.Join(dirRel, "manifest.json"))
	if !strings.Contains(logOut, evidenceManifestPath) {
		t.Errorf("evidence manifest not found in git log:\n%s", logOut)
	}

	// Working tree completely clean after Land.
	allStatus := r.runGit("status", "--porcelain")
	if strings.TrimSpace(allStatus) != "" {
		t.Errorf("working tree not clean after Land:\n%s", allStatus)
	}
}

// TestExecuteBead_NoLandPreservesEvidenceWithoutMainNoise proves that
// no-change/no-evidence/no-land outcomes publish the evidence bundle through an
// explicit controlled-copy path (publishEvidenceBundleToProjectRoot) and, after
// VerifyCleanWorktree commits it, leave the main worktree clean.
func TestExecuteBead_NoLandPreservesEvidenceWithoutMainNoise(t *testing.T) {
	r := newLandTestRepo(t)
	const attemptID = "20260508T000000-nolnd123"
	dirRel := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID))

	// Simulate the attempt worktree having evidence (but no agent commits).
	wt := t.TempDir()
	bundleDir := filepath.Join(wt, ExecuteBeadArtifactDir, attemptID)
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, bundleDir, "manifest.json", `{"attempt_id":"`+attemptID+`"}`)
	writeEvidenceFile(t, bundleDir, "result.json", `{"status":"no_changes"}`)
	writeEvidenceFile(t, bundleDir, "no_changes_rationale.txt", "verification_command: true\n")

	// Step 1: controlled publish (simulates what the deferred publish does for
	// no-land paths before the worktree is removed).
	publishEvidenceBundleToProjectRoot(r.dir, wt, dirRel)

	// After publish, evidence must be in project root (untracked at this point).
	if _, err := os.Stat(filepath.Join(r.dir, ExecuteBeadArtifactDir, attemptID)); err != nil {
		t.Fatalf("evidence not published to project root: %v", err)
	}

	// Step 2: VerifyCleanWorktree commits leftover untracked evidence.
	if err := VerifyCleanWorktree(r.dir, attemptID); err != nil {
		t.Fatalf("VerifyCleanWorktree: %v", err)
	}

	// After VerifyCleanWorktree the main working tree is clean.
	statusOut, _, _ := runGitStatus(r.dir, dirRel)
	if strings.TrimSpace(statusOut) != "" {
		t.Errorf("main worktree not clean after VerifyCleanWorktree:\n%s", statusOut)
	}

	// Evidence is now committed (reachable from HEAD).
	logOut := r.runGit("log", "--oneline", "--name-only", "HEAD")
	evidenceManifestPath := filepath.ToSlash(filepath.Join(dirRel, "manifest.json"))
	if !strings.Contains(logOut, evidenceManifestPath) {
		t.Errorf("evidence manifest not committed in HEAD log:\n%s", logOut)
	}
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// commitWithEvidence creates a commit at baseSHA in a throwaway worktree that
// includes evidence files under dirRel, simulating the worktree-origin path
// where commitEvidenceBundleInWorktree adds the bundle on top of the agent commit.
func (r *landTestRepo) commitWithEvidence(baseSHA, dirRel, attemptID string) string {
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

	evidenceDir := filepath.Join(wt, filepath.FromSlash(dirRel))
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		r.t.Fatal(err)
	}
	writeEvidenceFile(r.t, evidenceDir, "manifest.json", `{"attempt_id":"`+attemptID+`","bead_id":"ddx-land-test"}`)
	writeEvidenceFile(r.t, evidenceDir, "result.json", `{"status":"success","bead_id":"ddx-land-test"}`)
	writeEvidenceFile(r.t, evidenceDir, "prompt.md", "# Prompt\n")

	cmd := exec.Command("git", "-C", wt, "add", "--", filepath.FromSlash(dirRel))
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git add evidence: %s: %v", string(out), err)
	}
	cmd = exec.Command("git", "-C", wt,
		"-c", "user.name=ddx-land-coordinator",
		"-c", "user.email=coordinator@ddx.local",
		"commit", "--no-verify", "-m", "chore: add execution evidence ["+attemptID[:16]+"]",
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

// runGitStatus runs git status --porcelain for dirRel and returns (output, stdout, error).
func runGitStatus(dir, dirRel string) (string, string, error) {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain", "--", dirRel)
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	return s, s, err
}
