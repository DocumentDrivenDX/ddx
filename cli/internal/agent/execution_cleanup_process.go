package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ExecutionCleanupProcessInfo is one host process record used by the cleanup
// manager's conservative stale-attempt process census.
type ExecutionCleanupProcessInfo struct {
	PID     int
	PPID    int
	PGID    int
	Cwd     string
	Command string
}

// ExecutionCleanupProcessScanner scans host processes. Tests inject fakes; the
// real implementation is platform-specific.
type ExecutionCleanupProcessScanner interface {
	ScanExecutionProcesses(context.Context) ([]ExecutionCleanupProcessInfo, error)
}

// ExecutionCleanupProcessKiller terminates one process group or pid. Tests
// inject fakes so cleanup policy can be verified without killing processes.
type ExecutionCleanupProcessKiller interface {
	KillExecutionProcessGroup(context.Context, int) error
}

type executionCleanupProcessScannerFunc func(context.Context) ([]ExecutionCleanupProcessInfo, error)

func (f executionCleanupProcessScannerFunc) ScanExecutionProcesses(ctx context.Context) ([]ExecutionCleanupProcessInfo, error) {
	return f(ctx)
}

type executionCleanupProcessKillerFunc func(context.Context, int) error

func (f executionCleanupProcessKillerFunc) KillExecutionProcessGroup(ctx context.Context, pgid int) error {
	return f(ctx, pgid)
}

type cleanupProcessWorktree struct {
	path string
	meta ExecutionCleanupMetadata
}

func (m *ExecutionCleanupManager) cleanupAttemptDescendantProcesses(ctx context.Context, summary *ExecutionCleanupSummary, runStates []RunState, registered map[string]struct{}, probe ExecutionCleanupLivenessProbe, now time.Time) error {
	scanner := m.ProcessScanner
	if scanner == nil {
		scanner = defaultExecutionCleanupProcessScanner()
	}
	if scanner == nil {
		summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
			Path:    summary.TempRoot,
			Class:   "process_cleanup_unavailable",
			Message: "process cleanup is unavailable on this platform",
		})
		return nil
	}
	killer := m.ProcessKiller
	if killer == nil {
		killer = defaultExecutionCleanupProcessKiller()
	}

	worktrees, err := m.cleanupProcessWorktrees(summary.TempRoot)
	if err != nil {
		summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
			Path:    summary.TempRoot,
			Class:   "process_worktree_scan",
			Message: err.Error(),
		})
		return nil
	}
	if len(worktrees) == 0 {
		return nil
	}

	processes, err := scanner.ScanExecutionProcesses(ctx)
	if err != nil {
		summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
			Path:    summary.TempRoot,
			Class:   "process_scan",
			Message: err.Error(),
		})
		return nil
	}
	summary.ScannedProcesses = len(processes)

	groupSeen := map[int]struct{}{}
	for _, proc := range processes {
		if proc.PID <= 0 {
			continue
		}
		wt, ok := matchingCleanupProcessWorktree(proc, worktrees)
		if !ok {
			continue
		}
		matchedRunState := matchingRunStateForMeta(runStates, wt.meta)
		live, reason := probe.IsLive(wt.meta, matchedRunState, now)
		finding := ExecutionCleanupProcessFinding{
			PID:          proc.PID,
			PPID:         proc.PPID,
			PGID:         proc.PGID,
			Command:      truncateCleanupCommand(proc.Command),
			Cwd:          proc.Cwd,
			WorktreePath: wt.path,
			BeadID:       firstNonEmptyString(wt.meta.BeadID, runStateBeadID(matchedRunState)),
			AttemptID:    firstNonEmptyString(wt.meta.AttemptID, runStateAttemptID(matchedRunState)),
			Reason:       reason,
		}
		if live {
			finding.Preserved = true
			summary.PreservedAttemptProcesses++
			summary.Processes = append(summary.Processes, finding)
			continue
		}
		if _, ok := registered[filepath.Clean(wt.path)]; ok {
			finding.Preserved = true
			finding.Reason = "registered worktree"
			summary.PreservedAttemptProcesses++
			summary.Processes = append(summary.Processes, finding)
			continue
		}
		groupID := cleanupProcessGroupID(proc)
		if _, seen := groupSeen[groupID]; seen {
			continue
		}
		groupSeen[groupID] = struct{}{}
		finding.WouldKill = true
		finding.Reason = firstNonEmptyString(finding.Reason, "stale attempt descendant")
		summary.StaleAttemptProcesses++
		if !m.DryRun {
			if killer == nil {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    wt.path,
					Class:   "process_kill_unavailable",
					Message: fmt.Sprintf("no process killer for pid=%d pgid=%d", proc.PID, proc.PGID),
				})
			} else if err := killer.KillExecutionProcessGroup(ctx, groupID); err != nil {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    wt.path,
					Class:   "process_kill",
					Message: err.Error(),
				})
			} else {
				finding.Killed = true
				summary.ReapedProcessGroups++
			}
		}
		summary.Processes = append(summary.Processes, finding)
	}
	if len(summary.Processes) > 0 {
		summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
			Path:    summary.TempRoot,
			Class:   "attempt_process_census",
			Message: fmt.Sprintf("stale=%d preserved=%d reaped_groups=%d", summary.StaleAttemptProcesses, summary.PreservedAttemptProcesses, summary.ReapedProcessGroups),
		})
	}
	return nil
}

func (m *ExecutionCleanupManager) cleanupProcessWorktrees(tempRoot string) ([]cleanupProcessWorktree, error) {
	entries, err := os.ReadDir(tempRoot)
	if err != nil {
		return nil, err
	}
	worktrees := make([]cleanupProcessWorktree, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !hasAnyPrefix(entry.Name(), executionAttemptDirPrefixes()) {
			continue
		}
		path := filepath.Join(tempRoot, entry.Name())
		meta, err := ReadExecutionCleanupMetadata(path)
		if err != nil {
			meta = ExecutionCleanupMetadata{ProjectRoot: m.ProjectRoot, WorktreePath: path}
		}
		if meta.ProjectRoot != "" && !sameCleanPath(meta.ProjectRoot, m.ProjectRoot) {
			continue
		}
		meta.ProjectRoot = firstNonEmptyString(meta.ProjectRoot, m.ProjectRoot)
		meta.WorktreePath = firstNonEmptyString(meta.WorktreePath, path)
		worktrees = append(worktrees, cleanupProcessWorktree{path: path, meta: meta})
	}
	sort.Slice(worktrees, func(i, j int) bool {
		return len(worktrees[i].path) > len(worktrees[j].path)
	})
	return worktrees, nil
}

func matchingCleanupProcessWorktree(proc ExecutionCleanupProcessInfo, worktrees []cleanupProcessWorktree) (cleanupProcessWorktree, bool) {
	for _, wt := range worktrees {
		if processCwdWithin(proc.Cwd, wt.path) || processCommandMentionsAttempt(proc.Command, wt.path) {
			return wt, true
		}
	}
	return cleanupProcessWorktree{}, false
}

func cleanupProcessGroupID(proc ExecutionCleanupProcessInfo) int {
	if proc.PGID > 0 {
		return proc.PGID
	}
	return proc.PID
}

func truncateCleanupCommand(command string) string {
	command = strings.TrimSpace(command)
	if len(command) <= 200 {
		return command
	}
	return command[:200]
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func runStateBeadID(state *RunState) string {
	if state == nil {
		return ""
	}
	return state.BeadID
}

func runStateAttemptID(state *RunState) string {
	if state == nil {
		return ""
	}
	return state.AttemptID
}
