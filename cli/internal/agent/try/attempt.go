package try

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ratelimitpolicy"
)

const (
	StatusExecutionFailed            = "execution_failed"
	StatusLandConflict               = "land_conflict"
	StatusLandRetry                  = "land_retry"
	StatusLandOperatorAttention      = "land_operator_attention"
	StatusSuccess                    = "success"
	StatusNoChanges                  = "no_changes"
	StatusAlreadySatisfied           = "already_satisfied"
	StatusPushFailed                 = "push_failed"
	StatusPushConflict               = "push_conflict"
	StatusDeclinedNeedsDecomposition = "declined_needs_decomposition"
)

const (
	PushFailedReasonPrefix   = "landed locally; push failed:"
	PushConflictReasonPrefix = "landed locally; push conflict:"
)

type Report struct {
	BeadID                      string
	AttemptID                   string
	WorkerID                    string
	Harness                     string
	Provider                    string
	Model                       string
	ActualPower                 int
	Status                      string
	Detail                      string
	SessionID                   string
	BaseRev                     string
	ResultRev                   string
	ImplementationRev           string
	LandedRev                   string
	TargetBranch                string
	EvidenceRev                 string
	ProjectRoot                 string
	PreserveRef                 string
	RetryAfter                  string
	NoChangesRationale          string
	ReviewVerdict               string
	ReviewRationale             string
	CycleTrace                  []ExecutionCycleTrace
	PowerClass                  string
	ProbeResult                 string
	CostUSD                     float64
	DurationMS                  int64
	RequestedProfile            string
	RoutingIntentSource         string
	EstimatedDifficulty         string
	InferredPowerClass          string
	ResolvedPowerClass          string
	EscalationCount             int
	FinalPowerClass             string
	DecompositionRecommendation []string
	DecompositionRationale      string
	DecomposedChildIDs          []string
	ExecutionDecision           string
	Disrupted                   bool
	DisruptionReason            string
	OutcomeReason               string
	Error                       string
	Stderr                      string
	RateLimitBudget             time.Duration
	ResourceExhausted           any
}

type ExecutionCycleRouteFacts struct {
	Harness         string
	Provider        string
	Model           string
	ActualPower     int
	RouteReason     string
	ResolvedBaseURL string
}

type ExecutionCycleRequestedRouteFacts struct {
	Harness             string
	Provider            string
	Model               string
	Profile             string
	RoutingIntentSource string
	EstimatedDifficulty string
	InferredPowerClass  string
	RequestedPowerClass string
}

type ExecutionCycleReviewResult struct {
	Verdict        string
	Rationale      string
	Classification string
	PerAC          []ReviewAC
	Findings       []Finding
}

type ExecutionCycleTrace struct {
	CycleIndex           int
	AttemptID            string
	ResultRev            string
	ImplementerRoute     ExecutionCycleRouteFacts
	RequestedRoute       ExecutionCycleRequestedRouteFacts
	ActualRoute          ExecutionCycleRouteFacts
	ReviewGroupID        string
	ReviewerIndices      []int
	ReviewVerdicts       []string
	ReviewResult         ExecutionCycleReviewResult
	FinalDecision        string
	FailureClass         string
	RetryAction          string
	EscalationCount      int
	ReviewStatus         string
	ReviewSkipReason     string
	ReviewClassification string
	LandStatus           string
	ReconcileStatus      string
	DecomposedChildIDs   []string
	ExecutionDecision    string
}

type ReviewAC struct {
	Number   int
	Item     string
	Grade    string
	Evidence string
}

type Finding struct {
	Severity string
	Summary  string
	Location string
}

type Executor interface {
	Execute(ctx context.Context, beadID string) (Report, error)
}

type ExecutorFunc func(ctx context.Context, beadID string) (Report, error)

func (f ExecutorFunc) Execute(ctx context.Context, beadID string) (Report, error) {
	return f(ctx, beadID)
}

type Store interface {
	AppendEvent(beadID string, ev bead.BeadEvent) error
	CloseWithEvidence(beadID, sessionID, sha string) error
	Unclaim(beadID string) error
	SetExecutionCooldown(beadID string, until time.Time, status, detail, baseRev string) error
	UpdateWithLifecycleStatus(id string, status string, opts bead.LifecycleTransitionOptions, mutate func(*bead.Bead) error) error
	IncrNoChangesCount(beadID string) (int, error)
}

