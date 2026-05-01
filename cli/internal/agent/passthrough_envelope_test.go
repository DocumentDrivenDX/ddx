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
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/DocumentDrivenDX/fizeau"
)

// passthroughTestService is a minimal FizeauService stub that records the most
// recent Execute call's request and returns a synthetic success event.
type passthroughTestService struct {
	executeCalled       bool
	lastReq             agentlib.ServiceExecuteRequest
	listHarnessesCalled bool
}

func (s *passthroughTestService) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.executeCalled = true
	s.lastReq = req
	ch := make(chan agentlib.ServiceEvent, 1)
	ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
	close(ch)
	return ch, nil
}

func (s *passthroughTestService) ListHarnesses(ctx context.Context) ([]agentlib.HarnessInfo, error) {
	s.listHarnessesCalled = true
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
	return nil, nil
}

func (s *passthroughTestService) ListProfiles(ctx context.Context) ([]agentlib.ProfileInfo, error) {
	return nil, nil
}

func (s *passthroughTestService) ResolveProfile(ctx context.Context, name string) (*agentlib.ResolvedProfile, error) {
	return nil, nil
}

func (s *passthroughTestService) ProfileAliases(ctx context.Context) (map[string]string, error) {
	return nil, nil
}

func (s *passthroughTestService) HealthCheck(ctx context.Context, target agentlib.HealthTarget) error {
	return nil
}

func (s *passthroughTestService) ResolveRoute(ctx context.Context, req agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	return nil, nil
}

func (s *passthroughTestService) RecordRouteAttempt(ctx context.Context, attempt agentlib.RouteAttempt) error {
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
		Harness: harness,
		Model:   model,
	})
	return cfg.Resolve(config.CLIOverrides{
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
