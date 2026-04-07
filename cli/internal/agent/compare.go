package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RunCompare dispatches the same prompt to multiple harnesses,
// optionally in isolated worktrees, and returns a ComparisonRecord.
func (r *Runner) RunCompare(opts CompareOptions) (*ComparisonRecord, error) {
	if len(opts.Harnesses) == 0 {
		return nil, fmt.Errorf("agent: compare requires at least one harness")
	}

	prompt, err := r.resolvePrompt(opts.RunOptions)
	if err != nil {
		return nil, err
	}

	id := genCompareID()
	record := &ComparisonRecord{
		ID:        id,
		Timestamp: time.Now().UTC(),
		Prompt:    prompt,
		Arms:      make([]ComparisonArm, len(opts.Harnesses)),
	}

	// Resolve base working directory
	baseDir := opts.WorkDir
	if baseDir == "" {
		baseDir, _ = os.Getwd()
	}

	// Create worktrees sequentially (git worktree add takes a lock)
	// then run agent arms in parallel.
	worktrees := make([]string, len(opts.Harnesses))
	if opts.Sandbox {
		for i, harness := range opts.Harnesses {
			label := harness
			if l, ok := opts.ArmLabels[i]; ok {
				label = l
			}
			wt, err := createCompareWorktree(baseDir, id, label)
			if err != nil {
				record.Arms[i] = ComparisonArm{
					Harness:  label,
					ExitCode: 1,
					Error:    fmt.Sprintf("worktree: %s", err),
				}
				continue
			}
			worktrees[i] = wt
		}
	}

	var wg sync.WaitGroup
	for i, harness := range opts.Harnesses {
		// Skip arms that failed worktree creation
		if opts.Sandbox && worktrees[i] == "" && record.Arms[i].Error != "" {
			continue
		}
		wg.Add(1)
		go func(idx int, harnessName string) {
			defer wg.Done()
			record.Arms[idx] = r.runCompareArm(opts, idx, harnessName, baseDir, id, prompt, worktrees[idx])
		}(i, harness)
	}
	wg.Wait()

	// Cleanup worktrees unless --keep-sandbox
	if opts.Sandbox && !opts.KeepSandbox {
		r.cleanupCompareWorktrees(baseDir, id)
	}

	return record, nil
}

// runCompareArm executes one harness arm, optionally in a pre-created worktree.
func (r *Runner) runCompareArm(opts CompareOptions, armIdx int, harnessName, baseDir, compareID, prompt, worktreePath string) ComparisonArm {
	label := harnessName
	if l, ok := opts.ArmLabels[armIdx]; ok {
		label = l
	}
	arm := ComparisonArm{Harness: label}

	// Determine working directory
	workDir := baseDir
	if worktreePath != "" {
		workDir = worktreePath
	}

	// Resolve per-arm model override
	model := opts.Model
	if m, ok := opts.ArmModels[armIdx]; ok {
		model = m
	}

	// Run the agent
	runOpts := RunOptions{
		Harness:     harnessName,
		Prompt:      prompt,
		Model:       model,
		Effort:      opts.Effort,
		Timeout:     opts.Timeout,
		WorkDir:     workDir,
		Permissions: opts.Permissions,
		Correlation: opts.Correlation,
	}

	result, err := r.Run(runOpts)
	if err != nil {
		arm.ExitCode = 1
		arm.Error = err.Error()
	} else {
		arm.Model = result.Model
		arm.Output = result.Output
		arm.Tokens = result.Tokens
		arm.InputTokens = result.InputTokens
		arm.OutputTokens = result.OutputTokens
		arm.CostUSD = result.CostUSD
		arm.DurationMS = result.DurationMS
		arm.ExitCode = result.ExitCode
		arm.Error = result.Error
	}

	// Capture git diff if we're in a worktree
	if worktreePath != "" {
		arm.Diff = captureGitDiff(worktreePath)
	}

	// Run post-run command if specified
	if opts.PostRun != "" && workDir != "" {
		out, ok := runPostCommand(workDir, opts.PostRun)
		arm.PostRunOut = out
		arm.PostRunOK = &ok
	}

	return arm
}

// createCompareWorktree creates a git worktree for a comparison arm.
// It resolves the git root from the working directory to handle subdirectories.
func createCompareWorktree(workDir, compareID, harnessName string) (string, error) {
	// Resolve the actual git root — workDir may be a subdirectory
	gitRoot, err := resolveGitRoot(workDir)
	if err != nil {
		return "", fmt.Errorf("resolving git root: %w", err)
	}

	wtDir := filepath.Join(gitRoot, ".worktrees", fmt.Sprintf("%s-%s", compareID, harnessName))

	// Create worktree from HEAD (detached)
	cmd := exec.Command("git", "worktree", "add", "--detach", wtDir, "HEAD")
	cmd.Dir = gitRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %s\n%s", err, string(out))
	}
	return wtDir, nil
}

// resolveGitRoot finds the git repository root from any directory within it.
func resolveGitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %s", dir)
	}
	return strings.TrimSpace(string(out)), nil
}

// captureGitDiff captures the unified diff of all changes in a worktree.
func captureGitDiff(worktreePath string) string {
	// Include both tracked modifications and untracked new files
	cmd := exec.Command("git", "diff", "HEAD")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	diff := string(out)

	// Also capture untracked files as a diff-like listing
	cmd2 := exec.Command("git", "diff", "--no-index", "/dev/null", ".")
	cmd2.Dir = worktreePath
	// This will fail (exit 1) when there are differences, which is expected
	// Instead, just add untracked files to the diff
	cmd3 := exec.Command("git", "ls-files", "--others", "--exclude-standard")
	cmd3.Dir = worktreePath
	untrackedOut, _ := cmd3.Output()
	untracked := strings.TrimSpace(string(untrackedOut))
	if untracked != "" {
		// Add each untracked file's content as a pseudo-diff
		for _, f := range strings.Split(untracked, "\n") {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			content, err := os.ReadFile(filepath.Join(worktreePath, f))
			if err != nil {
				continue
			}
			diff += fmt.Sprintf("\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1 @@\n", f)
			for _, line := range strings.Split(string(content), "\n") {
				if line != "" || len(content) > 0 {
					diff += "+" + line + "\n"
				}
			}
		}
	}

	return strings.TrimSpace(diff)
}

// runPostCommand runs a shell command in the given directory.
func runPostCommand(dir, command string) (string, bool) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

// cleanupCompareWorktrees removes worktrees created for a comparison.
func (r *Runner) cleanupCompareWorktrees(repoDir, compareID string) {
	// Resolve git root in case repoDir is a subdirectory
	if root, err := resolveGitRoot(repoDir); err == nil {
		repoDir = root
	}
	wtBase := filepath.Join(repoDir, ".worktrees")
	entries, err := os.ReadDir(wtBase)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), compareID) {
			wtPath := filepath.Join(wtBase, e.Name())
			cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
			cmd.Dir = repoDir
			_ = cmd.Run()
		}
	}
	// Prune stale worktree references
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = repoDir
	_ = cmd.Run()
}

func genCompareID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "cmp-" + hex.EncodeToString(b)
}
