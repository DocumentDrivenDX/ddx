package agent

// passthrough_envelope_test.go verifies the AC3, AC5, AC6, and AC7 requirements
// from bead ddx-20047dd5: the AgentPassthrough envelope is invariant under
// power escalation, routing evidence records requested constraints separately,
// invalid passthrough values are not pre-validated by DDx, and
// passthrough+power conflicts surface as typed failure modes rather than
// mutating or widening the pins.

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// passthroughTestService is a minimal FizeauService stub that records the most
// recent Execute call's request and returns a synthetic success event.
type passthroughTestService struct {
	executeCalled       bool
	lastReq             agentlib.ServiceExecuteRequest
	listHarnessesCalled bool
	listModels          []agentlib.ModelInfo
	listPolicies        []agentlib.PolicyInfo
	executeEvents       []agentlib.ServiceEvent
	// executeErr, when non-nil, is returned as the pre-dispatch error from
	// Execute so tests can exercise typed provider-failure classification
	// (ddx-3b721804) without a real agent server.
	executeErr error
	// harnessInfos, when non-nil, overrides the harness list returned by
	// ListHarnesses. Lets tests report a harness as available + subscription
	// so seedRecentRouteAttemptsFromTracker skips exclusion-seeding for it.
	harnessInfos  []agentlib.HarnessInfo
	routeAttempts []agentlib.RouteAttempt
	// routeAttemptsAtExecute is the len(routeAttempts) at the moment Execute
	// is first invoked; route-health seed entries land at indices
	// [0, routeAttemptsAtExecute) and post-run entries (recordServiceRouteAttempt)
	// land at indices >= routeAttemptsAtExecute. Lets tests verify the seed
	// happens before the dispatched Execute call.
	routeAttemptsAtExecute int
}

func (s *passthroughTestService) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	if !s.executeCalled {
		s.routeAttemptsAtExecute = len(s.routeAttempts)
	}
	s.executeCalled = true
	s.lastReq = req
	if s.executeErr != nil {
		return nil, s.executeErr
	}
	ch := make(chan agentlib.ServiceEvent, len(s.executeEvents)+1)
	if len(s.executeEvents) > 0 {
		for _, evt := range s.executeEvents {
			ch <- evt
		}
	} else {
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
	}
	close(ch)
	return ch, nil
}

func (s *passthroughTestService) ListHarnesses(ctx context.Context) ([]agentlib.HarnessInfo, error) {
	s.listHarnessesCalled = true
	if s.harnessInfos != nil {
		return append([]agentlib.HarnessInfo(nil), s.harnessInfos...), nil
	}
	return []agentlib.HarnessInfo{{Name: "claude", Available: true}}, nil
}

func (s *passthroughTestService) TailSessionLog(ctx context.Context, sessionID string) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}

func (s *passthroughTestService) ListProviders(ctx context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}

func (s *passthroughTestService) ListModels(ctx context.Context, filter agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	return append([]agentlib.ModelInfo(nil), s.listModels...), nil
}

func (s *passthroughTestService) ListPolicies(ctx context.Context) ([]agentlib.PolicyInfo, error) {
	return append([]agentlib.PolicyInfo(nil), s.listPolicies...), nil
}

func (s *passthroughTestService) HealthCheck(ctx context.Context, target agentlib.HealthTarget) error {
	return nil
}

func (s *passthroughTestService) ResolveRoute(ctx context.Context, req agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	return nil, nil
}

func (s *passthroughTestService) RecordRouteAttempt(ctx context.Context, attempt agentlib.RouteAttempt) error {
	s.routeAttempts = append(s.routeAttempts, attempt)
	return nil
}

func (s *passthroughTestService) RouteStatus(ctx context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}

func (s *passthroughTestService) ListSessionLogs(ctx context.Context) ([]agentlib.SessionLogEntry, error) {
	return nil, nil
}

func (s *passthroughTestService) WriteSessionLog(ctx context.Context, sessionID string, w io.Writer) error {
	return nil
}

func (s *passthroughTestService) ReplaySession(ctx context.Context, sessionID string, w io.Writer) error {
	return nil
}

func (s *passthroughTestService) UsageReport(ctx context.Context, opts agentlib.UsageReportOptions) (*agentlib.UsageReport, error) {
	return nil, nil
}

// resolvedWithPassthrough builds a sealed ResolvedConfig with the given
// harness/provider/model/minPower/maxPower and no other overrides.
func resolvedWithPassthrough(harness, provider, model string, minPower, maxPower int) config.ResolvedConfig {
	cfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Model: model,
	})
	return cfg.Resolve(config.CLIOverrides{
		Harness:  harness,
		Provider: provider,
		MinPower: minPower,
		MaxPower: maxPower,
	})
}

