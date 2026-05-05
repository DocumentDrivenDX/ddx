package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// ExecuteBeadLoopRuntime carries the non-serializable plumbing and
// per-invocation runtime intent for an execute-bead loop run. Durable knobs
// (assignee, retry caps, harness/model, tier bounds, etc.) live on
// config.ResolvedConfig and are passed via Run's rcfg argument.
type ExecuteBeadLoopRuntime struct {
	Log                   io.Writer
	EventSink             io.Writer
	ProgressCh            chan<- ProgressEvent
	PreClaimHook          func(ctx context.Context) error
	PreDispatchLintHook   func(ctx context.Context, beadID string) (LintResult, error)
	PostAttemptTriageHook func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error)
	// ReviewCostCap, when non-nil, accumulates reviewer cost on the same
	// loop budget tracker used by the implementer attempts.
	ReviewCostCap *escalation.CostCapTracker
	// RoutePreflight, when non-nil, is invoked between nextCandidate and
	// Claim. It is expected to call upstream ResolveRoute against the
	// loop's resolved (harness, model) and return whatever typed routing
	// error the upstream surfaced — notably agent.ErrHarnessModelIncompatible
	// when the harness allow-list rejects the model. Any non-nil error
	// causes the loop to exit immediately without claiming the bead, with
	// a worker-level execution_failed record naming the rejected pair.
	// DDx does NOT duplicate the upstream allow-list; this gate only
	// consumes the typed-incompatibility surface.
	RoutePreflight func(ctx context.Context, harness, model string) error
	Once           bool
	PollInterval   time.Duration
	NoReview       bool
	LabelFilter    string
	SessionID      string
	WorkerID       string
	ProjectRoot    string
	// TargetBeadID, when non-empty, restricts nextCandidate to only return the
	// named bead from the execution-ready queue. Used by `ddx try <bead-id>`
	// to dispatch a single specific bead through the same claim → executor →
	// land path the queue drain uses. When empty, the picker behaves normally.
	TargetBeadID string
	// WakeCh, when non-nil, signals the idle-poll sleep to return immediately
	// so the loop re-scans the queue. Used by the operator-prompt approve /
	// auto-approve mutations (Story 15) to avoid a poll-interval-sized delay
	// before a freshly-approved bead is claimed. Implementations must send
	// non-blocking (server-side helpers do); the loop only waits for a
	// receive on WakeCh during the idle sleep, never elsewhere.
	WakeCh <-chan struct{}
}

// DefaultReviewMaxRetries is the number of reviewer attempts allowed per
// committed result_rev before the loop emits a terminal
// `review-manual-required` event and stops re-executing primary. FEAT-022 §14.
const DefaultReviewMaxRetries = 3

// MaxLoopCooldown is the absolute upper bound the execute-loop will set for
// any execute-loop-retry-after value. Year-scale parks effectively mean
// "never retry" and that should be a deliberate operator choice via
// `ddx bead update --set execute-loop-retry-after=...`, not an automatic
// loop decision. Beyond this cap, the loop refuses to lengthen the cooldown
// further; an operator extending it manually still works.
const MaxLoopCooldown = 24 * time.Hour

// LandConflictCooldown is the short cooldown applied when all conflict-recovery
// paths fail (3-way ort auto-merge + optional focused-resolve agent) and the
// bead is parked as land_conflict_unresolvable. Much shorter than
// MaxLoopCooldown because land conflicts often resolve quickly as sibling beads
// advance the base branch.
const LandConflictCooldown = 15 * time.Minute

// StoreErrorCooldown is the short cooldown applied when a Store.* operation in
// the outcome-handling block returns a transient error. It prevents an
// immediate re-queue of the affected bead while allowing the loop to continue
// to the next candidate. Store errors during outcome handling are non-fatal:
// they are logged and emitted as loop-error events but do not terminate the
// worker.
const StoreErrorCooldown = 5 * time.Minute

// SubmitWithPreMergeChecks is the canonical land-back step for the execute-bead
// loop. It runs the project's .ddx/checks/*.yaml gate against the worker's
// (baseRev, resultRev) before forwarding to submit. On Blocked, it preserves
// the worker's result under refs/ddx/iterations/, records checks-blocked
// events on the bead, and returns the synthesised LandResult{Status:"preserved"}
// without ever calling submit. On pass, honoured checks-bypass annotations are
// recorded as checks-bypass events and submit runs normally.
//
// submit is the project's land coordinator entry point (typically
// LocalLandCoordinator.Submit or the server-side coordinator submit).
//
// The implementation lives in checks_premerge.go alongside its supporting
// helpers (RunPreMergeChecks, AppendPreMergeChecksEvents,
// PreserveAfterPreMergeChecks); this thin wrapper exists so the execute-bead
// loop's land-back call site is one obvious helper rather than three nested
// ones, and so callers wiring the gate do not need to import the helper layer
// directly.
func SubmitWithPreMergeChecks(
	ctx context.Context,
	projectRoot string,
	b *bead.Bead,
	res *ExecuteBeadResult,
	submit func(LandRequest) (*LandResult, error),
	eventStore BeadEventAppender,
	actor, source string,
	now func() time.Time,
) (*LandResult, *PreMergeChecksOutcome, error) {
	if now == nil {
		now = time.Now
	}
	if submit == nil {
		return nil, nil, fmt.Errorf("submit-with-pre-merge-checks: submit callback required")
	}
	evidenceDir := filepath.Join(projectRoot, res.ExecutionDir)
	outcome, err := RunPreMergeChecks(ctx, projectRoot, b, res.BaseRev, res.ResultRev, evidenceDir)
	if err != nil {
		// Treat checks_bypass / loader errors as a hard preserve: the worker
		// did its job; the operator misconfigured the gate. Better to park
		// the result under an iteration ref than to silently land it.
		ref := PreserveRef(res.BeadID, res.BaseRev)
		if upErr := (&RealOrchestratorGitOps{}).UpdateRef(projectRoot, ref, res.ResultRev); upErr != nil {
			return nil, nil, fmt.Errorf("preserving result after checks setup error: %w (original error: %v)", upErr, err)
		}
		return &LandResult{
			Status:      "preserved",
			PreserveRef: ref,
			Reason:      "pre-merge checks setup error: " + err.Error(),
		}, nil, nil
	}
	if outcome.Blocked {
		land, perr := PreserveAfterPreMergeChecks(projectRoot, res, outcome, &RealOrchestratorGitOps{})
		if perr != nil {
			return nil, outcome, perr
		}
		if eventStore != nil {
			_ = AppendPreMergeChecksEvents(eventStore, res.BeadID, outcome, actor, source, now().UTC())
		}
		return land, outcome, nil
	}
	if eventStore != nil && len(outcome.Bypassed) > 0 {
		_ = AppendPreMergeChecksEvents(eventStore, res.BeadID, outcome, actor, source, now().UTC())
	}
	land, sErr := submit(BuildLandRequestFromResult(projectRoot, res))
	return land, outcome, sErr
}

