package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type productionRoutingHookRunner struct {
	difficulty             string
	close                  bool
	forbiddenTriageStrings []string
	triagePrompts          []string
	triageLeak             string
}

func (r *productionRoutingHookRunner) Run(opts agent.RunArgs) (*agent.Result, error) {
	switch opts.PromptSource {
	case "bead-lifecycle-intake":
		difficulty := ""
		if r.difficulty != "" {
			difficulty = fmt.Sprintf(`,"difficulty":{"estimated_difficulty":%q,"confidence":0.9,"reason":"test"}`, r.difficulty)
		}
		return &agent.Result{ExitCode: 0, Output: `{"classification":"ready","rationale":"single-slice","readiness_checks":[]` + difficulty + `}`}, nil
	case "bead-lifecycle-lint":
		return &agent.Result{ExitCode: 0, Output: `{"score":9,"rationale":"ok","suggested_fixes":[],"waivers_applied":[]}`}, nil
	case "bead-lifecycle-triage":
		r.triagePrompts = append(r.triagePrompts, opts.Prompt)
		for _, forbidden := range r.forbiddenTriageStrings {
			if forbidden != "" && strings.Contains(opts.Prompt, forbidden) {
				r.triageLeak = forbidden
				return nil, fmt.Errorf("concrete returned route identity %q leaked into control triage prompt", forbidden)
			}
		}
		if r.close {
			return &agent.Result{ExitCode: 0, Output: `{"classification":"already_satisfied","recommended_action":"close_already_satisfied","rationale":"done","suggested_amendments":[],"suggested_followup_beads":[]}`}, nil
		}
		return &agent.Result{ExitCode: 0, Output: `{"classification":"transport","recommended_action":"release_claim_retry","rationale":"retry","suggested_amendments":[],"suggested_followup_beads":[]}`}, nil
	default:
		return nil, fmt.Errorf("unexpected prompt source %q", opts.PromptSource)
	}
}

type productionRoutingResult struct {
	projectRoot                string
	beadID                     string
	requests                   []agentlib.ServiceExecuteRequest
	events                     []bead.BeadEvent
	bead                       *bead.Bead
	output                     string
	err                        error
	routeRequestsBeforeExecute int
	modelQueriesBeforeExecute  int
}

func runProductionRoutingEntryPoint(
	t *testing.T,
	entryPoint string,
	mutate func(*bead.Bead),
	flags []string,
	runner *productionRoutingHookRunner,
	executeFn func(agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error),
) productionRoutingResult {
	t.Helper()
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	implementationExecute := executeFn
	if implementationExecute == nil {
		implementationExecute = routeDelegationExecute
	}
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		if entryPoint == "work" && req.Role != "implementer" {
			return productionRoutingHookExecute(req, runner)
		}
		return implementationExecute(req)
	}
	beadID := "ddx-production-routing-" + strings.ReplaceAll(t.Name(), "/", "-")
	projectRoot := newRouteDelegationProject(t, beadID)
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	if mutate != nil {
		require.NoError(t, store.Update(context.Background(), beadID, mutate))
	}
	if runner == nil {
		runner = &productionRoutingHookRunner{}
	}
	factory := NewCommandFactory(projectRoot)
	if entryPoint == "try" {
		factory.AgentRunnerOverride = runner
	}
	args := []string{entryPoint}
	if entryPoint == "try" {
		args = append(args, beadID)
	} else {
		args = append(args, "--once")
	}
	args = append(args, "--project", projectRoot)
	args = append(args, flags...)
	args = append(args, "--no-review", "--no-review-i-know-what-im-doing")
	output, runErr := executeCommand(factory.NewRootCommand(), args...)
	events, eventsErr := store.Events(beadID)
	require.NoError(t, eventsErr)
	current, getErr := store.Get(context.Background(), beadID)
	require.NoError(t, getErr)
	return productionRoutingResult{
		projectRoot:                projectRoot,
		beadID:                     beadID,
		requests:                   capturedImplementationRequests(stub),
		events:                     events,
		bead:                       current,
		output:                     output,
		err:                        runErr,
		routeRequestsBeforeExecute: len(capturedRouteRequests(stub)),
		modelQueriesBeforeExecute:  modelQueriesBeforeExecute(stub),
	}
}

