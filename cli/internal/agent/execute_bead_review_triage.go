package agent

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/triage"
)

// TriageTierHintKey is the bead Extra key that records the triage policy's
// tier-pin hint for the next attempt. Populated when the BLOCK ladder reaches
// the escalate_tier rung; consumed by tier-aware executors to start above the
// default tier on the next drain pass.
const TriageTierHintKey = "triage.tier_hint"

// TriageNeedsHumanLabel is appended to a bead when the BLOCK ladder reaches
// its terminal rung. The execute-loop label-filter and the human triage UI
// both use this label to surface beads that no longer benefit from automated
// re-attempts.
const TriageNeedsHumanLabel = "needs_human"

// triageDecisionEventKind is the event kind used to record the triage
// policy's chosen action after a non-APPROVE review verdict.
const triageDecisionEventKind = "triage-decision"

// applyReviewTriageDecision runs the triage policy after a BLOCK or
// REQUEST_CHANGES review verdict has been recorded for `beadID`. It reads
// prior review events to compute the BLOCK count, biases the result toward
// re-attempt when the latest BLOCK was paired with a degraded reviewer
// (kind:review-pairing-degraded), then applies the chosen action by writing
// metadata, labels, and a kind:triage-decision event.
//
// Errors from the underlying store are best-effort: the loop continues
// regardless so a triage-side failure cannot strand a reopened bead.
func (w *ExecuteBeadWorker) applyReviewTriageDecision(beadID, actor string, now time.Time, currentTier string) error {
	events, err := w.Store.Events(beadID)
	if err != nil {
		return err
	}

	var blockTimestamps []time.Time
	var pairingDegraded []time.Time
	for _, ev := range events {
		switch {
		case ev.Kind == "review" && strings.TrimSpace(ev.Summary) == "BLOCK":
			blockTimestamps = append(blockTimestamps, ev.CreatedAt)
		case ev.Kind == ReviewPairingDegradedEventKind:
			pairingDegraded = append(pairingDegraded, ev.CreatedAt)
		}
	}

	// History excludes the current attempt. The just-appended BLOCK (if any)
	// is the last entry in blockTimestamps and must not be counted as prior
	// history per triage.Decide()'s contract.
	historyCount := len(blockTimestamps)
	if historyCount > 0 {
		historyCount--
	}
	history := make([]triage.FailureMode, 0, historyCount)
	for i := 0; i < historyCount; i++ {
		history = append(history, triage.FailureModeReviewBlock)
	}

	policy := triage.DefaultPolicy()
	action := policy.Decide(beadID, triage.FailureModeReviewBlock, history)

	pairedDegraded := latestBlockPairedDegraded(blockTimestamps, pairingDegraded)
	if pairedDegraded && action == triage.ActionEscalateTier {
		action = triage.ActionReAttemptWithContext
	}

	return w.applyTriageAction(beadID, actor, now, action, currentTier, pairedDegraded)
}

// latestBlockPairedDegraded reports whether the most recent BLOCK event was
// paired with a review-pairing-degraded event from the same attempt window
// (between the previous BLOCK and this one).
func latestBlockPairedDegraded(blocks, pairing []time.Time) bool {
	if len(blocks) == 0 || len(pairing) == 0 {
		return false
	}
	latest := blocks[len(blocks)-1]
	var prev time.Time
	if len(blocks) >= 2 {
		prev = blocks[len(blocks)-2]
	}
	for _, t := range pairing {
		if t.After(prev) && !t.After(latest) {
			return true
		}
	}
	return false
}

// applyTriageAction performs the side effects for a chosen Action: writes a
// tier-pin hint into bead.Extra (escalate_tier), appends a needs_human label
// (needs_human), and always records a kind:triage-decision event.
func (w *ExecuteBeadWorker) applyTriageAction(beadID, actor string, now time.Time, action triage.Action, currentTier string, pairedDegraded bool) error {
	body := map[string]any{
		"action": string(action),
		"mode":   string(triage.FailureModeReviewBlock),
	}
	if pairedDegraded {
		body["pairing_degraded"] = true
	}

	switch action {
	case triage.ActionEscalateTier:
		nextTier := nextEscalatedTier(currentTier)
		body["tier_hint"] = string(nextTier)
		_ = w.Store.Update(beadID, func(b *bead.Bead) {
			if b.Extra == nil {
				b.Extra = make(map[string]any)
			}
			b.Extra[TriageTierHintKey] = string(nextTier)
		})
	case triage.ActionNeedsHuman:
		_ = w.Store.Update(beadID, func(b *bead.Bead) {
			if !HasBeadLabel(b.Labels, TriageNeedsHumanLabel) {
				b.Labels = append(b.Labels, TriageNeedsHumanLabel)
			}
		})
	}

	bodyJSON, _ := json.Marshal(body)
	return w.Store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      triageDecisionEventKind,
		Summary:   string(triage.FailureModeReviewBlock) + ": " + string(action),
		Body:      string(bodyJSON),
		Actor:     actor,
		Source:    "ddx agent execute-loop",
		CreatedAt: now,
	})
}

// nextEscalatedTier returns the next tier above `current`. An unrecognised or
// empty current tier defaults to standard; smart is its own ceiling.
func nextEscalatedTier(current string) escalation.ModelTier {
	switch escalation.ModelTier(current) {
	case escalation.TierCheap:
		return escalation.TierStandard
	case escalation.TierStandard:
		return escalation.TierSmart
	case escalation.TierSmart:
		return escalation.TierSmart
	default:
		return escalation.TierStandard
	}
}
