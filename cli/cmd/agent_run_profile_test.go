package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agentTestDir creates a temp dir with a minimal DDx config (no harness set, so
// routing picks from all registered candidates).
// HOME is redirected to a clean temp dir to prevent the global ~/.config/agent/config.yaml
// from being loaded (which would cause tests to attempt real network connections).
func agentTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	cfg := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644))
	return dir
}

// agentTestDirWithHarness creates a temp dir with a DDx config specifying a harness.
func agentTestDirWithHarness(t *testing.T, harness string) string {
	t.Helper()
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	cfg := "version: \"1.0\"\nlibrary:\n  path: \".ddx/plugins/ddx\"\n  repository:\n    url: \"https://example.com/lib\"\n    branch: \"main\"\nagent:\n  harness: " + harness + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644))
	return dir
}

// TestAgentRunProfileFlagWithVirtualHarness verifies that --profile is accepted as a
// valid flag and does not interfere with an explicit --harness virtual invocation.
func TestAgentRunProfileFlagWithVirtualHarness(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	// Inject a virtual response for "hello"
	t.Setenv("DDX_VIRTUAL_RESPONSES", `[{"prompt_match":"hello","response":"hi from virtual"}]`)

	dir := agentTestDirWithHarness(t, "virtual")
	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "run",
		"--harness", "virtual",
		"--profile", "cheap",
		"--text", "hello",
	)
	require.NoError(t, err, "profile flag should be accepted alongside explicit --harness")
	assert.Contains(t, output, "hi from virtual")
}

// TestAgentRunProfileFlagRecognized verifies that --profile is a recognized flag
// (flag parsing does not fail with "unknown flag: --profile").
func TestAgentRunProfileFlagRecognized(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := agentTestDir(t)
	rootCmd := NewCommandFactory(dir).NewRootCommand()

	// The run will fail (agent provider not configured), but flag parsing must succeed.
	// A "unknown flag" error means the flag was not registered.
	// --timeout 3s prevents exponential-backoff retries from hanging the test.
	_, err := executeCommand(rootCmd, "agent", "run", "--profile", "cheap", "--text", "test", "--timeout", "3s")
	if err != nil {
		assert.NotContains(t, err.Error(), "unknown flag", "--profile must be a registered flag")
		assert.NotContains(t, err.Error(), "no harness available", "routing should find agent (always available)")
	}
}

// TestAgentRunProfileRoutingSelectsViableHarness verifies that --profile cheap routes
// to a viable harness (not a routing failure) when no --harness is specified.
// The agent harness is always available (embedded), so cheap routing picks it.
// We confirm routing succeeded by checking the error is from agent internals, not routing.
func TestAgentRunProfileRoutingSelectsViableHarness(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := agentTestDir(t)
	rootCmd := NewCommandFactory(dir).NewRootCommand()

	_, err := executeCommand(rootCmd, "agent", "run", "--profile", "cheap", "--text", "test", "--timeout", "3s")
	// Expect either success or an agent-provider error (not a routing or flag error).
	if err != nil {
		msg := err.Error()
		assert.NotContains(t, msg, "unknown flag", "flag must be recognized")
		assert.NotContains(t, msg, "unknown harness", "routing must resolve a harness")
		assert.NotContains(t, msg, "no harness available", "a viable harness must exist")
		// Error should be from agent runtime, not from routing layer.
		isAgentErr := strings.Contains(msg, "agent:") ||
			strings.Contains(msg, "provider") ||
			strings.Contains(msg, "timeout") ||
			strings.Contains(msg, "exited with code")
		assert.True(t, isAgentErr, "error should be from agent runtime after routing, got: %s", msg)
	}
}

// TestAgentListShowsEmbeddedAgentHarness verifies that ddx agent list includes the
// embedded 'agent' harness as available (it requires no external binary).
func TestAgentListShowsEmbeddedAgentHarness(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := agentTestDir(t)
	rootCmd := NewCommandFactory(dir).NewRootCommand()

	output, err := executeCommand(rootCmd, "agent", "list")
	require.NoError(t, err)
	assert.Contains(t, output, "agent", "agent harness must appear in list")
	assert.Contains(t, output, "ok", "agent harness must be available (embedded)")
}

