package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteBead_RoutingEvidenceRecorded(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ".ddx")
	const beadID = "ddx-int-0001"

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{"no-op"})

	beadStore := bead.NewStore(ddxDir)
	runner := NewRunner(Config{})
	gitOps := &RealGitOps{}

	res, err := ExecuteBead(context.Background(), projectRoot, beadID, ExecuteBeadOptions{
		Harness:     "script",
		Model:       dirFile,
		BeadEvents:  beadStore,
		AgentRunner: runner,
	}, gitOps)

	require.NoError(t, err)
	require.NotNil(t, res)

	events, evErr := beadStore.Events(beadID)
	require.NoError(t, evErr)

	var routingEvt *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "routing" {
			routingEvt = &events[i]
			break
		}
	}
	require.NotNil(t, routingEvt, "expected a kind=routing evidence event on bead %s", beadID)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(routingEvt.Body), &body),
		"routing evidence body must be valid JSON")

	assert.Equal(t, "routing", routingEvt.Kind)
	assert.NotEmpty(t, body["resolved_provider"], "resolved_provider must be set")
	assert.Contains(t, body, "route_reason", "route_reason field must be present")
	assert.NotEmpty(t, body["route_reason"], "route_reason must be non-empty")
	assert.Contains(t, body, "fallback_chain", "fallback_chain field must be present")
	assert.Contains(t, body, "resolved_model", "resolved_model field must be present")

	assert.Equal(t, "script", body["resolved_provider"])
	assert.Equal(t, "direct-override", body["route_reason"])
}

func TestExecuteBead_RoutingEvidenceNoAppenderIsNoop(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{"no-op"})

	runner := NewRunner(Config{})
	gitOps := &RealGitOps{}

	res, err := ExecuteBead(context.Background(), projectRoot, beadID, ExecuteBeadOptions{
		Harness:     "script",
		Model:       dirFile,
		BeadEvents:  nil,
		AgentRunner: runner,
	}, gitOps)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, ExecuteBeadOutcomeTaskNoChanges, res.Outcome)
}

func TestExecuteBead_RoutingEvidenceWithCommit(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ".ddx")
	const beadID = "ddx-int-0001"

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		"append-line output.txt routing-evidence-test",
		"commit chore: routing evidence test",
	})

	beadStore := bead.NewStore(ddxDir)
	runner := NewRunner(Config{})
	gitOps := &RealGitOps{}
	orchGitOps := &RealOrchestratorGitOps{}

	res, err := ExecuteBead(context.Background(), projectRoot, beadID, ExecuteBeadOptions{
		Harness:     "script",
		Model:       dirFile,
		BeadEvents:  beadStore,
		AgentRunner: runner,
	}, gitOps)
	require.NoError(t, err)
	require.NotNil(t, res)

	_, _ = LandBeadResult(projectRoot, res, orchGitOps, BeadLandingOptions{
		LandingAdvancer: func(r *ExecuteBeadResult) (*LandResult, error) {
			return Land(projectRoot, BuildLandRequestFromResult(projectRoot, r), RealLandingGitOps{})
		},
	})

	events, evErr := beadStore.Events(beadID)
	require.NoError(t, evErr)

	found := false
	for _, evt := range events {
		if evt.Kind == "routing" {
			found = true
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(evt.Body), &body))
			assert.Equal(t, "script", body["resolved_provider"])
			assert.Equal(t, "direct-override", body["route_reason"])
			break
		}
	}
	assert.True(t, found, "routing evidence must be recorded even on task_succeeded path")
}
