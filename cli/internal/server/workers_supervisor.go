package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
)

const (
	workerDesiredStateVersion = 1
	defaultWorkerCount        = 0
	defaultWorkerProfile      = "default"
	defaultWorkerIdleInterval = 30 * time.Second
	defaultRestartBackoff     = 30 * time.Second
	defaultRestartBackoffMax  = 10 * time.Minute
	defaultRestartLimit       = 6
)

// WorkerRestartPolicy controls whether the supervisor should restart
// unexpected exits and how aggressively it may retry.
type WorkerRestartPolicy struct {
	Enabled            bool                 `json:"enabled"`
	MaxRestartsPerHour int                  `json:"max_restarts_per_hour"`
	Backoff            executeloop.Duration `json:"backoff"`
	BackoffMax         executeloop.Duration `json:"backoff_max"`
}

// WorkerDesiredState is the durable desired-state model persisted at
// .ddx/workers/desired.json.
type WorkerDesiredState struct {
	Version      int                   `json:"version"`
	ProjectRoot  string                `json:"project_root"`
	DesiredCount int                   `json:"desired_count"`
	DefaultSpec  ExecuteLoopWorkerSpec `json:"default_spec"`
	Restart      WorkerRestartPolicy   `json:"restart"`
	UpdatedAt    time.Time             `json:"updated_at"`
}

// DefaultWorkerDesiredState returns a sane desired-state skeleton for a
// project. Callers normally override DesiredCount before saving.
func DefaultWorkerDesiredState(projectRoot string) WorkerDesiredState {
	state := WorkerDesiredState{
		Version:      workerDesiredStateVersion,
		ProjectRoot:  projectRoot,
		DesiredCount: defaultWorkerCount,
		DefaultSpec: ExecuteLoopWorkerSpec{
			Mode:         executeloop.ModeWatch,
			IdleInterval: executeloop.Duration{Duration: defaultWorkerIdleInterval},
			Profile:      defaultWorkerProfile,
		},
		Restart: WorkerRestartPolicy{
			Enabled:            true,
			MaxRestartsPerHour: defaultRestartLimit,
			Backoff:            executeloop.Duration{Duration: defaultRestartBackoff},
			BackoffMax:         executeloop.Duration{Duration: defaultRestartBackoffMax},
		},
		UpdatedAt: time.Now().UTC(),
	}
	state.DefaultSpec.ApplyDefaults()
	return state
}

// ApplyDefaults fills in sane defaults without mutating explicit operator
// choices. The project root is injected by the caller so the file can be
// validated independently of where it was loaded from.
func (s *WorkerDesiredState) ApplyDefaults(projectRoot string) {
	if s == nil {
		return
	}
	if s.Version == 0 {
		s.Version = workerDesiredStateVersion
	}
	if s.ProjectRoot == "" {
		s.ProjectRoot = projectRoot
	}
	if s.DefaultSpec.Mode == "" {
		s.DefaultSpec.Mode = executeloop.ModeWatch
	}
	if s.DefaultSpec.Mode == executeloop.ModeWatch && s.DefaultSpec.IdleInterval.Duration == 0 {
		s.DefaultSpec.IdleInterval = executeloop.Duration{Duration: defaultWorkerIdleInterval}
	}
	if s.DefaultSpec.Profile == "" {
		s.DefaultSpec.Profile = defaultWorkerProfile
	}
	s.DefaultSpec.ApplyDefaults()

	if s.Restart.Enabled {
		if s.Restart.MaxRestartsPerHour == 0 {
			s.Restart.MaxRestartsPerHour = defaultRestartLimit
		}
		if s.Restart.Backoff.Duration == 0 {
			s.Restart.Backoff = executeloop.Duration{Duration: defaultRestartBackoff}
		}
		if s.Restart.BackoffMax.Duration == 0 {
			s.Restart.BackoffMax = executeloop.Duration{Duration: defaultRestartBackoffMax}
		}
	}
}

// Validate checks the normalized desired state for internal consistency.
func (s WorkerDesiredState) Validate() error {
	if s.Version != workerDesiredStateVersion {
		return fmt.Errorf("worker desired state: unsupported version %d", s.Version)
	}
	if s.ProjectRoot == "" {
		return fmt.Errorf("worker desired state: project_root is required")
	}
	if s.DesiredCount < 0 {
		return fmt.Errorf("worker desired state: desired_count must be >= 0")
	}
	if s.DefaultSpec.Mode == "" {
		return fmt.Errorf("worker desired state: default_spec.mode is required")
	}
	if err := s.DefaultSpec.Validate(); err != nil {
		return fmt.Errorf("worker desired state: default_spec invalid: %w", err)
	}
	if s.Restart.Enabled {
		if s.Restart.MaxRestartsPerHour <= 0 {
			return fmt.Errorf("worker desired state: restart.max_restarts_per_hour must be > 0 when restart is enabled")
		}
		if s.Restart.Backoff.Duration <= 0 {
			return fmt.Errorf("worker desired state: restart.backoff must be > 0 when restart is enabled")
		}
		if s.Restart.BackoffMax.Duration <= 0 {
			return fmt.Errorf("worker desired state: restart.backoff_max must be > 0 when restart is enabled")
		}
		if s.Restart.Backoff.Duration > s.Restart.BackoffMax.Duration {
			return fmt.Errorf("worker desired state: restart.backoff must not exceed restart.backoff_max")
		}
	}
	if s.UpdatedAt.IsZero() {
		return fmt.Errorf("worker desired state: updated_at is required")
	}
	return nil
}

