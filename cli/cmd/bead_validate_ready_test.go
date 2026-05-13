package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeadValidateReady_EmptyQueueExitsClean(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	out, err := executeCommand(rootCmd, "bead", "validate-ready")
	require.NoError(t, err)
	assert.Contains(t, out, "No execution-ready beads.")
	// Close-with-evidence contract is documented even when the queue is empty.
	assert.Contains(t, out, "Close-with-evidence contract:")
}

func TestBeadValidateReady_AllMeasurableExitsClean(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	out, err := executeCommand(rootCmd, "bead", "create", "Wire telemetry",
		"--acceptance", "1. TestWireTelemetry passes\n2. cd cli && go test ./cmd/... green\n")
	require.NoError(t, err)
	require.NotEmpty(t, strings.TrimSpace(out))

	out, err = executeCommand(rootCmd, "bead", "validate-ready")
	require.NoError(t, err)
	assert.Contains(t, out, "PASS")
	assert.NotContains(t, out, "FAIL")
}

func TestBeadValidateReady_RejectsProseOnlyAcceptance(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	out, err := executeCommand(rootCmd, "bead", "create", "Improve clarity",
		"--acceptance", "1. improve clarity\n2. make it nicer\n3. handle edges well\n")
	require.NoError(t, err)
	require.NotEmpty(t, strings.TrimSpace(out))

	out, err = executeCommand(rootCmd, "bead", "validate-ready")
	require.Error(t, err, "validate-ready must fail when a ready bead has prose-only ACs")
	assert.Contains(t, err.Error(), "non-measurable acceptance")
	assert.Contains(t, out, "FAIL")
}

func TestBeadValidateReady_JSONIncludesCloseWithEvidence(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	_, err := executeCommand(rootCmd, "bead", "create", "Wire telemetry",
		"--acceptance", "1. TestWireTelemetry passes\n")
	require.NoError(t, err)

	out, err := executeCommand(rootCmd, "bead", "validate-ready", "--json")
	require.NoError(t, err)

	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.Equal(t, float64(1), report["total_ready"])
	assert.Equal(t, float64(0), report["failing_count"])

	cwe, ok := report["close_with_evidence"].(map[string]any)
	require.True(t, ok, "report must include close_with_evidence contract")
	assert.NotEmpty(t, cwe["summary"])
	reqs, ok := cwe["requirements"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, reqs)
	assert.NotEmpty(t, cwe["gate_behavior"])
}

func TestBeadValidateReady_HonorsExplicitThresholdOverride(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	// Bead with 1 verifiable + 1 prose AC -> score 0.5. Default threshold is
	// 0.5 so it passes. Raising to 0.75 must flip the verdict.
	_, err := executeCommand(rootCmd, "bead", "create", "Borderline bead",
		"--acceptance", "1. TestBorderline passes\n2. should feel good\n")
	require.NoError(t, err)

	// Default threshold (0.5) -> passes.
	_, err = executeCommand(rootCmd, "bead", "validate-ready")
	require.NoError(t, err)

	// Threshold override (0.75) -> fails.
	_, err = executeCommand(rootCmd, "bead", "validate-ready", "--threshold", "0.75")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-measurable acceptance")
}

func TestBeadValidateReady_SelectionMatchesReadyExecution(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	// Three beads: one execution-ready with prose-only ACs (should appear and
	// fail), one closed (must NOT appear), one in-progress / proposed.
	_, err := executeCommand(rootCmd, "bead", "create", "Open prose-only",
		"--acceptance", "1. make it better\n2. clean things up\n")
	require.NoError(t, err)

	createOut, err := executeCommand(rootCmd, "bead", "create", "Closed bead",
		"--acceptance", "1. TestClosed passes\n")
	require.NoError(t, err)
	closedID := strings.TrimSpace(createOut)
	_, err = executeCommand(rootCmd, "bead", "close", closedID)
	require.NoError(t, err)

	out, err := executeCommand(rootCmd, "bead", "validate-ready", "--json")
	require.Error(t, err)

	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	// Closed bead must not be in the set: only the open prose-only bead is.
	assert.Equal(t, float64(1), report["total_ready"])
	entries, ok := report["entries"].([]any)
	require.True(t, ok)
	require.Len(t, entries, 1)
	entry := entries[0].(map[string]any)
	assert.NotEqual(t, closedID, entry["id"])
	assert.Equal(t, false, entry["passes_threshold"])
}
