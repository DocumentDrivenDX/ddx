package agent

import (
	"context"

	"github.com/DocumentDrivenDX/ddx/internal/gitlock"
)

// recoverGitIndexLock is a package-private alias kept for callers that have
// not yet migrated to gitlock.RecoverGitIndexLock directly.
func recoverGitIndexLock(projectRoot string) (gitlock.IndexLockRecoveryResult, error) {
	return gitlock.RecoverGitIndexLock(projectRoot)
}

// runGitWithIndexLockRecovery is a package-private alias kept for callers that
// have not yet migrated to gitlock.RunGitWithIndexLockRecovery directly.
func runGitWithIndexLockRecovery(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return gitlock.RunGitWithIndexLockRecovery(ctx, dir, args...)
}

// isGitIndexLockError is a package-private alias for gitlock.IsIndexLockError.
func isGitIndexLockError(output string) bool {
	return gitlock.IsIndexLockError(output)
}
