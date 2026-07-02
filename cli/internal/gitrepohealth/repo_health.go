package gitrepohealth

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
)

const (
	IssueCoreBareCorruption = "git_core_bare_corruption"
	IssueStrayCoreWorktree  = "git_stray_core_worktree"
	IssueLocalHooksPath     = "git_local_hooks_path"
	IssueWorktreeConfig     = "git_worktree_config_disabled"
)

type Issue struct {
	Type        string
	Description string
	Remediation []string
	Fixed       bool
	Error       string
}

type RepairResult struct {
	Issues          []Issue
	RepairedTypes   []string
	Commands        []string
	StatusOutput    string
	StatusStderr    string
	StatusSucceeded bool
}

func Check(workingDir string, fix bool) []Issue {
	var issues []Issue
	ctx := context.Background()

	gdOut, err := gitpkg.Command(ctx, workingDir, "rev-parse", "--git-dir").Output()
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
		inWT := gitpkg.Command(ctx, workingDir, "rev-parse", "--is-inside-work-tree")
		if b, err := inWT.Output(); err == nil && strings.TrimSpace(string(b)) == "true" {
			hasWorkTreeOnDisk = true
		}
	}
	if !hasWorkTreeOnDisk {
		return nil
	}

	gitConfigValue := func(key string) (string, bool) {
		c := gitpkg.Command(ctx, workingDir, "config", "--local", "--get", key)
		b, err := c.Output()
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(b)), true
	}
	gitConfigUnset := func(key string) error {
		c := gitpkg.Command(ctx, workingDir, "config", "--local", "--unset", key)
		return c.Run()
	}
	gitConfigUnsetAll := func(key string) error {
		c := gitpkg.Command(ctx, workingDir, "config", "--local", "--unset-all", key)
		return c.Run()
	}

	if val, ok := gitConfigValue("core.bare"); ok && val == "true" {
		desc := "core.bare=true is set on a repository that has a working tree (not actually bare)"
		rem := []string{"git config --unset core.bare"}
		issue := Issue{Type: IssueCoreBareCorruption, Description: desc, Remediation: rem}
		if fix {
			if err := gitConfigUnset("core.bare"); err == nil {
				issue.Description += " — removed"
				issue.Fixed = true
			} else {
				issue.Error = err.Error()
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
			desc := fmt.Sprintf("core.worktree=%q does not match actual worktree %q", worktreeVal, actual)
			rem := []string{"git config --unset core.worktree"}
			issue := Issue{Type: IssueStrayCoreWorktree, Description: desc, Remediation: rem}
			if fix {
				if err := gitConfigUnset("core.worktree"); err == nil {
					issue.Description += " — removed"
					issue.Fixed = true
				} else {
					issue.Error = err.Error()
					issue.Description += fmt.Sprintf(" — unset failed: %v", err)
				}
			}
			issues = append(issues, issue)
		}
	}

	if hooksPathVal, ok := gitConfigValue("core.hooksPath"); ok && hooksPathVal != "" {
		desc := fmt.Sprintf("core.hooksPath=%q is set locally; lefthook may skip hook-path sync and run stale hooks", hooksPathVal)
		rem := []string{
			"git config --unset-all --local core.hooksPath",
			"lefthook install --reset-hooks-path",
		}
		issue := Issue{Type: IssueLocalHooksPath, Description: desc, Remediation: rem}
		if fix {
			if err := gitConfigUnsetAll("core.hooksPath"); err == nil {
				issue.Description += " — removed"
				issue.Fixed = true
			} else {
				issue.Error = err.Error()
				issue.Description += fmt.Sprintf(" — unset failed: %v", err)
			}
		}
		issues = append(issues, issue)
	}

	val, ok := gitConfigValue("extensions.worktreeConfig")
	if !ok || val != "true" {
		issues = append(issues, Issue{
			Type:        IssueWorktreeConfig,
			Description: "extensions.worktreeConfig is not enabled; per-worktree config changes can corrupt the shared .git/config",
			Remediation: []string{
				"git config extensions.worktreeConfig true",
			},
		})
	}

	return issues
}

func RepairKnownConfigCorruption(ctx context.Context, workingDir string) RepairResult {
	result := RepairResult{}
	issues := Check(workingDir, true)
	result.Issues = issues
	for _, issue := range issues {
		if !issue.Fixed {
			continue
		}
		result.RepairedTypes = append(result.RepairedTypes, issue.Type)
		if len(issue.Remediation) > 0 {
			result.Commands = append(result.Commands, issue.Remediation[0])
		}
	}

	statusOut, statusErr := gitpkg.Command(ctx, workingDir, "status", "--porcelain", "--untracked-files=all").CombinedOutput()
	result.StatusOutput, result.StatusStderr = splitGitCombinedOutput(statusOut)
	result.StatusSucceeded = statusErr == nil
	if statusErr != nil {
		if result.StatusStderr == "" {
			result.StatusStderr = strings.TrimSpace(string(statusOut))
		}
	}
	return result
}

func splitGitCombinedOutput(out []byte) (string, string) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return "", ""
	}
	lines := bytes.Split(out, []byte("\n"))
	var stdout, stderr []string
	for _, rawLine := range lines {
		line := strings.TrimSpace(string(rawLine))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "fatal:") || strings.HasPrefix(line, "error:") {
			stderr = append(stderr, line)
			continue
		}
		stdout = append(stdout, line)
	}
	if len(stderr) == 0 {
		return trimmed, ""
	}
	return strings.Join(stdout, "\n"), strings.Join(stderr, "\n")
}
