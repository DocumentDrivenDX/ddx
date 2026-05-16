package bead

import (
	"fmt"
	"strings"
)

// LifecycleTransitionOptions contains the explicit facts required to validate
// status changes whose meaning would otherwise be implicit in caller policy.
type LifecycleTransitionOptions struct {
	OperatorRequired      bool
	ExternalBlockerReason string
	ManualClose           bool
	ManualReopen          bool
	Reason                string
	Actor                 string
	Source                string
}

// LifecycleDependencyState is the small dependency shape the pure lifecycle
// core needs. Store callers can adapt richer dependency records into this type.
type LifecycleDependencyState struct {
	Status string
}

// LifecycleQueueFacts contains all queue-classification inputs as caller-owned
// facts. It deliberately avoids store access, filesystem access, configuration,
// process execution, git access, and clock reads.
type LifecycleQueueFacts struct {
	Status                  string
	Dependencies            []LifecycleDependencyState
	ClaimFresh              bool
	RetryCooldownActive     bool
	ExecutionEligible       bool
	ExecutionEligibleKnown  bool
	SupersededBy            string
	EpicContainer           bool
	ExternalBlockerReason   string
	LegacyLabels            []string
	LegacyMigrationWarnings []string
}

// LifecycleQueueBucket is the derived queue state used by dispatch, review,
// and operator-facing views. It is not persisted as bead.Status.
type LifecycleQueueBucket string

const (
	LifecycleBucketReady               LifecycleQueueBucket = "ready"
	LifecycleBucketWaitingDependencies LifecycleQueueBucket = "waiting_dependencies"
	LifecycleBucketProposed            LifecycleQueueBucket = "proposed"
	LifecycleBucketClaimed             LifecycleQueueBucket = "claimed"
	LifecycleBucketBlockedExternal     LifecycleQueueBucket = "blocked_external"
	LifecycleBucketClosedTerminal      LifecycleQueueBucket = "closed_terminal"
	LifecycleBucketCancelledTerminal   LifecycleQueueBucket = "cancelled_terminal"
	LifecycleBucketRetryCooldown       LifecycleQueueBucket = "retry_cooldown"
	LifecycleBucketNotEligible         LifecycleQueueBucket = "not_eligible"
	LifecycleBucketSuperseded          LifecycleQueueBucket = "superseded"
	LifecycleBucketEpicContainer       LifecycleQueueBucket = "epic_container"
	LifecycleBucketInvalid             LifecycleQueueBucket = "invalid"
)

// LifecycleIssueCode identifies validation findings from pure lifecycle
// evaluation. Issues explain bad facts; they do not mutate status or labels.
type LifecycleIssueCode string

const (
	LifecycleIssueInvalidStatus                LifecycleIssueCode = "invalid_status"
	LifecycleIssueBlockedMissingExternalReason LifecycleIssueCode = "blocked_missing_external_reason"
	LifecycleIssueTransitionRequiresOperator   LifecycleIssueCode = "transition_requires_operator"
	LifecycleIssueTransitionRequiresExternal   LifecycleIssueCode = "transition_requires_external_blocker"
	LifecycleIssueTransitionFromTerminal       LifecycleIssueCode = "transition_from_terminal"
	LifecycleIssueTransitionUnsupported        LifecycleIssueCode = "transition_unsupported"
	LifecycleIssueLegacyLabelLifecycleMetadata LifecycleIssueCode = "legacy_label_lifecycle_metadata"
	LifecycleIssueLegacyMigrationCompatibility LifecycleIssueCode = "legacy_migration_compatibility"
)

// LifecycleIssue is a deterministic, caller-displayable finding.
type LifecycleIssue struct {
	Code   LifecycleIssueCode
	Detail string
}

// LifecycleQueueDecision is the pure result of evaluating LifecycleQueueFacts.
type LifecycleQueueDecision struct {
	Bucket                LifecycleQueueBucket
	WorkerEligible        bool
	OperatorAttention     bool
	DependenciesSatisfied bool
	SatisfiesDependents   bool
	Issues                []LifecycleIssue
	Warnings              []string
	ExternalBlockerReason string
}

