package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/require"
)

func commitTrackerForTest(projectRoot string) error {
	return withTrackerLock(projectRoot, func() error {
		return commitTrackerLocked(projectRoot)
	})
}

// initTrackerRepo creates a temp git repo with an initial commit and a
// .ddx/beads.jsonl file already committed. Returns the project root.
func initTrackerRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = scrubbedGitEnvInteg()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@ddx.test")
	run("config", "user.name", "DDx Test")

	require.NoError(t, os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644))
	run("add", "seed.txt")
	run("commit", "-m", "chore: initial seed")

	ddxDir := filepath.Join(root, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(""), 0o644))
	run("add", ".ddx/beads.jsonl")
	run("commit", "-m", "chore: seed tracker")

	return root
}

func initConventionTrackerRepo(t *testing.T) (string, string) {
	t.Helper()

	t.Setenv("XDG_DATA_HOME", t.TempDir())

	root := t.TempDir()
	runGitInteg(t, root, "init", "-b", "main")
	runGitInteg(t, root, "config", "user.email", "test@ddx.test")
	runGitInteg(t, root, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644))
	runGitInteg(t, root, "add", "seed.txt")
	runGitInteg(t, root, "commit", "-m", "chore: initial seed")

	return root, ddxroot.Path(context.Background(), root)
}

// TestTrackerCommit_ConcurrentSafety verifies that two goroutines invoking
// CommitTracker against the same primary .git do not race on .git/index.lock.
// Without withTrackerLock around the git add/commit pair, one of the two
// goroutines would fail with: "fatal: Unable to create '.../.git/index.lock':
// File exists." (See bead description for the observed Phase 2 drain failure.)
func TestTrackerCommit_ConcurrentSafety(t *testing.T) {
	root := initTrackerRepo(t)
	tracker := filepath.Join(root, ddxroot.DirName, "beads.jsonl")

	const goroutines = 2
	const iterations = 8

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*iterations)
	// mu guards file writes so the on-disk content is well-defined; the tested
	// race is the git-index race inside CommitTracker, not file-write ordering.
	var mu sync.Mutex
	counter := 0

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				mu.Lock()
				counter++
				line := fmt.Sprintf(`{"id":"ddx-test-%04d-%04d","n":%d}`+"\n", g, i, counter)
				f, err := os.OpenFile(tracker, os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					mu.Unlock()
					errs <- fmt.Errorf("open tracker: %w", err)
					return
				}
				_, _ = f.WriteString(line)
				_ = f.Close()
				mu.Unlock()

				if err := commitTrackerForTest(root); err != nil {
					errs <- err
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			if strings.Contains(err.Error(), "index.lock") {
				t.Fatalf("git index.lock contention observed (lock not effective): %v", err)
			}
			t.Fatalf("unexpected CommitTracker error: %v", err)
		}
	}

	// Sanity: the lock dir must be cleaned up after the run.
	if _, err := os.Stat(trackerLockPath(root)); !os.IsNotExist(err) {
		t.Fatalf("tracker lock dir not cleaned up: stat err = %v", err)
	}
}

func TestTrackerCommit_OnlyCommitsTrackerPath(t *testing.T) {
	root := initTrackerRepo(t)
	tracker := filepath.Join(root, ddxroot.DirName, "beads.jsonl")

	require.NoError(t, os.WriteFile(filepath.Join(root, "operator.txt"), []byte("operator staged\n"), 0o644))
	cmd := exec.Command("git", "add", "operator.txt")
	cmd.Dir = root
	cmd.Env = scrubbedGitEnvInteg()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add operator.txt: %v\n%s", err, out)
	}

	require.NoError(t, os.WriteFile(tracker, []byte(`{"id":"ddx-only-tracker"}`+"\n"), 0o644))
	require.NoError(t, commitTrackerForTest(root))

	show := exec.Command("git", "show", "--name-only", "--format=", "HEAD")
	show.Dir = root
	show.Env = scrubbedGitEnvInteg()
	out, err := show.CombinedOutput()
	require.NoError(t, err, "git show HEAD: %s", out)
	names := strings.Fields(string(out))
	if len(names) != 1 || names[0] != ".ddx/beads.jsonl" {
		t.Fatalf("tracker commit touched %v, want only .ddx/beads.jsonl", names)
	}

	cached := exec.Command("git", "diff", "--cached", "--name-only")
	cached.Dir = root
	cached.Env = scrubbedGitEnvInteg()
	cachedOut, err := cached.CombinedOutput()
	require.NoError(t, err, "git diff --cached --name-only: %s", cachedOut)
	if strings.TrimSpace(string(cachedOut)) != "operator.txt" {
		t.Fatalf("pre-staged operator file was not preserved in index: %q", string(cachedOut))
	}
}