// TestPassthroughEnvelope_InvariantUnderPowerEscalation (AC3): bumping MinPower
// does not mutate harness/provider/model in the passthrough.
func TestPassthroughEnvelope_InvariantUnderPowerEscalation(t *testing.T) {
	rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-5-sonnet", 0, 0)
	pt := rcfg.Passthrough()

	// Simulate retry escalation: produce a new ResolvedConfig with higher MinPower
	// but the same harness/provider/model. The passthrough must stay the same.
	escalated := resolvedWithPassthrough("claude", "anthropic", "claude-3-5-sonnet", 50, 100)
	ptEscalated := escalated.Passthrough()

	if ptEscalated.Harness != pt.Harness {
		t.Errorf("Harness changed under escalation: got %q, want %q", ptEscalated.Harness, pt.Harness)
	}
	if ptEscalated.Provider != pt.Provider {
		t.Errorf("Provider changed under escalation: got %q, want %q", ptEscalated.Provider, pt.Provider)
	}
	if ptEscalated.Model != pt.Model {
		t.Errorf("Model changed under escalation: got %q, want %q", ptEscalated.Model, pt.Model)
	}

	// MaxPower must be preserved too.
	if escalated.MaxPower() != 100 {
		t.Errorf("MaxPower should be 100 after escalation, got %d", escalated.MaxPower())
	}
	if escalated.MinPower() != 50 {
		t.Errorf("MinPower should be 50 after escalation, got %d", escalated.MinPower())
	}
}

// TestPassthroughEnvelope_EmptyIsPreserved (AC3): zero-value passthrough stays
// zero across Resolve — no DDx-side defaulting injects harness/provider/model.
func TestPassthroughEnvelope_EmptyIsPreserved(t *testing.T) {
	cfg := config.NewTestConfigForRun(config.TestRunConfigOpts{})
	rcfg := cfg.Resolve(config.CLIOverrides{})
	pt := rcfg.Passthrough()

	if pt.Harness != "" || pt.Provider != "" || pt.Model != "" {
		t.Errorf("empty passthrough unexpectedly populated: %+v", pt)
	}
}

// TestExecuteOnService_InvalidPassthroughNotPrevalidated (AC6): DDx must NOT
// reject an obviously bogus harness/provider/model before forwarding to the
// service. The service is the authoritative validator.
func TestExecuteOnService_InvalidPassthroughNotPrevalidated(t *testing.T) {
	svc := &passthroughTestService{}
	rcfg := resolvedWithPassthrough("definitely-not-a-real-harness!!!", "bogus-provider", "bogus-model-xyz", 0, 0)

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	// DDx must not return an error from pre-validation; the service stub
	// returns success so we expect no error here.
	if err != nil {
		t.Errorf("DDx pre-validated passthrough and returned error (must not): %v", err)
	}
	if !svc.executeCalled {
		t.Error("Execute was never called — DDx must forward invalid passthrough to service unchanged")
	}
	if svc.lastReq.Harness != "definitely-not-a-real-harness!!!" {
		t.Errorf("Harness was mutated before forwarding: got %q", svc.lastReq.Harness)
	}
	if svc.lastReq.Provider != "bogus-provider" {
		t.Errorf("Provider was mutated before forwarding: got %q", svc.lastReq.Provider)
	}
	if svc.lastReq.Model != "bogus-model-xyz" {
		t.Errorf("Model was mutated before forwarding: got %q", svc.lastReq.Model)
	}
}

// TestExecuteOnService_PassthroughReachesServiceRequest (AC4/AC6): Harness,
// Provider, and Model in ServiceExecuteRequest come exclusively from the
// passthrough envelope, not from ad-hoc rcfg.Harness()/rcfg.Provider() calls.
func TestExecuteOnService_PassthroughReachesServiceRequest(t *testing.T) {
	svc := &passthroughTestService{}
	rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-7-sonnet", 0, 0)

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.lastReq.Harness != "claude" {
		t.Errorf("ServiceExecuteRequest.Harness = %q, want %q", svc.lastReq.Harness, "claude")
	}
	if svc.lastReq.Provider != "anthropic" {
		t.Errorf("ServiceExecuteRequest.Provider = %q, want %q", svc.lastReq.Provider, "anthropic")
	}
	if svc.lastReq.Model != "claude-3-7-sonnet" {
		t.Errorf("ServiceExecuteRequest.Model = %q, want %q", svc.lastReq.Model, "claude-3-7-sonnet")
	}
}

// TestExecuteOnService_RoleAndCorrelationIDReachServiceRequest verifies the
// new top-level ExecuteRequest fields used by execute-bead land in the public
// service request unchanged.
func TestExecuteOnService_RoleAndCorrelationIDReachServiceRequest(t *testing.T) {
	svc := &passthroughTestService{}
	rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-7-sonnet", 0, 0)

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt:        "hello",
		Role:          "implementer",
		CorrelationID: "ddx-bead-1:attempt-2",
	})
	require.NoError(t, err)
	if svc.lastReq.Role != "implementer" {
		t.Fatalf("ServiceExecuteRequest.Role = %q, want implementer", svc.lastReq.Role)
	}
	if svc.lastReq.CorrelationID != "ddx-bead-1:attempt-2" {
		t.Fatalf("ServiceExecuteRequest.CorrelationID = %q, want ddx-bead-1:attempt-2", svc.lastReq.CorrelationID)
	}
}

func TestExecuteOnService_PromptFeaturesReachServiceRequest(t *testing.T) {
	svc := &passthroughTestService{}
	rcfg := resolvedWithPassthrough("", "", "", 0, 0)

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt:                "hello routing features",
		EstimatedPromptTokens: 123,
		RequiresTools:         true,
	})
	require.NoError(t, err)
	assert.Equal(t, 123, svc.lastReq.EstimatedPromptTokens)
	assert.True(t, svc.lastReq.RequiresTools)
}

