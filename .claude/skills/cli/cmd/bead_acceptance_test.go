package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
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

func TestBeadCommandsUnsetCustomField(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Replay provenance", "--type", "task", "--set", "closing_commit_sha=9653820049db7edebe0374431544b1b8a8dbae88")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, "9653820049db7edebe0374431544b1b8a8dbae88", bead["closing_commit_sha"])

	_, err = executeCommand(rootCmd, "bead", "update", id, "--unset", "closing_commit_sha")
	require.NoError(t, err)

	updatedOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var updated map[string]any
	require.NoError(t, json.Unmarshal([]byte(updatedOut), &updated))
	_, ok := updated["closing_commit_sha"]
	assert.False(t, ok)
}

func TestBeadCommandsCanNormalizeClosingCommitShaOnClosedBead(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Replay provenance", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	fullSHA := gitHead(t, env.Dir, "HEAD")
	shortSHA := gitShortHead(t, env.Dir)

	_, err = executeCommand(rootCmd, "bead", "update", id, "--set", "closing_commit_sha="+shortSHA)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, "closed", bead["status"])
	assert.Equal(t, fullSHA, bead["closing_commit_sha"])
}

func TestBeadCommandsRejectInvalidClosingCommitShaRepair(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Replay provenance", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	_, err = executeCommand(rootCmd, "bead", "update", id, "--set", "closing_commit_sha=not-a-sha")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid closing_commit_sha")
}

func TestBeadCommandsUnsetRejectsProtectedEvidenceFields(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Evidence protection", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)

	_, err = executeCommand(rootCmd, "bead", "evidence", "add", id, "--kind", "summary", "--summary", "finished", "--body", "details", "--actor", "alice")
	require.NoError(t, err)

	_, err = executeCommand(rootCmd, "bead", "update", id, "--unset", "events")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot unset protected bead field")

	listOut, err := executeCommand(rootCmd, "bead", "evidence", "list", id, "--json")
	require.NoError(t, err)

	var events []map[string]any
	require.NoError(t, json.Unmarshal([]byte(listOut), &events))
	require.Len(t, events, 1)
	assert.Equal(t, "summary", events[0]["kind"])
	assert.Equal(t, "finished", events[0]["summary"])
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

func TestBeadCommandsCloseOmitsTrackerOnlyClosingCommitSha(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Close provenance", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)
	require.NotEmpty(t, id)

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	_, ok := bead["closing_commit_sha"]
	assert.False(t, ok)

	statusCmd := exec.Command("git", "status", "--short")
	statusCmd.Dir = env.Dir
	statusOut, err := statusCmd.CombinedOutput()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(statusOut)))
}

func TestBeadCommandsCloseOmitsMetadataOnlyReviewBeadClosingCommitSha(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Review issue", "--type", "task", "--labels", "helix,kind:planning,action:review")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)
	require.NotEmpty(t, id)

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	_, ok := bead["closing_commit_sha"]
	assert.False(t, ok)

	assert.Empty(t, gitStatusShort(t, env.Dir))
}

func TestBeadCommandsClosePreservesExistingClosingCommitShaOnMetadataOnlyReviewCloseWhenValid(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: never
  commit_prefix: beads
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Review issue", "--type", "task", "--labels", "helix,kind:planning,action:review")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)
	require.NotEmpty(t, id)

	seedSHA := gitHead(t, env.Dir, "HEAD")
	_, err = executeCommand(rootCmd, "bead", "update", id, "--set", "closing_commit_sha="+seedSHA)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var before map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &before))
	assert.Equal(t, seedSHA, before["closing_commit_sha"])

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	showOut, err = executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, seedSHA, bead["closing_commit_sha"])
	assert.Equal(t, "closed", bead["status"])
}

