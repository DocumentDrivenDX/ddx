package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/gitlock"
)

// runGitWithIndexLockRecovery is a package-private alias kept for callers that
// have not yet migrated to gitlock.RunGitWithIndexLockRecovery directly.
var runGitWithIndexLockRecovery = gitlock.RunGitWithIndexLockRecovery

// retryGitWithTransientContention retries a git mutation when the wrapped
// operation fails with transient index/ref contention. The inner operation may
// still perform its own index.lock recovery; this helper handles the broader
// transient forms such as "unable to write new index file" so tracker and
// durable-audit commits share the same retry policy.
func retryGitWithTransientContention(run func() ([]byte, error)) ([]byte, error) {
	var out []byte
	var lastErr error
	var lastDiag string
	for attempt := 0; attempt < gitlock.RecoveryAttempts; attempt++ {
		out, lastErr = run()
		if lastErr == nil {
			return out, nil
		}
		if !isTransientGitContention(lastErr) {
			return out, lastErr
		}
		lastDiag = strings.TrimSpace(lastErr.Error())
		if attempt+1 < gitlock.RecoveryAttempts {
			time.Sleep(gitlock.LiveOwnerWait)
		}
	}
	return out, fmt.Errorf("%s; transient git contention exhausted after %d attempts: %s",
		strings.TrimSpace(string(out)), gitlock.RecoveryAttempts, lastDiag)
}

// recoverGitIndexLock is a package-private alias kept for callers that have
// not yet migrated to gitlock.RecoverGitIndexLock directly.
func recoverGitIndexLock(projectRoot string) (gitlock.IndexLockRecoveryResult, error) {
	return gitlock.RecoverGitIndexLock(projectRoot)
}
