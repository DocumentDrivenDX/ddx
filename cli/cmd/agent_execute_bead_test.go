package cmd

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

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
	// wtDirty is returned by IsDirty for worktree paths.
	wtDirty bool
	// synthRev, if set, is applied as wtHeadRev when SynthesizeCommit is called.
	synthRev string
	// wtHeadRevErr, if set, is returned by HeadRev for worktree paths.
	wtHeadRevErr error
	dirty        bool
	mergeErr     error
	updateRefErr error

	addedWTs   []string
	addedWTRev string
	removedWTs []string
	refs       map[string]string // ref -> sha recorded by UpdateRef
	worktrees  []string          // paths returned by WorktreeList

	mergeCalls int
	mergeRev   string
}

func (f *fakeExecuteBeadGit) HeadRev(dir string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(dir, agent.ExecuteBeadWtPrefix) {
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
	if strings.Contains(dir, agent.ExecuteBeadWtPrefix) {
		return f.wtDirty, nil
	}
	return f.dirty, nil
}

func (f *fakeExecuteBeadGit) WorktreeAdd(dir, wtPath, rev string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addedWTs = append(f.addedWTs, wtPath)
	f.addedWTRev = rev
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

func (f *fakeExecuteBeadGit) SynthesizeCommit(dir, msg string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(dir, agent.ExecuteBeadWtPrefix) && f.synthRev != "" {
		f.wtHeadRev = f.synthRev
		return true, nil
	}
	// wtDirty is true but synthRev is empty: simulates all-noise worktree.
	return false, nil
}

func (f *fakeExecuteBeadGit) WorktreePrune(dir string) error { return nil }

func (f *fakeExecuteBeadGit) Merge(dir, rev string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mergeCalls++
	f.mergeRev = rev
	return f.mergeErr
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

func (f *fakeExecuteBeadGit) DeleteRef(dir, ref string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refs != nil {
		delete(f.refs, ref)
	}
	return nil
}

// fakeAgentRunner is a minimal mock agent runner for execute-bead tests.
type fakeAgentRunner struct {
	result *agent.Result
	err    error
	last   agent.RunOptions
	// sideEffect, when set, runs while the runner has the opts in hand. It
	// is used to simulate runtime state the embedded agent harness would
	// otherwise write (session logs, telemetry, etc.) so tests can assert
	// where those files land.
	sideEffect func(opts agent.RunOptions) error
}

func (r *fakeAgentRunner) Run(opts agent.RunOptions) (*agent.Result, error) {
	r.last = opts
	if r.sideEffect != nil {
		if err := r.sideEffect(opts); err != nil {
			return nil, err
		}
	}
	return r.result, r.err
}

// newExecuteBeadFactory builds a CommandFactory wired with the given fake git and agent runner.
func newExecuteBeadFactory(t *testing.T, git *fakeExecuteBeadGit, runner *fakeAgentRunner) *CommandFactory {
	t.Helper()
	f := NewCommandFactory(t.TempDir())
	seedDefaultExecuteBeads(t, f.WorkingDir)
	f.AgentRunnerOverride = runner
	f.executeBeadGitOverride = git
	f.executeBeadOrchestratorGitOverride = git
	f.executeBeadLandingAdvancerOverride = fakeLandingAdvancerFromGit(git)
	return f
}

// fakeLandingAdvancerFromGit returns a LandingAdvancer callback that maps the
// fake git's Merge/UpdateRef semantics onto the coordinator-pattern advancer
// interface. Semantically, "mergeCalls" now means "number of times
// LandBeadResult invoked the advancer (attempted to advance the target
// branch)". Used by tests that were written against the old Merge() path so
// they continue to pass after the land-coordinator refactor.
func fakeLandingAdvancerFromGit(git *fakeExecuteBeadGit) func(res *agent.ExecuteBeadResult) (*agent.LandResult, error) {
	return func(res *agent.ExecuteBeadResult) (*agent.LandResult, error) {
		if err := git.Merge("", res.ResultRev); err != nil {
			preserveRef := agent.PreserveRef(res.BeadID, res.BaseRev)
			// Record the preserve ref in the fake's refs map so tests that
			// assert git.refs[preserveRef] continue to pass.
			_ = git.UpdateRef("", preserveRef, res.ResultRev)
			return &agent.LandResult{
				Status:      "preserved",
				PreserveRef: preserveRef,
				Reason:      "merge failed",
			}, nil
		}
		return &agent.LandResult{Status: "landed", NewTip: res.ResultRev}, nil
	}
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
func runExecuteBead(t *testing.T, f *CommandFactory, git *fakeExecuteBeadGit, beadID string, extraArgs ...string) agent.ExecuteBeadResult {
	t.Helper()
	root := f.NewRootCommand()
	args := append([]string{"agent", "execute-bead", beadID, "--json"}, extraArgs...)
	out, err := executeCommand(root, args...)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	return parseExecuteBeadJSON(t, out)
}

func parseExecuteBeadJSON(t *testing.T, out string) agent.ExecuteBeadResult {
	t.Helper()
	// Strip any non-JSON prefix lines (e.g. stderr notes written to the shared buffer).
	jsonStart := strings.Index(out, "{")
	require.NotEqual(t, -1, jsonStart, "output should contain JSON: %s", out)
	jsonPart := out[jsonStart:]
	var res agent.ExecuteBeadResult
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

// TestExecuteBeadMerge verifies that when the agent produces a real commit and
// the target branch can advance, the outcome is "merged". Exercises real git
// plumbing (worktree add/remove, ref update, file sync) and the script harness
// driving an actual commit — no fakes for the components under test.
func TestExecuteBeadMerge(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead merge",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Directive file drives the script harness to make one real commit in the
	// worker worktree, replacing fakeAgentRunner's canned Result struct.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"create-file hello.txt hello-content\n"+
			"commit feat: add hello\n",
	), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	assert.Equal(t, "merged", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, baseSHA, res.BaseRev)
	assert.NotEmpty(t, res.ResultRev)
	assert.NotEqual(t, res.BaseRev, res.ResultRev)
	assert.Empty(t, res.PreserveRef)
	assert.Equal(t, "my-bead", res.BeadID)
	assert.NotEmpty(t, res.SessionID)

	// Real-git assertion: main must have advanced to ResultRev via Land().
	assert.Equal(t, res.ResultRev, runGit("rev-parse", "HEAD"),
		"main should advance to ResultRev after a successful merge")

	// The worker worktree must have been created and cleaned up: no entries
	// under the execute-bead prefix remain in `git worktree list`.
	wtList := runGit("worktree", "list", "--porcelain")
	assert.NotContains(t, wtList, agent.ExecuteBeadWtPrefix+"my-bead-",
		"execute-bead worktree should be removed after the run")

	// File from the agent's commit must be materialized on main through real
	// merge/ff semantics (regression guard for ddx-eaebaffb: update-ref
	// bypassing the working tree).
	content, err := os.ReadFile(filepath.Join(workDir, "hello.txt"))
	require.NoError(t, err, "hello.txt should materialize on main after Land()")
	assert.Equal(t, "hello-content", string(content))
}

// TestExecuteBeadPreserveOnMergeFailure verifies that when merge fails
// the result is preserved under a hidden ref. Exercises real git plumbing
// and the script harness producing an actual conflicting commit — no fakes
// for the components under test.
func TestExecuteBeadPreserveOnMergeFailure(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead preserve on merge failure",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Advance main past baseSHA with a sibling commit that writes hello.txt
	// with content different from what the agent will produce. Once the
	// agent commits its own hello.txt on a worktree rooted at baseSHA,
	// Land() must attempt a merge and hit a content conflict, triggering
	// the preserve path.
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "hello.txt"), []byte("sibling-content\n"), 0o644))
	runGit("add", "hello.txt")
	runGit("commit", "-m", "feat: sibling conflicting change")
	siblingSHA := runGit("rev-parse", "HEAD")

	// Directive drives the real script harness to emit one real commit
	// touching the same file as the sibling — replacing fakeAgentRunner's
	// canned Result struct with an actual worker commit.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"create-file hello.txt worker-content\n"+
			"commit feat: worker add hello\n",
	), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	assert.Equal(t, "preserved", res.Outcome)
	assert.Equal(t, baseSHA, res.BaseRev)
	assert.NotEmpty(t, res.ResultRev)
	assert.NotEqual(t, res.BaseRev, res.ResultRev)
	workerSHA := res.ResultRev
	assert.NotEmpty(t, res.PreserveRef)
	// Real Land() records the ref as refs/ddx/iterations/<bead>/<attempt>-<currentTip[:12]>,
	// where currentTip is the sibling commit that made the merge conflict.
	require.Regexp(t,
		fmt.Sprintf(`^refs/ddx/iterations/my-bead/.+-%s$`, regexp.QuoteMeta(siblingSHA[:12])),
		res.PreserveRef,
	)
	assert.NotEmpty(t, res.Reason, "merge-conflict preserve must carry a reason")

	// Real-git assertion: the hidden ref must exist and resolve to the worker
	// commit (pre-refactor: git.refs[preserveRef] == "cccc3333").
	preservedSHA := runGit("rev-parse", res.PreserveRef)
	assert.Equal(t, workerSHA, preservedSHA, "preserve ref must resolve to the worker commit")

	// Target branch must be untouched — merge was refused, not applied.
	// (Pre-refactor equivalent: assert.Equal(t, 1, git.mergeCalls) confirmed
	// a merge was attempted; with real git, the observable outcome is that
	// main never advances past the sibling and no merge commit exists.)
	assert.Equal(t, siblingSHA, runGit("rev-parse", "refs/heads/main"),
		"main must be unchanged when merge preserves")
	mergeCommits := runGit("log", "--merges", "--format=%H", "refs/heads/main")
	assert.Empty(t, mergeCommits, "preserve path must not produce a merge commit on main")
}

