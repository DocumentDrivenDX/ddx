package agent

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
)

// gitIndexLockStaleAge is the age after which a `.git/index.lock` with no
// identifiable live owner is considered stale and safe to remove.
//
// All DDx writers serialize through withMainGitLock (.ddx/.git-tracker.lock),
// so an unowned index.lock past this threshold represents either a crashed
// git process or an operator command that exited without cleanup.
var gitIndexLockStaleAge = 30 * time.Second

// gitIndexLockRecoveryAttempts bounds the total number of attempts
// (including the initial try) made by runGitWithIndexLockRecovery.
const gitIndexLockRecoveryAttempts = 3

// gitIndexLockLiveOwnerWait is the wait between retries when a live process
// is holding the lock. Short by design: stalling is not acceptable.
var gitIndexLockLiveOwnerWait = 500 * time.Millisecond

// isGitIndexLockError reports whether the git output is the
// `.git/index.lock`-exists error.
func isGitIndexLockError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "index.lock") &&
		(strings.Contains(lower, "file exists") || strings.Contains(lower, "another git process"))
}

// gitIndexLockPath returns the absolute path to .git/index.lock for
// projectRoot. The caller is responsible for ensuring projectRoot is a
// real worktree (the lock-recovery code handles missing files).
func gitIndexLockPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".git", "index.lock")
}

// gitIndexLockOwner attempts to identify the pid that currently holds
// lockPath open. Uses lsof if available on PATH. Returns (0, nil) if no
// owner can be determined (lsof missing, no process has it open, or
// lsof errored). Failure to identify is not itself an error.
func gitIndexLockOwner(lockPath string) (int, error) {
	lsof, err := exec.LookPath("lsof")
	if err != nil {
		return 0, nil
	}
	// lsof -t writes one pid per line for each process with the file open.
	out, err := exec.Command(lsof, "-t", "--", lockPath).Output()
	if err != nil {
		// lsof exits non-zero when no process has the file open, which is
		// the common case we want to treat as "no owner found".
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

// gitIndexLockRecoveryResult describes the outcome of one recovery attempt
// against `.git/index.lock`.
type gitIndexLockRecoveryResult struct {
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

// recoverGitIndexLock inspects `.git/index.lock` under projectRoot. If the
// lock has no live owner — either lsof identifies a dead pid, or no pid
// holds it and the mtime is past gitIndexLockStaleAge — the lock is
// removed. Otherwise the lock is left in place and the result describes
// the live owner.
func recoverGitIndexLock(projectRoot string) (gitIndexLockRecoveryResult, error) {
	lockPath := gitIndexLockPath(projectRoot)
	info, statErr := os.Lstat(lockPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return gitIndexLockRecoveryResult{
				Reason: "lock not present",
			}, nil
		}
		return gitIndexLockRecoveryResult{}, fmt.Errorf("stat %s: %w", lockPath, statErr)
	}
	age := time.Since(info.ModTime())
	pid, _ := gitIndexLockOwner(lockPath)

	if pid > 0 {
		alive := trackerProcessAlive(pid)
		if !alive {
			if rmErr := os.Remove(lockPath); rmErr != nil && !os.IsNotExist(rmErr) {
				return gitIndexLockRecoveryResult{
					OwnerPID: pid, Age: age,
				}, fmt.Errorf("remove lock owned by dead pid %d: %w", pid, rmErr)
			}
			return gitIndexLockRecoveryResult{
				Removed: true, OwnerPID: pid, OwnerAlive: false, Age: age,
				Reason: fmt.Sprintf("removed: owner pid %d not alive (lock age %s)", pid, age.Round(time.Second)),
			}, nil
		}
		return gitIndexLockRecoveryResult{
			OwnerPID: pid, OwnerAlive: true, Age: age,
			Reason: fmt.Sprintf("live owner pid %d (lock age %s)", pid, age.Round(time.Second)),
		}, nil
	}

	if age >= gitIndexLockStaleAge {
		if rmErr := os.Remove(lockPath); rmErr != nil && !os.IsNotExist(rmErr) {
			return gitIndexLockRecoveryResult{Age: age}, fmt.Errorf("remove unowned stale lock (age %s): %w", age, rmErr)
		}
		return gitIndexLockRecoveryResult{
			Removed: true, Age: age,
			Reason: fmt.Sprintf("removed: no owner found, age %s >= stale threshold %s", age.Round(time.Second), gitIndexLockStaleAge),
		}, nil
	}

	return gitIndexLockRecoveryResult{
		Age:    age,
		Reason: fmt.Sprintf("no owner found but lock is fresh (age %s < %s)", age.Round(time.Second), gitIndexLockStaleAge),
	}, nil
}

// runGitWithIndexLockRecovery runs `git args...` in dir and, on
// `.git/index.lock` contention, identifies the owner and either removes
// a stale lock or briefly waits for a live owner before retrying.
//
// Total attempts are bounded by gitIndexLockRecoveryAttempts; the final
// error is decorated with the most recent recovery diagnostic. The
// helper deliberately does NOT loop for tens of seconds — stalling is
// not acceptable here.
func runGitWithIndexLockRecovery(ctx context.Context, dir string, args ...string) ([]byte, error) {
	var lastOut []byte
	var lastErr error
	var lastDiag string
	for attempt := 0; attempt < gitIndexLockRecoveryAttempts; attempt++ {
		out, err := internalgit.Command(ctx, dir, args...).CombinedOutput()
		if err == nil {
			return out, nil
		}
		if !isGitIndexLockError(string(out)) {
			return out, err
		}
		lastOut = out
		lastErr = err
		result, recErr := recoverGitIndexLock(dir)
		if recErr != nil {
			return out, fmt.Errorf("%s; index-lock recovery failed: %w", strings.TrimSpace(string(out)), recErr)
		}
		lastDiag = result.Reason
		if !result.Removed {
			time.Sleep(gitIndexLockLiveOwnerWait)
		}
	}
	if lastDiag == "" {
		return lastOut, lastErr
	}
	return lastOut, fmt.Errorf("%s; index-lock owner: %s: %w",
		strings.TrimSpace(string(lastOut)), lastDiag, lastErr)
}
