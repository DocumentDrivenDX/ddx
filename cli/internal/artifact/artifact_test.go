package artifact

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestADRCreate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/adr")
	m := NewADRManager(dir)

	path, err := m.Create(TypeADR, "Use PostgreSQL for persistence", nil)
	require.NoError(t, err)
	assert.Contains(t, path, "ADR-001-use-postgresql-for-persistence.md")

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "id: ADR-001")
	assert.Contains(t, content, "# ADR-001: Use PostgreSQL for persistence")
	assert.Contains(t, content, "## Context")
	assert.Contains(t, content, "## Decision")
	assert.Contains(t, content, "## Alternatives")
	assert.Contains(t, content, "## Consequences")
	assert.Contains(t, content, "## Risks")
}

func TestADRCreateWithDeps(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/adr")
	m := NewADRManager(dir)

	path, err := m.Create(TypeADR, "Auth strategy", []string{"FEAT-001", "ADR-001"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "depends_on: [FEAT-001, ADR-001]")
}

func TestADRSequentialIDs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/adr")
	m := NewADRManager(dir)

	p1, err := m.Create(TypeADR, "First decision", nil)
	require.NoError(t, err)
	assert.Contains(t, filepath.Base(p1), "ADR-001")

	p2, err := m.Create(TypeADR, "Second decision", nil)
	require.NoError(t, err)
	assert.Contains(t, filepath.Base(p2), "ADR-002")

	p3, err := m.Create(TypeADR, "Third decision", nil)
	require.NoError(t, err)
	assert.Contains(t, filepath.Base(p3), "ADR-003")
}

func TestADRIDAfterGap(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/adr")
	m := NewADRManager(dir)

	// Create ADR-001 and ADR-003 (gap at 002)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	os.WriteFile(filepath.Join(dir, "ADR-001-first.md"), []byte("---\ndun:\n  id: ADR-001\n  depends_on: []\n---\n# ADR-001: First\n## Context\n## Decision\n## Alternatives\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "ADR-003-third.md"), []byte("---\ndun:\n  id: ADR-003\n  depends_on: []\n---\n# ADR-003: Third\n## Context\n## Decision\n## Alternatives\n"), 0o644)

	// Next should be ADR-004 (after max, not fill gap)
	p, err := m.Create(TypeADR, "Fourth", nil)
	require.NoError(t, err)
	assert.Contains(t, filepath.Base(p), "ADR-004")
}

func TestSDCreate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/designs")
	m := NewSDManager(dir)

	path, err := m.Create(TypeSD, "User authentication flow", []string{"FEAT-002"})
	require.NoError(t, err)
	assert.Contains(t, path, "SD-001-user-authentication-flow.md")

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "id: SD-001")
	assert.Contains(t, content, "# SD-001: User authentication flow")
	assert.Contains(t, content, "## Scope")
	assert.Contains(t, content, "## Acceptance Criteria")
	assert.Contains(t, content, "## Solution Approaches")
	assert.Contains(t, content, "depends_on: [FEAT-002]")
}

func TestADRList(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/adr")
	m := NewADRManager(dir)

	m.Create(TypeADR, "First decision", nil)
	m.Create(TypeADR, "Second decision", nil)

	infos, err := m.List(TypeADR)
	require.NoError(t, err)
	assert.Len(t, infos, 2)
	assert.Equal(t, "ADR-001", infos[0].ID)
	assert.Equal(t, "First decision", infos[0].Title)
	assert.Equal(t, "ADR-002", infos[1].ID)
}

func TestSDList(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/designs")
	m := NewSDManager(dir)

	m.Create(TypeSD, "Auth flow", nil)
	m.Create(TypeSD, "Data pipeline", nil)

	infos, err := m.List(TypeSD)
	require.NoError(t, err)
	assert.Len(t, infos, 2)
	assert.Equal(t, "SD-001", infos[0].ID)
}

func TestADRShow(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/adr")
	m := NewADRManager(dir)

	m.Create(TypeADR, "Test decision", nil)

	content, path, err := m.Show(TypeADR, "ADR-001")
	require.NoError(t, err)
	assert.Contains(t, content, "ADR-001")
	assert.Contains(t, path, "ADR-001")
}

func TestShowNotFound(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/adr")
	m := NewADRManager(dir)

	_, _, err := m.Show(TypeADR, "ADR-999")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestADRValidate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/adr")
	m := NewADRManager(dir)

	// Valid ADR
	path, _ := m.Create(TypeADR, "Valid ADR", nil)
	errs := m.Validate(TypeADR, path)
	assert.Empty(t, errs)
}

func TestADRValidateMissingSection(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/adr")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	// ADR missing Decision section
	path := filepath.Join(dir, "ADR-001-bad.md")
	content := "---\ndun:\n  id: ADR-001\n  depends_on: []\n---\n# ADR-001: Bad\n\n## Context\nSome context\n"
	os.WriteFile(path, []byte(content), 0o644)

	m := NewADRManager(dir)
	errs := m.Validate(TypeADR, path)
	assert.NotEmpty(t, errs)

	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Message)
	}
	assert.Contains(t, msgs, "missing required section: Decision")
	assert.Contains(t, msgs, "missing required section: Alternatives")
}

func TestADRValidateMissingFrontmatter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/adr")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	path := filepath.Join(dir, "ADR-001-no-fm.md")
	os.WriteFile(path, []byte("# ADR-001: No frontmatter\n\n## Context\n## Decision\n## Alternatives\n"), 0o644)

	m := NewADRManager(dir)
	errs := m.Validate(TypeADR, path)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "frontmatter")
}

func TestSDValidate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/designs")
	m := NewSDManager(dir)

	path, _ := m.Create(TypeSD, "Valid SD", nil)
	errs := m.Validate(TypeSD, path)
	assert.Empty(t, errs)
}

func TestSDValidateMissingSection(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/designs")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	path := filepath.Join(dir, "SD-001-bad.md")
	content := "---\ndun:\n  id: SD-001\n  depends_on: []\n---\n# SD-001: Bad\n\n## Scope\nSome scope\n"
	os.WriteFile(path, []byte(content), 0o644)

	m := NewSDManager(dir)
	errs := m.Validate(TypeSD, path)
	assert.Len(t, errs, 2) // missing Acceptance Criteria and Solution Approaches
}

func TestCreateEmptyTitle(t *testing.T) {
	m := NewADRManager(t.TempDir())
	_, err := m.Create(TypeADR, "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
}

func TestValidateAll(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/adr")
	m := NewADRManager(dir)

	m.Create(TypeADR, "Good ADR", nil)

	// Add a bad one
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "ADR-002-bad.md"),
		[]byte("---\ndun:\n  id: ADR-002\n  depends_on: []\n---\n# ADR-002: Bad\n\n## Context\n"),
		0o644,
	))

	errs := m.ValidateAll(TypeADR)
	assert.NotEmpty(t, errs) // Bad ADR should have errors
}

func TestListEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs/adr")
	m := NewADRManager(dir)

	infos, err := m.List(TypeADR)
	require.NoError(t, err)
	assert.Nil(t, infos)
}
