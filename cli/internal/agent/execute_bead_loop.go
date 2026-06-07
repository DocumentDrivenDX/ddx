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
	"sync"
	"time"

	agentlib "github.com/easel/fizeau"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/DocumentDrivenDX/ddx/internal/gitlock"
	"github.com/DocumentDrivenDX/ddx/internal/trackerpaths"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
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
	// PreClaimWarnRepeatThreshold is the number of consecutive pre-claim warn
	// fingerprints across distinct bead IDs that trips the operator-attention
	// guard. Zero means use DefaultPreClaimWarnRepeatThreshold.
	PreClaimWarnRepeatThreshold int
	// RouteResolutionTimeout bounds the routing preflight (RoutePreflight) and
	// the resolveRoute viability check so a hung resolver cannot wedge the
	// single-threaded worker for hours while a lease is held (ddx-8f2e0ebf).
	// Zero means use DefaultRouteResolutionTimeout (60s).
	RouteResolutionTimeout time.Duration
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
	// BinaryRefreshCheck, when non-nil, is checked before selecting the next
	// bead. A true result means the caller has started a replacement worker
	// with equivalent arguments and this loop should exit before claiming work.
	// Errors are logged and fail open so update-probing never blocks the queue.
	BinaryRefreshCheck func(ctx context.Context) (bool, error)
	// ProjectRootDirtyCheck, when non-nil, is called at the top of each loop
	// iteration before any bead is claimed. A non-empty return value means the
	// canonical project root has uncommitted tracked non-.ddx changes. The
	// worker emits a single loop.operator_attention event and stops instead of
	// proceeding to claim — preventing a churn loop that repeatedly claims then
	// fails workspace preparation because the root is dirty.
	ProjectRootDirtyCheck func(projectRoot string) []string
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

	// FinalizeDurableAudit, when non-nil, is called once per finalized attempt
	// after the loop has finished writing the attempt's durable tracker state.
	// Errors are terminal for autonomous work: the worker stops so DDx-managed
	// audit outputs are not left dirty while more beads are claimed.
	FinalizeDurableAudit func(report ExecuteBeadReport) error
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

// DefaultRouteResolutionTimeout bounds routing preflight and the resolveRoute
// viability check when ExecuteBeadLoopRuntime.RouteResolutionTimeout is unset.
// A hung resolver previously wedged the single-threaded worker for hours while
// holding a lease (ddx-8f2e0ebf); 60s is short enough to release promptly and
// long enough not to false-trip a slow-but-healthy resolver.
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

