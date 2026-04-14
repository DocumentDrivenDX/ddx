package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

type ExecuteBeadReport struct {
	BeadID      string `json:"bead_id"`
	AttemptID   string `json:"attempt_id,omitempty"`
	WorkerID    string `json:"worker_id,omitempty"`
	Harness     string `json:"harness,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
	Status      string `json:"status"`
	Detail      string `json:"detail,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	BaseRev     string `json:"base_rev,omitempty"`
	ResultRev   string `json:"result_rev,omitempty"`
	PreserveRef string `json:"preserve_ref,omitempty"`
	RetryAfter  string `json:"retry_after,omitempty"`
}

type ExecuteBeadExecutor interface {
	Execute(ctx context.Context, beadID string) (ExecuteBeadReport, error)
}

type ExecuteBeadExecutorFunc func(ctx context.Context, beadID string) (ExecuteBeadReport, error)

func (f ExecuteBeadExecutorFunc) Execute(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
	return f(ctx, beadID)
}

// SatisfactionChecker evaluates whether a bead that returned no_changes is
// already satisfied and should be closed, or is still unresolved and should
// receive retry suppression. noChangesCount is the cumulative count including
// the current attempt.
//
// When satisfied is true the caller closes the bead with the returned evidence
// string as the detail. When false the caller applies a retry cooldown and
// leaves the bead open.
type SatisfactionChecker interface {
	CheckSatisfied(ctx context.Context, beadID string, noChangesCount int) (satisfied bool, evidence string, err error)
}

// SatisfactionCheckerFunc is a functional adapter for SatisfactionChecker.
type SatisfactionCheckerFunc func(ctx context.Context, beadID string, noChangesCount int) (bool, string, error)

func (f SatisfactionCheckerFunc) CheckSatisfied(ctx context.Context, beadID string, noChangesCount int) (bool, string, error) {
	return f(ctx, beadID, noChangesCount)
}

type ExecuteBeadLoopStore interface {
	ReadyExecution() ([]bead.Bead, error)
	Claim(id, assignee string) error
	Unclaim(id string) error
	Heartbeat(id string) error
	CloseWithEvidence(id, sessionID, commitSHA string) error
	AppendEvent(id string, event bead.BeadEvent) error
	SetExecutionCooldown(id string, until time.Time, status, detail string) error
	IncrNoChangesCount(id string) (int, error)
}

// ProgressEvent is the FEAT-006 structured progress event. It is defined
// separately in the server package (server.ProgressEvent); this alias lets
// the agent package emit events without importing the server package.
// The field names and types are identical — the server package deserialises
// these directly from the channel.
//
// Terminal phases: done, preserved, failed.
type ProgressEvent struct {
	EventID   string    `json:"event_id"`
	AttemptID string    `json:"attempt_id"`
	WorkerID  string    `json:"worker_id"`
	ProjectID string    `json:"project_id"`
	BeadID    string    `json:"bead_id"`
	Harness   string    `json:"harness,omitempty"`
	Model     string    `json:"model,omitempty"`
	Profile   string    `json:"profile,omitempty"`
	Phase     string    `json:"phase"`
	PhaseSeq  int       `json:"phase_seq"`
	Heartbeat bool      `json:"heartbeat"`
	TS        time.Time `json:"ts"`
	ElapsedMS int64     `json:"elapsed_ms"`
	Message   string    `json:"message,omitempty"`
}

