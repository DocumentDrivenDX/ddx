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
func TestExecuteBeadHiddenRefUniqueness(t *testing.T) {
	makeRun := func(ts time.Time) agent.ExecuteBeadResult {
		oldNow := agent.NowFunc
		agent.NowFunc = func() time.Time { return ts }
		defer func() { agent.NowFunc = oldNow }()

		git := &fakeExecuteBeadGit{
			mainHeadRev: "aaaa1111",
			wtHeadRev:   "eeee5555",
			mergeErr:    fmt.Errorf("diverged"),
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
		wtDirty:     false,
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, "no-changes", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusNoChanges, res.Status)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Empty(t, res.PreserveRef)
}

// TestExecuteBeadDirtyWorktreeWithoutCommits verifies that tracked file edits
// left uncommitted by the agent are synthesized into a commit and treated as
// real output rather than being discarded as "no-changes".
func TestExecuteBeadDirtyWorktreeWithoutCommits(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111", // agent made no commits
		wtDirty:     true,       // but left tracked file edits
		synthRev:    "cccc3333", // SynthesizeCommit produces this rev
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.NotEqual(t, "no-changes", res.Outcome, "dirty worktree should not be classified as no-changes")
	assert.Equal(t, "cccc3333", res.ResultRev)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Equal(t, "merged", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, 1, git.mergeCalls)
	assert.Equal(t, "cccc3333", git.mergeRev)
}

func TestExecuteBeadMergePreservesContext(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222",
		mergeErr:    nil,
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, "merged", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Equal(t, "bbbb2222", res.ResultRev)
	assert.Equal(t, 1, git.mergeCalls)
	assert.Equal(t, "bbbb2222", git.mergeRev)
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
	f.executeBeadOrchestratorGitOverride = git
	f.executeBeadLandingAdvancerOverride = fakeLandingAdvancerFromGit(git)

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

func TestExecuteBeadResolvesPathStyleSpecID(t *testing.T) {
	workDir := t.TempDir()
	specPath := filepath.Join(workDir, "workflows", "README.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(specPath), 0o755))
	require.NoError(t, os.WriteFile(specPath, []byte("# Workflow\n"), 0o644))
	refs := agent.ResolveGoverningRefs(workDir, &bead.Bead{
		ID:    "path-bead",
		Title: "Resolve path style spec ids",
		Extra: map[string]any{"spec-id": "workflows/README.md"},
	})
	require.Len(t, refs, 1)
	assert.Equal(t, "workflows/README.md", refs[0].ID)
	assert.Equal(t, "workflows/README.md", refs[0].Path)
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
	f.executeBeadOrchestratorGitOverride = git
	f.executeBeadLandingAdvancerOverride = fakeLandingAdvancerFromGit(git)

	t.Setenv("DDX_WORKER_ID", "worker-test")
	res := runExecuteBead(t, f, git, "my-bead")

	require.Len(t, git.addedWTs, 1)
	manifestPath := filepath.Join(workDir, filepath.FromSlash(res.ManifestFile))
	resultPath := filepath.Join(workDir, filepath.FromSlash(res.ResultFile))
	require.FileExists(t, manifestPath)
	require.FileExists(t, resultPath)

	manifestRaw, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Contains(t, string(manifestRaw), `"bead_id": "my-bead"`)
	assert.Contains(t, string(manifestRaw), `"worker_id": "worker-test"`)
	assert.Contains(t, string(manifestRaw), `"prompt": "synthesized"`)
	// Worktree path is now under $TMPDIR/ddx-exec-wt/ — moved from .ddx/
	// so test runs inside the worktree don't corrupt the parent repo via
	// GIT_DIR inheritance. The leaf name still starts with
	// .execute-bead-wt- for orphan recovery via git worktree list.
	assert.Contains(t, string(manifestRaw), agent.ExecuteBeadWtPrefix+"my-bead-")

	resultRaw, err := os.ReadFile(resultPath)
	require.NoError(t, err)
	var recorded agent.ExecuteBeadResult
	require.NoError(t, json.Unmarshal(resultRaw, &recorded))
	assert.Equal(t, res.BeadID, recorded.BeadID)
	assert.Equal(t, "worker-test", recorded.WorkerID)
	assert.Equal(t, res.AttemptID, recorded.AttemptID)
	assert.Equal(t, res.Status, recorded.Status)
	assert.Equal(t, res.ResultFile, recorded.ResultFile)
	assert.NoDirExists(t, git.addedWTs[0])
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
	orphanPath := workDir + "/.ddx/" + agent.ExecuteBeadWtPrefix + "my-bead-old-attempt"
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

// TestExecuteBeadHarnessNoiseNotSynthesized verifies that when the agent makes no
// real commits but the worktree is dirty with only harness bookkeeping files
// (e.g. .ddx/agent-logs), SynthesizeCommit returns (false, nil) and the outcome
// is "no-changes", not "merged" or "success". ResultRev must equal BaseRev.
func TestExecuteBeadHarnessNoiseNotSynthesized(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111", // agent made no real commits
		wtDirty:     true,       // worktree is dirty (e.g. agent-logs written)
		// synthRev is intentionally empty: SynthesizeCommit returns (false, nil)
		// simulating that all dirty files were harness noise.
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, "no-changes", res.Outcome, "harness-noise-only dirty worktree must not produce a synthesis commit")
	assert.Equal(t, agent.ExecuteBeadStatusNoChanges, res.Status)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Equal(t, "aaaa1111", res.ResultRev, "ResultRev must equal BaseRev when no real commit was made")
	assert.Equal(t, 0, git.mergeCalls, "merge must not be called when outcome is no-changes")
}

// TestExecuteBeadAgentErrorNoCommits verifies that when the agent runner returns
// an error and makes no commits, the outcome is an execution error rather than
// a misleading no-change result.
func TestExecuteBeadAgentErrorNoCommits(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111", // no commits made
	}
	runner := &fakeAgentRunner{err: fmt.Errorf("agent crashed"), result: nil}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, 1, res.ExitCode)
	assert.Equal(t, "error", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
	assert.Equal(t, "agent crashed", res.Reason)
	assert.Equal(t, "agent crashed", res.Error)
	assert.Equal(t, "aaaa1111", res.BaseRev)
	assert.Empty(t, res.PreserveRef)
}

func TestExecuteBeadTimeoutNoCommitsReportsExecutionFailure(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111",
	}
	runner := &fakeAgentRunner{result: &agent.Result{
		ExitCode: -1,
		Error:    "timeout after 5m",
		Harness:  "codex",
	}}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, -1, res.ExitCode)
	assert.Equal(t, "error", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
	assert.Equal(t, "timeout after 5m", res.Reason)
	assert.Equal(t, "timeout after 5m", res.Error)
	assert.Equal(t, "aaaa1111", res.ResultRev)
	assert.Empty(t, res.PreserveRef)
}

// TestExecuteBeadAgentErrorWithCommitsPreservesBeforeLand verifies that a
// non-zero agent result preserves the iteration instead of touching the target
// branch, even if a merge would have succeeded.
func TestExecuteBeadAgentErrorWithCommitsPreservesBeforeLand(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222", // agent made commits
		mergeErr:    nil,        // merge succeeds
	}
	runner := &fakeAgentRunner{err: fmt.Errorf("agent crashed"), result: nil}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead")

	assert.Equal(t, 1, res.ExitCode)
	assert.Equal(t, "preserved", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, res.Status)
	assert.Equal(t, "bbbb2222", res.ResultRev)
	assert.NotEmpty(t, res.PreserveRef)
	assert.Equal(t, 0, git.mergeCalls)
}

