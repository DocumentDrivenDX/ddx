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
}

// AutoCommit stages and commits a file with a structured message.
// Returns the landed commit SHA when a commit is created.
// Returns an empty SHA and nil if auto_commit is "never" (or unset) or if
// not in a git repo.
func AutoCommit(filePath string, artifactID string, operation string, cfg AutoCommitConfig) (string, error) {
	// Default to "never"
	if cfg.AutoCommit == "" || cfg.AutoCommit == "never" {
		return "", nil
	}

	if cfg.AutoCommit == "prompt" {
		fmt.Fprintf(os.Stderr, "Auto-commit %s? [y/N] ", filePath)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(answer)) != "y" {
			return "", nil
		}
		// Fall through to commit logic.
	} else if cfg.AutoCommit != "always" {
		return "", nil
	}

	repoDir := filepath.Dir(filePath)
	if repoDir == "" {
		repoDir = "."
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

	// Stage the file.
	// Stage only the file name after switching into the file's parent
	// directory. This keeps relative callers working when filePath itself is
	// a nested relative path such as cli/cmd/doc.go.
	addCmd := Command(ctx, repoDir, "add", filepath.Base(filePath))
	if out, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add failed: %w\n%s", err, string(out))
	}

	// Commit with --no-verify because these are mechanical commits.
	commitCmd := Command(ctx, repoDir, "commit", "--no-verify", "-m", message)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit failed: %w\n%s", err, string(out))
	}

	shaCmd := Command(ctx, repoDir, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w", err)
	}

	return strings.TrimSpace(string(shaOut)), nil
}