func TestExecuteOnService_EstimatesPromptTokensWhenRuntimeOmitsEstimate(t *testing.T) {
	svc := &passthroughTestService{}
	rcfg := resolvedWithPassthrough("", "", "", 0, 0)

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello routing features",
	})
	require.NoError(t, err)
	assert.Greater(t, svc.lastReq.EstimatedPromptTokens, 0)
}

// TestExecuteOnService_IgnoresToolCallTranscriptProjection verifies the
// service path keeps Fizeau tool_call/tool_result events opaque rather than
// reconstructing a DDx tool transcript from them.
func TestExecuteOnService_IgnoresToolCallTranscriptProjection(t *testing.T) {
	svc := &passthroughTestService{
		executeEvents: []agentlib.ServiceEvent{
			{Type: "tool_call", Data: []byte(`{"id":"call-1","name":"Read","input":{"path":"README.md"}}`)},
			{Type: "tool_result", Data: []byte(`{"id":"call-1","output":"ok","duration_ms":1}`)},
			{Type: "final", Data: []byte(`{"status":"success","exit_code":0,"final_text":"done","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3},"cost_usd":0.25}`)},
		},
	}
	rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-7-sonnet", 0, 0)

	result, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.Output != "done" {
		t.Fatalf("Result.Output = %q, want %q", result.Output, "done")
	}
	if result.Tokens != 3 || result.InputTokens != 1 || result.OutputTokens != 2 {
		t.Fatalf("token projection mismatch: %+v", result)
	}
	if result.CostUSD != 0.25 {
		t.Fatalf("Result.CostUSD = %v, want 0.25", result.CostUSD)
	}
	if len(result.ToolCalls) != 0 {
		t.Fatalf("expected no reconstructed tool transcript, got %+v", result.ToolCalls)
	}
}

func TestExecuteOnService_FinalErrorWithZeroExitBecomesFailure(t *testing.T) {
	svc := &passthroughTestService{
		executeEvents: []agentlib.ServiceEvent{
			{Type: "final", Data: []byte(`{"status":"error","exit_code":0,"error":"ResolveRoute: no viable routing candidate: 3 candidates rejected"}`)},
		},
	}
	rcfg := resolvedWithPassthrough("", "", "", 0, 0)

	result, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ExitCode)
	assert.Contains(t, result.Error, "ResolveRoute: no viable routing candidate")
}

func TestExecuteOnService_RecordsFailedRouteAttempt(t *testing.T) {
	finalPayload, err := json.Marshal(map[string]any{
		"status":    "error",
		"exit_code": 1,
		"error":     `openai: Post "http://bragi:1234/v1/chat/completions": dial tcp 100.127.38.115:1234: i/o timeout`,
		"routing_actual": map[string]any{
			"harness":  "fiz",
			"provider": "bragi",
			"model":    "qwen3.5-27b",
			"power":    5,
		},
	})
	require.NoError(t, err)
	svc := &passthroughTestService{
		executeEvents: []agentlib.ServiceEvent{{Type: "final", Data: finalPayload}},
	}
	rcfg := resolvedWithPassthrough("", "", "", 0, 0)

	result, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, svc.routeAttempts, 1)
	attempt := svc.routeAttempts[0]
	assert.Equal(t, "failed", attempt.Status)
	assert.Equal(t, "provider_error", attempt.Reason)
	assert.Equal(t, "fiz", attempt.Harness)
	assert.Equal(t, "bragi", attempt.Provider)
	assert.Equal(t, "qwen3.5-27b", attempt.Model)
	assert.Contains(t, attempt.Error, "i/o timeout")
}

func TestSeedRecentRouteAttemptsFromTrackerReplaysConnectivityFailure(t *testing.T) {
	root := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, root)
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{ID: "seed-route-001", Title: "seed route"}))
	now := time.Date(2026, 5, 14, 8, 55, 0, 0, time.UTC)
	require.NoError(t, store.AppendEvent("seed-route-001", bead.BeadEvent{
		Kind:      "route-failure",
		Summary:   "provider=bragi model=qwen3.5-27b connectivity failure",
		Body:      `{"harness":"fiz","provider":"bragi","model":"qwen3.5-27b","error":"dial tcp 100.127.38.115:1234: i/o timeout","outcome_reason":"provider_connectivity"}`,
		CreatedAt: now.Add(-time.Minute),
	}))
	svc := &passthroughTestService{}

	seedRecentRouteAttemptsFromTracker(context.Background(), svc, root, now)

	require.Len(t, svc.routeAttempts, 1)
	assert.Equal(t, "failed", svc.routeAttempts[0].Status)
	assert.Equal(t, FailureModeProviderConnectivity, svc.routeAttempts[0].Reason)
	assert.Equal(t, "bragi", svc.routeAttempts[0].Provider)
	assert.Equal(t, "qwen3.5-27b", svc.routeAttempts[0].Model)
}

func TestSeedRecentRouteAttemptsFromTrackerReplaysFailedRouteExtra(t *testing.T) {
	root := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, root)
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	now := time.Date(2026, 5, 14, 8, 55, 0, 0, time.UTC)
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    "seed-extra-001",
		Title: "seed extra",
		Extra: map[string]any{
			executeLoopFailedRoutesKey: []FailedRouteEntry{{
				Provider:    "bragi",
				Model:       "qwen3.5-27b",
				ActualPower: 5,
				Reason:      FailureModeProviderConnectivity,
				At:          now.Add(-time.Minute).Format(time.RFC3339),
			}},
		},
	}))
	svc := &passthroughTestService{}

	seedRecentRouteAttemptsFromTracker(context.Background(), svc, root, now)

	require.Len(t, svc.routeAttempts, 1)
	assert.Equal(t, "failed", svc.routeAttempts[0].Status)
	assert.Equal(t, "bragi", svc.routeAttempts[0].Provider)
	assert.Equal(t, "qwen3.5-27b", svc.routeAttempts[0].Model)
}