// TestExecuteBeadAgentErrorWithCommitsPreserves verifies that when the agent
// runner returns an error, commits exist but merge fails, exitCode=1 and
// outcome="preserved" with a non-empty preserve ref.
func TestExecuteBeadAgentErrorWithCommitsPreserves(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222",
		mergeErr:    fmt.Errorf("merge conflict"),
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

// TestExecuteBeadModelFlagPassthrough locks in the resolution contract for
// execute-bead's model option: the value supplied via ExecuteBeadOptions.Model
// is passed verbatim to the runner, and an empty value is not silently replaced
// by any hardcoded or catalog-derived default. This regression test guards
// against routing layers injecting a model (e.g. a stale vendor/model like
// "z-ai/glm-5.1") when the caller did not request one — the case the agent
// harness resolves from ~/.config/agent/config.yaml must be preserved by
// ExecuteBead handing the runner an empty Model so the harness's own
// resolution chain runs.
func TestExecuteBeadModelFlagPassthrough(t *testing.T) {
	t.Run("empty model stays empty through ExecuteBead", func(t *testing.T) {
		git := &fakeExecuteBeadGit{
			mainHeadRev: "aaaa1111",
			wtHeadRev:   "bbbb2222",
		}
		runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
		f := newExecuteBeadFactory(t, git, runner)

		// No --model flag supplied to execute-bead.
		runExecuteBead(t, f, git, "my-bead")

		assert.Equal(t, "", runner.last.Model,
			"runner must receive an empty Model when no --model flag is provided; "+
				"any non-empty value here indicates a routing layer injected a default, "+
				"which would override the harness's own config-driven resolution")
	})

	t.Run("explicit model is forwarded verbatim", func(t *testing.T) {
		git := &fakeExecuteBeadGit{
			mainHeadRev: "aaaa1111",
			wtHeadRev:   "bbbb2222",
		}
		runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
		f := newExecuteBeadFactory(t, git, runner)

		runExecuteBead(t, f, git, "my-bead", "--model", "qwen3.5-27b")

		assert.Equal(t, "qwen3.5-27b", runner.last.Model,
			"runner must receive the exact --model value the caller passed")
	})
}

func TestExecuteBeadStatusMapping(t *testing.T) {
	cases := []struct {
		name     string
		result   agent.ExecuteBeadResult
		expected string
	}{
		{
			name:     "merged success",
			result:   agent.ExecuteBeadResult{Outcome: "merged", ExitCode: 0},
			expected: agent.ExecuteBeadStatusSuccess,
		},
		{
			name:     "no changes stays non-success",
			result:   agent.ExecuteBeadResult{Outcome: "no-changes", ExitCode: 0},
			expected: agent.ExecuteBeadStatusNoChanges,
		},
		{
			name:     "execution failure dominates preserved outcome",
			result:   agent.ExecuteBeadResult{Outcome: "preserved", ExitCode: 1, Reason: "agent execution failed"},
			expected: agent.ExecuteBeadStatusExecutionFailed,
		},
		{
			name:     "error outcome stays execution failure",
			result:   agent.ExecuteBeadResult{Outcome: "error", ExitCode: -1, Reason: "timeout after 5m"},
			expected: agent.ExecuteBeadStatusExecutionFailed,
		},
		{
			name:     "land conflict",
			result:   agent.ExecuteBeadResult{Outcome: "preserved", ExitCode: 0, Reason: "merge failed"},
			expected: agent.ExecuteBeadStatusLandConflict,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := tc.result
			res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
			res.Detail = agent.ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
			assert.Equal(t, tc.expected, res.Status)
			assert.NotEmpty(t, res.Detail)
		})
	}
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
func TestExecuteBeadGatePass(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222",
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0, Harness: "mock"}}
	f := newExecuteBeadFactory(t, git, runner)

	seedExecuteBead(t, f.WorkingDir, &bead.Bead{
		ID:        "gate-bead",
		Title:     "Bead with required gate",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
		Extra:     map[string]any{"spec-id": "FEAT-GATE-TEST"},
	})
	seedGateDocs(t, f.WorkingDir, []string{"true"})

	res := runExecuteBead(t, f, git, "gate-bead")

	assert.Equal(t, "merged", res.Outcome)
	require.Len(t, res.GateResults, 1, "required gate must be evaluated")
	assert.Equal(t, "pass", res.GateResults[0].Status)
}

