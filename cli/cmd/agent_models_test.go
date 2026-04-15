package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentModelsSuccess(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"qwen3-32b", "qwen3-7b", "llama3-8b"})
	dir := makeProviderTestDir(t, oaiAgentConfig(srv.URL+"/v1", "qwen3-32b"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "models",
	)
	require.NoError(t, err)
	// Configured model is marked with *.
	require.Contains(t, out, "* qwen3-32b")
	require.Contains(t, out, "qwen3-7b")
	require.Contains(t, out, "llama3-8b")
}

func TestAgentModelsAutoSelect(t *testing.T) {
	// No model configured — the first ranked model should be marked with >.
	srv := newOAIModelsStub(t, []string{"alpha-model", "beta-model"})
	cfg := "providers:\n  testprovider:\n    type: openai-compat\n    base_url: " +
		srv.URL + "/v1\ndefault: testprovider\n"
	dir := makeProviderTestDir(t, cfg)

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "models",
	)
	require.NoError(t, err)
	// One of the models should be auto-selected (">").
	require.Contains(t, out, "> ")
}

func TestAgentModelsAnthropic(t *testing.T) {
	cfg := `providers:
  claude:
    type: anthropic
    api_key: fake-key
    model: claude-opus-4-5
default: claude
`
	dir := makeProviderTestDir(t, cfg)

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "models",
	)
	require.NoError(t, err)
	require.Contains(t, out, "Anthropic does not support model listing")
	require.Contains(t, out, "claude-opus-4-5")
}

func TestAgentModelsAll(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"local-model"})
	cfg := `providers:
  testprovider:
    type: openai-compat
    base_url: ` + srv.URL + `/v1
    model: local-model
  claudeprov:
    type: anthropic
    api_key: fake-key
default: testprovider
`
	dir := makeProviderTestDir(t, cfg)

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "models", "--all",
	)
	require.NoError(t, err)
	require.Contains(t, out, "[testprovider]")
	require.Contains(t, out, "[claudeprov]")
	require.Contains(t, out, "local-model")
}

func TestAgentModelsUnreachable(t *testing.T) {
	dead := newOAIModelsStub(t, nil)
	deadURL := dead.URL
	dead.Close()

	dir := makeProviderTestDir(t, oaiAgentConfig(deadURL+"/v1", "some-model"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "models",
	)
	require.NoError(t, err)
	require.Contains(t, out, "(unavailable)")
}

func TestAgentModelsUnknownProvider(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"a-model"})
	dir := makeProviderTestDir(t, oaiAgentConfig(srv.URL+"/v1", "a-model"))

	_, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "models", "--provider", "nonexistent",
	)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "unknown provider"))
}

func TestAgentModelsConfigError(t *testing.T) {
	dir := makeProviderTestDir(t, "providers: [\nbroken yaml{{")

	_, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "models",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "loading agent config")
}
