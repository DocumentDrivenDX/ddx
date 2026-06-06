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
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type resolveRouteFailingService struct {
	mu             sync.Mutex
	executeCalled  bool
	resolveCalled  bool
	lastExecuteReq agentlib.ServiceExecuteRequest
	executeReqs    []agentlib.ServiceExecuteRequest
}

func (s *resolveRouteFailingService) Execute(_ context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.mu.Lock()
	s.executeCalled = true
	s.lastExecuteReq = req
	s.executeReqs = append(s.executeReqs, req)
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

func (s *resolveRouteFailingService) ListPolicies(_ context.Context) ([]agentlib.PolicyInfo, error) {
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

	ddxDir := filepath.Join(root, ddxroot.DirName)
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
// that server-managed work workers proceed through ExecuteBeadWithConfig
// without consulting ResolveRoute, even when the worker spec pins a model.
func TestWorkerExecutionDoesNotCallResolveRouteForPinnedProfileOrModel(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	svc := installResolveRouteFailingService(t)
	root := setupWorkerResolveRouteRepo(t)
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "ddx-worker-resolve-route-test",
		Title: "worker resolve-route regression bead",
	}))

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Harness: "fiz",
		Model:   "gpt-5.4-mini",
		Mode:    "once",
	})
	require.NoError(t, err)

	final := waitForWorkerExit(t, m, record.ID, 30*time.Second)
	assert.NotEqual(t, "failed", final.State, "worker should complete without a ResolveRoute failure")

	svc.mu.Lock()
	executeCalled := svc.executeCalled
	resolveCalled := svc.resolveCalled
	lastReq := svc.lastExecuteReq
	allReqs := append([]agentlib.ServiceExecuteRequest(nil), svc.executeReqs...)
	svc.mu.Unlock()

	require.True(t, executeCalled, "worker must reach Execute")
	assert.False(t, resolveCalled, "worker execution must not call ResolveRoute")
	foundPinnedExecute := false
	for _, req := range append(allReqs, lastReq) {
		if req.Harness == "fiz" && req.Model == "gpt-5.4-mini" {
			foundPinnedExecute = true
			break
		}
	}
	assert.True(t, foundPinnedExecute, "harness/model pins must pass through to the primary Execute call")
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
