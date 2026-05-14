package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type preClaimIntakeHookServiceStub struct {
	executeCalls int32
	lastReq      agentlib.ServiceExecuteRequest
	listPolicies []agentlib.PolicyInfo
	listModels   []agentlib.ModelInfo
	executeErr   error
	executeFunc  func(agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error)
	finalText    string
}

func (s *preClaimIntakeHookServiceStub) Execute(_ context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	atomic.AddInt32(&s.executeCalls, 1)
	s.lastReq = req
	if s.executeFunc != nil {
		return s.executeFunc(req)
	}
	if s.executeErr != nil {
		return nil, s.executeErr
	}
	text := s.finalText
	if text == "" {
		text = `{"classification":"atomic","confidence":0.99,"reasoning":"ready"}`
	}
	ch := make(chan agentlib.ServiceEvent, 1)
	ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":` + fmt.Sprintf("%q", text) + `}`)}
	close(ch)
	return ch, nil
}

func (s *preClaimIntakeHookServiceStub) ResolveRoute(_ context.Context, _ agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	return nil, fmt.Errorf("ResolveRoute should not be called in intake hook tests")
}

func (s *preClaimIntakeHookServiceStub) TailSessionLog(_ context.Context, _ string) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}

func (s *preClaimIntakeHookServiceStub) ListHarnesses(_ context.Context) ([]agentlib.HarnessInfo, error) {
	return nil, nil
}

func (s *preClaimIntakeHookServiceStub) ListProviders(_ context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}

func (s *preClaimIntakeHookServiceStub) ListModels(_ context.Context, _ agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	return append([]agentlib.ModelInfo(nil), s.listModels...), nil
}

func (s *preClaimIntakeHookServiceStub) HealthCheck(_ context.Context, _ agentlib.HealthTarget) error {
	return nil
}

func (s *preClaimIntakeHookServiceStub) ListPolicies(_ context.Context) ([]agentlib.PolicyInfo, error) {
	return append([]agentlib.PolicyInfo(nil), s.listPolicies...), nil
}

func (s *preClaimIntakeHookServiceStub) RecordRouteAttempt(_ context.Context, _ agentlib.RouteAttempt) error {
	return nil
}

func (s *preClaimIntakeHookServiceStub) RouteStatus(_ context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}

func (s *preClaimIntakeHookServiceStub) ListSessionLogs(_ context.Context) ([]agentlib.SessionLogEntry, error) {
	return nil, nil
}

func (s *preClaimIntakeHookServiceStub) WriteSessionLog(_ context.Context, _ string, _ io.Writer) error {
	return nil
}

func (s *preClaimIntakeHookServiceStub) ReplaySession(_ context.Context, _ string, _ io.Writer) error {
	return nil
}

func (s *preClaimIntakeHookServiceStub) UsageReport(_ context.Context, _ agentlib.UsageReportOptions) (*agentlib.UsageReport, error) {
	return nil, nil
}

func newPreClaimIntakeHookTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	skillDir := filepath.Join(root, ".agents", "skills", "ddx", "bead-lifecycle")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("intake"), 0o644))
	return root
}

func newPreClaimIntakeHookTestStore(t *testing.T, root string) (*bead.Store, *bead.Bead) {
	t.Helper()
	store := bead.NewStore(filepath.Join(root, ".ddx"))
	require.NoError(t, store.Init())
	b := &bead.Bead{
		ID:          "ddx-intake-1",
		Title:       "work: wire strong-model intake decomposition into ddx work",
		IssueType:   bead.DefaultType,
		Status:      bead.StatusOpen,
		Priority:    1,
		Description: "PROBLEM\nmissing intake\n\nROOT CAUSE\nno hook\n\nPROPOSED FIX\nadd hook\n",
		Acceptance:  "1. TestDecompositionHook_UsesStrongMinPower\n2. cd cli && go test ./internal/agent/... green\n3. lefthook run pre-commit passes",
		Labels:      []string{"phase:2", "area:agent", "kind:feature"},
	}
	require.NoError(t, store.Create(b))
	return store, b
}

func intakeHookTestConfig() config.ResolvedConfig {
	cfg := config.NewTestConfigForRun(config.TestRunConfigOpts{})
	return cfg.Resolve(config.CLIOverrides{})
}

