package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// ExecutionCleanupMetadataFileName is the filename used to describe ownership
// and liveness for a DDx temp execution directory.
const ExecutionCleanupMetadataFileName = "cleanup.json"

const defaultExecutionCleanupScratchMinAge = 24 * time.Hour
const defaultEvidenceRetainDays = 90

var defaultExecutionCleanupScratchPrefixes = []string{
	"ddx-test-",
	"ddx-e2e-",
	"ddx-test-bin-",
	"ddx-test-binary-",
	"ddx-exec-keepalive",
	"ddx-metric-keepalive",
	"ddx-metaprompt-keepalive",
	"ddx-persona-keepalive",
	"ddx-gate-wt-",
	"ddx-land-finalize-",
	"ddx-land-wt-",
	"ddx-push-recover-",
	"ddx-conflict-recover-",
}

// ExecutionCleanupLiveness records the refreshable liveness signal attached to
// a temp execution directory.
type ExecutionCleanupLiveness struct {
	PID          int       `json:"pid,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	RefreshedAt  time.Time `json:"refreshed_at,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	LastChecked  time.Time `json:"last_checked,omitempty"`
	HeartbeatSeq int       `json:"heartbeat_seq,omitempty"`
}

// ExecutionCleanupMetadata records the ownership metadata for one DDx temp
// execution directory.
type ExecutionCleanupMetadata struct {
	ProjectRoot  string `json:"project_root,omitempty"`
	BeadID       string `json:"bead_id,omitempty"`
	AttemptID    string `json:"attempt_id,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`
	Registered   bool   `json:"registered,omitempty"`
	Preserved    bool   `json:"preserved,omitempty"`
	// ActiveCandidateCycle is set by AttemptCycleCoordinator while it holds the
	// worktree open for candidate evaluation (checks, review, repair). The
	// cleanup manager preserves any worktree with this flag set so it is never
	// deleted mid-cycle before the coordinator reaches terminal disposition.
	ActiveCandidateCycle bool `json:"active_candidate_cycle,omitempty"`
	// Candidate-cycle metadata lets crash recovery distinguish a stale
	// unpinned worktree from one that still has a project-root candidate ref
	// worth preserving for operator inspection.
	CandidateCyclePhase string                    `json:"candidate_cycle_phase,omitempty"`
	CandidateRef        string                    `json:"candidate_ref,omitempty"`
	CandidateRev        string                    `json:"candidate_rev,omitempty"`
	CycleIndex          int                       `json:"cycle_index,omitempty"`
	ReviewActive        bool                      `json:"review_active,omitempty"`
	RepairActive        bool                      `json:"repair_active,omitempty"`
	CreatedAt           time.Time                 `json:"created_at,omitempty"`
	Liveness            *ExecutionCleanupLiveness `json:"liveness,omitempty"`
}

// ExecutionCleanupObservation is a structured note emitted by the cleanup
// manager when it preserves or removes a resource.
type ExecutionCleanupObservation struct {
	Path    string `json:"path"`
	Class   string `json:"class"`
	Message string `json:"message"`
	Bytes   int64  `json:"bytes,omitempty"`
	Inodes  int64  `json:"inodes,omitempty"`
}

// ExecutionCleanupWarning records a non-blocking issue encountered while the
// manager was scanning or reclaiming execution resources.
type ExecutionCleanupWarning struct {
	Path    string `json:"path,omitempty"`
	Class   string `json:"class"`
	Message string `json:"message"`
}

