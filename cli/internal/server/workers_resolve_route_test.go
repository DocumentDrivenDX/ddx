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
	"strings"
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

	finalText := "ok"
	if req.Permissions == "safe" {
		finalText = `{"classification":"ready","score":100,"rationale":"ready","difficulty":{"estimated_difficulty":"small"},"readiness_checks":[{"name":"scope","verdict":true}]}`
	}
	ch := make(chan agentlib.ServiceEvent, 1)
	final := map[string]any{
		"status":      "success",
		"final_text":  finalText,
		"exit_code":   0,
		"error":       "",
		"session_log": "session.log",
		"routing_actual": map[string]any{
			"harness":  nonEmptyOr(req.Harness, "claude-tui"),
			"provider": nonEmptyOr(req.Provider, "anthropic"),
			"model":    nonEmptyOr(req.Model, "opus-4.7"),
			"power":    positiveOr(req.MinPower, 10),
		},
	}
	data, err := json.Marshal(final)
	if err != nil {
		return nil, err
	}
	ch <- agentlib.ServiceEvent{Type: "final", Data: data}
	close(ch)
	return ch, nil
}

func nonEmptyOr(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func prefix(value string, limit int) string {
	value = strings.ReplaceAll(value, "\n", "\\n")
	if len(value) <= limit {
		return value
	}
	return value[:limit]
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
	t.Setenv("DDX_EXEC_WT_DIR", filepath.Join(root, ddxroot.DirName, "exec-worktrees"))
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# test\n"), 0o644))
	runCmd(t, root, "git", "init", "-b", "main")
	runCmd(t, root, "git", "config", "user.name", "Test")
	runCmd(t, root, "git", "config", "user.email", "test@test.local")
	runCmd(t, root, "git", "add", "-A")
	runCmd(t, root, "git", "commit", "-m", "init")

	ddxDir := filepath.Join(root, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
bead-quality:
  lint:
    block_threshold_score: 0
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(""), 0o644))
	skillDir := filepath.Join(root, ".agents", "skills", "ddx", "bead-lifecycle")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("intake"), 0o644))
	return root
}

func seedWorkerResolveRouteBead(t *testing.T, root, id string) {
	t.Helper()
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    id,
		Title: "worker resolve-route regression bead",
	}))
	runCmd(t, root, "git", "add", "-A")
	runCmd(t, root, "git", "commit", "-m", "seed worker bead")
}

// TestWorkerExecutionDoesNotCallResolveRouteForPinnedProfileOrModel verifies
// that server-managed work workers proceed through ExecuteBeadWithConfig
// without consulting ResolveRoute, even when the worker spec pins a model.
func TestWorkerExecutionDoesNotCallResolveRouteForPinnedProfileOrModel(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	svc := installResolveRouteFailingService(t)
	root := setupWorkerResolveRouteRepo(t)
	seedWorkerResolveRouteBead(t, root, "ddx-worker-resolve-route-test")

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	m.LandCoordinators.gitOpsOverride = &fakeLandingGitOps{}
	t.Cleanup(func() {
		m.LandCoordinators.StopAll()
	})

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Harness:  "fiz",
		Model:    "gpt-5.4-mini",
		NoReview: true,
		Mode:     "once",
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return svc.executeCalled
	}, 2*time.Second, 10*time.Millisecond, "worker must reach Execute promptly")

	final := waitForWorkerExit(t, m, record.ID, 15*time.Second)
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

// TestWorkerExecutionDefaultProfileUsesServiceExecute verifies the unpinned
// server-managed worker path used by project canaries. A default profile must
// still reach Fizeau Execute with unrestricted permissions and without a
// ResolveRoute preflight.
func TestWorkerExecutionDefaultProfileUsesServiceExecute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	svc := installResolveRouteFailingService(t)
	root := setupWorkerResolveRouteRepo(t)
	seedWorkerResolveRouteBead(t, root, "ddx-worker-default-profile-route-test")

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	m.LandCoordinators.gitOpsOverride = &fakeLandingGitOps{}
	t.Cleanup(func() {
		m.LandCoordinators.StopAll()
	})

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Profile:  "default",
		NoReview: true,
		Mode:     "once",
	})
	require.NoError(t, err)

	if !assert.Eventually(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for _, req := range svc.executeReqs {
			if req.Role == "implementer" {
				return true
			}
		}
		return false
	}, 20*time.Second, 10*time.Millisecond, "worker must reach implementation Execute promptly") {
		svc.mu.Lock()
		reqs := append([]agentlib.ServiceExecuteRequest(nil), svc.executeReqs...)
		svc.mu.Unlock()
		for i, req := range reqs {
			t.Logf("request[%d]: role=%q policy=%q harness=%q model=%q permissions=%q prompt_prefix=%q", i, req.Role, req.Policy, req.Harness, req.Model, req.Permissions, prefix(req.Prompt, 80))
		}
		current, showErr := m.Show(record.ID)
		t.Logf("worker state: record=%+v err=%v", current, showErr)
		if data, readErr := os.ReadFile(filepath.Join(root, record.StdoutPath)); readErr == nil {
			t.Logf("worker log:\n%s", string(data))
		} else {
			t.Logf("worker log read error: %v", readErr)
		}
		t.FailNow()
	}
	require.NoError(t, m.Stop(record.ID))

	svc.mu.Lock()
	executeCalled := svc.executeCalled
	resolveCalled := svc.resolveCalled
	var implementerReq agentlib.ServiceExecuteRequest
	for _, req := range svc.executeReqs {
		if req.Role == "implementer" {
			implementerReq = req
			break
		}
	}
	svc.mu.Unlock()

	require.True(t, executeCalled, "worker must reach Execute")
	assert.False(t, resolveCalled, "worker execution must not call ResolveRoute")
	assert.Equal(t, "default", implementerReq.Policy, "profile must pass through to Fizeau Execute")
	assert.Equal(t, "unrestricted", implementerReq.Permissions, "execute-bead workers must run with unrestricted permissions in the isolated worktree")
	assert.Empty(t, implementerReq.Harness, "default worker path must not synthesize a harness pin")
	assert.Empty(t, implementerReq.Model, "default worker path must not synthesize a model pin")
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
