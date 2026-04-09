package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeExecuteBeadGit is a mock implementation of executeBeadGitOps for tests.
type fakeExecuteBeadGit struct {
	mu sync.Mutex

	// mainHeadRev is returned by HeadRev/ResolveRev for the main working dir.
	mainHeadRev string
	// wtHeadRev is returned by HeadRev for worktree paths (after agent run).
	wtHeadRev    string
	dirty        bool
	ffMergeErr   error
	updateRefErr error

	stashCalled bool
	addedWTs    []string
	removedWTs  []string
	refs        map[string]string // ref -> sha recorded by UpdateRef
	worktrees   []string          // paths returned by WorktreeList
}

func (f *fakeExecuteBeadGit) HeadRev(dir string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(dir, executeBeadWtPrefix) {
		return f.wtHeadRev, nil
	}
	return f.mainHeadRev, nil
}

func (f *fakeExecuteBeadGit) ResolveRev(dir, rev string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mainHeadRev, nil
}

func (f *fakeExecuteBeadGit) IsDirty(dir string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dirty, nil
}

func (f *fakeExecuteBeadGit) Stash(dir string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stashCalled = true
	return nil
}

func (f *fakeExecuteBeadGit) WorktreeAdd(dir, wtPath, rev string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addedWTs = append(f.addedWTs, wtPath)
	return nil
}

func (f *fakeExecuteBeadGit) WorktreeRemove(dir, wtPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removedWTs = append(f.removedWTs, wtPath)
	return nil
}

func (f *fakeExecuteBeadGit) WorktreeList(dir string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.worktrees, nil
}

func (f *fakeExecuteBeadGit) WorktreePrune(dir string) error { return nil }

func (f *fakeExecuteBeadGit) FFMerge(dir, rev string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ffMergeErr
}

func (f *fakeExecuteBeadGit) UpdateRef(dir, ref, sha string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateRefErr != nil {
		return f.updateRefErr
	}
	if f.refs == nil {
		f.refs = make(map[string]string)
	}
	f.refs[ref] = sha
	return nil
}

// fakeAgentRunner is a minimal mock agent runner for execute-bead tests.
type fakeAgentRunner struct {
	result *agent.Result
	err    error
}

func (r *fakeAgentRunner) Run(opts agent.RunOptions) (*agent.Result, error) {
	return r.result, r.err
}

// newExecuteBeadFactory builds a CommandFactory wired with the given fake git and agent runner.
func newExecuteBeadFactory(t *testing.T, git *fakeExecuteBeadGit, runner *fakeAgentRunner) *CommandFactory {
	t.Helper()
	f := NewCommandFactory(t.TempDir())
	f.AgentRunnerOverride = runner
	f.executeBeadGitOverride = git
	return f
}

// runExecuteBead invokes the execute-bead command through the cobra tree and returns
// the parsed JSON result. It extracts the JSON object from the combined output,
// skipping any leading note/warning lines written to stderr.
func runExecuteBead(t *testing.T, f *CommandFactory, git *fakeExecuteBeadGit, beadID string, extraArgs ...string) ExecuteBeadResult {
	t.Helper()
	root := f.NewRootCommand()
	args := append([]string{"agent", "execute-bead", beadID, "--json"}, extraArgs...)
	out, err := executeCommand(root, args...)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	// Strip any non-JSON prefix lines (e.g. stderr notes written to the shared buffer).
	jsonStart := strings.Index(out, "{")
	require.NotEqual(t, -1, jsonStart, "output should contain JSON: %s", out)
	jsonPart := out[jsonStart:]
	var res ExecuteBeadResult
	require.NoError(t, json.Unmarshal([]byte(jsonPart), &res), "output should be valid JSON: %s", jsonPart)
	return res
}

