// Package try defines the sealed Outcome sum type that the execute-bead
// orchestrator will return once the C-series refactor lands. The types in
// this file are introduced ahead of any caller (see ddx-240f2082) so that
// later beads can migrate the orchestrator to produce Outcome values
// without simultaneously inventing the vocabulary.
package try

import (
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// Disposition is the closed set of post-attempt routing decisions the
// execute-loop can take on a bead. Every Outcome carries exactly one.
type Disposition int

const (
	DispositionMerged Disposition = iota
	DispositionAlreadyDone
	DispositionRetry
	DispositionPark
	DispositionNeedsHuman
	DispositionLoopError
)

func (d Disposition) String() string {
	switch d {
	case DispositionMerged:
		return "merged"
	case DispositionAlreadyDone:
		return "already_done"
	case DispositionRetry:
		return "retry"
	case DispositionPark:
		return "park"
	case DispositionNeedsHuman:
		return "needs_human"
	case DispositionLoopError:
		return "loop_error"
	default:
		return fmt.Sprintf("disposition(%d)", int(d))
	}
}

// ParkReason enumerates the closed vocabulary of reasons the loop may park
// a bead. String-typed for ergonomic comparison, but every value used in
// production must come from the constants below; ParkReasonValid enforces
// that at the boundary.
type ParkReason string

const (
	ParkReasonNeedsReview              ParkReason = "needs_review"
	ParkReasonDecomposition            ParkReason = "decomposition"
	ParkReasonPushFailed               ParkReason = "push_failed"
	ParkReasonPushConflict             ParkReason = "push_conflict"
	ParkReasonCostCap                  ParkReason = "cost_cap"
	ParkReasonLoopError                ParkReason = "loop_error"
	ParkReasonNoChangesUnverified      ParkReason = "no_changes_unverified"
	ParkReasonNoChangesUnjustified     ParkReason = "no_changes_unjustified"
	ParkReasonRateLimitBudgetExhausted ParkReason = "rate_limit_budget_exhausted"
	ParkReasonQuotaPaused              ParkReason = "quota_paused"
	ParkReasonLockContention           ParkReason = "lock_contention"
)

// AllParkReasons returns the canonical list of ParkReason values in a
// stable order. Used by tests and by callers that need to enumerate the
// vocabulary (e.g. for documentation generators).
func AllParkReasons() []ParkReason {
	return []ParkReason{
		ParkReasonNeedsReview,
		ParkReasonDecomposition,
		ParkReasonPushFailed,
		ParkReasonPushConflict,
		ParkReasonCostCap,
		ParkReasonLoopError,
		ParkReasonNoChangesUnverified,
		ParkReasonNoChangesUnjustified,
		ParkReasonRateLimitBudgetExhausted,
		ParkReasonQuotaPaused,
		ParkReasonLockContention,
	}
}

// ParkReasonValid reports whether r is a member of the closed ParkReason
// vocabulary. The empty string is invalid; use a zero ParkReason only on
// non-Park dispositions.
func ParkReasonValid(r ParkReason) bool {
	for _, p := range AllParkReasons() {
		if p == r {
			return true
		}
	}
	return false
}

func (p ParkReason) String() string { return string(p) }

// Outcome is the sealed sum type the execute-loop orchestrator will return
// for each bead attempt. It carries the routing decision (Disposition)
// along with the small set of facts the loop needs to apply that decision:
// timing, cost, the resulting commit (if any), and the bead-tracker events
// to record.
type Outcome struct {
	BeadID       string
	AttemptID    string
	BaseRev      string
	ResultRev    string
	SessionID    string
	DurationMS   int64
	CostUSD      float64
	Disposition  Disposition
	Cooldown     time.Duration
	ParkReason   ParkReason
	RecordEvents []bead.BeadEvent
}

// dispositionToStatus maps a (Disposition, ParkReason) pair onto the
// canonical execute-bead status string. The mapping is deterministic and
// invertible for the canonical reports produced by FromOutcome.
func dispositionToStatus(d Disposition, p ParkReason) string {
	switch d {
	case DispositionMerged:
		return agent.ExecuteBeadStatusSuccess
	case DispositionAlreadyDone:
		return agent.ExecuteBeadStatusAlreadySatisfied
	case DispositionRetry:
		return agent.ExecuteBeadStatusNoChanges
	case DispositionNeedsHuman:
		return agent.ExecuteBeadStatusLandConflictNeedsHuman
	case DispositionLoopError:
		return agent.ExecuteBeadStatusExecutionFailed
	case DispositionPark:
		switch p {
		case ParkReasonNeedsReview:
			return agent.ExecuteBeadStatusPreservedNeedsReview
		case ParkReasonDecomposition:
			return agent.ExecuteBeadStatusDeclinedNeedsDecomposition
		case ParkReasonPushFailed:
			return agent.ExecuteBeadStatusPushFailed
		case ParkReasonPushConflict:
			return agent.ExecuteBeadStatusPushConflict
		case ParkReasonNoChangesUnverified, ParkReasonNoChangesUnjustified:
			return agent.ExecuteBeadStatusNoChanges
		default:
			// Park reasons without a dedicated status share execution_failed
			// and rely on Detail (= ParkReason) to disambiguate on inverse.
			return agent.ExecuteBeadStatusExecutionFailed
		}
	}
	return ""
}

// FromOutcome produces the canonical ExecuteBeadReport that ToOutcome
// inverts. RecordEvents are not carried (ExecuteBeadReport has no events
// field); callers that need events handle them out-of-band.
func FromOutcome(o Outcome) agent.ExecuteBeadReport {
	rep := agent.ExecuteBeadReport{
		BeadID:     o.BeadID,
		AttemptID:  o.AttemptID,
		Status:     dispositionToStatus(o.Disposition, o.ParkReason),
		SessionID:  o.SessionID,
		BaseRev:    o.BaseRev,
		ResultRev:  o.ResultRev,
		CostUSD:    o.CostUSD,
		DurationMS: o.DurationMS,
	}
	if o.Cooldown > 0 {
		rep.RetryAfter = o.Cooldown.String()
	}
	if o.Disposition == DispositionPark && o.ParkReason != "" {
		rep.Detail = string(o.ParkReason)
	}
	return rep
}

// ToOutcome decodes an ExecuteBeadReport back into an Outcome. It is the
// inverse of FromOutcome on canonical reports; reports produced by other
// code paths may carry fields Outcome does not model, which are dropped.
func ToOutcome(r agent.ExecuteBeadReport) Outcome {
	o := Outcome{
		BeadID:     r.BeadID,
		AttemptID:  r.AttemptID,
		BaseRev:    r.BaseRev,
		ResultRev:  r.ResultRev,
		SessionID:  r.SessionID,
		DurationMS: r.DurationMS,
		CostUSD:    r.CostUSD,
	}
	if r.RetryAfter != "" {
		if d, err := time.ParseDuration(r.RetryAfter); err == nil {
			o.Cooldown = d
		}
	}
	switch r.Status {
	case agent.ExecuteBeadStatusSuccess:
		o.Disposition = DispositionMerged
	case agent.ExecuteBeadStatusAlreadySatisfied:
		o.Disposition = DispositionAlreadyDone
	case agent.ExecuteBeadStatusNoChanges:
		// Detail carrying a valid ParkReason promotes to Park; otherwise Retry.
		if pr := ParkReason(r.Detail); ParkReasonValid(pr) &&
			(pr == ParkReasonNoChangesUnverified || pr == ParkReasonNoChangesUnjustified) {
			o.Disposition = DispositionPark
			o.ParkReason = pr
		} else {
			o.Disposition = DispositionRetry
		}
	case agent.ExecuteBeadStatusPreservedNeedsReview:
		o.Disposition = DispositionPark
		o.ParkReason = ParkReasonNeedsReview
	case agent.ExecuteBeadStatusDeclinedNeedsDecomposition:
		o.Disposition = DispositionPark
		o.ParkReason = ParkReasonDecomposition
	case agent.ExecuteBeadStatusPushFailed:
		o.Disposition = DispositionPark
		o.ParkReason = ParkReasonPushFailed
	case agent.ExecuteBeadStatusPushConflict:
		o.Disposition = DispositionPark
		o.ParkReason = ParkReasonPushConflict
	case agent.ExecuteBeadStatusLandConflictNeedsHuman:
		o.Disposition = DispositionNeedsHuman
	case agent.ExecuteBeadStatusExecutionFailed:
		// Detail carrying a valid ParkReason disambiguates Park from LoopError.
		if pr := ParkReason(r.Detail); ParkReasonValid(pr) {
			o.Disposition = DispositionPark
			o.ParkReason = pr
		} else {
			o.Disposition = DispositionLoopError
		}
	}
	return o
}
