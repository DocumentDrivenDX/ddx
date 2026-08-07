package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

const (
	defaultWorkerDirStaleAge          = 30 * time.Minute
	executionRetentionArchiveRoot     = ".ddx/executions-archive"
	executionRetentionOverrideEnv     = "DDX_EXECUTION_RETAIN_DAYS"
	executionRetentionTimestampLayout = "20060102T150405"
)

type executionRetentionMode int

const (
	executionRetentionDisabled executionRetentionMode = iota
	executionRetentionArchive
	executionRetentionDelete
)

type executionRetentionPolicy struct {
	days int
	mode executionRetentionMode
}

type startupHousekeepingReport struct {
	ProjectRoot string
	TempRoot    string
	runStates   []agent.RunState
	registered  map[string]struct{}

	StaleWorktrees     int64
	StaleWorkerDirs    int64
	StaleExecutionDirs int64

	// WorkerSubprocessCount and TempWorktreeCount are non-destructive
	// observation counts: the total number of worker liveness dirs and temp
	// worktree dirs currently present, regardless of staleness. They exist
	// so resource-pressure reporting can see current resource usage without
	// waiting for the staleness thresholds that gate cleanup eligibility.
	WorkerSubprocessCount int64
	TempWorktreeCount     int64

	RemovedRegisteredWorktrees  int64
	RemovedUnregisteredTempDirs int64
	RemovedWorkerDirs           int64
	ArchivedExecutionDirs       int64
	DeletedExecutionDirs        int64

	Warnings []agent.ExecutionCleanupWarning
}

type startupHousekeepingRunner struct {
	projectRoot     string
	tempRoot        string
	worktreeMaxAge  time.Duration
	workerDirMaxAge time.Duration
	now             func() time.Time
	retentionPolicy executionRetentionPolicy
	processCleanup  startupProcessCleanupFunc
}

func newStartupHousekeepingRunner(projectRoot string) *startupHousekeepingRunner {
	return &startupHousekeepingRunner{
		projectRoot:     projectRoot,
		tempRoot:        config.ExecutionTempRoot(projectRoot),
		worktreeMaxAge:  worktreeReapMaxAgeFromEnv(),
		workerDirMaxAge: defaultWorkerDirStaleAge,
		retentionPolicy: resolveExecutionRetentionPolicy(projectRoot),
		processCleanup:  defaultStartupProcessCleanup,
	}
}

func (r *startupHousekeepingRunner) Cleanup(ctx context.Context) (agent.ExecutionCleanupSummary, error) {
	if r == nil {
		return agent.ExecutionCleanupSummary{}, nil
	}
	report, err := r.scan(ctx, true)
	if err != nil {
		return agent.ExecutionCleanupSummary{
			ProjectRoot: r.projectRoot,
			TempRoot:    r.tempRoot,
		}, err
	}
	summary := agent.ExecutionCleanupSummary{
		ProjectRoot:                 report.ProjectRoot,
		TempRoot:                    report.TempRoot,
		RemovedRegisteredWorktrees:  report.RemovedRegisteredWorktrees,
		RemovedUnregisteredTempDirs: report.RemovedUnregisteredTempDirs,
		RemovedWorkerDirs:           report.RemovedWorkerDirs,
		RemovedEvidenceDirs:         report.ArchivedExecutionDirs + report.DeletedExecutionDirs,
		Warnings:                    append([]agent.ExecutionCleanupWarning(nil), report.Warnings...),
	}
	if r.processCleanup != nil {
		now := time.Now().UTC()
		if r.now != nil {
			now = r.now()
		}
		if procErr := r.processCleanup(ctx, report.ProjectRoot, report.TempRoot, &summary, report.runStates, report.registered, now); procErr != nil {
			return summary, procErr
		}
	}
	return summary, nil
}

