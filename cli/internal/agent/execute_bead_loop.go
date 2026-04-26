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
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// ExecuteBeadLoopRuntime carries the non-serializable plumbing and
// per-invocation runtime intent for an execute-bead loop run. Durable knobs
// (assignee, retry caps, harness/model, tier bounds, etc.) live on
// config.ResolvedConfig and are passed via Run's rcfg argument.
type ExecuteBeadLoopRuntime struct {
	Log          io.Writer
	EventSink    io.Writer
	ProgressCh   chan<- ProgressEvent
	PreClaimHook func(ctx context.Context) error
	Once         bool
	PollInterval time.Duration
	NoReview     bool
	LabelFilter  string
	SessionID    string
	WorkerID     string
	ProjectRoot  string
}

// DefaultReviewMaxRetries is the number of reviewer attempts allowed per
// committed result_rev before the loop emits a terminal
// `review-manual-required` event and stops re-executing primary. FEAT-022 §14.
const DefaultReviewMaxRetries = 3

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
	// ReviewRationale carries the actionable reviewer-authored findings for
	// non-APPROVE review outcomes.
	ReviewRationale string `json:"review_rationale,omitempty"`
	// Tier is the model tier used for the final attempt (cheap, standard, smart).
	// Populated by tier-escalating executors; empty for single-tier attempts.
	Tier string `json:"tier,omitempty"`
	// ProbeResult is a brief summary of the provider health probe at attempt time.
	ProbeResult string `json:"probe_result,omitempty"`
	// CostUSD is the dollar cost of this attempt as reported by the harness.
	// Tier-escalating executors propagate this so the escalation trace can
	// compute wasted/effective spend.
	CostUSD float64 `json:"cost_usd,omitempty"`
	// DurationMS is the wall-clock duration of this attempt.
	DurationMS int64 `json:"duration_ms,omitempty"`
	// Profile routing telemetry. Populated when execute-loop uses a profile
	// ladder rather than an explicit harness/model pin.
	RequestedProfile string `json:"requested_profile,omitempty"`
	RequestedTier    string `json:"requested_tier,omitempty"`
	ResolvedTier     string `json:"resolved_tier,omitempty"`
	EscalationCount  int    `json:"escalation_count,omitempty"`
	FinalTier        string `json:"final_tier,omitempty"`
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
	Events(id string) ([]bead.BeadEvent, error)
	SetExecutionCooldown(id string, until time.Time, status, detail string) error
	IncrNoChangesCount(id string) (int, error)
	// Reopen sets a closed bead back to open, appending notes to the bead's
	// Notes field and recording a reopen event. Used by the post-merge review
	// step when the reviewer returns REQUEST_CHANGES or BLOCK.
	Reopen(id, reason, notes string) error
}

// readyDiagnoser is the optional interface the work loop uses to explain an
// empty execution queue. bead.Store satisfies it via ReadyExecutionBreakdown.
type readyDiagnoser interface {
	ReadyExecutionBreakdown() (bead.ReadyExecutionBreakdown, error)
}

