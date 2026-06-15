package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/require"
)

type stubAgentService struct {
	execute func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error)
}

func (s *stubAgentService) Execute(_ context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	if s.execute != nil {
		return s.execute(req)
	}
	ch := make(chan agentlib.ServiceEvent, 1)
	ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
	close(ch)
	return ch, nil
}

func (s *stubAgentService) ResolveRoute(_ context.Context, _ agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	return nil, fmt.Errorf("routinglint: ResolveRoute called in execution path — violates CONTRACT-003 / ddx-da19756a")
}

func (s *stubAgentService) TailSessionLog(_ context.Context, _ string) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}

func (s *stubAgentService) ListHarnesses(_ context.Context) ([]agentlib.HarnessInfo, error) {
	return []agentlib.HarnessInfo{{Name: "claude", Available: true}, {Name: "agent", Available: true}, {Name: "codex", Available: true}}, nil
}

func (s *stubAgentService) ListProviders(_ context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}

func (s *stubAgentService) ListModels(_ context.Context, _ agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	return nil, nil
}

func (s *stubAgentService) HealthCheck(_ context.Context, _ agentlib.HealthTarget) error {
	return nil
}

func (s *stubAgentService) ListPolicies(_ context.Context) ([]agentlib.PolicyInfo, error) {
	return nil, nil
}

func (s *stubAgentService) RecordRouteAttempt(_ context.Context, _ agentlib.RouteAttempt) error {
	return nil
}

func (s *stubAgentService) RouteStatus(_ context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}

func (s *stubAgentService) ListSessionLogs(_ context.Context) ([]agentlib.SessionLogEntry, error) {
	return nil, nil
}

func (s *stubAgentService) WriteSessionLog(_ context.Context, _ string, _ io.Writer) error {
	return nil
}

func (s *stubAgentService) ReplaySession(_ context.Context, _ string, _ io.Writer) error {
	return nil
}

func (s *stubAgentService) UsageReport(_ context.Context, _ agentlib.UsageReportOptions) (*agentlib.UsageReport, error) {
	return nil, nil
}

type executeCapturingStub struct {
	mu             sync.Mutex
	executeCalled  bool
	lastReq        agentlib.ServiceExecuteRequest
	requests       []agentlib.ServiceExecuteRequest
	executeFn      func(agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error)
	resolveRouteFn func(agentlib.RouteRequest) (*agentlib.RouteDecision, error)
	listModels     []agentlib.ModelInfo
	listPolicies   []agentlib.PolicyInfo
	routeRequests  []agentlib.RouteRequest
	modelFilters   []agentlib.ModelFilter
}

func (s *executeCapturingStub) Execute(_ context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.mu.Lock()
	s.executeCalled = true
	s.lastReq = req
	s.requests = append(s.requests, req)
	s.mu.Unlock()
	if s.executeFn != nil {
		return s.executeFn(req)
	}
	ch := make(chan agentlib.ServiceEvent, 1)
	ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
	close(ch)
	return ch, nil
}

func (s *executeCapturingStub) ResolveRoute(_ context.Context, req agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	s.mu.Lock()
	s.routeRequests = append(s.routeRequests, req)
	s.mu.Unlock()
	if s.resolveRouteFn != nil {
		return s.resolveRouteFn(req)
	}
	return nil, fmt.Errorf("routinglint: ResolveRoute called in execution path — violates CONTRACT-003 / ddx-da19756a")
}

func (s *executeCapturingStub) TailSessionLog(_ context.Context, _ string) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}

func (s *executeCapturingStub) ListHarnesses(_ context.Context) ([]agentlib.HarnessInfo, error) {
	return []agentlib.HarnessInfo{{Name: "claude", Available: true}, {Name: "agent", Available: true}, {Name: "codex", Available: true}}, nil
}

func (s *executeCapturingStub) ListProviders(_ context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}

func (s *executeCapturingStub) ListModels(_ context.Context, filter agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	s.mu.Lock()
	s.modelFilters = append(s.modelFilters, filter)
	s.mu.Unlock()
	return append([]agentlib.ModelInfo(nil), s.listModels...), nil
}

func (s *executeCapturingStub) ListPolicies(_ context.Context) ([]agentlib.PolicyInfo, error) {
	return append([]agentlib.PolicyInfo(nil), s.listPolicies...), nil
}

func (s *executeCapturingStub) HealthCheck(_ context.Context, _ agentlib.HealthTarget) error {
	return nil
}

func (s *executeCapturingStub) RecordRouteAttempt(_ context.Context, _ agentlib.RouteAttempt) error {
	return nil
}

func (s *executeCapturingStub) RouteStatus(_ context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}

func (s *executeCapturingStub) ListSessionLogs(_ context.Context) ([]agentlib.SessionLogEntry, error) {
	return nil, nil
}

