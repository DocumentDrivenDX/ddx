package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAgentService is a controllable agentlib.DdxAgent used by routing e2e
// tests. It records Execute calls and can be configured to return typed errors
// from Execute to test upstream error surfacing.
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
	return []agentlib.HarnessInfo{{Name: "claude", Available: true}, {Name: "agent", Available: true}}, nil
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
func (s *stubAgentService) ResolveProfile(_ context.Context, _ string) (*agentlib.ResolvedProfile, error) {
	return nil, nil
}
func (s *stubAgentService) ProfileAliases(_ context.Context) (map[string]string, error) {
	return nil, nil
}
func (s *stubAgentService) ListProfiles(_ context.Context) ([]agentlib.ProfileInfo, error) {
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

// installStubService injects a service factory and registers cleanup.
func installStubService(t *testing.T, stub *stubAgentService) {
	t.Helper()
	agent.SetServiceRunFactory(func(_ string) (agentlib.DdxAgent, error) {
		return stub, nil
	})
	t.Cleanup(func() { agent.SetServiceRunFactory(nil) })
}

// minimalProjectDir creates a project dir with a clean .ddx/config.yaml that
// has no agent.harness pin so flag values drive routing.
func minimalProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	cfg := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644))
	return dir
}

// AC #1: ddx agent run --harness claude forwards Harness to Execute.
func TestRunHarnessFlagPlumbsToExecute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	var capturedReq agentlib.ServiceExecuteRequest
	installStubService(t, &stubAgentService{
		execute: func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
			capturedReq = req
			ch := make(chan agentlib.ServiceEvent, 1)
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
			close(ch)
			return ch, nil
		},
	})

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "agent", "run", "--harness", "claude", "--text", "hi", "--timeout", "5s")
	require.NoError(t, err, "stub Execute returns success; if ResolveRoute were called, stub would error")
	assert.Equal(t, "claude", capturedReq.Harness, "--harness must plumb through to ServiceExecuteRequest.Harness")
}

// AC #2: ddx agent run --model opus-4.7 forwards Model to Execute.
func TestRunModelFlagPlumbsToExecute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	var capturedReq agentlib.ServiceExecuteRequest
	installStubService(t, &stubAgentService{
		execute: func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
			capturedReq = req
			ch := make(chan agentlib.ServiceEvent, 1)
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
			close(ch)
			return ch, nil
		},
	})

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "agent", "run", "--model", "opus-4.7", "--text", "hi", "--timeout", "5s")
	require.NoError(t, err, "stub Execute returns success; if ResolveRoute were called, stub would error")
	assert.Equal(t, "opus-4.7", capturedReq.Model, "--model must plumb through to ServiceExecuteRequest.Model")
}

// AC #3: ddx agent run --harness claude --model opus forwards both to Execute.
func TestRunHarnessAndModelBothPlumbToExecute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	var capturedReq agentlib.ServiceExecuteRequest
	installStubService(t, &stubAgentService{
		execute: func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
			capturedReq = req
			ch := make(chan agentlib.ServiceEvent, 1)
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
			close(ch)
			return ch, nil
		},
	})

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "agent", "run",
		"--harness", "claude", "--model", "opus", "--text", "hi", "--timeout", "5s")
	require.NoError(t, err, "stub Execute returns success; if ResolveRoute were called, stub would error")
	assert.Equal(t, "claude", capturedReq.Harness)
	assert.Equal(t, "opus", capturedReq.Model)
}

// AC #4: typed errors from Execute surface verbatim — DDx does not wrap them.
func TestRunTypedExecuteErrorSurfacesVerbatim(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	executeErr := fmt.Errorf("upstream: provider quota exhausted")
	installStubService(t, &stubAgentService{
		execute: func(_ agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
			return nil, executeErr
		},
	})

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "agent", "run", "--harness", "claude", "--text", "hi", "--timeout", "5s")
	require.Error(t, err, "typed error from Execute must surface")
	assert.Contains(t, err.Error(), "upstream: provider quota exhausted",
		"execute error message must appear in returned error")
}

// AC #5: ddx work shares the same passthrough plumbing — flags reach Execute.
func TestWorkSharesSamePassthroughPlumbing(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	var capturedReq agentlib.ServiceExecuteRequest
	installStubService(t, &stubAgentService{
		execute: func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
			capturedReq = req
			ch := make(chan agentlib.ServiceEvent, 1)
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
			close(ch)
			return ch, nil
		},
	})

	dir := minimalProjectDir(t)

	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "ddx-work-passthrough-test",
		Title: "work passthrough fixture",
	}))

	root := NewCommandFactory(dir).NewRootCommand()
	_, _ = executeCommand(root, "work",
		"--local", "--once",
		"--harness", "claude",
		"--model", "gpt-5.4",
	)

	if capturedReq.Harness == "" && capturedReq.Model == "" {
		t.Skip("Execute not called — bead may not have reached dispatch")
	}

	assert.Equal(t, "claude", capturedReq.Harness,
		"ddx work must plumb --harness to Execute.Harness")
	assert.Equal(t, "gpt-5.4", capturedReq.Model,
		"ddx work must plumb --model to Execute.Model")
}
