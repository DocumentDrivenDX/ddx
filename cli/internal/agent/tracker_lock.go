package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// trackerLockStaleAge is the age after which a held tracker lock is considered
// stale (owner crashed mid-commit) and may be forcibly broken.
var trackerLockStaleAge = 10 * time.Minute

// trackerLockWait bounds how long withTrackerLock will spin waiting for the
// lock before returning an error.
var trackerLockWait = 30 * time.Second

// trackerLockPath returns the tracker-commit lock directory for the given
// project root. The lock guards git index operations on the primary .git
// (staging and committing .ddx/beads.jsonl) so that multiple concurrent
// workers (e.g. several `ddx work --local` processes) do not race on
// .git/index.lock.
func trackerLockPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".ddx", ".git-tracker.lock")
}

// withTrackerLock acquires the process-shared tracker-commit lock for the
// given project root, runs fn, and releases the lock. The lock is a
// directory created via os.Mkdir (atomic across processes on POSIX and
// Windows) following the same pattern as cli/internal/bead/lock.go.
func withTrackerLock(projectRoot string, fn func() error) error {
	lockDir := trackerLockPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(lockDir), 0o755); err != nil {
		return fmt.Errorf("tracker lock dir: %w", err)
	}

	deadline := time.Now().Add(trackerLockWait)
	for {
		err := os.Mkdir(lockDir, 0o755)
		if err == nil {
			_ = os.WriteFile(filepath.Join(lockDir, "pid"),
				[]byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
			_ = os.WriteFile(filepath.Join(lockDir, "acquired_at"),
				[]byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
			break
		}

		if breakStaleTrackerLock(lockDir) {
			continue
		}

		if time.Now().After(deadline) {
			owner := "unknown"
			if pidData, perr := os.ReadFile(filepath.Join(lockDir, "pid")); perr == nil && len(pidData) > 0 {
				owner = strings.TrimSpace(string(pidData))
			}
			return fmt.Errorf("tracker lock timeout (owner pid: %s)", owner)
		}
		time.Sleep(50 * time.Millisecond)
	}

	defer os.RemoveAll(lockDir)
	return fn()
}

// breakStaleTrackerLock removes lockDir if its owner process is dead or the
// lock is older than trackerLockStaleAge. Returns true if the lock was broken.
func breakStaleTrackerLock(lockDir string) bool {
	pidData, err := os.ReadFile(filepath.Join(lockDir, "pid"))
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if err == nil && pid > 0 && pid != os.Getpid() {
			if !trackerProcessAlive(pid) {
				os.RemoveAll(lockDir)
				return true
			}
		}
	}

	acquiredData, err := os.ReadFile(filepath.Join(lockDir, "acquired_at"))
	if err == nil {
		acquired, err := time.Parse(time.RFC3339, strings.TrimSpace(string(acquiredData)))
		if err == nil && time.Since(acquired) > trackerLockStaleAge {
			os.RemoveAll(lockDir)
			return true
		}
	}

	return false
}