func (s *executeCapturingStub) WriteSessionLog(_ context.Context, _ string, _ io.Writer) error {
	return nil
}

func (s *executeCapturingStub) ReplaySession(_ context.Context, _ string, _ io.Writer) error {
	return nil
}

func (s *executeCapturingStub) UsageReport(_ context.Context, _ agentlib.UsageReportOptions) (*agentlib.UsageReport, error) {
	return nil, nil
}

func installExecuteCapturingStub(t *testing.T) *executeCapturingStub {
	t.Helper()
	stub := &executeCapturingStub{}
	agent.SetServiceRunFactory(func(_ string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { agent.SetServiceRunFactory(nil) })
	return stub
}

func canonicalFizeauPolicyFixture() ([]agentlib.PolicyInfo, []agentlib.ModelInfo) {
	return []agentlib.PolicyInfo{
			{Name: "cheap", MinPower: 5, MaxPower: 5, AllowLocal: true},
			{Name: "default", MinPower: 7, MaxPower: 8, AllowLocal: true},
			{Name: "smart", MinPower: 9, MaxPower: 10},
			{Name: "air-gapped", MinPower: 5, MaxPower: 5, AllowLocal: true, Require: []string{"no_remote"}},
		}, []agentlib.ModelInfo{
			{ID: "cheap-model", Power: 5, Available: true, AutoRoutable: true},
			{ID: "standard-model", Power: 7, Available: true, AutoRoutable: true},
			{ID: "smart-model", Power: 9, Available: true, AutoRoutable: true},
		}
}

func capturedImplementationRequests(stub *executeCapturingStub) []agentlib.ServiceExecuteRequest {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	out := make([]agentlib.ServiceExecuteRequest, 0, len(stub.requests))
	for _, req := range stub.requests {
		if req.Role == "implementer" {
			out = append(out, req)
		}
	}
	return out
}

func capturedRouteRequests(stub *executeCapturingStub) []agentlib.RouteRequest {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return append([]agentlib.RouteRequest(nil), stub.routeRequests...)
}

func capturedModelFilters(stub *executeCapturingStub) []agentlib.ModelFilter {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return append([]agentlib.ModelFilter(nil), stub.modelFilters...)
}

func minimalProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ddxDir := ddxroot.JoinProject(dir)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	cfg := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644))
	// Pre-stage the bead-lifecycle skill so ensureBeadLifecycleSkill is a no-op
	// in tests that dispatch through ddx work / ddx try. Without this, the
	// lifecycle hook installs ~50 untracked files into the test's git repo, and
	// the subsequent pre-execute-bead checkpoint refuses to absorb them
	// (matches setupWorkIntakeFixture's existing pre-install pattern).
	skillDir := filepath.Join(dir, ".agents", "skills", "ddx", "bead-lifecycle")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("intake"), 0o644))
	// Mirror the production .gitignore subset that keeps the pre-execute-bead
	// checkpoint from refusing on per-machine session log dirs written during
	// dispatch. Tests set HOME to the fixture root, so XDG state from Fizeau
	// also lands inside the repo unless this generated state path is ignored.
	gitignore := ".ddx/agent-logs/\n.ddx/workers/\n.ddx/backups/\n.local/state/fizeau/\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignore), 0o644))
	return dir
}

func appendTestRoutingEvidence(t *testing.T, dir, beadID, harness, provider, model, routeReason, baseURL string) {
	t.Helper()
	store := bead.NewStore(ddxroot.JoinProject(dir))
	body := struct {
		ResolvedProvider string   `json:"resolved_provider"`
		ResolvedModel    string   `json:"resolved_model,omitempty"`
		RouteReason      string   `json:"route_reason,omitempty"`
		FallbackChain    []string `json:"fallback_chain"`
		BaseURL          string   `json:"base_url,omitempty"`
	}{
		ResolvedProvider: provider,
		ResolvedModel:    model,
		RouteReason:      routeReason,
		FallbackChain:    []string{},
		BaseURL:          baseURL,
	}
	if body.ResolvedProvider == "" {
		body.ResolvedProvider = harness
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	require.NoError(t, store.AppendEvent(beadID, bead.BeadEvent{
		Kind:    "routing",
		Summary: fmt.Sprintf("provider=%s model=%s reason=%s", body.ResolvedProvider, body.ResolvedModel, body.RouteReason),
		Body:    string(raw),
		Actor:   "ddx",
		Source:  "ddx try",
	}))
}

func setupWorkIntakeFixture(t *testing.T) string {
	t.Helper()
	dir := minimalProjectDir(t)

	skillDir := filepath.Join(dir, ".agents", "skills", "ddx", "bead-lifecycle")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("intake"), 0o644))

	store := bead.NewStore(ddxroot.JoinProject(dir))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:        "ddx-intake-test",
		Title:     "work intake wiring test bead",
		IssueType: "docs",
	}))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644))
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "init").Run())
	return dir
}
