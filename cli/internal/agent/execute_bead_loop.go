package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	agentlib "github.com/easel/fizeau"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// ExecuteBeadLoopRuntime carries the non-serializable plumbing and
// per-invocation runtime intent for an execute-bead loop run. Durable knobs
// (assignee, retry caps, harness/model, powerClass bounds, etc.) live on
// config.ResolvedConfig and are passed via Run's rcfg argument.
type ExecuteBeadLoopRuntime struct {
	Log             io.Writer
	CleanupLog      io.Writer
	CleanupRunner   executionCleanupRunner
	ResourceChecker ExecutionResourceChecker
	CleanupInterval time.Duration
	CleanupTickCh   <-chan time.Time
	EventSink       io.Writer
	ProgressCh      chan<- ProgressEvent
	PreClaimHook    func(ctx context.Context) error
	// PreClaimIntakeHook runs after routing preflight and before Claim. It
	// classifies the candidate for actionability/scope; only
	// actionable_atomic proceeds directly to Claim. Non-atomic outcomes
	// skip the bead without claim. intake_error fail-opens so infrastructure
	// failures do not become hidden dispatch failures.
	PreClaimIntakeHook    PreClaimIntakeHook
	PreDispatchLintHook   func(ctx context.Context, beadID string) (LintResult, error)
	PostAttemptTriageHook func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error)
	// ProseEvidenceHook runs after a successful (landed) attempt and before
	// CloseWithEvidence so docs-changing attempts attach advisory prose-check
	// evidence to the bead's event history. Errors are logged and ignored;
	// prose findings never block closure.
	ProseEvidenceHook ProseEvidenceHook
	// PostAttemptDecompositionHook is called when a no_changes attempt signals
	// orchestrator_action: decompose in its rationale, indicating the bead is too
	// large for implementation-level splitting. The hook should run the same
	// orchestrator splitter used by the intake gate and return child specs and an
	// AC map. A nil return or error falls back to operator-required parking.
	PostAttemptDecompositionHook func(ctx context.Context, beadID string) (*PreClaimDecomposition, error)
	// ReviewCostCap, when non-nil, accumulates reviewer cost on the same
	// loop budget tracker used by the implementer attempts.
	ReviewCostCap *escalation.CostCapTracker
	// RoutePreflight, when non-nil, runs once during Run bootstrap before
	// the drain loop starts. It is expected to call upstream ResolveRoute
	// against the loop's resolved (harness, model) and return whatever typed
	// routing error the upstream surfaced — notably
	// agent.ErrHarnessModelIncompatible when the harness allow-list rejects
	// the model. Any non-nil error stops the worker before any bead is
	// claimed; DDx does NOT duplicate the upstream allow-list.
	RoutePreflight func(ctx context.Context, harness, model string) error
	// PreClaimTimeout bounds the pre-claim readiness hooks. Zero means use the
	// binary default so a hanging readiness check cannot park the worker
	// forever.
	PreClaimTimeout time.Duration
	// BudgetStop, when non-nil, is checked before selecting the next bead. It
	// lets CLI/server callers surface already-tripped budget state as a typed
	// work-layer StopCondition before any new bead is claimed. The companion
	// report is still appended for operator visibility, but the stop itself is
	// classified through work.StopConditionBudget.
	BudgetStop func() (work.StopDecision, ExecuteBeadReport, bool)
	// BinaryRefreshCheck, when non-nil, is checked before selecting the next
	// bead. A true result means the caller has started a replacement worker
	// with equivalent arguments and this loop should exit before claiming work.
	// Errors are logged and fail open so update-probing never blocks the queue.
	BinaryRefreshCheck func(ctx context.Context) (bool, error)
	// Mode and IdleInterval are the runtime loop intent. Once and
	// PollInterval remain for older tests/callers but production entry points
	// should set Mode and IdleInterval directly.
	Mode         executeloop.Mode
	IdleInterval time.Duration
	Once         bool
	PollInterval time.Duration
	NoReview     bool
	LabelFilter  string
	SessionID    string
	WorkerID     string
	ProjectRoot  string
	// TargetBeadID, when non-empty, restricts nextCandidate to only return the
	// named bead from the execution-ready queue. Used by `ddx try <bead-id>`
	// to dispatch a single specific bead through the same claim → executor →
	// land path the queue drain uses. When empty, the picker behaves normally.
	TargetBeadID string
	// WakeCh, when non-nil, signals the idle-poll sleep to return immediately
	// so the loop re-scans the queue. Used by the operator-prompt approve /
	// auto-approve mutations (Story 15) to avoid an idle-interval-sized delay
	// before a freshly-approved bead is claimed. Implementations must send
	// non-blocking (server-side helpers do); the loop only waits for a
	// receive on WakeCh during the idle sleep, never elsewhere.
	WakeCh <-chan struct{}

	// OnAttemptFinalized, when non-nil, is called once per finalized attempt
	// immediately after the report is appended to loop results. Best-effort:
	// the hook must not block and its errors are silently discarded.
	OnAttemptFinalized func(report ExecuteBeadReport)
	// PostLadderExhaustionHook, when non-nil, is called when a bead's
	// consecutive_ladder_exhaustions counter reaches the auto-recovery
	// threshold (>= 2). A nil hook or Attempted=false result falls through to
	// the existing loop path unchanged.
	PostLadderExhaustionHook PostLadderExhaustionHook
}

func (r ExecuteBeadLoopRuntime) loopIntent() (executeloop.Mode, time.Duration) {
	mode := r.Mode
	if mode == "" {
		if r.Once {
			mode = executeloop.ModeOnce
		} else {
			mode = executeloop.ModeDrain
		}
	}
	idleInterval := r.IdleInterval
	if idleInterval == 0 && mode == executeloop.ModeWatch {
		idleInterval = r.PollInterval
	}
	if idleInterval == 0 && mode == executeloop.ModeWatch {
		idleInterval = 30 * time.Second
	}
	return mode, idleInterval
}

// DefaultReviewMaxRetries is the number of reviewer attempts allowed per
// committed result_rev before the loop emits a terminal
// `review-manual-required` event and stops re-executing primary. FEAT-022 §14.
const DefaultReviewMaxRetries = 3

// ReviewerSkippedEmptyDiffEventKind is the event kind emitted when the
// reviewer dispatch is skipped because the implementer produced no commits
// (BaseRev == ResultRev). Per ADR-024 P1: cheapest first — paying a reviewer
// to evaluate an empty diff is unjustified cost.
const ReviewerSkippedEmptyDiffEventKind = "reviewer-skipped-empty-diff"

// MaxLoopCooldown is the absolute upper bound the work will set for
// any work-retry-after value. Year-scale parks effectively mean
// "never retry" and that should be a deliberate operator choice via
// `ddx bead update --set work-retry-after=...`, not an automatic
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

// ProviderUnavailableCooldown is the retry pause for route/provider outages
// where no configured powerClass can currently run the bead. This is transient
// scheduler state, not bead quality evidence, so it must not become a durable
// triage:needs-investigation park.
const ProviderUnavailableCooldown = 15 * time.Minute

// SmartRetryCooldown is the queue-fairness pause after a real no_changes
// implementation attempt asks for status=open/smart retry. The bead remains
// open with its powerClass hint, but a watch worker moves on instead of repeating the
// same empty-diff attempt every idle cycle.
const SmartRetryCooldown = 5 * time.Minute

// PausedInfraInterval is the polling interval the worker waits in paused-infra
// state before re-evaluating the queue. Infra failures (no_viable_provider,
// provider_connectivity) transition the worker to this state instead of parking
// individual beads: beads remain immediately reclaimable, and the worker retries
// after this window.
const PausedInfraInterval = 2 * time.Minute

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

// handlePostAttemptDecomposition runs the orchestrator-level splitter when a
// no_changes attempt signals orchestrator_action: decompose. It checks the
// queue-level max_decomposition_depth (not the implementation prompt cap),
// validates the AC map for completeness, and either creates children+deps or
// parks the parent for operator review if the split is lossy or depth-capped.
// The bead must already be unclaimed before this is called.
func (w *ExecuteBeadWorker) handlePostAttemptDecomposition(ctx context.Context, candidate *bead.Bead, runtime ExecuteBeadLoopRuntime, assignee string, rcfg config.ResolvedConfig, at time.Time) {
	emit := func(kind string, body map[string]any) {
		if runtime.EventSink == nil {
			return
		}
		data, _ := json.Marshal(map[string]any{"event": kind, "payload": body})
		_, _ = fmt.Fprintf(runtime.EventSink, "%s\n", data)
	}
	parkOperator := func(reason string) {
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "post-attempt decomposition blocked: %s (%s)\n", reason, candidate.ID)
		}
		emit("post_attempt_decomposition.blocked", map[string]any{
			"bead_id": candidate.ID,
			"reason":  reason,
		})
		_, berr := parkBeadPostIntakeRejection(w.Store, candidate, assignee, PreClaimIntakeOperatorRequired, "operator_required", reason, at)
		if berr != nil {
			if runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "post-attempt decomposition park error: %v\n", berr)
			}
		}
	}

	// Queue-level depth check: orchestrator uses its own cap, not the
	// implementation prompt's hardcoded depth-2 cap.
	maxDepth := rcfg.MaxDecompositionDepth()
	if maxDepth > 0 && storeBeadDepth(ctx, w.Store, candidate) >= maxDepth {
		overflowBody, _ := json.Marshal(map[string]any{
			"depth": storeBeadDepth(ctx, w.Store, candidate),
			"max":   maxDepth,
		})
		_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
			Kind:      "triage-overflow",
			Summary:   "depth cap exceeded during post-attempt decomposition",
			Body:      string(overflowBody),
			Actor:     assignee,
			Source:    "ddx work",
			CreatedAt: at,
		})
		parkOperator("queue-level depth cap exceeded; operator must split")
		return
	}

	decomp, err := runtime.PostAttemptDecompositionHook(ctx, candidate.ID)
	if err != nil {
		parkOperator(fmt.Sprintf("decomposition hook error: %s", err.Error()))
		return
	}
	if decomp == nil {
		parkOperator("decomposition hook returned no split")
		return
	}
	lossyOrEmpty := isDecompositionLossy(decomp.ACMap) || (len(decomp.ACMap) == 0 && strings.TrimSpace(candidate.Acceptance) != "")
	if lossyOrEmpty {
		parkOperator("decomposition AC map is incomplete; operator must produce a lossless split")
		return
	}

	childIDs, decompErr := applyPreClaimDecomposition(ctx, w.Store, candidate, decomp, assignee, at)
	if decompErr != nil {
		parkOperator(fmt.Sprintf("decomposition apply error: %s", decompErr.Error()))
		return
	}
	if runtime.Log != nil {
		_, _ = fmt.Fprintf(runtime.Log, "post-attempt decomposition: bead %s split into %s\n", candidate.ID, strings.Join(childIDs, ", "))
	}
	emit("post_attempt_decomposition.applied", map[string]any{
		"bead_id":   candidate.ID,
		"child_ids": childIDs,
	})
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
	if isDeterministicSystemOutcomeReason(report.OutcomeReason) {
		return report
	}
	triage, err := hook(ctx, candidate.ID, report)
	if err != nil {
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "post-attempt triage error (%s %s): %v (continuing)\n", candidate.ID, report.Status, err)
		}
		return report
	}
	triage = normalizeTriageResult(triage)
	hasWarnings := len(triage.DecodeWarnings) > 0 || triage.Malformed
	classificationRecognized := isRecognizedTriageClassification(triage.Classification)
	if hasWarnings && runtime.Log != nil {
		_, _ = fmt.Fprintf(runtime.Log, "post-attempt triage warning (%s %s): %s (continuing)\n", candidate.ID, report.Status, formatTriageWarnings(triage.DecodeWarnings))
	}
	if !classificationRecognized && !hasWarnings {
		return report
	}
	if classificationRecognized {
		report.OutcomeReason = triage.Classification
	}
	recordPostAttemptTriageEvent(w.Store, candidate.ID, report, triage, assignee, now().UTC())
	return report
}

// emitReviewerSkippedEmptyDiff records the structured event that fires when
// reviewer dispatch is skipped because the implementer produced no commits.
func emitReviewerSkippedEmptyDiff(store BeadEventAppender, beadID, assignee string, at time.Time) {
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      ReviewerSkippedEmptyDiffEventKind,
		Summary:   "reviewer skipped: empty diff (no commits produced)",
		Actor:     assignee,
		Source:    "ddx work",
		CreatedAt: at,
	})
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
	// ImplementationRev is the worker's own commit SHA before landing.
	// Mirrors ExecuteBeadResult.ImplementationRev; empty for no-changes attempts.
	ImplementationRev string `json:"implementation_rev,omitempty"`
	// LandedRev is the target branch tip after the coordinator landing.
	// Distinct from ImplementationRev on the merge-commit path.
	LandedRev string `json:"landed_rev,omitempty"`
	// TargetBranch is the resolved landing branch. The JSON field is named
	// landed_branch because it denotes the branch that now carries the result.
	TargetBranch string `json:"landed_branch,omitempty"`
	// EvidenceRev is the trailing evidence commit SHA when distinct from
	// ImplementationRev. Empty when not separately committed.
	EvidenceRev string `json:"evidence_rev,omitempty"`
	// ProjectRoot is the worktree root ddx try/work operated on for this report.
	ProjectRoot string `json:"project_root,omitempty"`
	PreserveRef string `json:"preserve_ref,omitempty"`
	// CandidateRef is the project-root git ref pinned before checks and review.
	// Format: refs/ddx/iterations/<attempt-id>/<cycle-index>.
	CandidateRef string `json:"candidate_ref,omitempty"`
	// CycleIndex is the zero-based repair-cycle index for this candidate.
	CycleIndex int    `json:"cycle_index,omitempty"`
	RetryAfter string `json:"retry_after,omitempty"`
	// NoChangesRationale carries the agent's explanation when status == no_changes.
	NoChangesRationale string `json:"no_changes_rationale,omitempty"`
	// ReviewVerdict is the post-merge review verdict (APPROVE, REQUEST_CHANGES,
	// or BLOCK) when a reviewer ran. Empty when review was skipped.
	ReviewVerdict string `json:"review_verdict,omitempty"`
	// ReviewRationale carries the actionable reviewer-authored findings for
	// non-APPROVE review outcomes.
	ReviewRationale string `json:"review_rationale,omitempty"`
	// CycleTrace carries the append-only execution cycle trace in order.
	CycleTrace []ExecutionCycleTrace `json:"cycle_trace,omitempty"`
	// PowerClass is the model powerClass used for the final attempt (cheap, standard, smart).
	// Populated by powerClass-escalating executors; empty for single-power attempts.
	PowerClass string `json:"power_class,omitempty"`
	// ProbeResult is a brief summary of the provider health probe at attempt time.
	ProbeResult string `json:"probe_result,omitempty"`
	// CostUSD is the dollar cost of this attempt as reported by the harness.
	// Power-escalating executors propagate this so the escalation trace can
	// compute wasted/effective spend.
	CostUSD float64 `json:"cost_usd,omitempty"`
	// DurationMS is the wall-clock duration of this attempt.
	DurationMS int64 `json:"duration_ms,omitempty"`
	// Profile routing telemetry. Populated when work uses a profile
	// ladder rather than an explicit harness/model pin.
	RequestedProfile   string `json:"requested_profile,omitempty"`
	InferredPowerClass string `json:"inferred_power_class,omitempty"`
	RoutingIntentNote  string `json:"routing_intent_note,omitempty"`
	ResolvedPowerClass string `json:"resolved_power_class,omitempty"`
	EscalationCount    int    `json:"escalation_count,omitempty"`
	FinalPowerClass    string `json:"final_power_class,omitempty"`
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
	// rejection). The work bypasses the no-progress cooldown for
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
	// ResourceExhausted carries the execution-root preflight result when the
	// attempt stopped before the agent could safely continue.
	ResourceExhausted any `json:"resource_exhausted,omitempty"`
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
	Get(args ...any) (*bead.Bead, error)
	Create(args ...any) error
	Claim(id, assignee string) error
	Unclaim(id string) error
	TouchClaimHeartbeat(id string) error
	CloseWithEvidence(id, sessionID, commitSHA string) error
	AppendEvent(id string, event bead.BeadEvent) error
	Events(id string) ([]bead.BeadEvent, error)
	SetExecutionCooldown(id string, until time.Time, status, detail, baseRev string) error
	AppendNotes(id string, notes string) error
	IncrNoChangesCount(id string) (int, error)
	// Reopen sets a closed bead back to open, appending notes to the bead's
	// Notes field and recording a reopen event. Used by review workflows that
	// intentionally transition a bead back into open state.
	Reopen(id, reason, notes string) error
	// Update mutates a bead in place. Used by the post-merge triage step to
	// add metadata hints (e.g. powerClass-pin) when the triage policy escalates after
	// repeated review BLOCKs.
	Update(args ...any) error
	UpdateWithLifecycleStatus(id string, status string, opts bead.LifecycleTransitionOptions, mutate func(*bead.Bead) error) error
	ParkToProposed(id string, reason bead.ParkReason, mutate func(*bead.Bead)) error
}

// readyDiagnoser is the optional interface the work loop uses to explain an
// empty execution queue. bead.Store satisfies it via ReadyExecutionBreakdown.
type readyDiagnoser interface {
	ReadyExecutionBreakdown() (bead.ReadyExecutionBreakdown, error)
}

// NoReadyWorkBreakdown explains why the execution-ready queue is empty.
// Populated on an ExecuteBeadLoopResult when NoReadyWork fires and the store
// exposes ReadyExecutionBreakdown.
type NoReadyWorkBreakdown struct {
	ExecutionReady            []string `json:"execution_ready,omitempty"`
	DependencyWaiting         []string `json:"dependency_waiting,omitempty"`
	ProposedOperatorAttention []string `json:"proposed_operator_attention,omitempty"`
	RetryCooldown             []string `json:"retry_cooldown,omitempty"`
	ExternalBlocked           []string `json:"external_blocked,omitempty"`
	NotEligible               []string `json:"not_eligible,omitempty"`
	Superseded                []string `json:"superseded,omitempty"`
	Epics                     []string `json:"epics,omitempty"`
	EpicClosureCandidates     []string `json:"epic_closure_candidates,omitempty"`
	NextRetryAfter            string   `json:"next_retry_after,omitempty"`
}