// TestExecuteBeadGateBlocksLanding verifies that execute-bead preserves the
// result instead of merging when a required gate fails.
// Wired by ddx-14c0e790 (interactive execute-bead gate eval).
func TestExecuteBeadGateBlocksLanding(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222",
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0, Harness: "mock"}}
	f := newExecuteBeadFactory(t, git, runner)

	seedExecuteBead(t, f.WorkingDir, &bead.Bead{
		ID:        "gate-bead-fail",
		Title:     "Bead with failing gate",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
		Extra:     map[string]any{"spec-id": "FEAT-GATE-TEST"},
	})
	seedGateDocs(t, f.WorkingDir, []string{"false"})

	res := runExecuteBead(t, f, git, "gate-bead-fail")

	assert.Equal(t, "preserved", res.Outcome, "failing required gate must preserve")
	require.Len(t, res.GateResults, 1)
	assert.Equal(t, "fail", res.GateResults[0].Status)
	assert.NotEmpty(t, res.PreserveRef, "failed-gate landing must preserve under refs/ddx/iterations")
}

// TestExecuteBeadNoGatesWhenNoChanges verifies that gates are not evaluated
// when the agent produces no changes (resultRev == baseRev).
func TestExecuteBeadNoGatesWhenNoChanges(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111", // same rev = no changes
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0, Harness: "mock"}}
	f := newExecuteBeadFactory(t, git, runner)

	seedExecuteBead(t, f.WorkingDir, &bead.Bead{
		ID:        "gate-bead-nochange",
		Title:     "Bead with gate but no changes",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
		Extra:     map[string]any{"spec-id": "FEAT-GATE-TEST"},
	})
	seedGateDocs(t, f.WorkingDir, []string{"false"})

	res := runExecuteBead(t, f, git, "gate-bead-nochange")

	assert.Equal(t, "no-changes", res.Outcome)
	assert.Empty(t, res.GateResults, "gates must not run when agent made no changes")
}