// ExecutionCleanupIssue records a blocking failure for a path the cleanup
// manager tried to reclaim.
type ExecutionCleanupIssue struct {
	Path     string `json:"path"`
	Class    string `json:"class"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}

// ExecutionCleanupSummary is the structured result of one cleanup pass.
type ExecutionCleanupSummary struct {
	ProjectRoot string `json:"project_root"`
	TempRoot    string `json:"temp_root"`

	ScannedTempDirs      int `json:"scanned_temp_dirs"`
	ScannedEvidenceDirs  int `json:"scanned_evidence_dirs"`
	CompleteEvidenceDirs int `json:"complete_evidence_dirs"`
	ScannedScratchDirs   int `json:"scanned_scratch_dirs"`

	RemovedUnregisteredTempDirs int64 `json:"removed_unregistered_temp_dirs"`
	RemovedRegisteredWorktrees  int64 `json:"removed_registered_worktrees"`
	RemovedRunStateFiles        int64 `json:"removed_run_state_files"`
	RemovedScratchDirs          int64 `json:"removed_scratch_dirs"`
	PreservedActiveScratchDirs  int64 `json:"preserved_active_scratch_dirs"`
	PrunedCandidateRefs         int64 `json:"pruned_candidate_refs"`
	RemovedEvidenceDirs         int64 `json:"removed_evidence_dirs"`
	RemovedAgentLogs            int64 `json:"removed_agent_logs"`
	RemovedWorkerDirs           int64 `json:"removed_worker_dirs"`

	BytesReclaimed         int64 `json:"bytes_reclaimed"`
	InodesReclaimed        int64 `json:"inodes_reclaimed"`
	ScratchBytesReclaimed  int64 `json:"scratch_bytes_reclaimed"`
	ScratchInodesReclaimed int64 `json:"scratch_inodes_reclaimed"`

	Warnings     []ExecutionCleanupWarning     `json:"warnings,omitempty"`
	Issues       []ExecutionCleanupIssue       `json:"issues,omitempty"`
	Observations []ExecutionCleanupObservation `json:"observations,omitempty"`
}

// ExecutionCleanupLivenessProbe decides whether a cleanup candidate should be
// preserved because it is still live.
type ExecutionCleanupLivenessProbe interface {
	IsLive(meta ExecutionCleanupMetadata, runState *RunState, now time.Time) (bool, string)
}

// defaultExecutionCleanupLivenessProbe preserves preserved metadata, a temp
// execution directory referenced by the live run-state file, or a candidate
// with an unexpired liveness marker. Tests can override the probe to model
// more specific host behavior.
type defaultExecutionCleanupLivenessProbe struct{}

func (defaultExecutionCleanupLivenessProbe) IsLive(meta ExecutionCleanupMetadata, runState *RunState, now time.Time) (bool, string) {
	if meta.Preserved {
		return true, "preserved metadata"
	}
	if meta.ActiveCandidateCycle {
		return true, candidateCycleRecoveryReason(meta, "active candidate cycle")
	}
	if strings.TrimSpace(meta.CandidateRef) != "" {
		return true, candidateCycleRecoveryReason(meta, "candidate ref recovery")
	}
	if runState != nil {
		if strings.TrimSpace(runState.CandidateCyclePhase) != "" {
			return true, candidateCycleRecoveryReason(candidateCycleMetadataFromRunState(*runState), "active candidate cycle")
		}
		if meta.WorktreePath != "" && filepath.Clean(runState.WorktreePath) == filepath.Clean(meta.WorktreePath) {
			return true, "matched live run-state"
		}
		if meta.AttemptID != "" && runState.AttemptID == meta.AttemptID {
			return true, "matched live run-state"
		}
	}
	if meta.Liveness != nil {
		if meta.Liveness.PID > 0 && trackerProcessAlive(meta.Liveness.PID) {
			return true, "live pid"
		}
		if !meta.Liveness.ExpiresAt.IsZero() && now.Before(meta.Liveness.ExpiresAt) {
			return true, "unexpired liveness"
		}
		if !meta.Liveness.RefreshedAt.IsZero() && now.Sub(meta.Liveness.RefreshedAt) <= 2*time.Minute {
			return true, "fresh liveness"
		}
	}
	return false, "stale liveness"
}

// ExecutionCleanupManager owns conservative reclamation of DDx temp execution
// resources for one project.
type ExecutionCleanupManager struct {
	ProjectRoot     string
	TempRoot        string
	ScratchRoots    []string
	ScratchPrefixes []string
	ScratchMinAge   time.Duration
	GitOps          GitOps
	DryRun          bool
	Now             func() time.Time
	Probe           ExecutionCleanupLivenessProbe
	// RetainDays controls how many days of evidence dirs under
	// .ddx/executions/ to retain. 0 disables the prune; default is 90.
	RetainDays int
}

// NewExecutionCleanupManager constructs a cleanup manager with the default
// DDx temp worktree root and the default liveness probe.
func NewExecutionCleanupManager(projectRoot string, gitOps GitOps) *ExecutionCleanupManager {
	return &ExecutionCleanupManager{
		ProjectRoot: projectRoot,
		TempRoot:    executionCleanupTempRoot(projectRoot),
		RetainDays:  executionCleanupRetainDays(projectRoot),
		GitOps:      gitOps,
		Now:         time.Now,
		Probe:       defaultExecutionCleanupLivenessProbe{},
	}
}

// executionCleanupRetainDays returns the configured retain_days for evidence
// dirs, defaulting to defaultEvidenceRetainDays when not set in config.
func executionCleanupRetainDays(projectRoot string) int {
	if projectRoot != "" {
		projectConfig := filepath.Join(projectRoot, ".ddx", "config.yaml")
		cfg, err := config.LoadFromFile(projectConfig)
		if err == nil && cfg != nil && cfg.Executions != nil {
			return cfg.Executions.ResolveRetainDays()
		}
	}
	return defaultEvidenceRetainDays
}

// Cleanup scans the configured DDx temp worktree root, consults the project
// run-state file and git worktree registry, and conservatively removes stale
// DDx-owned temp resources.
func (m *ExecutionCleanupManager) Cleanup(ctx context.Context) (ExecutionCleanupSummary, error) {
	summary := ExecutionCleanupSummary{
		ProjectRoot: m.ProjectRoot,
		TempRoot:    m.tempRoot(),
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
	}
	if summary.TempRoot == "" {
		summary.TempRoot = executionCleanupTempRoot(m.ProjectRoot)
	}
	if summary.TempRoot == "" {
		return summary, errors.New("execution cleanup: temp root is empty")
	}
	if m.ProjectRoot == "" {
		return summary, errors.New("execution cleanup: project root is empty")
	}
	now := time.Now
	if m.Now != nil {
		now = m.Now
	}
	probe := m.Probe
	if probe == nil {
		probe = defaultExecutionCleanupLivenessProbe{}
	}

	registered := map[string]struct{}{}
	liveRunStates := map[string]struct{}{}
	if m.GitOps != nil {
		paths, err := m.GitOps.WorktreeList(m.ProjectRoot)
		if err != nil {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    m.ProjectRoot,
				Class:   "git_worktree_list",
				Message: err.Error(),
			})
		} else {
			for _, p := range paths {
				registered[filepath.Clean(p)] = struct{}{}
			}
		}
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
	}

	runStates, runStateErr := ReadRunStates(m.ProjectRoot)
	if runStateErr != nil {
		summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
			Path:    runStateDirPath(m.ProjectRoot),
			Class:   "run_state_read",
			Message: runStateErr.Error(),
		})
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
	}

	entries, err := os.ReadDir(summary.TempRoot)
	if err != nil {
		return summary, fmt.Errorf("execution cleanup: read temp root: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return summary, err
			}
		}
		if !strings.HasPrefix(entry.Name(), ExecuteBeadWtPrefix) {
			continue
		}
		summary.ScannedTempDirs++
		path := filepath.Join(summary.TempRoot, entry.Name())

		meta, metaErr := ReadExecutionCleanupMetadata(path)
		missingMetadata := false
		if metaErr != nil {
			if errors.Is(metaErr, os.ErrNotExist) {
				missingMetadata = true
				meta = ExecutionCleanupMetadata{
					ProjectRoot:  m.ProjectRoot,
					WorktreePath: path,
				}
			} else {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    path,
					Class:   "metadata_read",
					Message: metaErr.Error(),
				})
				continue
			}
		}
		candidateRunStates := runStates
		ownershipMismatch := meta.ProjectRoot != "" && !sameCleanPath(meta.ProjectRoot, m.ProjectRoot)
		if ownershipMismatch {
			candidateRunStates = m.runStatesForMetadata(meta, runStates, &summary)
		}

		matchedRunState := matchingRunStateForMeta(candidateRunStates, meta)
		live, reason := probe.IsLive(meta, matchedRunState, now())
		if live {
			if matchedRunState != nil {
				liveRunStates[runStateLiveKey(*matchedRunState)] = struct{}{}
			}
			class := "preserved_temp_dir"
			if isLiveAttemptPreservation(reason) {
				class = "preserved_live_attempt"
			}
			summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
				Path:    path,
				Class:   class,
				Message: reason,
			})
			continue
		}
		if ownershipMismatch {
			if !m.canReclaimForeignTestOwnedPath(meta.ProjectRoot, path) {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    path,
					Class:   "ownership_mismatch",
					Message: fmt.Sprintf("metadata project_root=%s does not match manager project_root=%s", meta.ProjectRoot, m.ProjectRoot),
				})
				continue
			}
			if m.isRegisteredWorktree(meta.ProjectRoot, path, &summary) {
				summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
					Path:    path,
					Class:   "preserved_foreign_registered_worktree",
					Message: "foreign project worktree is still registered",
				})
				continue
			}
		}

		if _, ok := registered[filepath.Clean(path)]; ok {
			if missingMetadata {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    path,
					Class:   "registered_missing_metadata",
					Message: "registered DDx worktree has no cleanup metadata; preserving",
				})
				summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
					Path:    path,
					Class:   "preserved_registered_missing_metadata",
					Message: "registered worktree without cleanup metadata",
				})
				continue
			}
			if m.GitOps == nil {
				summary.Issues = append(summary.Issues, ExecutionCleanupIssue{
					Path:     path,
					Class:    "registered_worktree",
					Message:  "no git ops available to remove registered worktree",
					Blocking: true,
				})
				continue
			}
			reclaimedBytes, reclaimedInodes, measureErr := measureTreeWithContext(ctx, path)
			if measureErr != nil {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    path,
					Class:   "measure_tree",
					Message: measureErr.Error(),
				})
			}
			if !m.DryRun {
				if err := m.GitOps.WorktreeRemove(m.ProjectRoot, path); err != nil {
					summary.Issues = append(summary.Issues, ExecutionCleanupIssue{
						Path:     path,
						Class:    "registered_worktree_remove",
						Message:  err.Error(),
						Blocking: true,
					})
					continue
				}
			}
			summary.RemovedRegisteredWorktrees++
			summary.BytesReclaimed += reclaimedBytes
			summary.InodesReclaimed += reclaimedInodes
			class := "removed_registered_worktree"
			if m.DryRun {
				class = "would_remove_registered_worktree"
			}
			summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
				Path:    path,
				Class:   class,
				Message: reason,
				Bytes:   reclaimedBytes,
				Inodes:  reclaimedInodes,
			})
			continue
		}

		reclaimedBytes, reclaimedInodes, measureErr := measureTreeWithContext(ctx, path)
		if measureErr != nil {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    path,
				Class:   "measure_tree",
				Message: measureErr.Error(),
			})
		}
		if !m.DryRun {
			if err := os.RemoveAll(path); err != nil {
				summary.Issues = append(summary.Issues, ExecutionCleanupIssue{
					Path:     path,
					Class:    "unregistered_temp_dir_remove",
					Message:  err.Error(),
					Blocking: false,
				})
				continue
			}
		}
		summary.RemovedUnregisteredTempDirs++
		summary.BytesReclaimed += reclaimedBytes
		summary.InodesReclaimed += reclaimedInodes
		class := "removed_unregistered_temp_dir"
		if m.DryRun {
			class = "would_remove_unregistered_temp_dir"
		}
		summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
			Path:    path,
			Class:   class,
			Message: reason,
			Bytes:   reclaimedBytes,
			Inodes:  reclaimedInodes,
		})
	}

	if err := m.cleanupScratchRoots(ctx, &summary, runStates, registered, probe, now()); err != nil {
		return summary, err
	}

	if m.GitOps != nil && summary.RemovedRegisteredWorktrees > 0 && !m.DryRun {
		if err := m.GitOps.WorktreePrune(m.ProjectRoot); err != nil {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    m.ProjectRoot,
				Class:   "worktree_prune",
				Message: err.Error(),
			})
		}
	}

	// Stale run-state files are removed when they point at an execution
	// resource that is no longer live.
	for _, state := range runStates {
		if _, ok := liveRunStates[runStateLiveKey(state)]; ok {
			continue
		}
		if runStateWorktreeStillExists(state) {
			continue
		}
		if removed, bytes := m.removeStaleRunState(state, &summary); removed {
			summary.RemovedRunStateFiles++
			summary.BytesReclaimed += bytes
			summary.InodesReclaimed++
		}
	}

	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
	}
	summary.ScannedEvidenceDirs += scanCompleteEvidenceDirs(m.ProjectRoot, ExecuteBeadArtifactDir, "manifest.json", "result.json", &summary)
	m.pruneEvidenceDirs(ctx, &summary, runStates, now())
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
	}
	m.pruneAgentLogs(ctx, &summary, runStates, now())
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
	}
	m.pruneWorkerDirs(ctx, &summary, now())
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
	}
	summary.ScannedEvidenceDirs += scanCompleteEvidenceDirs(m.ProjectRoot, ".ddx/runs", "record.json", "", &summary)
	m.cleanupDurableLandedCandidateRefs(&summary)

	return summary, nil
}

func (m *ExecutionCleanupManager) cleanupScratchRoots(ctx context.Context, summary *ExecutionCleanupSummary, runStates []RunState, registered map[string]struct{}, probe ExecutionCleanupLivenessProbe, now time.Time) error {
	prefixes := m.scratchPrefixes()
	minAge := m.scratchMinAge()
	tempRoot := filepath.Clean(summary.TempRoot)

	for _, root := range m.scratchRoots(summary.TempRoot) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			if !os.IsNotExist(err) {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    root,
					Class:   "scratch_root_read",
					Message: err.Error(),
				})
			}
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || !hasAnyPrefix(entry.Name(), prefixes) {
				continue
			}
			if ctx != nil {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			path := filepath.Join(root, entry.Name())
			if sameCleanPath(path, tempRoot) || isPathWithin(path, tempRoot) {
				continue
			}
			summary.ScannedScratchDirs++

			info, err := entry.Info()
			if err != nil {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    path,
					Class:   "scratch_stat",
					Message: err.Error(),
				})
				continue
			}

			meta, metaErr := ReadExecutionCleanupMetadata(path)
			if metaErr != nil {
				if !errors.Is(metaErr, os.ErrNotExist) {
					summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
						Path:    path,
						Class:   "scratch_metadata_read",
						Message: metaErr.Error(),
					})
					continue
				}
				meta = ExecutionCleanupMetadata{
					ProjectRoot:  m.ProjectRoot,
					WorktreePath: path,
				}
			}
			if meta.WorktreePath == "" {
				meta.WorktreePath = path
			}

			candidateRunStates := runStates
			if meta.ProjectRoot != "" && !sameCleanPath(meta.ProjectRoot, m.ProjectRoot) {
				if !m.canReclaimForeignTestOwnedPath(meta.ProjectRoot, path) {
					summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
						Path:    path,
						Class:   "scratch_ownership_mismatch",
						Message: fmt.Sprintf("metadata project_root=%s does not match manager project_root=%s", meta.ProjectRoot, m.ProjectRoot),
					})
					continue
				}
				candidateRunStates = m.runStatesForMetadata(meta, runStates, summary)
				if m.isRegisteredWorktree(meta.ProjectRoot, path, summary) {
					summary.PreservedActiveScratchDirs++
					summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
						Path:    path,
						Class:   "preserved_active_scratch_dir",
						Message: "foreign project worktree is still registered",
					})
					continue
				}
			}
			if _, ok := registered[filepath.Clean(path)]; ok {
				summary.PreservedActiveScratchDirs++
				summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
					Path:    path,
					Class:   "preserved_active_scratch_dir",
					Message: "registered worktree",
				})
				continue
			}

			matchedRunState := matchingRunStateForMeta(candidateRunStates, meta)
			if live, reason := probe.IsLive(meta, matchedRunState, now); live {
				summary.PreservedActiveScratchDirs++
				summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
					Path:    path,
					Class:   "preserved_active_scratch_dir",
					Message: reason,
				})
				continue
			}

			if age := now.Sub(info.ModTime()); age < minAge {
				summary.PreservedActiveScratchDirs++
				summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
					Path:    path,
					Class:   "preserved_active_scratch_dir",
					Message: fmt.Sprintf("fresh scratch dir age=%s min_age=%s", age.Round(time.Second), minAge),
				})
				continue
			}

			reclaimedBytes, reclaimedInodes, measureErr := measureTreeWithContext(ctx, path)
			if measureErr != nil {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    path,
					Class:   "scratch_measure_tree",
					Message: measureErr.Error(),
				})
			}
			if !m.DryRun {
				if err := os.RemoveAll(path); err != nil {
					summary.Issues = append(summary.Issues, ExecutionCleanupIssue{
						Path:     path,
						Class:    "scratch_dir_remove",
						Message:  err.Error(),
						Blocking: false,
					})
					continue
				}
			}
			summary.RemovedScratchDirs++
			summary.ScratchBytesReclaimed += reclaimedBytes
			summary.ScratchInodesReclaimed += reclaimedInodes
			summary.BytesReclaimed += reclaimedBytes
			summary.InodesReclaimed += reclaimedInodes
			class := "removed_scratch_dir"
			if m.DryRun {
				class = "would_remove_scratch_dir"
			}
			summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
				Path:    path,
				Class:   class,
				Message: "stale DDx scratch dir",
				Bytes:   reclaimedBytes,
				Inodes:  reclaimedInodes,
			})
		}
	}
	return nil
}

func (m *ExecutionCleanupManager) runStatesForMetadata(meta ExecutionCleanupMetadata, fallback []RunState, summary *ExecutionCleanupSummary) []RunState {
	if meta.ProjectRoot == "" || sameCleanPath(meta.ProjectRoot, m.ProjectRoot) {
		return fallback
	}
	states, err := ReadRunStates(meta.ProjectRoot)
	if err != nil {
		summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
			Path:    runStateDirPath(meta.ProjectRoot),
			Class:   "foreign_run_state_read",
			Message: err.Error(),
		})
		return fallback
	}
	return states
}

func (m *ExecutionCleanupManager) canReclaimForeignTestOwnedPath(projectRoot, path string) bool {
	return projectRoot != "" && isPathWithin(projectRoot, os.TempDir()) && isPathWithin(path, os.TempDir())
}

func (m *ExecutionCleanupManager) isRegisteredWorktree(projectRoot, path string, summary *ExecutionCleanupSummary) bool {
	if m.GitOps == nil || projectRoot == "" || path == "" {
		return false
	}
	paths, err := m.GitOps.WorktreeList(projectRoot)
	if err != nil {
		summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
			Path:    projectRoot,
			Class:   "foreign_git_worktree_list",
			Message: err.Error(),
		})
		return false
	}
	clean := filepath.Clean(path)
	for _, p := range paths {
		if filepath.Clean(p) == clean {
			return true
		}
	}
	return false
}

func (m *ExecutionCleanupManager) scratchRoots(tempRoot string) []string {
	if len(m.ScratchRoots) > 0 {
		return cleanUniquePaths(m.ScratchRoots)
	}
	var roots []string
	if tempRoot != "" {
		parent := filepath.Dir(filepath.Clean(tempRoot))
		if parent != "" && parent != "." {
			roots = append(roots, parent)
		}
	}
	if len(roots) == 0 {
		roots = append(roots, os.TempDir())
	}
	return cleanUniquePaths(roots)
}

func (m *ExecutionCleanupManager) scratchPrefixes() []string {
	if len(m.ScratchPrefixes) > 0 {
		return append([]string(nil), m.ScratchPrefixes...)
	}
	return append([]string(nil), defaultExecutionCleanupScratchPrefixes...)
}

func (m *ExecutionCleanupManager) scratchMinAge() time.Duration {
	if m.ScratchMinAge > 0 {
		return m.ScratchMinAge
	}
	return defaultExecutionCleanupScratchMinAge
}

func cleanUniquePaths(paths []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		clean := filepath.Clean(p)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func hasAnyPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix != "" && strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func sameCleanPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func isPathWithin(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if cleanPath == cleanRoot {
		return true
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func matchingRunStateForMeta(states []RunState, meta ExecutionCleanupMetadata) *RunState {
	if meta.CandidateRef != "" {
		for i := range states {
			state := states[i]
			if state.CandidateRef != meta.CandidateRef {
				continue
			}
			if meta.CycleIndex != 0 && state.CycleIndex != meta.CycleIndex {
				continue
			}
			return &states[i]
		}
	}
	for i := range states {
		state := states[i]
		if meta.AttemptID != "" && state.AttemptID == meta.AttemptID {
			return &states[i]
		}
		if meta.WorktreePath != "" && state.WorktreePath != "" && filepath.Clean(state.WorktreePath) == filepath.Clean(meta.WorktreePath) {
			return &states[i]
		}
	}
	return nil
}

func isLiveAttemptPreservation(reason string) bool {
	if strings.HasPrefix(reason, "active candidate cycle") || strings.HasPrefix(reason, "candidate ref recovery") {
		return true
	}
	switch reason {
	case "matched live run-state", "unexpired liveness", "fresh liveness", "live pid", "active candidate cycle":
		return true
	default:
		return false
	}
}

func candidateCycleRecoveryReason(meta ExecutionCleanupMetadata, prefix string) string {
	var parts []string
	if meta.CandidateCyclePhase != "" {
		parts = append(parts, "phase="+meta.CandidateCyclePhase)
	}
	if meta.CandidateRef != "" {
		parts = append(parts, "candidate_ref="+meta.CandidateRef)
	}
	if meta.CandidateRev != "" {
		parts = append(parts, "candidate_rev="+meta.CandidateRev)
	}
	if meta.ReviewActive {
		parts = append(parts, "review_active=true")
	}
	if meta.RepairActive {
		parts = append(parts, "repair_active=true")
	}
	if len(parts) == 0 {
		return prefix
	}
	return prefix + " (" + strings.Join(parts, " ") + ")"
}

func candidateCycleMetadataFromRunState(state RunState) ExecutionCleanupMetadata {
	return ExecutionCleanupMetadata{
		BeadID:               state.BeadID,
		AttemptID:            state.AttemptID,
		WorktreePath:         state.WorktreePath,
		ActiveCandidateCycle: strings.TrimSpace(state.CandidateCyclePhase) != "",
		CandidateCyclePhase:  state.CandidateCyclePhase,
		CandidateRef:         state.CandidateRef,
		CandidateRev:         state.CandidateRev,
		CycleIndex:           state.CycleIndex,
		ReviewActive:         state.ReviewActive,
		RepairActive:         state.RepairActive,
	}
}

func runStateLiveKey(state RunState) string {
	if state.AttemptID != "" {
		return "attempt:" + state.AttemptID
	}
	if state.WorktreePath != "" {
		return "path:" + filepath.Clean(state.WorktreePath)
	}
	return "bead:" + state.BeadID
}

func runStateWorktreeStillExists(state RunState) bool {
	if state.WorktreePath == "" {
		return false
	}
	_, err := os.Stat(state.WorktreePath)
	return err == nil
}

func (m *ExecutionCleanupManager) removeStaleRunState(state RunState, summary *ExecutionCleanupSummary) (bool, int64) {
	path := runStatePath(m.ProjectRoot)
	clear := func() error {
		return ClearRunState(m.ProjectRoot)
	}
	if state.AttemptID != "" {
		if attemptPath, err := runStateAttemptPath(m.ProjectRoot, state.AttemptID); err == nil {
			path = attemptPath
			clear = func() error {
				return ClearRunStateAttempt(m.ProjectRoot, state.AttemptID)
			}
		}
	}

	var bytes int64
	present := false
	if info, statErr := os.Stat(path); statErr == nil {
		present = true
		bytes = info.Size()
	} else if os.IsNotExist(statErr) && path != runStatePath(m.ProjectRoot) {
		path = runStatePath(m.ProjectRoot)
		if info, legacyErr := os.Stat(path); legacyErr == nil {
			present = true
			bytes = info.Size()
			clear = func() error {
				return ClearRunState(m.ProjectRoot)
			}
		}
	}
	if !m.DryRun {
		if err := clear(); err != nil {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    path,
				Class:   "run_state_clear",
				Message: err.Error(),
			})
		}
	}
	if !present {
		return false, 0
	}
	class := "removed_run_state"
	if m.DryRun {
		class = "would_remove_run_state"
	}
	summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
		Path:    path,
		Class:   class,
		Message: "stale run-state",
		Bytes:   bytes,
		Inodes:  1,
	})
	return true, bytes
}

func (m *ExecutionCleanupManager) tempRoot() string {
	if m != nil && m.TempRoot != "" {
		return m.TempRoot
	}
	if m != nil {
		return executionCleanupTempRoot(m.ProjectRoot)
	}
	return executionCleanupTempRoot("")
}

type candidateRefResultMetadata struct {
	Status       string `json:"status"`
	CandidateRef string `json:"candidate_ref"`
	CycleIndex   int    `json:"cycle_index"`
	AttemptID    string `json:"attempt_id"`
	BaseRev      string `json:"base_rev"`
	ResultRev    string `json:"result_rev"`
}

func (m *ExecutionCleanupManager) cleanupDurableLandedCandidateRefs(summary *ExecutionCleanupSummary) {
	if m == nil || m.GitOps == nil || m.ProjectRoot == "" {
		return
	}
	resultRoot := filepath.Join(m.ProjectRoot, filepath.FromSlash(ExecuteBeadArtifactDir))
	entries, err := os.ReadDir(resultRoot)
	if err != nil {
		return
	}
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		resultPath := filepath.Join(resultRoot, entry.Name(), "result.json")
		data, readErr := os.ReadFile(resultPath)
		if readErr != nil {
			continue
		}
		var meta candidateRefResultMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    resultPath,
				Class:   "candidate_ref_result_parse",
				Message: err.Error(),
			})
			continue
		}
		if !candidateRefResultDurable(meta) {
			continue
		}
		if _, ok := seen[meta.CandidateRef]; ok {
			continue
		}
		seen[meta.CandidateRef] = struct{}{}
		if _, err := m.GitOps.ResolveRev(m.ProjectRoot, meta.CandidateRef); err != nil {
			continue
		}
		if !m.DryRun {
			if err := m.GitOps.DeleteRef(m.ProjectRoot, meta.CandidateRef); err != nil {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    meta.CandidateRef,
					Class:   "candidate_ref_prune",
					Message: err.Error(),
				})
				continue
			}
		}
		summary.PrunedCandidateRefs++
		class := "pruned_candidate_ref"
		if m.DryRun {
			class = "would_prune_candidate_ref"
		}
		summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
			Path:    meta.CandidateRef,
			Class:   class,
			Message: "landed candidate metadata durable in " + filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, entry.Name(), "result.json")),
		})
	}
}

func candidateRefResultDurable(meta candidateRefResultMetadata) bool {
	return meta.Status == ExecuteBeadStatusSuccess &&
		strings.HasPrefix(strings.TrimSpace(meta.CandidateRef), "refs/ddx/iterations/") &&
		strings.TrimSpace(meta.AttemptID) != "" &&
		strings.TrimSpace(meta.BaseRev) != "" &&
		strings.TrimSpace(meta.ResultRev) != ""
}

func executionCleanupTempRoot(projectRoot string) string {
	base := config.ExecutionWorktreeRoot(projectRoot)
	if base == "" {
		base = filepath.Join(os.TempDir(), ExecuteBeadTmpSubdir)
	}
	return base
}

// WriteExecutionCleanupMetadata writes the ownership metadata used by the
// cleanup manager for tests and future call sites.
func WriteExecutionCleanupMetadata(dir string, meta ExecutionCleanupMetadata) error {
	if dir == "" {
		return errors.New("write execution cleanup metadata: dir is empty")
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("write execution cleanup metadata: marshal: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("write execution cleanup metadata: mkdir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, ExecutionCleanupMetadataFileName), data, 0o644)
}

// ReadExecutionCleanupMetadata reads the cleanup metadata file from dir.
func ReadExecutionCleanupMetadata(dir string) (ExecutionCleanupMetadata, error) {
	data, err := os.ReadFile(filepath.Join(dir, ExecutionCleanupMetadataFileName))
	if err != nil {
		return ExecutionCleanupMetadata{}, err
	}
	var meta ExecutionCleanupMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return ExecutionCleanupMetadata{}, fmt.Errorf("read execution cleanup metadata: parse: %w", err)
	}
	return meta, nil
}

func measureTree(path string) (bytes int64, inodes int64, err error) {
	return measureTreeWithContext(context.Background(), path)
}

func measureTreeWithContext(ctx context.Context, path string) (bytes int64, inodes int64, err error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return 0, 0, err
	}
	if !info.IsDir() {
		return info.Size(), 1, nil
	}
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if walkErr != nil {
			return walkErr
		}
		inodes++
		fi, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		bytes += fi.Size()
		return nil
	})
	return bytes, inodes, err
}

// pruneEvidenceDirs deletes .ddx/executions/<attempt-id>/ dirs whose mtime
// is older than m.RetainDays. Dirs referenced by active run-states or lacking
// a result.json are preserved. No-op when RetainDays == 0 (disabled).
func (m *ExecutionCleanupManager) pruneEvidenceDirs(ctx context.Context, summary *ExecutionCleanupSummary, runStates []RunState, now time.Time) {
	if m.RetainDays == 0 {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -m.RetainDays)

	activeAttempts := map[string]struct{}{}
	for _, rs := range runStates {
		if rs.AttemptID != "" {
			activeAttempts[rs.AttemptID] = struct{}{}
		}
	}

	root := filepath.Join(m.ProjectRoot, filepath.FromSlash(ExecuteBeadArtifactDir))
	entries, err := os.ReadDir(root)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    root,
				Class:   "evidence_dir_read",
				Message: err.Error(),
			})
		}
		return
	}

	for _, entry := range entries {
		if ctx != nil && ctx.Err() != nil {
			return
		}
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		dirPath := filepath.Join(root, dirName)
		relPath := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, dirName))

		if _, ok := activeAttempts[dirName]; ok {
			summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
				Path:    relPath,
				Class:   "preserved_active_evidence_dir",
				Message: "active run-state reference",
			})
			continue
		}

		// Defensive: skip dirs with no result.json (may be in-flight).
		if _, statErr := os.Stat(filepath.Join(dirPath, "result.json")); statErr != nil {
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    dirPath,
				Class:   "evidence_dir_stat",
				Message: infoErr.Error(),
			})
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}

		if !m.DryRun {
			m.removeEvidenceDir(ctx, dirPath, summary)
		}
		summary.RemovedEvidenceDirs++
		class := "removed_evidence_dir"
		if m.DryRun {
			class = "would_remove_evidence_dir"
		}
		summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
			Path:    relPath,
			Class:   class,
			Message: fmt.Sprintf("evidence dir older than %d days", m.RetainDays),
		})
	}
}

// removeEvidenceDir stages tracked files for deletion via git rm, then
// removes any remaining files with os.RemoveAll.
func (m *ExecutionCleanupManager) removeEvidenceDir(ctx context.Context, dirPath string, summary *ExecutionCleanupSummary) {
	if m.ProjectRoot == "" {
		_ = os.RemoveAll(dirPath)
		return
	}
	rel, err := filepath.Rel(m.ProjectRoot, dirPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		_ = os.RemoveAll(dirPath)
		return
	}
	relSlash := filepath.ToSlash(rel)

	// Check for tracked files; if any, stage their deletion via git rm.
	lsOut, lsErr := internalgit.Command(ctx, m.ProjectRoot, "ls-files", relSlash).Output()
	if lsErr == nil && len(strings.TrimSpace(string(lsOut))) > 0 {
		rmOut, rmErr := internalgit.Command(ctx, m.ProjectRoot, "rm", "-rf", relSlash).CombinedOutput()
		if rmErr != nil {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    dirPath,
				Class:   "evidence_dir_git_rm",
				Message: fmt.Sprintf("git rm: %s: %v", strings.TrimSpace(string(rmOut)), rmErr),
			})
		}
	}
	// Remove remaining untracked files.
	_ = os.RemoveAll(dirPath)
}

// pruneAgentLogs deletes .ddx/agent-logs/agent-*.jsonl and svc-*.jsonl files
// whose mtime is older than m.RetainDays. Files whose session_id matches an
// active run-state are skipped. No-op when RetainDays == 0.
func (m *ExecutionCleanupManager) pruneAgentLogs(ctx context.Context, summary *ExecutionCleanupSummary, runStates []RunState, now time.Time) {
	if m.RetainDays == 0 {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -m.RetainDays)

	activeSessions := map[string]struct{}{}
	for _, rs := range runStates {
		if rs.SessionID != "" {
			activeSessions[rs.SessionID] = struct{}{}
		}
	}

	root := filepath.Join(m.ProjectRoot, ".ddx", "agent-logs")
	entries, err := os.ReadDir(root)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    root,
				Class:   "agent_log_read",
				Message: err.Error(),
			})
		}
		return
	}

	for _, entry := range entries {
		if ctx != nil && ctx.Err() != nil {
			return
		}
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		var sessionID string
		switch {
		case strings.HasPrefix(name, "agent-") && strings.HasSuffix(name, ".jsonl"):
			sessionID = strings.TrimSuffix(strings.TrimPrefix(name, "agent-"), ".jsonl")
		case strings.HasPrefix(name, "svc-") && strings.HasSuffix(name, ".jsonl"):
			sessionID = strings.TrimSuffix(strings.TrimPrefix(name, "svc-"), ".jsonl")
		default:
			continue
		}
		if _, ok := activeSessions[sessionID]; ok {
			summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
				Path:    filepath.ToSlash(filepath.Join(".ddx/agent-logs", name)),
				Class:   "preserved_active_agent_log",
				Message: "active run-state session reference",
			})
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    filepath.Join(root, name),
				Class:   "agent_log_stat",
				Message: infoErr.Error(),
			})
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}
		path := filepath.Join(root, name)
		if !m.DryRun {
			if err := os.Remove(path); err != nil {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    path,
					Class:   "agent_log_remove",
					Message: err.Error(),
				})
				continue
			}
		}
		summary.RemovedAgentLogs++
		class := "removed_agent_log"
		if m.DryRun {
			class = "would_remove_agent_log"
		}
		summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
			Path:    filepath.ToSlash(filepath.Join(".ddx/agent-logs", name)),
			Class:   class,
			Message: fmt.Sprintf("agent log older than %d days", m.RetainDays),
		})
	}
}

// workerDirPID reads the PID from .ddx/workers/<id>/status.json. Returns 0
// if the file cannot be read or has no PID.
func workerDirPID(dirPath string) int {
	data, err := os.ReadFile(filepath.Join(dirPath, "status.json"))
	if err != nil {
		return 0
	}
	var rec struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return 0
	}
	return rec.PID
}

// pruneWorkerDirs deletes .ddx/workers/<id>/ directories whose mtime is older
// than m.RetainDays. Directories whose status.json PID is still alive are
// skipped. No-op when RetainDays == 0.
func (m *ExecutionCleanupManager) pruneWorkerDirs(ctx context.Context, summary *ExecutionCleanupSummary, now time.Time) {
	if m.RetainDays == 0 {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -m.RetainDays)

	root := filepath.Join(m.ProjectRoot, ".ddx", "workers")
	entries, err := os.ReadDir(root)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    root,
				Class:   "worker_dir_read",
				Message: err.Error(),
			})
		}
		return
	}

	for _, entry := range entries {
		if ctx != nil && ctx.Err() != nil {
			return
		}
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(root, entry.Name())
		relPath := filepath.ToSlash(filepath.Join(".ddx/workers", entry.Name()))

		if pid := workerDirPID(dirPath); pid > 0 && trackerProcessAlive(pid) {
			summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
				Path:    relPath,
				Class:   "preserved_active_worker_dir",
				Message: fmt.Sprintf("worker pid %d is alive", pid),
			})
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    dirPath,
				Class:   "worker_dir_stat",
				Message: infoErr.Error(),
			})
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}

		if !m.DryRun {
			if err := os.RemoveAll(dirPath); err != nil {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    dirPath,
					Class:   "worker_dir_remove",
					Message: err.Error(),
				})
				continue
			}
		}
		summary.RemovedWorkerDirs++
		class := "removed_worker_dir"
		if m.DryRun {
			class = "would_remove_worker_dir"
		}
		summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
			Path:    relPath,
			Class:   class,
			Message: fmt.Sprintf("worker dir older than %d days", m.RetainDays),
		})
	}
}

func scanCompleteEvidenceDirs(projectRoot, rootRel string, primaryFile, secondaryFile string, summary *ExecutionCleanupSummary) int {
	root := filepath.Join(projectRoot, filepath.FromSlash(rootRel))
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		count++
		full := filepath.Join(root, entry.Name())
		complete := false
		if primaryFile != "" {
			if _, err := os.Stat(filepath.Join(full, primaryFile)); err == nil {
				complete = true
			}
		}
		if !complete && secondaryFile != "" {
			if _, err := os.Stat(filepath.Join(full, secondaryFile)); err == nil {
				complete = true
			}
		}
		if complete {
			summary.CompleteEvidenceDirs++
			summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
				Path:    filepath.ToSlash(filepath.Join(rootRel, entry.Name())),
				Class:   "complete_evidence",
				Message: "complete evidence preserved",
			})
		}
	}
	return count
}
