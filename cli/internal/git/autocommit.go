package git

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AutoCommitConfig holds configuration for auto-commit behaviour.
type AutoCommitConfig struct {
	// AutoCommit controls when to commit: "always", "prompt", or "never".
	// The default (empty string) is treated as "never".
	AutoCommit   string
	CommitPrefix string
	// IncludeStaged commits all staged changes in addition to the target file.
	// Use this only for workflows where pre-staged work is intentionally part
	// of the same checkpoint, such as closing a bead with staged implementation.
	IncludeStaged bool
	// RunGit, if non-nil, replaces the default git runner for staging and commit
	// calls. Callers can supply gitlock.RunGitWithIndexLockRecovery to recover
	// from transient .git/index.lock contention without changing semantics.
	RunGit func(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// AutoCommitAfterAddHook is invoked after git add succeeds and before git
// commit starts. Tests may swap this to widen the stage/commit race window when
// asserting that callers serialize the whole auto-commit critical section.
var AutoCommitAfterAddHook func(AutoCommitHookContext)

// AutoCommitHookContext describes one auto-commit invocation for test hooks.
type AutoCommitHookContext struct {
	RepoDir       string
	Message       string
	FilePaths     []string
	IncludeStaged bool
}

// AutoCommit stages and commits a file with a structured message.
// Returns the landed commit SHA when a commit is created.
// Returns an empty SHA and nil if auto_commit is "never" (or unset) or if
// not in a git repo.
func AutoCommit(filePath string, artifactID string, operation string, cfg AutoCommitConfig) (string, error) {
	return AutoCommitFiles([]string{filePath}, artifactID, operation, cfg)
}

// AutoCommitFiles stages and commits a bounded set of files with a structured
// message. All files must live in the same directory; unrelated staged work is
// preserved unless IncludeStaged is true.
func AutoCommitFiles(filePaths []string, artifactID string, operation string, cfg AutoCommitConfig) (string, error) {
	// Default to "never"
	if cfg.AutoCommit == "" || cfg.AutoCommit == "never" {
		return "", nil
	}
	if len(filePaths) == 0 {
		return "", nil
	}

	if cfg.AutoCommit == "prompt" {
		fmt.Fprintf(os.Stderr, "Auto-commit %s? [y/N] ", strings.Join(filePaths, ", "))
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(answer)) != "y" {
			return "", nil
		}
		// Fall through to commit logic.
	} else if cfg.AutoCommit != "always" {
		return "", nil
	}

	repoDir := filepath.Dir(filePaths[0])
	if repoDir == "" {
		repoDir = "."
	}
	for _, path := range filePaths[1:] {
		if filepath.Dir(path) != repoDir {
			return "", fmt.Errorf("auto-commit files must share one directory: %s and %s", filePaths[0], path)
		}
	}

	// Check we are inside a git repo (silently skip if not).
	if !IsRepository(repoDir) {
		return "", nil
	}

	prefix := cfg.CommitPrefix
	if prefix == "" {
		prefix = "docs"
	}

	message := fmt.Sprintf("%s(%s): %s [ddx: doc-stamp]", prefix, artifactID, operation)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runGit := cfg.RunGit
	if runGit == nil {
		runGit = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
			return Command(ctx, dir, args...).CombinedOutput()
		}
	}

	// Stage only file names after switching into the files' parent directory.
	// This keeps relative callers working when paths are nested under a repo.
	addArgs := []string{"add"}
	for _, path := range filePaths {
		addArgs = append(addArgs, filepath.Base(path))
	}
	if out, err := runGit(ctx, repoDir, addArgs...); err != nil {
		return "", fmt.Errorf("git add failed: %w\n%s", err, string(out))
	}
	if hook := AutoCommitAfterAddHook; hook != nil {
		hook(AutoCommitHookContext{
			RepoDir:       repoDir,
			Message:       message,
			FilePaths:     append([]string(nil), filePaths...),
			IncludeStaged: cfg.IncludeStaged,
		})
	}

	// Commit with --no-verify because these are mechanical commits.
	commitArgs := []string{"commit", "--no-verify", "-m", message}
	if !cfg.IncludeStaged {
		// Limit the commit to the target path so unrelated staged work stays
		// staged for its intended commit.
		commitArgs = append(commitArgs, "--only", "--")
		for _, path := range filePaths {
			commitArgs = append(commitArgs, filepath.Base(path))
		}
	}
	if out, err := runGit(ctx, repoDir, commitArgs...); err != nil {
		return "", fmt.Errorf("git commit failed: %w\n%s", err, string(out))
	}

	shaCmd := Command(ctx, repoDir, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w", err)
	}

	return strings.TrimSpace(string(shaOut)), nil
}