func productionRoutingHookExecute(req agentlib.ServiceExecuteRequest, runner *productionRoutingHookRunner) (<-chan agentlib.ServiceEvent, error) {
	if runner == nil {
		runner = &productionRoutingHookRunner{}
	}
	promptSource := "bead-lifecycle-lint"
	switch {
	case strings.Contains(req.Prompt, "MODE: intake"):
		promptSource = "bead-lifecycle-intake"
	case strings.Contains(req.Prompt, "MODE: triage"):
		promptSource = "bead-lifecycle-triage"
	}
	result, err := runner.Run(agent.RunArgs{Prompt: req.Prompt, PromptSource: promptSource})
	if err != nil {
		return nil, err
	}
	ch := make(chan agentlib.ServiceEvent, 1)
	payload, err := json.Marshal(map[string]any{"status": "success", "final_text": result.Output})
	if err != nil {
		return nil, err
	}
	ch <- agentlib.ServiceEvent{Type: "final", Data: payload}
	close(ch)
	return ch, nil
}

func routingIntentEventBody(t *testing.T, events []bead.BeadEvent) map[string]any {
	t.Helper()
	for _, event := range events {
		if event.Kind != "execution-routing-intent" {
			continue
		}
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
		return body
	}
	t.Fatal("execution-routing-intent event not found")
	return nil
}

func TestEstimatedDifficultyMapsOnlyMinPower(t *testing.T) {
	cases := []struct {
		name       string
		difficulty string
		want       int
		unrelated  bool
	}{
		{name: "easy", difficulty: "easy", want: 0},
		{name: "medium", difficulty: "medium", want: 7},
		{name: "hard", difficulty: "hard", want: 9},
		{name: "absent", want: 7},
		{name: "invalid", difficulty: "expensive", want: 7},
		{name: "unrelated_signals", want: 7, unrelated: true},
	}
	for _, entryPoint := range []string{"work", "try"} {
		for _, tc := range cases {
			t.Run(entryPoint+"/"+tc.name, func(t *testing.T) {
				result := runProductionRoutingEntryPoint(t, entryPoint, func(target *bead.Bead) {
					if tc.difficulty != "" {
						if target.Extra == nil {
							target.Extra = map[string]any{}
						}
						target.Extra[escalation.BeadEstimatedDifficultyKey] = tc.difficulty
					}
					if tc.unrelated {
						target.Priority = 0
						target.IssueType = "incident"
						target.Description = strings.Repeat("unrelated prose length must not route; ", 300)
						target.Acceptance = strings.Repeat("long acceptance criterion; ", 200)
						target.Labels = append(target.Labels, "power:smart", "kind:incident")
						if target.Extra == nil {
							target.Extra = map[string]any{}
						}
						target.Extra["work-next-min-power"] = 99
						target.Extra["numeric-unrelated"] = 12345
					}
				}, nil, nil, nil)
				require.NotEmpty(t, result.requests, "output=%q err=%v", result.output, result.err)
				req := result.requests[0]
				assert.Equal(t, tc.want, req.MinPower)
				assert.Empty(t, req.Policy)
				assert.Empty(t, req.Harness)
				assert.Empty(t, req.Provider)
				assert.Empty(t, req.Model)
				assert.Zero(t, result.routeRequestsBeforeExecute)
				assert.Zero(t, result.modelQueriesBeforeExecute)

				body := routingIntentEventBody(t, result.events)
				assert.Equal(t, float64(tc.want), body["inferred_min_power"])
				assert.Equal(t, float64(tc.want), body["requested_min_power"])
				assert.NotContains(t, body, "requested_power_class")
				assert.NotContains(t, body, "requested_profile")
			})
		}
	}
}