// TestAgentListEmbeddedAgentHarnessJSON verifies the JSON output of agent list
// includes the 'agent' harness marked as available.
func TestAgentListEmbeddedAgentHarnessJSON(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := agentTestDir(t)
	rootCmd := NewCommandFactory(dir).NewRootCommand()

	output, err := executeCommand(rootCmd, "agent", "list", "--json")
	require.NoError(t, err)

	var statuses []struct {
		Name      string `json:"name"`
		Available bool   `json:"available"`
		Binary    string `json:"binary"`
		Path      string `json:"path"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &statuses))

	found := false
	for _, s := range statuses {
		if s.Name == "agent" {
			found = true
			assert.True(t, s.Available, "agent harness must be available (embedded)")
			assert.Equal(t, "(embedded)", s.Path, "agent harness path must be '(embedded)'")
		}
	}
	assert.True(t, found, "agent harness must be in list")
}

// TestAgentCapabilitiesEmbeddedAgent verifies that ddx agent capabilities agent
// works and reports the embedded agent harness consistently.
func TestAgentCapabilitiesEmbeddedAgent(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := agentTestDir(t)
	rootCmd := NewCommandFactory(dir).NewRootCommand()

	output, err := executeCommand(rootCmd, "agent", "capabilities", "agent")
	require.NoError(t, err)
	assert.Contains(t, output, "Harness: agent")
	assert.Contains(t, output, "Binary: ddx-agent")
	assert.Contains(t, output, "harness: agent", "config example should show harness: agent")
}

// TestAgentCapabilitiesEmbeddedAgentJSON verifies JSON capabilities for the 'agent' harness.
func TestAgentCapabilitiesEmbeddedAgentJSON(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := agentTestDir(t)
	rootCmd := NewCommandFactory(dir).NewRootCommand()

	output, err := executeCommand(rootCmd, "agent", "capabilities", "--harness", "agent", "--json")
	require.NoError(t, err)

	var caps struct {
		Harness   string `json:"harness"`
		Available bool   `json:"available"`
		Binary    string `json:"binary"`
		IsLocal   bool   `json:"is_local"`
		CostClass string `json:"cost_class"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &caps))
	assert.Equal(t, "agent", caps.Harness)
	assert.True(t, caps.Available)
	assert.Equal(t, "ddx-agent", caps.Binary)
	assert.True(t, caps.IsLocal, "agent harness must be local (embedded)")
	assert.Equal(t, "local", caps.CostClass)
}

// TestAgentRunProfileNoViableHarness verifies that when --profile is given but
// no harness can satisfy the request, the command returns the routing-failure
// error containing "no viable harness found for profile".
//
// Strategy: pass --effort to eliminate the embedded harnesses (agent, virtual)
// which are always installed but do not advertise EffortFlag. An empty PATH
// ensures no external harnesses (codex, claude, etc.) are found either.
func TestAgentRunProfileNoViableHarness(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	// Remove all external harnesses from PATH; embedded harnesses (agent, virtual)
	// are always available but do not support --effort.
	emptyBinDir := t.TempDir()
	t.Setenv("PATH", emptyBinDir)

	dir := agentTestDir(t)
	rootCmd := NewCommandFactory(dir).NewRootCommand()

	_, err := executeCommand(rootCmd, "agent", "run",
		"--profile", "smart",
		"--effort", "high",
		"--text", "test",
	)
	require.Error(t, err, "should fail when no harness can satisfy the profile+effort request")
	// CONTRACT-003 / ddx-da19756a: execution paths pass Profile directly to
	// Execute (no ResolveRoute pre-flight). The upstream service returns a
	// provider-selection error when no live endpoint is available for the
	// requested profile.
	assert.Contains(t, err.Error(), "no selected provider candidate",
		"error must identify the routing failure cause")
}

