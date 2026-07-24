package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

const (
	workerDesiredStateVersion       = 2
	legacyWorkerDesiredStateVersion = 1
	defaultWorkerCount              = 0
	defaultWorkerIdleInterval       = 30 * time.Second
	defaultRestartBackoff           = 30 * time.Second
	defaultRestartBackoffMax        = 10 * time.Minute
	defaultRestartLimit             = 6

	// DefaultTerminalBlockTTL bounds how long a restart-blocked terminal
	// record (operator_attention, dirty_root, ...) can suppress restart
	// eligibility. Without this, a terminal newer than desired.UpdatedAt
	// would never clear via the freshness path and could block restarts
	// indefinitely.
	DefaultTerminalBlockTTL = 10 * time.Minute
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

// migrateWorkerDesiredState upgrades persisted desired state before defaults
// or validation run. Version 1 was written while DDx synthesized profile
// "default" for otherwise unpinned workers. Because v1 stored no provenance,
// that value is ambiguous with an operator explicitly choosing a Fizeau profile
// named "default". The route-neutral migration fails safe by clearing exactly
// that ambiguous value; every other explicit legacy profile is preserved.
func migrateWorkerDesiredState(s *WorkerDesiredState) {
	if s == nil || s.Version != legacyWorkerDesiredStateVersion {
		return
	}
	if s.DefaultSpec.Profile == "default" {
		s.DefaultSpec.Profile = ""
	}
	s.Version = workerDesiredStateVersion
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
	blockedTerminals map[string]blockedTerminal
	// terminalDiagnostics retains a read-only snapshot of interesting terminal
	// outcomes (restart-blocked and/or structured resource diagnoses) for
	// status callers. Restart policy still keys off blockedTerminals alone.
	terminalDiagnostics map[string]BlockedTerminalDiagnostic
	restartEvents       []time.Time
}

type blockedTerminal struct {
	Reason      string
	TerminalAt  time.Time
	Diagnosis   string
	Restartable bool
}

// BlockedTerminalDiagnostic is a read-only summary of a restart-blocked (or
// diagnosis-bearing) terminal worker, suitable for `ddx worker status` and
// other status consumers. It does not influence restart policy.
type BlockedTerminalDiagnostic struct {
	WorkerID   string    `json:"worker_id,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	TerminalAt time.Time `json:"terminal_at,omitempty"`
	// Diagnosis is a machine-readable classification when structured fields
	// are present (e.g. agent.ResourceExhaustionDiagnosisFD / "fd_exhaustion").
	Diagnosis string `json:"diagnosis,omitempty"`
	// Restartable is true when Diagnosis is worker-local: a fresh worker
	// process is expected to clear it (fd exhaustion), unlike root-storage
	// exhaustion which persists across restarts.
	Restartable bool `json:"restartable,omitempty"`
}

// NewWorkerSupervisor returns a supervisor for the given manager.
func NewWorkerSupervisor(m *WorkerManager) *WorkerSupervisor {
	return &WorkerSupervisor{
		manager:             m,
		seenTerminals:       map[string]time.Time{},
		blockedTerminals:    map[string]blockedTerminal{},
		terminalDiagnostics: map[string]BlockedTerminalDiagnostic{},
	}
}

// LatestBlockedTerminal returns the newest restart-blocked or
// diagnosis-bearing terminal known to this supervisor. ok is false when none
// have been recorded. Read-only: does not mutate desired state or restart
// policy.
func (s *WorkerSupervisor) LatestBlockedTerminal() (BlockedTerminalDiagnostic, bool) {
	if s == nil {
		return BlockedTerminalDiagnostic{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		best   BlockedTerminalDiagnostic
		found  bool
		bestAt time.Time
	)
	for _, diag := range s.terminalDiagnostics {
		if !found || diag.TerminalAt.After(bestAt) {
			best = diag
			bestAt = diag.TerminalAt
			found = true
		}
	}
	return best, found
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
	var desiredFields map[string]json.RawMessage
	if err := json.Unmarshal(data, &desiredFields); err != nil {
		return WorkerDesiredState{}, err
	}
	if rawDefaultSpec, ok := desiredFields["default_spec"]; ok {
		var defaultSpecFields map[string]json.RawMessage
		if err := json.Unmarshal(rawDefaultSpec, &defaultSpecFields); err != nil {
			return WorkerDesiredState{}, err
		}
		if err := rejectRemovedReviewRoutingFields(defaultSpecFields, "default_spec"); err != nil {
			return WorkerDesiredState{}, fmt.Errorf("worker desired state: %w", err)
		}
	}

	var state WorkerDesiredState
	if err := json.Unmarshal(data, &state); err != nil {
		return WorkerDesiredState{}, err
	}
	if state.ProjectRoot != "" && state.ProjectRoot != s.manager.projectRoot {
		return WorkerDesiredState{}, fmt.Errorf("worker desired state: project_root %q does not match manager root %q", state.ProjectRoot, s.manager.projectRoot)
	}
	migrateWorkerDesiredState(&state)
	state.ApplyDefaults(s.manager.projectRoot)
	if err := state.Validate(); err != nil {
		return WorkerDesiredState{}, err
	}
	return state, nil
}

// DesiredWorkerPresence is a read-only snapshot of desired vs live work
// workers for a project. When desired workers are absent, structured
// diagnosis fields (notably fd_exhaustion_diagnosis) explain why using
// managed-result / terminal-record data rather than freeform log scraping.
type DesiredWorkerPresence struct {
	DesiredCount          int    `json:"desired_count"`
	LiveCount             int    `json:"live_count"`
	MissingCount          int    `json:"missing_count"`
	FDExhaustionDiagnosis string `json:"fd_exhaustion_diagnosis,omitempty"`
	// LastTerminalWorkerID is the newest terminal work worker consulted when
	// computing diagnosis; empty when no terminal records exist.
	LastTerminalWorkerID string `json:"last_terminal_worker_id,omitempty"`
}

// DiagnoseDesiredWorkerPresence compares durable desired state against live
// work workers on disk and, when some are missing, surfaces a structured
// fd-exhaustion diagnosis from the newest terminal managed result or record.
// The helper is filesystem-only and safe for CLI `ddx worker status` callers
// that never run Reconcile().
func (s *WorkerSupervisor) DiagnoseDesiredWorkerPresence(state WorkerDesiredState, now time.Time) (DesiredWorkerPresence, error) {
	out := DesiredWorkerPresence{DesiredCount: state.DesiredCount}
	if s == nil || s.manager == nil {
		return out, fmt.Errorf("worker supervisor is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	active, _, terminals, _, err := s.snapshotWorkers(s.manager.projectRoot, now)
	if err != nil {
		// A missing workers directory is equivalent to zero live/terminal
		// workers (fresh project with only desired.json written).
		if !errors.Is(err, os.ErrNotExist) {
			return out, err
		}
	}
	out.LiveCount = len(active)
	if state.DesiredCount > out.LiveCount {
		out.MissingCount = state.DesiredCount - out.LiveCount
	}
	if out.MissingCount == 0 || len(terminals) == 0 {
		return out, nil
	}

	sort.Slice(terminals, func(i, j int) bool {
		ti := workerTerminalTime(terminals[i], now)
		tj := workerTerminalTime(terminals[j], now)
		if ti.Equal(tj) {
			return terminals[i].ID > terminals[j].ID
		}
		return ti.After(tj)
	})
	newest := terminals[0]
	out.LastTerminalWorkerID = newest.ID
	out.FDExhaustionDiagnosis = fdExhaustionDiagnosisForTerminal(s.manager.rootDir, newest)
	return out, nil
}

// fdExhaustionDiagnosisForTerminal returns agent.ResourceExhaustionDiagnosisFD
// when the terminal worker's structured managed result or record fields show
// fd exhaustion. Equality against known constants is intentional: the bead
// forbids a second EMFILE classifier and brittle freeform substring matching.
func fdExhaustionDiagnosisForTerminal(workersRoot string, rec WorkerRecord) string {
	if diagnosis := fdExhaustionDiagnosisFromRecord(rec); diagnosis != "" {
		return diagnosis
	}
	if workersRoot == "" || rec.ID == "" {
		return ""
	}
	res, ok := readManagedWorkerResult(filepath.Join(workersRoot, rec.ID))
	if !ok || res == nil {
		return ""
	}
	return fdExhaustionDiagnosisFromManagedResult(*res)
}

func fdExhaustionDiagnosisFromManagedResult(res ManagedWorkerResult) string {
	status := normalizeManagedWorkerReason(res.LastFailureStatus)
	stop := normalizeManagedWorkerReason(res.StopCondition)
	if status != agent.ExecuteBeadStatusResourceExhausted && stop != agent.ExecuteBeadStatusResourceExhausted {
		return ""
	}
	if res.LastFailureDetail == agent.FDExhaustionStopMessage {
		return agent.ResourceExhaustionDiagnosisFD
	}
	return ""
}

func fdExhaustionDiagnosisFromRecord(rec WorkerRecord) string {
	if rec.LastResult != nil {
		if normalizeManagedWorkerReason(rec.LastResult.Status) == agent.ExecuteBeadStatusResourceExhausted &&
			rec.LastResult.Detail == agent.FDExhaustionStopMessage {
			return agent.ResourceExhaustionDiagnosisFD
		}
	}
	if normalizeManagedWorkerReason(rec.Status) != agent.ExecuteBeadStatusResourceExhausted {
		return ""
	}
	if rec.LastError == agent.FDExhaustionStopMessage || rec.Error == agent.FDExhaustionStopMessage {
		return agent.ResourceExhaustionDiagnosisFD
	}
	if rec.LastResult != nil && rec.LastResult.Detail == agent.FDExhaustionStopMessage {
		return agent.ResourceExhaustionDiagnosisFD
	}
	return ""
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

	active, running, terminals, staleIDs, err := s.snapshotWorkers(projectRoot, now)
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

	if len(active) < state.DesiredCount {
		blockedCount := s.resolveBlockedTerminals(state, now)
		occupied := len(active) + blockedCount
		if occupied < state.DesiredCount && s.canStartMore(state, now) {
			missing := state.DesiredCount - occupied
			for i := 0; i < missing; i++ {
				spec := state.DefaultSpec
				spec.ProjectRoot = projectRoot
				if _, err := s.manager.StartExecuteLoop(spec); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (s *WorkerSupervisor) snapshotWorkers(projectRoot string, now time.Time) (active []WorkerRecord, running []WorkerRecord, terminals []WorkerRecord, staleIDs []string, err error) {
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

		switch rec.State {
		case "running", "stopping":
			if !s.workerRecordLive(rec, now) {
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
		s.blockedTerminals = map[string]blockedTerminal{}
	}
	if s.terminalDiagnostics == nil {
		s.terminalDiagnostics = map[string]BlockedTerminalDiagnostic{}
	}

	for _, rec := range terminals {
		if _, seen := s.seenTerminals[rec.ID]; seen {
			continue
		}
		s.seenTerminals[rec.ID] = now
		if isExpectedTerminalWorker(rec) {
			continue
		}

		reason, diagnosis, restartable := terminalDiagnosticFields(s.manager, rec)
		terminalAt := workerTerminalTime(rec, now)
		blocked := isRestartBlockedWorker(rec)

		if blocked {
			s.blockedTerminals[rec.ID] = blockedTerminal{
				Reason:      reason,
				TerminalAt:  terminalAt,
				Diagnosis:   diagnosis,
				Restartable: restartable,
			}
		} else {
			s.restartEvents = append(s.restartEvents, terminalAt)
		}

		// Retain diagnostics for status callers: every restart-blocked
		// terminal, plus any terminal that carries a structured diagnosis
		// (e.g. fd_exhaustion) even when restart remains allowed.
		if blocked || diagnosis != "" {
			s.terminalDiagnostics[rec.ID] = BlockedTerminalDiagnostic{
				WorkerID:    rec.ID,
				Reason:      reason,
				TerminalAt:  terminalAt,
				Diagnosis:   diagnosis,
				Restartable: restartable,
			}
		}
	}

	s.restartEvents = pruneTimeWindow(s.restartEvents, now, time.Hour)
}

// terminalDiagnosticFields extracts the human reason and structured diagnosis
// for a terminal worker. Structured ManagedWorkerResult / LastResult fields
// win over free-text matching so status consumers can rely on machine-readable
// classifications (e.g. "fd_exhaustion") without re-implementing EMFILE checks.
func terminalDiagnosticFields(m *WorkerManager, rec WorkerRecord) (reason, diagnosis string, restartable bool) {
	reason = firstNonEmpty(
		rec.ReapReason,
		rec.Status,
		rec.LastError,
		rec.Error,
		rec.State,
	)

	var managed *ManagedWorkerResult
	if m != nil && rec.ID != "" {
		if res, ok := readManagedWorkerResult(filepath.Join(m.rootDir, rec.ID)); ok {
			managed = res
		}
	}

	// Prefer explicit structured diagnosis from the managed-worker result.
	if managed != nil && strings.TrimSpace(managed.ResourceExhaustionDiagnosis) != "" {
		diagnosis = strings.TrimSpace(managed.ResourceExhaustionDiagnosis)
		restartable = managed.ResourceExhaustionRestartable
		if reason == "" || reason == rec.State {
			reason = firstNonEmpty(managed.LastFailureDetail, managed.LastFailureStatus, managed.StopCondition, reason)
		}
		return reason, diagnosis, restartable
	}

	// Prefer structured LastResult when the agent path already classified the
	// failure as resource_exhausted with the fd-exhaustion stop message.
	if rec.LastResult != nil {
		if d, ok := diagnosisFromResourceFields(rec.LastResult.Status, rec.LastResult.Detail); ok {
			if reason == "" || reason == rec.State {
				reason = firstNonEmpty(rec.LastResult.Detail, rec.LastResult.Status, reason)
			}
			return reason, d, true
		}
	}

	// Managed result without explicit diagnosis field: still map known
	// resource_exhausted + fd stop message through structured status/detail.
	if managed != nil {
		if d, ok := diagnosisFromResourceFields(managed.LastFailureStatus, managed.LastFailureDetail); ok {
			if reason == "" || reason == rec.State {
				reason = firstNonEmpty(managed.LastFailureDetail, managed.LastFailureStatus, reason)
			}
			return reason, d, true
		}
	}

	// Last resort: free-text fields that already carry the agent-emitted
	// FDExhaustionStopMessage constant (not a new EMFILE classifier).
	if d, ok := diagnosisFromResourceFields(rec.Status, firstNonEmpty(rec.LastError, rec.Error)); ok {
		return reason, d, true
	}

	return reason, "", false
}

// diagnosisFromResourceFields maps known agent-emitted resource-exhaustion
// status/detail pairs onto ResourceExhaustionDiagnosisFD. It only recognizes
// existing structured constants and fixtures — it does not classify raw
// errno values.
func diagnosisFromResourceFields(status, detail string) (string, bool) {
	status = strings.TrimSpace(status)
	detail = strings.TrimSpace(detail)
	if detail == agent.FDExhaustionStopMessage {
		return agent.ResourceExhaustionDiagnosisFD, true
	}
	if status == agent.ExecuteBeadStatusResourceExhausted &&
		strings.Contains(strings.ToLower(detail), "file-descriptor") {
		return agent.ResourceExhaustionDiagnosisFD, true
	}
	if status == agent.ResourceExhaustionDiagnosisFD ||
		strings.EqualFold(status, agent.ResourceExhaustionDiagnosisFD) {
		return agent.ResourceExhaustionDiagnosisFD, true
	}
	return "", false
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

// resolveBlockedTerminals reconciles the in-memory blocked-terminal set
// against current worktree state and returns how many active blocks remain.
// Each remaining active block consumes one desired worker slot rather than
// suppressing all starts. Blocks that expire and permit a restart are fed
// into restartEvents so the existing restart backoff throttles repeated
// expired-block restarts the same way it throttles other restart loops.
func (s *WorkerSupervisor) resolveBlockedTerminals(state WorkerDesiredState, now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.blockedTerminals) == 0 {
		return 0
	}

	dirtyPaths, known := projectRestartBlockingDirtyPaths(s.manager.projectRoot)
	if known {
		if len(dirtyPaths) == 0 {
			log.Printf("worker supervisor: clearing stale restart-blocked terminal(s) for %s; current worktree is clean enough for pre-claim", s.manager.projectRoot)
			for id, block := range s.blockedTerminals {
				s.recordExpiredBlockedTerminalLocked(id, block, now)
			}
			s.blockedTerminals = map[string]blockedTerminal{}
			return 0
		}
		log.Printf("worker supervisor: %d restart-blocked terminal(s) for %s consuming desired slot(s) due to current dirty paths: %s", len(s.blockedTerminals), s.manager.projectRoot, strings.Join(dirtyPaths, ", "))
		return len(s.blockedTerminals)
	}

	s.clearResolvedTerminalBlocksLocked(state.UpdatedAt, now)
	s.expireStaleBlockedTerminalsLocked(now)
	if len(s.blockedTerminals) > 0 {
		log.Printf("worker supervisor: %d restart-blocked terminal(s) for %s consuming desired slot(s) awaiting desired-state refresh", len(s.blockedTerminals), s.manager.projectRoot)
	}
	return len(s.blockedTerminals)
}

// expireStaleBlockedTerminalsLocked removes blocked-terminal entries older
// than DefaultTerminalBlockTTL, independent of desired.UpdatedAt freshness.
// Callers must hold s.mu. It only runs when the current dirty-path check is
// inconclusive (projectRestartBlockingDirtyPaths returned known=false) —
// when the worktree is confirmed dirty right now, that live signal, not the
// block's age, governs whether the block stays active. Without this, a
// terminal record newer than desired.UpdatedAt would never clear via
// clearResolvedTerminalBlocksLocked and could suppress restart eligibility
// forever.
func (s *WorkerSupervisor) expireStaleBlockedTerminalsLocked(now time.Time) {
	for id, block := range s.blockedTerminals {
		if block.TerminalAt.IsZero() {
			continue
		}
		if now.Sub(block.TerminalAt) < DefaultTerminalBlockTTL {
			continue
		}
		s.recordExpiredBlockedTerminalLocked(id, block, now)
		delete(s.blockedTerminals, id)
	}
}

// recordExpiredBlockedTerminalLocked logs the evidence for an expired
// restart-blocked terminal and feeds it into restartEvents, backdated to the
// terminal's own timestamp so the existing backoff window treats it the same
// as any other restart that happened at that time. Callers must hold s.mu.
func (s *WorkerSupervisor) recordExpiredBlockedTerminalLocked(id string, block blockedTerminal, now time.Time) {
	ts := block.TerminalAt
	if ts.IsZero() {
		ts = now
	}
	log.Printf("worker supervisor: expired restart-blocked terminal %s (reason=%s) for %s permits restart; feeding restart backoff", id, block.Reason, s.manager.projectRoot)
	s.restartEvents = append(s.restartEvents, ts)
}

func (s *WorkerSupervisor) canStartMore(state WorkerDesiredState, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

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

func projectRestartBlockingDirtyPaths(projectRoot string) ([]string, bool) {
	if projectRoot == "" {
		return nil, false
	}

	out, err := internalgit.Command(
		context.Background(),
		projectRoot,
		"status",
		"--porcelain",
		"--untracked-files=all",
		"--",
		".",
	).Output()
	if err != nil {
		return nil, false
	}

	var dirtyPaths []string
	seen := map[string]struct{}{}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" || len(line) < 4 {
			continue
		}

		path := strings.TrimSpace(line[3:])
		if path == "" {
			continue
		}
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = strings.TrimSpace(path[idx+4:])
		}
		if path == "" {
			continue
		}
		if strings.HasPrefix(path, ".ddx") {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		dirtyPaths = append(dirtyPaths, path)
	}
	return dirtyPaths, true
}

func (s *WorkerSupervisor) clearResolvedTerminalBlocksLocked(desiredUpdatedAt, now time.Time) {
	if desiredUpdatedAt.IsZero() || len(s.blockedTerminals) == 0 {
		return
	}
	for id, block := range s.blockedTerminals {
		if block.TerminalAt.IsZero() {
			continue
		}
		if desiredUpdatedAt.After(block.TerminalAt) {
			s.recordExpiredBlockedTerminalLocked(id, block, now)
			delete(s.blockedTerminals, id)
		}
	}
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

func workerTerminalTime(rec WorkerRecord, fallback time.Time) time.Time {
	if !rec.FinishedAt.IsZero() {
		return rec.FinishedAt
	}
	if !rec.StartedAt.IsZero() {
		return rec.StartedAt
	}
	return fallback
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

// workerRecordLive determines whether rec still represents a live worker.
// An existing in-process handle is authoritative: the manager itself started
// that worker and still owns its goroutine or subprocess, so no further
// checks run. Without a handle, a stale or reused PID could otherwise be
// mistaken for the original worker, so every available freshness signal must
// agree: the PID must exist, a recorded PGID must still match the live
// process group, a present liveness sidecar must have been touched within
// 2*bead.HeartbeatTTL, and a current attempt's run-state must not be
// expired. Signals that are absent (no sidecar, no run-state) are not
// treated as failures — only signals that are present and stale reject.
func (s *WorkerSupervisor) workerRecordLive(rec WorkerRecord, now time.Time) bool {
	if s == nil || s.manager == nil {
		return false
	}
	if s.manager.hasWorkerHandle(rec.ID) {
		return true
	}
	if rec.PID <= 0 || !processAlive(rec.PID) {
		return false
	}
	if rec.PGID > 0 {
		if pgid, ok := livePGID(rec.PID); ok && pgid != rec.PGID {
			return false
		}
	}
	if sidecar, err := workerstatus.ReadLiveness(s.manager.projectRoot, rec.ID); err == nil && !sidecar.LastActivityAt.IsZero() {
		if now.Sub(sidecar.LastActivityAt) > 2*bead.HeartbeatTTL {
			return false
		}
	}
	if rec.CurrentAttempt != nil && rec.CurrentAttempt.AttemptID != "" {
		if expired, found := currentAttemptRunStateExpired(s.manager.projectRoot, rec.CurrentAttempt.AttemptID, now); found && expired {
			return false
		}
	}
	return true
}

// currentAttemptRunStateExpired reports whether the run-state record for
// attemptID has expired as of now. found is false when no run-state record
// exists for that attempt, in which case expired is meaningless — callers
// must not reject liveness on absence alone.
func currentAttemptRunStateExpired(projectRoot, attemptID string, now time.Time) (expired bool, found bool) {
	states, err := agent.ReadRunStates(projectRoot)
	if err != nil {
		return false, false
	}
	for _, st := range states {
		if st.AttemptID != attemptID {
			continue
		}
		if st.ExpiresAt.IsZero() {
			return false, true
		}
		return now.After(st.ExpiresAt), true
	}
	return false, false
}
