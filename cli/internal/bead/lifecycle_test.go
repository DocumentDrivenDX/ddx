package bead

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleTransitionMatrix(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		opts LifecycleTransitionOptions
		want bool
	}{
		{name: "noop open", from: StatusOpen, to: StatusOpen, want: true},
		{name: "proposed approved", from: StatusProposed, to: StatusOpen, want: true},
		{name: "proposed cancelled", from: StatusProposed, to: StatusCancelled, want: true},
		{name: "open claimed", from: StatusOpen, to: StatusInProgress, want: true},
		{name: "open external blocked", from: StatusOpen, to: StatusBlocked, opts: LifecycleTransitionOptions{ExternalBlockerReason: "waiting on upstream API fix"}, want: true},
		{name: "open operator required", from: StatusOpen, to: StatusProposed, opts: LifecycleTransitionOptions{OperatorRequired: true}, want: true},
		{name: "open cancelled", from: StatusOpen, to: StatusCancelled, want: true},
		{name: "in progress released", from: StatusInProgress, to: StatusOpen, want: true},
		{name: "in progress closed", from: StatusInProgress, to: StatusClosed, want: true},
		{name: "in progress external blocked", from: StatusInProgress, to: StatusBlocked, opts: LifecycleTransitionOptions{ExternalBlockerReason: "release unavailable"}, want: true},
		{name: "in progress operator required", from: StatusInProgress, to: StatusProposed, opts: LifecycleTransitionOptions{OperatorRequired: true}, want: true},
		{name: "blocked unblocked", from: StatusBlocked, to: StatusOpen, want: true},
		{name: "blocked operator required", from: StatusBlocked, to: StatusProposed, opts: LifecycleTransitionOptions{OperatorRequired: true}, want: true},
		{name: "blocked cancelled", from: StatusBlocked, to: StatusCancelled, want: true},
		{name: "manual close from open", from: StatusOpen, to: StatusClosed, opts: LifecycleTransitionOptions{ManualClose: true}, want: true},
		{name: "manual reopen from closed", from: StatusClosed, to: StatusOpen, opts: LifecycleTransitionOptions{ManualReopen: true}, want: true},
		{name: "open cannot close directly", from: StatusOpen, to: StatusClosed, want: false},
		{name: "closed terminal", from: StatusClosed, to: StatusOpen, want: false},
		{name: "cancelled terminal", from: StatusCancelled, to: StatusOpen, want: false},
		{name: "proposed cannot claim directly", from: StatusProposed, to: StatusInProgress, want: false},
		{name: "open proposed requires operator", from: StatusOpen, to: StatusProposed, want: false},
		{name: "blocked proposed requires operator", from: StatusBlocked, to: StatusProposed, want: false},
		{name: "blocked requires external reason", from: StatusOpen, to: StatusBlocked, want: false},
		{name: "invalid from", from: "needs_human", to: StatusOpen, want: false},
		{name: "invalid to", from: StatusOpen, to: "needs_human", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLifecycleTransition(tt.from, tt.to, tt.opts)
			if tt.want {
				require.NoError(t, err)
				assert.True(t, ValidateLifecycleTransition(tt.from, tt.to, tt.opts) == nil)
			} else {
				require.Error(t, err)
				assert.False(t, ValidateLifecycleTransition(tt.from, tt.to, tt.opts) == nil)
			}
		})
	}
}

