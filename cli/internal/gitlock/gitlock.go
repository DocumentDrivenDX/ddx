// Package gitlock provides recovery helpers for transient .git/index.lock
// contention. It is shared by the agent execute-loop and the bead
// auto-commit path so both benefit from identical recovery semantics.
package gitlock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/lockmetrics"
)

// StaleAge is the age after which a .git/index.lock with no identifiable live
// owner is considered stale and safe to remove.
//
// All DDx writers serialize through withMainGitLock (.ddx/.git-tracker.lock),
// so an unowned index.lock past this threshold represents either a crashed git
// process or an operator command that exited without cleanup.
var StaleAge = 30 * time.Second

// RecoveryAttempts bounds the total number of attempts (including the initial
// try) made by RunGitWithIndexLockRecovery.
const RecoveryAttempts = 3

// LiveOwnerWait is the wait between retries when a live process holds the lock.
// Short by design: stalling is not acceptable here.
var LiveOwnerWait = 500 * time.Millisecond

type ownerProbeKind uint8

const (
	ownerProbeUnknown ownerProbeKind = iota
	ownerProbeAbsent
	ownerProbeLive
	ownerProbeDead
)

type ownerProbeResult struct {
	kind   ownerProbeKind
	pid    int
	detail string
}

type ownerProbeFunc func(string) ownerProbeResult

type staleLockTransitionStage string

const (
	staleLockStageSourceObserved staleLockTransitionStage = "source_observed"
	staleLockStageGuardAttempted staleLockTransitionStage = "guard_attempted"
	staleLockStageGuardContended staleLockTransitionStage = "guard_contended"
	staleLockStageGuardAcquired  staleLockTransitionStage = "guard_acquired"
	staleLockStageRenamed        staleLockTransitionStage = "renamed"
)

type staleLockTransitionObserver func(staleLockTransitionStage)

type staleLockTransitionGuard struct {
	file  *os.File
	mutex *sync.Mutex
}

var (
	staleLockGuardMutexes sync.Map
	staleLockTombstoneSeq atomic.Uint64
)

// IsIndexLockError reports whether the git output is the .git/index.lock-exists error.
func IsIndexLockError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "index.lock") &&
		(strings.Contains(lower, "file exists") || strings.Contains(lower, "another git process"))
}

