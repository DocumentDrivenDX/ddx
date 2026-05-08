package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPreClaimIntakeRewrite_AllowsCompressionReplacement verifies that a
// replacement description can remove stale/noisy old prose when explicit
// commitments and durable anchors are preserved. (AC1)
func TestPreClaimIntakeRewrite_AllowsCompressionReplacement(t *testing.T) {
	original := &bead.Bead{
		ID: "ddx-test0001",
		Description: "PROBLEM\nThe old intake system duplicated stale chat noise in every bead prompt." +
			" Extra: references to slack thread 2025-01, per internal RFC-47.\n\n" +
			"ROOT CAUSE\ncli/internal/agent/preclaim_intake_rewrite.go:119 enforces append-only check\n\n" +
			"PROPOSED FIX\nReplace substring check per FEAT-010 and ADR-023\n\n" +
			"NON-SCOPE\nDo not change acceptance criteria validator\n",
		Acceptance: "1. TestPreClaimIntakeRewrite_AllowsCompressionReplacement\n2. cd cli && go test ./internal/agent/... green\n",
	}
	// Compressed replacement: drops noisy prose, keeps all explicit commitments.
	compressedDesc := "PROBLEM\nAppend-only check rejects valid replacements per FEAT-010 and ADR-023.\n\n" +
		"ROOT CAUSE\ncli/internal/agent/preclaim_intake_rewrite.go:119 enforces substring containment\n\n" +
		"PROPOSED FIX\nReplace with commitment-based validation\n\n" +
		"NON-SCOPE\nDo not change acceptance criteria validator\n"

	rewrite := PreClaimIntakeRewrite{
		Description:   compressedDesc,
		ChangedFields: []string{"description"},
	}

	got, _, _, preservation, err := validateAndApplyPreClaimIntakeRewrite(original, rewrite)
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(compressedDesc), got.Description)
	assert.Contains(t, preservation, "FEAT-010")
	assert.Contains(t, preservation, "ADR-023")
	assert.Contains(t, preservation, "cli/internal/agent/preclaim_intake_rewrite.go:119")
	assert.Contains(t, preservation, "non_scope:Do not change acceptance criteria validator")
	// Stale prose is gone from the replacement.
	assert.NotContains(t, got.Description, "slack thread 2025-01")
	assert.NotContains(t, got.Description, "RFC-47")
}

// TestPreClaimIntakeRewrite_AllowsExpansionReplacement verifies that a vague
// one-line description can expand into a standalone executable bead when the
// added content is grounded in durable context and does not invent product
// semantics. (AC2)
func TestPreClaimIntakeRewrite_AllowsExpansionReplacement(t *testing.T) {
	original := &bead.Bead{
		ID:          "ddx-test0002",
		Description: "add a pre-claim rewrite hook",
		Acceptance:  "1. TestPreClaimIntakeRewrite_AllowsExpansionReplacement\n2. cd cli && go test ./internal/agent/... green\n",
	}
	// Expanded replacement: adds PROBLEM/ROOT CAUSE/PROPOSED FIX structure with
	// durable file:line anchors. Does not add any Test* functions or acceptance
	// criteria — those belong in the acceptance field.
	expandedDesc := "PROBLEM\nThe execute-loop dispatches beads without evaluating whether the " +
		"description is a fit-for-purpose implementation prompt.\n\n" +
		"ROOT CAUSE\ncli/internal/agent/execute_bead_loop.go:600 dispatches without readiness gate\n\n" +
		"PROPOSED FIX\nAdd PreClaimIntakeHook wired in the loop at claim time\n\n" +
		"NON-SCOPE\nDo not add acceptance criteria in this description\n"

	rewrite := PreClaimIntakeRewrite{
		Description:   expandedDesc,
		ChangedFields: []string{"description"},
	}

	got, _, _, preservation, err := validateAndApplyPreClaimIntakeRewrite(original, rewrite)
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(expandedDesc), got.Description)
	// No commitments in the vague one-liner, so preservation evidence is empty.
	assert.Empty(t, preservation)
	// Expansion adds durable context.
	assert.Contains(t, got.Description, "execute_bead_loop.go:600")
}