// compositeExecutionCleanupRunner runs lightweight startup housekeeping then
// the full ExecutionCleanupManager. Workers previously used only the
// lightweight path, which never scanned ~/.cache/ddx/fixture-bin or other
// process-lifetime scratch roots — leaving multi-GB dead fixture binaries
// unreclaimed forever while worktrees were reaped on the 10m cadence.
type compositeExecutionCleanupRunner struct {
	startup *startupHousekeepingRunner
	full    *agent.ExecutionCleanupManager
}

func newWorkLoopCleanupRunner(projectRoot string) *compositeExecutionCleanupRunner {
	startup := newStartupHousekeepingRunner(projectRoot)
	full := agent.NewExecutionCleanupManager(projectRoot, &agent.RealGitOps{})
	if startup != nil && startup.tempRoot != "" {
		full.TempRoot = startup.tempRoot
	}
	return &compositeExecutionCleanupRunner{startup: startup, full: full}
}

func (r *compositeExecutionCleanupRunner) Cleanup(ctx context.Context) (agent.ExecutionCleanupSummary, error) {
	if r == nil {
		return agent.ExecutionCleanupSummary{}, nil
	}
	var summary agent.ExecutionCleanupSummary
	var firstErr error
	if r.startup != nil {
		s, err := r.startup.Cleanup(ctx)
		summary = s
		firstErr = err
	}
	if r.full != nil {
		full, err := r.full.Cleanup(ctx)
		if firstErr == nil {
			firstErr = err
		}
		summary = mergeExecutionCleanupSummaries(summary, full)
	}
	return summary, firstErr
}

// mergeExecutionCleanupSummaries folds a full-manager pass into the lightweight
// startup scan counts. Overlapping worktree reclaim is idempotent (second pass
// finds empty slots), so additive counters remain a correct lower bound on
// total reclaimed work for this composite pass.
func mergeExecutionCleanupSummaries(base, extra agent.ExecutionCleanupSummary) agent.ExecutionCleanupSummary {
	out := base
	if out.ProjectRoot == "" {
		out.ProjectRoot = extra.ProjectRoot
	}
	if out.TempRoot == "" {
		out.TempRoot = extra.TempRoot
	}
	if extra.ScannedTempDirs > out.ScannedTempDirs {
		out.ScannedTempDirs = extra.ScannedTempDirs
	}
	if extra.ScannedEvidenceDirs > out.ScannedEvidenceDirs {
		out.ScannedEvidenceDirs = extra.ScannedEvidenceDirs
	}
	if extra.CompleteEvidenceDirs > out.CompleteEvidenceDirs {
		out.CompleteEvidenceDirs = extra.CompleteEvidenceDirs
	}
	if extra.ScannedScratchDirs > out.ScannedScratchDirs {
		out.ScannedScratchDirs = extra.ScannedScratchDirs
	}
	out.RemovedUnregisteredTempDirs += extra.RemovedUnregisteredTempDirs
	out.RemovedRegisteredWorktrees += extra.RemovedRegisteredWorktrees
	out.RemovedRunStateFiles += extra.RemovedRunStateFiles
	out.RemovedScratchDirs += extra.RemovedScratchDirs
	out.PreservedActiveScratchDirs += extra.PreservedActiveScratchDirs
	out.RemovedEvidenceDirs += extra.RemovedEvidenceDirs
	out.RemovedAgentLogs += extra.RemovedAgentLogs
	out.RemovedWorkerDirs += extra.RemovedWorkerDirs
	out.RemovedClaimLivenessTmpFiles += extra.RemovedClaimLivenessTmpFiles
	out.BytesReclaimed += extra.BytesReclaimed
	out.InodesReclaimed += extra.InodesReclaimed
	out.ScratchBytesReclaimed += extra.ScratchBytesReclaimed
	out.ScratchInodesReclaimed += extra.ScratchInodesReclaimed
	out.ClaimLivenessBytesReclaimed += extra.ClaimLivenessBytesReclaimed
	out.ClaimLivenessInodesReclaimed += extra.ClaimLivenessInodesReclaimed
	out.Warnings = append(out.Warnings, extra.Warnings...)
	out.Issues = append(out.Issues, extra.Issues...)
	out.Observations = append(out.Observations, extra.Observations...)
	out.ProcessFindings = append(out.ProcessFindings, extra.ProcessFindings...)
	return out
}

