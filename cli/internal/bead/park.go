package bead

import "fmt"

// ParkReason identifies the canonical reason for transitioning a bead to
// proposed status for operator attention. Each constant maps to a fixed
// (Reason, Source) pair used by ParkToProposed.
type ParkReason string

const (
	ParkIntakeRejection              ParkReason = "intake_rejection"
	ParkNoChangesOperatorRequired    ParkReason = "no_changes_operator_required"
	ParkPostReviewMalfunction        ParkReason = "post_review_malfunction"
	ParkReviewTerminal               ParkReason = "review_terminal"
	ParkConflictRecovery             ParkReason = "conflict_recovery"
	ParkReviewRequestClarification   ParkReason = "review_request_clarification"
	ParkLadderExhaustionManual       ParkReason = "ladder_exhaustion_manual"
	ParkAutoRecoveryFailed           ParkReason = "auto_recovery_failed"
	ParkProviderConnectivityRepeated ParkReason = "provider_connectivity_repeated"
)

type parkReasonMeta struct {
	Reason string
	Source string
}

var parkReasonMetaMap = map[ParkReason]parkReasonMeta{
	ParkIntakeRejection:              {Reason: "pre-claim intake blocked execution", Source: "legacy agent execute-loop"},
	ParkNoChangesOperatorRequired:    {Reason: "operator decision required before another automated attempt", Source: "legacy agent execute-loop"},
	ParkPostReviewMalfunction:        {Reason: "review BLOCK triage reached operator-required rung", Source: "legacy agent execute-loop"},
	ParkReviewTerminal:               {Reason: "terminal review block requires operator decision", Source: "legacy agent execute-loop"},
	ParkConflictRecovery:             {Reason: "land conflict requires operator judgment", Source: "legacy agent execute-loop"},
	ParkReviewRequestClarification:   {Reason: "reviewer cannot adjudicate needs-judgment AC without operator input", Source: "legacy agent execute-loop"},
	ParkLadderExhaustionManual:       {Reason: "recovery:manual label set; operator review required", Source: "legacy agent execute-loop"},
	ParkAutoRecoveryFailed:           {Reason: "automated recovery failed; operator review required", Source: "legacy agent execute-loop"},
	ParkProviderConnectivityRepeated: {Reason: "provider unreachable on repeated attempts; operator review required", Source: "legacy agent execute-loop"},
}

// ParkToProposed transitions the bead to proposed status for operator
// attention. It enforces OperatorRequired=true and the canonical Reason and
// Source from the ParkReason mapping. The mutate callback runs after the
// status transition; pass nil if no additional metadata is needed.
func (s *Store) ParkToProposed(id string, reason ParkReason, mutate func(*Bead)) error {
	meta, ok := parkReasonMetaMap[reason]
	if !ok {
		return fmt.Errorf("bead: unknown ParkReason %q", reason)
	}
	return s.TransitionLifecycle(id, StatusProposed, LifecycleTransitionOptions{
		OperatorRequired: true,
		Reason:           meta.Reason,
		Source:           meta.Source,
	}, func(b *Bead) error {
		if mutate != nil {
			mutate(b)
		}
		return nil
	})
}
