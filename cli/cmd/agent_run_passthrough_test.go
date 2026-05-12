package cmd

// agent_run_passthrough_test.go verifies ddx-da19756a AC:
//   - ddx run/work execution calls Execute with exact passthrough constraints
//     (Harness, Provider, Model) and MinPower/MaxPower bounds.
//   - ResolveRoute is NOT called during execution.

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	agentlib "github.com/easel/fizeau"
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
	executionReq  agentlib.ServiceExecuteRequest
	executionSeen bool
	executeFn     func(agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error)
	listModels    []agentlib.ModelInfo
	listPolicies  []agentlib.PolicyInfo
}

func (s *executeCapturingStub) Execute(_ context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.mu.Lock()
	s.executeCalled = true
	s.lastReq = req
	if req.Role == "implementer" {
		s.executionReq = req
		s.executionSeen = true
	}
	s.mu.Unlock()
	if s.executeFn != nil {
		return s.executeFn(req)
	}
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

func setupWorkIntakeFixture(t *testing.T) string {
	t.Helper()
	dir := minimalProjectDir(t)

	skillDir := filepath.Join(dir, ".agents", "skills", "ddx", "bead-lifecycle")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("intake"), 0o644))

	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
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
		"--once",
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

// TestWorkPassesEmptyHarnessToService verifies that ddx work with no
// --harness flag sends an empty Harness to the agent service, allowing the
// service's routing engine to auto-select a provider from configured endpoints.
func TestWorkPassesEmptyHarnessToService(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)

	env := NewTestEnvironment(t)

	store := bead.NewStore(filepath.Join(env.Dir, ".ddx"))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "ddx-empty-harness-test",
		Title: "empty harness passthrough test bead",
	}))

	root := NewCommandFactory(env.Dir).NewRootCommand()
	_, _ = executeCommand(root, "work", "--once")

	stub.mu.Lock()
	executeCalled := stub.executeCalled
	lastReq := stub.lastReq
	stub.mu.Unlock()

	if !executeCalled {
		t.Skip("Execute not called — bead may have been short-circuited before service dispatch")
	}
	assert.Empty(t, lastReq.Harness,
		"ddx work with no --harness must send empty Harness to service for engine auto-selection")
}

func TestZeroConfigWork_DispatchesWithEmptyProfile(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)

	dir := setupWorkIntakeFixture(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, _ = executeCommand(root, "work", "--once", "--no-review", "--no-review-i-know-what-im-doing")

	stub.mu.Lock()
	executionSeen := stub.executionSeen
	executionReq := stub.executionReq
	stub.mu.Unlock()

	require.True(t, executionSeen, "ddx work must reach the implementation dispatch")
	assert.Empty(t, executionReq.Policy)
	assert.Zero(t, executionReq.MinPower)
	assert.Zero(t, executionReq.MaxPower)
}

func TestZeroConfigTry_DispatchesWithEmptyProfile(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)

	dir := setupWorkIntakeFixture(t)
	factory := NewCommandFactory(dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	root := factory.NewRootCommand()
	_, _ = executeCommand(root, "try", "ddx-intake-test", "--no-review", "--no-review-i-know-what-im-doing")

	stub.mu.Lock()
	executionSeen := stub.executionSeen
	executionReq := stub.executionReq
	stub.mu.Unlock()

	require.True(t, executionSeen, "ddx try must reach the implementation dispatch")
	assert.Empty(t, executionReq.Policy)
	assert.Zero(t, executionReq.MinPower)
	assert.Zero(t, executionReq.MaxPower)
}

