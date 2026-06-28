package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

var errExecutionCleanupAttemptProcessUnavailable = errors.New("execution cleanup attempt process census unavailable")

type executionCleanupAttemptProcessScanner interface {
	Scan(context.Context) ([]executionCleanupAttemptProcess, error)
}

type executionCleanupAttemptProcessKiller interface {
	KillGroup(int) error
}

type executionCleanupAttemptProcessKillerFunc func(int) error

func (f executionCleanupAttemptProcessKillerFunc) KillGroup(pgid int) error {
	if f == nil {
		return nil
	}
	return f(pgid)
}

type executionCleanupAttemptProcess struct {
	PID       int
	PPID      int
	PGID      int
	Command   string
	Cwd       string
	Worktree  string
	StartedAt time.Time
}

type executionCleanupAttemptProcessGroup struct {
	GroupID int
	Members []executionCleanupAttemptProcess
}

func newExecutionCleanupAttemptProcessScanner() executionCleanupAttemptProcessScanner {
	return newExecutionCleanupAttemptProcessScannerImpl()
}

func newExecutionCleanupAttemptProcessKiller() executionCleanupAttemptProcessKiller {
	return executionCleanupAttemptProcessKillerFunc(killProcessGroup)
}

func (m *ExecutionCleanupManager) cleanupStaleAttemptProcessGroups(
	ctx context.Context,
	summary *ExecutionCleanupSummary,
	runStates []RunState,
	registered map[string]struct{},
	probe ExecutionCleanupLivenessProbe,
	now time.Time,
) error {
	if m == nil || summary == nil {
		return nil
	}
	if probe == nil {
		probe = defaultExecutionCleanupLivenessProbe{}
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	scanner := m.attemptProcessScanner
	if scanner == nil {
		scanner = newExecutionCleanupAttemptProcessScanner()
	}
	if scanner == nil {
		return nil
	}
	processes, err := scanner.Scan(ctx)
	if err != nil {
		if errors.Is(err, errExecutionCleanupAttemptProcessUnavailable) {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    m.ProjectRoot,
				Class:   "attempt_process_cleanup_unavailable",
				Message: err.Error(),
			})
			summary.ProcessFindings = append(summary.ProcessFindings, ExecutionCleanupProcessFinding{
				StaleReason: err.Error(),
			})
			return nil
		}
		summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
			Path:    m.ProjectRoot,
			Class:   "attempt_process_census",
			Message: err.Error(),
		})
		return nil
	}

	for _, group := range groupExecutionCleanupAttemptProcesses(processes) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		finding, stale, err := m.classifyExecutionCleanupAttemptProcessGroup(group.Members, runStates, registered, probe, now, summary)
		if err != nil {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    m.ProjectRoot,
				Class:   "attempt_process_classify",
				Message: err.Error(),
			})
			continue
		}
		if !stale {
			continue
		}
		finding.WouldKill = true
		if !m.DryRun {
			killer := m.attemptProcessKiller
			if killer == nil {
				killer = newExecutionCleanupAttemptProcessKiller()
			}
			if killer != nil {
				if killErr := killer.KillGroup(finding.PGID); killErr != nil {
					summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
						Path:    finding.Worktree,
						Class:   "attempt_process_kill",
						Message: killErr.Error(),
					})
					summary.ProcessFindings = append(summary.ProcessFindings, finding)
					continue
				}
			}
			finding.Terminated = true
		}
		summary.ProcessFindings = append(summary.ProcessFindings, finding)
	}
	return nil
}