// TestExecuteBeadMerge verifies that when fast-forward merge succeeds the outcome is "merged".
func TestExecuteBeadMerge(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222", // agent made a new commit
		ffMergeErr:  nil,        // merge succeeds
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0, Harness: "mock"}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, "merged", res.Outcome)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Equal(t, "bbbb2222", res.ResultRev)
	assert.Empty(t, res.PreserveRef)
	assert.Equal(t, "my-bead", res.BeadID)
	assert.NotEmpty(t, res.SessionID)

	// Worktree should have been created and cleaned up.
	require.Len(t, git.addedWTs, 1)
	assert.Contains(t, git.addedWTs[0], executeBeadWtPrefix+"my-bead-")
	require.Len(t, git.removedWTs, 1)
	assert.Equal(t, git.addedWTs[0], git.removedWTs[0])
}

// TestExecuteBeadPreserveOnFFFailure verifies that when fast-forward merge fails
// the result is preserved under a hidden ref.
func TestExecuteBeadPreserveOnFFFailure(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "cccc3333",
		ffMergeErr:  fmt.Errorf("not possible to fast-forward"),
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, "preserved", res.Outcome)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Equal(t, "cccc3333", res.ResultRev)
	assert.NotEmpty(t, res.PreserveRef)
	assert.True(t, strings.HasPrefix(res.PreserveRef, "refs/ddx/execute-bead/my-bead/"),
		"preserve ref should be under refs/ddx/execute-bead/my-bead/, got: %s", res.PreserveRef)
	assert.Equal(t, "ff-merge not possible", res.Reason)

	// Hidden ref should be recorded in the mock.
	require.Contains(t, git.refs, res.PreserveRef)
	assert.Equal(t, "cccc3333", git.refs[res.PreserveRef])
}

// TestExecuteBeadNoMerge verifies that --no-merge bypasses fast-forward and
// always preserves under a hidden ref.
func TestExecuteBeadNoMerge(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "dddd4444",
		ffMergeErr:  nil, // merge would succeed, but --no-merge suppresses it
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead", "--no-merge")

	assert.Equal(t, "preserved", res.Outcome)
	assert.Equal(t, "--no-merge specified", res.Reason)
	assert.NotEmpty(t, res.PreserveRef)
	assert.True(t, strings.HasPrefix(res.PreserveRef, "refs/ddx/execute-bead/my-bead/"))

	// FFMerge should not have been called; refs should still be recorded.
	require.Contains(t, git.refs, res.PreserveRef)
}

// TestExecuteBeadHiddenRefUniqueness verifies that two runs on the same bead-id
// produce distinct preserve refs (concurrent hidden-ref uniqueness).
func TestExecuteBeadHiddenRefUniqueness(t *testing.T) {
	makeRun := func() ExecuteBeadResult {
		git := &fakeExecuteBeadGit{
			mainHeadRev: "aaaa1111",
			wtHeadRev:   "eeee5555",
			ffMergeErr:  fmt.Errorf("diverged"),
		}
		runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
		f := newExecuteBeadFactory(t, git, runner)
		return runExecuteBead(t, f, git, "shared-bead")
	}

	res1 := makeRun()
	res2 := makeRun()

	assert.NotEqual(t, res1.PreserveRef, res2.PreserveRef,
		"concurrent runs must produce distinct preserve refs")
	assert.True(t, strings.HasPrefix(res1.PreserveRef, "refs/ddx/execute-bead/shared-bead/"))
	assert.True(t, strings.HasPrefix(res2.PreserveRef, "refs/ddx/execute-bead/shared-bead/"))
}

// TestExecuteBeadNoChanges verifies that when the agent makes no commits the
// outcome is "no-changes".
func TestExecuteBeadNoChanges(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111", // same as base — no commits made
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, "no-changes", res.Outcome)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Empty(t, res.PreserveRef)
}

// TestExecuteBeadDirtyWorktreeCheckpoint verifies that a dirty worktree is
// stashed (checkpointed) before execution begins.
func TestExecuteBeadDirtyWorktreeCheckpoint(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222",
		dirty:       true,
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	f := newExecuteBeadFactory(t, git, runner)

	runExecuteBead(t, f, git, "my-bead")

	assert.True(t, git.stashCalled, "stash should have been called for dirty worktree")
}

