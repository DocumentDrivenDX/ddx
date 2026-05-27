package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
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
	expandedDesc := "PROBLEM\nThe work dispatches beads without evaluating whether the " +
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
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
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

// ---- AC3: acceptancePreservesCriteria structural tests ------------------

// TestAcceptancePreservesCriteria_Identical verifies that identical text
// always preserves criteria.
func TestAcceptancePreservesCriteria_Identical(t *testing.T) {
	ac := "1. cargo test --workspace passes.\n2. cargo clippy -- -D warnings passes."
	assert.True(t, acceptancePreservesCriteria(ac, ac), "identical text must preserve criteria")
}

// TestAcceptancePreservesCriteria_ReformattedEquivalent verifies that
// renumbering (AC1. → 1.) and whitespace changes are not treated as drops.
func TestAcceptancePreservesCriteria_ReformattedEquivalent(t *testing.T) {
	before := "AC1. crates/axon-cypher/src/storage_adapter_store.rs exists.\n" +
		"AC2. cargo tree -p axon-storage does not list axon-cypher.\n" +
		"AC3. cargo test --workspace passes."

	// Same bodies, different bullet style.
	after := "1. crates/axon-cypher/src/storage_adapter_store.rs exists.\n" +
		"2. cargo tree -p axon-storage does not list axon-cypher.\n" +
		"3. cargo test --workspace passes."

	assert.True(t, acceptancePreservesCriteria(before, after),
		"renumbering AC1/AC2/AC3 to 1/2/3 must not be treated as a drop")
}

// TestAcceptancePreservesCriteria_ReorderedEquivalent verifies that reordering
// numbered criteria is not treated as a drop.
func TestAcceptancePreservesCriteria_ReorderedEquivalent(t *testing.T) {
	before := "1. cargo test --workspace passes.\n2. cargo clippy -- -D warnings passes."
	after := "1. cargo clippy -- -D warnings passes.\n2. cargo test --workspace passes."
	assert.True(t, acceptancePreservesCriteria(before, after),
		"reordering criteria must not be treated as a drop")
}

// TestAcceptancePreservesCriteria_CriterionActuallyDropped verifies that a
// rewrite which omits a verifiable assertion body is detected as a drop.
func TestAcceptancePreservesCriteria_CriterionActuallyDropped(t *testing.T) {
	before := "AC1. crates/axon-cypher/src/storage_adapter_store.rs exists.\n" +
		"AC2. cargo tree -p axon-storage does not list axon-cypher.\n" +
		"AC3. cargo test --workspace passes."

	// AC2 is silently removed in the rewrite.
	after := "1. crates/axon-cypher/src/storage_adapter_store.rs exists.\n" +
		"2. cargo test --workspace passes."

	assert.False(t, acceptancePreservesCriteria(before, after),
		"dropping a criterion must be detected even when count changes")
}

// TestAcceptancePreservesCriteria_UnstructuredOriginalExpandsToNumbered verifies
// that a vague one-sentence original AC is accepted when the rewrite expands it
// into numbered criteria (expansion, never a drop).
func TestAcceptancePreservesCriteria_UnstructuredOriginalExpandsToNumbered(t *testing.T) {
	before := "Contractor audit lineage applies the same redaction as entity reads and leaks no commercial values into the DOM."
	after := "1. Contractor audit lineage applies redactValue() before display.\n" +
		"2. Restricted values render as \"[redacted]\" in the DOM.\n" +
		"3. cd ui && pnpm test:e2e intent-audit-lineage passes."
	assert.True(t, acceptancePreservesCriteria(before, after),
		"expanding a vague sentence into numbered criteria must always be allowed")
}

// TestAcceptancePreservesCriteria_BulletDashPrefix verifies that dash-bullet
// criteria are parsed and matched correctly.
func TestAcceptancePreservesCriteria_BulletDashPrefix(t *testing.T) {
	before := "- cargo test --workspace passes.\n- cargo clippy -- -D warnings passes."
	after := "1. cargo test --workspace passes.\n2. cargo clippy -- -D warnings passes."
	assert.True(t, acceptancePreservesCriteria(before, after),
		"dash-bullet criteria must match numbered rewrite")
}