func TestCommitTrackerConventionModeCommitsXDGTracker(t *testing.T) {
	projectRoot, stateRoot := initConventionTrackerRepo(t)
	tracker := filepath.Join(stateRoot, "beads.jsonl")

	require.NoError(t, os.WriteFile(tracker, []byte(`{"id":"ddx-convention-tracker"}`+"\n"), 0o644))
	require.NoError(t, commitTrackerForTest(projectRoot))

	show := runGitInteg(t, stateRoot, "show", "--name-only", "--format=", "HEAD")
	require.Equal(t, "beads.jsonl", strings.TrimSpace(show))

	headTracker := runGitInteg(t, stateRoot, "show", "HEAD:beads.jsonl")
	require.Contains(t, headTracker, "ddx-convention-tracker")

	projectStatus := runGitInteg(t, projectRoot, "status", "--short", "--", ".ddx/beads.jsonl")
	require.Empty(t, projectStatus)
}

// TestTrackerLock_SharedAcrossLinkedWorktrees ensures the main-git lock uses a
// shared DDx workspace for linked worktrees, not the caller's checkout-local
// .ddx directory. Without this, two worktrees for the same repository can both
// enter the landing/tracker critical section and interleave ref updates.
func TestTrackerLock_SharedAcrossLinkedWorktrees(t *testing.T) {
	root := initTrackerRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	runGitInteg(t, root, "worktree", "add", "--detach", linked)
	t.Cleanup(func() { runGitInteg(t, root, "worktree", "remove", "--force", linked) })

	require.Equal(t, trackerLockPath(root), trackerLockPath(linked), "linked worktrees must share the same main-git lock")

	acquired := make(chan string, 2)
	release := make(chan struct{})
	done := make(chan error, 2)

	go func() {
		done <- withTrackerLock(root, func() error {
			acquired <- "root"
			<-release
			return nil
		})
	}()

	select {
	case who := <-acquired:
		require.Equal(t, "root", who)
	case <-time.After(2 * time.Second):
		t.Fatal("primary worktree did not acquire the tracker lock")
	}

	go func() {
		done <- withTrackerLock(linked, func() error {
			acquired <- "linked"
			return nil
		})
	}()

	select {
	case who := <-acquired:
		t.Fatalf("linked worktree acquired the tracker lock while the primary holder was still active: %s", who)
	case <-time.After(200 * time.Millisecond):
		// Expected: the second caller is blocked on the shared lock.
	}

	close(release)

	select {
	case who := <-acquired:
		require.Equal(t, "linked", who)
	case <-time.After(2 * time.Second):
		t.Fatal("linked worktree did not acquire the tracker lock after release")
	}

	for i := 0; i < 2; i++ {
		require.NoError(t, <-done)
	}
}

// TestTrackerCommit_MalformedRegularFileLockRecovery verifies that a stale
// regular file at the lock path (as opposed to the expected directory) is
// removed with single-path removal and that CommitTracker then succeeds.
func TestTrackerCommit_MalformedRegularFileLockRecovery(t *testing.T) {
	root := initTrackerRepo(t)
	tracker := filepath.Join(root, ddxroot.DirName, "beads.jsonl")

	lockPath := trackerLockPath(root)
	require.NoError(t, os.WriteFile(lockPath, []byte("stale"), 0o644))
	// Back-date the mtime so it exceeds trackerLockStaleAge.
	staleTime := time.Now().Add(-2 * trackerLockStaleAge)
	require.NoError(t, os.Chtimes(lockPath, staleTime, staleTime))

	require.NoError(t, os.WriteFile(tracker, []byte(`{"id":"ddx-malformed-regular"}`+"\n"), 0o644))

	if err := commitTrackerForTest(root); err != nil {
		t.Fatalf("CommitTracker failed to recover from malformed stale regular file: %v", err)
	}

	// Lock path must be gone after recovery (removed, then replaced by a dir, then dir removed).
	if _, err := os.Lstat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock path was not cleaned up after stale regular-file recovery: lstat err = %v", err)
	}

	cmd := exec.Command("git", "show", "HEAD:.ddx/beads.jsonl")
	cmd.Dir = root
	cmd.Env = scrubbedGitEnvInteg()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git show HEAD:.ddx/beads.jsonl: %s", out)
	if !strings.Contains(string(out), "ddx-malformed-regular") {
		t.Fatalf("tracker commit did not land: HEAD contents = %q", string(out))
	}
}