// TestExecuteBeadEmbeddedAgentStateRedirected verifies that when execute-bead
// invokes the embedded-agent harness, its session/telemetry runtime state is
// redirected into a DDx-owned directory inside the execution bundle instead
// of being written at the worktree root. Regression guard for ddx-cba2dc64.
func TestExecuteBeadEmbeddedAgentStateRedirected(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111cafe",
		wtHeadRev:   "bbbb2222beef",
	}

	// Snapshot of wtPath root contents captured during Run so the assertion
	// can run before ExecuteBead removes the worktree on return.
	var wtPathDuringRun string
	var wtRootBefore []string
	var wtRootAfter []string
	var sessionLogDirSeen string
	simulatedSessionFile := "agent-embedded-session.jsonl"

	runner := &fakeAgentRunner{
		result: &agent.Result{ExitCode: 0, Harness: "agent"},
		sideEffect: func(opts agent.RunOptions) error {
			wtPathDuringRun = opts.WorkDir
			sessionLogDirSeen = opts.SessionLogDir

			// Capture worktree root listing before simulating writes.
			entries, err := os.ReadDir(opts.WorkDir)
			if err != nil {
				return err
			}
			for _, e := range entries {
				wtRootBefore = append(wtRootBefore, e.Name())
			}

			// Simulate the embedded-agent harness writing runtime state.
			// It MUST land in opts.SessionLogDir, not opts.WorkDir. If the
			// execute-bead wiring is broken and SessionLogDir is empty or the
			// worktree root, this write will land at the worktree root and
			// the post-check below will catch it.
			if opts.SessionLogDir == "" {
				return fmt.Errorf("embedded agent runner received empty SessionLogDir; runtime state would land at worktree root")
			}
			if err := os.MkdirAll(opts.SessionLogDir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(opts.SessionLogDir, simulatedSessionFile), []byte(`{"event":"started"}`+"\n"), 0o644); err != nil {
				return err
			}

			// Capture worktree root listing after simulated writes.
			entries, err = os.ReadDir(opts.WorkDir)
			if err != nil {
				return err
			}
			for _, e := range entries {
				wtRootAfter = append(wtRootAfter, e.Name())
			}
			return nil
		},
	}
	f := newExecuteBeadFactory(t, git, runner)

	res := runExecuteBead(t, f, git, "my-bead", "--harness", "agent")
	require.Equal(t, "merged", res.Outcome, "execute-bead should succeed for this test")

	// The runner must have received a SessionLogDir override.
	require.NotEmpty(t, sessionLogDirSeen, "execute-bead must pass a SessionLogDir to the embedded harness")

	// The override must point inside the tracked execution bundle, not at
	// the worktree root.
	require.NotEmpty(t, res.ExecutionDir, "execute-bead must record an execution bundle dir")
	bundleAbs := filepath.Join(f.WorkingDir, filepath.FromSlash(res.ExecutionDir))
	absLog, err := filepath.Abs(sessionLogDirSeen)
	require.NoError(t, err)
	absBundle, err := filepath.Abs(bundleAbs)
	require.NoError(t, err)
	assert.Truef(t, strings.HasPrefix(absLog, absBundle+string(filepath.Separator)),
		"SessionLogDir (%s) must be inside the execution bundle (%s)", absLog, absBundle)
	assert.Equal(t, "embedded", filepath.Base(absLog),
		"SessionLogDir must be the bundle's embedded/ subdirectory")

	// The worktree root must not gain any files during the run.
	require.NotEmpty(t, wtPathDuringRun)
	assert.Equal(t, wtRootBefore, wtRootAfter,
		"worktree root entries must not change while the embedded harness runs (before=%v after=%v)",
		wtRootBefore, wtRootAfter)
	assert.NotContains(t, wtRootAfter, simulatedSessionFile,
		"simulated embedded session file must not land at the worktree root")
	assert.NotContains(t, wtRootAfter, ".agent-session.json",
		"embedded agent must not write .agent-session.json at the worktree root")

	// The simulated session file must exist at the redirected location.
	assert.FileExists(t, filepath.Join(sessionLogDirSeen, simulatedSessionFile))
}