func TestExplicitOperatorMinPowerSuppressesDifficultyInference(t *testing.T) {
	for _, entryPoint := range []string{"work", "try"} {
		t.Run(entryPoint, func(t *testing.T) {
			result := runProductionRoutingEntryPoint(t, entryPoint, setProductionDifficulty("hard"), []string{"--min-power", "4"}, nil, nil)
			require.NotEmpty(t, result.requests, "output=%q err=%v", result.output, result.err)
			assert.Equal(t, 4, result.requests[0].MinPower)
			body := routingIntentEventBody(t, result.events)
			assert.Equal(t, "cli", body["routing_intent_source"])
			assert.NotContains(t, body, "inferred_min_power")
			assert.Equal(t, float64(4), body["requested_min_power"])
		})
	}
}

func TestPublicPolicySuppressesDifficultyInference(t *testing.T) {
	const policy = " opaque:fizeau-policy "
	for _, entryPoint := range []string{"work", "try"} {
		t.Run(entryPoint, func(t *testing.T) {
			result := runProductionRoutingEntryPoint(t, entryPoint, setProductionDifficulty("hard"), []string{"--profile", policy}, nil, nil)
			require.NotEmpty(t, result.requests, "output=%q err=%v", result.output, result.err)
			assert.Equal(t, policy, result.requests[0].Policy)
			assert.Zero(t, result.requests[0].MinPower)
			body := routingIntentEventBody(t, result.events)
			assert.Equal(t, policy, body["requested_policy"])
			assert.NotContains(t, body, "inferred_min_power")
		})
	}
}

func TestOperatorRouteConstraintsDoNotSuppressDifficultyInference(t *testing.T) {
	for _, entryPoint := range []string{"work", "try"} {
		t.Run(entryPoint, func(t *testing.T) {
			flags := []string{"--harness", " opaque-harness ", "--provider", " opaque-provider ", "--model", " opaque-model ", "--max-power", "10"}
			result := runProductionRoutingEntryPoint(t, entryPoint, setProductionDifficulty("hard"), flags, nil, nil)
			require.NotEmpty(t, result.requests, "output=%q err=%v", result.output, result.err)
			req := result.requests[0]
			assert.Equal(t, 9, req.MinPower)
			assert.Equal(t, 10, req.MaxPower)
			assert.Equal(t, " opaque-harness ", req.Harness)
			assert.Equal(t, " opaque-provider ", req.Provider)
			assert.Equal(t, " opaque-model ", req.Model)
			assert.Empty(t, req.Policy)
		})
	}

	for _, entryPoint := range []string{"work", "try"} {
		t.Run(entryPoint+"_conflict", func(t *testing.T) {
			result := runProductionRoutingEntryPoint(t, entryPoint, setProductionDifficulty("hard"), []string{"--max-power", "8"}, nil, nil)
			assert.Empty(t, result.requests)
			assert.Contains(t, result.output+errorString(result.err), "inferred MinPower 9 conflicts with requested MaxPower 8")
		})
	}
}

