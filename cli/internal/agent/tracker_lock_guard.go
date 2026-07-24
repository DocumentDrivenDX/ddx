package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// trackerStaleLockGuardStage names internal stages of the main-git stale-break
// transition guard. Tests use these stages as deterministic barriers; production
// observers are optional.
type trackerStaleLockGuardStage string

const (
	trackerStaleGuardStageAttempted    trackerStaleLockGuardStage = "attempted"
	trackerStaleGuardStageContended    trackerStaleLockGuardStage = "contended"
	trackerStaleGuardStageAcquired     trackerStaleLockGuardStage = "acquired"
	trackerStaleGuardStageBeforeRename trackerStaleLockGuardStage = "before-rename"
)

// trackerStaleLockTransitionGuard is a crash-safe cross-process advisory lock
// held only for the short window of a stale-break (or token-safe release)
// transition. The OS releases the advisory lock automatically when the process
// dies or the handle is closed. Ordinary Acquire/Release never delete, truncate,
// rename, or recreate the sidecar inode — only open/lock/unlock/close.
type trackerStaleLockTransitionGuard struct {
	file  *os.File
	mutex *sync.Mutex
}

var trackerStaleLockGuardMutexes sync.Map

// trackerStaleLockBreakGuardPath returns the deterministic never-deleted sibling
// advisory-lock path adjacent to the canonical main-git lock directory. The
// name always ends in ".lock".
func trackerStaleLockBreakGuardPath(lockDir string) string {
	if lockDir == "" {
		return ""
	}
	return lockDir + ".stale-break.lock"
}

// trackerStaleLockGuardMutex returns the process-local mutex for guardPath so
// two goroutines in one process serialize before touching the shared fd.
// Keys are resolved absolute paths when available.
func trackerStaleLockGuardMutex(guardPath string) *sync.Mutex {
	mutex := &sync.Mutex{}
	actual, _ := trackerStaleLockGuardMutexes.LoadOrStore(guardPath, mutex)
	return actual.(*sync.Mutex)
}

// tryAcquireTrackerStaleLockTransitionGuard is the non-observed form used by
// tests and call sites that only need success/failure.
func tryAcquireTrackerStaleLockTransitionGuard(lockDir string) (*trackerStaleLockTransitionGuard, bool, error) {
	return tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir, nil)
}

// tryAcquireTrackerStaleLockTransitionGuardObserved opens the stable sidecar,
// acquires an exclusive OS advisory lock non-blockingly, and returns the held
// guard. Contention returns (nil, false, nil). Non-contention open/lock errors
// fail safe: they return an error and never report the guard as held.
func tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir string, observer func(trackerStaleLockGuardStage)) (*trackerStaleLockTransitionGuard, bool, error) {
	guardPath := trackerStaleLockBreakGuardPath(lockDir)
	if guardPath == "" {
		return nil, false, fmt.Errorf("tracker lock: stale-break guard requires lock directory")
	}
	resolved := guardPath
	if abs, err := filepath.Abs(guardPath); err == nil {
		resolved = abs
	}
	if observer != nil {
		observer(trackerStaleGuardStageAttempted)
	}
	mutex := trackerStaleLockGuardMutex(resolved)
	if !mutex.TryLock() {
		if observer != nil {
			observer(trackerStaleGuardStageContended)
		}
		return nil, false, nil
	}
	// Ensure the parent exists so O_CREATE can materialize the sidecar file.
	// Never delete or recreate an existing sidecar inode.
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		mutex.Unlock()
		return nil, false, err
	}
	guard, err := os.OpenFile(resolved, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		mutex.Unlock()
		return nil, false, err
	}
	locked, err := tryLockTrackerStaleBreakGuardFile(guard)
	if err != nil {
		_ = guard.Close()
		mutex.Unlock()
		return nil, false, err
	}
	if !locked {
		_ = guard.Close()
		mutex.Unlock()
		if observer != nil {
			observer(trackerStaleGuardStageContended)
		}
		return nil, false, nil
	}
	if observer != nil {
		observer(trackerStaleGuardStageAcquired)
	}
	return &trackerStaleLockTransitionGuard{file: guard, mutex: mutex}, true, nil
}

// acquireTrackerStaleLockTransitionGuardObserved waits up to guardWait for the
// stable transition guard. Used by token-safe lease release so contending
// stale-break and release paths serialize without unconditional RemoveAll.
func acquireTrackerStaleLockTransitionGuardObserved(lockDir string, guardWait time.Duration, observer func(trackerStaleLockGuardStage)) (*trackerStaleLockTransitionGuard, error) {
	if guardWait < 0 {
		guardWait = 0
	}
	deadline := time.Now().Add(guardWait)
	for {
		guard, acquired, err := tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir, observer)
		if err != nil {
			return nil, err
		}
		if acquired {
			return guard, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("stale-break guard timeout after %s", guardWait)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// releaseTrackerStaleLockBreakGuard unlocks and closes the advisory sidecar
// handle, then releases the process-local mutex. It never deletes, truncates,
// renames, or recreates the sidecar path.
func releaseTrackerStaleLockBreakGuard(guard *trackerStaleLockTransitionGuard) error {
	if guard == nil {
		return nil
	}
	var err error
	if guard.file != nil {
		err = errors.Join(unlockTrackerStaleBreakGuardFile(guard.file), guard.file.Close())
	}
	if guard.mutex != nil {
		guard.mutex.Unlock()
	}
	return err
}