// TestSeedExclusionsSkipsAvailableSubscriptionHarness reproduces the
// no_viable_provider regression: during a local-fleet outage, transient
// connectivity blips on a subscription harness (claude-tui) were recorded in the
// tracker and replayed as HARD route exclusions. With local providers excluded
// by config and openrouter blocked, excluding the only live subscription
// harness empties the candidate set. The seed must replay the unreachable
// local provider's failure but MUST NOT replay an available subscription
// harness's blip.
func TestSeedExclusionsSkipsAvailableSubscriptionHarness(t *testing.T) {
	root := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, root)
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{ID: "seed-sub-001", Title: "seed subscription"}))
	now := time.Date(2026, 5, 29, 8, 55, 0, 0, time.UTC)

	// Transient connectivity blip on an available subscription harness (claude-tui).
	require.NoError(t, store.AppendEvent("seed-sub-001", bead.BeadEvent{
		Kind:      "route-failure",
		Summary:   "provider_connectivity claude-tui",
		Body:      `{"harness":"claude-tui","provider":"claude-tui","model":"opus-4.7","error":"transient connection reset","outcome_reason":"provider_connectivity"}`,
		CreatedAt: now.Add(-time.Minute),
	}))
	// Unreachable local/HTTP provider: still a meaningful exclusion.
	require.NoError(t, store.AppendEvent("seed-sub-001", bead.BeadEvent{
		Kind:      "route-failure",
		Summary:   "provider_connectivity bragi",
		Body:      `{"harness":"fiz","provider":"bragi","model":"qwen3.5-27b","error":"dial tcp 100.127.38.115:1234: i/o timeout","outcome_reason":"provider_connectivity"}`,
		CreatedAt: now.Add(-time.Minute),
	}))

	svc := &passthroughTestService{
		harnessInfos: []agentlib.HarnessInfo{
			{Name: "claude-tui", Available: true, Billing: agentlib.BillingModelSubscription},
		},
	}

	seedRecentRouteAttemptsFromTracker(context.Background(), svc, root, now)

	var sawClaudeTUI, sawBragi bool
	for _, a := range svc.routeAttempts {
		if a.Provider == "claude-tui" || a.Harness == "claude-tui" {
			sawClaudeTUI = true
		}
		if a.Provider == "bragi" {
			sawBragi = true
		}
	}
	assert.False(t, sawClaudeTUI, "available subscription harness (claude-tui) must not be seeded as a route exclusion; got %+v", svc.routeAttempts)
	assert.True(t, sawBragi, "unreachable local provider (bragi) must still be seeded as a route exclusion; got %+v", svc.routeAttempts)
}

// TestExecutePolicySeedsRouteHealthFromTracker (ddx-d7c56c1b AC1) reproduces
// the production failure: tracker evidence for a recent provider_connectivity
// failure on bragi/qwen3.5-27b must be replayed into fizeau's route-health
// store with a fresh-enough timestamp that fizeau's default 30s TTL still
// considers it active when the routing engine reads ActiveAttempts. DDx's
// ProviderUnavailableCooldown is 15 min; without rebasing the timestamp the
// historical event is immediately expired by fizeau and policy/default routes
// keep selecting the failed provider.
func TestExecutePolicySeedsRouteHealthFromTracker(t *testing.T) {
	root := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, root)
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{ID: "seed-policy-001", Title: "seed policy"}))

	now := time.Now().UTC()
	// 5 minutes old: well inside DDx's 15-min replay window but well outside
	// fizeau's default 30s route-health TTL. This is the exact gap that lets
	// dial-class failures keep re-burning the same provider in production.
	failureAt := now.Add(-5 * time.Minute)
	require.NoError(t, store.AppendEvent("seed-policy-001", bead.BeadEvent{
		Kind:      "route-failure",
		Summary:   "provider_connectivity bragi/qwen3.5-27b",
		Body:      `{"harness":"fiz","provider":"bragi","model":"qwen3.5-27b","error":"dial tcp 100.127.38.115:1234: i/o timeout","outcome_reason":"provider_connectivity"}`,
		CreatedAt: failureAt,
	}))

	svc := &passthroughTestService{}
	rcfg := resolvedWithPassthrough("fiz", "", "", 0, 0)

	// Drive executeOnService end-to-end: seed must run before Execute is
	// dispatched, with the request still policy-driven (no provider/model pin
	// injected by DDx — AC3).
	_, err := executeOnService(context.Background(), svc, root, rcfg, AgentRunRuntime{Prompt: "hello"})
	require.NoError(t, err)

	require.True(t, svc.executeCalled, "executeOnService must dispatch Execute")
	require.GreaterOrEqual(t, svc.routeAttemptsAtExecute, 1, "seed must record a route-health attempt before Execute is called")

	seeded := svc.routeAttempts[:svc.routeAttemptsAtExecute]
	var bragi *agentlib.RouteAttempt
	for i := range seeded {
		if seeded[i].Provider == "bragi" && seeded[i].Model == "qwen3.5-27b" {
			bragi = &seeded[i]
			break
		}
	}
	require.NotNil(t, bragi, "tracker evidence for bragi/qwen3.5-27b must be replayed before Execute; got %+v", seeded)
	assert.Equal(t, "failed", bragi.Status)
	assert.Equal(t, FailureModeProviderConnectivity, bragi.Reason)

	// The DDx-side route-health hard-gate only works if the replayed
	// timestamp survives fizeau's default TTL window. routehealth.DefaultCooldown
	// is 30s; if DDx replays the historical timestamp verbatim, fizeau's
	// ActiveAttempts immediately expires the record and policy routing happily
	// re-picks the failed provider. The rebase to `now` is the contract.
	require.False(t, bragi.Timestamp.IsZero(), "Timestamp must be set")
	age := now.Sub(bragi.Timestamp)
	assert.LessOrEqual(t, age, 30*time.Second,
		"replayed RouteAttempt.Timestamp must fall within fizeau's default 30s route-health TTL so the hard-gate is honored on the next routing decision; got age %v (original event at %v)",
		age, failureAt)

	// AC3: the dispatched request must remain policy-driven — DDx must not
	// have pinned a provider/model in response to the failure.
	assert.Equal(t, "fiz", svc.lastReq.Harness)
	assert.Empty(t, svc.lastReq.Provider, "DDx must not pin a provider in response to tracker evidence")
	assert.Empty(t, svc.lastReq.Model, "DDx must not pin a model in response to tracker evidence")
	assert.Zero(t, svc.lastReq.MinPower, "DDx must not raise MinPower floor in response to tracker evidence")
}

