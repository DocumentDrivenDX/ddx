package cmd

// run_cmd_test.go verifies ddx-0248f921 AC:
//   - ddx run --help exits 0 and describes the layer-1 command.
//   - ddx run forwards harness/provider/model to Execute.
//   - ddx run forwards min-power/max-power bounds to Execute.
//   - ddx run --persona injects persona body into the prompt.
//   - ddx run does not call ResolveRoute (CONTRACT-003).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunCommandHelp verifies AC1: ddx run --help exits 0.
func TestRunCommandHelp(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	out, err := executeCommand(root, "run", "--help")
	require.NoError(t, err, "ddx run --help must exit 0")
	assert.Contains(t, out, "run", "help output must describe the run command")
}

// TestRunCommandRegisteredInRoot verifies AC2: the root command exposes run.
func TestRunCommandRegisteredInRoot(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	root := NewCommandFactory(minimalProjectDir(t)).NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	require.NoError(t, err, "root command must register ddx run")
	require.NotNil(t, runCmd, "root command must expose ddx run")
	assert.Equal(t, "run", runCmd.Use)
}

// TestRunPassthroughHarnessModelProviderToExecute verifies AC3: ddx run
// forwards --harness, --model, and --provider to ServiceExecuteRequest without
// calling ResolveRoute (CONTRACT-003).
func TestRunPassthroughHarnessModelProviderToExecute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "run",
		"--harness", "codex",
		"--model", "gpt-5.4",
		"--provider", "openai",
		"--text", "hello",
		"--timeout", "5s",
	)
	require.NoError(t, err, "ddx run must succeed; ResolveRoute call would fail the stub")

	stub.mu.Lock()
	executeCalled := stub.executeCalled
	lastReq := stub.lastReq
	stub.mu.Unlock()

	require.True(t, executeCalled, "Execute must be called")
	assert.Equal(t, "codex", lastReq.Harness, "Harness must pass through unchanged")
	assert.Equal(t, "gpt-5.4", lastReq.Model, "Model must pass through unchanged")
	assert.Equal(t, "openai", lastReq.Provider, "Provider must pass through unchanged")
}

// TestRunPassthroughMinMaxPowerToExecute verifies AC3: ddx run forwards
// --min-power and --max-power bounds to ServiceExecuteRequest unchanged.
func TestRunPassthroughMinMaxPowerToExecute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "run",
		"--min-power", "30",
		"--max-power", "80",
		"--text", "hello",
		"--timeout", "5s",
	)
	require.NoError(t, err)

	stub.mu.Lock()
	executeCalled := stub.executeCalled
	lastReq := stub.lastReq
	stub.mu.Unlock()

	require.True(t, executeCalled, "Execute must be called")
	assert.Equal(t, 30, lastReq.MinPower, "MinPower must pass through unchanged")
	assert.Equal(t, 80, lastReq.MaxPower, "MaxPower must pass through unchanged")
}

// TestRunDoesNotCallResolveRoute verifies AC5: ddx run must not call
// ResolveRoute in the execution path (CONTRACT-003).
func TestRunDoesNotCallResolveRoute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)
	_ = stub

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "run",
		"--harness", "claude",
		"--text", "hello",
		"--timeout", "5s",
	)
	if err != nil {
		require.NotContains(t, err.Error(), "ResolveRoute called in execution path",
			"ddx run must not call ResolveRoute (CONTRACT-003 / ddx-0248f921)")
	}
}

func TestRunJSONFinalErrorExitsNonZero(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		ch := make(chan agentlib.ServiceEvent, 1)
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"error","exit_code":0,"error":"ResolveRoute: no viable routing candidate: 3 candidates rejected"}`)}
		close(ch)
		return ch, nil
	}

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	out, err := executeCommand(root, "run",
		"--text", "hello",
		"--json",
		"--timeout", "5s",
	)
	require.Error(t, err)
	assert.Contains(t, out, `"exit_code": 1`)
	assert.Contains(t, out, `"error": "ResolveRoute: no viable routing candidate: 3 candidates rejected"`)
	assert.Contains(t, err.Error(), "agent exited with code 1")
}

// TestRunPersonaInjectsBodyIntoPrompt verifies AC3: ddx run --persona loads
// the named persona and prepends its body to the prompt dispatched to Execute.
func TestRunPersonaInjectsBodyIntoPrompt(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	var capturedPrompt string
	agent.SetServiceRunFactory(func(_ string) (agentlib.FizeauService, error) {
		return &stubAgentService{
			execute: func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
				capturedPrompt = req.Prompt
				ch := make(chan agentlib.ServiceEvent, 1)
				ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
				close(ch)
				return ch, nil
			},
		}, nil
	})
	t.Cleanup(func() { agent.SetServiceRunFactory(nil) })

	dir := minimalProjectDir(t)

	// Write a minimal persona file.
	personasDir := filepath.Join(dir, ".ddx", "personas")
	require.NoError(t, os.MkdirAll(personasDir, 0o755))
	personaBody := `---
name: test-reviewer
roles: [code-reviewer]
description: test persona
---
You are a strict code reviewer.`
	require.NoError(t, os.WriteFile(filepath.Join(personasDir, "test-reviewer.md"), []byte(personaBody), 0o644))

	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "run",
		"--persona", "test-reviewer",
		"--text", "review this code",
		"--timeout", "5s",
	)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(capturedPrompt, "You are a strict code reviewer."),
		"persona body must be prepended to prompt; got: %q", capturedPrompt)
	assert.Contains(t, capturedPrompt, "review this code",
		"original prompt text must appear after persona body")
}