// QueueSnapshot is the structured count-oriented companion to the legacy
// no_ready_work_detail ID lists.
type QueueSnapshot struct {
	ExecutionReadyCount            int                          `json:"execution_ready_count"`
	BlockedCount                   int                          `json:"blocked_count"`
	DependencyWaitingCount         int                          `json:"dependency_waiting_count"`
	ExternalBlockedCount           int                          `json:"external_blocked_count"`
	ProposedOperatorAttentionCount int                          `json:"proposed_operator_attention_count"`
	HumanReviewBlockerCount        int                          `json:"human_review_blocker_count"`
	HumanReviewBlockedTotal        int                          `json:"human_review_blocked_total"`
	HumanReviewBlockers            []HumanReviewBlockerSnapshot `json:"human_review_blockers,omitempty"`
	RetryCooldownCount             int                          `json:"retry_cooldown_count"`
	NextRetryAfter                 string                       `json:"next_retry_after,omitempty"`
	ExecutionIneligibleCount       int                          `json:"execution_ineligible_count"`
	SupersededCount                int                          `json:"superseded_count"`
	SkippedEpicsCount              int                          `json:"skipped_epics_count"`
	EpicClosureCandidatesCount     int                          `json:"epic_closure_candidates_count"`
}

// HumanReviewBlockerSnapshot reports one open human-review blocker and how
// many active downstream beads are transitively waiting on it.
type HumanReviewBlockerSnapshot struct {
	ID                     string `json:"id"`
	Title                  string `json:"title"`
	Priority               int    `json:"priority"`
	DownstreamBlockedCount int    `json:"downstream_blocked_count"`
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

// OperatorAttentionStop captures a project-level terminal condition that must
// stop autonomous work until an operator changes the local environment.
type OperatorAttentionStop struct {
	Reason      string   `json:"reason"`
	BeadID      string   `json:"bead_id,omitempty"`
	ProjectRoot string   `json:"project_root,omitempty"`
	DirtyPaths  []string `json:"dirty_paths,omitempty"`
	Message     string   `json:"message,omitempty"`
}

type ExecuteBeadLoopResult struct {
	Attempts          int                    `json:"attempts"`
	Successes         int                    `json:"successes"`
	Failures          int                    `json:"failures"`
	StopCondition     string                 `json:"stop_condition,omitempty"`
	ExitReason        string                 `json:"exit_reason,omitempty"`
	OperatorAttention *OperatorAttentionStop `json:"operator_attention,omitempty"`
	NoReadyWork       bool                   `json:"no_ready_work,omitempty"`
	NoReadyWorkDetail NoReadyWorkBreakdown   `json:"no_ready_work_detail,omitempty"`
	QueueSnapshot     *QueueSnapshot         `json:"queue_snapshot,omitempty"`
	LastSuccessAt     time.Time              `json:"last_success_at,omitempty"`
	LastFailureStatus string                 `json:"last_failure_status,omitempty"`
	Results           []ExecuteBeadReport    `json:"results,omitempty"`
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
	// Reviewer is retained for legacy/manual review helper tests. Automated
	// close eligibility is owned by pre-land candidate-cycle review, so the
	// work success path no longer calls this post-land reviewer.
	Reviewer BeadReviewer
	// ComplexityGate, when non-nil, is consulted before a bead is claimed.
	// The zero value fail-opens once and then allows.
	ComplexityGate work.Guard

	// ConflictResolver, when non-nil, is called after the 3-way ort auto-merge
	// step fails to recover a preserved iteration. The implementation should
	// attempt a focused conflict-resolution pass (e.g. a cheap-powerClass agent run
	// with the conflict files and bead AC) and return the new merged tip SHA on
	// success. isBlocking=true signals the conflict requires human judgment
	// (escalating to land_conflict_operator_required); false means failed-but-retriable.
	ConflictResolver func(ctx context.Context, beadID, preserveRef, projectRoot string) (newTip string, isBlocking bool, err error)

	// EscalationNextFloor, when non-nil, is called by the no-changes smart-retry
	// path to advance the MinPower floor by exactly one ladder step above
	// actualPower. Returns an error (e.g. ErrLadderExhausted) when already at the
	// top powerClass, causing the bead to be parked to status=proposed.
	EscalationNextFloor func(actualPower int) (int, error)

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

const preExecuteCheckpointDirtyMarker = "pre-execute-bead checkpoint: " + preDispatchCheckpointDirtyRefusalPrefix

func preExecuteCheckpointDirtyStop(report ExecuteBeadReport, err error, projectRoot, beadID string) (*OperatorAttentionStop, bool) {
	detail := strings.TrimSpace(firstNonEmpty(report.Detail, report.Error))
	if err != nil && strings.Contains(err.Error(), preExecuteCheckpointDirtyMarker) {
		detail = strings.TrimSpace(err.Error())
	} else if !strings.Contains(detail, preExecuteCheckpointDirtyMarker) {
		return nil, false
	}
	paths := parsePreExecuteCheckpointDirtyPaths(detail)
	message := "commit or clean the listed implementation files before restarting ddx work"
	if len(paths) > 0 {
		message = fmt.Sprintf("commit or clean the listed implementation files before restarting ddx work: %s", strings.Join(paths, ", "))
	}
	return &OperatorAttentionStop{
		Reason:      "checkpoint_dirty",
		BeadID:      beadID,
		ProjectRoot: projectRoot,
		DirtyPaths:  paths,
		Message:     message,
	}, true
}

func parsePreExecuteCheckpointDirtyPaths(detail string) []string {
	idx := strings.Index(detail, preDispatchCheckpointDirtyRefusalPrefix)
	if idx == -1 {
		return nil
	}
	pathsField := detail[idx+len(preDispatchCheckpointDirtyRefusalPrefix):]
	if semi := strings.Index(pathsField, ";"); semi >= 0 {
		pathsField = pathsField[:semi]
	}
	var paths []string
	for _, path := range strings.Split(pathsField, ",") {
		path = strings.TrimSpace(path)
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
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
	preClaimTimeout := runtime.PreClaimTimeout
	if preClaimTimeout <= 0 {
		preClaimTimeout = work.DefaultPreClaimTimeout
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
	provider := rcfg.Provider()
	model := rcfg.Model()
	profile := rcfg.Profile()
	loopMode, idleInterval := runtime.loopIntent()

	result := &ExecuteBeadLoopResult{}
	resultsResetIdx := 0
	// pausedInfraUntil is set when a no_viable_provider outcome is recorded.
	// The worker sleeps until this time (in Watch mode) before re-evaluating
	// the queue, leaving all beads immediately reclaimable. Zero means no pause.
	var pausedInfraUntil time.Time
	complexityGuard := work.NewComplexityGuard(w.ComplexityGate, runtime.Log)
	preclaimGuard := work.NewPreClaimGuard(runtime.PreClaimHook, w.Store, runtime.Log, now, 30*time.Second, preClaimTimeout)
	workLog := NewWorkLogRenderer(WorkLogRendererOptions{Now: now})
	wasIdle := false
	lastIdleQueueSignature := ""

	// Worker-side liveness sidecar. Updated on each heartbeat tick so an
	// operator's `ddx work focus` / GraphQL panel can answer "is this worker
	// alive and what is it doing?" without paying tracker rewrite cost. The
	// sidecar is best-effort: write/sink failures do not interrupt the loop.
	// The reporter is constructed here but does NOT emit until a bead is
	// claimed — that order keeps loop.start as the first envelope on the
	// EventSink so structured-event consumers continue to see it.
	var liveness *work.SidecarLivenessReporter
	if runtime.ProjectRoot != "" && runtime.SessionID != "" {
		liveness = work.NewSidecarLivenessReporter(runtime.ProjectRoot, runtime.SessionID, runtime.SessionID, runtime.EventSink)
	}

	emit := func(eventType string, data map[string]any) {
		writeLoopEvent(runtime.EventSink, runtime.SessionID, eventType, data, now().UTC())
	}
	stopAfterNonAttemptSkip := func() bool {
		return loopMode == executeloop.ModeOnce && runtime.TargetBeadID != ""
	}
	strictIntakeBlocking := func() bool {
		return runtime.TargetBeadID != ""
	}

	emit("loop.start", map[string]any{
		"worker_id":    runtime.WorkerID,
		"project_root": runtime.ProjectRoot,
		"harness":      harness,
		"model":        model,
		"session_id":   runtime.SessionID,
		"assignee":     assignee,
		"mode":         string(loopMode),
		"once":         loopMode == executeloop.ModeOnce,
	})
	// exitReason is populated as the loop exits to surface a structured reason
	// in the loop.end event (ddx-dc157075 AC #4). Work-owned terminal reasons
	// are classified through work.StopCondition; fatal_config,
	// preflight_failed, resource_exhausted, and future provider exhaustion are
	// still subsystem-specific exits.
	exitReason := ""
	cleanupLog := runtime.CleanupLog
	if cleanupLog == nil {
		cleanupLog = runtime.Log
	}
	cleanupStop := startExecutionCleanupWorker(ctx, runtime.ProjectRoot, runtime.CleanupRunner, runtime.CleanupInterval, runtime.CleanupTickCh, cleanupLog, emit)
	attemptStarted := false
	defer func() {
		cleanupStop(ctx.Err() != nil && attemptStarted)
	}()
	_, _, _ = runExecutionCleanupPass(ctx, runtime.ProjectRoot, runtime.CleanupRunner, cleanupLog, emit, "startup")
	setExit := func(condition, reason string) {
		exitReason = reason
		result.StopCondition = condition
		result.ExitReason = reason
	}
	applyStop := func(input work.StopInput) bool {
		decision, ok := work.ClassifyStop(input)
		if !ok {
			return false
		}
		setExit(string(decision.Condition), decision.ExitReason)
		return true
	}
	if runtime.RoutePreflight != nil {
		if rerr := runtime.RoutePreflight(ctx, harness, model); rerr != nil {
			detail := fmt.Sprintf("routing preflight rejected (harness=%s model=%s): %s",
				harness, model, rerr.Error())
			if runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "routing preflight: %s (startup)\n", detail)
			}
			emit("preflight.rejected", map[string]any{
				"harness": harness,
				"model":   model,
				"reason":  rerr.Error(),
				"startup": true,
			})
			report := ExecuteBeadReport{
				Status:           ExecuteBeadStatusExecutionFailed,
				Detail:           detail,
				Harness:          harness,
				Model:            model,
				Disrupted:        true,
				DisruptionReason: "preflight_rejected",
				OutcomeReason:    "preflight_failed",
			}
			emitDisruptionDetected(emit, w.Store, "", "preflight_rejected", detail, harness, model, assignee, now().UTC())
			result.Failures++
			result.LastFailureStatus = report.Status
			result.Results = append(result.Results, report)
			setExit("Preflight", "preflight_failed")
			return result, nil
		}
	}
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
		_, _, _ = runExecutionCleanupPass(ctx, runtime.ProjectRoot, runtime.CleanupRunner, cleanupLog, emit, "pre-claim")
		// Respect context cancellation between iterations. Without this check,
		// a Stop() request (which cancels ctx) would only take effect during
		// the idle poll sleep — the loop would happily claim the next ready
		// bead as soon as the current Execute returned, ignoring the cancel.
		if err := ctx.Err(); err != nil {
			if exitReason == "" {
				applyStop(work.StopInput{ContextErr: err})
			}
			return result, err
		}
		if runtime.BudgetStop != nil {
			if budgetDecision, budgetReport, stopped := runtime.BudgetStop(); stopped {
				if budgetDecision == (work.StopDecision{}) {
					budgetDecision = work.StopDecision{Condition: work.StopConditionBudget, ExitReason: "budget"}
				}
				setExit(string(budgetDecision.Condition), budgetDecision.ExitReason)
				if budgetReport.Status == "" {
					budgetReport.Status = ExecuteBeadStatusExecutionFailed
				}
				if budgetReport.Detail != "" && runtime.Log != nil {
					_, _ = fmt.Fprintln(runtime.Log, budgetReport.Detail)
				}
				result.Failures++
				result.LastFailureStatus = budgetReport.Status
				result.Results = append(result.Results, budgetReport)
				return result, nil
			}
		}
		if runtime.ResourceChecker != nil {
			checkResult, checkErr := runtime.ResourceChecker.Check(ctx)
			emitResourcePreflight(emit, "pre-claim", checkResult, checkErr)
			logResourcePreflight(runtime.Log, "pre-claim", checkResult, checkErr)
			if checkErr != nil {
				var resourceErr *ResourceExhaustedError
				if errors.As(checkErr, &resourceErr) {
					resourceResult := resourceErr.Result
					if resourceResult.ProjectRoot == "" && len(resourceResult.RootChecks) == 0 {
						resourceResult = checkResult
					}
					report := ExecuteBeadReport{
						WorkerID:          runtime.WorkerID,
						Harness:           harness,
						Model:             model,
						Status:            ExecuteBeadStatusResourceExhausted,
						Detail:            ResourceExhaustedStopMessage,
						Error:             resourceErr.Error(),
						SessionID:         runtime.SessionID,
						ResourceExhausted: &resourceResult,
						Disrupted:         true,
						DisruptionReason:  ReadinessSystemReasonResourceExhausted,
						OutcomeReason:     ReadinessSystemReasonResourceExhausted,
					}
					result.Failures++
					result.LastFailureStatus = report.Status
					result.Results = append(result.Results, report)
					setExit("ResourceExhausted", "resource_exhausted")
					emitResourceExhausted(emit, nil, "", report, assignee, now().UTC())
					if runtime.Log != nil {
						_, _ = fmt.Fprintln(runtime.Log, ResourceExhaustedStopMessage)
					}
					return result, nil
				}
				setExit("Preflight", "preflight_failed")
				return result, checkErr
			}
		}
		if runtime.BinaryRefreshCheck != nil {
			refreshed, refreshErr := runtime.BinaryRefreshCheck(ctx)
			if refreshErr != nil {
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "binary refresh check failed: %v; continuing\n", refreshErr)
				}
				emit("loop.binary_refresh_check_error", map[string]any{"error": refreshErr.Error()})
			} else if refreshed {
				setExit("BinaryRefresh", "binary_refresh")
				emit("loop.binary_refresh", map[string]any{"reason": "installed_binary_changed"})
				return result, nil
			}
		}

		candidate, skips, ok, err := w.nextCandidate(ctx, result.Results[resultsResetIdx:], []work.Guard{complexityGuard, preclaimGuard}, runtime.LabelFilter, runtime.TargetBeadID)
		if err != nil {
			setExit("FatalConfig", "fatal_config")
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
			if detail, systemic := systemicPreClaimSkipDetail(skips); systemic {
				if runtime.Log != nil && !wasIdle {
					_, _ = fmt.Fprintf(runtime.Log, "pre-claim hook blocked queue: %s; sleeping %s\n", detail, idleInterval)
				}
				emit("loop.idle", map[string]any{
					"reason":        "preclaim_systemic",
					"detail":        detail,
					"idle_interval": idleInterval.String(),
				})
				emitProgress(runtime.ProgressCh, ProgressEvent{
					EventID:   "evt-" + randomProgressID(),
					WorkerID:  runtime.WorkerID,
					ProjectID: runtime.ProjectRoot,
					Phase:     "loop.idle",
					Heartbeat: true,
					TS:        now().UTC(),
					Message:   "preclaim_systemic",
				})
				wasIdle = true
				if loopMode != executeloop.ModeWatch {
					setExit("Preflight", "preclaim_systemic")
					return result, nil
				}
				if err := sleepOrWake(ctx, idleInterval, runtime.WakeCh); err != nil {
					if exitReason == "" {
						applyStop(work.StopInput{ContextErr: err})
					}
					return result, err
				}
				continue
			}
			if hasGuardSkips(skips) {
				continue
			}

			// Before reporting NoReadyWork, check whether every cooldown in the
			// queue is from an infra-fault class. If so, transition the WORKER to
			// paused-infra and wait for the earliest cooldown to expire instead of
			// returning idle/drained — this keeps beads immediately reclaimable
			// and avoids the misleading "drained" signal when the real issue is infra.
			if diag, ok := w.Store.(readyDiagnoser); ok {
				if breakdown, bErr := diag.ReadyExecutionBreakdown(); bErr == nil && len(breakdown.ExecutionReady) == 0 && len(breakdown.RetryCooldown) > 0 {
					if resumeAt, allInfra := infraCooldownResumeAt(w.Store, breakdown.RetryCooldown); allInfra {
						emit("loop.paused-infra", map[string]any{
							"reason":    "all_infra_cooldowns",
							"resume_at": resumeAt.Format(time.RFC3339),
						})
						emitProgress(runtime.ProgressCh, ProgressEvent{
							EventID:   "evt-" + randomProgressID(),
							WorkerID:  runtime.WorkerID,
							ProjectID: runtime.ProjectRoot,
							Phase:     "loop.paused-infra",
							Heartbeat: true,
							TS:        now().UTC(),
							Message:   "all_infra_cooldowns",
						})
						sleepDur := time.Until(resumeAt)
						if sleepDur <= 0 {
							sleepDur = PausedInfraInterval
						}
						if err := sleepOrWake(ctx, sleepDur, runtime.WakeCh); err != nil {
							if exitReason == "" {
								applyStop(work.StopInput{ContextErr: err})
							}
							return result, err
						}
						continue
					}
				}
			}

			result.NoReadyWork = true
			if diag, ok := w.Store.(readyDiagnoser); ok {
				if breakdown, bErr := diag.ReadyExecutionBreakdown(); bErr == nil {
					result.NoReadyWorkDetail = noReadyWorkBreakdownFromLifecycle(breakdown)
					snapshot := queueSnapshotFromLifecycle(breakdown)
					result.QueueSnapshot = &snapshot
				}
			}
			if applyStop(work.StopInput{
				NoReadyWork: true,
				Once:        loopMode == executeloop.ModeOnce,
				Mode:        loopMode,
			}) {
				return result, nil
			}
			if runtime.Log != nil {
				signature := workLogQueueSnapshotSignature(result.QueueSnapshot)
				includeBlockers := signature != "" && signature != lastIdleQueueSignature
				_, _ = fmt.Fprint(runtime.Log, workLog.FormatWatchIdle(idleInterval, result.QueueSnapshot, includeBlockers))
				lastIdleQueueSignature = signature
			}
			wasIdle = true
			// Watch mode treats an empty queue as idle, not terminal. --once
			// and drain exits are classified above through work.StopCondition.
			// Emit a transient "no_ready_work"
			// event so server-managed workers can surface this as an idle substate
			// (ddx-dc157075 AC #5) instead of treating it as terminal.
			emit("loop.idle", map[string]any{
				"reason":        "no_ready_work",
				"idle_interval": idleInterval.String(),
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
			resultsResetIdx = len(result.Results)
			if err := sleepOrWake(ctx, idleInterval, runtime.WakeCh); err != nil {
				if exitReason == "" {
					applyStop(work.StopInput{ContextErr: err})
				}
				return result, err
			}
			continue
		}

		// Found a candidate: clear any "idle" substate set on the previous
		// no-candidate iteration (ddx-dc157075 AC #5).
		if wasIdle && runtime.Log != nil {
			_, _ = fmt.Fprint(runtime.Log, workLog.FormatNextReadyTransition(candidate.ID, candidate.Title))
		}
		wasIdle = false
		emitProgress(runtime.ProgressCh, ProgressEvent{
			EventID:   "evt-" + randomProgressID(),
			WorkerID:  runtime.WorkerID,
			ProjectID: runtime.ProjectRoot,
			BeadID:    candidate.ID,
			Phase:     "loop.active",
			Heartbeat: true,
			TS:        now().UTC(),
		})

		if allowed, reason := complexityGuard.Allow(ctx, candidate.ID); !allowed {
			if runtime.Log != nil && reason != "" {
				_, _ = fmt.Fprintf(runtime.Log, "complexity gate: %s (skipping %s)\n", reason, candidate.ID)
			}
			continue
		}
		// Claim atomically before model-backed intake and lint to prevent
		// concurrent workers from issuing duplicate model calls or appending
		// duplicate lint events for the same bead. Terminal non-actionable
		// outcomes unclaim and park the bead rather than proceeding to execution.
		if err := w.Store.Claim(candidate.ID, assignee); err != nil {
			// Another worker won the race for this bead. Emit a structured
			// claim_race event so concurrent-worker losses are observable
			// (ddx-9d55601f AC #4). The bead remains in `attempted` for this
			// run; on the next iteration it will be filtered out of
			// ReadyExecution naturally because Claim() flipped it to
			// in_progress, so the loser keeps moving down priority order.
			emit("picker.claim_race", map[string]any{
				"bead_id":    candidate.ID,
				"priority":   candidate.Priority,
				"queue_rank": queueRankValue(candidate.Extra["queue-rank"]),
				"reason":     err.Error(),
			})
			continue
		}

		emit("bead.claimed", map[string]any{
			"bead_id":  candidate.ID,
			"title":    candidate.Title,
			"assignee": assignee,
		})

		if runtime.Log != nil {
			workLog = workLog.WithCurrentBeadID(candidate.ID)
			_, _ = fmt.Fprint(runtime.Log, workLog.FormatHeader(candidate.ID, candidate.Title))
		}

		// Dangling-success recovery (ddx-2b2d114e): when we just reclaimed a
		// stale in_progress bead, check whether a prior task_succeeded result
		// exists with a result_rev already merged into HEAD. If so, close the
		// bead idempotently and skip re-execution — the work is already done.
		if candidate.Status == bead.StatusInProgress {
			recovered, recErr := recoverDanglingSuccess(
				w.Store, runtime.ProjectRoot, candidate.ID,
				true, assignee, now, emit,
			)
			if recErr != nil {
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "dangling-success recovery error (%s): %v\n", candidate.ID, recErr)
				}
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("recoverDanglingSuccess", assignee, result, recErr)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				continue
			}
			if recovered {
				result.Attempts++
				result.Successes++
				result.LastSuccessAt = now().UTC()
				result.Results = append(result.Results, ExecuteBeadReport{
					BeadID: candidate.ID,
					Status: ExecuteBeadStatusSuccess,
					Detail: "dangling-success recovery: bead closed idempotently (result_rev already merged)",
				})
				if applyStop(work.StopInput{Once: loopMode == executeloop.ModeOnce}) {
					return result, nil
				}
				continue
			}
		}

		skipPreClaimIntake := false

		// Queue-level decomposition depth cap: when the bead has already been split
		// to the configured limit, block it for operator review without invoking
		// the classifier or splitter (docs/triage/decomposition.md §Recursion depth cap).
		if runtime.PreClaimIntakeHook != nil && rcfg.MaxDecompositionDepth() > 0 {
			maxDepth := rcfg.MaxDecompositionDepth()
			depth := storeBeadDepth(ctx, w.Store, &candidate)
			if depth >= maxDepth {
				body, _ := json.Marshal(map[string]any{"depth": depth, "max": maxDepth})
				_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
					Kind:      "triage-overflow",
					Summary:   "depth cap exceeded",
					Body:      string(body),
					Actor:     assignee,
					Source:    "ddx work",
					CreatedAt: now().UTC(),
				})
				if lerr := addBeadLabel(ctx, w.Store, candidate.ID, "needs-human-decomposition"); lerr != nil && runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "triage-overflow label error: %v\n", lerr)
				}
				overflowDetail := fmt.Sprintf("bead depth %d reached max_decomposition_depth %d; operator must split", depth, maxDepth)
				if strictIntakeBlocking() {
					if parked, berr := parkBeadPostIntakeRejection(w.Store, &candidate, assignee, PreClaimIntakeOperatorRequired, "operator_required", overflowDetail, now().UTC()); berr != nil && runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "readiness park error: %v\n", berr)
					} else if parked {
						_ = w.Store.Unclaim(candidate.ID)
						if stopAfterNonAttemptSkip() {
							applyStop(work.StopInput{Once: true})
							return result, nil
						}
						continue
					}
				} else {
					appendPreClaimIntakeWarning(w.Store, candidate.ID, assignee, "decomposition_depth_cap", overflowDetail, now().UTC())
					emit("pre_claim_intake.warn", readinessDecisionBody(
						"pre_claim_intake.decomposition_depth_cap",
						"too_large",
						"pre_claim_intake",
						"best-effort",
						"attempt",
						"continue with implementation; operator attention is reserved for explicit targeted execution",
						map[string]any{
							"bead_id": candidate.ID,
							"outcome": string(PreClaimIntakeTooLargeDecomposed),
							"detail":  overflowDetail,
						},
					))
					skipPreClaimIntake = true
				}
			}
		}

		// Pre-dispatch intake runs after claiming so that only the owning worker
		// performs model-backed readiness evaluation. Concurrent workers that lose
		// the claim race skip intake entirely (picker.claim_race above). Terminal
		// non-actionable outcomes unclaim and park the bead so it does not
		// re-appear in ReadyExecution until an operator reviews it.
		if runtime.PreClaimIntakeHook != nil && !skipPreClaimIntake {
			if runtime.Log != nil {
				_, _ = fmt.Fprint(runtime.Log, workLog.FormatLifecycleLine(WorkLogLifecycleLine{
					Phase:    "readiness",
					BeadID:   candidate.ID,
					Message:  "check: starting",
					Harness:  harness,
					Provider: provider,
					Model:    model,
				}))
			}
			emit("pre_claim_intake.start", map[string]any{
				"bead_id": candidate.ID,
			})
			emitProgress(runtime.ProgressCh, ProgressEvent{
				EventID:   "evt-" + randomProgressID(),
				WorkerID:  runtime.WorkerID,
				ProjectID: runtime.ProjectRoot,
				BeadID:    candidate.ID,
				Phase:     "pre_claim_intake",
				Heartbeat: true,
				TS:        now().UTC(),
				Message:   "readiness check",
			})
			intakeResult, intakeErr := runPreClaimIntakeHookWithTimeout(ctx, runtime.PreClaimIntakeHook, candidate.ID, preClaimTimeout)
			intakeOutcome := intakeResult.normalizedOutcome()
			if intakeResult.SystemReason == ReadinessSystemReasonTimeout {
				timeoutDetail := strings.TrimSpace(intakeResult.Detail)
				if timeoutDetail == "" {
					timeoutDetail = fmt.Sprintf("readiness check timed out after %s", preClaimTimeout)
				}
				if runtime.Log != nil {
					_, _ = fmt.Fprint(runtime.Log, workLog.FormatLifecycleLine(WorkLogLifecycleLine{
						Phase:    "readiness",
						BeadID:   candidate.ID,
						Message:  fmt.Sprintf("check timed out after %s (continuing)", preClaimTimeout),
						Harness:  harness,
						Provider: provider,
						Model:    model,
					}))
				}
				eventBody := readinessDecisionBody(
					"pre_claim_intake.timeout",
					"timeout",
					"pre_claim_intake",
					func() string {
						if strictIntakeBlocking() {
							return "block"
						}
						return "warn-only"
					}(),
					func() string {
						if strictIntakeBlocking() {
							return "park"
						}
						return "continue"
					}(),
					func() string {
						if strictIntakeBlocking() {
							return "review the readiness timeout and retry the bead later"
						}
						return "continue with implementation; review should create follow-up work for any readiness gaps"
					}(),
					map[string]any{
						"bead_id":       candidate.ID,
						"command":       "ddx work readiness check",
						"context":       "pre-claim intake",
						"detail":        timeoutDetail,
						"harness":       harness,
						"model":         model,
						"mode":          string(loopMode),
						"outcome":       string(PreClaimIntakeError),
						"provider":      provider,
						"system_reason": ReadinessSystemReasonTimeout,
						"targeted":      strictIntakeBlocking(),
						"timeout":       preClaimTimeout.String(),
						"worker_id":     runtime.WorkerID,
					},
				)
				if strictIntakeBlocking() {
					emit("pre_claim_intake.blocked", eventBody)
					if parked, berr := parkBeadPostIntakeRejection(w.Store, &candidate, assignee, PreClaimIntakeError, ReadinessReasonSystemUnready, timeoutDetail, now().UTC()); berr != nil && runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "readiness park error: %v\n", berr)
					} else if parked {
						if runtime.Log != nil {
							_, _ = fmt.Fprintf(runtime.Log, "readiness timeout parked bead %s for operator review\n", candidate.ID)
						}
					}
					_ = w.Store.Unclaim(candidate.ID)
					if stopAfterNonAttemptSkip() {
						applyStop(work.StopInput{Once: true})
						return result, nil
					}
					continue
				} else {
					emit("pre_claim_intake.warn", eventBody)
					appendPreClaimIntakeWarning(w.Store, candidate.ID, assignee, "timeout", timeoutDetail, now().UTC())
					intakeOutcome = PreClaimIntakeActionableAtomic
				}
			}
			switch {
			case intakeErr != nil:
				if errors.Is(intakeErr, context.Canceled) || errors.Is(intakeErr, context.DeadlineExceeded) {
					if exitReason == "" {
						applyStop(work.StopInput{ContextErr: intakeErr})
					}
					return result, intakeErr
				}
				warning := trimDiagnosticPrefix(intakeErr.Error(), "pre-claim intake")
				classified := ClassifyReadiness(ReadinessClassificationSystemUnready, nil, warning)
				if runtime.Log != nil {
					_, _ = fmt.Fprint(runtime.Log, workLog.FormatLifecycleLine(WorkLogLifecycleLine{
						Phase:    "readiness",
						BeadID:   candidate.ID,
						Message:  fmt.Sprintf("check unavailable: %s (continuing)", warning),
						Harness:  harness,
						Provider: provider,
						Model:    model,
					}))
				}
				emit("pre_claim_intake.warn", readinessDecisionBody(
					"pre_claim_intake.system_unready",
					classified.Reason,
					"pre_claim_intake",
					"warn-only",
					"warn",
					"check the readiness route or harness configuration and retry",
					map[string]any{
						"bead_id":       candidate.ID,
						"outcome":       string(PreClaimIntakeError),
						"system_reason": classified.SystemReason,
						"detail":        warning,
					},
				))
			case intakeOutcome == PreClaimIntakeActionableAtomic:
				// pass-through
			case intakeOutcome == PreClaimIntakeActionableButRewritten:
				if err := applyPreClaimIntakeRewrite(w.Store, candidate.ID, assignee, intakeResult, now().UTC()); err != nil {
					warning := trimDiagnosticPrefix(err.Error(), "pre-claim intake rewrite")
					if runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "bead readiness: rewrite error: %s (continuing with original %s)\n", warning, candidate.ID)
					}
					emit("pre_claim_intake.warn", readinessDecisionBody(
						"pre_claim_intake.rewrite_rejected",
						"rewrite_rejected",
						"pre_claim_intake",
						"warn-only",
						"warn",
						"revise the rewrite so it preserves every explicit commitment",
						map[string]any{
							"bead_id": candidate.ID,
							"outcome": string(PreClaimIntakeActionableButRewritten),
							"detail":  warning,
						},
					))
					appendPreClaimIntakeWarning(w.Store, candidate.ID, assignee, "rewrite_rejected", warning, now().UTC())
				}
			case intakeOutcome == PreClaimIntakeError:
				warning := trimDiagnosticPrefix(intakeResult.Detail, "pre-claim intake")
				if warning == "" {
					warning = "readiness check returned intake_error"
				}
				reason := intakeResult.Reason
				systemReason := intakeResult.SystemReason
				if reason == "" && systemReason == "" {
					classified := ClassifyReadiness(ReadinessClassificationSystemUnready, nil, warning)
					reason = classified.Reason
					systemReason = classified.SystemReason
				}
				if runtime.Log != nil {
					_, _ = fmt.Fprint(runtime.Log, workLog.FormatLifecycleLine(WorkLogLifecycleLine{
						Phase:    "readiness",
						BeadID:   candidate.ID,
						Message:  fmt.Sprintf("check unavailable: %s (continuing)", warning),
						Harness:  harness,
						Provider: provider,
						Model:    model,
					}))
				}
				emit("pre_claim_intake.warn", readinessDecisionBody(
					"pre_claim_intake.system_unready",
					reason,
					"pre_claim_intake",
					"warn-only",
					"warn",
					"check the readiness route or harness configuration and retry",
					map[string]any{
						"bead_id":       candidate.ID,
						"outcome":       string(PreClaimIntakeError),
						"system_reason": systemReason,
						"detail":        warning,
					},
				))
			case intakeOutcome == PreClaimIntakeTooLargeDecomposed:
				// too_large_decomposed should move work forward by splitting before
				// claim. Some intake classifiers only identify the need to split;
				// when no concrete children are attached, ask the orchestrator
				// decomposer for executable child specs and then apply them.
				decomp := intakeResult.Decomposition
				if decomp == nil && runtime.PostAttemptDecompositionHook != nil {
					if runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "bead readiness requested decomposition; generating split (%s)\n", candidate.ID)
					}
					var hookErr error
					decomp, hookErr = runtime.PostAttemptDecompositionHook(ctx, candidate.ID)
					if hookErr != nil {
						if err := ctx.Err(); err != nil {
							applyStop(work.StopInput{ContextErr: err})
							return result, err
						}
						warning := fmt.Sprintf("decomposition hook unavailable: %s", hookErr.Error())
						if runtime.Log != nil {
							_, _ = fmt.Fprintf(runtime.Log, "bead decomposition unavailable: %s (%s); continuing with attempt\n", hookErr.Error(), candidate.ID)
						}
						appendPreClaimIntakeWarning(w.Store, candidate.ID, assignee, "decomposition_hook_unavailable", warning, now().UTC())
						emit("pre_claim_intake.warn", map[string]any{
							"bead_id": candidate.ID,
							"outcome": string(PreClaimIntakeTooLargeDecomposed),
							"reason":  "decomposition_hook_unavailable",
							"detail":  warning,
						})
						break
					}
				}
				if decomp == nil {
					warning := "decomposition hook returned no split"
					if runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "bead decomposition unavailable: %s (%s); continuing with attempt\n", warning, candidate.ID)
					}
					appendPreClaimIntakeWarning(w.Store, candidate.ID, assignee, "decomposition_hook_empty", warning, now().UTC())
					emit("pre_claim_intake.warn", map[string]any{
						"bead_id": candidate.ID,
						"outcome": string(PreClaimIntakeTooLargeDecomposed),
						"reason":  "decomposition_hook_empty",
						"detail":  warning,
					})
					break
				}
				// too_large_decomposed with concrete child specs: validate the AC map,
				// check the queue-level depth cap, then create children and wire deps.
				lossyOrEmpty := isDecompositionLossy(decomp.ACMap) || (len(decomp.ACMap) == 0 && strings.TrimSpace(candidate.Acceptance) != "")
				depthAtCap := storeBeadDepth(ctx, w.Store, &candidate) >= rcfg.MaxDecompositionDepth()
				if lossyOrEmpty || depthAtCap {
					// Cannot produce a lossless split: block for operator.
					blockedDetail := "decomposition AC map is incomplete or depth cap reached; operator review required"
					if depthAtCap {
						blockedDetail = "depth cap reached during decomposition; operator must split"
					}
					if runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "bead decomposition blocked: %s (%s)\n", blockedDetail, candidate.ID)
					}
					emit("pre_claim_intake.blocked", map[string]any{
						"bead_id": candidate.ID,
						"outcome": string(PreClaimIntakeOperatorRequired),
						"reason":  blockedDetail,
						"detail":  blockedDetail,
					})
					if strictIntakeBlocking() {
						if parked, berr := parkBeadPostIntakeRejection(w.Store, &candidate, assignee, PreClaimIntakeOperatorRequired, "operator_required", blockedDetail, now().UTC()); berr != nil && runtime.Log != nil {
							_, _ = fmt.Fprintf(runtime.Log, "readiness park error: %v\n", berr)
						} else if parked {
							_ = w.Store.Unclaim(candidate.ID)
							if stopAfterNonAttemptSkip() {
								applyStop(work.StopInput{Once: true})
								return result, nil
							}
							continue
						}
					} else {
						appendPreClaimIntakeWarning(w.Store, candidate.ID, assignee, "decomposition_blocked_best_effort", blockedDetail, now().UTC())
					}
					break
				}
				childIDs, decompErr := applyPreClaimDecomposition(ctx, w.Store, &candidate, decomp, assignee, now().UTC())
				if decompErr != nil {
					if runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "bead decomposition error: %v (%s)\n", decompErr, candidate.ID)
					}
					emit("pre_claim_intake.blocked", readinessDecisionBody(
						"pre_claim_intake."+strings.TrimSpace(string(PreClaimIntakeOperatorRequired)),
						"operator_required",
						"pre_claim_intake",
						"block",
						"park",
						"review intake result and accept, rewrite, split, block, or cancel",
						map[string]any{
							"bead_id": candidate.ID,
							"outcome": string(PreClaimIntakeOperatorRequired),
							"detail":  decompErr.Error(),
						},
					))
					if strictIntakeBlocking() {
						if parked, berr := parkBeadPostIntakeRejection(w.Store, &candidate, assignee, PreClaimIntakeOperatorRequired, "operator_required", decompErr.Error(), now().UTC()); berr != nil && runtime.Log != nil {
							_, _ = fmt.Fprintf(runtime.Log, "readiness park error: %v\n", berr)
						} else if parked {
							_ = w.Store.Unclaim(candidate.ID)
							if stopAfterNonAttemptSkip() {
								applyStop(work.StopInput{Once: true})
								return result, nil
							}
							continue
						}
					} else {
						appendPreClaimIntakeWarning(w.Store, candidate.ID, assignee, "decomposition_error_best_effort", decompErr.Error(), now().UTC())
					}
					break
				}
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "bead decomposed into %s (releasing %s)\n", strings.Join(childIDs, ", "), candidate.ID)
				}
				emit("pre_claim_intake.decomposed", map[string]any{
					"bead_id":   candidate.ID,
					"child_ids": childIDs,
				})
				// Parent stays open (not proposed) — it is now dependency-blocked on
				// children and will re-enter the queue only after they close.
				_ = w.Store.Unclaim(candidate.ID)
				if stopAfterNonAttemptSkip() {
					applyStop(work.StopInput{Once: true})
					return result, nil
				}
				continue
			default:
				// Terminal non-actionable intake outcomes are warnings during
				// broad queue drain: the worker should prefer making an attempt
				// and letting review/follow-up work handle gaps. Explicit
				// targeted execution keeps the stricter parking behavior.
				warning := trimDiagnosticPrefix(intakeResult.Detail, "pre-claim intake")
				if warning == "" {
					warning = string(intakeOutcome)
				}
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "bead readiness blocked: %s (%s)\n", warning, candidate.ID)
				}
				emit("pre_claim_intake.blocked", readinessDecisionBody(
					"pre_claim_intake."+strings.TrimSpace(string(intakeOutcome)),
					intakeResult.Reason,
					"pre_claim_intake",
					"best-effort",
					"attempt",
					"continue with implementation; review should create follow-up work for remaining gaps",
					map[string]any{
						"bead_id": candidate.ID,
						"outcome": string(intakeOutcome),
						"detail":  warning,
					},
				))
				if strictIntakeBlocking() {
					if parked, berr := parkBeadPostIntakeRejection(w.Store, &candidate, assignee, intakeOutcome, intakeResult.Reason, intakeResult.Detail, now().UTC()); berr != nil && runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "readiness park error: %v\n", berr)
					} else if parked {
						_ = w.Store.Unclaim(candidate.ID)
						if stopAfterNonAttemptSkip() {
							applyStop(work.StopInput{Once: true})
							return result, nil
						}
						continue
					}
				} else {
					appendPreClaimIntakeWarning(w.Store, candidate.ID, assignee, "readiness_best_effort", warning, now().UTC())
				}
			}
		}

		// Model-backed lint is only a dispatch gate when a block threshold is
		// configured. The default warn-only path relies on readiness diagnostics
		// and must not stall or add advisory event noise before real work.
		lintThreshold := rcfg.BeadQualityLintBlockThresholdScore()
		if runtime.PreDispatchLintHook != nil && lintThreshold > 0 {
			lintResult, lintErr := runPreDispatchLintHookWithTimeout(ctx, runtime.PreDispatchLintHook, candidate.ID, preClaimTimeout)
			appendPreDispatchLintEvent(w.Store, candidate.ID, lintResult, lintErr, lintThreshold, assignee, now().UTC())

			if lintErr != nil {
				classified := ClassifyReadiness(ReadinessClassificationSystemUnready, nil, lintErr.Error())
				if runtime.Log != nil {
					var lhe *LintHookError
					if errors.As(lintErr, &lhe) && lhe.Kind == LintHookErrorKindMissingHarness {
						_, _ = fmt.Fprint(runtime.Log, workLog.FormatLifecycleLine(WorkLogLifecycleLine{
							Phase:    "readiness",
							BeadID:   candidate.ID,
							Message:  "check unavailable: no harness configured; continuing",
							Harness:  harness,
							Provider: provider,
							Model:    model,
						}))
					} else {
						_, _ = fmt.Fprint(runtime.Log, workLog.FormatLifecycleLine(WorkLogLifecycleLine{
							Phase:    "readiness",
							BeadID:   candidate.ID,
							Message:  fmt.Sprintf("check unavailable: %v (continuing)", lintErr),
							Harness:  harness,
							Provider: provider,
							Model:    model,
						}))
					}
				}
				emit("pre_dispatch_lint.warn", map[string]any{
					"bead_id":       candidate.ID,
					"warning":       lintErr.Error(),
					"reason":        classified.Reason,
					"system_reason": classified.SystemReason,
				})
			} else if lintThreshold > 0 && lintResult.Score < lintThreshold {
				blockMsg := fmt.Sprintf(
					"bead-quality check blocked dispatch for %s: score=%d below threshold=%d; see bead-lifecycle MODE: lint guidance in .agents/skills/ddx/bead-lifecycle/SKILL.md",
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
				if strictIntakeBlocking() {
					_ = w.Store.Unclaim(candidate.ID)
					if stopAfterNonAttemptSkip() {
						applyStop(work.StopInput{Once: true})
						return result, nil
					}
					continue
				}
				appendPreClaimIntakeWarning(w.Store, candidate.ID, assignee, "lint_blocked_best_effort", blockMsg, now().UTC())
			}
		}

		// Generate a provisional attempt_id for progress events.
		// The real attempt_id is assigned inside ExecuteBead; we use this
		// for queueing/running events and replace with the real one once known.
		provAttemptID := time.Now().UTC().Format("20060102T150405") + "-" + randomProgressID()
		runStart := now()
		phaseSeq := 0
		phaseEmitter := newLoopPhaseEmitter(runtime, harness, model, profile, runStart, now, &phaseSeq, emit)

		_ = work.EmitPhase(ctx, phaseEmitter, candidate.ID, work.PhaseQueueing, work.Outcome{
			AttemptID: provAttemptID,
		})

		_ = work.EmitPhase(ctx, phaseEmitter, candidate.ID, work.PhaseRunning, work.Outcome{
			AttemptID: provAttemptID,
		})

		// tryExecutor preserves the legacy w.Executor.Execute(ctx, candidate.ID)
		// invocation while letting try.Attempt own conflict recovery.
		attemptCtx := ctx
		if liveness != nil {
			liveness.SetAttempt(candidate.ID, provAttemptID, string(work.PhaseRunning), profile, harness, model, profile, 0)
			// Tick once now so the sidecar shows the new attempt before the
			// first heartbeat fires; long attempts otherwise wait one
			// heartbeat interval before the sidecar reflects the current bead.
			liveness.OnTick(now())
		}
		attemptStarted = true
		attemptOut, err := work.WithHeartbeat(attemptCtx, candidate.ID, heartbeatInterval, w.Store, liveness, func() (agenttry.Outcome, error) {
			return agenttry.Attempt(attemptCtx, w.Store, candidate.ID, agenttry.AttemptOpts{
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
		})
		if liveness != nil {
			liveness.ClearAttempt()
			liveness.OnTick(now())
		}
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
		classifyLoopReportFailure(&report)
		if checkpointDirty, ok := preExecuteCheckpointDirtyStop(report, err, runtime.ProjectRoot, candidate.ID); ok {
			if unclaimErr := w.Store.Unclaim(candidate.ID); unclaimErr != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("Unclaim", assignee, result, unclaimErr)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				return result, unclaimErr
			}
			detail := strings.TrimSpace(firstNonEmpty(report.Detail, report.Error))
			if detail == "" && err != nil {
				detail = strings.TrimSpace(err.Error())
			}
			result.OperatorAttention = checkpointDirty
			setExit("OperatorAttention", "operator_attention")
			if runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log,
					"operator attention: project worktree %s has uncommitted implementation changes; released %s. %s\n",
					checkpointDirty.ProjectRoot,
					candidate.ID,
					checkpointDirty.Message,
				)
			}
			emit("loop.operator_attention", map[string]any{
				"reason":       checkpointDirty.Reason,
				"bead_id":      candidate.ID,
				"project_root": checkpointDirty.ProjectRoot,
				"dirty_paths":  checkpointDirty.DirtyPaths,
				"message":      checkpointDirty.Message,
				"detail":       detail,
			})
			return result, nil
		}
		if IsResourceExhaustedStatus(report.Status) {
			result.Attempts++
			setExit("ResourceExhausted", "resource_exhausted")
			if err := w.Store.Unclaim(candidate.ID); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("Unclaim", assignee, result, err)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				return result, err
			}
			emitResourceExhausted(emit, w.Store, candidate.ID, report, assignee, now().UTC())
			result.Results = append(result.Results, report)
			callAttemptFinalized(runtime.OnAttemptFinalized, report)
			result.Failures++
			result.LastFailureStatus = report.Status
			if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC())); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("AppendEvent", assignee, result, err)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				return result, err
			}
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
				Phase:     "failed",
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
				_, _ = fmt.Fprintln(runtime.Log, ResourceExhaustedStopMessage)
			}
			return result, nil
		}
		if isRoutingInfrastructureReport(report) {
			result.Attempts++
			report.OutcomeReason = FailureModeNoViableProvider
			report.Disrupted = true
			report.DisruptionReason = "routing"
			if err := w.Store.Unclaim(candidate.ID); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("Unclaim", assignee, result, err)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				return result, err
			}
			emitDisruptionDetected(emit, w.Store, candidate.ID,
				report.DisruptionReason, report.Detail, report.Harness, report.Model, assignee, now().UTC())
			appendExecutionRoutingIntentEvidence(w.Store, candidate, report, now().UTC())
			result.Results = append(result.Results, report)
			callAttemptFinalized(runtime.OnAttemptFinalized, report)
			result.Failures++
			result.LastFailureStatus = report.Status
			if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC())); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("AppendEvent", assignee, result, err)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				return result, err
			}
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
				Phase:     "failed",
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
				_, _ = fmt.Fprintln(runtime.Log, formatLoopResultLine(candidate.ID, report))
			}
			setExit("RoutingUnavailable", "routing_unavailable")
			return result, nil
		}
		// Per-bead cost budget exhausted: unclaim the bead (immediately
		// re-claimable), emit the TD-031 §5 per-bead-budget-exhausted event,
		// and increment the consecutive_ladder_exhaustions counter so the
		// auto-recovery hook (sister bead ddx-63155d5c) can fire when the
		// threshold is exceeded. No cooldown is set per ADR-024 Per-Bead Budget.
		if isPerBeadBudgetExhaustedReport(report) {
			result.Attempts++
			if err := w.Store.Unclaim(candidate.ID); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("Unclaim", assignee, result, err)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				return result, err
			}
			_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
				Kind:      "per-bead-budget-exhausted",
				Summary:   "per-bead cost budget exhausted; bead returned to open without cooldown",
				Body:      fmt.Sprintf("total_cost=%.4f\n%s", report.CostUSD, report.Detail),
				Actor:     assignee,
				Source:    "ddx work",
				CreatedAt: now().UTC(),
			})
			_ = incrementConsecutiveLadderExhaustions(ctx, w.Store, candidate.ID)
			if updated, getErr := w.Store.Get(ctx, candidate.ID); getErr == nil &&
				consecutiveLadderExhaustionsValue(updated.Extra[consecutiveLadderExhaustionsKey]) >= 2 {
				hasManualLabel := false
				for _, lbl := range candidate.Labels {
					if lbl == "recovery:manual" {
						hasManualLabel = true
						break
					}
				}
				if hasManualLabel {
					_ = w.Store.ParkToProposed(candidate.ID, bead.ParkLadderExhaustionManual, func(b *bead.Bead) {
						bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
							Reason: "recovery:manual label set",
							Since:  now().UTC().Format(time.RFC3339),
							Source: "ddx work",
						})
					})
				} else if runtime.PostLadderExhaustionHook != nil {
					failureClass := deriveRecoveryFailureClass(report)
					_, _ = runtime.PostLadderExhaustionHook(ctx, candidate.ID, failureClass)
				}
			}
			if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC())); err != nil {
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "outcome store error (AppendEvent %s): %v (continuing)\n", candidate.ID, err)
				}
			}
			result.Results = append(result.Results, report)
			callAttemptFinalized(runtime.OnAttemptFinalized, report)
			result.Failures++
			result.LastFailureStatus = report.Status
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
				_, _ = fmt.Fprintln(runtime.Log, formatLoopResultLine(candidate.ID, report))
			}
			continue
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
		if ctxErr := loopContextErr(ctx); ctxErr != nil && report.Status != ExecuteBeadStatusSuccess {
			if report.Detail == "" || report.Detail == ExecuteBeadStatusDetail(report.Status, "", "") {
				report.Detail = ctxErr.Error()
			}
			if report.Error == "" {
				report.Error = ctxErr.Error()
			}
			report.Disrupted = true
			if report.DisruptionReason == "" {
				report.DisruptionReason, _ = classifyDisruption(ctx, err)
			}
			result.Attempts++
			result.Results = append(result.Results, report)
			result.Failures++
			result.LastFailureStatus = report.Status
			if unclaimErr := w.Store.Unclaim(candidate.ID); unclaimErr != nil && runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "interrupted attempt cleanup error (Unclaim %s): %v\n", candidate.ID, unclaimErr)
			}
			if runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "interrupted attempt released %s without recording terminal outcome: %v\n", candidate.ID, ctxErr)
			}
			return result, ctxErr
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
		appendExecutionRoutingIntentEvidence(w.Store, candidate, report, now().UTC())

		if attemptOut.StoreErr != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError(attemptOut.StoreErrOp, assignee, result, attemptOut.StoreErr)
			})
		}

		if parking := attemptOut.Parking; parking != nil {
			if parking.Unclaim {
				if err := w.Store.Unclaim(candidate.ID); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("Unclaim", assignee, result, err)
					})
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
					Source:    "ddx work",
					CreatedAt: now().UTC(),
				})
			}
			if !parking.RetryAfter.IsZero() {
				if err := w.Store.SetExecutionCooldown(candidate.ID, parking.RetryAfter, report.Status, report.Detail, report.BaseRev); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("SetExecutionCooldown", assignee, result, err)
					})
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
				report.RetryAfter = parking.RetryAfter.Format(time.RFC3339)
			}
		}

		if attemptOut.Disposition == agenttry.OutcomeSuccess {
			finalizationCtx := context.Background()
			if err := clearExecuteLoopNoChangesMetadata(finalizationCtx, w.Store, candidate.ID); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("clearExecuteLoopNoChangesMetadata", assignee, result, err)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				continue
			}
			result.Successes++
			result.LastSuccessAt = now().UTC()
		} else if report.Status == ExecuteBeadStatusSuccess {
			finalizationCtx := context.Background()
			if err := clearExecuteLoopNoChangesMetadata(finalizationCtx, w.Store, candidate.ID); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("clearExecuteLoopNoChangesMetadata", assignee, result, err)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				continue
			}
			appendLoopRoutingEvidence(w.Store, candidate.ID, report, now().UTC(), readFailedRoutes(candidate.Extra))
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
			// Advisory prose-check evidence for docs-changing attempts. Runs
			// before CloseWithEvidence so findings land in the bead's history
			// while the attempt is still editable. Errors are advisory.
			if runtime.ProseEvidenceHook != nil {
				if proseErr := runtime.ProseEvidenceHook(ctx, candidate.ID, report); proseErr != nil && runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "prose evidence: %v (continuing)\n", proseErr)
				}
			}
			// Automated close eligibility is now owned by the pre-land
			// candidate-cycle reviewer. The old post-land/pre-close reviewer
			// remains available as a reusable/manual helper, but work
			// must not run it after a candidate has already landed.
			if err := w.Store.CloseWithEvidence(candidate.ID, report.SessionID, report.ResultRev); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("CloseWithEvidence", assignee, result, err)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				continue
			}

			result.Successes++
			result.LastSuccessAt = now().UTC()
		} else if attemptOut.Disposition == agenttry.OutcomePark {
			result.Failures++
			result.LastFailureStatus = report.Status
		} else if attemptOut.NoChanges != nil {
			noChanges := attemptOut.NoChanges
			if err := w.Store.Unclaim(candidate.ID); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("Unclaim", assignee, result, err)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				continue
			}

			// Post-attempt orchestrator decomposition: when the implementation
			// attempt signals orchestrator_action: decompose (because it hit the
			// implementation depth cap or the bead is too large for the worktree),
			// invoke the queue-level splitter. The orchestrator checks the
			// queue-level max_decomposition_depth, not the implementation cap.
			if report.NoChangesRationale != "" && runtime.PostAttemptDecompositionHook != nil {
				parsed := agenttry.ParseNoChangesRationale(report.NoChangesRationale)
				if parsed.OrchestratorAction == "decompose" {
					w.handlePostAttemptDecomposition(ctx, &candidate, runtime, assignee, rcfg, now().UTC())
					result.Failures++
					result.LastFailureStatus = report.Status
					continue
				}
			}

			if noChanges.EventKind != "" {
				_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
					Kind:      noChanges.EventKind,
					Summary:   noChanges.EventKind,
					Body:      noChanges.EventBody,
					Actor:     assignee,
					Source:    "ddx work",
					CreatedAt: now().UTC(),
				})
			}
			if noChanges.Label != "" {
				if err := addBeadLabel(ctx, w.Store, candidate.ID, noChanges.Label); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("addBeadLabel", assignee, result, err)
					})
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
			}
			switch noChanges.Action {
			case agenttry.NoChangesActionCloseAlreadySatisfied:
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
				finalizationCtx := context.Background()
				if err := clearExecuteLoopNoChangesMetadata(finalizationCtx, w.Store, candidate.ID); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("clearExecuteLoopNoChangesMetadata", assignee, result, err)
					})
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
				if err := w.Store.CloseWithEvidence(candidate.ID, report.SessionID, report.BaseRev); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("CloseWithEvidence", assignee, result, err)
					})
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
				result.Successes++
				result.LastSuccessAt = now().UTC()
			case agenttry.NoChangesActionKeepOpenSmartRetry:
				if err := applyNoChangesSmartRetry(w.Store, candidate.ID, assignee, noChanges, report.ActualPower, w.EscalationNextFloor); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("applyNoChangesSmartRetry", assignee, result, err)
					})
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
				if shouldSuppressNoProgress(report) {
					retryAfter := now().UTC().Add(SmartRetryCooldown)
					cooldownStatus := noChanges.EventKind
					if cooldownStatus == "" {
						cooldownStatus = report.Status
					}
					cooldownDetail := noChanges.Reason
					if cooldownDetail == "" {
						cooldownDetail = report.Detail
					}
					// This is a queue-fairness pause, not a stale-world
					// no-progress cooldown. Keep it wall-clock only so a
					// locally-ahead branch does not immediately invalidate it
					// against origin/main.
					if err := w.Store.SetExecutionCooldown(candidate.ID, retryAfter, cooldownStatus, cooldownDetail, ""); err != nil {
						_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
							return commitOutcomeError("SetExecutionCooldown", assignee, result, err)
						})
						if ctx.Err() != nil {
							return result, ctx.Err()
						}
						continue
					}
					report.RetryAfter = retryAfter.Format(time.RFC3339)
				}
				result.Failures++
				result.LastFailureStatus = report.Status
			case agenttry.NoChangesActionOperatorRequired:
				if err := applyNoChangesOperatorRequired(w.Store, candidate.ID, assignee, noChanges, now().UTC()); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("applyNoChangesOperatorRequired", assignee, result, err)
					})
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
				result.Failures++
				result.LastFailureStatus = report.Status
			case agenttry.NoChangesActionBlockedExternal:
				if err := applyNoChangesBlockedExternal(w.Store, candidate.ID, assignee, noChanges); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("applyNoChangesBlockedExternal", assignee, result, err)
					})
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
				result.Failures++
				result.LastFailureStatus = report.Status
			case agenttry.NoChangesActionRetryLaterCooldown:
				// Unresolved: suppress immediate retry so the queue can
				// move on to other beads.
				emitReviewerSkippedEmptyDiff(w.Store, candidate.ID, assignee, now().UTC())
				if noChanges.CooldownEligible && shouldSuppressNoProgress(report) {
					retryAfter := now().UTC().Add(CapLoopCooldown(noProgressCooldown))
					if err := w.Store.SetExecutionCooldown(candidate.ID, retryAfter, report.Status, report.Detail, report.BaseRev); err != nil {
						_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
							return commitOutcomeError("SetExecutionCooldown", assignee, result, err)
						})
						if ctx.Err() != nil {
							return result, ctx.Err()
						}
						continue
					}
					report.RetryAfter = retryAfter.Format(time.RFC3339)
				}
				result.Failures++
				result.LastFailureStatus = report.Status
			default:
				emitReviewerSkippedEmptyDiff(w.Store, candidate.ID, assignee, now().UTC())
				result.Failures++
				result.LastFailureStatus = report.Status
			}
		} else {
			if attemptOut.Parking == nil && attemptOut.Disposition != agenttry.OutcomePark {
				if err := w.Store.Unclaim(candidate.ID); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("Unclaim", assignee, result, err)
					})
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
			}
			if report.Status == ExecuteBeadStatusPreservedNeedsReview {
				if err := w.Store.AppendNotes(candidate.ID, preservedNeedsReviewNote(report)); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("AppendNotes", assignee, result, err)
					})
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
				result.Failures++
				result.LastFailureStatus = report.Status
			} else if report.Status == ExecuteBeadStatusRepairCycleExhausted {
				if err := applyRepairCycleExhaustedEscalation(w.Store, candidate.ID, assignee, report.ActualPower, now(), w.EscalationNextFloor); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("applyRepairCycleExhaustedEscalation", assignee, result, err)
					})
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
				}
				result.Failures++
				result.LastFailureStatus = report.Status
			} else {
				if isProviderConnectivityFailureReport(report) {
					report.OutcomeReason = FailureModeProviderConnectivity
					report.Disrupted = true
					report.DisruptionReason = "provider_connectivity"
					operatorPinned := isOperatorRoutingPinned(rcfg.Passthrough())
					if err := applyProviderConnectivityRouteExclusion(w.Store, candidate.ID, assignee, report, operatorPinned, w.EscalationNextFloor, now().UTC()); err != nil {
						if runtime.Log != nil {
							_, _ = fmt.Fprintf(runtime.Log, "route-exclusion update failed for %s: %v (continuing)\n", candidate.ID, err)
						}
					}
					emitRouteFailureEvent(w.Store, candidate.ID, assignee, report, now().UTC())
					// Route exclusion (TTL-based via RouteExclusionWindow) is the
					// correct mechanism — no per-bead cooldown. Bead is immediately
					// reclaimable; next attempt routes around the failed (provider,model).
				} else if isNoViableProviderReport(report) {
					report.OutcomeReason = FailureModeNoViableProvider
					report.Disrupted = true
					report.DisruptionReason = "no_viable_provider"
					// Transition the worker to paused-infra: leave every bead
					// immediately reclaimable, pause this worker for PausedInfraInterval,
					// then re-evaluate the full queue (P6 + ADR-024 §Infra Fallback).
					pausedInfraUntil = now().UTC().Add(PausedInfraInterval)
				} else {
					report = w.runPostAttemptTriage(ctx, candidate, report, runtime, assignee, now)
					if shouldSuppressNoProgress(report) {
						retryAfter := now().UTC().Add(CapLoopCooldown(noProgressCooldown))
						if err := w.Store.SetExecutionCooldown(candidate.ID, retryAfter, report.Status, report.Detail, report.BaseRev); err != nil {
							_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
								return commitOutcomeError("SetExecutionCooldown", assignee, result, err)
							})
							if ctx.Err() != nil {
								return result, ctx.Err()
							}
							continue
						}
						report.RetryAfter = retryAfter.Format(time.RFC3339)
					}
				}
				result.Failures++
				result.LastFailureStatus = report.Status
			}
		}

		result.Results = append(result.Results, report)
		callAttemptFinalized(runtime.OnAttemptFinalized, report)

		// Skip the late execute-bead append for already-satisfied beads —
		// the satisfied path appends its own terminal event before
		// CloseWithEvidence so the closure gate sees execution evidence.
		// Duplicating it here would yield two identical events.
		if report.Status != ExecuteBeadStatusAlreadySatisfied {
			if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC())); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("AppendEvent", assignee, result, err)
				})
				// Event recording failure is non-terminal: log it and continue.
				// result counters were already updated by the outcome block above;
				// do not double-count by re-running the outcome handler.
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "outcome store error (AppendEvent %s): %v (continuing)\n", candidate.ID, err)
				}
				if ctx.Err() == nil {
					_ = w.Store.SetExecutionCooldown(candidate.ID, now().UTC().Add(StoreErrorCooldown), "loop-error", "AppendEvent: "+err.Error(), "")
				}
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				continue
			}
		}

		// Use the real attempt_id from the report if available.
		finalAttemptID := report.AttemptID
		if finalAttemptID == "" {
			finalAttemptID = provAttemptID
		}
		_ = work.EmitPhase(ctx, phaseEmitter, candidate.ID, work.PhaseTerminal, phaseOutcomeFromAttemptOut(
			report,
			attemptOut,
			finalAttemptID,
			now().Sub(runStart).Milliseconds(),
		))

		if runtime.Log != nil {
			_, _ = fmt.Fprintln(runtime.Log, formatLoopResultLine(candidate.ID, report))
		}

		// Paused-infra: no_viable_provider transitions the WORKER (not the bead)
		// to a wait state. Emit the event always so observers can surface the
		// substate; only sleep in Watch mode (Once mode exits below).
		if !pausedInfraUntil.IsZero() {
			resumeAt := pausedInfraUntil
			pausedInfraUntil = time.Time{}
			emit("loop.paused-infra", map[string]any{
				"reason":    "no_viable_provider",
				"resume_at": resumeAt.Format(time.RFC3339),
			})
			emitProgress(runtime.ProgressCh, ProgressEvent{
				EventID:   "evt-" + randomProgressID(),
				WorkerID:  runtime.WorkerID,
				ProjectID: runtime.ProjectRoot,
				Phase:     "loop.paused-infra",
				Heartbeat: true,
				TS:        now().UTC(),
				Message:   "no_viable_provider",
			})
			if loopMode != executeloop.ModeOnce {
				if err := sleepOrWake(ctx, time.Until(resumeAt), runtime.WakeCh); err != nil {
					if exitReason == "" {
						applyStop(work.StopInput{ContextErr: err})
					}
					return result, err
				}
				continue
			}
		}

		if loopMode == executeloop.ModeOnce {
			applyStop(work.StopInput{Once: true})
			return result, nil
		}
	}
}

