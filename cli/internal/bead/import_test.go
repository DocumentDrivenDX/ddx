package bead

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportJSONL(t *testing.T) {
	s := newTestStore(t)

	// Create a JSONL file to import
	importFile := filepath.Join(t.TempDir(), "import.jsonl")
	jsonl := `{"id":"bx-aaaa0001","title":"Imported A","type":"task","status":"open","priority":2,"labels":[],"deps":[],"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}
{"id":"bx-aaaa0002","title":"Imported B","type":"bug","status":"open","priority":1,"labels":["backend"],"deps":[],"created":"2026-01-02T00:00:00Z","updated":"2026-01-02T00:00:00Z"}`
	require.NoError(t, os.WriteFile(importFile, []byte(jsonl), 0o644))

	n, err := s.Import("jsonl", importFile)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	beads, err := s.ReadAll(testCtx())
	require.NoError(t, err)
	assert.Len(t, beads, 2)
}

func TestImportPreservesAllLifecycleStatuses(t *testing.T) {
	s := newTestStore(t)

	importFile := filepath.Join(t.TempDir(), "import.jsonl")
	var lines []string
	for i, status := range CanonicalStatuses {
		lines = append(lines, fmt.Sprintf(`{"id":"bx-status%02d","title":"%s status","type":"task","status":"%s","priority":2,"labels":[],"deps":[],"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}`, i, status, status))
	}
	require.NoError(t, os.WriteFile(importFile, []byte(strings.Join(lines, "\n")), 0o644))

	n, err := s.Import("jsonl", importFile)
	require.NoError(t, err)
	assert.Equal(t, len(CanonicalStatuses), n)

	for i, status := range CanonicalStatuses {
		got, err := s.Get(testCtx(), fmt.Sprintf("bx-status%02d", i))
		require.NoError(t, err)
		assert.Equal(t, status, got.Status)
	}
}

func TestImportRejectsLegacyPseudoStatusesOutsideLifecycleMigrator(t *testing.T) {
	for _, pseudoStatus := range []string{"needs_human", "needs_investigation"} {
		t.Run(pseudoStatus, func(t *testing.T) {
			s := newTestStore(t)

			importFile := filepath.Join(t.TempDir(), "import.jsonl")
			jsonl := fmt.Sprintf(`{"id":"bx-legacy01","title":"Legacy status","type":"task","status":"%s","priority":2,"labels":[],"deps":[],"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}`, pseudoStatus)
			require.NoError(t, os.WriteFile(importFile, []byte(jsonl), 0o644))

			n, err := s.Import("jsonl", importFile)
			require.Error(t, err)
			assert.Equal(t, 0, n)
			assert.Contains(t, err.Error(), pseudoStatus)
			assert.Contains(t, err.Error(), "ddx bead migrate --lifecycle")

			beads, readErr := s.ReadAll(testCtx())
			require.NoError(t, readErr)
			assert.Empty(t, beads)
		})
	}
}

func TestImportSkipsDuplicates(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "Existing"}
	require.NoError(t, s.Create(testCtx(), b))

	// Import file with same ID
	importFile := filepath.Join(t.TempDir(), "import.jsonl")
	jsonl := `{"id":"` + b.ID + `","title":"Duplicate","type":"task","status":"open","priority":2,"labels":[],"deps":[],"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}
{"id":"bx-new00001","title":"New one","type":"task","status":"open","priority":2,"labels":[],"deps":[],"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(importFile, []byte(jsonl), 0o644))

	n, err := s.Import("jsonl", importFile)
	require.NoError(t, err)
	assert.Equal(t, 1, n) // only the new one

	beads, err := s.ReadAll(testCtx())
	require.NoError(t, err)
	assert.Len(t, beads, 2)
}

func TestImportJSONArray(t *testing.T) {
	s := newTestStore(t)

	importFile := filepath.Join(t.TempDir(), "import.json")
	jsonArr := `[{"id":"bx-arr00001","title":"From array","type":"task","status":"open","priority":2,"labels":[],"deps":[],"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}]`
	require.NoError(t, os.WriteFile(importFile, []byte(jsonArr), 0o644))

	n, err := s.Import("jsonl", importFile)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestImportPreservesUnknownFields(t *testing.T) {
	s := newTestStore(t)

	importFile := filepath.Join(t.TempDir(), "import.jsonl")
	jsonl := `{"id":"hx-helix001","title":"HELIX issue","type":"task","status":"open","priority":1,"labels":["helix","phase:build"],"deps":[],"spec-id":"FEAT-001","execution-eligible":true,"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(importFile, []byte(jsonl), 0o644))

	n, err := s.Import("jsonl", importFile)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := s.Get(testCtx(), "hx-helix001")
	require.NoError(t, err)
	assert.Equal(t, "FEAT-001", got.Extra["spec-id"])
	assert.Equal(t, true, got.Extra["execution-eligible"])
}

func TestExportRoundTrip(t *testing.T) {
	s := newTestStore(t)

	require.NoError(t, s.Create(testCtx(), &Bead{Title: "Task A", Labels: []string{"backend"}}))
	require.NoError(t, s.Create(testCtx(), &Bead{Title: "Task B", IssueType: "bug", Priority: 0}))

	// Export
	exportFile := filepath.Join(t.TempDir(), "export.jsonl")
	require.NoError(t, s.ExportToFile(exportFile))

	// Import into fresh store
	s2 := newTestStore(t)
	n, err := s2.Import("jsonl", exportFile)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	beads, err := s2.ReadAll(testCtx())
	require.NoError(t, err)
	assert.Len(t, beads, 2)
}

func TestImportNonexistentFile(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Import("jsonl", "/nonexistent/file.jsonl")
	assert.Error(t, err)
}
