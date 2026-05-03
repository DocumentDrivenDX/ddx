package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	agentlib "github.com/DocumentDrivenDX/fizeau"
)

type ExecuteLoopWorkerSpec struct {
	// ProjectRoot overrides the manager's default project root for this worker.
	// When set, the worker scans and executes beads from this project instead of
	// the server's primary working directory. Must be an absolute path to a
	// directory containing a .ddx/ folder. Validated by the server before the
	// worker starts.
	ProjectRoot  string        `json:"project_root,omitempty"`
	Harness      string        `json:"harness,omitempty"`
	Model        string        `json:"model,omitempty"`
	Profile      string        `json:"profile,omitempty"`
	Provider     string        `json:"provider,omitempty"`
	ModelRef     string        `json:"model_ref,omitempty"`
	Effort       string        `json:"effort,omitempty"`
	LabelFilter  string        `json:"label_filter,omitempty"`
	Once         bool          `json:"once,omitempty"`
	PollInterval time.Duration `json:"poll_interval,omitempty"`
	// Review options — controls the post-merge review agent.
	NoReview      bool   `json:"no_review,omitempty"`
	ReviewHarness string `json:"review_harness,omitempty"`
	ReviewModel   string `json:"review_model,omitempty"`
	// OpaquePassthrough, when true, skips DDx-side route validation and config
	// harness/model injection. Routing belongs to the agent service (CONTRACT-003
	// / FEAT-010 / ddx-c4231775). Set by ddx work; not set by ddx agent execute-loop.
	OpaquePassthrough bool `json:"opaque_passthrough,omitempty"`
}

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
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Status     string `json:"status,omitempty"`
	Detail     string `json:"detail,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	BaseRev    string `json:"base_rev,omitempty"`
	ResultRev  string `json:"result_rev,omitempty"`
	RetryAfter string `json:"retry_after,omitempty"`
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
	// wakeCh, when non-nil, signals an execute-loop worker's idle-poll
	// sleep to return early so the loop re-scans the ready queue. The
	// channel is buffered (cap 1) so a non-blocking send coalesces multiple
	// wake calls into at most one extra tick. Set on execute-loop workers;
	// nil on plugin-action workers.
	wakeCh chan struct{}
}

// WorkerManager manages in-process execute-loop workers as goroutines.
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
}

const (
	defaultWatchdogDeadline      = 6 * time.Hour
	defaultStallDeadline         = 1 * time.Hour
	defaultWatchdogCheckInterval = 1 * time.Minute
	defaultWatchdogKillGrace     = 30 * time.Second
)

// WakeProject signals every running execute-loop worker bound to projectRoot
// to skip its current idle-poll sleep and re-scan the ready queue. Used by
// the operator-prompt approve / auto-approve mutations (Story 15) so a
// freshly-approved bead is claimed immediately rather than after a full
// PollInterval. The send is non-blocking and the per-worker wake channel is
// buffered (cap 1), so multiple wakes within one tick coalesce. Returns the
// number of workers signalled (0 when no execute-loop is running for the
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
		rootDir:          filepath.Join(projectRoot, ".ddx", "workers"),
		workers:          map[string]*workerHandle{},
		LandCoordinators: newCoordinatorRegistry(),
		watchdogStop:     make(chan struct{}),
	}
	m.applyServerWatchdogConfig(projectRoot)
	return m
}

func lifecycleStartDetail(spec ExecuteLoopWorkerSpec) string {
	parts := []string{"kind=execute-loop"}
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

	// Pre-flight: validate harness availability and model compatibility
	// before creating the worker record or claiming any beads.
	// Skipped when OpaquePassthrough=true (ddx work path): routing belongs to
	// the agent service; DDx must not pre-resolve or validate the route.
	if !spec.OpaquePassthrough {
		if err := agent.ValidateForExecuteLoopViaService(context.Background(), effectiveRoot, spec.Harness, spec.Model, spec.Provider, spec.ModelRef); err != nil {
			return WorkerRecord{}, fmt.Errorf("execute-loop: %w", err)
		}
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
		Kind:         "execute-loop",
		State:        "running",
		Status:       "running",
		ProjectRoot:  effectiveRoot,
		Harness:      spec.Harness,
		Model:        spec.Model,
		Profile:      agent.NormalizeRoutingProfile(spec.Profile),
		Effort:       spec.Effort,
		Once:         spec.Once,
		PollInterval: spec.PollInterval.String(),
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
	handle := &workerHandle{
		record:       record,
		cancel:       cancel,
		logBuf:       logBuf,
		logFile:      logFile,
		progressCh:   progressCh,
		progressDone: make(chan struct{}),
		lastPhaseTS:  time.Now().UTC(),
		wakeCh:       wakeCh,
	}

	m.mu.Lock()
	m.workers[id] = handle
	m.mu.Unlock()

	m.ensureWatchdog()

	go m.drainProgress(id, handle, progressCh)
	go m.runWorker(ctx, id, dir, spec, effectiveRoot, handle, multiLog, eventsFile, progressCh)

	return record, nil
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
			AttemptID: id,
			WorkerID:  id,
			Status:    state,
			Detail:    fmt.Sprintf("%s plugin %s", spec.Action, spec.Name),
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

// buildRoutePreflight constructs the route preflight gate (FEAT-006 D3, ddx-98e6e9ef).
// Before claiming a bead, the loop consults upstream ResolveRoute against the
// resolved (harness, model). If the upstream surfaces agent.ErrHarnessModelIncompatible
// (or any other typed error), the loop exits with a worker-level execution_failed
// record — no bead is claimed, no executor invocation, no tier-attempt event burn.
// DDx does NOT duplicate the upstream allow-list; this gate only consumes the typed
// incompatibility surface.
//
// Returns nil (no preflight) when:
//   - hasBeadWorkerFactory is true (test injection path), OR
//   - spec.OpaquePassthrough is true (ddx work path — routing belongs to the agent
//     service), OR
//   - the operator pinned NOTHING (no Profile, no Model). In the auto-route case
//     the strict ResolveRoute path is too aggressive — it picks a default catalog
//     model from the harness's first entry and rejects exact-pin-only candidates.
//     Per ddx-9e4c238d / ddx-1e516bc9: bare auto-route must pass through to fizeau's
//     more lenient Execute path on each attempt.
func buildRoutePreflight(projectRoot string, spec ExecuteLoopWorkerSpec, hasBeadWorkerFactory bool) func(ctx context.Context, harness, model string) error {
	if hasBeadWorkerFactory || spec.OpaquePassthrough {
		return nil
	}
	// Auto-route case: operator pinned no Profile and no Model. Skip strict
	// preflight; let fizeau's Execute handle routing on each attempt.
	if spec.Profile == "" && spec.Model == "" {
		return nil
	}
	preflightSvc, preflightErr := agent.NewServiceFromWorkDir(projectRoot)
	return func(ctx context.Context, harness, model string) error {
		svc := preflightSvc
		if svc == nil {
			return preflightErr
		}
		req := agentlib.RouteRequest{
			Profile: agent.NormalizeRoutingProfile(spec.Profile),
		}
		if harness != "" {
			req.Harness = harness
		}
		if model != "" {
			req.Model = model
		}
		_, rErr := svc.ResolveRoute(ctx, req)
		return rErr
	}
}

func sendProgress(ch chan<- agent.ProgressEvent, evt agent.ProgressEvent) {
	select {
	case ch <- evt:
	default:
	}
}

func (m *WorkerManager) runWorker(ctx context.Context, id, dir string, spec ExecuteLoopWorkerSpec, projectRoot string, handle *workerHandle, log io.Writer, eventSink io.WriteCloser, progressCh chan agent.ProgressEvent) {
	if eventSink != nil {
		defer eventSink.Close() //nolint:errcheck
	}
	store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))

	var worker *agent.ExecuteBeadWorker
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

		// singleTierAttempt runs one execution at a specific harness/model.
		singleTierAttempt := func(ctx context.Context, beadID string, tier escalation.ModelTier, resolvedHarness, resolvedProvider, resolvedModel string) (agent.ExecuteBeadReport, error) {
			gitOps := &agent.RealGitOps{}
			attemptProvider := spec.Provider
			if resolvedProvider != "" {
				attemptProvider = resolvedProvider
			}
			attemptRcfg, _ := config.LoadAndResolve(projectRoot, config.CLIOverrides{
				Harness:           resolvedHarness,
				Model:             resolvedModel,
				Provider:          attemptProvider,
				ModelRef:          spec.ModelRef,
				Effort:            spec.Effort,
				OpaquePassthrough: spec.OpaquePassthrough,
			})
			beadStore := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
			res, err := agent.ExecuteBeadWithConfig(ctx, projectRoot, beadID, attemptRcfg, agent.ExecuteBeadRuntime{
				BeadEvents: beadStore,
				BeadCancel: beadStore,
			}, gitOps)
			if err != nil && res == nil {
				return agent.ExecuteBeadReport{}, err
			}
			// Preserve operator-cancel results as-is; the worker has already
			// classified them (preserved / operator_cancel / preserved_needs_review)
			// and overriding here would lose the cancel signal.
			operatorCancel := res != nil && res.Reason == agent.OperatorCancelReason
			if res != nil && res.ResultRev != "" && res.ResultRev != res.BaseRev && res.ExitCode == 0 && !operatorCancel {
				if landErr := evaluateGatesAndSubmit(projectRoot, res, gitOps, coordinator, landSafetyConfigFromConfig(projectRoot), log); landErr != nil && err == nil {
					err = landErr
				}
			} else if res != nil && res.ResultRev == res.BaseRev && !operatorCancel {
				res.Outcome = "no-changes"
				res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
			} else if res != nil && res.ExitCode != 0 && !operatorCancel {
				res.Outcome = "preserved"
				res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
			}
			// Safety net: commit any leftover evidence files that Land()
			// did not pick up (e.g. HEAD was detached, or no land ran).
			if res != nil && res.AttemptID != "" {
				_ = agent.VerifyCleanWorktree(projectRoot, res.AttemptID)
			}
			if err != nil {
				return agent.ExecuteBeadReport{}, err
			}
			tierStr := ""
			if tier != "" {
				tierStr = string(tier)
			}
			return agent.ExecuteBeadReport{
				BeadID:             res.BeadID,
				AttemptID:          res.AttemptID,
				WorkerID:           res.WorkerID,
				Harness:            res.Harness,
				Provider:           res.Provider,
				Model:              res.Model,
				ActualPower:        res.ActualPower,
				Tier:               tierStr,
				Status:             res.Status,
				Detail:             res.Detail,
				SessionID:          res.SessionID,
				BaseRev:            res.BaseRev,
				ResultRev:          res.ResultRev,
				PreserveRef:        res.PreserveRef,
				NoChangesRationale: res.NoChangesRationale,
				CostUSD:            res.CostUSD,
				DurationMS:         int64(res.DurationMS),
			}, nil
		}

		// Cost-cap state for this worker run. Subscription / local providers do
		// not contribute (see escalation.CountsTowardCostCap).
		// TODO(ddx-785d02f7): expose maxCostUSD as a worker spec field
		// once the spec config knob lands.
		costCap := escalation.NewCostCapTracker(escalation.DefaultMaxCostUSD, func(harnessName string) bool {
			svc, svcErr := agent.NewServiceFromWorkDir(projectRoot)
			if svcErr != nil {
				return true
			}
			infos, err := svc.ListHarnesses(context.Background())
			if err != nil {
				return true
			}
			for _, h := range infos {
				if h.Name == harnessName {
					return escalation.CountsTowardCostCap(h.IsLocal, h.IsSubscription, h.CostClass)
				}
			}
			return true
		})
		accumulateBilledCost := func(report agent.ExecuteBeadReport) {
			costCap.Add(report.Harness, report.CostUSD)
		}
		costCapTripped := func() (agent.ExecuteBeadReport, bool) {
			detail, tripped := costCap.Tripped()
			if !tripped {
				return agent.ExecuteBeadReport{}, false
			}
			return agent.ExecuteBeadReport{
				Status: agent.ExecuteBeadStatusExecutionFailed,
				Detail: detail,
			}, true
		}

		executor := agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			if cappedReport, capped := costCapTripped(); capped {
				cappedReport.BeadID = beadID
				return cappedReport, nil
			}
			report, err := singleTierAttempt(ctx, beadID, "", spec.Harness, spec.Provider, spec.Model)
			if err == nil {
				accumulateBilledCost(report)
				if cappedReport, capped := costCapTripped(); capped {
					cappedReport.BeadID = beadID
					return cappedReport, nil
				}
			}
			return report, err
		})

		// Build post-merge reviewer. On-by-default unless NoReview is set in spec.
		var reviewer agent.BeadReviewer
		if !spec.NoReview {
			reviewer = &agent.DefaultBeadReviewer{
				ProjectRoot: projectRoot,
				BeadStore:   bead.NewStore(filepath.Join(projectRoot, ".ddx")),
				BeadEvents:  bead.NewStore(filepath.Join(projectRoot, ".ddx")),
				Harness:     spec.ReviewHarness,
				Model:       spec.ReviewModel,
			}
		}

		worker = &agent.ExecuteBeadWorker{
			Store:    store,
			Executor: executor,
			Reviewer: reviewer,
		}
	}

	// SD-024 Stage 1: server dispatch flows through LoadAndResolve +
	// RunWithConfig so .ddx/config.yaml's durable knobs (review_max_retries,
	// no_progress_cooldown, max_no_changes_before_close, heartbeat_interval,
	// agent.harness/model/permissions) reach the running loop without
	// per-knob plumbing. Spec fields layer onto config as overrides via
	// CLIOverrides — when set they win over the on-disk config. Each
	// request reloads the target project's config fresh (no cache).
	overrides := config.CLIOverrides{
		Assignee:          "ddx",
		Harness:           spec.Harness,
		Model:             spec.Model,
		Provider:          spec.Provider,
		ModelRef:          spec.ModelRef,
		Profile:           agent.NormalizeRoutingProfile(spec.Profile),
		Effort:            spec.Effort,
		OpaquePassthrough: spec.OpaquePassthrough,
	}
	rcfg, _ := config.LoadAndResolve(projectRoot, overrides)

	landingOps := agent.RealLandingGitOps{}
	routePreflight := buildRoutePreflight(projectRoot, spec, m.BeadWorkerFactory != nil)

	loopResult, err := worker.Run(ctx, rcfg, agent.ExecuteBeadLoopRuntime{
		Once:           spec.Once,
		PollInterval:   spec.PollInterval,
		Log:            log,
		EventSink:      eventSink,
		WorkerID:       id,
		ProjectRoot:    projectRoot,
		LabelFilter:    spec.LabelFilter,
		ProgressCh:     progressCh,
		PreClaimHook:   buildPreClaimHook(projectRoot, landingOps),
		NoReview:       spec.NoReview,
		RoutePreflight: routePreflight,
		WakeCh:         handle.wakeCh,
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
				BeadID:     last.BeadID,
				AttemptID:  last.AttemptID,
				WorkerID:   last.WorkerID,
				Harness:    last.Harness,
				Provider:   last.Provider,
				Model:      last.Model,
				Status:     last.Status,
				Detail:     last.Detail,
				SessionID:  last.SessionID,
				BaseRev:    last.BaseRev,
				ResultRev:  last.ResultRev,
				RetryAfter: last.RetryAfter,
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

	// Compute PIDAlive for each record so operators can see liveness at a
	// glance without running prune. This is a syscall per record; it's safe
	// because List() is not on a hot path (workers are long-lived).
	for i := range out {
		if out[i].PID > 0 {
			alive := isPIDAlive(out[i].PID)
			out[i].PIDAlive = &alive
		}
	}

	return out, nil
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
//  3. Send SIGTERM to the worker's process group; escalate to SIGKILL
//     after WatchdogKillGrace if the leader is still alive. Pure-goroutine
//     workers have no PID — ctx cancellation below is the only lever.
//  4. Cancel the worker's context so the loop and in-flight executor exit.
//  5. Mark state=stopped and persist. runWorker preserves this terminal
//     state when it writes its final record.
//
// Stop is idempotent: a second call is a no-op. It returns an error only
// when the worker is unknown to the manager (already exited / never existed).
// stopStaleDiskEntry handles the case where a worker's disk record shows
// state=running but no live goroutine exists (e.g. after a server restart).
// It releases any held bead claim and flips the on-disk state to "stopped".
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

	if beadID != "" {
		store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
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
		store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
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

	// Escalate to the process group if we know the PID.
	_, _, _, grace := m.watchdogDeadlines()
	if pid > 0 {
		terminateProcessGroup(pid, grace)
	}

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

		h.reaped = true
		picks = append(picks, candidate{
			id:      id,
			handle:  h,
			runtime: runtime,
			stalled: stalled,
			beadID:  beadID,
			pid:     rec.PID,
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
//  3. SIGTERM → grace → SIGKILL the worker's process group, if a PID is
//     registered. Fall back to ctx cancellation for pure-goroutine workers.
//  4. Mark the WorkerRecord state=reaped and persist it.
func (m *WorkerManager) reapWorker(id string, handle *workerHandle, pid int, beadID string, runtime, stalled time.Duration, reason string) {
	now := time.Now().UTC()

	m.mu.Lock()
	rec := handle.record
	projectRoot := rec.ProjectRoot
	if projectRoot == "" {
		projectRoot = m.projectRoot
	}
	m.mu.Unlock()

	// 1. Emit the reap event and release the bead claim before killing, so
	//    the claim is not leaked even if the kill blocks for the full grace.
	if beadID != "" {
		store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
		body := fmt.Sprintf(
			"worker=%s runtime=%s stalled=%s pid=%d reason=%s",
			id, runtime.Round(time.Second), stalled.Round(time.Second), pid, reason,
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

	// 2. Escalate to the worker process group if we know the PID.
	_, _, _, grace := m.watchdogDeadlines()
	if pid > 0 {
		terminateProcessGroup(pid, grace)
	}

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
	dir := filepath.Join(m.rootDir, id)
	snapshot := handle.record
	m.mu.Unlock()

	_ = m.writeRecord(dir, snapshot)
}

func (m *WorkerManager) Logs(id string) (string, string, error) {
	m.mu.Lock()
	if handle, ok := m.workers[id]; ok {
		log := handle.logBuf.String()
		sessionLog := m.readActiveSessionLog(handle)
		m.mu.Unlock()
		if sessionLog != "" {
			return log + "\n" + sessionLog, "", nil
		}
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

// readActiveSessionLog reads the latest session log entries for an active worker.
// The Fizeau library writes per-iteration entries to .ddx/agent-logs/agent-*.jsonl
// in real-time, so this gives live visibility into what the model provider is doing.
func (m *WorkerManager) readActiveSessionLog(handle *workerHandle) string {
	logDir := filepath.Join(m.projectRoot, ".ddx", "agent-logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return ""
	}

	// Find the most recent agent-*.jsonl file that was modified in the last 30 minutes
	var newest string
	var newestMod time.Time
	cutoff := time.Now().Add(-30 * time.Minute)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "agent-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		// Skip loop event files — they contain loop milestones, not agent session
		// entries. Loop milestone progress is already captured in worker.log.
		if strings.HasPrefix(entry.Name(), "agent-loop-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestMod) && info.ModTime().After(cutoff) {
			newest = filepath.Join(logDir, entry.Name())
			newestMod = info.ModTime()
		}
	}
	if newest == "" {
		return ""
	}

	// Read the last N lines of the session log and format them as readable progress
	data, err := os.ReadFile(newest)
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// Show the last 50 entries
	start := 0
	if len(lines) > 50 {
		start = len(lines) - 50
	}

	return agent.FormatSessionLogLines(lines[start:])
}

// buildPreClaimHook returns a PreClaimHook function that fetches origin and
// verifies ancestry before each bead claim. It resolves the target branch at
// call time via LandingGitOps.CurrentBranch so detached-HEAD and non-main
// trunks are handled correctly.
func buildPreClaimHook(projectRoot string, gitOps agent.LandingGitOps) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		branch, err := gitOps.CurrentBranch(projectRoot)
		if err != nil {
			// Can't determine branch — skip rather than block.
			return nil
		}
		res, err := gitOps.FetchOriginAncestryCheck(projectRoot, branch)
		if err != nil {
			// Fetch failure is non-fatal (air-gap friendly); skip the check.
			return nil
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
	gitOps agent.GitOps,
	coordinator gateLandSubmitter,
	safety landSafetyConfig,
	log io.Writer,
) error {
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

// workerHarnessHealthy reports whether the upstream service has an active
// failure cooldown recorded against the given harness. RouteStatus is the
// service-owned health source. When RouteStatus is unavailable, the harness is
// considered healthy.
func workerHarnessHealthy(ctx context.Context, svc agentlib.FizeauService, harness string) bool {
	if svc == nil || harness == "" {
		return true
	}
	report, err := svc.RouteStatus(ctx)
	if err != nil || report == nil {
		return true
	}
	for _, route := range report.Routes {
		for _, cand := range route.Candidates {
			if cand.Provider != harness {
				continue
			}
			if !cand.Healthy {
				return false
			}
		}
	}
	return true
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
			store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
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
			store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
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
