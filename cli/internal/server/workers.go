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

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
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
	Provider     string        `json:"provider,omitempty"`
	ModelRef     string        `json:"model_ref,omitempty"`
	Effort       string        `json:"effort,omitempty"`
	Once         bool          `json:"once,omitempty"`
	PollInterval time.Duration `json:"poll_interval,omitempty"`
	// Review options — controls the post-merge review agent.
	NoReview      bool   `json:"no_review,omitempty"`
	ReviewHarness string `json:"review_harness,omitempty"`
	ReviewModel   string `json:"review_model,omitempty"`
	// Tier escalation bounds. Empty strings use the defaults (cheap and smart).
	// Ignored when Harness or Model is pinned (escalation disabled).
	MinTier string `json:"min_tier,omitempty"`
	MaxTier string `json:"max_tier,omitempty"`
}

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

type WorkerRecord struct {
	ID             string                 `json:"id"`
	Kind           string                 `json:"kind"`
	State          string                 `json:"state"`
	Status         string                 `json:"status,omitempty"`
	ProjectRoot    string                 `json:"project_root"`
	Harness        string                 `json:"harness,omitempty"`
	Provider       string                 `json:"provider,omitempty"`
	Model          string                 `json:"model,omitempty"`
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
	LandSummary    *CoordinatorMetrics    `json:"land_summary,omitempty"`
	// PID is the OS process id of an external worker subprocess, if any.
	// Zero for purely in-process (goroutine-only) workers. Surfaced so the
	// autonomous watchdog can send SIGTERM/SIGKILL to the process group when
	// cancelling the context is not enough.
	PID int `json:"pid,omitempty"`
	// ReapReason is populated when the watchdog forcibly terminates a worker;
	// set to "watchdog" today.
	ReapReason string `json:"reap_reason,omitempty"`
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
	if err := agent.ValidateForExecuteLoopViaService(context.Background(), effectiveRoot, spec.Harness, spec.Model, spec.Provider, spec.ModelRef); err != nil {
		return WorkerRecord{}, fmt.Errorf("execute-loop: %w", err)
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
		Effort:       spec.Effort,
		Once:         spec.Once,
		PollInterval: spec.PollInterval.String(),
		StdoutPath:   relToProject(m.projectRoot, logPath),
		SpecPath:     relToProject(m.projectRoot, filepath.Join(dir, "spec.json")),
		StartedAt:    time.Now().UTC(),
	}
	_ = m.writeRecord(dir, record)

	ctx, cancel := context.WithCancel(context.Background())
	logBuf := &bytes.Buffer{}
	multiLog := io.MultiWriter(logBuf, logFile)

	progressCh := make(chan agent.ProgressEvent, 64)
	handle := &workerHandle{
		record:       record,
		cancel:       cancel,
		logBuf:       logBuf,
		logFile:      logFile,
		progressCh:   progressCh,
		progressDone: make(chan struct{}),
		lastPhaseTS:  time.Now().UTC(),
	}

	m.mu.Lock()
	m.workers[id] = handle
	m.mu.Unlock()

	m.ensureWatchdog()

	go m.drainProgress(id, handle, progressCh)
	go m.runWorker(ctx, id, dir, spec, effectiveRoot, handle, multiLog, eventsFile, progressCh)

	return record, nil
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
		singleTierAttempt := func(ctx context.Context, beadID string, tier agent.ModelTier, resolvedHarness, resolvedModel string) (agent.ExecuteBeadReport, error) {
			gitOps := &agent.RealGitOps{}
			res, err := agent.ExecuteBead(ctx, projectRoot, beadID, agent.ExecuteBeadOptions{
				Harness:    resolvedHarness,
				Model:      resolvedModel,
				Provider:   spec.Provider,
				ModelRef:   spec.ModelRef,
				Effort:     spec.Effort,
				BeadEvents: bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			}, gitOps)
			if err != nil && res == nil {
				return agent.ExecuteBeadReport{}, err
			}
			if res != nil && res.ResultRev != "" && res.ResultRev != res.BaseRev && res.ExitCode == 0 {
				landReq := agent.BuildLandRequestFromResult(projectRoot, res)
				landRes, landErr := coordinator.Submit(landReq)
				if landErr == nil {
					agent.ApplyLandResultToExecuteBeadResult(res, landRes)
				} else if err == nil {
					err = landErr
				}
			} else if res != nil && res.ResultRev == res.BaseRev {
				res.Outcome = "no-changes"
				res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
			} else if res != nil && res.ExitCode != 0 {
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

		// escalationEnabled when neither Harness nor Model is pinned.
		escalationEnabled := spec.Harness == "" && spec.Model == ""

		executor := agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			if !escalationEnabled {
				return singleTierAttempt(ctx, beadID, "", spec.Harness, spec.Model)
			}

			// Tier escalation: cheap → standard → smart (bounded by MinTier/MaxTier).
			tiers := agent.TiersInRange(agent.ModelTier(spec.MinTier), agent.ModelTier(spec.MaxTier))
			if len(tiers) == 0 {
				return agent.ExecuteBeadReport{
					BeadID: beadID,
					Status: agent.ExecuteBeadStatusExecutionFailed,
					Detail: "execute-loop: no tiers in range (check min_tier / max_tier)",
				}, nil
			}

			beadStore := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
			svc, svcErr := agent.NewServiceFromWorkDir(projectRoot)
			if svcErr != nil {
				return agent.ExecuteBeadReport{
					BeadID: beadID,
					Status: agent.ExecuteBeadStatusExecutionFailed,
					Detail: "execute-loop: failed to initialize routing service: " + svcErr.Error(),
				}, nil
			}
			var lastReport agent.ExecuteBeadReport
			var escalationAttempts []agent.TierAttemptRecord

			for _, tier := range tiers {
				// Resolve the best harness for this tier via service.ResolveRoute.
				dec, routeErr := svc.ResolveRoute(ctx, agentlib.RouteRequest{
					ModelRef: string(tier),
					Provider: spec.Provider,
					Effort:   spec.Effort,
				})
				probeResult := "ok"
				// Treat cooldown-marked harnesses as unavailable for this tier.
				if routeErr == nil && !agent.GlobalProviderHealth.IsHealthy(dec.Harness) {
					routeErr = fmt.Errorf("provider cooldown")
				}
				if routeErr != nil {
					probeResult = "no viable provider"
					_ = beadStore.AppendEvent(beadID, bead.BeadEvent{
						Kind:      "tier-attempt",
						Summary:   "skipped",
						Body:      agent.FormatTierAttemptBody(string(tier), "", "", probeResult, "no viable harness found"),
						Actor:     "ddx",
						Source:    "ddx agent execute-loop",
						CreatedAt: time.Now().UTC(),
					})
					escalationAttempts = append(escalationAttempts, agent.TierAttemptRecord{
						Tier:   string(tier),
						Status: "skipped",
					})
					continue
				}

				report, attemptErr := singleTierAttempt(ctx, beadID, tier, dec.Harness, dec.Model)
				if attemptErr != nil {
					report = agent.ExecuteBeadReport{
						BeadID:      beadID,
						Tier:        string(tier),
						Harness:     dec.Harness,
						Model:       dec.Model,
						Status:      agent.ExecuteBeadStatusExecutionFailed,
						Detail:      attemptErr.Error(),
						ProbeResult: probeResult,
					}
				} else {
					report.ProbeResult = probeResult
				}
				lastReport = report
				escalationAttempts = append(escalationAttempts, agent.TierAttemptRecord{
					Tier:       string(tier),
					Harness:    report.Harness,
					Model:      report.Model,
					Status:     report.Status,
					CostUSD:    report.CostUSD,
					DurationMS: report.DurationMS,
				})

				_ = beadStore.AppendEvent(beadID, bead.BeadEvent{
					Kind:      "tier-attempt",
					Summary:   report.Status,
					Body:      agent.FormatTierAttemptBody(string(tier), report.Harness, report.Model, probeResult, report.Detail),
					Actor:     "ddx",
					Source:    "ddx agent execute-loop",
					CreatedAt: time.Now().UTC(),
				})

				if report.Status == agent.ExecuteBeadStatusSuccess {
					_ = agent.AppendEscalationSummaryEvent(beadStore, beadID, "ddx", escalationAttempts, string(tier), time.Now().UTC())
					return report, nil
				}
				if !agent.ShouldEscalate(report.Status) {
					_ = agent.AppendEscalationSummaryEvent(beadStore, beadID, "ddx", escalationAttempts, "", time.Now().UTC())
					return report, nil
				}
				if report.Status == agent.ExecuteBeadStatusExecutionFailed {
					agent.GlobalProviderHealth.Mark(dec.Harness, time.Now().Add(agent.ProviderCooldownDuration))
				}
			}

			_ = agent.AppendEscalationSummaryEvent(beadStore, beadID, "ddx", escalationAttempts, "", time.Now().UTC())

			if lastReport.BeadID == "" {
				return agent.ExecuteBeadReport{
					BeadID: beadID,
					Status: agent.ExecuteBeadStatusExecutionFailed,
					Detail: "execute-loop: all tiers exhausted — no viable provider found",
				}, nil
			}
			lastReport.Detail = "escalation exhausted: " + lastReport.Detail
			return lastReport, nil
		})

		// Build post-merge reviewer. On-by-default unless NoReview is set in spec.
		var reviewer agent.BeadReviewer
		if !spec.NoReview {
			reviewer = &agent.DefaultBeadReviewer{
				ProjectRoot: projectRoot,
				BeadStore:   bead.NewStore(filepath.Join(projectRoot, ".ddx")),
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

	landingOps := agent.RealLandingGitOps{}
	loopResult, err := worker.Run(ctx, agent.ExecuteBeadLoopOptions{
		Assignee:     "ddx",
		Once:         spec.Once,
		PollInterval: spec.PollInterval,
		Log:          log,
		EventSink:    eventSink,
		WorkerID:     id,
		ProjectRoot:  projectRoot,
		Harness:      spec.Harness,
		Model:        spec.Model,
		Provider:     spec.Provider,
		ModelRef:     spec.ModelRef,
		ProgressCh:   progressCh,
		PreClaimHook: buildPreClaimHook(projectRoot, landingOps),
		NoReview:     spec.NoReview,
		MinTier:      spec.MinTier,
		MaxTier:      spec.MaxTier,
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
func (m *WorkerManager) Stop(id string) error {
	m.mu.Lock()
	handle := m.workers[id]
	if handle == nil || handle.cancel == nil {
		m.mu.Unlock()
		return fmt.Errorf("worker not running")
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
// The ddx-agent library writes per-iteration entries to .ddx/agent-logs/agent-*.jsonl
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

func relToProject(projectRoot, path string) string {
	rel, err := filepath.Rel(projectRoot, path)
	if err != nil {
		return path
	}
	return rel
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