func TestReadinessDifficultyAffectsCurrentDispatchWithoutBeadMutation(t *testing.T) {
	for _, entryPoint := range []string{"work", "try"} {
		for _, authored := range []bool{false, true} {
			name := entryPoint + "/readiness"
			if authored {
				name = entryPoint + "/authored_wins"
			}
			t.Run(name, func(t *testing.T) {
				result := runProductionRoutingEntryPoint(t, entryPoint, func(target *bead.Bead) {
					target.Extra = map[string]any{"unrelated": "preserved"}
					if authored {
						target.Extra[escalation.BeadEstimatedDifficultyKey] = string(escalation.DifficultyEasy)
					}
				}, nil, &productionRoutingHookRunner{difficulty: "hard"}, nil)
				require.NotEmpty(t, result.requests, "output=%q err=%v", result.output, result.err)
				want := 9
				wantSource := "readiness"
				if authored {
					want = 0
					wantSource = "bead_hint"
				}
				assert.Equal(t, want, result.requests[0].MinPower)
				assert.Equal(t, "preserved", result.bead.Extra["unrelated"])
				if !authored {
					assert.NotContains(t, result.bead.Extra, escalation.BeadEstimatedDifficultyKey)
					assert.NotContains(t, result.bead.Extra, "triage.power_hint")
					assert.NotContains(t, result.bead.Extra, "work-next-min-power")
					assert.NotContains(t, result.bead.Extra, "routing.min_power")
					assert.NotContains(t, result.bead.Extra, "inferred_min_power")
				}
				body := routingIntentEventBody(t, result.events)
				assert.Equal(t, wantSource, body["routing_intent_source"])
				assert.Equal(t, float64(want), body["inferred_min_power"])
			})
		}
	}
}

func TestExecutionRoutingIntentRecordsInferredMinPower(t *testing.T) {
	for _, tc := range []struct {
		name            string
		difficulty      string
		flags           []string
		wantMin         int
		wantInferred    int
		inferredPresent bool
		wantPolicy      string
	}{
		{name: "easy_present_zero", difficulty: "easy", flags: []string{"--max-power", "10"}, wantMin: 0, wantInferred: 0, inferredPresent: true},
		{name: "hard_nine", difficulty: "hard", flags: []string{"--max-power", "10"}, wantMin: 9, wantInferred: 9, inferredPresent: true},
		{name: "policy_suppression", difficulty: "hard", flags: []string{"--profile", "opaque-policy", "--max-power", "10"}, wantMin: 0, wantPolicy: "opaque-policy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runProductionRoutingEntryPoint(t, "work", setProductionDifficulty(tc.difficulty), tc.flags, nil, nil)
			require.NotEmpty(t, result.requests)
			body := routingIntentEventBody(t, result.events)
			value, present := body["inferred_min_power"]
			assert.Equal(t, tc.inferredPresent, present)
			if present {
				assert.Equal(t, float64(tc.wantInferred), value)
			}
			assert.Equal(t, float64(tc.wantMin), body["requested_min_power"])
			assert.Equal(t, float64(10), body["requested_max_power"])
			assert.Equal(t, tc.wantPolicy, body["requested_policy"])
			assert.Equal(t, "fizeau-chosen-harness", body["actual_harness"])
			assert.Equal(t, "fizeau-chosen-provider", body["actual_provider"])
			assert.Equal(t, "fizeau-chosen-model", body["actual_model"])
			assert.Equal(t, float64(8), body["actual_power"])
			assert.NotContains(t, body, "requested_power_class")
			assert.NotContains(t, body, "requested_profile")
		})
	}
}

