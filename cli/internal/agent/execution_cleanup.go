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
)

// ExecutionCleanupMetadataFileName is the filename used to describe ownership
// and liveness for a DDx temp execution directory.
const ExecutionCleanupMetadataFileName = "cleanup.json"

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
	ProjectRoot  string                    `json:"project_root,omitempty"`
	BeadID       string                    `json:"bead_id,omitempty"`
	AttemptID    string                    `json:"attempt_id,omitempty"`
	WorktreePath string                    `json:"worktree_path,omitempty"`
	Registered   bool                      `json:"registered,omitempty"`
	Preserved    bool                      `json:"preserved,omitempty"`
	CreatedAt    time.Time                 `json:"created_at,omitempty"`
	Liveness     *ExecutionCleanupLiveness `json:"liveness,omitempty"`
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

	RemovedUnregisteredTempDirs int64 `json:"removed_unregistered_temp_dirs"`
	RemovedRegisteredWorktrees  int64 `json:"removed_registered_worktrees"`
	RemovedRunStateFiles        int64 `json:"removed_run_state_files"`

	BytesReclaimed  int64 `json:"bytes_reclaimed"`
	InodesReclaimed int64 `json:"inodes_reclaimed"`

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
	if runState != nil {
		if meta.WorktreePath != "" && filepath.Clean(runState.WorktreePath) == filepath.Clean(meta.WorktreePath) {
			return true, "matched live run-state"
		}
		if meta.AttemptID != "" && runState.AttemptID == meta.AttemptID {
			return true, "matched live run-state"
		}
	}
	if meta.Liveness != nil {
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
	ProjectRoot string
	TempRoot    string
	GitOps      GitOps
	Now         func() time.Time
	Probe       ExecutionCleanupLivenessProbe
}

// NewExecutionCleanupManager constructs a cleanup manager with the default
// DDx temp worktree root and the default liveness probe.
func NewExecutionCleanupManager(projectRoot string, gitOps GitOps) *ExecutionCleanupManager {
	return &ExecutionCleanupManager{
		ProjectRoot: projectRoot,
		TempRoot:    executionCleanupTempRoot(),
		GitOps:      gitOps,
		Now:         time.Now,
		Probe:       defaultExecutionCleanupLivenessProbe{},
	}
}

