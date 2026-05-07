package cmd

import (
	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// reviewRepairAttemptArgs bundles the arguments that must be forwarded to
// singleTierAttempt for a review_fixable_gap repair cycle. MinPower is raised
// above the implementer's actual power; all other routing pins are forwarded
// unchanged from the original attempt.
type reviewRepairAttemptArgs struct {
	MinPower int
	Harness  string
	Model    string
	Provider string
	Profile  string
	// MaxPower is the operator's explicit cap, preserved as-is (0 = unset).
	MaxPower int
}

// reviewFixableGapRepairMinPower returns the MinPower floor for a repair
// attempt that follows a review_fixable_gap verdict. It advances the
// escalation ladder from the implementer's actual power to the next higher
// floor so the repair runs on a stronger model than the original implementer.
//
// If the ladder has no higher floor (exhausted), it returns implActualPower
// so the repair attempt at least carries the review rationale as additional
// context even if the same power tier is used.
func reviewFixableGapRepairMinPower(repairCtx *agent.RepairContextFromReviewGroup, ladder escalationFloorFinder) int {
	if repairCtx == nil {
		return 0
	}
	next, err := nextEscalationFloor(ladder, repairCtx.ImplementerActualPower)
	if err != nil {
		return repairCtx.ImplementerActualPower
	}
	return next
}

// reviewFixableGapRepairArgs computes the singleTierAttempt arguments for a
// repair attempt. MinPower is raised above the implementer's actual power
// using the escalation ladder. All explicit harness/model/provider/profile
// pins are forwarded unchanged. MaxPower is preserved from the operator's
// original setting (0 means unset — the repair must not add a cap the
// operator did not request).
func reviewFixableGapRepairArgs(
	repairCtx *agent.RepairContextFromReviewGroup,
	harness, model, provider, profile string,
	maxPower int,
	ladder escalationFloorFinder,
) reviewRepairAttemptArgs {
	return reviewRepairAttemptArgs{
		MinPower: reviewFixableGapRepairMinPower(repairCtx, ladder),
		Harness:  harness,
		Model:    model,
		Provider: provider,
		Profile:  profile,
		MaxPower: maxPower,
	}
}
