package bead

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WithLock acquires the file lock, runs fn, then releases the lock.
func (s *Store) WithLock(fn func() error) error {
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
			// Write PID file
			pidFile := filepath.Join(s.LockDir, "pid")
			os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
			return nil
		}

		if time.Now().After(deadline) {
			owner := "unknown"
			pidData, err := os.ReadFile(filepath.Join(s.LockDir, "pid"))
			if err == nil {
				owner = string(pidData)
			}
			return fmt.Errorf("bead: lock timeout (owner pid: %s)", owner)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (s *Store) releaseLock() {
	os.RemoveAll(s.LockDir)
}