// NoReadyWorkBreakdown explains why the execution-ready queue is empty when
// dependency-ready beads exist. Populated on an ExecuteBeadLoopResult when
// NoReadyWork fires and the store exposes ReadyExecutionBreakdown.
type NoReadyWorkBreakdown struct {
	SkippedEpics       []string `json:"skipped_epics,omitempty"`
	SkippedOnCooldown  []string `json:"skipped_on_cooldown,omitempty"`
	SkippedNotEligible []string `json:"skipped_not_eligible,omitempty"`
	SkippedSuperseded  []string `json:"skipped_superseded,omitempty"`
	NextRetryAfter     string   `json:"next_retry_after,omitempty"`
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

type ExecuteBeadLoopResult struct {
	Attempts          int                  `json:"attempts"`
	Successes         int                  `json:"successes"`
	Failures          int                  `json:"failures"`
	NoReadyWork       bool                 `json:"no_ready_work,omitempty"`
	NoReadyWorkDetail NoReadyWorkBreakdown `json:"no_ready_work_detail,omitempty"`
	LastSuccessAt     time.Time            `json:"last_success_at,omitempty"`
	LastFailureStatus string               `json:"last_failure_status,omitempty"`
	Results           []ExecuteBeadReport  `json:"results,omitempty"`
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

// emitProgress sends a ProgressEvent to runtime.ProgressCh non-blocking.
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
func newProgressEvent(workerID, projectID, beadID, attemptID, harness, model, profile, phase string, phaseSeq int, heartbeat bool, elapsedMS int64) ProgressEvent {
	return ProgressEvent{
		EventID:   "evt-" + randomProgressID(),
		AttemptID: attemptID,
		WorkerID:  workerID,
		ProjectID: projectID,
		BeadID:    beadID,
		Harness:   harness,
		Model:     model,
		Profile:   profile,
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

func (w *ExecuteBeadWorker) Run(ctx context.Context, rcfg config.ResolvedConfig, runtime ExecuteBeadLoopRuntime) (*ExecuteBeadLoopResult, error) {
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
	assignee := rcfg.Assignee()
	if assignee == "" {
		assignee = "ddx"
	}
	noProgressCooldown := rcfg.NoProgressCooldown()
	if noProgressCooldown <= 0 {
		noProgressCooldown = 6 * time.Hour
	}
	maxNoChangesBeforeClose := rcfg.MaxNoChangesBeforeClose()
	if maxNoChangesBeforeClose <= 0 {
		maxNoChangesBeforeClose = 3
	}
	heartbeatInterval := rcfg.HeartbeatInterval()
	if heartbeatInterval <= 0 {
		heartbeatInterval = bead.HeartbeatInterval
	}
	harness := rcfg.Harness()
	model := rcfg.Model()
	profile := rcfg.Profile()

	result := &ExecuteBeadLoopResult{}
	attempted := make(map[string]struct{})

	emit := func(eventType string, data map[string]any) {
		writeLoopEvent(runtime.EventSink, runtime.SessionID, eventType, data, now().UTC())
	}

	emit("loop.start", map[string]any{
		"worker_id":    runtime.WorkerID,
		"project_root": runtime.ProjectRoot,
		"harness":      harness,
		"model":        model,
		"session_id":   runtime.SessionID,
		"assignee":     assignee,
		"once":         runtime.Once,
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

		candidate, ok, err := w.nextCandidate(attempted, runtime.LabelFilter)
		if err != nil {
			return result, err
		}
		if !ok {
			if result.Attempts == 0 {
				result.NoReadyWork = true
				if diag, ok := w.Store.(readyDiagnoser); ok {
					if breakdown, bErr := diag.ReadyExecutionBreakdown(); bErr == nil {
						result.NoReadyWorkDetail = NoReadyWorkBreakdown{
							SkippedEpics:       breakdown.SkippedEpics,
							SkippedOnCooldown:  breakdown.SkippedOnCooldown,
							SkippedNotEligible: breakdown.SkippedNotEligible,
							SkippedSuperseded:  breakdown.SkippedSuperseded,
							NextRetryAfter:     breakdown.NextRetryAfter,
						}
					}
				}
			}
			if runtime.PollInterval <= 0 {
				return result, nil
			}
			if err := sleepWithContext(ctx, runtime.PollInterval); err != nil {
				return result, err
			}
			continue
		}

		attempted[candidate.ID] = struct{}{}

		// Pre-claim hook: fetch origin + verify ancestry before claiming.
		// On error the bead is skipped for this iteration; the loop
		// continues (ctx is not cancelled).
		if runtime.PreClaimHook != nil {
			if hookErr := runtime.PreClaimHook(ctx); hookErr != nil {
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "pre-claim hook: %v (skipping %s)\n", hookErr, candidate.ID)
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

		if runtime.Log != nil {
			if candidate.Title != "" {
				_, _ = fmt.Fprintf(runtime.Log, "\n▶ %s: %s\n", candidate.ID, candidate.Title)
			} else {
				_, _ = fmt.Fprintf(runtime.Log, "\n▶ %s\n", candidate.ID)
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
			emitProgress(runtime.ProgressCh, newProgressEvent(
				runtime.WorkerID, runtime.ProjectRoot, candidate.ID, provAttemptID,
				harness, model, profile,
				phase, phaseSeq, heartbeat, now().Sub(runStart).Milliseconds(),
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
			appendLoopRoutingEvidence(w.Store, candidate.ID, report, now().UTC())
			// Close the bead early when review is skipped. The closure gate
			// (ddx-e30e60a9) accepts this path because closing_commit_sha is
			// set and there is no malformed-APPROVE event to reject.
			reviewApproved := true
			reviewSkipped := w.Reviewer == nil || runtime.NoReview || HasBeadLabel(candidate.Labels, "review:skip")

			if reviewSkipped {
				if err := w.Store.CloseWithEvidence(candidate.ID, report.SessionID, report.ResultRev); err != nil {
					return result, err
				}
			}

			if w.Reviewer != nil && !runtime.NoReview && !HasBeadLabel(candidate.Labels, "review:skip") {
				reviewRes, reviewErr := w.Reviewer.ReviewBead(ctx, candidate.ID, report.ResultRev, report.Harness, report.Model)
				if reviewErr != nil {
					// FEAT-022 §12+§14: classify the failure into the four-class
					// taxonomy and count prior review-error events scoped to the
					// current result_rev. On the Nth failure emit a terminal
					// review-manual-required event instead of yet another
					// review-error, parking the bead so a subsequent loop
					// iteration does NOT re-execute primary.
					class := classifyReviewError(reviewErr, reviewRes)
					prior := countPriorReviewErrors(w.Store, candidate.ID, report.ResultRev)
					attemptCount := prior + 1
					maxRetries := rcfg.ReviewMaxRetries()
					if maxRetries <= 0 {
						maxRetries = DefaultReviewMaxRetries
					}
					errSummary := EventBodySummary{
						InputBytes:  0,
						OutputBytes: 0,
					}
					if reviewRes != nil {
						errSummary = EventBodySummary{
							Harness:     reviewRes.ReviewerHarness,
							Model:       reviewRes.ReviewerModel,
							InputBytes:  reviewRes.InputBytes,
							OutputBytes: reviewRes.OutputBytes,
							ElapsedMS:   reviewRes.DurationMS,
						}
					}
					if attemptCount >= maxRetries {
						body := appendEventSummary(reviewErrorEventBody(class, attemptCount, report.ResultRev, reviewErr.Error()), errSummary)
						_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
							Kind:      "review-manual-required",
							Summary:   class,
							Body:      body,
							Actor:     assignee,
							Source:    "ddx agent execute-loop",
							CreatedAt: now().UTC(),
						})
						// Park the bead with a long cooldown so subsequent
						// loop iterations do NOT re-pick it for primary
						// re-execution. Reviewer-failure invariant (§13) is
						// preserved: the bead is NOT closed.
						parkUntil := now().UTC().Add(365 * 24 * time.Hour)
						_ = w.Store.SetExecutionCooldown(candidate.ID, parkUntil, "review-manual-required", class)
					} else {
						body := appendEventSummary(reviewErrorEventBody(class, attemptCount, report.ResultRev, reviewErr.Error()), errSummary)
						_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
							Kind:      "review-error",
							Summary:   class,
							Body:      body,
							Actor:     assignee,
							Source:    "ddx agent execute-loop",
							CreatedAt: now().UTC(),
						})
					}
				} else {
					report.ReviewVerdict = string(reviewRes.Verdict)
					report.ReviewRationale = reviewRes.Rationale
					// Persist the full reviewer stream as an artifact so the
					// event body never carries the raw stream (ddx-f8a11202).
					// On error the artifact path is empty and the event body
					// still contains the short verdict summary; callers of this
					// loop can recover the full text from the reviewer session
					// log if the artifact write failed.
					artifactPath, artifactErr := persistReviewerStream(runtime.ProjectRoot, candidate.ID, report.AttemptID, reviewRes.RawOutput)
					if artifactErr != nil && runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "reviewer stream artifact: %v\n", artifactErr)
					}

					reviewSummary := EventBodySummary{
						Harness:     reviewRes.ReviewerHarness,
						Model:       reviewRes.ReviewerModel,
						InputBytes:  reviewRes.InputBytes,
						OutputBytes: reviewRes.OutputBytes,
						ElapsedMS:   reviewRes.DurationMS,
					}
					switch reviewRes.Verdict {
					case VerdictApprove:
						// Approved: record the verdict event and then close.
						// Closure must land AFTER the review event so the
						// gate (ddx-e30e60a9) sees the terminal verdict.
						_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
							Kind:      "review",
							Summary:   "APPROVE",
							Body:      appendEventSummary(reviewEventBody("APPROVE", reviewRes.Rationale, artifactPath), reviewSummary),
							Actor:     assignee,
							Source:    "ddx agent execute-loop",
							CreatedAt: now().UTC(),
						})
						if cerr := w.Store.CloseWithEvidence(candidate.ID, report.SessionID, report.ResultRev); cerr != nil {
							return result, cerr
						}
					case VerdictRequestChanges:
						// Needs fixes: record the review verdict, then reopen
						// with findings in notes. The review event must land
						// even on non-approve paths so review-outcomes can
						// attribute the rejection to the originating tier.
						_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
							Kind:      "review",
							Summary:   "REQUEST_CHANGES",
							Body:      appendEventSummary(reviewEventBody("REQUEST_CHANGES", reviewRes.Rationale, artifactPath), reviewSummary),
							Actor:     assignee,
							Source:    "ddx agent execute-loop",
							CreatedAt: now().UTC(),
						})
						reopenNotes := reviewRes.Rationale
						if reopenNotes == "" {
							reopenNotes = reviewRes.RawOutput
						}
						if reopenErr := w.Store.Reopen(candidate.ID, "review: REQUEST_CHANGES", reopenNotes); reopenErr != nil {
							return result, reopenErr
						}
						report.Status = ExecuteBeadStatusReviewRequestChanges
						report.Detail = "post-merge review: REQUEST_CHANGES"
						reviewApproved = false
					case VerdictBlock:
						rationale := strings.TrimSpace(reviewRes.Rationale)
						if rationale == "" {
							_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
								Kind:      "review-malfunction",
								Summary:   "BLOCK without rationale",
								Body:      appendEventSummary(reviewEventBody("BLOCK without rationale", "", artifactPath), reviewSummary),
								Actor:     assignee,
								Source:    "ddx agent execute-loop",
								CreatedAt: now().UTC(),
							})
							report.Status = ExecuteBeadStatusReviewMalfunction
							report.Detail = "post-merge review: malformed BLOCK verdict (missing rationale)"
							report.ReviewRationale = ""
							reviewApproved = false
							break
						}
						// Cannot proceed: record the verdict, then reopen and
						// flag for human with BLOCK marker plus actionable rationale.
						_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
							Kind:      "review",
							Summary:   "BLOCK",
							Body:      appendEventSummary(rationale, reviewSummary),
							Actor:     assignee,
							Source:    "ddx agent execute-loop",
							CreatedAt: now().UTC(),
						})
						blockNotes := "REVIEW:BLOCK\n\n" + rationale
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
				satisfied, evidence, aerr := w.adjudicateNoChanges(ctx, candidate.ID, count, maxNoChangesBeforeClose, report.NoChangesRationale, candidate.Acceptance, runtime.ProjectRoot)
				if aerr != nil {
					return result, aerr
				}
				if satisfied {
					// Adjudication confirmed bead is already satisfied.
					// Set the terminal status BEFORE the close so the late
					// executeBeadLoopEvent append captures "already_satisfied"
					// (not "no_changes"), and emit an early execute-bead
					// evidence event so the closure gate accepts even when
					// BaseRev is empty (test fixtures and genuinely-no-commit
					// satisfied beads).
					report.Status = ExecuteBeadStatusAlreadySatisfied
					if evidence != "" {
						// Checker evidence explains why the bead is being closed;
						// it takes precedence over the executor's attempt detail.
						report.Detail = evidence
					}
					_ = w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC()))
					if cerr := w.Store.CloseWithEvidence(candidate.ID, report.SessionID, report.BaseRev); cerr != nil {
						return result, cerr
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

		// Skip the late execute-bead append for already-satisfied beads —
		// the satisfied path appends its own terminal event before
		// CloseWithEvidence so the closure gate sees execution evidence.
		// Duplicating it here would yield two identical events.
		if report.Status != ExecuteBeadStatusAlreadySatisfied {
			if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC())); err != nil {
				return result, err
			}
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
		emitProgress(runtime.ProgressCh, ProgressEvent{
			EventID:   "evt-" + randomProgressID(),
			AttemptID: finalAttemptID,
			WorkerID:  runtime.WorkerID,
			ProjectID: runtime.ProjectRoot,
			BeadID:    candidate.ID,
			Harness:   harness,
			Model:     model,
			Profile:   profile,
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

		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "✓ %s → %s\n", candidate.ID, formatLoopResult(report))
		}

		if runtime.Once {
			return result, nil
		}
	}
}

