package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExecuteBeadPreCommitEvidence_InvalidatedByStagedMutation proves that the
// pre-commit evidence fingerprint changes when either the staged tree or the
// hook inputs change, which forces a fresh hook run instead of reusing stale
// green evidence.
func TestExecuteBeadPreCommitEvidence_InvalidatedByStagedMutation(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)

	hookPath := filepath.Join(projectRoot, "lefthook.yml")
	if err := os.WriteFile(hookPath, []byte("pre-commit:\n  commands: {}\n"), 0o644); err != nil {
		t.Fatalf("writing baseline hook config: %v", err)
	}

	stagedRel := "precommit-fingerprint.txt"
	stagedAbs := filepath.Join(projectRoot, stagedRel)
	if err := os.WriteFile(stagedAbs, []byte("one\n"), 0o644); err != nil {
		t.Fatalf("writing staged file: %v", err)
	}
	runGitInteg(t, projectRoot, "add", stagedRel)

	first, err := ComputePreCommitEvidenceFingerprint(projectRoot)
	if err != nil {
		t.Fatalf("first fingerprint: %v", err)
	}

	if err := os.WriteFile(stagedAbs, []byte("two\n"), 0o644); err != nil {
		t.Fatalf("rewriting staged file: %v", err)
	}
	runGitInteg(t, projectRoot, "add", stagedRel)

	second, err := ComputePreCommitEvidenceFingerprint(projectRoot)
	if err != nil {
		t.Fatalf("second fingerprint: %v", err)
	}
	if first.Encode() == second.Encode() {
		t.Fatalf("staged-tree mutation should invalidate pre-commit evidence fingerprint: %s", first.Encode())
	}

	hookConfig, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("reading hook config: %v", err)
	}
	if err := os.WriteFile(hookPath, append(append([]byte(nil), hookConfig...), []byte("\n# fingerprint mutation\n")...), 0o644); err != nil {
		t.Fatalf("rewriting hook config: %v", err)
	}

	third, err := ComputePreCommitEvidenceFingerprint(projectRoot)
	if err != nil {
		t.Fatalf("third fingerprint: %v", err)
	}
	if first.Encode() == third.Encode() {
		t.Fatalf("hook-config mutation should invalidate pre-commit evidence fingerprint: %s", first.Encode())
	}
}
