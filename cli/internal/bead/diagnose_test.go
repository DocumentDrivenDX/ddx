package bead

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDiagnose is a table-driven check that Diagnose returns the expected
// BlockReasonCode for one fixture per code in FEAT-010 §Idle-Path Diagnosis
// and Auto-Remediation, including the no_diagnosis fallback and the
// parent_child_state_conflict case (AC #1, #2).
func TestDiagnose(t *testing.T) {
	type fixture struct {
		name     string
		bead     Bead
		children map[string][]Bead
		open     map[string][]Bead
		deps     map[string][]Dependency
		wantCode BlockReasonCode
	}

	cases := []fixture{
		{
			name: "superseded_pending_close",
			bead: Bead{
				ID:     "A",
				Status: StatusOpen,
				Extra:  map[string]any{"superseded-by": "B"},
			},
			children: map[string][]Bead{
				"": {{ID: "B", Status: StatusClosed}},
			},
			open:     map[string][]Bead{},
			deps:     map[string][]Dependency{},
			wantCode: ReasonSupersededPendingClose,
		},
		{
			name: "closure_candidate_misclassified",
			bead: Bead{ID: "P", Status: StatusOpen, IssueType: "epic"},
			children: map[string][]Bead{
				"P": {{ID: "C1", Status: StatusClosed}, {ID: "C2", Status: StatusClosed}},
			},
			open:     map[string][]Bead{"P": nil},
			deps:     map[string][]Dependency{},
			wantCode: ReasonClosureCandidateMisclassified,
		},
		{
			name: "dead_intermediate_all_children_closed",
			bead: Bead{
				ID:        "M",
				Status:    StatusOpen,
				IssueType: "task",
				Extra:     map[string]any{ExtraExecutionElig: false},
			},
			children: map[string][]Bead{
				"M": {{ID: "C1", Status: StatusClosed}},
			},
			open:     map[string][]Bead{"M": nil},
			deps:     map[string][]Dependency{},
			wantCode: ReasonDeadIntermediateAllChildrenClosed,
		},
		{
			name: "dead_intermediate_open_children_pending",
			bead: Bead{
				ID:        "M",
				Status:    StatusOpen,
				IssueType: "task",
				Extra:     map[string]any{ExtraExecutionElig: false},
			},
			children: map[string][]Bead{
				"M": {{ID: "C1", Status: StatusOpen}},
			},
			open: map[string][]Bead{
				"M": {{ID: "C1", Status: StatusOpen}},
			},
			deps:     map[string][]Dependency{},
			wantCode: ReasonDeadIntermediateOpenChildrenPending,
		},
		{
			name: "epic_of_epic",
			bead: Bead{ID: "E", Status: StatusOpen, IssueType: "task"},
			children: map[string][]Bead{
				"E": {{ID: "EC", Status: StatusOpen, IssueType: "epic"}},
			},
			open: map[string][]Bead{
				"E": {{ID: "EC", Status: StatusOpen, IssueType: "epic"}},
			},
			deps:     map[string][]Dependency{},
			wantCode: ReasonEpicOfEpic,
		},
		{
			name: "dep_blocked_by",
			bead: Bead{
				ID:     "A",
				Status: StatusOpen,
				Dependencies: []Dependency{
					{IssueID: "A", DependsOnID: "Z", Type: "blocks"},
				},
			},
			children: map[string][]Bead{
				"": {{ID: "Z", Status: StatusOpen}},
			},
			open:     map[string][]Bead{},
			deps:     map[string][]Dependency{},
			wantCode: ReasonDepBlockedBy,
		},
		{
			name:     "genuinely_needs_decomposition",
			bead:     Bead{ID: "E", Status: StatusOpen, IssueType: "epic"},
			children: map[string][]Bead{},
			open:     map[string][]Bead{},
			deps:     map[string][]Dependency{},
			wantCode: ReasonGenuinelyNeedsDecomposition,
		},
		{
			name: "parent_child_state_conflict",
			bead: Bead{
				ID:     "C",
				Status: StatusInProgress,
				Parent: "P",
			},
			children: map[string][]Bead{
				"":  {{ID: "P", Status: StatusClosed}},
				"P": {{ID: "C", Status: StatusInProgress}},
			},
			open:     map[string][]Bead{},
			deps:     map[string][]Dependency{},
			wantCode: ReasonParentChildStateConflict,
		},
		{
			name:     "claimed_in_progress",
			bead:     Bead{ID: "A", Status: StatusInProgress},
			children: map[string][]Bead{},
			open:     map[string][]Bead{},
			deps:     map[string][]Dependency{},
			wantCode: ReasonClaimedInProgress,
		},
		{
			name: "provider_route_unavailable",
			bead: Bead{
				ID:     "A",
				Status: StatusOpen,
				Extra: map[string]any{
					ExtraLastDetail: "ResolveRoute: no live provider for power=4",
				},
			},
			children: map[string][]Bead{},
			open:     map[string][]Bead{},
			deps:     map[string][]Dependency{},
			wantCode: ReasonProviderRouteUnavailable,
		},
		{
			name: "gated_by_budget_or_cooldown",
			bead: Bead{
				ID:     "A",
				Status: StatusOpen,
				Extra:  map[string]any{ExtraRetryAfter: "2026-12-31T00:00:00Z"},
			},
			children: map[string][]Bead{},
			open:     map[string][]Bead{},
			deps:     map[string][]Dependency{},
			wantCode: ReasonGatedByBudgetOrCooldown,
		},
		{
			name: "malformed_parent_or_dep_ref",
			bead: Bead{
				ID:     "A",
				Status: StatusOpen,
				Parent: "ghost",
			},
			children: map[string][]Bead{},
			open:     map[string][]Bead{},
			deps:     map[string][]Dependency{},
			wantCode: ReasonMalformedParentOrDepRef,
		},
		{
			name: "dependency_cycle",
			bead: Bead{
				ID:     "A",
				Status: StatusOpen,
				Dependencies: []Dependency{
					{IssueID: "A", DependsOnID: "B", Type: "blocks"},
				},
			},
			children: map[string][]Bead{
				"": {{ID: "B", Status: StatusClosed}},
			},
			open: map[string][]Bead{},
			deps: map[string][]Dependency{
				"A": {{IssueID: "A", DependsOnID: "B"}},
				"B": {{IssueID: "B", DependsOnID: "A"}},
			},
			wantCode: ReasonDependencyCycle,
		},
		{
			name: "closed_or_missing_parent",
			bead: Bead{
				ID:     "C",
				Status: StatusOpen,
				Parent: "P",
			},
			children: map[string][]Bead{
				"":  {{ID: "P", Status: StatusClosed}},
				"P": {{ID: "C", Status: StatusOpen}},
			},
			open:     map[string][]Bead{},
			deps:     map[string][]Dependency{},
			wantCode: ReasonClosedOrMissingParent,
		},
		{
			name: "stale_graph_index",
			bead: Bead{ID: "P", Status: StatusOpen, IssueType: "task"},
			children: map[string][]Bead{
				"P": {{ID: "C1", Status: StatusClosed}},
			},
			open: map[string][]Bead{
				"P": {{ID: "C1", Status: StatusClosed}},
			},
			deps:     map[string][]Dependency{},
			wantCode: ReasonStaleGraphIndex,
		},
		{
			name: "auto_remediation_exhausted",
			bead: Bead{
				ID:     "A",
				Status: StatusOpen,
				Labels: []string{"auto-remediation:exhausted"},
			},
			children: map[string][]Bead{},
			open:     map[string][]Bead{},
			deps:     map[string][]Dependency{},
			wantCode: ReasonAutoRemediationExhausted,
		},
		{
			name:     "no_diagnosis",
			bead:     Bead{ID: "A", Status: StatusOpen, IssueType: "task"},
			children: map[string][]Bead{},
			open:     map[string][]Bead{},
			deps:     map[string][]Dependency{},
			wantCode: ReasonNoDiagnosis,
		},
	}

	for _, tc := range cases {
		t.Run(string(tc.wantCode), func(t *testing.T) {
			got := Diagnose(tc.bead, tc.children, tc.open, tc.deps)
			assert.Equal(t, tc.wantCode, got.Code, "fixture %s: detail=%q", tc.name, got.Detail)
		})
	}
}

// TestDiagnose_PriorityMatch covers AC #3: a bead that matches both
// superseded_pending_close and dep_blocked_by must report
// superseded_pending_close because it appears first in the FEAT-010 table.
func TestDiagnose_PriorityMatch(t *testing.T) {
	b := Bead{
		ID:     "A",
		Status: StatusOpen,
		Extra:  map[string]any{"superseded-by": "Y"},
		Dependencies: []Dependency{
			{IssueID: "A", DependsOnID: "Z", Type: "blocks"},
		},
	}
	children := map[string][]Bead{
		"": {
			{ID: "Y", Status: StatusClosed},
			{ID: "Z", Status: StatusOpen},
		},
	}
	got := Diagnose(b, children, map[string][]Bead{}, map[string][]Dependency{})
	assert.Equal(t, ReasonSupersededPendingClose, got.Code)
	assert.Contains(t, got.CitedIDs, "Y")
}
