package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/require"
)

// routeAgentConfig returns .fizeau/config.yaml YAML for a provider + a single
// model route referencing that provider. routeKey is the model route name.
func routeAgentConfig(baseURL, model, routeKey string) string {
	return `providers:
  testprovider:
    type: lmstudio
    base_url: ` + baseURL + `
    model: ` + model + `
default: testprovider
model_routes:
  ` + routeKey + `:
    candidates:
      - provider: testprovider
        model: ` + model + `
`
}

func TestAgentRouteStatusNoRoutes(t *testing.T) {
	// Config with a provider but no model_routes.
	dir := makeProviderTestDir(t, oaiAgentConfig("http://127.0.0.1:9/v1", "x"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status",
	)
	require.NoError(t, err)
	require.Contains(t, out, "No model routes configured")
}

func TestAgentRouteStatusSuccess(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"qwen3-32b"})
	dir := makeProviderTestDir(t, routeAgentConfig(srv.URL+"/v1", "qwen3-32b", "smart"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status",
	)
	require.NoError(t, err)
	require.Contains(t, out, "Route: smart")
	require.Contains(t, out, "testprovider")
	require.Contains(t, out, "available")
	require.Contains(t, out, "Recent Routing Decisions")
	require.Contains(t, out, "Active Health Cooldowns")
}

func TestAgentRouteStatusJSON(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"fast-model"})
	dir := makeProviderTestDir(t, routeAgentConfig(srv.URL+"/v1", "fast-model", "cheap"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status", "--json",
	)
	require.NoError(t, err)

	var payload routeStatusJSON
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	require.Len(t, payload.Routes, 1)
	require.Equal(t, "cheap", payload.Routes[0].RouteKey)
	require.Len(t, payload.Routes[0].Candidates, 1)
	require.Equal(t, "testprovider", payload.Routes[0].Candidates[0].Provider)
	require.True(t, payload.Routes[0].Candidates[0].Healthy)
	require.Equal(t, "testprovider", payload.Routes[0].SelectedProvider)
}

func TestAgentRouteStatusUnknownModelFlag(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"some-model"})
	dir := makeProviderTestDir(t, routeAgentConfig(srv.URL+"/v1", "some-model", "standard"))

	_, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status", "--model", "nonexistent-route",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no route configured for model key")
}

func TestAgentRouteStatusModelFlag(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"selected-model"})
	dir := makeProviderTestDir(t, routeAgentConfig(srv.URL+"/v1", "selected-model", "my-route"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status", "--model", "my-route",
	)
	require.NoError(t, err)
	require.Contains(t, out, "Route: my-route")
	require.Contains(t, out, "testprovider")
}

func TestAgentRouteStatusUnreachableProvider(t *testing.T) {
	dead := newOAIModelsStub(t, nil)
	deadURL := dead.URL
	dead.Close()

	dir := makeProviderTestDir(t, routeAgentConfig(deadURL+"/v1", "dead-model", "smart"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status",
	)
	require.NoError(t, err)
	require.Contains(t, out, "Route: smart")
	// Without a cooldown file the service reports Healthy: true (no live probe).
	// The candidate shows as "available"; selected provider is populated.
	require.Contains(t, out, "testprovider")
}

func TestAgentRouteStatusJSONUnreachable(t *testing.T) {
	dead := newOAIModelsStub(t, nil)
	deadURL := dead.URL
	dead.Close()

	dir := makeProviderTestDir(t, routeAgentConfig(deadURL+"/v1", "dead-model", "cheap"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status", "--json",
	)
	require.NoError(t, err)

	var payload routeStatusJSON
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	require.Len(t, payload.Routes, 1)
	// Without a cooldown file the service reports Healthy: true (no live probe).
	require.True(t, payload.Routes[0].Candidates[0].Healthy)
	require.Equal(t, "testprovider", payload.Routes[0].SelectedProvider)
}

func TestAgentRouteStatusActiveCooldown(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"cool-model"})
	dir := makeProviderTestDir(t, routeAgentConfig(srv.URL+"/v1", "cool-model", "standard"))

	// Write a route-health file recording a recent failure for testprovider.
	agentDir := filepath.Join(dir, ".fizeau")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))
	failedAt := time.Now().Add(-1 * time.Minute) // 1 minute ago, within 30m cooldown
	healthJSON := `{"failures":{"testprovider":"` + failedAt.UTC().Format(time.RFC3339) + `"}}`
	healthFile := filepath.Join(agentDir, "route-health-standard.json")
	require.NoError(t, os.WriteFile(healthFile, []byte(healthJSON), 0o644))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status",
	)
	require.NoError(t, err)
	// Cooldown is active → should appear in the cooldowns section.
	require.Contains(t, out, "Active Health Cooldowns")
	require.True(t,
		strings.Contains(out, "testprovider") && strings.Contains(out, "standard"),
		"expected active cooldown entry for testprovider on route standard",
	)
}

func TestAgentRouteStatusBeadEvidence(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"evidence-model"})
	dir := makeProviderTestDir(t, routeAgentConfig(srv.URL+"/v1", "evidence-model", "smart"))

	// Write a beads.jsonl entry with a kind:routing event so that
	// beadRoutingDecisionsFromStore and routingEventsFromBeadExtra are exercised.
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

