package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteBead_RoutingEvidenceRecorded(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	const beadID = "ddx-int-0001"

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		"run mkdir -p .ddx/executions/$DDX_ATTEMPT_ID && printf 'already satisfied in base' > .ddx/executions/$DDX_ATTEMPT_ID/no_changes_rationale.txt",
	})

	beadStore := bead.NewStore(ddxDir)
	runner := NewRunner(Config{})
	gitOps := &RealGitOps{}

	cfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Model: dirFile,
	})
	rcfg := cfg.Resolve(config.CLIOverrides{Harness: "script"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
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
	writeDirectiveFile(t, dirFile, []string{
		"run mkdir -p .ddx/executions/$DDX_ATTEMPT_ID && printf 'already satisfied in base' > .ddx/executions/$DDX_ATTEMPT_ID/no_changes_rationale.txt",
	})

	runner := NewRunner(Config{})
	gitOps := &RealGitOps{}

	cfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Model: dirFile,
	})
	rcfg := cfg.Resolve(config.CLIOverrides{Harness: "script"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		BeadEvents:  nil,
		AgentRunner: runner,
	}, gitOps)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, ExecuteBeadOutcomeTaskNoChanges, res.Outcome)
}

func TestExecuteBead_RoutingEvidenceWithCommit(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	const beadID = "ddx-int-0001"

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		"append-line output.txt routing-evidence-test",
		"commit chore: routing evidence test",
	})

	beadStore := bead.NewStore(ddxDir)
	runner := NewRunner(Config{})
	gitOps := &RealGitOps{}
	orchGitOps := &RealGitOps{}

	cfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Model: dirFile,
	})
	rcfg := cfg.Resolve(config.CLIOverrides{Harness: "script"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
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

func TestExecutionRoutingIntentRecordsEstimatedDifficulty(t *testing.T) {
	app := &stubBeadEventAppender{}
	target := bead.Bead{
		ID: "ddx-0001",
		Extra: map[string]any{
			escalation.BeadEstimatedDifficultyKey: string(escalation.DifficultyHard),
		},
	}
	appendExecutionRoutingIntentEvidence(app, target, ExecuteBeadReport{
		AttemptID:          "20260515T185832-test",
		Harness:            "claude",
		Provider:           "anthropic",
		Model:              "claude-sonnet-4-6",
		RequestedProfile:   "default",
		InferredPowerClass: "smart",
		RoutingIntentNote:  "actual route facts unavailable",
	}, time.Date(2026, 4, 21, 16, 0, 0, 0, time.UTC))

	require.Len(t, app.events, 1)
	assert.Equal(t, "ddx-0001", app.events[0].BeadID)
	assert.Equal(t, "execution-routing-intent", app.events[0].Event.Kind)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(app.events[0].Event.Body), &body))
	assert.Equal(t, "bead_hint", body["routing_intent_source"])
	assert.Equal(t, "hard", body["estimated_difficulty"])
	assert.Equal(t, "smart", body["requested_power_class"])
	assert.Equal(t, "default", body["requested_profile"])
	assert.NotContains(t, body, "smart_justification")
	assert.Contains(t, app.events[0].Event.Summary, "difficulty=hard")
	assert.Contains(t, app.events[0].Event.Summary, "powerClass=smart")
}

func TestExecutionRoutingIntentCLISourceSuppressesBeadDifficulty(t *testing.T) {
	app := &stubBeadEventAppender{}
	target := bead.Bead{
		ID: "ddx-0001",
		Extra: map[string]any{
			escalation.BeadEstimatedDifficultyKey: string(escalation.DifficultyHard),
		},
	}
	appendExecutionRoutingIntentEvidence(app, target, ExecuteBeadReport{
		AttemptID:           "20260515T185832-test",
		RoutingIntentSource: string(escalation.ExecutionIntentSourceCLIPassthru),
		Harness:             "claude",
		Model:               "claude-sonnet-4-6",
	}, time.Date(2026, 4, 21, 16, 0, 0, 0, time.UTC))

	require.Len(t, app.events, 1)
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(app.events[0].Event.Body), &body))
	assert.Equal(t, "cli", body["routing_intent_source"])
	assert.Empty(t, body["estimated_difficulty"])
	assert.Empty(t, body["requested_power_class"])
	assert.NotContains(t, app.events[0].Event.Summary, "difficulty=")
	assert.NotContains(t, app.events[0].Event.Summary, "powerClass=")
}

func TestAppendLoopRoutingEvidenceRecordsProfileTelemetry(t *testing.T) {
	app := &stubBeadEventAppender{}
	target := bead.Bead{
		ID: "ddx-0001",
		Extra: map[string]any{
			escalation.BeadEstimatedDifficultyKey: string(escalation.DifficultyEasy),
		},
	}
	appendLoopRoutingEvidence(app, target, ExecuteBeadReport{
		Provider:           "openai",
		Model:              "gpt-5.4",
		RequestedProfile:   "default",
		InferredPowerClass: "cheap",
		ResolvedPowerClass: "standard",
		EscalationCount:    1,
		FinalPowerClass:    "standard",
	}, time.Date(2026, 4, 21, 16, 0, 0, 0, time.UTC), nil)

	require.Len(t, app.events, 1)
	assert.Equal(t, "ddx-0001", app.events[0].BeadID)
	assert.Equal(t, "routing", app.events[0].Event.Kind)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(app.events[0].Event.Body), &body))
	assert.Equal(t, "easy", body["estimated_difficulty"])
	assert.Equal(t, "default", body["requested_profile"])
	assert.Equal(t, "cheap", body["requested_power_class"])
	assert.Equal(t, "standard", body["resolved_power_class"])
	assert.Equal(t, float64(1), body["escalation_count"])
	assert.Equal(t, "standard", body["final_power_class"])
}

// TestAppendLoopRoutingEvidence_RouteFailureFallbackChain proves that prior
// route-failure entries on a bead get serialised into the routing event's
// fallback_chain field so post-hoc routing analytics can see which
// provider/model tuples were excluded before the resolved route was selected.
func TestAppendLoopRoutingEvidence_RouteFailureFallbackChain(t *testing.T) {
	app := &stubBeadEventAppender{}
	failed := []FailedRouteEntry{
		{Provider: "bragi", Model: "qwen3.5-27b", ActualPower: 50, Reason: FailureModeProviderConnectivity},
	}
	appendLoopRoutingEvidence(app, bead.Bead{ID: "ddx-0001"}, ExecuteBeadReport{
		Provider: "openai",
		Model:    "gpt-5.4",
	}, time.Date(2026, 4, 21, 16, 0, 0, 0, time.UTC), failed)

	require.Len(t, app.events, 1)
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(app.events[0].Event.Body), &body))
	chain, ok := body["fallback_chain"].([]any)
	require.True(t, ok, "fallback_chain must be a JSON array")
	require.Len(t, chain, 1)
	first := chain[0].(map[string]any)
	assert.Equal(t, "bragi", first["provider"])
	assert.Equal(t, "qwen3.5-27b", first["model"])
	assert.Equal(t, float64(50), first["actual_power"])
	assert.Equal(t, FailureModeProviderConnectivity, first["reason"])
	assert.Equal(t, "openai", body["resolved_provider"])
}
