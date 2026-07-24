package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/DocumentDrivenDX/ddx/internal/gitlock"
	"github.com/DocumentDrivenDX/ddx/internal/trackerpaths"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

const serverUnavailableLogMessage = "server unreachable: holding queue until /api/health returns"
const serverUnavailableStatePhase = "server.unavailable"

var defaultOrphanHarnessProcessScanner = func() orphanHarnessProcessScanner {
	return newOrphanHarnessProcessScanner()
}

// ExecuteBeadLoopRuntime carries the non-serializable plumbing and
// per-invocation runtime intent for an execute-bead loop run. Durable knobs
// (assignee, retry caps, harness/model, powerClass bounds, etc.) live on
// config.ResolvedConfig and are passed via Run's rcfg argument.
type ExecuteBeadLoopRuntime struct {
	Log             io.Writer
	CleanupLog      io.Writer
	CleanupRunner   executionCleanupRunner
	ResourceChecker ExecutionResourceChecker
	// ResourcePressureChecker, when non-nil, is checked before claim on every
	// iteration alongside ResourceChecker. Unlike ResourceChecker, pressure
	// findings never stop the worker — warn and operator_attention
	// severities are surfaced as diagnostics only (ddx-e9182ba1).
	ResourcePressureChecker ResourcePressureChecker
	// LoadPressureSnapshot, when non-nil, supplies the host load sample used
	// immediately before Claim. LoadPressureSleeper is the corresponding
	// context-cancellable delay seam. A nil snapshot disables load pacing so
	// direct Run callers remain deterministic; production entry points wire
	// workerstatus.HostLoadPressureSnapshot explicitly.
	LoadPressureSnapshot func() workerstatus.LoadPressureSnapshot
	LoadPressureSleeper  func(context.Context, time.Duration) error
	// LoadPressureThreshold overrides the default five-minute load per CPU
	// threshold. Zero selects workerstatus.DefaultLoadPressureThreshold.
	LoadPressureThreshold float64
	CleanupInterval       time.Duration
	CleanupTickCh         <-chan time.Time
	EventSink             io.Writer
	ProgressCh            chan<- ProgressEvent
	PreClaimHook          func(ctx context.Context) error
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
	// PreClaimTimeout bounds the pre-claim readiness hooks. Zero means use the
	// binary default so a hanging readiness check cannot park the worker
	// forever.
	PreClaimTimeout time.Duration
	// PreClaimWarnRepeatThreshold is the number of consecutive pre-claim warn
	// fingerprints across distinct bead IDs that trips the operator-attention
	// guard. Zero means use DefaultPreClaimWarnRepeatThreshold.
	PreClaimWarnRepeatThreshold int
	// RouteResolutionTimeout bounds the Fizeau route stage, from Execute dispatch
	// until the first routing_decision event. It does not bound provider execution.
	// Zero means use DefaultRouteResolutionTimeout (60s).
	RouteResolutionTimeout time.Duration
	// routeGuardStartGate is a test-only scheduling seam. When non-nil, the
	// route guard waits for the gate before observing the Execute-relative deadline.
	// Production callers leave it nil.
	routeGuardStartGate <-chan struct{}
	// routeGuardRemainingObserver is a test-only observation seam. When non-nil,
	// the route guard reports the Execute-relative duration immediately after the
	// non-negative clamp. Production callers leave it nil.
	routeGuardRemainingObserver func(time.Duration)
	// ConsecutiveWedgeThreshold is the number of consecutive wedges
	// (route_resolution_timeout / progress_watchdog) on the same bead that
	// trips the consecutive-wedge guard: the worker stops re-claiming the bead
	// and parks it for operator attention. Zero means use
	// DefaultConsecutiveWedgeThreshold (2) (ddx-9714eaac).
	ConsecutiveWedgeThreshold int
	// ClaimSuccessRateWindow and ClaimSuccessRateThreshold control the rolling
	// claim-throughput warning emitted by ddx work. A full window whose claim
	// success rate is at or below the configured threshold triggers a single
	// operator-visible warning until a later successful claim clears the warned
	// state.
	ClaimSuccessRateWindow    int
	ClaimSuccessRateThreshold float64
	// BudgetStop, when non-nil, is checked before selecting the next bead. It
	// lets CLI/server callers surface already-tripped budget state as a typed
	// work-layer StopCondition before any new bead is claimed. The companion
	// report is still appended for operator visibility, but the stop itself is
	// classified through work.StopConditionBudget.
	BudgetStop func() (work.StopDecision, ExecuteBeadReport, bool)
	// GoalGateCheck, when non-nil, evaluates the named gate in a persisted
	// work-drive goal. It is only consulted when the queue is empty and the
	// durable goal's stop predicate is active.
	GoalGateCheck func(ctx context.Context, gate string) (bool, error)
	// BinaryRefreshCheck, when non-nil, is checked before selecting the next
	// bead. A true result means the caller has started a replacement worker
	// with equivalent arguments and this loop should exit before claiming work.
	// Errors are logged and fail open so update-probing never blocks the queue.
	BinaryRefreshCheck func(ctx context.Context) (bool, error)
	// OrphanHarnessProcessScanner, when non-nil, overrides the startup
	// orphan-harness reap pass. Nil uses the package default scanner factory.
	OrphanHarnessProcessScanner orphanHarnessProcessScanner
	// ProjectRootDirtyCheck, when non-nil, is called at the top of each loop
	// iteration before any bead is claimed. A non-empty return value means the
	// canonical project root has uncommitted tracked non-.ddx changes. Startup
	// dirt stops the worker, while watch-mode dirt that appears after this
	// worker already ran an attempt is preserved under a DDx ref and cleaned so
	// autonomous work can continue. Repeated startup exits for the same dirty
	// path-set are tracked durably so a relaunch can escalate into a cooldown
	// instead of spinning immediately.
	ProjectRootDirtyCheck func(projectRoot string) []string
	// ServerHealthProbe, when non-nil, is used while the worker is in
	// server-unavailable backoff. Healthy should return true only after /api/health
	// succeeds and the lightweight smoke path is confirmed usable again.
	ServerHealthProbe func(ctx context.Context) (bool, error)
	// TrackerSyncEnabled controls claim-boundary git sync. When enabled the
	// worker fetches and merges origin/main before selecting a candidate, then
	// publishes tracker commits immediately after claim and close boundaries.
	TrackerSyncEnabled bool
	// ServerFailureWindow bounds the distinct-bead lookback used to detect
	// repeated no_viable_provider outcomes that should be treated as a server-level
	// outage rather than a bead-level failure. Zero means use the documented
	// 10-minute default.
	ServerFailureWindow time.Duration
	// ServerFailureThreshold is the number of distinct beads that must report
	// no_viable_provider inside ServerFailureWindow before the worker enters the
	// server-unavailable probe-and-resume state. Zero means use the documented
	// default of 3.
	ServerFailureThreshold int
	// ServerHealthProbeInterval is the cadence used while the worker is in the
	// server-unavailable state. Zero means use the documented 30s default.
	ServerHealthProbeInterval time.Duration
	// Mode and IdleInterval are the runtime loop intent. Once and
	// PollInterval remain for older tests/callers but production entry points
	// should set Mode and IdleInterval directly.
	Mode         executeloop.Mode
	IdleInterval time.Duration
	Once         bool
	PollInterval time.Duration
	NoReview     bool
	// PostMergeReview enables the legacy post-land/pre-close review state
	// machine for callers that explicitly opt into that manual flow. The
	// default production close path remains owned by candidate-cycle review.
	PostMergeReview        bool
	LabelFilter            string
	IgnoreCooldown         bool
	CooldownOverrideReason string
	SessionID              string
	WorkerID               string
	ProjectRoot            string
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

	// FinalizeDurableAudit, when non-nil, accumulates one finalized attempt
	// after the loop has finished writing the attempt's durable tracker state.
	// FlushDurableAudit is called exactly once at every runIteration epilogue.
	// Splitting the two keeps Git/index work out of all tracker mutation paths.
	FinalizeDurableAudit func(report ExecuteBeadReport) error
	FlushDurableAudit    func() error
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

// DefaultRouteResolutionTimeout bounds only Fizeau's opaque route stage: from
// Execute dispatch until the service emits routing_decision. The timer is
// permanently disarmed at that event and never bounds provider execution.
const DefaultRouteResolutionTimeout = 60 * time.Second

// effectiveRouteResolutionTimeout returns the configured route-resolution
// timeout, falling back to DefaultRouteResolutionTimeout when unset.
func (r ExecuteBeadLoopRuntime) effectiveRouteResolutionTimeout() time.Duration {
	if r.RouteResolutionTimeout > 0 {
		return r.RouteResolutionTimeout
	}
	return DefaultRouteResolutionTimeout
}

// DefaultConsecutiveWedgeThreshold is the number of consecutive wedges on the
// same bead that trips the consecutive-wedge guard when
// ExecuteBeadLoopRuntime.ConsecutiveWedgeThreshold is unset. A single transient
// wedge (count 1) stays below the default so the bead remains re-claimable; two
// consecutive wedges sideline it for operator attention (ddx-9714eaac).
const DefaultConsecutiveWedgeThreshold = 2

// effectiveConsecutiveWedgeThreshold returns the configured consecutive-wedge
// threshold, falling back to DefaultConsecutiveWedgeThreshold when unset.
func (r ExecuteBeadLoopRuntime) effectiveConsecutiveWedgeThreshold() int {
	if r.ConsecutiveWedgeThreshold > 0 {
		return r.ConsecutiveWedgeThreshold
	}
	return DefaultConsecutiveWedgeThreshold
}

// DefaultClaimSuccessRateWindow is the number of recent claim attempts used to
// compute the rolling claim-success rate warning.
const DefaultClaimSuccessRateWindow = 10

// effectiveClaimSuccessRateWindow returns the configured claim-success window,
// falling back to DefaultClaimSuccessRateWindow when unset.
func (r ExecuteBeadLoopRuntime) effectiveClaimSuccessRateWindow() int {
	if r.ClaimSuccessRateWindow > 0 {
		return r.ClaimSuccessRateWindow
	}
	return DefaultClaimSuccessRateWindow
}

// DefaultClaimSuccessRateThreshold is the minimum rolling claim-success rate
// before the loop emits a warning. Zero means only a full window of
// non-successes trips the signal.
const DefaultClaimSuccessRateThreshold = 0.0

// effectiveClaimSuccessRateThreshold returns the configured claim-success
// threshold, falling back to DefaultClaimSuccessRateThreshold when unset.
func (r ExecuteBeadLoopRuntime) effectiveClaimSuccessRateThreshold() float64 {
	if r.ClaimSuccessRateThreshold >= 0 {
		return r.ClaimSuccessRateThreshold
	}
	return DefaultClaimSuccessRateThreshold
}

// DefaultServerFailureWindow is the lookback window used to classify repeated
// no_viable_provider outcomes as a worker-level server outage.
const DefaultServerFailureWindow = 10 * time.Minute

// DefaultServerFailureThreshold is the number of distinct beads that must hit
// no_viable_provider inside the lookback window before the worker enters
// server-unavailable mode.
const DefaultServerFailureThreshold = 3

// DefaultServerHealthProbeInterval is the cadence used while the worker is in
// server-unavailable mode.
const DefaultServerHealthProbeInterval = 30 * time.Second

func (r ExecuteBeadLoopRuntime) effectiveServerFailureWindow() time.Duration {
	if r.ServerFailureWindow > 0 {
		return r.ServerFailureWindow
	}
	return DefaultServerFailureWindow
}

func (r ExecuteBeadLoopRuntime) effectiveServerFailureThreshold() int {
	if r.ServerFailureThreshold > 0 {
		return r.ServerFailureThreshold
	}
	return DefaultServerFailureThreshold
}

func (r ExecuteBeadLoopRuntime) effectiveServerHealthProbeInterval() time.Duration {
	if r.ServerHealthProbeInterval > 0 {
		return r.ServerHealthProbeInterval
	}
	return DefaultServerHealthProbeInterval
}

// DefaultPreClaimWarnRepeatThreshold is the number of consecutive identical
// pre-claim warn fingerprints across distinct bead IDs required to trip the
// operator-attention guard.
const DefaultPreClaimWarnRepeatThreshold = 5

const (
	preClaimIntakeProjectRootMutationRejectedCode   = "project_root_mutation_rejected"
	preClaimIntakeProjectRootMutationRejectedDetail = "project root mutation rejected"
)

// effectivePreClaimWarnRepeatThreshold returns the configured pre-claim warn
// repeat threshold, falling back to DefaultPreClaimWarnRepeatThreshold when
// unset.
func (r ExecuteBeadLoopRuntime) effectivePreClaimWarnRepeatThreshold() int {
	if r.PreClaimWarnRepeatThreshold > 0 {
		return r.PreClaimWarnRepeatThreshold
	}
	return DefaultPreClaimWarnRepeatThreshold
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
// open with transient smart-retry metadata, but a watch worker moves on instead
// of repeating the same empty-diff attempt every idle cycle.
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
	if err := VerifyCandidateHasNoExecutionEvidence(projectRoot, res.BaseRev, res.ResultRev); err != nil {
		return nil, nil, fmt.Errorf("submit-with-pre-merge-checks: %w", err)
	}
	evidenceDir := filepath.Join(projectRoot, res.ExecutionDir)
	outcome, err := RunPreMergeChecks(ctx, projectRoot, b, res.BaseRev, res.ResultRev, evidenceDir)
	if err != nil {
		// Treat checks_bypass / loader errors as a hard preserve: the worker
		// did its job; the operator misconfigured the gate. Better to park
		// the result under an iteration ref than to silently land it.
		ref := PreserveRef(res.BeadID, res.BaseRev)
		if upErr := (&RealGitOps{}).UpdateRef(projectRoot, ref, res.ResultRev); upErr != nil {
			return nil, nil, fmt.Errorf("preserving result after checks setup error: %w (original error: %v)", upErr, err)
		}
		return &LandResult{
			Status:      "preserved",
			PreserveRef: ref,
			Reason:      "pre-merge checks setup error: " + err.Error(),
		}, nil, nil
	}
	if outcome.Blocked {
		land, perr := PreserveAfterPreMergeChecks(projectRoot, res, outcome, &RealGitOps{})
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
	var submitted *LandResult
	cfg, cfgErr := config.LoadWithWorkingDir(projectRoot)
	_, landErr := LandBeadResult(projectRoot, res, &RealGitOps{}, BeadLandingOptions{
		LandingAdvancer: func(res *ExecuteBeadResult) (*LandResult, error) {
			req := BuildLandRequestFromResult(projectRoot, res)
			if cfgErr == nil && cfg != nil && cfg.Git != nil && len(cfg.Git.PostLandCommand) > 0 {
				req.PostLandCommand = append([]string(nil), cfg.Git.PostLandCommand...)
			}
			land, err := submit(req)
			if err == nil {
				submitted = land
			}
			return land, err
		},
	})
	return submitted, outcome, landErr
}

type postAttemptDecompositionDecision struct {
	ChildIDs          []string
	ExecutionDecision string
	BackEdgeDetected  bool
}

// handlePostAttemptDecomposition runs the orchestrator-level splitter when a
// no_changes attempt signals orchestrator_action: decompose. It checks the
// queue-level max_decomposition_depth (not the implementation prompt cap),
// validates the AC map for completeness, and either creates children+deps or
// parks the parent for operator review if the split is lossy, depth-capped, or
// introduces a parent back-edge. The bead must already be unclaimed before
// this is called.
func (w *ExecuteBeadWorker) handlePostAttemptDecomposition(ctx context.Context, candidate *bead.Bead, runtime ExecuteBeadLoopRuntime, assignee string, rcfg config.ResolvedConfig, at time.Time) postAttemptDecompositionDecision {
	decision := postAttemptDecompositionDecision{ExecutionDecision: "proposed"}
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
		return decision
	}

	decomp, err := runtime.PostAttemptDecompositionHook(ctx, candidate.ID)
	if err != nil {
		parkOperator(fmt.Sprintf("decomposition hook error: %s", err.Error()))
		return decision
	}
	if decomp == nil {
		parkOperator("decomposition hook returned no split")
		return decision
	}
	lossyOrEmpty := isDecompositionLossy(decomp.ACMap) || (len(decomp.ACMap) == 0 && strings.TrimSpace(candidate.Acceptance) != "")
	if lossyOrEmpty {
		parkOperator("decomposition AC map is incomplete; operator must produce a lossless split")
		return decision
	}

	childIDs, decompErr := applyPreClaimDecomposition(ctx, w.Store, candidate, decomp, assignee, at)
	decision.ChildIDs = append([]string(nil), childIDs...)
	if decompErr != nil {
		parkOperator(fmt.Sprintf("decomposition apply error: %s", decompErr.Error()))
		return decision
	}
	decision.ExecutionDecision = "execution_ineligible"
	if runtime.Log != nil {
		_, _ = fmt.Fprintf(runtime.Log, "post-attempt decomposition: bead %s split into %s\n", candidate.ID, strings.Join(childIDs, ", "))
	}
	emit("post_attempt_decomposition.applied", map[string]any{
		"bead_id":   candidate.ID,
		"child_ids": childIDs,
	})

	backEdgeChildIDs, err := detectPostAttemptDecompositionBackEdge(ctx, w.Store, candidate.ID, childIDs)
	if err != nil {
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "post-attempt decomposition back-edge check failed: %v\n", err)
		}
		return decision
	}
	if len(backEdgeChildIDs) == 0 {
		return decision
	}
	if runtime.Log != nil {
		_, _ = fmt.Fprintf(runtime.Log, "post-attempt decomposition back-edge: bead %s children %s still depend on the parent\n", candidate.ID, strings.Join(backEdgeChildIDs, ", "))
	}
	emit("post_attempt_decomposition.parent_back_edge", map[string]any{
		"bead_id":   candidate.ID,
		"child_ids": backEdgeChildIDs,
	})
	if parkErr := parkParentBackEdgeForOperator(w.Store, candidate.ID, assignee, backEdgeChildIDs, at); parkErr != nil && runtime.Log != nil {
		_, _ = fmt.Fprintf(runtime.Log, "post-attempt decomposition parent back-edge park error: %v\n", parkErr)
	}
	decision.ExecutionDecision = "proposed"
	decision.BackEdgeDetected = true
	return decision
}

func detectPostAttemptDecompositionBackEdge(ctx context.Context, store ExecuteBeadLoopStore, parentID string, childIDs []string) ([]string, error) {
	if store == nil || parentID == "" || len(childIDs) == 0 {
		return nil, nil
	}
	var backEdgeChildIDs []string
	for _, childID := range childIDs {
		child, err := store.Get(ctx, childID)
		if err != nil {
			return nil, err
		}
		if child == nil || child.Parent != parentID {
			continue
		}
		if child.HasDep(parentID) {
			backEdgeChildIDs = append(backEdgeChildIDs, childID)
		}
	}
	return backEdgeChildIDs, nil
}

func parkParentBackEdgeForOperator(store ExecuteBeadLoopStore, beadID, actor string, childIDs []string, at time.Time) error {
	diagnosis := fmt.Sprintf(
		"fresh child beads still depend on the attempted parent bead %s; re-claiming the parent would keep the child split blocked",
		beadID,
	)
	if err := parkToProposedWithOperatorMeta(store, beadID, bead.ParkNoChangesOperatorRequired, ParkToProposedOpts{
		Reason:          "parent_back_edge",
		Summary:         "child split still depends on the parent bead",
		SuggestedAction: "remove the parent back-edge from the child split and retry once the children are independent",
		Since:           at,
		CleanupLabels:   false,
	}); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"reason":    "parent_back_edge",
		"bead_id":   beadID,
		"child_ids": append([]string(nil), childIDs...),
		"diagnosis": diagnosis,
	})
	return store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "operator_attention",
		Summary:   "parent_back_edge",
		Body:      string(body),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
	})
}