func (w *ExecuteBeadWorker) nextCandidate(attempted map[string]struct{}, labelFilter string) (bead.Bead, bool, error) {
	ready, err := w.Store.ReadyExecution()
	if err != nil {
		return bead.Bead{}, false, err
	}
	for _, candidate := range ready {
		if _, seen := attempted[candidate.ID]; seen {
			continue
		}
		if labelFilter != "" && !HasBeadLabel(candidate.Labels, labelFilter) {
			continue
		}
		return candidate, true, nil
	}
	return bead.Bead{}, false, nil
}

// appendLoopRoutingEvidence records a kind:routing evidence event on the bead
// from the executor's ExecuteBeadReport, so that review-outcomes analytics can
// attribute a subsequent review verdict to the originating provider/model tier.
// Best-effort: errors and missing-provider cases are silently ignored.
func appendLoopRoutingEvidence(store BeadEventAppender, beadID string, report ExecuteBeadReport, createdAt time.Time) {
	if store == nil || beadID == "" {
		return
	}
	provider := report.Provider
	if provider == "" {
		provider = report.Harness
	}
	if provider == "" {
		return
	}
	body, err := json.Marshal(map[string]any{
		"resolved_provider": provider,
		"resolved_model":    report.Model,
		"fallback_chain":    []string{},
		"requested_profile": report.RequestedProfile,
		"requested_tier":    report.RequestedTier,
		"resolved_tier":     report.ResolvedTier,
		"escalation_count":  report.EscalationCount,
		"final_tier":        report.FinalTier,
	})
	if err != nil {
		return
	}
	summary := "provider=" + provider
	if report.Model != "" {
		summary += " model=" + report.Model
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "routing",
		Summary:   summary,
		Body:      string(body),
		Actor:     "ddx",
		Source:    "ddx agent execute-loop",
		CreatedAt: createdAt,
	})
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
	if report.ReviewRationale != "" {
		parts = append(parts, report.ReviewRationale)
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
func (w *ExecuteBeadWorker) adjudicateNoChanges(ctx context.Context, beadID string, noChangesCount, maxNoChangesBeforeClose int, rationale, acceptance, projectRoot string) (bool, string, error) {
	if w.SatisfactionChecker != nil {
		return w.SatisfactionChecker.CheckSatisfied(ctx, beadID, noChangesCount)
	}
	candidate := false
	evidence := ""
	switch {
	case rationaleIsSpecific(rationale):
		candidate = true
		evidence = rationale
	case noChangesCount >= maxNoChangesBeforeClose:
		candidate = true
		evidence = fmt.Sprintf("no_changes on %d consecutive attempt(s); bead treated as already satisfied", noChangesCount)
	}
	if !candidate {
		return false, "", nil
	}
	// Tighten the gate: when AC names structural properties (test functions,
	// deleted files, removed struct fields), refuse already_satisfied unless
	// each property holds in the worktree / rationale. Prevents false closes
	// where a regression suite passes but the AC's specific contract is unmet.
	if claims := ParseACClaims(acceptance); len(claims) > 0 {
		if ok, why := VerifyACClaims(claims, projectRoot, rationale); !ok {
			return false, why, nil
		}
	}
	return true, evidence, nil
}

func shouldSuppressNoProgress(report ExecuteBeadReport) bool {
	if report.BaseRev == "" || report.ResultRev == "" {
		return false
	}
	return report.BaseRev == report.ResultRev
}

// classifyReviewError maps a reviewer error and partial result into one of the
// four FEAT-022 §12 taxonomy classes. Resolution order:
//  1. reviewRes.Error if it's already one of the canonical class identifiers
//     (the reviewer set it explicitly — preferred path).
//  2. The error string text (for backwards-compatible reviewers that only
//     embed the class in their message).
//  3. ErrReviewVerdictUnparseable as a fallback when the strict-parse error
//     leaks through a reviewer that did not set Error.
//  4. Default to transport (network/provider-side failure) when nothing else
//     matches.
func classifyReviewError(reviewErr error, reviewRes *ReviewResult) string {
	if reviewRes != nil {
		switch reviewRes.Error {
		case evidence.OutcomeReviewContextOverflow,
			evidence.OutcomeReviewProviderEmpty,
			evidence.OutcomeReviewUnparseable,
			evidence.OutcomeReviewTransport:
			return reviewRes.Error
		}
	}
	msg := ""
	if reviewErr != nil {
		msg = reviewErr.Error()
	}
	switch {
	case strings.Contains(msg, evidence.OutcomeReviewContextOverflow):
		return evidence.OutcomeReviewContextOverflow
	case strings.Contains(msg, evidence.OutcomeReviewProviderEmpty):
		return evidence.OutcomeReviewProviderEmpty
	case strings.Contains(msg, evidence.OutcomeReviewUnparseable):
		return evidence.OutcomeReviewUnparseable
	case strings.Contains(msg, evidence.OutcomeReviewTransport):
		return evidence.OutcomeReviewTransport
	case reviewErr != nil && strings.Contains(msg, ErrReviewVerdictUnparseable.Error()):
		return evidence.OutcomeReviewUnparseable
	}
	return evidence.OutcomeReviewTransport
}

// reResultRevField extracts the result_rev value from a structured review-error
// event body (one `result_rev=<sha>` line per event, written by
// reviewErrorEventBody). The format is intentionally line-oriented so it
// survives the AppendEvent body cap without losing the rev association.
var reResultRevField = regexp.MustCompile(`(?m)^result_rev=([^\s]+)\s*$`)

// countPriorReviewErrors returns the number of `review-error` events already
// recorded against this bead whose body cites the given result_rev. This is
// the FEAT-022 §14 retry counter — it is event-scoped (no separate counter
// field on the bead) so a fresh result_rev naturally resets the count.
func countPriorReviewErrors(store ExecuteBeadLoopStore, beadID, resultRev string) int {
	if resultRev == "" {
		return 0
	}
	events, err := store.Events(beadID)
	if err != nil {
		return 0
	}
	n := 0
	for _, ev := range events {
		if ev.Kind != "review-error" {
			continue
		}
		m := reResultRevField.FindStringSubmatch(ev.Body)
		if m == nil {
			continue
		}
		if m[1] == resultRev {
			n++
		}
	}
	return n
}

// reviewErrorEventBody is the canonical body shape for review-error and
// review-manual-required events. It carries the failure class, attempt count,
// and result_rev as discrete lines so operators can grep without parsing the
// full reviewer error text. The trailing free-form message is the raw
// reviewer-error string for forensics.
func reviewErrorEventBody(class string, attemptCount int, resultRev, message string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "failure_class=%s\n", class)
	fmt.Fprintf(&b, "attempt_count=%d\n", attemptCount)
	fmt.Fprintf(&b, "result_rev=%s\n", resultRev)
	if message != "" {
		b.WriteString("\n")
		b.WriteString(message)
	}
	return b.String()
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