// TestTrackerCommit_MalformedFreshRegularFileFailsFast verifies that a fresh
// regular file at the lock path causes fail-fast (< 100ms) without consuming
// the retry budget and that the file is left in place.
func TestTrackerCommit_MalformedFreshRegularFileFailsFast(t *testing.T) {
	root := initTrackerRepo(t)

	lockPath := trackerLockPath(root)
	require.NoError(t, os.WriteFile(lockPath, []byte("fresh"), 0o644))

	policy := LockRetryPolicy{
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
		MaxRetries:     100,
		MaxElapsed:     60 * time.Second,
	}

	start := time.Now()
	err := withTrackerLockPolicy(root, policy, func() error { return nil })
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected malformed-lock error, got nil")
	}
	if elapsed >= 100*time.Millisecond {
		t.Fatalf("expected fail-fast (< 100ms), took %v; err = %v", elapsed, err)
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected malformed-lock diagnostic, got: %v", err)
	}

	// Fresh file must remain in place.
	info, statErr := os.Lstat(lockPath)
	if statErr != nil {
		t.Fatalf("fresh regular file was unexpectedly removed: %v", statErr)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("lock path is no longer a regular file: mode = %v", info.Mode())
	}
}

// TestTrackerCommit_MalformedSymlinkLockFailsFast verifies that a symlink at
// the lock path causes fail-fast without removing the symlink or its target.
// Skipped on platforms that do not support symlink creation.
func TestTrackerCommit_MalformedSymlinkLockFailsFast(t *testing.T) {
	root := initTrackerRepo(t)
	lockPath := trackerLockPath(root)

	targetPath := lockPath + ".target"
	require.NoError(t, os.WriteFile(targetPath, []byte("target"), 0o644))
	if err := os.Symlink(targetPath, lockPath); err != nil {
		t.Skipf("symlink creation unsupported on this platform: %v", err)
	}

	policy := LockRetryPolicy{
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
		MaxRetries:     100,
		MaxElapsed:     60 * time.Second,
	}

	start := time.Now()
	err := withTrackerLockPolicy(root, policy, func() error { return nil })
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected malformed-lock error for symlink, got nil")
	}
	if elapsed >= 100*time.Millisecond {
		t.Fatalf("expected fail-fast (< 100ms), took %v; err = %v", elapsed, err)
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected malformed-lock diagnostic, got: %v", err)
	}

	// Symlink must remain in place.
	if _, err := os.Lstat(lockPath); err != nil {
		t.Fatalf("symlink was removed: %v", err)
	}
	// Symlink target must remain in place.
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("symlink target was removed: %v", err)
	}
}

// TestTrackerCommit_MissingOwnerDoesNotReportUnknown verifies that a real lock
// directory without a pid sidecar reports "owner pid: missing" (not "unknown")
// in the timeout diagnostic.
func TestTrackerCommit_MissingOwnerDoesNotReportUnknown(t *testing.T) {
	root := initTrackerRepo(t)

	lockDir := trackerLockPath(root)
	require.NoError(t, os.MkdirAll(lockDir, 0o755))
	// Write a fresh acquired_at so age-based stale detection does not fire.
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "acquired_at"),
		[]byte(time.Now().UTC().Format(time.RFC3339)), 0o644))
	// Deliberately omit the pid sidecar.
	defer os.RemoveAll(lockDir)

	policy := LockRetryPolicy{
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     1 * time.Millisecond,
		Multiplier:     1.0,
		MaxRetries:     1,
	}

	err := withTrackerLockPolicy(root, policy, func() error { return nil })
	if err == nil {
		t.Fatalf("expected lock timeout error, got nil")
	}
	if strings.Contains(err.Error(), "owner pid: unknown") {
		t.Fatalf("diagnostic must not say 'owner pid: unknown', got: %v", err)
	}
	if !strings.Contains(err.Error(), "owner pid: missing") {
		t.Fatalf("diagnostic must say 'owner pid: missing', got: %v", err)
	}
}

