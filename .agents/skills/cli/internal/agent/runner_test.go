package agent

import "testing"

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
// detail="exec: no command", duration_ms=0, exit_code=-1. Cheap tier
// was burning <1s per bead and escalating straight to smart tier.
func TestRunLMStudioDispatchesThroughEmbeddedAgent(t *testing.T) {
	mock := &mockExecutor{output: "should not be called"}
	r := newTestRunner(mock)

	// Invoke with explicit --harness lmstudio. Without AgentProvider set,
	// RunAgent will fail to resolve a provider — that's fine; the check
	// is that the exec path is NEVER reached.
	_, _ = r.Run(RunOptions{Harness: "lmstudio", Prompt: "hello"})

	// The mockExecutor's ExecuteInDir records the last binary it was
	// asked to run. If the dispatch fix is working, it was never called
	// and lastBinary stays empty.
	if mock.lastBinary != "" {
		t.Errorf("lmstudio dispatch leaked to exec path: got lastBinary=%q (want empty — dispatch should route through RunAgent)", mock.lastBinary)
	}
}

// Same check for openrouter — same root cause, same fix.
func TestRunOpenRouterDispatchesThroughEmbeddedAgent(t *testing.T) {
	mock := &mockExecutor{output: "should not be called"}
	r := newTestRunner(mock)

	_, _ = r.Run(RunOptions{Harness: "openrouter", Prompt: "hello"})

	if mock.lastBinary != "" {
		t.Errorf("openrouter dispatch leaked to exec path: got lastBinary=%q (want empty)", mock.lastBinary)
	}
}