func noReadyWorkBreakdownFromLifecycle(b bead.ReadyExecutionBreakdown) NoReadyWorkBreakdown {
	return NoReadyWorkBreakdown{
		ExecutionReady:            b.ExecutionReady,
		DependencyWaiting:         b.DependencyWaiting,
		ProposedOperatorAttention: b.ProposedOperatorAttention,
		RetryCooldown:             b.RetryCooldown,
		ExternalBlocked:           b.ExternalBlocked,
		NotEligible:               b.NotEligible,
		Superseded:                b.Superseded,
		Epics:                     b.Epics,
		EpicClosureCandidates:     b.EpicClosureCandidates,
		NextRetryAfter:            b.NextRetryAfter,
	}
}

func queueSnapshotFromLifecycle(b bead.ReadyExecutionBreakdown) QueueSnapshot {
	humanBlockers := make([]HumanReviewBlockerSnapshot, 0, len(b.HumanReviewBlockers))
	for _, blocker := range b.HumanReviewBlockers {
		humanBlockers = append(humanBlockers, HumanReviewBlockerSnapshot{
			ID:                     blocker.ID,
			Title:                  blocker.Title,
			Priority:               blocker.Priority,
			DownstreamBlockedCount: blocker.DownstreamBlockedCount,
		})
	}
	return QueueSnapshot{
		ExecutionReadyCount:            len(b.ExecutionReady),
		BlockedCount:                   len(b.DependencyWaiting) + len(b.ExternalBlocked),
		DependencyWaitingCount:         len(b.DependencyWaiting),
		ExternalBlockedCount:           len(b.ExternalBlocked),
		ProposedOperatorAttentionCount: len(b.ProposedOperatorAttention),
		HumanReviewBlockerCount:        len(b.HumanReviewBlockers),
		HumanReviewBlockedTotal:        b.HumanReviewBlockedTotal,
		HumanReviewBlockers:            humanBlockers,
		RetryCooldownCount:             len(b.RetryCooldown),
		NextRetryAfter:                 b.NextRetryAfter,
		ExecutionIneligibleCount:       len(b.NotEligible),
		SupersededCount:                len(b.Superseded),
		SkippedEpicsCount:              len(b.Epics),
		EpicClosureCandidatesCount:     len(b.EpicClosureCandidates),
	}
}