// TestRecordRouteAttemptRouteHealthGatesPolicyExecute (ddx-d7c56c1b AC2)
// proves the seed-then-dispatch ordering executeOnService uses for
// policy/default Execute: every Execute call first runs the route-health seed
// from tracker evidence, then issues the actual Execute. This is the
// DDx-side contract for AC2; fizeau v0.13.1 service_routing.go:102 already
// applies route-attempt cooldowns inside ResolveRoute (which Execute's
// under-specified path delegates to), so honoring the seed end-to-end through
// Execute requires only that DDx populate the store before each dispatch.
func TestRecordRouteAttemptRouteHealthGatesPolicyExecute(t *testing.T) {
	root := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, root)
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    "hardgate-001",
		Title: "hard-gate via failed-route extra",
		Extra: map[string]any{
			executeLoopFailedRoutesKey: []FailedRouteEntry{{
				Provider:    "bragi",
				Model:       "qwen3.5-27b",
				ActualPower: 5,
				Reason:      FailureModeProviderConnectivity,
				At:          time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339),
			}},
		},
	}))

	svc := &passthroughTestService{}
	// Policy-driven request: no provider/model/min-power pin. The under-specified
	// Execute path delegates to ResolveRoute which applies route-attempt
	// cooldowns (fizeau v0.13.1 service_routing.go:102 -> applyRouteAttemptCooldowns).
	rcfg := resolvedWithPassthrough("fiz", "", "", 0, 0)

	_, err := executeOnService(context.Background(), svc, root, rcfg, AgentRunRuntime{Prompt: "hello"})
	require.NoError(t, err)

	require.True(t, svc.executeCalled)
	require.GreaterOrEqual(t, svc.routeAttemptsAtExecute, 1, "RecordRouteAttempt must fire before Execute so the route-health hard-gate is in place")

	// The pre-Execute seed must carry the failed marker, not a success — a
	// success would clear the failure in routehealth.Store.
	preExecute := svc.routeAttempts[:svc.routeAttemptsAtExecute]
	var bragiSeed *agentlib.RouteAttempt
	for i := range preExecute {
		if preExecute[i].Provider == "bragi" && preExecute[i].Model == "qwen3.5-27b" {
			bragiSeed = &preExecute[i]
			break
		}
	}
	require.NotNil(t, bragiSeed, "failed-route extra for bragi/qwen3.5-27b must be seeded before Execute")
	assert.Equal(t, "failed", bragiSeed.Status)
	assert.Equal(t, FailureModeProviderConnectivity, bragiSeed.Reason)

	// Rebased timestamp survives fizeau's 30s default TTL.
	now := time.Now().UTC()
	require.False(t, bragiSeed.Timestamp.IsZero())
	assert.LessOrEqual(t, now.Sub(bragiSeed.Timestamp), 30*time.Second,
		"seeded Timestamp must fall within fizeau's default route-health TTL so policy Execute hard-gates the failed provider")

	// AC3: the dispatched request stays policy-driven; DDx must not introduce
	// a provider/model pin or a hardcoded MinPower floor in response to the
	// failed-route extra.
	assert.Empty(t, svc.lastReq.Provider)
	assert.Empty(t, svc.lastReq.Model)
	assert.Zero(t, svc.lastReq.MinPower)
}

