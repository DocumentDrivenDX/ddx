package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for the interactive `ddx agent execute-bead` gate-eval wiring
// added by ddx-14c0e790. The wiring rebuilds an ephemeral worktree at
// ResultRev (since ExecuteBead has already torn down the worker worktree)
// so required-gate evaluation can run before the orchestrator decides
// merge vs preserve.

// TestExecuteBead_RequiredGatePass_Merges verifies that when a required
// gate exits 0 the interactive path merges, populates GateResults / summary,
// and records checks.json relative to the execution bundle.
//
// Migrated off fakeExecuteBeadGit / fakeAgentRunner per concerns.md §testing
// ("no mocks, period"; "never mock the thing you are testing"). Exercises the
// real ExecuteBead → LandBeadResult → Land pipeline against an isolated real
// git repo, with the script harness driving an actual commit via a per-attempt
// directive file. Parent: ddx-d9df348d.
func TestExecuteBead_RequiredGatePass_Merges(t *testing.T) {
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
		ID:        "gate-pass-bead",
		Title:     "Bead with passing required gate",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
		Extra:     map[string]any{"spec-id": "FEAT-GATE-TEST"},
	})
	// Gate docs must be committed so the real worker worktree (created at
	// baseSHA) sees them; real git only materializes tracked content.
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

	res := runExecuteBead(t, f, nil, "gate-pass-bead",
		"--from", baseSHA,
		"--harness", "script", "--model", directivePath)

	assert.Equal(t, "merged", res.Outcome, "passing required gate must allow merge")
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, "pass", res.RequiredExecSummary)
	require.Len(t, res.GateResults, 1, "the required gate must be evaluated")
	assert.Equal(t, "pass", res.GateResults[0].Status)
	assert.NotEmpty(t, res.ChecksFile, "checks.json relative path must be recorded on success")
}

// TestExecuteBead_RequiredGateFail_Preserves verifies that when a required
// gate exits non-zero the interactive path preserves under
// refs/ddx/iterations and surfaces the post-run-checks reason.
func TestExecuteBead_RequiredGateFail_Preserves(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222",
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0, Harness: "mock"}}
	f := newExecuteBeadFactory(t, git, runner)

	seedExecuteBead(t, f.WorkingDir, &bead.Bead{
		ID:        "gate-fail-bead",
		Title:     "Bead with failing required gate",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
		Extra:     map[string]any{"spec-id": "FEAT-GATE-TEST"},
	})
	seedGateDocs(t, f.WorkingDir, []string{"false"})

	res := runExecuteBead(t, f, git, "gate-fail-bead")

	assert.Equal(t, "preserved", res.Outcome, "failing required gate must preserve")
	assert.Equal(t, agent.ExecuteBeadStatusPostRunCheckFailed, res.Status)
	assert.Equal(t, "fail", res.RequiredExecSummary)
	assert.NotEmpty(t, res.PreserveRef, "preserve ref must be recorded when required gate fails")
	assert.Contains(t, res.Reason, "post-run checks failed")
	require.Len(t, res.GateResults, 1)
	assert.Equal(t, "fail", res.GateResults[0].Status)
	assert.NotEmpty(t, res.ChecksFile, "checks.json must be recorded even when gates fail")
}

// TestExecuteBead_NoGoverningIDs_Merges verifies the backward-compat path:
// when the bead has no spec-id (and therefore the manifest declares no
// governing IDs), the interactive path merges without running gate eval
// and writes no checks.json.
func TestExecuteBead_NoGoverningIDs_Merges(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "bbbb2222",
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0, Harness: "mock"}}
	f := newExecuteBeadFactory(t, git, runner)

	seedExecuteBead(t, f.WorkingDir, &bead.Bead{
		ID:        "no-govern-bead",
		Title:     "Bead with no governing IDs",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
		// no spec-id => no governing IDs in the manifest
	})

	res := runExecuteBead(t, f, git, "no-govern-bead")

	assert.Equal(t, "merged", res.Outcome, "merge proceeds when there are no governing IDs to gate")
	assert.Empty(t, res.GateResults, "gate eval must be skipped when no governing IDs")
	assert.Empty(t, res.ChecksFile, "no checks.json when gate eval is skipped")
	assert.Empty(t, res.PreserveRef, "no preserve ref when gate eval is skipped")
}