// TestAcceptancePreservesCriteria_StrongBead_AxonFixture verifies the exact
// shape from axon-11508cde: 7 AC-prefixed criteria rewritten to 7 numbered
// criteria must pass validation.
func TestAcceptancePreservesCriteria_StrongBead_AxonFixture(t *testing.T) {
	// Verbatim from axon-11508cde (trimmed to key assertions).
	before := "AC1. crates/axon-cypher/src/storage_adapter_store.rs exists; crates/axon-storage/src/storage_adapter_store.rs does not.\n" +
		"AC2. cargo tree -p axon-storage -e normal does not list axon-cypher (or axon-cypher-ast) as a direct dep.\n" +
		"AC3. cargo tree -p axon-cypher -e normal lists axon-storage as a direct dep.\n" +
		"AC4. axon_cypher::StorageAdapterQueryStore re-exports the type at the same public name.\n" +
		"AC5. The 15 unit tests originally under axon_storage::storage_adapter_store::tests are present in axon-cypher and pass via cargo test -p axon-cypher.\n" +
		"AC6. cargo test --workspace passes; cargo clippy --workspace -- -D warnings passes.\n" +
		"AC7. The dep cycle described in axon-5956e527's intake.blocked event is gone: cargo build -p axon-cypher succeeds with the StorageAdapter-backed QueryStore physically in axon-cypher."

	// Same assertions, renumbered to plain 1./2./…/7.
	after := "1. crates/axon-cypher/src/storage_adapter_store.rs exists; crates/axon-storage/src/storage_adapter_store.rs does not.\n" +
		"2. cargo tree -p axon-storage -e normal does not list axon-cypher (or axon-cypher-ast) as a direct dep.\n" +
		"3. cargo tree -p axon-cypher -e normal lists axon-storage as a direct dep.\n" +
		"4. axon_cypher::StorageAdapterQueryStore re-exports the type at the same public name.\n" +
		"5. The 15 unit tests originally under axon_storage::storage_adapter_store::tests are present in axon-cypher and pass via cargo test -p axon-cypher.\n" +
		"6. cargo test --workspace passes; cargo clippy --workspace -- -D warnings passes.\n" +
		"7. The dep cycle described in axon-5956e527's intake.blocked event is gone: cargo build -p axon-cypher succeeds with the StorageAdapter-backed QueryStore physically in axon-cypher."

	assert.True(t, acceptancePreservesCriteria(before, after),
		"axon-11508cde: 7 AC-prefixed criteria renumbered to 1–7 must not appear as a drop")
}

// ---- AC1/AC2: end-to-end hook tests with mocked LLM --------------------

