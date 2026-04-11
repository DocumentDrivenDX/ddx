package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentExecuteLoopUsesProjectRootForNoWorkScan(t *testing.T) {
	env := NewTestEnvironment(t)
	subdir := filepath.Join(env.Dir, "nested", "path")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	factory := NewCommandFactory(subdir)
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "agent", "execute-loop", "--json")
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
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111",
		dirty:       true,
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0, Harness: "mock"}}
	f := newExecuteBeadFactory(t, git, runner)

	res, err := f.invokeExecuteBeadFromLoop(context.Background(), "my-bead", executeLoopCommandOptions{})
	require.NoError(t, err)
	assert.Equal(t, "my-bead", res.BeadID)
	assert.Equal(t, "no-changes", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusNoChanges, res.Status)
	assert.Equal(t, "aaaa1111", res.BaseRev)
}
