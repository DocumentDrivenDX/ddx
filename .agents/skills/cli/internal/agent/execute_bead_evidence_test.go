package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestExecuteBeadLandsEvidence is the AC (1) regression test: a simulated
// successful execute-bead attempt against a real temp git repo asserts that
// (a) after the run returns the working tree is clean, and (b) at least one
// file under .ddx/executions/<attempt-id>/ is present in a committed state.
// AC (2): the worker's closing_commit_sha still resolves after the merge.
func TestExecuteBeadLandsEvidence(t *testing.T) {
	r := newLandTestRepo(t)

	// Simulate the orchestrator materializing evidence files in the project root.
	attemptID := "20260416T181205-aabb1122"
	evidenceDir := filepath.Join(".ddx", "executions", attemptID)
	fullDir := filepath.Join(r.dir, evidenceDir)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, fullDir, "manifest.json", `{"attempt_id":"`+attemptID+`","bead_id":"ddx-evidence-test"}`)
	writeEvidenceFile(t, fullDir, "result.json", `{"status":"success","bead_id":"ddx-evidence-test"}`)
	writeEvidenceFile(t, fullDir, "prompt.md", "# Prompt\nExecute bead ddx-evidence-test")

	// Create a worker commit (simulates agent output).
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature content\n", "feat: implement feature [ddx-evidence-test]")

	// Land with evidence — fast-forward path.
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-evidence-test",
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  filepath.ToSlash(evidenceDir),
	}
	land, err := Land(r.dir, req, RealLandingGitOps{})
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed, got %q (reason=%q)", land.Status, land.Reason)
	}

	// AC (1a): working tree is clean (git status --porcelain is empty).
	statusOut := r.runGit("status", "--porcelain")
	if strings.TrimSpace(statusOut) != "" {
		t.Errorf("AC 1a FAILED: working tree not clean after Land:\n%s", statusOut)
	}

	// AC (1b): at least one file under .ddx/executions/<attempt-id>/ is present
	// in git log for the result or successor commit.
	logOut := r.runGit("log", "--all", "--oneline", "--name-only")
	evidenceManifestPath := filepath.ToSlash(filepath.Join(evidenceDir, "manifest.json"))
	if !strings.Contains(logOut, evidenceManifestPath) {
		t.Errorf("AC 1b FAILED: %s not found in git log:\n%s", evidenceManifestPath, logOut)
	}

	// Evidence commit was created and NewTip advanced past the worker commit.
	if land.EvidenceCommitSHA == "" {
		t.Errorf("expected EvidenceCommitSHA to be set")
	}
	if land.NewTip == workerSHA {
		t.Errorf("NewTip should be the evidence commit, not the worker commit")
	}
	if land.NewTip != land.EvidenceCommitSHA {
		t.Errorf("NewTip (%s) != EvidenceCommitSHA (%s)", land.NewTip, land.EvidenceCommitSHA)
	}

	// AC (2): closing_commit_sha (the tip that will be recorded) still resolves.
	checkCmd := exec.Command("git", "-C", r.dir, "cat-file", "-e", land.NewTip)
	if err := checkCmd.Run(); err != nil {
		t.Errorf("AC 2 FAILED: closing_commit_sha %s does not resolve: %v", land.NewTip, err)
	}
	// The worker's original commit must also still resolve.
	checkCmd = exec.Command("git", "-C", r.dir, "cat-file", "-e", workerSHA)
	if err := checkCmd.Run(); err != nil {
		t.Errorf("AC 2 FAILED: worker commit %s no longer resolves: %v", workerSHA, err)
	}
	// Worker commit must be reachable from main.
	if !r.shaReachable("refs/heads/main", workerSHA) {
		t.Errorf("worker commit %s not reachable from main", workerSHA)
	}
}