// runPostAttemptTriage invokes the optional post-attempt triage hook after
// the attempt outcome has been finalized but before any cooldown decision is
// applied. Hook failures are fail-open: the report is returned unchanged and
// legacy cooldown behavior remains in force.
func (w *ExecuteBeadWorker) runPostAttemptTriage(ctx context.Context, candidate bead.Bead, report ExecuteBeadReport, runtime ExecuteBeadLoopRuntime, assignee string, now func() time.Time) ExecuteBeadReport {
	hook := runtime.PostAttemptTriageHook
	if hook == nil {
		return report
	}
	triage, err := hook(ctx, candidate.ID, report)
	if err != nil {
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "post-attempt triage error (%s %s): %v (continuing)\n", candidate.ID, report.Status, err)
		}
		return report
	}
	if strings.TrimSpace(triage.Classification) == "" {
		return report
	}
	report.OutcomeReason = triage.Classification
	recordPostAttemptTriageEvent(w.Store, candidate.ID, report, triage, assignee, now().UTC())
	return report
}

// CapLoopCooldown clamps a loop-set cooldown duration to MaxLoopCooldown.
// The loop uses this for every SetExecutionCooldown call so no automatic
// path can silently park a bead for a year.
func CapLoopCooldown(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	if d > MaxLoopCooldown {
		return MaxLoopCooldown
	}
	return d
}

