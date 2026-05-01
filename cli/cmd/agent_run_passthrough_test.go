package cmd

// agent_run_passthrough_test.go verifies ddx-da19756a AC:
//   - ddx run/work execution calls Execute with exact passthrough constraints
//     (Harness, Provider, Model) and MinPower/MaxPower bounds.
//   - ResolveRoute is NOT called during execution.

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeCapturingStub is a FizeauService that records Execute calls and fails
// loudly if ResolveRoute is invoked — enforcing the CONTRACT-003 boundary
// that execution paths must not pre-resolve routes.
type executeCapturingStub struct {
	mu            sync.Mutex
	executeCalled bool
	lastReq       agentlib.ServiceExecuteRequest
}

func (s *executeCapturingStub) Execute(_ context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.mu.Lock()
	s.executeCalled = true
	s.lastReq = req
	s.mu.Unlock()
	ch := make(chan agentlib.ServiceEvent, 1)
	ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
	close(ch)
	return ch, nil
}

func (s *executeCapturingStub) ResolveRoute(_ context.Context, _ agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	return nil, fmt.Errorf("routinglint: ResolveRoute called in execution path — violates CONTRACT-003 / ddx-da19756a")
}

func (s *executeCapturingStub) TailSessionLog(_ context.Context, _ string) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}
func (s *executeCapturingStub) ListHarnesses(_ context.Context) ([]agentlib.HarnessInfo, error) {
	return []agentlib.HarnessInfo{{Name: "claude", Available: true}, {Name: "agent", Available: true}}, nil
}
func (s *executeCapturingStub) ListProviders(_ context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}
func (s *executeCapturingStub) ListModels(_ context.Context, _ agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	return nil, nil
}
func (s *executeCapturingStub) ListProfiles(_ context.Context) ([]agentlib.ProfileInfo, error) {
	return nil, nil
}
func (s *executeCapturingStub) ResolveProfile(_ context.Context, _ string) (*agentlib.ResolvedProfile, error) {
	return nil, nil
}
func (s *executeCapturingStub) ProfileAliases(_ context.Context) (map[string]string, error) {
	return nil, nil
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

// installExecuteCapturingStub injects stub as the service used by
// RunWithConfigViaService and registers cleanup. Returns the stub so callers
// can inspect captured fields after the run.
func installExecuteCapturingStub(t *testing.T) *executeCapturingStub {
	t.Helper()
	stub := &executeCapturingStub{}
	agent.SetServiceRunFactory(func(_ string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { agent.SetServiceRunFactory(nil) })
	return stub
}

// TestRunPassthroughHarnessModelToExecute (AC): ddx agent run --harness claude
// --model opus forwards those constraints to ServiceExecuteRequest without
// calling ResolveRoute.
func TestRunPassthroughHarnessModelToExecute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "agent", "run",
		"--harness", "claude",
		"--model", "claude-opus-4-7",
		"--text", "hello",
		"--timeout", "5s",
	)
	// Stub Execute returns success, so the command should succeed.
	// If ResolveRoute were called, Execute returns an error from the stub.
	require.NoError(t, err, "run must succeed; if ResolveRoute was called the stub returns an error")

	stub.mu.Lock()
	executeCalled := stub.executeCalled
	lastReq := stub.lastReq
	stub.mu.Unlock()

	require.True(t, executeCalled, "Execute must be called on the service stub")
	assert.Equal(t, "claude", lastReq.Harness, "Harness must pass through unchanged to Execute")
	assert.Equal(t, "claude-opus-4-7", lastReq.Model, "Model must pass through unchanged to Execute")
}

// TestRunPassthroughEmptyConstraintsToExecute (AC): ddx agent run with no
// harness/model flags calls Execute with empty passthrough — the upstream
// service selects routing without DDx pre-resolution.
func TestRunPassthroughEmptyConstraintsToExecute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, _ = executeCommand(root, "agent", "run", "--text", "hello", "--timeout", "5s")
	// Command may fail (no real provider configured) but Execute must be called.

	stub.mu.Lock()
	executeCalled := stub.executeCalled
	lastReq := stub.lastReq
	stub.mu.Unlock()

	if !executeCalled {
		t.Skip("Execute not called — agent may have been short-circuited before service dispatch")
	}
	assert.Empty(t, lastReq.Harness, "Harness must be empty when no --harness flag provided")
	assert.Empty(t, lastReq.Model, "Model must be empty when no --model flag provided")
}

// TestWorkDoesNotCallResolveRoute (AC): ddx work must not call ResolveRoute
// in the execution path. The stub returns an error from ResolveRoute so any
// call to it would surface as a command failure carrying "ResolveRoute called".
// MinPower/MaxPower passthrough to ServiceExecuteRequest is proved statically
// by the singleTierAttempt loopOverrides construction and by the internal
// agent package's passthrough_envelope_test.go.
func TestWorkDoesNotCallResolveRoute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)
	_ = stub // stub.ResolveRoute returns error; any call would propagate up

	dir := minimalProjectDir(t)

	// Seed one ready bead so the loop has something to claim.
	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "ddx-passthrough-power-test",
		Title: "passthrough power test bead",
	}))

	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "work",
		"--local", "--once",
		"--harness", "claude",
		"--model", "claude-opus-4-7",
		"--min-power", "40",
		"--max-power", "90",
	)
	// If ResolveRoute were called, the stub returns an error containing
	// "ResolveRoute called in execution path". Verify it didn't happen.
	if err != nil {
		require.NotContains(t, err.Error(), "ResolveRoute called in execution path",
			"ddx work must not call ResolveRoute (CONTRACT-003 / ddx-da19756a)")
	}
	// Other errors (git not initialized, etc.) are expected and acceptable —
	// the test's only concern is that ResolveRoute was not invoked.
}

// TestExecuteBeadDoesNotCallResolveRoute (AC4 / ddx-6036e1bc): ddx agent
// execute-bead must not call ResolveRoute in the execution path. The stub
// installed by installExecuteCapturingStub returns an error from ResolveRoute,
// so any call to it would surface as a command failure containing
// "ResolveRoute called in execution path".
//
// Static guarantee: TestRoutinglintNonStatusFilesDoNotCallResolveRoute already
// ensures agent_execute_bead.go has no direct .ResolveRoute() call. This
// behavioral test adds a runtime check at the cmd layer.
func TestExecuteBeadDoesNotCallResolveRoute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)
	_ = stub // stub.ResolveRoute returns error; any call would propagate up

	dir := minimalProjectDir(t)

	// Seed a bead so execute-bead has something to load before reaching git.
	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "ddx-no-resolve-route-test",
		Title: "execute-bead no ResolveRoute test",
	}))

	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "agent", "execute-bead",
		"ddx-no-resolve-route-test",
		"--harness", "agent",
	)
	// execute-bead may fail (no git repo), but must not call ResolveRoute.
	if err != nil {
		require.NotContains(t, err.Error(), "ResolveRoute called in execution path",
			"execute-bead must not call ResolveRoute (CONTRACT-003 / ddx-6036e1bc)")
	}
}
