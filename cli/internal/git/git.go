package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ddxDirSegment = ".ddx"

// FindProjectRoot walks up from startDir to find the git repository root.
// It returns the top-level directory of the git working tree. If startDir
// is not inside a git repository, it returns startDir unchanged.
//
// This is analogous to `git rev-parse --show-toplevel` and ensures that
// ddx always operates from the repository root regardless of the caller's
// working directory. Without this, running `ddx` from a subdirectory that
// contains its own `.ddx/` folder would silently use the wrong workspace.
func FindProjectRoot(startDir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := Command(ctx, startDir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		// Not in a git repo — fall back to the original directory.
		return startDir
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return startDir
	}
	if physical := physicalGitRoot(startDir); physical != "" && !samePath(physical, root) {
		return physical
	}
	return root
}

// physicalGitRoot walks upward on disk until it finds a .git directory or
// linked-worktree .git file. This intentionally bypasses git's configured
// worktree resolution so a corrupted local core.worktree cannot redirect ddx
// away from the checkout the operator actually invoked.
func physicalGitRoot(startDir string) string {
	current, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	for {
		if _, statErr := os.Stat(filepath.Join(current, ".git")); statErr == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	cleanA, err := filepath.Abs(a)
	if err != nil {
		cleanA = filepath.Clean(a)
	}
	cleanB, err := filepath.Abs(b)
	if err != nil {
		cleanB = filepath.Clean(b)
	}
	if resolvedA, err := filepath.EvalSymlinks(cleanA); err == nil {
		cleanA = resolvedA
	}
	if resolvedB, err := filepath.EvalSymlinks(cleanB); err == nil {
		cleanB = resolvedB
	}
	return cleanA == cleanB
}

// FindNearestDDxWorkspace walks up from startDir to find the nearest ancestor
// inside the current git repository that contains a .ddx workspace.
//
// When startDir is inside a linked git worktree (e.g. an execute-bead
// isolated worktree under .ddx/.execute-bead-wt-* or a worktrunk sibling
// like repo.feature-branch/), it resolves to the PRIMARY worktree's .ddx/
// first, not the linked worktree's own .ddx/. This is critical because:
//
//   - bead store mutations must land in the canonical project bead queue,
//     not in an isolated worktree's private snapshot that will be discarded
//   - the primary worktree is the operator's source of truth; linked
//     worktrees are ephemeral execution contexts
//
// If the primary worktree has no .ddx/ (or we aren't in a linked worktree),
// it falls back to walking up from startDir within the current git
// repository.
//
// Returns an empty string if no .ddx/ is found.
func FindNearestDDxWorkspace(startDir string) string {
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}

	// If we're inside a linked worktree, prefer the primary worktree's .ddx/.
	if primary := primaryWorktreeRoot(abs); primary != "" && primary != FindProjectRoot(abs) {
		candidate := filepath.Join(primary, ddxDirSegment)
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return primary
		}
	}

	gitRoot := FindProjectRoot(abs)
	current := abs
	for {
		candidate := filepath.Join(current, ddxDirSegment)
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return current
		}
		if current == gitRoot {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

// IsRepository checks if the current directory is a git repository.
func IsRepository(path string) bool {
	if path == "" {
		path = "."
	}

	cleanPath := filepath.Clean(path)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := Command(ctx, cleanPath, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// primaryWorktreeRoot returns the primary (non-linked) worktree directory
// for a given path inside a git repository, or "" if the path is not inside
// a linked worktree (or if resolution fails).
//
// Detection: `git rev-parse --git-common-dir` returns the shared .git
// directory. If that differs from `git rev-parse --git-dir`, we're in a
// linked worktree. The primary worktree is the parent directory of the
// common .git dir (unless the shared dir is a bare repo).
func primaryWorktreeRoot(startDir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gitDirCmd := Command(ctx, startDir, "rev-parse", "--path-format=absolute", "--git-dir")
	gitDirOut, err := gitDirCmd.Output()
	if err != nil {
		return ""
	}
	gitDir := strings.TrimSpace(string(gitDirOut))

	commonDirCmd := Command(ctx, startDir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	commonDirOut, err := commonDirCmd.Output()
	if err != nil {
		return ""
	}
	commonDir := strings.TrimSpace(string(commonDirOut))

	if gitDir == commonDir {
		// Not a linked worktree.
		return ""
	}

	// Common dir is either a bare repo or the primary worktree's .git dir.
	// If it's a .git directory, the primary worktree is its parent.
	if filepath.Base(commonDir) == ".git" {
		return filepath.Dir(commonDir)
	}
	// Bare repo: no primary worktree. Caller falls back to walk-up.
	return ""
}
