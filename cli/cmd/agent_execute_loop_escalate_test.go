package cmd

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// TestAgentExecuteLoopEscalateFlagRecognized covers ddx-755f5881 AC #5:
// the --escalate flag remains a registered flag so the tier-ladder path
// stays reachable. Previously escalation was implicit-on whenever no
// harness/model was pinned; ddx-755f5881 made it opt-in.
func TestAgentExecuteLoopEscalateFlagRecognized(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	dir := agentTestDir(t)
	rootCmd := NewCommandFactory(dir).NewRootCommand()

	// --once short-circuits when no ready beads exist; --local avoids the
	// server submission path. Pass --escalate to exercise the registration.
	_, err := executeCommand(rootCmd, "agent", "execute-loop",
		"--escalate", "--once", "--local",
	)
	if err != nil {
		// The "unknown flag" error would mean the flag isn't registered;
		// any other error (e.g. routing-related) is acceptable.
		if errMsg := err.Error(); contains(errMsg, "unknown flag") {
			t.Fatalf("--escalate must be a registered flag, got: %v", err)
		}
	}
}

// TestAgentExecuteLoopDefaultPathDoesNotEscalate verifies that a default
// `ddx agent execute-loop --once --local` invocation (no --escalate) does
// not invoke the tier-ladder helpers. With no ready beads the loop exits
// before the executor is called, but ResolveProfileLadder/ResolveTierModelRef
// must still be untouched on this path.
func TestAgentExecuteLoopDefaultPathDoesNotEscalate(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	dir := agentTestDir(t)
	agent.ResetRoutingCallCounters()

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	_, _ = executeCommand(rootCmd, "agent", "execute-loop", "--once", "--local")

	if got := agent.ResolveProfileLadderCallCount(); got != 0 {
		t.Errorf("ResolveProfileLadder must not be called on default execute-loop path, got %d", got)
	}
	if got := agent.ResolveTierModelRefCallCount(); got != 0 {
		t.Errorf("ResolveTierModelRef must not be called on default execute-loop path, got %d", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