type ConflictAutoRecoverFn func(wd, preserveRef string) (string, error)

type ConflictResolverFn func(ctx context.Context, beadID, preserveRef, projectRoot string) (newTip string, isBlocking bool, err error)

type AttemptOpts struct {
	Bead                   bead.Bead
	Executor               Executor
	Store                  Store
	ProjectRoot            string
	SatisfactionChecker    SatisfactionChecker
	VerificationRunner     VerificationCommandRunner
	AutoRecover            ConflictAutoRecoverFn
	ConflictResolver       ConflictResolverFn
	Assignee               string
	Now                    func() time.Time
	Cooldown               time.Duration
	RateLimitBudget        time.Duration
	RateLimitPerWaitCap    time.Duration
	RateLimitWaitEvaluator func(retryAfter time.Duration, attempt int, elapsed, budget, perWaitCap time.Duration) RateLimitWaitDecision
	RateLimitSleep         func(ctx context.Context, d time.Duration) error
	RateLimitNow           func() time.Time
	RateLimitOnRetry       func(ctx context.Context, info RateLimitRetryInfo)
}

type Disposition string

const (
	OutcomeReported Disposition = "reported"
	OutcomeSuccess  Disposition = "merged"
	OutcomePark     Disposition = "park"
)

type Outcome struct {
	Report      Report
	Disposition Disposition
	NoChanges   *NoChangesOutcome
	Parking     *ParkingOutcome
	StoreErrOp  string
	StoreErr    error
}

// ParkingOutcome carries the structured instructions for outcomes that must
// be parked instead of retried immediately.
type ParkingOutcome struct {
	Unclaim              bool
	RetryAfter           time.Time
	RunPostAttemptTriage bool
	Event                *ParkingEvent
}

// ParkingEvent is the event template returned by Attempt for structured park
// outcomes. The execute loop converts it into a bead.BeadEvent when it applies
// the parking instructions.
type ParkingEvent struct {
	Kind    string
	Summary string
	Body    string
}

