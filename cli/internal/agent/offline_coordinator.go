package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// offlineCoordinationLockDirName is the project-scoped lock directory that
// serializes offline coordination mutations (claims, tracker transitions,
// journal appends, local landing) across independent CLI processes.
//
// ADR-022 rev 6: disconnected workers perform the same coordination mutations
// locally under a cross-process project coordination lock.
const offlineCoordinationLockDirName = "offline-coordination.lock"

// DefaultOfflineCoordinationLockWait bounds how long WithLock will spin
// waiting for another process to release the project coordination lock.
const DefaultOfflineCoordinationLockWait = 30 * time.Second

// offlineCoordinationLockStaleAge is the age after which a held offline
// coordination lock may be forcibly broken when the owner PID is dead or
// the acquisition timestamp is older than this threshold.
var offlineCoordinationLockStaleAge = 10 * time.Minute

// OfflineCoordinator serializes offline/manual coordination mutations for one
// project root under a durable cross-process lock. Connected coordination
// uses the server; this type owns the disconnected mutation window.
//
// Construction is cheap; the lock is held only for the duration of WithLock.
// Do not hold the lock across LLM/harness waits (same lifetime rule as the
// git index and tracker locks — this lock is a separate project-scoped
// coordination primitive and does not change those contracts).
type OfflineCoordinator struct {
	projectRoot string
	// Wait bounds lock acquisition. Zero means DefaultOfflineCoordinationLockWait.
	Wait time.Duration
}

// NewOfflineCoordinator returns a coordinator scoped to projectRoot. The
// project root must be non-empty; callers supply the same root used by
// ddx try / ddx work for that working tree.
func NewOfflineCoordinator(projectRoot string) *OfflineCoordinator {
	return &OfflineCoordinator{projectRoot: projectRoot}
}

// OfflineCoordinationLockPath returns the durable project-scoped lock path
// used by OfflineCoordinator.WithLock. Tests and production share this path
// so serialization proofs exercise the same filesystem object workers use.
// Production local land coordination (NewLocalLandCoordinator) acquires this
// same path around agent.Land mutation windows.
func OfflineCoordinationLockPath(projectRoot string) string {
	return ddxroot.JoinProject(projectRoot, offlineCoordinationLockDirName)
}

// WithLock acquires the project offline coordination lock, runs fn, and
// releases the lock. Only one process across the host may hold the lock for
// a given project root at a time. fn is the protected mutation window
// (claims, tracker transitions, journal writes, local landing).
//
// The lock is never held across an LLM subprocess: callers must invoke
// WithLock only around short coordination mutations.
func (c *OfflineCoordinator) WithLock(ctx context.Context, fn func() error) error {
	if c == nil {
		return fmt.Errorf("offline coordinator: nil receiver")
	}
	if strings.TrimSpace(c.projectRoot) == "" {
		return fmt.Errorf("offline coordinator: project root is empty")
	}
	if fn == nil {
		return fmt.Errorf("offline coordinator: fn is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	wait := c.Wait
	if wait <= 0 {
		wait = DefaultOfflineCoordinationLockWait
	}

	release, err := acquireOfflineCoordinationLock(ctx, c.projectRoot, wait)
	if err != nil {
		return err
	}
	defer release()

	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}

func acquireOfflineCoordinationLock(ctx context.Context, projectRoot string, wait time.Duration) (func(), error) {
	lockDir := OfflineCoordinationLockPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(lockDir), 0o755); err != nil {
		return nil, fmt.Errorf("offline coordination lock: parent dir: %w", err)
	}

	deadline := time.Now().Add(wait)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		err := os.Mkdir(lockDir, 0o755)
		if err == nil {
			_ = os.WriteFile(filepath.Join(lockDir, "pid"),
				[]byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
			_ = os.WriteFile(filepath.Join(lockDir, "acquired_at"),
				[]byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o644)
			return func() { _ = os.RemoveAll(lockDir) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("offline coordination lock: mkdir: %w", err)
		}

		if breakStaleOfflineCoordinationLock(lockDir) {
			continue
		}

		if time.Now().After(deadline) {
			owner := "unknown"
			if pidData, readErr := os.ReadFile(filepath.Join(lockDir, "pid")); readErr == nil && len(pidData) > 0 {
				owner = strings.TrimSpace(string(pidData))
			}
			return nil, fmt.Errorf("offline coordination lock: timeout waiting for project lock (owner pid: %s path: %s)", owner, lockDir)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// breakStaleOfflineCoordinationLock removes lockDir when the owner process is
// dead or the lock is older than offlineCoordinationLockStaleAge. Returns true
// if the lock was broken.
func breakStaleOfflineCoordinationLock(lockDir string) bool {
	pidData, err := os.ReadFile(filepath.Join(lockDir, "pid"))
	if err == nil {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if parseErr == nil && pid > 0 && pid != os.Getpid() {
			if !trackerProcessAlive(pid) {
				_ = os.RemoveAll(lockDir)
				return true
			}
		}
	}

	acquiredData, err := os.ReadFile(filepath.Join(lockDir, "acquired_at"))
	if err == nil {
		// Accept both RFC3339 and RFC3339Nano.
		acquired, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(acquiredData)))
		if parseErr != nil {
			acquired, parseErr = time.Parse(time.RFC3339, strings.TrimSpace(string(acquiredData)))
		}
		if parseErr == nil && time.Since(acquired) > offlineCoordinationLockStaleAge {
			_ = os.RemoveAll(lockDir)
			return true
		}
	}
	return false
}
