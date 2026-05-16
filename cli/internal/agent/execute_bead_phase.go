package agent

import (
	"context"
	"time"

	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
)

type loopPhaseEmitter struct {
	progressCh chan<- ProgressEvent
	emit       func(string, map[string]any)
	workerID   string
	projectID  string
	harness    string
	model      string
	profile    string
	runStart   time.Time
	now        func() time.Time
	phaseSeq   *int
}

func newLoopPhaseEmitter(runtime ExecuteBeadLoopRuntime, harness, model, profile string, runStart time.Time, now func() time.Time, phaseSeq *int, emit func(string, map[string]any)) *loopPhaseEmitter {
	if now == nil {
		now = time.Now
	}
	return &loopPhaseEmitter{
		progressCh: runtime.ProgressCh,
		emit:       emit,
		workerID:   runtime.WorkerID,
		projectID:  runtime.ProjectRoot,
		harness:    harness,
		model:      model,
		profile:    profile,
		runStart:   runStart,
		now:        now,
		phaseSeq:   phaseSeq,
	}
}

func (e *loopPhaseEmitter) EmitProgress(_ context.Context, beadID string, phase work.Phase, outcome work.Outcome) error {
	if e == nil {
		return nil
	}
	phaseSeq := e.nextPhaseSeq()
	progressPhase := string(phase)
	if phase == work.PhaseTerminal {
		progressPhase = terminalProgressPhase(outcome)
	}
	evt := newProgressEvent(
		e.workerID,
		e.projectID,
		beadID,
		outcome.AttemptID,
		e.harness,
		e.model,
		e.profile,
		progressPhase,
		phaseSeq,
		false,
		e.now().Sub(e.runStart).Milliseconds(),
	)
	evt.TS = e.now().UTC()
	evt.Message = outcome.Detail
	emitProgress(e.progressCh, evt)
	return nil
}

func (e *loopPhaseEmitter) EmitResult(_ context.Context, beadID string, outcome work.Outcome) error {
	if e == nil || e.emit == nil {
		return nil
	}
	durationMS := outcome.DurationMS
	if durationMS == 0 {
		durationMS = e.now().Sub(e.runStart).Milliseconds()
	}
	e.emit("bead.result", map[string]any{
		"bead_id":              beadID,
		"status":               outcome.Status,
		"detail":               outcome.Detail,
		"session_id":           outcome.SessionID,
		"result_rev":           outcome.ResultRev,
		"base_rev":             outcome.BaseRev,
		"preserve_ref":         outcome.PreserveRef,
		"no_changes_rationale": outcome.NoChangesRationale,
		"duration_ms":          durationMS,
	})
	return nil
}

func (e *loopPhaseEmitter) nextPhaseSeq() int {
	if e == nil || e.phaseSeq == nil {
		return 0
	}
	*e.phaseSeq = *e.phaseSeq + 1
	return *e.phaseSeq
}

func terminalProgressPhase(outcome work.Outcome) string {
	switch outcome.Status {
	case ExecuteBeadStatusSuccess, ExecuteBeadStatusAlreadySatisfied:
		return "done"
	default:
		if outcome.PreserveRef != "" {
			return "preserved"
		}
		return "failed"
	}
}

func phaseOutcomeFromReport(report ExecuteBeadReport, attemptID string, durationMS int64) work.Outcome {
	return work.Outcome{
		AttemptID:          attemptID,
		Status:             report.Status,
		Detail:             report.Detail,
		SessionID:          report.SessionID,
		ResultRev:          report.ResultRev,
		BaseRev:            report.BaseRev,
		PreserveRef:        report.PreserveRef,
		NoChangesRationale: report.NoChangesRationale,
		DurationMS:         durationMS,
	}
}

func phaseOutcomeFromAttemptOut(report ExecuteBeadReport, attemptOut agenttry.Outcome, attemptID string, durationMS int64) work.Outcome {
	outcome := phaseOutcomeFromReport(report, attemptID, durationMS)
	outcome.Disposition = string(attemptOut.Disposition)
	if attemptOut.Parking != nil && attemptOut.Parking.Event != nil {
		outcome.ParkReason = attemptOut.Parking.Event.Kind
	}
	return outcome
}
