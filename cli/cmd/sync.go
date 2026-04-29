package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/spf13/cobra"
)

// ddxManagedPaths are the only paths (relative to git root) that ddx sync is
// allowed to stash, stage, and commit. Nothing outside this list is ever touched.
var ddxManagedPaths = []string{
	".ddx/beads.jsonl",
	".ddx/executions",
	".ddx/plugins",
}

// SyncFailure is persisted to .ddx/sync-failure.json when sync aborts so that
// 'ddx doctor' can surface it without the user having to remember to check.
type SyncFailure struct {
	Timestamp time.Time `json:"timestamp"`
	Reason    string    `json:"reason"`
}

// syncGitRunner is the function type used to invoke git commands in sync operations.
// Extracted for testability — production code uses realSyncGitRun; tests inject fakes.
type syncGitRunner func(ctx context.Context, dir string, args ...string) ([]byte, error)

// realSyncGitRun runs a real git command via the standard gitpkg.Command wrapper.
func realSyncGitRun(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return gitpkg.Command(ctx, dir, args...).CombinedOutput()
}

// checkSyncFailure reads the sync-failure.json at failurePath and returns a
// DiagnosticIssue if a recorded failure is found. Returns nil when the path
// does not exist or is unreadable.
func checkSyncFailure(failurePath string) *DiagnosticIssue {
	data, err := os.ReadFile(failurePath)
	if err != nil {
		return nil
	}
	var failure SyncFailure
	if err := json.Unmarshal(data, &failure); err != nil {
		return nil
	}
	return &DiagnosticIssue{
		Type: "sync_aborted",
		Description: fmt.Sprintf("sync aborted at %s: %s",
			failure.Timestamp.Format(time.RFC3339), failure.Reason),
		Remediation: []string{
			"Resolve the issue manually, then run 'ddx sync'",
			fmt.Sprintf("To clear this warning without syncing: rm %s", failurePath),
		},
	}
}

