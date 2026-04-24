package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RunStateFileName is the basename of the live run-state file, relative to
// the project's .ddx directory.
const RunStateFileName = "run-state.json"

// RunState is the on-disk record of the currently-executing bead. It is
// written at execute-bead start and removed on completion (or on orphan
// recovery of a crashed worker). Operators and HELIX consume this file to
// observe what is running without polling the bead tracker.
//
// See CONTRACT-001 §5 (Always-on runtime metrics and provenance).
type RunState struct {
	BeadID       string    `json:"bead_id"`
	AttemptID    string    `json:"attempt_id"`
	Harness      string    `json:"harness,omitempty"`
	Model        string    `json:"model,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	WorktreePath string    `json:"worktree_path"`
}

// runStatePath returns the absolute path of the run-state file for the
// project rooted at projectRoot.
func runStatePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".ddx", RunStateFileName)
}

// WriteRunState atomically writes state to .ddx/run-state.json under
// projectRoot. The write goes to a sibling tmp file and is then renamed into
// place so readers never observe a partial record.
func WriteRunState(projectRoot string, state RunState) error {
	if projectRoot == "" {
		return errors.New("WriteRunState: projectRoot is empty")
	}
	dir := filepath.Join(projectRoot, ".ddx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("run-state: mkdir .ddx: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("run-state: marshal: %w", err)
	}
	data = append(data, '\n')

	final := runStatePath(projectRoot)
	tmp, err := os.CreateTemp(dir, "run-state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("run-state: create tmp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("run-state: write tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("run-state: close tmp: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("run-state: rename: %w", err)
	}
	return nil
}

// ReadRunState returns the live run-state for projectRoot, or (nil, nil) when
// the file does not exist. A malformed file surfaces as an error.
func ReadRunState(projectRoot string) (*RunState, error) {
	data, err := os.ReadFile(runStatePath(projectRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s RunState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("run-state: parse: %w", err)
	}
	return &s, nil
}

// ClearRunState removes .ddx/run-state.json. A missing file is not an error.
func ClearRunState(projectRoot string) error {
	err := os.Remove(runStatePath(projectRoot))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