// TestExecuteBeadLandsEvidence_MergePath exercises the merge path (target
// branch has advanced since the worker started) and verifies evidence is
// committed and the worktree is clean.
func TestExecuteBeadLandsEvidence_MergePath(t *testing.T) {
	r := newLandTestRepo(t)

	attemptID := "20260416T181206-ccdd3344"
	evidenceDir := filepath.Join(".ddx", "executions", attemptID)
	fullDir := filepath.Join(r.dir, evidenceDir)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, fullDir, "manifest.json", `{"attempt_id":"`+attemptID+`"}`)
	writeEvidenceFile(t, fullDir, "result.json", `{"status":"merged"}`)

	// Worker branches off baseSHA.
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature\n", "feat: worker")

	// Sibling advances main. Sync the working tree so it matches siblingSHA's
	// tree — update-ref alone only moves the ref, leaving the working tree stale.
	siblingSHA := r.commitOn(r.baseSHA, "sibling.txt", "sibling\n", "feat: sibling")
	r.runGit("update-ref", "refs/heads/main", siblingSHA)
	r.runGit("read-tree", "HEAD")
	r.runGit("checkout-index", "-f", "-a")

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-evidence-merge",
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  filepath.ToSlash(evidenceDir),
	}
	land, err := Land(r.dir, req, RealLandingGitOps{})
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected landed, got %q", land.Status)
	}
	if !land.Merged {
		t.Errorf("expected Merged=true on merge path")
	}

	statusOut := r.runGit("status", "--porcelain")
	if strings.TrimSpace(statusOut) != "" {
		t.Errorf("working tree not clean after merge-path land:\n%s", statusOut)
	}

	if land.EvidenceCommitSHA == "" {
		t.Errorf("expected EvidenceCommitSHA on merge path")
	}

	// Worker commit still resolves (closing_commit_sha contract).
	checkCmd := exec.Command("git", "-C", r.dir, "cat-file", "-e", workerSHA)
	if err := checkCmd.Run(); err != nil {
		t.Errorf("worker SHA %s doesn't resolve after merge: %v", workerSHA, err)
	}
	if !r.shaReachable("refs/heads/main", workerSHA) {
		t.Errorf("worker commit %s not reachable from main after merge", workerSHA)
	}
}

// TestLandNoEvidenceDir verifies that when EvidenceDir is empty, no evidence
// commit is created and behavior matches the pre-fix code path.
func TestLandNoEvidenceDir(t *testing.T) {
	r := newLandTestRepo(t)

	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "hello\n", "feat: hello")

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-no-evidence",
		AttemptID:    "20260416T181207-eeff5566",
		TargetBranch: "main",
	}
	land, err := Land(r.dir, req, RealLandingGitOps{})
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected landed, got %q", land.Status)
	}
	if land.EvidenceCommitSHA != "" {
		t.Errorf("expected no evidence commit, got %s", land.EvidenceCommitSHA)
	}
	if land.NewTip != workerSHA {
		t.Errorf("NewTip = %s, want %s (no evidence commit)", land.NewTip, workerSHA)
	}
}

// TestVerifyCleanWorktree verifies the safety net: VerifyCleanWorktree
// commits leftover evidence files that the land flow did not pick up.
func TestVerifyCleanWorktree(t *testing.T) {
	r := newLandTestRepo(t)

	attemptID := "20260416T181208-verify01"
	evidenceDir := filepath.Join(r.dir, ExecuteBeadArtifactDir, attemptID)
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, evidenceDir, "result.json", `{"status":"test"}`)

	// Before VerifyCleanWorktree, evidence is untracked.
	statusBefore := r.runGit("status", "--porcelain", "--", filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID)))
	if strings.TrimSpace(statusBefore) == "" {
		t.Fatal("expected untracked evidence files before verification")
	}

	if err := VerifyCleanWorktree(r.dir, attemptID); err != nil {
		t.Fatalf("VerifyCleanWorktree: %v", err)
	}

	// After, evidence dir should be clean.
	statusAfter := r.runGit("status", "--porcelain", "--", filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID)))
	if strings.TrimSpace(statusAfter) != "" {
		t.Errorf("evidence dir still dirty after VerifyCleanWorktree:\n%s", statusAfter)
	}
}

// TestVerifyCleanWorktreeNoOp verifies VerifyCleanWorktree is a no-op
// when the evidence dir is already committed.
func TestVerifyCleanWorktreeNoOp(t *testing.T) {
	r := newLandTestRepo(t)

	attemptID := "20260416T181209-noop0001"
	if err := VerifyCleanWorktree(r.dir, attemptID); err != nil {
		t.Errorf("VerifyCleanWorktree should be no-op when evidence dir doesn't exist: %v", err)
	}
}

func writeEvidenceFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
