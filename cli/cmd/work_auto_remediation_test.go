package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkAutoRemediationFlags covers AC #2 (flags): ddx work exposes the
// three idle-path auto-remediation override flags, each defaulting to false.
func TestWorkAutoRemediationFlags(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()
	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err)

	for _, name := range []string{
		"no-auto-supersede-close",
		"no-auto-epic-decompose",
		"no-auto-closure-reclassify",
	} {
		flag := workCmd.Flags().Lookup(name)
		require.NotNil(t, flag, "ddx work must expose --%s", name)
		assert.Equal(t, "false", flag.DefValue, "--%s must default to false (override off)", name)
	}
}
