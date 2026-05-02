package bead

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// breakStaleLock checks if the existing lock is stale (owner dead or too old)
// and breaks it if so. Returns true if lock was broken.
func (s *Store) breakStaleLock() bool {
	return breakStaleLockDir(s.LockDir)
}

func breakStaleLockDir(lockDir string) bool {
	pidData, err := os.ReadFile(filepath.Join(lockDir, "pid"))
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if err == nil && pid > 0 && pid != os.Getpid() {
			if !processAlive(pid) {
				os.RemoveAll(lockDir)
				return true
			}
		}
	}

	acquiredData, err := os.ReadFile(filepath.Join(lockDir, "acquired_at"))
	if err == nil {
		acquired, err := time.Parse(time.RFC3339, strings.TrimSpace(string(acquiredData)))
		if err == nil && time.Since(acquired) > StaleLockAge {
			os.RemoveAll(lockDir)
			return true
		}
	}

	return false
}

func (s *Store) releaseLock() {
	os.RemoveAll(s.LockDir)
}