func markExecutionIneligible(store ExecuteBeadLoopStore, beadID string) error {
	if store == nil || beadID == "" {
		return nil
	}
	return store.Update(context.Background(), beadID, func(b *bead.Bead) {
		ensureBeadExtra(b)
		b.Extra[bead.ExtraExecutionElig] = false
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
	// stronger reviewer without selecting a concrete route.
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
	// EvidenceRev is retained for legacy execution records. Current attempts
	// keep local evidence out of Git and leave this empty.
	EvidenceRev string `json:"evidence_rev,omitempty"`
	// ProjectRoot is the worktree root ddx try/work operated on for this report.
	ProjectRoot string `json:"project_root,omitempty"`
	PreserveRef string `json:"preserve_ref,omitempty"`
	// NoEvidencePaths names dirty paths preserved when a no-evidence attempt
	// exited without committing work or writing no_changes_rationale.txt.
	NoEvidencePaths []string `json:"no_evidence_paths,omitempty"`
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
	// ReviewSkipReason carries the durable review:skip-reason:* label when a
	// success path is allowed to close without running a reviewer.
	ReviewSkipReason string `json:"review_skip_reason,omitempty"`
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
	RequestedProfile    string `json:"requested_profile,omitempty"`
	RoutingIntentSource string `json:"routing_intent_source,omitempty"`
	EstimatedDifficulty string `json:"estimated_difficulty,omitempty"`
	InferredPowerClass  string `json:"inferred_power_class,omitempty"`
	// Current requests use only the opaque Fizeau policy and numeric power
	// bounds below. The profile/power-class fields above are retained solely
	// for reading historical execution records.
	RequestedPolicy         string `json:"requested_policy,omitempty"`
	InferredMinPower        int    `json:"inferred_min_power"`
	InferredMinPowerPresent bool   `json:"inferred_min_power_present,omitempty"`
	RequestedMinPower       int    `json:"requested_min_power"`
	RequestedMaxPower       int    `json:"requested_max_power"`
	RoutingIntentNote       string `json:"routing_intent_note,omitempty"`
	ResolvedPowerClass      string `json:"resolved_power_class,omitempty"`
	EscalationCount         int    `json:"escalation_count,omitempty"`
	FinalPowerClass         string `json:"final_power_class,omitempty"`
	// DecompositionRecommendation carries the structured list of recommended
	// sub-bead titles when Status == declined_needs_decomposition. The loop
	// records these on the bead as a `decomposition-recommendation` event so
	// operators (or helix-evolve) can split the bead without re-deriving the
	// recommendation.
	DecompositionRecommendation []string `json:"decomposition_recommendation,omitempty"`
	// DecompositionRationale is a free-form explanation accompanying
	// DecompositionRecommendation. Optional.
	DecompositionRationale string `json:"decomposition_rationale,omitempty"`
	// DecomposedChildIDs carries child bead IDs created by a decomposition
	// outcome so result/event audit can explain why the parent is not retried.
	DecomposedChildIDs []string `json:"decomposed_child_ids,omitempty"`
	// ExecutionDecision records the queue-level disposition selected after an
	// attempt, such as execution_ineligible, proposed, or dependency_waiting.
	ExecutionDecision string `json:"execution_decision,omitempty"`
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
	// ProviderFailureEvidence is the durable route-health record attached when
	// the attempt failed for a typed provider reason (ddx-3b721804): requested
	// constraints, resolved route if any, typed failure, retryability, and the
	// fallback decision.
	ProviderFailureEvidence *ProviderFailureEvidence `json:"provider_failure_evidence,omitempty"`
	// ResourceExhausted carries the execution-root preflight result when the
	// attempt stopped before the agent could safely continue.
	ResourceExhausted any `json:"resource_exhausted,omitempty"`
	// ResourceExhaustionDiagnosis classifies a resource_exhausted report
	// beyond "some root became unhealthy". Set to ResourceExhaustionDiagnosisFD
	// ("fd_exhaustion") when the preflight failure was the process/host
	// open-file-descriptor limit (EMFILE/ENFILE); empty for ordinary
	// byte/inode or unwritable-root failures.
	ResourceExhaustionDiagnosis string `json:"resource_exhaustion_diagnosis,omitempty"`
	// ResourceExhaustionRestartable is true when ResourceExhaustionDiagnosis
	// is worker-local: a fresh worker process (e.g. after a supervisor
	// restart) is expected to clear it, unlike root-storage exhaustion which
	// persists across worker restarts.
	ResourceExhaustionRestartable bool `json:"resource_exhaustion_restartable,omitempty"`
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
	Get(ctx context.Context, id string) (*bead.Bead, error)
	Create(ctx context.Context, b *bead.Bead) error
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
	Update(ctx context.Context, id string, mutate func(*bead.Bead)) error
	UpdateWithLifecycleStatus(id string, status string, opts bead.LifecycleTransitionOptions, mutate func(*bead.Bead) error) error
	ParkToProposed(id string, reason bead.ParkReason, mutate func(*bead.Bead)) error
	ParkToProposedWithIntakeEvent(id, actor, outcome, reason, detail string, body map[string]any, at time.Time, mutate func(*bead.Bead)) error
}

// releaseWorkerClaim atomically releases a worker-owned claim when the store
// supports the atomic Release path. Older test doubles fall back to Unclaim so
// they keep compiling, but production stores use the atomic release.
func releaseWorkerClaim(store ExecuteBeadLoopStore, beadID, assignee string) error {
	if store == nil || beadID == "" {
		return nil
	}
	if releaser, ok := store.(leaseReleaser); ok {
		return releaser.Release(beadID, assignee, bead.StatusOpen)
	}
	return store.Unclaim(beadID)
}

// readyDiagnoser is the optional interface the work loop uses to explain an
// empty execution queue. The tracker backend satisfies it via
// ReadyExecutionBreakdown.
type readyDiagnoser interface {
	ReadyExecutionBreakdown() (bead.ReadyExecutionBreakdown, error)
}

// epicCloser is the optional interface the work loop uses for idle-path closure
// cascade: closing epics whose children have all reached a terminal state.
// The tracker backend satisfies both methods.
type epicCloser interface {
	EpicClosureCandidates(ctx context.Context) ([]bead.Bead, error)
	Close(ctx context.Context, id string) error
}

type proposedOperatorAttentionStore interface {
	ProposedOperatorAttention() ([]bead.Bead, error)
}

type claimWithOptionsStore interface {
	ClaimWithOptions(id, assignee, session, worktree string) error
}

type cooldownOverrideReporter interface {
	CooldownOverrideInfo(beadID string) (string, bool)
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
	RetryAfter  string   `json:"retry_after,omitempty"`
	Message     string   `json:"message,omitempty"`
}

const (
	// DefaultDirtyRootEscalationThreshold is the number of consecutive dirty-root
	// exits with the same dirty-path fingerprint before the loop escalates the
	// stop into a cooldown-bearing operator-attention response.
	DefaultDirtyRootEscalationThreshold = 3

	// DirtyRootEscalationCooldown is the backoff window used once the repeated
	// dirty-root guard trips. It is intentionally short enough to stop instant
	// restart spin but long enough for an operator to clean or commit the root.
	DirtyRootEscalationCooldown = 5 * time.Minute

	dirtyRootGuardStateFileName = "dirty-root-guard.json"
)

type dirtyRootGuardState struct {
	Fingerprint     string    `json:"fingerprint,omitempty"`
	DirtyPaths      []string  `json:"dirty_paths,omitempty"`
	Count           int       `json:"count,omitempty"`
	FirstObservedAt time.Time `json:"first_observed_at,omitempty"`
	LastObservedAt  time.Time `json:"last_observed_at,omitempty"`
	RetryAfter      time.Time `json:"retry_after,omitempty"`
}

type ExecuteBeadLoopResult struct {
	Attempts          int                    `json:"attempts"`
	Successes         int                    `json:"successes"`
	Failures          int                    `json:"failures"`
	StopCondition     string                 `json:"stop_condition,omitempty"`
	ExitReason        string                 `json:"exit_reason,omitempty"`
	OperatorAttention *OperatorAttentionStop `json:"operator_attention,omitempty"`
	NoReadyWork       bool                   `json:"no_ready_work,omitempty"`
	Goal              *WorkGoal              `json:"goal,omitempty"`
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
	// preDispatchDirtyPreserver replaces the watch-mode preserve-and-clean path
	// in tests so checkpoint dirt fallback behavior can be exercised without a
	// real git repo.
	preDispatchDirtyPreserver func(projectRoot string, dirtyPaths []string) (*PreDispatchDirtyPreservation, error)
	// preDispatchGitRepairer replaces the project-root git config repair
	// preflight in tests so failed/unresolved repairs can be exercised without
	// corrupting a real repository in unsupported ways.
	preDispatchGitRepairer preDispatchGitRepairerFunc

	// transientCandidateSkips is an in-memory per-Run filter for queue entries
	// that were returned from an older snapshot but rejected by a fresh
	// pre-claim/pre-dispatch store read. It is reset when the loop goes idle.
	transientCandidateSkips map[string]string
}

type forcedCooldownMetadata struct {
	Active     bool
	RetryAfter string
	BaseRev    string
	LastStatus string
	LastDetail string
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

func executeLoopEventSource(runtime ExecuteBeadLoopRuntime) string {
	if runtime.TargetBeadID != "" {
		return "ddx try"
	}
	return "ddx work"
}

func goalWaitSleepInterval(runtime ExecuteBeadLoopRuntime, idleInterval time.Duration) time.Duration {
	sleep := idleInterval
	if sleep <= 0 {
		sleep = runtime.PollInterval
	}
	if sleep <= 0 {
		sleep = 30 * time.Second
	}
	return sleep
}

func evaluatePersistedWorkGoal(ctx context.Context, goal *WorkGoal, runtime ExecuteBeadLoopRuntime) (bool, string) {
	if goal == nil || !goal.IsActive() {
		return true, ""
	}
	switch goal.StopPredicate.Kind {
	case WorkGoalStopPredicateQueueEmpty:
		return true, ""
	case WorkGoalStopPredicateQueueEmptyAndGateGreen:
		gate := strings.TrimSpace(goal.StopPredicate.Gate)
		if gate == "" {
			return false, "goal stop predicate requires a gate name"
		}
		if runtime.GoalGateCheck == nil {
			return false, fmt.Sprintf("goal gate %q check unavailable", gate)
		}
		green, err := runtime.GoalGateCheck(ctx, gate)
		if err != nil {
			return false, fmt.Sprintf("goal gate %q check failed: %v", gate, err)
		}
		if !green {
			return false, fmt.Sprintf("goal gate %q is not green yet", gate)
		}
		return true, ""
	default:
		return false, fmt.Sprintf("unsupported goal stop predicate %q", goal.StopPredicate.Kind)
	}
}

func forcedCooldownSnapshot(b bead.Bead, retryAfter string, overridden bool) forcedCooldownMetadata {
	if !overridden {
		return forcedCooldownMetadata{}
	}
	if retryAfter == "" && b.Extra != nil {
		retryAfter, _ = b.Extra[bead.ExtraRetryAfter].(string)
	}
	baseRev := ""
	lastStatus := ""
	lastDetail := ""
	if b.Extra != nil {
		baseRev, _ = b.Extra[bead.ExtraCooldownBaseRev].(string)
		lastStatus, _ = b.Extra[bead.ExtraLastStatus].(string)
		lastDetail, _ = b.Extra[bead.ExtraLastDetail].(string)
	}
	return forcedCooldownMetadata{
		Active:     retryAfter != "",
		RetryAfter: retryAfter,
		BaseRev:    baseRev,
		LastStatus: lastStatus,
		LastDetail: lastDetail,
	}
}

func restoreForcedCooldownMetadata(ctx context.Context, store ExecuteBeadLoopStore, beadID string, meta forcedCooldownMetadata) error {
	if !meta.Active {
		return nil
	}
	return store.Update(ctx, beadID, func(b *bead.Bead) {
		ensureBeadExtra(b)
		setOrDeleteBeadExtra(b.Extra, bead.ExtraRetryAfter, meta.RetryAfter)
		setOrDeleteBeadExtra(b.Extra, bead.ExtraCooldownBaseRev, meta.BaseRev)
		setOrDeleteBeadExtra(b.Extra, bead.ExtraLastStatus, meta.LastStatus)
		setOrDeleteBeadExtra(b.Extra, bead.ExtraLastDetail, meta.LastDetail)
	})
}

func appendForceClaimEvent(store BeadEventAppender, beadID, assignee, source, reason, retryAfter string, createdAt time.Time) {
	if store == nil || beadID == "" {
		return
	}
	body, err := json.Marshal(map[string]any{
		"reason":      strings.TrimSpace(reason),
		"retry_after": retryAfter,
		"forced_at":   createdAt.Format(time.RFC3339),
	})
	if err != nil {
		body = []byte(fmt.Sprintf("reason=%s\nretry_after=%s\nforced_at=%s",
			strings.TrimSpace(reason),
			retryAfter,
			createdAt.Format(time.RFC3339),
		))
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "force-claim",
		Summary:   "retry cooldown overridden for targeted execution",
		Body:      string(body),
		Actor:     assignee,
		Source:    source,
		CreatedAt: createdAt,
	})
}

func danglingSuccessRecoveryReport(beadID string, recovery *DanglingSuccessRecovery) ExecuteBeadReport {
	report := ExecuteBeadReport{BeadID: beadID}
	if recovery == nil {
		report.Status = ExecuteBeadStatusExecutionFailed
		report.Detail = "dangling-success recovery returned no terminal outcome"
		report.OutcomeReason = "operator_required"
		return report
	}
	report.AttemptID = recovery.AttemptID
	report.SessionID = recovery.SessionID
	report.BaseRev = recovery.BaseRev
	report.ResultRev = recovery.ResultRev
	report.PreserveRef = recovery.PreserveRef

	switch recovery.Outcome {
	case danglingSuccessOutcomeClosed:
		report.Status = ExecuteBeadStatusSuccess
		report.Detail = "dangling-success recovery: bead closed idempotently (result_rev already merged)"
	case danglingSuccessOutcomePreserved:
		report.Status = ExecuteBeadStatusPreservedNeedsReview
		report.Detail = "dangling-success: preserved successful result for operator landing"
	case danglingSuccessOutcomeOperatorRequired:
		report.Status = ExecuteBeadStatusExecutionFailed
		report.OutcomeReason = "operator_required"
		if recovery.FailureReason != "" {
			report.Detail = "dangling-success: successful result could not be landed automatically: " + recovery.FailureReason
		} else {
			report.Detail = "dangling-success: successful result could not be landed automatically"
		}
	default:
		report.Status = ExecuteBeadStatusExecutionFailed
		report.OutcomeReason = "operator_required"
		report.Detail = "dangling-success: unrecognized recovery outcome"
	}
	return report
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

func dirtyRootGuardStatePath(projectRoot string) string {
	return ddxroot.JoinProject(projectRoot, dirtyRootGuardStateFileName)
}

func existingDirtyRootGuardStatePath(projectRoot string) (string, bool) {
	return ddxroot.ExistingJoinProject(context.Background(), projectRoot, dirtyRootGuardStateFileName)
}

func readDirtyRootGuardState(projectRoot string) (*dirtyRootGuardState, error) {
	path, ok := existingDirtyRootGuardStatePath(projectRoot)
	if !ok {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var state dirtyRootGuardState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func writeDirtyRootGuardState(projectRoot string, state dirtyRootGuardState) error {
	path := dirtyRootGuardStatePath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), "dirty-root-guard-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func clearDirtyRootGuardState(projectRoot string) error {
	path, ok := existingDirtyRootGuardStatePath(projectRoot)
	if !ok {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func normalizeDirtyRootPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	uniq := make(map[string]struct{}, len(paths))
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := uniq[path]; ok {
			continue
		}
		uniq[path] = struct{}{}
		normalized = append(normalized, path)
	}
	if len(normalized) == 0 {
		return nil
	}
	sort.Strings(normalized)
	return normalized
}

func dirtyRootPathFingerprint(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return strings.Join(paths, "\x00")
}

func updateDirtyRootGuardState(projectRoot string, dirtyPaths []string, at time.Time, threshold int, cooldown time.Duration) (*dirtyRootGuardState, bool, error) {
	if projectRoot == "" {
		return nil, false, nil
	}
	if threshold <= 0 {
		threshold = DefaultDirtyRootEscalationThreshold
	}
	if cooldown <= 0 {
		cooldown = DirtyRootEscalationCooldown
	}
	paths := normalizeDirtyRootPaths(dirtyPaths)
	if len(paths) == 0 {
		if err := clearDirtyRootGuardState(projectRoot); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}
	fingerprint := dirtyRootPathFingerprint(paths)
	state, err := readDirtyRootGuardState(projectRoot)
	if err != nil {
		return nil, false, err
	}
	next := dirtyRootGuardState{
		Fingerprint:     fingerprint,
		DirtyPaths:      append([]string(nil), paths...),
		Count:           1,
		FirstObservedAt: at.UTC(),
		LastObservedAt:  at.UTC(),
	}
	if state != nil && state.Fingerprint == fingerprint {
		next.FirstObservedAt = state.FirstObservedAt
		if next.FirstObservedAt.IsZero() {
			next.FirstObservedAt = at.UTC()
		}
		next.Count = state.Count
		next.RetryAfter = state.RetryAfter
		if state.RetryAfter.After(at) {
			// The cooldown is still active. Preserve the existing marker so the
			// relaunch sees the same backoff instead of tightening the spin loop.
			next.Count = state.Count
			next.RetryAfter = state.RetryAfter
			if err := writeDirtyRootGuardState(projectRoot, next); err != nil {
				return nil, false, err
			}
			return &next, true, nil
		}
		next.Count = state.Count + 1
		if next.Count >= threshold {
			next.RetryAfter = at.UTC().Add(cooldown)
		} else {
			next.RetryAfter = time.Time{}
		}
	} else {
		next.Count = 1
		next.RetryAfter = time.Time{}
	}
	if next.Count >= threshold && next.RetryAfter.IsZero() {
		next.RetryAfter = at.UTC().Add(cooldown)
	}
	if err := writeDirtyRootGuardState(projectRoot, next); err != nil {
		return nil, false, err
	}
	return &next, next.Count >= threshold, nil
}

// isTransientGitContention reports git index/ref contention errors that are
// transient under concurrent workers and must be retried rather than treated
// as a worker-stopping failure (ddx-23ac2796 and its sibling variants). The
// gitlock package owns the shared output/error classifier for the concrete git
// forms; this wrapper adds the DDx tracker-lock timeout sentinel and the
// context-deadline cases used by the loop's durable-audit stop logic.
func isTransientGitContention(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, TrackerLockTimeoutErr) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return gitlock.IsTransientGitContention("", err)
}

type executeBeadLoopState struct {
	now                         func() time.Time
	assignee                    string
	preClaimTimeout             time.Duration
	noProgressCooldown          time.Duration
	heartbeatInterval           time.Duration
	harness                     string
	provider                    string
	model                       string
	profile                     string
	loopMode                    executeloop.Mode
	idleInterval                time.Duration
	result                      *ExecuteBeadLoopResult
	resultsResetIdx             int
	transientCandidateSkips     map[string]string
	pausedInfraUntil            time.Time
	complexityGuard             *work.ComplexityGuard
	preclaimGuard               *work.PreClaimGuard
	workLog                     WorkLogRenderer
	wasIdle                     bool
	lastIdleQueueSignature      string
	preClaimIdleDetail          string
	preClaimIdleStreak          int
	preClaimIdleFirstAt         time.Time
	preClaimIdleEscalated       bool
	preClaimGitCorruptDetail    string
	preClaimGitCorruptStreak    int
	preClaimGitCorruptFirstAt   time.Time
	preClaimGitCorruptEscalated bool
	liveness                    *work.SidecarLivenessReporter
	serverOutage                *serverOutageTracker
	serverUnavailableLogged     bool
	sawParentBackEdge           bool
	claimSuccessRateWindowSize  int
	claimSuccessRateThreshold   float64
	claimSuccessRateWindow      []bool
	claimSuccessRateSuccesses   int
	claimSuccessRateWarned      bool
	preClaimWarnThreshold       int
	preClaimWarnState           preClaimWarnRepeatState
	exitReason                  string
	cleanupLog                  io.Writer
	attemptStarted              *bool
}

type executeBeadIterationOutcome struct {
	Stop     bool
	Continue bool
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
	transientCandidateSkips := map[string]string{}
	w.transientCandidateSkips = transientCandidateSkips
	defer func() {
		w.transientCandidateSkips = nil
	}()
	// pausedInfraUntil is set when a no_viable_provider outcome is recorded.
	// The worker sleeps until this time (in Watch mode) before re-evaluating
	// the queue, leaving all beads immediately reclaimable. Zero means no pause.
	var pausedInfraUntil time.Time
	complexityGuard := work.NewComplexityGuard(w.ComplexityGate, runtime.Log)
	preclaimGuard := work.NewPreClaimGuard(runtime.PreClaimHook, w.Store, runtime.Log, now, 30*time.Second, preClaimTimeout)
	workLog := NewWorkLogRenderer(WorkLogRendererOptions{Now: now})
	wasIdle := false
	lastIdleQueueSignature := ""

	// Pre-claim idle escalation state (ddx-df77e668 AC #3). When the worker
	// idles on the same pre-claim blocker for preClaimIdleEscalationThreshold
	// consecutive cycles, it emits a non-terminal operator-attention event so
	// the stall is visible instead of looping silently. The streak resets when
	// the blocker detail changes or the worker stops idling on it.
	preClaimIdleDetail := ""
	preClaimIdleStreak := 0
	preClaimIdleFirstAt := time.Time{}
	preClaimIdleEscalated := false

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
		workerPID := os.Getpid()
		liveness.SetChildProbe(func(route, harness, phase string) []workerstatus.ProviderChild {
			return scanProviderChildrenForStatus(context.Background(), workerPID, route, harness, phase, time.Now().UTC())
		})
	}
	serverOutage := newServerOutageTracker(
		runtime.effectiveServerFailureWindow(),
		runtime.effectiveServerFailureThreshold(),
		runtime.effectiveServerHealthProbeInterval(),
	)
	serverUnavailableLogged := false

	emit := func(eventType string, data map[string]any) {
		writeLoopEvent(runtime.EventSink, runtime.SessionID, eventType, data, now().UTC())
	}
	sawParentBackEdge := false

	// Wire the tracker-lock metrics sink so every acquire+release cycle is
	// visible in the loop event stream and the terminal log. SetTrackerLockMetricsSink
	// is safe for concurrent workers (mutex-protected); restored on Run exit.
	workerLog := runtime.Log
	prevTrackerSink := SetTrackerLockMetricsSink(func(s TrackerLockSample) {
		emit("loop.tracker_lock", map[string]any{
			"section": s.Section,
			"wait_ms": s.Wait.Milliseconds(),
			"hold_ms": s.Hold.Milliseconds(),
			"retries": s.Retries,
		})
		if workerLog != nil {
			_, _ = fmt.Fprintf(workerLog, "tracker_lock section=%s wait=%s hold=%s retries=%d\n",
				s.Section, s.Wait.Round(time.Millisecond), s.Hold.Round(time.Millisecond), s.Retries)
		}
	})
	defer func() { SetTrackerLockMetricsSink(prevTrackerSink) }()

	claimSuccessRateWindowSize := runtime.effectiveClaimSuccessRateWindow()
	claimSuccessRateThreshold := runtime.effectiveClaimSuccessRateThreshold()
	claimSuccessRateWindow := make([]bool, 0, claimSuccessRateWindowSize)
	claimSuccessRateSuccesses := 0
	claimSuccessRateWarned := false
	preClaimWarnThreshold := runtime.effectivePreClaimWarnRepeatThreshold()
	preClaimWarnState := preClaimWarnRepeatState{}
	// exitReason is populated as the loop exits to surface a structured reason
	// in the loop.end event (ddx-dc157075 AC #4). Work-owned terminal reasons
	// are classified through work.StopCondition; fatal_config,
	// preflight_failed, resource_exhausted, and future provider exhaustion are
	// still subsystem-specific exits.
	exitReason := ""

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
	leaseReader, _ := w.Store.(orphanHarnessLeaseReader)
	releaser, _ := w.Store.(orphanHarnessLeaseReleaser)
	appender, _ := w.Store.(orphanHarnessEventAppender)
	orphanScanner := runtime.OrphanHarnessProcessScanner
	if orphanScanner == nil {
		orphanScanner = defaultOrphanHarnessProcessScanner()
	}
	if reaped, reapErr := reapOrphanedHarnessChildren(
		ctx,
		runtime.ProjectRoot,
		orphanScanner,
		leaseReader,
		releaser,
		appender,
		assignee,
		runtime.Log,
		emit,
		func(pid int) error { return killProcessGroup(pid) },
	); reapErr != nil {
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "startup orphan harness reaper failed: %v\n", reapErr)
		}
		emit("loop.operator_attention", map[string]any{
			"reason":       "orphan_harness_reaper_failed",
			"project_root": runtime.ProjectRoot,
			"error":        reapErr.Error(),
		})
	} else if reaped > 0 && runtime.Log != nil {
		_, _ = fmt.Fprintf(runtime.Log, "startup orphan harness reaper killed %d orphaned harness process(es)\n", reaped)
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

	state := &executeBeadLoopState{
		now:                        now,
		assignee:                   assignee,
		preClaimTimeout:            preClaimTimeout,
		noProgressCooldown:         noProgressCooldown,
		heartbeatInterval:          heartbeatInterval,
		harness:                    harness,
		provider:                   provider,
		model:                      model,
		profile:                    profile,
		loopMode:                   loopMode,
		idleInterval:               idleInterval,
		result:                     result,
		resultsResetIdx:            resultsResetIdx,
		transientCandidateSkips:    transientCandidateSkips,
		pausedInfraUntil:           pausedInfraUntil,
		complexityGuard:            complexityGuard,
		preclaimGuard:              preclaimGuard,
		workLog:                    workLog,
		wasIdle:                    wasIdle,
		lastIdleQueueSignature:     lastIdleQueueSignature,
		preClaimIdleDetail:         preClaimIdleDetail,
		preClaimIdleStreak:         preClaimIdleStreak,
		preClaimIdleFirstAt:        preClaimIdleFirstAt,
		preClaimIdleEscalated:      preClaimIdleEscalated,
		liveness:                   liveness,
		serverOutage:               serverOutage,
		serverUnavailableLogged:    serverUnavailableLogged,
		sawParentBackEdge:          sawParentBackEdge,
		claimSuccessRateWindowSize: claimSuccessRateWindowSize,
		claimSuccessRateThreshold:  claimSuccessRateThreshold,
		claimSuccessRateWindow:     claimSuccessRateWindow,
		claimSuccessRateSuccesses:  claimSuccessRateSuccesses,
		claimSuccessRateWarned:     claimSuccessRateWarned,
		preClaimWarnThreshold:      preClaimWarnThreshold,
		preClaimWarnState:          preClaimWarnState,
		exitReason:                 exitReason,
		cleanupLog:                 cleanupLog,
		attemptStarted:             &attemptStarted,
	}
	for {
		outcome, err := w.runIteration(ctx, rcfg, runtime, state)
		exitReason = state.exitReason
		// A runIteration may finalize the same report through more than one
		// terminal path while recording tracker events. Flush the accumulated
		// durable audit once, here, after the whole iteration has unwound.
		if runtime.FlushDurableAudit != nil {
			if flushErr := runtime.FlushDurableAudit(); flushErr != nil {
				if isTransientGitContention(flushErr) {
					if runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "transient git/tracker contention committing durable audit outputs; not stopping (will retry next iteration): %v\n", flushErr)
					}
					emit("loop.durable_audit_transient", map[string]any{
						"reason": "git_tracker_contention",
						"detail": strings.TrimSpace(flushErr.Error()),
					})
					// Even ModeOnce needs one more empty iteration: the accumulator
					// retains the failed batch and the epilogue retries it there.
					outcome.Stop = false
					outcome.Continue = true
				} else {
					beadID := ""
					projectRoot := runtime.ProjectRoot
					if n := len(result.Results); n > 0 {
						beadID = result.Results[n-1].BeadID
						if projectRoot == "" {
							projectRoot = result.Results[n-1].ProjectRoot
						}
					}
					result.OperatorAttention = &OperatorAttentionStop{
						Reason:      "durable_audit_commit_failed",
						BeadID:      beadID,
						ProjectRoot: projectRoot,
						DirtyPaths:  append([]string(nil), trackerpaths.ManagedPathspecs()...),
						Message:     "DDx could not commit durable audit outputs; resolve the git failure before continuing autonomous work.",
					}
					state.exitReason = "operator_attention"
					exitReason = state.exitReason
					emit("loop.operator_attention", map[string]any{
						"reason":       result.OperatorAttention.Reason,
						"bead_id":      beadID,
						"project_root": projectRoot,
						"dirty_paths":  result.OperatorAttention.DirtyPaths,
						"message":      result.OperatorAttention.Message,
						"detail":       strings.TrimSpace(flushErr.Error()),
					})
					return result, nil
				}
			}
		}
		if err != nil {
			return result, err
		}
		if outcome.Stop {
			return result, nil
		}
	}
}

