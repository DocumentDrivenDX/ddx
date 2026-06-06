package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeadCreateRejectsDependsOnParentAncestor(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	rootOut, err := executeCommand(rootCmd, "bead", "create", "Root", "--type", "task")
	require.NoError(t, err)
	rootID := strings.TrimSpace(rootOut)
	require.NotEmpty(t, rootID)

	parentOut, err := executeCommand(rootCmd, "bead", "create", "Parent", "--type", "task", "--parent", rootID)
	require.NoError(t, err)
	parentID := strings.TrimSpace(parentOut)
	require.NotEmpty(t, parentID)

	t.Run("direct-parent", func(t *testing.T) {
		_, err := executeCommand(rootCmd, "bead", "create", "Child direct", "--type", "task", "--parent", parentID, "--depends-on", parentID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ancestor in the parent chain")
		assert.Contains(t, err.Error(), parentID)
		assert.Contains(t, err.Error(), parentID+" -> "+rootID)
	})

	t.Run("grandparent-ancestor", func(t *testing.T) {
		_, err := executeCommand(rootCmd, "bead", "create", "Child ancestor", "--type", "task", "--parent", parentID, "--depends-on", rootID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ancestor in the parent chain")
		assert.Contains(t, err.Error(), rootID)
		assert.Contains(t, err.Error(), parentID+" -> "+rootID)
	})
}