func (f *CommandFactory) newSyncCommand() *cobra.Command {
	var watchMode bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize DDx-managed files with origin",
		Long: `Synchronize DDx-managed files with origin/main.

The canonical sync flow:
  1. git fetch origin
  2. stash DDx-managed dirty files (.ddx/beads.jsonl, executions/, plugins/)
  3. git merge origin/main  (no rebase — preserves execute-bead history)
  4. git stash pop
  5. commit DDx-managed files with structured messages:
       .ddx/beads.jsonl  → "chore: tracker"
       .ddx/executions/  → "chore: add execution evidence"
  6. git push origin main  (retries once on non-fast-forward)

Sync never touches files outside the DDx-managed allowlist. Non-DDx dirty
files survive unchanged.

On abort (stash-pop conflict, double push failure), a failure record is
written to .ddx/sync-failure.json. Run 'ddx doctor' to surface it.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner := f.syncGitRunnerOverride
			if runner == nil {
				runner = realSyncGitRun
			}
			return f.runSync(cmd, runner, watchMode, interval)
		},
	}
	cmd.Flags().BoolVar(&watchMode, "watch", false, "Run sync on an interval until killed")
	cmd.Flags().DurationVar(&interval, "interval", 15*time.Minute, "Interval between syncs (requires --watch)")
	return cmd
}

func (f *CommandFactory) runSync(cmd *cobra.Command, runner syncGitRunner, watch bool, interval time.Duration) error {
	repoRoot := gitpkg.FindProjectRoot(f.WorkingDir)
	ddxWorkspace := gitpkg.FindNearestDDxWorkspace(f.WorkingDir)
	if ddxWorkspace == "" {
		return fmt.Errorf("sync: no .ddx workspace found; run 'ddx init' first")
	}

	s := &syncer{
		repoRoot: repoRoot,
		ddxDir:   filepath.Join(ddxWorkspace, ".ddx"),
		runner:   runner,
		out:      cmd.OutOrStdout(),
	}

	if !watch {
		return s.run(cmd.Context())
	}

	fmt.Fprintf(s.out, "sync: watching (interval %s, Ctrl-C to stop)\n", interval)
	if err := s.run(cmd.Context()); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "sync: %v\n", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-cmd.Context().Done():
			return nil
		case <-ticker.C:
			if err := s.run(cmd.Context()); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "sync: %v\n", err)
			}
		}
	}
}

// syncer holds the state for a single sync session.
type syncer struct {
	repoRoot string
	ddxDir   string
	runner   syncGitRunner
	out      io.Writer
}

func (s *syncer) git(ctx context.Context, args ...string) ([]byte, error) {
	return s.runner(ctx, s.repoRoot, args...)
}

func (s *syncer) writeFailure(reason string) error {
	failure := SyncFailure{Timestamp: time.Now().UTC(), Reason: reason}
	data, _ := json.Marshal(failure)
	_ = os.MkdirAll(s.ddxDir, 0o755)
	_ = os.WriteFile(filepath.Join(s.ddxDir, "sync-failure.json"), data, 0o644)
	return fmt.Errorf("sync aborted: %s", reason)
}

func (s *syncer) run(ctx context.Context) error {
	return s.runOnce(ctx, false)
}

func (s *syncer) runOnce(ctx context.Context, isRetry bool) error {
	// a. git fetch origin
	fmt.Fprintln(s.out, "sync: fetching origin...")
	if _, err := s.git(ctx, "fetch", "origin"); err != nil {
		return s.writeFailure(fmt.Sprintf("fetch failed: %v", err))
	}

	// b. stash DDx-managed dirty tracked files
	stashed, err := s.stashDDxPaths(ctx)
	if err != nil {
		return s.writeFailure(fmt.Sprintf("stash failed: %v", err))
	}

	// c. git merge origin/main (no rebase — preserves execute-bead history)
	fmt.Fprintln(s.out, "sync: merging origin/main...")
	if out, err := s.git(ctx, "merge", "origin/main"); err != nil {
		if stashed {
			_, _ = s.git(ctx, "stash", "pop")
		}
		return s.writeFailure(fmt.Sprintf("merge failed: %s", strings.TrimSpace(string(out))))
	}

	// d. git stash pop
	if stashed {
		fmt.Fprintln(s.out, "sync: applying stashed changes...")
		if out, err := s.git(ctx, "stash", "pop"); err != nil {
			return s.writeFailure(fmt.Sprintf("stash-pop conflict: resolve manually then run 'git stash pop' (%s)",
				strings.TrimSpace(string(out))))
		}
	}

	// e. commit DDx-managed dirty paths with structured messages
	if err := s.commitDDxPaths(ctx); err != nil {
		return s.writeFailure(fmt.Sprintf("commit failed: %v", err))
	}

	// f. git push origin main; retry once on non-fast-forward
	fmt.Fprintln(s.out, "sync: pushing to origin...")
	pushOut, pushErr := s.git(ctx, "push", "origin", "main")
	if pushErr != nil {
		pushMsg := string(pushOut)
		if isRetry {
			return s.writeFailure(fmt.Sprintf("push failed twice: %s", strings.TrimSpace(pushMsg)))
		}
		if strings.Contains(pushMsg, "non-fast-forward") || strings.Contains(pushMsg, "rejected") {
			fmt.Fprintln(s.out, "sync: push rejected (non-fast-forward), retrying from fetch...")
			return s.runOnce(ctx, true)
		}
		return s.writeFailure(fmt.Sprintf("push failed: %s", strings.TrimSpace(pushMsg)))
	}

	// Clear any existing failure marker on success.
	_ = os.Remove(filepath.Join(s.ddxDir, "sync-failure.json"))
	fmt.Fprintln(s.out, "sync: done")
	return nil
}

// stashDDxPaths stashes tracked dirty files in the DDx-managed allowlist.
// Returns true if a stash was created, false if there was nothing to stash.
// Untracked files (new execution artifacts) are not stashed; git merge
// does not affect them.
func (s *syncer) stashDDxPaths(ctx context.Context) (bool, error) {
	// Check for tracked dirty DDx paths (ignore untracked lines starting with "??").
	checkArgs := append([]string{"status", "--porcelain", "--"}, ddxManagedPaths...)
	out, err := s.git(ctx, checkArgs...)
	if err != nil {
		return false, fmt.Errorf("status check failed: %v", err)
	}
	hasTrackedDirty := false
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" || strings.HasPrefix(line, "??") {
			continue
		}
		hasTrackedDirty = true
		break
	}
	if !hasTrackedDirty {
		return false, nil
	}

	stashArgs := append([]string{"stash", "push", "--"}, ddxManagedPaths...)
	if _, err := s.git(ctx, stashArgs...); err != nil {
		return false, fmt.Errorf("stash push failed: %v", err)
	}
	return true, nil
}

// commitDDxPaths stages and commits DDx-managed paths with structured messages.
// Commits only what is dirty; no-ops if a path group is clean.
func (s *syncer) commitDDxPaths(ctx context.Context) error {
	if err := s.commitIfDirty(ctx, []string{".ddx/beads.jsonl"}, "chore: tracker"); err != nil {
		return err
	}
	if err := s.commitIfDirty(ctx, []string{".ddx/executions", ".ddx/plugins"}, "chore: add execution evidence"); err != nil {
		return err
	}
	return nil
}

// commitIfDirty stages the given paths and commits them with message if they
// have any changes (staged, modified, or untracked). A no-op when clean.
func (s *syncer) commitIfDirty(ctx context.Context, paths []string, message string) error {
	checkArgs := append([]string{"status", "--porcelain", "--"}, paths...)
	out, err := s.git(ctx, checkArgs...)
	if err != nil {
		return fmt.Errorf("status check failed: %v", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil
	}

	addArgs := append([]string{"add", "--"}, paths...)
	if _, err := s.git(ctx, addArgs...); err != nil {
		return fmt.Errorf("git add failed: %v", err)
	}

	if _, err := s.git(ctx, "commit", "-m", message); err != nil {
		return fmt.Errorf("commit failed: %v", err)
	}
	return nil
}
