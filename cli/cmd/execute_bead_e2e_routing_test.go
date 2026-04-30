package cmd

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteBeadRoutingEvidencePersisted is an end-to-end integration test that
// drives runAgentExecuteBead via a cobra Command.Execute call and asserts that
// routing evidence (resolved_provider, resolved_model, route_reason) is persisted
// as a kind:routing bead event in the bead store.
//
// This test exercises the full pipeline:
//
//	CLI flags → ExecuteBeadOptions → AgentRunner.Run → appendBeadRoutingEvidence → bead store
func TestExecuteBeadRoutingEvidencePersisted(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222", // agent produced a commit
		mergeErr:    nil,
	}
	runner := &fakeAgentRunner{result: &agent.Result{
		ExitCode:        0,
		Harness:         "mock-harness",
		Provider:        "test-provider",
		Model:           "test-model-7b",
		RouteReason:     "catalog-match",
		ResolvedBaseURL: "http://test.example.com/v1",
	}}
	f := newExecuteBeadFactory(t, git, runner)

	// Drive the full command via cobra Command.Execute — not by calling internal
	// functions directly. This exercises the CLI flag → ExecuteBeadOptions path.
	res := runExecuteBead(t, f, git, "my-bead")

	// Sanity-check: the command itself succeeded.
	require.Equal(t, "merged", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, "test-provider", res.Provider)
	assert.Equal(t, "test-model-7b", res.Model)

	// Read routing events directly from the bead store that runAgentExecuteBead
	// created (backed by f.WorkingDir/.ddx). This verifies end-to-end persistence,
	// not just that appendBeadRoutingEvidence was called in isolation.
	store := bead.NewStore(filepath.Join(f.WorkingDir, ".ddx"))
	routingEvents, err := store.EventsByKind("my-bead", "routing")
	require.NoError(t, err)
	require.Len(t, routingEvents, 1, "expected exactly one kind:routing event")

	evt := routingEvents[0]
	assert.Equal(t, "routing", evt.Kind)
	assert.Equal(t, "ddx", evt.Actor)
	assert.Equal(t, "ddx agent execute-bead", evt.Source)
	assert.NotEmpty(t, evt.Summary)

	// Parse the JSON body and assert all three required routing fields.
	var body struct {
		ResolvedProvider string `json:"resolved_provider"`
		ResolvedModel    string `json:"resolved_model"`
		RouteReason      string `json:"route_reason"`
		BaseURL          string `json:"base_url"`
	}
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &body),
		"routing event body should be valid JSON: %s", evt.Body)
	assert.Equal(t, "test-provider", body.ResolvedProvider)
	assert.Equal(t, "test-model-7b", body.ResolvedModel)
	assert.Equal(t, "catalog-match", body.RouteReason)
	assert.Equal(t, "http://test.example.com/v1", body.BaseURL)
}

// TestExecuteBeadRoutingEvidenceProviderFallsBackToHarness verifies that when
// the runner returns an empty Provider, the routing event's resolved_provider
// falls back to the harness name (as implemented in appendBeadRoutingEvidence).
func TestExecuteBeadRoutingEvidenceProviderFallsBackToHarness(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222",
	}
	// Runner returns no Provider — harness should be used as resolved_provider.
	runner := &fakeAgentRunner{result: &agent.Result{
		ExitCode:    0,
		Harness:     "fallback-harness",
		Provider:    "", // empty — triggers harness fallback
		Model:       "fallback-model",
		RouteReason: "first-available",
	}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")
	require.Equal(t, "merged", res.Outcome)

	store := bead.NewStore(filepath.Join(f.WorkingDir, ".ddx"))
	routingEvents, err := store.EventsByKind("my-bead", "routing")
	require.NoError(t, err)
	require.Len(t, routingEvents, 1)

	var body struct {
		ResolvedProvider string `json:"resolved_provider"`
		ResolvedModel    string `json:"resolved_model"`
		RouteReason      string `json:"route_reason"`
	}
	require.NoError(t, json.Unmarshal([]byte(routingEvents[0].Body), &body))
	// Provider was empty, so resolved_provider falls back to harness.
	assert.Equal(t, "fallback-harness", body.ResolvedProvider)
	assert.Equal(t, "fallback-model", body.ResolvedModel)
	assert.Equal(t, "first-available", body.RouteReason)
}

// TestExecuteBeadRoutingEvidenceNoEvidence verifies that routing evidence is
// persisted even when the agent exits without commits or rationale.
func TestExecuteBeadRoutingEvidenceNoEvidence(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111", // same rev — no commits made
		wtDirty:     false,
	}
	runner := &fakeAgentRunner{result: &agent.Result{
		ExitCode:    0,
		Harness:     "mock-harness",
		Provider:    "no-change-provider",
		Model:       "no-change-model",
		RouteReason: "direct-override",
	}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")
	require.Equal(t, "no-evidence", res.Outcome)

	store := bead.NewStore(filepath.Join(f.WorkingDir, ".ddx"))
	routingEvents, err := store.EventsByKind("my-bead", "routing")
	require.NoError(t, err)
	require.Len(t, routingEvents, 1, "routing event should be persisted even on no-evidence outcome")

	var body struct {
		ResolvedProvider string `json:"resolved_provider"`
		ResolvedModel    string `json:"resolved_model"`
		RouteReason      string `json:"route_reason"`
	}
	require.NoError(t, json.Unmarshal([]byte(routingEvents[0].Body), &body))
	assert.Equal(t, "no-change-provider", body.ResolvedProvider)
	assert.Equal(t, "no-change-model", body.ResolvedModel)
	assert.Equal(t, "direct-override", body.RouteReason)
}
