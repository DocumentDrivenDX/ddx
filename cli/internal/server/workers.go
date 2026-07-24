package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	policyescalation "github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

type ExecuteLoopWorkerSpec = executeloop.ExecuteLoopSpec

type PluginActionWorkerSpec struct {
	ProjectRoot string `json:"project_root,omitempty"`
	Name        string `json:"name"`
	Action      string `json:"action"`
	Scope       string `json:"scope"`
}

type PluginActionExecutor func(ctx context.Context) (string, error)

// Terminal phases per FEAT-006.
var terminalPhases = map[string]bool{
	"done":      true,
	"preserved": true,
	"failed":    true,
}

// CurrentAttemptInfo is the in-flight attempt summary embedded in WorkerRecord.
type CurrentAttemptInfo struct {
	AttemptID string    `json:"attempt_id"`
	BeadID    string    `json:"bead_id"`
	BeadTitle string    `json:"bead_title,omitempty"`
	Harness   string    `json:"harness,omitempty"`
	Model     string    `json:"model,omitempty"`
	Profile   string    `json:"profile,omitempty"`
	Phase     string    `json:"phase"`
	PhaseSeq  int       `json:"phase_seq"`
	StartedAt time.Time `json:"started_at"`
	ElapsedMS int64     `json:"elapsed_ms"`
}

// PhaseTransition is one phase-transition entry in WorkerRecord.RecentPhases.
// Only phase-transition events (heartbeat=false) are stored here; heartbeats
// are not retained.
type PhaseTransition struct {
	Phase    string    `json:"phase"`
	TS       time.Time `json:"ts"`
	PhaseSeq int       `json:"phase_seq"`
}