type ExecuteBeadLoopOptions struct {
	Assignee                string
	Once                    bool
	PollInterval            time.Duration
	NoProgressCooldown      time.Duration
	MaxNoChangesBeforeClose int
	// HeartbeatInterval, if > 0, overrides bead.HeartbeatInterval for this
	// worker's claim heartbeat loop. Tests use this to shorten the tick.
	HeartbeatInterval time.Duration
	Log               io.Writer

	// EventSink receives structured JSONL progress events emitted at
	// loop.start, bead.claimed, bead.result, and loop.end milestones.
	// When nil, no structured events are written. Log (terminal text)
	// is independent and still emitted for human operators.
	EventSink io.Writer

	// ProgressCh, when non-nil, receives FEAT-006 ProgressEvents for each
	// bead execution managed by this loop. The caller is responsible for
	// draining the channel; the loop sends non-blocking (events are dropped
	// if the channel is full). The loop does NOT close this channel; the
	// caller (WorkerManager.runWorker) closes it after Run returns.
	ProgressCh chan<- ProgressEvent

	// Worker/session metadata included in loop.start events so log
	// aggregators can correlate structured output with the executing
	// harness/worker. None of these are required.
	WorkerID    string
	ProjectRoot string
	Harness     string
	Model       string
	SessionID   string
}

type ExecuteBeadLoopResult struct {
	Attempts          int                 `json:"attempts"`
	Successes         int                 `json:"successes"`
	Failures          int                 `json:"failures"`
	NoReadyWork       bool                `json:"no_ready_work,omitempty"`
	LastSuccessAt     time.Time           `json:"last_success_at,omitempty"`
	LastFailureStatus string              `json:"last_failure_status,omitempty"`
	Results           []ExecuteBeadReport `json:"results,omitempty"`
}

// ExecuteBeadWorker drains the current single-project execution-ready queue.
// It intentionally does not retry a failed/conflicted bead again in the same
// process run; a later operator-driven invocation can create the next attempt.
type ExecuteBeadWorker struct {
	Store               ExecuteBeadLoopStore
	Executor            ExecuteBeadExecutor
	SatisfactionChecker SatisfactionChecker // nil → count-based default
	Now                 func() time.Time
}

// emitProgress sends a ProgressEvent to opts.ProgressCh non-blocking.
// If ch is nil or full the event is silently dropped.
func emitProgress(ch chan<- ProgressEvent, evt ProgressEvent) {
	if ch == nil {
		return
	}
	select {
	case ch <- evt:
	default:
	}
}

// newProgressEvent builds a ProgressEvent with a random event_id and current timestamp.
func newProgressEvent(workerID, projectID, beadID, attemptID, phase string, phaseSeq int, heartbeat bool, elapsedMS int64, opts ExecuteBeadLoopOptions) ProgressEvent {
	return ProgressEvent{
		EventID:   "evt-" + randomProgressID(),
		AttemptID: attemptID,
		WorkerID:  workerID,
		ProjectID: projectID,
		BeadID:    beadID,
		Harness:   opts.Harness,
		Model:     opts.Model,
		Phase:     phase,
		PhaseSeq:  phaseSeq,
		Heartbeat: heartbeat,
		TS:        time.Now().UTC(),
		ElapsedMS: elapsedMS,
	}
}

func randomProgressID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())[:8]
}