// DefaultPreClaimWarnRepeatThreshold is the number of consecutive identical
// pre-claim warn fingerprints across distinct bead IDs required to trip the
// operator-attention guard.
const DefaultPreClaimWarnRepeatThreshold = 5

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
	var submitted *LandResult
	cfg, cfgErr := config.LoadWithWorkingDir(projectRoot)
	_, landErr := LandBeadResult(projectRoot, res, &RealOrchestratorGitOps{}, BeadLandingOptions{
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

// handlePostAttemptDecomposition runs the orchestrator-level splitter when a
// no_changes attempt signals orchestrator_action: decompose. It checks the
// queue-level max_decomposition_depth (not the implementation prompt cap),
// validates the AC map for completeness, and either creates children+deps or
// parks the parent for operator review if the split is lossy, depth-capped, or
// introduces a parent back-edge. The bead must already be unclaimed before
// this is called.
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

	backEdgeChildIDs, err := detectPostAttemptDecompositionBackEdge(ctx, w.Store, candidate.ID, childIDs)
	if err != nil {
		if runtime.Log != nil {
			_, _ = fmt.Fprintf(runtime.Log, "post-attempt decomposition back-edge check failed: %v\n", err)
		}
		return
	}
	if len(backEdgeChildIDs) == 0 {
		return
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
	RequestedProfile    string `json:"requested_profile,omitempty"`
	RoutingIntentSource string `json:"routing_intent_source,omitempty"`
	EstimatedDifficulty string `json:"estimated_difficulty,omitempty"`
	InferredPowerClass  string `json:"inferred_power_class,omitempty"`
	RoutingIntentNote   string `json:"routing_intent_note,omitempty"`
	ResolvedPowerClass  string `json:"resolved_power_class,omitempty"`
	EscalationCount     int    `json:"escalation_count,omitempty"`
	FinalPowerClass     string `json:"final_power_class,omitempty"`
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
// empty execution queue. bead.Store satisfies it via ReadyExecutionBreakdown.
type readyDiagnoser interface {
	ReadyExecutionBreakdown() (bead.ReadyExecutionBreakdown, error)
}

// epicCloser is the optional interface the work loop uses for idle-path closure
// cascade: closing epics whose children have all reached a terminal state.
// bead.Store satisfies both methods.
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
	// preDispatchDirtyPreserver replaces the watch-mode preserve-and-clean path
	// in tests so checkpoint dirt fallback behavior can be exercised without a
	// real git repo.
	preDispatchDirtyPreserver func(projectRoot string, dirtyPaths []string) (*PreDispatchDirtyPreservation, error)

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

// isTransientGitContention reports git index/ref contention errors that are
// transient under concurrent workers and must be retried rather than treated
// as a worker-stopping failure (ddx-23ac2796 and its sibling variants). It
// covers .git/index.lock conflicts plus the DDx tracker-lock timeout sentinel
// and the "unable to write new index file" / "cannot lock ref" forms git emits
// when a concurrent process holds the index/ref. It also covers SIGKILL /
// context-deadline errors: when the 30s durable-audit timeout fires while git
// is blocked behind a concurrent lock, Go sends SIGKILL and exec.CommandContext
// returns "signal: killed"; context.DeadlineExceeded may or may not be
// unwrappable from the ExitError, so both the sentinel and the string forms are
// matched. The audit commit is idempotent on re-entry, so retrying is safe.
// A persistent cause (e.g. ENOSPC) simply keeps releasing+retrying the bead,
// which is still preferable to halting the whole unattended drain.
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
	msg := err.Error()
	if gitlock.IsIndexLockError(msg) {
		return true
	}
	lower := strings.ToLower(msg)
	for _, marker := range []string{
		"unable to write new index file",
		"cannot lock ref",
		"unable to update the ref",
		"tracker lock timeout",
		"signal: killed",
		"context deadline exceeded",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
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
	resetPreClaimIdleStreak := func() {
		preClaimIdleDetail = ""
		preClaimIdleStreak = 0
		preClaimIdleEscalated = false
	}

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
	setExit := func(condition, reason string) {
		exitReason = reason
		result.StopCondition = condition
		result.ExitReason = reason
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
	if reaped, reapErr := reapOrphanedHarnessChildren(
		ctx,
		runtime.ProjectRoot,
		newOrphanHarnessProcessScanner(),
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
	finalizeDurableAuditOrStop := func(candidateID string, report ExecuteBeadReport) bool {
		if runtime.FinalizeDurableAudit == nil {
			return false
		}
		if err := runtime.FinalizeDurableAudit(report); err != nil {
			// Transient .git/index.lock contention from concurrent workers must
			// not stop the drain (ddx-23ac2796). The pending tracker/audit
			// changes remain staged and are committed on a later iteration, so
			// treat lock contention as retryable rather than operator attention —
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
	applyStop := func(input work.StopInput) bool {
		decision, ok := work.ClassifyStop(input)
		if !ok {
			return false
		}
		setExit(string(decision.Condition), decision.ExitReason)
		return true
	}
	if runtime.RoutePreflight != nil {
		routeTimeout := runtime.effectiveRouteResolutionTimeout()
		rerr, timedOut := runRoutePreflightBounded(ctx, routeTimeout, func(c context.Context) error {
			return runtime.RoutePreflight(c, harness, model)
		})
		if timedOut {
			// Slow-but-live resolver: no bead lease is held at startup so there
			// is nothing to release. Log, emit a non-fatal warning, and continue
			// into the drain loop — the next claim iteration calls RoutePreflight
			// again, giving a slow resolver another window to succeed (D1c).
			detail := fmt.Sprintf("routing preflight timed out after %s (harness=%s model=%s); no lease held — continuing",
				routeTimeout, harness, model)
			if runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "routing preflight: %s\n", detail)
			}
			emit("loop.warn", map[string]any{
				"reason":  "startup_route_preflight_timeout",
				"harness": harness,
				"model":   model,
				"timeout": routeTimeout.String(),
				"detail":  detail,
			})
		} else if rerr != nil {
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
		if runtime.ProjectRootDirtyCheck != nil && runtime.ProjectRoot != "" {
			if dirtyPaths := runtime.ProjectRootDirtyCheck(runtime.ProjectRoot); len(dirtyPaths) > 0 {
				detail := fmt.Sprintf(
					"project root has uncommitted tracked changes (%s); resolve before resuming autonomous work",
					strings.Join(dirtyPaths, ", "),
				)
				result.OperatorAttention = &OperatorAttentionStop{
					Reason:      "dirty_project_root",
					ProjectRoot: runtime.ProjectRoot,
					DirtyPaths:  dirtyPaths,
					Message:     detail,
				}
				setExit("OperatorAttention", "operator_attention")
				emit("loop.operator_attention", map[string]any{
					"reason":       "dirty_project_root",
					"project_root": runtime.ProjectRoot,
					"dirty_paths":  dirtyPaths,
					"message":      detail,
				})
				if runtime.Log != nil {
					_, _ = fmt.Fprintf(runtime.Log, "operator attention: %s\n", detail)
				}
				return result, nil
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
					emit("loop.operator_attention", map[string]any{
						"reason":       "preclaim_idle_escalation",
						"bead_id":      idleBeadID,
						"detail":       detail,
						"elapsed_idle": elapsedIdle.String(),
						"idle_count":   preClaimIdleStreak,
					})
					if runtime.Log != nil {
						_, _ = fmt.Fprintf(runtime.Log, "operator attention: pre-claim idled %d consecutive cycles on the same blocker (%s; idle for %s); manual intervention may be required\n", preClaimIdleStreak, detail, elapsedIdle)
					}
				}

				if loopMode != executeloop.ModeWatch {
					setExit("Preflight", reasonCode)
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

			// Idle-path closure cascade (FEAT-004 §Queue Semantics For Epics):
			// before declaring no ready work, close any epics whose children have
			// all reached a terminal state. If any were closed, loop again —
			// those closures may unblock downstream work.
			if closed, _ := w.runEpicClosureCascade(ctx, emit); closed > 0 {
				continue
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
			continue
		}
		freshCandidate, staleSkip, refreshErr := w.refreshCandidateBeforeClaim(ctx, candidate)
		if refreshErr != nil {
			return result, refreshErr
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
				return result, nil
			}
			continue
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
			continue
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
				"queue_rank": queueRankValue(candidate.Extra["queue-rank"]),
				"reason":     claimErr.Error(),
			})
			recordClaimAttempt(false, candidate.ID)
			continue
		}
		recordClaimAttempt(true, candidate.ID)

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
				return result, ctx.Err()
			}
			continue
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
				return result, nil
			}
			if applyStop(work.StopInput{Once: loopMode == executeloop.ModeOnce}) {
				return result, nil
			}
			continue
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
							return result, nil
						}
						continue
					}
				} else {
					if appendPreClaimWarn(candidate.ID, "decomposition_depth_cap", overflowDetail, now().UTC()) {
						return result, nil
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
			intakeResult, intakeErr := runPreClaimIntakeHookWithTimeout(ctx, runtime.PreClaimIntakeHook, candidate.ID, preClaimTimeout)
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
						return result, nil
					}
					continue
				} else {
					emit("pre_claim_intake.warn", eventBody)
					if appendPreClaimWarn(candidate.ID, "timeout", timeoutDetail, now().UTC()) {
						return result, nil
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
				if appendPreClaimWarn(candidate.ID, "system_unready", warning, now().UTC()) {
					return result, nil
				}
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
						return result, nil
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
				if appendPreClaimWarn(candidate.ID, "system_unready", warning, now().UTC()) {
					return result, nil
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
						if appendPreClaimWarn(candidate.ID, "decomposition_hook_unavailable", warning, now().UTC()) {
							return result, nil
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
						return result, nil
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
								return result, nil
							}
							continue
						}
					} else {
						if appendPreClaimWarn(candidate.ID, "decomposition_blocked_best_effort", blockedDetail, now().UTC()) {
							return result, nil
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
								return result, nil
							}
							continue
						}
					} else {
						if appendPreClaimWarn(candidate.ID, "decomposition_error_best_effort", decompErr.Error(), now().UTC()) {
							return result, nil
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
							return result, nil
						}
						continue
					}
				} else {
					if appendPreClaimWarn(candidate.ID, "readiness_best_effort", warning, now().UTC()) {
						return result, nil
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
						return result, nil
					}
					continue
				}
				if appendPreClaimWarn(candidate.ID, "lint_blocked_best_effort", blockMsg, now().UTC()) {
					return result, nil
				}
			}
		}
		freshCandidate, staleSkip, refreshErr = w.refreshClaimedCandidateBeforeAttempt(ctx, candidate)
		if refreshErr != nil {
			if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("Unclaim", assignee, result, err)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				return result, err
			}
			return result, refreshErr
		}
		if staleSkip != nil {
			if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
				_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
					return commitOutcomeError("Unclaim", assignee, result, err)
				})
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				continue
			}
			transientCandidateSkips[candidate.ID] = staleCandidateSkipReason
			if runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "picker skip stale candidate %s before attempt: %s\n", candidate.ID, staleSkip.Detail)
			}
			emitStaleCandidateSkip(emit, candidate.ID, staleSkip, freshCandidate, "pre_attempt")
			if stopAfterNonAttemptSkip() {
				applyStop(work.StopInput{Once: true})
				return result, nil
			}
			continue
		}
		candidate = *freshCandidate

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
		var onRouteResolved func(harness, provider, model string)
		if liveness != nil {
			beadIDForRoute := candidate.ID
			onRouteResolved = func(h, p, m string) { liveness.UpdateRoute(beadIDForRoute, h, m, p) }
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
						Executor:            tryExecutor(w.Executor, onRouteResolved),
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
					Executor:            tryExecutor(w.Executor, onRouteResolved),
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
			emit("loop.attempt_terminated", map[string]any{
				"bead_id": candidate.ID,
				"reason":  termReason,
			})
			if runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "attempt %s terminated by %s; lease released\n", candidate.ID, termReason)
			}
			continue
		}
		// The attempt ran to completion without a wedge guard firing — the route
		// resolved and the agent executed (real progress). Reset any
		// consecutive-wedge marker so a single transient wedge does not
		// permanently sideline a bead that subsequently makes progress
		// (ddx-9714eaac AC #2).
		clearConsecutiveWedge(w.Store, candidate.ID)
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
			if unclaimErr := releaseWorkerClaim(w.Store, candidate.ID, assignee); unclaimErr != nil {
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
					continue
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
			return result, nil
		}
		if IsResourceExhaustedStatus(report.Status) {
			result.Attempts++
			setExit("ResourceExhausted", "resource_exhausted")
			if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
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
			if finalizeDurableAuditOrStop(candidate.ID, report) {
				return result, nil
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
			if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
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
			if finalizeDurableAuditOrStop(candidate.ID, report) {
				return result, nil
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
				transientCandidateSkips[candidate.ID] = routingUnavailableSkipReason
				continue
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
			if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
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
				return result, nil
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
			if unclaimErr := releaseWorkerClaim(w.Store, candidate.ID, assignee); unclaimErr != nil && runtime.Log != nil {
				_, _ = fmt.Fprintf(runtime.Log, "interrupted attempt cleanup error (Unclaim %s): %v\n", candidate.ID, unclaimErr)
			}
			if finalizeDurableAuditOrStop(candidate.ID, report) {
				return result, ctxErr
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
				if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
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
			appendLoopRoutingEvidence(w.Store, candidate, report, now().UTC(), readFailedRoutes(candidate.Extra))
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
						return result, ctx.Err()
					}
					continue
				}
				if reviewOut.Approved {
					result.Successes++
					result.LastSuccessAt = now().UTC()
				} else {
					result.Failures++
					result.LastFailureStatus = report.Status
				}
			} else {
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
				parsed := ParseNoChangesRationale(report.NoChangesRationale)
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
			case agenttry.NoChangesActionBadAttemptNoCooldown:
				emitReviewerSkippedEmptyDiff(w.Store, candidate.ID, assignee, now().UTC())
				if err := applyNoChangesBadAttemptEscalation(w.Store, candidate.ID, assignee, noChanges, report.ActualPower, w.EscalationNextFloor); err != nil {
					_ = commitOutcome(ctx, w.Store, candidate.ID, func() error {
						return commitOutcomeError("applyNoChangesBadAttemptEscalation", assignee, result, err)
					})
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					continue
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
				if err := releaseWorkerClaim(w.Store, candidate.ID, assignee); err != nil {
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
					if report.Detail == mixedCommitAndNoChangesRationaleReason &&
						countRecentMixedCommitEvents(w.Store, candidate.ID, mixedCommitCooldownWindow, now().UTC()) >= 1 {
						if parkErr := parkToProposedSimple(w.Store, candidate.ID, bead.ParkNoChangesOperatorRequired,
							"circuit-breaker: "+mixedCommitAndNoChangesRationaleReason+" repeated within 24h; operator review required",
							now().UTC()); parkErr != nil && runtime.Log != nil {
							_, _ = fmt.Fprintf(runtime.Log, "mixed_commit circuit-breaker park failed for %s: %v (continuing)\n", candidate.ID, parkErr)
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
					return result, ctx.Err()
				}
				continue
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
					return result, ctx.Err()
				}
				continue
			}
		}
		if finalizeDurableAuditOrStop(candidate.ID, report) {
			return result, nil
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

func hasGuardSkips(skips []pickerSkip) bool {
	for _, skip := range skips {
		switch skip.Reason {
		case "label_filter", "in_attempted", "claim_race", "eligibility_filter", "retry_cooldown", "target_bead", staleCandidateSkipReason:
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
		case work.IsTrackerContentionPreClaimSkipReason(skip.Reason):
			if reasonCode != preClaimIdleReasonSystemic {
				reasonCode = preClaimIdleReasonTrackerContention
				detail = work.TrackerContentionPreClaimDetail(skip.Reason)
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

func queueRankValue(raw any) any {
	rank, ok := parseQueueRank(raw)
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
	return fresh, nil, nil
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
			if _, skipped := w.transientCandidateSkips[candidate.ID]; skipped {
				skips = append(skips, pickerSkip{BeadID: candidate.ID, Priority: candidate.Priority, BeadRank: queueRankPtr(candidate.Extra["queue-rank"]), Reason: staleCandidateSkipReason})
				continue
			}
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
func appendLoopRoutingEvidence(store BeadEventAppender, target bead.Bead, report ExecuteBeadReport, createdAt time.Time, failedRoutes []FailedRouteEntry) {
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
	chain := make([]map[string]any, 0, len(failedRoutes))
	for _, e := range failedRoutes {
		chain = append(chain, map[string]any{
			"provider":     e.Provider,
			"model":        e.Model,
			"actual_power": e.ActualPower,
			"reason":       e.Reason,
		})
	}
	intentSource, estimatedDifficulty, requestedPowerClass, _ := executionRoutingIntentFacts(target, report)
	body, err := json.Marshal(map[string]any{
		"resolved_provider":     provider,
		"resolved_model":        report.Model,
		"fallback_chain":        chain,
		"routing_intent_source": intentSource,
		"estimated_difficulty":  estimatedDifficulty,
		"requested_profile":     report.RequestedProfile,
		"requested_power_class": requestedPowerClass,
		"routing_intent_note":   report.RoutingIntentNote,
		"resolved_power_class":  report.ResolvedPowerClass,
		"escalation_count":      report.EscalationCount,
		"final_power_class":     report.FinalPowerClass,
	})
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
	intentSource, estimatedDifficulty, requestedPowerClass, rejectedRoutePins := executionRoutingIntentFacts(target, report)
	body := map[string]any{
		"bead_id":                 target.ID,
		"attempt_id":              report.AttemptID,
		"routing_intent_source":   intentSource,
		"estimated_difficulty":    estimatedDifficulty,
		"requested_power_class":   requestedPowerClass,
		"requested_profile":       report.RequestedProfile,
		"actual_harness":          report.Harness,
		"actual_provider":         report.Provider,
		"actual_model":            report.Model,
		"actual_power":            report.ActualPower,
		"routing_intent_degraded": false,
		"routing_intent_note":     "",
		"rejected_route_pins":     rejectedRoutePins,
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
	if requestedPowerClass != "" {
		summary += fmt.Sprintf(" powerClass=%s", requestedPowerClass)
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

func executionRoutingIntentFacts(target bead.Bead, report ExecuteBeadReport) (source, estimatedDifficulty, requestedPowerClass string, rejectedRoutePins []string) {
	intent := escalation.ParseExecutionHint(&target)
	source = string(intent.Source)
	estimatedDifficulty = string(intent.EstimatedDifficulty)
	rejectedRoutePins = intent.RejectedRoutePins

	if report.RoutingIntentSource != "" {
		source = report.RoutingIntentSource
		switch escalation.ExecutionIntentSource(report.RoutingIntentSource) {
		case escalation.ExecutionIntentSourceCLIPassthru, escalation.ExecutionIntentSourceProject:
			estimatedDifficulty = ""
			requestedPowerClass = ""
		case escalation.ExecutionIntentSourceDefault:
			estimatedDifficulty = ""
			requestedPowerClass = string(escalation.PowerStandard)
		case escalation.ExecutionIntentSourceReadiness:
			estimatedDifficulty = ""
			if strings.TrimSpace(report.EstimatedDifficulty) != "" {
				readinessIntent := escalation.ResolveExecutionHint(escalation.ExecutionHintInput{
					ReadinessEstimatedDifficulty: report.EstimatedDifficulty,
				})
				requestedPowerClass = string(readinessIntent.InferredPowerClass)
			}
		}
	}
	if report.EstimatedDifficulty != "" {
		estimatedDifficulty = report.EstimatedDifficulty
	}

	if requestedPowerClass == "" &&
		source != string(escalation.ExecutionIntentSourceCLIPassthru) &&
		source != string(escalation.ExecutionIntentSourceProject) {
		requestedPowerClass = string(intent.InferredPowerClass)
	}
	if report.InferredPowerClass != "" {
		requestedPowerClass = report.InferredPowerClass
	}
	return source, estimatedDifficulty, requestedPowerClass, rejectedRoutePins
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
	// Endpoint is the concrete provider URL the failed dispatch dialed
	// (e.g. http://vidar:1235/v1/chat/completions), parsed from the worker's
	// transport error. Actionable route fact for operators diagnosing an
	// unreachable endpoint; empty when no URL could be parsed.
	Endpoint string `json:"endpoint,omitempty"`
	// TimeoutClass is the transport failure class parsed from the error
	// (e.g. "i/o timeout", "connection refused", "no route to host"). It
	// distinguishes a down endpoint (connection refused) from an unreachable
	// one (i/o timeout / no route to host) so remediation can be targeted.
	TimeoutClass string `json:"timeout_class,omitempty"`
	At           string `json:"at,omitempty"`
	Count        int    `json:"count,omitempty"`
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
				endpoint := e.Endpoint
				if endpoint == "" {
					endpoint = collapsed[i].Endpoint
				}
				timeoutClass := e.TimeoutClass
				if timeoutClass == "" {
					timeoutClass = collapsed[i].TimeoutClass
				}
				collapsed[i] = FailedRouteEntry{
					Provider:     e.Provider,
					Model:        e.Model,
					ActualPower:  e.ActualPower,
					Reason:       e.Reason,
					Endpoint:     endpoint,
					TimeoutClass: timeoutClass,
					At:           e.At,
					Count:        existingCount + ec,
				}
			} else {
				collapsed[i].Count = existingCount + ec
				if collapsed[i].Endpoint == "" {
					collapsed[i].Endpoint = e.Endpoint
				}
				if collapsed[i].TimeoutClass == "" {
					collapsed[i].TimeoutClass = e.TimeoutClass
				}
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
			if entry.Endpoint != "" {
				existing[i].Endpoint = entry.Endpoint
			}
			if entry.TimeoutClass != "" {
				existing[i].TimeoutClass = entry.TimeoutClass
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

// filterShieldedExcludedRoutes drops any ExcludedRoute whose Provider names a
// shielded subscription CLI harness (binary on PATH, billing==subscription).
// Such a route must never be hard-excluded from a RouteRequest just because a
// recent connectivity blip was recorded against it: doing so can empty the
// candidate set during a local-fleet outage where the subscription harness is
// the only live option (the no_viable_provider bug). shielded is the set
// returned by shieldedSubscriptionHarnesses; an empty set is a no-op.
func filterShieldedExcludedRoutes(excluded []agentlib.ExcludedRoute, shielded map[string]struct{}) []agentlib.ExcludedRoute {
	if len(excluded) == 0 || len(shielded) == 0 {
		return excluded
	}
	out := excluded[:0:0]
	for _, e := range excluded {
		if isShieldedSubscriptionHarness(e.Provider, shielded) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// leaseReleaser is the narrow capability CheckAndApplyRouteExclusions needs to
// atomically release a held lease on a route-resolution timeout. *bead.Store
// satisfies it via Store.Release (ddx-449baa1d).
type leaseReleaser interface {
	Release(id, assignee, targetStatus string) error
}

// resolveRouteBounded runs resolveRoute under a route-resolution deadline. The
// resolver is invoked in a goroutine and selected against the deadline so a
// resolver that ignores context cancellation cannot wedge the single-threaded
// worker past the timeout (the hang observed in ddx-8f2e0ebf). timedOut is true
// only when the deadline fired before the resolver returned; a parent-context
// cancellation reports timedOut=false.
func resolveRouteBounded(
	ctx context.Context,
	timeout time.Duration,
	resolveRoute func(ctx context.Context, req agentlib.RouteRequest) (*agentlib.RouteDecision, error),
	req agentlib.RouteRequest,
) (decision *agentlib.RouteDecision, err error, timedOut bool) {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type routeResult struct {
		decision *agentlib.RouteDecision
		err      error
	}
	done := make(chan routeResult, 1)
	go func() {
		d, e := resolveRoute(tctx, req)
		done <- routeResult{decision: d, err: e}
	}()
	select {
	case res := <-done:
		return res.decision, res.err, false
	case <-tctx.Done():
		return nil, tctx.Err(), errors.Is(tctx.Err(), context.DeadlineExceeded)
	}
}

// runRoutePreflightBounded runs fn under a route-resolution deadline, returning
// timedOut=true only when the deadline fired before fn returned. fn runs in a
// goroutine so a preflight resolver that ignores context cancellation cannot
// wedge the worker past the timeout.
func runRoutePreflightBounded(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) (err error, timedOut bool) {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- fn(tctx) }()
	select {
	case e := <-done:
		return e, false
	case <-tctx.Done():
		return tctx.Err(), errors.Is(tctx.Err(), context.DeadlineExceeded)
	}
}

// routeResolutionTimeoutReport atomically releases the bead's lease (via
// Store.Release when the store supports it), appends a route.timeout event
// carrying bead-id, attempt-id, last_activity_at, and a diagnosis, and returns
// the execution_failed / route_resolution_timeout report marked Disrupted=true
// so the loop does not apply a no-progress cooldown — with fizeau quota
// fail-open (F1) a slow-but-live resolver is retryable without operator
// attention (D1c). lastActivityAt is reserved for D3 wedge-counter gating.
func routeResolutionTimeoutReport(store ExecuteBeadLoopStore, beadID, assignee, attemptID string, now time.Time, timeout time.Duration, lastActivityAt time.Time) ExecuteBeadReport {
	diagnosis := fmt.Sprintf(
		"route resolution exceeded %s; released lease (retryable — next claim will re-resolve)",
		timeout,
	)
	if releaser, ok := store.(leaseReleaser); ok && beadID != "" {
		_ = releaser.Release(beadID, assignee, "")
	}
	if store != nil && beadID != "" {
		body, _ := json.Marshal(map[string]any{
			"reason":           FailureModeRouteResolutionTimeout,
			"bead_id":          beadID,
			"attempt_id":       attemptID,
			"last_activity_at": now.UTC().Format(time.RFC3339),
			"heartbeat_at":     lastActivityAt.UTC().Format(time.RFC3339),
			"diagnosis":        diagnosis,
			"timeout":          timeout.String(),
		})
		_ = store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "route.timeout",
			Summary:   FailureModeRouteResolutionTimeout,
			Body:      string(body),
			Actor:     assignee,
			Source:    "ddx work",
			CreatedAt: now.UTC(),
		})
	}
	return ExecuteBeadReport{
		BeadID:           beadID,
		Status:           ExecuteBeadStatusExecutionFailed,
		Detail:           diagnosis,
		OutcomeReason:    FailureModeRouteResolutionTimeout,
		Disrupted:        true,
		DisruptionReason: FailureModeRouteResolutionTimeout,
	}
}

// attemptTermination records which guard (the progress watchdog or the
// external-close watcher) terminated an attempt. Both guards run concurrently;
// set serialises them so only the first to fire releases the lease, and reason
// is read after the guards have stopped so the loop knows to skip normal
// failure escalation for an already-handled termination.
type attemptTermination struct {
	mu     sync.Mutex
	reason string
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

// get returns the recorded termination reason, or "" when no guard fired.
func (t *attemptTermination) get() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.reason
}

// beadStatusReader is the narrow read capability the external-close watcher
// needs. ExecuteBeadLoopStore satisfies it via Get.
type beadStatusReader interface {
	Get(args ...any) (*bead.Bead, error)
}

// beadClosedExternally reports whether beadID has transitioned to closed —
// the signal that a parallel attempt landed the same bead, so the current
// attempt can stop holding its lease (ddx-8f2e0ebf).
func beadClosedExternally(store beadStatusReader, beadID string) (bool, error) {
	if store == nil || beadID == "" {
		return false, nil
	}
	b, err := store.Get(beadID)
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
		body, _ := json.Marshal(map[string]any{
			"reason":           FailureModeProgressWatchdog,
			"bead_id":          beadID,
			"attempt_id":       attemptID,
			"last_activity_at": lastActivityAt.UTC().Format(time.RFC3339),
			"diagnosis":        diagnosis,
			"budget":           budget.String(),
		})
		_ = store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "operator_attention",
			Summary:   FailureModeProgressWatchdog,
			Body:      string(body),
			Actor:     assignee,
			Source:    "ddx work",
			CreatedAt: at,
		})
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
// Returns a zero-value marker when absent or malformed; never errors. Mirrors
// readFailedRoutes' marshal-then-unmarshal round-trip so a marker reloaded from
// JSONL (a map[string]any) decodes the same as one set in-process.
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
	body, _ := json.Marshal(map[string]any{
		"reason":           FailureModeConsecutiveWedge,
		"bead_id":          beadID,
		"count":            marker.Count,
		"threshold":        threshold,
		"last_reason":      marker.LastReason,
		"last_activity_at": marker.At,
		"diagnosis":        diagnosis,
	})
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "operator_attention",
		Summary:   FailureModeConsecutiveWedge,
		Body:      string(body),
		Actor:     assignee,
		Source:    "ddx work",
		CreatedAt: at,
	})
	return nil
}

// CheckAndApplyRouteExclusions builds ExcludedRoutes from the bead's
// failed-routes list, calls resolveRoute to verify a viable candidate
// remains, and when none does returns a no-viable-provider report so the
// current execution path can escalate without mutating bead metadata.
//
// The resolveRoute viability check is bounded by timeout (defaulting to
// DefaultRouteResolutionTimeout): on timeout the lease is released atomically,
// an operator-attention event is emitted, and a route_resolution_timeout report
// is returned (ddx-d8970a7b).
//
// Returns (report, true) when dispatch should be skipped (all routes
// excluded, resolveRoute returned an error, or route resolution timed out);
// (zero, false) when the caller may proceed with dispatch normally.
// A nil resolveRoute is treated as a no-op and returns false.
func CheckAndApplyRouteExclusions(
	ctx context.Context,
	svc agentlib.FizeauService,
	store ExecuteBeadLoopStore,
	beadID, assignee string,
	extra map[string]any,
	now time.Time,
	minPower int,
	resolveRoute func(ctx context.Context, req agentlib.RouteRequest) (*agentlib.RouteDecision, error),
	nextFloorFn func(int) (int, error),
	timeout time.Duration,
	attemptID string,
	lastActivityAt time.Time,
) (ExecuteBeadReport, bool) {
	if resolveRoute == nil {
		return ExecuteBeadReport{}, false
	}
	failedRoutes := readFailedRoutes(extra)
	excluded := buildExcludedRoutes(failedRoutes, now, RouteExclusionWindow)
	// Never hard-exclude a subscription CLI harness whose binary is on PATH:
	// the failed-route entries replay connectivity blips that must not empty
	// the candidate set during a local-fleet outage (same shield as the
	// tracker-seeding path in service_run.go — single source of truth).
	shielded := shieldedSubscriptionHarnesses(ctx, svc)
	excluded = filterShieldedExcludedRoutes(excluded, shielded)
	if len(excluded) == 0 {
		return ExecuteBeadReport{}, false
	}
	req := agentlib.RouteRequest{
		MinPower:       minPower,
		ExcludedRoutes: excluded,
	}
	if timeout <= 0 {
		timeout = DefaultRouteResolutionTimeout
	}
	_, routeErr, timedOut := resolveRouteBounded(ctx, timeout, resolveRoute, req)
	if timedOut {
		return routeResolutionTimeoutReport(store, beadID, assignee, attemptID, now, timeout, lastActivityAt), true
	}
	if routeErr == nil {
		return ExecuteBeadReport{}, false
	}
	// No viable candidate at minPower with the current exclusions: let the
	// current execution path escalate in-memory rather than persisting a bead
	// retry floor.
	nextFloor := minPower + 1
	if nextFloorFn != nil {
		if floor, err := nextFloorFn(minPower); err == nil {
			nextFloor = floor
		}
	}
	detail := fmt.Sprintf(
		"ResolveRoute: no viable routing candidate: all routes at power %d excluded by recent failures; escalating current retry floor to %d",
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
// to the bead's failed-route list. The lifecycle status is unchanged (bead
// stays open). Retry power escalation is derived from route-failure evidence
// in the current execution path rather than a persisted bead floor.
//
// When the ladder is exhausted, it emits an execution-escalation-aborted event.
// Repeated provider connectivity failures remain retryable route-health
// evidence: the bead stays open and Fizeau receives recent failed routes so it
// can pick another viable route without blocking the dependency tree behind an
// operator-attention state.
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
		repeatFailure bool
		repeatCount   int
	)
	endpoint, timeoutClass := parseProviderConnectivityFacts(report)
	if err := store.Update(context.Background(), beadID, func(b *bead.Bead) {
		existing := readFailedRoutes(b.Extra)
		for _, e := range existing {
			if e.Provider == report.Provider && e.Model == report.Model {
				repeatFailure = true
				repeatCount = e.Count + 1
				break
			}
		}
		appendFailedRoute(b, FailedRouteEntry{
			Provider:     report.Provider,
			Model:        report.Model,
			ActualPower:  report.ActualPower,
			Reason:       FailureModeProviderConnectivity,
			Endpoint:     endpoint,
			TimeoutClass: timeoutClass,
			At:           at.UTC().Format(time.RFC3339),
		})
	}); err != nil {
		return err
	}
	if repeatFailure {
		body, _ := json.Marshal(map[string]any{
			"provider":      report.Provider,
			"model":         report.Model,
			"actual_power":  report.ActualPower,
			"count":         repeatCount,
			"reason":        FailureModeProviderConnectivity,
			"endpoint":      endpoint,
			"timeout_class": timeoutClass,
			"action":        "keep_open_for_autonomous_retry",
		})
		_ = store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "provider_connectivity.auto_retry",
			Summary:   "repeated provider connectivity failure kept open for autonomous retry",
			Body:      string(body),
			Actor:     actor,
			Source:    "ddx work",
			CreatedAt: at,
		})
	}
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
	endpoint, timeoutClass := parseProviderConnectivityFacts(report)
	body, err := json.Marshal(map[string]any{
		"provider":       report.Provider,
		"model":          report.Model,
		"harness":        report.Harness,
		"actual_power":   report.ActualPower,
		"endpoint":       endpoint,
		"timeout_class":  timeoutClass,
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
	if endpoint != "" {
		summary += " endpoint=" + endpoint
	}
	if timeoutClass != "" {
		summary += " (" + timeoutClass + ")"
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
