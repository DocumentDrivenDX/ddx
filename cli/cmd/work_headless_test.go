package cmd

import (
	"os"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkHeadlessPhaseDispatchDisablesClaudeTUIDefault(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv(agent.FizeauDisableClaudeTUIDefaultEnv, "operator-value")
	stub := installExecuteCapturingStub(t)

	var mu sync.Mutex
	var valuesAtDispatch []string
	stub.executeFn = func(_ agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		mu.Lock()
		valuesAtDispatch = append(valuesAtDispatch, os.Getenv(agent.FizeauDisableClaudeTUIDefaultEnv))
		mu.Unlock()

		ch := make(chan agentlib.ServiceEvent, 1)
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"ready\",\"rationale\":\"ok\",\"readiness_checks\":[],\"score\":9,\"suggested_fixes\":[],\"waivers_applied\":[],\"recommended_action\":\"release_claim_retry\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}"}`)}
		close(ch)
		return ch, nil
	}

	dir := setupWorkIntakeFixture(t)
	factory := NewCommandFactory(dir)
	factory.workInteractiveTerminalOverride = func() bool { return false }
	_, _ = executeCommand(
		factory.NewRootCommand(),
		"work",
		"--once",
		"--project", dir,
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)

	mu.Lock()
	observed := append([]string(nil), valuesAtDispatch...)
	mu.Unlock()
	require.NotEmpty(t, observed, "headless work must reach the captured phase-dispatch boundary")
	for _, value := range observed {
		assert.Equal(t, "1", value, "every headless work phase must disable Fizeau's claude-tui default before Execute")
	}
	assert.Equal(t, "operator-value", os.Getenv(agent.FizeauDisableClaudeTUIDefaultEnv),
		"work completion must restore the caller's prior routing environment")
}

func TestWorkHeadlessExplicitClaudeTUIFailsBeforeDispatch(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	dir := minimalProjectDir(t)
	factory := NewCommandFactory(dir)
	factory.workInteractiveTerminalOverride = func() bool { return false }

	out, err := executeCommand(
		factory.NewRootCommand(),
		"work",
		"--once",
		"--project", dir,
		"--harness", "claude-tui",
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a PTY-capable interactive terminal")
	assert.Contains(t, out, "no controlling terminal is attached")
	assert.False(t, stub.executeCalled, "explicit headless claude-tui must fail before phase dispatch")
}