// TestServiceRun_ForwardsOpaqueFizeauEvents verifies that a future/unknown
// service event type does not disturb the final projection path. DDx should
// pass through the event stream without trying to interpret or rewrite the
// opaque payload.
func TestServiceRun_ForwardsOpaqueFizeauEvents(t *testing.T) {
	opaque := []byte(`{"future_field":"keep-me","nested":{"count":3}}`)
	svc := &passthroughTestService{
		executeEvents: []agentlib.ServiceEvent{
			{Type: "future.event", Data: opaque},
			{Type: "final", Data: []byte(`{"status":"success","exit_code":0,"final_text":"done"}`)},
		},
	}
	rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-7-sonnet", 0, 0)

	result, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	if result.Output != "done" {
		t.Fatalf("Result.Output = %q, want done", result.Output)
	}
	if len(result.ToolCalls) != 0 {
		t.Fatalf("expected no reconstructed tool transcript, got %+v", result.ToolCalls)
	}
}

// TestServiceRun_FinalResultProjectionOnly verifies the service adapter reads
// only the final projection fields needed for DDx Result and run indexing.
func TestServiceRun_FinalResultProjectionOnly(t *testing.T) {
	routingPayload, err := json.Marshal(map[string]any{
		"harness":  "fiz",
		"provider": "anthropic",
		"model":    "claude-3-5-sonnet",
		"candidates": []map[string]any{
			{
				"model":                  "claude-3-5-sonnet",
				"eligible":               true,
				"cost_usd_per_1k_tokens": 0.0125,
				"cost_source":            "catalog",
				"components": map[string]any{
					"power":     65,
					"speed_tps": 42.5,
				},
			},
		},
	})
	require.NoError(t, err)

	finalPayload, err := json.Marshal(map[string]any{
		"status":           "success",
		"exit_code":        0,
		"final_text":       "final answer",
		"usage":            map[string]any{"input_tokens": 11, "output_tokens": 22, "total_tokens": 33},
		"cost_usd":         0.25,
		"session_log_path": "/tmp/session.jsonl",
		"routing_actual": map[string]any{
			"harness":  "fiz",
			"provider": "anthropic",
			"model":    "claude-3-5-sonnet",
			"power":    65,
		},
	})
	require.NoError(t, err)

	svc := &passthroughTestService{
		executeEvents: []agentlib.ServiceEvent{
			{Type: "routing_decision", Data: routingPayload},
			{Type: "future.event", Data: []byte(`{"opaque":"value"}`)},
			{Type: "final", Data: finalPayload},
		},
	}
	rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-7-sonnet", 0, 0)

	result, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	if result.Output != "final answer" {
		t.Fatalf("Result.Output = %q, want final answer", result.Output)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Result.ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Tokens != 33 || result.InputTokens != 11 || result.OutputTokens != 22 {
		t.Fatalf("token projection mismatch: %+v", result)
	}
	if result.CostUSD != 0.25 {
		t.Fatalf("Result.CostUSD = %v, want 0.25", result.CostUSD)
	}
	if result.AgentSessionID != "/tmp/session.jsonl" {
		t.Fatalf("Result.AgentSessionID = %q, want /tmp/session.jsonl", result.AgentSessionID)
	}
	if result.Provider != "anthropic" || result.Model != "claude-3-5-sonnet" || result.Harness != "fiz" {
		t.Fatalf("route projection mismatch: %+v", result)
	}
	if result.ActualPower != 65 || result.PredictedPower != 65 {
		t.Fatalf("power projection mismatch: %+v", result)
	}
	if result.PredictedSpeedTPS != 42.5 || result.PredictedCostUSDPer1kTokens != 0.0125 || result.PredictedCostSource != "catalog" {
		t.Fatalf("route economics mismatch: %+v", result)
	}
	if len(result.ToolCalls) != 0 {
		t.Fatalf("expected no reconstructed tool transcript, got %+v", result.ToolCalls)
	}
}

// TestClassifyFailureMode_BlockedByPassthroughConstraint (AC7): error strings
// that indicate a passthrough+power conflict must classify as
// blocked_by_passthrough_constraint, not the generic failure modes.
func TestClassifyFailureMode_BlockedByPassthroughConstraint(t *testing.T) {
	cases := []string{
		"passthrough constraint unsatisfiable: harness=claude min_power=90",
		"passthrough constraint: harness pin incompatible with requested min_power",
		"max_power is less than min_power",
		"harness cannot satisfy power constraint",
		"harness pin incompatible with power bounds",
		"model pin incompatible with power bounds",
		"provider pin incompatible with power bounds",
	}
	for _, msg := range cases {
		got := ClassifyFailureMode("task_failed", 1, msg)
		if got != FailureModeBlockedByPassthroughConstraint {
			t.Errorf("ClassifyFailureMode(%q) = %q, want %q", msg, got, FailureModeBlockedByPassthroughConstraint)
		}
	}
}

// TestClassifyFailureMode_AgentPowerUnsatisfied (AC7): error strings that
// indicate no model meets min_power must classify as agent_power_unsatisfied.
func TestClassifyFailureMode_AgentPowerUnsatisfied(t *testing.T) {
	cases := []string{
		"agent power unsatisfied: no model meets min_power=80",
		"no model meets min_power constraint",
		"no model with power >= 80",
		"minimum power not achievable",
		"min_power constraint cannot be satisfied",
	}
	for _, msg := range cases {
		got := ClassifyFailureMode("task_failed", 1, msg)
		if got != FailureModeAgentPowerUnsatisfied {
			t.Errorf("ClassifyFailureMode(%q) = %q, want %q", msg, got, FailureModeAgentPowerUnsatisfied)
		}
	}
}

// TestClassifyFailureMode_PassthroughModesDistinctFromGenericFailure (AC7):
// passthrough failure modes are not the same as no_viable_provider, timeout,
// auth_error, or unknown — the error must not be misclassified.
func TestClassifyFailureMode_PassthroughModesDistinctFromGenericFailure(t *testing.T) {
	forbiddenModes := []string{
		FailureModeNoViableProvider,
		FailureModeTimeout,
		FailureModeAuthError,
		FailureModeUnknown,
	}

	constraintMsg := "passthrough constraint unsatisfiable: harness=claude"
	powerMsg := "no model meets min_power=80"

	for _, forbidden := range forbiddenModes {
		got := ClassifyFailureMode("task_failed", 1, constraintMsg)
		if got == forbidden {
			t.Errorf("passthrough constraint error misclassified as %q", forbidden)
		}
		got = ClassifyFailureMode("task_failed", 1, powerMsg)
		if got == forbidden {
			t.Errorf("power unsatisfied error misclassified as %q", forbidden)
		}
	}
}

// stubBeadEventAppenderForPassthrough is a minimal BeadEventAppender that
// records all events in memory for inspection.
type stubBeadEventAppenderForPassthrough struct {
	events []bead.BeadEvent
}

func (s *stubBeadEventAppenderForPassthrough) AppendEvent(_ string, evt bead.BeadEvent) error {
	s.events = append(s.events, evt)
	return nil
}

// TestExecuteOnService_MinMaxPowerReachServiceRequest (AC1): Execute must receive
// MinPower and MaxPower from the resolved config in ServiceExecuteRequest so the
// upstream service can enforce power constraints.
func TestExecuteOnService_MinMaxPowerReachServiceRequest(t *testing.T) {
	svc := &passthroughTestService{}
	rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-7-sonnet", 40, 90)

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.lastReq.MinPower != 40 {
		t.Errorf("ServiceExecuteRequest.MinPower = %d, want 40", svc.lastReq.MinPower)
	}
	if svc.lastReq.MaxPower != 90 {
		t.Errorf("ServiceExecuteRequest.MaxPower = %d, want 90", svc.lastReq.MaxPower)
	}
}

// TestFizeauAutoRoutingDefaultWorkRequestLeavesRouteUnpinned verifies that a
// zero-config work/try execution request leaves harness/provider/model/profile
// empty so Fizeau can auto-route while still receiving the requested power
// bounds.
func TestFizeauAutoRoutingDefaultWorkRequestLeavesRouteUnpinned(t *testing.T) {
	svc := &passthroughTestService{}
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{
		MinPower: 40,
		MaxPower: 90,
	})

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	require.NoError(t, err)

	assert.Empty(t, svc.lastReq.Harness)
	assert.Empty(t, svc.lastReq.Provider)
	assert.Empty(t, svc.lastReq.Model)
	assert.Empty(t, svc.lastReq.Policy)
	assert.Equal(t, 40, svc.lastReq.MinPower)
	assert.Equal(t, 90, svc.lastReq.MaxPower)
}

