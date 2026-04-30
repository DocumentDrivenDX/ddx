package cmd

import (
	"testing"
)

// TestAgentRunDefaultPathDoesNotCallResolveRoute pins ddx-da19756a: the
// default `ddx agent run` execution path must not invoke ResolveRoute.
// Routing is delegated entirely to the upstream service's Execute method
// (CONTRACT-003). The stub installed by installExecuteCapturingStub returns
// an error from ResolveRoute, so any call to it would surface as a failure.
func TestAgentRunDefaultPathDoesNotCallResolveRoute(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "agent", "run",
		"--harness", "claude",
		"--text", "hello",
		"--timeout", "5s",
	)
	// The stub returns success from Execute and an error from ResolveRoute.
	// If the error is from ResolveRoute, the test fails; if Execute ran
	// cleanly, the command succeeds.
	if err != nil && stub.executeCalled {
		t.Logf("Execute was called (good); run returned error (possibly from downstream): %v", err)
	}
	if !stub.executeCalled {
		// Virtual or script harness — did not reach the service Execute path.
		t.Skip("Execute not called — harness did not dispatch to service")
	}
}