// NewNoReadyWorkLoopResult builds the same terminal loop result used by the
// worker's empty-queue path without starting the long-lived worker machinery.
func NewNoReadyWorkLoopResult(mode executeloop.Mode, b bead.ReadyExecutionBreakdown) *ExecuteBeadLoopResult {
	result := &ExecuteBeadLoopResult{
		NoReadyWork:       true,
		NoReadyWorkDetail: noReadyWorkBreakdownFromLifecycle(b),
	}
	snapshot := queueSnapshotFromLifecycle(b)
	result.QueueSnapshot = &snapshot
	if decision, ok := work.ClassifyStop(work.StopInput{
		NoReadyWork: true,
		Once:        mode == executeloop.ModeOnce,
		Mode:        mode,
	}); ok {
		result.StopCondition = string(decision.Condition)
		result.ExitReason = decision.ExitReason
	}
	return result
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
				"bead_id":    s.BeadID,
				"priority":   s.Priority,
				"queue_rank": queueRankValue(s.BeadRank),
				"reason":     s.Reason,
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
	BeadRank *int
	Reason   string
}

func hasGuardSkips(skips []pickerSkip) bool {
	for _, skip := range skips {
		switch skip.Reason {
		case "label_filter", "in_attempted", "claim_race", "eligibility_filter", "retry_cooldown", "target_bead":
			continue
		default:
			return true
		}
	}
	return false
}

