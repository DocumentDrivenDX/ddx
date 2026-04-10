package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeExecuteBeadGit is a mock implementation of executeBeadGitOps for tests.
type fakeExecuteBeadGit struct {
	mu sync.Mutex

	// mainHeadRev is returned by HeadRev/ResolveRev for the main working dir.
	mainHeadRev string
	// headRevSeq, when set, is returned in order for successive main-dir HeadRev calls.
	headRevSeq []string
	headRevIdx int
	// wtHeadRev is returned by HeadRev for worktree paths (after agent run).
	wtHeadRev string
	// wtHeadRevErr, if set, is returned by HeadRev for worktree paths.
	wtHeadRevErr error
	// rebaseResultRev, if set, replaces wtHeadRev after a successful rebase.
	rebaseResultRev string
	dirty           bool
	ffMergeErr      error
	rebaseErr       error
	updateRefErr    error
	stashPopErr     error

	stashCalled bool
	addedWTs    []string
	removedWTs  []string
	refs        map[string]string // ref -> sha recorded by UpdateRef
	worktrees   []string          // paths returned by WorktreeList

	rebaseCalls   int
	rebaseOntoRev string
	ffMergeCalls  int
	ffMergeRev    string
}

func (f *fakeExecuteBeadGit) HeadRev(dir string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(dir, executeBeadWtPrefix) {
		if f.wtHeadRevErr != nil {
			return "", f.wtHeadRevErr
		}
		return f.wtHeadRev, nil
	}
	if len(f.headRevSeq) > 0 {
		idx := f.headRevIdx
		if idx >= len(f.headRevSeq) {
			idx = len(f.headRevSeq) - 1
		}
		rev := f.headRevSeq[idx]
		f.headRevIdx++
		return rev, nil
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

func (f *fakeExecuteBeadGit) StashPop(dir string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stashPopErr
}

func (f *fakeExecuteBeadGit) WorktreeAdd(dir, wtPath, rev string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addedWTs = append(f.addedWTs, wtPath)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		return err
	}
	beadFile := filepath.Join(dir, ".ddx", "beads.jsonl")
	if _, err := os.Stat(beadFile); err == nil {
		if err := copyTestFile(beadFile, filepath.Join(wtPath, ".ddx", "beads.jsonl")); err != nil {
			return err
		}
	}
	docsDir := filepath.Join(dir, "docs")
	if _, err := os.Stat(docsDir); err == nil {
		if err := copyTree(docsDir, filepath.Join(wtPath, "docs")); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeExecuteBeadGit) WorktreeRemove(dir, wtPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removedWTs = append(f.removedWTs, wtPath)
	if err := os.RemoveAll(wtPath); err != nil {
		return err
	}
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
	f.ffMergeCalls++
	f.ffMergeRev = rev
	return f.ffMergeErr
}

func (f *fakeExecuteBeadGit) Rebase(wtPath, ontoRev string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rebaseCalls++
	f.rebaseOntoRev = ontoRev
	if f.rebaseErr == nil && f.rebaseResultRev != "" {
		f.wtHeadRev = f.rebaseResultRev
	}
	return f.rebaseErr
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
	last   agent.RunOptions
}

func (r *fakeAgentRunner) Run(opts agent.RunOptions) (*agent.Result, error) {
	r.last = opts
	return r.result, r.err
}

// newExecuteBeadFactory builds a CommandFactory wired with the given fake git and agent runner.
func newExecuteBeadFactory(t *testing.T, git *fakeExecuteBeadGit, runner *fakeAgentRunner) *CommandFactory {
	t.Helper()
	f := NewCommandFactory(t.TempDir())
	seedDefaultExecuteBeads(t, f.WorkingDir)
	f.AgentRunnerOverride = runner
	f.executeBeadGitOverride = git
	return f
}

func assertPreserveRef(t *testing.T, ref, beadID, baseRev string) {
	t.Helper()
	shortSHA := baseRev
	if len(shortSHA) > 12 {
		shortSHA = shortSHA[:12]
	}
	pattern := fmt.Sprintf(`^refs/ddx/iterations/%s/\d{8}T\d{6}Z-%s$`,
		regexp.QuoteMeta(beadID), regexp.QuoteMeta(shortSHA))
	require.Regexp(t, pattern, ref)
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
	return parseExecuteBeadJSON(t, out)
}

func parseExecuteBeadJSON(t *testing.T, out string) ExecuteBeadResult {
	t.Helper()
	// Strip any non-JSON prefix lines (e.g. stderr notes written to the shared buffer).
	jsonStart := strings.Index(out, "{")
	require.NotEqual(t, -1, jsonStart, "output should contain JSON: %s", out)
	jsonPart := out[jsonStart:]
	var res ExecuteBeadResult
	dec := json.NewDecoder(bytes.NewBufferString(jsonPart))
	require.NoError(t, dec.Decode(&res), "output should be valid JSON: %s", jsonPart)
	return res
}

func seedExecuteBead(t *testing.T, workDir string, b *bead.Bead) {
	t.Helper()
	store := bead.NewStore(filepath.Join(workDir, ".ddx"))
	require.NoError(t, store.Init())
	if _, err := store.Get(b.ID); err == nil {
		return
	}
	require.NoError(t, store.Create(b))
}

func seedDefaultExecuteBeads(t *testing.T, workDir string) {
	t.Helper()
	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead",
		Status:    bead.StatusOpen,
		Priority:  0,
		IssueType: bead.DefaultType,
	})
	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "shared-bead",
		Title:     "Shared execute-bead",
		Status:    bead.StatusOpen,
		Priority:  0,
		IssueType: bead.DefaultType,
	})
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return os.Chmod(target, info.Mode())
	})
}

func copyTestFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
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
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
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
	assert.Equal(t, 0, git.rebaseCalls)
	assert.Equal(t, "bbbb2222", git.ffMergeRev)
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
	assert.Equal(t, agent.ExecuteBeadStatusLandConflict, res.Status)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Equal(t, "cccc3333", res.ResultRev)
	assert.NotEmpty(t, res.PreserveRef)
	assertPreserveRef(t, res.PreserveRef, "my-bead", "aaaa1111")
	assert.Equal(t, "ff-merge not possible", res.Reason)

	// Hidden ref should be recorded in the mock.
	require.Contains(t, git.refs, res.PreserveRef)
	assert.Equal(t, "cccc3333", git.refs[res.PreserveRef])
	assert.Equal(t, 0, git.rebaseCalls)
	assert.Equal(t, "cccc3333", git.ffMergeRev)
}

// TestExecuteBeadRebasesBeforeMerge verifies that when the target branch
// advances during execution, execute-bead rebases the worktree commit before
// attempting the fast-forward land.
func TestExecuteBeadRebasesBeforeMerge(t *testing.T) {
	git := &fakeExecuteBeadGit{
		headRevSeq:      []string{"aaaa1111", "cccc3333"},
		wtHeadRev:       "bbbb2222",
		rebaseResultRev: "dddd4444",
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, "merged", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Equal(t, "dddd4444", res.ResultRev)
	assert.Equal(t, 1, git.rebaseCalls)
	assert.Equal(t, "cccc3333", git.rebaseOntoRev)
	assert.Equal(t, 1, git.ffMergeCalls)
	assert.Equal(t, "dddd4444", git.ffMergeRev)
}

// TestExecuteBeadRebaseFailurePreserves verifies that a rebase conflict
// preserves the iteration without attempting the ff-only land.
func TestExecuteBeadRebaseFailurePreserves(t *testing.T) {
	git := &fakeExecuteBeadGit{
		headRevSeq: []string{"aaaa1111", "cccc3333"},
		wtHeadRev:  "bbbb2222",
		rebaseErr:  fmt.Errorf("conflict"),
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, "preserved", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusLandConflict, res.Status)
	assert.Equal(t, "rebase failed", res.Reason)
	assert.Equal(t, "bbbb2222", res.ResultRev)
	assert.Equal(t, 1, git.rebaseCalls)
	assert.Equal(t, "cccc3333", git.rebaseOntoRev)
	assert.Equal(t, 0, git.ffMergeCalls)
	require.Contains(t, git.refs, res.PreserveRef)
	assert.Equal(t, "bbbb2222", git.refs[res.PreserveRef])
	assertPreserveRef(t, res.PreserveRef, "my-bead", "aaaa1111")
	require.FileExists(t, filepath.Join(f.WorkingDir, filepath.FromSlash(res.ResultFile)))
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
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, "--no-merge specified", res.Reason)
	assert.NotEmpty(t, res.PreserveRef)
	assertPreserveRef(t, res.PreserveRef, "my-bead", "aaaa1111")

	// FFMerge should not have been called; refs should still be recorded.
	require.Contains(t, git.refs, res.PreserveRef)
}

