package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
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
	// NoChangesRationale carries the agent's explanation when status == no_changes.
	NoChangesRationale string `json:"no_changes_rationale,omitempty"`
	// ReviewVerdict is the post-merge review verdict (APPROVE, REQUEST_CHANGES,
	// or BLOCK) when a reviewer ran. Empty when review was skipped.
	ReviewVerdict string `json:"review_verdict,omitempty"`
	// Tier is the model tier used for the final attempt (cheap, standard, smart).
	// Populated by tier-escalating executors; empty for single-tier attempts.
	Tier string `json:"tier,omitempty"`
	// ProbeResult is a brief summary of the provider health probe at attempt time.
	ProbeResult string `json:"probe_result,omitempty"`
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
	// Reopen sets a closed bead back to open, appending notes to the bead's
	// Notes field and recording a reopen event. Used by the post-merge review
	// step when the reviewer returns REQUEST_CHANGES or BLOCK.
	Reopen(id, reason, notes string) error
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
	// NoReview, when true, skips the post-merge review step even when
	// ExecuteBeadWorker.Reviewer is configured. Use for doc-only beads or
	// tight iteration loops where review latency is not acceptable.
	NoReview bool

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

	// PreClaimHook, when non-nil, is called before Store.Claim for each
	// candidate bead. If it returns an error the bead is not claimed and the
	// loop continues to the next iteration (ctx is NOT cancelled). A nil hook
	// disables the check. Server and CLI paths wire a real implementation
	// backed by LandingGitOps.FetchOriginAncestryCheck; tests may inject nil
	// or a stub that always returns nil.
	PreClaimHook func(ctx context.Context) error

	// Worker/session metadata included in loop.start events so log
	// aggregators can correlate structured output with the executing
	// harness/worker. None of these are required.
	WorkerID    string
	ProjectRoot string
	Harness     string
	Model       string
	Provider    string
	ModelRef    string
	SessionID   string

	// MinTier and MaxTier bound the tier escalation range when the executor
	// uses tier-based auto-escalation. Empty string uses the defaults (cheap
	// and smart). Ignored when the executor has escalation disabled (e.g. an
	// explicit --harness or --model override was specified).
	MinTier string
	MaxTier string
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
	// Reviewer, when non-nil, is called after every successful merge to
	// validate the commit against the bead's acceptance criteria. When nil,
	// post-merge review is skipped (same behaviour as --no-review).
	Reviewer BeadReviewer
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

		// Pre-claim hook: fetch origin + verify ancestry before claiming.
		// On error the bead is skipped for this iteration; the loop
		// continues (ctx is not cancelled).
		if opts.PreClaimHook != nil {
			if hookErr := opts.PreClaimHook(ctx); hookErr != nil {
				if opts.Log != nil {
					_, _ = fmt.Fprintf(opts.Log, "pre-claim hook: %v (skipping %s)\n", hookErr, candidate.ID)
				}
				emit("preclaim.skipped", map[string]any{
					"bead_id": candidate.ID,
					"reason":  hookErr.Error(),
				})
				continue
			}
		}

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

			// Post-merge review — on-by-default when Reviewer is configured.
			// Skip when: --no-review flag, "review:skip" bead label, or no reviewer.
			reviewApproved := true
			if w.Reviewer != nil && !opts.NoReview && !HasBeadLabel(candidate.Labels, "review:skip") {
				reviewRes, reviewErr := w.Reviewer.ReviewBead(ctx, candidate.ID, report.ResultRev, report.Harness, report.Model)
				if reviewErr != nil {
					// Review error: log event but leave bead closed (don't block on reviewer failures).
					_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
						Kind:      "review-error",
						Summary:   "review agent error",
						Body:      reviewErr.Error(),
						Actor:     assignee,
						Source:    "ddx agent execute-loop",
						CreatedAt: now().UTC(),
					})
				} else {
					report.ReviewVerdict = string(reviewRes.Verdict)
					switch reviewRes.Verdict {
					case VerdictApprove:
						// Approved: attach evidence and leave bead closed.
						_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
							Kind:      "review",
							Summary:   "APPROVE",
							Body:      reviewRes.RawOutput,
							Actor:     assignee,
							Source:    "ddx agent execute-loop",
							CreatedAt: now().UTC(),
						})
					case VerdictRequestChanges:
						// Needs fixes: reopen with review findings in notes.
						if reopenErr := w.Store.Reopen(candidate.ID, "review: REQUEST_CHANGES", reviewRes.RawOutput); reopenErr != nil {
							return result, reopenErr
						}
						report.Status = ExecuteBeadStatusReviewRequestChanges
						report.Detail = "post-merge review: REQUEST_CHANGES"
						reviewApproved = false
					case VerdictBlock:
						// Cannot proceed: reopen and flag for human with BLOCK marker.
						blockNotes := "REVIEW:BLOCK\n\n" + reviewRes.RawOutput
						if reopenErr := w.Store.Reopen(candidate.ID, "review: BLOCK", blockNotes); reopenErr != nil {
							return result, reopenErr
						}
						report.Status = ExecuteBeadStatusReviewBlock
						report.Detail = "post-merge review: BLOCK (flagged for human)"
						reviewApproved = false
					}
				}
			}

			if reviewApproved {
				result.Successes++
				result.LastSuccessAt = now().UTC()
			} else {
				result.Failures++
				result.LastFailureStatus = report.Status
			}
		} else {
			if err := w.Store.Unclaim(candidate.ID); err != nil {
				return result, err
			}
			if report.Status == ExecuteBeadStatusNoChanges {
				count, cerr := w.Store.IncrNoChangesCount(candidate.ID)
				if cerr != nil {
					return result, cerr
				}
				satisfied, evidence, aerr := w.adjudicateNoChanges(ctx, candidate.ID, count, maxNoChangesBeforeClose, report.NoChangesRationale)
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
			"bead_id":              candidate.ID,
			"status":               report.Status,
			"detail":               report.Detail,
			"session_id":           report.SessionID,
			"result_rev":           report.ResultRev,
			"base_rev":             report.BaseRev,
			"preserve_ref":         report.PreserveRef,
			"no_changes_rationale": report.NoChangesRationale,
			"duration_ms":          now().Sub(runStart).Milliseconds(),
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
	if report.Tier != "" {
		parts = append(parts, fmt.Sprintf("tier=%s", report.Tier))
	}
	if report.ProbeResult != "" {
		parts = append(parts, fmt.Sprintf("probe_result=%s", report.ProbeResult))
	}
	if report.NoChangesRationale != "" {
		parts = append(parts, fmt.Sprintf("rationale: %s", report.NoChangesRationale))
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
		if report.NoChangesRationale != "" {
			return fmt.Sprintf("no_changes: %s", report.NoChangesRationale)
		}
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

// reCommitSHA matches a 7-to-40 character lowercase hex string that looks like a
// git commit SHA. Used to detect whether a no_changes rationale cites a prior commit.
var reCommitSHA = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)

// reTestFuncName matches a Go test function name (TestXxx or BenchmarkXxx).
var reTestFuncName = regexp.MustCompile(`\b(?:Test|Benchmark)[A-Z]\w*\b`)

// rationaleIsSpecific returns true when the rationale string contains a reference
// specific enough to treat a no_changes outcome as already_satisfied on the first
// attempt. Currently this means: the rationale cites a commit SHA (7+ hex chars)
// or a Go test function name. Vague rationales ("nothing to do") return false.
func rationaleIsSpecific(rationale string) bool {
	if rationale == "" {
		return false
	}
	return reCommitSHA.MatchString(rationale) || reTestFuncName.MatchString(rationale)
}

// adjudicateNoChanges runs the no-change adjudication step for a bead.
// It returns (satisfied, evidence, err). When satisfied is true the bead
// should be closed as already_satisfied with the evidence string. When false
// retry suppression (cooldown) should be applied and the bead left open.
//
// If a SatisfactionChecker is configured it is called first. Otherwise:
//   - When the report carries a specific rationale (cites a commit SHA or test
//     name), the bead is closed as already_satisfied on the first occurrence.
//   - Otherwise the default count-based rule applies (close after maxNoChangesBeforeClose).
func (w *ExecuteBeadWorker) adjudicateNoChanges(ctx context.Context, beadID string, noChangesCount, maxNoChangesBeforeClose int, rationale string) (bool, string, error) {
	if w.SatisfactionChecker != nil {
		return w.SatisfactionChecker.CheckSatisfied(ctx, beadID, noChangesCount)
	}
	if rationaleIsSpecific(rationale) {
		evidence := rationale
		return true, evidence, nil
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
