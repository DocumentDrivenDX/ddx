package agent

import (
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// ParkToProposedOpts contains optional configuration for parking a bead to proposed status.
type ParkToProposedOpts struct {
	Reason           string
	Summary          string
	SuggestedAction  string
	Since            time.Time
	CleanupLabels    bool
	AdditionalMutate func(*bead.Bead)
}

// parkToProposedWithOperatorMeta parks a bead to proposed status with common operator-required metadata.
// It handles label cleanup and NeedsHumanMeta setup consistently across the agent.
func parkToProposedWithOperatorMeta(store ExecuteBeadLoopStore, beadID string, parkReason bead.ParkReason, opts ParkToProposedOpts) error {
	if opts.Since.IsZero() {
		opts.Since = time.Now()
	}
	return store.ParkToProposed(beadID, parkReason, func(b *bead.Bead) {
		// Defensive removal for legacy rows that escaped the lifecycle migration or arrived via external import.
		if opts.CleanupLabels {
			b.Labels = removeBeadLabels(b.Labels, TriageNeedsHumanLabel, bead.LabelNeedsHuman, bead.LabelNeedsInvestigation)
			clearReviewTriageClaimMetadata(b)
		}
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason:          opts.Reason,
			Since:           opts.Since.UTC().Format(time.RFC3339),
			Source:          "ddx work",
			SuggestedAction: opts.SuggestedAction,
			Summary:         opts.Summary,
		})
		if opts.AdditionalMutate != nil {
			opts.AdditionalMutate(b)
		}
	})
}

// parkToProposedWithIntakeMeta parks a bead from pre-claim intake rejection with label cleanup and metadata.
func parkToProposedWithIntakeMeta(store ExecuteBeadLoopStore, beadID string, parkReason bead.ParkReason, opts ParkToProposedOpts) error {
	if opts.Since.IsZero() {
		opts.Since = time.Now()
	}
	return store.ParkToProposed(beadID, parkReason, func(b *bead.Bead) {
		ensureBeadExtra(b)
		// Migration-only cleanup: defensive removal for legacy rows that escaped
		// the lifecycle migration or arrived via external import.
		b.Labels = removeBeadLabels(b.Labels, bead.LabelNeedsHuman, bead.LabelNeedsInvestigation)
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason:          opts.Reason,
			Since:           opts.Since.UTC().Format(time.RFC3339),
			Source:          "ddx work",
			SuggestedAction: opts.SuggestedAction,
			Summary:         opts.Summary,
		})
		if opts.AdditionalMutate != nil {
			opts.AdditionalMutate(b)
		}
	})
}

// parkToProposedSimple parks a bead with minimal metadata (just NeedsHumanMeta, no label cleanup).
func parkToProposedSimple(store ExecuteBeadLoopStore, beadID string, parkReason bead.ParkReason, reason string, at time.Time) error {
	return store.ParkToProposed(beadID, parkReason, func(b *bead.Bead) {
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason: reason,
			Since:  at.UTC().Format(time.RFC3339),
			Source: "ddx work",
		})
	})
}
