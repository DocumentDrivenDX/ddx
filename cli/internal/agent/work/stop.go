package work

import (
	"context"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
)

// StopCondition is the typed work-drain terminal decision vocabulary. Keep
// these values stable; evidence and operator surfaces derive from them.
type StopCondition string

const (
	StopConditionDrained    StopCondition = "Drained"
	StopConditionOnce       StopCondition = "Once"
	StopConditionSignal     StopCondition = "Signal"
	StopConditionBudget     StopCondition = "Budget"
	StopConditionNoProgress StopCondition = "NoProgress"
	StopConditionBlocked    StopCondition = "Blocked"
	StopConditionGoal       StopCondition = "Goal"
)

// StopInput describes one possible terminal drain decision point.
type StopInput struct {
	ContextErr  error
	NoReadyWork bool
	Once        bool
	Mode        executeloop.Mode
	Budget      bool
	NoProgress  bool
	Blocked     bool
	// GoalSatisfied means the durable work-drive goal's stop predicate has
	// been met. It is orthogonal to the bead queue: the queue can still be
	// empty while the worker keeps waiting for the goal gate.
	GoalSatisfied bool
}

// StopDecision is the classified terminal decision plus the legacy-compatible
// loop.end exit_reason string.
type StopDecision struct {
	Condition  StopCondition
	ExitReason string
}

// ClassifyStop maps scattered drain-loop terminal checks to a single typed
// vocabulary while preserving existing exit_reason strings.
func ClassifyStop(in StopInput) (StopDecision, bool) {
	switch {
	case in.ContextErr != nil:
		return StopDecision{
			Condition:  StopConditionSignal,
			ExitReason: signalExitReason(in.ContextErr),
		}, true
	case in.Budget:
		return StopDecision{Condition: StopConditionBudget, ExitReason: "budget"}, true
	case in.NoProgress:
		return StopDecision{Condition: StopConditionNoProgress, ExitReason: "no_progress"}, true
	case in.Blocked:
		return StopDecision{Condition: StopConditionBlocked, ExitReason: "blocked"}, true
	case in.GoalSatisfied:
		return StopDecision{Condition: StopConditionGoal, ExitReason: "goal_satisfied"}, true
	case in.Once:
		return StopDecision{Condition: StopConditionOnce, ExitReason: "once_complete"}, true
	case in.NoReadyWork && in.Mode != executeloop.ModeWatch:
		return StopDecision{Condition: StopConditionDrained, ExitReason: "drained"}, true
	default:
		return StopDecision{}, false
	}
}

func signalExitReason(err error) string {
	switch err {
	case context.Canceled:
		return "sigint"
	case context.DeadlineExceeded:
		return "sigterm"
	default:
		return "context_cancelled"
	}
}