// IsTransientGitContention reports whether a git stderr / error pair is a
// transient index/ref contention failure that should be retried rather than
// surfaced as a hard stop.
func IsTransientGitContention(output string, err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(output + "\n" + err.Error()))
	if IsIndexLockError(msg) {
		return true
	}
	for _, marker := range []string{
		"unable to write new index file",
		"cannot lock ref",
		"unable to update the ref",
		"tracker lock timeout",
		"signal: killed",
		"context deadline exceeded",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// IndexLockPath returns the absolute path to Git's index.lock for projectRoot.
// It returns an empty string when Git cannot resolve the path. Recovery uses
// the error-returning resolver directly and therefore fails closed.
func IndexLockPath(projectRoot string) string {
	path, err := resolveIndexLockPath(projectRoot)
	if err != nil {
		return ""
	}
	return path
}

func resolveIndexLockPath(projectRoot string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := internalgit.Command(
		ctx,
		projectRoot,
		"rev-parse",
		"--path-format=absolute",
		"--git-path",
		"index.lock",
	).Output()
	if err != nil {
		return "", fmt.Errorf("resolve git index.lock from %s: %w", projectRoot, err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("resolve git index.lock from %s: empty path", projectRoot)
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("resolve git index.lock from %s: non-absolute path %q", projectRoot, path)
	}
	// rev-parse resolves an existing index.lock symlink to its target. Retain
	// the exact --git-path result for ordinary files, but recover the lexical
	// gitdir sibling when it is itself a malformed non-regular object so the
	// safety check can reject the canonical symlink rather than inspect or
	// mutate its target.
	gitDirOut, gitDirErr := internalgit.Command(
		ctx,
		projectRoot,
		"rev-parse",
		"--path-format=absolute",
		"--git-dir",
	).Output()
	if gitDirErr != nil {
		return "", fmt.Errorf("resolve gitdir from %s: %w", projectRoot, gitDirErr)
	}
	gitDir := strings.TrimSpace(string(gitDirOut))
	if gitDir == "" || !filepath.IsAbs(gitDir) {
		return "", fmt.Errorf("resolve gitdir from %s: invalid absolute path %q", projectRoot, gitDir)
	}
	lexicalLockPath := filepath.Join(filepath.Clean(gitDir), "index.lock")
	if info, lstatErr := os.Lstat(lexicalLockPath); lstatErr == nil && !info.Mode().IsRegular() {
		return lexicalLockPath, nil
	} else if lstatErr != nil && !os.IsNotExist(lstatErr) {
		return "", fmt.Errorf("inspect lexical git index.lock %s: %w", lexicalLockPath, lstatErr)
	}
	return filepath.Clean(path), nil
}

// LsofTimeout bounds the lsof call in IndexLockOwner. On Linux, lsof can
// hang indefinitely while scanning /proc entries with stale NFS mounts or
// zombie processes. A 2s cap keeps index-lock recovery from wedging the test
// suite or the live drain loop. Tests may lower this to reduce wall time.
var LsofTimeout = 2 * time.Second

// IndexLockOwner attempts to identify the pid that currently holds lockPath
// open. A nil error with pid zero means lsof positively reported no owner.
// Missing tooling, timeouts, execution failures, and malformed output return
// an error because those unknown outcomes must not authorize stale recovery.
func IndexLockOwner(lockPath string) (int, error) {
	probe := probeIndexLockOwner(lockPath)
	switch probe.kind {
	case ownerProbeLive, ownerProbeDead:
		return probe.pid, nil
	case ownerProbeAbsent:
		return 0, nil
	default:
		return 0, errors.New(probe.detail)
	}
}

type lsofRunner func(context.Context, string, string) ([]byte, []byte, error)

func probeIndexLockOwner(lockPath string) ownerProbeResult {
	return probeIndexLockOwnerWith(lockPath, exec.LookPath, runLsof)
}

func runLsof(ctx context.Context, executable, lockPath string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, executable, "-t", "--", lockPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	return out, stderr.Bytes(), err
}

func probeIndexLockOwnerWith(
	lockPath string,
	lookPath func(string) (string, error),
	run lsofRunner,
) ownerProbeResult {
	lsof, err := lookPath("lsof")
	if err != nil {
		return ownerProbeResult{kind: ownerProbeUnknown, detail: fmt.Sprintf("owner probe unknown: lsof unavailable: %v", err)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), LsofTimeout)
	defer cancel()
	out, stderr, err := run(ctx, lsof, lockPath)
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	return classifyLsofProbe(out, stderr, exitCode, err, ctx.Err() != nil)
}

func classifyLsofProbe(out, stderr []byte, exitCode int, runErr error, timedOut bool) ownerProbeResult {
	if strings.TrimSpace(string(stderr)) != "" {
		return ownerProbeResult{kind: ownerProbeUnknown, detail: "owner probe unknown: lsof wrote diagnostics: " + strings.TrimSpace(string(stderr))}
	}
	if runErr != nil {
		if timedOut || errors.Is(runErr, context.DeadlineExceeded) {
			return ownerProbeResult{kind: ownerProbeUnknown, detail: "owner probe unknown: lsof timed out"}
		}
		if exitCode == 1 &&
			strings.TrimSpace(string(out)) == "" && strings.TrimSpace(string(stderr)) == "" {
			return ownerProbeResult{kind: ownerProbeAbsent, detail: "no owner found"}
		}
		detail := strings.TrimSpace(string(stderr))
		if detail == "" {
			detail = runErr.Error()
		}
		return ownerProbeResult{kind: ownerProbeUnknown, detail: "owner probe unknown: lsof execution failed: " + detail}
	}
	if exitCode != 0 {
		return ownerProbeResult{kind: ownerProbeUnknown, detail: fmt.Sprintf("owner probe unknown: lsof exited %d without an execution error", exitCode)}
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return ownerProbeResult{kind: ownerProbeUnknown, detail: "owner probe unknown: lsof returned empty successful output"}
	}
	lines := strings.Split(s, "\n")
	pids := make([]int, 0, len(lines))
	seen := make(map[int]struct{}, len(lines))
	for _, line := range lines {
		parsed, parseErr := strconv.Atoi(strings.TrimSpace(line))
		if parseErr != nil || parsed <= 0 {
			return ownerProbeResult{kind: ownerProbeUnknown, detail: "owner probe unknown: malformed lsof output"}
		}
		if _, exists := seen[parsed]; !exists {
			seen[parsed] = struct{}{}
			pids = append(pids, parsed)
		}
	}
	for _, pid := range pids {
		if processAlive(pid) {
			return ownerProbeResult{kind: ownerProbeLive, pid: pid, detail: "live owner"}
		}
	}
	if len(pids) != 1 {
		return ownerProbeResult{kind: ownerProbeUnknown, detail: "owner probe unknown: ambiguous lsof owner set"}
	}
	return ownerProbeResult{kind: ownerProbeDead, pid: pids[0], detail: "identified owner is dead"}
}

// IndexLockRecoveryResult describes the outcome of one recovery attempt
// against .git/index.lock.
type IndexLockRecoveryResult struct {
	// Removed is true if the lock file was deleted.
	Removed bool
	// OwnerPID is the pid identified as holding the lock, or 0 if unknown.
	OwnerPID int
	// OwnerAlive reports the liveness of OwnerPID at inspection time.
	// Meaningful only when OwnerPID != 0.
	OwnerAlive bool
	// Age is the lock-file age at inspection (zero if lock was missing).
	Age time.Duration
	// Reason is a human-readable summary suitable for log output.
	Reason string
}

// RecoverGitIndexLock inspects Git's actual index.lock under projectRoot. A
// stale candidate is moved only while holding a stable advisory transition
// guard and only when the current pathname still names the exact regular-file
// identity inspected by this attempt.
//
// projectRoot may be a subdirectory of the git repo; RecoverGitIndexLock
// walks up to find the actual .git directory so callers that operate on
// subdirectories (e.g. .ddx/) still find the correct lock file.
func RecoverGitIndexLock(projectRoot string) (IndexLockRecoveryResult, error) {
	return recoverGitIndexLockObserved(projectRoot, nil, probeIndexLockOwner)
}

func recoverGitIndexLockObserved(
	projectRoot string,
	observer staleLockTransitionObserver,
	probeOwner ownerProbeFunc,
) (IndexLockRecoveryResult, error) {
	lockPath, err := resolveIndexLockPath(projectRoot)
	if err != nil {
		return IndexLockRecoveryResult{}, err
	}
	info, statErr := os.Lstat(lockPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return IndexLockRecoveryResult{Reason: "lock not present"}, nil
		}
		return IndexLockRecoveryResult{}, fmt.Errorf("stat %s: %w", lockPath, statErr)
	}
	if !info.Mode().IsRegular() {
		return IndexLockRecoveryResult{
			Reason: fmt.Sprintf("lock path is not a regular file: %s", lockPath),
		}, nil
	}
	age := time.Since(info.ModTime())
	probe := probeOwner(lockPath)
	base := IndexLockRecoveryResult{OwnerPID: probe.pid, Age: age}
	stale := false
	switch probe.kind {
	case ownerProbeLive:
		base.OwnerAlive = true
		base.Reason = fmt.Sprintf("live owner pid %d (lock age %s)", probe.pid, age.Round(time.Second))
		return base, nil
	case ownerProbeDead:
		stale = true
	case ownerProbeAbsent:
		if age < StaleAge {
			base.Reason = fmt.Sprintf("no owner found but lock is fresh (age %s < %s)", age.Round(time.Second), StaleAge)
			return base, nil
		}
		stale = true
	case ownerProbeUnknown:
		base.Reason = probe.detail
		return base, nil
	default:
		base.Reason = "owner probe unknown: invalid probe outcome"
		return base, nil
	}
	if !stale {
		base.Reason = "lock is not stale"
		return base, nil
	}

	// Pin the source object before attempting the transition. The pre-probe
	// lstat and post-probe open must still describe the same regular file so an
	// ownership observation for one inode can never authorize another.
	candidate, openErr := openPinnedStaleLock(lockPath)
	if openErr != nil {
		if os.IsNotExist(openErr) {
			base.Reason = "lock identity changed before source capture"
			return base, nil
		}
		return base, fmt.Errorf("open stale index lock %s: %w", lockPath, openErr)
	}
	candidateOpen := true
	defer func() {
		if candidateOpen {
			_ = candidate.Close()
		}
	}()
	pinned, statErr := candidate.Stat()
	if statErr != nil {
		return base, fmt.Errorf("stat pinned index lock %s: %w", lockPath, statErr)
	}
	if !pinned.Mode().IsRegular() || !os.SameFile(info, pinned) {
		base.Reason = "lock identity changed before source capture"
		return base, nil
	}
	if observer != nil {
		observer(staleLockStageSourceObserved)
	}

	guard, acquired, guardErr := tryAcquireStaleLockTransitionGuard(lockPath, observer)
	if guardErr != nil {
		return base, fmt.Errorf("acquire index-lock stale-break guard: %w", guardErr)
	}
	if !acquired {
		base.Reason = "stale-break guard contended"
		return base, nil
	}

	tombstone, renamed, transitionErr := renamePinnedStaleLock(lockPath, pinned)
	base.Removed = renamed
	if transitionErr == nil && renamed && observer != nil {
		observer(staleLockStageRenamed)
	}
	releaseErr := releaseStaleLockTransitionGuard(guard)
	if transitionErr != nil {
		return base, errors.Join(transitionErr, releaseErr)
	}
	if releaseErr != nil {
		return base, fmt.Errorf("release index-lock stale-break guard: %w", releaseErr)
	}
	if !renamed {
		base.Reason = "lock identity changed before guarded rename"
		return base, nil
	}

	// The advisory guard is intentionally released before private tombstone
	// disposal. Recheck identity after release and remove only the exact unique
	// tombstone produced by this contender; never remove the canonical path.
	tombstoneInfo, tombstoneErr := os.Lstat(tombstone)
	if tombstoneErr != nil {
		return base, fmt.Errorf("verify stale index-lock tombstone %s: %w", tombstone, tombstoneErr)
	}
	if !tombstoneInfo.Mode().IsRegular() || !os.SameFile(pinned, tombstoneInfo) {
		return base, fmt.Errorf("verify stale index-lock tombstone %s: identity changed", tombstone)
	}
	if closeErr := candidate.Close(); closeErr != nil {
		candidateOpen = false
		return base, fmt.Errorf("close pinned stale index lock before tombstone removal: %w", closeErr)
	}
	candidateOpen = false
	if removeErr := os.Remove(tombstone); removeErr != nil {
		return base, fmt.Errorf("remove stale index-lock tombstone %s: %w", tombstone, removeErr)
	}
	base.Removed = true
	if probe.kind == ownerProbeDead {
		base.Reason = fmt.Sprintf("removed: owner pid %d not alive (lock age %s)", probe.pid, age.Round(time.Second))
	} else {
		base.Reason = fmt.Sprintf("removed: proven no owner, age %s >= stale threshold %s", age.Round(time.Second), StaleAge)
	}
	return base, nil
}

func staleLockTransitionGuardPath(lockPath string) string {
	return lockPath + ".stale-break.lock"
}

func staleLockTombstonePath(lockPath string) string {
	suffix := fmt.Sprintf("%d-%d-%d", os.Getpid(), time.Now().UnixNano(), staleLockTombstoneSeq.Add(1))
	return filepath.Join(filepath.Dir(lockPath), filepath.Base(lockPath)+".tombstone."+suffix+".lock")
}

func staleLockGuardMutex(path string) *sync.Mutex {
	mutex := &sync.Mutex{}
	actual, _ := staleLockGuardMutexes.LoadOrStore(path, mutex)
	return actual.(*sync.Mutex)
}

func tryAcquireStaleLockTransitionGuard(lockPath string, observer staleLockTransitionObserver) (*staleLockTransitionGuard, bool, error) {
	guardPath, err := filepath.Abs(staleLockTransitionGuardPath(lockPath))
	if err != nil {
		return nil, false, err
	}
	if observer != nil {
		observer(staleLockStageGuardAttempted)
	}
	mutex := staleLockGuardMutex(guardPath)
	if !mutex.TryLock() {
		if observer != nil {
			observer(staleLockStageGuardContended)
		}
		return nil, false, nil
	}
	prior, priorErr := os.Lstat(guardPath)
	if priorErr != nil && !os.IsNotExist(priorErr) {
		mutex.Unlock()
		return nil, false, priorErr
	}
	if priorErr == nil && !prior.Mode().IsRegular() {
		mutex.Unlock()
		return nil, false, fmt.Errorf("stale-break guard is not a regular file: %s", guardPath)
	}
	guardFile, err := os.OpenFile(guardPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		mutex.Unlock()
		return nil, false, err
	}
	opened, err := guardFile.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		_ = guardFile.Close()
		mutex.Unlock()
		if err != nil {
			return nil, false, err
		}
		return nil, false, fmt.Errorf("opened stale-break guard is not a regular file: %s", guardPath)
	}
	current, err := os.Lstat(guardPath)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) ||
		(priorErr == nil && !os.SameFile(prior, opened)) {
		_ = guardFile.Close()
		mutex.Unlock()
		if err != nil {
			return nil, false, err
		}
		return nil, false, fmt.Errorf("stale-break guard identity changed: %s", guardPath)
	}
	locked, err := tryLockStaleLockGuardFile(guardFile)
	if err != nil {
		_ = guardFile.Close()
		mutex.Unlock()
		return nil, false, err
	}
	if !locked {
		_ = guardFile.Close()
		mutex.Unlock()
		if observer != nil {
			observer(staleLockStageGuardContended)
		}
		return nil, false, nil
	}
	lockedInfo, statErr := guardFile.Stat()
	lockedCurrent, lstatErr := os.Lstat(guardPath)
	if statErr != nil || lstatErr != nil || !lockedInfo.Mode().IsRegular() ||
		!lockedCurrent.Mode().IsRegular() || !os.SameFile(lockedInfo, lockedCurrent) ||
		!os.SameFile(opened, lockedInfo) {
		unlockErr := unlockStaleLockGuardFile(guardFile)
		closeErr := guardFile.Close()
		mutex.Unlock()
		return nil, false, errors.Join(
			fmt.Errorf("stale-break guard identity changed after advisory acquisition: %s", guardPath),
			statErr,
			lstatErr,
			unlockErr,
			closeErr,
		)
	}
	if observer != nil {
		observer(staleLockStageGuardAcquired)
	}
	return &staleLockTransitionGuard{file: guardFile, mutex: mutex}, true, nil
}