func TestPreClaimIntakeHook_DispatchesWithCheapestProfileNoStrongPowerTrick(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{
		listPolicies: []agentlib.PolicyInfo{
			{Name: "cheap", MinPower: 5, MaxPower: 5},
			{Name: "smart", MinPower: 9, MaxPower: 10},
		},
		listModels: []agentlib.ModelInfo{
			{ID: "cheap", Power: 5, Available: true, AutoRoutable: true},
			{ID: "smart", Power: 9, Available: true, AutoRoutable: true},
		},
		finalText: `{"classification":"atomic","confidence":0.99,"reasoning":"single-slice"}`,
	}

	hook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, int32(1), atomic.LoadInt32(&svc.executeCalls))
	assert.Contains(t, svc.lastReq.Prompt, "MODE: intake")
	assert.Equal(t, root, svc.lastReq.WorkDir)
	assert.Empty(t, svc.lastReq.Harness)
	assert.Empty(t, svc.lastReq.Provider)
	assert.Empty(t, svc.lastReq.Model)
	assert.Equal(t, "cheap", svc.lastReq.Policy)
	assert.Zero(t, svc.lastReq.MinPower)
	assert.Zero(t, svc.lastReq.MaxPower)
}

func TestDecompositionHook_CatalogUnavailableUsesAutoRouteWithoutMagicPower(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"classification":"atomic","confidence":0.99,"reasoning":"single-slice"}`,
	}

	hook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, int32(1), atomic.LoadInt32(&svc.executeCalls))
	assert.Empty(t, svc.lastReq.Policy)
	assert.Zero(t, svc.lastReq.MinPower)
	assert.Zero(t, svc.lastReq.MaxPower)
}

func TestDecompositionHook_AcceptsStringConfidence(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"classification":"atomic","confidence":"0.99","reasoning":"ready despite string confidence"}`,
	}

	hook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, "ready despite string confidence", got.Detail)
}

func TestDecompositionHook_SmartProfileUnavailableFallsBackToAutoRoute(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{}
	svc.executeFunc = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		ch := make(chan agentlib.ServiceEvent, 1)
		if req.Policy == "smart" {
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"error","exit_code":1,"error":"ResolveRoute: no live provider supports profile=smart"}`)}
			close(ch)
			return ch, nil
		}
		text := `{"classification":"atomic","confidence":0.99,"reasoning":"fallback-ready"}`
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":` + fmt.Sprintf("%q", text) + `}`)}
		close(ch)
		return ch, nil
	}

	hook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, int32(1), atomic.LoadInt32(&svc.executeCalls))
	assert.Empty(t, svc.lastReq.Policy)
	assert.Zero(t, svc.lastReq.MinPower)
	assert.Zero(t, svc.lastReq.MaxPower)
}

func TestPreClaimIntakeHook_PreservesExplicitRoutingPins(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"classification":"atomic","confidence":0.95,"reasoning":"pins intact"}`,
	}

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{
		Harness: "codex",
		Model:   "gpt-5.4-mini",
	})

	hook := NewPreClaimIntakeHook(root, store, rcfg, svc, nil)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, "codex", svc.lastReq.Harness)
	assert.Empty(t, svc.lastReq.Provider)
	assert.Equal(t, "gpt-5.4-mini", svc.lastReq.Model)
	assert.Empty(t, svc.lastReq.Policy)
	assert.Zero(t, svc.lastReq.MinPower)
}

func TestLifecycleHooks_UnpinnedWorkersStillUseProfileSelection(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)
	rcfg := intakeHookTestConfig()

	intakeSvc := &preClaimIntakeHookServiceStub{
		listPolicies: []agentlib.PolicyInfo{
			{Name: "cheap", MinPower: 5, MaxPower: 5},
			{Name: "smart", MinPower: 9, MaxPower: 10},
		},
		listModels: []agentlib.ModelInfo{
			{ID: "cheap-model", Power: 5, Available: true, AutoRoutable: true},
			{ID: "smart-model", Power: 9, Available: true, AutoRoutable: true},
		},
		finalText: `{"classification":"atomic","confidence":0.95,"reasoning":"profile selected"}`,
	}
	intakeHook := NewPreClaimIntakeHook(root, store, rcfg, intakeSvc, nil)
	intake, err := intakeHook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, intake.Outcome)
	assert.Equal(t, "cheap", intakeSvc.lastReq.Policy)
	assert.Empty(t, intakeSvc.lastReq.Harness)
	assert.Empty(t, intakeSvc.lastReq.Provider)
	assert.Empty(t, intakeSvc.lastReq.Model)

	lintSvc := &passthroughTestService{
		listPolicies: []agentlib.PolicyInfo{
			{Name: "standard", MinPower: 7, MaxPower: 8},
			{Name: "cheap", MinPower: 5, MaxPower: 5},
		},
		listModels: []agentlib.ModelInfo{
			{ID: "cheap-model", Power: 5, Available: true, AutoRoutable: true},
		},
		executeEvents: []agentlib.ServiceEvent{
			{
				Type: "final",
				Data: []byte(`{"status":"success","final_text":"{\"score\":8,\"rationale\":\"profile selected\",\"suggested_fixes\":[],\"waivers_applied\":[]}"}`),
			},
		},
	}
	lint, err := NewPreDispatchLintHook(root, store, rcfg, lintSvc, nil)(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, 8, lint.Score)
	assert.Equal(t, "cheap", lintSvc.lastReq.Policy)
	assert.Empty(t, lintSvc.lastReq.Harness)
	assert.Empty(t, lintSvc.lastReq.Provider)
	assert.Empty(t, lintSvc.lastReq.Model)
}

func TestDecompositionHook_StrongPowerUnsatisfiedReturnsIntakeError(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{
		listModels: []agentlib.ModelInfo{
			{ID: "cheap", Power: 12},
		},
		executeErr: fmt.Errorf("passthrough constraint unsatisfiable: harness=claude min_power=90"),
	}

	hook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeError, got.Outcome)
	assert.Contains(t, got.Detail, "readiness route unavailable")
	assert.Contains(t, got.Detail, "passthrough constraint unsatisfiable")
	assert.Equal(t, int32(1), atomic.LoadInt32(&svc.executeCalls))
}

func TestDecompositionHook_ClearsImplementationPowerBounds(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{
		listPolicies: []agentlib.PolicyInfo{
			{Name: "smart", MinPower: 9, MaxPower: 10},
		},
		listModels: []agentlib.ModelInfo{
			{ID: "smart", Power: 9, Available: true, AutoRoutable: true},
		},
	}
	svc.executeFunc = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		assert.Zero(t, req.MinPower)
		assert.Empty(t, req.Policy)
		assert.Equal(t, "claude", req.Harness)
		assert.Empty(t, req.Model)
		assert.Zero(t, req.MaxPower, "pre-claim intake must not inherit implementation max_power pins")
		ch := make(chan agentlib.ServiceEvent, 1)
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"atomic\",\"confidence\":0.99,\"reasoning\":\"frontier-ready\"}"}`)}
		close(ch)
		return ch, nil
	}
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Model: "claude-sonnet-4-6",
	}).Resolve(config.CLIOverrides{
		Harness:  "claude",
		MaxPower: 8,
	})

	hook := NewPreClaimIntakeHook(root, store, rcfg, svc, nil)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, "frontier-ready", got.Detail)
	assert.Equal(t, int32(1), atomic.LoadInt32(&svc.executeCalls), "pre-claim intake must still dispatch when the worker has a low max_power")
}

