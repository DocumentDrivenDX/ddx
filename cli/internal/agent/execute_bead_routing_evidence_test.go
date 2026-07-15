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
	runner := scriptHarnessAgentRunner{}
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

	runner := scriptHarnessAgentRunner{}
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
	runner := scriptHarnessAgentRunner{}
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

func TestAppendExecutionRoutingIntentRecordsInferredMinPower(t *testing.T) {
	app := &stubBeadEventAppender{}
	target := bead.Bead{
		ID: "ddx-0001",
		Extra: map[string]any{
			escalation.BeadEstimatedDifficultyKey: string(escalation.DifficultyEasy),
		},
	}
	appendExecutionRoutingIntentEvidence(app, target, ExecuteBeadReport{
		AttemptID:               "20260515T185832-test",
		Harness:                 "claude",
		Provider:                "anthropic",
		Model:                   "claude-sonnet-4-6",
		RoutingIntentSource:     string(escalation.ExecutionIntentSourceBeadHint),
		EstimatedDifficulty:     string(escalation.DifficultyEasy),
		InferredMinPower:        0,
		InferredMinPowerPresent: true,
		RequestedPolicy:         "opaque-policy",
		RequestedMinPower:       0,
		RequestedMaxPower:       10,
	}, time.Date(2026, 4, 21, 16, 0, 0, 0, time.UTC))

	require.Len(t, app.events, 1)
	assert.Equal(t, "ddx-0001", app.events[0].BeadID)
	assert.Equal(t, "execution-routing-intent", app.events[0].Event.Kind)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(app.events[0].Event.Body), &body))
	assert.Equal(t, "bead_hint", body["routing_intent_source"])
	assert.Equal(t, "easy", body["estimated_difficulty"])
	assert.Equal(t, float64(0), body["inferred_min_power"], "easy's zero floor must remain present")
	assert.Equal(t, "opaque-policy", body["requested_policy"])
	assert.Equal(t, float64(0), body["requested_min_power"])
	assert.Equal(t, float64(10), body["requested_max_power"])
	assert.NotContains(t, body, "requested_power_class")
	assert.NotContains(t, body, "requested_profile")
	assert.Contains(t, app.events[0].Event.Summary, "difficulty=easy")
	assert.Contains(t, app.events[0].Event.Summary, "minPower=0")
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
	assert.NotContains(t, body, "inferred_min_power")
	assert.NotContains(t, app.events[0].Event.Summary, "difficulty=")
	assert.NotContains(t, app.events[0].Event.Summary, "minPower=")
}

func TestAppendLoopRoutingEvidenceRecordsNumericIntent(t *testing.T) {
	app := &stubBeadEventAppender{}
	target := bead.Bead{
		ID: "ddx-0001",
		Extra: map[string]any{
			escalation.BeadEstimatedDifficultyKey: string(escalation.DifficultyEasy),
		},
	}
	appendLoopRoutingEvidence(app, target, ExecuteBeadReport{
		Provider:                "openai",
		Model:                   "gpt-5.4",
		InferredMinPower:        0,
		InferredMinPowerPresent: true,
		RequestedMinPower:       0,
		RequestedMaxPower:       10,
		ResolvedPowerClass:      "standard",
		EscalationCount:         1,
		FinalPowerClass:         "standard",
	}, time.Date(2026, 4, 21, 16, 0, 0, 0, time.UTC))

	require.Len(t, app.events, 1)
	assert.Equal(t, "ddx-0001", app.events[0].BeadID)
	assert.Equal(t, "routing", app.events[0].Event.Kind)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(app.events[0].Event.Body), &body))
	assert.Equal(t, "easy", body["estimated_difficulty"])
	assert.Equal(t, float64(0), body["inferred_min_power"])
	assert.Equal(t, float64(0), body["requested_min_power"])
	assert.Equal(t, float64(10), body["requested_max_power"])
	assert.NotContains(t, body, "requested_profile")
	assert.NotContains(t, body, "requested_power_class")
	assert.Equal(t, "standard", body["resolved_power_class"])
	assert.Equal(t, float64(1), body["escalation_count"])
	assert.Equal(t, "standard", body["final_power_class"])
}

// TestAppendLoopRoutingEvidence_DoesNotSynthesizeFallbackChain proves DDx does
// not convert historical concrete routes into routing policy.
func TestRoutingEvidenceReturnedRouteIdentityIsEvidenceOnly(t *testing.T) {
	app := &stubBeadEventAppender{}
	appendLoopRoutingEvidence(app, bead.Bead{ID: "ddx-0001"}, ExecuteBeadReport{
		Harness:           "codex",
		Provider:          "openai",
		Model:             "gpt-5.4",
		ActualPower:       9,
		RequestedPolicy:   "opaque-policy",
		RequestedMinPower: 7,
		RequestedMaxPower: 10,
	}, time.Date(2026, 4, 21, 16, 0, 0, 0, time.UTC))

	require.Len(t, app.events, 1)
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(app.events[0].Event.Body), &body))
	chain, ok := body["fallback_chain"].([]any)
	require.True(t, ok, "fallback_chain must be a JSON array")
	require.Empty(t, chain)
	assert.Equal(t, "openai", body["resolved_provider"])
	assert.Equal(t, "opaque-policy", body["requested_policy"])
	assert.Equal(t, float64(7), body["requested_min_power"])
	assert.Equal(t, float64(10), body["requested_max_power"])
	assert.NotContains(t, body, "requested_profile")
	assert.NotContains(t, body, "requested_power_class")
}