func TestReturnedRouteIdentityIsEvidenceOnly(t *testing.T) {
	identities := []struct{ harness, provider, model string }{
		{harness: "returned-harness-a", provider: "returned-provider-a", model: "returned-model-a"},
		{harness: "returned-harness-b", provider: "returned-provider-b", model: "returned-model-b"},
	}
	for _, failure := range []struct {
		name       string
		error      string
		alwaysFail bool
		flags      []string
	}{
		{name: "semantic_retry", error: "build failed"},
		{name: "connectivity_retry", error: "provider request failed: connect: connection refused"},
		{name: "outage_exhaustion", error: "provider 503 service unavailable", alwaysFail: true, flags: []string{"--max-power", "8"}},
	} {
		t.Run(failure.name, func(t *testing.T) {
			var baselineMinPowers []int
			var baselineStatus string
			var baselineTriageSummaries []string
			for variant, identity := range identities {
				t.Run(fmt.Sprintf("identity_%d", variant), func(t *testing.T) {
					runner := &productionRoutingHookRunner{forbiddenTriageStrings: []string{identity.harness, identity.provider, identity.model}}
					calls := 0
					executeFn := func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
						calls++
						ch := make(chan agentlib.ServiceEvent, 1)
						if calls == 1 || failure.alwaysFail {
							payload := fmt.Sprintf(`{"status":"error","exit_code":1,"error":%q,"routing_actual":{"harness":%q,"provider":%q,"model":%q,"power":7}}`, failure.error, identity.harness, identity.provider, identity.model)
							ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(payload)}
						} else {
							payload := fmt.Sprintf(`{"status":"success","final_text":"ok","routing_actual":{"harness":%q,"provider":%q,"model":%q,"power":8}}`, identity.harness, identity.provider, identity.model)
							ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(payload)}
						}
						close(ch)
						return ch, nil
					}
					result := runProductionRoutingEntryPoint(t, "work", nil, failure.flags, runner, executeFn)
					assert.Empty(t, runner.triageLeak)
					require.NotEmpty(t, result.requests, "output=%q err=%v", result.output, result.err)
					minPowers := make([]int, 0, len(result.requests))
					for _, req := range result.requests {
						minPowers = append(minPowers, req.MinPower)
						assert.Empty(t, req.Harness)
						assert.Empty(t, req.Provider)
						assert.Empty(t, req.Model)
						assert.Empty(t, req.Policy)
					}
					if variant == 0 {
						baselineMinPowers = minPowers
						baselineStatus = result.bead.Status
						baselineTriageSummaries = eventSummaries(result.events, "bead-quality.triage")
					} else {
						assert.Equal(t, baselineMinPowers, minPowers, "identity changed retry decision")
						assert.Equal(t, baselineStatus, result.bead.Status, "identity changed terminal decision")
						assert.Equal(t, baselineTriageSummaries, eventSummaries(result.events, "bead-quality.triage"), "identity changed triage decision")
					}
					body := routingIntentEventBody(t, result.events)
					assert.Equal(t, identity.harness, body["actual_harness"])
					assert.Equal(t, identity.provider, body["actual_provider"])
					assert.Equal(t, identity.model, body["actual_model"])
				})
			}
		})
	}

	t.Run("triage", func(t *testing.T) {
		var baselineStatus string
		var baselineSummaries []string
		var baselineClassifications []string
		for variant, identity := range identities {
			runner := &productionRoutingHookRunner{
				close:                  true,
				forbiddenTriageStrings: []string{identity.harness, identity.provider, identity.model},
			}
			executeFn := func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
				ch := make(chan agentlib.ServiceEvent, 1)
				payload := fmt.Sprintf(`{"status":"error","exit_code":1,"error":"build failed","routing_actual":{"harness":%q,"provider":%q,"model":%q,"power":7}}`, identity.harness, identity.provider, identity.model)
				ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(payload)}
				close(ch)
				return ch, nil
			}
			result := runProductionRoutingEntryPoint(t, "work", nil, []string{"--max-power", "8"}, runner, executeFn)
			assert.Empty(t, runner.triageLeak)
			require.NotEmpty(t, runner.triagePrompts)
			require.Len(t, result.requests, 1)
			summaries := eventSummaries(result.events, "bead-quality.triage")
			classifications := eventBodyStringValues(t, result.events, "bead-quality.triage", "classification")
			require.NotEmpty(t, summaries)
			require.Equal(t, []string{"already_satisfied"}, classifications)
			if variant == 0 {
				baselineStatus = result.bead.Status
				baselineSummaries = summaries
				baselineClassifications = classifications
			} else {
				assert.Equal(t, baselineStatus, result.bead.Status)
				assert.Equal(t, baselineSummaries, summaries)
				assert.Equal(t, baselineClassifications, classifications)
			}
			assert.Empty(t, result.requests[0].Harness)
			assert.Empty(t, result.requests[0].Provider)
			assert.Empty(t, result.requests[0].Model)
		}
	})

	t.Run("close", func(t *testing.T) {
		for _, identity := range identities {
			executeFn := func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
				attemptID := req.Metadata["attempt_id"]
				require.NotEmpty(t, attemptID)
				bundleDir := filepath.Join(req.WorkDir, ddxroot.DirName, "executions", attemptID)
				require.NoError(t, os.MkdirAll(bundleDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "no_changes_rationale.txt"), []byte("verification_command: true\n"), 0o644))
				ch := make(chan agentlib.ServiceEvent, 1)
				payload := fmt.Sprintf(`{"status":"success","final_text":"ok","routing_actual":{"harness":%q,"provider":%q,"model":%q,"power":8}}`, identity.harness, identity.provider, identity.model)
				ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(payload)}
				close(ch)
				return ch, nil
			}
			result := runProductionRoutingEntryPoint(t, "work", nil, nil, nil, executeFn)
			require.Len(t, result.requests, 1)
			assert.Equal(t, bead.StatusClosed, result.bead.Status)
			assert.Empty(t, result.requests[0].Harness)
			assert.Empty(t, result.requests[0].Provider)
			assert.Empty(t, result.requests[0].Model)
		}
	})
}