func systemicPreClaimSkipDetail(skips []pickerSkip) (string, bool) {
	detail := ""
	seenSystemic := false
	for _, skip := range skips {
		switch skip.Reason {
		case "label_filter", "in_attempted", "claim_race", "eligibility_filter", "retry_cooldown", "target_bead":
			continue
		default:
			if !work.IsSystemicPreClaimSkipReason(skip.Reason) {
				return "", false
			}
			seenSystemic = true
			if detail == "" {
				detail = work.SystemicPreClaimDetail(skip.Reason)
			}
		}
	}
	return detail, seenSystemic
}

func queueRankValue(raw any) any {
	rank, ok := parseQueueRank(raw)
	if !ok {
		return nil
	}
	return rank
}

// nextCandidate returns the next claimable bead from the execution-ready
// queue along with the list of higher-priority beads it skipped (and the
// reason for each skip). The returned skips slice is only meaningful when
// ok=true: it contains every entry that came BEFORE the chosen candidate
// in the priority-sorted ReadyExecution result.
//
// It delegates filter+sort to PreviewQueue and then additionally filters out
// beads already present in the current drain results slice (which is
// non-deterministic across runs and therefore excluded from the stable
// PreviewQueue surface).
func (w *ExecuteBeadWorker) nextCandidate(ctx context.Context, results []ExecuteBeadReport, guards []work.Guard, labelFilter, targetBeadID string) (bead.Bead, []pickerSkip, bool, error) {
	// Use PreviewQueue for the stable filter+sort logic. Limit=0 returns all
	// entries so we can scan for the first candidate not already present in
	// the current drain results slice.
	entries, err := PreviewQueue(w.Store, PickerFilters{LabelFilter: labelFilter}, 0)
	if err != nil {
		return bead.Bead{}, nil, false, err
	}

	// Rebuild the ready list from the preview entries in picker order so we
	// can apply the per-run drain results slice on top. We need the original
	// bead values for the return; fetch them from ReadyExecution (already
	// ordered).
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
			skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, BeadRank: queueRankPtr(candidate.Extra["queue-rank"]), Reason: "target_bead"})
			continue
		}
		if hasResultForBead(results, candidate.ID) {
			skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, BeadRank: queueRankPtr(candidate.Extra["queue-rank"]), Reason: "in_attempted"})
			continue
		}
		if entry.FilterDecision == FilterDecisionSkipped {
			// PreviewQueue already applied label_filter; record as skip.
			skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, BeadRank: queueRankPtr(candidate.Extra["queue-rank"]), Reason: "label_filter"})
			continue
		}
		allowed := true
		for _, guard := range guards {
			if guard == nil {
				continue
			}
			ok, reason := guard.Allow(ctx, candidate.ID)
			if ok {
				continue
			}
			allowed = false
			if reason != "" {
				skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, BeadRank: queueRankPtr(candidate.Extra["queue-rank"]), Reason: reason})
			}
			break
		}
		if !allowed {
			continue
		}
		return candidate, skips, true, nil
	}
	return bead.Bead{}, skips, false, nil
}

func hasResultForBead(results []ExecuteBeadReport, beadID string) bool {
	for _, report := range results {
		if report.BeadID == beadID {
			return true
		}
	}
	return false
}

// appendLoopRoutingEvidence records a kind:routing evidence event on the bead
// from the executor's ExecuteBeadReport, so that review-outcomes analytics can
// attribute a subsequent review verdict to the originating provider/model powerClass.
// Best-effort: errors and missing-provider cases are silently ignored.
//
// failedRoutes, when non-empty, populates fallback_chain so the routing event
// records which earlier provider/model tuples were excluded before this
// successful (or current) route was selected. Pass nil when there is no prior
// route-failure evidence on the bead.
func appendLoopRoutingEvidence(store BeadEventAppender, beadID string, report ExecuteBeadReport, createdAt time.Time, failedRoutes []FailedRouteEntry) {
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
	chain := make([]map[string]any, 0, len(failedRoutes))
	for _, e := range failedRoutes {
		chain = append(chain, map[string]any{
			"provider":     e.Provider,
			"model":        e.Model,
			"actual_power": e.ActualPower,
			"reason":       e.Reason,
		})
	}
	body, err := json.Marshal(map[string]any{
		"resolved_provider":    provider,
		"resolved_model":       report.Model,
		"fallback_chain":       chain,
		"requested_profile":    report.RequestedProfile,
		"inferred_power_class": report.InferredPowerClass,
		"routing_intent_note":  report.RoutingIntentNote,
		"resolved_power_class": report.ResolvedPowerClass,
		"escalation_count":     report.EscalationCount,
		"final_power_class":    report.FinalPowerClass,
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
		Source:    "ddx work",
		CreatedAt: createdAt,
	})
}