func (w *ExecuteBeadWorker) runIteration(ctx context.Context, rcfg config.ResolvedConfig, runtime ExecuteBeadLoopRuntime, state *executeBeadLoopState) (executeBeadIterationOutcome, error) {
	now := state.now
	assignee := state.assignee
	preClaimTimeout := state.preClaimTimeout
	noProgressCooldown := state.noProgressCooldown
	heartbeatInterval := state.heartbeatInterval
	harness := state.harness
	provider := state.provider
	model := state.model
	profile := state.profile
	loopMode := state.loopMode
	idleInterval := state.idleInterval
	result := state.result
	resultsResetIdx := state.resultsResetIdx
	transientCandidateSkips := state.transientCandidateSkips
	pausedInfraUntil := state.pausedInfraUntil
	complexityGuard := state.complexityGuard
	preclaimGuard := state.preclaimGuard
	workLog := state.workLog
	wasIdle := state.wasIdle
	lastIdleQueueSignature := state.lastIdleQueueSignature
	preClaimIdleDetail := state.preClaimIdleDetail
	preClaimIdleStreak := state.preClaimIdleStreak
	preClaimIdleFirstAt := state.preClaimIdleFirstAt
	preClaimIdleEscalated := state.preClaimIdleEscalated
	preClaimGitCorruptDetail := state.preClaimGitCorruptDetail
	preClaimGitCorruptStreak := state.preClaimGitCorruptStreak
	preClaimGitCorruptFirstAt := state.preClaimGitCorruptFirstAt
	preClaimGitCorruptEscalated := state.preClaimGitCorruptEscalated
	liveness := state.liveness
	serverOutage := state.serverOutage
	serverUnavailableLogged := state.serverUnavailableLogged
	sawParentBackEdge := state.sawParentBackEdge
	claimSuccessRateWindowSize := state.claimSuccessRateWindowSize
	claimSuccessRateThreshold := state.claimSuccessRateThreshold
	claimSuccessRateWindow := state.claimSuccessRateWindow
	claimSuccessRateSuccesses := state.claimSuccessRateSuccesses
	claimSuccessRateWarned := state.claimSuccessRateWarned
	preClaimWarnThreshold := state.preClaimWarnThreshold
	preClaimWarnState := state.preClaimWarnState
	exitReason := state.exitReason
	cleanupLog := state.cleanupLog
	attemptStarted := false
	if state.attemptStarted != nil {
		attemptStarted = *state.attemptStarted
	}
	defer func() {
		state.resultsResetIdx = resultsResetIdx
		state.pausedInfraUntil = pausedInfraUntil
		state.workLog = workLog
		state.wasIdle = wasIdle
		state.lastIdleQueueSignature = lastIdleQueueSignature
		state.preClaimIdleDetail = preClaimIdleDetail
		state.preClaimIdleStreak = preClaimIdleStreak
		state.preClaimIdleFirstAt = preClaimIdleFirstAt
		state.preClaimIdleEscalated = preClaimIdleEscalated
		state.preClaimGitCorruptDetail = preClaimGitCorruptDetail
		state.preClaimGitCorruptStreak = preClaimGitCorruptStreak
		state.preClaimGitCorruptFirstAt = preClaimGitCorruptFirstAt
		state.preClaimGitCorruptEscalated = preClaimGitCorruptEscalated
		state.serverUnavailableLogged = serverUnavailableLogged
		state.sawParentBackEdge = sawParentBackEdge
		state.claimSuccessRateWindow = claimSuccessRateWindow
		state.claimSuccessRateSuccesses = claimSuccessRateSuccesses
		state.claimSuccessRateWarned = claimSuccessRateWarned
		state.preClaimWarnState = preClaimWarnState
		state.exitReason = exitReason
		if state.attemptStarted != nil {
			*state.attemptStarted = attemptStarted
		}
	}()

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
	workGoal, goalErr := ReadRunGoal(runtime.ProjectRoot)
	if goalErr != nil && runtime.Log != nil {
		_, _ = fmt.Fprintf(runtime.Log, "goal load failed: %v (continuing mechanically)\n", goalErr)
	}
	if workGoal != nil && workGoal.IsActive() {
		result.Goal = workGoal
	} else {
		result.Goal = nil
	}
	resetPreClaimIdleStreak := func() {
		preClaimIdleDetail = ""
		preClaimIdleStreak = 0
		preClaimIdleEscalated = false
	}
	setServerUnavailableState := func(at time.Time) {
		if liveness == nil {
			return
		}
		liveness.SetWorkerState(serverUnavailableStatePhase, serverUnavailableLogMessage)
		liveness.OnTick(at)
	}
	clearServerUnavailableState := func(at time.Time) {
		if liveness == nil {
			return
		}
		liveness.SetWorkerState("", "")
		liveness.OnTick(at)
	}
	emit := func(eventType string, data map[string]any) {
		writeLoopEvent(runtime.EventSink, runtime.SessionID, eventType, data, now().UTC())
	}
	appendPreClaimWarn := func(beadID, reason, detail string, at time.Time) bool {
		escalated := appendPreClaimIntakeWarning(w.Store, emit, &preClaimWarnState, preClaimWarnThreshold, beadID, assignee, reason, detail, at)
		if !escalated {
			return false
		}
		detailText := fmt.Sprintf(
			"pre-claim warn fingerprint repeated %d times across %d distinct bead IDs",
			preClaimWarnState.Count,
			len(preClaimWarnState.DistinctBeadIDs),
		)
		result.OperatorAttention = &OperatorAttentionStop{
			Reason:      "preclaim_warn_repeated",
			BeadID:      beadID,
			ProjectRoot: runtime.ProjectRoot,
			Message:     detailText,
		}
		setExit("OperatorAttention", "operator_attention")
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "operator attention: %s\n", detailText)
		}
		return true
	}
	// Best-effort readiness is deliberately fail-open. Repeated failures still
	// need durable operator attention, but they must not turn an advisory route
	// outage into a queue-wide stop after otherwise executable beads were claimed.
	appendBestEffortPreClaimWarn := func(beadID, reason, detail string, at time.Time) {
		escalated := appendPreClaimIntakeWarning(w.Store, emit, &preClaimWarnState, preClaimWarnThreshold, beadID, assignee, reason, detail, at)
		if !escalated || runtime.Log == nil {
			return
		}
		_, _ = fmt.Fprintf(runtime.Log, "operator attention (non-terminal): pre-claim warn fingerprint repeated %d times across %d distinct bead IDs\n", preClaimWarnState.Count, len(preClaimWarnState.DistinctBeadIDs))
	}
	recordClaimAttempt := func(success bool, beadID string) {
		if claimSuccessRateWindowSize <= 0 {
			return
		}
		if len(claimSuccessRateWindow) == claimSuccessRateWindowSize {
			if claimSuccessRateWindow[0] {
				claimSuccessRateSuccesses--
			}
			copy(claimSuccessRateWindow, claimSuccessRateWindow[1:])
			claimSuccessRateWindow = claimSuccessRateWindow[:claimSuccessRateWindowSize-1]
		}
		claimSuccessRateWindow = append(claimSuccessRateWindow, success)
		if success {
			claimSuccessRateSuccesses++
			claimSuccessRateWarned = false
			return
		}
		if len(claimSuccessRateWindow) < claimSuccessRateWindowSize {
			return
		}
		rate := float64(claimSuccessRateSuccesses) / float64(claimSuccessRateWindowSize)
		if rate > claimSuccessRateThreshold || claimSuccessRateWarned {
			return
		}
		claimSuccessRateWarned = true
		detail := fmt.Sprintf(
			"claim success rate %.3f over the last %d claim attempts (%d successes, %d non-successes) is at or below threshold %.3f; manual intervention may be required",
			rate,
			claimSuccessRateWindowSize,
			claimSuccessRateSuccesses,
			claimSuccessRateWindowSize-claimSuccessRateSuccesses,
			claimSuccessRateThreshold,
		)
		emit("loop.operator_attention", map[string]any{
			"reason":        "claim_success_rate_below_threshold",
			"bead_id":       beadID,
			"detail":        detail,
			"window_size":   claimSuccessRateWindowSize,
			"successes":     claimSuccessRateSuccesses,
			"non_successes": claimSuccessRateWindowSize - claimSuccessRateSuccesses,
			"success_rate":  rate,
			"threshold":     claimSuccessRateThreshold,
		})
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "operator attention: %s\n", detail)
		}
	}
	clearTransientCandidateSkips := func() {
		for beadID := range transientCandidateSkips {
			delete(transientCandidateSkips, beadID)
		}
	}
	stopAfterNonAttemptSkip := func() bool {
		return loopMode == executeloop.ModeOnce && runtime.TargetBeadID != ""
	}
	strictIntakeBlocking := func() bool {
		return runtime.TargetBeadID != ""
	}
	finalizeDurableAuditOrStop := func(candidateID string, report ExecuteBeadReport) bool {
		if runtime.FinalizeDurableAudit == nil {
			return false
		}
		if err := runtime.FinalizeDurableAudit(report); err != nil {
			// Transient .git/index.lock contention from concurrent workers must
			// not stop the drain (ddx-23ac2796). The pending tracker/audit
			// changes remain staged and are committed on a later iteration, so
			// treat lock contention as retryable rather than operator attention --
			// a transient lock conflict must never halt an unattended worker.
			if isTransientGitContention(err) {
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log,
						"transient git/tracker contention committing durable audit outputs for %s; not stopping (will retry next iteration): %v\n",
						candidateID, err)
				}
				emit("loop.durable_audit_transient", map[string]any{
					"reason":  "git_tracker_contention",
					"bead_id": candidateID,
					"detail":  strings.TrimSpace(err.Error()),
				})
				return false
			}
			projectRoot := strings.TrimSpace(firstNonEmpty(report.ProjectRoot, runtime.ProjectRoot))
			dirtyPaths := append([]string(nil), trackerpaths.ManagedPathspecs()...)
			result.OperatorAttention = &OperatorAttentionStop{
				Reason:      "durable_audit_commit_failed",
				BeadID:      candidateID,
				ProjectRoot: projectRoot,
				DirtyPaths:  dirtyPaths,
				Message:     "DDx could not commit durable audit outputs; resolve the git failure before continuing autonomous work.",
			}
			setExit("OperatorAttention", "operator_attention")
			if runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log,
					"operator attention: failed to commit durable audit outputs for %s; stopping work. %v\n",
					candidateID,
					err,
				)
			}
			emit("loop.operator_attention", map[string]any{
				"reason":       "durable_audit_commit_failed",
				"bead_id":      candidateID,
				"project_root": projectRoot,
				"dirty_paths":  dirtyPaths,
				"message":      result.OperatorAttention.Message,
				"detail":       strings.TrimSpace(err.Error()),
			})
			return true
		}
		return false
	}
	_, _, _ = runExecutionCleanupPass(ctx, runtime.ProjectRoot, runtime.CleanupRunner, cleanupLog, emit, "pre-claim")
	// Respect context cancellation between iterations. Without this check,
	// a Stop() request (which cancels ctx) would only take effect during
	// the idle poll sleep — the loop would happily claim the next ready
	// bead as soon as the current Execute returned, ignoring the cancel.
	if err := ctx.Err(); err != nil {
		if exitReason == "" {
			applyStop(work.StopInput{ContextErr: err})
		}
		return executeBeadIterationOutcome{Stop: true}, err
	}
	if loopMode == executeloop.ModeWatch && serverOutage.Active() {
		if !serverUnavailableLogged && runtime.Log != nil {
			_, _ = fmt.Fprintln(runtime.Log, serverUnavailableLogMessage)
			serverUnavailableLogged = true
		}
		setServerUnavailableState(now().UTC())
		for serverOutage.Active() {
			delay := time.Until(serverOutage.NextProbeAt())
			if delay < 0 {
				delay = 0
			}
			if err := sleepOrWake(ctx, delay, runtime.WakeCh); err != nil {
				if exitReason == "" {
					applyStop(work.StopInput{ContextErr: err})
				}
				return executeBeadIterationOutcome{Stop: true}, err
			}
			probeAt := now().UTC()
			setServerUnavailableState(probeAt)
			healthy := false
			var probeErr error
			if runtime.ServerHealthProbe != nil {
				healthy, probeErr = runtime.ServerHealthProbe(ctx)
				if probeErr != nil && runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "server health probe failed: %v (retrying)\n", probeErr)
				}
			}
			serverOutage.MarkProbeAttempt(probeAt)
			if healthy {
				reason := serverOutage.Reason()
				serverOutage.Clear()
				clearServerUnavailableState(now().UTC())
				serverUnavailableLogged = false
				emit("loop.server_recovered", map[string]any{
					"reason": reason,
				})
				if runtime.Log != nil {
					_, _ = fmt.Fprintln(runtime.Log, "server reachable: resuming queue")
				}
				break
			}
		}
		if serverOutage.Active() {
			return executeBeadIterationOutcome{Continue: true}, nil
		}
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
			return executeBeadIterationOutcome{Stop: true}, nil
		}
	}
	if runtime.ResourcePressureChecker != nil {
		pressureReport, pressureErr := runtime.ResourcePressureChecker.Check(ctx)
		if pressureErr != nil {
			if runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "resource pressure check failed: %v\n", pressureErr)
			}
		} else {
			emitResourcePressure(emit, runtime.Log, "pre-claim", pressureReport)
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
				detail := ResourceExhaustedStopMessage
				diagnosis := ""
				restartable := false
				if resourceResult.FDExhausted() {
					detail = FDExhaustionStopMessage
					diagnosis = ResourceExhaustionDiagnosisFD
					restartable = true
				}
				report := ExecuteBeadReport{
					WorkerID:                      runtime.WorkerID,
					Harness:                       harness,
					Model:                         model,
					Status:                        ExecuteBeadStatusResourceExhausted,
					Detail:                        detail,
					Error:                         resourceErr.Error(),
					SessionID:                     runtime.SessionID,
					ResourceExhausted:             &resourceResult,
					ResourceExhaustionDiagnosis:   diagnosis,
					ResourceExhaustionRestartable: restartable,
					Disrupted:                     true,
					DisruptionReason:              ReadinessSystemReasonResourceExhausted,
					OutcomeReason:                 ReadinessSystemReasonResourceExhausted,
				}
				result.Failures++
				result.LastFailureStatus = report.Status
				result.Results = append(result.Results, report)
				setExit("ResourceExhausted", "resource_exhausted")
				emitResourceExhausted(emit, nil, "", report, assignee, now().UTC())
				if runtime.Log != nil {
					_, _ = fmt.Fprintln(runtime.Log, detail)
				}
				return executeBeadIterationOutcome{Stop: true}, nil
			}
			setExit("Preflight", "preflight_failed")
			return executeBeadIterationOutcome{Stop: true}, checkErr
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
			return executeBeadIterationOutcome{Stop: true}, nil
		}
	}
	if runtime.ProjectRootDirtyCheck != nil && runtime.ProjectRoot != "" {
		if dirtyPaths := runtime.ProjectRootDirtyCheck(runtime.ProjectRoot); len(dirtyPaths) > 0 {
			if loopMode == executeloop.ModeWatch && attemptStarted {
				preserveDirty := w.preDispatchDirtyPreserver
				if preserveDirty == nil {
					preserveDirty = preservePreDispatchDirtyPaths
				}
				if preserved, preserveErr := preserveDirty(runtime.ProjectRoot, dirtyPaths); preserveErr == nil && preserved != nil {
					if runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log,
							"preserved DDx-created landing dirt in %s under %s; continuing watch. dirty paths: %s. recover with %s\n",
							runtime.ProjectRoot,
							preserved.PreserveRef,
							strings.Join(preserved.DirtyPaths, ", "),
							preserved.RecoverCommand,
						)
					}
					emit("loop.dirty_root_preserved", map[string]any{
						"reason":          "ddx_created_landing_dirt",
						"project_root":    runtime.ProjectRoot,
						"dirty_paths":     preserved.DirtyPaths,
						"preserve_ref":    preserved.PreserveRef,
						"recover_command": preserved.RecoverCommand,
					})
					if guardErr := clearDirtyRootGuardState(runtime.ProjectRoot); guardErr != nil && runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "dirty-root guard clear failed: %v\n", guardErr)
					}
					return executeBeadIterationOutcome{Continue: true}, nil
				} else if preserveErr != nil && runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "preserving DDx-created landing dirt failed: %v\n", preserveErr)
				}
			}
			guardState, escalated, guardErr := updateDirtyRootGuardState(
				runtime.ProjectRoot,
				dirtyPaths,
				now().UTC(),
				DefaultDirtyRootEscalationThreshold,
				DirtyRootEscalationCooldown,
			)
			if guardErr != nil && runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "dirty-root guard persistence failed: %v\n", guardErr)
			}
			reason := "dirty_project_root"
			retryAfter := ""
			message := fmt.Sprintf(
				"project root has uncommitted tracked changes (%s); resolve before resuming autonomous work",
				strings.Join(normalizeDirtyRootPaths(dirtyPaths), ", "),
			)
			if guardState != nil {
				if len(guardState.DirtyPaths) > 0 {
					dirtyPaths = append([]string(nil), guardState.DirtyPaths...)
				}
				if escalated {
					reason = "dirty_project_root_repeated"
					retryAfter = guardState.RetryAfter.UTC().Format(time.RFC3339)
					message = fmt.Sprintf(
						"project root has uncommitted tracked changes (%s); relaunch suppressed until %s",
						strings.Join(dirtyPaths, ", "),
						retryAfter,
					)
				}
			}
			result.OperatorAttention = &OperatorAttentionStop{
				Reason:      reason,
				ProjectRoot: runtime.ProjectRoot,
				DirtyPaths:  dirtyPaths,
				RetryAfter:  retryAfter,
				Message:     message,
			}
			setExit("OperatorAttention", "operator_attention")
			emit("loop.operator_attention", map[string]any{
				"reason":       reason,
				"project_root": runtime.ProjectRoot,
				"dirty_paths":  dirtyPaths,
				"retry_after":  retryAfter,
				"message":      message,
			})
			if runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "operator attention: %s\n", message)
			}
			return executeBeadIterationOutcome{Stop: true}, nil
		}
		if guardErr := clearDirtyRootGuardState(runtime.ProjectRoot); guardErr != nil && runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "dirty-root guard clear failed: %v\n", guardErr)
		}
	}
	if runtime.TrackerSyncEnabled && runtime.ProjectRoot != "" {
		if err := syncTrackerBeforeClaim(ctx, runtime.ProjectRoot, runtime.Log, emit); err != nil {
			corruptDetail := strings.TrimSpace(err.Error())
			if corruptDetail == "" {
				corruptDetail = "project git corruption detected during pre-claim tracker sync"
			}
			if corruptDetail == preClaimGitCorruptDetail {
				preClaimGitCorruptStreak++
			} else {
				preClaimGitCorruptDetail = corruptDetail
				preClaimGitCorruptStreak = 1
				preClaimGitCorruptFirstAt = now().UTC()
				preClaimGitCorruptEscalated = false
			}
			if preClaimGitCorruptStreak >= 2 && !preClaimGitCorruptEscalated {
				preClaimGitCorruptEscalated = true
				elapsed := now().UTC().Sub(preClaimGitCorruptFirstAt)
				result.OperatorAttention = &OperatorAttentionStop{
					Reason:      "project_git_corrupt",
					ProjectRoot: runtime.ProjectRoot,
					Message:     corruptDetail,
				}
				setExit("OperatorAttention", "operator_attention")
				emit("loop.operator_attention", map[string]any{
					"reason":       "project_git_corrupt",
					"project_root": runtime.ProjectRoot,
					"detail":       corruptDetail,
					"elapsed_idle": elapsed.String(),
					"streak_count": preClaimGitCorruptStreak,
				})
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "operator attention: project git corruption repeated %d times during pre-claim sync (%s); stopping work\n", preClaimGitCorruptStreak, corruptDetail)
				}
				return executeBeadIterationOutcome{Stop: true}, nil
			}
			if runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "pre-claim tracker sync corruption observed; will stop if repeated: %s\n", corruptDetail)
			}
		} else {
			preClaimGitCorruptDetail = ""
			preClaimGitCorruptStreak = 0
			preClaimGitCorruptEscalated = false
		}
	}

	if runtime.TargetBeadID == "" {
		if reopened, reopenErr := autoReopenRetryableProviderConnectivityProposals(ctx, w.Store, assignee, now().UTC(), emit); reopenErr != nil {
			if runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "provider-connectivity auto-reopen failed: %v; continuing\n", reopenErr)
			}
			emit("provider_connectivity.auto_reopen_error", map[string]any{
				"error": reopenErr.Error(),
			})
		} else if reopened > 0 && runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "provider-connectivity auto-reopened %d operator-attention bead(s)\n", reopened)
		}
	}

	candidate, skips, ok, err := w.nextCandidate(ctx, result.Results[resultsResetIdx:], []work.Guard{complexityGuard, preclaimGuard}, runtime.LabelFilter, runtime.TargetBeadID)
	if err != nil {
		setExit("FatalConfig", "fatal_config")
		return executeBeadIterationOutcome{Stop: true}, err
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
		if detail, reasonCode, idleBeadID, isIdle := preClaimIdleSkip(skips); isIdle {
			if runtime.Log != nil && !wasIdle {
				_, _ = fmt.Fprintf(runtime.Log, "pre-claim hook blocked queue: %s; sleeping %s\n", detail, idleInterval)
			}
			emit("loop.idle", map[string]any{
				"reason":        reasonCode,
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
				Message:   reasonCode,
			})
			wasIdle = true

			// Same-detail escalation (ddx-df77e668 AC #3): count consecutive
			// idle cycles on an identical blocker and emit a non-terminal
			// operator-attention event once the streak crosses the threshold,
			// instead of looping silently forever.
			if detail == preClaimIdleDetail {
				preClaimIdleStreak++
			} else {
				preClaimIdleDetail = detail
				preClaimIdleStreak = 1
				preClaimIdleFirstAt = now().UTC()
				preClaimIdleEscalated = false
			}
			if preClaimIdleStreak >= preClaimIdleEscalationThreshold && !preClaimIdleEscalated {
				preClaimIdleEscalated = true
				elapsedIdle := now().UTC().Sub(preClaimIdleFirstAt)
				payload := map[string]any{
					"reason":       "preclaim_idle_escalation",
					"bead_id":      idleBeadID,
					"detail":       detail,
					"elapsed_idle": elapsedIdle.String(),
					"idle_count":   preClaimIdleStreak,
				}
				for k, v := range preClaimDecisionAudit("preclaim_idle_escalation", preClaimIdleStreak) {
					payload[k] = v
				}
				emit("loop.operator_attention", payload)
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "operator attention: pre-claim idled %d consecutive cycles on the same blocker (%s; idle for %s); manual intervention may be required\n", preClaimIdleStreak, detail, elapsedIdle)
				}
			}

			if loopMode != executeloop.ModeWatch {
				setExit("Preflight", reasonCode)
				return executeBeadIterationOutcome{Stop: true}, nil
			}
			if err := sleepOrWake(ctx, idleInterval, runtime.WakeCh); err != nil {
				if exitReason == "" {
					applyStop(work.StopInput{ContextErr: err})
				}
				return executeBeadIterationOutcome{Stop: true}, err
			}
			return executeBeadIterationOutcome{Continue: true}, nil
		}
		if hasGuardSkips(skips) {
			return executeBeadIterationOutcome{Continue: true}, nil
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
						return executeBeadIterationOutcome{Stop: true}, err
					}
					return executeBeadIterationOutcome{Continue: true}, nil
				}
			}
		}

		// Idle-path closure cascade (FEAT-004 §Queue Semantics For Epics):
		// before declaring no ready work, close any epics whose children have
		// all reached a terminal state. If any were closed, loop again —
		// those closures may unblock downstream work.
		if closed, _ := w.runEpicClosureCascade(ctx, emit); closed > 0 {
			return executeBeadIterationOutcome{Continue: true}, nil
		}

		result.NoReadyWork = true
		if diag, ok := w.Store.(readyDiagnoser); ok {
			if breakdown, bErr := diag.ReadyExecutionBreakdown(); bErr == nil {
				result.NoReadyWorkDetail = noReadyWorkBreakdownFromLifecycle(breakdown)
				snapshot := queueSnapshotFromLifecycle(breakdown)
				result.QueueSnapshot = &snapshot
			}
		}
		goalSleep := idleInterval
		if result.Goal != nil {
			goalSatisfied, goalDetail := evaluatePersistedWorkGoal(ctx, result.Goal, runtime)
			if goalSatisfied {
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "goal satisfied: %s\n", FormatWorkGoal(result.Goal))
				}
				emit("loop.goal_satisfied", map[string]any{
					"goal":         FormatWorkGoal(result.Goal),
					"project_root": runtime.ProjectRoot,
				})
				if applyStop(work.StopInput{
					NoReadyWork:   true,
					GoalSatisfied: true,
					Once:          loopMode == executeloop.ModeOnce,
					Mode:          loopMode,
				}) {
					return executeBeadIterationOutcome{Stop: true}, nil
				}
			} else {
				goalSleep = goalWaitSleepInterval(runtime, idleInterval)
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "goal pending: %s; %s; sleeping %s\n", FormatWorkGoal(result.Goal), goalDetail, goalSleep)
				}
				emit("loop.goal_wait", map[string]any{
					"goal":           FormatWorkGoal(result.Goal),
					"goal_detail":    goalDetail,
					"sleep_interval": goalSleep.String(),
					"project_root":   runtime.ProjectRoot,
				})
			}
		} else if applyStop(work.StopInput{
			NoReadyWork: true,
			Once:        loopMode == executeloop.ModeOnce,
			Mode:        loopMode,
		}) {
			return executeBeadIterationOutcome{Stop: true}, nil
		}
		if runtime.Log != nil {
			signature := workLogQueueSnapshotSignature(result.QueueSnapshot)
			includeBlockers := signature != "" && signature != lastIdleQueueSignature
			_, _ = fmt.Fprint(runtime.Log, workLog.FormatWatchIdle(goalSleep, result.QueueSnapshot, includeBlockers))
			lastIdleQueueSignature = signature
		}
		clearTransientCandidateSkips()
		resetPreClaimIdleStreak()
		wasIdle = true
		// Watch mode treats an empty queue as idle, not terminal. --once
		// and drain exits are classified above through work.StopCondition.
		// Emit a transient "no_ready_work"
		// event so server-managed workers can surface this as an idle substate
		// (ddx-dc157075 AC #5) instead of treating it as terminal.
		emit("loop.idle", map[string]any{
			"reason":        "no_ready_work",
			"idle_interval": goalSleep.String(),
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
		if err := sleepOrWake(ctx, goalSleep, runtime.WakeCh); err != nil {
			if exitReason == "" {
				applyStop(work.StopInput{ContextErr: err})
			}
			return executeBeadIterationOutcome{Stop: true}, err
		}
		return executeBeadIterationOutcome{Continue: true}, nil
	}

	// Found a candidate: clear any "idle" substate set on the previous
	// no-candidate iteration (ddx-dc157075 AC #5).
	if wasIdle && runtime.Log != nil {
		_, _ = fmt.Fprint(runtime.Log, workLog.FormatNextReadyTransition(candidate.ID, candidate.Title))
	}
	wasIdle = false
	resetPreClaimIdleStreak()
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
		return executeBeadIterationOutcome{Continue: true}, nil
	}
	// Pace host load after candidate/complexity acceptance but before the final
	// freshness read. No claim, lease, or tracker mutation exists during the
	// cancellable delay; refreshCandidateBeforeClaim revalidates immediately
	// afterward so the worker never claims from a stale pre-sleep snapshot.
	if loadErr := waitForLoadPressureBeforeClaim(ctx, runtime, candidate.ID, emit); loadErr != nil {
		if exitReason == "" {
			applyStop(work.StopInput{ContextErr: loadErr})
		}
		return executeBeadIterationOutcome{Stop: true}, loadErr
	}
	freshCandidate, staleSkip, refreshErr := w.refreshCandidateBeforeClaim(ctx, candidate)
	if refreshErr != nil {
		return executeBeadIterationOutcome{Stop: true}, refreshErr
	}
	if staleSkip != nil {
		transientCandidateSkips[candidate.ID] = staleCandidateSkipReason
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "picker skip stale candidate %s before claim: %s\n", candidate.ID, staleSkip.Detail)
		}
		emitStaleCandidateSkip(emit, candidate.ID, staleSkip, freshCandidate, "pre_claim")
		recordClaimAttempt(false, candidate.ID)
		if stopAfterNonAttemptSkip() {
			applyStop(work.StopInput{Once: true})
			return executeBeadIterationOutcome{Stop: true}, nil
		}
		return executeBeadIterationOutcome{Continue: true}, nil
	}
	candidate = *freshCandidate
	// Consecutive-wedge guard (ddx-9714eaac): if this bead wedged
	// (route_resolution_timeout / progress_watchdog) on its immediately
	// preceding claim(s) up to the threshold, stop re-claiming it. A
	// route-independent wedge carries no failed-route entry, so without this
	// guard a freshly released wedged bead is re-claimed and re-wedged
	// indefinitely while ready work starves (parent ddx-8f2e0ebf criterion E).
	// The bead is parked to proposed for operator attention; `continue` moves
	// the loop on to the next ready bead so the queue keeps draining.
	if marker := readWedgeMarker(candidate.Extra); consecutiveWedgeGuardTrips(marker, runtime.effectiveConsecutiveWedgeThreshold()) {
		threshold := runtime.effectiveConsecutiveWedgeThreshold()
		if parkErr := flagConsecutiveWedgeForOperator(w.Store, candidate.ID, assignee, marker, threshold, now().UTC()); parkErr != nil && runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "consecutive-wedge guard: failed to park %s for operator attention: %v\n", candidate.ID, parkErr)
		}
		emit("loop.consecutive_wedge_guard", map[string]any{
			"bead_id":     candidate.ID,
			"count":       marker.Count,
			"threshold":   threshold,
			"last_reason": marker.LastReason,
		})
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "consecutive-wedge guard: %s wedged %d consecutive claims (>= %d); flagged for operator attention, continuing to next ready bead\n", candidate.ID, marker.Count, threshold)
		}
		recordClaimAttempt(false, candidate.ID)
		return executeBeadIterationOutcome{Continue: true}, nil
	}
	// Claim atomically before model-backed intake and lint to prevent
	// concurrent workers from issuing duplicate model calls or appending
	// duplicate lint events for the same bead. Terminal non-actionable
	// outcomes unclaim and park the bead rather than proceeding to execution.
	var claimErr error
	if claimer, ok := w.Store.(claimWithOptionsStore); ok {
		claimErr = claimer.ClaimWithOptions(candidate.ID, assignee, runtime.SessionID, "")
	} else {
		claimErr = w.Store.Claim(candidate.ID, assignee)
	}
	if claimErr != nil {
		// Another worker won the race for this bead. Emit a structured
		// claim_race event so concurrent-worker losses are observable
		// (ddx-9d55601f AC #4). The bead remains in `attempted` for this
		// run; on the next iteration it will be filtered out of
		// ReadyExecution naturally because the winner now holds the fresh
		// claim lease sidecar, so the loser keeps moving down priority
		// order.
		emit("picker.claim_race", map[string]any{
			"bead_id":    candidate.ID,
			"priority":   candidate.Priority,
			"queue_rank": queueRankValue(candidate.Extra),
			"reason":     claimErr.Error(),
		})
		recordClaimAttempt(false, candidate.ID)
		return executeBeadIterationOutcome{Continue: true}, nil
	}
	recordClaimAttempt(true, candidate.ID)
	if runtime.TrackerSyncEnabled && runtime.ProjectRoot != "" {
		syncTrackerAfterClaim(ctx, runtime.ProjectRoot, candidate.ID, runtime.Log, emit)
	}

	overrideRetryAfter := ""
	overrideMeta := forcedCooldownMetadata{}
	if runtime.IgnoreCooldown {
		if reporter, ok := w.Store.(cooldownOverrideReporter); ok {
			if retryAfter, overridden := reporter.CooldownOverrideInfo(candidate.ID); overridden {
				overrideRetryAfter = retryAfter
				overrideMeta = forcedCooldownSnapshot(candidate, retryAfter, true)
				appendForceClaimEvent(w.Store, candidate.ID, assignee, executeLoopEventSource(runtime), runtime.CooldownOverrideReason, retryAfter, now().UTC())
			}
		}
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

	recovery, recErr := recoverDanglingSuccess(
		w.Store, runtime.ProjectRoot, candidate.ID,
		assignee, now, emit,
	)
	if recErr != nil {
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "dangling-success recovery error (%s): %v\n", candidate.ID, recErr)
		}
		_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
			return commitOutcomeError("recoverDanglingSuccess", assignee, result, recErr)
		})
		if ctx.Err() != nil {
			return executeBeadIterationOutcome{Stop: true}, ctx.Err()
		}
		return executeBeadIterationOutcome{Continue: true}, nil
	}
	if recovery != nil {
		report := danglingSuccessRecoveryReport(candidate.ID, recovery)
		result.Attempts++
		if recovery.Outcome == danglingSuccessOutcomeClosed {
			result.Successes++
			result.LastSuccessAt = now().UTC()
		} else {
			result.Failures++
			result.LastFailureStatus = report.Status
		}
		result.Results = append(result.Results, report)
		if runtime.Log != nil {
			_, _ = fmt.Fprintln(runtime.Log, formatLoopResultLine(candidate.ID, report))
		}
		if finalizeDurableAuditOrStop(candidate.ID, report) {
			return executeBeadIterationOutcome{Stop: true}, nil
		}
		if applyStop(work.StopInput{Once: loopMode == executeloop.ModeOnce}) {
			return executeBeadIterationOutcome{Stop: true}, nil
		}
		return executeBeadIterationOutcome{Continue: true}, nil
	}

	repairer := w.preDispatchGitRepairer
	if repairer == nil {
		repairer = defaultPreDispatchGitRepairer
	}
	gitRepair := repairer(ctx, runtime.ProjectRoot)
	if detail, failed := preDispatchGitRepairFailure(gitRepair); failed {
		if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("Unclaim", assignee, result, err)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Stop: true}, err
		}
		stop := &OperatorAttentionStop{
			Reason:      preDispatchGitRepairFailedReason,
			BeadID:      candidate.ID,
			ProjectRoot: runtime.ProjectRoot,
			Message:     "DDx could not repair project git config; resolve the git status failure before restarting ddx work",
		}
		result.OperatorAttention = stop
		setExit("OperatorAttention", "operator_attention")
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log,
				"operator attention: pre-dispatch git repair failed for %s at %s; released bead. %s\n",
				candidate.ID,
				runtime.ProjectRoot,
				detail,
			)
		}
		emit("loop.operator_attention", map[string]any{
			"reason":          stop.Reason,
			"bead_id":         candidate.ID,
			"project_root":    runtime.ProjectRoot,
			"message":         stop.Message,
			"detail":          detail,
			"git_stderr":      gitRepair.StatusStderr,
			"issue_types":     gitRepair.RepairedTypes,
			"repair_commands": gitRepair.Commands,
		})
		return executeBeadIterationOutcome{Stop: true}, nil
	}
	if len(gitRepair.RepairedTypes) > 0 {
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log,
				"repaired project git config before readiness for %s: %s (%s)\n",
				candidate.ID,
				strings.Join(gitRepair.RepairedTypes, ", "),
				strings.Join(gitRepair.Commands, "; "),
			)
		}
		emit("loop.pre_dispatch_git_repaired", map[string]any{
			"bead_id":          candidate.ID,
			"project_root":     runtime.ProjectRoot,
			"issue_types":      gitRepair.RepairedTypes,
			"repair_commands":  gitRepair.Commands,
			"status_rechecked": gitRepair.StatusSucceeded,
			"stage":            "readiness",
		})
	}

	skipPreClaimIntake := false
	readinessEstimatedDifficulty := ""

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
					_ = releaseWorkerClaim(w.Store, candidate.ID, assignee)
					if stopAfterNonAttemptSkip() {
						applyStop(work.StopInput{Once: true})
						return executeBeadIterationOutcome{Stop: true}, nil
					}
					return executeBeadIterationOutcome{Continue: true}, nil
				}
			} else {
				if appendPreClaimWarn(candidate.ID, "decomposition_depth_cap", overflowDetail, now().UTC()) {
					return executeBeadIterationOutcome{Stop: true}, nil
				}
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
		// Keep the external claim lease alive while the model-backed
		// readiness/intake hook runs. This closes the pre-attempt gap where
		// the owned bead was previously heartbeated only after intake
		// completed, allowing a live worker to look stale and be reclaimed.
		intakeResult, intakeErr := work.WithHeartbeat(ctx, candidate.ID, heartbeatInterval, w.Store, liveness, func() (PreClaimIntakeResult, error) {
			return runPreClaimIntakeHookWithTimeout(ctx, runtime.PreClaimIntakeHook, candidate.ID, preClaimTimeout)
		})
		if intakeErr == nil {
			readinessEstimatedDifficulty = normalizeReadinessEstimatedDifficulty(intakeResult.EstimatedDifficulty)
		}
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
			operatorOverrideTimeout, _ := detectIntakeBlockedOperatorOverride(w.Store, &candidate, "pre_claim_intake.timeout", ReadinessReasonSystemUnready, "pre_claim_intake", "block", "park", "review intake result and accept, rewrite, split, block, or cancel")
			eventBody["operator_override"] = operatorOverrideTimeout
			if strictIntakeBlocking() {
				emit("pre_claim_intake.blocked", eventBody)
				if parked, berr := parkBeadPostIntakeRejection(w.Store, &candidate, assignee, PreClaimIntakeError, ReadinessReasonSystemUnready, timeoutDetail, now().UTC()); berr != nil && runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "readiness park error: %v\n", berr)
				} else if parked {
					if runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "readiness timeout parked bead %s for operator review\n", candidate.ID)
					}
				}
				_ = releaseWorkerClaim(w.Store, candidate.ID, assignee)
				if stopAfterNonAttemptSkip() {
					applyStop(work.StopInput{Once: true})
					return executeBeadIterationOutcome{Stop: true}, nil
				}
				return executeBeadIterationOutcome{Continue: true}, nil
			} else {
				emit("pre_claim_intake.warn", eventBody)
				if appendPreClaimWarn(candidate.ID, "timeout", timeoutDetail, now().UTC()) {
					return executeBeadIterationOutcome{Stop: true}, nil
				}
				intakeOutcome = PreClaimIntakeActionableAtomic
			}
		}
		switch {
		case intakeErr != nil:
			if errors.Is(intakeErr, context.Canceled) || errors.Is(intakeErr, context.DeadlineExceeded) {
				if exitReason == "" {
					applyStop(work.StopInput{ContextErr: intakeErr})
				}
				return executeBeadIterationOutcome{Stop: true}, intakeErr
			}
			warning := trimDiagnosticPrefix(intakeErr.Error(), "pre-claim intake")
			classified := ClassifyReadiness(ReadinessClassificationSystemUnready, nil, warning)
			rootMutationError := sawParentBackEdge && isProjectRootMutationRejectedDetail(warning)
			warningCode, warningDetail := normalizePreClaimSystemUnreadyWarning(warning)
			message := fmt.Sprintf("check unavailable: %s (continuing)", warningDetail)
			eventType := "pre_claim_intake.warn"
			policyMode := "warn-only"
			decision := "warn"
			suggestedAction := "check the readiness route or harness configuration and retry"
			if rootMutationError {
				message = fmt.Sprintf("check unavailable: %s (code=%s; continuing)", warningDetail, warningCode)
				suggestedAction = "inspect the parent back-edge and clean the project root before retrying"
			}
			if runtime.Log != nil {
				_, _ = fmt.Fprint(runtime.Log, workLog.FormatLifecycleLine(WorkLogLifecycleLine{
					Phase:    "readiness",
					BeadID:   candidate.ID,
					Message:  message,
					Harness:  harness,
					Provider: provider,
					Model:    model,
				}))
			}
			eventBody := readinessDecisionBody(
				"pre_claim_intake.system_unready",
				classified.Reason,
				"pre_claim_intake",
				policyMode,
				decision,
				suggestedAction,
				map[string]any{
					"bead_id":       candidate.ID,
					"outcome":       string(PreClaimIntakeError),
					"system_reason": classified.SystemReason,
					"detail":        warningDetail,
					"error_detail":  warning,
				},
			)
			if warningCode != "" {
				eventBody["code"] = warningCode
			}
			emit(eventType, eventBody)
			appendBestEffortPreClaimWarn(candidate.ID, "system_unready", warningDetail, now().UTC())
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
				if appendPreClaimWarn(candidate.ID, "rewrite_rejected", warning, now().UTC()) {
					return executeBeadIterationOutcome{Stop: true}, nil
				}
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
			rootMutationError := sawParentBackEdge && isProjectRootMutationRejectedDetail(warning)
			warningCode, warningDetail := normalizePreClaimSystemUnreadyWarning(warning)
			message := fmt.Sprintf("check unavailable: %s (continuing)", warningDetail)
			eventType := "pre_claim_intake.warn"
			policyMode := "warn-only"
			decision := "warn"
			suggestedAction := "check the readiness route or harness configuration and retry"
			if rootMutationError {
				message = fmt.Sprintf("check unavailable: %s (code=%s; continuing)", warningDetail, warningCode)
				suggestedAction = "inspect the parent back-edge and clean the project root before retrying"
			}
			if runtime.Log != nil {
				_, _ = fmt.Fprint(runtime.Log, workLog.FormatLifecycleLine(WorkLogLifecycleLine{
					Phase:    "readiness",
					BeadID:   candidate.ID,
					Message:  message,
					Harness:  harness,
					Provider: provider,
					Model:    model,
				}))
			}
			eventBody := readinessDecisionBody(
				"pre_claim_intake.system_unready",
				reason,
				"pre_claim_intake",
				policyMode,
				decision,
				suggestedAction,
				map[string]any{
					"bead_id":       candidate.ID,
					"outcome":       string(PreClaimIntakeError),
					"system_reason": systemReason,
					"detail":        warningDetail,
					"error_detail":  warning,
				},
			)
			if warningCode != "" {
				eventBody["code"] = warningCode
			}
			emit(eventType, eventBody)
			if appendPreClaimWarn(candidate.ID, "system_unready", warningDetail, now().UTC()) {
				return executeBeadIterationOutcome{Stop: true}, nil
			}
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
				decomp, hookErr = runPreclaimDecompositionHookWithResolvingLiveness(
					ctx,
					runtime.PostAttemptDecompositionHook,
					candidate.ID,
					liveness,
					harness,
					model,
					profile,
					heartbeatInterval,
					now,
				)
				if hookErr != nil {
					if err := ctx.Err(); err != nil {
						applyStop(work.StopInput{ContextErr: err})
						return executeBeadIterationOutcome{Stop: true}, err
					}
					warning := fmt.Sprintf("decomposition hook unavailable: %s", hookErr.Error())
					if runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "bead decomposition unavailable: %s (%s); continuing with attempt\n", hookErr.Error(), candidate.ID)
					}
					if appendPreClaimWarn(candidate.ID, "decomposition_hook_unavailable", warning, now().UTC()) {
						return executeBeadIterationOutcome{Stop: true}, nil
					}
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
				if appendPreClaimWarn(candidate.ID, "decomposition_hook_empty", warning, now().UTC()) {
					return executeBeadIterationOutcome{Stop: true}, nil
				}
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
				operatorOverrideDecompBlocked, _ := detectIntakeBlockedOperatorOverride(w.Store, &candidate, "pre_claim_intake."+strings.TrimSpace(string(PreClaimIntakeOperatorRequired)), "operator_required", "pre_claim_intake", "block", "park", "review intake result and accept, rewrite, split, block, or cancel")
				emit("pre_claim_intake.blocked", map[string]any{
					"bead_id":           candidate.ID,
					"operator_override": operatorOverrideDecompBlocked,
					"outcome":           string(PreClaimIntakeOperatorRequired),
					"reason":            blockedDetail,
					"detail":            blockedDetail,
				})
				if strictIntakeBlocking() {
					if parked, berr := parkBeadPostIntakeRejection(w.Store, &candidate, assignee, PreClaimIntakeOperatorRequired, "operator_required", blockedDetail, now().UTC()); berr != nil && runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "readiness park error: %v\n", berr)
					} else if parked {
						_ = releaseWorkerClaim(w.Store, candidate.ID, assignee)
						if stopAfterNonAttemptSkip() {
							applyStop(work.StopInput{Once: true})
							return executeBeadIterationOutcome{Stop: true}, nil
						}
						return executeBeadIterationOutcome{Continue: true}, nil
					}
				} else {
					if appendPreClaimWarn(candidate.ID, "decomposition_blocked_best_effort", blockedDetail, now().UTC()) {
						return executeBeadIterationOutcome{Stop: true}, nil
					}
				}
				break
			}
			childIDs, decompErr := applyPreClaimDecomposition(ctx, w.Store, &candidate, decomp, assignee, now().UTC())
			if decompErr != nil {
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "bead decomposition error: %v (%s)\n", decompErr, candidate.ID)
				}
				operatorOverrideDecompErr, _ := detectIntakeBlockedOperatorOverride(w.Store, &candidate, "pre_claim_intake."+strings.TrimSpace(string(PreClaimIntakeOperatorRequired)), "operator_required", "pre_claim_intake", "block", "park", "review intake result and accept, rewrite, split, block, or cancel")
				emit("pre_claim_intake.blocked", readinessDecisionBody(
					"pre_claim_intake."+strings.TrimSpace(string(PreClaimIntakeOperatorRequired)),
					"operator_required",
					"pre_claim_intake",
					"block",
					"park",
					"review intake result and accept, rewrite, split, block, or cancel",
					map[string]any{
						"bead_id":           candidate.ID,
						"operator_override": operatorOverrideDecompErr,
						"outcome":           string(PreClaimIntakeOperatorRequired),
						"detail":            decompErr.Error(),
					},
				))
				if strictIntakeBlocking() {
					if parked, berr := parkBeadPostIntakeRejection(w.Store, &candidate, assignee, PreClaimIntakeOperatorRequired, "operator_required", decompErr.Error(), now().UTC()); berr != nil && runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "readiness park error: %v\n", berr)
					} else if parked {
						_ = releaseWorkerClaim(w.Store, candidate.ID, assignee)
						if stopAfterNonAttemptSkip() {
							applyStop(work.StopInput{Once: true})
							return executeBeadIterationOutcome{Stop: true}, nil
						}
						return executeBeadIterationOutcome{Continue: true}, nil
					}
				} else {
					if appendPreClaimWarn(candidate.ID, "decomposition_error_best_effort", decompErr.Error(), now().UTC()) {
						return executeBeadIterationOutcome{Stop: true}, nil
					}
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
			// Parent stays open (not proposed) — it is now execution-ineligible
			// and will close once the decomposed children reach terminal state.
			_ = releaseWorkerClaim(w.Store, candidate.ID, assignee)
			if stopAfterNonAttemptSkip() {
				applyStop(work.StopInput{Once: true})
				return executeBeadIterationOutcome{Stop: true}, nil
			}
			return executeBeadIterationOutcome{Continue: true}, nil
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
			operatorOverrideTerminal, _ := detectIntakeBlockedOperatorOverride(w.Store, &candidate, "pre_claim_intake."+strings.TrimSpace(string(intakeOutcome)), intakeResult.Reason, "pre_claim_intake", "best-effort", "attempt", "continue with implementation; review should create follow-up work for remaining gaps")
			emit("pre_claim_intake.blocked", readinessDecisionBody(
				"pre_claim_intake."+strings.TrimSpace(string(intakeOutcome)),
				intakeResult.Reason,
				"pre_claim_intake",
				"best-effort",
				"attempt",
				"continue with implementation; review should create follow-up work for remaining gaps",
				map[string]any{
					"bead_id":           candidate.ID,
					"operator_override": operatorOverrideTerminal,
					"outcome":           string(intakeOutcome),
					"detail":            warning,
				},
			))
			if strictIntakeBlocking() {
				if parked, berr := parkBeadPostIntakeRejection(w.Store, &candidate, assignee, intakeOutcome, intakeResult.Reason, intakeResult.Detail, now().UTC()); berr != nil && runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "readiness park error: %v\n", berr)
				} else if parked {
					_ = releaseWorkerClaim(w.Store, candidate.ID, assignee)
					if stopAfterNonAttemptSkip() {
						applyStop(work.StopInput{Once: true})
						return executeBeadIterationOutcome{Stop: true}, nil
					}
					return executeBeadIterationOutcome{Continue: true}, nil
				}
			} else {
				if appendPreClaimWarn(candidate.ID, "readiness_best_effort", warning, now().UTC()) {
					return executeBeadIterationOutcome{Stop: true}, nil
				}
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
				_ = releaseWorkerClaim(w.Store, candidate.ID, assignee)
				if stopAfterNonAttemptSkip() {
					applyStop(work.StopInput{Once: true})
					return executeBeadIterationOutcome{Stop: true}, nil
				}
				return executeBeadIterationOutcome{Continue: true}, nil
			}
			if appendPreClaimWarn(candidate.ID, "lint_blocked_best_effort", blockMsg, now().UTC()) {
				return executeBeadIterationOutcome{Stop: true}, nil
			}
		}
	}
	refreshClaimedBeforeAttempt := func(claimed bead.Bead) (bead.Bead, *executeBeadIterationOutcome, error) {
		fresh, skip, err := w.refreshClaimedCandidateBeforeAttempt(ctx, claimed)
		if err != nil {
			if releaseErr := releaseWorkerClaim(w.Store, claimed.ID, assignee); releaseErr != nil {
				_ = commitOutcome(ctx, w.Store, claimed.ID, func() error {
					return commitOutcomeError("Unclaim", assignee, result, releaseErr)
				})
				if ctx.Err() != nil {
					outcome := executeBeadIterationOutcome{Stop: true}
					return claimed, &outcome, ctx.Err()
				}
				outcome := executeBeadIterationOutcome{Stop: true}
				return claimed, &outcome, releaseErr
			}
			outcome := executeBeadIterationOutcome{Stop: true}
			return claimed, &outcome, err
		}
		if skip == nil {
			return *fresh, nil, nil
		}
		if releaseErr := releaseWorkerClaim(w.Store, claimed.ID, assignee); releaseErr != nil {
			_ = commitOutcome(ctx, w.Store, claimed.ID, func() error {
				return commitOutcomeError("Unclaim", assignee, result, releaseErr)
			})
			if ctx.Err() != nil {
				outcome := executeBeadIterationOutcome{Stop: true}
				return claimed, &outcome, ctx.Err()
			}
			outcome := executeBeadIterationOutcome{Continue: true}
			return claimed, &outcome, nil
		}
		transientCandidateSkips[claimed.ID] = staleCandidateSkipReason
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "picker skip stale candidate %s before attempt: %s\n", claimed.ID, skip.Detail)
		}
		emitStaleCandidateSkip(emit, claimed.ID, skip, fresh, "pre_attempt")
		if stopAfterNonAttemptSkip() {
			applyStop(work.StopInput{Once: true})
			outcome := executeBeadIterationOutcome{Stop: true}
			return claimed, &outcome, nil
		}
		outcome := executeBeadIterationOutcome{Continue: true}
		return claimed, &outcome, nil
	}

	// Preserve the original stale-state precedence: a bead closed,
	// superseded, or made ineligible during readiness is released before git
	// repair can turn that harmless race into operator attention.
	refreshedCandidate, refreshOutcome, refreshErr := refreshClaimedBeforeAttempt(candidate)
	if refreshOutcome != nil {
		return *refreshOutcome, refreshErr
	}
	candidate = refreshedCandidate

	gitRepair = repairer(ctx, runtime.ProjectRoot)
	if detail, failed := preDispatchGitRepairFailure(gitRepair); failed {
		if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("Unclaim", assignee, result, err)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Stop: true}, err
		}
		stop := &OperatorAttentionStop{
			Reason:      preDispatchGitRepairFailedReason,
			BeadID:      candidate.ID,
			ProjectRoot: runtime.ProjectRoot,
			Message:     "DDx could not repair project git config; resolve the git status failure before restarting ddx work",
		}
		result.OperatorAttention = stop
		setExit("OperatorAttention", "operator_attention")
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log,
				"operator attention: pre-dispatch git repair failed for %s at %s; released bead. %s\n",
				candidate.ID,
				runtime.ProjectRoot,
				detail,
			)
		}
		emit("loop.operator_attention", map[string]any{
			"reason":          stop.Reason,
			"bead_id":         candidate.ID,
			"project_root":    runtime.ProjectRoot,
			"message":         stop.Message,
			"detail":          detail,
			"git_stderr":      gitRepair.StatusStderr,
			"issue_types":     gitRepair.RepairedTypes,
			"repair_commands": gitRepair.Commands,
		})
		return executeBeadIterationOutcome{Stop: true}, nil
	}
	if len(gitRepair.RepairedTypes) > 0 {
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log,
				"repaired project git config before dispatch for %s: %s (%s)\n",
				candidate.ID,
				strings.Join(gitRepair.RepairedTypes, ", "),
				strings.Join(gitRepair.Commands, "; "),
			)
		}
		emit("loop.pre_dispatch_git_repaired", map[string]any{
			"bead_id":          candidate.ID,
			"project_root":     runtime.ProjectRoot,
			"issue_types":      gitRepair.RepairedTypes,
			"repair_commands":  gitRepair.Commands,
			"status_rechecked": gitRepair.StatusSucceeded,
			"stage":            "attempt",
		})
	}

	// This is the final tracker read before an attempt becomes observable.
	// Keep it after readiness, lint, and git repair so a dependency added by
	// any of those slow phases cannot slip into dispatch on a stale snapshot.
	refreshedCandidate, refreshOutcome, refreshErr = refreshClaimedBeforeAttempt(candidate)
	if refreshOutcome != nil {
		return *refreshOutcome, refreshErr
	}
	candidate = refreshedCandidate

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
	attemptCtx := ContextWithReadinessEstimatedDifficulty(ctx, readinessEstimatedDifficulty)
	// A cancelable attempt context so the progress watchdog and the
	// external-close watcher can terminate a wedged or already-landed
	// attempt promptly (ddx-dc23f001).
	attemptCtx, attemptCancel := context.WithCancel(attemptCtx)
	if liveness != nil {
		liveness.SetAttempt(candidate.ID, provAttemptID, string(work.PhaseRunning), "", harness, model, profile, 0)
		// Tick once now so the sidecar shows the new attempt before the
		// first heartbeat fires; long attempts otherwise wait one
		// heartbeat interval before the sidecar reflects the current bead.
		liveness.OnTick(now())
	}
	// Per-attempt route callback: once fizeau resolves the route
	// mid-stream, UpdateRoute overwrites the liveness record so the
	// progress watchdog sees a non-empty route field and does not mistake
	// a healthy long-running attempt for a wedge (ddx-6190edc6).
	routeResolved := make(chan struct{})
	var routeResolvedOnce sync.Once
	executeStarted := make(chan struct{})
	var executeStartedOnce sync.Once
	var executeStartedAt time.Time
	beadIDForRoute := candidate.ID
	routeGuardEnabled := candidate.IssueType != bead.IssueTypeReviewFinding && candidate.IssueType != bead.IssueTypeAlignmentReview
	onExecuteStart := func() {
		executeStartedOnce.Do(func() {
			// Use the process's monotonic clock for the route deadline. w.Now is an
			// audit/event clock and may be fixed in tests; it must not control a
			// real cancellation deadline. Closing executeStarted publishes this
			// timestamp to the guard without sharing a live timer across goroutines.
			executeStartedAt = time.Now()
			close(executeStarted)
		})
	}
	onRouteResolved := func(h, p, m string) {
		routeResolvedOnce.Do(func() { close(routeResolved) })
		if liveness != nil {
			liveness.UpdateRoute(beadIDForRoute, h, m, p)
		}
	}
	attemptStarted = true

	// Progress watchdog + external-close watcher run alongside WithHeartbeat
	// for the lifetime of this attempt. The watchdog fires when phase-empty
	// heartbeats (harness/model/route all empty) persist past the phase
	// budget — an 80-minute wedge that the liveness TTL alone treats as
	// healthy (ddx-8f2e0ebf criterion B). The external-close watcher fires
	// when a parallel attempt closes the same bead, so a wedged worker stops
	// holding a stale lease. The first guard to fire releases the lease;
	// term carries the reason so the loop skips normal failure escalation.
	term := &attemptTermination{}
	var guardWG sync.WaitGroup
	candidateID := candidate.ID
	if routeGuardEnabled {
		guardWG.Add(1)
		go func() {
			defer guardWG.Done()
			if runtime.routeGuardStartGate != nil {
				select {
				case <-runtime.routeGuardStartGate:
				case <-attemptCtx.Done():
					return
				}
			}
			// DDx-local worktree/prompt setup is outside the route-stage budget.
			// Derive the remaining duration from the absolute Execute deadline so
			// delayed goroutine scheduling cannot grant Fizeau a fresh budget.
			select {
			case <-executeStarted:
			case <-routeResolved:
				return
			case <-attemptCtx.Done():
				return
			}
			timeout := runtime.effectiveRouteResolutionTimeout()
			remaining := time.Until(executeStartedAt.Add(timeout))
			if remaining < 0 {
				remaining = 0
			}
			if runtime.routeGuardRemainingObserver != nil {
				runtime.routeGuardRemainingObserver(remaining)
			}
			routeStageTimer := time.NewTimer(remaining)
			defer routeStageTimer.Stop()
			select {
			case <-routeResolved:
				return
			case <-attemptCtx.Done():
				return
			case <-routeStageTimer.C:
				// Prefer a concurrently delivered routing_decision over the
				// deadline tick; after that event this guard has no authority.
				select {
				case <-routeResolved:
					return
				default:
				}
				if term.set(FailureModeRouteResolutionTimeout) {
					// Stop the active Fizeau attempt as soon as the Execute-relative
					// deadline wins. Durable lease release and operator-attention
					// bookkeeping can block on tracker I/O, but must not extend the
					// expired execution budget. guardWG still keeps the worker alive
					// until that bookkeeping completes.
					attemptCancel()
					_, releaseErr := routeResolutionTimeoutReport(
						w.Store,
						candidateID,
						assignee,
						provAttemptID,
						now().UTC(),
						timeout,
						executeStartedAt.UTC(),
					)
					if releaseErr != nil {
						// A failed release is not a handled termination. Preserve the
						// error and clear the reason so ordinary attempt cleanup still
						// gets a chance to release the claim.
						term.releaseFailed(FailureModeRouteResolutionTimeout, releaseErr)
					}
				}
			}
		}()
	}
	guardWG.Add(1)
	go func() {
		defer guardWG.Done()
		work.RunExternalCloseWatcher(attemptCtx, heartbeatInterval, work.ExternalCloseWatcherConfig{
			IsClosed: func() (bool, error) { return beadClosedExternally(w.Store, candidateID) },
			OnClosed: func() {
				if term.set("external_close") {
					releaseClosedExternally(w.Store, candidateID, assignee, now().UTC())
					attemptCancel()
				}
			},
		})
	}()
	if liveness != nil {
		guardWG.Add(1)
		go func() {
			defer guardWG.Done()
			work.RunProgressWatchdog(attemptCtx, heartbeatInterval, work.ProgressWatchdogConfig{
				Budgets:  work.DefaultPhaseBudgets(),
				Snapshot: liveness.Snapshot,
				Now:      now,
				OnWedged: func(rec workerstatus.LivenessRecord, budget time.Duration) {
					if term.set("progress_watchdog") {
						flagWedgedForOperatorAttention(w.Store, candidateID, assignee, rec.AttemptID, rec.Phase, rec.LastActivityAt, budget, now().UTC())
						attemptCancel()
					}
				},
			})
		}()
	}
	verificationRunner := w.VerificationRunner
	if verificationRunner == nil {
		verificationRunner = defaultVerificationCommandRunnerForConfig(rcfg)
	}

	// Dispatch system kinds (review-finding, alignment-review) to dedicated handlers
	// instead of the normal attempt flow.
	var attemptOut agenttry.Outcome
	var dispatchErr error
	if candidate.IssueType == bead.IssueTypeReviewFinding || candidate.IssueType == bead.IssueTypeAlignmentReview {
		sysReport, handled, sysErr := systemKindDispatcher(attemptCtx, candidate.ID, &candidate, w.Store, runtime.ProjectRoot, assignee, now)
		if sysErr != nil {
			attemptOut = agenttry.Outcome{
				Disposition: agenttry.OutcomeReported,
				Report: agenttry.Report{
					SessionID: provAttemptID,
					BeadID:    candidate.ID,
					BaseRev:   "",
					ResultRev: "",
					Status:    sysReport.Status,
					Error:     fmt.Sprintf("system kind dispatch: %v", sysErr),
				},
			}
		} else if handled {
			attemptOut = agenttry.Outcome{
				Disposition: agenttry.OutcomeSuccess,
				Report: agenttry.Report{
					SessionID: provAttemptID,
					BeadID:    candidate.ID,
					BaseRev:   "",
					ResultRev: "",
					Status:    sysReport.Status,
					Detail:    sysReport.Detail,
				},
			}
		} else {
			// Not a system kind, fall through to normal attempt
			attemptOut, dispatchErr = work.WithHeartbeat(attemptCtx, candidate.ID, heartbeatInterval, w.Store, liveness, func() (agenttry.Outcome, error) {
				return agenttry.Attempt(attemptCtx, w.Store, candidate.ID, agenttry.AttemptOpts{
					Bead:                candidate,
					Executor:            tryExecutor(w.Executor, onExecuteStart, onRouteResolved),
					Store:               w.Store,
					ProjectRoot:         runtime.ProjectRoot,
					SatisfactionChecker: w.SatisfactionChecker,
					VerificationRunner:  verificationRunner,
					AutoRecover:         tryAutoRecover(w.conflictAutoRecoverFn),
					ConflictResolver:    w.ConflictResolver,
					Assignee:            assignee,
					Now:                 now,
					Cooldown:            LandConflictCooldown,
					RateLimitOnRetry: func(_ context.Context, info agenttry.RateLimitRetryInfo) {
						appendRateLimitRetryEvent(w.Store, candidate.ID, fromTryRateLimitRetryInfo(info))
					},
				})
			})
		}
	} else {
		attemptOut, dispatchErr = work.WithHeartbeat(attemptCtx, candidate.ID, heartbeatInterval, w.Store, liveness, func() (agenttry.Outcome, error) {
			return agenttry.Attempt(attemptCtx, w.Store, candidate.ID, agenttry.AttemptOpts{
				Bead:                candidate,
				Executor:            tryExecutor(w.Executor, onExecuteStart, onRouteResolved),
				Store:               w.Store,
				ProjectRoot:         runtime.ProjectRoot,
				SatisfactionChecker: w.SatisfactionChecker,
				VerificationRunner:  verificationRunner,
				AutoRecover:         tryAutoRecover(w.conflictAutoRecoverFn),
				ConflictResolver:    w.ConflictResolver,
				Assignee:            assignee,
				Now:                 now,
				Cooldown:            LandConflictCooldown,
				RateLimitOnRetry: func(_ context.Context, info agenttry.RateLimitRetryInfo) {
					appendRateLimitRetryEvent(w.Store, candidate.ID, fromTryRateLimitRetryInfo(info))
				},
			})
		})
	}
	// The attempt has returned. Stop the guards and wait for any in-flight
	// OnWedged/OnClosed callback (lease release + event emit) to finish
	// before reading the termination reason.
	attemptCancel()
	guardWG.Wait()
	err = dispatchErr
	if routeGuardErr := term.err(); routeGuardErr != nil {
		err = errors.Join(err, routeGuardErr)
	}
	if liveness != nil {
		liveness.ClearAttempt()
		liveness.OnTick(now())
	}
	// A guard terminated this attempt: the lease was already released and,
	// for a wedge, operator attention was flagged. Skip normal report
	// processing so a context-cancelled attempt is not re-escalated as a
	// failure (ddx-dc23f001). The bead is either closed (external close) or
	// released to open with an operator_attention event (wedge).
	if termReason := term.get(); termReason != "" {
		if termReason == FailureModeRouteResolutionTimeout {
			// Keep draining the queue, but do not immediately reclaim the same
			// bead after releasing its timed-out lease. The durable wedge marker
			// remains for a later Run invocation and the consecutive-wedge guard.
			transientCandidateSkips[candidate.ID] = routeStageTimeoutSkipReason
		}
		emit("loop.attempt_terminated", map[string]any{
			"bead_id": candidate.ID,
			"reason":  termReason,
		})
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "attempt %s terminated by %s; lease released\n", candidate.ID, termReason)
		}
		return executeBeadIterationOutcome{Continue: true}, nil
	}
	// The attempt ran to completion without a wedge guard firing — the route
	// resolved and the agent executed (real progress). Reset any
	// consecutive-wedge marker so a single transient wedge does not
	// permanently sideline a bead that subsequently makes progress
	// (ddx-9714eaac AC #2).
	if term.err() == nil {
		clearConsecutiveWedge(w.Store, candidate.ID)
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
	if routeGuardErr := term.err(); routeGuardErr != nil {
		report.Error = routeGuardErr.Error()
		report.Detail = routeGuardErr.Error()
	}
	classifyLoopReportFailure(&report)
	if gitRepairStop, detail, ok := preDispatchGitRepairStop(report, err, runtime.ProjectRoot, candidate.ID); ok {
		if unclaimErr := releaseWorkerClaim(w.Store, candidate.ID, assignee); unclaimErr != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("Unclaim", assignee, result, unclaimErr)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Stop: true}, unclaimErr
		}
		result.OperatorAttention = gitRepairStop
		setExit("OperatorAttention", "operator_attention")
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log,
				"operator attention: pre-dispatch git repair failed for %s at %s; released bead. %s\n",
				candidate.ID,
				runtime.ProjectRoot,
				detail,
			)
		}
		emit("loop.operator_attention", map[string]any{
			"reason":       gitRepairStop.Reason,
			"bead_id":      candidate.ID,
			"project_root": runtime.ProjectRoot,
			"message":      gitRepairStop.Message,
			"detail":       detail,
			"git_stderr":   detail,
		})
		return executeBeadIterationOutcome{Stop: true}, nil
	}
	if checkpointDirty, ok := preExecuteCheckpointDirtyStop(report, err, runtime.ProjectRoot, candidate.ID); ok {
		if unclaimErr := releaseWorkerClaim(w.Store, candidate.ID, assignee); unclaimErr != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("Unclaim", assignee, result, unclaimErr)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Stop: true}, unclaimErr
		}
		detail := strings.TrimSpace(firstNonEmpty(report.Detail, report.Error))
		if detail == "" && err != nil {
			detail = strings.TrimSpace(err.Error())
		}
		if loopMode == executeloop.ModeWatch && runtime.ProjectRoot != "" {
			preserveDirty := w.preDispatchDirtyPreserver
			if preserveDirty == nil {
				preserveDirty = preservePreDispatchDirtyPaths
			}
			if preserved, preserveErr := preserveDirty(runtime.ProjectRoot, checkpointDirty.DirtyPaths); preserveErr == nil && preserved != nil {
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log,
						"preserved pre-dispatch implementation changes in %s under %s; released %s and continuing watch. dirty paths: %s. recover with %s\n",
						runtime.ProjectRoot,
						preserved.PreserveRef,
						candidate.ID,
						strings.Join(preserved.DirtyPaths, ", "),
						preserved.RecoverCommand,
					)
				}
				emit("loop.pre_dispatch_dirty_preserved", map[string]any{
					"reason":          checkpointDirty.Reason,
					"bead_id":         candidate.ID,
					"project_root":    runtime.ProjectRoot,
					"dirty_paths":     preserved.DirtyPaths,
					"preserve_ref":    preserved.PreserveRef,
					"recover_command": preserved.RecoverCommand,
					"detail":          detail,
				})
				return executeBeadIterationOutcome{Continue: true}, nil
			}
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
		return executeBeadIterationOutcome{Stop: true}, nil
	}
	if report.Status == ExecuteBeadStatusNoEvidenceProduced {
		result.Attempts++
		result.Results = append(result.Results, report)
		result.Failures++
		result.LastFailureStatus = report.Status
		if unclaimErr := releaseWorkerClaim(w.Store, candidate.ID, assignee); unclaimErr != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("Unclaim", assignee, result, unclaimErr)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Stop: true}, unclaimErr
		}
		at := now().UTC()
		detail := strings.TrimSpace(firstNonEmpty(report.Detail, report.Error))
		if detail == "" {
			detail = "agent exited without a commit or no_changes_rationale.txt"
		}
		if err := parkNoEvidenceForOperator(w.Store, candidate.ID, assignee, report, detail, at); err != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("parkNoEvidenceForOperator", assignee, result, err)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Stop: true}, err
		}
		attention := &OperatorAttentionStop{
			Reason:      FailureModeNoEvidenceProduced,
			BeadID:      candidate.ID,
			ProjectRoot: runtime.ProjectRoot,
			DirtyPaths:  append([]string(nil), report.NoEvidencePaths...),
			Message:     detail,
		}
		result.OperatorAttention = attention
		setExit("OperatorAttention", "operator_attention")
		if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, at)); err != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("AppendEvent", assignee, result, err)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Stop: true}, err
		}
		if finalizeDurableAuditOrStop(candidate.ID, report) {
			return executeBeadIterationOutcome{Stop: true}, nil
		}
		emit("loop.operator_attention", map[string]any{
			"reason":       attention.Reason,
			"bead_id":      candidate.ID,
			"project_root": runtime.ProjectRoot,
			"dirty_paths":  attention.DirtyPaths,
			"preserve_ref": report.PreserveRef,
			"message":      detail,
		})
		emit("bead.result", map[string]any{
			"bead_id":           candidate.ID,
			"status":            report.Status,
			"detail":            report.Detail,
			"session_id":        report.SessionID,
			"result_rev":        report.ResultRev,
			"base_rev":          report.BaseRev,
			"preserve_ref":      report.PreserveRef,
			"no_evidence_paths": append([]string(nil), report.NoEvidencePaths...),
			"duration_ms":       now().Sub(runStart).Milliseconds(),
		})
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "operator attention: %s produced no evidence; released and parked. %s\n", candidate.ID, detail)
		}
		return executeBeadIterationOutcome{Stop: true}, nil
	}
	if IsResourceExhaustedStatus(report.Status) {
		result.Attempts++
		setExit("ResourceExhausted", "resource_exhausted")
		if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("Unclaim", assignee, result, err)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Stop: true}, err
		}
		emitResourceExhausted(emit, w.Store, candidate.ID, report, assignee, now().UTC())
		result.Results = append(result.Results, report)
		result.Failures++
		result.LastFailureStatus = report.Status
		if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC())); err != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("AppendEvent", assignee, result, err)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Stop: true}, err
		}
		if finalizeDurableAuditOrStop(candidate.ID, report) {
			return executeBeadIterationOutcome{Stop: true}, nil
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
		return executeBeadIterationOutcome{Stop: true}, nil
	}
	if isRoutingInfrastructureReport(report) {
		result.Attempts++
		report.OutcomeReason = FailureModeNoViableProvider
		report.Disrupted = true
		report.DisruptionReason = "routing"
		serverOutageActivated := false
		if activated, _ := serverOutage.Record(report, candidate.ID, now().UTC()); activated {
			serverOutageActivated = true
			pausedInfraUntil = time.Time{}
		}
		if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("Unclaim", assignee, result, err)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Stop: true}, err
		}
		emitDisruptionDetected(emit, w.Store, candidate.ID,
			report.DisruptionReason, report.Detail, report.Harness, report.Model, assignee, now().UTC())
		appendExecutionRoutingIntentEvidence(w.Store, candidate, report, now().UTC())
		result.Results = append(result.Results, report)
		result.Failures++
		result.LastFailureStatus = report.Status
		if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC())); err != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("AppendEvent", assignee, result, err)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Stop: true}, err
		}
		if finalizeDurableAuditOrStop(candidate.ID, report) {
			return executeBeadIterationOutcome{Stop: true}, nil
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
		// In watch mode a single bead that cannot route (no_viable_provider)
		// must not terminate an otherwise-healthy long-running drain
		// (ddx-a827d07f): skip it for the rest of the pass and keep draining
		// other ready beads. The bead is left open and untouched (no
		// park/cooldown) so it stays immediately re-claimable; the in-memory
		// skip clears when the queue next drains empty, giving routing a
		// fresh retry. Once and drain modes retain the existing
		// stop-on-routing-failure contract.
		if loopMode == executeloop.ModeWatch {
			if !serverOutageActivated {
				transientCandidateSkips[candidate.ID] = routingUnavailableSkipReason
			}
			return executeBeadIterationOutcome{Continue: true}, nil
		}
		setExit("RoutingUnavailable", "routing_unavailable")
		return executeBeadIterationOutcome{Stop: true}, nil
	}
	// Per-bead cost budget exhausted: unclaim the bead (immediately
	// re-claimable), emit the TD-031 §5 per-bead-budget-exhausted event,
	// and increment the consecutive_ladder_exhaustions counter so the
	// auto-recovery hook (sister bead ddx-63155d5c) can fire when the
	// threshold is exceeded. No cooldown is set per ADR-024 Per-Bead Budget.
	if isPerBeadBudgetExhaustedReport(report) {
		result.Attempts++
		if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("Unclaim", assignee, result, err)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Stop: true}, err
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
				_ = parkToProposedSimple(w.Store, candidate.ID, bead.ParkLadderExhaustionManual, "recovery:manual label set", now().UTC())
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
		result.Failures++
		result.LastFailureStatus = report.Status
		if finalizeDurableAuditOrStop(candidate.ID, report) {
			return executeBeadIterationOutcome{Stop: true}, nil
		}
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
		return executeBeadIterationOutcome{Continue: true}, nil
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
		if unclaimErr := releaseWorkerClaim(w.Store, candidate.ID, assignee); unclaimErr != nil && runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "interrupted attempt cleanup error (Unclaim %s): %v\n", candidate.ID, unclaimErr)
		}
		if finalizeDurableAuditOrStop(candidate.ID, report) {
			return executeBeadIterationOutcome{Stop: true}, ctxErr
		}
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "interrupted attempt released %s without recording terminal outcome: %v\n", candidate.ID, ctxErr)
		}
		return executeBeadIterationOutcome{Stop: true}, ctxErr
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
			if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("Unclaim", assignee, result, err)
				})
				if ctx.Err() != nil {
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
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
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
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
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Continue: true}, nil
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
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Continue: true}, nil
		}
		appendLoopRoutingEvidence(w.Store, candidate, report, now().UTC())
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
		// remains available only through an explicit runtime opt-in.
		if runtime.PostMergeReview {
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
			if reviewOut.StoreErr != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError(reviewOut.StoreErrOp, assignee, result, reviewOut.StoreErr)
				})
				if ctx.Err() != nil {
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
			}
			if reviewOut.Approved {
				result.Successes++
				result.LastSuccessAt = now().UTC()
			} else {
				result.Failures++
				result.LastFailureStatus = report.Status
			}
		} else {
			closeBlocked := false
			if successReportHasEmptyReviewResult(report) && !hasDurableReviewSkipReason(candidate.Labels) {
				reason := "empty review result before close"
				appendReviewGateError(w.Store, candidate.ID, assignee, now().UTC(), report.ResultRev, evidence.OutcomeReviewUnparseable, reason)
				report.Status = ExecuteBeadStatusReviewMalfunction
				report.Detail = "pre-close review: " + reason
				if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("Unclaim", assignee, result, err)
					})
					if ctx.Err() != nil {
						return executeBeadIterationOutcome{Stop: true}, ctx.Err()
					}
					return executeBeadIterationOutcome{Continue: true}, nil
				}
				result.Failures++
				result.LastFailureStatus = report.Status
				closeBlocked = true
			}
			if !closeBlocked {
				if err := w.Store.CloseWithEvidence(candidate.ID, report.SessionID, report.ResultRev); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("CloseWithEvidence", assignee, result, err)
					})
					if ctx.Err() != nil {
						return executeBeadIterationOutcome{Stop: true}, ctx.Err()
					}
					return executeBeadIterationOutcome{Continue: true}, nil
				}
				result.Successes++
				result.LastSuccessAt = now().UTC()
			}
		}
	} else if attemptOut.Disposition == agenttry.OutcomePark {
		result.Failures++
		result.LastFailureStatus = report.Status
	} else if attemptOut.NoChanges != nil {
		noChanges := attemptOut.NoChanges
		if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("Unclaim", assignee, result, err)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Continue: true}, nil
		}

		// Post-attempt orchestrator decomposition: when the implementation
		// attempt signals orchestrator_action: decompose (because it hit the
		// implementation depth cap or the bead is too large for the worktree),
		// invoke the queue-level splitter. The orchestrator checks the
		// queue-level max_decomposition_depth, not the implementation cap.
		if report.NoChangesRationale != "" && runtime.PostAttemptDecompositionHook != nil {
			parsed := ParseNoChangesRationale(report.NoChangesRationale)
			if parsed.OrchestratorAction == "decompose" {
				decompDecision := w.handlePostAttemptDecomposition(ctx, &candidate, runtime, assignee, rcfg, now().UTC())
				if decompDecision.BackEdgeDetected {
					sawParentBackEdge = true
				}
				report.OutcomeReason = "decomposed"
				report.DecomposedChildIDs = append([]string(nil), decompDecision.ChildIDs...)
				report.ExecutionDecision = decompDecision.ExecutionDecision
				attachDecisionAuditTraceIfMissing(&report)
				result.Failures++
				result.LastFailureStatus = report.Status
				result.Results = append(result.Results, report)
				if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC())); err != nil && runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "outcome store error (AppendEvent %s): %v (continuing)\n", candidate.ID, err)
				}
				if finalizeDurableAuditOrStop(candidate.ID, report) {
					return executeBeadIterationOutcome{Stop: true}, nil
				}
				return executeBeadIterationOutcome{Continue: true}, nil
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
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
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
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
			}
			if err := w.Store.CloseWithEvidence(candidate.ID, report.SessionID, report.BaseRev); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("CloseWithEvidence", assignee, result, err)
				})
				if ctx.Err() != nil {
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
			}
			result.Successes++
			result.LastSuccessAt = now().UTC()
		case agenttry.NoChangesActionKeepOpenSmartRetry:
			if err := applyNoChangesSmartRetry(w.Store, candidate.ID, assignee, noChanges, report.ActualPower, w.EscalationNextFloor); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("applyNoChangesSmartRetry", assignee, result, err)
				})
				if ctx.Err() != nil {
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
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
						return executeBeadIterationOutcome{Stop: true}, ctx.Err()
					}
					return executeBeadIterationOutcome{Continue: true}, nil
				}
				report.RetryAfter = retryAfter.Format(time.RFC3339)
			}
			result.Failures++
			result.LastFailureStatus = report.Status
		case agenttry.NoChangesActionBadAttemptNoCooldown:
			emitReviewerSkippedEmptyDiff(w.Store, candidate.ID, assignee, now().UTC())
			if err := applyNoChangesBadAttemptEscalation(w.Store, candidate.ID, assignee, noChanges, report.ActualPower, w.EscalationNextFloor); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("applyNoChangesBadAttemptEscalation", assignee, result, err)
				})
				if ctx.Err() != nil {
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
			}
			result.Failures++
			result.LastFailureStatus = report.Status
		case agenttry.NoChangesActionOperatorRequired:
			if err := applyNoChangesOperatorRequired(w.Store, candidate.ID, assignee, noChanges, now().UTC()); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("applyNoChangesOperatorRequired", assignee, result, err)
				})
				if ctx.Err() != nil {
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
			}
			result.Failures++
			result.LastFailureStatus = report.Status
		case agenttry.NoChangesActionBlockedExternal:
			if err := applyNoChangesBlockedExternal(w.Store, candidate.ID, assignee, noChanges); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("applyNoChangesBlockedExternal", assignee, result, err)
				})
				if ctx.Err() != nil {
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
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
						return executeBeadIterationOutcome{Stop: true}, ctx.Err()
					}
					return executeBeadIterationOutcome{Continue: true}, nil
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
			if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("Unclaim", assignee, result, err)
				})
				if ctx.Err() != nil {
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
			}
		}
		if report.Status == ExecuteBeadStatusPreservedNeedsReview {
			if err := w.Store.AppendNotes(candidate.ID, preservedNeedsReviewNote(report)); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("AppendNotes", assignee, result, err)
				})
				if ctx.Err() != nil {
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
			}
			if isLargeDeletionGateDetail(report.Detail) {
				if err := stampPreservedReviewBlockMarkers(ctx, w.Store, candidate.ID, report, now().UTC()); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("stampPreservedReviewBlockMarkers", assignee, result, err)
					})
					if ctx.Err() != nil {
						return executeBeadIterationOutcome{Stop: true}, ctx.Err()
					}
					return executeBeadIterationOutcome{Continue: true}, nil
				}
			}
		} else if report.Status == ExecuteBeadStatusRepairCycleExhausted {
			if err := applyRepairCycleExhaustedEscalation(w.Store, candidate.ID, assignee, report.ActualPower, now(), w.EscalationNextFloor); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("applyRepairCycleExhaustedEscalation", assignee, result, err)
				})
				if ctx.Err() != nil {
					return executeBeadIterationOutcome{Stop: true}, ctx.Err()
				}
				return executeBeadIterationOutcome{Continue: true}, nil
			}
			result.Failures++
			result.LastFailureStatus = report.Status
		} else if report.Status == ExecuteBeadStatusLandRetry {
			report.OutcomeReason = FailureModeLandRetry
			report.Disrupted = true
			report.DisruptionReason = FailureModeLandRetry
			_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
				Kind:    "land-coordination",
				Summary: "retry-land",
				Body: strings.Join([]string{
					"action=retry-land",
					"detail=" + report.Detail,
					"result_rev=" + report.ResultRev,
					"base_rev=" + report.BaseRev,
					decisionAuditEventBodyLine(report),
				}, "\n"),
				Actor:     assignee,
				Source:    "ddx work",
				CreatedAt: now().UTC(),
			})
			result.Failures++
			result.LastFailureStatus = report.Status
		} else if report.Status == ExecuteBeadStatusLandOperatorAttention {
			report.OutcomeReason = FailureModeLandOperatorAttention
			reason := report.Detail
			if strings.TrimSpace(reason) == "" {
				reason = FailureModeLandOperatorAttention
			}
			if parkErr := parkToProposedSimple(w.Store, candidate.ID, bead.ParkAutoRecoveryFailed, reason, now().UTC()); parkErr != nil && runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "land coordination operator-attention park failed for %s: %v (continuing)\n", candidate.ID, parkErr)
			}
			_ = w.Store.AppendEvent(candidate.ID, bead.BeadEvent{
				Kind:    "operator_attention",
				Summary: FailureModeLandOperatorAttention,
				Body: strings.Join([]string{
					"action=operator-attention",
					"detail=" + report.Detail,
					"result_rev=" + report.ResultRev,
					"base_rev=" + report.BaseRev,
					decisionAuditEventBodyLine(report),
				}, "\n"),
				Actor:     assignee,
				Source:    "ddx work",
				CreatedAt: now().UTC(),
			})
			result.Failures++
			result.LastFailureStatus = report.Status
		} else {
			if isProviderConnectivityFailureReport(report) {
				report.OutcomeReason = FailureModeProviderConnectivity
				report.Disrupted = true
				report.DisruptionReason = "provider_connectivity"
				emitRouteFailureEvent(w.Store, candidate.ID, assignee, report, now().UTC())
			} else if isNoViableProviderReport(report) {
				report.OutcomeReason = FailureModeNoViableProvider
				report.Disrupted = true
				report.DisruptionReason = "no_viable_provider"
				serverOutageActivated := false
				if activated, _ := serverOutage.Record(report, candidate.ID, now().UTC()); activated {
					serverOutageActivated = true
					pausedInfraUntil = time.Time{}
				} else {
					// Transition the worker to paused-infra: leave every bead
					// immediately reclaimable, pause this worker for PausedInfraInterval,
					// then re-evaluate the full queue (P6 + ADR-024 §Infra Fallback).
					pausedInfraUntil = now().UTC().Add(PausedInfraInterval)
				}
				if loopMode == executeloop.ModeWatch {
					if !serverOutageActivated {
						transientCandidateSkips[candidate.ID] = routingUnavailableSkipReason
					}
					return executeBeadIterationOutcome{Continue: true}, nil
				}
			} else {
				serverOutageActivated := false
				if activated, _ := serverOutage.Record(report, candidate.ID, now().UTC()); activated {
					serverOutageActivated = true
					pausedInfraUntil = time.Time{}
				}
				if !serverOutageActivated {
					report = w.runPostAttemptTriage(ctx, candidate, report, runtime, assignee, now)
					if report.Detail == mixedCommitAndNoChangesRationaleReason && report.NoChangesRationale != "" {
						parsed := ParseNoChangesRationale(report.NoChangesRationale)
						if parsed.OrchestratorAction == "decompose" {
							report.OutcomeReason = "decomposed"
							if runtime.PostAttemptDecompositionHook != nil {
								decompDecision := w.handlePostAttemptDecomposition(ctx, &candidate, runtime, assignee, rcfg, now().UTC())
								if decompDecision.BackEdgeDetected {
									sawParentBackEdge = true
								}
								report.DecomposedChildIDs = append([]string(nil), decompDecision.ChildIDs...)
								report.ExecutionDecision = decompDecision.ExecutionDecision
							} else if parkErr := markExecutionIneligible(w.Store, candidate.ID); parkErr != nil && runtime.Log != nil {
								_, _ = fmt.Fprintf(runtime.Log, "mixed_commit decomposition park failed for %s: %v (continuing)\n", candidate.ID, parkErr)
							} else {
								report.ExecutionDecision = "execution_ineligible"
							}
							attachDecisionAuditTraceIfMissing(&report)
							result.Failures++
							result.LastFailureStatus = report.Status
							result.Results = append(result.Results, report)
							if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC())); err != nil && runtime.Log != nil {
								_, _ = fmt.Fprintf(runtime.Log, "outcome store error (AppendEvent %s): %v (continuing)\n", candidate.ID, err)
							}
							if finalizeDurableAuditOrStop(candidate.ID, report) {
								return executeBeadIterationOutcome{Stop: true}, nil
							}
							return executeBeadIterationOutcome{Continue: true}, nil
						}
					}
					if shouldSuppressNoProgress(report) {
						retryAfter := now().UTC().Add(CapLoopCooldown(noProgressCooldown))
						if err := w.Store.SetExecutionCooldown(candidate.ID, retryAfter, report.Status, report.Detail, report.BaseRev); err != nil {
							_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
								return commitOutcomeError("SetExecutionCooldown", assignee, result, err)
							})
							if ctx.Err() != nil {
								return executeBeadIterationOutcome{Stop: true}, ctx.Err()
							}
							return executeBeadIterationOutcome{Continue: true}, nil
						}
						report.RetryAfter = retryAfter.Format(time.RFC3339)
					}
					if report.Detail == mixedCommitAndNoChangesRationaleReason &&
						countRecentMixedCommitEvents(w.Store, candidate.ID, mixedCommitCooldownWindow, now().UTC()) >= 1 {
						if parkErr := parkToProposedSimple(w.Store, candidate.ID, bead.ParkNoChangesOperatorRequired,
							"circuit-breaker: "+mixedCommitAndNoChangesRationaleReason+" repeated within 24h; operator review required",
							now().UTC()); parkErr != nil && runtime.Log != nil {
							_, _ = fmt.Fprintf(runtime.Log, "mixed_commit circuit-breaker park failed for %s: %v (continuing)\n", candidate.ID, parkErr)
						}
					}
				}
			}
			result.Failures++
			result.LastFailureStatus = report.Status
		}
	}

	if overrideMeta.Active {
		if err := restoreForcedCooldownMetadata(context.Background(), w.Store, candidate.ID, overrideMeta); err != nil {
			_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
				return commitOutcomeError("restoreForcedCooldownMetadata", assignee, result, err)
			})
			if ctx.Err() != nil {
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Continue: true}, nil
		}
		if report.Status != ExecuteBeadStatusSuccess && report.Status != ExecuteBeadStatusAlreadySatisfied {
			report.RetryAfter = overrideRetryAfter
		}
	}

	result.Results = append(result.Results, report)

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
				return executeBeadIterationOutcome{Stop: true}, ctx.Err()
			}
			return executeBeadIterationOutcome{Continue: true}, nil
		}
	}
	if finalizeDurableAuditOrStop(candidate.ID, report) {
		return executeBeadIterationOutcome{Stop: true}, nil
	}
	if runtime.TrackerSyncEnabled && runtime.ProjectRoot != "" && (report.Status == ExecuteBeadStatusSuccess || report.Status == ExecuteBeadStatusAlreadySatisfied) {
		syncTrackerAfterClose(ctx, runtime.ProjectRoot, candidate.ID, runtime.Log, emit)
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
				return executeBeadIterationOutcome{Stop: true}, err
			}
			return executeBeadIterationOutcome{Continue: true}, nil
		}
	}

	if loopMode == executeloop.ModeOnce {
		applyStop(work.StopInput{Once: true})
		return executeBeadIterationOutcome{Stop: true}, nil
	}
	return executeBeadIterationOutcome{Continue: true}, nil
}