func TestLifecycleQueueDerivation(t *testing.T) {
	tests := []struct {
		name              string
		facts             LifecycleQueueFacts
		wantBucket        LifecycleQueueBucket
		wantWorker        bool
		wantAttention     bool
		wantDepSatisfied  bool
		wantSatisfiesDeps bool
		wantIssue         LifecycleIssueCode
	}{
		{
			name:             "execution-ready open",
			facts:            LifecycleQueueFacts{Status: StatusOpen},
			wantBucket:       LifecycleBucketReady,
			wantWorker:       true,
			wantDepSatisfied: true,
		},
		{
			name:             "dependency-waiting open",
			facts:            LifecycleQueueFacts{Status: StatusOpen, Dependencies: []LifecycleDependencyState{{Status: StatusClosed}, {Status: StatusOpen}}},
			wantBucket:       LifecycleBucketWaitingDependencies,
			wantDepSatisfied: false,
		},
		{
			name:             "proposed operator attention",
			facts:            LifecycleQueueFacts{Status: StatusProposed},
			wantBucket:       LifecycleBucketProposed,
			wantAttention:    true,
			wantDepSatisfied: true,
		},
		{
			name:             "in progress claimed",
			facts:            LifecycleQueueFacts{Status: StatusInProgress, ClaimFresh: true},
			wantBucket:       LifecycleBucketClaimed,
			wantDepSatisfied: true,
		},
		{
			name:             "externally blocked",
			facts:            LifecycleQueueFacts{Status: StatusBlocked, ExternalBlockerReason: "fizeau release unavailable"},
			wantBucket:       LifecycleBucketBlockedExternal,
			wantDepSatisfied: true,
		},
		{
			name:              "closed terminal",
			facts:             LifecycleQueueFacts{Status: StatusClosed},
			wantBucket:        LifecycleBucketClosedTerminal,
			wantDepSatisfied:  true,
			wantSatisfiesDeps: true,
		},
		{
			name:             "cancelled terminal",
			facts:            LifecycleQueueFacts{Status: StatusCancelled},
			wantBucket:       LifecycleBucketCancelledTerminal,
			wantDepSatisfied: true,
		},
		{
			name:             "retry cooldown",
			facts:            LifecycleQueueFacts{Status: StatusOpen, RetryCooldownActive: true},
			wantBucket:       LifecycleBucketRetryCooldown,
			wantDepSatisfied: true,
		},
		{
			name:             "execution explicitly ineligible",
			facts:            LifecycleQueueFacts{Status: StatusOpen, ExecutionEligibleKnown: true, ExecutionEligible: false},
			wantBucket:       LifecycleBucketNotEligible,
			wantDepSatisfied: true,
		},
		{
			name:             "invalid status",
			facts:            LifecycleQueueFacts{Status: "needs_human"},
			wantBucket:       LifecycleBucketInvalid,
			wantDepSatisfied: true,
			wantIssue:        LifecycleIssueInvalidStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateLifecycleQueue(tt.facts)
			assert.Equal(t, tt.wantBucket, got.Bucket)
			assert.Equal(t, tt.wantWorker, got.WorkerEligible)
			assert.Equal(t, tt.wantAttention, got.OperatorAttention)
			assert.Equal(t, tt.wantDepSatisfied, got.DependenciesSatisfied)
			assert.Equal(t, tt.wantSatisfiesDeps, got.SatisfiesDependents)
			if tt.wantIssue != "" {
				require.NotEmpty(t, got.Issues)
				assert.Equal(t, tt.wantIssue, got.Issues[0].Code)
			} else {
				assert.Empty(t, got.Issues)
			}
		})
	}
}

func TestLifecycleDependencySatisfactionClosedOnly(t *testing.T) {
	for _, status := range CanonicalStatuses {
		got := LifecycleStatusSatisfiesDependency(status)
		assert.Equalf(t, status == StatusClosed, got, "status %s", status)
	}
	assert.False(t, LifecycleStatusSatisfiesDependency("needs_human"))
	assert.True(t, LifecycleDependenciesSatisfied([]LifecycleDependencyState{{Status: StatusClosed}, {Status: StatusClosed}}))
	assert.False(t, LifecycleDependenciesSatisfied([]LifecycleDependencyState{{Status: StatusClosed}, {Status: StatusCancelled}}))
}

func TestLifecycleBlockedRequiresExternalReason(t *testing.T) {
	waiting := EvaluateLifecycleQueue(LifecycleQueueFacts{
		Status:       StatusOpen,
		Dependencies: []LifecycleDependencyState{{Status: StatusOpen}},
	})
	assert.Equal(t, LifecycleBucketWaitingDependencies, waiting.Bucket)
	assert.Empty(t, waiting.Issues)

	missingReason := EvaluateLifecycleQueue(LifecycleQueueFacts{Status: StatusBlocked})
	require.Equal(t, LifecycleBucketBlockedExternal, missingReason.Bucket)
	require.NotEmpty(t, missingReason.Issues)
	assert.Equal(t, LifecycleIssueBlockedMissingExternalReason, missingReason.Issues[0].Code)

	emptyReason := EvaluateLifecycleQueue(LifecycleQueueFacts{Status: StatusBlocked, ExternalBlockerReason: "  "})
	require.NotEmpty(t, emptyReason.Issues)
	assert.Equal(t, LifecycleIssueBlockedMissingExternalReason, emptyReason.Issues[0].Code)

	withReason := EvaluateLifecycleQueue(LifecycleQueueFacts{Status: StatusBlocked, ExternalBlockerReason: "waiting for upstream release"})
	assert.Equal(t, LifecycleBucketBlockedExternal, withReason.Bucket)
	assert.Equal(t, "waiting for upstream release", withReason.ExternalBlockerReason)
	assert.Empty(t, withReason.Issues)
}