func appendExecutionRoutingIntentEvidence(store BeadEventAppender, target bead.Bead, report ExecuteBeadReport, createdAt time.Time) {
	if store == nil || target.ID == "" {
		return
	}
	intent := escalation.ParseExecutionHint(&target)
	inferredPolicy := string(intent.InferredPowerClass)
	if report.InferredPowerClass != "" {
		inferredPolicy = report.InferredPowerClass
	}
	body := map[string]any{
		"bead_id":                 target.ID,
		"attempt_id":              report.AttemptID,
		"routing_intent_source":   string(intent.Source),
		"estimated_difficulty":    string(intent.EstimatedDifficulty),
		"inferred_power_class":    inferredPolicy,
		"requested_profile":       report.RequestedProfile,
		"actual_harness":          report.Harness,
		"actual_provider":         report.Provider,
		"actual_model":            report.Model,
		"actual_power":            report.ActualPower,
		"routing_intent_degraded": false,
		"routing_intent_note":     "",
		"rejected_route_pins":     intent.RejectedRoutePins,
	}
	degraded := false
	note := ""
	if strings.TrimSpace(report.RoutingIntentNote) != "" {
		degraded = true
		if note == "" {
			note = report.RoutingIntentNote
		}
	}
	if report.Harness == "" || report.Model == "" {
		degraded = true
		if note == "" {
			note = "actual route facts unavailable"
		}
	}
	body["routing_intent_degraded"] = degraded
	body["routing_intent_note"] = note
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	summary := fmt.Sprintf("source=%s powerClass=%s", intent.Source, inferredPolicy)
	if report.Model != "" {
		summary += " model=" + report.Model
	}
	if report.Harness != "" {
		summary += " harness=" + report.Harness
	}
	if degraded && note != "" {
		summary += " note=" + note
	}
	_ = store.AppendEvent(target.ID, bead.BeadEvent{
		Kind:      "execution-routing-intent",
		Summary:   summary,
		Body:      string(data),
		Actor:     "ddx",
		Source:    "ddx work",
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
			Source:    "ddx work",
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
		Source:    "ddx work",
		CreatedAt: createdAt,
	})
}

// callAttemptFinalized invokes hook when non-nil. Best-effort: panics inside
// the hook are not recovered; callers should ensure hooks are non-blocking.
func callAttemptFinalized(hook func(ExecuteBeadReport), report ExecuteBeadReport) {
	if hook != nil {
		hook(report)
	}
}

func executeBeadLoopEvent(report ExecuteBeadReport, actor string, createdAt time.Time) bead.BeadEvent {
	parts := []string{}
	if report.Status == ExecuteBeadStatusPreservedNeedsReview {
		parts = append(parts, preservedNeedsReviewNote(report))
	}
	if report.Detail != "" {
		parts = append(parts, report.Detail)
	}
	if report.PowerClass != "" {
		parts = append(parts, fmt.Sprintf("powerClass=%s", report.PowerClass))
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
	if len(report.CycleTrace) > 0 {
		if traceJSON, err := json.Marshal(report.CycleTrace); err == nil {
			parts = append(parts, "cycle_trace="+string(traceJSON))
		}
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
		Source:    "ddx work",
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
// the Fizeau harness (session_id/seq/type/ts/data) so existing event
// consumers can parse the stream uniformly. Errors are swallowed:
// structured logging must never break the core work.
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
	frame := make([]byte, len(line)+1)
	copy(frame, line)
	frame[len(line)] = '\n'
	_, _ = sink.Write(frame)
}

func resourceExhaustedCheckResult(report ExecuteBeadReport) (ExecutionResourceCheckResult, bool) {
	switch v := report.ResourceExhausted.(type) {
	case ExecutionResourceCheckResult:
		return v, true
	case *ExecutionResourceCheckResult:
		if v != nil {
			return *v, true
		}
	}
	return ExecutionResourceCheckResult{}, false
}

func emitResourcePreflight(emit func(string, map[string]any), phase string, result ExecutionResourceCheckResult, checkErr error) {
	if emit == nil {
		return
	}
	status := "ok"
	if checkErr != nil {
		status = "error"
		var resourceErr *ResourceExhaustedError
		if errors.As(checkErr, &resourceErr) {
			status = ExecuteBeadStatusResourceExhausted
		}
	}
	summary := result.CleanupSummary
	emit("resource.preflight", map[string]any{
		"phase":                     phase,
		"status":                    status,
		"project_root":              result.ProjectRoot,
		"temp_root":                 result.TempRoot,
		"evidence_roots":            result.EvidenceRoots,
		"root_checks_before":        result.BeforeRootChecks,
		"root_checks_after":         result.RootChecks,
		"cleanup_summary":           summary,
		"cleanup_bytes_reclaimed":   summary.BytesReclaimed + summary.ScratchBytesReclaimed,
		"cleanup_inodes_reclaimed":  summary.InodesReclaimed + summary.ScratchInodesReclaimed,
		"cleanup_scratch_dirs":      summary.RemovedScratchDirs,
		"cleanup_temp_dirs":         summary.RemovedUnregisteredTempDirs,
		"cleanup_registered_trees":  summary.RemovedRegisteredWorktrees,
		"cleanup_run_state_files":   summary.RemovedRunStateFiles,
		"cleanup_preserved_scratch": summary.PreservedActiveScratchDirs,
	})
}

func logResourcePreflight(log io.Writer, phase string, result ExecutionResourceCheckResult, checkErr error) {
	if log == nil {
		return
	}
	if checkErr == nil && len(result.BeforeRootChecks) == 0 && !executionCleanupSummaryMeaningful(result.CleanupSummary) {
		return
	}
	status := "ok"
	if checkErr != nil {
		status = "error"
		var resourceErr *ResourceExhaustedError
		if errors.As(checkErr, &resourceErr) {
			status = ExecuteBeadStatusResourceExhausted
		}
	}
	before := result.BeforeRootChecks
	if len(before) == 0 {
		before = result.RootChecks
	}
	_, _ = fmt.Fprintf(
		log,
		"resource preflight (%s): status=%s before=[%s] after=[%s] cleanup_reclaimed_bytes=%d cleanup_reclaimed_inodes=%d\n",
		phase,
		status,
		formatResourceRootChecks(before),
		formatResourceRootChecks(result.RootChecks),
		result.CleanupSummary.BytesReclaimed+result.CleanupSummary.ScratchBytesReclaimed,
		result.CleanupSummary.InodesReclaimed+result.CleanupSummary.ScratchInodesReclaimed,
	)
}

func formatResourceRootChecks(checks []ExecutionResourceRootCheck) string {
	if len(checks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(checks))
	for _, check := range checks {
		part := fmt.Sprintf("%s bytes_free=%d inodes_free=%d", check.Path, check.BytesFree, check.InodesFree)
		if len(check.Notes) > 0 {
			part += " notes=" + strings.Join(check.Notes, ",")
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}

func emitResourceExhausted(emit func(string, map[string]any), store ExecuteBeadLoopStore, beadID string, report ExecuteBeadReport, actor string, createdAt time.Time) {
	var detail map[string]any
	if result, ok := resourceExhaustedCheckResult(report); ok {
		detail = map[string]any{
			"bead_id":                   beadID,
			"project_root":              result.ProjectRoot,
			"temp_root":                 result.TempRoot,
			"evidence_roots":            result.EvidenceRoots,
			"root_checks_before":        result.BeforeRootChecks,
			"root_checks_after":         result.RootChecks,
			"root_checks":               result.RootChecks,
			"cleanup_summary":           result.CleanupSummary,
			"cleanup_bytes_reclaimed":   result.CleanupSummary.BytesReclaimed + result.CleanupSummary.ScratchBytesReclaimed,
			"cleanup_inodes_reclaimed":  result.CleanupSummary.InodesReclaimed + result.CleanupSummary.ScratchInodesReclaimed,
			"cleanup_scratch_dirs":      result.CleanupSummary.RemovedScratchDirs,
			"cleanup_temp_dirs":         result.CleanupSummary.RemovedUnregisteredTempDirs,
			"cleanup_registered_trees":  result.CleanupSummary.RemovedRegisteredWorktrees,
			"cleanup_run_state_files":   result.CleanupSummary.RemovedRunStateFiles,
			"cleanup_preserved_scratch": result.CleanupSummary.PreservedActiveScratchDirs,
			"detail":                    report.Detail,
			"status":                    report.Status,
		}
		body, err := json.Marshal(detail)
		if err != nil {
			detail = map[string]any{
				"bead_id": beadID,
				"detail":  report.Detail,
				"status":  report.Status,
			}
		} else {
			detail["body"] = string(body)
		}
	} else {
		detail = map[string]any{
			"bead_id": beadID,
			"detail":  report.Detail,
			"status":  report.Status,
		}
	}
	emit("resource.exhausted", detail)
	if store == nil {
		return
	}
	body, err := json.Marshal(detail)
	if err != nil {
		body = []byte(report.Detail)
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "resource-exhausted",
		Summary:   ResourceExhaustedStopMessage,
		Body:      string(body),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: createdAt,
	})
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
	case ExecuteBeadStatusResourceExhausted:
		if report.Detail != "" {
			return fmt.Sprintf("resource_exhausted: %s", report.Detail)
		}
		return "resource_exhausted"
	case ExecuteBeadStatusNoChanges:
		if report.NoChangesRationale != "" {
			return fmt.Sprintf("no_changes: %s", report.NoChangesRationale)
		}
		return "no_changes"
	case ExecuteBeadStatusNoEvidenceProduced:
		detail := report.Detail
		if detail == "" {
			detail = "agent exited without a commit or no_changes_rationale.txt"
		}
		return fmt.Sprintf("no_evidence_produced: %s", detail)
	default:
		detail := report.Detail
		if detail == "" {
			detail = report.Status
		}
		if report.OutcomeReason != "" {
			return fmt.Sprintf("%s: %s", report.OutcomeReason, detail)
		}
		if report.PreserveRef != "" {
			return fmt.Sprintf("preserved: %s", detail)
		}
		return fmt.Sprintf("error: %s", detail)
	}
}

func formatLoopResultLine(beadID string, report ExecuteBeadReport) string {
	return fmt.Sprintf("%s %s → %s", loopResultMarker(report), beadID, formatLoopResult(report))
}

func loopResultMarker(report ExecuteBeadReport) string {
	switch report.Status {
	case ExecuteBeadStatusSuccess, ExecuteBeadStatusAlreadySatisfied:
		return "✓"
	case ExecuteBeadStatusNoChanges, ExecuteBeadStatusResourceExhausted:
		return "•"
	default:
		return "✗"
	}
}

func classifyLoopReportFailure(report *ExecuteBeadReport) {
	if report == nil || report.Status == ExecuteBeadStatusSuccess || report.Status == ExecuteBeadStatusAlreadySatisfied {
		return
	}
	if report.OutcomeReason != "" {
		return
	}
	combined := strings.TrimSpace(strings.Join([]string{
		report.Detail,
		report.Error,
		report.Stderr,
	}, "\n"))
	if system := ClassifyReadiness("", nil, combined); system.SystemReason != "" {
		switch system.SystemReason {
		case ReadinessSystemReasonQuota:
			report.OutcomeReason = "quota"
		case ReadinessSystemReasonTransport:
			report.OutcomeReason = "transport"
			report.Disrupted = true
			report.DisruptionReason = "transport_error"
		case ReadinessSystemReasonResourceExhausted:
			report.OutcomeReason = ReadinessSystemReasonResourceExhausted
			report.Disrupted = true
			report.DisruptionReason = ReadinessSystemReasonResourceExhausted
		case ReadinessSystemReasonRepoConcurrency:
			report.OutcomeReason = FailureModeLockContention
			report.Disrupted = true
			report.DisruptionReason = FailureModeLockContention
		case ReadinessSystemReasonRouting:
			mode := ClassifyFailureMode(report.Status, 1, combined)
			if mode == "" || mode == FailureModeUnknown {
				report.OutcomeReason = "routing"
			} else {
				report.OutcomeReason = mode
			}
		default:
			report.OutcomeReason = "recoverable"
		}
		return
	}
	if report.Status == ExecuteBeadStatusNoEvidenceProduced {
		report.OutcomeReason = FailureModeNoEvidenceProduced
		return
	}
	mode := ClassifyFailureMode(report.Status, 1, combined)
	if mode == "" || mode == FailureModeUnknown {
		return
	}
	report.OutcomeReason = mode
	if mode == FailureModeLockContention {
		report.Disrupted = true
		report.DisruptionReason = FailureModeLockContention
	}
}

// storeBeadDepth walks the parent chain of b using the loop's store and returns
// the consecutive decomposition depth, not ordinary parent ancestry. A bead only
// consumes decomposition budget when it is a child marked with the decomposed
// label; epics and other organizational parents do not make execution less
// eligible.
func storeBeadDepth(ctx context.Context, store ExecuteBeadLoopStore, b *bead.Bead) int {
	if b == nil {
		return 0
	}
	depth := beadDecomposedChildDepth(b)
	seen := map[string]struct{}{}
	current := b
	for {
		parentID := strings.TrimSpace(current.Parent)
		if parentID == "" {
			break
		}
		if _, ok := seen[parentID]; ok {
			break
		}
		seen[parentID] = struct{}{}
		parent, err := store.Get(ctx, parentID)
		if err != nil || parent == nil {
			break
		}
		if beadDecomposedChildDepth(parent) == 0 {
			break
		}
		depth++
		current = parent
	}
	return depth
}

// applyPreClaimDecomposition creates child beads, wires parent→child dependency
// edges, and appends a triage-decomposed event to the parent. It returns the
// IDs of the created children so the caller can log or record them.
func applyPreClaimDecomposition(ctx context.Context, store ExecuteBeadLoopStore, parent *bead.Bead, decomp *PreClaimDecomposition, actor string, at time.Time) ([]string, error) {
	childIDs := make([]string, 0, len(decomp.Children))
	for _, child := range decomp.Children {
		nb := &bead.Bead{
			Title:       child.Title,
			Description: child.Description,
			Acceptance:  child.Acceptance,
			Labels:      append([]string(nil), child.Labels...),
			Parent:      parent.ID,
		}
		if err := store.Create(ctx, nb); err != nil {
			return childIDs, fmt.Errorf("decompose: create child %q: %w", child.Title, err)
		}
		childIDs = append(childIDs, nb.ID)
	}

	// Wire parent → children dependency edges so the parent is blocked until
	// all children close.
	if err := store.Update(ctx, parent.ID, func(b *bead.Bead) {
		for _, childID := range childIDs {
			b.AddDep(childID, "depends_on")
		}
	}); err != nil {
		return childIDs, fmt.Errorf("decompose: wire deps on parent %s: %w", parent.ID, err)
	}

	body, _ := json.Marshal(map[string]any{
		"child_ids": childIDs,
		"rationale": decomp.Rationale,
		"ac_map":    decomp.ACMap,
	})
	return childIDs, store.AppendEvent(parent.ID, bead.BeadEvent{
		Kind:      "triage-decomposed",
		Summary:   fmt.Sprintf("decomposed into %s", strings.Join(childIDs, ", ")),
		Body:      string(body),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
	})
}

// parkBeadPostIntakeRejection moves the bead to proposed and appends an
// intake.blocked event so the bead is filtered from ReadyExecution until an
// operator reviews the intake decision. It does not unclaim — the caller must
// call Unclaim after this returns (whether or not parking succeeds).
func parkBeadPostIntakeRejection(store ExecuteBeadLoopStore, candidate *bead.Bead, actor string, outcome PreClaimIntakeOutcome, reason, detail string, at time.Time) (bool, error) {
	if candidate == nil {
		return false, fmt.Errorf("pre-claim intake park requires a bead candidate")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = string(outcome)
	}
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = reason
	}
	ruleID := "pre_claim_intake." + strings.TrimSpace(string(outcome))
	findingFingerprint := preClaimIntakeFindingFingerprint(candidate, ruleID, reason, "pre_claim_intake", "block", "park", "review intake result and accept, rewrite, split, block, or cancel")
	promptFingerprint := bead.PromptFingerprint(*candidate)
	if honored, err := preClaimIntakeOverrideHonored(store, candidate, findingFingerprint); err != nil {
		return false, err
	} else if honored {
		appendPreClaimIntakeOverrideHonoredEvent(store, candidate.ID, actor, findingFingerprint, promptFingerprint, at)
		return false, nil
	}
	body := readinessDecisionBody(
		ruleID,
		reason,
		"pre_claim_intake",
		"block",
		"park",
		"review intake result and accept, rewrite, split, block, or cancel",
		map[string]any{
			"intake_outcome":       string(outcome),
			"detail":               detail,
			"prompt_fingerprint":   promptFingerprint,
			"accepted_fingerprint": findingFingerprint,
		},
	)
	body["fingerprint"] = findingFingerprint
	body["prompt_fingerprint"] = promptFingerprint
	bodyJSON, _ := json.Marshal(body)
	if err := store.ParkToProposed(candidate.ID, bead.ParkIntakeRejection, func(b *bead.Bead) {
		ensureBeadExtra(b)
		// Migration-only cleanup: defensive removal for legacy rows that escaped
		// the lifecycle migration or arrived via external import.
		b.Labels = removeBeadLabels(b.Labels, bead.LabelNeedsHuman, bead.LabelNeedsInvestigation)
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason:          reason,
			Since:           at.UTC().Format(time.RFC3339),
			Source:          "ddx work",
			SuggestedAction: "review intake result and accept, rewrite, split, block, or cancel",
			Summary:         "pre-claim intake blocked execution",
		})
	}); err != nil {
		return false, err
	}
	if err := store.AppendEvent(candidate.ID, bead.BeadEvent{
		Kind:      "intake.blocked",
		Summary:   string(outcome),
		Body:      string(bodyJSON),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func appendPreClaimIntakeWarning(store ExecuteBeadLoopStore, beadID, actor, reason, detail string, at time.Time) {
	if store == nil {
		return
	}
	body := readinessDecisionBodyJSON(
		"pre_claim_intake."+strings.TrimSpace(reason),
		reason,
		"pre_claim_intake",
		"warn-only",
		"warn",
		"revise the rewrite so it preserves every explicit commitment",
		map[string]any{
			"reason": reason,
			"detail": detail,
		},
	)
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "intake.warn",
		Summary:   reason,
		Body:      body,
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
	})
}