// WorkerSupervisor reconciles durable desired worker state with the live
// server-owned worker registry.
type WorkerSupervisor struct {
	manager *WorkerManager

	mu               sync.Mutex
	seenTerminals    map[string]time.Time
	blockedTerminals map[string]string
	restartEvents    []time.Time
}

// NewWorkerSupervisor returns a supervisor for the given manager.
func NewWorkerSupervisor(m *WorkerManager) *WorkerSupervisor {
	return &WorkerSupervisor{
		manager:          m,
		seenTerminals:    map[string]time.Time{},
		blockedTerminals: map[string]string{},
	}
}

func (s *WorkerSupervisor) desiredStatePath() string {
	if s == nil || s.manager == nil {
		return ""
	}
	return filepath.Join(s.manager.rootDir, "desired.json")
}

// SaveDesiredState normalizes, validates, and persists the desired worker
// state. UpdatedAt is refreshed on every save.
func (s *WorkerSupervisor) SaveDesiredState(state *WorkerDesiredState) error {
	if s == nil || s.manager == nil {
		return fmt.Errorf("worker supervisor is not configured")
	}
	if state == nil {
		return fmt.Errorf("worker desired state is required")
	}

	state.ApplyDefaults(s.manager.projectRoot)
	state.UpdatedAt = time.Now().UTC()
	if err := state.Validate(); err != nil {
		return err
	}

	if err := os.MkdirAll(s.manager.rootDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := s.desiredStatePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.desiredStatePath())
}

// LoadDesiredState reads and validates the durable desired-state file.
func (s *WorkerSupervisor) LoadDesiredState() (WorkerDesiredState, error) {
	if s == nil || s.manager == nil {
		return WorkerDesiredState{}, fmt.Errorf("worker supervisor is not configured")
	}

	data, err := os.ReadFile(s.desiredStatePath())
	if err != nil {
		return WorkerDesiredState{}, err
	}

	var state WorkerDesiredState
	if err := json.Unmarshal(data, &state); err != nil {
		return WorkerDesiredState{}, err
	}
	if state.ProjectRoot != "" && state.ProjectRoot != s.manager.projectRoot {
		return WorkerDesiredState{}, fmt.Errorf("worker desired state: project_root %q does not match manager root %q", state.ProjectRoot, s.manager.projectRoot)
	}
	state.ApplyDefaults(s.manager.projectRoot)
	if err := state.Validate(); err != nil {
		return WorkerDesiredState{}, err
	}
	return state, nil
}

// Reconcile loads the desired state and brings the worker registry toward it.
func (s *WorkerSupervisor) Reconcile() error {
	return s.ReconcileAt(time.Now().UTC())
}