func (r *startupHousekeepingRunner) scan(ctx context.Context, apply bool) (startupHousekeepingReport, error) {
	if r == nil {
		return startupHousekeepingReport{}, nil
	}
	report := startupHousekeepingReport{
		ProjectRoot: r.projectRoot,
		TempRoot:    r.tempRoot,
	}
	if r.projectRoot == "" {
		return report, nil
	}
	now := time.Now().UTC()
	if r.now != nil {
		now = r.now()
	}

	runStates, err := agent.ReadRunStates(r.projectRoot)
	if err != nil {
		return report, err
	}
	report.runStates = append([]agent.RunState(nil), runStates...)
	if err := r.scanWorktrees(ctx, now, runStates, apply, &report); err != nil {
		return report, err
	}
	if err := r.scanWorkerDirs(ctx, now, apply, &report); err != nil {
		return report, err
	}
	if err := r.scanExecutionDirs(ctx, now, runStates, apply, &report); err != nil {
		return report, err
	}
	return report, nil
}

func (r *startupHousekeepingRunner) scanWorktrees(ctx context.Context, now time.Time, runStates []agent.RunState, apply bool, report *startupHousekeepingReport) error {
	if report.TempRoot == "" {
		return nil
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(report.TempRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	registered := map[string]struct{}{}
	if paths, listErr := (&agent.RealGitOps{}).WorktreeList(r.projectRoot); listErr == nil {
		for _, path := range paths {
			registered[filepath.Clean(path)] = struct{}{}
		}
	}
	report.registered = registered

	for _, entry := range entries {
		// Clones (.execute-bead-clone-*) are the production attempt worktrees;
		// only scanning the legacy .execute-bead-wt-* prefix left multi-GB
		// clone trees unreaped by startup housekeeping.
		if !entry.IsDir() || !agent.IsExecutionAttemptDirName(entry.Name()) {
			continue
		}
		report.TempWorktreeCount++
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		info, err := entry.Info()
		if err != nil {
			report.Warnings = append(report.Warnings, agent.ExecutionCleanupWarning{
				Path:    filepath.Join(report.TempRoot, entry.Name()),
				Class:   "worktree_stat",
				Message: err.Error(),
			})
			continue
		}

		path := filepath.Join(report.TempRoot, entry.Name())
		meta, metaErr := agent.ReadExecutionCleanupMetadata(path)
		missingMetadata := false
		if metaErr == nil {
			meta.WorktreePath = firstNonEmpty(meta.WorktreePath, path)
		} else {
			missingMetadata = true
			meta = agent.ExecutionCleanupMetadata{WorktreePath: path}
		}
		// Liveness is authoritative (same rules as ExecutionCleanupManager).
		if worktreeStillLive(meta, runStates, now) {
			continue
		}

		// Proven dead: a run-state matches this tree but is no longer live
		// (dead PID / expired heartbeat). Reclaim immediately regardless of
		// age — this is the multi-GB leftover case the 72h gate used to block.
		matchedRunState := false
		for i := range runStates {
			if runStateMatchesWorktree(runStates[i], meta) {
				matchedRunState = true
				break
			}
		}
		if !matchedRunState {
			// No run-state match yet. Keep a short setup grace so:
			//  1) local-clone setup (cleanup.json before WriteRunState) is not
			//     reaped mid-flight (that race produced worktree_lost), and
			//  2) metadata-less attempt dirs (test leftovers / abandoned trees)
			//     are reaped on the same clock as ExecutionCleanupManager —
			//     not the multi-day DDX_WORKTREE_REAP_MAX_AGE hangover that
			//     left multi-GB orphans unreclaimed for 72h.
			// Abandoned foreign/test project trees skip grace (same rule as
			// ExecutionCleanupManager): the owning project is gone, so there
			// is no mid-setup race, and package-gate tests that share the
			// host exec-wt cache would otherwise pin leftovers for 30m each run.
			if !agent.AbandonedForeignProjectRoot(meta.ProjectRoot, r.projectRoot) {
				grace := defaultWorktreeReapGraceAge
				if grace <= 0 || (r.worktreeMaxAge > 0 && r.worktreeMaxAge < grace) {
					grace = r.worktreeMaxAge
				}
				if now.Sub(info.ModTime()) < grace {
					continue
				}
			}
		}

		// Registered worktrees without cleanup metadata are preserved — same
		// rule as ExecutionCleanupManager — so we never force-remove a linked
		// worktree we cannot attribute.
		if missingMetadata {
			if _, ok := registered[filepath.Clean(path)]; ok {
				continue
			}
		}

		report.StaleWorktrees++
		if !apply {
			continue
		}

		if _, ok := registered[filepath.Clean(path)]; ok {
			if err := (&agent.RealGitOps{}).WorktreeRemove(r.projectRoot, path); err != nil {
				report.Warnings = append(report.Warnings, agent.ExecutionCleanupWarning{
					Path:    path,
					Class:   "registered_worktree_remove",
					Message: err.Error(),
				})
				continue
			}
			report.RemovedRegisteredWorktrees++
			continue
		}

		if err := os.RemoveAll(path); err != nil {
			report.Warnings = append(report.Warnings, agent.ExecutionCleanupWarning{
				Path:    path,
				Class:   "unregistered_worktree_remove",
				Message: err.Error(),
			})
			continue
		}
		report.RemovedUnregisteredTempDirs++
	}

	return nil
}

func (r *startupHousekeepingRunner) scanWorkerDirs(ctx context.Context, now time.Time, apply bool, report *startupHousekeepingReport) error {
	root := workerstatus.LivenessDir(r.projectRoot)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "agent-loop-") {
			continue
		}
		report.WorkerSubprocessCount++
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}

		rec, err := workerstatus.ReadLiveness(r.projectRoot, entry.Name())
		if err != nil {
			report.Warnings = append(report.Warnings, agent.ExecutionCleanupWarning{
				Path:    filepath.Join(root, entry.Name(), "status.json"),
				Class:   "worker_dir_read",
				Message: err.Error(),
			})
			continue
		}
		stale, _ := staleWorkerDir(rec, now, r.workerDirMaxAge)
		if !stale {
			continue
		}

		report.StaleWorkerDirs++
		if !apply {
			continue
		}

		dirPath := filepath.Join(root, entry.Name())
		if err := os.RemoveAll(dirPath); err != nil {
			report.Warnings = append(report.Warnings, agent.ExecutionCleanupWarning{
				Path:    dirPath,
				Class:   "worker_dir_remove",
				Message: err.Error(),
			})
			continue
		}
		report.RemovedWorkerDirs++
	}

	return nil
}

