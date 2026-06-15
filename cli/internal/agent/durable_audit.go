package agent

import (
	"context"
	"fmt"
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
// paths when they are dirty. The commit is serialized through the main git lock
// so concurrent workers cannot interleave tracker and audit commits.
func CommitDurableAuditOutputs(projectRoot, attemptID string) error {
	return withTrackerLock(projectRoot, "durable_audit", func() error {
		return commitDurableAuditOutputsLocked(projectRoot, attemptID)
	})
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
	err := lockmetrics.Instrument("index.lock", op, func() error {
		var lastErr error
		var lastDiag string
		for attempt := 0; attempt < gitlock.RecoveryAttempts; attempt++ {
			out, lastErr = runDurableAuditGit(gitDir, args...)
			if lastErr == nil {
				return nil
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
	return out, err
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