// TestExecuteBeadPromptIsXMLTagged verifies that the synthesized execute-bead
// prompt is emitted as a well-structured XML document with the tags required
// by FEAT-006's Prompt Rationalizer Contract. It also guards against regression
// to the old markdown-heading-only prompt structure.
func TestExecuteBeadPromptIsXMLTagged(t *testing.T) {
	workDir := t.TempDir()
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
	specPath := filepath.Join(workDir, "docs", "feat-xml.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(specPath), 0o755))
	require.NoError(t, os.WriteFile(specPath, []byte(`---
ddx:
  id: FEAT-XML-TEST
  title: XML Test Spec
---
# XML Test Spec
`), 0o644))

	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111cafe",
		wtHeadRev:   "bbbb2222beef",
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0, Harness: "mock"}}
	f := NewCommandFactory(workDir)
	seedDefaultExecuteBeads(t, workDir)
	f.AgentRunnerOverride = runner
	f.executeBeadGitOverride = git
	f.executeBeadOrchestratorGitOverride = git
	f.executeBeadLandingAdvancerOverride = fakeLandingAdvancerFromGit(git)

	_ = runExecuteBead(t, f, git, "xml-bead")

	require.NotEmpty(t, runner.last.PromptFile)
	promptRaw, err := os.ReadFile(runner.last.PromptFile)
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
	assert.Contains(t, promptText, `base-rev="aaaa1111cafe"`)
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
