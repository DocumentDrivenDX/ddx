package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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

	ddxDir := filepath.Join(root, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(""), 0o644))
	run("add", ".ddx/beads.jsonl")
	run("commit", "-m", "chore: seed tracker")

	return root
}

// TestTrackerCommit_ConcurrentSafety verifies that two goroutines invoking
// CommitTracker against the same primary .git do not race on .git/index.lock.
// Without withTrackerLock around the git add/commit pair, one of the two
// goroutines would fail with: "fatal: Unable to create '.../.git/index.lock':
// File exists." (See bead description for the observed Phase 2 drain failure.)
func TestTrackerCommit_ConcurrentSafety(t *testing.T) {
	root := initTrackerRepo(t)
	tracker := filepath.Join(root, ".ddx", "beads.jsonl")

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

				if err := CommitTracker(root); err != nil {
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

// TestTrackerCommit_StaleLockRecovery verifies that a stale lock left behind
// by a crashed prior process (acquired_at older than trackerLockStaleAge,
// pid pointing at a non-existent process) is forcibly broken so a later
// CommitTracker call can proceed.
func TestTrackerCommit_StaleLockRecovery(t *testing.T) {
	root := initTrackerRepo(t)
	tracker := filepath.Join(root, ".ddx", "beads.jsonl")

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
	if err := CommitTracker(root); err != nil {
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
		tracker := filepath.Join(root, ".ddx", "beads.jsonl")
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
		if err := CommitTracker(root); err != nil {
			t.Fatalf("CommitTracker did not retry through transient contention: %v", err)
		}
	})
}