// runEpicClosureCascade closes any epics whose children have all reached a
// terminal state (FEAT-004 §Queue Semantics For Epics). Returns the count of
// epics closed. Idempotent: a second call closes nothing new.
func (w *ExecuteBeadWorker) runEpicClosureCascade(ctx context.Context, emit func(string, map[string]any)) (int, error) {
	closer, ok := w.Store.(epicCloser)
	if !ok {
		return 0, nil
	}
	candidates, err := closer.EpicClosureCandidates(ctx)
	if err != nil || len(candidates) == 0 {
		return 0, err
	}
	closed := 0
	for _, epic := range candidates {
		if err := closer.Close(ctx, epic.ID); err == nil {
			closed++
			emit("loop.epic_auto_closed", map[string]any{
				"bead_id": epic.ID,
				"title":   epic.Title,
			})
		}
	}
	return closed, nil
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
//	label_filter, in_attempted, claim_race, eligibility_filter, retry_cooldown, stale_candidate
//
// Note: eligibility_filter and retry_cooldown are applied upstream in
// Store.ReadyExecution (so beads filtered for those reasons never reach
// nextCandidate at all). stale_candidate is the transient post-selection guard
// used when a previously returned queue row disappears on a fresh store read.
// These remain part of the reason vocabulary so future picker rearrangements
// can re-emit them without changing the schema.
type pickerSkip struct {
	BeadID   string
	Priority int
	BeadRank *int
	Reason   string
}

const staleCandidateSkipReason = "stale_candidate"

// routingUnavailableSkipReason marks a bead transiently skipped for the rest of
// a drain pass after a per-bead no_viable_provider routing failure, so the
// watcher keeps draining instead of exiting (ddx-a827d07f).
const routingUnavailableSkipReason = "routing_unavailable"

// routeStageTimeoutSkipReason keeps a route-wedged bead out of the remainder
// of the current drain pass after its lease is released. This prevents an
// immediate same-worker reclaim loop while preserving the durable wedge marker
// for later Run invocations.
const routeStageTimeoutSkipReason = "route_stage_timeout"

func hasGuardSkips(skips []pickerSkip) bool {
	for _, skip := range skips {
		switch skip.Reason {
		case "label_filter", "in_attempted", "claim_race", "eligibility_filter", "retry_cooldown", "target_bead", staleCandidateSkipReason, routeStageTimeoutSkipReason:
			continue
		default:
			return true
		}
	}
	return false
}

// preClaimIdleReasonSystemic and preClaimIdleReasonTrackerContention are the
// loop.idle reason codes for the two non-cooldown pre-claim skip classes.
// Tracker contention is distinguished from systemic so operators can tell
// transient multi-worker tracker churn apart from a real repo-config wedge
// (ddx-df77e668).
const (
	preClaimIdleReasonSystemic          = "preclaim_systemic"
	preClaimIdleReasonTrackerContention = "preclaim_tracker_contention"
)

// preClaimIdleEscalationThreshold is the number of consecutive loop.idle cycles
// with an identical pre-claim blocker detail after which the worker emits a
// non-terminal operator-attention event (ddx-df77e668 AC #3).
const preClaimIdleEscalationThreshold = 5

// preClaimIdleSkip reports whether the picker's skips are entirely
// transient/eligibility skips plus at least one non-cooldown pre-claim skip
// (systemic or tracker-contention). When ok is true the loop idles without
// cooldowning any bead. detail is the human-readable blocker, reasonCode is the
// loop.idle reason, and beadID names a representative skipped bead. If any skip
// is a bead-specific failure, ok is false so the caller handles it elsewhere.
// Systemic takes precedence over tracker contention when both are present.
func preClaimIdleSkip(skips []pickerSkip) (detail, reasonCode, beadID string, ok bool) {
	for _, skip := range skips {
		switch skip.Reason {
		case "label_filter", "in_attempted", "claim_race", "eligibility_filter", "retry_cooldown", "target_bead", staleCandidateSkipReason:
			continue
		}
		switch {
		case work.IsSystemicPreClaimSkipReason(skip.Reason):
			reasonCode = preClaimIdleReasonSystemic
			detail = work.SystemicPreClaimDetail(skip.Reason)
			if beadID == "" {
				beadID = skip.BeadID
			}
			ok = true
		case isTransientTrackerContentionSkipReason(skip.Reason):
			if reasonCode != preClaimIdleReasonSystemic {
				reasonCode = preClaimIdleReasonTrackerContention
				detail = trackerContentionSkipDetail(skip.Reason)
			}
			if beadID == "" {
				beadID = skip.BeadID
			}
			ok = true
		default:
			return "", "", "", false
		}
	}
	return detail, reasonCode, beadID, ok
}

func isTransientTrackerContentionSkipReason(reason string) bool {
	if reason == "" {
		return false
	}
	if work.IsTrackerContentionPreClaimSkipReason(reason) {
		return true
	}
	return strings.Contains(reason, "tracker lock timeout")
}

func trackerContentionSkipDetail(reason string) string {
	if work.IsTrackerContentionPreClaimSkipReason(reason) {
		return work.TrackerContentionPreClaimDetail(reason)
	}
	return reason
}

func queueRankValue(raw any) any {
	var rank int
	var ok bool
	if extra, isExtra := raw.(map[string]any); isExtra {
		rank, ok = bead.QueueRank(extra)
	} else {
		rank, ok = parseQueueRank(raw)
	}
	if !ok {
		return nil
	}
	return rank
}

type candidateSkipDecision struct {
	Reason string
	Detail string
}

func (w *ExecuteBeadWorker) refreshCandidateBeforeClaim(ctx context.Context, candidate bead.Bead) (*bead.Bead, *candidateSkipDecision, error) {
	fresh, skip, err := w.lookupFreshCandidate(ctx, candidate.ID)
	if err != nil || skip != nil {
		return fresh, skip, err
	}

	ready, err := w.Store.ReadyExecution()
	if err != nil {
		return nil, nil, err
	}
	for _, readyBead := range ready {
		if readyBead.ID == candidate.ID {
			return fresh, nil, nil
		}
	}

	return fresh, candidateSkipFromReadyView(candidate.ID, fresh, w.readyExecutionBreakdown()), nil
}

func (w *ExecuteBeadWorker) refreshClaimedCandidateBeforeAttempt(ctx context.Context, candidate bead.Bead) (*bead.Bead, *candidateSkipDecision, error) {
	fresh, skip, err := w.lookupFreshCandidate(ctx, candidate.ID)
	if err != nil || skip != nil {
		return fresh, skip, err
	}
	if skip = candidateSkipFromClaimedState(fresh); skip != nil {
		return fresh, skip, nil
	}
	dependencyWaiting, err := w.claimedCandidateDependencyWaiting(ctx, fresh)
	if err != nil {
		return fresh, nil, err
	}
	if dependencyWaiting {
		return fresh, candidateSkipFromFreshState(fresh, "dependency_waiting"), nil
	}
	return fresh, nil, nil
}

// claimedCandidateDependencyWaiting evaluates the dependency graph directly
// from the fresh claimed bead. ReadyExecution and ReadyExecutionBreakdown are
// intentionally unsafe here because their lifecycle filtering masks an
// in-progress bead, which can hide dependency_waiting after claim.
func (w *ExecuteBeadWorker) claimedCandidateDependencyWaiting(ctx context.Context, fresh *bead.Bead) (bool, error) {
	if fresh == nil {
		return false, nil
	}
	for _, depID := range fresh.DepIDs() {
		dependency, err := w.Store.Get(ctx, depID)
		if err != nil {
			// A missing dependency cannot satisfy readiness. Other read errors
			// must reach the existing refresh-error claim-release path.
			if errors.Is(err, bead.ErrNotFound) || strings.Contains(strings.ToLower(err.Error()), "not found") {
				return true, nil
			}
			return false, err
		}
		if dependency == nil || !bead.LifecycleStatusSatisfiesDependency(dependency.Status) {
			return true, nil
		}
	}
	return false, nil
}

func (w *ExecuteBeadWorker) lookupFreshCandidate(ctx context.Context, beadID string) (*bead.Bead, *candidateSkipDecision, error) {
	fresh, err := w.Store.Get(ctx, beadID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, &candidateSkipDecision{
				Reason: "missing",
				Detail: "bead no longer exists in the tracker",
			}, nil
		}
		return nil, nil, err
	}
	return fresh, nil, nil
}

