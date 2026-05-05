package try

import (
	"context"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

const (
	StatusExecutionFailed  = "execution_failed"
	StatusLandConflict     = "land_conflict"
	StatusSuccess          = "success"
	StatusNoChanges        = "no_changes"
	StatusAlreadySatisfied = "already_satisfied"
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
	PreserveRef                 string
	RetryAfter                  string
	NoChangesRationale          string
	ReviewVerdict               string
	ReviewRationale             string
	Tier                        string
	ProbeResult                 string
	CostUSD                     float64
	DurationMS                  int64
	RequestedProfile            string
	RequestedTier               string
	ResolvedTier                string
	EscalationCount             int
	FinalTier                   string
	DecompositionRecommendation []string
	DecompositionRationale      string
	Disrupted                   bool
	DisruptionReason            string
	OutcomeReason               string
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
	SetExecutionCooldown(beadID string, until time.Time, status, detail string) error
	IncrNoChangesCount(beadID string) (int, error)
}

type ConflictAutoRecoverFn func(wd, preserveRef string) (string, error)

type ConflictResolverFn func(ctx context.Context, beadID, preserveRef, projectRoot string) (newTip string, isBlocking bool, err error)

type AttemptOpts struct {
	Bead                bead.Bead
	Executor            Executor
	Store               Store
	ProjectRoot         string
	SatisfactionChecker SatisfactionChecker
	VerificationRunner  VerificationCommandRunner
	AutoRecover         ConflictAutoRecoverFn
	ConflictResolver    ConflictResolverFn
	Assignee            string
	Now                 func() time.Time
	Cooldown            time.Duration
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
	StoreErrOp  string
	StoreErr    error
}

func Attempt(ctx context.Context, store Store, beadID string, opts AttemptOpts) (Outcome, error) {
	if opts.Executor == nil {
		return Outcome{}, fmt.Errorf("try attempt: executor is required")
	}
	report, err := opts.Executor.Execute(ctx, beadID)
	if err != nil {
		report = Report{
			BeadID: beadID,
			Status: StatusExecutionFailed,
			Detail: err.Error(),
		}
	}
	if report.BeadID == "" {
		report.BeadID = beadID
	}
	if report.Status == "" {
		report.Status = StatusExecutionFailed
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

	return Outcome{Report: report, Disposition: OutcomeReported}, err
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