func TestBeadCommandsCloseClearsMetadataOnlyTrackerOnlyReviewBeadClosingCommitSha(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	sourceOut, err := executeCommand(rootCmd, "bead", "create", "Tracker issue", "--type", "task", "--labels", "helix,kind:planning,action:review")
	require.NoError(t, err)
	sourceID := strings.TrimSpace(sourceOut)
	require.NotEmpty(t, sourceID)

	_, err = executeCommand(rootCmd, "bead", "close", sourceID)
	require.NoError(t, err)

	trackerOnlySHA := gitHead(t, env.Dir, "HEAD")
	require.NotEmpty(t, trackerOnlySHA)

	staleOut, err := executeCommand(rootCmd, "bead", "create", "Stale tracker provenance", "--type", "task", "--labels", "helix,kind:planning,action:review")
	require.NoError(t, err)
	staleID := strings.TrimSpace(staleOut)
	require.NotEmpty(t, staleID)

	_, err = executeCommand(rootCmd, "bead", "update", staleID, "--set", "closing_commit_sha="+trackerOnlySHA)
	require.NoError(t, err)

	_, err = executeCommand(rootCmd, "bead", "close", staleID)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", staleID, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, "closed", bead["status"])
	_, ok := bead["closing_commit_sha"]
	assert.False(t, ok)
}

func TestBeadCommandsCloseOmitsMetadataOnlyReviewFindingClosingCommitSha(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()
	seedSHA := gitHead(t, env.Dir, "HEAD")

	sourceOut, err := executeCommand(rootCmd, "bead", "create", "Tracker issue", "--type", "task", "--labels", "helix,kind:planning,action:review")
	require.NoError(t, err)
	sourceID := strings.TrimSpace(sourceOut)
	require.NotEmpty(t, sourceID)

	_, err = executeCommand(rootCmd, "bead", "close", sourceID)
	require.NoError(t, err)

	findingOut, err := executeCommand(rootCmd, "bead", "create", "Tracker-only review finding", "--type", "task", "--labels", "helix,phase:build,review-finding")
	require.NoError(t, err)
	findingID := strings.TrimSpace(findingOut)
	require.NotEmpty(t, findingID)

	_, err = executeCommand(rootCmd, "bead", "update", findingID, "--set", "closing_commit_sha="+seedSHA)
	require.NoError(t, err)

	_, err = executeCommand(rootCmd, "bead", "close", findingID)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", findingID, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, "closed", bead["status"])
	_, ok := bead["closing_commit_sha"]
	assert.False(t, ok)
	assert.NotEmpty(t, seedSHA)
}

func TestBeadCommandsCloseRecordsClosingCommitForReviewBeadWithExplicitCommit(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Review issue", "--type", "task", "--labels", "helix,kind:planning,action:review")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)
	require.NotEmpty(t, id)

	fullSHA := gitHead(t, env.Dir, "HEAD")
	shortHead := gitShortHead(t, env.Dir)

	_, err = executeCommand(rootCmd, "bead", "close", id, "--commit", shortHead)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, fullSHA, bead["closing_commit_sha"])
	assert.Empty(t, gitStatusShort(t, env.Dir))
}

func TestBeadCommandsCloseRecordsClosingCommitForReviewBeadWithMixedClose(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Review issue", "--type", "task", "--labels", "helix,kind:planning,action:review")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)
	require.NotEmpty(t, id)

	stagePath := filepath.Join(env.Dir, "review-note.txt")
	require.NoError(t, os.WriteFile(stagePath, []byte("review close with implementation work\n"), 0o644))
	stageCmd := exec.Command("git", "add", "review-note.txt")
	stageCmd.Dir = env.Dir
	require.NoError(t, stageCmd.Run())

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.NotEmpty(t, bead["closing_commit_sha"])
	assert.Equal(t, gitHead(t, env.Dir, "HEAD^"), bead["closing_commit_sha"])
	assert.Empty(t, gitStatusShort(t, env.Dir))
}