// ValidateLifecycleTransition returns nil when from -> to is a valid persisted
// status transition under the supplied explicit facts.
func ValidateLifecycleTransition(from, to string, opts LifecycleTransitionOptions) error {
	if err := validateLifecycleStatus(from); err != nil {
		return err
	}
	if err := validateLifecycleStatus(to); err != nil {
		return err
	}
	if from == to {
		return nil
	}
	if to == StatusClosed && opts.ManualClose {
		switch from {
		case StatusOpen, StatusInProgress, StatusBlocked, StatusProposed:
			return nil
		}
	}
	if from == StatusClosed && to == StatusOpen && opts.ManualReopen {
		return nil
	}
	if from == StatusClosed || from == StatusCancelled {
		return lifecycleTransitionError(LifecycleIssueTransitionFromTerminal, from, to)
	}
	if to == StatusProposed && !opts.OperatorRequired {
		return lifecycleTransitionError(LifecycleIssueTransitionRequiresOperator, from, to)
	}
	if to == StatusBlocked && strings.TrimSpace(opts.ExternalBlockerReason) == "" {
		return lifecycleTransitionError(LifecycleIssueTransitionRequiresExternal, from, to)
	}
	if lifecycleTransitionAllowed(from, to) {
		return nil
	}
	return lifecycleTransitionError(LifecycleIssueTransitionUnsupported, from, to)
}

// LifecycleStatusSatisfiesDependency reports whether a persisted status
// satisfies dependent beads. Only closed satisfies dependents.
func LifecycleStatusSatisfiesDependency(status string) bool {
	return status == StatusClosed
}

// LifecycleDependenciesSatisfied reports whether every dependency is closed.
func LifecycleDependenciesSatisfied(deps []LifecycleDependencyState) bool {
	for _, dep := range deps {
		if !LifecycleStatusSatisfiesDependency(dep.Status) {
			return false
		}
	}
	return true
}

// EvaluateLifecycleQueue derives queue state from caller-provided facts. Legacy
// lifecycle labels are reported as warnings only; they never choose the bucket.
func EvaluateLifecycleQueue(f LifecycleQueueFacts) LifecycleQueueDecision {
	decision := LifecycleQueueDecision{
		DependenciesSatisfied: LifecycleDependenciesSatisfied(f.Dependencies),
		SatisfiesDependents:   LifecycleStatusSatisfiesDependency(f.Status),
	}
	decision.Warnings = append(decision.Warnings, lifecycleLegacyLabelWarnings(f.LegacyLabels)...)
	for _, warning := range f.LegacyMigrationWarnings {
		trimmed := strings.TrimSpace(warning)
		if trimmed == "" {
			continue
		}
		decision.Issues = append(decision.Issues, LifecycleIssue{
			Code:   LifecycleIssueLegacyMigrationCompatibility,
			Detail: trimmed,
		})
		decision.Warnings = append(decision.Warnings, trimmed)
	}

	if err := validateLifecycleStatus(f.Status); err != nil {
		decision.Bucket = LifecycleBucketInvalid
		decision.Issues = append(decision.Issues, LifecycleIssue{
			Code:   LifecycleIssueInvalidStatus,
			Detail: err.Error(),
		})
		return decision
	}

	switch f.Status {
	case StatusClosed:
		decision.Bucket = LifecycleBucketClosedTerminal
	case StatusCancelled:
		decision.Bucket = LifecycleBucketCancelledTerminal
	case StatusProposed:
		decision.Bucket = LifecycleBucketProposed
		decision.OperatorAttention = true
	case StatusInProgress:
		if f.ClaimFresh {
			decision.Bucket = LifecycleBucketClaimed
			return decision
		}
		decision = evaluateOpenLifecycleQueue(f, decision)
	case StatusBlocked:
		decision.Bucket = LifecycleBucketBlockedExternal
		decision.ExternalBlockerReason = strings.TrimSpace(f.ExternalBlockerReason)
		if decision.ExternalBlockerReason == "" {
			decision.Issues = append(decision.Issues, LifecycleIssue{
				Code:   LifecycleIssueBlockedMissingExternalReason,
				Detail: "blocked status requires an explicit external blocker reason",
			})
		}
	case StatusOpen:
		if f.ClaimFresh {
			decision.Bucket = LifecycleBucketClaimed
			return decision
		}
		decision = evaluateOpenLifecycleQueue(f, decision)
	}
	return decision
}

