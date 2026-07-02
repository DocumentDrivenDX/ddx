package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerCmd_SetPersistsDesiredState proves `ddx worker set` writes a
// valid .ddx/workers/desired.json that the server-side supervisor can
// load without further intervention. This closes the CLI → filesystem →
// supervisor loop that Phase 1 of ddx-9d1af129 introduces.
func TestWorkerCmd_SetPersistsDesiredState(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))

	f := &CommandFactory{WorkingDir: projectRoot}
	cmd := f.newWorkerSetCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{
		"--project", projectRoot,
		"--count", "2",
		"--harness", "fiz",
		"--model", "haiku",
		"--restart-enabled=true",
	})
	require.NoError(t, cmd.Execute())

	desiredPath := ddxroot.JoinProject(projectRoot, "workers", "desired.json")
	raw, err := os.ReadFile(desiredPath)
	require.NoError(t, err, "worker set must write desired.json at the supervisor-canonical path")

	var state server.WorkerDesiredState
	require.NoError(t, json.Unmarshal(raw, &state))
	assert.Equal(t, 2, state.DesiredCount)
	assert.Equal(t, "fiz", state.DefaultSpec.Harness)
	assert.Equal(t, "haiku", state.DefaultSpec.Model)
	assert.True(t, state.Restart.Enabled)

	// Round-trip through the supervisor: prove the file is not just
	// syntactically valid JSON but also passes the supervisor's
	// Validate + ApplyDefaults gate.
	sup, err := workerNewSupervisor(projectRoot)
	require.NoError(t, err)
	loaded, err := sup.LoadDesiredState()
	require.NoError(t, err)
	assert.Equal(t, 2, loaded.DesiredCount)
	assert.Equal(t, "fiz", loaded.DefaultSpec.Harness)
	assert.True(t, loaded.Restart.Enabled)
}

// TestWorkerCmd_EnableIsShortcut proves `ddx worker enable` is equivalent
// to `set --count 1 --restart-enabled=true`, honoring the operator UX
// promise that enabling supervision is a one-liner.
func TestWorkerCmd_EnableIsShortcut(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))

	f := &CommandFactory{WorkingDir: projectRoot}
	cmd := f.newWorkerEnableCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--project", projectRoot})
	require.NoError(t, cmd.Execute())

	sup, err := workerNewSupervisor(projectRoot)
	require.NoError(t, err)
	loaded, err := sup.LoadDesiredState()
	require.NoError(t, err)
	assert.Equal(t, 1, loaded.DesiredCount)
	assert.True(t, loaded.Restart.Enabled)
}

// TestWorkerCmd_StatusReportsAbsent proves `ddx worker status` gracefully
// reports the "not yet configured" state instead of erroring out, so
// operators can query freshly-cloned projects without pre-arming.
func TestWorkerCmd_StatusReportsAbsent(t *testing.T) {
	projectRoot := t.TempDir()

	f := &CommandFactory{WorkingDir: projectRoot}
	cmd := f.newWorkerStatusCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--project", projectRoot})
	require.NoError(t, cmd.Execute())

	assert.Contains(t, stdout.String(), "no desired state persisted",
		"status must gracefully report the absent-configuration case, not error")
}