func (w *ExecuteBeadWorker) Run(ctx context.Context, opts ExecuteBeadLoopOptions) (*ExecuteBeadLoopResult, error) {
	if w.Store == nil {
		return nil, fmt.Errorf("execute-bead loop: store is required")
	}
	if w.Executor == nil {
		return nil, fmt.Errorf("execute-bead loop: executor is required")
	}

	now := w.Now
	if now == nil {
		now = time.Now
	}
	assignee := opts.Assignee
	if assignee == "" {
		assignee = "ddx"
	}
	noProgressCooldown := opts.NoProgressCooldown
	if noProgressCooldown <= 0 {
		noProgressCooldown = 6 * time.Hour
	}
	maxNoChangesBeforeClose := opts.MaxNoChangesBeforeClose
	if maxNoChangesBeforeClose <= 0 {
		maxNoChangesBeforeClose = 3
	}
	heartbeatInterval := opts.HeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = bead.HeartbeatInterval
	}

	result := &ExecuteBeadLoopResult{}
	attempted := make(map[string]struct{})

	emit := func(eventType string, data map[string]any) {
		writeLoopEvent(opts.EventSink, opts.SessionID, eventType, data, now().UTC())
	}

	emit("loop.start", map[string]any{
		"worker_id":    opts.WorkerID,
		"project_root": opts.ProjectRoot,
		"harness":      opts.Harness,
		"model":        opts.Model,
		"session_id":   opts.SessionID,
		"assignee":     assignee,
		"once":         opts.Once,
	})
	defer func() {
		emit("loop.end", map[string]any{
			"attempts":            result.Attempts,
			"successes":           result.Successes,
			"failures":            result.Failures,
			"last_failure_status": result.LastFailureStatus,
		})
	}()

	for {
		// Respect context cancellation between iterations. Without this check,
		// a Stop() request (which cancels ctx) would only take effect during
		// the idle poll sleep — the loop would happily claim the next ready
		// bead as soon as the current Execute returned, ignoring the cancel.
		if err := ctx.Err(); err != nil {
			return result, err
		}

		candidate, ok, err := w.nextCandidate(attempted)
		if err != nil {
			return result, err
		}
		if !ok {
			if result.Attempts == 0 {
				result.NoReadyWork = true
			}
			if opts.PollInterval <= 0 {
				return result, nil
			}
			if err := sleepWithContext(ctx, opts.PollInterval); err != nil {
				return result, err
			}
			continue
		}

		attempted[candidate.ID] = struct{}{}
		if err := w.Store.Claim(candidate.ID, assignee); err != nil {
			continue
		}

		emit("bead.claimed", map[string]any{
			"bead_id":  candidate.ID,
			"title":    candidate.Title,
			"assignee": assignee,
		})

		if opts.Log != nil {
			if candidate.Title != "" {
				_, _ = fmt.Fprintf(opts.Log, "\n▶ %s: %s\n", candidate.ID, candidate.Title)
			} else {
				_, _ = fmt.Fprintf(opts.Log, "\n▶ %s\n", candidate.ID)
			}
		}

		// Generate a provisional attempt_id for progress events.
		// The real attempt_id is assigned inside ExecuteBead; we use this
		// for queueing/running events and replace with the real one once known.
		provAttemptID := time.Now().UTC().Format("20060102T150405") + "-" + randomProgressID()
		runStart := now()
		phaseSeq := 0
		nextPhase := func(phase string, heartbeat bool) {
			phaseSeq++
			emitProgress(opts.ProgressCh, newProgressEvent(
				opts.WorkerID, opts.ProjectRoot, candidate.ID, provAttemptID,
				phase, phaseSeq, heartbeat, now().Sub(runStart).Milliseconds(), opts,
			))
		}

		nextPhase("queueing", false)

		hbCtx, hbCancel := context.WithCancel(ctx)
		var hbWG sync.WaitGroup
		hbWG.Add(1)
		go func(beadID string) {
			defer hbWG.Done()
			ticker := time.NewTicker(heartbeatInterval)
			defer ticker.Stop()
			for {
				select {
				case <-hbCtx.Done():
					return
				case <-ticker.C:
					_ = w.Store.Heartbeat(beadID)
				}
			}
		}(candidate.ID)

		nextPhase("running", false)

		report, err := w.Executor.Execute(ctx, candidate.ID)
		hbCancel()
		hbWG.Wait()
		if err != nil {
			report = ExecuteBeadReport{
				BeadID: candidate.ID,
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: err.Error(),
			}
		}
		if report.BeadID == "" {
			report.BeadID = candidate.ID
		}
		if report.Status == "" {
			report.Status = ExecuteBeadStatusExecutionFailed
		}
		if report.Detail == "" {
			report.Detail = ExecuteBeadStatusDetail(report.Status, "", "")
		}

		result.Attempts++

		if report.Status == ExecuteBeadStatusSuccess {
			if err := w.Store.CloseWithEvidence(candidate.ID, report.SessionID, report.ResultRev); err != nil {
				return result, err
			}
			result.Successes++
			result.LastSuccessAt = now().UTC()
		} else {
			if err := w.Store.Unclaim(candidate.ID); err != nil {
				return result, err
			}
			if report.Status == ExecuteBeadStatusNoChanges {
				count, cerr := w.Store.IncrNoChangesCount(candidate.ID)
				if cerr != nil {
					return result, cerr
				}
				satisfied, evidence, aerr := w.adjudicateNoChanges(ctx, candidate.ID, count, maxNoChangesBeforeClose)
				if aerr != nil {
					return result, aerr
				}
				if satisfied {
					// Adjudication confirmed bead is already satisfied: close
					// with accumulated no-changes evidence so the queue drains.
					if cerr := w.Store.CloseWithEvidence(candidate.ID, report.SessionID, report.BaseRev); cerr != nil {
						return result, cerr
					}
					report.Status = ExecuteBeadStatusAlreadySatisfied
					if evidence != "" {
						// Checker evidence explains why the bead is being closed;
						// it takes precedence over the executor's attempt detail.
						report.Detail = evidence
					}
					result.Successes++
					result.LastSuccessAt = now().UTC()
				} else {
					// Unresolved: suppress immediate retry so the queue can
					// move on to other beads.
					if shouldSuppressNoProgress(report) {
						retryAfter := now().UTC().Add(noProgressCooldown)
						if cerr := w.Store.SetExecutionCooldown(candidate.ID, retryAfter, report.Status, report.Detail); cerr != nil {
							return result, cerr
						}
						report.RetryAfter = retryAfter.Format(time.RFC3339)
					}
					result.Failures++
					result.LastFailureStatus = report.Status
				}
			} else {
				if shouldSuppressNoProgress(report) {
					retryAfter := now().UTC().Add(noProgressCooldown)
					if err := w.Store.SetExecutionCooldown(candidate.ID, retryAfter, report.Status, report.Detail); err != nil {
						return result, err
					}
					report.RetryAfter = retryAfter.Format(time.RFC3339)
				}
				result.Failures++
				result.LastFailureStatus = report.Status
			}
		}

		result.Results = append(result.Results, report)

		if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC())); err != nil {
			return result, err
		}

		// Emit terminal progress phase event.
		terminalPhase := "failed"
		if report.Status == ExecuteBeadStatusSuccess || report.Status == ExecuteBeadStatusAlreadySatisfied {
			terminalPhase = "done"
		} else if report.PreserveRef != "" {
			terminalPhase = "preserved"
		}
		// Use the real attempt_id from the report if available.
		finalAttemptID := report.AttemptID
		if finalAttemptID == "" {
			finalAttemptID = provAttemptID
		}
		phaseSeq++
		emitProgress(opts.ProgressCh, ProgressEvent{
			EventID:   "evt-" + randomProgressID(),
			AttemptID: finalAttemptID,
			WorkerID:  opts.WorkerID,
			ProjectID: opts.ProjectRoot,
			BeadID:    candidate.ID,
			Harness:   opts.Harness,
			Model:     opts.Model,
			Phase:     terminalPhase,
			PhaseSeq:  phaseSeq,
			Heartbeat: false,
			TS:        now().UTC(),
			ElapsedMS: now().Sub(runStart).Milliseconds(),
			Message:   report.Detail,
		})

		emit("bead.result", map[string]any{
			"bead_id":      candidate.ID,
			"status":       report.Status,
			"detail":       report.Detail,
			"session_id":   report.SessionID,
			"result_rev":   report.ResultRev,
			"base_rev":     report.BaseRev,
			"preserve_ref": report.PreserveRef,
			"duration_ms":  now().Sub(runStart).Milliseconds(),
		})

		if opts.Log != nil {
			_, _ = fmt.Fprintf(opts.Log, "✓ %s → %s\n", candidate.ID, formatLoopResult(report))
		}

		if opts.Once {
			return result, nil
		}
	}
}

