package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
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

func TestBeadCommandsCloseRecordsLandedCommit(t *testing.T) {
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

	stateFile := filepath.Join(env.Dir, "fake-git-state")
	countFile := filepath.Join(env.Dir, "fake-git-count")
	closeCommitSHA := "3333333333333333333333333333333333333333"
	provenanceCommitSHA := "4444444444444444444444444444444444444444"
	require.NoError(t, os.WriteFile(stateFile, []byte("1111111111111111111111111111111111111111"), 0o644))
	require.NoError(t, os.WriteFile(countFile, []byte("0"), 0o644))

	binDir := filepath.Join(env.Dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	fakeGit := filepath.Join(binDir, "git")
	require.NoError(t, os.WriteFile(fakeGit, []byte(`#!/bin/sh
set -eu
state_file="$DDX_FAKE_GIT_STATE"
count_file="$DDX_FAKE_GIT_COUNT"
head_one="$DDX_FAKE_GIT_SHA_1"
head_two="$DDX_FAKE_GIT_SHA_2"
head_three="$DDX_FAKE_GIT_SHA_3"

case "$1" in
  rev-parse)
    if [ "${2:-}" = "--git-dir" ]; then
      exit 0
    fi
    if [ "${2:-}" = "HEAD" ]; then
      cat "$state_file"
      exit 0
    fi
    exit 0
    ;;
  add)
    exit 0
    ;;
  commit)
    count="$(cat "$count_file")"
    count=$((count + 1))
    printf '%s' "$count" > "$count_file"
    if [ "$count" -eq 1 ]; then
      printf '%s' "$head_one" > "$state_file"
    elif [ "$count" -eq 2 ]; then
      printf '%s' "$head_two" > "$state_file"
    elif [ "$count" -eq 3 ]; then
      printf '%s' "$head_three" > "$state_file"
    fi
    exit 0
    ;;
  status)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DDX_FAKE_GIT_STATE", stateFile)
	t.Setenv("DDX_FAKE_GIT_COUNT", countFile)
	t.Setenv("DDX_FAKE_GIT_SHA_1", "2222222222222222222222222222222222222222")
	t.Setenv("DDX_FAKE_GIT_SHA_2", closeCommitSHA)
	t.Setenv("DDX_FAKE_GIT_SHA_3", provenanceCommitSHA)

	factory := newBeadTestRoot(t, env.Dir)
	rootCmd := factory.NewRootCommand()

	createOut, err := executeCommand(rootCmd, "bead", "create", "Close provenance", "--type", "task")
	require.NoError(t, err)
	id := strings.TrimSpace(createOut)
	require.NotEmpty(t, id)

	headBeforeClose := gitHead(t, env.Dir)
	require.NotEmpty(t, headBeforeClose)

	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err)

	headAfterClose := gitHead(t, env.Dir)
	require.NotEmpty(t, headAfterClose)
	assert.NotEqual(t, headBeforeClose, headAfterClose)
	assert.Equal(t, "3", mustReadFile(t, countFile))

	showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
	require.NoError(t, err)

	var bead map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &bead))
	assert.Equal(t, closeCommitSHA, bead["closing_commit_sha"])
	assert.Equal(t, provenanceCommitSHA, headAfterClose)

	statusCmd := exec.Command("git", "status", "--short")
	statusCmd.Dir = env.Dir
	statusOut, err := statusCmd.CombinedOutput()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(statusOut)))
}

func TestBeadCommandsCloseLeavesCleanGitStatus(t *testing.T) {
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