// TestPreClaimIntakeRewrite_RejectsDeletedCommitment verifies that non-scope
// items, governing artifact references, named tests, or unresolved constraints
// cannot be silently removed by a replacement rewrite. (AC3)
func TestPreClaimIntakeRewrite_RejectsDeletedCommitment(t *testing.T) {
	tests := []struct {
		name    string
		oldDesc string
		newDesc string
		wantErr string
	}{
		{
			name: "governing_ref_dropped",
			oldDesc: "PROBLEM\nfix intake per FEAT-010 and ADR-023\n\n" +
				"ROOT CAUSE\ncli/internal/agent/foo.go:10\n",
			newDesc: "PROBLEM\nfix intake\n\nROOT CAUSE\ncli/internal/agent/foo.go:10\n",
			wantErr: `drops commitment "FEAT-010"`,
		},
		{
			name: "non_scope_bullet_dropped",
			oldDesc: "PROBLEM\nfix intake\n\n" +
				"NON-SCOPE\nDo not rename existing structs\n",
			newDesc: "PROBLEM\nfix intake\n",
			wantErr: `drops commitment "non_scope:Do not rename existing structs"`,
		},
		{
			name: "named_test_dropped",
			oldDesc: "PROBLEM\nfix intake\n\n" +
				"ROOT CAUSE\nmust satisfy TestPreClaimIntakeHook\n",
			newDesc: "PROBLEM\nfix intake\n\nROOT CAUSE\nsee failing check\n",
			wantErr: `drops commitment "TestPreClaimIntakeHook"`,
		},
		{
			name: "file_line_dropped",
			oldDesc: "PROBLEM\nfix intake\n\n" +
				"ROOT CAUSE\ncli/internal/agent/preclaim_intake_rewrite.go:119\n",
			newDesc: "PROBLEM\nfix intake\n\nROOT CAUSE\nsome other cause\n",
			wantErr: `drops commitment "cli/internal/agent/preclaim_intake_rewrite.go:119"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := &bead.Bead{
				ID:          "ddx-test0003",
				Description: tt.oldDesc,
				Acceptance:  "1. TestPreClaimIntakeRewrite_RejectsDeletedCommitment\n",
			}
			rewrite := PreClaimIntakeRewrite{
				Description:   tt.newDesc,
				ChangedFields: []string{"description"},
			}
			_, _, _, _, err := validateAndApplyPreClaimIntakeRewrite(original, rewrite)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestPreClaimIntakeRewrite_RecordsReplacementEvidence verifies that the
// intake-rewritten event records changed_fields, rationale, before/after
// hashes, and preservation_evidence sufficient to audit the original text
// without duplicating it in the replacement body. (AC4)
func TestPreClaimIntakeRewrite_RecordsReplacementEvidence(t *testing.T) {
	root := t.TempDir()
	store := bead.NewStore(filepath.Join(root, ".ddx"))
	require.NoError(t, store.Init())

	oldDesc := "PROBLEM\nstale chat noise from RFC-47\n\n" +
		"ROOT CAUSE\ncli/internal/agent/preclaim_intake_rewrite.go:119 enforces append-only\n\n" +
		"PROPOSED FIX\nReplace per FEAT-010\n\n" +
		"NON-SCOPE\nDo not change acceptance criteria validator\n"
	newDesc := "PROBLEM\nAppend-only check rejects valid replacements per FEAT-010.\n\n" +
		"ROOT CAUSE\ncli/internal/agent/preclaim_intake_rewrite.go:119 enforces substring containment\n\n" +
		"NON-SCOPE\nDo not change acceptance criteria validator\n"

	b := &bead.Bead{
		ID:          "ddx-test0004",
		Title:       "test: replacement evidence",
		IssueType:   bead.DefaultType,
		Status:      bead.StatusOpen,
		Priority:    1,
		Description: oldDesc,
		Acceptance:  "1. TestPreClaimIntakeRewrite_RecordsReplacementEvidence\n",
	}
	require.NoError(t, store.Create(b))

	intake := PreClaimIntakeResult{
		Outcome: PreClaimIntakeActionableButRewritten,
		Detail:  "compressed stale RFC-47 prose",
		Rewrite: PreClaimIntakeRewrite{
			Description:   newDesc,
			ChangedFields: []string{"description"},
		},
	}
	at := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
	err := applyPreClaimIntakeRewrite(store, b.ID, "test-worker", intake, at)
	require.NoError(t, err)

	events, err := store.Events(b.ID)
	require.NoError(t, err)

	var found bool
	for _, ev := range events {
		if ev.Kind != "intake-rewritten" {
			continue
		}
		found = true
		var body preClaimIntakeRewriteEventBody
		require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))

		// Rationale and changed fields are recorded.
		assert.Equal(t, "compressed stale RFC-47 prose", body.Rationale)
		assert.Equal(t, []string{"description"}, body.ChangedFields)

		// Before/after hashes are present and different.
		assert.NotEmpty(t, body.Before.DescriptionSHA)
		assert.NotEmpty(t, body.After.DescriptionSHA)
		assert.NotEqual(t, body.Before.DescriptionSHA, body.After.DescriptionSHA)

		// Preservation evidence lists the durable anchors that were checked.
		assert.Contains(t, body.PreservationEvidence, "FEAT-010")
		assert.Contains(t, body.PreservationEvidence, "cli/internal/agent/preclaim_intake_rewrite.go:119")
		assert.Contains(t, body.PreservationEvidence, "non_scope:Do not change acceptance criteria validator")

		// Original body is NOT duplicated in the event.
		assert.NotContains(t, ev.Body, "stale chat noise from RFC-47")
	}
	require.True(t, found, "intake-rewritten event must be recorded")
}

// TestPreClaimIntakePrompt_AsksForFitForPurposeValidatedReplacement verifies
// that the intake prompt tells the model to optimize prompt fitness, avoid
// append-only noise, and classify ambiguous cases as needs-human. (AC5)
func TestPreClaimIntakePrompt_AsksForFitForPurposeValidatedReplacement(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	prompt, err := buildPreClaimIntakePrompt(root, store, b)
	require.NoError(t, err)

	lower := strings.ToLower(prompt)

	// Prompt must describe fit-for-purpose / prompt fitness as a rewrite goal.
	assert.True(t,
		strings.Contains(lower, "prompt fitness") || strings.Contains(lower, "fit-for-purpose"),
		"prompt must mention prompt fitness or fit-for-purpose; got:\n%s", prompt)

	// Prompt must allow replacement (not just appending).
	assert.True(t,
		strings.Contains(lower, "replacement") || strings.Contains(lower, "replace"),
		"prompt must allow replacement rewrites; got:\n%s", prompt)

	// Prompt must instruct the model to classify unclear cases as ambiguous_needs_human.
	assert.True(t,
		strings.Contains(lower, "ambiguous_needs_human") || strings.Contains(lower, "needs_human"),
		"prompt must instruct classification as ambiguous_needs_human for unclear cases; got:\n%s", prompt)
}
