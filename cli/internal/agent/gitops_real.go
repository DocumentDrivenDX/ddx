package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// RealGitOps implements GitOps via os/exec git commands.
type RealGitOps struct{}

func (r *RealGitOps) HeadRev(dir string) (string, error) {
	return r.ResolveRev(dir, "HEAD")
}

func (r *RealGitOps) ResolveRev(dir, rev string) (string, error) {
	out, err := internalgit.Command(context.Background(), dir, "rev-parse", rev).Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", rev, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *RealGitOps) WorktreeAdd(dir, wtPath, rev string) error {
	out, err := internalgit.Command(context.Background(), dir, "worktree", "add", "--detach", wtPath, rev).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (r *RealGitOps) WorktreeRemove(dir, wtPath string) error {
	out, err := internalgit.Command(context.Background(), dir, "worktree", "remove", "--force", wtPath).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))

	// Interrupted `git worktree add` leaves admin dir "locked" (often with
	// reason "initializing"). Unlock and retry force-remove so cleanup can
	// reclaim multi-GB orphans instead of preserving them forever.
	if unlockGitWorktreeAdmin(dir, wtPath) {
		out2, err2 := internalgit.Command(context.Background(), dir, "worktree", "remove", "--force", wtPath).CombinedOutput()
		if err2 == nil {
			return nil
		}
		msg = strings.TrimSpace(string(out2))
		err = err2
	}

	// Path already unregistered but checkout leftover remains, or registry
	// entry is a ghost: prune + best-effort RemoveAll.
	_ = r.WorktreePrune(dir)
	if _, statErr := os.Stat(wtPath); statErr == nil {
		if rmErr := os.RemoveAll(wtPath); rmErr == nil {
			_ = r.WorktreePrune(dir)
			return nil
		}
	} else if os.IsNotExist(statErr) && strings.Contains(msg, "is not a working tree") {
		_ = r.WorktreePrune(dir)
		return nil
	}

	return fmt.Errorf("git worktree remove: %s: %w", msg, err)
}

func (r *RealGitOps) WorktreeList(dir string) ([]string, error) {
	out, err := internalgit.Command(context.Background(), dir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			paths = append(paths, strings.TrimPrefix(line, "worktree "))
		}
	}
	return paths, nil
}

func (r *RealGitOps) WorktreePrune(dir string) error {
	return internalgit.Command(context.Background(), dir, "worktree", "prune").Run()
}

// unlockGitWorktreeAdmin removes a stale "locked" file under
// .git/worktrees/<name>/ so `git worktree remove --force` can proceed after
// a crashed or hung `git worktree add`. Returns true when a lock file was
// removed (caller should retry remove).
func unlockGitWorktreeAdmin(repoDir, wtPath string) bool {
	name := filepath.Base(filepath.Clean(wtPath))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return false
	}
	// Common layout: <repo>/.git/worktrees/<basename>/locked
	candidates := []string{
		filepath.Join(repoDir, ".git", "worktrees", name, "locked"),
	}
	// Linked worktrees / gitdir files: resolve .git file in the worktree if present.
	if data, err := os.ReadFile(filepath.Join(wtPath, ".git")); err == nil {
		line := strings.TrimSpace(string(data))
		if strings.HasPrefix(line, "gitdir: ") {
			gitdir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
			if !filepath.IsAbs(gitdir) {
				gitdir = filepath.Join(wtPath, gitdir)
			}
			candidates = append(candidates, filepath.Join(gitdir, "locked"))
		}
	}
	removed := false
	for _, lockPath := range candidates {
		if err := os.Remove(lockPath); err == nil {
			removed = true
		}
	}
	return removed
}

