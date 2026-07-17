package bead

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// StaleLockAge is the duration after which a lock is considered stale
// and can be forcibly broken. Default: 2 hours.
var StaleLockAge = 2 * time.Hour

var staleLockTombstoneSeq atomic.Uint64

const collectionLockOwnerTokenFile = "owner_token"

type dirLockLease struct {
	lockDir    string
	ownerToken string
	guardWait  time.Duration
}

func (l dirLockLease) Release() error {
	return releaseDirLockObserved(l.lockDir, l.ownerToken, l.guardWait, nil)
}

func (l dirLockLease) releaseObserved(observer func(staleLockGuardStage)) error {
	return releaseDirLockObserved(l.lockDir, l.ownerToken, l.guardWait, observer)
}

type staleLockTransitionGuard struct {
	file  *os.File
	mutex *sync.Mutex
}

type staleLockGuardStage string

const (
	staleLockGuardStageAttempted    staleLockGuardStage = "attempted"
	staleLockGuardStageContended    staleLockGuardStage = "contended"
	staleLockGuardStageAcquired     staleLockGuardStage = "acquired"
	staleLockGuardStageBeforeRename staleLockGuardStage = "before-rename"
)

var staleLockGuardMutexes sync.Map

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
func (s *Store) WithLock(fn func() error) (err error) {
	if s.backend != nil {
		return s.backend.WithLock(fn)
	}
	waitStart := time.Now()
	lease, err := s.acquireLock()
	if err != nil {
		return err
	}
	waitDur := time.Since(waitStart)
	holdStart := time.Now()
	defer func() {
		err = errors.Join(err, lease.Release())
	}()
	err = fn()
	if sink := LockMetricsSink; sink != nil {
		sink(LockSample{LockDir: s.LockDir, Wait: waitDur, Hold: time.Since(holdStart)})
	}
	return err
}

func (s *Store) acquireLock() (dirLockLease, error) {
	return acquireDirLock(s.Dir, s.LockDir, s.LockWait)
}

// acquireDirLock is the file-lock primitive used by both Store and the
// standalone JSONLBackend. dir is the directory to ensure exists before the
// lock is taken; lockDir is the lock directory itself; wait bounds the spin.
func acquireDirLock(dir, lockDir string, wait time.Duration) (dirLockLease, error) {
	return acquireDirLockWithMetadataWriter(dir, lockDir, wait, writeCollectionLockMetadata)
}

type collectionLockMetadataWriter func(lockDir string, lease dirLockLease) error