func eventSummaries(events []bead.BeadEvent, kind string) []string {
	var summaries []string
	for _, event := range events {
		if event.Kind == kind {
			summaries = append(summaries, event.Summary)
		}
	}
	return summaries
}

func eventBodyStringValues(t *testing.T, events []bead.BeadEvent, kind, field string) []string {
	t.Helper()
	var values []string
	for _, event := range events {
		if event.Kind != kind {
			continue
		}
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
		if value, ok := body[field].(string); ok {
			values = append(values, value)
		}
	}
	return values
}

func TestServerManagedDifficultyInferencePreservesMinPowerPresence(t *testing.T) {
	for _, tc := range []struct {
		name        string
		explicitSet bool
		want        int
	}{
		{name: "omitted_infers_hard", want: 9},
		{name: "explicit_zero_suppresses", explicitSet: true, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
			stub := installExecuteCapturingStub(t)
			runner := &productionRoutingHookRunner{}
			stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
				if req.Role != "implementer" {
					return productionRoutingHookExecute(req, runner)
				}
				return routeDelegationExecute(req)
			}
			beadID := "ddx-managed-routing-" + tc.name
			projectRoot := newRouteDelegationProject(t, beadID)
			store := bead.NewStore(ddxroot.JoinProject(projectRoot))
			require.NoError(t, store.Update(context.Background(), beadID, setProductionDifficulty("hard")))
			spec := serverpkg.ExecuteLoopWorkerSpec{
				ProjectRoot: projectRoot,
				Mode:        "once",
				NoReview:    true,
				MinPowerSet: tc.explicitSet,
			}
			args := serverpkg.ManagedWorkerCommandArgs(spec, "worker-test")
			factory := NewCommandFactory(projectRoot)
			output, err := executeCommand(factory.NewRootCommand(), args...)
			requests := capturedImplementationRequests(stub)
			require.NotEmpty(t, requests, "output=%q err=%v args=%v", output, err, args)
			assert.Equal(t, tc.want, requests[0].MinPower)
		})
	}
}

func setProductionDifficulty(difficulty string) func(*bead.Bead) {
	return func(target *bead.Bead) {
		if target.Extra == nil {
			target.Extra = map[string]any{}
		}
		target.Extra[escalation.BeadEstimatedDifficultyKey] = difficulty
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestProductionRoutingHelperDoesNotMutateInputSlices(t *testing.T) {
	// Guard the test harness itself: flag slices are reused by several table
	// cases and must remain stable or byte-preservation assertions become weak.
	flags := []string{"--model", " opaque-model "}
	want := append([]string(nil), flags...)
	_ = runProductionRoutingEntryPoint(t, "work", nil, flags, nil, nil)
	assert.True(t, reflect.DeepEqual(want, flags))
}
