package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
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

// TestWorkerStatusReportsFDExhaustionForMissingDesiredWorker proves text
// status names the missing desired worker and explains fd exhaustion when
// desired_count=1 and the last terminal managed worker exited from fd
// exhaustion (ddx-744b0996).
func TestWorkerStatusReportsFDExhaustionForMissingDesiredWorker(t *testing.T) {
	projectRoot := seedMissingDesiredWorkerFDExhaustion(t)

	f := &CommandFactory{WorkingDir: projectRoot}
	cmd := f.newWorkerStatusCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--project", projectRoot})
	require.NoError(t, cmd.Execute())

	out := stdout.String()
	assert.Contains(t, out, "desired_count: 1")
	assert.Contains(t, out, "missing_count: 1")
	assert.Contains(t, out, "missing: 1 desired worker")
	assert.Contains(t, out, "fd_exhaustion_diagnosis: fd_exhaustion")
	assert.Contains(t, out, "fd exhaustion")
	assert.Contains(t, out, "worker-20260716T000001-fd")
}

// TestWorkerStatusJSONIncludesFDExhaustionForMissingDesiredWorker proves
// JSON status includes desired_count, missing worker count, and
// fd_exhaustion_diagnosis fields (ddx-744b0996).
func TestWorkerStatusJSONIncludesFDExhaustionForMissingDesiredWorker(t *testing.T) {
	projectRoot := seedMissingDesiredWorkerFDExhaustion(t)

	f := &CommandFactory{WorkingDir: projectRoot}
	cmd := f.newWorkerStatusCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--project", projectRoot, "--json"})
	require.NoError(t, cmd.Execute())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	assert.EqualValues(t, 1, payload["desired_count"])
	assert.EqualValues(t, 1, payload["missing_count"])
	assert.EqualValues(t, 0, payload["live_count"])
	assert.Equal(t, "fd_exhaustion", payload["fd_exhaustion_diagnosis"])
	assert.Equal(t, "worker-20260716T000001-fd", payload["last_terminal_worker_id"])
}

// seedMissingDesiredWorkerFDExhaustion writes desired_count=1 with no live
// worker and a terminal managed worker whose structured result is fd
// exhaustion — the operator-visible gap this bead closes.
func seedMissingDesiredWorkerFDExhaustion(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))

	sup, err := workerNewSupervisor(projectRoot)
	require.NoError(t, err)
	state := server.DefaultWorkerDesiredState(projectRoot)
	state.DesiredCount = 1
	state.Restart.Enabled = true
	require.NoError(t, sup.SaveDesiredState(&state))

	workerID := "worker-20260716T000001-fd"
	terminalAt := time.Date(2026, 7, 16, 0, 1, 0, 0, time.UTC)
	dir := ddxroot.JoinProject(projectRoot, "workers", workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	rec := server.WorkerRecord{
		ID:          workerID,
		Kind:        "work",
		State:       "exited",
		Status:      agent.ExecuteBeadStatusResourceExhausted,
		ProjectRoot: projectRoot,
		StartedAt:   terminalAt.Add(-time.Minute),
		FinishedAt:  terminalAt,
		LastError:   agent.FDExhaustionStopMessage,
		LastResult: &server.WorkerExecutionResult{
			Status: agent.ExecuteBeadStatusResourceExhausted,
			Detail: agent.FDExhaustionStopMessage,
		},
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.json"), append(raw, '\n'), 0o644))

	require.NoError(t, server.WriteManagedWorkerResult(projectRoot, workerID, server.ManagedWorkerResult{
		StopCondition:     agent.ExecuteBeadStatusResourceExhausted,
		LastFailureStatus: agent.ExecuteBeadStatusResourceExhausted,
		LastFailureDetail: agent.FDExhaustionStopMessage,
	}))

	return projectRoot
}
