package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/gitlock"
	"github.com/DocumentDrivenDX/ddx/internal/lockmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/trackerpaths"
)

var (
	durableAuditGitTimeout = 30 * time.Second
	durableAuditGitRunner  = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
		return internalgit.Command(ctx, gitDir, args...).CombinedOutput()
	}
	durableAuditCommandContext = func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), durableAuditGitTimeout)
	}
	// durableAuditIndexLockOwnerLookup resolves the live owner of an index.lock
	// path for the durable-audit cancel-recovery path. Var for testing without lsof.
	durableAuditIndexLockOwnerLookup = gitlock.IndexLockOwner
)

type durableAuditBeadReader interface {
	Get(ctx context.Context, id string) (*bead.Bead, error)
}

// FinalizeDurableAttemptAudit appends the per-attempt metrics row, when the
// report carries an attempt_id, and commits any DDx-managed durable audit files
// that changed during the attempt.
func FinalizeDurableAttemptAudit(projectRoot string, store durableAuditBeadReader, report ExecuteBeadReport) error {
	if strings.TrimSpace(projectRoot) == "" {
		projectRoot = report.ProjectRoot
	}
	if strings.TrimSpace(projectRoot) == "" {
		return fmt.Errorf("finalize durable audit: project root required")
	}
	if strings.TrimSpace(report.AttemptID) != "" {
		if err := attemptmetrics.AppendRow(projectRoot, buildAttemptMetricsRow(store, report, time.Now().UTC())); err != nil {
			return fmt.Errorf("append attempt metrics: %w", err)
		}
	}
	if err := CommitDurableAuditOutputs(projectRoot, report.AttemptID); err != nil {
		return fmt.Errorf("commit durable audit outputs: %w", err)
	}
	return nil
}

// CommitDurableAuditOutputs stages and commits the DDx-managed durable audit
// paths when they are dirty. It waits for any in-flight tracker mutation to
// finish, then releases tracker.lock before running git status/add/commit so
// durable audit commits never hold the tracker lock across index operations.
func CommitDurableAuditOutputs(projectRoot, attemptID string) error {
	if err := withTrackerLock(projectRoot, "durable_audit", func() error { return nil }); err != nil {
		return err
	}
	return commitDurableAuditOutputsLocked(projectRoot, attemptID)
}

func buildAttemptMetricsRow(store durableAuditBeadReader, report ExecuteBeadReport, finishedAt time.Time) attemptmetrics.AttemptRow {
	specID := ""
	if report.BeadID != "" && store != nil {
		if b, err := store.Get(context.Background(), report.BeadID); err == nil && b != nil {
			specID, _ = b.Extra["spec-id"].(string)
		}
	}
	tsEnd := attemptmetrics.Rfc3339(finishedAt)
	tsStart := ""
	if report.DurationMS > 0 {
		start := finishedAt.Add(-time.Duration(report.DurationMS) * time.Millisecond)
		tsStart = attemptmetrics.Rfc3339(start)
	}
	return attemptmetrics.AttemptRow{
		SchemaVersion: attemptmetrics.SchemaVersion,
		AttemptID:     report.AttemptID,
		BeadID:        report.BeadID,
		SessionID:     report.SessionID,
		TSStart:       tsStart,
		TSEnd:         tsEnd,
		Model:         report.Model,
		Harness:       report.Harness,
		Profile:       report.RequestedProfile,
		Provider:      report.Provider,
		SpecID:        specID,
		Outcome:       report.Status,
		CostUSD:       report.CostUSD,
		DurationMS:    int(report.DurationMS),
		ReviewVerdict: report.ReviewVerdict,
	}
}

func commitDurableAuditOutputsLocked(projectRoot, attemptID string) error {
	gitDir, managedPathspecs := ddxStateGitScope(projectRoot, trackerpaths.ManagedPathspecs()...)

	out, err := runDurableAuditGit(gitDir, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil
	}

	// --ignored=matching surfaces DDx-managed audit paths that the project
	// intentionally gitignores (e.g. .ddx/metrics, .ddx/attachments) so they are
	// still detected as dirty here. The pathspec scope keeps this limited to
	// managed paths, so unrelated ignored files are never reported.
	statusArgs := []string{"status", "--short", "--untracked-files=all", "--ignored=matching", "--"}
	statusArgs = append(statusArgs, managedPathspecs...)
	statusOut, err := runDurableAuditGit(gitDir, statusArgs...)
	if err != nil {
		return fmt.Errorf("checking durable audit status: %w", err)
	}
	dirtyPaths := dirtyDurableAuditPaths(string(statusOut))
	if len(dirtyPaths) == 0 {
		return nil
	}

	// -f force-stages DDx-managed audit paths even when the project gitignores
	// them; the pathspec list is restricted to managed dirty paths reported by
	// the status above, so this never force-adds arbitrary ignored files.
	addArgs := []string{"add", "-f", "-A", "--"}
	addArgs = append(addArgs, dirtyPaths...)
	addOut, err := runDurableAuditGitWithIndexLockRecovery(gitDir, addArgs...)
	if err != nil {
		return fmt.Errorf("staging durable audit outputs: %s: %w", strings.TrimSpace(string(addOut)), err)
	}

	cachedArgs := []string{"diff", "--cached", "--"}
	cachedArgs = append(cachedArgs, dirtyPaths...)
	if cached, err := runDurableAuditGit(gitDir, cachedArgs...); err == nil && strings.TrimSpace(string(cached)) == "" {
		return nil
	}

	commitArgs := []string{"commit", "--no-verify", "--only", "-m", durableAuditCommitMessage(attemptID), "--"}
	commitArgs = append(commitArgs, dirtyPaths...)
	commitArgs = ddxStateCommitArgs(projectRoot, gitDir, commitArgs...)
	commitOut, err := runDurableAuditGitWithIndexLockRecovery(gitDir, commitArgs...)
	if err != nil {
		if durableAuditPathsClean(gitDir, dirtyPaths) {
			return nil
		}
		return fmt.Errorf("committing durable audit outputs: %s: %w", strings.TrimSpace(string(commitOut)), err)
	}
	return nil
}

