package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRemovedInstallCommandsAreUnavailable proves top-level install,
// installed, and uninstall are no longer registered on the root command.
// Plugin installation remains under `ddx plugin install`.
func TestRemovedInstallCommandsAreUnavailable(t *testing.T) {
	for _, name := range []string{"install", "installed", "uninstall"} {
		t.Run(name, func(t *testing.T) {
			factory := NewCommandFactory(t.TempDir())
			root := factory.NewRootCommand()

			// Root must not list the removed command.
			for _, c := range root.Commands() {
				assert.NotEqual(t, name, c.Name(), "root must not register %q", name)
			}

			output, err := executeCommand(root, name)
			require.Error(t, err, "ddx %s must fail as an unknown command", name)
			combined := strings.ToLower(output + " " + err.Error())
			assert.Contains(t, combined, "unknown command",
				"ddx %s must surface an unknown-command error; got output=%q err=%v", name, output, err)
		})
	}

	t.Run("help_does_not_list_removed", func(t *testing.T) {
		factory := NewCommandFactory(t.TempDir())
		output, err := executeCommand(factory.NewRootCommand(), "help")
		require.NoError(t, err, output)

		// Match root help lines only (word boundary on the left as cobra prints).
		for _, line := range strings.Split(output, "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			cmdName := fields[0]
			assert.NotEqual(t, "install", cmdName)
			assert.NotEqual(t, "installed", cmdName)
			assert.NotEqual(t, "uninstall", cmdName)
		}
	})
}