// TestExecuteBeadHiddenRefUniqueness verifies that two runs on the same bead-id
// produce distinct preserve refs (concurrent hidden-ref uniqueness).
func TestExecuteBeadHiddenRefUniqueness(t *testing.T) {
	makeRun := func(ts time.Time) ExecuteBeadResult {
		oldNow := executeBeadNow
		executeBeadNow = func() time.Time { return ts }
		defer func() { executeBeadNow = oldNow }()

		git := &fakeExecuteBeadGit{
			mainHeadRev: "aaaa1111",
			wtHeadRev:   "eeee5555",
			ffMergeErr:  fmt.Errorf("diverged"),
		}
		runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
		f := newExecuteBeadFactory(t, git, runner)
		return runExecuteBead(t, f, git, "shared-bead")
	}

	res1 := makeRun(time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC))
	res2 := makeRun(time.Date(2026, 4, 10, 0, 0, 1, 0, time.UTC))

	assert.NotEqual(t, res1.PreserveRef, res2.PreserveRef,
		"concurrent runs must produce distinct preserve refs")
	assertPreserveRef(t, res1.PreserveRef, "shared-bead", "aaaa1111")
	assertPreserveRef(t, res2.PreserveRef, "shared-bead", "aaaa1111")
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
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Empty(t, res.PreserveRef)
}

func TestExecuteBeadSynthesizesPromptAndArtifacts(t *testing.T) {
	workDir := t.TempDir()
	seedExecuteBead(t, workDir, &bead.Bead{
		ID:          "my-bead",
		Title:       "Improve execute-bead prompt synthesis",
		Status:      bead.StatusOpen,
		Priority:    0,
		IssueType:   bead.DefaultType,
		Description: "Replace the bare fallback prompt with deterministic bead context.",
		Acceptance:  "Prompt contains bead context and governing references.",
		Labels:      []string{"area:agent", "phase:build"},
		Extra:       map[string]any{"spec-id": "FEAT-006"},
	})
	specPath := filepath.Join(workDir, "docs", "feature.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(specPath), 0o755))
	require.NoError(t, os.WriteFile(specPath, []byte(`---
ddx:
  id: FEAT-006
---
# Agent Service
`), 0o644))

	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222",
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0, Harness: "mock"}}
	f := NewCommandFactory(workDir)
	seedDefaultExecuteBeads(t, workDir)
	f.AgentRunnerOverride = runner
	f.executeBeadGitOverride = git

	res := runExecuteBead(t, f, git, "my-bead")

	require.NotEmpty(t, runner.last.PromptFile)
	require.FileExists(t, runner.last.PromptFile)
	promptRaw, err := os.ReadFile(runner.last.PromptFile)
	require.NoError(t, err)
	promptText := string(promptRaw)
	assert.Contains(t, promptText, "Improve execute-bead prompt synthesis")
	assert.Contains(t, promptText, "Replace the bare fallback prompt")
	assert.Contains(t, promptText, "Prompt contains bead context and governing references.")
	assert.Contains(t, promptText, "docs/feature.md")
	assert.NotContains(t, promptText, "Work on bead my-bead.")

	require.NotEmpty(t, res.ExecutionDir)
	require.NotEmpty(t, res.PromptFile)
	require.NotEmpty(t, res.ManifestFile)
	require.NotEmpty(t, res.ResultFile)
	assert.True(t, strings.HasSuffix(res.PromptFile, "prompt.md"))
	assert.True(t, strings.HasSuffix(res.ManifestFile, "manifest.json"))
	assert.True(t, strings.HasSuffix(res.ResultFile, "result.json"))
}