func (w *ExecuteBeadWorker) readyExecutionBreakdown() *bead.ReadyExecutionBreakdown {
	diag, ok := w.Store.(readyDiagnoser)
	if !ok {
		return nil
	}
	breakdown, err := diag.ReadyExecutionBreakdown()
	if err != nil {
		return nil
	}
	return &breakdown
}

func candidateSkipFromReadyView(beadID string, fresh *bead.Bead, breakdown *bead.ReadyExecutionBreakdown) *candidateSkipDecision {
	reason := ""
	switch {
	case breakdown != nil && containsReadyBreakdownID(breakdown.NotEligible, beadID):
		reason = "execution_ineligible"
	case breakdown != nil && containsReadyBreakdownID(breakdown.Superseded, beadID):
		reason = "superseded"
	case breakdown != nil && containsReadyBreakdownID(breakdown.DependencyWaiting, beadID):
		reason = "dependency_waiting"
	case breakdown != nil && containsReadyBreakdownID(breakdown.RetryCooldown, beadID):
		reason = "retry_cooldown"
	case breakdown != nil && containsReadyBreakdownID(breakdown.ProposedOperatorAttention, beadID):
		reason = "operator_attention"
	case breakdown != nil && containsReadyBreakdownID(breakdown.ExternalBlocked, beadID):
		reason = "external_blocked"
	case breakdown != nil && (containsReadyBreakdownID(breakdown.Epics, beadID) || containsReadyBreakdownID(breakdown.EpicClosureCandidates, beadID)):
		reason = "epic_container"
	}
	return candidateSkipFromFreshState(fresh, reason)
}