func TestDecompositionHook_RoutingFailureReturnsIntakeErrorWithoutDDxPins(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{
		executeErr: fmt.Errorf("runner error: ResolveRoute: no viable routing candidate: 3 candidates rejected"),
	}
	svc.executeFunc = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		assert.Empty(t, req.Policy)
		assert.Zero(t, req.MinPower)
		assert.Zero(t, req.MaxPower)
		return nil, svc.executeErr
	}
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Model: "claude-sonnet-4-6",
	}).Resolve(config.CLIOverrides{
		Harness:  "claude",
		MinPower: 7,
		MaxPower: 8,
	})

	hook := NewPreClaimIntakeHook(root, store, rcfg, svc, nil)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeError, got.Outcome)
	assert.Contains(t, got.Detail, "readiness route unavailable")
	assert.NotContains(t, got.Detail, "min_power=7")
	assert.NotContains(t, got.Detail, "max_power=8")
	assert.Equal(t, int32(1), atomic.LoadInt32(&svc.executeCalls))
}

func TestIntakeResultPayload_EmptyOutputPreservesRunnerError(t *testing.T) {
	_, err := intakeResultPayload(&Result{
		ExitCode: 1,
		Output:   "",
		Error:    "fork/exec /home/erik/.local/bin/claude: argument list too long",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "argument list too long")
	assert.NotContains(t, err.Error(), "empty output")
}

func TestPreClaimReadiness_DecodesLegacyIntakeJSON(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantOutcome PreClaimIntakeOutcome
		wantDetail  string
	}{
		{
			name:        "atomic",
			payload:     `{"classification":"atomic","confidence":0.99,"reasoning":"single slice"}`,
			wantOutcome: PreClaimIntakeActionableAtomic,
			wantDetail:  "single slice",
		},
		{
			name:        "ok",
			payload:     `{"classification":"ok","confidence":0.99,"reasoning":"ready"}`,
			wantOutcome: PreClaimIntakeActionableAtomic,
			wantDetail:  "ready",
		},
		{
			name:        "decomposable",
			payload:     `{"classification":"decomposable","reasoning":"too broad"}`,
			wantOutcome: PreClaimIntakeTooLargeDecomposed,
			wantDetail:  "too broad",
		},
		{
			name:        "operator_required",
			payload:     `{"classification":"operator_required","rationale":"unclear scope"}`,
			wantOutcome: PreClaimIntakeOperatorRequired,
			wantDetail:  "ambiguous_scope: unclear scope",
		},
		{
			name:        "legacy ambiguous is rejected",
			payload:     `{"classification":"ambiguous","reasoning":"unclear scope"}`,
			wantOutcome: "",
			wantDetail:  "",
		},
		{
			name:        "rewritten",
			payload:     `{"classification":"rewritten","reasoning":"safe fix","rewrite":{"changed_fields":["acceptance"],"acceptance":"1. TestFoo\n2. cd cli && go test ./..."}}`,
			wantOutcome: PreClaimIntakeActionableButRewritten,
			wantDetail:  "safe fix",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodePreClaimIntakePayloadResult(tt.payload)
			if tt.wantOutcome == "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "legacy classification")
				assert.Contains(t, err.Error(), string(PreClaimIntakeOperatorRequired))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOutcome, got.Outcome)
			assert.Equal(t, tt.wantDetail, got.Detail)
		})
	}
}