// TestAgentRunHarnessAgentAccepted verifies that --harness agent is accepted as
// a valid harness name (the stable embedded DDx agent alias).
func TestAgentRunHarnessAgentAccepted(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := agentTestDir(t)
	rootCmd := NewCommandFactory(dir).NewRootCommand()

	_, err := executeCommand(rootCmd, "agent", "run", "--harness", "agent", "--text", "test")
	// Must not fail with "unknown harness" — the harness is registered.
	if err != nil {
		assert.NotContains(t, err.Error(), "unknown harness: agent",
			"'agent' must be a recognized harness name")
	}
}

// TestAgentRunOutputText verifies that --output text emits only the final
// assistant text (no JSONL wrapping).
func TestAgentRunOutputText(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_VIRTUAL_RESPONSES", `[{"prompt_match":"hello","response":"plain text response"}]`)

	dir := agentTestDirWithHarness(t, "virtual")
	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "run",
		"--harness", "virtual",
		"--text", "hello",
		"--output", "text",
	)
	require.NoError(t, err)
	assert.Equal(t, "plain text response", strings.TrimSpace(output))
}

// TestAgentRunOutputTextIsDefault verifies that the default output mode is text.
func TestAgentRunOutputTextIsDefault(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_VIRTUAL_RESPONSES", `[{"prompt_match":"hello","response":"default output"}]`)

	dir := agentTestDirWithHarness(t, "virtual")
	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "run",
		"--harness", "virtual",
		"--text", "hello",
	)
	require.NoError(t, err)
	assert.Equal(t, "default output", strings.TrimSpace(output))
}

// TestAgentRunOutputJSONResult verifies that --output json-result emits the
// result struct as JSON including harness and output fields.
func TestAgentRunOutputJSONResult(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_VIRTUAL_RESPONSES", `[{"prompt_match":"hello","response":"the answer"}]`)

	dir := agentTestDirWithHarness(t, "virtual")
	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "run",
		"--harness", "virtual",
		"--text", "hello",
		"--output", "json-result",
	)
	require.NoError(t, err)

	var result struct {
		Harness  string `json:"harness"`
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &result), "output must be valid JSON")
	assert.Equal(t, "virtual", result.Harness)
	assert.Equal(t, "the answer", result.Output)
	assert.Equal(t, 0, result.ExitCode)
}

// TestAgentRunOutputJSONFlag verifies that --json is a backward-compatible
// alias for --output json-result.
func TestAgentRunOutputJSONFlag(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_VIRTUAL_RESPONSES", `[{"prompt_match":"hello","response":"json alias"}]`)

	dir := agentTestDirWithHarness(t, "virtual")
	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "run",
		"--harness", "virtual",
		"--text", "hello",
		"--json",
	)
	require.NoError(t, err)

	var result struct {
		Harness string `json:"harness"`
		Output  string `json:"output"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	assert.Equal(t, "virtual", result.Harness)
	assert.Equal(t, "json alias", result.Output)
}

// TestAgentRunOutputSessionJSONL verifies that --output session-jsonl emits
// the raw harness output (current/legacy behavior).
func TestAgentRunOutputSessionJSONL(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_VIRTUAL_RESPONSES", `[{"prompt_match":"hello","response":"raw session output"}]`)

	dir := agentTestDirWithHarness(t, "virtual")
	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "run",
		"--harness", "virtual",
		"--text", "hello",
		"--output", "session-jsonl",
	)
	require.NoError(t, err)
	assert.Equal(t, "raw session output", strings.TrimSpace(output))
}

// TestAgentRunOutputInvalidValue verifies that an unknown --output value
// returns a descriptive error.
func TestAgentRunOutputInvalidValue(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_VIRTUAL_RESPONSES", `[{"prompt_match":"hello","response":"x"}]`)

	dir := agentTestDirWithHarness(t, "virtual")
	rootCmd := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(rootCmd, "agent", "run",
		"--harness", "virtual",
		"--text", "hello",
		"--output", "bogus",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
	assert.Contains(t, err.Error(), "valid:")
}
