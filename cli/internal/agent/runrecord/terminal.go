package runrecord

import (
	"fmt"
	"strings"
	"time"
)

// TerminalInput carries typed terminal fields for an atomic phase update.
// Callers supply only public Fizeau result fields and DDx repository-evaluation
// outcome/evidence — never provider-session canonical state.
type TerminalInput struct {
	// Outcome is the typed DDx disposition (required for a meaningful terminal).
	Outcome Outcome
	// Public holds optional public Fizeau result fields (immediate error or final).
	Public *FizeauPublicResult
	// AdditionalEvidence is merged into the existing evidence list by Name
	// (same Name replaces; new Names append).
	AdditionalEvidence []EvidenceLink
}

// TransitionToTerminal atomically advances an existing dispatching or running
// record to phase terminal with typed outcome fields, FinishedAt, optional
// public Fizeau fields (including cost/tokens when present), and merged
// repository-evaluation evidence links.
//
// Behavior:
//   - Missing record: error.
//   - Phase already terminal: no-op success (idempotent).
//   - Phase interrupted: error (must not overwrite interruption recovery).
//   - Phase dispatching or running: publish phase=terminal with UpdatedAt and
//     FinishedAt advanced; merge Fizeau and evidence as specified.
func TransitionToTerminal(projectRoot, runID string, in TerminalInput) error {
	runID = strings.TrimSpace(runID)
	if strings.TrimSpace(projectRoot) == "" {
		return fmt.Errorf("runrecord: transition terminal: empty project root")
	}
	if runID == "" {
		return fmt.Errorf("runrecord: transition terminal: empty run_id")
	}

	rec, err := Read(projectRoot, runID)
	if err != nil {
		return fmt.Errorf("runrecord: transition terminal: read: %w", err)
	}
	if rec == nil {
		return fmt.Errorf("runrecord: transition terminal: record missing for %s", runID)
	}

	switch rec.Phase {
	case PhaseTerminal:
		return nil
	case PhaseInterrupted:
		return fmt.Errorf("runrecord: transition terminal: cannot advance phase %q to terminal", rec.Phase)
	case PhaseDispatching, PhaseRunning:
		// continue
	default:
		return fmt.Errorf("runrecord: transition terminal: unknown phase %q", rec.Phase)
	}

	now := time.Now().UTC()
	rec.Phase = PhaseTerminal
	rec.UpdatedAt = now
	finished := now
	rec.FinishedAt = &finished

	out := in.Outcome
	rec.Outcome = &out

	if in.Public != nil && !in.Public.IsEmpty() {
		rec.Fizeau = mergeFizeauPublic(rec.Fizeau, in.Public)
	}
	if len(in.AdditionalEvidence) > 0 {
		rec.Evidence = mergeEvidence(rec.Evidence, in.AdditionalEvidence)
	}

	if err := Publish(projectRoot, *rec); err != nil {
		return fmt.Errorf("runrecord: transition terminal: publish: %w", err)
	}
	return nil
}

// mergeFizeauPublic returns a copy of existing with non-empty fields from update
// overlaid. Nil update leaves existing unchanged (caller still may set nil).
func mergeFizeauPublic(existing, update *FizeauPublicResult) *FizeauPublicResult {
	if update == nil || update.IsEmpty() {
		if existing == nil {
			return nil
		}
		copied := *existing
		return &copied
	}
	var out FizeauPublicResult
	if existing != nil {
		out = *existing
	}
	if update.SessionLogPath != "" {
		out.SessionLogPath = update.SessionLogPath
	}
	if update.PublicSessionRef != "" {
		out.PublicSessionRef = update.PublicSessionRef
	}
	if update.PublicResultRef != "" {
		out.PublicResultRef = update.PublicResultRef
	}
	if update.ImmediateError != "" {
		out.ImmediateError = update.ImmediateError
	}
	if update.FinalStatus != "" {
		out.FinalStatus = update.FinalStatus
	}
	if update.FinalExitCode != nil {
		v := *update.FinalExitCode
		out.FinalExitCode = &v
	}
	if update.DurationMS != nil {
		v := *update.DurationMS
		out.DurationMS = &v
	}
	if update.CostUSD != nil {
		v := *update.CostUSD
		out.CostUSD = &v
	}
	if update.InputTokens != nil {
		v := *update.InputTokens
		out.InputTokens = &v
	}
	if update.OutputTokens != nil {
		v := *update.OutputTokens
		out.OutputTokens = &v
	}
	if update.TotalTokens != nil {
		v := *update.TotalTokens
		out.TotalTokens = &v
	}
	if update.CachedTokens != nil {
		v := *update.CachedTokens
		out.CachedTokens = &v
	}
	return &out
}

// mergeEvidence appends or replaces by Name. Existing order is preserved;
// replacements keep the original index; new names append in input order.
func mergeEvidence(existing, additional []EvidenceLink) []EvidenceLink {
	if len(additional) == 0 {
		return existing
	}
	out := make([]EvidenceLink, len(existing))
	copy(out, existing)
	indexByName := make(map[string]int, len(out))
	for i, e := range out {
		if e.Name != "" {
			indexByName[e.Name] = i
		}
	}
	for _, e := range additional {
		if e.Name == "" {
			out = append(out, e)
			continue
		}
		if i, ok := indexByName[e.Name]; ok {
			out[i] = e
			continue
		}
		indexByName[e.Name] = len(out)
		out = append(out, e)
	}
	return out
}
