package bead

import (
	"context"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/gitlock"
)

// AutoCommitFilesWithRecovery wraps gitpkg.AutoCommitFiles with index-lock
// recovery so transient .git/index.lock contention (e.g. from a concurrent
// Claude Code statusline poller) does not bounce bead mutations.
func AutoCommitFilesWithRecovery(filePaths []string, artifactID, operation string, cfg gitpkg.AutoCommitConfig) (string, error) {
	cfg.RunGit = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return gitlock.RunGitWithIndexLockRecovery(ctx, dir, args...)
	}
	return gitpkg.AutoCommitFiles(filePaths, artifactID, operation, cfg)
}
