package workerstatus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferBead_FromWorktreePath(t *testing.T) {
	cmdline := "ddx work --once"
	cwd := "/tmp/ddx-exec-wt/.execute-bead-wt-ddx-c3219628-20260513T231309-0e6f776b"

	beadID, worktree := InferBead(cmdline, cwd)
	if beadID != "ddx-c3219628" {
		t.Fatalf("bead id mismatch: got %q", beadID)
	}
	if worktree == "" {
		t.Fatalf("worktree should be populated from cwd, got empty")
	}
}

func TestInferBead_FromTryPositionalArg(t *testing.T) {
	cmdline := "ddx try ddx-abc12345 --no-review"
	cwd := "/home/user/project"

	beadID, worktree := InferBead(cmdline, cwd)
	if beadID != "ddx-abc12345" {
		t.Fatalf("bead id mismatch: got %q", beadID)
	}
	if worktree != "" {
		t.Fatalf("worktree should be empty when cwd is the project root, got %q", worktree)
	}
}

func TestInferBead_NoSignal(t *testing.T) {
	beadID, worktree := InferBead("ddx work --watch", "/home/user/project")
	if beadID != "" {
		t.Fatalf("bead id should be empty, got %q", beadID)
	}
	if worktree != "" {
		t.Fatalf("worktree should be empty, got %q", worktree)
	}
}

func TestFilterByProject_KeepsMatchesAndDropsOthers(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()

	workers := []LiveWorker{
		{PID: 1, ProjectRoot: dir, Command: "ddx work"},
		{PID: 2, ProjectRoot: other, Command: "ddx work"},
		{PID: 3, ProjectRoot: dir, Command: "ddx try ddx-abc12345"},
	}

	got := FilterByProject(workers, dir)
	if len(got) != 2 {
		t.Fatalf("expected 2 workers for project A, got %d", len(got))
	}
	for _, w := range got {
		if w.ProjectRoot != dir {
			t.Errorf("filtered worker has wrong project root %q", w.ProjectRoot)
		}
	}
}

func TestFilterByProject_ResolvesSymlinkedProjectRoot(t *testing.T) {
	realDir := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}

	workers := []LiveWorker{
		{PID: 1, ProjectRoot: realDir, Command: "ddx work"},
	}
	got := FilterByProject(workers, link)
	if len(got) != 1 {
		t.Fatalf("expected symlink to match real path, got %d", len(got))
	}
}

func TestSamePath_EmptyInputs(t *testing.T) {
	if SamePath("", "/some/path") {
		t.Fatalf("empty paths must not match")
	}
	if SamePath("/some/path", "") {
		t.Fatalf("empty paths must not match")
	}
}