// TestPreClaimIntakeHook_StrongBead_NeverOperatorRequired verifies that a
// bead with 7 numbered ACs, file:line refs, and named tests is not classified
// as operator_required (AC1 from ddx-ea3e3415).
func TestPreClaimIntakeHook_StrongBead_NeverOperatorRequired(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init())

	// Strong bead fixture derived from axon-11508cde: 7 numbered ACs with
	// cargo tree flags, file paths, and a rollback plan.
	strongBead := &bead.Bead{
		ID:        "ddx-strongbead",
		Title:     "refactor(axon-cypher): relocate StorageAdapterQueryStore from axon-storage",
		IssueType: bead.DefaultType,
		Status:    bead.StatusOpen,
		Priority:  0,
		Labels:    []string{"helix", "feat-009", "area:cypher", "area:storage", "kind:refactor"},
		Acceptance: "AC1. crates/axon-cypher/src/storage_adapter_store.rs exists; crates/axon-storage/src/storage_adapter_store.rs does not.\n" +
			"AC2. cargo tree -p axon-storage -e normal does not list axon-cypher (or axon-cypher-ast) as a direct dep.\n" +
			"AC3. cargo tree -p axon-cypher -e normal lists axon-storage as a direct dep.\n" +
			"AC4. axon_cypher::StorageAdapterQueryStore re-exports the type at the same public name.\n" +
			"AC5. The 15 unit tests originally under axon_storage::storage_adapter_store::tests are present in axon-cypher and pass via cargo test -p axon-cypher.\n" +
			"AC6. cargo test --workspace passes; cargo clippy --workspace -- -D warnings passes.\n" +
			"AC7. The dep cycle described in axon-5956e527's intake.blocked event is gone.",
		Description: "Move StorageAdapterQueryStore from axon-storage to axon-cypher.\n\n" +
			"In-scope files:\n  - crates/axon-cypher/src/storage_adapter_store.rs (moved)\n  - crates/axon-storage/src/lib.rs\n\n" +
			"Rollback: revert all six file changes atomically.",
	}
	require.NoError(t, store.Create(strongBead))

	// The LLM returns actionable_atomic for this well-formed bead.
	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"classification":"atomic","confidence":0.97,"reasoning":"7 numbered ACs with cargo tree flags and rollback; single-slice refactor"}`,
	}

	hook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)
	got, err := hook(context.Background(), strongBead.ID)
	require.NoError(t, err)
	assert.NotEqual(t, PreClaimIntakeOperatorRequired, got.Outcome,
		"strong bead with 7 numbered ACs must not be classified operator_required; got outcome=%s detail=%s",
		got.Outcome, got.Detail)
}

// TestPreClaimIntakeHook_StrongBead_RewriteACPrefixToNumeric verifies that
// when the LLM rewrites a bead with AC-prefixed criteria to plain-numeric
// criteria, the rewrite is accepted (not rejected as "criteria dropped or
// altered") (AC4 from ddx-ea3e3415).
func TestPreClaimIntakeHook_StrongBead_RewriteACPrefixToNumeric(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init())

	strongBead := &bead.Bead{
		ID:        "ddx-strongbead2",
		Title:     "refactor(axon-cypher): relocate StorageAdapterQueryStore from axon-storage",
		IssueType: bead.DefaultType,
		Status:    bead.StatusOpen,
		Priority:  0,
		Acceptance: "AC1. crates/axon-cypher/src/storage_adapter_store.rs exists; crates/axon-storage/src/storage_adapter_store.rs does not.\n" +
			"AC2. cargo tree -p axon-storage -e normal does not list axon-cypher.\n" +
			"AC3. cargo test --workspace passes; cargo clippy --workspace -- -D warnings passes.",
		Description: "Move StorageAdapterQueryStore from axon-storage to axon-cypher.",
	}
	require.NoError(t, store.Create(strongBead))

	// LLM rewrites the AC field: same assertions, plain 1./2./3. numbering.
	rewrittenAC := "1. crates/axon-cypher/src/storage_adapter_store.rs exists; crates/axon-storage/src/storage_adapter_store.rs does not.\n" +
		"2. cargo tree -p axon-storage -e normal does not list axon-cypher.\n" +
		"3. cargo test --workspace passes; cargo clippy --workspace -- -D warnings passes."
	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"classification":"rewritten","confidence":0.95,"reasoning":"normalised AC prefix style","rewrite":{"changed_fields":["acceptance"],"acceptance":"` +
			strings.ReplaceAll(rewrittenAC, "\n", `\n`) + `"}}`,
	}

	hook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)
	got, err := hook(context.Background(), strongBead.ID)
	require.NoError(t, err)
	assert.NotEqual(t, PreClaimIntakeOperatorRequired, got.Outcome,
		"reformatting AC1/AC2/AC3 to 1/2/3 must not be rejected as criteria-dropped; got outcome=%s detail=%s",
		got.Outcome, got.Detail)
}