func Attempt(ctx context.Context, store Store, beadID string, opts AttemptOpts) (Outcome, error) {
	if opts.Executor == nil {
		return Outcome{}, fmt.Errorf("try attempt: executor is required")
	}
	rateCfg := resolveRateLimitRetryConfig(opts)
	var (
		report Report
		err    error
	)
	for retryCount, elapsed := 0, time.Duration(0); ; {
		report, err = opts.Executor.Execute(ctx, beadID)
		if err != nil {
			report = Report{
				BeadID: beadID,
				Status: StatusExecutionFailed,
				Detail: err.Error(),
				Error:  err.Error(),
			}
		}
		if report.BeadID == "" {
			report.BeadID = beadID
		}
		if report.Status == "" {
			report.Status = StatusExecutionFailed
		}
		if ctx != nil && ctx.Err() != nil {
			if err == nil {
				err = ctx.Err()
			}
			if report.Error == "" {
				report.Error = ctx.Err().Error()
			}
			if report.Detail == "" {
				report.Detail = ctx.Err().Error()
			}
			return Outcome{Report: report, Disposition: OutcomeReported}, err
		}

		rateLimited := IsRateLimitReport(&report)
		effectiveBudget := rateCfg.Budget
		if opts.RateLimitBudget == 0 && report.RateLimitBudget != 0 {
			effectiveBudget = report.RateLimitBudget
		}
		decision, retryAfter := evaluateRateLimitAttempt(report, rateCfg, retryCount+1, elapsed, effectiveBudget)
		if !decision.ShouldRetry {
			if decision.Reason != "" {
				report.Error = decision.Reason
			}
			if rateLimited && decision.Reason != "" && rateCfg.OnRetry != nil {
				rateCfg.OnRetry(ctx, RateLimitRetryInfo{
					Attempt:    retryCount + 1,
					Source:     decision.Source,
					Report:     &report,
					Elapsed:    elapsed,
					RetryAfter: retryAfter,
					OverBudget: true,
				})
			}
			break
		}

		retryCount++
		info := RateLimitRetryInfo{
			Attempt:    retryCount,
			Wait:       decision.Wait,
			Source:     decision.Source,
			Report:     &report,
			Elapsed:    elapsed,
			RetryAfter: retryAfter,
		}
		if rateCfg.OnRetry != nil {
			rateCfg.OnRetry(ctx, info)
		}
		if err := rateCfg.Sleep(ctx, decision.Wait); err != nil {
			return Outcome{}, err
		}
		elapsed += decision.Wait
	}

	if ShouldAttemptConflictRecovery(report, opts.ProjectRoot) {
		if opts.Store == nil {
			opts.Store = store
		}
		return ConflictRecoveryOutcome(ctx, ConflictRecoveryInput{
			Bead:             opts.Bead,
			Report:           report,
			ProjectRoot:      opts.ProjectRoot,
			AutoRecover:      opts.AutoRecover,
			ConflictResolver: opts.ConflictResolver,
			Store:            opts.Store,
			Assignee:         opts.Assignee,
			Now:              opts.Now,
			Cooldown:         opts.Cooldown,
		}), err
	}

	if report.Status == StatusNoChanges {
		if opts.Store == nil {
			opts.Store = store
		}
		if opts.Store == nil {
			return Outcome{Report: report, StoreErrOp: "IncrNoChangesCount", StoreErr: fmt.Errorf("try attempt: no_changes store is required")}, nil
		}
		noChangesCount, countErr := opts.Store.IncrNoChangesCount(beadID)
		if countErr != nil {
			return Outcome{Report: report, StoreErrOp: "IncrNoChangesCount", StoreErr: countErr}, nil
		}
		report.EscalationCount = noChangesCount
		noChangesOut, adjudicatedReport, adjErr := adjudicateNoChangesContract(ctx, beadID, report, opts.ProjectRoot, noChangesCount, opts.SatisfactionChecker, opts.VerificationRunner)
		if adjErr != nil {
			return Outcome{Report: report, StoreErrOp: "adjudicateNoChanges", StoreErr: adjErr}, nil
		}
		return Outcome{
			Report:      adjudicatedReport,
			Disposition: OutcomeReported,
			NoChanges:   &noChangesOut,
		}, nil
	}

	if report.Status == StatusDeclinedNeedsDecomposition {
		effectiveStore := opts.Store
		if effectiveStore == nil {
			effectiveStore = store
		}
		if effectiveStore != nil {
			if err := effectiveStore.UpdateWithLifecycleStatus(beadID, bead.StatusOpen, bead.LifecycleTransitionOptions{
				Reason: "declined: bead requires decomposition before execution",
				Source: "legacy agent try",
			}, func(b *bead.Bead) error {
				if b.Extra == nil {
					b.Extra = make(map[string]any)
				}
				b.Extra[bead.ExtraExecutionElig] = false
				return nil
			}); err != nil {
				return Outcome{Report: report, StoreErrOp: "UpdateWithLifecycleStatus", StoreErr: err}, nil
			}
		}
		event := buildDecompositionParkingEvent(report)
		return Outcome{
			Report:      report,
			Disposition: OutcomePark,
			Parking: &ParkingOutcome{
				Unclaim:              true,
				RunPostAttemptTriage: true,
				Event:                event,
			},
		}, nil
	}

	if report.Status == StatusPushConflict {
		parkUntil := nowFn(opts.Now).UTC().Add(maxAttemptCooldown)
		report.RetryAfter = parkUntil.Format(time.RFC3339)
		event := buildPushConflictParkingEvent(report)
		return Outcome{
			Report:      report,
			Disposition: OutcomePark,
			Parking: &ParkingOutcome{
				Unclaim:              true,
				RetryAfter:           parkUntil,
				RunPostAttemptTriage: true,
				Event:                event,
			},
		}, nil
	}

	return Outcome{Report: report, Disposition: OutcomeReported}, err
}

func nowFn(fn func() time.Time) time.Time {
	if fn != nil {
		return fn()
	}
	return time.Now()
}

const maxAttemptCooldown = 24 * time.Hour

func buildDecompositionParkingEvent(report Report) *ParkingEvent {
	body, mErr := json.Marshal(map[string]any{
		"rationale":            report.DecompositionRationale,
		"recommended_subbeads": report.DecompositionRecommendation,
		"detail":               report.Detail,
		"base_rev":             report.BaseRev,
		"session_id":           report.SessionID,
	})
	bodyStr := ""
	if mErr == nil {
		bodyStr = string(body)
	} else {
		bodyStr = fmt.Sprintf("rationale=%s\nrecommended_subbeads=%v",
			report.DecompositionRationale, report.DecompositionRecommendation)
	}
	summary := "agent declined: needs decomposition"
	if n := len(report.DecompositionRecommendation); n > 0 {
		summary = fmt.Sprintf("%s (%d sub-beads)", summary, n)
	}
	return &ParkingEvent{
		Kind:    "decomposition-recommendation",
		Summary: summary,
		Body:    bodyStr,
	}
}