// TestExecuteBeadNoMerge verifies that --no-merge skips merge and
// always preserves under a hidden ref. Exercises real git plumbing and
// the script harness producing an actual worker commit — no fakes for
// the components under test.
func TestExecuteBeadNoMerge(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead no-merge",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Directive file drives the script harness to make one real commit in
	// the worker worktree — replacing fakeAgentRunner's canned Result struct
	// with an actual worker commit. The merge would succeed, but --no-merge
	// must suppress it.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"create-file hello.txt hello-content\n"+
			"commit feat: add hello\n",
	), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--no-merge",
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	assert.Equal(t, "preserved", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, "--no-merge specified", res.Reason)
	assert.Equal(t, baseSHA, res.BaseRev)
	assert.NotEmpty(t, res.ResultRev)
	assert.NotEqual(t, res.BaseRev, res.ResultRev)
	workerSHA := res.ResultRev
	assert.NotEmpty(t, res.PreserveRef)
	assertPreserveRef(t, res.PreserveRef, "my-bead", baseSHA)

	// Real-git assertion: the hidden ref must exist and resolve to the
	// worker commit (pre-refactor: git.refs[preserveRef] == "dddd4444").
	preservedSHA := runGit("rev-parse", res.PreserveRef)
	assert.Equal(t, workerSHA, preservedSHA, "preserve ref must resolve to the worker commit")

	// Target branch must not advance — merge was suppressed, not applied.
	// (Pre-refactor equivalent: assert.Equal(t, 0, git.mergeCalls).)
	assert.Equal(t, baseSHA, runGit("rev-parse", "refs/heads/main"),
		"main must be unchanged when --no-merge preserves")
	mergeCommits := runGit("log", "--merges", "--format=%H", "refs/heads/main")
	assert.Empty(t, mergeCommits, "no-merge path must not produce a merge commit on main")
}

// TestExecuteBeadHiddenRefUniqueness verifies that two runs on the same bead-id
// produce distinct preserve refs (concurrent hidden-ref uniqueness).
// Exercises real git plumbing and the script harness producing actual
// conflicting commits — no fakes for the components under test.
func TestExecuteBeadHiddenRefUniqueness(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "shared-bead",
		Title:     "Test execute-bead hidden-ref uniqueness",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Sibling commit conflicts with the worker's hello.txt content, forcing
	// every run to take the merge-conflict preserve path.
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "hello.txt"), []byte("sibling-content\n"), 0o644))
	runGit("add", "hello.txt")
	runGit("commit", "-m", "feat: sibling conflicting change")
	siblingSHA := runGit("rev-parse", "HEAD")

	// Each run drives the script harness to emit a real commit touching the
	// same file as the sibling, so Land() must preserve under a hidden ref.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"create-file hello.txt worker-content\n"+
			"commit feat: worker add hello\n",
	), 0o644))

	makeRun := func() agent.ExecuteBeadResult {
		t.Helper()
		f := NewCommandFactory(workDir)
		f.AgentRunnerOverride = agent.NewRunner(agent.Config{})
		root := f.NewRootCommand()
		out, err := executeCommand(root, "agent", "execute-bead", "shared-bead", "--json",
			"--from", baseSHA,
			"--harness", "script", "--model", directivePath)
		require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
		return parseExecuteBeadJSON(t, out)
	}

	res1 := makeRun()
	res2 := makeRun()

	assert.Equal(t, "preserved", res1.Outcome)
	assert.Equal(t, "preserved", res2.Outcome)
	assert.NotEqual(t, res1.PreserveRef, res2.PreserveRef,
		"concurrent runs must produce distinct preserve refs")
	// Real Land() records the ref as refs/ddx/iterations/<bead>/<attempt>-<currentTip[:12]>.
	// The first run's currentTip is siblingSHA; the second run's tip differs
	// because execute-bead checkpoints tracker/worktree state between runs.
	require.Regexp(t,
		fmt.Sprintf(`^refs/ddx/iterations/shared-bead/.+-%s$`, regexp.QuoteMeta(siblingSHA[:12])),
		res1.PreserveRef)
	require.Regexp(t, `^refs/ddx/iterations/shared-bead/.+-[0-9a-f]{12}$`, res2.PreserveRef)

	// Both hidden refs must exist in the real repo and resolve to their
	// respective worker commits.
	assert.Equal(t, res1.ResultRev, runGit("rev-parse", res1.PreserveRef))
	assert.Equal(t, res2.ResultRev, runGit("rev-parse", res2.PreserveRef))
}

// TestExecuteBeadNoChanges verifies that when the agent makes no commits the
// outcome is "no-changes". Exercises real git + the script harness: the
// directive file tells the harness to no-op, so execute-bead observes an
// unchanged worktree HEAD and classifies the run accordingly.
func TestExecuteBeadNoChanges(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead no-changes outcome",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Script harness does nothing — no file creation, no commit.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte("no-op\n"), 0o644))

	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})
	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	assert.Equal(t, "no-changes", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusNoChanges, res.Status)
	assert.Equal(t, baseSHA, res.BaseRev)
	assert.Empty(t, res.PreserveRef)
}