func (w *ExecuteBeadWorker) nextCandidate(attempted map[string]struct{}) (bead.Bead, bool, error) {
	ready, err := w.Store.ReadyExecution()
	if err != nil {
		return bead.Bead{}, false, err
	}
	for _, candidate := range ready {
		if _, seen := attempted[candidate.ID]; seen {
			continue
		}
		return candidate, true, nil
	}
	return bead.Bead{}, false, nil
}

func executeBeadLoopEvent(report ExecuteBeadReport, actor string, createdAt time.Time) bead.BeadEvent {
	parts := []string{}
	if report.Detail != "" {
		parts = append(parts, report.Detail)
	}
	if report.PreserveRef != "" {
		parts = append(parts, fmt.Sprintf("preserve_ref=%s", report.PreserveRef))
	}
	if report.ResultRev != "" {
		parts = append(parts, fmt.Sprintf("result_rev=%s", report.ResultRev))
	}
	if report.BaseRev != "" {
		parts = append(parts, fmt.Sprintf("base_rev=%s", report.BaseRev))
	}
	if report.RetryAfter != "" {
		parts = append(parts, fmt.Sprintf("retry_after=%s", report.RetryAfter))
	}

	return bead.BeadEvent{
		Kind:      "execute-bead",
		Summary:   report.Status,
		Body:      strings.Join(parts, "\n"),
		Actor:     actor,
		Source:    "ddx agent execute-loop",
		CreatedAt: createdAt,
	}
}