func buildPushConflictParkingEvent(report Report) *ParkingEvent {
	body, mErr := json.Marshal(map[string]any{
		"detail":     report.Detail,
		"base_rev":   report.BaseRev,
		"result_rev": report.ResultRev,
		"session_id": report.SessionID,
	})
	bodyStr := report.Detail
	if mErr == nil {
		bodyStr = string(body)
	}
	return &ParkingEvent{
		Kind:    "push-conflict",
		Summary: "auto-merge after push race could not be resolved",
		Body:    bodyStr,
	}
}

type RateLimitRetryConfig struct {
	Budget     time.Duration
	PerWaitCap time.Duration
	Sleep      func(ctx context.Context, d time.Duration) error
	Now        func() time.Time
	Evaluator  func(retryAfter time.Duration, attempt int, elapsed, budget, perWaitCap time.Duration) RateLimitWaitDecision
	OnRetry    func(ctx context.Context, info RateLimitRetryInfo)
}

const (
	RateLimitRetryDefaultBudget     = ratelimitpolicy.DefaultBudget
	RateLimitRetryDefaultPerWaitCap = ratelimitpolicy.DefaultPerWaitCap
	RateLimitBudgetExhaustedReason  = ratelimitpolicy.BudgetExhaustedReason
)

type RateLimitWaitDecision = ratelimitpolicy.WaitDecision

type RateLimitRetryInfo struct {
	Attempt    int
	Wait       time.Duration
	Source     string
	Report     *Report
	Elapsed    time.Duration
	RetryAfter time.Duration
	OverBudget bool
}

func resolveRateLimitRetryConfig(opts AttemptOpts) RateLimitRetryConfig {
	cfg := RateLimitRetryConfig{
		Budget:     opts.RateLimitBudget,
		PerWaitCap: opts.RateLimitPerWaitCap,
		Evaluator:  opts.RateLimitWaitEvaluator,
		Sleep:      opts.RateLimitSleep,
		Now:        opts.RateLimitNow,
		OnRetry:    opts.RateLimitOnRetry,
	}
	if cfg.Budget == 0 {
		cfg.Budget = RateLimitRetryDefaultBudget
	}
	if cfg.PerWaitCap == 0 {
		cfg.PerWaitCap = RateLimitRetryDefaultPerWaitCap
	}
	if cfg.Sleep == nil {
		cfg.Sleep = ctxSleep
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Evaluator == nil {
		cfg.Evaluator = EvaluateRateLimitWait
	}
	return cfg
}

func ctxSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func evaluateRateLimitAttempt(report Report, cfg RateLimitRetryConfig, attempt int, elapsed, budget time.Duration) (RateLimitWaitDecision, time.Duration) {
	if cfg.Budget < 0 || budget < 0 {
		return RateLimitWaitDecision{ShouldRetry: false}, 0
	}
	if !IsRateLimitReport(&report) {
		return RateLimitWaitDecision{ShouldRetry: false}, 0
	}

	retryAfter := ratelimitpolicy.ExtractRetryAfterFromStderr(report.Stderr, cfg.Now())
	decision := cfg.Evaluator(retryAfter, attempt, elapsed, budget, cfg.PerWaitCap)
	if !decision.ShouldRetry {
		decision.Reason = RateLimitBudgetExhaustedReason
	}
	return decision, retryAfter
}

func EvaluateRateLimitWait(retryAfter time.Duration, attempt int, elapsed, budget, perWaitCap time.Duration) RateLimitWaitDecision {
	return ratelimitpolicy.EvaluateRateLimitWait(retryAfter, attempt, elapsed, budget, perWaitCap)
}

func IsRateLimitReport(report *Report) bool {
	if report == nil {
		return false
	}
	return ratelimitpolicy.IsRateLimitText(report.Error + "\n" + report.Stderr + "\n" + report.Detail)
}

func ShouldAttemptConflictRecovery(report Report, projectRoot string) bool {
	if report.PreserveRef == "" || projectRoot == "" {
		return false
	}
	switch report.Status {
	case StatusLandConflict:
		return true
	case StatusExecutionFailed:
		return report.BaseRev != "" && report.ResultRev != "" && report.ResultRev != report.BaseRev
	}
	return false
}
