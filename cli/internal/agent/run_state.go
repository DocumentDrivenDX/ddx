package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// RunStateFileName is the basename of the live run-state file, relative to
// the project's .ddx directory.
const RunStateFileName = "run-state.json"

// RunStateDirName is the .ddx subdirectory containing per-attempt live-state
// records. The legacy .ddx/run-state.json file remains as a compatibility
// summary view for older operators.
const RunStateDirName = "run-state"

// RunStateLivenessTTL is the default expiry window written to refreshed
// attempt liveness records.
const RunStateLivenessTTL = 2 * time.Minute

// RunState is the on-disk record of an executing bead attempt. It is written at
// execute-bead start, refreshed while the agent runs, and removed on completion
// (or on orphan recovery of a crashed worker). Operators and HELIX consume this
// state to observe what is running without polling the bead tracker.
//
// See CONTRACT-001 §5 (Always-on runtime metrics and provenance).
type RunState struct {
	BeadID       string    `json:"bead_id"`
	AttemptID    string    `json:"attempt_id"`
	Harness      string    `json:"harness,omitempty"`
	Model        string    `json:"model,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	WorktreePath string    `json:"worktree_path"`
	PID          int       `json:"pid,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	RefreshedAt  time.Time `json:"refreshed_at,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	// Candidate-cycle fields mirror cleanup metadata so operators can see
	// which candidate phase owns a long-lived attempt worktree.
	CandidateCyclePhase string `json:"candidate_cycle_phase,omitempty"`
	CandidateRef        string `json:"candidate_ref,omitempty"`
	CandidateRev        string `json:"candidate_rev,omitempty"`
	CycleIndex          int    `json:"cycle_index,omitempty"`
	ReviewActive        bool   `json:"review_active,omitempty"`
	RepairActive        bool   `json:"repair_active,omitempty"`
}

// runStatePath returns the absolute path of the run-state file for the
// project rooted at projectRoot.
func runStatePath(projectRoot string) string {
	return filepath.Join(beadStoreRoot(projectRoot), RunStateFileName)
}

func runStateDirPath(projectRoot string) string {
	return filepath.Join(beadStoreRoot(projectRoot), RunStateDirName)
}

func existingRunStatePath(projectRoot string) (string, bool) {
	root, ok := existingBeadStoreRoot(projectRoot)
	if !ok {
		return "", false
	}
	path := filepath.Join(root, RunStateFileName)
	if _, err := os.Stat(path); err == nil {
		return path, true
	}
	return "", false
}

func existingRunStateDirPath(projectRoot string) (string, bool) {
	root, ok := existingBeadStoreRoot(projectRoot)
	if !ok {
		return "", false
	}
	path := filepath.Join(root, RunStateDirName)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path, true
	}
	return "", false
}

func existingBeadStoreRoot(projectRoot string) (string, bool) {
	if projectRoot == "" {
		return "", false
	}
	inTree := filepath.Join(projectRoot, ddxroot.DirName)
	if info, err := os.Stat(inTree); err == nil && info.IsDir() {
		return inTree, true
	}
	if root, ok := ddxroot.ExistingPath(context.Background(), projectRoot); ok {
		return root, true
	}
	return "", false
}

func runStateAttemptPath(projectRoot, attemptID string) (string, error) {
	name, err := runStateAttemptFileName(attemptID)
	if err != nil {
		return "", err
	}
	return filepath.Join(runStateDirPath(projectRoot), name), nil
}

