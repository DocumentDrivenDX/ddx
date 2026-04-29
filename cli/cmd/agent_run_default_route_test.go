package cmd

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// TestAgentRunDefaultPathCallsResolveRouteOnceWithDefaultProfile pins
// ddx-755f5881 AC #2 + AC #3:
//
//   - The default `ddx agent run` path (zero flags) issues exactly one
//     ResolveRoute call with Profile: "default" and no other fields set.
//   - ResolveProfileLadder and ResolveTierModelRef are NOT in the call graph
//     on the default path.
//
// We exercise the real CLI surface — the agent harness is always available
// (embedded), so routing succeeds, and the recorder + counter test seams
// observe the call shape without us mocking the routing service.
func TestAgentRunDefaultPathCallsResolveRouteOnceWithDefaultProfile(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_VIRTUAL_RESPONSES", `[{"prompt_match":"hello","response":"hi"}]`)

	dir := agentTestDir(t)

	ResetRecordedRouteRequestsForTest()
	agent.ResetRoutingCallCounters()

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	// Zero routing flags. --text/--timeout are required to not hang or hit
	// an external network. The run may fail (no real provider), but flag
	// parsing and routing must succeed and record exactly one call.
	_, _ = executeCommand(rootCmd, "agent", "run", "--text", "hello", "--timeout", "2s")

	reqs := RecordedRouteRequestsForTest()
	if len(reqs) != 1 {
		t.Fatalf("default `ddx agent run` must issue exactly one ResolveRoute call, got %d: %#v", len(reqs), reqs)
	}
	r := reqs[0]
	if r.Profile != "default" {
		t.Errorf("Profile: want %q, got %q", "default", r.Profile)
	}
	if r.Harness != "" {
		t.Errorf("Harness must be empty on default path, got %q", r.Harness)
	}
	if r.Model != "" {
		t.Errorf("Model must be empty on default path, got %q", r.Model)
	}
	if r.Provider != "" {
		t.Errorf("Provider must be empty on default path, got %q", r.Provider)
	}
	if r.ModelRef != "" {
		t.Errorf("ModelRef must be empty on default path, got %q", r.ModelRef)
	}

	// AC #3: tier-ladder helpers must NOT be in the default-path call graph.
	if got := agent.ResolveProfileLadderCallCount(); got != 0 {
		t.Errorf("ResolveProfileLadder must not be called on default path, got %d", got)
	}
	if got := agent.ResolveTierModelRefCallCount(); got != 0 {
		t.Errorf("ResolveTierModelRef must not be called on default path, got %d", got)
	}
}
