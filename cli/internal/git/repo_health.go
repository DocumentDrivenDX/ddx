package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RepoHealthIssue describes a known-safe local git config issue and the
// remediation DDx can apply automatically when fix is enabled.
type RepoHealthIssue struct {
	Type        string
	Description string
	Remediation []string
	Fixed       bool
	RepairError string
}

// CheckGitRepoHealth detects the small set of safe local git config
// corruptions DDx knows how to repair:
//   - core.bare=true on a checkout with a real worktree
//   - stray core.worktree that does not match the actual checkout
//   - local core.hooksPath that interferes with lefthook hook syncing
//   - extensions.worktreeConfig not enabled (warning only)
//
// When fix is true, the first three are un-set in place. The worktreeConfig
// warning remains warning-only and is never auto-enabled.
func CheckGitRepoHealth(workingDir string, fix bool) []RepoHealthIssue {
	var issues []RepoHealthIssue
	ctx := context.Background()

	gitDirProbe := Command(ctx, workingDir, "rev-parse", "--git-dir")
	gdOut, err := gitDirProbe.Output()
	if err != nil {
		return nil
	}
	gitDirRaw := strings.TrimSpace(string(gdOut))
	if gitDirRaw == "" {
		return nil
	}

	hasWorkTreeOnDisk := false
	if _, statErr := os.Stat(filepath.Join(workingDir, ".git")); statErr == nil {
		hasWorkTreeOnDisk = true
	}
	if !hasWorkTreeOnDisk {
		inWT := Command(ctx, workingDir, "rev-parse", "--is-inside-work-tree")
		if b, err := inWT.Output(); err == nil && strings.TrimSpace(string(b)) == "true" {
			hasWorkTreeOnDisk = true
		}
	}
	if !hasWorkTreeOnDisk {
		return nil
	}

	gitConfigValue := func(key string) (string, bool) {
		c := Command(ctx, workingDir, "config", "--local", "--get", key)
		b, err := c.Output()
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(b)), true
	}
	gitConfigUnset := func(key string) error {
		return Command(ctx, workingDir, "config", "--local", "--unset", key).Run()
	}
	gitConfigUnsetAll := func(key string) error {
		return Command(ctx, workingDir, "config", "--local", "--unset-all", key).Run()
	}

	if val, ok := gitConfigValue("core.bare"); ok && val == "true" {
		issue := RepoHealthIssue{
			Type:        "git_core_bare_corruption",
			Description: "core.bare=true is set on a repository that has a working tree (not actually bare)",
			Remediation: []string{"git config --unset core.bare"},
		}
		if fix {
			if err := gitConfigUnset("core.bare"); err == nil {
				issue.Fixed = true
				issue.Description += " — removed"
			} else {
				issue.RepairError = err.Error()
				issue.Description += fmt.Sprintf(" — unset failed: %v", err)
			}
		}
		issues = append(issues, issue)
	}

	if worktreeVal, ok := gitConfigValue("core.worktree"); ok && worktreeVal != "" {
		actual := workingDir

		cmpVal := worktreeVal
		if !filepath.IsAbs(cmpVal) {
			cmpVal = filepath.Clean(filepath.Join(gitDirRaw, worktreeVal))
		}
		resolvedCmp, _ := filepath.EvalSymlinks(cmpVal)
		if resolvedCmp == "" {
			resolvedCmp = cmpVal
		}
		resolvedActual, _ := filepath.EvalSymlinks(actual)
		if resolvedActual == "" {
			resolvedActual = actual
		}

		if actual != "" && resolvedCmp != resolvedActual {
			issue := RepoHealthIssue{
				Type:        "git_stray_core_worktree",
				Description: fmt.Sprintf("core.worktree=%q does not match actual worktree %q", worktreeVal, actual),
				Remediation: []string{"git config --unset core.worktree"},
			}
			if fix {
				if err := gitConfigUnset("core.worktree"); err == nil {
					issue.Fixed = true
					issue.Description += " — removed"
				} else {
					issue.RepairError = err.Error()
					issue.Description += fmt.Sprintf(" — unset failed: %v", err)
				}
			}
			issues = append(issues, issue)
		}
	}

	if hooksPathVal, ok := gitConfigValue("core.hooksPath"); ok && hooksPathVal != "" {
		issue := RepoHealthIssue{
			Type:        "git_local_hooks_path",
			Description: fmt.Sprintf("core.hooksPath=%q is set locally; lefthook may skip hook-path sync and run stale hooks", hooksPathVal),
			Remediation: []string{
				"git config --unset-all --local core.hooksPath",
				"lefthook install --reset-hooks-path",
			},
		}
		if fix {
			if err := gitConfigUnsetAll("core.hooksPath"); err == nil {
				issue.Fixed = true
				issue.Description += " — removed"
			} else {
				issue.RepairError = err.Error()
				issue.Description += fmt.Sprintf(" — unset failed: %v", err)
			}
		}
		issues = append(issues, issue)
	}

	val, ok := gitConfigValue("extensions.worktreeConfig")
	if !ok || val != "true" {
		issues = append(issues, RepoHealthIssue{
			Type:        "git_worktree_config_disabled",
			Description: "extensions.worktreeConfig is not enabled; per-worktree config changes can corrupt the shared .git/config",
			Remediation: []string{"git config extensions.worktreeConfig true"},
		})
	}

	return issues
}
