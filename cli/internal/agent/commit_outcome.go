package agent

import (
	"context"
	"errors"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

type commitOutcomeFailure struct {
	operation string
	assignee  string
	result    *ExecuteBeadLoopResult
	err       error
}

func (e *commitOutcomeFailure) Error() string {
	return e.err.Error()
}

func commitOutcomeError(operation, assignee string, result *ExecuteBeadLoopResult, err error) error {
	if err == nil {
		return nil
	}
	return &commitOutcomeFailure{
		operation: operation,
		assignee:  assignee,
		result:    result,
		err:       err,
	}
}

// commitOutcome keeps the drain loop moving after a post-outcome store failure.
// The wrapped op supplies the operation label via commitOutcomeError so this
// helper can preserve the loop-error event and cooldown behavior without
// hard-erroring the worker or surfacing the store error to Drain.
func commitOutcome(ctx context.Context, store ExecuteBeadLoopStore, beadID string, op func() error) error {
	if err := op(); err != nil {
		var failure *commitOutcomeFailure
		if errors.As(err, &failure) {
			if failure.result != nil {
				failure.result.Failures++
				failure.result.LastFailureStatus = "loop-error"
			}
			_ = store.AppendEvent(beadID, bead.BeadEvent{
				Kind:      "loop-error",
				Summary:   failure.operation + " failed",
				Body:      failure.err.Error(),
				Actor:     failure.assignee,
				Source:    "ddx work",
				CreatedAt: time.Now().UTC(),
			})
			_ = store.SetExecutionCooldown(beadID, time.Now().UTC().Add(StoreErrorCooldown), "loop-error", failure.operation+": "+failure.err.Error(), "")
			return nil
		}
		_ = store.SetExecutionCooldown(beadID, time.Now().UTC().Add(StoreErrorCooldown), "loop-error", err.Error(), "")
		return nil
	}
	return nil
}