func acquireDirLockWithMetadataWriter(dir, lockDir string, wait time.Duration, writeMetadata collectionLockMetadataWriter) (dirLockLease, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return dirLockLease{}, fmt.Errorf("bead: lock dir: %w", err)
	}

	ownerToken, err := newCollectionLockOwnerToken()
	if err != nil {
		return dirLockLease{}, fmt.Errorf("bead: lock owner token: %w", err)
	}
	// Store the same effective budget used by acquisition in the lease. In
	// particular, a caller-supplied zero or negative wait means one immediate
	// attempt; guarded release must not silently expand that budget to 10s.
	effectiveWait := wait
	if effectiveWait < 0 {
		effectiveWait = 0
	}
	lease := dirLockLease{lockDir: lockDir, ownerToken: ownerToken, guardWait: effectiveWait}

	deadline := time.Now().Add(effectiveWait)
	for {
		err := os.Mkdir(lockDir, 0o755)
		if err == nil {
			if err := writeMetadata(lockDir, lease); err != nil {
				// Once owner_token is durable enough to read, guarded release can
				// safely clean this partial acquisition. If writing the token itself
				// failed, leave the malformed canonical directory in place rather
				// than risk deleting a replacement after an unguarded cleanup race.
				return dirLockLease{}, errors.Join(err, lease.Release())
			}
			return lease, nil
		}

		broke, breakErr := breakStaleLockDir(lockDir)
		if breakErr != nil {
			return dirLockLease{}, fmt.Errorf("bead: break stale lock: %w", breakErr)
		}
		if broke {
			continue
		}

		if time.Now().After(deadline) {
			owner := "unknown"
			pidData, _ := os.ReadFile(filepath.Join(lockDir, "pid"))
			if len(pidData) > 0 {
				owner = strings.TrimSpace(string(pidData))
			}
			return dirLockLease{}, fmt.Errorf("bead: lock timeout (owner pid: %s)", owner)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func newCollectionLockOwnerToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func writeCollectionLockMetadata(lockDir string, lease dirLockLease) error {
	if lease.ownerToken == "" {
		return fmt.Errorf("bead: lock owner token is empty")
	}
	if err := os.WriteFile(filepath.Join(lockDir, collectionLockOwnerTokenFile), []byte(lease.ownerToken), 0o600); err != nil {
		return fmt.Errorf("bead: write lock owner token: %w", err)
	}
	if err := os.WriteFile(filepath.Join(lockDir, "pid"), []byte(fmt.Sprintf("%d", os.Getpid())), 0o644); err != nil {
		return fmt.Errorf("bead: write lock pid: %w", err)
	}
	if err := os.WriteFile(filepath.Join(lockDir, "acquired_at"), []byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
		return fmt.Errorf("bead: write lock acquired_at: %w", err)
	}
	return nil
}

// breakStaleLockDir serializes stale inspection and canonical-path mutation
// across processes. The stable advisory guard is intentionally separate from
// the canonical directory: ordinary lock acquisition and release never remove
// or replace its inode, and a crashed breaker releases the advisory lock when
// the process closes its file descriptors.
func breakStaleLockDir(lockDir string) (bool, error) {
	return breakStaleLockDirObserved(lockDir, nil)
}

func breakStaleLockDirObserved(lockDir string, observer func(staleLockGuardStage)) (bool, error) {
	guard, acquired, err := tryAcquireStaleLockTransitionGuardObserved(lockDir, observer)
	if err != nil {
		return false, err
	}
	if !acquired {
		return false, nil
	}

	// No observation made before taking guard can authorize this rename. The
	// canonical directory and its metadata are inspected again under guard,
	// and only the unique tombstone returned by that guarded transition is
	// eligible for removal.
	tombstoneDir, broke, renameErr := renameFreshlyStaleLockDirObserved(lockDir, observer)
	guardErr := releaseStaleLockBreakGuard(guard)
	if renameErr != nil {
		return false, errors.Join(renameErr, guardErr)
	}
	if guardErr != nil {
		return broke, guardErr
	}
	if !broke {
		return false, nil
	}
	if err := os.RemoveAll(tombstoneDir); err != nil {
		return true, fmt.Errorf("remove stale lock tombstone %s: %w", tombstoneDir, err)
	}
	return true, nil
}

// renameFreshlyStaleLockDir must only be called while the caller owns the
// stable stale-break guard for lockDir. It returns the unique tombstone owned
// by this transition; callers must never remove the canonical lock directory.
func renameFreshlyStaleLockDir(lockDir string) (string, bool, error) {
	return renameFreshlyStaleLockDirObserved(lockDir, nil)
}

func renameFreshlyStaleLockDirObserved(lockDir string, observer func(staleLockGuardStage)) (string, bool, error) {
	inspected, stale := freshlyInspectStaleLockDir(lockDir)
	if !stale {
		return "", false, nil
	}
	if observer != nil {
		observer(staleLockGuardStageBeforeRename)
	}

	// Revalidate after the pre-rename stage. Tests pause at that exact stage to
	// install a fresh canonical owner; the only safe response is to decline the
	// rename when the directory identity changed after stale classification.
	current, err := os.Lstat(lockDir)
	if err != nil || !current.IsDir() || !os.SameFile(inspected, current) {
		return "", false, nil
	}

	tombstoneDir := staleLockTombstoneDir(lockDir)
	if tombstoneDir == "" {
		return "", false, nil
	}
	if err := os.Rename(lockDir, tombstoneDir); err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return tombstoneDir, true, nil
}

// freshlyInspectStaleLockDir evaluates the two legacy stale criteria from the
// canonical directory while its caller owns the stale-break guard. The
// criteria intentionally remain ORed: either a valid dead owner PID or a valid
// over-age acquired_at timestamp is enough. Missing or malformed metadata does
// not itself authorize removal.
func freshlyInspectStaleLockDir(lockDir string) (os.FileInfo, bool) {
	inspected, err := os.Lstat(lockDir)
	if err != nil || !inspected.IsDir() {
		return nil, false
	}

	stale := false
	pidData, err := os.ReadFile(filepath.Join(lockDir, "pid"))
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if err == nil && pid > 0 && pid != os.Getpid() {
			if !processAlive(pid) {
				stale = true
			}
		}
	}

	acquiredData, err := os.ReadFile(filepath.Join(lockDir, "acquired_at"))
	if err == nil {
		acquired, err := time.Parse(time.RFC3339, strings.TrimSpace(string(acquiredData)))
		if err == nil && time.Since(acquired) > StaleLockAge {
			stale = true
		}
	}

	current, err := os.Lstat(lockDir)
	if err != nil || !current.IsDir() || !os.SameFile(inspected, current) {
		return nil, false
	}
	return inspected, stale
}

func staleLockBreakGuardPath(lockDir string) string {
	if lockDir == "" {
		return ""
	}
	return lockDir + ".stale-break.lock"
}

func tryAcquireStaleLockBreakGuard(lockDir string) (*staleLockTransitionGuard, bool) {
	guard, acquired, _ := tryAcquireStaleLockTransitionGuard(lockDir)
	return guard, acquired
}

func tryAcquireStaleLockTransitionGuard(lockDir string) (*staleLockTransitionGuard, bool, error) {
	return tryAcquireStaleLockTransitionGuardObserved(lockDir, nil)
}

func tryAcquireStaleLockTransitionGuardObserved(lockDir string, observer func(staleLockGuardStage)) (*staleLockTransitionGuard, bool, error) {
	guardPath := staleLockBreakGuardPath(lockDir)
	if guardPath == "" {
		return nil, false, fmt.Errorf("bead: stale-break guard requires lock directory")
	}
	if observer != nil {
		observer(staleLockGuardStageAttempted)
	}
	mutex := staleLockGuardMutex(guardPath)
	if !mutex.TryLock() {
		if observer != nil {
			observer(staleLockGuardStageContended)
		}
		return nil, false, nil
	}
	guard, err := os.OpenFile(guardPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		mutex.Unlock()
		return nil, false, err
	}
	locked, err := tryLockStaleBreakGuardFile(guard)
	if err != nil {
		_ = guard.Close()
		mutex.Unlock()
		return nil, false, err
	}
	if !locked {
		_ = guard.Close()
		mutex.Unlock()
		if observer != nil {
			observer(staleLockGuardStageContended)
		}
		return nil, false, nil
	}
	if observer != nil {
		observer(staleLockGuardStageAcquired)
	}
	return &staleLockTransitionGuard{file: guard, mutex: mutex}, true, nil
}

func acquireStaleLockTransitionGuard(lockDir string, wait time.Duration) (*staleLockTransitionGuard, error) {
	return acquireStaleLockTransitionGuardObserved(lockDir, wait, nil)
}

func acquireStaleLockTransitionGuardObserved(lockDir string, wait time.Duration, observer func(staleLockGuardStage)) (*staleLockTransitionGuard, error) {
	if wait < 0 {
		wait = 0
	}
	deadline := time.Now().Add(wait)
	for {
		guard, acquired, err := tryAcquireStaleLockTransitionGuardObserved(lockDir, observer)
		if err != nil {
			return nil, err
		}
		if acquired {
			return guard, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("bead: stale-break guard timeout after %s", wait)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func staleLockGuardMutex(guardPath string) *sync.Mutex {
	mutex := &sync.Mutex{}
	actual, _ := staleLockGuardMutexes.LoadOrStore(guardPath, mutex)
	return actual.(*sync.Mutex)
}

func releaseStaleLockBreakGuard(guard *staleLockTransitionGuard) error {
	if guard == nil {
		return nil
	}
	var err error
	if guard.file != nil {
		err = errors.Join(unlockStaleBreakGuardFile(guard.file), guard.file.Close())
	}
	if guard.mutex != nil {
		guard.mutex.Unlock()
	}
	return err
}

func staleLockTombstoneDir(lockDir string) string {
	if lockDir == "" {
		return ""
	}
	suffix := fmt.Sprintf("%d-%d-%d", os.Getpid(), time.Now().UnixNano(), staleLockTombstoneSeq.Add(1))
	return filepath.Join(filepath.Dir(lockDir), filepath.Base(lockDir)+".tombstone."+suffix+".lock")
}

func releaseDirLockObserved(lockDir, ownerToken string, guardWait time.Duration, observer func(staleLockGuardStage)) error {
	if lockDir == "" || ownerToken == "" {
		return fmt.Errorf("bead: release lock requires canonical path and owner token")
	}
	guard, err := acquireStaleLockTransitionGuardObserved(lockDir, guardWait, observer)
	if err != nil {
		// Fail safe: an advisory-lock or sidecar I/O failure must never
		// authorize removing an unverified canonical directory.
		return fmt.Errorf("bead: acquire stale-break guard for release: %w", err)
	}
	tombstoneDir, owned, renameErr := renameOwnedCollectionLockDir(lockDir, ownerToken)
	guardErr := releaseStaleLockBreakGuard(guard)
	if renameErr != nil {
		return errors.Join(renameErr, guardErr)
	}
	if guardErr != nil {
		return guardErr
	}
	if !owned {
		return nil
	}
	if err := os.RemoveAll(tombstoneDir); err != nil {
		return fmt.Errorf("bead: remove owned lock tombstone %s: %w", tombstoneDir, err)
	}
	return nil
}

func renameOwnedCollectionLockDir(lockDir, ownerToken string) (string, bool, error) {
	inspected, err := os.Lstat(lockDir)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !inspected.IsDir() {
		return "", false, fmt.Errorf("bead: canonical lock path is not a directory: %s", lockDir)
	}
	tokenData, err := os.ReadFile(filepath.Join(lockDir, collectionLockOwnerTokenFile))
	if err != nil {
		return "", false, err
	}
	currentToken := strings.TrimSpace(string(tokenData))
	if currentToken != ownerToken {
		if _, err := hex.DecodeString(currentToken); err != nil || len(currentToken) != 64 {
			return "", false, fmt.Errorf("bead: canonical lock owner token is malformed")
		}
		return "", false, nil
	}
	current, err := os.Lstat(lockDir)
	if err != nil || !current.IsDir() || !os.SameFile(inspected, current) {
		return "", false, fmt.Errorf("bead: canonical lock identity changed during release")
	}
	tombstoneDir := staleLockTombstoneDir(lockDir)
	if tombstoneDir == "" {
		return "", false, fmt.Errorf("bead: cannot derive lock tombstone path")
	}
	if err := os.Rename(lockDir, tombstoneDir); err != nil {
		return "", false, err
	}
	return tombstoneDir, true, nil
}
