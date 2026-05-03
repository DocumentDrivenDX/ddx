package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentExecuteLoopUsesProjectRootForNoWorkScan(t *testing.T) {
	env := NewTestEnvironment(t)
	subdir := filepath.Join(env.Dir, "nested", "path")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	factory := NewCommandFactory(subdir)
	root := factory.NewRootCommand()

	// Explicit --poll-interval=0 selects legacy drain-and-exit semantics
	// (ddx-dc157075). The default flipped to 30s so the worker stays alive
	// across empty polls; tests that want to assert the no-ready-work
	// outcome must opt out of the long-running default.
	out, err := executeCommand(root, "agent", "execute-loop", "--local", "--json", "--poll-interval=0")
	require.NoError(t, err)

	var res struct {
		ProjectRoot string `json:"project_root"`
		NoReadyWork bool   `json:"no_ready_work"`
		Attempts    int    `json:"attempts"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.Equal(t, env.Dir, res.ProjectRoot)
	assert.True(t, res.NoReadyWork)
	assert.Equal(t, 0, res.Attempts)
}

func TestInvokeExecuteBeadFromLoopParsesJSONAmidWarnings(t *testing.T) {
	workDir := t.TempDir()
	// Init git repo so HEAD can be resolved
	out, err := exec.Command("git", "init", workDir).CombinedOutput()
	require.NoError(t, err, string(out))
	// Create an initial commit so HEAD exists
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# test"), 0o644))
	out, err = exec.Command("git", "-C", workDir, "add", "-A").CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("git", "-C", workDir, "-c", "user.name=Test", "-c", "user.email=test@test.com", "commit", "-m", "init").CombinedOutput()
	require.NoError(t, err, string(out))

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:     "my-bead",
		Title:  "Test bead",
		Status: bead.StatusOpen,
	})
	// Commit the beads.jsonl so it's in the worktree snapshot
	out, err = exec.Command("git", "-C", workDir, "add", ".ddx/beads.jsonl").CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("git", "-C", workDir, "-c", "user.name=Test", "-c", "user.email=test@test.com", "commit", "-m", "add beads").CombinedOutput()
	require.NoError(t, err, string(out))

	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111",
		dirty:       true,
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0, Harness: "mock"}}

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})
	res, err := agent.ExecuteBeadWithConfig(context.Background(), workDir, "my-bead", rcfg, agent.ExecuteBeadRuntime{AgentRunner: runner}, git)
	require.NoError(t, err)
	assert.Equal(t, "my-bead", res.BeadID)
	assert.Equal(t, agent.ExecuteBeadStatusNoEvidenceProduced, res.Status)
}
