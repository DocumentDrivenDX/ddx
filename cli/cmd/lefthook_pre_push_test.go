package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadLefthookFullTestsSection reads lefthook.yml and returns the run content
// of the pre-push full-tests command (a window from "full-tests:" forward).
func loadLefthookFullTestsSection(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "lefthook.yml"))
	require.NoError(t, err)
	content := string(data)

	idx := strings.Index(content, "full-tests:")
	require.True(t, idx >= 0, "lefthook.yml: full-tests command not found under pre-push")
	end := idx + 2048
	if end > len(content) {
		end = len(content)
	}
	return content[idx:end]
}

// TestLefthookPrePushFullTestsSetsCGOForRace verifies the pre-push full-tests
// command explicitly sets CGO_ENABLED=1 so environments that inherit
// CGO_ENABLED=0 do not fail before any test runs.
func TestLefthookPrePushFullTestsSetsCGOForRace(t *testing.T) {
	section := loadLefthookFullTestsSection(t)
	assert.Contains(t, section, "CGO_ENABLED=1",
		"full-tests must set CGO_ENABLED=1 so -race works when env has CGO_ENABLED=0")
	assert.Contains(t, section, "go test -race",
		"full-tests must still invoke go test -race")
}

// TestLefthookPrePushFullTestsHasActionableCGOUnavailablePath verifies the
// full-tests hook has an explicit path for when cgo is genuinely unavailable,
// so operators receive an actionable error rather than a cryptic Go toolchain
// message.
func TestLefthookPrePushFullTestsHasActionableCGOUnavailablePath(t *testing.T) {
	section := loadLefthookFullTestsSection(t)
	hasCCompilerCheck := strings.Contains(section, "gcc") ||
		strings.Contains(section, "clang") ||
		strings.Contains(section, "command -v cc")
	assert.True(t, hasCCompilerCheck,
		"full-tests hook must check for a C compiler (gcc/clang/cc) before running -race")
	assert.Contains(t, section, "C compiler",
		"full-tests hook must emit an actionable 'C compiler' message when cgo is unavailable")
}