func (r *startupHousekeepingRunner) scanExecutionDirs(ctx context.Context, now time.Time, runStates []agent.RunState, apply bool, report *startupHousekeepingReport) error {
	policy := r.retentionPolicy
	if policy.mode == executionRetentionDisabled {
		return nil
	}

	root := ddxroot.JoinProject(r.projectRoot, "executions")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	activeAttempts := map[string]struct{}{}
	for _, rs := range runStates {
		if rs.AttemptID != "" {
			activeAttempts[rs.AttemptID] = struct{}{}
		}
	}
	cutoff := now.AddDate(0, 0, -policy.days)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ok := activeAttempts[entry.Name()]; ok {
			continue
		}
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}

		dirPath := filepath.Join(root, entry.Name())
		if !hasExecutionEvidenceMarker(dirPath) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			report.Warnings = append(report.Warnings, agent.ExecutionCleanupWarning{
				Path:    dirPath,
				Class:   "execution_dir_stat",
				Message: err.Error(),
			})
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}

		report.StaleExecutionDirs++
		if !apply {
			continue
		}

		switch policy.mode {
		case executionRetentionArchive:
			if err := archiveExecutionDir(r.projectRoot, dirPath, entry.Name(), info.ModTime()); err != nil {
				report.Warnings = append(report.Warnings, agent.ExecutionCleanupWarning{
					Path:    dirPath,
					Class:   "execution_dir_archive",
					Message: err.Error(),
				})
				continue
			}
			report.ArchivedExecutionDirs++
		case executionRetentionDelete:
			if err := os.RemoveAll(dirPath); err != nil {
				report.Warnings = append(report.Warnings, agent.ExecutionCleanupWarning{
					Path:    dirPath,
					Class:   "execution_dir_remove",
					Message: err.Error(),
				})
				continue
			}
			report.DeletedExecutionDirs++
		}
	}

	return nil
}