func candidateSkipFromClaimedState(fresh *bead.Bead) *candidateSkipDecision {
	return candidateSkipFromFreshState(fresh, "")
}

func candidateSkipFromFreshState(fresh *bead.Bead, fallbackReason string) *candidateSkipDecision {
	if fresh == nil {
		return &candidateSkipDecision{
			Reason: "missing",
			Detail: "bead no longer exists in the tracker",
		}
	}
	reason := fallbackReason
	if executionEligible, known := freshCandidateExecutionEligible(*fresh); known && !executionEligible {
		if HasBeadLabel(fresh.Labels, "decomposed") {
			reason = "decomposed"
		} else {
			reason = "execution_ineligible"
		}
	} else if supersededBy := freshCandidateSupersededBy(*fresh); supersededBy != "" {
		reason = "superseded"
	} else if fresh.Status == bead.StatusProposed {
		reason = "operator_attention"
	} else if fresh.Status == bead.StatusBlocked {
		reason = "external_blocked"
	} else if fresh.Status == bead.StatusClosed || fresh.Status == bead.StatusCancelled {
		reason = "closed"
	}
	if reason == "" {
		return nil
	}
	return &candidateSkipDecision{
		Reason: reason,
		Detail: candidateSkipDetail(reason, fresh),
	}
}

func freshCandidateExecutionEligible(b bead.Bead) (bool, bool) {
	raw, ok := b.Extra[bead.ExtraExecutionElig]
	if !ok {
		return true, false
	}
	eligible, isBool := raw.(bool)
	if !isBool {
		return true, false
	}
	return eligible, true
}

func freshCandidateSupersededBy(b bead.Bead) string {
	if b.Extra == nil {
		return ""
	}
	raw, ok := b.Extra["superseded-by"]
	if !ok {
		return ""
	}
	value, _ := raw.(string)
	return strings.TrimSpace(value)
}

func candidateSkipDetail(reason string, fresh *bead.Bead) string {
	switch reason {
	case "decomposed":
		return "bead is marked decomposed and execution-ineligible"
	case "execution_ineligible":
		return "bead is no longer execution-eligible"
	case "superseded":
		if fresh != nil {
			if supersededBy := freshCandidateSupersededBy(*fresh); supersededBy != "" {
				return fmt.Sprintf("bead is superseded by %s", supersededBy)
			}
		}
		return "bead is superseded"
	case "dependency_waiting":
		return "bead is no longer execution-ready because dependencies remain open"
	case "retry_cooldown":
		return "bead is in retry cooldown"
	case "operator_attention":
		return "bead moved to proposed/operator attention"
	case "external_blocked":
		return "bead is externally blocked"
	case "epic_container":
		return "bead is an epic container, not ordinary executable work"
	case "closed":
		if fresh != nil && fresh.Status != "" {
			return fmt.Sprintf("bead moved to %s", fresh.Status)
		}
		return "bead is already closed"
	case "missing":
		return "bead no longer exists in the tracker"
	default:
		return "bead dropped out of the fresh execution-ready view"
	}
}

