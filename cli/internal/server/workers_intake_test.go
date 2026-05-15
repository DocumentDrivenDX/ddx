package server

import (
	"context"
	"fmt"
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

type workerIntakeServiceStub struct {
	mu         sync.Mutex
	modes      []string
	listModels []agentlib.ModelInfo
	executeErr error
	intakeErr  error // when set, returned from Execute for intake-mode calls
}

func (s *workerIntakeServiceStub) Execute(_ context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	mode := "execute"
	switch {
	case strings.Contains(req.Prompt, "MODE: intake"):
		mode = "intake"
	case strings.Contains(req.Prompt, "MODE: lint"):
		mode = "lint"
	case strings.Contains(req.Prompt, "MODE: triage"):
		mode = "triage"
	}
	s.mu.Lock()
	s.modes = append(s.modes, mode)
	s.mu.Unlock()

	if s.executeErr != nil && mode == "execute" {
		return nil, s.executeErr
	}
	if s.intakeErr != nil && mode == "intake" {
		return nil, s.intakeErr
	}

	finalText := `ok`
	switch mode {
	case "intake":
		finalText = `{"classification":"atomic","confidence":0.99,"reasoning":"single-slice"}`
	case "lint":
		finalText = `{"score":9,"rationale":"ok","suggested_fixes":[],"waivers_applied":[]}`
	case "triage":
		finalText = `{"classification":"already_satisfied","recommended_action":"close_already_satisfied","rationale":"ok","suggested_amendments":[],"suggested_followup_beads":[]}`
	}

	ch := make(chan agentlib.ServiceEvent, 1)
	ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":` + fmt.Sprintf("%q", finalText) + `}`)}
	close(ch)
	return ch, nil
}

func (s *workerIntakeServiceStub) ResolveRoute(_ context.Context, _ agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	return nil, fmt.Errorf("ResolveRoute should not be called in worker intake tests")
}

func (s *workerIntakeServiceStub) TailSessionLog(_ context.Context, _ string) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}

func (s *workerIntakeServiceStub) ListHarnesses(_ context.Context) ([]agentlib.HarnessInfo, error) {
	return []agentlib.HarnessInfo{{Name: "claude", Available: true}, {Name: "agent", Available: true}}, nil
}

func (s *workerIntakeServiceStub) ListProviders(_ context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}

func (s *workerIntakeServiceStub) ListModels(_ context.Context, _ agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	if len(s.listModels) > 0 {
		return append([]agentlib.ModelInfo(nil), s.listModels...), nil
	}
	return []agentlib.ModelInfo{{ID: "smart", Power: 94}}, nil
}

func (s *workerIntakeServiceStub) HealthCheck(_ context.Context, _ agentlib.HealthTarget) error {
	return nil
}

func (s *workerIntakeServiceStub) ListPolicies(_ context.Context) ([]agentlib.PolicyInfo, error) {
	return nil, nil
}

func (s *workerIntakeServiceStub) RecordRouteAttempt(_ context.Context, _ agentlib.RouteAttempt) error {
	return nil
}

func (s *workerIntakeServiceStub) RouteStatus(_ context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}

func (s *workerIntakeServiceStub) ListSessionLogs(_ context.Context) ([]agentlib.SessionLogEntry, error) {
	return nil, nil
}

func (s *workerIntakeServiceStub) WriteSessionLog(_ context.Context, _ string, _ io.Writer) error {
	return nil
}

func (s *workerIntakeServiceStub) ReplaySession(_ context.Context, _ string, _ io.Writer) error {
	return nil
}

func (s *workerIntakeServiceStub) UsageReport(_ context.Context, _ agentlib.UsageReportOptions) (*agentlib.UsageReport, error) {
	return nil, nil
}

func installWorkerIntakeStub(t *testing.T, stub *workerIntakeServiceStub) {
	t.Helper()
	agent.SetServiceRunFactory(func(_ string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { agent.SetServiceRunFactory(nil) })
}

func setupWorkerIntakeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("DDX_EXEC_WT_DIR", filepath.Join(root, ddxroot.DirName, "exec-worktrees"))
	ddxDir := filepath.Join(root, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	cfg := `version: "1.0"
bead-quality:
  lint:
    block_threshold_score: 1
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644))
	skillDir := filepath.Join(root, ".agents", "skills", "ddx", "bead-lifecycle")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("intake"), 0o644))
	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "ddx-worker-intake",
		Title: "worker intake wiring test bead",
	}))
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# test\n"), 0o644))
	runCmd(t, root, "git", "init")
	runCmd(t, root, "git", "config", "user.email", "test@example.com")
	runCmd(t, root, "git", "config", "user.name", "Test User")
	runCmd(t, root, "git", "add", ".")
	runCmd(t, root, "git", "commit", "--allow-empty", "-m", "init")
	return root
}

func TestServerWorker_WiresPreClaimIntakeHook(t *testing.T) {
	root := setupWorkerIntakeFixture(t)
	stub := &workerIntakeServiceStub{}
	installWorkerIntakeStub(t, stub)

	m := NewWorkerManager(root)
	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		ProjectRoot: root,
		Harness:     "claude",
		Mode:        "once",
		NoReview:    true,
	})
	require.NoError(t, err)
	_ = waitForWorkerExit(t, m, record.ID, 10*time.Second)

	stub.mu.Lock()
	got := append([]string(nil), stub.modes...)
	stub.mu.Unlock()

	require.GreaterOrEqual(t, len(got), 2, "server worker must invoke intake and lint hooks")
	assert.Equal(t, "intake", got[0], "server-managed workers must run intake before claim")
	assert.Equal(t, "lint", got[1], "server-managed workers must wire lint after intake")
}

// TestServerWorker_ReadinessUnavailableEvidence (AC2, AC6 / ddx-30bc30ed):
// when a server-managed worker encounters an intake service failure, it must
// record readiness-unavailable evidence in the worker log. The test fails if
// that evidence is dropped.
func TestServerWorker_ReadinessUnavailableEvidence(t *testing.T) {
	root := setupWorkerIntakeFixture(t)
	stub := &workerIntakeServiceStub{
		intakeErr: fmt.Errorf("readiness service unavailable: connection refused"),
	}
	installWorkerIntakeStub(t, stub)

	m := NewWorkerManager(root)
	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		ProjectRoot: root,
		Harness:     "claude",
		Mode:        "once",
		NoReview:    true,
	})
	require.NoError(t, err)
	_ = waitForWorkerExit(t, m, record.ID, 10*time.Second)

	// AC2: worker log must contain actionable readiness-unavailable evidence.
	logContent, _, err := m.Logs(record.ID)
	require.NoError(t, err)
	assert.Contains(t, logContent, "readiness check unavailable",
		"server worker must record readiness-unavailable evidence in worker log; got: %s", logContent)
}