// TestPreClaimIntakeHook_WeakBead_ReturnsConcreteSuggestedFixes verifies that
// a vague single-sentence AC either yields concrete suggested_fixes or yields
// operator_required only when rationale cites a specific unresolved question
// (AC2 from ddx-ea3e3415).
func TestPreClaimIntakeHook_WeakBead_ReturnsConcreteSuggestedFixes(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init())

	// Weak bead fixture derived from axon-044a5b5b (pre-refinement): vague AC.
	weakBead := &bead.Bead{
		ID:        "ddx-weakbead",
		Title:     "fix(ui): contractor audit lineage redacts metadata fields",
		IssueType: bead.DefaultType,
		Status:    bead.StatusOpen,
		Priority:  0,
		Acceptance: "Contractor audit lineage applies the same redaction as entity reads " +
			"and leaks no commercial values into the DOM. Covered by intent-audit-lineage.spec.ts.",
		Description: "Ensure contractor audit lineage metadata fields are redacted before display.",
	}
	require.NoError(t, store.Create(weakBead))

	// LLM returns a rewrite with expanded numbered ACs (the ideal case).
	refinedAC := "1. Lineage metadata fields rendered at audit/+page.svelte pass through redactValue().\n" +
		"2. Restricted values render as \"[redacted]\" for contractor policy users.\n" +
		"3. cd ui && pnpm test:e2e intent-audit-lineage passes."
	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"classification":"rewritten","confidence":0.88,"reasoning":"expanded vague AC into numbered verifiable assertions","rewrite":{"changed_fields":["acceptance"],"acceptance":"` +
			strings.ReplaceAll(refinedAC, "\"", `\"`) + strings.ReplaceAll("\n", "\n", `\n`) + `"}}`,
	}
	// Build the JSON manually to avoid escaping issues.
	refinedACEscaped := strings.ReplaceAll(refinedAC, "\n", `\n`)
	refinedACEscaped = strings.ReplaceAll(refinedACEscaped, `"`, `\"`)
	svc.finalText = `{"classification":"rewritten","confidence":0.88,"reasoning":"expanded vague AC into numbered verifiable assertions","rewrite":{"changed_fields":["acceptance"],"acceptance":"` + refinedACEscaped + `"}}`

	hook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)
	got, err := hook(context.Background(), weakBead.ID)
	require.NoError(t, err)

	// If the outcome is operator_required, the detail must cite a specific unresolved
	// question — not just the generic "acceptance criteria dropped or altered".
	if got.Outcome == PreClaimIntakeOperatorRequired {
		assert.NotEqual(t, "acceptance criteria dropped or altered", got.Detail,
			"operator_required for a weak bead must cite a specific unresolved question, not the generic dropped-or-altered string")
		assert.NotEmpty(t, got.Detail,
			"operator_required must always include a non-empty rationale")
	}
}

// ---- AC4: axon fixture integration tests --------------------------------

// TestAcceptancePreservesCriteria_AxonFixture_044a5b5b verifies that the
// refined axon-044a5b5b acceptance field (7 numbered ACs) is accepted when
// replacing the original single-sentence AC.
func TestAcceptancePreservesCriteria_AxonFixture_044a5b5b(t *testing.T) {
	// Original single-sentence AC from axon-044a5b5b.
	before := "Contractor audit lineage applies the same redaction as entity reads " +
		"and leaks no commercial values into the DOM. Covered by intent-audit-lineage.spec.ts."

	// Refined form (the actual acceptance field from the refined bead).
	after := "AC1. Lineage metadata fields rendered at ui/src/routes/tenants/[tenant]/databases/[database]/audit/+page.svelte:397-434 (decision, policy_version, schema_version, approver.actor, approver.user_id, reason, origin.surface, origin.tool_name) pass through redactValue() before display, using the redaction list returned by the intent-detail GraphQL query.\n" +
		"AC2. Existing redaction of data_before/data_after at audit/+page.svelte:437-442 is unchanged.\n" +
		"AC3. ui/tests/e2e/intent-audit-lineage.spec.ts adds a test named \"contractor lineage redacts metadata fields\" asserting that, for a fixture user with contractor policy, lineage metadata fields render \"[redacted]\" for restricted values and plain text for permitted ones.\n" +
		"AC4. Same spec adds DOM-leakage assertions per the axon-c3895a14 sibling pattern: redacted values are absent from DOM snapshots, window.localStorage / window.sessionStorage, copied text (clipboard simulation), and console logs (page.evaluate inspections).\n" +
		"AC5. The existing 8 tests in ui/tests/e2e/intent-audit-lineage.spec.ts continue to pass for the operator role.\n" +
		"AC6. cd ui && pnpm test:e2e intent-audit-lineage passes.\n" +
		"AC7. cd ui && pnpm lint && pnpm check passes."

	assert.True(t, acceptancePreservesCriteria(before, after),
		"axon-044a5b5b: replacing vague single-sentence AC with 7 numbered ACs must be accepted as expansion")
}

