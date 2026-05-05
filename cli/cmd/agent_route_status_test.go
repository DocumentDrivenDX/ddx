package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentRouteStatusShowsLiveProvidersAndModels(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"qwen3-32b"})
	dir := makeProviderTestDir(t, oaiAgentConfig(srv.URL+"/v1", "qwen3-32b"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status",
	)
	require.NoError(t, err)
	require.Contains(t, out, "Providers")
	require.Contains(t, out, "Models")
	require.Contains(t, out, "testprovider")
	require.Contains(t, out, "qwen3-32b")
	require.NotContains(t, out, "model_routes")
	require.NotContains(t, out, "Route:")
}

func TestAgentRouteStatusJSON(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"fast-model"})
	dir := makeProviderTestDir(t, oaiAgentConfig(srv.URL+"/v1", "fast-model"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status", "--json",
	)
	require.NoError(t, err)

	var payload routeStatusJSON
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	require.NotEmpty(t, payload.Providers)
	require.NotEmpty(t, payload.Models)
	require.True(t, routeStatusJSONHasProvider(payload, "testprovider"))
	require.True(t, routeStatusJSONHasModel(payload, "fast-model"))
}

func routeStatusJSONHasProvider(payload routeStatusJSON, name string) bool {
	for _, p := range payload.Providers {
		if p.Name == name {
			return true
		}
	}
	return false
}

func routeStatusJSONHasModel(payload routeStatusJSON, model string) bool {
	for _, m := range payload.Models {
		if m.Model == model {
			return true
		}
	}
	return false
}

func TestAgentRouteStatusModelFlagFiltersConcreteModel(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"selected-model", "other-model"})
	dir := makeProviderTestDir(t, oaiAgentConfig(srv.URL+"/v1", "selected-model"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status", "--model", "selected-model",
	)
	require.NoError(t, err)
	require.Contains(t, out, "selected-model")
	require.NotContains(t, out, "other-model")
}

func TestAgentRouteStatusBeadEvidence(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"evidence-model"})
	dir := makeProviderTestDir(t, oaiAgentConfig(srv.URL+"/v1", "evidence-model"))

	beadLine := `{"id":"bead-001","title":"Test bead","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-15T00:00:00Z","updated_at":"2026-04-15T00:00:00Z","events":[{"kind":"routing","summary":"routed to testprovider","body":"{\"resolved_provider\":\"testprovider\",\"resolved_model\":\"evidence-model\",\"route_reason\":\"first-available\"}","created_at":"2026-04-15T00:01:00Z"}]}` + "\n"
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(beadLine), 0o644))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status",
	)
	require.NoError(t, err)
	require.Contains(t, out, "Recent Routing Decisions")
	require.Contains(t, out, "bead-evidence")
	require.Contains(t, out, "testprovider")
}

func TestAgentRouteStatusConfigError(t *testing.T) {
	dir := makeProviderTestDir(t, "providers: [\nbad yaml{{{")

	_, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "loading agent config")
}

func TestAgentRouteStatusJSONActiveCooldown(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"cool-model"})
	dir := makeProviderTestDir(t, oaiAgentConfig(srv.URL+"/v1", "cool-model"))

	providerHealthDir := filepath.Join(dir, ".fizeau")
	require.NoError(t, os.MkdirAll(providerHealthDir, 0o755))
	cooldownUntil := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	healthJSON := `{"providers":{"testprovider":{"until":"` + cooldownUntil + `","reason":"test"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(providerHealthDir, "provider-health.json"), []byte(healthJSON), 0o644))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status", "--json",
	)
	require.NoError(t, err)

	var payload routeStatusJSON
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	require.NotNil(t, payload)
}