func TestBeadCommandsPreserveUnrelatedClosingCommitShaAcrossMutations(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	seedSHA := gitHead(t, env.Dir, "HEAD")

	preservedOut, err := executeCommand(rootCmd, "bead", "create", "Preserved provenance", "--type", "task")
	require.NoError(t, err)
	preservedID := strings.TrimSpace(preservedOut)
	require.NotEmpty(t, preservedID)

	_, err = executeCommand(rootCmd, "bead", "close", preservedID)
	require.NoError(t, err)

	_, err = executeCommand(rootCmd, "bead", "update", preservedID, "--set", "closing_commit_sha="+seedSHA)
	require.NoError(t, err)

	verifyPreserved := func(t *testing.T) {
		t.Helper()
		showOut, err := executeCommand(rootCmd, "bead", "show", preservedID, "--json")
		require.NoError(t, err)

		var bead map[string]any
		require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
		assert.Equal(t, "closed", bead["status"])
		assert.Equal(t, seedSHA, bead["closing_commit_sha"])
	}

	verifyPreserved(t)

	updatedOut, err := executeCommand(rootCmd, "bead", "create", "Update target", "--type", "task")
	require.NoError(t, err)
	updatedID := strings.TrimSpace(updatedOut)
	require.NotEmpty(t, updatedID)

	_, err = executeCommand(rootCmd, "bead", "update", updatedID, "--status", "in_progress")
	require.NoError(t, err)

	verifyPreserved(t)

	closedOut, err := executeCommand(rootCmd, "bead", "create", "Close target", "--type", "task")
	require.NoError(t, err)
	closedID := strings.TrimSpace(closedOut)
	require.NotEmpty(t, closedID)

	_, err = executeCommand(rootCmd, "bead", "close", closedID)
	require.NoError(t, err)

	verifyPreserved(t)
}

func TestBeadCommandsCloseNormalizesProvidedCommitSHA(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Normalize commit", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)
	require.NotEmpty(t, id)

	fullSHA := gitHead(t, env.Dir, "HEAD")
	shortSHA := gitShortHead(t, env.Dir)

	_, err = executeCommand(rootCmd, "bead", "close", id, "--commit", shortSHA)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, fullSHA, bead["closing_commit_sha"])
}

func TestBeadCommandsCanUnsetClosingCommitShaOnClosedBead(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Unset provenance", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)
	require.NotEmpty(t, id)

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	_, err = executeCommand(rootCmd, "bead", "update", id, "--unset", "closing_commit_sha")
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, "closed", bead["status"])
	_, ok := bead["closing_commit_sha"]
	assert.False(t, ok)

	assert.Empty(t, gitStatusShort(t, env.Dir))
}

func TestBeadCommandsClosePreservesFullCommitSHAWithoutGitRepo(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Manual provenance", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)
	require.NotEmpty(t, id)

	const fullSHA = "1234567890abcdef1234567890abcdef12345678"

	_, err = executeCommand(rootCmd, "bead", "close", id, "--commit", fullSHA)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, "closed", bead["status"])
	assert.Equal(t, fullSHA, bead["closing_commit_sha"])
}

func TestBeadCommandsClosePreservesExistingClosingCommitShaWithoutGitRepo(t *testing.T) {
	env := NewTestEnvironment(t, WithGitInit(false))
	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Manual review provenance", "--type", "task", "--labels", "helix,kind:planning,action:review")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)
	require.NotEmpty(t, id)

	const fullSHA = "1234567890abcdef1234567890abcdef12345678"

	_, err = executeCommand(rootCmd, "bead", "update", id, "--set", "closing_commit_sha="+fullSHA)
	require.NoError(t, err)

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, "closed", bead["status"])
	assert.Equal(t, fullSHA, bead["closing_commit_sha"])
}

func TestBeadCommandsCloseRecordsClosingCommitForTrackedDdxWork(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)

	notesPath := filepath.Join(env.Dir, ".ddx", "notes.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(notesPath), 0o755))
	require.NoError(t, os.WriteFile(notesPath, []byte("tracked ddx notes\n"), 0o644))
	gitAddAndCommit(t, env.Dir, "track ddx config and notes", ".ddx/config.yaml", ".ddx/notes.txt")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Close provenance", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)
	require.NotEmpty(t, id)

	require.NoError(t, os.WriteFile(notesPath, []byte("tracked ddx notes updated\n"), 0o644))
	stageCmd := exec.Command("git", "add", ".ddx/notes.txt")
	stageCmd.Dir = env.Dir
	require.NoError(t, stageCmd.Run())

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	statusOut := gitStatusShort(t, env.Dir)
	assert.Empty(t, statusOut)

	headContent := gitShowFile(t, env.Dir, "HEAD", ".ddx/beads.jsonl")
	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(headContent)), &bead))
	assert.Equal(t, "closed", bead["status"])
	assert.NotEmpty(t, bead["closing_commit_sha"])

	parentSHA := gitHead(t, env.Dir, "HEAD^")
	assert.Equal(t, parentSHA, bead["closing_commit_sha"])
}