// TestPreClaimIntakePrompt_AsksForFitForPurposeValidatedReplacement verifies
// that the intake prompt keeps prompt-fitness edits advisory during intake and
// classifies ambiguous cases as operator-required. (AC5)
func TestPreClaimIntakePrompt_AsksForFitForPurposeValidatedReplacement(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	prompt, err := buildPreClaimIntakePrompt(root, store, b)
	require.NoError(t, err)

	lower := strings.ToLower(prompt)

	assert.Contains(t, lower, "prompt-quality improvements")
	assert.Contains(t, lower, "suggested_fixes")
	assert.Contains(t, lower, "suggested_fixes must be a json array")
	assert.Contains(t, lower, "flat string list")
	assert.Contains(t, lower, "tractability")
	assert.Contains(t, lower, "score")
	assert.Contains(t, lower, "rationale")
	assert.Contains(t, lower, "suggested_child_beads")
	assert.Contains(t, lower, "waivers_applied")

	// Prompt must instruct the model to classify unclear cases as operator_required.
	assert.Contains(t, lower, "operator_required",
		"prompt must instruct classification as operator_required for unclear cases; got:\n%s", prompt)
	assert.NotContains(t, lower, "ambiguous_needs_human")
}

func TestBuildPreClaimIntakePrompt_UsesDocumentedReadinessSchema(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	prompt, err := buildPreClaimIntakePrompt(root, store, b)
	require.NoError(t, err)

	schema := compileReadinessChecksSchema(t)
	var documented any
	require.NoError(t, json.Unmarshal([]byte(`{"classification":"needs_refine","rationale":"check","readiness_checks":[{"reason":"missing_verification","evidence":"AC lacks go test","checkable_before_attempt":true,"verdict":true}]}`), &documented))
	require.NoError(t, schema.Validate(documented))

	lower := strings.ToLower(prompt)
	for _, want := range []string{
		readinessChecksSchemaPath,
		"classification",
		"tractability",
		"score",
		"rationale",
		"readiness_checks",
		"suggested_fixes",
		"rewrite",
		"suggested_child_beads",
		"waivers_applied",
		"verdict may be a json bool, string, null, or omitted",
	} {
		assert.Contains(t, lower, want)
	}
	assert.NotContains(t, lower, "confidence")
	assert.NotContains(t, lower, "reasoning")
}

func TestPreClaimReadinessPromptSpecifiesOutputShapes(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	prompt, err := buildPreClaimIntakePrompt(root, store, b)
	require.NoError(t, err)

	lower := strings.ToLower(prompt)
	assert.Contains(t, lower, "suggested_child_beads must be a json array of objects")
	assert.Contains(t, lower, "prefer a json string array of numbered criteria")
	assert.Contains(t, lower, "decoder tolerates a single string fallback")
	assert.Contains(t, lower, "waivers_applied must be a json array")
	assert.Contains(t, lower, "prefer waiver objects with reason, criteria, and evidence")
	assert.Contains(t, lower, "decoder tolerates a flat string list fallback")
}

func TestBuildPreClaimIntakePrompt_ForbidsScalarReadinessChecks(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	prompt, err := buildPreClaimIntakePrompt(root, store, b)
	require.NoError(t, err)

	lower := strings.ToLower(prompt)
	assert.Contains(t, lower, "readiness_checks must be a json array")
	assert.Contains(t, lower, "must not be an object or string")
	assert.Contains(t, lower, "reason")
	assert.Contains(t, lower, "verdict")
	assert.Contains(t, lower, "evidence")
	assert.Contains(t, lower, "checkable_before_attempt")
}
