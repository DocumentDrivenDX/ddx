package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// TestRunLMStudioDispatchesThroughEmbeddedAgent is the regression for
// ddx-501e87ef. An lmstudio candidate has Binary="" and
// Surface="embedded-openai" because lmstudio is an HTTP-only provider —
// there's no CLI binary to exec. The dispatch must recognize this and
// route through RunAgent (the embedded OpenAI-compatible runtime),
// NOT fall through to the exec-a-binary path which would call
// r.Executor.ExecuteInDir(ctx, "", args, ...) and produce a zero-duration
// "exec: no command" error.
//
// Symptom in production (host 'eitri', 2026-04-17): routing picked
// lmstudio correctly; exec dispatch returned status=execution_failed,
// detail="exec: no command", duration_ms=0, exit_code=-1. Cheap powerClass
// was burning <1s per bead and escalating straight to smart powerClass.
func TestRunLMStudioDispatchesThroughEmbeddedAgent(t *testing.T) {
	// Isolate from the user's fizeau global config (~/.config/fizeau/config.yaml)
	// so runAgentViaService does not make real HTTP connections to configured
	// providers (which would block until TCP timeout).
	homeDir, err := os.MkdirTemp("", "ddx-home-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(homeDir) })
	t.Setenv("HOME", homeDir)
	mock := &mockExecutor{output: "should not be called"}
	r := newTestRunner(mock)

	// Invoke with explicit --harness lmstudio. Without any provider config,
	// RunAgent will fail to resolve a provider — that's fine; the check
	// is that the exec path is NEVER reached.
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{Harness: "lmstudio"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = runnerRunWithConfig(r, ctx, rcfg, AgentRunRuntime{Prompt: "hello", WorkDir: t.TempDir()})

	// The mockExecutor's ExecuteInDir records the last binary it was
	// asked to run. If the dispatch fix is working, it was never called
	// and lastBinary stays empty.
	if mock.lastBinary != "" {
		t.Errorf("lmstudio dispatch leaked to exec path: got lastBinary=%q (want empty — dispatch should route through RunAgent)", mock.lastBinary)
	}
}

// Same check for openrouter — same root cause, same fix.
func TestRunOpenRouterDispatchesThroughEmbeddedAgent(t *testing.T) {
	homeDir, err := os.MkdirTemp("", "ddx-home-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(homeDir) })
	t.Setenv("HOME", homeDir)
	mock := &mockExecutor{output: "should not be called"}
	r := newTestRunner(mock)

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{Harness: "openrouter"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = runnerRunWithConfig(r, ctx, rcfg, AgentRunRuntime{Prompt: "hello", WorkDir: t.TempDir()})

	if mock.lastBinary != "" {
		t.Errorf("openrouter dispatch leaked to exec path: got lastBinary=%q (want empty)", mock.lastBinary)
	}
}
