package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// WorkerDesiredStateVersion is the schema version written to
// .ddx/workers/desired.json. Bumped when the on-disk shape changes
// incompatibly.
const WorkerDesiredStateVersion = 1

// WorkerDefaultSpec is the worker spec the supervisor uses when it starts a
// worker to satisfy desired_count. It is a serialisable projection of the
// fields a server-managed drain worker needs; the supervisor expands it into
// an ExecuteLoopWorkerSpec at start time.
type WorkerDefaultSpec struct {
	Mode         string `json:"mode,omitempty"`
	IdleInterval string `json:"idle_interval,omitempty"`
	Profile      string `json:"profile,omitempty"`
	Harness      string `json:"harness,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	LabelFilter  string `json:"label_filter,omitempty"`
}

// WorkerRestartPolicy controls whether and how fast the supervisor restarts a
// server-managed worker that exits unexpectedly.
type WorkerRestartPolicy struct {
	Enabled            bool   `json:"enabled"`
	MaxRestartsPerHour int    `json:"max_restarts_per_hour,omitempty"`
	Backoff            string `json:"backoff,omitempty"`
	BackoffMax         string `json:"backoff_max,omitempty"`
}

// WorkerDesiredState is the durable per-project intention persisted at
// .ddx/workers/desired.json. The server reconcile loop reads it on every tick
// and on explicit wake to keep the actual number of server-managed workers
// equal to DesiredCount.
type WorkerDesiredState struct {
	Version      int                 `json:"version"`
	ProjectRoot  string              `json:"project_root,omitempty"`
	DesiredCount int                 `json:"desired_count"`
	DefaultSpec  WorkerDefaultSpec   `json:"default_spec"`
	Restart      WorkerRestartPolicy `json:"restart"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

// Validate checks the desired-state invariants. It returns an error for values
// that would make reconcile unsafe; it does not mutate the receiver.
func (s *WorkerDesiredState) Validate() error {
	if s == nil {
		return fmt.Errorf("worker desired state is nil")
	}
	if s.Version != WorkerDesiredStateVersion {
		return fmt.Errorf("unsupported worker desired-state version %d (want %d)", s.Version, WorkerDesiredStateVersion)
	}
	if s.DesiredCount < 0 {
		return fmt.Errorf("desired_count must be >= 0, got %d", s.DesiredCount)
	}
	if s.Restart.MaxRestartsPerHour < 0 {
		return fmt.Errorf("restart.max_restarts_per_hour must be >= 0, got %d", s.Restart.MaxRestartsPerHour)
	}
	if s.DefaultSpec.IdleInterval != "" {
		if _, err := time.ParseDuration(s.DefaultSpec.IdleInterval); err != nil {
			return fmt.Errorf("default_spec.idle_interval %q is not a valid duration: %w", s.DefaultSpec.IdleInterval, err)
		}
	}
	if s.Restart.Backoff != "" {
		if _, err := time.ParseDuration(s.Restart.Backoff); err != nil {
			return fmt.Errorf("restart.backoff %q is not a valid duration: %w", s.Restart.Backoff, err)
		}
	}
	if s.Restart.BackoffMax != "" {
		if _, err := time.ParseDuration(s.Restart.BackoffMax); err != nil {
			return fmt.Errorf("restart.backoff_max %q is not a valid duration: %w", s.Restart.BackoffMax, err)
		}
	}
	return nil
}

// workerDesiredStatePath returns the on-disk location of the desired-state
// file for projectRoot.
func workerDesiredStatePath(projectRoot string) string {
	return ddxroot.JoinProject(projectRoot, "workers", "desired.json")
}

// LoadWorkerDesiredState reads .ddx/workers/desired.json for projectRoot. The
// returned error wraps os.ErrNotExist when the file is absent so callers can
// treat "no desired state" as "manage nothing" via errors.Is.
func LoadWorkerDesiredState(projectRoot string) (*WorkerDesiredState, error) {
	path := workerDesiredStatePath(projectRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state WorkerDesiredState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := state.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &state, nil
}

// SaveWorkerDesiredState validates and atomically persists state to
// .ddx/workers/desired.json. It stamps UpdatedAt and Version, and fills
// ProjectRoot when empty, before writing.
func SaveWorkerDesiredState(projectRoot string, state *WorkerDesiredState) error {
	if state == nil {
		return fmt.Errorf("worker desired state is nil")
	}
	if state.Version == 0 {
		state.Version = WorkerDesiredStateVersion
	}
	if state.ProjectRoot == "" {
		state.ProjectRoot = projectRoot
	}
	state.UpdatedAt = time.Now().UTC()
	if err := state.Validate(); err != nil {
		return err
	}
	path := workerDesiredStatePath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