type preClaimIntakeHookCallResult struct {
	result PreClaimIntakeResult
	err    error
}

type preDispatchLintHookCallResult struct {
	result LintResult
	err    error
}

func runPreClaimIntakeHookWithTimeout(ctx context.Context, hook PreClaimIntakeHook, beadID string, timeout time.Duration) (PreClaimIntakeResult, error) {
	if hook == nil {
		return PreClaimIntakeResult{}, nil
	}
	if timeout <= 0 {
		timeout = work.DefaultPreClaimTimeout
	}
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan preClaimIntakeHookCallResult, 1)
	go func() {
		result, err := hook(hookCtx, beadID)
		resultCh <- preClaimIntakeHookCallResult{result: result, err: err}
	}()

	select {
	case call := <-resultCh:
		if errors.Is(hookCtx.Err(), context.DeadlineExceeded) || errors.Is(call.err, context.DeadlineExceeded) {
			return timeoutPreClaimIntakeResult(beadID, timeout), nil
		}
		return call.result, call.err
	case <-hookCtx.Done():
		if errors.Is(hookCtx.Err(), context.DeadlineExceeded) {
			return timeoutPreClaimIntakeResult(beadID, timeout), nil
		}
		return PreClaimIntakeResult{}, hookCtx.Err()
	}
}

func runPreDispatchLintHookWithTimeout(ctx context.Context, hook func(context.Context, string) (LintResult, error), beadID string, timeout time.Duration) (LintResult, error) {
	if hook == nil {
		return LintResult{}, nil
	}
	if timeout <= 0 {
		timeout = work.DefaultPreClaimTimeout
	}
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan preDispatchLintHookCallResult, 1)
	go func() {
		result, err := hook(hookCtx, beadID)
		resultCh <- preDispatchLintHookCallResult{result: result, err: err}
	}()

	select {
	case call := <-resultCh:
		if errors.Is(hookCtx.Err(), context.DeadlineExceeded) || errors.Is(call.err, context.DeadlineExceeded) {
			return LintResult{}, &LintHookError{Kind: LintHookErrorKindCanceled, Err: context.DeadlineExceeded}
		}
		return call.result, call.err
	case <-hookCtx.Done():
		if errors.Is(hookCtx.Err(), context.DeadlineExceeded) {
			return LintResult{}, &LintHookError{Kind: LintHookErrorKindCanceled, Err: context.DeadlineExceeded}
		}
		return LintResult{}, hookCtx.Err()
	}
}

func timeoutPreClaimIntakeResult(beadID string, timeout time.Duration) PreClaimIntakeResult {
	detail := fmt.Sprintf("readiness check timed out after %s (%s)", timeout, beadID)
	return PreClaimIntakeResult{
		Outcome:      PreClaimIntakeError,
		Reason:       ReadinessReasonSystemUnready,
		SystemReason: ReadinessSystemReasonTimeout,
		Detail:       detail,
	}
}

// addBeadLabel mutates the bead in place to add a label idempotently. The
// store handles persistence; concurrent callers serialize via the store lock.
func addBeadLabel(ctx context.Context, store ExecuteBeadLoopStore, beadID, label string) error {
	if label == "" {
		return nil
	}
	return store.Update(ctx, beadID, func(b *bead.Bead) {
		for _, existing := range b.Labels {
			if existing == label {
				return
			}
		}
		b.Labels = append(b.Labels, label)
	})
}

const (
	executeLoopSmartRetryKey                = "work-smart-retry"
	executeLoopSmartRetryReasonKey          = "work-smart-retry-reason"
	executeLoopSmartRetrySuggestedActionKey = "work-smart-retry-suggested-action"
	executeLoopSuggestedActionKey           = "work-suggested-action"
)

func applyNoChangesSmartRetry(store ExecuteBeadLoopStore, beadID, actor string, noChanges *agenttry.NoChangesOutcome, actualPower int, nextFloorFn func(int) (int, error)) error {
	reason := strings.TrimSpace(noChanges.Reason)
	if reason == "" {
		reason = "autonomous work remains possible"
	}
	suggestedAction := strings.TrimSpace(noChanges.SuggestedAction)
	if suggestedAction == "" {
		suggestedAction = "retry with a smart agent"
	}

	// When a ladder is available, advance by one step. If the ladder is
	// exhausted (already at top powerClass), the work requires human input.
	if nextFloorFn != nil {
		nextFloor, err := nextFloorFn(actualPower)
		if err != nil {
			emitEscalationAbortedEvent(store, beadID, actor, "", "", actualPower, time.Now().UTC())
			return applyNoChangesOperatorRequired(store, beadID, actor, noChanges, time.Now().UTC())
		}
		return store.UpdateWithLifecycleStatus(beadID, bead.StatusOpen, bead.LifecycleTransitionOptions{
			Reason: reason,
			Actor:  actor,
			Source: "ddx work",
		}, func(b *bead.Bead) error {
			ensureBeadExtra(b)
			clearNoChangesLifecycleLabels(b)
			bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{})
			setNoChangesLifecycleMetadata(b, noChanges.EventKind, reason, suggestedAction)
			b.Extra[TriagePowerHintKey] = nextFloor
			b.Extra[executeLoopSmartRetryKey] = true
			b.Extra[executeLoopSmartRetryReasonKey] = reason
			b.Extra[executeLoopSmartRetrySuggestedActionKey] = suggestedAction
			return nil
		})
	}

	return store.UpdateWithLifecycleStatus(beadID, bead.StatusOpen, bead.LifecycleTransitionOptions{
		Reason: reason,
		Actor:  actor,
		Source: "ddx work",
	}, func(b *bead.Bead) error {
		ensureBeadExtra(b)
		clearNoChangesLifecycleLabels(b)
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{})
		setNoChangesLifecycleMetadata(b, noChanges.EventKind, reason, suggestedAction)
		b.Extra[TriagePowerHintKey] = string(escalation.PowerSmart)
		b.Extra[executeLoopSmartRetryKey] = true
		b.Extra[executeLoopSmartRetryReasonKey] = reason
		b.Extra[executeLoopSmartRetrySuggestedActionKey] = suggestedAction
		return nil
	})
}

// applyRepairCycleExhaustedEscalation bumps the implementer MinPower to the
// next powerClass when repair cycles are exhausted. If the ladder is already at the
// top powerClass, the bead is parked to proposed for operator review.
func applyRepairCycleExhaustedEscalation(store ExecuteBeadLoopStore, beadID, actor string, actualPower int, at time.Time, nextFloorFn func(int) (int, error)) error {
	if nextFloorFn != nil {
		nextFloor, err := nextFloorFn(actualPower)
		if err == nil {
			return store.UpdateWithLifecycleStatus(beadID, bead.StatusOpen, bead.LifecycleTransitionOptions{
				Reason: "repair cycle exhausted: escalating implementer to higher powerClass",
				Actor:  actor,
				Source: "ddx work",
			}, func(b *bead.Bead) error {
				ensureBeadExtra(b)
				b.Extra[TriagePowerHintKey] = nextFloor
				return nil
			})
		}
		emitEscalationAbortedEvent(store, beadID, actor, "", "", actualPower, at)
	}
	return store.ParkToProposed(beadID, bead.ParkPostReviewMalfunction, func(b *bead.Bead) {
		ensureBeadExtra(b)
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason:          "repair cycle exhausted at top power: operator decision required",
			Since:           at.UTC().Format(time.RFC3339),
			Source:          "ddx work",
			SuggestedAction: "review the blocked attempt and accept, split, or retry with a stronger model",
			Summary:         "repair cycle exhausted: operator attention required",
		})
	})
}

func applyNoChangesOperatorRequired(store ExecuteBeadLoopStore, beadID, actor string, noChanges *agenttry.NoChangesOutcome, at time.Time) error {
	reason := strings.TrimSpace(noChanges.Reason)
	if reason == "" {
		reason = "operator decision required before another automated attempt"
	}
	suggestedAction := strings.TrimSpace(noChanges.SuggestedAction)
	if suggestedAction == "" {
		suggestedAction = "review and accept, split, block, or cancel this proposed work"
	}
	return store.ParkToProposed(beadID, bead.ParkNoChangesOperatorRequired, func(b *bead.Bead) {
		ensureBeadExtra(b)
		clearNoChangesLifecycleLabels(b)
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{})
		clearSmartRetryMetadata(b)
		setNoChangesLifecycleMetadata(b, noChanges.EventKind, reason, suggestedAction)
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason:          reason,
			Since:           at.UTC().Format(time.RFC3339),
			Source:          "ddx work",
			SuggestedAction: suggestedAction,
			Summary:         "no_changes requested operator attention",
		})
	})
}

func applyNoChangesBlockedExternal(store ExecuteBeadLoopStore, beadID, actor string, noChanges *agenttry.NoChangesOutcome) error {
	reason := strings.TrimSpace(noChanges.Reason)
	if reason == "" {
		reason = "external recheckable blocker"
	}
	suggestedAction := strings.TrimSpace(noChanges.SuggestedAction)
	if suggestedAction == "" {
		suggestedAction = "recheck the external blocker and move status to open when cleared"
	}
	return store.UpdateWithLifecycleStatus(beadID, bead.StatusBlocked, bead.LifecycleTransitionOptions{
		ExternalBlockerReason: reason,
		Reason:                reason,
		Actor:                 actor,
		Source:                "ddx work",
	}, func(b *bead.Bead) error {
		ensureBeadExtra(b)
		clearNoChangesLifecycleLabels(b)
		clearSmartRetryMetadata(b)
		setNoChangesLifecycleMetadata(b, noChanges.EventKind, reason, suggestedAction)
		return nil
	})
}

func ensureBeadExtra(b *bead.Bead) {
	if b.Extra == nil {
		b.Extra = make(map[string]any)
	}
}

func clearNoChangesLifecycleLabels(b *bead.Bead) {
	// Migration-only cleanup: LabelNeedsInvestigation removal is defensive for
	// legacy rows that escaped the lifecycle migration or arrived via external import.
	b.Labels = removeBeadLabels(b.Labels,
		bead.LabelNoChangesUnverified,
		bead.LabelNoChangesUnjustified,
		bead.LabelNeedsInvestigation,
	)
}

func removeBeadLabels(labels []string, remove ...string) []string {
	if len(labels) == 0 || len(remove) == 0 {
		return labels
	}
	removeSet := make(map[string]struct{}, len(remove))
	for _, label := range remove {
		removeSet[label] = struct{}{}
	}
	out := labels[:0]
	for _, label := range labels {
		if _, drop := removeSet[label]; drop {
			continue
		}
		out = append(out, label)
	}
	return out
}

func setNoChangesLifecycleMetadata(b *bead.Bead, lastStatus, detail, suggestedAction string) {
	ensureBeadExtra(b)
	delete(b.Extra, bead.ExtraRetryAfter)
	setOrDeleteBeadExtra(b.Extra, bead.ExtraLastStatus, strings.TrimSpace(lastStatus))
	setOrDeleteBeadExtra(b.Extra, bead.ExtraLastDetail, strings.TrimSpace(detail))
	setOrDeleteBeadExtra(b.Extra, executeLoopSuggestedActionKey, strings.TrimSpace(suggestedAction))
}

func clearSmartRetryMetadata(b *bead.Bead) {
	if b.Extra == nil {
		return
	}
	delete(b.Extra, TriagePowerHintKey)
	delete(b.Extra, executeLoopSmartRetryKey)
	delete(b.Extra, executeLoopSmartRetryReasonKey)
	delete(b.Extra, executeLoopSmartRetrySuggestedActionKey)
}

func setOrDeleteBeadExtra(extra map[string]any, key, value string) {
	if value == "" {
		delete(extra, key)
		return
	}
	extra[key] = value
}

func clearExecuteLoopNoChangesMetadata(ctx context.Context, store ExecuteBeadLoopStore, beadID string) error {
	return store.Update(ctx, beadID, func(b *bead.Bead) {
		if b.Extra == nil {
			return
		}
		delete(b.Extra, "work-retry-after")
		delete(b.Extra, bead.ExtraCooldownBaseRev)
		delete(b.Extra, "work-last-status")
		delete(b.Extra, "work-last-detail")
		delete(b.Extra, executeLoopSuggestedActionKey)
		clearSmartRetryMetadata(b)
	})
}

// isValidImplementationAttempt reports whether the given report came from a
// real implementation attempt — one where the implementer had checkout context
// established and a result that can be compared against the base revision for
// no-progress accounting. Returns false for system failures, infrastructure
// issues, intake/scheduling outcomes, review/operator-managed states, and
// other non-implementation outcomes that should not consume the no-progress
// retry budget or trigger long cooldowns.
func isValidImplementationAttempt(report ExecuteBeadReport) bool {
	if report.Disrupted {
		return false
	}
	if isTransientOutcomeReason(report.OutcomeReason) {
		return false
	}
	// Extended system-failure and non-implementation outcome reasons.
	switch report.OutcomeReason {
	case "preflight_failed",
		"intake_block",
		"claim_race",
		"actionable_but_rewritten",
		"too_large_decomposed",
		"intake_error",
		"ambiguous_needs_human",
		FailureModeAuthError,
		FailureModeHarnessNotInstalled,
		FailureModeAgentPowerUnsatisfied,
		FailureModeBlockedByPassthroughConstraint,
		FailureModeNoEvidenceProduced,
		FailureModeWorktreeLost,
		ReadinessSystemReasonResourceExhausted,
		ReadinessSystemReasonRepoConcurrency,
		"operator_required":
		return false
	}
	// Operator/review status codes where the implementer did not run a full
	// implementation pass.
	switch report.Status {
	case ExecuteBeadStatusReviewMalfunction,
		ExecuteBeadStatusDeclinedNeedsDecomposition,
		ExecuteBeadStatusLandConflictOperatorRequired,
		ExecuteBeadStatusReviewTerminalBlock:
		return false
	}
	// Without a BaseRev the implementer never had a commit baseline.
	if report.BaseRev == "" {
		return false
	}
	return true
}

func shouldSuppressNoProgress(report ExecuteBeadReport) bool {
	if !isValidImplementationAttempt(report) {
		return false
	}
	if report.ResultRev == "" {
		return false
	}
	return report.BaseRev == report.ResultRev
}

func isTransientOutcomeReason(reason string) bool {
	switch reason {
	case "transport", "quota", "routing", "timeout", "merge_conflict",
		ReadinessSystemReasonResourceExhausted,
		ReadinessSystemReasonRepoConcurrency,
		FailureModeLockContention,
		FailureModeNoViableProvider,
		FailureModeWorktreeLost:
		return true
	default:
		return false
	}
}

// isInfraFaultCooldownStatus returns true for work-last-status values that
// represent infra-class failures where time passing (not model/bead work) is
// what resolves the condition. Used by the paused-infra detector to
// distinguish infra-cooled beads from bead-fault-cooled beads.
func isInfraFaultCooldownStatus(status string) bool {
	switch status {
	case FailureModeProviderConnectivity, FailureModeNoViableProvider,
		"loop-error":
		return true
	default:
		return false
	}
}

// infraCooldownResumeAt inspects the IDs in cooldownIDs, gets each bead's
// work-last-status, and returns (earliestRetry, true) when every bead carries
// an infra-fault status. If any bead has a non-infra status or cannot be read,
// it returns (zero, false).
func infraCooldownResumeAt(store ExecuteBeadLoopStore, cooldownIDs []string) (time.Time, bool) {
	var earliest time.Time
	for _, id := range cooldownIDs {
		b, err := store.Get(id)
		if err != nil || b == nil {
			return time.Time{}, false
		}
		status, _ := b.Extra[bead.ExtraLastStatus].(string)
		if !isInfraFaultCooldownStatus(status) {
			return time.Time{}, false
		}
		retryStr, _ := b.Extra[bead.ExtraRetryAfter].(string)
		if retryStr != "" {
			if t, parseErr := time.Parse(time.RFC3339, retryStr); parseErr == nil {
				if earliest.IsZero() || t.Before(earliest) {
					earliest = t
				}
			}
		}
	}
	if earliest.IsZero() {
		earliest = time.Now().UTC().Add(PausedInfraInterval)
	}
	return earliest, true
}

func isNoViableProviderReport(report ExecuteBeadReport) bool {
	if report.OutcomeReason == FailureModeNoViableProvider {
		return true
	}
	combined := strings.TrimSpace(strings.Join([]string{
		report.Detail,
		report.Error,
		report.Stderr,
	}, "\n"))
	return ClassifyFailureMode(report.Status, 0, combined) == FailureModeNoViableProvider
}

// providerConnectivityMarkers names the substrings that indicate the routed
// provider endpoint itself was unreachable (TCP-level dial failure, connection
// refused, network down). These are stricter than the broader transport
// markers because we only treat them as route-exclusion evidence when paired
// with a non-empty Provider in the report.
var providerConnectivityMarkers = []string{
	"dial tcp",
	"connection refused",
	"i/o timeout",
	"no route to host",
	"network is unreachable",
	"bad gateway",
	"service unavailable",
	"gateway timeout",
}

// isProviderConnectivityFailureReport reports whether a worker report describes
// a routed provider endpoint that could not be reached. Requires both an
// identified route (Provider) and a transport-level error marker. Reports that
// already classify as no_viable_provider or routing-infrastructure failures
// keep their own paths; this is for the narrower "fizeau picked a route, the
// HTTP call to that endpoint failed" case.
func isProviderConnectivityFailureReport(report ExecuteBeadReport) bool {
	if report.Status != ExecuteBeadStatusExecutionFailed {
		return false
	}
	if strings.TrimSpace(report.Provider) == "" {
		return false
	}
	if isNoViableProviderReport(report) || isRoutingInfrastructureReport(report) {
		return false
	}
	combined := strings.ToLower(strings.Join([]string{
		report.Detail,
		report.Error,
		report.Stderr,
	}, "\n"))
	for _, marker := range providerConnectivityMarkers {
		if strings.Contains(combined, marker) {
			return true
		}
	}
	return false
}