func runDurableAuditGit(gitDir string, args ...string) ([]byte, error) {
	ctx, cancel := durableAuditCommandContext()
	defer cancel()
	return durableAuditGitRunner(ctx, gitDir, args...)
}

func runDurableAuditGitWithIndexLockRecovery(gitDir string, args ...string) ([]byte, error) {
	var out []byte
	op := durableAuditIndexOperation(args)
	timeout := durableAuditIndexGitTimeout()
	err := lockmetrics.Instrument("index.lock", op, func() error {
		var lastErr error
		var lastDiag string
		for attempt := 0; attempt < gitlock.RecoveryAttempts; attempt++ {
			out, lastErr = runDurableAuditGitWithTimeout(gitDir, timeout, args...)
			if lastErr == nil {
				return nil
			}
			if errors.Is(lastErr, context.DeadlineExceeded) || errors.Is(lastErr, context.Canceled) {
				return lastErr
			}
			if !gitlock.IsTransientGitContention(string(out), lastErr) {
				return lastErr
			}
			result, recErr := gitlock.RecoverGitIndexLock(gitDir)
			if recErr != nil {
				return fmt.Errorf("%s; index-lock recovery failed: %w", strings.TrimSpace(string(out)), recErr)
			}
			lastDiag = result.Reason
			if !result.Removed {
				time.Sleep(gitlock.LiveOwnerWait)
			}
		}
		return fmt.Errorf("%s; index-lock recovery exhausted after %d attempts: %s", strings.TrimSpace(string(out)), gitlock.RecoveryAttempts, lastDiag)
	})
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		recoverStaleIndexLockAfterDurableAuditCancel(gitDir)
	}
	return out, err
}

// recoverStaleIndexLockAfterDurableAuditCancel removes the durable-audit git
// dir's index.lock after a capped/cancelled commit, but only when the lock has
// no live owner. A killed git subprocess leaves a fresh (age < gitlock.StaleAge)
// unowned lock that RecoverGitIndexLock would refuse to remove; this targeted
// cleanup clears it. A lock held by a live concurrent git process is left untouched.
func recoverStaleIndexLockAfterDurableAuditCancel(gitDir string) {
	lockPath := worktreeIndexLockPath(gitDir)
	if _, statErr := os.Lstat(lockPath); statErr != nil {
		return
	}
	if pid, _ := durableAuditIndexLockOwnerLookup(lockPath); pid > 0 && processAlive(pid) {
		return
	}
	_ = os.Remove(lockPath)
}

func runDurableAuditGitWithTimeout(gitDir string, timeout time.Duration, args ...string) ([]byte, error) {
	if timeout <= 0 || timeout == durableAuditGitTimeout {
		return runDurableAuditGit(gitDir, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := durableAuditGitRunner(ctx, gitDir, args...)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return out, ctxErr
		}
	}
	return out, err
}

func durableAuditIndexGitTimeout() time.Duration {
	timeout := durableAuditGitTimeout
	if cfg := lockmetrics.CapConfigFor("index.lock"); cfg.Cap > 0 && (timeout <= 0 || cfg.Cap < timeout) {
		timeout = cfg.Cap
		// Keep the git subprocess comfortably below the active cap so ordinary
		// durable-audit commits do not trigger the watchdog boundary.
		return timeout - minDuration(timeout/10, time.Second)
	}
	return timeout
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func durableAuditPathsClean(gitDir string, paths []string) bool {
	if len(paths) == 0 {
		return true
	}
	statusArgs := []string{"status", "--short", "--untracked-files=all", "--ignored=matching", "--"}
	statusArgs = append(statusArgs, paths...)
	statusOut, err := runDurableAuditGit(gitDir, statusArgs...)
	if err != nil {
		return false
	}
	if strings.TrimSpace(string(statusOut)) != "" {
		return false
	}
	cachedArgs := []string{"diff", "--cached", "--"}
	cachedArgs = append(cachedArgs, paths...)
	cachedOut, err := runDurableAuditGit(gitDir, cachedArgs...)
	return err == nil && strings.TrimSpace(string(cachedOut)) == ""
}

func durableAuditIndexOperation(args []string) string {
	if len(args) == 0 {
		return "index"
	}
	return "index." + args[0]
}

func dirtyDurableAuditPaths(statusOutput string) []string {
	lines := strings.Split(statusOutput, "\n")
	seen := make(map[string]struct{}, len(lines))
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if arrow := strings.LastIndex(path, " -> "); arrow >= 0 {
			path = strings.TrimSpace(path[arrow+4:])
		}
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func durableAuditCommitMessage(attemptID string) string {
	id := strings.TrimSpace(attemptID)
	if id == "" {
		id = time.Now().UTC().Format("20060102T150405")
	}
	return fmt.Sprintf("chore: update tracker (execute-bead %s)", id)
}
