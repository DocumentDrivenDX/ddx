package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newOAIModelsStub starts a test HTTP server that responds to /v1/models
// with the given model IDs. The server is closed when the test ends.
func newOAIModelsStub(t *testing.T, modelIDs []string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		type modelEntry struct {
			ID string `json:"id"`
		}
		type resp struct {
			Data []modelEntry `json:"data"`
		}
		var body resp
		for _, id := range modelIDs {
			body.Data = append(body.Data, modelEntry{ID: id})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// makeProviderTestDir creates a temp directory wired up for provider command
// tests. It writes an .agent/config.yaml with agentCfgYAML and a minimal
// .ddx/config.yaml, then isolates the test from the global agent config and
// any AGENT_* environment variables.
func makeProviderTestDir(t *testing.T, agentCfgYAML string) string {
	t.Helper()
	dir := t.TempDir()

	// Isolate from ~/.config/agent/config.yaml.
	t.Setenv("HOME", dir)

	// Clear AGENT_* env overrides so the real environment cannot override
	// the test provider config.
	t.Setenv("AGENT_PROVIDER", "")
	t.Setenv("AGENT_BASE_URL", "")
	t.Setenv("AGENT_API_KEY", "")
	t.Setenv("AGENT_MODEL", "")

	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	agentDir := filepath.Join(dir, ".agent")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(agentDir, "config.yaml"),
		[]byte(agentCfgYAML),
		0o644,
	))

	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(ddxDir, "config.yaml"),
		[]byte(`version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`),
		0o644,
	))

	return dir
}

// oaiAgentConfig returns .agent/config.yaml YAML for a single openai-compat
// provider pointing at baseURL (e.g. "http://127.0.0.1:PORT/v1").
func oaiAgentConfig(baseURL, model string) string {
	return "providers:\n  testprovider:\n    type: lmstudio\n    base_url: " +
		baseURL + "\n    model: " + model + "\ndefault: testprovider\n"
}

func TestAgentProvidersSuccess(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"alpha-model", "beta-model"})
	dir := makeProviderTestDir(t, oaiAgentConfig(srv.URL+"/v1", "alpha-model"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "providers",
	)
	require.NoError(t, err)
	require.Contains(t, out, "testprovider")
	require.Contains(t, out, "connected")
}

func TestAgentProvidersJSON(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"alpha-model", "beta-model"})
	dir := makeProviderTestDir(t, oaiAgentConfig(srv.URL+"/v1", "alpha-model"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "providers", "--json",
	)
	require.NoError(t, err)

	var entries []providerStatusEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	require.Len(t, entries, 1)
	require.Equal(t, "testprovider", entries[0].Name)
	require.Equal(t, "lmstudio", entries[0].Type)
	require.True(t, entries[0].Default)
	require.True(t, strings.Contains(entries[0].Status, "connected"))
}

func TestAgentProvidersUnreachable(t *testing.T) {
	// Start then immediately close a server to get a URL that will refuse connections.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	dir := makeProviderTestDir(t, oaiAgentConfig(deadURL+"/v1", "some-model"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "providers",
	)
	require.NoError(t, err)
	require.Contains(t, out, "testprovider")
	// Provider is unreachable — output must not say "connected".
	require.False(t, strings.Contains(out, "connected"), "expected no 'connected' status for closed server")
}

func TestAgentProvidersUnreachableJSON(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	dir := makeProviderTestDir(t, oaiAgentConfig(deadURL+"/v1", "some-model"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "providers", "--json",
	)
	require.NoError(t, err)

	var entries []providerStatusEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	require.Len(t, entries, 1)
	require.False(t, strings.Contains(entries[0].Status, "connected"))
}

func TestAgentProvidersConfigError(t *testing.T) {
	// Write deliberately invalid YAML to .agent/config.yaml.
	dir := makeProviderTestDir(t, "providers: [\ninvalid yaml{{{{")

	_, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "providers",
	)
	require.Error(t, err)
	// After CONTRACT-003 migration the error wrapper changed from
	// "loading agent config" to "constructing agent service" (the agentlib.New
	// constructor wraps the underlying agentconfig.Load error).
	require.Contains(t, err.Error(), "constructing agent service")
}

func TestAgentProvidersAnthropicWithKey(t *testing.T) {
	cfg := `providers:
  claude:
    type: anthropic
    api_key: test-key-abc
default: claude
`
	dir := makeProviderTestDir(t, cfg)

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "providers",
	)
	require.NoError(t, err)
	require.Contains(t, out, "claude")
	// After CONTRACT-003 migration the anthropic probe returns "connected"
	// when an API key is present (no /v1/models endpoint to probe; key-presence
	// is the connectivity signal). Old wording was "api key configured".
	require.Contains(t, out, "connected")
}
