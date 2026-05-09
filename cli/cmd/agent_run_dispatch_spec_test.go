package cmd

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/pflag"
)

func TestAgentRunCLISpecParity(t *testing.T) {
	root := NewCommandFactory(t.TempDir()).NewRootCommand()
	runCmd, _, err := root.Find([]string{"agent", "run"})
	if err != nil {
		t.Fatalf("find agent run command: %v", err)
	}
	if runCmd == nil {
		t.Fatal("agent run command not found")
	}

	categories := agent.AgentRunDispatchCLIFlagCategories()
	seen := map[string]bool{}
	runCmd.Flags().VisitAll(func(flag *pflag.Flag) {
		seen[flag.Name] = true
		if _, ok := categories[flag.Name]; !ok {
			t.Errorf("agent run flag %q is missing from dispatch spec parity categories", flag.Name)
		}
	})
	for name := range categories {
		if !seen[name] {
			t.Errorf("dispatch spec parity category %q does not match an agent run flag", name)
		}
	}
}