func TestExecuteBeadWritesResultArtifactBundle(t *testing.T) {
	workDir := t.TempDir()
	seedExecuteBead(t, workDir, &bead.Bead{
		ID:         "my-bead",
		Title:      "Record execution artifacts",
		Status:     bead.StatusOpen,
		Priority:   0,
		IssueType:  bead.DefaultType,
		Acceptance: "Artifacts are written for later inspection.",
	})
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222",
	}
	runner := &fakeAgentRunner{result: &agent.Result{
		ExitCode: 0,
		Harness:  "mock",
		Model:    "gpt-test",
		Tokens:   17,
	}}
	f := NewCommandFactory(workDir)
	seedDefaultExecuteBeads(t, workDir)
	f.AgentRunnerOverride = runner
	f.executeBeadGitOverride = git

	res := runExecuteBead(t, f, git, "my-bead")

	require.Len(t, git.addedWTs, 1)
	manifestPath := filepath.Join(workDir, filepath.FromSlash(res.ManifestFile))
	resultPath := filepath.Join(workDir, filepath.FromSlash(res.ResultFile))
	require.FileExists(t, manifestPath)
	require.FileExists(t, resultPath)

	manifestRaw, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Contains(t, string(manifestRaw), `"bead_id": "my-bead"`)
	assert.Contains(t, string(manifestRaw), `"prompt": "synthesized"`)
	assert.Contains(t, string(manifestRaw), `"worktree": ".ddx/`)

	resultRaw, err := os.ReadFile(resultPath)
	require.NoError(t, err)
	var recorded ExecuteBeadResult
	require.NoError(t, json.Unmarshal(resultRaw, &recorded))
	assert.Equal(t, res.BeadID, recorded.BeadID)
	assert.Equal(t, res.Status, recorded.Status)
	assert.Equal(t, res.ResultFile, recorded.ResultFile)
	assert.NoDirExists(t, git.addedWTs[0])
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
	seedDefaultExecuteBeads(t, workDir)
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
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Empty(t, res.PreserveRef)
}

// TestExecuteBeadAgentErrorWithCommitsPreservesBeforeLand verifies that a
// non-zero agent result preserves the iteration instead of touching the target
// branch, even if a fast-forward land would have succeeded.
func TestExecuteBeadAgentErrorWithCommitsPreservesBeforeLand(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222", // agent made commits
		ffMergeErr:  nil,        // merge succeeds
	}
	runner := &fakeAgentRunner{err: fmt.Errorf("agent crashed"), result: nil}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, 1, res.ExitCode)
	assert.Equal(t, "preserved", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
	assert.Equal(t, "bbbb2222", res.ResultRev)
	assert.NotEmpty(t, res.PreserveRef)
	assert.Equal(t, 0, git.ffMergeCalls)
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
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
	assert.Equal(t, "bbbb2222", res.ResultRev)
	assert.NotEmpty(t, res.PreserveRef)
	assertPreserveRef(t, res.PreserveRef, "my-bead", "aaaa1111")
}

// TestExecuteBeadAgentErrorMessageInOutput verifies that when the agent runner
// returns an error, the error message appears in the JSON output Error field.
func TestExecuteBeadAgentErrorMessageInOutput(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111", // no commits made
	}
	runner := &fakeAgentRunner{err: fmt.Errorf("agent crashed with detail"), result: nil}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, 1, res.ExitCode)
	assert.Equal(t, "agent crashed with detail", res.Error)
}

// TestExecuteBeadHeadRevFailure verifies that when HeadRev fails after the agent
// runs, the outcome is "error" and the reason contains the original error message.
// This covers the path at agent_execute_bead.go lines 282-309.
func TestExecuteBeadHeadRevFailure(t *testing.T) {
	t.Run("json output", func(t *testing.T) {
		git := &fakeExecuteBeadGit{
			mainHeadRev:  "aaaa1111",
			wtHeadRevErr: fmt.Errorf("disk read error"),
		}
		runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
		f := newExecuteBeadFactory(t, git, runner)

		root := f.NewRootCommand()
		out, cmdErr := executeCommand(root, "agent", "execute-bead", "my-bead", "--json")
		require.Error(t, cmdErr)
		res := parseExecuteBeadJSON(t, out)

		assert.Equal(t, "error", res.Outcome)
		assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
		assert.Contains(t, res.Reason, "disk read error")
		assert.Equal(t, 1, res.ExitCode)
	})

	t.Run("text output", func(t *testing.T) {
		git := &fakeExecuteBeadGit{
			mainHeadRev:  "aaaa1111",
			wtHeadRevErr: fmt.Errorf("disk read error"),
		}
		runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
		f := newExecuteBeadFactory(t, git, runner)

		root := f.NewRootCommand()
		out, cmdErr := executeCommand(root, "agent", "execute-bead", "my-bead")
		require.Error(t, cmdErr)

		assert.Contains(t, out, "outcome: error")
		assert.Contains(t, out, "disk read error")
	})
}