func TestPreClaimReadiness_DecodesCanonicalReadinessJSON(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantOutcome PreClaimIntakeOutcome
		wantDetail  string
	}{
		{
			name:        "actionable_atomic",
			payload:     `{"outcome":"actionable_atomic","reason":"single slice"}`,
			wantOutcome: PreClaimIntakeActionableAtomic,
			wantDetail:  "single slice",
		},
		{
			name:        "too_large_decomposed",
			payload:     `{"outcome":"too_large_decomposed","reason":"too broad"}`,
			wantOutcome: PreClaimIntakeTooLargeDecomposed,
			wantDetail:  "too broad",
		},
		{
			name:        "operator_required",
			payload:     `{"outcome":"operator_required","reason":"unclear scope"}`,
			wantOutcome: PreClaimIntakeOperatorRequired,
			wantDetail:  "unclear scope",
		},
		{
			name:        "readiness_error_fails_open",
			payload:     `{"outcome":"readiness_error","reason":"skill missing"}`,
			wantOutcome: PreClaimIntakeError,
			wantDetail:  "skill missing",
		},
		{
			name:        "system_unready_fails_open",
			payload:     `{"outcome":"system_unready","reason":"infra failure"}`,
			wantOutcome: PreClaimIntakeError,
			wantDetail:  "infra failure",
		},
		{
			name:        "actionable_but_rewritten",
			payload:     `{"outcome":"actionable_but_rewritten","reason":"needs clarification","rewrite":{"changed_fields":["acceptance"],"description":"","acceptance":"1. TestFoo\n2. cd cli && go test ./..."}}`,
			wantOutcome: PreClaimIntakeActionableButRewritten,
			wantDetail:  "needs clarification",
		},
		{
			name:        "detail_field_fallback",
			payload:     `{"outcome":"actionable_atomic","detail":"via detail field"}`,
			wantOutcome: PreClaimIntakeActionableAtomic,
			wantDetail:  "via detail field",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodePreClaimIntakePayloadResult(tt.payload)
			require.NoError(t, err)
			assert.Equal(t, tt.wantOutcome, got.Outcome)
			assert.Equal(t, tt.wantDetail, got.Detail)
		})
	}

	for _, payload := range []string{
		`{"outcome":"ambiguous_needs_human","reason":"unclear scope"}`,
		`{"outcome":"needs_human","reason":"unclear scope"}`,
	} {
		_, err := decodePreClaimIntakePayloadResult(payload)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "legacy readiness outcome")
		assert.Contains(t, err.Error(), string(PreClaimIntakeOperatorRequired))
	}
}

func TestPreClaimReadiness_UnknownReasonActionableError(t *testing.T) {
	_, err := decodePreClaimIntakePayloadResult(`{"outcome":"not_a_real_outcome","reason":"some reason"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not_a_real_outcome")
	assert.Contains(t, err.Error(), "actionable_atomic")
	assert.Contains(t, err.Error(), "system_unready")
}

func TestDecompositionHook_ActionableButRewrittenParsesRewrite(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)
	escapedDescription := strings.ReplaceAll(b.Description, "\n", `\n`)

	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"classification":"rewritten","confidence":0.91,"reasoning":"safe refinement","rewrite":{"changed_fields":["acceptance","description"],"description":"` + escapedDescription + `\n\nAdd an explicit validation step.","acceptance":"1. TestDecompositionHook_ActionableButRewrittenParsesRewrite\n2. cd cli && go test ./internal/agent/... -run \"TestIntake_.*Rewrite|TestLintHook\" -count=1\n3. lefthook run pre-commit"}}`,
	}

	hook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableButRewritten, got.Outcome)
	assert.Equal(t, "safe refinement", got.Detail)
	assert.Equal(t, []string{"acceptance", "description"}, got.Rewrite.ChangedFields)
	assert.Contains(t, got.Rewrite.Description, "Add an explicit validation step.")
	assert.Contains(t, got.Rewrite.Acceptance, "lefthook run pre-commit")
}
