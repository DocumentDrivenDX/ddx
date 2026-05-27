// Package gitlock provides recovery helpers for transient .git/index.lock
// contention. It is shared by the agent execute-loop and the bead
// auto-commit path so both benefit from identical recovery semantics.
package gitlock

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// IsIndexLockError reports whether the git output is the .git/index.lock-exists error.
func IsIndexLockError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "index.lock") &&
		(strings.Contains(lower, "file exists") || strings.Contains(lower, "another git process"))
}

// IndexLockPath returns the absolute path to .git/index.lock for projectRoot.
func IndexLockPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".git", "index.lock")
}

// findGitRoot walks up from dir to find the directory that contains a .git
// entry. Returns dir unchanged if .git cannot be found (graceful fallback).
func findGitRoot(dir string) string {
	current := dir
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return dir
		}
		current = parent
	}
}

// IndexLockOwner attempts to identify the pid that currently holds lockPath
// open. Uses lsof if available on PATH. Returns (0, nil) if no owner can be
// determined. Failure to identify is not itself an error.
func IndexLockOwner(lockPath string) (int, error) {
	lsof, err := exec.LookPath("lsof")
	if err != nil {
		return 0, nil
	}
	out, err := exec.Command(lsof, "-t", "--", lockPath).Output()
	if err != nil {
		// lsof exits non-zero when no process has the file open.
		return 0, nil
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, nil
	}
	first := strings.SplitN(s, "\n", 2)[0]
	pid, perr := strconv.Atoi(strings.TrimSpace(first))
	if perr != nil {
		return 0, nil
	}
	return pid, nil
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

// RecoverGitIndexLock inspects .git/index.lock under projectRoot. If the lock
// has no live owner — either lsof identifies a dead pid, or no pid holds it
// and the mtime is past StaleAge — the lock is removed. Otherwise the lock is
// left in place and the result describes the live owner.
//
// projectRoot may be a subdirectory of the git repo; RecoverGitIndexLock
// walks up to find the actual .git directory so callers that operate on
// subdirectories (e.g. .ddx/) still find the correct lock file.
func RecoverGitIndexLock(projectRoot string) (IndexLockRecoveryResult, error) {
	projectRoot = findGitRoot(projectRoot)
	lockPath := IndexLockPath(projectRoot)
	info, statErr := os.Lstat(lockPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return IndexLockRecoveryResult{Reason: "lock not present"}, nil
		}
		return IndexLockRecoveryResult{}, fmt.Errorf("stat %s: %w", lockPath, statErr)
	}
	age := time.Since(info.ModTime())
	pid, _ := IndexLockOwner(lockPath)

	if pid > 0 {
		alive := processAlive(pid)
		if !alive {
			if rmErr := os.Remove(lockPath); rmErr != nil && !os.IsNotExist(rmErr) {
				return IndexLockRecoveryResult{OwnerPID: pid, Age: age},
					fmt.Errorf("remove lock owned by dead pid %d: %w", pid, rmErr)
			}
			return IndexLockRecoveryResult{
				Removed: true, OwnerPID: pid, OwnerAlive: false, Age: age,
				Reason: fmt.Sprintf("removed: owner pid %d not alive (lock age %s)", pid, age.Round(time.Second)),
			}, nil
		}
		return IndexLockRecoveryResult{
			OwnerPID: pid, OwnerAlive: true, Age: age,
			Reason: fmt.Sprintf("live owner pid %d (lock age %s)", pid, age.Round(time.Second)),
		}, nil
	}

	if age >= StaleAge {
		if rmErr := os.Remove(lockPath); rmErr != nil && !os.IsNotExist(rmErr) {
			return IndexLockRecoveryResult{Age: age},
				fmt.Errorf("remove unowned stale lock (age %s): %w", age, rmErr)
		}
		return IndexLockRecoveryResult{
			Removed: true, Age: age,
			Reason: fmt.Sprintf("removed: no owner found, age %s >= stale threshold %s",
				age.Round(time.Second), StaleAge),
		}, nil
	}

	return IndexLockRecoveryResult{
		Age:    age,
		Reason: fmt.Sprintf("no owner found but lock is fresh (age %s < %s)", age.Round(time.Second), StaleAge),
	}, nil
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
		if !IsIndexLockError(string(out)) {
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
