package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRecoverGitIndexLock_NoLock returns "not present" cleanly when the
// lock file does not exist.
func TestRecoverGitIndexLock_NoLock(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("setup .git: %v", err)
	}
	result, err := recoverGitIndexLock(dir)
	if err != nil {
		t.Fatalf("recoverGitIndexLock: %v", err)
	}
	if result.Removed {
		t.Fatalf("Removed=true for missing lock")
	}
	if !strings.Contains(result.Reason, "not present") {
		t.Fatalf("Reason: %q", result.Reason)
	}
}

// TestRecoverGitIndexLock_StaleByAge removes an unowned lock once it is
// older than gitIndexLockStaleAge.
func TestRecoverGitIndexLock_StaleByAge(t *testing.T) {
	prev := gitIndexLockStaleAge
	gitIndexLockStaleAge = 50 * time.Millisecond
	t.Cleanup(func() { gitIndexLockStaleAge = prev })

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("setup .git: %v", err)
	}
	lockPath := filepath.Join(dir, ".git", "index.lock")
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("create lock: %v", err)
	}
	old := time.Now().Add(-1 * time.Second)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	result, err := recoverGitIndexLock(dir)
	if err != nil {
		t.Fatalf("recoverGitIndexLock: %v", err)
	}
	if !result.Removed {
		t.Fatalf("expected Removed=true, got reason=%q", result.Reason)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock still exists after recovery: %v", err)
	}
}

// TestRecoverGitIndexLock_FreshUnowned leaves a fresh unowned lock in
// place — the lock might belong to a transient operator command that has
// not yet been picked up by lsof, so we wait rather than break it.
func TestRecoverGitIndexLock_FreshUnowned(t *testing.T) {
	prev := gitIndexLockStaleAge
	gitIndexLockStaleAge = 1 * time.Hour
	t.Cleanup(func() { gitIndexLockStaleAge = prev })

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("setup .git: %v", err)
	}
	lockPath := filepath.Join(dir, ".git", "index.lock")
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("create lock: %v", err)
	}

	result, err := recoverGitIndexLock(dir)
	if err != nil {
		t.Fatalf("recoverGitIndexLock: %v", err)
	}
	if result.Removed {
		t.Fatalf("fresh unowned lock should not be removed")
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock should still exist: %v", err)
	}
}

// TestRecoverGitIndexLock_DeadOwner removes the lock when lsof identifies
// a pid that is not alive. This test requires lsof and is skipped if
// unavailable.
func TestRecoverGitIndexLock_DeadOwner(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skip("lsof not available on PATH")
	}

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("setup .git: %v", err)
	}
	lockPath := filepath.Join(dir, ".git", "index.lock")

	// Spawn a short-lived process that opens the lock file, then exits.
	// While it is alive its pid owns the file; after exit, the file is
	// unowned and lsof returns nothing.
	cmd := exec.Command("sh", "-c", "exec 9>>"+lockPath+"; sleep 0.05")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper exited with error: %v", err)
	}
	// Helper is dead; lsof now returns nothing. Age the lock past stale
	// threshold so the unowned-stale branch removes it.
	prev := gitIndexLockStaleAge
	gitIndexLockStaleAge = 1 * time.Millisecond
	t.Cleanup(func() { gitIndexLockStaleAge = prev })

	time.Sleep(5 * time.Millisecond)
	result, err := recoverGitIndexLock(dir)
	if err != nil {
		t.Fatalf("recoverGitIndexLock: %v", err)
	}
	if !result.Removed {
		t.Fatalf("expected removal, reason=%q owner=%d alive=%v",
			result.Reason, result.OwnerPID, result.OwnerAlive)
	}
}

func TestIsGitIndexLockError(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want bool
	}{
		{"fatal_file_exists", "fatal: Unable to create '/repo/.git/index.lock': File exists.", true},
		{"combined_git_message", "fatal: Unable to create '/repo/.git/index.lock': File exists.\n\nAnother git process seems to be running in this repository", true},
		{"unrelated", "fatal: not a git repository", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isGitIndexLockError(tc.s); got != tc.want {
				t.Fatalf("isGitIndexLockError(%q) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}