func releaseStaleLockTransitionGuard(guard *staleLockTransitionGuard) error {
	if guard == nil {
		return nil
	}
	var err error
	if guard.file != nil {
		err = errors.Join(unlockStaleLockGuardFile(guard.file), guard.file.Close())
	}
	if guard.mutex != nil {
		guard.mutex.Unlock()
	}
	return err
}

func renamePinnedStaleLock(lockPath string, pinned os.FileInfo) (string, bool, error) {
	current, err := os.Lstat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if !current.Mode().IsRegular() || !os.SameFile(pinned, current) {
		return "", false, nil
	}
	tombstone := staleLockTombstonePath(lockPath)
	if err := os.Rename(lockPath, tombstone); err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	renamedInfo, err := os.Lstat(tombstone)
	if err == nil && renamedInfo.Mode().IsRegular() && os.SameFile(pinned, renamedInfo) {
		return tombstone, true, nil
	}
	// A pathname replacement in the final check/rename window must not be
	// deleted or restored over a possibly fresh canonical lock. Leave the
	// contender-unique tombstone untouched for operator inspection.
	return tombstone, true, errors.Join(fmt.Errorf("stale index-lock identity changed during rename"), err)
}

// RunGitWithIndexLockRecovery runs `git args...` in dir and, on
// .git/index.lock contention, identifies the owner and either removes a stale
// lock or briefly waits for a live owner before retrying.
//
// Total attempts are bounded by RecoveryAttempts; the final error is decorated
// with the most recent recovery diagnostic. The helper deliberately does NOT
// loop for tens of seconds — stalling is not acceptable here.
func RunGitWithIndexLockRecovery(ctx context.Context, dir string, args ...string) ([]byte, error) {
	var out []byte
	err := lockmetrics.Instrument("index.lock", indexLockOperation(args), func() error {
		var e error
		out, e = runGitWithIndexLockRecovery(ctx, dir, args...)
		return e
	})
	return out, err
}