// Cleanup scans the configured DDx temp worktree root, consults the project
// run-state file and git worktree registry, and conservatively removes stale
// DDx-owned temp resources.
func (m *ExecutionCleanupManager) Cleanup(ctx context.Context) (ExecutionCleanupSummary, error) {
	_ = ctx

	summary := ExecutionCleanupSummary{
		ProjectRoot: m.ProjectRoot,
		TempRoot:    m.tempRoot(),
	}
	if summary.TempRoot == "" {
		summary.TempRoot = executionCleanupTempRoot()
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

	runState, runStateErr := ReadRunState(m.ProjectRoot)
	if runStateErr != nil {
		summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
			Path:    runStatePath(m.ProjectRoot),
			Class:   "run_state_read",
			Message: runStateErr.Error(),
		})
	}

	entries, err := os.ReadDir(summary.TempRoot)
	if err != nil {
		return summary, fmt.Errorf("execution cleanup: read temp root: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), ExecuteBeadWtPrefix) {
			continue
		}
		summary.ScannedTempDirs++
		path := filepath.Join(summary.TempRoot, entry.Name())

		meta, metaErr := ReadExecutionCleanupMetadata(path)
		if metaErr != nil {
			if !errors.Is(metaErr, os.ErrNotExist) {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    path,
					Class:   "metadata_read",
					Message: metaErr.Error(),
				})
			}
			continue
		}
		if meta.ProjectRoot != "" && filepath.Clean(meta.ProjectRoot) != filepath.Clean(m.ProjectRoot) {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    path,
				Class:   "ownership_mismatch",
				Message: fmt.Sprintf("metadata project_root=%s does not match manager project_root=%s", meta.ProjectRoot, m.ProjectRoot),
			})
			continue
		}

		live, reason := probe.IsLive(meta, runState, now())
		if live {
			summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
				Path:    path,
				Class:   "preserved_temp_dir",
				Message: reason,
			})
			continue
		}

		if _, ok := registered[filepath.Clean(path)]; ok {
			if m.GitOps == nil {
				summary.Issues = append(summary.Issues, ExecutionCleanupIssue{
					Path:     path,
					Class:    "registered_worktree",
					Message:  "no git ops available to remove registered worktree",
					Blocking: true,
				})
				continue
			}
			reclaimedBytes, reclaimedInodes, measureErr := measureTree(path)
			if measureErr != nil {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    path,
					Class:   "measure_tree",
					Message: measureErr.Error(),
				})
			}
			if err := m.GitOps.WorktreeRemove(m.ProjectRoot, path); err != nil {
				summary.Issues = append(summary.Issues, ExecutionCleanupIssue{
					Path:     path,
					Class:    "registered_worktree_remove",
					Message:  err.Error(),
					Blocking: true,
				})
				continue
			}
			summary.RemovedRegisteredWorktrees++
			summary.BytesReclaimed += reclaimedBytes
			summary.InodesReclaimed += reclaimedInodes
			summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
				Path:    path,
				Class:   "removed_registered_worktree",
				Message: reason,
				Bytes:   reclaimedBytes,
				Inodes:  reclaimedInodes,
			})
			continue
		}

		reclaimedBytes, reclaimedInodes, measureErr := measureTree(path)
		if measureErr != nil {
			summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
				Path:    path,
				Class:   "measure_tree",
				Message: measureErr.Error(),
			})
		}
		if err := os.RemoveAll(path); err != nil {
			summary.Issues = append(summary.Issues, ExecutionCleanupIssue{
				Path:     path,
				Class:    "unregistered_temp_dir_remove",
				Message:  err.Error(),
				Blocking: false,
			})
			continue
		}
		summary.RemovedUnregisteredTempDirs++
		summary.BytesReclaimed += reclaimedBytes
		summary.InodesReclaimed += reclaimedInodes
		summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
			Path:    path,
			Class:   "removed_unregistered_temp_dir",
			Message: reason,
			Bytes:   reclaimedBytes,
			Inodes:  reclaimedInodes,
		})
	}

	if m.GitOps != nil && summary.RemovedRegisteredWorktrees > 0 {
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
	if runState != nil {
		runStateLive := false
		if runState.WorktreePath != "" {
			meta := ExecutionCleanupMetadata{
				ProjectRoot:  m.ProjectRoot,
				AttemptID:    runState.AttemptID,
				WorktreePath: runState.WorktreePath,
			}
			if live, _ := probe.IsLive(meta, runState, now()); live {
				runStateLive = true
			}
		}
		if !runStateLive {
			runStatePath := runStatePath(m.ProjectRoot)
			var runStateBytes int64
			var runStatePresent bool
			if info, statErr := os.Stat(runStatePath); statErr == nil {
				runStatePresent = true
				runStateBytes = info.Size()
			}
			if err := ClearRunState(m.ProjectRoot); err != nil {
				summary.Warnings = append(summary.Warnings, ExecutionCleanupWarning{
					Path:    runStatePath,
					Class:   "run_state_clear",
					Message: err.Error(),
				})
			} else {
				if runStatePresent {
					summary.RemovedRunStateFiles++
					summary.BytesReclaimed += runStateBytes
					summary.InodesReclaimed++
				}
				summary.Observations = append(summary.Observations, ExecutionCleanupObservation{
					Path:    runStatePath,
					Class:   "removed_run_state",
					Message: "stale run-state",
					Bytes:   runStateBytes,
					Inodes:  1,
				})
			}
		}
	}

	summary.ScannedEvidenceDirs += scanCompleteEvidenceDirs(m.ProjectRoot, ExecuteBeadArtifactDir, "manifest.json", "result.json", &summary)
	summary.ScannedEvidenceDirs += scanCompleteEvidenceDirs(m.ProjectRoot, ".ddx/runs", "record.json", "", &summary)

	return summary, nil
}

func (m *ExecutionCleanupManager) tempRoot() string {
	if m != nil && m.TempRoot != "" {
		return m.TempRoot
	}
	return executionCleanupTempRoot()
}

func executionCleanupTempRoot() string {
	base := os.Getenv("DDX_EXEC_WT_DIR")
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
	info, err := os.Lstat(path)
	if err != nil {
		return 0, 0, err
	}
	if !info.IsDir() {
		return info.Size(), 1, nil
	}
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
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
