package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadLefthookPreCommitSection(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "lefthook.yml"))
	require.NoError(t, err)
	content := string(data)

	idx := strings.Index(content, "pre-commit:")
	require.True(t, idx >= 0, "lefthook.yml: pre-commit block not found")
	end := strings.Index(content[idx:], "\nci:")
	require.True(t, end >= 0, "lefthook.yml: ci block not found after pre-commit")
	return content[idx : idx+end]
}

func TestLefthookPreCommitIncludesDDxRootAudit(t *testing.T) {
	section := loadLefthookPreCommitSection(t)

	assert.Contains(t, section, "ddxroot-path-audit:")
	assert.Contains(t, section, `filepath\.Join\([^)]*"\.ddx"`)
	assert.Contains(t, section, `filepath\.Join\(\s*(projectRoot|[A-Za-z0-9_\.]*WorkingDir)\s*,\s*[A-Za-z_][A-Za-z0-9_]*RelPath\s*\)`)
	assert.Contains(t, section, `(projectRoot|[A-Za-z0-9_\.]*WorkingDir)\s*\+\s*"/\.ddx(?:/|")`)
	assert.Contains(t, section, `grep -Ev '(^cli/internal/ddxroot/)|(_test\.go$)'`)
	assert.Contains(t, section, "Hardcoded DDx state-root callsites found outside cli/internal/ddxroot/ and approved test fixtures")
}