func runStateAttemptFileName(attemptID string) (string, error) {
	attemptID = strings.TrimSpace(attemptID)
	if attemptID == "" {
		return "", errors.New("run-state: attempt_id is empty")
	}
	if filepath.Base(attemptID) != attemptID || strings.ContainsAny(attemptID, `/\`) {
		return "", fmt.Errorf("run-state: invalid attempt_id %q", attemptID)
	}
	return attemptID + ".json", nil
}

func normalizeRunState(state RunState) RunState {
	now := time.Now().UTC()
	if state.StartedAt.IsZero() {
		state.StartedAt = now
	}
	if state.RefreshedAt.IsZero() {
		state.RefreshedAt = now
	}
	if state.ExpiresAt.IsZero() {
		state.ExpiresAt = state.RefreshedAt.Add(RunStateLivenessTTL)
	}
	if state.PID == 0 {
		state.PID = os.Getpid()
	}
	return state
}

// WriteRunState atomically writes state to a per-attempt file under
// .ddx/run-state/ and refreshes .ddx/run-state.json as a legacy compatibility
// summary. Writes go to sibling tmp files and are then renamed into place so
// readers never observe a partial record.
func WriteRunState(projectRoot string, state RunState) error {
	if projectRoot == "" {
		return errors.New("WriteRunState: projectRoot is empty")
	}
	state = normalizeRunState(state)
	attemptPath, err := runStateAttemptPath(projectRoot, state.AttemptID)
	if err != nil {
		return err
	}
	ddxDir := beadStoreRoot(projectRoot)
	attemptDir := filepath.Dir(attemptPath)
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		return fmt.Errorf("run-state: mkdir .ddx: %w", err)
	}
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return fmt.Errorf("run-state: mkdir attempts: %w", err)
	}
	if err := writeRunStateJSON(attemptPath, state); err != nil {
		return err
	}
	return writeRunStateJSON(runStatePath(projectRoot), state)
}

func writeRunStateJSON(final string, state RunState) error {
	dir := filepath.Dir(final)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("run-state: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("run-state: marshal: %w", err)
	}
	data = append(data, '\n')

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

// ReadRunState returns a compatibility live-state summary for projectRoot, or
// (nil, nil) when no live attempt exists. A malformed compatibility file
// surfaces as an error; if the compatibility file is missing, the newest
// per-attempt record is returned.
func ReadRunState(projectRoot string) (*RunState, error) {
	path, ok := existingRunStatePath(projectRoot)
	if !ok {
		return nil, nil
	}
	state, err := readRunStateFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		states, statesErr := readRunStateAttemptFiles(projectRoot)
		if statesErr != nil {
			return nil, statesErr
		}
		if latest, ok := latestRunState(states); ok {
			return &latest, nil
		}
		return nil, nil
	}
	return state, nil
}

func readRunStateFile(path string) (*RunState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		return nil, err
	}
	var s RunState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("run-state: parse: %w", err)
	}
	return &s, nil
}

// ReadRunStates returns all live attempt records. The per-attempt directory is
// authoritative; when it is absent or empty, this falls back to the legacy
// compatibility file so older on-disk state can still be recovered.
func ReadRunStates(projectRoot string) ([]RunState, error) {
	states, err := readRunStateAttemptFiles(projectRoot)
	if err != nil {
		return nil, err
	}
	if len(states) > 0 {
		return states, nil
	}
	path, ok := existingRunStatePath(projectRoot)
	if !ok {
		return nil, nil
	}
	state, err := readRunStateFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if state == nil {
		return nil, nil
	}
	return []RunState{*state}, nil
}

func readRunStateAttemptFiles(projectRoot string) ([]RunState, error) {
	dir, ok := existingRunStateDirPath(projectRoot)
	if !ok {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	states := make([]RunState, 0, len(names))
	for _, name := range names {
		state, err := readRunStateFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if state != nil {
			states = append(states, *state)
		}
	}
	return states, nil
}

func latestRunState(states []RunState) (RunState, bool) {
	if len(states) == 0 {
		return RunState{}, false
	}
	latest := states[0]
	for _, state := range states[1:] {
		if runStateNewer(state, latest) {
			latest = state
		}
	}
	return latest, true
}

func runStateNewer(a, b RunState) bool {
	if !a.RefreshedAt.Equal(b.RefreshedAt) {
		return a.RefreshedAt.After(b.RefreshedAt)
	}
	if !a.StartedAt.Equal(b.StartedAt) {
		return a.StartedAt.After(b.StartedAt)
	}
	return a.AttemptID > b.AttemptID
}

// ClearRunState removes all run-state records, including the compatibility
// summary and per-attempt files. A missing file or directory is not an error.
func ClearRunState(projectRoot string) error {
	err := os.Remove(runStatePath(projectRoot))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	entries, err := os.ReadDir(runStateDirPath(projectRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(runStateDirPath(projectRoot), entry.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// ClearRunStateAttempt removes one attempt record and refreshes the legacy
// compatibility summary from the newest remaining attempt. Other live attempts
// are preserved.
func ClearRunStateAttempt(projectRoot, attemptID string) error {
	attemptPath, err := runStateAttemptPath(projectRoot, attemptID)
	if err != nil {
		return err
	}
	if err := os.Remove(attemptPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	states, err := readRunStateAttemptFiles(projectRoot)
	if err != nil {
		return err
	}
	if latest, ok := latestRunState(states); ok {
		return writeRunStateJSON(runStatePath(projectRoot), latest)
	}
	err = os.Remove(runStatePath(projectRoot))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