type preClaimWarnRepeatState struct {
	Fingerprint      string
	Count            int
	DistinctBeadIDs  []string
	seenBeadIDs      map[string]struct{}
	ExampleBeadID    string
	ExampleDetail    string
	ExamplePayload   map[string]any
	FirstObservedAt  time.Time
	EscalationIssued bool
}

func (s *preClaimWarnRepeatState) reset(fingerprint, beadID, detail string, payload map[string]any, at time.Time) {
	s.Fingerprint = fingerprint
	s.Count = 1
	s.DistinctBeadIDs = []string{beadID}
	if s.seenBeadIDs == nil {
		s.seenBeadIDs = map[string]struct{}{}
	} else {
		for k := range s.seenBeadIDs {
			delete(s.seenBeadIDs, k)
		}
	}
	if beadID != "" {
		s.seenBeadIDs[beadID] = struct{}{}
	}
	s.ExampleBeadID = beadID
	s.ExampleDetail = detail
	s.ExamplePayload = cloneStringAnyMap(payload)
	s.FirstObservedAt = at.UTC()
	s.EscalationIssued = false
}

func (s *preClaimWarnRepeatState) record(fingerprint, beadID, detail string, payload map[string]any, at time.Time, threshold int) (preClaimWarnRepeatState, bool) {
	if threshold <= 0 {
		threshold = DefaultPreClaimWarnRepeatThreshold
	}
	if fingerprint == "" {
		return preClaimWarnRepeatState{}, false
	}
	if s.Count == 0 || s.Fingerprint != fingerprint || s.seenBeadIDs == nil {
		s.reset(fingerprint, beadID, detail, payload, at)
		return *s, false
	}
	if _, seen := s.seenBeadIDs[beadID]; seen {
		s.reset(fingerprint, beadID, detail, payload, at)
		return *s, false
	}
	s.Count++
	s.DistinctBeadIDs = append(s.DistinctBeadIDs, beadID)
	s.seenBeadIDs[beadID] = struct{}{}
	if s.ExampleBeadID == "" {
		s.ExampleBeadID = beadID
	}
	if s.ExampleDetail == "" {
		s.ExampleDetail = detail
	}
	if len(s.ExamplePayload) == 0 {
		s.ExamplePayload = cloneStringAnyMap(payload)
	}
	if s.FirstObservedAt.IsZero() {
		s.FirstObservedAt = at.UTC()
	}
	if s.Count >= threshold && !s.EscalationIssued {
		s.EscalationIssued = true
		return *s, true
	}
	return *s, false
}

func cloneStringAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func preClaimIntakeWarningFingerprint(reason, detail string) string {
	normalized := strings.Join([]string{
		"pre_claim_intake.warn",
		strings.ToLower(strings.TrimSpace(reason)),
		strings.TrimSpace(detail),
	}, "\x00")
	if strings.TrimSpace(normalized) == "" {
		return ""
	}
	return hashText(normalized)
}

func isProjectRootMutationRejectedDetail(detail string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(detail)), "project root mutation rejected")
}

func normalizePreClaimSystemUnreadyWarning(detail string) (code, normalizedDetail string) {
	if isProjectRootMutationRejectedDetail(detail) {
		return preClaimIntakeProjectRootMutationRejectedCode, preClaimIntakeProjectRootMutationRejectedDetail
	}
	return "", detail
}

func emitStaleCandidateSkip(emit func(string, map[string]any), beadID string, skip *candidateSkipDecision, fresh *bead.Bead, stage string) {
	if emit == nil || skip == nil {
		return
	}
	payload := map[string]any{
		"bead_id": beadID,
		"reason":  skip.Reason,
		"detail":  skip.Detail,
		"stage":   stage,
	}
	for k, v := range preClaimDecisionAudit(skip.Reason, 0) {
		payload[k] = v
	}
	if fresh != nil {
		payload["status"] = fresh.Status
		if eligible, known := freshCandidateExecutionEligible(*fresh); known {
			payload["execution_eligible"] = eligible
		}
		if HasBeadLabel(fresh.Labels, "decomposed") {
			payload["decomposed"] = true
		}
		if supersededBy := freshCandidateSupersededBy(*fresh); supersededBy != "" {
			payload["superseded_by"] = supersededBy
		}
	}
	emit("picker.skip_stale_candidate", payload)
}

func containsReadyBreakdownID(ids []string, beadID string) bool {
	for _, id := range ids {
		if id == beadID {
			return true
		}
	}
	return false
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
		if w.transientCandidateSkips != nil {
			if reason, skipped := w.transientCandidateSkips[candidate.ID]; skipped {
				skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, BeadRank: queueRankPtr(candidate.Extra), Reason: reason})
				continue
			}
		}
		if targetBeadID != "" && candidate.ID != targetBeadID {
			skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, BeadRank: queueRankPtr(candidate.Extra), Reason: "target_bead"})
			continue
		}
		if hasResultForBead(results, candidate.ID) {
			skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, BeadRank: queueRankPtr(candidate.Extra), Reason: "in_attempted"})
			continue
		}
		if entry.FilterDecision == FilterDecisionSkipped {
			// PreviewQueue already applied label_filter; record as skip.
			skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, BeadRank: queueRankPtr(candidate.Extra), Reason: "label_filter"})
			continue
		}
		allowed := true
		reason := ""
		for _, guard := range guards {
			if guard == nil {
				continue
			}
			guardOK, guardReason := guard.Allow(ctx, candidate.ID)
			if guardOK {
				continue
			}
			allowed = false
			reason = guardReason
			if reason != "" {
				skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, BeadRank: queueRankPtr(candidate.Extra), Reason: reason})
			}
			break
		}
		if !allowed {
			if isTransientTrackerContentionSkipReason(reason) {
				return bead.Bead{}, skips, false, nil
			}
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
func appendLoopRoutingEvidence(store BeadEventAppender, target bead.Bead, report ExecuteBeadReport, createdAt time.Time) {
	if store == nil || target.ID == "" {
		return
	}
	provider := report.Provider
	if provider == "" {
		provider = report.Harness
	}
	if provider == "" {
		return
	}
	intentSource, estimatedDifficulty, inferredMinPower, hasInferredMinPower, _ := executionRoutingIntentFacts(target, report)
	body := map[string]any{
		"resolved_provider":     provider,
		"resolved_model":        report.Model,
		"fallback_chain":        []map[string]any{},
		"routing_intent_source": intentSource,
		"estimated_difficulty":  estimatedDifficulty,
		"requested_policy":      report.RequestedPolicy,
		"requested_min_power":   report.RequestedMinPower,
		"requested_max_power":   report.RequestedMaxPower,
		"routing_intent_note":   report.RoutingIntentNote,
		"resolved_power_class":  report.ResolvedPowerClass,
		"escalation_count":      report.EscalationCount,
		"final_power_class":     report.FinalPowerClass,
	}
	if hasInferredMinPower {
		body["inferred_min_power"] = inferredMinPower
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return
	}
	summary := "provider=" + provider
	if report.Model != "" {
		summary += " model=" + report.Model
	}
	_ = store.AppendEvent(target.ID, bead.BeadEvent{
		Kind:      "routing",
		Summary:   summary,
		Body:      string(encoded),
		Actor:     "ddx",
		Source:    "ddx work",
		CreatedAt: createdAt,
	})
}

func appendExecutionRoutingIntentEvidence(store BeadEventAppender, target bead.Bead, report ExecuteBeadReport, createdAt time.Time) {
	if store == nil || target.ID == "" {
		return
	}
	intentSource, estimatedDifficulty, inferredMinPower, hasInferredMinPower, rejectedRoutePins := executionRoutingIntentFacts(target, report)
	body := map[string]any{
		"bead_id":                 target.ID,
		"attempt_id":              report.AttemptID,
		"routing_intent_source":   intentSource,
		"estimated_difficulty":    estimatedDifficulty,
		"requested_policy":        report.RequestedPolicy,
		"requested_min_power":     report.RequestedMinPower,
		"requested_max_power":     report.RequestedMaxPower,
		"actual_harness":          report.Harness,
		"actual_provider":         report.Provider,
		"actual_model":            report.Model,
		"actual_power":            report.ActualPower,
		"routing_intent_degraded": false,
		"routing_intent_note":     "",
		"rejected_route_pins":     rejectedRoutePins,
	}
	if hasInferredMinPower {
		body["inferred_min_power"] = inferredMinPower
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
	summary := fmt.Sprintf("source=%s", intentSource)
	if estimatedDifficulty != "" {
		summary += fmt.Sprintf(" difficulty=%s", estimatedDifficulty)
	}
	if hasInferredMinPower {
		summary += fmt.Sprintf(" minPower=%d", inferredMinPower)
	}
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

func executionRoutingIntentFacts(target bead.Bead, report ExecuteBeadReport) (source, estimatedDifficulty string, inferredMinPower int, hasInferredMinPower bool, rejectedRoutePins []string) {
	intent := escalation.ParseExecutionHint(&target)
	source = string(intent.Source)
	estimatedDifficulty = string(intent.EstimatedDifficulty)
	inferredMinPower = intent.InferredMinPower
	hasInferredMinPower = intent.HasInferredMinPower
	rejectedRoutePins = intent.RejectedRoutePins

	if report.RoutingIntentSource != "" {
		source = report.RoutingIntentSource
		estimatedDifficulty = report.EstimatedDifficulty
		inferredMinPower = report.InferredMinPower
		hasInferredMinPower = report.InferredMinPowerPresent
	}
	return source, estimatedDifficulty, inferredMinPower, hasInferredMinPower, rejectedRoutePins
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
	if auditLine := decisionAuditEventBodyLine(report); auditLine != "" {
		parts = append(parts, auditLine)
	}
	if report.PreserveRef != "" {
		parts = append(parts, fmt.Sprintf("preserve_ref=%s", report.PreserveRef))
	}
	if len(report.NoEvidencePaths) > 0 {
		parts = append(parts, fmt.Sprintf("no_evidence_paths=%s", strings.Join(report.NoEvidencePaths, ",")))
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

// stampPreservedReviewBlockMarkers records durable Bead.Extra block markers
// through the store API when the large-deletion safety gate preserves an
// attempt for review. Deterministic ready-queue eligibility (bead.Store)
// reads these markers to exclude the bead from worker readiness until an
// operator stamps a matching preserved-review-unblocked-at/-attempt pair via
// `ddx bead update --set` (ddx-ec1c1f89).
func stampPreservedReviewBlockMarkers(ctx context.Context, store ExecuteBeadLoopStore, beadID string, report ExecuteBeadReport, at time.Time) error {
	fingerprint := hashText(oneLineGateSummary(report.Detail))
	return store.Update(ctx, beadID, func(b *bead.Bead) {
		ensureBeadExtra(b)
		b.Extra[bead.ExtraPreservedReviewBlockedAt] = at.Format(time.RFC3339)
		b.Extra[bead.ExtraPreservedReviewBlockedAttempt] = report.AttemptID
		b.Extra[bead.ExtraPreservedReviewGate] = bead.PreservedReviewGateLargeDeletion
		b.Extra[bead.ExtraPreservedReviewFingerprint] = fingerprint
	})
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
		"cleanup_claim_liveness_tmp_files_reclaimed": summary.RemovedClaimLivenessTmpFiles,
		"cleanup_claim_liveness_inodes_reclaimed":    summary.ClaimLivenessInodesReclaimed,
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

// emitResourcePressure surfaces a non-blocking FD pressure diagnostic. Warn
// severity emits "loop.resource_pressure"; operator_attention severity emits
// "loop.operator_attention" (reason resource_pressure_fd) so the same
// operator-facing surface used elsewhere in the loop carries the finding.
// OK severity is not reported — pressure diagnostics only exist to surface
// approaching exhaustion, not steady-state health.
func emitResourcePressure(emit func(string, map[string]any), log io.Writer, phase string, report ResourcePressureReport) {
	if report.Severity != ResourcePressureWarn && report.Severity != ResourcePressureOperatorAttention {
		return
	}
	fields := map[string]any{
		"phase":                     phase,
		"severity":                  string(report.Severity),
		"fd_used":                   report.FDUsed,
		"fd_limit":                  report.FDLimit,
		"fd_ratio":                  report.FDRatio,
		"worker_subprocess_count":   report.WorkerSubprocessCount,
		"temp_worktree_count":       report.TempWorktreeCount,
		"stale_execution_dir_count": report.StaleExecutionDirCount,
	}
	if emit != nil {
		if report.Severity == ResourcePressureOperatorAttention {
			attention := map[string]any{"reason": "resource_pressure_fd"}
			for k, v := range fields {
				attention[k] = v
			}
			emit("loop.operator_attention", attention)
		} else {
			emit("loop.resource_pressure", fields)
		}
	}
	if log != nil {
		_, _ = fmt.Fprintf(log,
			"resource pressure (%s) severity=%s fd_used=%d fd_limit=%d fd_ratio=%.2f worker_subprocess_count=%d temp_worktree_count=%d stale_execution_dir_count=%d\n",
			phase, report.Severity, report.FDUsed, report.FDLimit, report.FDRatio,
			report.WorkerSubprocessCount, report.TempWorktreeCount, report.StaleExecutionDirCount,
		)
	}
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
			"diagnosis":                 report.ResourceExhaustionDiagnosis,
			"restartable":               report.ResourceExhaustionRestartable,
			"worker_local":              report.ResourceExhaustionRestartable,
		}
		body, err := json.Marshal(detail)
		if err != nil {
			detail = map[string]any{
				"bead_id":     beadID,
				"detail":      report.Detail,
				"status":      report.Status,
				"diagnosis":   report.ResourceExhaustionDiagnosis,
				"restartable": report.ResourceExhaustionRestartable,
			}
		} else {
			detail["body"] = string(body)
		}
	} else {
		detail = map[string]any{
			"bead_id":     beadID,
			"detail":      report.Detail,
			"status":      report.Status,
			"diagnosis":   report.ResourceExhaustionDiagnosis,
			"restartable": report.ResourceExhaustionRestartable,
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
	summary := report.Detail
	if summary == "" {
		summary = ResourceExhaustedStopMessage
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "resource-exhausted",
		Summary:   summary,
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
	case ExecuteBeadStatusPreservedNeedsReview:
		detail := report.Detail
		if detail == "" {
			detail = "safety gate preserved result"
		}
		return fmt.Sprintf("preserved: %s", detail)
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
	case ExecuteBeadStatusNoChanges, ExecuteBeadStatusPreservedNeedsReview, ExecuteBeadStatusResourceExhausted:
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
	if report.Status == ExecuteBeadStatusLandRetry {
		report.OutcomeReason = FailureModeLandRetry
		report.Disrupted = true
		report.DisruptionReason = FailureModeLandRetry
		return
	}
	if report.Status == ExecuteBeadStatusLandOperatorAttention {
		report.OutcomeReason = FailureModeLandOperatorAttention
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

// applyPreClaimDecomposition creates child beads, parks the parent as
// execution-ineligible, and appends a triage-decomposed event to the parent.
// It returns the IDs of the created children so the caller can log or record
// them.
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

	// Park the parent so it does not re-enter execution while the generated
	// children are outstanding.
	if err := store.Update(ctx, parent.ID, func(b *bead.Bead) {
		ensureBeadExtra(b)
		b.Extra[bead.ExtraExecutionElig] = false
	}); err != nil {
		return childIDs, fmt.Errorf("decompose: park parent %s after decomposition: %w", parent.ID, err)
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
// operator reviews the intake decision. It does not release the worker-owned
// claim — the caller must release it after this returns (whether or not
// parking succeeds).
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
			"operator_override":    false,
			"prompt_fingerprint":   promptFingerprint,
			"accepted_fingerprint": findingFingerprint,
		},
	)
	body["fingerprint"] = findingFingerprint
	body["rule_fingerprint"] = findingFingerprint
	body["prompt_fingerprint"] = promptFingerprint

	if err := store.ParkToProposedWithIntakeEvent(candidate.ID, actor, string(outcome), reason, detail, body, at, func(b *bead.Bead) {
		ensureBeadExtra(b)
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
	return true, nil
}

func appendPreClaimIntakeWarning(store ExecuteBeadLoopStore, emit func(string, map[string]any), warnState *preClaimWarnRepeatState, threshold int, beadID, actor, reason, detail string, at time.Time) bool {
	if store == nil {
		return false
	}
	fingerprint := preClaimIntakeWarningFingerprint(reason, detail)
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
	if fingerprint != "" {
		var bodyMap map[string]any
		_ = json.Unmarshal([]byte(body), &bodyMap)
		if bodyMap == nil {
			bodyMap = map[string]any{}
		}
		bodyMap["fingerprint"] = fingerprint
		bodyMap["warning_fingerprint"] = fingerprint
		if encoded, err := json.Marshal(bodyMap); err == nil {
			body = string(encoded)
		}
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "intake.warn",
		Summary:   reason,
		Body:      body,
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
	})
	if warnState == nil {
		return false
	}
	snapshot, escalated := warnState.record(fingerprint, beadID, detail, map[string]any{
		"bead_id":     beadID,
		"reason":      reason,
		"detail":      detail,
		"fingerprint": fingerprint,
	}, at, threshold)
	if !escalated {
		return false
	}
	detailText := fmt.Sprintf(
		"pre-claim warn fingerprint repeated %d times across %d distinct bead IDs",
		snapshot.Count,
		len(snapshot.DistinctBeadIDs),
	)
	payload := map[string]any{
		"reason":            "preclaim_warn_repeated",
		"detail":            detailText,
		"fingerprint":       snapshot.Fingerprint,
		"count":             snapshot.Count,
		"distinct_bead_ids": append([]string(nil), snapshot.DistinctBeadIDs...),
		"example_bead_id":   snapshot.ExampleBeadID,
		"example_detail":    snapshot.ExampleDetail,
		"first_observed_at": snapshot.FirstObservedAt.UTC().Format(time.RFC3339),
		"example_payload":   cloneStringAnyMap(snapshot.ExamplePayload),
	}
	for k, v := range preClaimDecisionAudit("preclaim_warn_repeated", snapshot.Count) {
		payload[k] = v
	}
	if emit != nil {
		emit("loop.operator_attention", payload)
	}
	attentionBody, _ := json.Marshal(payload)
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "operator_attention",
		Summary:   "preclaim_warn_repeated",
		Body:      string(attentionBody),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
	})
	return true
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
	executeLoopNoChangesNextMinPowerKey     = "work-no-changes-next-min-power"
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
	nextFloor := 0
	if nextFloorFn != nil {
		var err error
		nextFloor, err = nextFloorFn(actualPower)
		if err != nil {
			emitEscalationAbortedEvent(store, beadID, actor, "", "", actualPower, time.Now().UTC())
			return applyNoChangesOperatorRequired(store, beadID, actor, noChanges, time.Now().UTC())
		}
	}

	// When a ladder is available, advance by one step. If the ladder is
	// exhausted (already at top powerClass), the work requires human input.
	if nextFloorFn != nil {
		return store.UpdateWithLifecycleStatus(beadID, bead.StatusOpen, bead.LifecycleTransitionOptions{
			Reason: reason,
			Actor:  actor,
			Source: "ddx work",
		}, func(b *bead.Bead) error {
			ensureBeadExtra(b)
			clearNoChangesLifecycleLabels(b)
			bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{})
			setNoChangesLifecycleMetadata(b, noChanges.EventKind, reason, suggestedAction)
			b.Extra[executeLoopSmartRetryKey] = true
			b.Extra[executeLoopSmartRetryReasonKey] = reason
			b.Extra[executeLoopSmartRetrySuggestedActionKey] = suggestedAction
			setNoChangesNextMinPower(b, nextFloor)
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
		b.Extra[executeLoopSmartRetryKey] = true
		b.Extra[executeLoopSmartRetryReasonKey] = reason
		b.Extra[executeLoopSmartRetrySuggestedActionKey] = suggestedAction
		setNoChangesNextMinPower(b, nextFloor)
		return nil
	})
}

func applyNoChangesBadAttemptEscalation(store ExecuteBeadLoopStore, beadID, actor string, noChanges *agenttry.NoChangesOutcome, actualPower int, nextFloorFn func(int) (int, error)) error {
	reason := strings.TrimSpace(noChanges.Reason)
	if reason == "" {
		reason = "operator review required before another automated attempt"
	}

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
			clearSmartRetryMetadata(b)
			setNoChangesNextMinPower(b, nextFloor)
			return nil
		})
	}

	return store.UpdateWithLifecycleStatus(beadID, bead.StatusOpen, bead.LifecycleTransitionOptions{
		Reason: reason,
		Actor:  actor,
		Source: "ddx work",
	}, func(b *bead.Bead) error {
		ensureBeadExtra(b)
		clearSmartRetryMetadata(b)
		return nil
	})
}

// applyRepairCycleExhaustedEscalation keeps the bead open when a stronger retry
// remains available. If the ladder is already at the top powerClass, the bead
// is parked to proposed for operator review.
func applyRepairCycleExhaustedEscalation(store ExecuteBeadLoopStore, beadID, actor string, actualPower int, at time.Time, nextFloorFn func(int) (int, error)) error {
	if nextFloorFn != nil {
		if _, err := nextFloorFn(actualPower); err == nil {
			return store.UpdateWithLifecycleStatus(beadID, bead.StatusOpen, bead.LifecycleTransitionOptions{
				Reason: "repair cycle exhausted: escalating implementer to higher powerClass",
				Actor:  actor,
				Source: "ddx work",
			}, nil)
		}
		emitEscalationAbortedEvent(store, beadID, actor, "", "", actualPower, at)
	}
	return parkToProposedWithOperatorMeta(store, beadID, bead.ParkPostReviewMalfunction, ParkToProposedOpts{
		Reason:          "repair cycle exhausted at top power: operator decision required",
		Summary:         "repair cycle exhausted: operator attention required",
		SuggestedAction: "review the blocked attempt and accept, split, or retry with a stronger model",
		Since:           at,
		CleanupLabels:   false,
		AdditionalMutate: func(b *bead.Bead) {
			ensureBeadExtra(b)
		},
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
	return parkToProposedWithOperatorMeta(store, beadID, bead.ParkNoChangesOperatorRequired, ParkToProposedOpts{
		Reason:          reason,
		Summary:         "no_changes requested operator attention",
		SuggestedAction: suggestedAction,
		Since:           at,
		CleanupLabels:   false,
		AdditionalMutate: func(b *bead.Bead) {
			ensureBeadExtra(b)
			clearNoChangesLifecycleLabels(b)
			clearSmartRetryMetadata(b)
			clearNoChangesNextMinPower(b)
			setNoChangesLifecycleMetadata(b, noChanges.EventKind, reason, suggestedAction)
		},
	})
}

func parkNoEvidenceForOperator(store ExecuteBeadLoopStore, beadID, actor string, report ExecuteBeadReport, detail string, at time.Time) error {
	reason := FailureModeNoEvidenceProduced
	if strings.TrimSpace(report.PreserveRef) != "" || len(report.NoEvidencePaths) > 0 {
		reason = "no_evidence_dirty_rescue"
	}
	if err := parkToProposedWithOperatorMeta(store, beadID, bead.ParkNoChangesOperatorRequired, ParkToProposedOpts{
		Reason:          reason,
		Summary:         "attempt produced dirty rescue but no landed evidence",
		SuggestedAction: "inspect the dirty rescue patch, commit valid work manually, then move the bead back to open if another automated attempt is needed",
		Since:           at,
		CleanupLabels:   false,
	}); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"reason":            FailureModeNoEvidenceProduced,
		"bead_id":           beadID,
		"attempt_id":        report.AttemptID,
		"detail":            detail,
		"preserve_ref":      report.PreserveRef,
		"dirty_paths":       append([]string(nil), report.NoEvidencePaths...),
		"base_rev":          report.BaseRev,
		"result_rev":        report.ResultRev,
		"suggested_action":  "salvage or discard the rescue patch before retrying",
		"automation_action": "released claim and parked bead to proposed",
	})
	return store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "operator_attention",
		Summary:   FailureModeNoEvidenceProduced,
		Body:      string(body),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
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
		clearNoChangesNextMinPower(b)
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
	delete(b.Extra, executeLoopSmartRetryKey)
	delete(b.Extra, executeLoopSmartRetryReasonKey)
	delete(b.Extra, executeLoopSmartRetrySuggestedActionKey)
}

func setNoChangesNextMinPower(b *bead.Bead, minPower int) {
	if minPower <= 0 {
		return
	}
	ensureBeadExtra(b)
	b.Extra[executeLoopNoChangesNextMinPowerKey] = minPower
}

func clearNoChangesNextMinPower(b *bead.Bead) {
	if b.Extra == nil {
		return
	}
	delete(b.Extra, executeLoopNoChangesNextMinPowerKey)
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
		delete(b.Extra, executeLoopNoChangesNextMinPowerKey)
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
		"decomposed",
		"intake_error",
		"ambiguous_needs_human",
		FailureModeAuthError,
		FailureModeHarnessNotInstalled,
		FailureModeAgentPowerUnsatisfied,
		FailureModeBlockedByPassthroughConstraint,
		FailureModeNoEvidenceProduced,
		FailureModeLandRetry,
		FailureModeLandOperatorAttention,
		FailureModeWorktreeLost,
		// Non-retryable typed provider failures (ddx-3b721804): a config bug
		// or unclassified provider fault is not an implementation attempt.
		FailureModeProviderConfigInvalid,
		FailureModeUnknownProviderFailure,
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
		ExecuteBeadStatusLandRetry,
		ExecuteBeadStatusLandOperatorAttention,
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

// mixedCommitCooldownWindow is the lookback period for the mixed_commit circuit-breaker.
const mixedCommitCooldownWindow = 24 * time.Hour

// countRecentMixedCommitEvents returns the number of execute-bead events in the
// store for beadID within window of now that carry the
// mixed_commit_and_no_changes_rationale marker. Used by the circuit-breaker to
// detect a repeated-contradiction loop before the current attempt is appended.
func countRecentMixedCommitEvents(store ExecuteBeadLoopStore, beadID string, window time.Duration, now time.Time) int {
	events, err := store.Events(beadID)
	if err != nil {
		return 0
	}
	cutoff := now.Add(-window)
	count := 0
	for _, ev := range events {
		if ev.Kind == "execute-bead" && ev.CreatedAt.After(cutoff) &&
			strings.Contains(ev.Body, mixedCommitAndNoChangesRationaleReason) {
			count++
		}
	}
	return count
}

func isTransientOutcomeReason(reason string) bool {
	switch reason {
	case "transport", "quota", "routing", "timeout", "merge_conflict",
		ReadinessSystemReasonResourceExhausted,
		ReadinessSystemReasonRepoConcurrency,
		FailureModeLockContention,
		FailureModeLandRetry,
		FailureModeNoViableProvider,
		FailureModeServerUnavailable,
		FailureModeWorktreeLost,
		// Typed provider-availability failures (ddx-3b721804): transient
		// route-health conditions, not implementation attempts.
		FailureModeProviderConnectivity,
		FailureModeProviderAuth,
		FailureModeProviderRateLimit,
		FailureModeProviderQuota,
		FailureModeProviderModelUnavailable,
		FailureModeProviderHarnessUnavailable:
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
		b, err := store.Get(context.Background(), id)
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

// providerConnectivityMarkers names the substrings that indicate the selected
// endpoint was unreachable (TCP-level dial failure, connection refused,
// network down). Concrete route identity is audit evidence only and must not
// participate in this control-flow classification.
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
// an endpoint that could not be reached. It depends only on typed outcome,
// status, and error text: Fizeau's returned harness/provider/model are retained
// for audit but cannot change DDx retry, disruption, or triage behavior.
// Reports that already classify as no_viable_provider or routing-infrastructure
// failures keep their own paths.
func isProviderConnectivityFailureReport(report ExecuteBeadReport) bool {
	if report.Status != ExecuteBeadStatusExecutionFailed {
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

// providerEndpointURLRe extracts the first http(s) URL from a transport error
// message (e.g. `openai POST http://vidar:1235/v1/chat/completions dial tcp ...`).
// Stops at whitespace or a closing quote so a quoted Go URL error
// (`Post "http://bragi:1234/v1/chat": dial tcp...`) yields the bare URL.
var providerEndpointURLRe = regexp.MustCompile(`https?://[^\s"'\\]+`)

// timeoutClassMarkers orders transport failure classes most-specific first so a
// compound message like "dial tcp ...: i/o timeout" classifies as "i/o timeout"
// rather than the generic "dial tcp" prefix.
var timeoutClassMarkers = []string{
	"connection refused",
	"connection reset",
	"i/o timeout",
	"no route to host",
	"network is unreachable",
	"bad gateway",
	"service unavailable",
	"gateway timeout",
	"dial tcp",
}

// parseProviderConnectivityFacts extracts the actionable route facts —
// the dialed endpoint URL and the transport timeout class — from a provider
// connectivity failure report. Both returns are best-effort: an empty string
// means the corresponding fact could not be parsed from Detail/Error/Stderr.
func parseProviderConnectivityFacts(report ExecuteBeadReport) (endpoint, timeoutClass string) {
	combined := strings.Join([]string{
		report.Detail,
		report.Error,
		report.Stderr,
	}, "\n")
	endpoint = strings.TrimSpace(providerEndpointURLRe.FindString(combined))
	lower := strings.ToLower(combined)
	for _, marker := range timeoutClassMarkers {
		if strings.Contains(lower, marker) {
			timeoutClass = marker
			break
		}
	}
	return endpoint, timeoutClass
}

// leaseReleaser is the narrow capability attempt guards use to atomically
// release a held lease when the Fizeau route stage times out.
type leaseReleaser interface {
	Release(id, assignee, targetStatus string) error
}

// routeResolutionTimeoutReport releases the bead's lease, records the
// consecutive wedge, emits operator_attention evidence, and returns a typed
// disrupted report. lastActivityAt identifies the last observed execution
// activity before Fizeau's routing_decision deadline expired. If lease release
// fails it returns that error without recording the timeout as handled.
func routeResolutionTimeoutReport(store ExecuteBeadLoopStore, beadID, assignee, attemptID string, now time.Time, timeout time.Duration, lastActivityAt time.Time) (ExecuteBeadReport, error) {
	if err := releaseWorkerClaim(store, beadID, assignee); err != nil {
		return ExecuteBeadReport{}, fmt.Errorf("release route-timed-out bead %s: %w", beadID, err)
	}
	diagnosis := fmt.Sprintf(
		"Fizeau route stage exceeded %s before routing_decision; released lease and flagged for operator attention",
		timeout,
	)
	recordConsecutiveWedge(store, beadID, FailureModeRouteResolutionTimeout, now)
	if store != nil && beadID != "" {
		appendWorkEvent(store, beadID, "operator_attention", FailureModeRouteResolutionTimeout, map[string]any{
			"reason":           FailureModeRouteResolutionTimeout,
			"bead_id":          beadID,
			"attempt_id":       attemptID,
			"last_activity_at": lastActivityAt.UTC().Format(time.RFC3339),
			"diagnosis":        diagnosis,
			"timeout":          timeout.String(),
		}, assignee, now)
	}
	return ExecuteBeadReport{
		BeadID:           beadID,
		Status:           ExecuteBeadStatusExecutionFailed,
		Detail:           diagnosis,
		OutcomeReason:    FailureModeRouteResolutionTimeout,
		Disrupted:        true,
		DisruptionReason: FailureModeRouteResolutionTimeout,
	}, nil
}

// attemptTermination records which guard (the progress watchdog or the
// external-close watcher) terminated an attempt. Both guards run concurrently;
// set serialises them so only the first to fire releases the lease, and reason
// is read after the guards have stopped so the loop knows to skip normal
// failure escalation for an already-handled termination.
type attemptTermination struct {
	mu       sync.Mutex
	reason   string
	guardErr error
}

// set records reason and reports whether this caller was the first to fire. A
// second guard firing observes reason already set and returns false so it does
// not double-release the lease.
func (t *attemptTermination) set(reason string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.reason != "" {
		return false
	}
	t.reason = reason
	return true
}

// releaseFailed rolls back a reserved termination when its lease mutation
// failed. The worker may still cancel the wedged subprocess, but normal attempt
// cleanup remains authoritative because the bead is still claimed.
func (t *attemptTermination) releaseFailed(reason string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.reason == reason {
		t.reason = ""
	}
	t.guardErr = err
}

// get returns the recorded termination reason, or "" when no guard fired.
func (t *attemptTermination) get() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.reason
}

func (t *attemptTermination) err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.guardErr
}

// beadStatusReader is the narrow read capability the external-close watcher
// needs. ExecuteBeadLoopStore satisfies it via Get.
type beadStatusReader interface {
	Get(ctx context.Context, id string) (*bead.Bead, error)
}

// beadClosedExternally reports whether beadID has transitioned to closed —
// the signal that a parallel attempt landed the same bead, so the current
// attempt can stop holding its lease (ddx-8f2e0ebf).
func beadClosedExternally(store beadStatusReader, beadID string) (bool, error) {
	if store == nil || beadID == "" {
		return false, nil
	}
	b, err := store.Get(context.Background(), beadID)
	if err != nil {
		return false, err
	}
	if b == nil {
		return false, nil
	}
	return b.Status == bead.StatusClosed, nil
}

// releaseClosedExternally releases the lease on a bead another attempt closed
// and records a non-failure termination event. The work is already done, so no
// failure mode is escalated. Store.Release leaves a closed bead's status
// untouched (it only clears Owner and the lease), so the bead stays closed.
func releaseClosedExternally(store ExecuteBeadLoopStore, beadID, assignee string, at time.Time) {
	if releaser, ok := store.(leaseReleaser); ok && beadID != "" {
		_ = releaser.Release(beadID, assignee, "")
	}
	if store != nil && beadID != "" {
		body, _ := json.Marshal(map[string]any{
			"reason":  "bead_closed_externally",
			"bead_id": beadID,
		})
		_ = store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "attempt.terminated",
			Summary:   "bead closed by another attempt; released lease without failure",
			Body:      string(body),
			Actor:     assignee,
			Source:    "ddx work",
			CreatedAt: at,
		})
	}
}

// flagWedgedForOperatorAttention atomically releases a wedged worker's lease
// and appends a durable operator_attention event carrying bead-id, attempt-id,
// last_activity_at, and a diagnosis. It mirrors routeResolutionTimeoutReport's
// release-and-flag contract for the phase-empty progress watchdog
// (ddx-dc23f001).
func flagWedgedForOperatorAttention(store ExecuteBeadLoopStore, beadID, assignee, attemptID, phase string, lastActivityAt time.Time, budget time.Duration, at time.Time) {
	diagnosis := fmt.Sprintf(
		"phase-empty heartbeats (harness/model/route empty) persisted past the %s budget while phase=%q; released lease and flagged for operator attention",
		budget, phase,
	)
	if releaser, ok := store.(leaseReleaser); ok && beadID != "" {
		_ = releaser.Release(beadID, assignee, "")
	}
	// Only count as a wedge when the heartbeat itself was stale for at least the
	// phase budget: a phase-empty snapshot whose lastActivityAt is recent indicates
	// the worker was alive and progressing (e.g. slow route resolution) rather than
	// truly stuck (ddx-bd47e2c4).
	if at.Sub(lastActivityAt) >= budget {
		recordConsecutiveWedge(store, beadID, FailureModeProgressWatchdog, at)
	}
	if store != nil && beadID != "" {
		appendWorkEvent(store, beadID, "operator_attention", FailureModeProgressWatchdog, map[string]any{
			"reason":           FailureModeProgressWatchdog,
			"bead_id":          beadID,
			"attempt_id":       attemptID,
			"last_activity_at": lastActivityAt.UTC().Format(time.RFC3339),
			"diagnosis":        diagnosis,
			"budget":           budget.String(),
		}, assignee, at)
	}
}

// executeLoopWedgeMarkerKey is the bead Extra key holding the consecutive-wedge
// marker — recorded when an attempt is released due to a route-resolution
// timeout (ddx-d8970a7b) or a progress-watchdog fire (ddx-dc23f001). A
// route-independent wedge (resolver hang, phase-empty heartbeats) records no
// failed (provider, model) route, so without this marker a freshly released
// wedged bead is immediately re-claimed and re-wedged (ddx-9714eaac, parent
// ddx-8f2e0ebf criterion E). The bead store clears this key on any transition
// to StatusOpen (ddx-5c549120).
const executeLoopWedgeMarkerKey = bead.ExtraConsecutiveWedgeMarker

// WedgeMarker is the JSON shape persisted under executeLoopWedgeMarkerKey. Count
// is the number of consecutive wedges on the bead's most recent claims; it is
// reset to zero (the marker removed) when the bead next makes real progress.
type WedgeMarker struct {
	Count      int    `json:"count"`
	LastReason string `json:"last_reason,omitempty"`
	At         string `json:"at,omitempty"`
}

// readWedgeMarker decodes the consecutive-wedge marker from a bead's Extra.
// Returns a zero-value marker when absent or malformed; never errors. The
// marshal-then-unmarshal round trip also handles a marker reloaded from JSONL.
func readWedgeMarker(extra map[string]any) WedgeMarker {
	if len(extra) == 0 {
		return WedgeMarker{}
	}
	raw, ok := extra[executeLoopWedgeMarkerKey]
	if !ok {
		return WedgeMarker{}
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return WedgeMarker{}
	}
	var m WedgeMarker
	if err := json.Unmarshal(encoded, &m); err != nil {
		return WedgeMarker{}
	}
	return m
}

// recordConsecutiveWedge increments the bead's consecutive-wedge marker and
// persists it via store.Update. Called from the wedge-release primitives
// (routeResolutionTimeoutReport, flagWedgedForOperatorAttention) so a
// route-independent wedge is counted even when no failed-route entry is written.
func recordConsecutiveWedge(store ExecuteBeadLoopStore, beadID, reason string, at time.Time) {
	if store == nil || beadID == "" {
		return
	}
	_ = store.Update(context.Background(), beadID, func(b *bead.Bead) {
		ensureBeadExtra(b)
		m := readWedgeMarker(b.Extra)
		m.Count++
		m.LastReason = reason
		m.At = at.UTC().Format(time.RFC3339)
		b.Extra[executeLoopWedgeMarkerKey] = m
	})
}

// clearConsecutiveWedge removes the bead's consecutive-wedge marker after the
// bead makes real progress (a resolved route / LLM call / commit), so a single
// transient wedge does not permanently sideline a bead that subsequently makes
// progress (ddx-9714eaac AC #2).
func clearConsecutiveWedge(store ExecuteBeadLoopStore, beadID string) {
	if store == nil || beadID == "" {
		return
	}
	_ = store.Update(context.Background(), beadID, func(b *bead.Bead) {
		if len(b.Extra) == 0 {
			return
		}
		delete(b.Extra, executeLoopWedgeMarkerKey)
	})
}

// consecutiveWedgeGuardTrips reports whether the bead's consecutive-wedge count
// has reached threshold, meaning the worker must stop re-claiming it. A
// non-positive threshold falls back to DefaultConsecutiveWedgeThreshold.
func consecutiveWedgeGuardTrips(marker WedgeMarker, threshold int) bool {
	if threshold <= 0 {
		threshold = DefaultConsecutiveWedgeThreshold
	}
	return marker.Count >= threshold
}

// flagConsecutiveWedgeForOperator parks a bead that wedged on consecutive claims
// to proposed (operator attention) and appends a durable operator_attention
// event. It is the consecutive-wedge guard's terminal action: the
// single-threaded worker stops re-claiming and re-wedging this bead while the
// rest of the queue keeps draining (ddx-9714eaac AC #1).
func flagConsecutiveWedgeForOperator(store ExecuteBeadLoopStore, beadID, assignee string, marker WedgeMarker, threshold int, at time.Time) error {
	if store == nil || beadID == "" {
		return nil
	}
	if threshold <= 0 {
		threshold = DefaultConsecutiveWedgeThreshold
	}
	diagnosis := fmt.Sprintf(
		"bead wedged on %d consecutive claims (>= threshold %d; last wedge %q); stopped re-claiming and flagged for operator attention",
		marker.Count, threshold, marker.LastReason,
	)
	suggestedAction := "investigate the repeated wedge (resolver hang / phase-empty heartbeats) and clear the operator-attention state once resolved"
	if err := store.ParkToProposed(beadID, bead.ParkAutoRecoveryFailed, func(b *bead.Bead) {
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason:          FailureModeConsecutiveWedge,
			Since:           at.UTC().Format(time.RFC3339),
			Source:          "ddx work",
			SuggestedAction: suggestedAction,
			Summary:         diagnosis,
		})
		// Clear the wedge marker so that when an operator reopens this bead
		// (sets status back to open), the consecutive-wedge guard does not
		// immediately re-park it without attempting it (ddx-bd47e2c4 AC #2).
		if len(b.Extra) > 0 {
			delete(b.Extra, executeLoopWedgeMarkerKey)
		}
	}); err != nil {
		return err
	}
	appendWorkEvent(store, beadID, "operator_attention", FailureModeConsecutiveWedge, map[string]any{
		"reason":           FailureModeConsecutiveWedge,
		"bead_id":          beadID,
		"count":            marker.Count,
		"threshold":        threshold,
		"last_reason":      marker.LastReason,
		"last_activity_at": marker.At,
		"diagnosis":        diagnosis,
	}, assignee, at)
	return nil
}

func autoReopenRetryableProviderConnectivityProposals(
	ctx context.Context,
	store ExecuteBeadLoopStore,
	actor string,
	at time.Time,
	emit func(string, map[string]any),
) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	proposedStore, ok := store.(proposedOperatorAttentionStore)
	if !ok {
		return 0, nil
	}
	proposed, err := proposedStore.ProposedOperatorAttention()
	if err != nil {
		return 0, err
	}
	var reopened int
	for _, b := range proposed {
		if !isRetryableProviderConnectivityProposal(b) {
			continue
		}
		if err := store.UpdateWithLifecycleStatus(b.ID, bead.StatusOpen, bead.LifecycleTransitionOptions{
			Reason: "provider connectivity failure is retryable route evidence; reopening for autonomous fallback",
			Source: "ddx work",
			Actor:  actor,
		}, func(b *bead.Bead) error {
			ensureBeadExtra(b)
			b.Labels = removeBeadLabels(b.Labels, bead.LabelNeedsHuman, bead.LabelNeedsInvestigation)
			bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{})
			return nil
		}); err != nil {
			return reopened, err
		}
		body, _ := json.Marshal(map[string]any{
			"reason": "provider_connectivity",
			"action": "reopen_for_autonomous_retry",
		})
		_ = store.AppendEvent(b.ID, bead.BeadEvent{
			Kind:      "provider_connectivity.auto_reopen",
			Summary:   "reopened provider-connectivity operator-attention bead for autonomous retry",
			Body:      string(body),
			Actor:     actor,
			Source:    "ddx work",
			CreatedAt: at,
		})
		if emit != nil {
			emit("provider_connectivity.auto_reopen", map[string]any{
				"bead_id": b.ID,
				"reason":  "provider_connectivity",
			})
		}
		reopened++
	}
	return reopened, nil
}

