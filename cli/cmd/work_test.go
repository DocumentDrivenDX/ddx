package cmd

import (
	"encoding/json"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkCommandHasPassthroughFlags verifies that ddx work exposes the
// harness/provider/model/min-power/max-power passthrough flags that operators
// use to constrain agent routing. These flags are forwarded opaquely; ddx work
// does not validate or branch on their string values.
func TestWorkCommandHasPassthroughFlags(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err, "ddx work must exist")
	require.NotNil(t, workCmd)

	for _, name := range []string{"harness", "provider", "model", "min-power", "max-power"} {
		f := workCmd.Flags().Lookup(name)
		assert.NotNil(t, f, "ddx work must have --%s passthrough flag", name)
	}
}

// TestWorkCommandHasAllExecuteLoopFlags verifies that ddx work exposes the
// full set of flags that operators need to control queue drain behavior.
// All execute-loop flags must be present so operators can use work as the
// primary queue drain surface without switching commands.
func TestWorkCommandHasAllExecuteLoopFlags(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err, "ddx work must exist")

	loopCmd, _, err := root.Find([]string{"agent", "execute-loop"})
	require.NoError(t, err, "ddx agent execute-loop must exist")

	loopFlags := map[string]bool{}
	loopCmd.Flags().VisitAll(func(f *pflag.Flag) {
		loopFlags[f.Name] = true
	})

	for name := range loopFlags {
		if workCmd.Flags().Lookup(name) == nil {
			t.Errorf("ddx work missing flag --%s from execute-loop", name)
		}
	}
}

// TestWorkPassthroughNotValidated is the primary AC test: ddx work must NOT
// call ValidateForExecuteLoopViaService. When an unknown harness is provided
// and no ready beads exist, work must succeed with no_ready_work rather than
// failing on harness validation. This proves harness/provider/model are opaque
// passthrough — ddx work does not validate or branch on their string values.
func TestWorkPassthroughNotValidated(t *testing.T) {
	env := NewTestEnvironment(t)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	// "bogus_harness_xyz_notreal" does not exist in any harness registry.
	// ValidateForExecuteLoopViaService would fail with "unknown harness: ..."
	// if called. ddx work must skip that call and proceed to the queue scan,
	// finding no ready beads and returning successfully.
	out, err := executeCommand(root, "work",
		"--json", "--once",
		"--harness=bogus_harness_xyz_notreal",
	)
	require.NoError(t, err,
		"ddx work must not fail on unknown harness when there are no ready beads; "+
			"harness is opaque passthrough, not validated by ddx work")

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork,
		"ddx work with no ready beads must report no_ready_work=true")
}
