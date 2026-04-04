package bead

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBdSchemaCompatibility verifies that DDx bead JSONL output matches the
// bd (Dolt-backed bead tracker) schema exactly. This test locks in the field
// names so we never accidentally diverge from bd/br interchange format.
func TestBdSchemaCompatibility(t *testing.T) {
	// This is a real bd export line (from `bd export --format jsonl`)
	bdExport := `{"id":"bd-test-1x4","title":"test issue","status":"open","priority":2,"issue_type":"task","owner":"user@example.com","created_at":"2026-04-04T01:23:39Z","created_by":"Test User","updated_at":"2026-04-04T01:23:39Z","dependencies":[{"issue_id":"bd-test-1x4","depends_on_id":"bd-test-ioz","type":"blocks","created_at":"2026-04-03T21:23:54Z","created_by":"Test User","metadata":"{}"}],"dependency_count":1,"dependent_count":0,"comment_count":0}`

	// DDx must be able to unmarshal bd export without loss of known fields
	b, err := unmarshalBead([]byte(bdExport))
	require.NoError(t, err)

	assert.Equal(t, "bd-test-1x4", b.ID)
	assert.Equal(t, "test issue", b.Title)
	assert.Equal(t, "open", b.Status)
	assert.Equal(t, 2, b.Priority)
	assert.Equal(t, "task", b.IssueType)
	assert.Equal(t, "user@example.com", b.Owner)
	assert.Equal(t, "Test User", b.CreatedBy)
	assert.False(t, b.CreatedAt.IsZero())
	assert.False(t, b.UpdatedAt.IsZero())

	// Dependencies must be parsed
	require.Len(t, b.Dependencies, 1)
	assert.Equal(t, "bd-test-1x4", b.Dependencies[0].IssueID)
	assert.Equal(t, "bd-test-ioz", b.Dependencies[0].DependsOnID)
	assert.Equal(t, "blocks", b.Dependencies[0].Type)

	// bd-specific computed fields (dependency_count etc.) should be preserved in Extra
	assert.Equal(t, float64(1), b.Extra["dependency_count"])
	assert.Equal(t, float64(0), b.Extra["dependent_count"])
	assert.Equal(t, float64(0), b.Extra["comment_count"])
}

// TestBdRoundTrip verifies that a bd-exported bead survives DDx unmarshal→marshal
// without losing or renaming any fields.
func TestBdRoundTrip(t *testing.T) {
	bdExport := `{"id":"bd-test-abc","title":"Round trip","status":"open","priority":1,"issue_type":"epic","owner":"dev@co.com","created_at":"2026-01-01T00:00:00Z","created_by":"Dev","updated_at":"2026-02-01T00:00:00Z","custom_field":"preserved","dependency_count":0}`

	b, err := unmarshalBead([]byte(bdExport))
	require.NoError(t, err)

	// Marshal back
	out, err := marshalBead(b)
	require.NoError(t, err)

	// Parse the output and verify all original fields survive
	var result map[string]any
	require.NoError(t, json.Unmarshal(out, &result))

	// bd-compatible field names
	assert.Equal(t, "bd-test-abc", result["id"])
	assert.Equal(t, "Round trip", result["title"])
	assert.Equal(t, "open", result["status"])
	assert.Equal(t, float64(1), result["priority"])
	assert.Equal(t, "epic", result["issue_type"])
	assert.Equal(t, "dev@co.com", result["owner"])
	assert.Equal(t, "Dev", result["created_by"])

	// Custom fields preserved via Extra
	assert.Equal(t, "preserved", result["custom_field"])

	// Verify NO old DDx field names leak through
	assert.Nil(t, result["type"], "must use issue_type, not type")
	assert.Nil(t, result["assignee"], "must use owner, not assignee")
	assert.Nil(t, result["created"], "must use created_at, not created")
	assert.Nil(t, result["updated"], "must use updated_at, not updated")
	assert.Nil(t, result["deps"], "must use dependencies, not deps")
}

// TestDdxBeadFieldNames verifies that DDx-created beads use bd-compatible
// field names in their JSONL output.
func TestDdxBeadFieldNames(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "New bead", IssueType: "task"}
	require.NoError(t, s.Create(b))

	// Marshal and check field names
	out, err := marshalBead(*b)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(out, &result))

	// Must use bd-compatible names
	assert.Contains(t, result, "issue_type", "must use issue_type")
	assert.Contains(t, result, "created_at", "must use created_at")
	assert.Contains(t, result, "updated_at", "must use updated_at")

	// Must NOT use old DDx names
	assert.NotContains(t, result, "type", "must not use 'type' (use issue_type)")
	assert.NotContains(t, result, "assignee", "must not use 'assignee' (use owner)")
	assert.NotContains(t, result, "created", "must not use 'created' (use created_at)")
	assert.NotContains(t, result, "updated", "must not use 'updated' (use updated_at)")
	assert.NotContains(t, result, "deps", "must not use 'deps' (use dependencies)")
}
