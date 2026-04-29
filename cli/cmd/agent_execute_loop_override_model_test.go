package cmd

import (
	"testing"
)

// TestAgentExecuteLoopOverrideModelFlagRegistered covers AC #2 of bead
// ddx-87fb72c2: --override-model is a real flag on `ddx agent execute-loop`.
// The flag gates whether agent.routing.model_overrides is consulted; default
// is off.
func TestAgentExecuteLoopOverrideModelFlagRegistered(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	dir := agentTestDir(t)
	rootCmd := NewCommandFactory(dir).NewRootCommand()

	_, err := executeCommand(rootCmd, "agent", "execute-loop",
		"--override-model", "--once", "--local",
	)
	if err != nil {
		if errMsg := err.Error(); contains(errMsg, "unknown flag") {
			t.Fatalf("--override-model must be a registered flag, got: %v", err)
		}
	}
}