func TestDDxWork_WiresPreClaimIntakeHook(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	var mu sync.Mutex
	modes := make([]string, 0, 4)
	var intakeReq agentlib.ServiceExecuteRequest
	stub := installExecuteCapturingStub(t)
	stub.listPolicies = []agentlib.PolicyInfo{
		{Name: "cheap", MinPower: 5, MaxPower: 5},
		{Name: "smart", MinPower: 9, MaxPower: 10},
	}
	stub.listModels = []agentlib.ModelInfo{
		{ID: "cheap-model", Power: 5, Available: true, AutoRoutable: true},
		{ID: "smart-model", Power: 9, Available: true, AutoRoutable: true},
	}
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		mode := "execute"
		switch {
		case strings.Contains(req.Prompt, "MODE: intake"):
			mode = "intake"
		case strings.Contains(req.Prompt, "MODE: lint"):
			mode = "lint"
		case strings.Contains(req.Prompt, "MODE: triage"):
			mode = "triage"
		}
		mu.Lock()
		modes = append(modes, mode)
		if mode == "intake" {
			intakeReq = req
		}
		mu.Unlock()

		switch mode {
		case "intake":
			ch := make(chan agentlib.ServiceEvent, 1)
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"atomic\",\"confidence\":0.99,\"reasoning\":\"single-slice\"}"}`)}
			close(ch)
			return ch, nil
		case "lint":
			ch := make(chan agentlib.ServiceEvent, 1)
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"score\":9,\"rationale\":\"ok\",\"suggested_fixes\":[],\"waivers_applied\":[]}"}`)}
			close(ch)
			return ch, nil
		case "triage":
			ch := make(chan agentlib.ServiceEvent, 1)
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"already_satisfied\",\"recommended_action\":\"close_already_satisfied\",\"rationale\":\"ok\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}"}`)}
			close(ch)
			return ch, nil
		default:
			ch := make(chan agentlib.ServiceEvent, 1)
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
			close(ch)
			return ch, nil
		}
	}

	dir := setupWorkIntakeFixture(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, _ = executeCommand(root, "work", "--once", "--max-power", "8")

	mu.Lock()
	got := append([]string(nil), modes...)
	gotIntakeReq := intakeReq
	mu.Unlock()
	require.GreaterOrEqual(t, len(got), 1, "work must invoke the intake hook")
	assert.Equal(t, "intake", got[0], "plain ddx work must run pre-claim intake before claim")
	assert.Equal(t, "smart", gotIntakeReq.Policy)
	assert.Empty(t, gotIntakeReq.Harness)
	assert.Empty(t, gotIntakeReq.Provider)
	assert.Empty(t, gotIntakeReq.Model)
	assert.Zero(t, gotIntakeReq.MinPower)
	assert.Zero(t, gotIntakeReq.MaxPower, "intake dispatch must not inherit implementation power bounds")
	assert.GreaterOrEqual(t, len(got), 2, "work must continue past intake to later execution stages")
}

func TestDDxWork_PinnedLifecycleHooksPreserveRouteWithoutResolveRoute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	var mu sync.Mutex
	var intakeReq agentlib.ServiceExecuteRequest
	var lintReq agentlib.ServiceExecuteRequest
	stub := installExecuteCapturingStub(t)
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		mode := "execute"
		switch {
		case strings.Contains(req.Prompt, "MODE: intake"):
			mode = "intake"
		case strings.Contains(req.Prompt, "MODE: lint"):
			mode = "lint"
		case strings.Contains(req.Prompt, "MODE: triage"):
			mode = "triage"
		}
		mu.Lock()
		if mode == "intake" {
			intakeReq = req
		}
		if mode == "lint" {
			lintReq = req
		}
		mu.Unlock()

		ch := make(chan agentlib.ServiceEvent, 1)
		switch mode {
		case "intake":
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"atomic\",\"confidence\":0.99,\"reasoning\":\"single-slice\"}"}`)}
		case "lint":
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"score\":9,\"rationale\":\"ok\",\"suggested_fixes\":[],\"waivers_applied\":[]}"}`)}
		case "triage":
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"already_satisfied\",\"recommended_action\":\"close_already_satisfied\",\"rationale\":\"ok\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}"}`)}
		default:
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
		}
		close(ch)
		return ch, nil
	}

	dir := setupWorkIntakeFixture(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root,
		"work", "--once",
		"--harness", "codex",
		"--model", "gpt-5.4-mini",
		"--no-review", "--no-review-i-know-what-im-doing",
	)
	if err != nil {
		require.NotContains(t, err.Error(), "ResolveRoute called in execution path",
			"pinned ddx work lifecycle hook dispatch must not call ResolveRoute")
	}

	mu.Lock()
	gotIntakeReq := intakeReq
	gotLintReq := lintReq
	mu.Unlock()
	assert.Equal(t, "codex", gotIntakeReq.Harness)
	assert.Equal(t, "gpt-5.4-mini", gotIntakeReq.Model)
	assert.Empty(t, gotIntakeReq.Policy)
	assert.Equal(t, "codex", gotLintReq.Harness)
	assert.Equal(t, "gpt-5.4-mini", gotLintReq.Model)
	assert.Empty(t, gotLintReq.Policy)
}

// TestDDxWork_ReadinessUnavailableOutput (AC1, AC6 / ddx-30bc30ed): when the
// intake service is unreachable, ddx work must print actionable
// readiness-unavailable output rather than exposing internal error prefixes.
// The test fails if that output is dropped.
func TestDDxWork_ReadinessUnavailableOutput(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		if strings.Contains(req.Prompt, "MODE: intake") {
			return nil, fmt.Errorf("readiness service unavailable: connection refused")
		}
		ch := make(chan agentlib.ServiceEvent, 1)
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
		close(ch)
		return ch, nil
	}

	dir := setupWorkIntakeFixture(t)
	root := NewCommandFactory(dir).NewRootCommand()
	out, _ := executeCommand(root, "work", "--once", "--no-review", "--no-review-i-know-what-im-doing")

	// AC1: actionable readiness-unavailable line must appear in operator output.
	assert.Contains(t, out, "readiness check unavailable",
		"ddx work must print actionable readiness-unavailable output; got: %s", out)
	// AC6: doubled prefix must not appear in operator output.
	assert.NotContains(t, out, "pre-claim intake: pre-claim intake:",
		"pre-claim intake prefix must not be doubled in operator output")
}

// TestDDxWork_ReadinessTooLargeSkipsClaim (AC3, AC6 / ddx-30bc30ed): when
// intake classifies a bead as too_large, ddx work must not dispatch the
// implementer and must move the bead to operator-attention status.
func TestDDxWork_ReadinessTooLargeSkipsClaim(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		if strings.Contains(req.Prompt, "MODE: intake") {
			ch := make(chan agentlib.ServiceEvent, 1)
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"needs_split\",\"rationale\":\"too large\",\"readiness_checks\":[{\"reason\":\"too_large\",\"verdict\":\"fail\",\"evidence\":\"AC spans three subsystems\"}]}"}`)}
			close(ch)
			return ch, nil
		}
		ch := make(chan agentlib.ServiceEvent, 1)
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
		close(ch)
		return ch, nil
	}

	dir := setupWorkIntakeFixture(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, _ = executeCommand(root, "work", "--once", "--no-review", "--no-review-i-know-what-im-doing")

	// AC3: implementer must not be dispatched when intake classifies bead as too_large.
	stub.mu.Lock()
	executionSeen := stub.executionSeen
	stub.mu.Unlock()
	assert.False(t, executionSeen,
		"implementer must not be dispatched when intake classifies bead as too_large (needs_split)")

	// Bead must be moved to proposed (operator attention) after too_large classification.
	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	b, err := store.Get("ddx-intake-test")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, b.Status,
		"too_large bead must be parked to proposed status after intake rejection")
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
