package git

import (
	"context"
	"fmt"
	"os/exec"
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
// Returns nil if auto_commit is "never" (or unset) or if not in a git repo.
func AutoCommit(filePath string, artifactID string, operation string, cfg AutoCommitConfig) error {
	// Default to "never"
	if cfg.AutoCommit == "" || cfg.AutoCommit == "never" {
		return nil
	}

	// Only "always" is handled automatically; "prompt" is not yet interactive.
	if cfg.AutoCommit != "always" {
		return nil
	}

	// Check we are inside a git repo (silently skip if not).
	if !IsRepository(".") {
		return nil
	}

	prefix := cfg.CommitPrefix
	if prefix == "" {
		prefix = "docs"
	}

	message := fmt.Sprintf("%s(%s): %s [ddx: doc-stamp]", prefix, artifactID, operation)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stage the file.
	addCmd := exec.CommandContext(ctx, "git", "add", filePath)
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w\n%s", err, string(out))
	}

	// Commit with --no-verify because these are mechanical commits.
	commitCmd := exec.CommandContext(ctx, "git", "commit", "--no-verify", "-m", message)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %w\n%s", err, string(out))
	}

	return nil
}