// TestExecuteBeadDirtyWorktreeWithoutCommits verifies that tracked file edits
// left uncommitted by the agent are synthesized into a commit and treated as
// real output rather than being discarded as "no-changes". Exercises real git
// + the script harness: the directive file edits a tracked file without
// committing, so the worktree HEAD is unchanged but dirty, and execute-bead's
// synthesize-commit path lands the edit onto main.
func TestExecuteBeadDirtyWorktreeWithoutCommits(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead dirty-worktree synthesis",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Script harness edits a tracked file but never commits — worktree is
	// dirty, HEAD is unchanged.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte("append-line seed.txt agent-edit\n"), 0o644))

	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})
	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	assert.NotEqual(t, "no-changes", res.Outcome, "dirty worktree should not be classified as no-changes")
	assert.Equal(t, "merged", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, baseSHA, res.BaseRev)
	assert.NotEmpty(t, res.ResultRev)
	assert.NotEqual(t, baseSHA, res.ResultRev, "synthesized commit must advance past base")

	// The synthesized edit must have landed on main.
	mainHead := runGit("rev-parse", "HEAD")
	assert.Equal(t, res.ResultRev, mainHead, "merged result rev must be main HEAD")
	seedContent, err := os.ReadFile(filepath.Join(workDir, "seed.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(seedContent), "agent-edit")
}

// TestExecuteBeadMergePreservesContext verifies that when the agent produces
// multiple real commits and the merge lands, every intermediate commit on the
// worker branch remains reachable from main — i.e., the merge preserves the
// worker's commit context rather than collapsing or rewriting it. Exercises
// real git + RealLandingGitOps via the script harness driving actual commits;
// no fakes for the components under test.
func TestExecuteBeadMergePreservesContext(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead merge preserves context",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Script harness emits two distinct commits on the worker branch. After
	// landing, both commits must remain reachable from main — that's the
	// "preserves context" property.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"create-file step1.txt step1-content\n"+
			"commit feat: step 1\n"+
			"create-file step2.txt step2-content\n"+
			"commit feat: step 2\n",
	), 0o644))

	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})
	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	assert.Equal(t, "merged", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, baseSHA, res.BaseRev)
	assert.NotEmpty(t, res.ResultRev)
	assert.NotEqual(t, baseSHA, res.ResultRev)
	assert.Empty(t, res.PreserveRef)

	// Main must have advanced to include ResultRev through real land semantics.
	mainHead := runGit("rev-parse", "HEAD")
	assert.Equal(t, res.ResultRev, mainHead, "main HEAD should advance to ResultRev")

	// Context-preservation: ResultRev must still point to the worker's final
	// commit with its full parent chain intact. Both worker commits must be
	// reachable from main, proving the merge didn't squash or rewrite them.
	log := runGit("log", "--format=%s", baseSHA+"..HEAD")
	assert.Contains(t, log, "feat: step 1")
	assert.Contains(t, log, "feat: step 2")

	// Files from both worker commits must materialize on main.
	step1, err := os.ReadFile(filepath.Join(workDir, "step1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "step1-content", string(step1))
	step2, err := os.ReadFile(filepath.Join(workDir, "step2.txt"))
	require.NoError(t, err)
	assert.Equal(t, "step2-content", string(step2))
}

// TestExecuteBeadSynthesizesPromptAndArtifacts verifies that execute-bead
// synthesizes a prompt with the bead's description, acceptance, and governing
// references, and emits the expected artifact bundle. Exercises real git +
// RealLandingGitOps via the script harness — no fakes for the components
// under test. The script harness is driven by a no-op directive; the prompt
// file synthesized by the orchestrator is read directly from the execution
// bundle on disk.
func TestExecuteBeadSynthesizesPromptAndArtifacts(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	// Spec referenced via spec-id must exist in the committed tree so the
	// worker worktree (created by real `git worktree add`) sees it when the
	// orchestrator resolves governing refs.
	specPath := filepath.Join(workDir, "docs", "feature.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(specPath), 0o755))
	require.NoError(t, os.WriteFile(specPath, []byte(`---
ddx:
  id: FEAT-006
---
# Agent Service
`), 0o644))
	runGit("add", "docs/feature.md")
	runGit("commit", "-m", "docs: add feature spec")

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
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Script harness directive: no-op. The assertion target is the synthesized
	// prompt file, not any worker output. A no-op directive keeps the run
	// deterministic while still exercising the real runner + real git path.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte("no-op\n"), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	require.NotEmpty(t, res.PromptFile)
	promptPath := filepath.Join(workDir, filepath.FromSlash(res.PromptFile))
	require.FileExists(t, promptPath)
	promptRaw, err := os.ReadFile(promptPath)
	require.NoError(t, err)
	promptText := string(promptRaw)
	assert.Contains(t, promptText, "Improve execute-bead prompt synthesis")
	assert.Contains(t, promptText, "Replace the bare fallback prompt")
	assert.Contains(t, promptText, "Prompt contains bead context and governing references.")
	assert.Contains(t, promptText, "docs/feature.md")
	assert.NotContains(t, promptText, "Work on bead my-bead.")

	require.NotEmpty(t, res.ExecutionDir)
	require.NotEmpty(t, res.ManifestFile)
	require.NotEmpty(t, res.ResultFile)
	assert.True(t, strings.HasSuffix(res.PromptFile, "prompt.md"))
	assert.True(t, strings.HasSuffix(res.ManifestFile, "manifest.json"))
	assert.True(t, strings.HasSuffix(res.ResultFile, "result.json"))
}

// TestExecuteBeadResolvesPathStyleSpecID verifies that a bead whose spec-id is
// a relative path (e.g. "workflows/README.md") — not an ID from the docgraph —
// is resolved into a governing reference whose ID and Path both equal the given
// path, and that the synthesized prompt surfaces that path. Exercises real git
// + RealLandingGitOps via the script harness — no fakes for the components
// under test. The script harness is driven by a no-op directive; the
// orchestrator synthesizes the prompt from the seeded bead.
func TestExecuteBeadResolvesPathStyleSpecID(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	// Path-style spec target: a file at workflows/README.md with NO docgraph
	// id frontmatter, so ResolveGoverningRefs must fall through to the path
	// resolver rather than matching against docgraph.BuildGraphWithConfig.
	specPath := filepath.Join(workDir, "workflows", "README.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(specPath), 0o755))
	require.NoError(t, os.WriteFile(specPath, []byte("# Workflow\n"), 0o644))
	runGit("add", "workflows/README.md")
	runGit("commit", "-m", "docs: add workflow readme")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:          "path-bead",
		Title:       "Resolve path style spec ids",
		Status:      bead.StatusOpen,
		Priority:    0,
		IssueType:   bead.DefaultType,
		Description: "Make sure path-style spec-ids resolve to the on-disk file.",
		Acceptance:  "Governing reference carries the relative path unchanged.",
		Extra:       map[string]any{"spec-id": "workflows/README.md"},
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Script harness directive: no-op. The assertion target is the synthesized
	// prompt file, not any worker output.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte("no-op\n"), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "path-bead", "--json",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	require.NotEmpty(t, res.PromptFile)
	promptPath := filepath.Join(workDir, filepath.FromSlash(res.PromptFile))
	require.FileExists(t, promptPath)
	promptRaw, err := os.ReadFile(promptPath)
	require.NoError(t, err)
	promptText := string(promptRaw)
	// The path-style spec-id must appear unchanged in the synthesized prompt
	// (both as ID and Path — ResolveGoverningRefs sets them equal for path
	// resolution). The bead title and description round out the synthesis
	// assertions so a regression in prompt building is also caught here.
	assert.Contains(t, promptText, "workflows/README.md")
	assert.Contains(t, promptText, "Resolve path style spec ids")
	assert.Contains(t, promptText, "Make sure path-style spec-ids resolve to the on-disk file.")
	assert.Contains(t, promptText, "Governing reference carries the relative path unchanged.")
}

// TestExecuteBeadWritesResultArtifactBundle exercises real git + RealLandingGitOps
// via the script harness — no fakes for the components under test. The script
// harness emits a single real commit; the orchestrator synthesizes the prompt
// from the seeded bead and writes the manifest/result bundle. Assertions target
// the on-disk artifact bundle and worktree cleanup, not the fake's bookkeeping.
func TestExecuteBeadWritesResultArtifactBundle(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:         "my-bead",
		Title:      "Record execution artifacts",
		Status:     bead.StatusOpen,
		Priority:   0,
		IssueType:  bead.DefaultType,
		Acceptance: "Artifacts are written for later inspection.",
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")

	// Directive drives the real script harness to emit one real commit so the
	// result bundle records a non-trivial execution, replacing fakeAgentRunner.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"create-file artifact.txt artifact-content\n"+
			"commit feat: add artifact\n",
	), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	t.Setenv("DDX_WORKER_ID", "worker-test")

	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	manifestPath := filepath.Join(workDir, filepath.FromSlash(res.ManifestFile))
	resultPath := filepath.Join(workDir, filepath.FromSlash(res.ResultFile))
	require.FileExists(t, manifestPath)
	require.FileExists(t, resultPath)

	manifestRaw, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Contains(t, string(manifestRaw), `"bead_id": "my-bead"`)
	assert.Contains(t, string(manifestRaw), `"worker_id": "worker-test"`)
	assert.Contains(t, string(manifestRaw), `"prompt": "synthesized"`)
	// Worktree path is under $TMPDIR/ddx-exec-wt/ so test runs inside the
	// worktree don't corrupt the parent repo via GIT_DIR inheritance. The
	// leaf name still starts with .execute-bead-wt- for orphan recovery
	// via git worktree list.
	assert.Contains(t, string(manifestRaw), agent.ExecuteBeadWtPrefix+"my-bead-")

	// Extract the worktree path from the manifest to assert cleanup.
	var manifest struct {
		Paths struct {
			Worktree string `json:"worktree"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(manifestRaw, &manifest))
	require.NotEmpty(t, manifest.Paths.Worktree)

	resultRaw, err := os.ReadFile(resultPath)
	require.NoError(t, err)
	var recorded agent.ExecuteBeadResult
	require.NoError(t, json.Unmarshal(resultRaw, &recorded))
	assert.Equal(t, res.BeadID, recorded.BeadID)
	assert.Equal(t, "worker-test", recorded.WorkerID)
	assert.Equal(t, res.AttemptID, recorded.AttemptID)
	assert.Equal(t, res.Status, recorded.Status)
	assert.Equal(t, res.ResultFile, recorded.ResultFile)
	assert.NoDirExists(t, manifest.Paths.Worktree,
		"execute-bead worktree must be removed after the run")
}

// TestExecuteBeadFromRevFlag verifies that --from resolves a custom revision
// and uses it as the base for the worktree. Exercises real git + the script
// harness: the --from tag points to an earlier commit than HEAD, and the
// script directive is a no-op so we avoid merge logic. BaseRev in the result
// must equal the SHA that the custom ref resolves to.
func TestExecuteBeadFromRevFlag(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test --from flag resolves custom revision",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")

	// Tag this commit as the custom ref --from will resolve against. The
	// worktree snapshot at this tag contains the bead. HEAD will advance past
	// this commit below, so BaseRev != HEAD unless --from is honored.
	customSHA := runGit("rev-parse", "HEAD")
	runGit("tag", "custom-rev")

	// Advance HEAD past the custom ref with an unrelated commit so --from is
	// observable: if it were ignored, BaseRev would equal the newer HEAD.
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "after.txt"), []byte("after\n"), 0o644))
	runGit("add", "after.txt")
	runGit("commit", "-m", "chore: after tag")

	// no-op directive: agent makes no commits, so no merge logic is triggered.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte("no-op\n"), 0o644))

	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})
	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--from", "custom-rev",
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	assert.Equal(t, customSHA, res.BaseRev, "BaseRev must equal SHA resolved from --from tag")
}

// TestExecuteBeadOrphanRecovery verifies that worktrees matching the bead's
// prefix are cleaned up at the start of a new run. Exercises real git: an
// orphan worktree is registered via `git worktree add` under a basename
// matching ExecuteBeadWtPrefix+"my-bead-"; after execute-bead runs, the
// orphan must no longer be listed by `git worktree list` and its directory
// must be gone from disk.
func TestExecuteBeadOrphanRecovery(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test orphan recovery",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")

	// Register a real orphan worktree whose basename matches the
	// execute-bead prefix for "my-bead". RecoverOrphans discovers it via
	// `git worktree list` and must remove it before the new attempt runs.
	orphanPath := filepath.Join(workDir, ".ddx", agent.ExecuteBeadWtPrefix+"my-bead-old-attempt")
	require.NoError(t, os.MkdirAll(filepath.Dir(orphanPath), 0o755))
	runGit("worktree", "add", "--detach", orphanPath, "HEAD")
	require.DirExists(t, orphanPath, "orphan worktree must exist before execute-bead runs")

	// no-op directive: agent makes no commits, so the new attempt produces
	// a no-changes outcome and doesn't interfere with the orphan assertion.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte("no-op\n"), 0o644))

	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})
	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "output: %s", out)

	// The orphan worktree must be gone from disk and from git's worktree list.
	assert.NoDirExists(t, orphanPath,
		"orphan worktree directory should be removed before the new run")
	wtList := runGit("worktree", "list", "--porcelain")
	assert.NotContains(t, wtList, orphanPath,
		"orphan worktree should be unregistered from git worktree list")
}

// TestExecuteBeadHarnessNoiseNotSynthesized verifies that when the agent makes no
// real commits but the worktree is dirty with only harness bookkeeping files
// (e.g. .ddx/executions/<attempt>/embedded/*), SynthesizeCommit returns
// (false, nil) and the outcome is "no-changes", not "merged" or "success".
// ResultRev must equal BaseRev. Exercises real git + script harness: the
// directive writes a noise file under the embedded/ pathspec that
// RealGitOps.SynthesizeCommit excludes, so IsDirty=true but nothing stages
// and no iteration commit lands on main.
func TestExecuteBeadHarnessNoiseNotSynthesized(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test harness-noise no-synthesis",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")

	// Capture main HEAD before the run so we can assert that no iteration
	// commit or merge advanced main (the fake's mergeCalls==0 invariant).
	mainBefore := runGit("rev-parse", "HEAD")

	// Directive writes a noise file under .claude/skills, which
	// RealGitOps.synthesizeCommitExcludePathspecs excludes from staging.
	// IsDirty(wtPath) reports true (untracked file in worktree), but
	// SynthesizeCommit stages nothing and returns (false, nil) — the
	// all-noise worktree case.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"create-file .claude/skills/noise.md harness-noise-content\n",
	), 0o644))

	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})
	res := runExecuteBead(t, f, nil, "my-bead",
		"--harness", "script", "--model", directivePath)

	assert.Equal(t, "no-changes", res.Outcome, "harness-noise-only dirty worktree must not produce a synthesis commit")
	assert.Equal(t, agent.ExecuteBeadStatusNoChanges, res.Status)
	assert.NotEmpty(t, res.BaseRev, "BaseRev must be set to real HEAD SHA")
	assert.Equal(t, res.BaseRev, res.ResultRev, "ResultRev must equal BaseRev when no real commit was made")

	// merge must not be called when outcome is no-changes: main HEAD must
	// not advance and no merge commit must land.
	mainAfter := runGit("rev-parse", "HEAD")
	assert.Equal(t, mainBefore, mainAfter, "main HEAD must not advance when outcome is no-changes")
	mergeLog := runGit("log", "--merges", "--format=%H", mainBefore+"..HEAD")
	assert.Empty(t, mergeLog, "no merge commit must land when outcome is no-changes")
}

// TestExecuteBeadAgentErrorNoCommits verifies that when the agent runner returns
// an error and makes no commits, the outcome is an execution error rather than
// a misleading no-change result. Exercises real git + script harness: the
// directive forces a synthetic script-harness failure at the first step, so the
// runner returns a non-nil error with no commits, and the orchestrator classifies
// the landing as outcome="error" / status=execution_failed.
func TestExecuteBeadAgentErrorNoCommits(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test agent error no commits",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Directive triggers a synthetic script-harness failure at directive 0.
	// The runner returns (result, err) with a non-nil error and no commits
	// made in the worker worktree, replacing fakeAgentRunner's canned error.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte("fail-during 0\n"), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	res := runExecuteBead(t, f, nil, "my-bead",
		"--harness", "script", "--model", directivePath)

	assert.Equal(t, 1, res.ExitCode)
	assert.Equal(t, "error", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
	assert.NotEmpty(t, res.Error, "Error must carry the agent failure message")
	assert.Contains(t, res.Error, "script harness",
		"Error should carry the script-harness failure message")
	assert.Equal(t, res.Error, res.Reason,
		"Reason must mirror Error when the agent failed with no commits")
	assert.Equal(t, baseSHA, res.BaseRev)
	assert.Empty(t, res.PreserveRef)

	// Real-git assertion: no commits made, main must not advance.
	assert.Equal(t, baseSHA, runGit("rev-parse", "HEAD"),
		"main HEAD must not advance when the agent failed with no commits")
}

// TestExecuteBeadTimeoutNoCommitsReportsExecutionFailure verifies that when the
// agent exits with a negative status (canonical timeout shape: ExitCode=-1 with
// a non-empty Error) and makes no commits, the outcome is an execution error
// rather than a misleading no-change result. Exercises real git + script
// harness: the directive first pins exit=-1, then triggers a synthetic harness
// failure, producing (result{ExitCode:-1}, err!=nil) with no commits in the
// worker worktree — the same observable shape the old fakeAgentRunner canned
// Result struct used to fabricate.
func TestExecuteBeadTimeoutNoCommitsReportsExecutionFailure(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test timeout no commits",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Pin ExitCode=-1 (canonical timeout shape) and then trigger a synthetic
	// failure. The script harness emits a result with ExitCode=-1 and a
	// non-empty Error, without making any commits in the worker worktree —
	// replacing the fakeAgentRunner's canned Result{ExitCode:-1, Error:"timeout after 5m"}.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte("set-exit -1\nfail-during 1\n"), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	res := runExecuteBead(t, f, nil, "my-bead",
		"--harness", "script", "--model", directivePath)

	assert.Equal(t, -1, res.ExitCode)
	assert.Equal(t, "error", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
	assert.NotEmpty(t, res.Error, "Error must carry the harness failure message")
	assert.Contains(t, res.Error, "script harness",
		"Error should carry the script-harness failure message")
	assert.Equal(t, res.Error, res.Reason,
		"Reason must mirror Error when the agent failed with no commits")
	assert.Equal(t, baseSHA, res.BaseRev)
	assert.Equal(t, baseSHA, res.ResultRev,
		"ResultRev must equal BaseRev when no commits were made")
	assert.Empty(t, res.PreserveRef)

	// Real-git assertion: no commits made, main must not advance.
	assert.Equal(t, baseSHA, runGit("rev-parse", "HEAD"),
		"main HEAD must not advance when the agent timed out with no commits")
}

// TestExecuteBeadAgentErrorWithCommitsPreservesBeforeLand verifies that a
// non-zero agent result preserves the iteration instead of touching the target
// branch, even when a merge would have succeeded cleanly. Exercises real git +
// RealLandingGitOps via the script harness — no fakes for the components
// under test. The script harness produces a real commit, then trips a
// synthetic failure so the runner returns an error alongside the commit;
// the orchestrator must preserve without attempting a merge.
func TestExecuteBeadAgentErrorWithCommitsPreservesBeforeLand(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead agent error with commits preserves before land",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Script harness: create a real commit in the worker worktree, then
	// trip a synthetic failure so runScriptFn returns (result, execErr).
	// This mimics the original fakeAgentRunner{err: ..., result: nil} case
	// — runner errored after commits were already made. The merge would
	// have fast-forwarded cleanly (no sibling advance), so the test proves
	// execute-bead preserves based on the error signal alone, not on any
	// merge-side failure.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"create-file crashfile.txt pre-crash-content\n"+
			"commit feat: partial work before crash\n"+
			"fail-during 2\n",
	), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	assert.Equal(t, 1, res.ExitCode)
	assert.Equal(t, "preserved", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
	assert.Equal(t, baseSHA, res.BaseRev)
	assert.NotEmpty(t, res.ResultRev)
	assert.NotEqual(t, baseSHA, res.ResultRev, "worker must have produced a commit before the failure")
	assert.NotEmpty(t, res.PreserveRef)

	// Hidden ref must resolve to the worker's commit, proving it was
	// preserved rather than discarded.
	preservedSHA := runGit("rev-parse", res.PreserveRef)
	assert.Equal(t, res.ResultRev, preservedSHA, "preserve ref must resolve to the worker commit")

	// Target branch must NOT advance — an agent error, even with commits
	// and a clean would-have-succeeded merge, must preserve before land.
	// (Pre-refactor equivalent: assert.Equal(t, 0, git.mergeCalls).)
	assert.Equal(t, baseSHA, runGit("rev-parse", "refs/heads/main"),
		"main must be unchanged when agent errors with commits")
	mergeCommits := runGit("log", "--merges", "--format=%H", "refs/heads/main")
	assert.Empty(t, mergeCommits, "agent-error path must not produce a merge commit on main")
}

// TestExecuteBeadAgentErrorWithCommitsPreserves verifies that when the agent
// runner returns an error and commits exist, exitCode=1 and outcome="preserved"
// with a non-empty, correctly-formatted preserve ref. Exercises real git +
// RealLandingGitOps via the script harness — no fakes for the components under
// test.
func TestExecuteBeadAgentErrorWithCommitsPreserves(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead agent error with commits preserves",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Script harness: create a real commit in the worker worktree, then
	// trip a synthetic failure so runScriptFn returns (result, execErr).
	// Mirrors the original fakeAgentRunner{err: ..., result: nil} with
	// commits present.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"create-file crashfile.txt pre-crash-content\n"+
			"commit feat: partial work before crash\n"+
			"fail-during 2\n",
	), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	assert.Equal(t, 1, res.ExitCode)
	assert.Equal(t, "preserved", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
	assert.NotEmpty(t, res.ResultRev)
	assert.NotEqual(t, baseSHA, res.ResultRev, "worker must have produced a commit before the failure")
	assert.NotEmpty(t, res.PreserveRef)
	assertPreserveRef(t, res.PreserveRef, "my-bead", baseSHA)

	// Preserve ref must resolve to the worker's commit.
	preservedSHA := runGit("rev-parse", res.PreserveRef)
	assert.Equal(t, res.ResultRev, preservedSHA, "preserve ref must resolve to the worker commit")
}

// TestExecuteBeadAgentErrorMessageInOutput verifies that when the agent runner
// returns an error, the error message appears in the JSON output Error field.
// Exercises real git + RealLandingGitOps via the script harness — no fakes for
// the components under test.
func TestExecuteBeadAgentErrorMessageInOutput(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead agent error message in output",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Script harness: fail immediately on the first directive so the runner
	// returns (result, execErr) with no commits made. Mirrors the original
	// fakeAgentRunner{err: ..., result: nil} with no commits.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"fail-during 0\n",
	), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	root := f.NewRootCommand()
	out, _ := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)
	res := parseExecuteBeadJSON(t, out)

	assert.Equal(t, 1, res.ExitCode)
	// Agent runner's error message must appear in the JSON Error field.
	assert.Equal(t, "script harness: synthetic failure at directive 0", res.Error)
}

// TestExecuteBeadHeadRevFailure verifies that when HeadRev fails after the agent
// runs, the outcome is "error" and the reason contains the original error message.
// This covers the path at agent_execute_bead.go lines 282-309.
//
// Exercises real git + RealLandingGitOps via the script harness — no fakes for
// the components under test. The script directive deletes the worktree's .git
// pointer file as its only action, so the agent reports success but the
// post-agent `git rev-parse HEAD` against the worktree fails for real.
func TestExecuteBeadHeadRevFailure(t *testing.T) {
	setup := func(t *testing.T) (workDir, baseSHA, directivePath string) {
		t.Helper()
		workDir = t.TempDir()

		scrubEnv := func() []string {
			parent := os.Environ()
			env := make([]string, 0, len(parent))
			for _, kv := range parent {
				if strings.HasPrefix(kv, "GIT_") {
					continue
				}
				env = append(env, kv)
			}
			return env
		}
		runGit := func(args ...string) string {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = workDir
			cmd.Env = scrubEnv()
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "git %v: %s", args, string(out))
			return strings.TrimSpace(string(out))
		}

		runGit("init", "-b", "main")
		runGit("config", "user.email", "test@ddx.test")
		runGit("config", "user.name", "DDx Test")
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
		runGit("add", ".")
		runGit("commit", "-m", "chore: initial seed")

		seedExecuteBead(t, workDir, &bead.Bead{
			ID:        "my-bead",
			Title:     "Test execute-bead HeadRev failure",
			Status:    bead.StatusOpen,
			IssueType: bead.DefaultType,
		})
		runGit("add", ".ddx/beads.jsonl")
		runGit("commit", "-m", "chore: seed bead")
		baseSHA = runGit("rev-parse", "HEAD")

		// Script directive: delete the worktree's .git pointer file. The agent
		// itself reports success (no error directive), but the post-agent
		// `git rev-parse HEAD` against the worktree must fail because there is
		// no longer a discoverable git directory at that path. Mirrors the
		// original fakeExecuteBeadGit{wtHeadRevErr: ...} contract using real
		// git instead of a hand-rolled error string.
		directivePath = filepath.Join(t.TempDir(), "directives.txt")
		require.NoError(t, os.WriteFile(directivePath, []byte(
			"delete-file .git\n",
		), 0o644))
		return
	}

	t.Run("json output", func(t *testing.T) {
		workDir, baseSHA, directivePath := setup(t)

		// Real runner + real git ops: no overrides that fake the thing under test.
		f := NewCommandFactory(workDir)
		f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

		root := f.NewRootCommand()
		out, cmdErr := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
			"--from", baseSHA,
			"--harness", "script", "--model", directivePath)
		require.Error(t, cmdErr)
		res := parseExecuteBeadJSON(t, out)

		assert.Equal(t, "error", res.Outcome)
		assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
		assert.Contains(t, res.Reason, "rev-parse",
			"Reason must reflect the real git rev-parse failure on the corrupted worktree")
		assert.Equal(t, 1, res.ExitCode)
	})

	t.Run("text output", func(t *testing.T) {
		workDir, baseSHA, directivePath := setup(t)

		f := NewCommandFactory(workDir)
		f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

		root := f.NewRootCommand()
		out, cmdErr := executeCommand(root, "agent", "execute-bead",
			"my-bead",
			"--from", baseSHA,
			"--harness", "script", "--model", directivePath)
		require.Error(t, cmdErr)

		assert.Contains(t, out, "outcome: error")
		assert.Contains(t, out, "rev-parse")
	})
}

// TestExecuteBeadCompoundErrorAgentAndHeadRevFailure verifies that when the
// agent runner returns an error AND HeadRev fails on the worktree, both the
// Error field (agent message) and the Reason field (rev error) are present in
// the JSON output. This covers the path at agent_execute_bead.go that
// previously dropped the agent error message when revErr was non-nil.
//
// Exercises real git + RealLandingGitOps via the script harness — no fakes
// for the components under test. The script directive first deletes the
// worktree's .git pointer (so the post-agent `git rev-parse HEAD` fails for
// real) and then triggers a synthetic agent failure, producing the compound
// error the test asserts against.
func TestExecuteBeadCompoundErrorAgentAndHeadRevFailure(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead compound agent and HeadRev failure",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Script directives:
	//   directive 0 — delete-file .git: corrupts the worktree so post-agent
	//                 `git rev-parse HEAD` fails for real.
	//   directive 1 — fail-during 1: returns an agent-level error to the
	//                 worker, populating res.Error.
	// Together these reproduce the compound (agentErr + revErr) path without
	// any fakes of the components under test.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"delete-file .git\n"+
			"fail-during 1\n",
	), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	root := f.NewRootCommand()
	out, cmdErr := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)
	require.Error(t, cmdErr)
	res := parseExecuteBeadJSON(t, out)

	assert.Equal(t, 1, res.ExitCode)
	assert.Equal(t, "error", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
	assert.Contains(t, res.Error, "synthetic failure at directive 1",
		"agent error message must be preserved even when HeadRev also fails")
	assert.Contains(t, res.Reason, "rev-parse",
		"Reason must reflect the real git rev-parse failure on the corrupted worktree")
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
			workDir := t.TempDir()

			scrubEnv := func() []string {
				parent := os.Environ()
				env := make([]string, 0, len(parent))
				for _, kv := range parent {
					if strings.HasPrefix(kv, "GIT_") {
						continue
					}
					env = append(env, kv)
				}
				return env
			}
			runGit := func(args ...string) string {
				t.Helper()
				cmd := exec.Command("git", args...)
				cmd.Dir = workDir
				cmd.Env = scrubEnv()
				out, err := cmd.CombinedOutput()
				require.NoError(t, err, "git %v: %s", args, string(out))
				return strings.TrimSpace(string(out))
			}

			runGit("init", "-b", "main")
			runGit("config", "user.email", "test@ddx.test")
			runGit("config", "user.name", "DDx Test")
			require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
			runGit("add", ".")
			runGit("commit", "-m", "chore: initial seed")
			baseSHA := runGit("rev-parse", "HEAD")

			// Real runner + real git ops: no overrides that fake the thing
			// under test. The script harness would be driven by a per-attempt
			// directive file if execution reached the agent, but the invalid
			// bead ID must be rejected before any git or agent work occurs.
			directivePath := filepath.Join(t.TempDir(), "directives.txt")
			require.NoError(t, os.WriteFile(directivePath, []byte(""), 0o644))

			f := NewCommandFactory(workDir)
			f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

			root := f.NewRootCommand()
			_, cmdErr := executeCommand(root, "agent", "execute-bead", id,
				"--harness", "script", "--model", directivePath)
			require.Error(t, cmdErr)
			assert.Contains(t, cmdErr.Error(), "invalid bead ID")

			// No git or agent operations should have been attempted: no
			// execute-bead worktree was created, and main did not advance.
			wtList := runGit("worktree", "list", "--porcelain")
			assert.NotContains(t, wtList, agent.ExecuteBeadWtPrefix,
				"no worktree should be created for invalid bead ID")
			assert.Equal(t, baseSHA, runGit("rev-parse", "HEAD"),
				"main must not advance when the bead ID is rejected")
		})
	}
}

// TestExecuteBeadEvidenceFields verifies that runtime evidence fields are
// populated in the JSON output. Exercises real git plumbing + the script
// harness so the fields are filled by the real runner, not a canned Result.
func TestExecuteBeadEvidenceFields(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead evidence fields",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"create-file evidence.txt evidence-content\n"+
			"commit feat: add evidence\n",
	), 0o644))

	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	root := f.NewRootCommand()
	out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
		"--harness", "script", "--model", directivePath)
	require.NoError(t, err, "execute-bead should not return an error; output: %s", out)
	res := parseExecuteBeadJSON(t, out)

	assert.Equal(t, "script", res.Harness)
	assert.Equal(t, directivePath, res.Model)
	assert.Equal(t, 0, res.Tokens, "script harness reports no token usage")
	assert.Equal(t, 0.0, res.CostUSD, "script harness reports no cost")
	assert.NotEmpty(t, res.SessionID)
	assert.False(t, res.StartedAt.IsZero())
	assert.False(t, res.FinishedAt.IsZero())
	assert.Equal(t, baseSHA, res.BaseRev)
	assert.NotEmpty(t, res.ResultRev)
	assert.NotEqual(t, res.BaseRev, res.ResultRev, "agent commit must produce a distinct ResultRev")
}

// modelPassthroughCapture is a passthrough wrapper around an agent.AgentRunner
// that records the most recent RunOptions before forwarding to the underlying
// runner. It instruments TestExecuteBeadModelFlagPassthrough so the test can
// assert what ExecuteBead handed the runner without itself faking the runner —
// every Run call is forwarded to the real script harness so production routing
// semantics still execute end-to-end.
type modelPassthroughCapture struct {
	inner agent.AgentRunner
	last  agent.RunOptions
}

func (c *modelPassthroughCapture) Run(opts agent.RunOptions) (*agent.Result, error) {
	c.last = opts
	return c.inner.Run(opts)
}

// TestExecuteBeadModelFlagPassthrough locks in the resolution contract for
// execute-bead's model option: the value supplied via ExecuteBeadOptions.Model
// is passed verbatim to the runner, and an empty value is not silently replaced
// by any hardcoded or catalog-derived default. This regression test guards
// against routing layers injecting a model (e.g. a stale vendor/model like
// "z-ai/glm-5.1") when the caller did not request one — the case the agent
// harness resolves from ~/.config/agent/config.yaml must be preserved by
// ExecuteBead handing the runner an empty Model so the harness's own
// resolution chain runs.
//
// The test runs against a real git repo and the script harness (wrapped in a
// passthrough capture so the test can observe opts.Model). The script harness
// resolves its directive file from opts.PromptFile when opts.Model is empty or
// not a readable file, so both the empty-model and the explicit-non-path-model
// subtests exercise the real runner end-to-end.
func TestExecuteBeadModelFlagPassthrough(t *testing.T) {
	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	setupRepo := func(t *testing.T) string {
		t.Helper()
		workDir := t.TempDir()
		runGit := func(args ...string) {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = workDir
			cmd.Env = scrubEnv()
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "git %v: %s", args, string(out))
		}
		runGit("init", "-b", "main")
		runGit("config", "user.email", "test@ddx.test")
		runGit("config", "user.name", "DDx Test")
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
		runGit("add", ".")
		runGit("commit", "-m", "chore: initial seed")
		seedExecuteBead(t, workDir, &bead.Bead{
			ID:        "my-bead",
			Title:     "Test execute-bead model passthrough",
			Status:    bead.StatusOpen,
			IssueType: bead.DefaultType,
		})
		runGit("add", ".ddx/beads.jsonl")
		runGit("commit", "-m", "chore: seed bead")
		return workDir
	}

	t.Run("empty model stays empty through ExecuteBead", func(t *testing.T) {
		workDir := setupRepo(t)
		// PromptFile fallback drives the script harness when --model is empty.
		// A no-op directive file at PromptFile would require routing internals
		// the test does not control; instead, supply a directive via PromptFile
		// override on the command line so the script harness has something
		// parseable. The capture still records opts.Model="" because that is
		// what ExecuteBead forwarded.
		directivePath := filepath.Join(t.TempDir(), "directives.txt")
		require.NoError(t, os.WriteFile(directivePath, []byte("no-op\n"), 0o644))

		capture := &modelPassthroughCapture{inner: agent.NewRunner(agent.Config{})}
		f := NewCommandFactory(workDir)
		f.AgentRunnerOverride = capture

		root := f.NewRootCommand()
		// No --model flag supplied. --prompt-file feeds the script harness.
		out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
			"--harness", "script", "--prompt", directivePath)
		require.NoError(t, err, "execute-bead should not return an error; output: %s", out)

		assert.Equal(t, "", capture.last.Model,
			"runner must receive an empty Model when no --model flag is provided; "+
				"any non-empty value here indicates a routing layer injected a default, "+
				"which would override the harness's own config-driven resolution")
	})

	t.Run("explicit model is forwarded verbatim", func(t *testing.T) {
		workDir := setupRepo(t)
		// Pass a non-path string as --model. The script harness sees Model is
		// not a readable file and falls back to PromptFile for directives.
		directivePath := filepath.Join(t.TempDir(), "directives.txt")
		require.NoError(t, os.WriteFile(directivePath, []byte("no-op\n"), 0o644))

		capture := &modelPassthroughCapture{inner: agent.NewRunner(agent.Config{})}
		f := NewCommandFactory(workDir)
		f.AgentRunnerOverride = capture

		root := f.NewRootCommand()
		out, err := executeCommand(root, "agent", "execute-bead", "my-bead", "--json",
			"--harness", "script", "--model", "qwen3.5-27b",
			"--prompt", directivePath)
		require.NoError(t, err, "execute-bead should not return an error; output: %s", out)

		assert.Equal(t, "qwen3.5-27b", capture.last.Model,
			"runner must receive the exact --model value the caller passed")
	})
}

// TestExecuteBeadStatusMapping exercises the real (agent, orchestrator, Land)
// stack through the script harness for every status the classifier produces.
// Each subtest sets up an isolated real git repo, drives the script harness
// with a per-attempt directive file that encodes the scenario (no commits,
// dirty worktree, failing exit, conflicting sibling), and asserts that the
// Status written onto the ExecuteBeadResult by ApplyLandingToResult matches
// the expected supervisor-visible status.
//
// Migrated off fakeExecuteBeadGit / fakeAgentRunner per concerns.md §testing
// ("no mocks, period"; "never mock the thing you are testing"). Parent bead
// ddx-d9df348d.
func TestExecuteBeadStatusMapping(t *testing.T) {
	// setupRepo builds a real git repo with one seed commit, then commits a
	// single open bead on top so the execute-bead command can find it.
	// Returns (workDir, baseSHA, runGit).
	setupRepo := func(t *testing.T) (string, string, func(args ...string) string) {
		t.Helper()
		workDir := t.TempDir()

		scrubEnv := func() []string {
			parent := os.Environ()
			env := make([]string, 0, len(parent))
			for _, kv := range parent {
				if strings.HasPrefix(kv, "GIT_") {
					continue
				}
				env = append(env, kv)
			}
			return env
		}
		runGit := func(args ...string) string {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = workDir
			cmd.Env = scrubEnv()
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "git %v: %s", args, string(out))
			return strings.TrimSpace(string(out))
		}

		runGit("init", "-b", "main")
		runGit("config", "user.email", "test@ddx.test")
		runGit("config", "user.name", "DDx Test")
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
		runGit("add", ".")
		runGit("commit", "-m", "chore: initial seed")

		seedExecuteBead(t, workDir, &bead.Bead{
			ID:        "status-bead",
			Title:     "Test status mapping",
			Status:    bead.StatusOpen,
			IssueType: bead.DefaultType,
		})
		runGit("add", ".ddx/beads.jsonl")
		runGit("commit", "-m", "chore: seed bead")
		baseSHA := runGit("rev-parse", "HEAD")
		return workDir, baseSHA, runGit
	}

	// runWithDirective writes a directive file, invokes execute-bead with the
	// real agent runner and the script harness, and returns the parsed result.
	// No fake git or runner — the full ExecuteBead → LandBeadResult → Land
	// pipeline runs against the real repo.
	runWithDirective := func(t *testing.T, workDir, baseSHA, directives string) agent.ExecuteBeadResult {
		t.Helper()
		directivePath := filepath.Join(t.TempDir(), "directives.txt")
		require.NoError(t, os.WriteFile(directivePath, []byte(directives), 0o644))
		f := NewCommandFactory(workDir)
		f.AgentRunnerOverride = agent.NewRunner(agent.Config{})
		return runExecuteBead(t, f, nil, "status-bead",
			"--from", baseSHA,
			"--harness", "script", "--model", directivePath)
	}

	t.Run("merged success", func(t *testing.T) {
		workDir, baseSHA, _ := setupRepo(t)
		// Real commit + clean fast-forward → outcome "merged" → status success.
		res := runWithDirective(t, workDir, baseSHA,
			"create-file out.txt content\ncommit feat: add out\n")
		assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
		assert.NotEmpty(t, res.Detail)
	})

	t.Run("no changes stays non-success", func(t *testing.T) {
		workDir, baseSHA, _ := setupRepo(t)
		// Harness does nothing → worktree HEAD == BaseRev → outcome "no-changes".
		res := runWithDirective(t, workDir, baseSHA, "no-op\n")
		assert.Equal(t, agent.ExecuteBeadStatusNoChanges, res.Status)
		assert.NotEmpty(t, res.Detail)
	})

	t.Run("execution failure dominates preserved outcome", func(t *testing.T) {
		workDir, baseSHA, _ := setupRepo(t)
		// Real commit + non-zero exit: LandBeadResult preserves (commits
		// exist) but ClassifyExecuteBeadStatus must still report
		// execution_failed because ExitCode != 0 dominates.
		res := runWithDirective(t, workDir, baseSHA,
			"create-file out.txt content\ncommit feat: add out\nset-exit 1\n")
		assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
		assert.NotEmpty(t, res.Detail)
	})

	t.Run("error outcome stays execution failure", func(t *testing.T) {
		workDir, baseSHA, _ := setupRepo(t)
		// Pin ExitCode=-1 (canonical timeout shape), then trip a synthetic
		// harness failure before any commit is made. LandBeadResult maps
		// (ExitCode!=0, no commits) to outcome "error".
		res := runWithDirective(t, workDir, baseSHA,
			"set-exit -1\nfail-during 1\n")
		assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
		assert.NotEmpty(t, res.Detail)
	})

	t.Run("land conflict", func(t *testing.T) {
		workDir, baseSHA, runGit := setupRepo(t)
		// Advance main with a sibling edit to out.txt, then have the worker
		// commit a conflicting version starting from baseSHA. Real Land()
		// hits a merge conflict and preserves with reason "merge conflict".
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "out.txt"), []byte("sibling\n"), 0o644))
		runGit("add", "out.txt")
		runGit("commit", "-m", "feat: sibling conflicting change")
		res := runWithDirective(t, workDir, baseSHA,
			"create-file out.txt worker\ncommit feat: worker add out\n")
		assert.Equal(t, agent.ExecuteBeadStatusLandConflict, res.Status)
		assert.NotEmpty(t, res.Detail)
	})
}

// seedGateDocs writes a governing spec doc and a required execution gate doc
// into workDir/docs/ so they are copied into worktrees by fakeExecuteBeadGit.
func seedGateDocs(t *testing.T, workDir string, gateCommand []string) {
	t.Helper()
	docsDir := filepath.Join(workDir, "docs")
	require.NoError(t, os.MkdirAll(docsDir, 0o755))

	specDoc := "---\nddx:\n  id: FEAT-GATE-TEST\n---\n# Spec: Gate Test\n"
	require.NoError(t, os.WriteFile(filepath.Join(docsDir, "spec-gate-test.md"), []byte(specDoc), 0o644))

	var cmdYAML string
	for _, part := range gateCommand {
		cmdYAML += fmt.Sprintf("      - \"%s\"\n", part)
	}
	gateDoc := fmt.Sprintf("---\nddx:\n  id: EXEC-GATE-TEST\n  depends_on:\n    - FEAT-GATE-TEST\n  execution:\n    kind: command\n    required: true\n    command:\n%s---\n# Gate\n", cmdYAML)
	require.NoError(t, os.WriteFile(filepath.Join(docsDir, "exec-gate-test.md"), []byte(gateDoc), 0o644))
}

// TestExecuteBeadGatePass verifies that execute-bead evaluates required gates
// against an ephemeral worktree at ResultRev and merges when all gates pass.
// Wired by ddx-14c0e790 (interactive execute-bead gate eval).
//
// Migrated off fakeExecuteBeadGit / fakeAgentRunner per concerns.md §testing
// ("no mocks, period"; "never mock the thing you are testing"). Exercises the
// real ExecuteBead → LandBeadResult → Land pipeline against an isolated real
// git repo, with the script harness driving an actual commit via a per-attempt
// directive file. Parent: ddx-d9df348d.
func TestExecuteBeadGatePass(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "gate-bead",
		Title:     "Bead with required gate",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
		Extra:     map[string]any{"spec-id": "FEAT-GATE-TEST"},
	})
	// Gate docs must be committed so the real worker worktree (created at
	// baseSHA) sees them; the fake git path copied them in-process, but real
	// git only materializes tracked content.
	seedGateDocs(t, workDir, []string{"true"})
	runGit("add", ".ddx/beads.jsonl", "docs")
	runGit("commit", "-m", "chore: seed bead and gate docs")
	baseSHA := runGit("rev-parse", "HEAD")

	// Directive file drives the script harness to make one real commit in the
	// worker worktree — replaces fakeAgentRunner's canned Result struct.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"create-file out.txt content\n"+
			"commit feat: add out\n",
	), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	res := runExecuteBead(t, f, nil, "gate-bead",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)

	assert.Equal(t, "merged", res.Outcome)
	require.Len(t, res.GateResults, 1, "required gate must be evaluated")
	assert.Equal(t, "pass", res.GateResults[0].Status)
}

// TestExecuteBeadGateBlocksLanding verifies that execute-bead preserves the
// result instead of merging when a required gate fails.
// Wired by ddx-14c0e790 (interactive execute-bead gate eval).
//
// Migrated off fakeExecuteBeadGit / fakeAgentRunner per concerns.md §testing
// ("no mocks, period"; "never mock the thing you are testing"). Exercises the
// real ExecuteBead → LandBeadResult → Land pipeline against an isolated real
// git repo, with the script harness driving an actual commit via a per-attempt
// directive file. Parent: ddx-d9df348d.
func TestExecuteBeadGateBlocksLanding(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "gate-bead-fail",
		Title:     "Bead with failing gate",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
		Extra:     map[string]any{"spec-id": "FEAT-GATE-TEST"},
	})
	// Gate docs must be committed so the real worker worktree (created at
	// baseSHA) sees them; the fake git path copied them in-process, but real
	// git only materializes tracked content.
	seedGateDocs(t, workDir, []string{"false"})
	runGit("add", ".ddx/beads.jsonl", "docs")
	runGit("commit", "-m", "chore: seed bead and gate docs")
	baseSHA := runGit("rev-parse", "HEAD")

	// Directive file drives the script harness to make one real commit in the
	// worker worktree — replaces fakeAgentRunner's canned Result struct.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte(
		"create-file out.txt content\n"+
			"commit feat: add out\n",
	), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	res := runExecuteBead(t, f, nil, "gate-bead-fail",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)

	assert.Equal(t, "preserved", res.Outcome, "failing required gate must preserve")
	require.Len(t, res.GateResults, 1)
	assert.Equal(t, "fail", res.GateResults[0].Status)
	assert.NotEmpty(t, res.PreserveRef, "failed-gate landing must preserve under refs/ddx/iterations")
}

// TestExecuteBeadNoGatesWhenNoChanges verifies that gates are not evaluated
// when the agent produces no changes (resultRev == baseRev).
//
// Migrated off fakeExecuteBeadGit / fakeAgentRunner per concerns.md §testing
// ("no mocks, period"; "never mock the thing you are testing"). Exercises the
// real ExecuteBead → LandBeadResult → Land pipeline against an isolated real
// git repo, with the script harness driven by a per-attempt directive file
// that emits zero commits — the real no-changes signal. Parent: ddx-d9df348d.
func TestExecuteBeadNoGatesWhenNoChanges(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "gate-bead-nochange",
		Title:     "Bead with gate but no changes",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
		Extra:     map[string]any{"spec-id": "FEAT-GATE-TEST"},
	})
	// Gate docs must be committed so the real worker worktree (created at
	// baseSHA) sees them.
	seedGateDocs(t, workDir, []string{"false"})
	runGit("add", ".ddx/beads.jsonl", "docs")
	runGit("commit", "-m", "chore: seed bead and gate docs")
	baseSHA := runGit("rev-parse", "HEAD")

	// Directive file drives the script harness to emit no commits — the real
	// signal that the agent produced no changes. Replaces fakeAgentRunner's
	// canned Result struct + mainHeadRev==wtHeadRev.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte("no-op\n"), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	res := runExecuteBead(t, f, nil, "gate-bead-nochange",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)

	assert.Equal(t, "no-changes", res.Outcome)
	assert.Empty(t, res.GateResults, "gates must not run when agent made no changes")
}

// TestExecuteBeadEmbeddedAgentStateRedirected verifies that when execute-bead
// invokes the embedded-agent harness, its session/telemetry runtime state is
// redirected into a DDx-owned directory inside the execution bundle instead
// of being written at the worktree root. Regression guard for ddx-cba2dc64.
//
// Migrated off fakeExecuteBeadGit / fakeAgentRunner per concerns.md §testing
// ("no mocks, period"; "never mock the thing you are testing"). Drives the
// real ExecuteBead → LandBeadResult → Land pipeline against an isolated
// real-git repo. A script-harness directive file simulates the embedded
// agent: it records worktree-root contents before and after writing runtime
// state, and writes that state to $DDX_SESSION_LOG_DIR — which the script
// harness interpolates from opts.SessionLogDir. If execute-bead's wiring is
// broken and SessionLogDir is empty or points at the worktree root, the
// assertions below fail. Parent: ddx-d9df348d.
func TestExecuteBeadEmbeddedAgentStateRedirected(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "embedded-state-bead",
		Title:     "Bead for embedded-state redirection test",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Out-of-worktree record dir: the directive writes `ls -A` of the worktree
	// root to before.txt/after.txt, bracketing the simulated embedded-agent
	// write, so the test can compare worktree-root contents mid-run even
	// though the worktree is cleaned up after ExecuteBead returns.
	recordDir := t.TempDir()
	beforePath := filepath.Join(recordDir, "before.txt")
	afterPath := filepath.Join(recordDir, "after.txt")

	simulatedSessionFile := "agent-embedded-session.jsonl"
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	directive := strings.Join([]string{
		// Force a result commit so ExecuteBead lands "merged".
		"create-file feature.txt hello",
		`commit "feat: embedded redirect hello"`,
		// Capture worktree-root listing before simulated embedded writes.
		"run ls -A > " + beforePath,
		// Simulate the embedded-agent harness writing runtime state to
		// $DDX_SESSION_LOG_DIR (interpolated by the script harness from
		// opts.SessionLogDir). Empty/unset SessionLogDir collapses to
		// writing at "/" which sh will reject — any wiring regression is
		// caught by the bundle/embedded existence check below.
		`run mkdir -p "$DDX_SESSION_LOG_DIR" && printf '{"event":"started"}\n' > "$DDX_SESSION_LOG_DIR/` + simulatedSessionFile + `"`,
		// Capture worktree-root listing after simulated embedded writes.
		"run ls -A > " + afterPath,
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(directivePath, []byte(directive), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	res := runExecuteBead(t, f, nil, "embedded-state-bead",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)
	require.Equal(t, "merged", res.Outcome, "execute-bead should succeed for this test")

	// The execution bundle must exist and contain an embedded/ subdirectory —
	// the concrete effect of runOpts.SessionLogDir being set to
	// <bundle>/embedded in execute_bead.go.
	require.NotEmpty(t, res.ExecutionDir, "execute-bead must record an execution bundle dir")
	bundleAbs, err := filepath.Abs(filepath.Join(workDir, filepath.FromSlash(res.ExecutionDir)))
	require.NoError(t, err)
	embeddedDir := filepath.Join(bundleAbs, "embedded")
	info, statErr := os.Stat(embeddedDir)
	require.NoError(t, statErr, "execution bundle must contain an embedded/ subdir")
	require.True(t, info.IsDir(), "embedded path must be a directory")
	assert.Equal(t, "embedded", filepath.Base(embeddedDir),
		"SessionLogDir must be the bundle's embedded/ subdirectory")

	// The simulated embedded-agent session file must exist under <bundle>/embedded
	// — proves runOpts.SessionLogDir propagated to the harness and pointed there.
	assert.FileExists(t, filepath.Join(embeddedDir, simulatedSessionFile))

	// Worktree-root listings captured mid-run must match: the simulated
	// embedded write must not have leaked any new entry into the worktree root.
	beforeRaw, err := os.ReadFile(beforePath)
	require.NoError(t, err, "before-run worktree listing must have been written")
	afterRaw, err := os.ReadFile(afterPath)
	require.NoError(t, err, "after-run worktree listing must have been written")
	wtRootBefore := strings.Fields(string(beforeRaw))
	wtRootAfter := strings.Fields(string(afterRaw))
	require.NotEmpty(t, wtRootBefore, "worktree root must have at least one entry at checkpoint")
	assert.Equal(t, wtRootBefore, wtRootAfter,
		"worktree root entries must not change while the embedded harness runs (before=%v after=%v)",
		wtRootBefore, wtRootAfter)
	assert.NotContains(t, wtRootAfter, simulatedSessionFile,
		"simulated embedded session file must not land at the worktree root")
	assert.NotContains(t, wtRootAfter, ".agent-session.json",
		"embedded agent must not write .agent-session.json at the worktree root")
}

// TestExecuteBeadPromptIsXMLTagged verifies that the synthesized execute-bead
// prompt is emitted as a well-structured XML document with the tags required
// by FEAT-006's Prompt Rationalizer Contract. It also guards against regression
// to the old markdown-heading-only prompt structure.
func TestExecuteBeadPromptIsXMLTagged(t *testing.T) {
	workDir := t.TempDir()

	scrubEnv := func() []string {
		parent := os.Environ()
		env := make([]string, 0, len(parent))
		for _, kv := range parent {
			if strings.HasPrefix(kv, "GIT_") {
				continue
			}
			env = append(env, kv)
		}
		return env
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = scrubEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@ddx.test")
	runGit("config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial seed")

	specPath := filepath.Join(workDir, "docs", "feat-xml.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(specPath), 0o755))
	require.NoError(t, os.WriteFile(specPath, []byte(`---
ddx:
  id: FEAT-XML-TEST
  title: XML Test Spec
---
# XML Test Spec
`), 0o644))
	runGit("add", "docs/feat-xml.md")
	runGit("commit", "-m", "docs: add xml spec")

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:          "xml-bead",
		Title:       "Adopt XML-tagged execute-bead prompt template",
		Status:      bead.StatusOpen,
		Priority:    0,
		IssueType:   bead.DefaultType,
		Parent:      "ddx-parent",
		Description: "Replace the markdown-heading prompt with an XML-tagged structure so downstream tooling can diff and validate sections deterministically.",
		Acceptance:  "Prompt is XML-tagged with <execute-bead>, <bead>, <governing>, and <instructions>.",
		Labels:      []string{"area:agent", "area:docs"},
		Extra:       map[string]any{"spec-id": "FEAT-XML-TEST"},
	})
	runGit("add", ".ddx/beads.jsonl")
	runGit("commit", "-m", "chore: seed bead")
	baseSHA := runGit("rev-parse", "HEAD")

	// Script harness directive: no-op. The assertion target is the synthesized
	// prompt file, not worker output.
	directivePath := filepath.Join(t.TempDir(), "directives.txt")
	require.NoError(t, os.WriteFile(directivePath, []byte("no-op\n"), 0o644))

	// Real runner + real git ops: no overrides that fake the thing under test.
	f := NewCommandFactory(workDir)
	f.AgentRunnerOverride = agent.NewRunner(agent.Config{})

	res := runExecuteBead(t, f, nil, "xml-bead",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)

	require.NotEmpty(t, res.PromptFile)
	promptPath := filepath.Join(workDir, filepath.FromSlash(res.PromptFile))
	promptRaw, err := os.ReadFile(promptPath)
	require.NoError(t, err)
	promptText := string(promptRaw)

	// Required root and subsection tags.
	assert.Contains(t, promptText, "<execute-bead>")
	assert.Contains(t, promptText, "</execute-bead>")
	assert.Contains(t, promptText, `<bead id="xml-bead">`)
	assert.Contains(t, promptText, "</bead>")
	assert.Contains(t, promptText, "<title>Adopt XML-tagged execute-bead prompt template</title>")
	assert.Contains(t, promptText, "<description>")
	assert.Contains(t, promptText, "</description>")
	assert.Contains(t, promptText, "<acceptance>")
	assert.Contains(t, promptText, "</acceptance>")
	assert.Contains(t, promptText, "<labels>area:agent, area:docs</labels>")
	assert.Contains(t, promptText, `parent="ddx-parent"`)
	assert.Contains(t, promptText, `spec-id="FEAT-XML-TEST"`)
	assert.Contains(t, promptText, fmt.Sprintf(`base-rev="%s"`, baseSHA))
	assert.Contains(t, promptText, `<metadata `)
	assert.Contains(t, promptText, "<governing>")
	assert.Contains(t, promptText, "</governing>")
	assert.Contains(t, promptText, `<ref id="FEAT-XML-TEST"`)
	assert.Contains(t, promptText, "<instructions>")
	assert.Contains(t, promptText, "</instructions>")

	// Regression guard: no markdown-heading-only sections.
	assert.NotContains(t, promptText, "# Execute Bead\n")
	assert.NotContains(t, promptText, "## Bead\n")
	assert.NotContains(t, promptText, "## Description\n")
	assert.NotContains(t, promptText, "## Acceptance Criteria\n")
	assert.NotContains(t, promptText, "## Governing References\n")
	assert.NotContains(t, promptText, "## Execution Rules\n")

	// The prompt must be parseable as a well-formed XML document.
	decoder := xml.NewDecoder(bytes.NewBufferString(promptText))
	for {
		_, tokErr := decoder.Token()
		if tokErr == io.EOF {
			break
		}
		require.NoError(t, tokErr, "prompt must be well-formed XML: %s", promptText)
	}
}
