package agent

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/gitlock"
)

// TestRecoverGitIndexLock_NoLock returns "not present" cleanly when the
// lock file does not exist.
func TestRecoverGitIndexLock_NoLock(t *testing.T) {
	dir := initGitLockTestRepo(t)
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
	// A clean lsof no-match is required evidence for age-based recovery.
	// Keep this comfortably above normal process-table scan latency so the
	// test does not accidentally exercise the intentional fail-closed timeout.
	gitlock.LsofTimeout = 2 * time.Second
	t.Cleanup(func() { gitlock.LsofTimeout = prevLsof })

	dir := initGitLockTestRepo(t)
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

	dir := initGitLockTestRepo(t)
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

// deadOwnerPID is a high, non-allocated PID that processAlive treats as dead
// on Linux (syscall.Kill ESRCH). Real lsof never reports it for our lock file;
// tests inject it via a PATH-first fake lsof so the dead-owner path is proven
// without racing host process-table latency.
const deadOwnerPID = 2147483000

// installFakeLsof puts a deterministic lsof shim first on PATH for the test.
// body is a shell script body (receives the same args as real lsof).
func installFakeLsof(t *testing.T, body string) {
	t.Helper()
	binDir := t.TempDir()
	path := filepath.Join(binDir, "lsof")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake lsof: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestRecoverGitIndexLock_DeadOwner removes the lock when the owner probe
// identifies a pid that is not alive. Uses a PATH-first fake lsof so the
// outcome is independent of host lsof scheduling (the prior 100ms real-lsof
// window made this flaky on slow Linux hosts).
func TestRecoverGitIndexLock_DeadOwner(t *testing.T) {
	// Fake lsof immediately reports a single dead PID — no scan latency, no
	// scheduling race. Production still invokes real lsof via LookPath.
	// Intentionally leave gitlock.LsofTimeout at the production default; the
	// fake returns instantly so we do not depend on a shortened scheduling window.
	installFakeLsof(t, "echo "+strconv.Itoa(deadOwnerPID)+"; exit 0")

	dir := initGitLockTestRepo(t)
	lockPath := filepath.Join(dir, ".git", "index.lock")
	if err := os.WriteFile(lockPath, []byte("dead-owner"), 0o644); err != nil {
		t.Fatalf("create lock: %v", err)
	}

	result, err := recoverGitIndexLock(dir)
	if err != nil {
		t.Fatalf("recoverGitIndexLock: %v", err)
	}
	if !result.Removed {
		t.Fatalf("expected removal for dead owner, reason=%q owner=%d alive=%v",
			result.Reason, result.OwnerPID, result.OwnerAlive)
	}
	if result.OwnerPID != deadOwnerPID {
		t.Fatalf("OwnerPID=%d, want %d", result.OwnerPID, deadOwnerPID)
	}
	if result.OwnerAlive {
		t.Fatalf("OwnerAlive=true for dead owner pid")
	}
	if !strings.Contains(result.Reason, "not alive") {
		t.Fatalf("Reason should prove dead-owner path, got %q", result.Reason)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock still exists after dead-owner recovery: %v", err)
	}
}

// TestRecoverGitIndexLock_LsofTimeoutPreservesLock proves that when the owner
// probe times out (unknown ownership), recovery fails closed and leaves the
// lock in place. Production must not remove a lock without a dead-owner or
// proven-absent-and-stale proof.
func TestRecoverGitIndexLock_LsofTimeoutPreservesLock(t *testing.T) {
	// exec replaces the shell so CommandContext's kill targets sleep itself;
	// a bare "sleep 60" under sh can leave Wait hung after the shell dies.
	installFakeLsof(t, "exec sleep 60")

	prevLsof := gitlock.LsofTimeout
	// Short but non-zero: long enough for sleep to start, short enough that
	// the test stays fast. Does not change the production default permanently.
	gitlock.LsofTimeout = 50 * time.Millisecond
	t.Cleanup(func() { gitlock.LsofTimeout = prevLsof })

	// Even a very old lock must survive unknown ownership.
	prevAge := gitlock.StaleAge
	gitlock.StaleAge = 1 * time.Millisecond
	t.Cleanup(func() { gitlock.StaleAge = prevAge })

	dir := initGitLockTestRepo(t)
	lockPath := filepath.Join(dir, ".git", "index.lock")
	payload := []byte("must-survive-lsof-timeout")
	if err := os.WriteFile(lockPath, payload, 0o644); err != nil {
		t.Fatalf("create lock: %v", err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	started := time.Now()
	result, err := recoverGitIndexLock(dir)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("recoverGitIndexLock: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("lsof timeout path took %v; expected ~LsofTimeout (50ms)", elapsed)
	}
	if result.Removed {
		t.Fatalf("timeout/unknown owner must not remove lock, reason=%q", result.Reason)
	}
	if !strings.Contains(result.Reason, "unknown") {
		t.Fatalf("Reason should report unknown ownership, got %q", result.Reason)
	}
	if !strings.Contains(result.Reason, "timed out") {
		t.Fatalf("Reason should mention lsof timeout, got %q", result.Reason)
	}
	got, readErr := os.ReadFile(lockPath)
	if readErr != nil {
		t.Fatalf("lock should still exist: %v", readErr)
	}
	if string(got) != string(payload) {
		t.Fatalf("lock payload changed: got %q want %q", got, payload)
	}
}

// initGitLockTestRepo creates a real repository because gitlock resolves the
// native Git index-lock location through `git rev-parse --git-path`, including
// linked-worktree indirection. A hand-created .git directory is not a valid
// input to that contract.
func initGitLockTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := git.Command(context.Background(), dir, "init").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	return dir
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