func TestLifecycleLegacyLabelsDoNotControlLifecycle(t *testing.T) {
	labels := []string{LabelNeedsHuman, LabelNeedsInvestigation, LabelNoChangesUnverified}

	open := EvaluateLifecycleQueue(LifecycleQueueFacts{
		Status:       StatusOpen,
		LegacyLabels: labels,
	})
	assert.Equal(t, LifecycleBucketReady, open.Bucket)
	assert.True(t, open.WorkerEligible)
	assert.Empty(t, open.Issues)
	require.Len(t, open.Warnings, len(labels))

	proposed := EvaluateLifecycleQueue(LifecycleQueueFacts{
		Status:       StatusProposed,
		LegacyLabels: labels,
	})
	assert.Equal(t, LifecycleBucketProposed, proposed.Bucket)
	assert.True(t, proposed.OperatorAttention)
	assert.False(t, proposed.WorkerEligible)

	migrating := EvaluateLifecycleQueue(LifecycleQueueFacts{
		Status:                  StatusOpen,
		LegacyLabels:            labels,
		LegacyMigrationWarnings: []string{"legacy label needs_human requires migration review"},
	})
	assert.Equal(t, LifecycleBucketReady, migrating.Bucket)
	require.NotEmpty(t, migrating.Issues)
	assert.Equal(t, LifecycleIssueLegacyMigrationCompatibility, migrating.Issues[0].Code)
}

// TestIsOrdinaryEpicContainer_ZeroOpenChildrenRoutesToClosureCandidate locks in
// the routing rule introduced for ddx-e9f3adaf: epics with no children at all
// must leave the container bucket (so the work-loop layer can diagnose them as
// genuinely-undecomposed), while epics with only-closed children remain
// containers and surface as closure candidates. Epics with at least one open
// child stay as ordinary containers, and non-epic beads are never containers.
func TestIsOrdinaryEpicContainer_ZeroOpenChildrenRoutesToClosureCandidate(t *testing.T) {
	tests := []struct {
		name                 string
		bead                 Bead
		openChildCount       int
		totalChildCount      int
		wantContainer        bool
		wantClosureCandidate bool
	}{
		{
			name:                 "epic with only closed children is a closure candidate",
			bead:                 Bead{ID: "ddx-epic-closed", IssueType: "epic", Title: "Closed-child epic"},
			openChildCount:       0,
			totalChildCount:      3,
			wantContainer:        true,
			wantClosureCandidate: true,
		},
		{
			name:                 "epic with zero children is not a container",
			bead:                 Bead{ID: "ddx-epic-empty", IssueType: "epic", Title: "Empty epic"},
			openChildCount:       0,
			totalChildCount:      0,
			wantContainer:        false,
			wantClosureCandidate: false,
		},
		{
			name:                 "epic with open children stays an ordinary container",
			bead:                 Bead{ID: "ddx-epic-open", IssueType: "epic", Title: "Active epic"},
			openChildCount:       2,
			totalChildCount:      4,
			wantContainer:        true,
			wantClosureCandidate: false,
		},
		{
			name:                 "non-epic bead with children is not an epic container",
			bead:                 Bead{ID: "ddx-task", IssueType: "task", Title: "Plain task"},
			openChildCount:       1,
			totalChildCount:      2,
			wantContainer:        false,
			wantClosureCandidate: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotContainer := isOrdinaryEpicContainer(tc.bead, tc.openChildCount, tc.totalChildCount)
			assert.Equal(t, tc.wantContainer, gotContainer, "isOrdinaryEpicContainer")
			gotClosure := isEpicClosureCandidate(tc.bead, tc.openChildCount, tc.totalChildCount)
			assert.Equal(t, tc.wantClosureCandidate, gotClosure, "isEpicClosureCandidate")
		})
	}
}
