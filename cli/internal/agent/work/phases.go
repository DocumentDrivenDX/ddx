package work

import (
	"context"
	"fmt"
)

// Phase is the typed work phase vocabulary.
type Phase string

const (
	PhaseQueueing  Phase = "queueing"
	PhaseResolving Phase = "resolving"
	PhaseRunning   Phase = "running"
	PhaseTerminal  Phase = "terminal"
)

// Outcome carries the state needed to render the terminal phase/result pair.
// The execute loop populates the fields it already knows; the phase helper
// forwards them to the store so tests can verify the transition contract.
type Outcome struct {
	AttemptID          string
	Status             string
	Detail             string
	SessionID          string
	ResultRev          string
	BaseRev            string
	PreserveRef        string
	NoChangesRationale string
	Disposition        string
	ParkReason         string
	DurationMS         int64
}

// PhaseStore receives the phase transition emissions.
type PhaseStore interface {
	EmitProgress(ctx context.Context, beadID string, phase Phase, outcome Outcome) error
	EmitResult(ctx context.Context, beadID string, outcome Outcome) error
}

// emitPhase centralises the phase-state transition contract so callers only
// need to name the phase. Terminal transitions emit both the terminal phase
// record and the durable bead.result record.
func emitPhase(ctx context.Context, store PhaseStore, beadID string, phase Phase, outcome Outcome) error {
	if store == nil {
		return nil
	}
	switch phase {
	case PhaseQueueing, PhaseRunning:
		return store.EmitProgress(ctx, beadID, phase, outcome)
	case PhaseTerminal:
		if err := store.EmitProgress(ctx, beadID, phase, outcome); err != nil {
			return err
		}
		return store.EmitResult(ctx, beadID, outcome)
	default:
		return fmt.Errorf("unknown phase %q", phase)
	}
}

// EmitPhase is the exported wrapper used by the agent package.
func EmitPhase(ctx context.Context, store PhaseStore, beadID string, phase Phase, outcome Outcome) error {
	return emitPhase(ctx, store, beadID, phase, outcome)
}