// LastAttemptInfo summarises the most recently completed attempt.
type LastAttemptInfo struct {
	AttemptID string    `json:"attempt_id"`
	BeadID    string    `json:"bead_id"`
	Phase     string    `json:"phase"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	ElapsedMS int64     `json:"elapsed_ms"`
}

type WorkerLifecycleEvent struct {
	Action    string    `json:"action"`
	Actor     string    `json:"actor"`
	Timestamp time.Time `json:"timestamp"`
	Detail    string    `json:"detail,omitempty"`
	BeadID    string    `json:"bead_id,omitempty"`
}

type WorkerRecord struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	State string `json:"state"`
	// Substate is a non-terminal annotation on a running worker. Currently
	// the only value is "idle" — set when the long-running drain loop has
	// no ready work and is sleeping between polls (ddx-dc157075 AC #5).
	// Cleared as soon as the loop picks up a candidate. Always empty for
	// terminal states (exited/failed/stopped/reaped).
	Substate       string                 `json:"substate,omitempty"`
	Status         string                 `json:"status,omitempty"`
	ProjectRoot    string                 `json:"project_root"`
	Harness        string                 `json:"harness,omitempty"`
	Provider       string                 `json:"provider,omitempty"`
	Model          string                 `json:"model,omitempty"`
	Profile        string                 `json:"profile,omitempty"`
	Effort         string                 `json:"effort,omitempty"`
	Once           bool                   `json:"once,omitempty"`
	PollInterval   string                 `json:"poll_interval,omitempty"`
	StartedAt      time.Time              `json:"started_at,omitempty"`
	FinishedAt     time.Time              `json:"finished_at,omitempty"`
	Error          string                 `json:"error,omitempty"`
	StdoutPath     string                 `json:"stdout_path,omitempty"`
	SpecPath       string                 `json:"spec_path,omitempty"`
	Attempts       int                    `json:"attempts,omitempty"`
	Successes      int                    `json:"successes,omitempty"`
	Failures       int                    `json:"failures,omitempty"`
	CurrentBead    string                 `json:"current_bead,omitempty"`
	LastError      string                 `json:"last_error,omitempty"`
	LastResult     *WorkerExecutionResult `json:"last_result,omitempty"`
	CurrentAttempt *CurrentAttemptInfo    `json:"current_attempt,omitempty"`
	RecentPhases   []PhaseTransition      `json:"recent_phases,omitempty"`
	LastAttempt    *LastAttemptInfo       `json:"last_attempt,omitempty"`
	Lifecycle      []WorkerLifecycleEvent `json:"lifecycle,omitempty"`
	LandSummary    *CoordinatorMetrics    `json:"land_summary,omitempty"`
	// PID is the OS process id of an external worker subprocess, if any.
	// Zero for purely in-process (goroutine-only) workers. Surfaced so the
	// autonomous watchdog can send SIGTERM/SIGKILL to the process group when
	// cancelling the context is not enough.
	PID int `json:"pid,omitempty"`
	// PGID is the OS process-group id of an external worker subprocess, if any.
	// For server-managed workers on Unix this matches PID because the child
	// starts in its own process group. Zero for purely in-process workers.
	PGID int `json:"pgid,omitempty"`
	// ReapReason is populated when the watchdog forcibly terminates a worker;
	// set to "watchdog" today.
	ReapReason string `json:"reap_reason,omitempty"`
	// PIDAlive is a computed field populated by List(); it is never persisted
	// to disk. True when PID > 0 and the process is alive, false when PID > 0
	// but the process has exited. Omitted (nil) when PID == 0 (goroutine-only).
	PIDAlive *bool `json:"pid_alive,omitempty"`
}

type WorkerExecutionResult struct {
	BeadID     string `json:"bead_id,omitempty"`
	AttemptID  string `json:"attempt_id,omitempty"`
	WorkerID   string `json:"worker_id,omitempty"`
	Harness    string `json:"harness,omitempty"`
	PowerClass string `json:"power_class,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Status     string `json:"status,omitempty"`
	Detail     string `json:"detail,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	BaseRev    string `json:"base_rev,omitempty"`
	// ResultRev is the backwards-compat field. After a successful land it mirrors
	// LandedRev. Prefer ImplementationRev and LandedRev for new consumers.
	ResultRev         string `json:"result_rev,omitempty"`
	ImplementationRev string `json:"implementation_rev,omitempty"`
	LandedRev         string `json:"landed_rev,omitempty"`
	EvidenceRev       string `json:"evidence_rev,omitempty"`
	RetryAfter        string `json:"retry_after,omitempty"`
}

type WorkerClaimCleanupReport struct {
	ReleasedClaims  []string `json:"released_claims,omitempty"`
	PreservedClaims []string `json:"preserved_claims,omitempty"`
}

type workerHandle struct {
	record  WorkerRecord
	cancel  context.CancelFunc
	logBuf  *bytes.Buffer
	logFile *os.File
	// progressCh receives ProgressEvents from the execute-bead loop.
	// The WorkerManager drains this channel to update WorkerRecord and
	// broadcast to SSE subscribers.
	progressCh chan agent.ProgressEvent
	// progressSubs holds active SSE subscriber channels for this worker.
	progressSubs []chan agent.ProgressEvent
	// progressDone is closed when drainProgress exits, signalling that
	// no further events will arrive and all new subscriptions should
	// receive an immediately-closed channel.
	progressDone chan struct{}
	// lastPhaseTS is the wall-clock time of the most recent non-heartbeat
	// ProgressEvent. The watchdog uses this to detect stalled attempts.
	lastPhaseTS time.Time
	// reaped is set true once the watchdog has escalated this worker. It is
	// checked under m.mu to make reaping idempotent.
	reaped bool
	// stopped is set true once an operator-driven Stop has started the
	// graceful termination path. Checked under m.mu so a second Stop is a
	// no-op and runWorker can preserve the "stopped" state across its final
	// record write.
	stopped bool
	// managed marks a subprocess-backed worker launched via the server-managed
	// path. The parent manager does not watchdog these workers because the
	// worker loop lives in a separate OS process.
	managed bool
	// cleanupPGIDs stores additional child process groups registered for this
	// worker when the worker itself does not own the full process tree. The
	// slice is protected by m.mu and consumed by the stop/reap cleanup helper.
	cleanupPGIDs []int
	// wakeCh, when non-nil, signals an work worker's idle-poll
	// sleep to return early so the loop re-scans the ready queue. The
	// channel is buffered (cap 1) so a non-blocking send coalesces multiple
	// wake calls into at most one extra tick. Set on work workers;
	// nil on plugin-action workers.
	wakeCh chan struct{}
}

// WorkerManager manages in-process work workers as goroutines.
type WorkerManager struct {
	projectRoot string
	rootDir     string
	// BeadWorkerFactory, when non-nil, is called by runWorker to create the
	// ExecuteBeadWorker instead of building one from the real agent runner.
	// Override in tests to inject a fake executor.
	BeadWorkerFactory func(store agent.ExecuteBeadLoopStore) *agent.ExecuteBeadWorker

	// LandCoordinators is the per-project registry of land coordinators.
	// Exported so tests and server integration tests can stop coordinators
	// on teardown, or inject a custom LandingGitOps via
	// LandCoordinators.gitOpsOverride.
	LandCoordinators *coordinatorRegistry

	// Watchdog parameters. Zero values fall back to defaults:
	//   WatchdogDeadline      = 6h  (total worker runtime budget)
	//   StallDeadline         = 1h  (max phase-transition gap before reap)
	//   WatchdogCheckInterval = 1m  (how often the supervisor sweeps)
	//   WatchdogKillGrace     = 30s (SIGTERM → SIGKILL grace window)
	// Tests override these to run the watchdog on millisecond scales.
	WatchdogDeadline      time.Duration
	StallDeadline         time.Duration
	WatchdogCheckInterval time.Duration
	WatchdogKillGrace     time.Duration

	mu      sync.Mutex
	workers map[string]*workerHandle

	watchdogOnce sync.Once
	watchdogStop chan struct{}

	// managedLaunch switches StartExecuteLoop onto the subprocess-backed
	// server-managed launch path. Tests keep the default in-process behavior;
	// the CLI server enables this when it wants real process-group ownership.
	managedLaunch bool
}

const (
	defaultWatchdogDeadline      = 6 * time.Hour
	defaultStallDeadline         = 1 * time.Hour
	defaultWatchdogCheckInterval = 1 * time.Minute
	defaultWatchdogKillGrace     = 30 * time.Second
)

// WakeProject signals every running work worker bound to projectRoot
// to skip its current idle-poll sleep and re-scan the ready queue. Used by
// the operator-prompt approve / auto-approve mutations (Story 15) so a
// freshly-approved bead is claimed immediately rather than after a full
// PollInterval. The send is non-blocking and the per-worker wake channel is
// buffered (cap 1), so multiple wakes within one tick coalesce. Returns the
// number of workers signalled (0 when no work is running for the
// project — submit/approve still succeeds, the next tick on a future worker
// will pick the bead up).
func (m *WorkerManager) WakeProject(projectRoot string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	signalled := 0
	for _, h := range m.workers {
		if h == nil || h.wakeCh == nil {
			continue
		}
		if h.record.ProjectRoot != projectRoot {
			continue
		}
		select {
		case h.wakeCh <- struct{}{}:
			signalled++
		default:
			// Wake already pending — coalesce.
			signalled++
		}
	}
	return signalled
}

func NewWorkerManager(projectRoot string) *WorkerManager {
	m := &WorkerManager{
		projectRoot:      projectRoot,
		rootDir:          ddxroot.JoinProject(projectRoot, "workers"),
		workers:          map[string]*workerHandle{},
		LandCoordinators: newCoordinatorRegistry(),
		watchdogStop:     make(chan struct{}),
	}
	m.applyServerWatchdogConfig(projectRoot)
	return m
}

func (m *WorkerManager) enableManagedLaunch() {
	if m == nil {
		return
	}
	m.managedLaunch = true
}

func appendUniqueInt(dst []int, value int) []int {
	if value <= 0 {
		return dst
	}
	for _, existing := range dst {
		if existing == value {
			return dst
		}
	}
	return append(dst, value)
}

func (m *WorkerManager) registerManagedWorkerProcessGroup(id string, pgid int) {
	if pgid <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	handle := m.workers[id]
	if handle == nil {
		return
	}
	handle.cleanupPGIDs = appendUniqueInt(handle.cleanupPGIDs, pgid)
}

func lifecycleStartDetail(spec ExecuteLoopWorkerSpec) string {
	parts := []string{"kind=work"}
	if spec.Harness != "" {
		parts = append(parts, "harness="+spec.Harness)
	}
	if spec.Profile != "" {
		parts = append(parts, "profile="+agent.NormalizeRoutingProfile(spec.Profile))
	}
	if spec.Effort != "" {
		parts = append(parts, "effort="+spec.Effort)
	}
	if spec.LabelFilter != "" {
		parts = append(parts, "label_filter="+spec.LabelFilter)
	}
	return strings.Join(parts, " ")
}

func executeLoopOnce(spec ExecuteLoopWorkerSpec) bool {
	return spec.Mode == executeloop.ModeOnce
}

func executeLoopPollInterval(spec ExecuteLoopWorkerSpec) time.Duration {
	if spec.Mode != executeloop.ModeWatch {
		return 0
	}
	return spec.IdleInterval.Duration
}

func executeLoopRouteResolutionTimeout(spec ExecuteLoopWorkerSpec) time.Duration {
	return spec.RouteResolutionTimeout.Duration
}

func executeLoopMaxCostUSD(spec ExecuteLoopWorkerSpec) float64 {
	if spec.MaxCostUSD == 0 {
		return policyescalation.DefaultMaxCostUSD
	}
	return spec.MaxCostUSD
}

func prepareExecuteLoopWorkerSpec(projectRoot string, spec executeloop.ExecuteLoopSpec, defaultMode executeloop.Mode) (ExecuteLoopWorkerSpec, error) {
	spec.ProjectRoot = projectRoot
	// Server-created workers use the same route-neutral request boundary as the
	// CLI: only fields present in this dispatch spec may reach Fizeau.
	spec.OpaquePassthrough = true
	if spec.Mode == "" && defaultMode != "" {
		spec.Mode = defaultMode
	}
	spec.ApplyDefaults()
	if err := spec.Validate(); err != nil {
		return ExecuteLoopWorkerSpec{}, err
	}
	return ExecuteLoopWorkerSpec(spec), nil
}

func resolveManagedWorkerBinaryPath() (string, error) {
	if env := strings.TrimSpace(os.Getenv("DDX_BIN")); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env, nil
		}
		return "", fmt.Errorf("managed worker launch: DDX_BIN %q does not exist", env)
	}

	if exe, err := os.Executable(); err == nil && looksLikeDDXBinary(exe) {
		return exe, nil
	}
	if len(os.Args) > 0 && looksLikeDDXBinary(os.Args[0]) {
		return os.Args[0], nil
	}
	if path, err := exec.LookPath("ddx"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("managed worker launch requires a ddx binary; run make install or set DDX_BIN")
}

func looksLikeDDXBinary(path string) bool {
	if path == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(path))
	return base == "ddx" || base == "ddx.exe"
}

// ManagedWorkerCommandArgs reconstructs the CLI invocation for a persisted
// server-managed worker spec. MinPower presence is preserved so an omitted
// floor does not become an explicit --min-power=0 constraint.
func ManagedWorkerCommandArgs(spec ExecuteLoopWorkerSpec, workerID string) []string {
	args := []string{"work", "--server-managed", workerID}
	if spec.ProjectRoot != "" {
		args = append(args, "--project", spec.ProjectRoot)
	}
	if spec.FromRev != "" {
		args = append(args, "--from", spec.FromRev)
	}
	if spec.Harness != "" {
		args = append(args, "--harness", spec.Harness)
	}
	if spec.Model != "" {
		args = append(args, "--model", spec.Model)
	}
	if spec.Profile != "" {
		args = append(args, "--profile", spec.Profile)
	}
	if spec.Provider != "" {
		args = append(args, "--provider", spec.Provider)
	}
	if spec.LabelFilter != "" {
		args = append(args, "--label-filter", spec.LabelFilter)
	}
	if spec.Effort != "" {
		args = append(args, "--effort", spec.Effort)
	}
	if spec.AttemptBackend != "" {
		args = append(args, "--attempt-backend", spec.AttemptBackend)
	}
	if spec.IgnoreCooldown {
		args = append(args, "--ignore-cooldown")
		if spec.CooldownOverrideReason != "" {
			args = append(args, "--reason", spec.CooldownOverrideReason)
		}
	}
	switch spec.Mode {
	case executeloop.ModeOnce:
		args = append(args, "--once")
	case executeloop.ModeWatch:
		args = append(args, "--watch")
		if spec.IdleInterval.Duration > 0 {
			args = append(args, "--idle-interval", spec.IdleInterval.String())
		}
	}
	if spec.NoReview {
		args = append(args, "--no-review", "--no-review-i-know-what-im-doing")
	}
	if spec.ReviewTier != "" {
		args = append(args, "--review-tier", spec.ReviewTier)
	}
	args = append(args,
		"--max-cost", fmt.Sprintf("%g", spec.MaxCostUSD),
		"--max-bead-cost", fmt.Sprintf("%g", spec.MaxBeadCostUSD),
		"--max-recovery-cost", fmt.Sprintf("%g", spec.MaxRecoveryCostUSD),
		"--preclaim-timeout", spec.PreClaimTimeout.String(),
		"--route-resolution-timeout", spec.RouteResolutionTimeout.String(),
		"--request-timeout", spec.RequestTimeout.String(),
		"--rate-limit-max-wait", spec.RateLimitMaxWait.String(),
		"--max-power", fmt.Sprintf("%d", spec.MaxPower),
	)
	if spec.MinPowerSet || spec.MinPower != 0 {
		args = append(args, "--min-power", fmt.Sprintf("%d", spec.MinPower))
	}
	return args
}

// applyServerWatchdogConfig reads .ddx/config.yaml at projectRoot and applies
// any server.watchdog_deadline / server.stall_deadline overrides. Invalid or
// missing values are silently ignored — defaults are filled in by the
// watchdog loop at runtime.
func (m *WorkerManager) applyServerWatchdogConfig(projectRoot string) {
	cfg, err := config.LoadWithWorkingDir(projectRoot)
	if err != nil || cfg == nil || cfg.Server == nil {
		return
	}
	if d, err := time.ParseDuration(cfg.Server.WatchdogDeadline); err == nil && d > 0 {
		m.WatchdogDeadline = d
	}
	if d, err := time.ParseDuration(cfg.Server.StallDeadline); err == nil && d > 0 {
		m.StallDeadline = d
	}
}

// watchdogDeadlines returns the effective deadlines, applying defaults for
// any zero-valued fields.
func (m *WorkerManager) watchdogDeadlines() (watchdog, stall, check, grace time.Duration) {
	watchdog = m.WatchdogDeadline
	if watchdog <= 0 {
		watchdog = defaultWatchdogDeadline
	}
	stall = m.StallDeadline
	if stall <= 0 {
		stall = defaultStallDeadline
	}
	check = m.WatchdogCheckInterval
	if check <= 0 {
		check = defaultWatchdogCheckInterval
	}
	grace = m.WatchdogKillGrace
	if grace <= 0 {
		grace = defaultWatchdogKillGrace
	}
	return
}

func (m *WorkerManager) StartExecuteLoop(spec ExecuteLoopWorkerSpec) (WorkerRecord, error) {
	// Resolve the effective project root: spec override takes priority over the
	// manager's default so callers can target any registered project.
	effectiveRoot := spec.ProjectRoot
	if effectiveRoot == "" {
		effectiveRoot = m.projectRoot
	}
	spec.ProjectRoot = effectiveRoot
	spec.ApplyDefaults()
	if err := spec.Validate(); err != nil {
		return WorkerRecord{}, err
	}

	if err := os.MkdirAll(m.rootDir, 0o755); err != nil {
		return WorkerRecord{}, err
	}

	id := "worker-" + time.Now().UTC().Format("20060102T150405") + "-" + randomSuffix(4)
	dir := filepath.Join(m.rootDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return WorkerRecord{}, err
	}

	// Write spec
	specData, _ := json.MarshalIndent(spec, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "spec.json"), append(specData, '\n'), 0o644)

	// Open log file
	logPath := filepath.Join(dir, "worker.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return WorkerRecord{}, err
	}

	// Open structured event sink — loop milestones (bead.claimed, bead.result,
	// loop.start/end) land here as JSONL so log aggregators and future server
	// endpoints can parse them independently of the human-readable worker.log.
	eventsPath := filepath.Join(dir, "worker-events.jsonl")
	eventsFile, eventsErr := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if eventsErr != nil {
		eventsFile = nil // non-fatal; structured events silently disabled
	}

	record := WorkerRecord{
		ID:           id,
		Kind:         "work",
		State:        "running",
		Status:       "running",
		ProjectRoot:  effectiveRoot,
		Harness:      spec.Harness,
		Model:        spec.Model,
		Profile:      agent.NormalizeRoutingProfile(spec.Profile),
		Effort:       spec.Effort,
		Once:         executeLoopOnce(spec),
		PollInterval: executeLoopPollInterval(spec).String(),
		StdoutPath:   relToProject(m.projectRoot, logPath),
		SpecPath:     relToProject(m.projectRoot, filepath.Join(dir, "spec.json")),
		StartedAt:    time.Now().UTC(),
	}
	record.Lifecycle = append(record.Lifecycle, WorkerLifecycleEvent{
		Action:    "start",
		Actor:     "local-operator",
		Timestamp: record.StartedAt,
		Detail:    lifecycleStartDetail(spec),
	})
	_ = m.writeRecord(dir, record)

	ctx, cancel := context.WithCancel(context.Background())
	logBuf := &bytes.Buffer{}
	multiLog := io.MultiWriter(logBuf, logFile)

	progressCh := make(chan agent.ProgressEvent, 64)
	wakeCh := make(chan struct{}, 1)
	if m.managedLaunch {
		wakeCh = nil
	}
	handle := &workerHandle{
		record:       record,
		cancel:       cancel,
		logBuf:       logBuf,
		logFile:      logFile,
		progressCh:   progressCh,
		progressDone: make(chan struct{}),
		lastPhaseTS:  time.Now().UTC(),
		wakeCh:       wakeCh,
		managed:      m.managedLaunch,
	}

	m.mu.Lock()
	m.workers[id] = handle
	m.mu.Unlock()

	m.ensureWatchdog()

	if m.managedLaunch {
		managedRecord, err := m.launchManagedExecuteLoop(id, dir, spec, effectiveRoot, handle, multiLog, eventsFile, progressCh)
		if err != nil {
			m.mu.Lock()
			delete(m.workers, id)
			m.mu.Unlock()
			_ = logFile.Close()
			if eventsFile != nil {
				_ = eventsFile.Close()
			}
			_ = os.RemoveAll(dir)
			return WorkerRecord{}, err
		}
		return managedRecord, nil
	}

	go m.drainProgress(id, handle, progressCh)
	go m.runWorker(ctx, id, dir, spec, effectiveRoot, handle, multiLog, eventsFile, progressCh)

	return record, nil
}

func (m *WorkerManager) launchManagedExecuteLoop(id, dir string, spec ExecuteLoopWorkerSpec, projectRoot string, handle *workerHandle, log io.Writer, eventSink io.WriteCloser, progressCh chan agent.ProgressEvent) (WorkerRecord, error) {
	binary, err := resolveManagedWorkerBinaryPath()
	if err != nil {
		return WorkerRecord{}, err
	}

	cmd := exec.Command(binary, ManagedWorkerCommandArgs(spec, id)...)
	cmd.Dir = projectRoot
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = newManagedWorkerSysProcAttr()

	if err := cmd.Start(); err != nil {
		return WorkerRecord{}, err
	}

	m.mu.Lock()
	record := handle.record
	record.PID = cmd.Process.Pid
	record.PGID = cmd.Process.Pid
	handle.managed = true
	handle.record = record
	m.mu.Unlock()
	if err := m.writeRecord(dir, record); err != nil {
		_ = cleanupManagedWorkerProcessTree(cmd.Process.Pid, nil, 0)
		_ = cmd.Wait()
		return WorkerRecord{}, err
	}

	go m.drainProgress(id, handle, progressCh)
	go m.waitManagedWorkerExit(cmd, id, dir, handle, eventSink, progressCh)

	return record, nil
}

func (m *WorkerManager) waitManagedWorkerExit(cmd *exec.Cmd, id, dir string, handle *workerHandle, eventSink io.WriteCloser, progressCh chan agent.ProgressEvent) {
	waitErr := cmd.Wait()
	close(progressCh)
	<-handle.progressDone

	now := time.Now().UTC()
	m.mu.Lock()
	record := handle.record
	preservedState := ""
	if record.State == "stopped" || record.State == "reaped" {
		preservedState = record.State
	}
	if preservedState == "" {
		// A handle-less stop (stopStaleDiskEntry / the supervisor stale sweep)
		// records the terminal label on disk only — the in-memory handle still
		// says "running". Honour it so a SIGKILL-induced wait error does not
		// relabel an operator stop as "failed". Without this the outcome
		// depends on whether the child died inside the 250ms grace.
		if onDisk, derr := m.readRecord(dir); derr == nil &&
			(onDisk.State == "stopped" || onDisk.State == "reaped") {
			preservedState = onDisk.State
		}
	}
	record.FinishedAt = now
	record.Substate = ""
	if eventSink != nil {
		_ = eventSink.Close()
	}
	_ = handle.logFile.Close()
	if waitErr != nil {
		record.State = "failed"
		record.Status = "failed"
		record.Error = waitErr.Error()
		record.LastError = waitErr.Error()
	} else {
		record.State = "exited"
		record.Status = "success"
		// A clean exit is otherwise indistinguishable from a real drain, so
		// consult the structured result the subprocess wrote. Operator-attention
		// stops (e.g. a dirty project root) must classify as restart-blocked so
		// the supervisor parks the worker instead of relaunching it in a tight
		// loop. See ddx-3d57bc30.
		if res, ok := readManagedWorkerResult(dir); ok && res.IsRestartBlocking() {
			record.Status = "operator_attention"
			record.ReapReason = "operator_attention"
		}
	}
	if preservedState != "" {
		record.State = preservedState
		record.Status = preservedState
	}
	handle.record = record
	m.mu.Unlock()

	_ = m.writeRecord(dir, record)
}

func (m *WorkerManager) StartPluginAction(spec PluginActionWorkerSpec, run PluginActionExecutor) (WorkerRecord, error) {
	if run == nil {
		return WorkerRecord{}, fmt.Errorf("plugin action executor is required")
	}

	effectiveRoot := spec.ProjectRoot
	if effectiveRoot == "" {
		effectiveRoot = m.projectRoot
	}

	if err := os.MkdirAll(m.rootDir, 0o755); err != nil {
		return WorkerRecord{}, err
	}

	id := "worker-" + time.Now().UTC().Format("20060102T150405") + "-" + randomSuffix(4)
	dir := filepath.Join(m.rootDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return WorkerRecord{}, err
	}

	spec.ProjectRoot = effectiveRoot
	specData, _ := json.MarshalIndent(spec, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "spec.json"), append(specData, '\n'), 0o644)

	logPath := filepath.Join(dir, "worker.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return WorkerRecord{}, err
	}

	startedAt := time.Now().UTC()
	record := WorkerRecord{
		ID:          id,
		Kind:        "plugin-dispatch",
		State:       "running",
		Status:      "running",
		ProjectRoot: effectiveRoot,
		StdoutPath:  relToProject(m.projectRoot, logPath),
		SpecPath:    relToProject(m.projectRoot, filepath.Join(dir, "spec.json")),
		StartedAt:   startedAt,
		Lifecycle: []WorkerLifecycleEvent{{
			Action:    "start",
			Actor:     "local-operator",
			Timestamp: startedAt,
			Detail:    fmt.Sprintf("%s plugin %s (%s)", spec.Action, spec.Name, spec.Scope),
		}},
	}
	_ = m.writeRecord(dir, record)

	ctx, cancel := context.WithCancel(context.Background())
	logBuf := &bytes.Buffer{}
	multiLog := io.MultiWriter(logBuf, logFile)
	progressCh := make(chan agent.ProgressEvent, 16)
	handle := &workerHandle{
		record:       record,
		cancel:       cancel,
		logBuf:       logBuf,
		logFile:      logFile,
		progressCh:   progressCh,
		progressDone: make(chan struct{}),
		lastPhaseTS:  startedAt,
	}

	m.mu.Lock()
	m.workers[id] = handle
	m.mu.Unlock()

	m.ensureWatchdog()

	go m.drainProgress(id, handle, progressCh)
	go m.runPluginAction(ctx, id, dir, spec, handle, multiLog, progressCh, run)

	return record, nil
}

func (m *WorkerManager) runPluginAction(ctx context.Context, id, dir string, spec PluginActionWorkerSpec, handle *workerHandle, log io.Writer, progressCh chan agent.ProgressEvent, run PluginActionExecutor) {
	startedAt := time.Now().UTC()
	phaseSeq := 1
	sendProgress(progressCh, agent.ProgressEvent{
		EventID:   "evt-" + randomSuffix(8),
		AttemptID: id,
		WorkerID:  id,
		ProjectID: spec.ProjectRoot,
		Phase:     "running",
		PhaseSeq:  phaseSeq,
		TS:        startedAt,
		Message:   fmt.Sprintf("%s plugin %s", spec.Action, spec.Name),
	})
	if log != nil {
		_, _ = fmt.Fprintf(log, "%s plugin %s (%s)\n", spec.Action, spec.Name, spec.Scope)
	}

	state, err := run(ctx)
	if ctxErr := ctx.Err(); ctxErr != nil && err == nil {
		err = ctxErr
	}

	phase := "done"
	message := state
	if err != nil {
		phase = "failed"
		message = err.Error()
	}
	phaseSeq++
	sendProgress(progressCh, agent.ProgressEvent{
		EventID:   "evt-" + randomSuffix(8),
		AttemptID: id,
		WorkerID:  id,
		ProjectID: spec.ProjectRoot,
		Phase:     phase,
		PhaseSeq:  phaseSeq,
		TS:        time.Now().UTC(),
		ElapsedMS: time.Since(startedAt).Milliseconds(),
		Message:   message,
	})

	if log != nil {
		if err != nil {
			_, _ = fmt.Fprintf(log, "failed: %s\n", err)
		} else {
			_, _ = fmt.Fprintf(log, "completed: %s\n", state)
		}
	}

	close(progressCh)
	<-handle.progressDone

	m.mu.Lock()
	record := handle.record
	preservedState := ""
	if record.State == "stopped" || record.State == "reaped" {
		preservedState = record.State
	}
	record.FinishedAt = time.Now().UTC()
	_ = handle.logFile.Close()
	if err != nil {
		record.State = "failed"
		record.Status = "failed"
		record.Error = err.Error()
		record.LastError = err.Error()
	} else {
		record.State = "exited"
		record.Status = "success"
		record.LastResult = &WorkerExecutionResult{
			AttemptID:  id,
			WorkerID:   id,
			PowerClass: "",
			Status:     state,
			Detail:     fmt.Sprintf("%s plugin %s", spec.Action, spec.Name),
		}
	}
	if preservedState != "" {
		record.State = preservedState
		record.Status = preservedState
	}
	_ = m.writeRecord(dir, record)
	handle.record = record
	m.mu.Unlock()
}

func sendProgress(ch chan<- agent.ProgressEvent, evt agent.ProgressEvent) {
	select {
	case ch <- evt:
	default:
	}
}

func isBudgetExhaustedFailure(report agent.ExecuteBeadReport) bool {
	return strings.Contains(report.Detail, agent.RateLimitBudgetExhaustedReason)
}

func applyServerExecutionRoutingIntent(report *agent.ExecuteBeadReport, intent policyescalation.ExecutionHint, requestedPolicy string, requestedMinPower, requestedMaxPower int) {
	if report == nil {
		return
	}
	report.RoutingIntentSource = string(intent.Source)
	report.EstimatedDifficulty = string(intent.EstimatedDifficulty)
	report.InferredMinPower = intent.InferredMinPower
	report.InferredMinPowerPresent = intent.HasInferredMinPower
	report.RequestedPolicy = requestedPolicy
	report.RequestedMinPower = requestedMinPower
	report.RequestedMaxPower = requestedMaxPower
}

func runEscalatingPowerAttempts(
	ctx context.Context,
	initialMinPower int,
	maxPower int,
	attempt func(context.Context, int) (agent.ExecuteBeadReport, error),
	recordAttempt func(agent.ExecuteBeadReport),
	allowInfrastructureRetry bool,
) (agent.ExecuteBeadReport, error) {
	minPower := initialMinPower
	for {
		report, err := attempt(ctx, minPower)
		if recordAttempt != nil && report.BeadID != "" {
			recordAttempt(report)
		}
		if err != nil {
			return report, err
		}
		transition := executeloop.DecideAttemptTransition(executeloop.AttemptTransitionInput{
			Status:                   report.Status,
			Detail:                   report.Detail,
			CurrentMinPower:          minPower,
			MaxPower:                 maxPower,
			ActualPower:              report.ActualPower,
			Disrupted:                report.Disrupted,
			BudgetExhausted:          isBudgetExhaustedFailure(report),
			AllowInfrastructureRetry: allowInfrastructureRetry,
		})
		if transition.Action != executeloop.TryLoopActionRetryPower {
			return report, nil
		}
		minPower = transition.NextMinPower
	}
}

func appendPowerAttemptEvent(store agent.BeadEventAppender, beadID string, report agent.ExecuteBeadReport, actor string, createdAt time.Time) {
	if store == nil || beadID == "" {
		return
	}
	body := policyescalation.FormatPowerAttemptBody(report.PowerClass, report.Harness, report.Model, report.ProbeResult, report.Detail)
	summary := report.Status
	if report.PowerClass != "" {
		summary = fmt.Sprintf("%s powerClass=%s", summary, report.PowerClass)
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "power-attempt",
		Summary:   summary,
		Body:      body,
		Actor:     actor,
		Source:    "legacy agent work",
		CreatedAt: createdAt,
	})
}

func (m *WorkerManager) runWorker(ctx context.Context, id, dir string, spec ExecuteLoopWorkerSpec, projectRoot string, handle *workerHandle, log io.Writer, eventSink io.WriteCloser, progressCh chan agent.ProgressEvent) {
	if eventSink != nil {
		defer eventSink.Close() //nolint:errcheck
	}
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	overrides := config.CLIOverrides{
		Assignee:          "ddx",
		Harness:           spec.Harness,
		Model:             spec.Model,
		Provider:          spec.Provider,
		Profile:           spec.Profile,
		Effort:            spec.Effort,
		MinPower:          spec.MinPower,
		MaxPower:          spec.MaxPower,
		OpaquePassthrough: true,
	}
	requestTimeout := spec.RequestTimeout.Duration
	if requestTimeout > 0 {
		overrides.ProviderRequestTimeout = &requestTimeout
	}
	rcfg, _ := config.LoadAndResolve(projectRoot, overrides)

	var lintHook func(ctx context.Context, beadID string) (agent.LintResult, error)
	var intakeHook agent.PreClaimIntakeHook
	var triageHook func(ctx context.Context, beadID string, report agent.ExecuteBeadReport) (agent.TriageResult, error)
	if m.BeadWorkerFactory == nil {
		var qualityRunner agent.AgentRunner
		lintHook = agent.NewPreDispatchLintHook(projectRoot, store, rcfg, nil, qualityRunner)
		intakeHook = agent.NewPreClaimIntakeHookWithLog(projectRoot, store, rcfg, nil, qualityRunner, log)
		intakeHook = agent.NewACQualityPreClaimGate(store, rcfg.BeadQualityMode(), rcfg.ACQualityMinScore(), intakeHook)
		triageHook = agent.NewPostAttemptTriageHook(projectRoot, store, rcfg, nil, qualityRunner, nil)
	}

	var worker *agent.ExecuteBeadWorker
	var costCap *policyescalation.CostCapTracker
	if m.BeadWorkerFactory != nil {
		worker = m.BeadWorkerFactory(store)
	} else {
		// Build an executor that calls agent.ExecuteBead in-process, then
		// submits the result to the project's land coordinator. The
		// coordinator (a single goroutine per projectRoot) serializes all
		// target-ref writes for this project — this is the server-side
		// implementation of the human-PR landing model. See ddx-8746d8a6
		// for the rationale. Prior to this rewrite, runWorker never called
		// LandBeadResult at all, so commits produced by server-managed
		// workers were silently lost (ddx-e14efc58).
		coordinator := m.LandCoordinators.Get(projectRoot)

		// singlePolicyAttempt runs one execution at a specific harness/model.
		// The caller controls MinPower per rung; this helper keeps the
		// profile stable and only advances the power floor.
		var reviewer agent.CandidateReviewer
		var postMergeReviewer agent.BeadReviewer
		singlePolicyAttempt := func(ctx context.Context, beadID string, requestedMinPower int, resolvedHarness, resolvedProvider, resolvedModel string) (agent.ExecuteBeadReport, error) {
			gitOps := &agent.RealGitOps{}
			attemptProvider := spec.Provider
			if resolvedProvider != "" {
				attemptProvider = resolvedProvider
			}
			loopOverrides := config.CLIOverrides{
				Harness:           resolvedHarness,
				Model:             resolvedModel,
				Provider:          attemptProvider,
				Profile:           spec.Profile,
				Effort:            spec.Effort,
				MinPower:          requestedMinPower,
				MaxPower:          spec.MaxPower,
				OpaquePassthrough: true,
			}
			if requestTimeout > 0 {
				loopOverrides.ProviderRequestTimeout = &requestTimeout
			}
			attemptRcfg, _ := config.LoadAndResolve(projectRoot, loopOverrides)
			beadStore := bead.NewStore(ddxroot.JoinProject(projectRoot))
			res, err := agent.ExecuteBeadWithConfig(ctx, projectRoot, beadID, attemptRcfg, agent.ExecuteBeadRuntime{
				FromRev:          spec.FromRev,
				BeadEvents:       beadStore,
				BeadCancel:       beadStore,
				Reviewer:         reviewer,
				NoReview:         spec.NoReview,
				RateLimitMaxWait: spec.RateLimitMaxWait.Duration,
			}, gitOps)
			if err != nil && res == nil {
				return agent.ExecuteBeadReport{}, err
			}
			// Verify local evidence before landing. A staged/tracked evidence
			// violation must stop here, before the coordinator can mutate history.
			if res != nil && res.AttemptID != "" {
				if candidateErr := agent.VerifyCandidateHasNoExecutionEvidence(projectRoot, res.BaseRev, res.ResultRev); candidateErr != nil {
					return agent.ExecuteBeadReport{}, errors.Join(err, fmt.Errorf("validating local execution evidence boundary: %w", candidateErr))
				}
				if retentionErr := agent.VerifyCleanWorktree(projectRoot, res.AttemptID); retentionErr != nil {
					return agent.ExecuteBeadReport{}, errors.Join(err, fmt.Errorf("retaining local execution evidence: %w", retentionErr))
				}
			}
			// Preserve operator-cancel results as-is; the worker has already
			// classified them (preserved / operator_cancel / preserved_needs_review)
			// and overriding here would lose the cancel signal.
			operatorCancel := res != nil && res.Reason == agent.OperatorCancelReason
			if err == nil && !operatorCancel && agent.PrepareCandidateCycleLanding(res) {
				if landErr := evaluateGatesAndSubmit(projectRoot, res, nil, gitOps, coordinator, landSafetyConfigFromConfig(projectRoot), log); landErr != nil {
					err = landErr
				}
			} else if res != nil && res.ResultRev == res.BaseRev && !operatorCancel {
				res.Outcome = "no-changes"
				res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
			} else if res != nil && res.ExitCode != 0 && !operatorCancel {
				res.Outcome = "preserved"
				res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
			}
			if err != nil {
				return agent.ExecuteBeadReport{}, err
			}
			return agent.ExecuteBeadReport{
				BeadID:             res.BeadID,
				AttemptID:          res.AttemptID,
				WorkerID:           res.WorkerID,
				Harness:            res.Harness,
				Provider:           res.Provider,
				Model:              res.Model,
				ActualPower:        res.ActualPower,
				Status:             res.Status,
				Detail:             res.Detail,
				SessionID:          res.SessionID,
				BaseRev:            res.BaseRev,
				ResultRev:          res.ResultRev,
				ImplementationRev:  res.ImplementationRev,
				LandedRev:          res.LandedRev,
				EvidenceRev:        res.EvidenceRev,
				PreserveRef:        res.PreserveRef,
				NoChangesRationale: res.NoChangesRationale,
				CostUSD:            res.CostUSD,
				DurationMS:         int64(res.DurationMS),
			}, nil
		}

		// Cost-cap state for this worker run consumes the cost Fizeau reports.
		// DDx does not inspect harness inventory or infer billing properties.
		maxCostUSD := executeLoopMaxCostUSD(spec)
		costCap = policyescalation.NewCostCapTracker(maxCostUSD, nil)
		accumulateBilledCost := func(report agent.ExecuteBeadReport) {
			costCap.Add(report.Harness, report.CostUSD)
		}
		costCapTripped := func() (agent.ExecuteBeadReport, bool) {
			if _, tripped := costCap.Tripped(); !tripped {
				return agent.ExecuteBeadReport{}, false
			}
			spent := costCap.Spent()
			return agent.ExecuteBeadReport{
				Status: agent.ExecuteBeadStatusExecutionFailed,
				Detail: fmt.Sprintf("cost cap reached: $%.2f reported >= $%.2f cap; raise the cap or set 0 to disable.", spent, maxCostUSD),
			}, true
		}
		attemptWithCostCap := func(ctx context.Context, beadID string, requestedMinPower int, intent policyescalation.ExecutionHint) (agent.ExecuteBeadReport, error) {
			if cappedReport, capped := costCapTripped(); capped {
				cappedReport.BeadID = beadID
				applyServerExecutionRoutingIntent(&cappedReport, intent, spec.Profile, requestedMinPower, spec.MaxPower)
				return cappedReport, nil
			}
			report, err := singlePolicyAttempt(ctx, beadID, requestedMinPower, spec.Harness, spec.Provider, spec.Model)
			applyServerExecutionRoutingIntent(&report, intent, spec.Profile, requestedMinPower, spec.MaxPower)
			if err == nil {
				accumulateBilledCost(report)
				if cappedReport, capped := costCapTripped(); capped {
					cappedReport.BeadID = beadID
					applyServerExecutionRoutingIntent(&cappedReport, intent, spec.Profile, requestedMinPower, spec.MaxPower)
					return cappedReport, nil
				}
			}
			return report, err
		}

		executor := agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			target, err := store.Get(ctx, beadID)
			if err != nil {
				return agent.ExecuteBeadReport{}, err
			}
			intent := policyescalation.ResolveExecutionHint(policyescalation.ExecutionHintInput{
				Bead:                         target,
				ReadinessEstimatedDifficulty: agent.ReadinessEstimatedDifficultyFromContext(ctx),
				ExplicitMinPower:             spec.MinPowerSet || spec.MinPower != 0,
				PublicPolicy:                 spec.Profile,
			})
			initialMinPower := rcfg.MinPower()
			if intent.HasInferredMinPower {
				initialMinPower = intent.InferredMinPower
			}
			if spec.MaxPower > 0 && initialMinPower > spec.MaxPower {
				return agent.ExecuteBeadReport{}, fmt.Errorf("inferred MinPower %d conflicts with requested MaxPower %d", initialMinPower, spec.MaxPower)
			}
			attempts := make([]policyescalation.PowerAttemptRecord, 0, 3)
			recordAttempt := func(report agent.ExecuteBeadReport) {
				if report.PowerClass == "" && report.Harness == "" && report.Model == "" && report.ProbeResult == "" && report.CostUSD == 0 && report.DurationMS == 0 {
					return
				}
				attempts = append(attempts, policyescalation.PowerAttemptRecord{
					PowerClass: report.PowerClass,
					Harness:    report.Harness,
					Model:      report.Model,
					Status:     report.Status,
					CostUSD:    report.CostUSD,
					DurationMS: report.DurationMS,
				})
				appendPowerAttemptEvent(store, beadID, report, "ddx", time.Now().UTC())
			}
			report, err := runEscalatingPowerAttempts(ctx, initialMinPower, rcfg.MaxPower(), func(ctx context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
				return attemptWithCostCap(ctx, beadID, requestedMinPower, intent)
			}, recordAttempt, strings.TrimSpace(spec.Harness) == "" && strings.TrimSpace(spec.Provider) == "" && strings.TrimSpace(spec.Model) == "")
			if len(attempts) > 0 {
				winningPowerClass := ""
				if report.Status == agent.ExecuteBeadStatusSuccess && report.PowerClass != "" {
					winningPowerClass = report.PowerClass
				}
				_ = policyescalation.AppendEscalationSummaryEvent(store, beadID, "ddx", attempts, winningPowerClass, time.Now().UTC())
			}
			return report, err
		})

		// Build post-merge reviewer. On-by-default unless NoReview is set in spec.
		if !spec.NoReview {
			postMergeReviewer = newServerReviewer(projectRoot, ddxroot.JoinProject(projectRoot), spec.ReviewTier, rcfg)
			reviewer = postMergeReviewer.(agent.CandidateReviewer)
		}

		worker = &agent.ExecuteBeadWorker{
			Store:    store,
			Executor: executor,
			Reviewer: postMergeReviewer,
		}
	}

	landingOps := agent.RealLandingGitOps{}

	loopResult, err := worker.Run(ctx, rcfg, agent.ExecuteBeadLoopRuntime{
		Mode:                   spec.Mode,
		IdleInterval:           executeLoopPollInterval(spec),
		Log:                    log,
		CleanupLog:             log,
		EventSink:              eventSink,
		WorkerID:               id,
		ProjectRoot:            projectRoot,
		CleanupRunner:          agent.NewExecutionCleanupManager(projectRoot, &agent.RealGitOps{}),
		LabelFilter:            spec.LabelFilter,
		ProgressCh:             progressCh,
		PreClaimHook:           buildPreClaimHook(projectRoot, landingOps),
		PreClaimIntakeHook:     intakeHook,
		RouteResolutionTimeout: executeLoopRouteResolutionTimeout(spec),
		LoadPressureThreshold:  rcfg.LoadPressureThreshold(),
		LoadPressureSnapshot: func() workerstatus.LoadPressureSnapshot {
			return workerstatus.HostLoadPressureSnapshot(rcfg.LoadPressureThreshold())
		},
		PreDispatchLintHook:   lintHook,
		PostAttemptTriageHook: triageHook,
		NoReview:              spec.NoReview,
		ReviewCostCap:         costCap,
		WakeCh:                handle.wakeCh,
	})
	// Signal end of progress events so drainProgress can finish
	close(progressCh)
	// Wait for drainProgress to process all remaining events (including live
	// counter increments) before we overwrite handle.record with the final state.
	<-handle.progressDone

	m.mu.Lock()
	record := handle.record
	// Preserve terminal state set by Stop() or the watchdog so the final
	// writeRecord below does not overwrite "stopped" / "reaped" with
	// "exited" / "failed".
	preservedState := ""
	if record.State == "stopped" || record.State == "reaped" {
		preservedState = record.State
	}
	record.FinishedAt = time.Now().UTC()
	// Terminal worker: clear any "idle" substate (ddx-dc157075 AC #5; substate
	// is only meaningful while the worker is running).
	record.Substate = ""
	_ = handle.logFile.Close()

	if err != nil {
		record.State = "failed"
		record.Status = "failed"
		record.Error = err.Error()
		record.LastError = err.Error()
	} else {
		record.State = "exited"
		record.Attempts = loopResult.Attempts
		record.Successes = loopResult.Successes
		record.Failures = loopResult.Failures

		if loopResult.NoReadyWork {
			record.Status = "no_ready_work"
		} else if loopResult.Failures > 0 && loopResult.Successes == 0 {
			record.Status = "execution_failed"
			if loopResult.LastFailureStatus != "" {
				record.Status = loopResult.LastFailureStatus
			}
		} else if loopResult.Successes > 0 {
			record.Status = "success"
		} else {
			record.Status = "exited"
		}

		if len(loopResult.Results) > 0 {
			last := loopResult.Results[len(loopResult.Results)-1]
			r := WorkerExecutionResult{
				BeadID:            last.BeadID,
				AttemptID:         last.AttemptID,
				WorkerID:          last.WorkerID,
				Harness:           last.Harness,
				PowerClass:        last.PowerClass,
				Provider:          last.Provider,
				Model:             last.Model,
				Status:            last.Status,
				Detail:            last.Detail,
				SessionID:         last.SessionID,
				BaseRev:           last.BaseRev,
				ResultRev:         last.ResultRev,
				ImplementationRev: last.ImplementationRev,
				LandedRev:         last.LandedRev,
				EvidenceRev:       last.EvidenceRev,
				RetryAfter:        last.RetryAfter,
			}
			record.CurrentBead = last.BeadID
			record.LastResult = &r
			if last.Detail != "" {
				record.LastError = last.Detail
			}
			if last.Harness != "" && record.Harness == "" {
				record.Harness = last.Harness
			}
			if last.Model != "" && record.Model == "" {
				record.Model = last.Model
			}
			if last.Provider != "" && record.Provider == "" {
				record.Provider = last.Provider
			}
		}
	}
	// Terminal-state override: if Stop() or the watchdog already marked
	// this worker, keep that label so external consumers see the reason.
	if preservedState != "" {
		record.State = preservedState
		record.Status = preservedState
	}
	_ = m.writeRecord(dir, record)
	handle.record = record
	m.mu.Unlock()
}

func newServerReviewer(projectRoot, beadStoreRoot, reviewTier string, rcfg config.ResolvedConfig) *agent.DefaultBeadReviewer {
	return &agent.DefaultBeadReviewer{
		ProjectRoot:           projectRoot,
		BeadStore:             bead.NewStore(beadStoreRoot),
		BeadEvents:            bead.NewStore(beadStoreRoot),
		ReviewTier:            reviewTier,
		PrimaryConfigSnapshot: &rcfg,
		Caps:                  rcfg.EvidenceCapsForRole(config.EvidenceRoleReviewer),
		CapsConfigured:        true,
	}
}

func (m *WorkerManager) List() ([]WorkerRecord, error) {
	if err := os.MkdirAll(m.rootDir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return nil, err
	}
	var out []WorkerRecord
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rec, err := m.readRecord(filepath.Join(m.rootDir, entry.Name()))
		if err != nil {
			continue
		}
		if !isWorkerRegistryRecord(rec) {
			continue
		}
		out = append(out, rec)
	}

	// Merge in-memory state for active workers
	m.mu.Lock()
	for i := range out {
		if handle, ok := m.workers[out[i].ID]; ok {
			out[i] = handle.record
		}
	}
	m.mu.Unlock()

	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})

	// Compute PIDAlive for each non-terminal record so operators can see
	// liveness at a glance without running prune. Terminal records deliberately
	// omit it: their stored PIDs are historical, and the OS may have reused the
	// PID for an unrelated process by the time an operator lists workers.
	for i := range out {
		if out[i].PID > 0 && !isTerminalWorkerState(out[i].State) {
			alive := isPIDAlive(out[i].PID)
			out[i].PIDAlive = &alive
		}
	}

	return out, nil
}

func isWorkerRegistryRecord(rec WorkerRecord) bool {
	return rec.ID != "" || rec.Kind != "" || rec.State != ""
}

func isTerminalWorkerState(state string) bool {
	switch state {
	case "exited", "failed", "stopped", "reaped":
		return true
	default:
		return false
	}
}

func (m *WorkerManager) Show(id string) (WorkerRecord, error) {
	m.mu.Lock()
	if handle, ok := m.workers[id]; ok {
		rec := handle.record
		m.mu.Unlock()
		return rec, nil
	}
	m.mu.Unlock()
	return m.readRecord(filepath.Join(m.rootDir, id))
}

// Stop performs a graceful termination of the worker:
//  1. Mark state=stopping and persist so observers see the transition.
//  2. Emit bead.stopped event + release the bead claim (if one is held).
//  3. Send SIGTERM to the worker's process group and any registered child
//     process groups; escalate to SIGKILL after WatchdogKillGrace if
//     anything remains alive. Pure-goroutine workers have no PID — ctx
//     cancellation below is the only lever.
//  4. Cancel the worker's context so the loop and in-flight executor exit.
//  5. Mark state=stopped and persist. runWorker preserves this terminal
//     state when it writes its final record.
//
// Stop is idempotent: a second call is a no-op. It returns an error only
// when the worker is unknown to the manager (already exited / never existed).
// stopStaleDiskEntry handles the case where a worker's disk record shows
// state=running but no live goroutine exists (e.g. after a server restart).
// It releases any held bead claim and flips the on-disk state to "stopped",
// except when a live same-machine PID is backed by a fresh claim lease or
// heartbeat — that process and claim are preserved (regression ddx-8c8675ba).
func (m *WorkerManager) stopStaleDiskEntry(id string) error {
	dir := filepath.Join(m.rootDir, id)
	rec, err := m.readRecord(dir)
	if err != nil {
		return fmt.Errorf("worker not running")
	}
	// Terminal state — nothing left to clean up.
	if rec.State != "running" && rec.State != "stopping" {
		return nil
	}

	now := time.Now().UTC()
	beadID := ""
	if rec.CurrentAttempt != nil {
		beadID = rec.CurrentAttempt.BeadID
	}
	if beadID == "" {
		beadID = rec.CurrentBead
	}
	projectRoot := rec.ProjectRoot
	if projectRoot == "" {
		projectRoot = m.projectRoot
	}

	// Classify reclaimability before any signal or state rewrite so a
	// mid-attempt worker with a fresh lease cannot be killed first and
	// only then "protected" by the claim check (incident: managed-worker
	// mid-attempt stop).
	canReleaseClaim := true
	var store *bead.Store
	if beadID != "" {
		store = bead.NewStore(ddxroot.JoinProject(projectRoot))
		canReleaseClaim = staleDiskEntryCanReleaseClaim(store, beadID)
	}
	pidAlive := rec.PID > 0 && processAlive(rec.PID)
	if pidAlive && !canReleaseClaim {
		reason := "fresh-claim-or-heartbeat"
		detail := fmt.Sprintf(
			"reason=%s worker=%s pid=%d bead=%s",
			reason, id, rec.PID, beadID,
		)
		log.Printf("worker supervisor: preserve live disk-record worker %s", detail)
		rec.Lifecycle = append(rec.Lifecycle, WorkerLifecycleEvent{
			Action:    "preserve",
			Actor:     "server-workers",
			Timestamp: now,
			Detail:    detail,
			BeadID:    beadID,
		})
		// Leave state running/stopping; do not kill the process or unclaim.
		return m.writeRecord(dir, rec)
	}

	if pidAlive {
		cleanupManagedWorkerProcessTree(rec.PID, nil, 250*time.Millisecond)
	}

	if beadID != "" && canReleaseClaim && store != nil {
		body := fmt.Sprintf(
			"worker=%s pid=%d reason=stop-stale",
			id, rec.PID,
		)
		_ = store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "bead.stopped",
			Summary:   "stop (stale)",
			Body:      body,
			Actor:     "ddx",
			Source:    "server-workers",
			CreatedAt: now,
		})
		_ = store.Unclaim(beadID)
	}

	rec.State = "stopped"
	rec.Status = "stopped"
	if rec.FinishedAt.IsZero() {
		rec.FinishedAt = now
	}
	rec.Lifecycle = append(rec.Lifecycle, WorkerLifecycleEvent{
		Action:    "stop",
		Actor:     "local-operator",
		Timestamp: now,
		Detail:    fmt.Sprintf("reason=stop-stale pid=%d", rec.PID),
		BeadID:    beadID,
	})
	return m.writeRecord(dir, rec)
}

func (m *WorkerManager) UnjamStaleClaims(ctx context.Context) (WorkerClaimCleanupReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	report := WorkerClaimCleanupReport{
		ReleasedClaims:  []string{},
		PreservedClaims: []string{},
	}
	seen := map[string]struct{}{}

	entries, err := os.ReadDir(m.rootDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		dir := filepath.Join(m.rootDir, id)
		rec, err := m.readRecord(dir)
		if err != nil {
			continue
		}
		if rec.State != "running" && rec.State != "stopping" {
			continue
		}
		beadID := ""
		if rec.CurrentAttempt != nil {
			beadID = rec.CurrentAttempt.BeadID
		}
		if beadID == "" {
			beadID = rec.CurrentBead
		}
		if beadID == "" {
			continue
		}
		if _, ok := seen[beadID]; ok {
			continue
		}
		seen[beadID] = struct{}{}

		projectRoot := rec.ProjectRoot
		if projectRoot == "" {
			projectRoot = m.projectRoot
		}
		store := bead.NewStore(ddxroot.JoinProject(projectRoot))
		if staleDiskEntryCanReleaseClaim(store, beadID) {
			if err := m.stopStaleDiskEntry(id); err != nil {
				return report, err
			}
			report.ReleasedClaims = append(report.ReleasedClaims, beadID)
			continue
		}
		report.PreservedClaims = append(report.PreservedClaims, beadID)
	}

	claimRoot := bead.ClaimLivenessRoot(ddxroot.JoinProject(m.projectRoot))
	claimEntries, err := os.ReadDir(claimRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	for _, entry := range claimEntries {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		beadID := strings.TrimSuffix(entry.Name(), ".json")
		if _, ok := seen[beadID]; ok {
			continue
		}
		seen[beadID] = struct{}{}
		store := bead.NewStore(ddxroot.JoinProject(m.projectRoot))
		if staleDiskEntryCanReleaseClaim(store, beadID) {
			if err := store.Unclaim(beadID); err != nil {
				return report, err
			}
			now := time.Now().UTC()
			_ = store.AppendEvent(beadID, bead.BeadEvent{
				Kind:      "bead.stopped",
				Summary:   "stop (unjam)",
				Body:      "reason=doctor-unjam",
				Actor:     "ddx",
				Source:    "doctor-unjam",
				CreatedAt: now,
			})
			report.ReleasedClaims = append(report.ReleasedClaims, beadID)
			continue
		}
		report.PreservedClaims = append(report.PreservedClaims, beadID)
	}

	return report, nil
}

// staleDiskEntryCanReleaseClaim reports whether stopStaleDiskEntry may safely
// release the bead claim for beadID. Fresh leases are preserved; when the
// heartbeat sidecar is absent or stale, the stale record cleanup can reclaim.
func staleDiskEntryCanReleaseClaim(store *bead.Store, beadID string) bool {
	lease, found, err := store.ClaimLease(beadID)
	if err != nil {
		return false
	}
	if !found {
		return true
	}
	if lease.UpdatedAt.IsZero() {
		return false
	}

	age := time.Since(lease.UpdatedAt)
	if age <= bead.HeartbeatTTL {
		return false
	}

	machine := os.Getenv("DDX_MACHINE_ID")
	if machine == "" {
		machine, _ = os.Hostname()
	}
	if lease.Machine != "" && lease.Machine == machine && lease.PID > 0 && processAlive(lease.PID) {
		return age > 2*time.Hour
	}

	return true
}

func (m *WorkerManager) Stop(id string) error {
	m.mu.Lock()
	handle := m.workers[id]
	if handle == nil || handle.cancel == nil {
		m.mu.Unlock()
		// Not in-memory: try to clean up a stale disk record left by a
		// previous server run or an abruptly killed goroutine.
		return m.stopStaleDiskEntry(id)
	}
	if handle.stopped {
		m.mu.Unlock()
		return nil
	}
	handle.stopped = true

	now := time.Now().UTC()
	projectRoot := handle.record.ProjectRoot
	if projectRoot == "" {
		projectRoot = m.projectRoot
	}
	pid := handle.record.PID
	beadID := ""
	if handle.record.CurrentAttempt != nil {
		beadID = handle.record.CurrentAttempt.BeadID
	}
	if beadID == "" {
		beadID = handle.record.CurrentBead
	}
	startedAt := handle.record.StartedAt
	cleanupPGIDs := append([]int(nil), handle.cleanupPGIDs...)
	handle.record.State = "stopping"
	handle.record.Status = "stopping"
	handle.record.Lifecycle = append(handle.record.Lifecycle, WorkerLifecycleEvent{
		Action:    "stop",
		Actor:     "local-operator",
		Timestamp: now,
		Detail:    fmt.Sprintf("reason=stop pid=%d", pid),
		BeadID:    beadID,
	})
	dir := filepath.Join(m.rootDir, id)
	transitionSnapshot := handle.record
	cancel := handle.cancel
	m.mu.Unlock()

	_ = m.writeRecord(dir, transitionSnapshot)

	// Release the bead claim first — this is durable and must not be
	// leaked even if the SIGKILL path blocks for the full grace window.
	if beadID != "" {
		store := bead.NewStore(ddxroot.JoinProject(projectRoot))
		runtime := time.Duration(0)
		if !startedAt.IsZero() {
			runtime = now.Sub(startedAt)
		}
		body := fmt.Sprintf(
			"worker=%s runtime=%s pid=%d reason=stop",
			id, runtime.Round(time.Second), pid,
		)
		_ = store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "bead.stopped",
			Summary:   "stop",
			Body:      body,
			Actor:     "ddx",
			Source:    "server-workers",
			CreatedAt: now,
		})
		_ = store.Unclaim(beadID)
	}

	// Escalate to the worker process tree if we know any server-owned
	// process groups. This keeps operator Stop scoped to the worker while
	// still reaching provider shells and their descendants.
	_, _, _, grace := m.watchdogDeadlines()
	cleanupReport := cleanupManagedWorkerProcessTree(pid, cleanupPGIDs, grace)

	// Cancel the worker goroutine so any in-process code sees context.Canceled.
	cancel()

	// Flip in-memory state to the terminal "stopped" label. For real
	// workers, runWorker's final writeRecord (with preservedState) will
	// persist this to disk — we deliberately do not writeRecord here a
	// second time because runWorker may still be mid-finalization and a
	// double-write races the test cleanup. Idle handles (no runWorker)
	// have their state observable in-memory; callers that need disk
	// persistence for those can call writeRecord directly.
	m.mu.Lock()
	if cleanupSummary := cleanupReport.String(); cleanupSummary != "" {
		if len(handle.record.Lifecycle) > 0 {
			handle.record.Lifecycle[len(handle.record.Lifecycle)-1].Detail =
				fmt.Sprintf("reason=stop pid=%d cleanup=%s", pid, cleanupSummary)
		}
	}
	handle.record.State = "stopped"
	handle.record.Status = "stopped"
	// Only stamp FinishedAt for handles with no attached runWorker
	// goroutine (logFile is the tell — StartExecuteLoop always sets it).
	// For real workers, runWorker sets FinishedAt after its own cleanup.
	if handle.logFile == nil && handle.record.FinishedAt.IsZero() {
		handle.record.FinishedAt = time.Now().UTC()
		finalSnapshot := handle.record
		m.mu.Unlock()
		_ = m.writeRecord(dir, finalSnapshot)
		return nil
	}
	m.mu.Unlock()
	return nil
}

// Shutdown stops every live server-owned worker, reaps any running worker
// records left on disk after a restart, and waits for managed workers that
// still have a live goroutine to persist their terminal stopped state.
// It is idempotent and best-effort: shutdown continues past individual worker
// errors and returns the first one encountered.
func (m *WorkerManager) Shutdown() error {
	if m == nil {
		return nil
	}

	m.StopWatchdog()

	type liveWorker struct {
		id         string
		waitForEnd bool
	}

	m.mu.Lock()
	live := make([]liveWorker, 0, len(m.workers))
	liveIDs := make(map[string]struct{}, len(m.workers))
	for id, handle := range m.workers {
		if handle == nil {
			continue
		}
		rec := handle.record
		if rec.State != "running" && rec.State != "stopping" {
			continue
		}
		if !rec.FinishedAt.IsZero() {
			continue
		}
		liveIDs[id] = struct{}{}
		live = append(live, liveWorker{
			id:         id,
			waitForEnd: handle.logFile != nil,
		})
	}
	m.mu.Unlock()

	var firstErr error
	for _, worker := range live {
		if err := m.Stop(worker.id); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if entries, err := os.ReadDir(m.rootDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			id := entry.Name()
			if _, ok := liveIDs[id]; ok {
				continue
			}

			rec, err := m.readRecord(filepath.Join(m.rootDir, id))
			if err != nil {
				continue
			}
			if rec.State != "running" && rec.State != "stopping" {
				continue
			}
			if rec.PID > 0 || rec.PGID > 0 {
				if err := m.shutdownManagedDiskEntry(rec); err != nil && firstErr == nil {
					firstErr = err
				}
				continue
			}
			if err := m.stopStaleDiskEntry(id); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) && firstErr == nil {
		firstErr = err
	}

	const shutdownWaitTimeout = 5 * time.Second
	for _, worker := range live {
		if !worker.waitForEnd {
			continue
		}
		if err := m.waitForStoppedRecord(worker.id, shutdownWaitTimeout); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (m *WorkerManager) waitForStoppedRecord(id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rec, err := m.Show(id)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				time.Sleep(25 * time.Millisecond)
				continue
			}
			return err
		}
		if rec.State == "stopped" && rec.Status == "stopped" && !rec.FinishedAt.IsZero() {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("worker %s did not persist stopped state before shutdown timeout", id)
}

func (m *WorkerManager) shutdownManagedDiskEntry(rec WorkerRecord) error {
	now := time.Now().UTC()
	projectRoot := rec.ProjectRoot
	if projectRoot == "" {
		projectRoot = m.projectRoot
	}
	beadID := ""
	if rec.CurrentAttempt != nil {
		beadID = rec.CurrentAttempt.BeadID
	}
	if beadID == "" {
		beadID = rec.CurrentBead
	}
	rootPID := rec.PID
	if rootPID <= 0 {
		rootPID = rec.PGID
	}

	if beadID != "" {
		store := bead.NewStore(ddxroot.JoinProject(projectRoot))
		body := fmt.Sprintf("worker=%s pid=%d reason=shutdown", rec.ID, rootPID)
		_ = store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "bead.stopped",
			Summary:   "shutdown",
			Body:      body,
			Actor:     "ddx",
			Source:    "server-workers",
			CreatedAt: now,
		})
		_ = store.Unclaim(beadID)
	}

	_, _, _, grace := m.watchdogDeadlines()
	cleanupReport := cleanupManagedWorkerProcessTree(rootPID, nil, grace)
	cleanupSummary := cleanupReport.String()

	rec.State = "stopped"
	rec.Status = "stopped"
	if rec.FinishedAt.IsZero() {
		rec.FinishedAt = now
	}
	rec.Lifecycle = append(rec.Lifecycle, WorkerLifecycleEvent{
		Action:    "stop",
		Actor:     "local-operator",
		Timestamp: now,
		Detail:    fmt.Sprintf("reason=shutdown pid=%d cleanup=%s", rootPID, cleanupSummary),
		BeadID:    beadID,
	})
	dir := filepath.Join(m.rootDir, rec.ID)
	return m.writeRecord(dir, rec)
}

// ensureWatchdog starts the supervisor goroutine exactly once per manager.
// The goroutine runs until StopWatchdog() is called (or the process exits).
func (m *WorkerManager) ensureWatchdog() {
	m.watchdogOnce.Do(func() {
		go m.watchdogLoop()
	})
}

// StopWatchdog halts the supervisor goroutine. Idempotent; used by tests.
func (m *WorkerManager) StopWatchdog() {
	defer func() { _ = recover() }() // tolerate double-close
	close(m.watchdogStop)
}

// watchdogLoop periodically inspects every registered workerHandle and reaps
// those that have outlived WatchdogDeadline with no phase transition in
// StallDeadline.
func (m *WorkerManager) watchdogLoop() {
	_, _, check, _ := m.watchdogDeadlines()
	ticker := time.NewTicker(check)
	defer ticker.Stop()

	for {
		select {
		case <-m.watchdogStop:
			return
		case <-ticker.C:
			m.watchdogSweep(time.Now().UTC())
		}
	}
}

// watchdogSweep inspects every handle once. Split out from watchdogLoop so
// tests can drive the check deterministically without relying on tickers.
func (m *WorkerManager) watchdogSweep(now time.Time) {
	watchdogDL, stallDL, _, _ := m.watchdogDeadlines()

	type candidate struct {
		id      string
		handle  *workerHandle
		runtime time.Duration
		stalled time.Duration
		beadID  string
		pid     int
	}

	m.mu.Lock()
	var picks []candidate
	for id, h := range m.workers {
		if h == nil || h.reaped {
			continue
		}
		rec := h.record
		if !rec.FinishedAt.IsZero() {
			continue
		}
		if rec.StartedAt.IsZero() {
			continue
		}
		runtime := now.Sub(rec.StartedAt)
		if runtime <= watchdogDL {
			continue
		}
		// Stall check — require an in-flight attempt; a worker that is
		// between beads (CurrentAttempt == nil) has no phase to wedge on.
		if rec.CurrentAttempt == nil {
			continue
		}
		lastPhase := h.lastPhaseTS
		if lastPhase.IsZero() {
			lastPhase = rec.StartedAt
		}
		stalled := now.Sub(lastPhase)
		if stalled <= stallDL {
			continue
		}

		beadID := ""
		if rec.CurrentAttempt != nil {
			beadID = rec.CurrentAttempt.BeadID
		}
		if beadID == "" {
			beadID = rec.CurrentBead
		}

		pid := rec.PID
		if pid <= 0 && h.managed {
			pid = rec.PGID
		}

		h.reaped = true
		picks = append(picks, candidate{
			id:      id,
			handle:  h,
			runtime: runtime,
			stalled: stalled,
			beadID:  beadID,
			pid:     pid,
		})
	}
	m.mu.Unlock()

	for _, c := range picks {
		m.reapWorker(c.id, c.handle, c.pid, c.beadID, c.runtime, c.stalled, "watchdog")
	}
}

// reapWorker performs the escalation for a stalled worker:
//  1. Emit bead.reaped event on the bead tracker (if a bead is claimed).
//  2. Release the bead claim (Unclaim → status=open).
//  3. SIGTERM → grace → SIGKILL the worker's process group and any
//     registered child process groups, if any are present. Fall back to
//     ctx cancellation for pure-goroutine workers.
//  4. Mark the WorkerRecord state=reaped and persist it.
func (m *WorkerManager) reapWorker(id string, handle *workerHandle, pid int, beadID string, runtime, stalled time.Duration, reason string) {
	now := time.Now().UTC()

	m.mu.Lock()
	rec := handle.record
	projectRoot := rec.ProjectRoot
	if projectRoot == "" {
		projectRoot = m.projectRoot
	}
	processRoot := pid
	if processRoot <= 0 && handle.managed {
		processRoot = rec.PGID
	}
	cleanupPGIDs := append([]int(nil), handle.cleanupPGIDs...)
	m.mu.Unlock()

	// 1. Emit the reap event and release the bead claim before killing, so
	//    the claim is not leaked even if the kill blocks for the full grace.
	if beadID != "" {
		store := bead.NewStore(ddxroot.JoinProject(projectRoot))
		body := fmt.Sprintf(
			"worker=%s runtime=%s stalled=%s pid=%d reason=%s",
			id, runtime.Round(time.Second), stalled.Round(time.Second), processRoot, reason,
		)
		_ = store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "bead.reaped",
			Summary:   reason,
			Body:      body,
			Actor:     "ddx-watchdog",
			Source:    "server-workers",
			CreatedAt: now,
		})
		_ = store.Unclaim(beadID)
	}

	// 2. Escalate to the worker process tree if we know any server-owned
	//    process groups.
	_, _, _, grace := m.watchdogDeadlines()
	cleanupReport := cleanupManagedWorkerProcessTree(processRoot, cleanupPGIDs, grace)

	// 3. Cancel the goroutine so any in-process code sees context.Canceled.
	if handle.cancel != nil {
		handle.cancel()
	}

	// 4. Flip state=reaped and persist. runWorker may still race to
	//    overwrite this with "exited" when it returns; that's fine — the
	//    bead.reaped event plus released claim are the durable record.
	m.mu.Lock()
	handle.record.State = "reaped"
	handle.record.Status = "reaped"
	handle.record.ReapReason = reason
	if handle.record.FinishedAt.IsZero() {
		handle.record.FinishedAt = now
	}
	if handle.record.LastError == "" {
		handle.record.LastError = fmt.Sprintf("watchdog reaped worker after runtime=%s stalled=%s",
			runtime.Round(time.Second), stalled.Round(time.Second))
	}
	if cleanupSummary := cleanupReport.String(); cleanupSummary != "" {
		handle.record.LastError = fmt.Sprintf("%s; cleanup=%s", handle.record.LastError, cleanupSummary)
	}
	dir := filepath.Join(m.rootDir, id)
	snapshot := handle.record
	m.mu.Unlock()

	_ = m.writeRecord(dir, snapshot)
}

func (m *WorkerManager) Logs(id string) (string, string, error) {
	m.mu.Lock()
	if handle, ok := m.workers[id]; ok {
		log := handle.logBuf.String()
		m.mu.Unlock()
		return log, "", nil
	}
	m.mu.Unlock()

	// Fall back to reading from disk for completed workers
	rec, err := m.Show(id)
	if err != nil {
		return "", "", err
	}
	if rec.StdoutPath == "" {
		return "", "", nil
	}
	data, err := os.ReadFile(filepath.Join(m.projectRoot, rec.StdoutPath))
	if err != nil {
		return "", "", err
	}
	return string(data), "", nil
}

// drainProgress reads ProgressEvents from ch and:
//  1. Updates the WorkerRecord's CurrentAttempt, RecentPhases, and LastAttempt fields.
//  2. Broadcasts each event to all active SSE subscribers for the worker.
//
// It runs as a goroutine alongside runWorker; it exits when ch is closed.
func (m *WorkerManager) drainProgress(workerID string, handle *workerHandle, ch <-chan agent.ProgressEvent) {
	const maxRecentPhases = 20
	for evt := range ch {
		m.mu.Lock()
		rec := handle.record

		// Substate signals (ddx-dc157075 AC #5). loop.idle marks the worker as
		// "idle" without disturbing CurrentAttempt; loop.active clears it when
		// the loop picks up the next candidate. These are not real attempt
		// phases, so they bypass the normal RecentPhases / CurrentAttempt path.
		switch evt.Phase {
		case "loop.idle":
			rec.Substate = "idle"
			handle.record = rec
			m.mu.Unlock()
			continue
		case "loop.active":
			rec.Substate = ""
			handle.record = rec
			m.mu.Unlock()
			continue
		}

		if !evt.Heartbeat {
			// Phase-transition: record in RecentPhases (capped at maxRecentPhases)
			rec.RecentPhases = append(rec.RecentPhases, PhaseTransition{
				Phase:    evt.Phase,
				TS:       evt.TS,
				PhaseSeq: evt.PhaseSeq,
			})
			if len(rec.RecentPhases) > maxRecentPhases {
				rec.RecentPhases = rec.RecentPhases[len(rec.RecentPhases)-maxRecentPhases:]
			}
			// Stamp lastPhaseTS so the watchdog can detect stalled attempts.
			handle.lastPhaseTS = evt.TS
		}

		if terminalPhases[evt.Phase] {
			// Increment live counters so Show() reflects progress before the
			// loop exits. runWorker will overwrite these with authoritative
			// loopResult values after progressDone is signalled, which is the
			// same value — so no double-counting occurs.
			rec.Attempts++
			if evt.Phase == "done" {
				rec.Successes++
			} else {
				rec.Failures++
			}

			// Move CurrentAttempt → LastAttempt
			if rec.CurrentAttempt != nil {
				rec.LastAttempt = &LastAttemptInfo{
					AttemptID: rec.CurrentAttempt.AttemptID,
					BeadID:    rec.CurrentAttempt.BeadID,
					Phase:     evt.Phase,
					StartedAt: rec.CurrentAttempt.StartedAt,
					EndedAt:   evt.TS,
					ElapsedMS: evt.ElapsedMS,
				}
			}
			rec.CurrentAttempt = nil
		} else {
			// Update or initialise CurrentAttempt
			if rec.CurrentAttempt == nil {
				rec.CurrentAttempt = &CurrentAttemptInfo{
					AttemptID: evt.AttemptID,
					BeadID:    evt.BeadID,
					StartedAt: evt.TS,
				}
			}
			rec.CurrentAttempt.AttemptID = evt.AttemptID
			rec.CurrentAttempt.BeadID = evt.BeadID
			rec.CurrentAttempt.Phase = evt.Phase
			rec.CurrentAttempt.PhaseSeq = evt.PhaseSeq
			rec.CurrentAttempt.ElapsedMS = evt.ElapsedMS
			if evt.Harness != "" {
				rec.CurrentAttempt.Harness = evt.Harness
			}
			if evt.Model != "" {
				rec.CurrentAttempt.Model = evt.Model
			}
			if evt.Profile != "" {
				rec.CurrentAttempt.Profile = evt.Profile
			}
		}

		handle.record = rec

		// Broadcast to SSE subscribers (non-blocking; slow subscribers are dropped)
		subs := handle.progressSubs
		m.mu.Unlock()

		for _, sub := range subs {
			select {
			case sub <- evt:
			default:
				// Subscriber channel full — skip rather than block
			}
		}
	}

	// Channel closed: clear CurrentAttempt if still set (worker exited)
	m.mu.Lock()
	if handle.record.CurrentAttempt != nil {
		handle.record.CurrentAttempt = nil
	}
	// Close and remove all subscriber channels
	for _, sub := range handle.progressSubs {
		close(sub)
	}
	handle.progressSubs = nil
	m.mu.Unlock()

	// Signal that no further events will arrive
	if handle.progressDone != nil {
		close(handle.progressDone)
	}
}

// SubscribeProgress returns a channel that receives ProgressEvents for the
// given worker, plus an unsubscribe function. If the worker is not active or
// has already finished, the returned channel is pre-closed so SSE handlers
// can detect idle/done state immediately.
func (m *WorkerManager) SubscribeProgress(workerID string) (<-chan agent.ProgressEvent, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	handle, ok := m.workers[workerID]
	if !ok {
		// Worker never started or was never registered
		ch := make(chan agent.ProgressEvent)
		close(ch)
		return ch, func() {}
	}

	// Check if drainProgress has already exited (worker done)
	if handle.progressDone != nil {
		select {
		case <-handle.progressDone:
			ch := make(chan agent.ProgressEvent)
			close(ch)
			return ch, func() {}
		default:
		}
	}

	ch := make(chan agent.ProgressEvent, 64)
	handle.progressSubs = append(handle.progressSubs, ch)

	unsub := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if h, ok := m.workers[workerID]; ok {
			for i, sub := range h.progressSubs {
				if sub == ch {
					h.progressSubs = append(h.progressSubs[:i], h.progressSubs[i+1:]...)
					break
				}
			}
		}
	}
	return ch, unsub
}

type preClaimGitOps interface {
	CurrentBranch(dir string) (string, error)
	LocalAncestryCheck(dir, targetBranch string) (agent.PreClaimResult, error)
}

// buildPreClaimHook returns a PreClaimHook function that verifies the local
// target branch against the last-observed origin remote-tracking ref before
// each bead claim. It resolves the target branch at call time via CurrentBranch
// so detached-HEAD and non-main trunks are handled correctly. It performs no
// network I/O (reliability principle P9): origin refresh is operator-driven via
// `ddx sync`.
func buildPreClaimHook(projectRoot string, gitOps preClaimGitOps) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		branch, err := gitOps.CurrentBranch(projectRoot)
		if err != nil {
			// Can't determine branch — skip rather than block.
			return nil
		}
		res, err := gitOps.LocalAncestryCheck(projectRoot, branch)
		if err != nil {
			if !agent.IsIgnorableFetchOriginError(err) {
				return err
			}
			return nil // fetch failure is non-fatal
		}
		if res.Action == "diverged" {
			return fmt.Errorf("local branch %s has diverged from origin (local=%s origin=%s); reconcile manually before claiming",
				branch, res.LocalSHA, res.OriginSHA)
		}
		return nil
	}
}

func (m *WorkerManager) writeRecord(dir string, record WorkerRecord) error {
	if record.Status == "" {
		record.Status = record.State
	}
	record.PIDAlive = nil // computed field; never persisted
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "status.json"), append(data, '\n'), 0o644)
}

func (m *WorkerManager) readRecord(dir string) (WorkerRecord, error) {
	data, err := os.ReadFile(filepath.Join(dir, "status.json"))
	if err != nil {
		return WorkerRecord{}, err
	}
	var record WorkerRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return WorkerRecord{}, err
	}
	if record.Status == "" {
		record.Status = record.State
	}
	return record, nil
}

// gateLandSubmitter is the subset of *LandCoordinator that
// evaluateGatesAndSubmit needs. Defined here so tests can drive the gate
// landing path against either a real coordinator or a fake.
type gateLandSubmitter interface {
	Submit(req agent.LandRequest) (*agent.LandResult, error)
}

type landSafetyConfig struct {
	postLandCommand            []string
	largeDeletionLineThreshold int
}

// evaluateGatesAndSubmit runs the required-gate evaluation BEFORE submitting
// res to the coordinator. When a required gate fails (or a ratchet misses),
// it preserves the result directly via update-ref and skips coordinator
// submission entirely — Land() stays a pure ref-advance contract; gate
// enforcement happens upstream. When gates pass (or no governing IDs are
// declared in the manifest), it submits the LandRequest to the coordinator
// and applies the LandResult onto res.
//
// Mirrors the interactive path in cmd/agent_execute_bead.go. The preserve
// reason/status fields are set the same way ApplyLandingToResult would set
// them on the same scenario, so server and interactive paths produce
// identical preserve evidence.
//
// Returns the coordinator submit error when one occurs; gate-context and
// gate-eval errors are soft-logged and treated as no-eval (the existing
// submit path continues).
func evaluateGatesAndSubmit(
	projectRoot string,
	res *agent.ExecuteBeadResult,
	executionErr error,
	gitOps agent.GitOps,
	coordinator gateLandSubmitter,
	safety landSafetyConfig,
	log io.Writer,
) error {
	// A non-nil execution error is authoritative even when the paired result
	// still carries a success-looking outcome. In particular, isolated-clone
	// publication can fail after the candidate and evidence bundle exist; no
	// such result may reach a preserve ref, gate worktree, or coordinator.
	if executionErr != nil {
		return executionErr
	}
	if res == nil {
		return fmt.Errorf("cannot evaluate landing gates for a nil result")
	}
	// Defense in depth at the server's final submit boundary: scan every
	// candidate commit before gates can preserve it or the coordinator can land
	// it. This catches add-then-delete histories whose final tree is clean.
	if err := agent.VerifyCandidateHasNoExecutionEvidence(projectRoot, res.BaseRev, res.ResultRev); err != nil {
		return fmt.Errorf("validating server landing candidate: %w", err)
	}
	wt, ids, cleanup, ctxErr := agent.BuildLandingGateContext(projectRoot, res, gitOps)
	if ctxErr != nil {
		// Soft-fail: log and skip gate eval rather than abort the land.
		_, _ = fmt.Fprintf(log, "ddx: warning: gate-context setup failed: %v (skipping required-gate eval)\n", ctxErr)
	}
	defer cleanup()

	if wt != "" {
		checksAbs := filepath.Join(projectRoot, res.ExecutionDir, "checks.json")
		checksRel := filepath.Join(res.ExecutionDir, "checks.json")
		anyFailed, ratchetFailed, evalErr := agent.EvaluateRequiredGatesForResult(wt, ids, res, projectRoot, checksAbs, checksRel)
		if evalErr != nil {
			// Log and treat as no-eval; existing path continues.
			_, _ = fmt.Fprintf(log, "ddx: warning: gate evaluation failed: %v (skipping)\n", evalErr)
		} else if anyFailed || ratchetFailed {
			// Preserve directly. Mirror LandBeadResult's preserve path so the
			// server produces identical evidence to the interactive path.
			// PreserveRef helper produces refs/ddx/iterations/<bead>/<ts>-<shortSHA>;
			// using it keeps server- and interactive-managed evidence indistinguishable.
			preserveRef := agent.PreserveRef(res.BeadID, res.BaseRev)
			if upErr := gitOps.UpdateRef(projectRoot, preserveRef, res.ResultRev); upErr != nil {
				_, _ = fmt.Fprintf(log, "ddx: warning: preserving result ref %s failed: %v\n", preserveRef, upErr)
			} else {
				res.PreserveRef = preserveRef
			}
			res.Outcome = "preserved"
			if ratchetFailed {
				res.Reason = agent.RatchetPreserveReason
			} else {
				res.Reason = "post-run checks failed"
			}
			res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
			res.Detail = agent.ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
			_ = agent.WriteExecuteBeadResultArtifact(projectRoot, res)
			return nil
		}
	}

	// Gates passed (or no governing IDs / soft-failure): submit to coordinator.
	landReq := agent.BuildLandRequestFromResult(projectRoot, res)
	landReq.PostLandCommand = append([]string(nil), safety.postLandCommand...)
	landReq.LargeDeletionLineThreshold = safety.largeDeletionLineThreshold
	landRes, landErr := coordinator.Submit(landReq)
	if landErr != nil {
		return landErr
	}
	agent.ApplyLandResultToExecuteBeadResult(res, landRes)
	_ = agent.WriteExecuteBeadResultArtifact(projectRoot, res)
	return nil
}

func landSafetyConfigFromConfig(projectRoot string) landSafetyConfig {
	cfg, err := config.LoadWithWorkingDir(projectRoot)
	if err != nil || cfg == nil || cfg.Git == nil {
		return landSafetyConfig{}
	}
	return landSafetyConfig{
		postLandCommand:            append([]string(nil), cfg.Git.PostLandCommand...),
		largeDeletionLineThreshold: cfg.Git.LargeDeletionLineThreshold,
	}
}

func relToProject(projectRoot, path string) string {
	rel, err := filepath.Rel(projectRoot, path)
	if err != nil {
		return path
	}
	return rel
}

// WorkerPruneResult describes one worker entry reaped by Prune.
type WorkerPruneResult struct {
	ID      string `json:"id"`
	BeadID  string `json:"bead_id,omitempty"`
	Harness string `json:"harness,omitempty"`
	Age     string `json:"age"`
	Reason  string `json:"reason"`
}

// Prune reaps registry entries whose recorded PID is no longer alive, or
// whose age exceeds maxAge (when maxAge > 0). Only entries with state=running
// that are not attached to a live goroutine in this manager are eligible.
// For each pruned entry the bead claim is released and the on-disk record is
// updated to state=reaped. Returns the list of reaped entries.
func (m *WorkerManager) Prune(maxAge time.Duration) ([]WorkerPruneResult, error) {
	recs, err := m.List()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var results []WorkerPruneResult

	for _, rec := range recs {
		if rec.State != "running" {
			continue
		}

		// Skip workers that are genuinely alive in-memory.
		m.mu.Lock()
		_, inMemory := m.workers[rec.ID]
		m.mu.Unlock()
		if inMemory {
			continue
		}

		// Determine staleness.
		stale := false
		reason := ""

		if rec.PID > 0 {
			if !isPIDAlive(rec.PID) {
				stale = true
				reason = fmt.Sprintf("pid=%d not alive", rec.PID)
			}
		} else {
			// PID=0: goroutine-only worker not in m.workers — server restart.
			stale = true
			reason = "goroutine not running (server restarted?)"
		}

		if !stale && maxAge > 0 && !rec.StartedAt.IsZero() {
			if age := now.Sub(rec.StartedAt); age > maxAge {
				stale = true
				reason = fmt.Sprintf("age=%s exceeds max-age=%s",
					age.Round(time.Second), maxAge.Round(time.Second))
			}
		}

		if !stale {
			continue
		}

		beadID := ""
		if rec.CurrentAttempt != nil {
			beadID = rec.CurrentAttempt.BeadID
		}
		if beadID == "" {
			beadID = rec.CurrentBead
		}
		projectRoot := rec.ProjectRoot
		if projectRoot == "" {
			projectRoot = m.projectRoot
		}

		if beadID != "" {
			store := bead.NewStore(ddxroot.JoinProject(projectRoot))
			body := fmt.Sprintf("worker=%s pid=%d reason=prune %s", rec.ID, rec.PID, reason)
			_ = store.AppendEvent(beadID, bead.BeadEvent{
				Kind:      "bead.reaped",
				Summary:   "prune",
				Body:      body,
				Actor:     "ddx",
				Source:    "server-workers",
				CreatedAt: now,
			})
			_ = store.Unclaim(beadID)
		}

		rec.State = "reaped"
		rec.Status = "reaped"
		rec.ReapReason = "prune"
		if rec.FinishedAt.IsZero() {
			rec.FinishedAt = now
		}
		rec.LastError = reason
		rec.Lifecycle = append(rec.Lifecycle, WorkerLifecycleEvent{
			Action:    "prune",
			Actor:     "local-operator",
			Timestamp: now,
			Detail:    reason,
			BeadID:    beadID,
		})
		dir := filepath.Join(m.rootDir, rec.ID)
		_ = m.writeRecord(dir, rec)

		age := "-"
		if !rec.StartedAt.IsZero() {
			age = now.Sub(rec.StartedAt).Round(time.Second).String()
		}
		results = append(results, WorkerPruneResult{
			ID:      rec.ID,
			BeadID:  beadID,
			Harness: rec.Harness,
			Age:     age,
			Reason:  reason,
		})
	}

	return results, nil
}

// ReconcileStaleWorkers scans the on-disk worker registry and marks entries
// that are still in state=running but have a dead (or missing) PID as
// "exited". Called once at server startup to repair records left running by a
// previous server crash without starting new goroutines for them. Bead claims
// are released so the queue drainer can pick up the work again.
func (m *WorkerManager) ReconcileStaleWorkers() {
	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(m.rootDir, entry.Name())
		rec, err := m.readRecord(dir)
		if err != nil || rec.State != "running" {
			continue
		}

		// Skip workers that are genuinely alive in-memory (shouldn't happen
		// at startup but be defensive).
		m.mu.Lock()
		_, ok := m.workers[rec.ID]
		m.mu.Unlock()
		if ok {
			continue
		}

		// PID > 0 and still alive: don't touch it.
		if rec.PID > 0 && isPIDAlive(rec.PID) {
			continue
		}

		beadID := ""
		if rec.CurrentAttempt != nil {
			beadID = rec.CurrentAttempt.BeadID
		}
		if beadID == "" {
			beadID = rec.CurrentBead
		}
		projectRoot := rec.ProjectRoot
		if projectRoot == "" {
			projectRoot = m.projectRoot
		}

		if beadID != "" {
			store := bead.NewStore(ddxroot.JoinProject(projectRoot))
			body := fmt.Sprintf("worker=%s pid=%d reason=server-restart", rec.ID, rec.PID)
			_ = store.AppendEvent(beadID, bead.BeadEvent{
				Kind:      "bead.reaped",
				Summary:   "server-restart",
				Body:      body,
				Actor:     "ddx-server",
				Source:    "server-workers",
				CreatedAt: now,
			})
			_ = store.Unclaim(beadID)
		}

		rec.State = "exited"
		rec.Status = "exited"
		rec.ReapReason = "server-restart"
		if rec.FinishedAt.IsZero() {
			rec.FinishedAt = now
		}
		rec.LastError = "server restarted while worker was running"
		_ = m.writeRecord(dir, rec)
	}
}

func randomSuffix(n int) string {
	if n <= 0 {
		n = 4
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())[:n]
	}
	return hex.EncodeToString(buf)[:n]
}
