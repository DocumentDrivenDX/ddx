package bead

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// StaleLockAge is the duration after which a lock is considered stale
// and can be forcibly broken. Default: 2 hours.
var StaleLockAge = 2 * time.Hour

// WithLock acquires the file lock, runs fn, then releases the lock.
// For external backends, locking is delegated to the backend tool.
func (s *Store) WithLock(fn func() error) error {
	if s.backend != nil {
		return s.backend.WithLock(fn)
	}
	if err := s.acquireLock(); err != nil {
		return err
	}
	defer s.releaseLock()
	return fn()
}

func (s *Store) acquireLock() error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("bead: lock dir: %w", err)
	}

	deadline := time.Now().Add(s.LockWait)
	for {
		err := os.Mkdir(s.LockDir, 0o755)
		if err == nil {
			// Write PID and acquired_at
			os.WriteFile(filepath.Join(s.LockDir, "pid"),
				[]byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
			os.WriteFile(filepath.Join(s.LockDir, "acquired_at"),
				[]byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
			return nil
		}

		// Check for stale lock
		if s.breakStaleLock() {
			continue // retry immediately after breaking
		}

		if time.Now().After(deadline) {
			owner := "unknown"
			pidData, _ := os.ReadFile(filepath.Join(s.LockDir, "pid"))
			if len(pidData) > 0 {
				owner = strings.TrimSpace(string(pidData))
			}
			return fmt.Errorf("bead: lock timeout (owner pid: %s)", owner)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// breakStaleLock checks if the existing lock is stale (owner dead or too old)
// and breaks it if so. Returns true if lock was broken.
func (s *Store) breakStaleLock() bool {
	// Check if owner PID is still alive (skip if it's our own PID)
	pidData, err := os.ReadFile(filepath.Join(s.LockDir, "pid"))
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if err == nil && pid > 0 && pid != os.Getpid() {
			// syscall.Kill with signal 0 checks process existence.
			// ESRCH = no such process (dead). EPERM = exists but different user (alive).
			killErr := syscall.Kill(pid, 0)
			if killErr == syscall.ESRCH {
				// Process is dead — break the lock
				os.RemoveAll(s.LockDir)
				return true
			}
			// EPERM or nil means process is alive — don't break
		}
	}

	// Check age
	acquiredData, err := os.ReadFile(filepath.Join(s.LockDir, "acquired_at"))
	if err == nil {
		acquired, err := time.Parse(time.RFC3339, strings.TrimSpace(string(acquiredData)))
		if err == nil && time.Since(acquired) > StaleLockAge {
			os.RemoveAll(s.LockDir)
			return true
		}
	}

	return false
}

func (s *Store) releaseLock() {
	os.RemoveAll(s.LockDir)
}
