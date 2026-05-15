package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

var durableAuditManagedPathspecs = []string{
	".ddx/beads.jsonl",
	".ddx/beads-archive.jsonl",
	".ddx/metrics/attempts.jsonl",
	".ddx/attachments",
}

type durableAuditBeadReader interface {
	Get(args ...any) (*bead.Bead, error)
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
	return withTrackerLock(projectRoot, func() error {
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := internalgit.Command(ctx, projectRoot, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil
	}

	statusArgs := []string{"status", "--short", "--untracked-files=all", "--"}
	statusArgs = append(statusArgs, durableAuditManagedPathspecs...)
	statusOut, err := internalgit.Command(ctx, projectRoot, statusArgs...).Output()
	if err != nil {
		return fmt.Errorf("checking durable audit status: %w", err)
	}
	dirtyPaths := dirtyDurableAuditPaths(string(statusOut))
	if len(dirtyPaths) == 0 {
		return nil
	}

	addArgs := []string{"add", "-A", "--"}
	addArgs = append(addArgs, dirtyPaths...)
	addOut, err := runGitWithIndexLockRecovery(ctx, projectRoot, addArgs...)
	if err != nil {
		return fmt.Errorf("staging durable audit outputs: %s: %w", strings.TrimSpace(string(addOut)), err)
	}

	cachedArgs := []string{"diff", "--cached", "--"}
	cachedArgs = append(cachedArgs, dirtyPaths...)
	if cached, err := internalgit.Command(ctx, projectRoot, cachedArgs...).Output(); err == nil && strings.TrimSpace(string(cached)) == "" {
		return nil
	}

	commitArgs := []string{"commit", "--no-verify", "--only", "-m", durableAuditCommitMessage(attemptID), "--"}
	commitArgs = append(commitArgs, dirtyPaths...)
	commitOut, err := runGitWithIndexLockRecovery(ctx, projectRoot, commitArgs...)
	if err != nil {
		return fmt.Errorf("committing durable audit outputs: %s: %w", strings.TrimSpace(string(commitOut)), err)
	}
	return nil
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