// TestExecuteBeadCompoundErrorAgentAndHeadRevFailure verifies that when the
// agent runner returns an error AND HeadRev fails on the worktree, both the
// Error field (agent message) and the Reason field (rev error) are present in
// the JSON output. This covers the path at agent_execute_bead.go that
// previously dropped the agent error message when revErr was non-nil.
func TestExecuteBeadCompoundErrorAgentAndHeadRevFailure(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev:  "aaaa1111",
		wtHeadRevErr: fmt.Errorf("worktree HEAD unreadable"),
	}
	runner := &fakeAgentRunner{err: fmt.Errorf("agent exploded"), result: nil}
	f := newExecuteBeadFactory(t, git, runner)

	root := f.NewRootCommand()
	out, cmdErr := executeCommand(root, "agent", "execute-bead", "my-bead", "--json")
	require.Error(t, cmdErr)
	res := parseExecuteBeadJSON(t, out)

	assert.Equal(t, 1, res.ExitCode)
	assert.Equal(t, "error", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
	assert.Equal(t, "agent exploded", res.Error,
		"agent error message must be preserved even when HeadRev also fails")
	assert.Contains(t, res.Reason, "worktree HEAD unreadable",
		"Reason must reflect the HeadRev failure")
}

// TestExecuteBeadInvalidBeadID verifies that beadIDs with characters illegal
// in git ref names are rejected with a clear error before any git or agent
// operations are attempted.
func TestExecuteBeadInvalidBeadID(t *testing.T) {
	invalidIDs := []string{
		"bead with spaces",
		"bead~1",
		"bead^1",
		"bead:name",
		"bead[0]",
	}
	for _, id := range invalidIDs {
		t.Run(id, func(t *testing.T) {
			git := &fakeExecuteBeadGit{mainHeadRev: "aaaa1111"}
			runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
			f := newExecuteBeadFactory(t, git, runner)

			root := f.NewRootCommand()
			_, cmdErr := executeCommand(root, "agent", "execute-bead", id)
			require.Error(t, cmdErr)
			assert.Contains(t, cmdErr.Error(), "invalid bead ID")

			// No git or agent operations should have been attempted.
			assert.Empty(t, git.addedWTs, "no worktree should be created for invalid bead ID")
		})
	}
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

func TestExecuteBeadStatusMapping(t *testing.T) {
	cases := []struct {
		name     string
		result   ExecuteBeadResult
		expected string
	}{
		{
			name:     "merged success",
			result:   ExecuteBeadResult{Outcome: "merged", ExitCode: 0},
			expected: agent.ExecuteBeadStatusSuccess,
		},
		{
			name:     "no changes success",
			result:   ExecuteBeadResult{Outcome: "no-changes", ExitCode: 0},
			expected: agent.ExecuteBeadStatusSuccess,
		},
		{
			name:     "execution failure dominates preserved outcome",
			result:   ExecuteBeadResult{Outcome: "preserved", ExitCode: 1, Reason: "agent execution failed"},
			expected: agent.ExecuteBeadStatusExecutionFailed,
		},
		{
			name:     "land conflict",
			result:   ExecuteBeadResult{Outcome: "preserved", ExitCode: 0, Reason: "ff-merge not possible"},
			expected: agent.ExecuteBeadStatusLandConflict,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := tc.result
			populateExecuteBeadStatus(&res)
			assert.Equal(t, tc.expected, res.Status)
			assert.NotEmpty(t, res.Detail)
		})
	}
}
