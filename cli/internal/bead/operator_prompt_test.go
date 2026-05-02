package bead

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchemaAcceptsOperatorPromptIssueType validates that a JSON record with
// issue_type=operator-prompt and status=proposed passes the bead-record JSON
// schema (Story 15 §Implementation #1: schema validator must accept the new
// issueType and the new proposed status).
func TestSchemaAcceptsOperatorPromptIssueType(t *testing.T) {
	schema := compileBeadSchema(t)

	doc := `{"id":"ddx-op-1","title":"do a thing","status":"proposed","priority":2,"issue_type":"operator-prompt","created_at":"2026-05-02T00:00:00Z","updated_at":"2026-05-02T00:00:00Z"}`
	require.NoError(t, validateJSONAgainstSchema(t, schema, doc))

	cancelled := `{"id":"ddx-op-2","title":"cancel me","status":"cancelled","priority":2,"issue_type":"operator-prompt","created_at":"2026-05-02T00:00:00Z","updated_at":"2026-05-02T00:00:00Z"}`
	require.NoError(t, validateJSONAgainstSchema(t, schema, cancelled))
}

// TestStoreAcceptsOperatorPromptBead exercises the validator on the live
// store path: an operator-prompt bead in proposed status must persist and
// re-load round-trip clean.
func TestStoreAcceptsOperatorPromptBead(t *testing.T) {
	store := newTestStore(t)
	b := NewOperatorPromptBead("create a P2 bead titled 'demo'\nbody line two", 2)
	require.Equal(t, IssueTypeOperatorPrompt, b.IssueType)
	require.Equal(t, StatusProposed, b.Status)
	require.NoError(t, store.Create(b))

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, IssueTypeOperatorPrompt, got.IssueType)
	assert.Equal(t, StatusProposed, got.Status)
	assert.Contains(t, got.Labels, OperatorPromptLabelKind)
	assert.Contains(t, got.Labels, OperatorPromptLabelSource)
	assert.Equal(t, OperatorPromptDefaultAcceptance, got.Acceptance)
	assert.Equal(t, "create a P2 bead titled 'demo'", got.Title)
	assert.True(t, strings.Contains(got.Description, "body line two"))
}

// TestNewOperatorPromptBeadDefaults covers tier clamping and the
// empty-prompt fallback so operators cannot smuggle in nonsense priorities
// or zero-length titles.
func TestNewOperatorPromptBeadDefaults(t *testing.T) {
	clamped := NewOperatorPromptBead("p", 99)
	assert.Equal(t, MaxPriority, clamped.Priority)

	negative := NewOperatorPromptBead("p", -3)
	assert.Equal(t, MinPriority, negative.Priority)

	empty := NewOperatorPromptBead("   \n   ", 2)
	assert.Equal(t, "(empty operator prompt)", empty.Title)
}

// TestStatusTransitionsForProposed locks down the documented state machine:
// proposed → open and proposed → cancelled are allowed; everything else
// out of proposed is rejected. Callers (UI approval flow) rely on this
// envelope to decide which buttons to enable.
func TestStatusTransitionsForProposed(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{StatusProposed, StatusOpen, true},
		{StatusProposed, StatusCancelled, true},
		{StatusProposed, StatusInProgress, false},
		{StatusProposed, StatusClosed, false},
		{StatusProposed, StatusBlocked, false},
		{StatusProposed, StatusProposed, false},
		{StatusOpen, StatusInProgress, true},
		{StatusInProgress, StatusClosed, true},
		{StatusClosed, StatusOpen, true},
		{StatusCancelled, StatusOpen, true},
		{"", StatusOpen, false},
		{StatusOpen, "", false},
		{"bogus", StatusOpen, false},
	}
	for _, tc := range cases {
		t.Run(tc.from+"->"+tc.to, func(t *testing.T) {
			assert.Equal(t, tc.want, IsValidStatusTransition(tc.from, tc.to))
		})
	}
}

// TestOperatorPromptMutationGuardMatrix covers the 2x2 actor/target matrix
// from Story 15 §Additional security controls: only an operator-prompt
// actor mutating an operator-prompt target is denied.
func TestOperatorPromptMutationGuardMatrix(t *testing.T) {
	cases := []struct {
		name        string
		actor       string
		target      string
		expectAllow bool
	}{
		{"no actor allows everything", "", IssueTypeOperatorPrompt, true},
		{"task actor → operator-prompt target allowed", "task", IssueTypeOperatorPrompt, true},
		{"operator-prompt actor → task target allowed", IssueTypeOperatorPrompt, "task", true},
		{"operator-prompt actor → empty target allowed", IssueTypeOperatorPrompt, "", true},
		{"operator-prompt actor → operator-prompt target denied", IssueTypeOperatorPrompt, IssueTypeOperatorPrompt, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := OperatorPromptMutationGuard(tc.actor, tc.target)
			if tc.expectAllow {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "operator-prompt")
		})
	}
}

// TestStoreCreateRejectsOperatorPromptSelfMutation verifies the no-self-mutation
// rule is enforced at bead-create time when the actor's issue_type is exposed
// via DDX_ACTOR_ISSUE_TYPE (set by the execute-bead harness). A task actor must
// be allowed to create operator-prompt beads; only operator-prompt → operator-prompt
// is denied.
func TestStoreCreateRejectsOperatorPromptSelfMutation(t *testing.T) {
	store := newTestStore(t)

	t.Setenv("DDX_ACTOR_ISSUE_TYPE", IssueTypeOperatorPrompt)
	denied := NewOperatorPromptBead("nested prompt", 2)
	err := store.Create(denied)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operator-prompt")

	t.Setenv("DDX_ACTOR_ISSUE_TYPE", "task")
	allowed := NewOperatorPromptBead("first prompt", 2)
	require.NoError(t, store.Create(allowed))

	t.Setenv("DDX_ACTOR_ISSUE_TYPE", "")
	human := NewOperatorPromptBead("human-submitted prompt", 2)
	require.NoError(t, store.Create(human))
}

// TestStoreRejectsInvalidStatus protects the validateBead enum boundary:
// arbitrary status strings must still be rejected even though the enum
// grew to include `proposed` and `cancelled`.
func TestStoreRejectsInvalidStatus(t *testing.T) {
	store := newTestStore(t)
	b := &Bead{Title: "bad status", IssueType: "task", Priority: 2, Status: "weird"}
	err := store.Create(b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}