func (m *ExecutionCleanupManager) classifyExecutionCleanupAttemptProcessGroup(
	group []executionCleanupAttemptProcess,
	runStates []RunState,
	registered map[string]struct{},
	probe ExecutionCleanupLivenessProbe,
	now time.Time,
	summary *ExecutionCleanupSummary,
) (ExecutionCleanupProcessFinding, bool, error) {
	if len(group) == 0 {
		return ExecutionCleanupProcessFinding{}, false, nil
	}
	var staleProc *executionCleanupAttemptProcess
	var staleMeta ExecutionCleanupMetadata
	var staleReason string

	for i := range group {
		proc := group[i]
		if proc.PID <= 0 || proc.Worktree == "" {
			continue
		}
		if !isPathWithin(proc.Worktree, m.tempRoot()) {
			continue
		}

		meta, err := ReadExecutionCleanupMetadata(proc.Worktree)
		candidateRunStates := runStates
		var matchedRunState *RunState

		switch {
		case err == nil:
			if _, ok := registered[filepath.Clean(proc.Worktree)]; ok {
				// Registered worktrees are active project state and are preserved
				// even when the run-state file has already gone stale.
				continue
			}
			if meta.ProjectRoot != "" && !sameCleanPath(meta.ProjectRoot, m.ProjectRoot) {
				if !m.canReclaimForeignTestOwnedPath(meta.ProjectRoot, proc.Worktree) {
					continue
				}
				candidateRunStates = m.runStatesForMetadata(meta, runStates, summary)
			}
			matchedRunState = matchingRunStateForMeta(candidateRunStates, meta)
		case errors.Is(err, os.ErrNotExist):
			if _, ok := registered[filepath.Clean(proc.Worktree)]; ok {
				// Registered worktree without metadata stays conservative and
				// does not get reaped from the process census alone.
				continue
			}
			matchedRunState = matchingRunStateForMeta(runStates, ExecutionCleanupMetadata{WorktreePath: proc.Worktree})
			if matchedRunState == nil {
				continue
			}
			meta = candidateCycleMetadataFromRunState(*matchedRunState)
			meta.ProjectRoot = m.ProjectRoot
		default:
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    proc.Worktree,
				Class:   "attempt_process_metadata_read",
				Message: err.Error(),
			})
			continue
		}

		live, reason := probe.IsLive(meta, matchedRunState, now)
		if live {
			return ExecutionCleanupProcessFinding{}, false, nil
		}

		if staleProc == nil {
			staleCopy := proc
			staleProc = &staleCopy
			staleMeta = meta
			staleReason = reason
		}
	}

	if staleProc == nil {
		return ExecutionCleanupProcessFinding{}, false, nil
	}

	finding := ExecutionCleanupProcessFinding{
		PID:         staleProc.PID,
		PPID:        staleProc.PPID,
		PGID:        staleProc.PGID,
		BeadID:      staleMeta.BeadID,
		AttemptID:   staleMeta.AttemptID,
		Command:     staleProc.Command,
		Cwd:         staleProc.Cwd,
		Worktree:    staleProc.Worktree,
		StartedAt:   staleProc.StartedAt,
		StaleReason: staleReason,
		WouldKill:   true,
		Members:     make([]ExecutionCleanupProcessFact, 0, len(group)),
	}
	if finding.PGID <= 0 {
		finding.PGID = finding.PID
	}
	for _, proc := range group {
		finding.Members = append(finding.Members, ExecutionCleanupProcessFact(proc))
	}
	return finding, true, nil
}

func groupExecutionCleanupAttemptProcesses(processes []executionCleanupAttemptProcess) []executionCleanupAttemptProcessGroup {
	if len(processes) == 0 {
		return nil
	}
	buckets := make(map[int][]executionCleanupAttemptProcess, len(processes))
	for _, proc := range processes {
		if proc.PID <= 0 || proc.PID == os.Getpid() {
			continue
		}
		groupID := proc.PGID
		if groupID <= 0 {
			groupID = proc.PID
		}
		buckets[groupID] = append(buckets[groupID], proc)
	}
	if len(buckets) == 0 {
		return nil
	}

	groups := make([]executionCleanupAttemptProcessGroup, 0, len(buckets))
	for groupID, members := range buckets {
		sort.Slice(members, func(i, j int) bool {
			if members[i].PID == members[j].PID {
				return members[i].Command < members[j].Command
			}
			return members[i].PID < members[j].PID
		})
		groups = append(groups, executionCleanupAttemptProcessGroup{
			GroupID: groupID,
			Members: members,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].GroupID == groups[j].GroupID {
			return pickExecutionCleanupAttemptRepresentative(groups[i].Members).PID < pickExecutionCleanupAttemptRepresentative(groups[j].Members).PID
		}
		return groups[i].GroupID < groups[j].GroupID
	})
	return groups
}

func pickExecutionCleanupAttemptRepresentative(processes []executionCleanupAttemptProcess) executionCleanupAttemptProcess {
	if len(processes) == 0 {
		return executionCleanupAttemptProcess{}
	}
	for _, proc := range processes {
		if proc.PID == proc.PGID && proc.PID > 0 {
			return proc
		}
	}
	return processes[0]
}

func executionCleanupAttemptWorktreeRoot(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.TrimSuffix(path, " (deleted)")
	if !strings.Contains(path, ExecuteBeadWtPrefix) {
		return ""
	}
	start := strings.Index(path, ExecuteBeadWtPrefix)
	if start < 0 {
		return ""
	}
	end := start + len(ExecuteBeadWtPrefix)
	for end < len(path) {
		switch path[end] {
		case '/', '\\', ' ', '\t', '\x00':
			return filepath.Clean(path[:end])
		}
		end++
	}
	return filepath.Clean(path)
}

func executionCleanupAttemptFactFromRaw(proc executionCleanupAttemptProcess) ExecutionCleanupProcessFact {
	return ExecutionCleanupProcessFact(proc)
}

func executionCleanupAttemptProcessFromWorkerStatus(cmdline, cwd string, pid, ppid, pgid int, startedAt time.Time) executionCleanupAttemptProcess {
	_, worktree := workerstatus.InferBead(cmdline, cwd)
	worktree = executionCleanupAttemptWorktreeRoot(worktree)
	return executionCleanupAttemptProcess{
		PID:       pid,
		PPID:      ppid,
		PGID:      pgid,
		Command:   cmdline,
		Cwd:       cwd,
		Worktree:  worktree,
		StartedAt: startedAt,
	}
}
