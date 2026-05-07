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
	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type preClaimIntakeHookServiceStub struct {
	executeCalls int32
	lastReq      agentlib.ServiceExecuteRequest
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

func (s *preClaimIntakeHookServiceStub) ResolveProfile(_ context.Context, _ string) (*agentlib.ResolvedProfile, error) {
	return nil, nil
}

func (s *preClaimIntakeHookServiceStub) ProfileAliases(_ context.Context) (map[string]string, error) {
	return nil, nil
}

func (s *preClaimIntakeHookServiceStub) ListProfiles(_ context.Context) ([]agentlib.ProfileInfo, error) {
	return nil, nil
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
	cfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Model: "claude-sonnet-4-6",
	})
	return cfg.Resolve(config.CLIOverrides{Harness: "claude"})
}

func TestDecompositionHook_UsesStrongMinPower(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{
		listModels: []agentlib.ModelInfo{
			{ID: "cheap", Power: 21},
			{ID: "smart", Power: 94},
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
	assert.Equal(t, "smart", svc.lastReq.Profile)
	assert.GreaterOrEqual(t, svc.lastReq.MinPower, 94)
}

func TestDecompositionHook_CatalogUnavailableUsesSmartProfileWithoutMagicPower(t *testing.T) {
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
	assert.Equal(t, "smart", svc.lastReq.Profile)
	assert.Zero(t, svc.lastReq.MinPower)
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

func TestDecompositionHook_SmartProfileUnavailableFallsBackToDefaultProfile(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{}
	svc.executeFunc = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		ch := make(chan agentlib.ServiceEvent, 1)
		if req.Profile == "smart" {
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"error","exit_code":1,"error":"ResolveRoute: no live provider supports prompt of 0 tokens with tools=false at tier ≥ smart"}`)}
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
	assert.Equal(t, int32(2), atomic.LoadInt32(&svc.executeCalls))
	assert.Equal(t, DefaultRoutingProfile, svc.lastReq.Profile)
	assert.Zero(t, svc.lastReq.MinPower)
}

func TestDecompositionHook_PreservesPassthroughConstraints(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{
		listModels: []agentlib.ModelInfo{
			{ID: "smart", Power: 96},
		},
		finalText: `{"classification":"atomic","confidence":0.95,"reasoning":"passthrough intact"}`,
	}

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Model: "claude-sonnet-4-6",
	}).Resolve(config.CLIOverrides{
		Harness:  "claude",
		Provider: "anthropic",
	})

	hook := NewPreClaimIntakeHook(root, store, rcfg, svc, nil)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, "claude", svc.lastReq.Harness)
	assert.Equal(t, "anthropic", svc.lastReq.Provider)
	assert.Equal(t, "claude-sonnet-4-6", svc.lastReq.Model)
	assert.Equal(t, "smart", svc.lastReq.Profile)
	assert.GreaterOrEqual(t, svc.lastReq.MinPower, 96)
}

func TestDecompositionHook_StrongPowerUnsatisfiedSkipsIntake(t *testing.T) {
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
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Contains(t, got.Detail, "pre-claim intake skipped")
	assert.Equal(t, int32(1), atomic.LoadInt32(&svc.executeCalls))
}

func TestDecompositionHook_StrongMinPowerAboveMaxPowerSkipsBeforeDispatch(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{
		listModels: []agentlib.ModelInfo{
			{ID: "smart", Power: 90},
		},
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
	assert.Contains(t, got.Detail, "pre-claim intake skipped")
	assert.Contains(t, got.Detail, "min_power=90")
	assert.Contains(t, got.Detail, "max_power=8")
	assert.Equal(t, int32(0), atomic.LoadInt32(&svc.executeCalls), "invalid power envelope must not reach Fizeau")
}

func TestDecompositionHook_RoutePinsUnsatisfiedSkipsIntake(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	store, b := newPreClaimIntakeHookTestStore(t, root)

	svc := &preClaimIntakeHookServiceStub{
		executeErr: fmt.Errorf("runner error: ResolveRoute: no viable routing candidate for pins min_power=7 max_power=8: 3 candidates rejected"),
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
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Contains(t, got.Detail, "pre-claim intake skipped")
	assert.Contains(t, got.Detail, "no viable routing candidate for pins")
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
