package cmd

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestWorkCommandFlags(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	// "ddx work" must exist and expose the same flags as "ddx agent execute-loop"
	workCmd, _, err := root.Find([]string{"work"})
	if err != nil {
		t.Fatalf("ddx work not found: %v", err)
	}

	loopCmd, _, err := root.Find([]string{"agent", "execute-loop"})
	if err != nil {
		t.Fatalf("ddx agent execute-loop not found: %v", err)
	}

	// Collect local flag names from each command
	workFlags := map[string]bool{}
	workCmd.Flags().VisitAll(func(f *pflag.Flag) {
		workFlags[f.Name] = true
	})

	loopFlags := map[string]bool{}
	loopCmd.Flags().VisitAll(func(f *pflag.Flag) {
		loopFlags[f.Name] = true
	})

	// Every execute-loop flag must be present on work
	for name := range loopFlags {
		if !workFlags[name] {
			t.Errorf("ddx work missing flag --%s from execute-loop", name)
		}
	}
}
