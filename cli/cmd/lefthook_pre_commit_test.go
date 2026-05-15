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
	assert.Contains(t, section, `rg 'filepath\.Join\([^)]*"\.ddx"' cli/ --type go | grep -v '^cli/internal/ddxroot/'`)
	assert.Contains(t, section, "Hardcoded .ddx filepath.Join callsites found outside cli/internal/ddxroot/")
}
