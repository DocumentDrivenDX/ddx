package bead

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// StaleLockAge is the duration after which a lock is considered stale
// and can be forcibly broken. Default: 2 hours.
var StaleLockAge = 2 * time.Hour

var staleLockTombstoneSeq uint64

// LockSample carries timing metrics for one Store.WithLock acquire/release cycle.
type LockSample struct {
	LockDir string
	Wait    time.Duration // time from WithLock entry to lock acquisition
	Hold    time.Duration // time the callback held the lock
}

// LockMetricsSink is called after each successful Store.WithLock acquire+release
// cycle. The zero value (nil) is a no-op. Tests may swap this to capture metrics.
var LockMetricsSink func(LockSample)

// WithLock acquires the file lock, runs fn, then releases the lock.
// For external backends, locking is delegated to the backend tool.
func (s *Store) WithLock(fn func() error) error {
	if s.backend != nil {
		return s.backend.WithLock(fn)
	}
	waitStart := time.Now()
	if err := s.acquireLock(); err != nil {
		return err
	}
	waitDur := time.Since(waitStart)
	holdStart := time.Now()
	defer s.releaseLock()
	fnErr := fn()
	if sink := LockMetricsSink; sink != nil {
		sink(LockSample{LockDir: s.LockDir, Wait: waitDur, Hold: time.Since(holdStart)})
	}
	return fnErr
}

func (s *Store) acquireLock() error {
	return acquireDirLock(s.Dir, s.LockDir, s.LockWait)
}

// acquireDirLock is the file-lock primitive used by both Store and the
// standalone JSONLBackend. dir is the directory to ensure exists before the
// lock is taken; lockDir is the lock directory itself; wait bounds the spin.
func acquireDirLock(dir, lockDir string, wait time.Duration) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("bead: lock dir: %w", err)
	}

	deadline := time.Now().Add(wait)
	for {
		err := os.Mkdir(lockDir, 0o755)
		if err == nil {
			_ = os.WriteFile(filepath.Join(lockDir, "pid"),
				[]byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
			_ = os.WriteFile(filepath.Join(lockDir, "acquired_at"),
				[]byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
			return nil
		}

		if breakStaleLockDir(lockDir) {
			continue
		}

		if time.Now().After(deadline) {
			owner := "unknown"
			pidData, _ := os.ReadFile(filepath.Join(lockDir, "pid"))
			if len(pidData) > 0 {
				owner = strings.TrimSpace(string(pidData))
			}
			return fmt.Errorf("bead: lock timeout (owner pid: %s)", owner)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// breakStaleLockDir checks if the existing lock is stale (owner dead or too
// old) and breaks it if so. Returns true if lock was broken.
func breakStaleLockDir(lockDir string) bool {
	pidData, err := os.ReadFile(filepath.Join(lockDir, "pid"))
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if err == nil && pid > 0 && pid != os.Getpid() {
			if !processAlive(pid) {
				return renameStaleLockDir(lockDir)
			}
		}
	}

	acquiredData, err := os.ReadFile(filepath.Join(lockDir, "acquired_at"))
	if err == nil {
		acquired, err := time.Parse(time.RFC3339, strings.TrimSpace(string(acquiredData)))
		if err == nil && time.Since(acquired) > StaleLockAge {
			return renameStaleLockDir(lockDir)
		}
	}

	return false
}

func renameStaleLockDir(lockDir string) bool {
	tombstoneDir := tombstoneLockDir(lockDir)
	if tombstoneDir == "" {
		return false
	}
	if err := os.Rename(lockDir, tombstoneDir); err != nil {
		return false
	}
	_ = os.RemoveAll(tombstoneDir)
	return true
}

func tombstoneLockDir(lockDir string) string {
	if lockDir == "" {
		return ""
	}
	suffix := fmt.Sprintf("%d-%d-%d", os.Getpid(), time.Now().UnixNano(), atomic.AddUint64(&staleLockTombstoneSeq, 1))
	return filepath.Join(filepath.Dir(lockDir), filepath.Base(lockDir)+".tombstone."+suffix)
}

func (s *Store) releaseLock() {
	os.RemoveAll(s.LockDir)
}
