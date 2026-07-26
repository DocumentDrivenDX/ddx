package runrecord

import (
	"fmt"
	"strings"
	"time"
)

// TransitionToRunning atomically advances an existing dispatching record to
// phase running, attaching only the supplied public Fizeau fields. It does not
// query provider sessions or invent process metadata.
//
// Behavior:
//   - Missing record: error (caller expected a pre-dispatch substrate).
//   - Phase already running: no-op success (idempotent).
//   - Phase terminal or interrupted: error (must not regress).
//   - Phase dispatching: publish phase=running with UpdatedAt advanced and
//     Fizeau set when public is non-nil and non-empty.
func TransitionToRunning(projectRoot, runID string, public *FizeauPublicResult) error {
	runID = strings.TrimSpace(runID)
	if strings.TrimSpace(projectRoot) == "" {
		return fmt.Errorf("runrecord: transition running: empty project root")
	}
	if runID == "" {
		return fmt.Errorf("runrecord: transition running: empty run_id")
	}

	rec, err := Read(projectRoot, runID)
	if err != nil {
		return fmt.Errorf("runrecord: transition running: read: %w", err)
	}
	if rec == nil {
		return fmt.Errorf("runrecord: transition running: record missing for %s", runID)
	}

	switch rec.Phase {
	case PhaseRunning:
		return nil
	case PhaseTerminal, PhaseInterrupted:
		return fmt.Errorf("runrecord: transition running: cannot advance phase %q to running", rec.Phase)
	case PhaseDispatching:
		// continue
	default:
		return fmt.Errorf("runrecord: transition running: unknown phase %q", rec.Phase)
	}

	now := time.Now().UTC()
	rec.Phase = PhaseRunning
	rec.UpdatedAt = now
	if public != nil && !public.IsEmpty() {
		// Replace (not invent from sessions). First public data wins for this
		// transition; terminal updates are a separate writer path.
		copied := *public
		rec.Fizeau = &copied
	}
	if err := Publish(projectRoot, *rec); err != nil {
		return fmt.Errorf("runrecord: transition running: publish: %w", err)
	}
	return nil
}

// IsEmpty reports whether no public Fizeau fields are set.
func (f *FizeauPublicResult) IsEmpty() bool {
	if f == nil {
		return true
	}
	return f.SessionLogPath == "" &&
		f.PublicSessionRef == "" &&
		f.PublicResultRef == "" &&
		f.ImmediateError == "" &&
		f.FinalStatus == "" &&
		f.FinalExitCode == nil &&
		f.DurationMS == nil &&
		f.CostUSD == nil &&
		f.InputTokens == nil &&
		f.OutputTokens == nil &&
		f.TotalTokens == nil &&
		f.CachedTokens == nil
}
