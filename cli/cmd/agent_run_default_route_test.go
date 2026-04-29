package cmd

import (
	"testing"
)

// TestAgentRunDefaultPathCallsResolveRouteOnceWithDefaultProfile pins
// ddx-755f5881 AC #2: the default `ddx agent run` path (zero flags) issues
// exactly one ResolveRoute call with Profile: "default" and no other fields set.
func TestAgentRunDefaultPathCallsResolveRouteOnceWithDefaultProfile(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_VIRTUAL_RESPONSES", `[{"prompt_match":"hello","response":"hi"}]`)

	dir := agentTestDir(t)

	ResetRecordedRouteRequestsForTest()

	rootCmd := NewCommandFactory(dir).NewRootCommand()
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
}
