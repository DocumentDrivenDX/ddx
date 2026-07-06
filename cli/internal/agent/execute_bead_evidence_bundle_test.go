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

	// Step 2: VerifyCleanWorktree leaves gitignored evidence untracked (it must
	// never be committed — ddx-d10073a8).
	if err := VerifyCleanWorktree(r.dir, attemptID); err != nil {
		t.Fatalf("VerifyCleanWorktree: %v", err)
	}

	// The main working tree is clean: evidence is gitignored, so neither dirty
	// nor committed.
	statusOut, _, _ := runGitStatus(r.dir, dirRel)
	if strings.TrimSpace(statusOut) != "" {
		t.Errorf("main worktree not clean after VerifyCleanWorktree:\n%s", statusOut)
	}

	// Evidence is preserved on disk but must NEVER be committed.
	if _, err := os.Stat(filepath.Join(r.dir, ExecuteBeadArtifactDir, attemptID, "manifest.json")); err != nil {
		t.Errorf("evidence must remain on disk: %v", err)
	}
	logOut := r.runGit("log", "--oneline", "--name-only", "HEAD")
	evidenceManifestPath := filepath.ToSlash(filepath.Join(dirRel, "manifest.json"))
	if strings.Contains(logOut, evidenceManifestPath) {
		t.Errorf("evidence manifest must NOT be committed:\n%s", logOut)
	}
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------


// runGitStatus runs git status --porcelain for dirRel and returns (output, stdout, error).
func runGitStatus(dir, dirRel string) (string, string, error) {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain", "--", dirRel)
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	return s, s, err
}