func TestBeadCommandsCloseRecordsClosingCommitForMixedClose(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: beads
`)

	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Close provenance", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)
	require.NotEmpty(t, id)

	stagePath := filepath.Join(env.Dir, "mixed-close.txt")
	require.NoError(t, os.WriteFile(stagePath, []byte("mixed close\n"), 0o644))
	stageCmd := exec.Command("git", "add", "mixed-close.txt")
	stageCmd.Dir = env.Dir
	require.NoError(t, stageCmd.Run())

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	statusOut := gitStatusShort(t, env.Dir)
	assert.Empty(t, statusOut)

	headContent := gitShowFile(t, env.Dir, "HEAD", ".ddx/beads.jsonl")
	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(headContent)), &bead))
	assert.Equal(t, "closed", bead["status"])
	assert.NotEmpty(t, bead["closing_commit_sha"])

	parentSHA := gitHead(t, env.Dir, "HEAD^")
	assert.Equal(t, parentSHA, bead["closing_commit_sha"])
}

func gitHead(t *testing.T, dir string, ref ...string) string {
	t.Helper()

	target := "HEAD"
	if len(ref) > 0 && ref[0] != "" {
		target = ref[0]
	}
	cmd := exec.Command("git", "rev-parse", target)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func gitShortHead(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func gitStatusShort(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "status", "--short")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git status should succeed: %s", string(out))
	return strings.TrimSpace(string(out))
}

func gitShowFile(t *testing.T, dir, ref, path string) string {
	t.Helper()

	cmd := exec.Command("git", "show", ref+":"+path)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git show should succeed: %s", string(out))
	return string(out)
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()

	out, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func gitAddAndCommit(t *testing.T, dir, message string, paths ...string) {
	t.Helper()

	addArgs := append([]string{"add"}, paths...)
	addCmd := exec.Command("git", addArgs...)
	addCmd.Dir = dir
	require.NoError(t, addCmd.Run())

	commitCmd := exec.Command("git", "commit", "-m", message)
	commitCmd.Dir = dir
	require.NoError(t, commitCmd.Run())
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

func TestBeadBlockedSurfacesRetryParkedBeads(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	depRootOut, err := executeCommand(rootCmd, "bead", "create", "Dep root", "--priority", "1")
	require.NoError(t, err)
	depRootID := strings.TrimSpace(depRootOut)

	depBlockedOut, err := executeCommand(rootCmd, "bead", "create", "Dep blocked child", "--priority", "2")
	require.NoError(t, err)
	depBlockedID := strings.TrimSpace(depBlockedOut)

	parkedOut, err := executeCommand(rootCmd, "bead", "create", "Retry parked", "--priority", "0")
	require.NoError(t, err)
	parkedID := strings.TrimSpace(parkedOut)

	_, err = executeCommand(rootCmd, "bead", "dep", "add", depBlockedID, depRootID)
	require.NoError(t, err)

	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	until := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Second)
	require.NoError(t, store.SetExecutionCooldown(parkedID, until, "no_changes", "agent made no commits"))

	blockedJSON, err := executeCommand(rootCmd, "bead", "blocked", "--json")
	require.NoError(t, err)

	var entries []map[string]any
	require.NoError(t, json.Unmarshal([]byte(blockedJSON), &entries))
	require.Len(t, entries, 2, "dep-blocked and retry-parked beads must both surface")

	byID := map[string]map[string]any{}
	for _, e := range entries {
		id, _ := e["id"].(string)
		byID[id] = e
	}

	depEntry, ok := byID[depBlockedID]
	require.True(t, ok, "dep-blocked entry missing from JSON: %s", blockedJSON)
	require.Equal(t, "Dep blocked child", depEntry["title"])
	depBlocker, ok := depEntry["blocker"].(map[string]any)
	require.True(t, ok, "dep entry missing blocker object: %#v", depEntry)
	assert.Equal(t, "dependency", depBlocker["kind"])
	depIDs, _ := depBlocker["unclosed_dep_ids"].([]any)
	require.Len(t, depIDs, 1)
	assert.Equal(t, depRootID, depIDs[0])
	_, hasNextEligible := depBlocker["next_eligible_at"]
	assert.False(t, hasNextEligible, "dependency blocker must not emit next_eligible_at")

	parkedEntry, ok := byID[parkedID]
	require.True(t, ok, "retry-parked entry missing from JSON: %s", blockedJSON)
	require.Equal(t, "Retry parked", parkedEntry["title"])
	parkedBlocker, ok := parkedEntry["blocker"].(map[string]any)
	require.True(t, ok, "parked entry missing blocker object: %#v", parkedEntry)
	assert.Equal(t, "retry-cooldown", parkedBlocker["kind"])
	assert.Equal(t, until.Format(time.RFC3339), parkedBlocker["next_eligible_at"])
	assert.Equal(t, "no_changes", parkedBlocker["last_status"])
	assert.Equal(t, "agent made no commits", parkedBlocker["last_detail"])
	_, hasDepField := parkedBlocker["unclosed_dep_ids"]
	assert.False(t, hasDepField, "cooldown blocker must not emit unclosed_dep_ids")

	// Non-JSON output must distinguish blocker kinds without dropping the existing dep line.
	// Rebuild the root command so the --json flag state does not leak across invocations.
	textCmd := factory.NewRootCommand()
	textOut, err := executeCommand(textCmd, "bead", "blocked")
	require.NoError(t, err)
	assert.Contains(t, textOut, depBlockedID+"  P2  Dep blocked child  deps: "+depRootID,
		"dep-blocked line missing or malformed: %s", textOut)
	assert.Contains(t, textOut, parkedID+"  P0  Retry parked  retry-after: "+until.Format(time.RFC3339),
		"retry-parked line missing or malformed: %s", textOut)

	// ReadyExecution filtering must be unchanged: parked bead stays suppressed.
	execReady, err := store.ReadyExecution()
	require.NoError(t, err)
	require.Len(t, execReady, 1)
	assert.Equal(t, depRootID, execReady[0].ID,
		"parked bead must remain suppressed from ReadyExecution")
}

func TestBeadReopenSetsStatusToOpen(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Reopen me", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)
	var closed map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &closed))
	assert.Equal(t, "closed", closed["status"])

	_, err = executeCommand(rootCmd, "bead", "reopen", id)
	require.NoError(t, err)

	showOut, err = executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)
	var reopened map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &reopened))
	assert.Equal(t, "open", reopened["status"])
}

func TestBeadReopenRecordsEventAndAppendNotes(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Reopen with notes", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)

	_, err = executeCommand(rootCmd, "bead", "update", id, "--notes", "original notes")
	require.NoError(t, err)

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	_, err = executeCommand(rootCmd, "bead", "reopen", id, "--reason", "failed review", "--append-notes", "second attempt needed")
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)
	var b map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &b))

	assert.Equal(t, "open", b["status"])
	assert.Contains(t, b["notes"], "original notes")
	assert.Contains(t, b["notes"], "second attempt needed")

	// Verify reopen event was recorded
	listOut, err := executeCommand(rootCmd, "bead", "evidence", "list", id, "--json")
	require.NoError(t, err)
	var events []map[string]any
	require.NoError(t, json.Unmarshal([]byte(listOut), &events))
	require.Len(t, events, 1)
	assert.Equal(t, "reopen", events[0]["kind"])
	assert.Equal(t, "failed review", events[0]["summary"])
}

func TestBeadReopenClearsClaimFields(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Claimed bead", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)

	// Claim it first
	_, err = executeCommand(rootCmd, "bead", "update", id, "--claim", "--assignee", "agent-1")
	require.NoError(t, err)

	// Close it directly via update (simulating a forced close of an in-progress bead)
	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	_, err = executeCommand(rootCmd, "bead", "reopen", id)
	require.NoError(t, err)

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)
	var b map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &b))

	assert.Equal(t, "open", b["status"])
	assert.Empty(t, b["owner"])
	_, hasClaimed := b["claimed-at"]
	assert.False(t, hasClaimed, "claimed-at should be cleared on reopen")
	_, hasClaimedPID := b["claimed-pid"]
	assert.False(t, hasClaimedPID, "claimed-pid should be cleared on reopen")
}
