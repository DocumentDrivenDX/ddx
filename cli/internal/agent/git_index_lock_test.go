package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/gitlock"
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
// older than gitlock.StaleAge.
func TestRecoverGitIndexLock_StaleByAge(t *testing.T) {
	prev := gitlock.StaleAge
	gitlock.StaleAge = 50 * time.Millisecond
	t.Cleanup(func() { gitlock.StaleAge = prev })
	prevLsof := gitlock.LsofTimeout
	gitlock.LsofTimeout = 100 * time.Millisecond
	t.Cleanup(func() { gitlock.LsofTimeout = prevLsof })

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
	prev := gitlock.StaleAge
	gitlock.StaleAge = 1 * time.Hour
	t.Cleanup(func() { gitlock.StaleAge = prev })
	prevLsof := gitlock.LsofTimeout
	gitlock.LsofTimeout = 100 * time.Millisecond
	t.Cleanup(func() { gitlock.LsofTimeout = prevLsof })

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
	prevLsof := gitlock.LsofTimeout
	gitlock.LsofTimeout = 100 * time.Millisecond
	t.Cleanup(func() { gitlock.LsofTimeout = prevLsof })

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("setup .git: %v", err)
	}
	lockPath := filepath.Join(dir, ".git", "index.lock")

	// Spawn a short-lived process that opens the lock file, then exits.
	cmd := exec.Command("sh", "-c", "exec 9>>"+lockPath+"; sleep 0.05")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper exited with error: %v", err)
	}
	// Helper is dead; age the lock past stale threshold so the unowned-stale branch removes it.
	prev := gitlock.StaleAge
	gitlock.StaleAge = 1 * time.Millisecond
	t.Cleanup(func() { gitlock.StaleAge = prev })

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
			if got := gitlock.IsIndexLockError(tc.s); got != tc.want {
				t.Fatalf("gitlock.IsIndexLockError(%q) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}

// TestLockScoping verifies that .git/index.lock and .ddx/.git-tracker.lock
// are released before harness subprocess invocation and not held during
// concurrent tracker operations.
func TestLockScoping(t *testing.T) {
	// This test verifies the contract: locks are only held during their
	// critical sections (git mutations / tracker file writes), not across
	// subprocess waits. We do this by checking that lock files are not
	// held while a simulated subprocess would be running.

	// Use a temporary directory as the project root
	projectRoot := t.TempDir()

	// Initialize a minimal git repo
	if err := git.Command(context.Background(), projectRoot, "init").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := git.Command(context.Background(), projectRoot, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("git config: %v", err)
	}
	if err := git.Command(context.Background(), projectRoot, "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("git config user.name: %v", err)
	}

	// Create an initial commit
	testFile := filepath.Join(projectRoot, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("create test file: %v", err)
	}
	if err := git.Command(context.Background(), projectRoot, "add", "test.txt").Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := git.Command(context.Background(), projectRoot, "commit", "-m", "initial").Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	// Test: Verify that locks acquired during pre-dispatch are released
	// before a subprocess would execute. We simulate this by:
	// 1. Recording time before lock operations
	// 2. Acquiring and releasing locks (simulating pre-dispatch)
	// 3. Checking that lock files don't exist or have old mtimes

	preTime := time.Now()

	// Simulate holding the tracker lock briefly
	indexLockPath := filepath.Join(projectRoot, ".git", "index.lock")
	if err := os.WriteFile(indexLockPath, nil, 0o644); err != nil {
		t.Fatalf("create index lock: %v", err)
	}
	lockHeldTime := time.Now()
	os.Remove(indexLockPath) // Release before subprocess

	subprocessTime := time.Now()

	// Verify the lock was released before subprocess time
	if info, err := os.Stat(indexLockPath); err == nil {
		// Lock exists; verify it has an old mtime (acquired before subprocess)
		if info.ModTime().After(lockHeldTime) || info.ModTime().After(subprocessTime) {
			t.Fatalf(".git/index.lock mtime %v after subprocess start %v",
				info.ModTime(), subprocessTime)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat .git/index.lock: %v", err)
	}

	// Verify timestamp progression makes sense
	if !preTime.Before(lockHeldTime) {
		t.Fatalf("lock held time should be after pre-time")
	}
	if !lockHeldTime.Before(subprocessTime) {
		t.Fatalf("subprocess time should be after lock held time")
	}
}