// executeLoopFailedRoutesKey is the bead Extra key holding the JSON-encoded
// list of (provider, model, actual_power) tuples that have failed with a
// provider-connectivity error. Subsequent routing-evidence events read this
// list to populate fallback_chain so post-hoc review can see what was
// excluded; the power-hint nudge on the same bead biases fizeau's next
// RouteRequest off the failed power tier when a ladder is available.
const executeLoopFailedRoutesKey = "work-failed-routes"

// failedRoutesMaxEntries is the FIFO ring cap on the work-failed-routes list.
// When the list is full, the oldest entry is evicted on insert.
const failedRoutesMaxEntries = 32

// RouteExclusionWindow is the time window within which a recorded
// work-failed-routes entry suppresses a (provider, model) pair in
// the next RouteRequest.ExcludedRoutes payload. Entries older than this
// window remain in the bead's audit list but do not constrain routing,
// allowing recovery once the provider returns.
const RouteExclusionWindow = time.Hour

// FailedRouteEntry is the JSON shape persisted under executeLoopFailedRoutesKey.
// Exported so external callers (test helpers, tooling) can decode it without
// duplicating the schema.
type FailedRouteEntry struct {
	Provider    string `json:"provider"`
	Model       string `json:"model,omitempty"`
	ActualPower int    `json:"actual_power,omitempty"`
	Reason      string `json:"reason,omitempty"`
	At          string `json:"at,omitempty"`
	Count       int    `json:"count,omitempty"`
}

// readFailedRoutes decodes the failed-route list from a bead's Extra. Returns
// nil when absent or malformed; never errors. Transparently normalizes legacy
// data: collapses duplicates by (provider, model) and caps at failedRoutesMaxEntries.
func readFailedRoutes(extra map[string]any) []FailedRouteEntry {
	if len(extra) == 0 {
		return nil
	}
	raw, ok := extra[executeLoopFailedRoutesKey]
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var entries []FailedRouteEntry
	if err := json.Unmarshal(encoded, &entries); err != nil {
		return nil
	}
	return normalizeFailedRoutes(entries)
}

// normalizeFailedRoutes collapses duplicate (provider, model) entries keeping
// the newest At timestamp and summing counts, then caps the list at
// failedRoutesMaxEntries by evicting oldest entries (FIFO). Used by
// readFailedRoutes to transparently migrate legacy bead data on read.
func normalizeFailedRoutes(entries []FailedRouteEntry) []FailedRouteEntry {
	if len(entries) == 0 {
		return entries
	}
	type key struct{ provider, model string }
	index := make(map[key]int, len(entries))
	collapsed := make([]FailedRouteEntry, 0, len(entries))
	for _, e := range entries {
		k := key{e.Provider, e.Model}
		if i, ok := index[k]; ok {
			existingCount := collapsed[i].Count
			if existingCount == 0 {
				existingCount = 1
			}
			ec := e.Count
			if ec == 0 {
				ec = 1
			}
			if e.At > collapsed[i].At {
				collapsed[i] = FailedRouteEntry{
					Provider:    e.Provider,
					Model:       e.Model,
					ActualPower: e.ActualPower,
					Reason:      e.Reason,
					At:          e.At,
					Count:       existingCount + ec,
				}
			} else {
				collapsed[i].Count = existingCount + ec
			}
		} else {
			index[k] = len(collapsed)
			c := e
			if c.Count == 0 {
				c.Count = 1
			}
			collapsed = append(collapsed, c)
		}
	}
	if len(collapsed) > failedRoutesMaxEntries {
		collapsed = collapsed[len(collapsed)-failedRoutesMaxEntries:]
	}
	return collapsed
}

// appendFailedRoute records entry on b.Extra under executeLoopFailedRoutesKey.
// If an identical (provider, model) already exists, its At timestamp is updated
// and its Count is incremented rather than appending a duplicate. When the list
// is at failedRoutesMaxEntries capacity, the oldest entry is evicted (FIFO).
func appendFailedRoute(b *bead.Bead, entry FailedRouteEntry) {
	ensureBeadExtra(b)
	existing := readFailedRoutes(b.Extra)
	for i, e := range existing {
		if e.Provider == entry.Provider && e.Model == entry.Model {
			existing[i].At = entry.At
			if entry.ActualPower != 0 {
				existing[i].ActualPower = entry.ActualPower
			}
			if entry.Reason != "" {
				existing[i].Reason = entry.Reason
			}
			existing[i].Count = e.Count + 1
			b.Extra[executeLoopFailedRoutesKey] = existing
			return
		}
	}
	if len(existing) >= failedRoutesMaxEntries {
		existing = existing[1:]
	}
	entry.Count = 1
	b.Extra[executeLoopFailedRoutesKey] = append(existing, entry)
}

// buildExcludedRoutes converts FailedRouteEntry values whose At timestamp
// falls within [now-window, now] into agentlib.ExcludedRoute entries for
// inclusion in a Fizeau RouteRequest.ExcludedRoutes payload. Entries outside
// the window are omitted from the result; the caller's input slice is never
// modified. An empty or nil input returns nil.
func buildExcludedRoutes(failedRoutes []FailedRouteEntry, now time.Time, window time.Duration) []agentlib.ExcludedRoute {
	if len(failedRoutes) == 0 {
		return nil
	}
	var out []agentlib.ExcludedRoute
	for _, r := range failedRoutes {
		if r.Provider == "" || r.At == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339, r.At)
		if err != nil {
			continue
		}
		if now.Sub(at) > window {
			continue
		}
		out = append(out, agentlib.ExcludedRoute{
			Provider: r.Provider,
			Model:    r.Model,
		})
	}
	return out
}

// CheckAndApplyRouteExclusions builds ExcludedRoutes from the bead's
// failed-routes list, calls resolveRoute to verify a viable candidate
// remains, and when none does escalates TriagePowerHintKey to the next
// ladder floor (composing with the ddx-8a7a6843 ladder-exhaustion path).
//
// Returns (report, true) when dispatch should be skipped (all routes
// excluded or resolveRoute returned an error); (zero, false) when the
// caller may proceed with dispatch normally.
// A nil resolveRoute is treated as a no-op and returns false.
func CheckAndApplyRouteExclusions(
	ctx context.Context,
	store ExecuteBeadLoopStore,
	beadID, actor string,
	extra map[string]any,
	now time.Time,
	minPower int,
	resolveRoute func(ctx context.Context, req agentlib.RouteRequest) (*agentlib.RouteDecision, error),
	nextFloorFn func(int) (int, error),
) (ExecuteBeadReport, bool) {
	if resolveRoute == nil {
		return ExecuteBeadReport{}, false
	}
	failedRoutes := readFailedRoutes(extra)
	excluded := buildExcludedRoutes(failedRoutes, now, RouteExclusionWindow)
	if len(excluded) == 0 {
		return ExecuteBeadReport{}, false
	}
	req := agentlib.RouteRequest{
		MinPower:       minPower,
		ExcludedRoutes: excluded,
	}
	if _, routeErr := resolveRoute(ctx, req); routeErr == nil {
		return ExecuteBeadReport{}, false
	}
	// No viable candidate at minPower with the current exclusions: escalate
	// TriagePowerHintKey so the next attempt routes to a stronger tier.
	nextFloor := minPower + 1
	if nextFloorFn != nil {
		if floor, err := nextFloorFn(minPower); err == nil {
			nextFloor = floor
		}
	}
	_ = store.Update(beadID, func(b *bead.Bead) {
		ensureBeadExtra(b)
		b.Extra[TriagePowerHintKey] = nextFloor
	})
	detail := fmt.Sprintf(
		"ResolveRoute: no viable routing candidate: all routes at power %d excluded by recent failures; escalating TriagePowerHintKey to %d",
		minPower, nextFloor,
	)
	return ExecuteBeadReport{
		BeadID:        beadID,
		Status:        ExecuteBeadStatusExecutionFailed,
		Detail:        detail,
		OutcomeReason: FailureModeNoViableProvider,
	}, true
}

// isOperatorRoutingPinned reports whether the resolved passthrough envelope
// carries any explicit operator pin (harness, model, or provider). When true,
// the route-exclusion path records the failure but does not bump the power
// hint — pinned routes must retry exactly as the operator requested.
func isOperatorRoutingPinned(pt config.AgentPassthrough) bool {
	return strings.TrimSpace(pt.Harness) != "" ||
		strings.TrimSpace(pt.Model) != "" ||
		strings.TrimSpace(pt.Provider) != ""
}

// applyProviderConnectivityRouteExclusion appends the failed (provider, model)
// to the bead's failed-route list and (when no operator pin is in force and a
// ladder step is available) advances TriagePowerHintKey above the failed
// route's actualPower so the next attempt's RouteRequest naturally picks a
// different candidate. The lifecycle status is unchanged (bead stays open).
//
// When the ladder is exhausted, it emits an execution-escalation-aborted event
// and sets TriagePowerHintKey to actualPower+1 as a sentinel so downstream
// routing knows to avoid the failed tier.
//
// When the same (provider, model) appears in the failed-routes list for the
// second time, the bead is promoted to operator_required.
func applyProviderConnectivityRouteExclusion(
	store ExecuteBeadLoopStore,
	beadID string,
	actor string,
	report ExecuteBeadReport,
	operatorPinned bool,
	nextFloorFn func(int) (int, error),
	at time.Time,
) error {
	var (
		ladderExhausted bool
		repeatFailure   bool
	)
	if err := store.Update(beadID, func(b *bead.Bead) {
		existing := readFailedRoutes(b.Extra)
		for _, e := range existing {
			if e.Provider == report.Provider && e.Model == report.Model {
				repeatFailure = true
				break
			}
		}
		appendFailedRoute(b, FailedRouteEntry{
			Provider:    report.Provider,
			Model:       report.Model,
			ActualPower: report.ActualPower,
			Reason:      FailureModeProviderConnectivity,
			At:          at.UTC().Format(time.RFC3339),
		})
		if operatorPinned || nextFloorFn == nil {
			return
		}
		nextFloor, err := nextFloorFn(report.ActualPower)
		if err != nil {
			ladderExhausted = true
			b.Extra[TriagePowerHintKey] = report.ActualPower + 1
			return
		}
		b.Extra[TriagePowerHintKey] = nextFloor
	}); err != nil {
		return err
	}
	if ladderExhausted {
		emitEscalationAbortedEvent(store, beadID, actor, report.Provider, report.Model, report.ActualPower, at)
	}
	if repeatFailure {
		msg := fmt.Sprintf("provider %s / model %s unreachable on 2+ attempts; check ddx config or provider health", report.Provider, report.Model)
		return store.ParkToProposed(beadID, bead.ParkProviderConnectivityRepeated, func(b *bead.Bead) {
			ensureBeadExtra(b)
			bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
				Reason:          msg,
				Since:           at.UTC().Format(time.RFC3339),
				Source:          "ddx work",
				SuggestedAction: "check provider health or reconfigure endpoints in .ddx/config.yaml",
				Summary:         "provider unreachable on repeated attempts",
			})
		})
	}
	return nil
}

// emitEscalationAbortedEvent records a kind=execution-escalation-aborted event
// when nextFloorFn returns an error (typically ErrLadderExhausted). Best-effort.
func emitEscalationAbortedEvent(store BeadEventAppender, beadID, actor, provider, model string, actualPower int, at time.Time) {
	if store == nil || beadID == "" {
		return
	}
	body, err := json.Marshal(map[string]any{
		"provider":     provider,
		"model":        model,
		"actual_power": actualPower,
		"reason":       fmt.Sprintf("ladder exhausted at power %d", actualPower),
	})
	if err != nil {
		return
	}
	summary := fmt.Sprintf("escalation aborted: ladder exhausted at power %d provider=%s", actualPower, provider)
	if model != "" {
		summary += " model=" + model
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "execution-escalation-aborted",
		Summary:   summary,
		Body:      string(body),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
	})
}

// emitRouteFailureEvent records a kind=route-failure event capturing the
// failed (provider, model) tuple and the surface error. Best-effort.
func emitRouteFailureEvent(store BeadEventAppender, beadID, actor string, report ExecuteBeadReport, at time.Time) {
	if store == nil || beadID == "" {
		return
	}
	body, err := json.Marshal(map[string]any{
		"provider":       report.Provider,
		"model":          report.Model,
		"harness":        report.Harness,
		"actual_power":   report.ActualPower,
		"detail":         report.Detail,
		"error":          report.Error,
		"outcome_reason": FailureModeProviderConnectivity,
	})
	if err != nil {
		return
	}
	summary := "provider=" + report.Provider
	if report.Model != "" {
		summary += " model=" + report.Model
	}
	summary += " connectivity failure"
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "route-failure",
		Summary:   summary,
		Body:      string(body),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
	})
}

func isRoutingInfrastructureReport(report ExecuteBeadReport) bool {
	if report.Status != ExecuteBeadStatusExecutionFailed {
		return false
	}
	combined := strings.TrimSpace(strings.Join([]string{
		report.Detail,
		report.Error,
		report.Stderr,
	}, "\n"))
	if combined == "" {
		return false
	}
	lower := strings.ToLower(combined)
	return containsAny(lower,
		"resolveroute:",
		"no viable routing candidate",
		"no live provider supports",
		"no candidate satisfying local endpoint")
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

func loopContextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	switch err := ctx.Err(); err {
	case context.Canceled, context.DeadlineExceeded:
		return err
	default:
		return nil
	}
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
		Source:    "ddx work",
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
			evidence.OutcomeReviewTransport,
			evidence.OutcomeReviewCostCapExceeded,
			evidence.OutcomeReviewReviewerUnavailable:
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
	case strings.Contains(msg, evidence.OutcomeReviewCostCapExceeded):
		return evidence.OutcomeReviewCostCapExceeded
	case strings.Contains(msg, evidence.OutcomeReviewReviewerUnavailable):
		return evidence.OutcomeReviewReviewerUnavailable
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
var reReviewerIndexField = regexp.MustCompile(`(?m)^reviewer_index=([0-9]+)\s*$`)

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

// CountPriorReviewErrorsForSlot is the two-slot review-group retry counter.
// It scopes retry budget to the candidate result_rev and reviewer slot so a
// malformed/empty/transport failure in one slot does not consume the other
// slot's retry allowance.
func CountPriorReviewErrorsForSlot(store ExecuteBeadLoopStore, beadID, resultRev string, reviewerIndex int) int {
	if resultRev == "" {
		return 0
	}
	events, err := store.Events(beadID)
	if err != nil {
		return 0
	}
	wantIndex := fmt.Sprintf("%d", reviewerIndex)
	n := 0
	for _, ev := range events {
		if ev.Kind != "review-error" {
			continue
		}
		rev := reResultRevField.FindStringSubmatch(ev.Body)
		if rev == nil || rev[1] != resultRev {
			continue
		}
		idx := reReviewerIndexField.FindStringSubmatch(ev.Body)
		if idx == nil || idx[1] != wantIndex {
			continue
		}
		n++
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

func ReviewErrorEventBodyForSlot(class string, attemptCount int, resultRev string, reviewerIndex int, message string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "failure_class=%s\n", class)
	fmt.Fprintf(&b, "attempt_count=%d\n", attemptCount)
	fmt.Fprintf(&b, "result_rev=%s\n", resultRev)
	fmt.Fprintf(&b, "reviewer_index=%d\n", reviewerIndex)
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

// consecutiveLadderExhaustionsKey is the Extra field key that tracks how many
// consecutive times a bead has exhausted the escalation ladder or the per-bead
// budget without producing a success. The auto-recovery hook (sister bead
// ddx-63155d5c) fires when this counter reaches the configured threshold.
const consecutiveLadderExhaustionsKey = "consecutive_ladder_exhaustions"

// isPerBeadBudgetExhaustedReport reports whether the given report signals that
// the per-bead cost budget was exhausted. The marker is set by
// runEscalatingPowerAttempts (cmd package) when the PerBeadCostTracker
// trips. Using the escalation constant avoids string duplication.
func isPerBeadBudgetExhaustedReport(report ExecuteBeadReport) bool {
	return strings.Contains(report.Detail, escalation.PerBeadBudgetExhaustedReason)
}

// incrementConsecutiveLadderExhaustions increments the
// consecutive_ladder_exhaustions counter stored in the bead's Extra map.
// The counter is read by the auto-recovery hook to trigger reframe/decompose
// once the threshold is exceeded. The value is stored as int but may be read
// back as float64 after a JSON round-trip through the bead store.
func incrementConsecutiveLadderExhaustions(args ...any) error {
	ctx := context.Background()
	var store ExecuteBeadLoopStore
	var beadID string
	for _, arg := range args {
		switch v := arg.(type) {
		case context.Context:
			if v != nil {
				ctx = v
			}
		case ExecuteBeadLoopStore:
			store = v
		case string:
			if beadID == "" {
				beadID = v
			}
		}
	}
	if store == nil {
		return fmt.Errorf("incrementConsecutiveLadderExhaustions: store required")
	}
	if beadID == "" {
		return fmt.Errorf("incrementConsecutiveLadderExhaustions: bead id required")
	}
	return store.Update(ctx, beadID, func(b *bead.Bead) {
		ensureBeadExtra(b)
		current := consecutiveLadderExhaustionsValue(b.Extra[consecutiveLadderExhaustionsKey])
		b.Extra[consecutiveLadderExhaustionsKey] = current + 1
	})
}

func consecutiveLadderExhaustionsValue(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	}
	return 0
}

func trimDiagnosticPrefix(message, prefix string) string {
	message = strings.TrimSpace(message)
	prefix = strings.TrimSpace(prefix)
	if message == "" || prefix == "" {
		return message
	}
	for {
		next := strings.TrimSpace(strings.TrimPrefix(message, prefix+":"))
		if next == message {
			return message
		}
		message = next
	}
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