// indexLockOperation labels an index.lock hold by the git subcommand that
// took it, e.g. "index.commit" or "index.add".
func indexLockOperation(args []string) string {
	if len(args) == 0 {
		return "index"
	}
	return "index." + args[0]
}

func runGitWithIndexLockRecovery(ctx context.Context, dir string, args ...string) ([]byte, error) {
	var lastOut []byte
	var lastErr error
	var lastDiag string
	for attempt := 0; attempt < RecoveryAttempts; attempt++ {
		out, err := internalgit.Command(ctx, dir, args...).CombinedOutput()
		if err == nil {
			return out, nil
		}
		if !IsTransientGitContention(string(out), err) {
			return out, err
		}
		lastOut = out
		lastErr = err
		result, recErr := RecoverGitIndexLock(dir)
		if recErr != nil {
			return out, fmt.Errorf("%s; index-lock recovery failed: %w", strings.TrimSpace(string(out)), recErr)
		}
		lastDiag = result.Reason
		if !result.Removed {
			time.Sleep(LiveOwnerWait)
		}
	}
	if lastDiag == "" {
		return lastOut, lastErr
	}
	return lastOut, fmt.Errorf("%s; index-lock owner: %s: %w",
		strings.TrimSpace(string(lastOut)), lastDiag, lastErr)
}
