package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBeadTestRoot(t *testing.T, workingDir string) *CommandFactory {
	t.Helper()
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_BEAD_DIR", "")
	return NewCommandFactory(workingDir)
}

func TestBeadCommandsCRUDLifecycle(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Fix auth bug", "--type", "bug", "--priority", "1", "--labels", "backend,urgent", "--acceptance", "bug is fixed")
	require.NoError(t, err)

	createdID := strings.TrimSpace(createOut)
	require.NotEmpty(t, createdID)
	assert.FileExists(t, filepath.Join(workingDir, ".ddx", "beads.jsonl"))

	showOut, err := executeCommand(rootCmd, "bead", "show", createdID, "--json")
	require.NoError(t, err)

	var created map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &created))
	assert.Equal(t, createdID, created["id"])
	assert.Equal(t, "Fix auth bug", created["title"])
	assert.Equal(t, "bug", created["issue_type"])
	assert.Equal(t, "open", created["status"])
	assert.Equal(t, float64(1), created["priority"])

	_, err = executeCommand(rootCmd, "bead", "update", createdID, "--status", "in_progress", "--assignee", "me", "--labels", "backend")
	require.NoError(t, err)

	updatedOut, err := executeCommand(rootCmd, "bead", "show", createdID, "--json")
	require.NoError(t, err)

	var updated map[string]any
	require.NoError(t, json.Unmarshal([]byte(updatedOut), &updated))
	assert.Equal(t, "in_progress", updated["status"])
	assert.Equal(t, "me", updated["owner"])
	require.Len(t, updated["labels"], 1)

	listOut, err := executeCommand(rootCmd, "bead", "list", "--status", "in_progress", "--json")
	require.NoError(t, err)

	var listed []map[string]any
	require.NoError(t, json.Unmarshal([]byte(listOut), &listed))
	require.Len(t, listed, 1)
	assert.Equal(t, createdID, listed[0]["id"])

	_, err = executeCommand(rootCmd, "bead", "close", createdID)
	require.NoError(t, err)

	statusOut, err := executeCommand(rootCmd, "bead", "status", "--json")
	require.NoError(t, err)

	var status map[string]any
	require.NoError(t, json.Unmarshal([]byte(statusOut), &status))
	assert.Equal(t, float64(1), status["total"])
	assert.Equal(t, float64(1), status["closed"])
	assert.Equal(t, float64(0), status["open"])
}

func TestBeadCommandsClaimUsesExplicitAssignee(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Claim me", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)

	_, err = executeCommand(rootCmd, "bead", "update", id, "--claim", "--assignee", "alice")
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, "in_progress", bead["status"])
	assert.Equal(t, "alice", bead["owner"])
	assert.NotEmpty(t, bead["claimed-at"])
	assert.NotEmpty(t, bead["claimed-pid"])
}

func TestBeadCommandsClaimFallsBackToCallerIdentity(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	t.Setenv("USER", "runtime-agent")

	createOut, err := executeCommand(rootCmd, "bead", "create", "Claim me too", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)

	_, err = executeCommand(rootCmd, "bead", "update", id, "--claim")
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, "runtime-agent", bead["owner"])
}

func TestBeadCommandsEvidenceAppendAndList(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Evidence bead", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)

	_, err = executeCommand(rootCmd, "bead", "evidence", "add", id, "--kind", "summary", "--summary", "finished", "--body", "details", "--actor", "alice")
	require.NoError(t, err)

	listOut, err := executeCommand(rootCmd, "bead", "evidence", "list", id, "--json")
	require.NoError(t, err)

	var events []map[string]any
	require.NoError(t, json.Unmarshal([]byte(listOut), &events))
	require.Len(t, events, 1)
	assert.Equal(t, "summary", events[0]["kind"])
	assert.Equal(t, "finished", events[0]["summary"])
	assert.Equal(t, "alice", events[0]["actor"])

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	rawEvents, ok := bead["events"].([]any)
	require.True(t, ok)
	require.Len(t, rawEvents, 1)
}

func TestBeadCommandsDependencyViews(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	firstOut, err := executeCommand(rootCmd, "bead", "create", "First task", "--priority", "0")
	require.NoError(t, err)
	firstID := strings.TrimSpace(firstOut)

	secondOut, err := executeCommand(rootCmd, "bead", "create", "Second task", "--priority", "2")
	require.NoError(t, err)
	secondID := strings.TrimSpace(secondOut)

	_, err = executeCommand(rootCmd, "bead", "dep", "add", secondID, firstID)
	require.NoError(t, err)

	readyOut, err := executeCommand(rootCmd, "bead", "ready", "--json")
	require.NoError(t, err)

	var ready []map[string]any
	require.NoError(t, json.Unmarshal([]byte(readyOut), &ready))
	require.Len(t, ready, 1)
	assert.Equal(t, firstID, ready[0]["id"])

	blockedOut, err := executeCommand(rootCmd, "bead", "blocked", "--json")
	require.NoError(t, err)

	var blocked []map[string]any
	require.NoError(t, json.Unmarshal([]byte(blockedOut), &blocked))
	require.Len(t, blocked, 1)
	assert.Equal(t, secondID, blocked[0]["id"])

	treeOut, err := executeCommand(rootCmd, "bead", "dep", "tree")
	require.NoError(t, err)
	assert.Contains(t, treeOut, firstID)
	assert.Contains(t, treeOut, secondID)
	assert.Contains(t, treeOut, "First task")
	assert.Contains(t, treeOut, "Second task")

	_, err = executeCommand(rootCmd, "bead", "close", firstID)
	require.NoError(t, err)

	readyAfterCloseOut, err := executeCommand(rootCmd, "bead", "ready", "--json")
	require.NoError(t, err)

	var readyAfterClose []map[string]any
	require.NoError(t, json.Unmarshal([]byte(readyAfterCloseOut), &readyAfterClose))
	require.Len(t, readyAfterClose, 1)
	assert.Equal(t, secondID, readyAfterClose[0]["id"])

	statusOut, err := executeCommand(rootCmd, "bead", "status", "--json")
	require.NoError(t, err)

	var status map[string]any
	require.NoError(t, json.Unmarshal([]byte(statusOut), &status))
	assert.Equal(t, float64(2), status["total"])
	assert.Equal(t, float64(1), status["open"])
	assert.Equal(t, float64(1), status["closed"])
	assert.Equal(t, float64(1), status["ready"])
	assert.Equal(t, float64(0), status["blocked"])
}