// TestTrackerCommit_StaleLockRecovery verifies that a stale lock left behind
// by a crashed prior process (acquired_at older than trackerLockStaleAge,
// pid pointing at a non-existent process) is forcibly broken so a later
// CommitTracker call can proceed.
func TestTrackerCommit_StaleLockRecovery(t *testing.T) {
	root := initTrackerRepo(t)
	tracker := filepath.Join(root, ddxroot.DirName, "beads.jsonl")

	// Simulate a crash: create the lock dir manually with an old timestamp
	// and a pid that does not exist.
	lockDir := trackerLockPath(root)
	require.NoError(t, os.MkdirAll(lockDir, 0o755))
	// PID 0 is reserved and signal(0) returns ESRCH on POSIX; on Windows
	// the age-based fallback handles it.
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "pid"), []byte("0"), 0o644))
	old := time.Now().Add(-2 * trackerLockStaleAge).UTC().Format(time.RFC3339)
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "acquired_at"), []byte(old), 0o644))

	// Make a tracker change so CommitTracker has something to commit.
	require.NoError(t, os.WriteFile(tracker, []byte(`{"id":"ddx-stale-test"}`+"\n"), 0o644))

	// CommitTracker must break the stale lock and succeed.
	if err := commitTrackerForTest(root); err != nil {
		t.Fatalf("CommitTracker failed to recover from stale lock: %v", err)
	}

	// Lock dir must be cleaned up.
	if _, err := os.Stat(lockDir); !os.IsNotExist(err) {
		t.Fatalf("tracker lock dir not cleaned up after stale recovery: stat err = %v", err)
	}

	// And the change must be in HEAD.
	cmd := exec.Command("git", "show", "HEAD:.ddx/beads.jsonl")
	cmd.Dir = root
	cmd.Env = scrubbedGitEnvInteg()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git show HEAD:.ddx/beads.jsonl: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "ddx-stale-test") {
		t.Fatalf("tracker commit did not land: HEAD contents = %q", string(out))
	}
}