// TestFizeauAutoRoutingExplicitPinsRemainPassthrough verifies that explicit
// implementation and review routing pins are forwarded unchanged instead of
// being normalized or collapsed by DDx.
func TestFizeauAutoRoutingExplicitPinsRemainPassthrough(t *testing.T) {
	svc := &passthroughTestService{}
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{
		Harness:  "claude",
		Provider: "anthropic",
		Model:    "claude-3-7-sonnet",
		Profile:  "fast",
		MinPower: 40,
		MaxPower: 90,
	})

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	require.NoError(t, err)

	assert.Equal(t, "claude", svc.lastReq.Harness)
	assert.Equal(t, "anthropic", svc.lastReq.Provider)
	assert.Equal(t, "claude-3-7-sonnet", svc.lastReq.Model)
	assert.Equal(t, "fast", svc.lastReq.Policy)
	assert.Equal(t, 40, svc.lastReq.MinPower)
	assert.Equal(t, 90, svc.lastReq.MaxPower)

	impl := ImplementerRouting{
		Harness:     "claude",
		Provider:    "anthropic",
		Model:       "claude-3-7-sonnet",
		ActualPower: 40,
	}
	runRuntime := BuildReviewExecuteRequest(impl, "review-harness", "review-profile")
	reviewer := &DefaultBeadReviewer{Model: "review-model"}
	pinned := reviewer.applyExplicitReviewerPins(&runRuntime)

	assert.Equal(t, "review-harness", runRuntime.HarnessOverride)
	assert.Equal(t, "review-profile", runRuntime.ProfileOverride)
	assert.Equal(t, "review-model", runRuntime.ModelOverride)
	assert.Equal(t, "review-model", pinned)
	assert.True(t, runRuntime.ClearRoutingPins)
	assert.True(t, runRuntime.ClearProfile)
	assert.True(t, runRuntime.ClearMaxPower)
}

// TestRunServiceRequestCarriesPolicyForProfileDrivenHarnessRouting verifies
// that an explicit harness plus profile-only route forwards the profile as a
// Fizeau policy so empty-model requests remain routable without a model pin.
func TestRunServiceRequestCarriesPolicyForProfileDrivenHarnessRouting(t *testing.T) {
	svc := &passthroughTestService{}
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{
		Harness: "codex",
		Profile: "smart",
	})

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	require.NoError(t, err)

	assert.Equal(t, "codex", svc.lastReq.Harness)
	assert.Equal(t, "smart", svc.lastReq.Policy)
	assert.Empty(t, svc.lastReq.Model, "profile-driven harness routing must keep the model empty")
}