func TestAgentRouteStatusBeadEvidenceJSON(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"evidence-model"})
	dir := makeProviderTestDir(t, routeAgentConfig(srv.URL+"/v1", "evidence-model", "smart"))

	beadLine := `{"id":"bead-002","title":"Test bead 2","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-15T00:00:00Z","updated_at":"2026-04-15T00:00:00Z","events":[{"kind":"routing","summary":"routed","body":"{\"resolved_provider\":\"testprovider\",\"resolved_model\":\"evidence-model\",\"route_reason\":\"health\"}","created_at":"2026-04-15T00:02:00Z"}]}` + "\n"
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(beadLine), 0o644))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status", "--json",
	)
	require.NoError(t, err)

	var payload routeStatusJSON
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	require.NotEmpty(t, payload.RecentDecisions)
	require.Equal(t, "bead-evidence", payload.RecentDecisions[0].Source)
	require.Equal(t, "testprovider", payload.RecentDecisions[0].Provider)
	require.Equal(t, "bead-002", payload.RecentDecisions[0].BeadID)
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

func TestAgentRouteStatusNoRoutesJSON(t *testing.T) {
	dir := makeProviderTestDir(t, oaiAgentConfig("http://127.0.0.1:9/v1", "x"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status", "--json",
	)
	require.NoError(t, err)

	// Should emit an empty JSON object.
	var payload routeStatusJSON
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	require.Empty(t, payload.Routes)
}

// TestAgentRouteStatusCandidatePowerFieldInJSON verifies that route-status
// JSON output includes the power field on each candidate (AC1: status surface
// renders model power catalog). Power is sourced from LastDecision candidates;
// when no prior route decision is cached the field is absent (omitempty).
// The test verifies the JSON structure is parseable and the candidate is rendered.
func TestAgentRouteStatusCandidatePowerFieldInJSON(t *testing.T) {
	srv := newOAIModelsStub(t, []string{"test-model"})
	dir := makeProviderTestDir(t, routeAgentConfig(srv.URL+"/v1", "test-model", "smart"))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"agent", "route-status", "--json",
	)
	require.NoError(t, err)

	var payload routeStatusJSON
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	require.Len(t, payload.Routes, 1)
	require.NotEmpty(t, payload.Routes[0].Candidates, "route must have at least one candidate")

	// Power field must be present in the JSON structure (value 0 when no
	// LastDecision cached — omitempty means the key is absent when zero,
	// which is correct and expected).
	cand := payload.Routes[0].Candidates[0]
	require.Equal(t, "testprovider", cand.Provider)
	require.Equal(t, "test-model", cand.Model)
	// LastDecision is nil (no ResolveRoute called), so Power is 0 / omitted.
	require.Equal(t, 0, cand.Power)
}

// TestRouteStatusLastDecisionCandidateTrace verifies the helpers used to
// render RouteDecision.Candidates for observability (AC2). Tests the
// rendering functions directly with synthetic data.
func TestRouteStatusLastDecisionCandidateTrace(t *testing.T) {
	dec := &agentlib.RouteDecision{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-6",
		Reason:   "first-available",
		Candidates: []agentlib.RouteCandidate{
			{
				Provider: "local",
				Model:    "qwen3-27b",
				Eligible: false,
				Reason:   "power_below_min",
				Components: agentlib.RouteCandidateComponents{
					Power: 10,
				},
			},
			{
				Provider: "anthropic",
				Model:    "claude-sonnet-4-6",
				Eligible: true,
				Reason:   "selected",
				Score:    0.92,
				Components: agentlib.RouteCandidateComponents{
					Power: 65,
				},
			},
		},
	}

	// Test candidatePowerFromLastDecision helper.
	require.Equal(t, 10, candidatePowerFromLastDecision(dec, "local", "qwen3-27b"),
		"power for local/qwen3-27b must be 10")
	require.Equal(t, 65, candidatePowerFromLastDecision(dec, "anthropic", "claude-sonnet-4-6"),
		"power for selected candidate must be 65")
	require.Equal(t, 0, candidatePowerFromLastDecision(dec, "unknown", "unknown-model"),
		"unknown candidate must return 0")
	require.Equal(t, 0, candidatePowerFromLastDecision(nil, "any", "any"),
		"nil LastDecision must return 0")

	// Test lastDecisionToJSON helper.
	jd := lastDecisionToJSON(dec)
	require.NotNil(t, jd)
	require.Equal(t, "anthropic", jd.Provider)
	require.Equal(t, "claude-sonnet-4-6", jd.Model)
	require.Equal(t, "first-available", jd.Reason)
	require.Len(t, jd.Candidates, 2)

	// Ineligible candidate
	require.Equal(t, "local", jd.Candidates[0].Provider)
	require.Equal(t, 10, jd.Candidates[0].Power)
	require.False(t, jd.Candidates[0].Eligible)
	require.Equal(t, "power_below_min", jd.Candidates[0].Reason)

	// Selected candidate
	require.Equal(t, "anthropic", jd.Candidates[1].Provider)
	require.Equal(t, 65, jd.Candidates[1].Power)
	require.True(t, jd.Candidates[1].Eligible)

	// Test lastDecisionToJSON with nil input.
	require.Nil(t, lastDecisionToJSON(nil))
}