func isRetryableProviderConnectivityProposal(b bead.Bead) bool {
	if b.Status != bead.StatusProposed {
		return false
	}
	meta := bead.GetNeedsHumanMeta(b)
	text := strings.ToLower(strings.Join([]string{
		meta.Reason,
		meta.Summary,
		meta.SuggestedAction,
		meta.Source,
	}, "\n"))
	if !strings.Contains(text, "provider") {
		return false
	}
	return strings.Contains(text, "provider_connectivity") ||
		strings.Contains(text, "provider unreachable") ||
		strings.Contains(text, "unreachable on 2+ attempts") ||
		strings.Contains(text, "connectivity")
}

// emitEscalationAbortedEvent records a kind=execution-escalation-aborted event
// when nextFloorFn returns an error (typically ErrLadderExhausted). Best-effort.
func emitEscalationAbortedEvent(store BeadEventAppender, beadID, actor, provider, model string, actualPower int, at time.Time) {
	if store == nil || beadID == "" {
		return
	}
	summary := fmt.Sprintf("escalation aborted: ladder exhausted at power %d provider=%s", actualPower, provider)
	if model != "" {
		summary += " model=" + model
	}
	appendWorkEvent(store, beadID, "execution-escalation-aborted", summary, map[string]any{
		"provider":     provider,
		"model":        model,
		"actual_power": actualPower,
		"reason":       fmt.Sprintf("ladder exhausted at power %d", actualPower),
	}, actor, at)
}

// emitRouteFailureEvent records a kind=route-failure event capturing the
// failed (provider, model) tuple and the surface error. Best-effort.
func emitRouteFailureEvent(store BeadEventAppender, beadID, actor string, report ExecuteBeadReport, at time.Time) {
	if store == nil || beadID == "" {
		return
	}
	endpoint, timeoutClass := parseProviderConnectivityFacts(report)
	summary := "provider=" + report.Provider
	if report.Model != "" {
		summary += " model=" + report.Model
	}
	if endpoint != "" {
		summary += " endpoint=" + endpoint
	}
	if timeoutClass != "" {
		summary += " (" + timeoutClass + ")"
	}
	summary += " connectivity failure"
	appendWorkEvent(store, beadID, "route-failure", summary, map[string]any{
		"provider":       report.Provider,
		"model":          report.Model,
		"harness":        report.Harness,
		"actual_power":   report.ActualPower,
		"endpoint":       endpoint,
		"timeout_class":  timeoutClass,
		"detail":         report.Detail,
		"error":          report.Error,
		"outcome_reason": FailureModeProviderConnectivity,
	}, actor, at)
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

const (
	defaultLoadPressureBackoffBase = 5 * time.Second
	defaultLoadPressureBackoffMax  = 30 * time.Second
)

func (r ExecuteBeadLoopRuntime) effectiveLoadPressureThreshold() float64 {
	if r.LoadPressureThreshold > 0 && !math.IsNaN(r.LoadPressureThreshold) && !math.IsInf(r.LoadPressureThreshold, 0) {
		return r.LoadPressureThreshold
	}
	return workerstatus.DefaultLoadPressureThreshold
}

func loadPressureBackoffDelay(snapshot workerstatus.LoadPressureSnapshot) time.Duration {
	if snapshot.Threshold <= 0 || math.IsNaN(snapshot.Threshold) || math.IsInf(snapshot.Threshold, 0) ||
		math.IsNaN(snapshot.NormalizedRatio) || snapshot.NormalizedRatio <= snapshot.Threshold {
		return 0
	}
	factor := snapshot.NormalizedRatio / snapshot.Threshold
	maxFactor := float64(defaultLoadPressureBackoffMax) / float64(defaultLoadPressureBackoffBase)
	// Clamp while the value is still floating point. Converting a huge or
	// infinite factor to time.Duration before the clamp can overflow and turn
	// an extreme load ratio into the minimum delay instead of the maximum.
	if math.IsInf(factor, 1) || factor >= maxFactor {
		return defaultLoadPressureBackoffMax
	}
	delay := time.Duration(float64(defaultLoadPressureBackoffBase) * factor)
	if delay < defaultLoadPressureBackoffBase {
		return defaultLoadPressureBackoffBase
	}
	if delay > defaultLoadPressureBackoffMax {
		return defaultLoadPressureBackoffMax
	}
	return delay
}

func waitForLoadPressureBeforeClaim(
	ctx context.Context,
	runtime ExecuteBeadLoopRuntime,
	beadID string,
	emit func(string, map[string]any),
) error {
	// Nil is intentionally a no-op. Production entry points wire the host
	// sampler explicitly; direct Run tests stay deterministic and portable.
	if runtime.LoadPressureSnapshot == nil {
		return nil
	}
	threshold := runtime.effectiveLoadPressureThreshold()
	snapshot := runtime.LoadPressureSnapshot()
	snapshot.Threshold = threshold
	snapshot.Overloaded = snapshot.Supported && snapshot.Available && snapshot.NormalizedRatio > threshold

	sourceState := "available"
	if !snapshot.Supported {
		sourceState = "unsupported"
	} else if !snapshot.Available {
		sourceState = "unavailable"
	}
	fields := map[string]any{
		"bead_id":          beadID,
		"load5":            snapshot.Load5,
		"cpu_count":        snapshot.CPUCount,
		"normalized_ratio": snapshot.NormalizedRatio,
		"threshold":        snapshot.Threshold,
		"supported":        snapshot.Supported,
		"available":        snapshot.Available,
		"source_state":     sourceState,
	}
	if !snapshot.Supported || !snapshot.Available {
		diagnostic := snapshot.Diagnostic
		if diagnostic == "" {
			diagnostic = "load pressure snapshot unavailable"
		}
		fields["diagnostic"] = diagnostic
		// This is best-effort loop telemetry only. It intentionally does not
		// append durable bead evidence or mutate the candidate.
		if emit != nil {
			emit("loop.load_pressure_unavailable", fields)
		}
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "load pressure unavailable before claim (%s): %s; proceeding without backoff\n", beadID, diagnostic)
		}
		return nil
	}
	if !snapshot.Overloaded {
		return nil
	}

	delay := loadPressureBackoffDelay(snapshot)
	fields["delay"] = delay.String()
	fields["delay_ms"] = delay.Milliseconds()
	if emit != nil {
		emit("loop.load_pressure_backoff", fields)
	}
	if runtime.Log != nil {
		_, _ = fmt.Fprintf(runtime.Log, "load pressure backoff before claim (%s): load5=%.2f cpus=%d normalized=%.2f threshold=%.2f delay=%s\n",
			beadID, snapshot.Load5, snapshot.CPUCount, snapshot.NormalizedRatio, snapshot.Threshold, delay)
	}
	sleeper := runtime.LoadPressureSleeper
	if sleeper == nil {
		sleeper = sleepWithContext
	}
	return sleeper(ctx, delay)
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
