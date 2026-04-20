package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentCheckSuccess(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"fast-model", "slow-model"})
	dir := makeProviderTestDir(t, oaiAgentConfig(srv.URL+"/v1", "fast-model"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "check",
	)
	require.NoError(t, err)
	require.Contains(t, out, "testprovider")
	require.Contains(t, out, "OK")
}

func TestAgentCheckAnthropicWithKey(t *testing.T) {
	cfg := `providers:
  claude:
    type: anthropic
    api_key: test-api-key-xyz
default: claude
`
	dir := makeProviderTestDir(t, cfg)

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "check",
	)
	require.NoError(t, err)
	require.Contains(t, out, "claude")
	require.Contains(t, out, "OK")
}

// TestAgentCheckMixedReachability verifies that when two providers are
// configured and one is unreachable, the command prints UNREACHABLE for the
// dead provider but exits without error because at least one provider is OK.
// Using two providers avoids triggering os.Exit(1).
func TestAgentCheckMixedReachability(t *testing.T) {
	live := newOAIModelsStub(t, []string{"good-model"})
	dead := newOAIModelsStub(t, nil)
	deadURL := dead.URL
	dead.Close()

	cfg := `providers:
  liveprov:
    type: lmstudio
    base_url: ` + live.URL + `/v1
    model: good-model
  deadprov:
    type: lmstudio
    base_url: ` + deadURL + `/v1
    model: bad-model
default: liveprov
`
	dir := makeProviderTestDir(t, cfg)

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "check",
	)
	// At least one reachable → no os.Exit(1) → command returns nil.
	require.NoError(t, err)
	require.Contains(t, out, "liveprov")
	require.Contains(t, out, "OK")
	require.Contains(t, out, "deadprov")
	require.Contains(t, out, "UNREACHABLE")
}

func TestAgentCheckSingleProviderFlag(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"model-a", "model-b"})
	cfg := `providers:
  prov1:
    type: lmstudio
    base_url: ` + srv.URL + `/v1
    model: model-a
  prov2:
    type: lmstudio
    base_url: ` + srv.URL + `/v1
    model: model-b
default: prov1
`
	dir := makeProviderTestDir(t, cfg)

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "check", "--provider", "prov2",
	)
	require.NoError(t, err)
	require.Contains(t, out, "prov2")
	require.Contains(t, out, "OK")
	// prov1 should not appear (filtered to prov2 only).
	require.False(t, strings.Contains(out, "prov1"), "prov1 should be excluded by --provider flag")
}

func TestAgentCheckUnknownProvider(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"model-x"})
	dir := makeProviderTestDir(t, oaiAgentConfig(srv.URL+"/v1", "model-x"))

	_, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "check", "--provider", "does-not-exist",
	)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "unknown provider"))
}

func TestAgentCheckConfigError(t *testing.T) {
	dir := makeProviderTestDir(t, "providers: [\nbad yaml{{{{")

	_, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "check",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "loading agent config")
}

func TestAgentCheckJSON(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"fast-model", "slow-model"})
	dir := makeProviderTestDir(t, oaiAgentConfig(srv.URL+"/v1", "fast-model"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "check", "--json",
	)
	require.NoError(t, err)

	var entries []checkResultEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	require.Len(t, entries, 1)
	require.Equal(t, "testprovider", entries[0].Provider)
	require.Equal(t, "lmstudio", entries[0].Harness)
	require.Equal(t, "ok", entries[0].Status)
	require.GreaterOrEqual(t, entries[0].LatencyMs, int64(0))
	require.Empty(t, entries[0].Error)
}

func TestAgentCheckJSONAnthropicWithKey(t *testing.T) {
	cfg := `providers:
  claude:
    type: anthropic
    api_key: test-api-key-xyz
default: claude
`
	dir := makeProviderTestDir(t, cfg)

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "check", "--json",
	)
	require.NoError(t, err)

	var entries []checkResultEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	require.Len(t, entries, 1)
	require.Equal(t, "claude", entries[0].Provider)
	require.Equal(t, "anthropic", entries[0].Harness)
	require.Equal(t, "ok", entries[0].Status)
	require.Empty(t, entries[0].Error)
}

func TestAgentCheckJSONMixedReachability(t *testing.T) {
	live := newOAIModelsStub(t, []string{"good-model"})
	dead := newOAIModelsStub(t, nil)
	deadURL := dead.URL
	dead.Close()

	cfg := `providers:
  liveprov:
    type: lmstudio
    base_url: ` + live.URL + `/v1
    model: good-model
  deadprov:
    type: lmstudio
    base_url: ` + deadURL + `/v1
    model: bad-model
default: liveprov
`
	dir := makeProviderTestDir(t, cfg)

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "check", "--json",
	)
	require.NoError(t, err)

	var entries []checkResultEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	require.Len(t, entries, 2)

	entryMap := make(map[string]checkResultEntry)
	for _, e := range entries {
		entryMap[e.Provider] = e
	}

	live_ := entryMap["liveprov"]
	require.Equal(t, "ok", live_.Status)
	require.Empty(t, live_.Error)

	dead_ := entryMap["deadprov"]
	require.Equal(t, "unreachable", dead_.Status)
	require.NotEmpty(t, dead_.Error)
}