// UpdateRef updates ref in dir to sha via `git update-ref`.
func (r *RealGitOps) UpdateRef(dir, ref, sha string) error {
	out, err := internalgit.Command(context.Background(), dir, "update-ref", ref, sha).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git update-ref %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// DeleteRef removes ref from dir via `git update-ref -d`.
func (r *RealGitOps) DeleteRef(dir, ref string) error {
	out, err := internalgit.Command(context.Background(), dir, "update-ref", "-d", ref).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git update-ref -d %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// IsDirty reports whether dir has any uncommitted changes (tracked modifications or untracked files).
func (r *RealGitOps) IsDirty(dir string) (bool, error) {
	out, _ := internalgit.Command(context.Background(), dir, "status", "--porcelain", "--", ".", ":(exclude)"+ExecutionCleanupMetadataFileName).Output()
	return len(bytes.TrimSpace(out)) > 0, nil
}

func dirtyWorktreePaths(dir string) []string {
	out, err := internalgit.Command(context.Background(), dir, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		return nil
	}
	var paths []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = strings.TrimSpace(path[idx+4:])
		}
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

// EvidenceReviewExcludePathspecs returns git pathspec-exclusion fragments
// applied when review-prompt synthesis reads legacy revisions from before
// execution evidence became local-only. Those histories can contain embedded
// logs, prompt.md, and usage.json; filter that historical noise so it cannot
// inflate current reviewer prompts. New execution paths never commit evidence.
//
// Regression anchor: ddx-39e27896.
func EvidenceReviewExcludePathspecs() []string {
	return []string{
		":(exclude,glob).ddx/executions/*/embedded/**",
		":(exclude,glob).ddx/executions/*/prompt.md",
		":(exclude,glob).ddx/executions/*/usage.json",
	}
}

// SynthesizeCommit stages real file changes, explicitly excluding harness noise
// paths, and creates a commit with msg as the commit message. Returns (true, nil)
// when a commit was made, (false, nil) when nothing real remained to commit
// after exclusions, and (false, err) on failure.
func (r *RealGitOps) SynthesizeCommit(dir, msg string) (bool, error) {
	// Do NOT list already-gitignored paths (.ddx/agent-logs, .ddx/workers) as
	// :(exclude) pathspecs. Git treats a path named by :(exclude) as explicitly
	// referenced, so when the path is also .gitignored git emits "The following
	// paths are ignored by one of your .gitignore files" AND exits 1 — even
	// though the pathspec is trying to SKIP it. Paths already in .gitignore are
	// excluded by default; excludes here are only for paths that would
	// otherwise be tracked.
	addArgs := []string{
		"add", "-A", "--",
		".",
	}
	addArgs = append(addArgs, synthesizeCommitExcludePathspecs(dir)...)
	if err := internalgit.Command(context.Background(), dir, addArgs...).Run(); err != nil {
		return false, fmt.Errorf("staging changes: %w", err)
	}
	// Execution evidence is per-machine state. The pathspecs avoid staging it
	// under normal ignore configurations; this reset is the fail-safe for stale
	// or selectively-unignored project rules and preserves every file on disk.
	if err := internalgit.Command(context.Background(), dir, "reset", "-q", "HEAD", "--", ".ddx/executions").Run(); err != nil {
		return false, fmt.Errorf("excluding local execution evidence from synthesized commit: %w", err)
	}
	statusOut, _ := internalgit.Command(context.Background(), dir, "diff", "--cached", "--name-only").Output()
	if len(bytes.TrimSpace(statusOut)) == 0 {
		return false, nil
	}
	if msg == "" {
		msg = "chore: execute-bead synthesized result commit"
	}
	out, err := internalgit.Command(context.Background(), dir, "commit", "-m", msg).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("synthesize commit: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return true, nil
}

func synthesizeCommitExcludePathspecs(dir string) []string {
	// Never-deleted main-git stale-break guard sidecar. Derived from the
	// package-local helper so the suffix cannot drift from the on-disk path.
	// Code-level exclusion (independent of project .gitignore).
	guardRel := filepath.ToSlash(trackerStaleLockBreakGuardPath(filepath.Join(ddxroot.DirName, ".git-tracker.lock")))
	candidates := []struct {
		pathspec    string
		ignoreProbe string
	}{
		{
			pathspec:    ":(exclude,glob).ddx/executions/**",
			ignoreProbe: ".ddx/executions/.ddx-check-ignore",
		},
		{
			pathspec:    ":(exclude).claude/skills",
			ignoreProbe: ".claude/skills",
		},
		{
			pathspec:    ":(exclude).agents/skills",
			ignoreProbe: ".agents/skills",
		},
		{
			pathspec:    ":(exclude)" + ExecutionCleanupMetadataFileName,
			ignoreProbe: ExecutionCleanupMetadataFileName,
		},
		{
			// Tracker-lock coordination dir (.ddx/.git-tracker.lock/{pid,
			// acquired_at}). Present while withTrackerLock is held — must
			// not be staged by a SynthesizeCommit running inside the lock
			// (regression: HEAD-race fix folded SynthesizeCommit into the
			// locked critical section, exposing this directory to
			// `git add -A`).
			pathspec:    ":(exclude).ddx/.git-tracker.lock",
			ignoreProbe: ".ddx/.git-tracker.lock/pid",
		},
		{
			// Stable stale-break advisory sidecar left by single-winner disposal.
			pathspec:    ":(exclude)" + guardRel,
			ignoreProbe: guardRel,
		},
	}

	pathspecs := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if isGitIgnored(dir, c.ignoreProbe) {
			continue
		}
		pathspecs = append(pathspecs, c.pathspec)
	}
	return pathspecs
}

func isGitIgnored(dir, path string) bool {
	err := internalgit.Command(context.Background(), dir, "check-ignore", "-q", "--", path).Run()
	return err == nil
}