type ExecuteBeadReport struct {
	BeadID    string `json:"bead_id"`
	AttemptID string `json:"attempt_id,omitempty"`
	WorkerID  string `json:"worker_id,omitempty"`
	Harness   string `json:"harness,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	// ActualPower is the routing-decision power of the model that actually
	// served the implementer's call. Forwarded to the post-merge reviewer so
	// it can request MinPower=actualPower+1 and bias routing toward a
	// stronger reviewer (R4 pairing).
	ActualPower                 int           `json:"actual_power,omitempty"`
	PredictedPower              int           `json:"predicted_power,omitempty"`
	PredictedSpeedTPS           float64       `json:"predicted_speed_tps,omitempty"`
	PredictedCostUSDPer1kTokens float64       `json:"predicted_cost_usd_per_1k_tokens,omitempty"`
	PredictedCostSource         string        `json:"predicted_cost_source,omitempty"`
	Status                      string        `json:"status"`
	Detail                      string        `json:"detail,omitempty"`
	Error                       string        `json:"error,omitempty"`
	Stderr                      string        `json:"stderr,omitempty"`
	RateLimitBudget             time.Duration `json:"rate_limit_budget,omitempty"`
	SessionID                   string        `json:"session_id,omitempty"`
	BaseRev                     string        `json:"base_rev,omitempty"`
	ResultRev                   string        `json:"result_rev,omitempty"`
	PreserveRef                 string        `json:"preserve_ref,omitempty"`
	RetryAfter                  string        `json:"retry_after,omitempty"`
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
	// DecompositionRecommendation carries the structured list of recommended
	// sub-bead titles when Status == declined_needs_decomposition. The loop
	// records these on the bead as a `decomposition-recommendation` event so
	// operators (or helix-evolve) can split the bead without re-deriving the
	// recommendation.
	DecompositionRecommendation []string `json:"decomposition_recommendation,omitempty"`
	// DecompositionRationale is a free-form explanation accompanying
	// DecompositionRecommendation. Optional.
	DecompositionRationale string `json:"decomposition_rationale,omitempty"`
	// Disrupted marks a failed attempt as worker-disrupted rather than
	// model-gave-up: the model was prevented from making progress by an
	// external cause (context cancellation, executor SIGKILL/SIGTERM,
	// transport-class provider error, server restart, routing preflight
	// rejection). The execute-loop bypasses the no-progress cooldown for
	// Disrupted reports so the bead is immediately re-claimable instead of
	// being parked for hours on a transient outage. ddx-5b3e57f4.
	Disrupted bool `json:"disrupted,omitempty"`
	// DisruptionReason carries the kind of disruption detected when
	// Disrupted=true (e.g. "context_canceled", "context_deadline",
	// "transport_error", "preflight_rejected"). Used in the
	// `disruption_detected` event body so operators can see which class is
	// occurring.
	DisruptionReason string `json:"disruption_reason,omitempty"`
	// OutcomeReason carries the machine-readable lifecycle classification for
	// the attempt outcome. It complements Disrupted/DisruptionReason without
	// changing their mechanical interruption semantics.
	OutcomeReason string `json:"outcome_reason,omitempty"`
}

type ExecuteBeadExecutor interface {
	Execute(ctx context.Context, beadID string) (ExecuteBeadReport, error)
}

type ExecuteBeadExecutorFunc func(ctx context.Context, beadID string) (ExecuteBeadReport, error)

func (f ExecuteBeadExecutorFunc) Execute(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
	return f(ctx, beadID)
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
	AppendNotes(id string, notes string) error
	IncrNoChangesCount(id string) (int, error)
	// Reopen sets a closed bead back to open, appending notes to the bead's
	// Notes field and recording a reopen event. Used by the post-merge review
	// step when the reviewer returns REQUEST_CHANGES or BLOCK.
	Reopen(id, reason, notes string) error
	// Update mutates a bead in place. Used by the post-merge triage step to
	// add labels (e.g. "needs_human") and metadata hints (e.g. tier-pin) when
	// the triage policy escalates after repeated review BLOCKs.
	Update(id string, mutate func(*bead.Bead)) error
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
	SatisfactionChecker agenttry.SatisfactionChecker // nil → NoChangesContract default
	// VerificationRunner runs a verification_command parsed out of a
	// no_changes_rationale.txt under NoChangesContract (TD-031 §8.1). When
	// nil, DefaultVerificationCommandRunner is used.
	VerificationRunner agenttry.VerificationCommandRunner
	Now                func() time.Time
	// Reviewer, when non-nil, is called after every successful merge to
	// validate the commit against the bead's acceptance criteria. When nil,
	// post-merge review is skipped (same behaviour as --no-review).
	Reviewer BeadReviewer

	// ConflictResolver, when non-nil, is called after the 3-way ort auto-merge
	// step fails to recover a preserved iteration. The implementation should
	// attempt a focused conflict-resolution pass (e.g. a cheap-tier agent run
	// with the conflict files and bead AC) and return the new merged tip SHA on
	// success. isBlocking=true signals the conflict requires human judgment
	// (escalating to land_conflict_needs_human); false means failed-but-retriable.
	ConflictResolver func(ctx context.Context, beadID, preserveRef, projectRoot string) (newTip string, isBlocking bool, err error)

	// conflictAutoRecoverFn replaces the default landConflictAutoRecover. Set
	// in tests to inject controlled recovery results without a real git repo.
	conflictAutoRecoverFn func(wd, preserveRef string, gitOps LandingGitOps) (string, error)
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
	heartbeatInterval := rcfg.HeartbeatInterval()
	if heartbeatInterval <= 0 {
		heartbeatInterval = bead.HeartbeatInterval
	}
	harness := rcfg.Harness()
	model := rcfg.Model()
	profile := rcfg.Profile()

	result := &ExecuteBeadLoopResult{}
	attempted := make(map[string]struct{})
	// hookFailed tracks beads whose pre-claim hook failed on first presentation
	// in this run. A bead in hookFailed but not attempted gets one retry: on the
	// second hook failure it moves to attempted so nextCandidate will skip it and
	// the loop can exit gracefully. This prevents infinite spinning when the hook
	// always fails while still allowing transient hook failures to be retried.
	hookFailed := make(map[string]struct{})

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
	// exitReason is populated as the loop exits to surface a structured reason
	// in the loop.end event (ddx-dc157075 AC #4). Recognized values: "sigterm",
	// "sigint", "fatal_config", "once_complete", "explicit_poll_zero". The
	// "providers_exhausted" slot is reserved for ddx-aede917d (quota pause).
	exitReason := ""
	defer func() {
		emit("loop.end", map[string]any{
			"attempts":            result.Attempts,
			"successes":           result.Successes,
			"failures":            result.Failures,
			"last_failure_status": result.LastFailureStatus,
			"exit_reason":         exitReason,
		})
	}()

	for {
		// Respect context cancellation between iterations. Without this check,
		// a Stop() request (which cancels ctx) would only take effect during
		// the idle poll sleep — the loop would happily claim the next ready
		// bead as soon as the current Execute returned, ignoring the cancel.
		if err := ctx.Err(); err != nil {
			if exitReason == "" {
				switch err {
				case context.Canceled:
					exitReason = "sigint"
				case context.DeadlineExceeded:
					exitReason = "sigterm"
				default:
					exitReason = "context_cancelled"
				}
			}
			return result, err
		}

		candidate, skips, ok, err := w.nextCandidate(attempted, runtime.LabelFilter, runtime.TargetBeadID)
		if err != nil {
			exitReason = "fatal_config"
			return result, err
		}
		// Diagnostic: when nextCandidate selected a lower-priority bead while
		// at least one higher-priority bead was skipped, emit a structured
		// picker.priority_skip event naming each skipped bead and the reason
		// (ddx-9d55601f AC #4). Without this surface, an operator who sees a
		// worker claim a P2 while P0s sit in `ddx bead ready` cannot tell
		// whether the picker has a bug, the P0s are filtered upstream, or a
		// label filter is in play.
		if ok {
			emitPickerPrioritySkips(emit, candidate, skips)
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
				// Legacy "drain-and-exit" semantics: operator either passed
				// --once (handled at end-of-iteration) or explicitly set
				// --poll-interval=0. ddx-dc157075: with default --poll-interval
				// flipped to 30s in CLI/server, this path now only fires for
				// the explicit opt-out case.
				if runtime.Once {
					exitReason = "once_complete"
				} else {
					exitReason = "explicit_poll_zero"
				}
				return result, nil
			}
			// --once with poll-interval > 0: an empty queue still means there
			// is no work to pick, and --once explicitly asked for at-most-one.
			// Returning here preserves the operator-visible "drain and stop"
			// semantics of --once even when the long-running default applies.
			if runtime.Once {
				exitReason = "once_complete"
				return result, nil
			}
			// Long-running drain (poll-interval > 0): emit a transient
			// "no_ready_work" event so server-managed workers can surface this
			// as an idle substate (ddx-dc157075 AC #5) instead of treating it
			// as terminal.
			emit("loop.idle", map[string]any{
				"reason":        "no_ready_work",
				"poll_interval": runtime.PollInterval.String(),
			})
			// Surface the idle substate via the progress channel so server-side
			// drainProgress can flip WorkerRecord.Substate to "idle" without
			// reading the JSONL event log. Phase="loop.idle" is treated as a
			// substate signal, not a per-bead attempt phase.
			emitProgress(runtime.ProgressCh, ProgressEvent{
				EventID:   "evt-" + randomProgressID(),
				WorkerID:  runtime.WorkerID,
				ProjectID: runtime.ProjectRoot,
				Phase:     "loop.idle",
				Heartbeat: true,
				TS:        now().UTC(),
				Message:   "no_ready_work",
			})
			// Reset per-Run attempted/hookFailed maps before sleeping
			// (ddx-dc157075 AC #2). Without this reset, beads attempted earlier
			// in this Run remain filtered out of nextCandidate forever, so the
			// "attempted-once = exit" trap reasserts itself: a bead whose
			// cooldown has expired since its last attempt would never be
			// re-picked by the same long-running worker.
			for k := range attempted {
				delete(attempted, k)
			}
			for k := range hookFailed {
				delete(hookFailed, k)
			}
			if err := sleepOrWake(ctx, runtime.PollInterval, runtime.WakeCh); err != nil {
				if exitReason == "" {
					switch err {
					case context.Canceled:
						exitReason = "sigint"
					case context.DeadlineExceeded:
						exitReason = "sigterm"
					default:
						exitReason = "context_cancelled"
					}
				}
				return result, err
			}
			continue
		}

		// Found a candidate: clear any "idle" substate set on the previous
		// no-candidate iteration (ddx-dc157075 AC #5).
		emitProgress(runtime.ProgressCh, ProgressEvent{
			EventID:   "evt-" + randomProgressID(),
			WorkerID:  runtime.WorkerID,
			ProjectID: runtime.ProjectRoot,
			BeadID:    candidate.ID,
			Phase:     "loop.active",
			Heartbeat: true,
			TS:        now().UTC(),
		})

		// Pre-claim hook: fetch origin + verify ancestry before claiming.
		// On error the bead is skipped for this iteration; the loop
		// continues (ctx is not cancelled). The bead is NOT added to
		// attempted on the FIRST failure so a transient hook failure (e.g.
		// diverged branch) allows a retry on the next iteration. On a
		// SECOND failure for the same bead in the same run it is moved into
		// attempted so nextCandidate will skip it, preventing an infinite
		// loop when the hook always fails.
		if runtime.PreClaimHook != nil {
			if hookErr := runtime.PreClaimHook(ctx); hookErr != nil {
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "pre-claim hook: %v (skipping %s)\n", hookErr, candidate.ID)
				}
				emit("preclaim.skipped", map[string]any{
					"bead_id": candidate.ID,
					"reason":  hookErr.Error(),
				})
				if _, alreadyFailed := hookFailed[candidate.ID]; alreadyFailed {
					// Second failure in this run: stop retrying this bead.
					attempted[candidate.ID] = struct{}{}
				} else {
					hookFailed[candidate.ID] = struct{}{}
				}
				continue
			}
		}

		attempted[candidate.ID] = struct{}{}

		// Routing preflight gate (FEAT-006 D3, ddx-98e6e9ef): consult the
		// upstream typed-incompatibility surface BEFORE claiming. If the
		// configured (harness, model) cannot serve the bead, exit the loop
		// with a worker-level failure record — no claim, no executor
		// invocation, no tier-attempt event burn.
		if runtime.RoutePreflight != nil {
			if rerr := runtime.RoutePreflight(ctx, harness, model); rerr != nil {
				detail := fmt.Sprintf("routing preflight rejected (harness=%s model=%s): %s",
					harness, model, rerr.Error())
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "routing preflight: %s (skipping %s)\n",
						detail, candidate.ID)
				}
				emit("preflight.rejected", map[string]any{
					"bead_id": candidate.ID,
					"harness": harness,
					"model":   model,
					"reason":  rerr.Error(),
				})
				report := ExecuteBeadReport{
					BeadID:           candidate.ID,
					Status:           ExecuteBeadStatusExecutionFailed,
					Detail:           detail,
					Harness:          harness,
					Model:            model,
					Disrupted:        true,
					DisruptionReason: "preflight_rejected",
				}
				// ddx-5b3e57f4: a routing preflight rejection is an operator
				// configuration issue (harness/model allow-list mismatch), not
				// evidence the model could not make progress. Mark the report
				// Disrupted and emit the diagnostic event so operators can see
				// disruption rates without parking the bead in a no-progress
				// cooldown.
				emitDisruptionDetected(emit, w.Store, candidate.ID,
					"preflight_rejected", detail, harness, model, assignee, now().UTC())
				result.Attempts++
				result.Failures++
				result.LastFailureStatus = report.Status
				result.Results = append(result.Results, report)
				// ddx-dc157075 AC #3: do NOT abandon the rest of the queue on
				// a single bead's preflight rejection. Continue to the next
				// candidate. If --once was requested we still honour it.
				if runtime.Once {
					exitReason = "once_complete"
					return result, nil
				}
				continue
			}
		}

		if runtime.PreDispatchLintHook != nil {
			lintResult, lintErr := runtime.PreDispatchLintHook(ctx, candidate.ID)
			lintThreshold := rcfg.BeadQualityLintBlockThresholdScore()
			appendPreDispatchLintEvent(w.Store, candidate.ID, lintResult, lintErr, lintThreshold, assignee, now().UTC())

			if lintErr != nil {
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "pre-dispatch lint: %v (continuing to claim %s)\n", lintErr, candidate.ID)
				}
				emit("pre_dispatch_lint.warn", map[string]any{
					"bead_id": candidate.ID,
					"warning": lintErr.Error(),
				})
			} else if lintThreshold > 0 && lintResult.Score < lintThreshold {
				blockMsg := fmt.Sprintf(
					"pre-dispatch lint blocked dispatch for %s: score=%d below threshold=%d; see bead-lifecycle MODE: lint guidance in .agents/skills/ddx/bead-lifecycle/SKILL.md",
					candidate.ID, lintResult.Score, lintThreshold,
				)
				if runtime.Log != nil {
					_, _ = fmt.Fprintln(runtime.Log, blockMsg)
				}
				emit("pre_dispatch_lint.blocked", map[string]any{
					"bead_id":          candidate.ID,
					"score":            lintResult.Score,
					"threshold_score":  lintThreshold,
					"skill":            "bead-lifecycle",
					"skill_path":       ".agents/skills/ddx/bead-lifecycle/SKILL.md",
					"dispatch_skipped": true,
					"warning_mode":     false,
					"rationale":        lintResult.Rationale,
					"suggested_fixes":  lintResult.SuggestedFixes,
					"waivers_applied":  lintResult.WaiversApplied,
				})
				continue
			}
		}

		if err := w.Store.Claim(candidate.ID, assignee); err != nil {
			// Another worker won the race for this bead. Emit a structured
			// claim_race event so concurrent-worker losses are observable
			// (ddx-9d55601f AC #4). The bead remains in `attempted` for this
			// run; on the next iteration it will be filtered out of
			// ReadyExecution naturally because Claim() flipped it to
			// in_progress, so the loser keeps moving down priority order.
			emit("picker.claim_race", map[string]any{
				"bead_id":  candidate.ID,
				"priority": candidate.Priority,
				"reason":   err.Error(),
			})
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

		// tryExecutor preserves the legacy w.Executor.Execute(ctx, candidate.ID)
		// invocation while letting try.Attempt own conflict recovery.
		attemptOut, err := agenttry.Attempt(ctx, w.Store, candidate.ID, agenttry.AttemptOpts{
			Bead:                candidate,
			Executor:            tryExecutor(w.Executor),
			Store:               w.Store,
			ProjectRoot:         runtime.ProjectRoot,
			SatisfactionChecker: w.SatisfactionChecker,
			VerificationRunner:  w.VerificationRunner,
			AutoRecover:         tryAutoRecover(w.conflictAutoRecoverFn),
			ConflictResolver:    w.ConflictResolver,
			Assignee:            assignee,
			Now:                 now,
			Cooldown:            LandConflictCooldown,
		})
		hbCancel()
		hbWG.Wait()
		report := fromTryReport(attemptOut.Report)
		if report.BeadID == "" {
			report.BeadID = candidate.ID
		}
		if report.Status == "" {
			report.Status = ExecuteBeadStatusExecutionFailed
		}
		if report.Detail == "" {
			report.Detail = ExecuteBeadStatusDetail(report.Status, "", "")
		}
		// ddx-5b3e57f4: distinguish worker-disrupted from model-gave-up.
		// A failed attempt where the loop ctx was cancelled, or the executor
		// surfaced a transport-class error, is not evidence the model could
		// not make progress — skip the no-progress cooldown so the bead is
		// immediately re-claimable. If the executor itself already marked
		// report.Disrupted (e.g. a future SIGKILL/SIGTERM or server-restart
		// classifier inside ExecuteBeadWithConfig), preserve it.
		if !report.Disrupted && report.Status != ExecuteBeadStatusSuccess {
			if reason, disrupted := classifyDisruption(ctx, err); disrupted {
				report.Disrupted = true
				report.DisruptionReason = reason
			}
		}
		if report.Disrupted {
			reason := report.DisruptionReason
			if reason == "" {
				reason = "unknown"
			}
			emitDisruptionDetected(emit, w.Store, candidate.ID,
				reason, report.Detail, report.Harness, report.Model, assignee, now().UTC())
		}

		result.Attempts++

		if attemptOut.StoreErr != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError(attemptOut.StoreErrOp, assignee, result, attemptOut.StoreErr)
			})
		}

		if parking := attemptOut.Parking; parking != nil {
			if parking.Unclaim {
				if !commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("Unclaim", assignee, result, w.Store.Unclaim(candidate.ID))
				}) {
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
			}
			if parking.RunPostAttemptTriage {
				report = w.runPostAttemptTriage(ctx, candidate, report, runtime, assignee, now)
			}
			if parking.Event != nil {
				_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
					Kind:      parking.Event.Kind,
					Summary:   parking.Event.Summary,
					Body:      parking.Event.Body,
					Actor:     assignee,
					Source:    "ddx agent execute-loop",
					CreatedAt: now().UTC(),
				})
			}
			if !parking.RetryAfter.IsZero() {
				if !commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("SetExecutionCooldown", assignee, result, w.Store.SetExecutionCooldown(candidate.ID, parking.RetryAfter, report.Status, report.Detail))
				}) {
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
				report.RetryAfter = parking.RetryAfter.Format(time.RFC3339)
			}
		}

		if attemptOut.Disposition == agenttry.OutcomeSuccess {
			result.Successes++
			result.LastSuccessAt = now().UTC()
		} else if report.Status == ExecuteBeadStatusSuccess {
			appendLoopRoutingEvidence(w.Store, candidate.ID, report, now().UTC())
			// Story 15: when an operator-prompt bead succeeds, scan
			// base..result for affected beads and artifacts, and append
			// origin_operator_prompt_id back-link events. Failure is
			// logged but does not fail the attempt — the bead's own
			// commit has already landed and the audit data is best-effort.
			if candidate.IssueType == bead.IssueTypeOperatorPrompt {
				affected, affErr := computeOperatorPromptAffected(runtime.ProjectRoot, report.BaseRev, report.ResultRev, candidate.ID)
				if affErr != nil {
					if runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "operator-prompt backlinks scan: %v\n", affErr)
					}
				} else if recErr := recordOperatorPromptBacklinks(w.Store, candidate.ID, affected, assignee, now().UTC()); recErr != nil {
					if runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "operator-prompt backlinks: %v\n", recErr)
					}
				}
			}
			// Post-merge review state machine (C3 ddx-a921ff01): the close /
			// reopen / event-emission / triage logic now lives in
			// try.RunPostMergeReview so the loop only sees a structured outcome
			// (updated report + approved bool / Disposition). Store errors are
			// surfaced via StoreErrOp/StoreErr so this loop continues to drive
			// commitOutcome unchanged.
			reviewOut := RunPostMergeReview(ctx, PostMergeReviewInput{
				Bead:          candidate,
				Report:        report,
				Reviewer:      w.Reviewer,
				Store:         w.Store,
				ProjectRoot:   runtime.ProjectRoot,
				Rcfg:          rcfg,
				NoReview:      runtime.NoReview,
				Log:           runtime.Log,
				Assignee:      assignee,
				Now:           now,
				ReviewCostCap: runtime.ReviewCostCap,
			})
			report = reviewOut.Report
			reviewApproved := reviewOut.Approved
			if reviewOut.StoreErr != nil {
				if !commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError(reviewOut.StoreErrOp, assignee, result, reviewOut.StoreErr)
				}) {
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
			}

			if reviewApproved {
				result.Successes++
				result.LastSuccessAt = now().UTC()
			} else {
				result.Failures++
				result.LastFailureStatus = report.Status
			}
		} else if attemptOut.Disposition == agenttry.OutcomePark {
			result.Failures++
			result.LastFailureStatus = report.Status
		} else if attemptOut.NoChanges != nil {
			noChanges := attemptOut.NoChanges
			if !commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("Unclaim", assignee, result, w.Store.Unclaim(candidate.ID))
			}) {
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				continue
			}
			if noChanges.EventKind != "" {
				_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
					Kind:      noChanges.EventKind,
					Summary:   noChanges.EventKind,
					Body:      noChanges.EventBody,
					Actor:     assignee,
					Source:    "ddx agent execute-loop",
					CreatedAt: now().UTC(),
				})
			}
			if noChanges.Label != "" {
				if !commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("addBeadLabel", assignee, result, addBeadLabel(w.Store, candidate.ID, noChanges.Label))
				}) {
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
			}
			if noChanges.Satisfied {
				// Adjudication confirmed bead is already satisfied.
				// Set the terminal status BEFORE the close so the late
				// executeBeadLoopEvent append captures "already_satisfied"
				// (not "no_changes"), and emit an early execute-bead
				// evidence event so the closure gate accepts even when
				// BaseRev is empty (test fixtures and genuinely-no-commit
				// satisfied beads).
				report.Status = ExecuteBeadStatusAlreadySatisfied
				if noChanges.Evidence != "" {
					// Checker evidence explains why the bead is being closed;
					// it takes precedence over the executor's attempt detail.
					report.Detail = noChanges.Evidence
				}
				_ = w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC()))
				if !commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("CloseWithEvidence", assignee, result, w.Store.CloseWithEvidence(candidate.ID, report.SessionID, report.BaseRev))
				}) {
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
				result.Successes++
				result.LastSuccessAt = now().UTC()
			} else {
				// Unresolved: suppress immediate retry so the queue can
				// move on to other beads.
				report = w.runPostAttemptTriage(ctx, candidate, report, runtime, assignee, now)
				if shouldSuppressNoProgress(report) {
					retryAfter := now().UTC().Add(CapLoopCooldown(noProgressCooldown))
					if !commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("SetExecutionCooldown", assignee, result, w.Store.SetExecutionCooldown(candidate.ID, retryAfter, report.Status, report.Detail))
					}) {
						if ctx.Err() != nil {
							return result, ctx.Err()
						}
						continue
					}
					report.RetryAfter = retryAfter.Format(time.RFC3339)
				}
				result.Failures++
				result.LastFailureStatus = report.Status
			}
		} else {
			if attemptOut.Parking == nil && attemptOut.Disposition != agenttry.OutcomePark {
				if !commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("Unclaim", assignee, result, w.Store.Unclaim(candidate.ID))
				}) {
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
			}
			if report.Status == ExecuteBeadStatusPreservedNeedsReview {
				if !commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("AppendNotes", assignee, result, w.Store.AppendNotes(candidate.ID, preservedNeedsReviewNote(report)))
				}) {
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
				result.Failures++
				result.LastFailureStatus = report.Status
			} else {
				report = w.runPostAttemptTriage(ctx, candidate, report, runtime, assignee, now)
				if shouldSuppressNoProgress(report) {
					retryAfter := now().UTC().Add(CapLoopCooldown(noProgressCooldown))
					if !commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("SetExecutionCooldown", assignee, result, w.Store.SetExecutionCooldown(candidate.ID, retryAfter, report.Status, report.Detail))
					}) {
						if ctx.Err() != nil {
							return result, ctx.Err()
						}
						continue
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
				// Event recording failure is non-terminal: log it and continue.
				// result counters were already updated by the outcome block above;
				// do not double-count by re-running the outcome handler.
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "outcome store error (AppendEvent %s): %v (continuing)\n", candidate.ID, err)
				}
				if ctx.Err() == nil {
					_ = w.Store.SetExecutionCooldown(candidate.ID, now().UTC().Add(StoreErrorCooldown), "loop-error", "AppendEvent: "+err.Error())
				}
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				continue
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
			exitReason = "once_complete"
			return result, nil
		}
	}
}

// emitPickerPrioritySkips fires a picker.priority_skip event when the picker
// chose `chosen` while at least one strictly higher-priority bead was passed
// over (ddx-9d55601f AC #4). Skips at the same priority as `chosen` are NOT
// reported — those are FIFO order or label-filter siblings, not starvation.
//
// The emit is guarded so it costs nothing when the chosen bead is the highest-
// priority bead the picker saw (the common case).
func emitPickerPrioritySkips(emit func(string, map[string]any), chosen bead.Bead, skips []pickerSkip) {
	if len(skips) == 0 {
		return
	}
	var higher []map[string]any
	for _, s := range skips {
		if s.Priority < chosen.Priority {
			higher = append(higher, map[string]any{
				"bead_id":  s.BeadID,
				"priority": s.Priority,
				"reason":   s.Reason,
			})
		}
	}
	if len(higher) == 0 {
		return
	}
	emit("picker.priority_skip", map[string]any{
		"chosen_bead_id":  chosen.ID,
		"chosen_priority": chosen.Priority,
		"skipped":         higher,
	})
}

// pickerSkip records a single bead that the picker passed over while looking
// for the next candidate. It is used to emit a structured picker.priority_skip
// event when a worker claims a lower-priority bead while a higher-priority
// bead was skipped, so future starvation regressions are observable.
//
// Reason values are bounded to the set named in ddx-9d55601f AC #4:
//
//	label_filter, in_attempted, claim_race, eligibility_filter, retry_cooldown
//
// Note: eligibility_filter and retry_cooldown are applied upstream in
// Store.ReadyExecution (so beads filtered for those reasons never reach
// nextCandidate at all). They are part of the reason vocabulary so future
// picker rearrangements can re-emit them without changing the schema.
type pickerSkip struct {
	BeadID   string
	Priority int
	Reason   string
}

// nextCandidate returns the next claimable bead from the execution-ready
// queue along with the list of higher-priority beads it skipped (and the
// reason for each skip). The returned skips slice is only meaningful when
// ok=true: it contains every entry that came BEFORE the chosen candidate
// in the priority-sorted ReadyExecution result.
//
// It delegates filter+sort to PreviewQueue and then additionally filters out
// beads present in the per-Run attempted map (which is non-deterministic across
// runs and therefore excluded from the stable PreviewQueue surface).
func (w *ExecuteBeadWorker) nextCandidate(attempted map[string]struct{}, labelFilter, targetBeadID string) (bead.Bead, []pickerSkip, bool, error) {
	// Use PreviewQueue for the stable filter+sort logic. Limit=0 returns all
	// entries so we can scan for the first non-attempted candidate.
	entries, err := PreviewQueue(w.Store, PickerFilters{LabelFilter: labelFilter}, 0)
	if err != nil {
		return bead.Bead{}, nil, false, err
	}

	// Rebuild the ready list from the preview entries in picker order so we
	// can apply the per-Run attempted map on top. We need the original bead
	// values for the return; fetch them from ReadyExecution (already ordered).
	ready, err := w.Store.ReadyExecution()
	if err != nil {
		return bead.Bead{}, nil, false, err
	}
	// Index by ID for O(1) lookup.
	byID := make(map[string]bead.Bead, len(ready))
	for _, b := range ready {
		byID[b.ID] = b
	}

	var skips []pickerSkip
	for _, entry := range entries {
		candidate, ok := byID[entry.BeadID]
		if !ok {
			// Should not happen; skip defensively.
			continue
		}
		if targetBeadID != "" && candidate.ID != targetBeadID {
			skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, Reason: "target_bead"})
			continue
		}
		if _, seen := attempted[candidate.ID]; seen {
			skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, Reason: "in_attempted"})
			continue
		}
		if entry.FilterDecision == FilterDecisionSkipped {
			// PreviewQueue already applied label_filter; record as skip.
			skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, Reason: "label_filter"})
			continue
		}
		return candidate, skips, true, nil
	}
	return bead.Bead{}, skips, false, nil
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

func appendPreDispatchLintEvent(store BeadEventAppender, beadID string, result LintResult, lintErr error, threshold int, actor string, createdAt time.Time) {
	if store == nil || beadID == "" {
		return
	}
	body := map[string]any{
		"score":           result.Score,
		"rationale":       result.Rationale,
		"suggested_fixes": result.SuggestedFixes,
		"waivers_applied": result.WaiversApplied,
	}
	summary := fmt.Sprintf("score=%d", result.Score)
	if threshold > 0 {
		body["threshold_score"] = threshold
	}
	if lintErr != nil {
		body["warning"] = lintErr.Error()
		summary = "warning " + summary
	} else if threshold > 0 && result.Score < threshold {
		body["dispatch_blocked"] = true
	}
	if encoded, err := json.Marshal(body); err == nil {
		_ = store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "bead-quality.lint",
			Summary:   summary,
			Body:      string(encoded),
			Actor:     actor,
			Source:    "ddx agent execute-loop",
			CreatedAt: createdAt,
		})
		return
	}
	parts := []string{
		fmt.Sprintf("score=%d", result.Score),
		"rationale=" + result.Rationale,
		fmt.Sprintf("suggested_fixes=%v", result.SuggestedFixes),
		fmt.Sprintf("waivers_applied=%v", result.WaiversApplied),
	}
	if threshold > 0 {
		parts = append(parts, fmt.Sprintf("threshold_score=%d", threshold))
	}
	if lintErr != nil {
		parts = append(parts, "warning="+lintErr.Error())
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "bead-quality.lint",
		Summary:   summary,
		Body:      strings.Join(parts, "\n"),
		Actor:     actor,
		Source:    "ddx agent execute-loop",
		CreatedAt: createdAt,
	})
}

func executeBeadLoopEvent(report ExecuteBeadReport, actor string, createdAt time.Time) bead.BeadEvent {
	parts := []string{}
	if report.Status == ExecuteBeadStatusPreservedNeedsReview {
		parts = append(parts, preservedNeedsReviewNote(report))
	}
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
	if report.OutcomeReason != "" {
		parts = append(parts, fmt.Sprintf("outcome_reason=%s", report.OutcomeReason))
	}
	if report.PredictedPower > 0 {
		parts = append(parts, fmt.Sprintf("predicted_power=%d", report.PredictedPower))
	}
	if report.PredictedSpeedTPS > 0 {
		parts = append(parts, fmt.Sprintf("predicted_speed_tps=%.1f", report.PredictedSpeedTPS))
	}
	if report.PredictedCostUSDPer1kTokens > 0 {
		cost := fmt.Sprintf("predicted_cost_usd_per_1k_tokens=%.6f", report.PredictedCostUSDPer1kTokens)
		if report.PredictedCostSource != "" {
			cost += fmt.Sprintf(" source=%s", report.PredictedCostSource)
		}
		parts = append(parts, cost)
	} else if report.PredictedCostSource != "" {
		parts = append(parts, fmt.Sprintf("predicted_cost_source=%s", report.PredictedCostSource))
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

func preservedNeedsReviewNote(report ExecuteBeadReport) string {
	parts := []string{
		"preserved-needs-review",
		"gate_summary=" + oneLineGateSummary(report.Detail),
	}
	if report.PreserveRef != "" {
		parts = append(parts, "preserve_ref="+report.PreserveRef)
	}
	if report.ResultRev != "" {
		parts = append(parts, "result_rev="+report.ResultRev)
	}
	return strings.Join(parts, "\n")
}

func oneLineGateSummary(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return "safety gate preserved result"
	}
	if idx := strings.IndexByte(detail, '\n'); idx >= 0 {
		detail = strings.TrimSpace(detail[:idx])
	}
	return detail
}

// writeLoopEvent emits one structured JSONL line to sink describing a
// milestone in an execute-bead loop run. Entries use the same envelope as
// the Fizeau harness (session_id/seq/type/ts/data) so existing log
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

// addBeadLabel mutates the bead in place to add a label idempotently. The
// store handles persistence; concurrent callers serialize via the store lock.
func addBeadLabel(store ExecuteBeadLoopStore, beadID, label string) error {
	if label == "" {
		return nil
	}
	return store.Update(beadID, func(b *bead.Bead) {
		for _, existing := range b.Labels {
			if existing == label {
				return
			}
		}
		b.Labels = append(b.Labels, label)
	})
}

func shouldSuppressNoProgress(report ExecuteBeadReport) bool {
	if report.Disrupted {
		// ddx-5b3e57f4: a worker-disrupted attempt is not evidence the model
		// could not make progress. Skip the 6h no-progress cooldown so the
		// bead is immediately re-claimable by the next worker.
		return false
	}
	if isTransientOutcomeReason(report.OutcomeReason) {
		return false
	}
	if report.BaseRev == "" || report.ResultRev == "" {
		return false
	}
	return report.BaseRev == report.ResultRev
}

func isTransientOutcomeReason(reason string) bool {
	switch reason {
	case "transport", "quota", "routing", "timeout", "merge_conflict":
		return true
	default:
		return false
	}
}

// classifyDisruption examines the loop ctx and the executor's error to decide
// whether a failed ExecuteBead attempt was caused by external disruption
// (worker killed, context cancelled, transport error) rather than the model
// genuinely producing nothing. Returns ok=true plus a short reason kind when
// disruption is detected; the caller marks the report Disrupted to bypass the
// no-progress cooldown (ddx-5b3e57f4). Reason kinds are stable strings used
// in the disruption_detected event:
//
//	context_canceled, context_deadline, transport_error
func classifyDisruption(ctx context.Context, executorErr error) (string, bool) {
	if ctx != nil {
		switch ctx.Err() {
		case context.Canceled:
			return "context_canceled", true
		case context.DeadlineExceeded:
			return "context_deadline", true
		}
	}
	if executorErr == nil {
		return "", false
	}
	if isTransportError(executorErr) {
		return "transport_error", true
	}
	return "", false
}

// isTransportError returns true when err looks like a transport-class failure
// (network reset, connection refused, TLS error, gateway, provider 5xx). The
// signal is fuzzy on purpose: callers (classifyDisruption) only use it to
// flag a failure as Disrupted so the bead skips the long no-progress cooldown
// and is retried promptly. False positives are cheap (one quick retry); false
// negatives are expensive (6h park for a transient outage).
func isTransportError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"connection refused",
		"connection reset",
		"connection closed",
		"no route to host",
		"network is unreachable",
		"i/o timeout",
		"context deadline exceeded",
		"tls handshake",
		"eof",
		"broken pipe",
		"bad gateway",
		"service unavailable",
		"gateway timeout",
		"transport",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// emitDisruptionDetected records a structured `disruption_detected` event for
// the bead and the loop event sink so operators can see disruption rates and
// which class is firing (ddx-5b3e57f4 AC #5).
func emitDisruptionDetected(emit func(string, map[string]any), store BeadEventAppender, beadID, reason, detail, harness, model, assignee string, now time.Time) {
	emit("disruption_detected", map[string]any{
		"bead_id": beadID,
		"reason":  reason,
		"detail":  detail,
		"harness": harness,
		"model":   model,
	})
	if store == nil || beadID == "" {
		return
	}
	body, err := json.Marshal(map[string]any{
		"reason":  reason,
		"detail":  detail,
		"harness": harness,
		"model":   model,
	})
	bodyStr := detail
	if err == nil {
		bodyStr = string(body)
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "disruption_detected",
		Summary:   reason,
		Body:      bodyStr,
		Actor:     assignee,
		Source:    "ddx agent execute-loop",
		CreatedAt: now,
	})
}

// ClassifyReviewError maps a reviewer error and partial result into one of the
// four FEAT-022 §12 taxonomy classes. Resolution order:
//  1. reviewRes.Error if it's already one of the canonical class identifiers
//     (the reviewer set it explicitly — preferred path).
//  2. The error string text (for backwards-compatible reviewers that only
//     embed the class in their message).
//  3. ErrReviewVerdictUnparseable as a fallback when the strict-parse error
//     leaks through a reviewer that did not set Error.
//  4. Default to transport (network/provider-side failure) when nothing else
//     matches.
func ClassifyReviewError(reviewErr error, reviewRes *ReviewResult) string {
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

// CountPriorReviewErrors returns the number of `review-error` events already
// recorded against this bead whose body cites the given result_rev. This is
// the FEAT-022 §14 retry counter — it is event-scoped (no separate counter
// field on the bead) so a fresh result_rev naturally resets the count.
func CountPriorReviewErrors(store ExecuteBeadLoopStore, beadID, resultRev string) int {
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

// ReviewErrorEventBody is the canonical body shape for review-error and
// review-manual-required events. It carries the failure class, attempt count,
// and result_rev as discrete lines so operators can grep without parsing the
// full reviewer error text. The trailing free-form message is the raw
// reviewer-error string for forensics.
func ReviewErrorEventBody(class string, attemptCount int, resultRev, message string) string {
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

// ReviewCostDeferredEventBody records that review would have exceeded the
// configured cost cap after charging the reviewer cost against the shared
// loop accumulator.
func ReviewCostDeferredEventBody(resultRev string, reviewCostUSD, spentUSD, maxCostUSD float64) string {
	return fmt.Sprintf("result_rev=%s\nreview_cost_usd=%.4f\nspent_usd=%.4f\nmax_cost_usd=%.4f", resultRev, reviewCostUSD, spentUSD, maxCostUSD)
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

// sleepOrWake sleeps for d unless ctx is cancelled or a wake signal arrives on
// wakeCh. A wake signal returns nil (the loop re-scans the queue) so callers
// like operatorPromptApprove can shorten the next-tick latency from a full
// PollInterval to ~0. wakeCh may be nil — in which case this is equivalent to
// sleepWithContext.
func sleepOrWake(ctx context.Context, d time.Duration, wakeCh <-chan struct{}) error {
	if d <= 0 {
		return nil
	}
	if wakeCh == nil {
		return sleepWithContext(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	case <-wakeCh:
		return nil
	}
}