func staleWorkerDir(rec workerstatus.LivenessRecord, now time.Time, maxAge time.Duration) (bool, string) {
	if rec.PID > 0 && !processAlive(rec.PID) {
		return true, fmt.Sprintf("worker pid %d is no longer live", rec.PID)
	}
	if !rec.LastActivityAt.IsZero() && now.Sub(rec.LastActivityAt) > maxAge {
		return true, fmt.Sprintf("last activity older than %s", maxAge)
	}
	return false, ""
}

func resolveExecutionRetentionPolicy(projectRoot string) executionRetentionPolicy {
	if raw := strings.TrimSpace(os.Getenv(executionRetentionOverrideEnv)); raw != "" {
		if days, err := strconv.Atoi(raw); err == nil && days >= 0 {
			if days == 0 {
				return executionRetentionPolicy{days: 0, mode: executionRetentionDelete}
			}
			return executionRetentionPolicy{days: days, mode: executionRetentionArchive}
		}
	}

	cfgPath := ddxroot.JoinProject(projectRoot, "config.yaml")
	cfg, err := config.LoadFromFile(cfgPath)
	if err != nil || cfg == nil || cfg.Executions == nil {
		return executionRetentionPolicy{days: 90, mode: executionRetentionArchive}
	}
	days := cfg.Executions.ResolveRetainDays()
	if days == 0 {
		return executionRetentionPolicy{days: 0, mode: executionRetentionDisabled}
	}
	return executionRetentionPolicy{days: days, mode: executionRetentionArchive}
}

func hasExecutionEvidenceMarker(dirPath string) bool {
	for _, name := range []string{"result.json", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(dirPath, name)); err == nil {
			return true
		}
	}
	return false
}

func archiveExecutionDir(projectRoot, dirPath, attemptID string, modTime time.Time) error {
	year, month := executionArchiveBucket(attemptID, modTime.UTC())
	dstRoot := filepath.Join(projectRoot, filepath.FromSlash(executionRetentionArchiveRoot), year, month)
	if err := os.MkdirAll(dstRoot, 0o755); err != nil {
		return fmt.Errorf("mkdir archive dir: %w", err)
	}
	dstPath := filepath.Join(dstRoot, attemptID)
	if _, err := os.Stat(dstPath); err == nil {
		return fmt.Errorf("archive destination already exists: %s", dstPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(dirPath, dstPath)
}

func executionArchiveBucket(attemptID string, modTime time.Time) (string, string) {
	ts := attemptID
	if idx := strings.IndexByte(ts, '-'); idx >= 0 {
		ts = ts[:idx]
	}
	if parsed, err := time.ParseInLocation(executionRetentionTimestampLayout, ts, time.UTC); err == nil {
		return parsed.Format("2006"), parsed.Format("01")
	}
	if modTime.IsZero() {
		modTime = time.Now().UTC()
	}
	return modTime.Format("2006"), modTime.Format("01")
}