// ReconcileAt is the deterministic reconciliation entry point used by tests.
func (s *WorkerSupervisor) ReconcileAt(now time.Time) error {
	if s == nil || s.manager == nil {
		return fmt.Errorf("worker supervisor is not configured")
	}

	state, err := s.LoadDesiredState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	projectRoot := state.ProjectRoot
	if projectRoot == "" {
		projectRoot = s.manager.projectRoot
	}

	active, running, terminals, staleIDs, err := s.snapshotWorkers(projectRoot)
	if err != nil {
		return err
	}

	for _, id := range staleIDs {
		if err := s.manager.stopStaleDiskEntry(id); err != nil {
			return err
		}
	}

	s.recordTerminalHistory(terminals, now)

	if len(active) > state.DesiredCount {
		excess := len(active) - state.DesiredCount
		if err := s.stopNewestExcess(running, excess); err != nil {
			return err
		}
	}

	if len(active) < state.DesiredCount && s.canStartMore(state, now) {
		missing := state.DesiredCount - len(active)
		for i := 0; i < missing; i++ {
			spec := state.DefaultSpec
			spec.ProjectRoot = projectRoot
			if _, err := s.manager.StartExecuteLoop(spec); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *WorkerSupervisor) snapshotWorkers(projectRoot string) (active []WorkerRecord, running []WorkerRecord, terminals []WorkerRecord, staleIDs []string, err error) {
	if s == nil || s.manager == nil {
		return nil, nil, nil, nil, fmt.Errorf("worker supervisor is not configured")
	}

	entries, err := os.ReadDir(s.manager.rootDir)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dir := filepath.Join(s.manager.rootDir, entry.Name())
		rec, readErr := s.manager.readRecord(dir)
		if readErr != nil {
			continue
		}
		if rec.Kind != "work" {
			continue
		}
		if rec.ProjectRoot != "" && rec.ProjectRoot != projectRoot {
			continue
		}

		hasHandle := s.manager.hasWorkerHandle(rec.ID)
		switch rec.State {
		case "running", "stopping":
			if !hasHandle {
				staleIDs = append(staleIDs, rec.ID)
				continue
			}
			active = append(active, rec)
			if rec.State == "running" {
				running = append(running, rec)
			}
		default:
			terminals = append(terminals, rec)
		}
	}

	sort.Slice(active, func(i, j int) bool {
		if active[i].StartedAt.Equal(active[j].StartedAt) {
			return active[i].ID > active[j].ID
		}
		return active[i].StartedAt.After(active[j].StartedAt)
	})
	sort.Slice(running, func(i, j int) bool {
		if running[i].StartedAt.Equal(running[j].StartedAt) {
			return running[i].ID > running[j].ID
		}
		return running[i].StartedAt.After(running[j].StartedAt)
	})

	return active, running, terminals, staleIDs, nil
}

func (s *WorkerSupervisor) recordTerminalHistory(terminals []WorkerRecord, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(terminals) == 0 {
		return
	}

	if s.seenTerminals == nil {
		s.seenTerminals = map[string]time.Time{}
	}
	if s.blockedTerminals == nil {
		s.blockedTerminals = map[string]string{}
	}

	for _, rec := range terminals {
		if _, seen := s.seenTerminals[rec.ID]; seen {
			continue
		}
		s.seenTerminals[rec.ID] = now
		if isExpectedTerminalWorker(rec) {
			continue
		}
		if isRestartBlockedWorker(rec) {
			s.blockedTerminals[rec.ID] = firstNonEmpty(
				rec.ReapReason,
				rec.Status,
				rec.LastError,
				rec.Error,
				rec.State,
			)
			continue
		}
		ts := rec.FinishedAt
		if ts.IsZero() {
			ts = rec.StartedAt
		}
		if ts.IsZero() {
			ts = now
		}
		s.restartEvents = append(s.restartEvents, ts)
	}

	s.restartEvents = pruneTimeWindow(s.restartEvents, now, time.Hour)
}

func (s *WorkerSupervisor) stopNewestExcess(running []WorkerRecord, excess int) error {
	if excess <= 0 {
		return nil
	}

	limit := excess
	if len(running) < limit {
		limit = len(running)
	}
	for i := 0; i < limit; i++ {
		if err := s.manager.Stop(running[i].ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *WorkerSupervisor) canStartMore(state WorkerDesiredState, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.blockedTerminals) > 0 {
		return false
	}

	events := pruneTimeWindow(s.restartEvents, now, time.Hour)
	s.restartEvents = events
	if !state.Restart.Enabled {
		return len(events) == 0
	}
	if len(events) == 0 {
		return true
	}
	if len(events) > state.Restart.MaxRestartsPerHour {
		return false
	}
	delay := restartBackoffDelay(state.Restart, len(events))
	last := events[0]
	for _, ts := range events[1:] {
		if ts.After(last) {
			last = ts
		}
	}
	return !now.Before(last.Add(delay))
}

func restartBackoffDelay(policy WorkerRestartPolicy, recentEvents int) time.Duration {
	delay := policy.Backoff.Duration
	if delay <= 0 {
		return 0
	}
	if recentEvents <= 1 {
		if policy.BackoffMax.Duration > 0 && delay > policy.BackoffMax.Duration {
			return policy.BackoffMax.Duration
		}
		return delay
	}

	shift := recentEvents - 1
	for shift > 0 {
		if delay > time.Duration(1<<62) {
			delay = policy.BackoffMax.Duration
			break
		}
		delay *= 2
		shift--
		if policy.BackoffMax.Duration > 0 && delay >= policy.BackoffMax.Duration {
			delay = policy.BackoffMax.Duration
			break
		}
	}
	if policy.BackoffMax.Duration > 0 && delay > policy.BackoffMax.Duration {
		delay = policy.BackoffMax.Duration
	}
	return delay
}

func pruneTimeWindow(times []time.Time, now time.Time, window time.Duration) []time.Time {
	if window <= 0 || len(times) == 0 {
		return append([]time.Time(nil), times...)
	}
	cutoff := now.Add(-window)
	out := make([]time.Time, 0, len(times))
	for _, ts := range times {
		if ts.Before(cutoff) {
			continue
		}
		out = append(out, ts)
	}
	return out
}

func isExpectedTerminalWorker(rec WorkerRecord) bool {
	switch rec.State {
	case "stopped":
		return true
	case "exited":
		switch rec.Status {
		case "success", "no_ready_work":
			return true
		}
	}
	return false
}

func isRestartBlockedWorker(rec WorkerRecord) bool {
	fields := []string{
		rec.ReapReason,
		rec.Status,
		rec.LastError,
		rec.Error,
		rec.State,
	}
	for _, field := range fields {
		lower := strings.ToLower(field)
		if strings.Contains(lower, "operator_attention") ||
			strings.Contains(lower, "dirty_root") ||
			strings.Contains(lower, "dirty-root") ||
			strings.Contains(lower, "dirty root") {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (m *WorkerManager) hasWorkerHandle(id string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.workers[id]
	return ok
}