// TestExecuteBeadFromRevFlag verifies that --from resolves a custom revision
// and uses it as the base for the worktree.
func TestExecuteBeadFromRevFlag(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "custom-sha-123",
		wtHeadRev:   "custom-sha-123", // no-changes so we don't need merge logic
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead", "--from", "custom-rev")

	assert.Equal(t, "custom-sha-123", res.BaseRev)
}

// TestExecuteBeadOrphanRecovery verifies that worktrees matching the bead's
// prefix are cleaned up at the start of a new run.
func TestExecuteBeadOrphanRecovery(t *testing.T) {
	workDir := t.TempDir()
	orphanPath := workDir + "/.ddx/" + executeBeadWtPrefix + "my-bead-old-attempt"
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111",
		worktrees:   []string{orphanPath},
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = runner
	f.executeBeadGitOverride = git

	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json")
	require.NoError(t, err, "output: %s", out)

	// The orphan worktree should have been removed.
	assert.Contains(t, git.removedWTs, orphanPath,
		"orphan worktree should be removed before the new run")
}

// TestExecuteBeadAgentErrorNoCommits verifies that when the agent runner returns
// an error but makes no commits, exitCode=1 and outcome="no-changes".
func TestExecuteBeadAgentErrorNoCommits(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111", // no commits made
	}
	runner := &fakeAgentRunner{err: fmt.Errorf("agent crashed"), result: nil}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, 1, res.ExitCode)
	assert.Equal(t, "no-changes", res.Outcome)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Empty(t, res.PreserveRef)
}

// TestExecuteBeadAgentErrorWithCommitsMerges verifies that when the agent runner
// returns an error but commits exist and ff-merge succeeds, exitCode=1 and
// outcome="merged".
func TestExecuteBeadAgentErrorWithCommitsMerges(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222", // agent made commits
		ffMergeErr:  nil,        // merge succeeds
	}
	runner := &fakeAgentRunner{err: fmt.Errorf("agent crashed"), result: nil}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, 1, res.ExitCode)
	assert.Equal(t, "merged", res.Outcome)
	assert.Equal(t, "bbbb2222", res.ResultRev)
}

// TestExecuteBeadAgentErrorWithCommitsPreserves verifies that when the agent
// runner returns an error, commits exist but ff-merge fails, exitCode=1 and
// outcome="preserved" with a non-empty preserve ref.
func TestExecuteBeadAgentErrorWithCommitsPreserves(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222",
		ffMergeErr:  fmt.Errorf("not possible to fast-forward"),
	}
	runner := &fakeAgentRunner{err: fmt.Errorf("agent crashed"), result: nil}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, 1, res.ExitCode)
	assert.Equal(t, "preserved", res.Outcome)
	assert.Equal(t, "bbbb2222", res.ResultRev)
	assert.NotEmpty(t, res.PreserveRef)
	assert.True(t, strings.HasPrefix(res.PreserveRef, "refs/ddx/execute-bead/my-bead/"))
}

// TestExecuteBeadEvidenceFields verifies that runtime evidence fields are
// populated in the JSON output.
func TestExecuteBeadEvidenceFields(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222",
	}
	runner := &fakeAgentRunner{result: &agent.Result{
		ExitCode: 0,
		Harness:  "testharness",
		Model:    "test-model",
		Tokens:   42,
		CostUSD:  0.001,
	}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, "testharness", res.Harness)
	assert.Equal(t, "test-model", res.Model)
	assert.Equal(t, 42, res.Tokens)
	assert.InDelta(t, 0.001, res.CostUSD, 1e-9)
	assert.NotEmpty(t, res.SessionID)
	assert.False(t, res.StartedAt.IsZero())
	assert.False(t, res.FinishedAt.IsZero())
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Equal(t, "bbbb2222", res.ResultRev)
}