// writeLoopEvent emits one structured JSONL line to sink describing a
// milestone in an execute-bead loop run. Entries use the same envelope as
// the ddx-agent harness (session_id/seq/type/ts/data) so existing log
// aggregators (FormatSessionLogLines, ddx server workers log) can parse
// the stream uniformly. Errors are swallowed: structured logging must
// never break the core execute-loop.
func writeLoopEvent(sink io.Writer, sessionID, eventType string, data map[string]any, ts time.Time) {
	if sink == nil {
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	entry := map[string]any{
		"session_id": sessionID,
		"type":       eventType,
		"ts":         ts.UTC().Format(time.RFC3339Nano),
		"data":       data,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = sink.Write(line)
	_, _ = sink.Write([]byte("\n"))
}

// formatLoopResult returns a concise human-readable summary of a bead execution
// result using merged/preserved/error terminology instead of raw status codes.
func formatLoopResult(report ExecuteBeadReport) string {
	switch report.Status {
	case ExecuteBeadStatusSuccess:
		shortRev := report.ResultRev
		if len(shortRev) > 8 {
			shortRev = shortRev[:8]
		}
		if shortRev != "" {
			return fmt.Sprintf("merged (%s)", shortRev)
		}
		return "merged"
	case ExecuteBeadStatusAlreadySatisfied:
		return "already_satisfied"
	case ExecuteBeadStatusNoChanges:
		return "no_changes"
	default:
		detail := report.Detail
		if detail == "" {
			detail = report.Status
		}
		if report.PreserveRef != "" {
			return fmt.Sprintf("preserved: %s", detail)
		}
		return fmt.Sprintf("error: %s", detail)
	}
}

// adjudicateNoChanges runs the no-change adjudication step for a bead.
// It returns (satisfied, evidence, err). When satisfied is true the bead
// should be closed as already_satisfied with the evidence string. When false
// retry suppression (cooldown) should be applied and the bead left open.
//
// If a SatisfactionChecker is configured it is called first. Otherwise the
// default count-based rule closes the bead once noChangesCount reaches
// maxNoChangesBeforeClose.
func (w *ExecuteBeadWorker) adjudicateNoChanges(ctx context.Context, beadID string, noChangesCount, maxNoChangesBeforeClose int) (bool, string, error) {
	if w.SatisfactionChecker != nil {
		return w.SatisfactionChecker.CheckSatisfied(ctx, beadID, noChangesCount)
	}
	if noChangesCount >= maxNoChangesBeforeClose {
		return true, fmt.Sprintf("no_changes on %d consecutive attempt(s); bead treated as already satisfied", noChangesCount), nil
	}
	return false, "", nil
}

func shouldSuppressNoProgress(report ExecuteBeadReport) bool {
	if report.BaseRev == "" || report.ResultRev == "" {
		return false
	}
	return report.BaseRev == report.ResultRev
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