// TestTrackerCommit_RetryBackoffPolicy verifies the retry/backoff curve
// owned by this bead (ddx-da11a34a AC#4): exponential growth, capped at
// MaxBackoff, bounded by MaxRetries, and surfacing a timeout error
// (consumed upstream as lock_contention by triage, never a persisted
// status change per TD-031 §8.5).
func TestTrackerCommit_RetryBackoffPolicy(t *testing.T) {
	t.Run("step curve grows exponentially then caps", func(t *testing.T) {
		p := LockRetryPolicy{
			InitialBackoff: 10 * time.Millisecond,
			MaxBackoff:     80 * time.Millisecond,
			Multiplier:     2.0,
			MaxRetries:     10,
		}
		// n=0: 10ms, n=1: 20ms, n=2: 40ms, n=3: 80ms (cap), n=4..: 80ms.
		want := []time.Duration{
			10 * time.Millisecond,
			20 * time.Millisecond,
			40 * time.Millisecond,
			80 * time.Millisecond,
			80 * time.Millisecond,
			80 * time.Millisecond,
		}
		for i, w := range want {
			got := p.step(i)
			if got != w {
				t.Fatalf("step(%d) = %v, want %v", i, got, w)
			}
		}
	})

	t.Run("default policy has at least 3 backoff steps before exhaustion", func(t *testing.T) {
		// AC#1 (sibling triage child): "at least 3 backoff steps before
		// escalation". The lock-level policy must not exhaust before
		// triage gets a chance to escalate.
		p := DefaultLockRetryPolicy()
		if p.MaxRetries < 3 {
			t.Fatalf("DefaultLockRetryPolicy MaxRetries = %d, want >= 3", p.MaxRetries)
		}
		if p.InitialBackoff <= 0 || p.MaxBackoff <= 0 {
			t.Fatalf("DefaultLockRetryPolicy backoff bounds invalid: %+v", p)
		}
		if p.Multiplier < 1 {
			t.Fatalf("DefaultLockRetryPolicy multiplier < 1: %v", p.Multiplier)
		}
	})

	t.Run("max retries exhaustion returns timeout without crashing", func(t *testing.T) {
		root := initTrackerRepo(t)

		// Pre-acquire the lock under a fake live PID (current process)
		// so breakStaleTrackerLock cannot reclaim it.
		lockDir := trackerLockPath(root)
		require.NoError(t, os.MkdirAll(lockDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, "pid"),
			[]byte(fmt.Sprintf("%d", os.Getpid())), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, "acquired_at"),
			[]byte(time.Now().UTC().Format(time.RFC3339)), 0o644))
		defer os.RemoveAll(lockDir)

		policy := LockRetryPolicy{
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff:     2 * time.Millisecond,
			Multiplier:     2.0,
			MaxRetries:     3,
			MaxElapsed:     1 * time.Second,
		}

		called := false
		err := withTrackerLockPolicy(root, policy, func() error {
			called = true
			return nil
		})
		if err == nil {
			t.Fatalf("expected timeout error, got nil")
		}
		if called {
			t.Fatalf("fn must not run when lock acquisition exhausts retries")
		}
		if !strings.Contains(err.Error(), "tracker lock timeout") {
			t.Fatalf("expected tracker lock timeout error, got: %v", err)
		}
	})

	t.Run("retry succeeds when contender releases mid-curve", func(t *testing.T) {
		root := initTrackerRepo(t)
		tracker := filepath.Join(root, ddxroot.DirName, "beads.jsonl")
		require.NoError(t, os.WriteFile(tracker, []byte(`{"id":"ddx-retry-test"}`+"\n"), 0o644))

		// Hold the lock for ~50ms then release.
		lockDir := trackerLockPath(root)
		require.NoError(t, os.MkdirAll(lockDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, "pid"),
			[]byte(fmt.Sprintf("%d", os.Getpid())), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, "acquired_at"),
			[]byte(time.Now().UTC().Format(time.RFC3339)), 0o644))

		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = os.RemoveAll(lockDir)
		}()

		// Default policy should easily ride out a 50ms hold.
		if err := commitTrackerForTest(root); err != nil {
			t.Fatalf("CommitTracker did not retry through transient contention: %v", err)
		}
	})
}

// TestTrackerLock_RecordsRetryCount asserts that TrackerLockMetricsSink
// captures a non-zero retry count under simulated lock contention.
func TestTrackerLock_RecordsRetryCount(t *testing.T) {
	root := initTrackerRepo(t)

	var mu sync.Mutex
	var captured []TrackerLockSample
	prev := TrackerLockMetricsSink
	TrackerLockMetricsSink = func(s TrackerLockSample) {
		mu.Lock()
		captured = append(captured, s)
		mu.Unlock()
	}
	defer func() { TrackerLockMetricsSink = prev }()

	// Pre-acquire the lock with the current PID so breakStaleTrackerLock
	// cannot reclaim it.
	lockDir := trackerLockPath(root)
	require.NoError(t, os.MkdirAll(lockDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "pid"),
		[]byte(fmt.Sprintf("%d", os.Getpid())), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "acquired_at"),
		[]byte(time.Now().UTC().Format(time.RFC3339)), 0o644))

	// Release the lock after 30ms to allow at least a few retry iterations.
	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = os.RemoveAll(lockDir)
	}()

	policy := LockRetryPolicy{
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     2.0,
		MaxRetries:     200,
		MaxElapsed:     5 * time.Second,
	}
	err := withTrackerLockPolicy(root, policy, func() error { return nil })
	require.NoError(t, err)

	mu.Lock()
	got := append([]TrackerLockSample(nil), captured...)
	mu.Unlock()

	if len(got) == 0 {
		t.Fatal("TrackerLockMetricsSink was not called")
	}
	if got[0].Retries == 0 {
		t.Fatalf("expected Retries > 0 for contended lock, got %d", got[0].Retries)
	}
	if got[0].Wait < 5*time.Millisecond {
		t.Fatalf("expected Wait >= 5ms, got %v", got[0].Wait)
	}
	if got[0].LockDir != lockDir {
		t.Fatalf("LockDir = %q, want %q", got[0].LockDir, lockDir)
	}
}
