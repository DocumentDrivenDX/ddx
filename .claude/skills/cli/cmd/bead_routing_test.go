package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeadRoutingNoEvents(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	id, err := executeCommand(rootCmd, "bead", "create", "test bead")
	require.NoError(t, err)
	id = strings.TrimSpace(id)

	out, err := executeCommand(rootCmd, "bead", "routing", id)
	require.NoError(t, err)
	assert.Contains(t, out, "routing decisions: 0")
}

func TestBeadRoutingTextOutput(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	id, err := executeCommand(rootCmd, "bead", "create", "test bead")
	require.NoError(t, err)
	id = strings.TrimSpace(id)

	// Add routing events via evidence add.
	_, err = executeCommand(rootCmd, "bead", "evidence", "add", id,
		"--kind", "routing",
		"--summary", "provider=claude model=opus reason=config",
		"--body", `{"resolved_provider":"claude","resolved_model":"claude-opus-4-6","route_reason":"config","fallback_chain":[]}`,
	)
	require.NoError(t, err)

	_, err = executeCommand(rootCmd, "bead", "evidence", "add", id,
		"--kind", "routing",
		"--summary", "provider=claude model=sonnet reason=fallback",
		"--body", `{"resolved_provider":"claude","resolved_model":"claude-sonnet-4-6","route_reason":"fallback","fallback_chain":[]}`,
	)
	require.NoError(t, err)

	// Add a non-routing event that should be excluded.
	_, err = executeCommand(rootCmd, "bead", "evidence", "add", id,
		"--kind", "summary",
		"--summary", "task completed",
	)
	require.NoError(t, err)

	out, err := executeCommand(rootCmd, "bead", "routing", id)
	require.NoError(t, err)

	assert.Contains(t, out, "routing decisions: 2")
	assert.Contains(t, out, "provider=claude model=opus reason=config")
	assert.Contains(t, out, "provider=claude model=sonnet reason=fallback")
	// Summary section.
	assert.Contains(t, out, "providers:")
	assert.Contains(t, out, "models:")
	assert.Contains(t, out, "reasons:")
	// Non-routing event must NOT appear.
	assert.NotContains(t, out, "task completed")
}

func TestBeadRoutingJSONOutput(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	id, err := executeCommand(rootCmd, "bead", "create", "test bead")
	require.NoError(t, err)
	id = strings.TrimSpace(id)

	_, err = executeCommand(rootCmd, "bead", "evidence", "add", id,
		"--kind", "routing",
		"--summary", "provider=claude model=opus reason=config",
		"--body", `{"resolved_provider":"claude","resolved_model":"claude-opus-4-6","route_reason":"config","fallback_chain":[]}`,
	)
	require.NoError(t, err)

	out, err := executeCommand(rootCmd, "bead", "routing", id, "--json")
	require.NoError(t, err)

	var events []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &events))
	require.Len(t, events, 1)
	assert.Equal(t, "claude", events[0]["resolved_provider"])
	assert.Equal(t, "claude-opus-4-6", events[0]["resolved_model"])
	assert.Equal(t, "config", events[0]["route_reason"])
}

func TestBeadRoutingLastFlag(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	id, err := executeCommand(rootCmd, "bead", "create", "test bead")
	require.NoError(t, err)
	id = strings.TrimSpace(id)

	for _, model := range []string{"opus", "sonnet", "haiku"} {
		_, err = executeCommand(rootCmd, "bead", "evidence", "add", id,
			"--kind", "routing",
			"--summary", "provider=claude model="+model+" reason=config",
		)
		require.NoError(t, err)
	}

	out, err := executeCommand(rootCmd, "bead", "routing", id, "--last", "2", "--json")
	require.NoError(t, err)

	var events []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &events))
	require.Len(t, events, 2)
	// The last 2 should be sonnet and haiku (insertion order preserved).
	assert.Contains(t, events[0]["summary"], "sonnet")
	assert.Contains(t, events[1]["summary"], "haiku")
}