func TestExecuteOnService_InvalidPowerBoundsFailBeforeService(t *testing.T) {
	svc := &passthroughTestService{}
	rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-7-sonnet", 90, 8)

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	if err == nil {
		t.Fatal("expected invalid power bounds error")
	}
	if !strings.Contains(err.Error(), "min_power=90 must be less than max_power=8") {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.lastReq.Prompt != "" {
		t.Fatalf("service should not receive invalid power bounds, got request: %+v", svc.lastReq)
	}
}

func TestRouting_PassthroughUnsatisfiedStops(t *testing.T) {
	svc := &passthroughTestService{}
	rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-7-sonnet", 0, 8)

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt:           "repair this",
		MinPowerOverride: 90,
	})
	require.Error(t, err)
	assert.False(t, svc.executeCalled, "invalid passthrough+power request must stop before Execute")
	assert.Equal(t,
		FailureModeBlockedByPassthroughConstraint,
		ClassifyFailureMode("task_failed", 1, err.Error()),
	)
}

// TestAppendBeadRoutingEvidence_RecordsPassthroughConstraintsSeparately (AC5):
// The routing evidence body must contain requested_harness/provider/model and
// requested_min_power/max_power as distinct fields, separate from the
// resolved/actual values.
func TestAppendBeadRoutingEvidence_RecordsPassthroughConstraintsSeparately(t *testing.T) {
	app := &stubBeadEventAppenderForPassthrough{}
	pt := config.AgentPassthrough{
		Harness:  "claude",
		Provider: "anthropic",
		Model:    "claude-opus-4-6",
	}
	appendBeadRoutingEvidence(app, "ddx-test-01",
		"claude", "anthropic", "claude-3-5-sonnet-20241022", // actual/resolved
		"route-reason", "https://api.anthropic.com",
		pt, 40, 90, 70)

	if len(app.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(app.events))
	}
	evt := app.events[0]
	if evt.Kind != "routing" {
		t.Errorf("event kind = %q, want %q", evt.Kind, "routing")
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(evt.Body), &body); err != nil {
		t.Fatalf("unmarshal routing body: %v", err)
	}

	// Actual/resolved values
	if body["resolved_provider"] != "anthropic" {
		t.Errorf("resolved_provider = %v, want %q", body["resolved_provider"], "anthropic")
	}
	if body["resolved_model"] != "claude-3-5-sonnet-20241022" {
		t.Errorf("resolved_model = %v, want %q", body["resolved_model"], "claude-3-5-sonnet-20241022")
	}

	// Requested passthrough constraints (separate from resolved)
	if body["requested_harness"] != "claude" {
		t.Errorf("requested_harness = %v, want %q", body["requested_harness"], "claude")
	}
	if body["requested_provider"] != "anthropic" {
		t.Errorf("requested_provider = %v, want %q", body["requested_provider"], "anthropic")
	}
	if body["requested_model"] != "claude-opus-4-6" {
		t.Errorf("requested_model = %v, want %q", body["requested_model"], "claude-opus-4-6")
	}

	// Requested power bounds (separate from passthrough constraints)
	if body["requested_min_power"] != float64(40) {
		t.Errorf("requested_min_power = %v, want 40", body["requested_min_power"])
	}
	if body["requested_max_power"] != float64(90) {
		t.Errorf("requested_max_power = %v, want 90", body["requested_max_power"])
	}

	// AC3: actual_power must be a top-level numeric field so retry policy can
	// read prior actual_power without inspecting passthrough strings (AC4).
	if body["actual_power"] != float64(70) {
		t.Errorf("actual_power = %v, want 70", body["actual_power"])
	}
}

// TestAppendBeadRoutingEvidence_ActualPowerReadableWithoutPassthrough (AC4):
// actual_power is a top-level numeric field in the routing evidence body,
// independent of the passthrough envelope strings. Retry policy can read it
// directly from the JSON without inspecting harness/provider/model strings.
func TestAppendBeadRoutingEvidence_ActualPowerReadableWithoutPassthrough(t *testing.T) {
	app := &stubBeadEventAppenderForPassthrough{}
	// Empty passthrough — actual_power must still be independently readable.
	appendBeadRoutingEvidence(app, "ddx-test-02",
		"codex", "", "gpt-4o",
		"", "",
		config.AgentPassthrough{}, 0, 0, 85)

	if len(app.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(app.events))
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(app.events[0].Body), &body); err != nil {
		t.Fatalf("unmarshal routing body: %v", err)
	}

	// Verify actual_power is a plain number — readable without string parsing.
	power, ok := body["actual_power"].(float64)
	if !ok {
		t.Fatalf("actual_power is not a number in routing evidence: %T %v", body["actual_power"], body["actual_power"])
	}
	if int(power) != 85 {
		t.Errorf("actual_power = %d, want 85", int(power))
	}
	// Passthrough fields are absent (empty), confirming actual_power stands alone.
	if body["requested_harness"] != nil {
		t.Errorf("requested_harness should be absent when passthrough is empty, got %v", body["requested_harness"])
	}
}