func evaluateOpenLifecycleQueue(f LifecycleQueueFacts, decision LifecycleQueueDecision) LifecycleQueueDecision {
	if !decision.DependenciesSatisfied {
		decision.Bucket = LifecycleBucketWaitingDependencies
		return decision
	}
	if strings.TrimSpace(f.SupersededBy) != "" {
		decision.Bucket = LifecycleBucketSuperseded
		return decision
	}
	if f.EpicContainer {
		decision.Bucket = LifecycleBucketEpicContainer
		return decision
	}
	if f.RetryCooldownActive {
		decision.Bucket = LifecycleBucketRetryCooldown
		return decision
	}
	if f.ExecutionEligibleKnown && !f.ExecutionEligible {
		decision.Bucket = LifecycleBucketNotEligible
		return decision
	}
	decision.Bucket = LifecycleBucketReady
	decision.WorkerEligible = true
	return decision
}

func validateLifecycleStatus(status string) error {
	if IsCanonicalStatus(status) {
		return nil
	}
	return fmt.Errorf("%s: %q", LifecycleIssueInvalidStatus, status)
}

func lifecycleTransitionAllowed(from, to string) bool {
	switch from {
	case StatusProposed:
		return to == StatusOpen || to == StatusCancelled
	case StatusOpen:
		return to == StatusInProgress || to == StatusBlocked || to == StatusProposed || to == StatusCancelled
	case StatusInProgress:
		return to == StatusOpen || to == StatusClosed || to == StatusBlocked || to == StatusProposed
	case StatusBlocked:
		return to == StatusOpen || to == StatusProposed || to == StatusCancelled
	default:
		return false
	}
}

func lifecycleTransitionError(code LifecycleIssueCode, from, to string) error {
	return fmt.Errorf("%s: %s -> %s", code, from, to)
}

func lifecycleLegacyLabelWarnings(labels []string) []string {
	var warnings []string
	for _, label := range labels {
		switch label {
		case LabelNeedsHuman, LabelNeedsInvestigation:
			warnings = append(warnings, fmt.Sprintf("%s: label %q is explanatory metadata, not lifecycle state", LifecycleIssueLegacyLabelLifecycleMetadata, label))
		default:
			if hasNoChangesTriageLabelValue(label) {
				warnings = append(warnings, fmt.Sprintf("%s: label %q is explanatory metadata, not lifecycle state", LifecycleIssueLegacyLabelLifecycleMetadata, label))
			}
		}
	}
	return warnings
}

func hasNoChangesTriageLabelValue(label string) bool {
	for _, triageLabel := range noChangesTriageLabels {
		if label == triageLabel {
			return true
		}
	}
	return false
}

// ClaimMetadataExtraKeys is the canonical list of legacy tracked claim-metadata
// keys in Bead.Extra. New live worker claim data now lives in the external
// claim-lease sidecar; these keys remain only for compatibility cleanup of
// imported or pre-migration rows.
var ClaimMetadataExtraKeys = []string{
	"claimed-at",
	"claimed-pid",
	"claimed-machine",
	"claimed-session",
	"claimed-worktree",
}

// ClaimHeartbeatExtraKey is the Extra key for the work heartbeat
// timestamp. It is cleared by both Unclaim and Reopen but is kept separate
// from ClaimMetadataExtraKeys because its cleanup semantics differ in some
// call sites (e.g. SetExecutionCooldown does not touch it).
const ClaimHeartbeatExtraKey = "work-heartbeat-at"
