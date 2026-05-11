package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeadLint_CLI_PrintsClassification(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	// Create a bead with mixed AC text: 1 test-name (verifiable) + 2 prose.
	out, err := executeCommand(rootCmd, "bead", "create", "Lint test bead",
		"--acceptance", "1. TestFooBar passes\n2. improve clarity\n3. better error messages\n")
	require.NoError(t, err)
	beadID := strings.TrimSpace(out)
	require.NotEmpty(t, beadID)

	// Human-readable output must include kind labels.
	out, err = executeCommand(rootCmd, "bead", "lint", beadID)
	require.NoError(t, err)
	assert.Contains(t, out, "test-name")
	assert.Contains(t, out, "prose")
	assert.Contains(t, out, "bead lint for "+beadID)

	// JSON output must be parseable and include per-AC items.
	out, err = executeCommand(rootCmd, "bead", "lint", beadID, "--json")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &result), "JSON output must be valid: %s", out)
	assert.Equal(t, float64(3), result["total"])
	assert.Equal(t, float64(1), result["verifiable_count"])
	assert.Equal(t, float64(2), result["prose_count"])
	items, ok := result["items"].([]any)
	require.True(t, ok, "items must be a JSON array")
	assert.Len(t, items, 3)
}
