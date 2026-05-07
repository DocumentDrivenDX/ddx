package server

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type resolveRouteFailingService struct {
	mu             sync.Mutex
	executeCalled  bool
	resolveCalled  bool
	lastExecuteReq agentlib.ServiceExecuteRequest
}

func (s *resolveRouteFailingService) Execute(_ context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.mu.Lock()
	s.executeCalled = true
	s.lastExecuteReq = req
	s.mu.Unlock()

	ch := make(chan agentlib.ServiceEvent, 1)
	final := map[string]any{
		"status":      "success",
		"final_text":  "ok",
		"exit_code":   0,
		"error":       "",
		"session_log": "session.log",
	}
	data, err := json.Marshal(final)
	if err != nil {
		return nil, err
	}
	ch <- agentlib.ServiceEvent{Type: "final", Data: data}
	close(ch)
	return ch, nil
}

func (s *resolveRouteFailingService) ResolveRoute(_ context.Context, _ agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	s.mu.Lock()
	s.resolveCalled = true
	s.mu.Unlock()
	return nil, fmt.Errorf("routinglint: ResolveRoute called in execution path")
}

func (s *resolveRouteFailingService) TailSessionLog(_ context.Context, _ string) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}

func (s *resolveRouteFailingService) ListHarnesses(_ context.Context) ([]agentlib.HarnessInfo, error) {
	return []agentlib.HarnessInfo{{Name: "fiz", Available: true}}, nil
}

func (s *resolveRouteFailingService) ListProviders(_ context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}

func (s *resolveRouteFailingService) ListModels(_ context.Context, _ agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	return nil, nil
}

func (s *resolveRouteFailingService) ListProfiles(_ context.Context) ([]agentlib.ProfileInfo, error) {
	return nil, nil
}

func (s *resolveRouteFailingService) ResolveProfile(_ context.Context, _ string) (*agentlib.ResolvedProfile, error) {
	return nil, nil
}

func (s *resolveRouteFailingService) ProfileAliases(_ context.Context) (map[string]string, error) {
	return nil, nil
}

func (s *resolveRouteFailingService) HealthCheck(_ context.Context, _ agentlib.HealthTarget) error {
	return nil
}

func (s *resolveRouteFailingService) RecordRouteAttempt(_ context.Context, _ agentlib.RouteAttempt) error {
	return nil
}

func (s *resolveRouteFailingService) RouteStatus(_ context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}

func (s *resolveRouteFailingService) UsageReport(_ context.Context, _ agentlib.UsageReportOptions) (*agentlib.UsageReport, error) {
	return nil, nil
}

func (s *resolveRouteFailingService) ListSessionLogs(_ context.Context) ([]agentlib.SessionLogEntry, error) {
	return nil, nil
}

func (s *resolveRouteFailingService) WriteSessionLog(_ context.Context, _ string, _ io.Writer) error {
	return nil
}

func (s *resolveRouteFailingService) ReplaySession(_ context.Context, _ string, _ io.Writer) error {
	return nil
}

func installResolveRouteFailingService(t *testing.T) *resolveRouteFailingService {
	t.Helper()
	svc := &resolveRouteFailingService{}
	agent.SetServiceRunFactory(func(_ string) (agentlib.FizeauService, error) {
		return svc, nil
	})
	t.Cleanup(func() {
		agent.SetServiceRunFactory(nil)
	})
	return svc
}

func setupWorkerResolveRouteRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# test\n"), 0o644))
	runCmd(t, root, "git", "init", "-b", "main")
	runCmd(t, root, "git", "config", "user.name", "Test")
	runCmd(t, root, "git", "config", "user.email", "test@test.local")
	runCmd(t, root, "git", "add", "-A")
	runCmd(t, root, "git", "commit", "-m", "init")

	ddxDir := filepath.Join(root, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(""), 0o644))
	return root
}

// TestWorkerExecutionDoesNotCallResolveRouteForPinnedProfileOrModel verifies
// that server-managed execute-loop workers proceed through ExecuteBeadWithConfig
// without consulting ResolveRoute, even when the worker spec pins a model.
func TestWorkerExecutionDoesNotCallResolveRouteForPinnedProfileOrModel(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	svc := installResolveRouteFailingService(t)
	root := setupWorkerResolveRouteRepo(t)
	store := bead.NewStore(filepath.Join(root, ".ddx"))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "ddx-worker-resolve-route-test",
		Title: "worker resolve-route regression bead",
	}))

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Harness: "fiz",
		Model:   "gpt-5.4-mini",
		Once:    true,
	})
	require.NoError(t, err)

	final := waitForWorkerExit(t, m, record.ID, 10*time.Second)
	assert.NotEqual(t, "failed", final.State, "worker should complete without a ResolveRoute failure")

	svc.mu.Lock()
	executeCalled := svc.executeCalled
	resolveCalled := svc.resolveCalled
	lastReq := svc.lastExecuteReq
	svc.mu.Unlock()

	require.True(t, executeCalled, "worker must reach Execute")
	assert.False(t, resolveCalled, "worker execution must not call ResolveRoute")
	assert.Equal(t, "fiz", lastReq.Harness, "harness pin must pass through to Execute")
	assert.Equal(t, "gpt-5.4-mini", lastReq.Model, "model pin must pass through to Execute")
}

// TestWorkerRoutinglintNoResolveRouteInExecutionPaths guards the server-side
// execution path against future ResolveRoute reintroduction in workers.go.
func TestWorkerRoutinglintNoResolveRouteInExecutionPaths(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "workers.go", nil, 0)
	require.NoError(t, err)

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == "ResolveRoute" {
			t.Errorf("routinglint: workers.go calls .ResolveRoute() at %s — execution paths must not pre-resolve routes", fset.Position(call.Pos()))
		}
		return true
	})
}
