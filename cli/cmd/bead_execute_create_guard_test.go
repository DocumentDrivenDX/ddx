package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteBeadRejectsTopLevelSampleChildBeads(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	run := func(args ...string) (string, error) {
		return executeCommand(factory.NewRootCommand(), args...)
	}

	parentOut, err := run("bead", "create", "Parent decomposition bead", "--type", "task")
	require.NoError(t, err)
	parentID := strings.TrimSpace(parentOut)
	require.NotEmpty(t, parentID)

	t.Setenv(agent.DDXModeEnvKey, agent.DDXModeBeadExecution)
	t.Setenv(agent.DDXBeadIDEnvKey, parentID)
	t.Setenv(agent.DDXAttemptIDEnvKey, "attempt-sample-reject")

	_, err = run("bead", "create", "Sample: lint cleanup", "--type", "task")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sample_fixture_title")

	store := factory.beadStore()
	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	for _, b := range all {
		if b.ID == parentID {
			continue
		}
		assert.NotEqual(t, "Sample: lint cleanup", b.Title, "invalid sample child must not be persisted")
	}

	parent, err := store.Get(context.Background(), parentID)
	require.NoError(t, err)
	require.NotNil(t, parent)
	events, err := store.Events(parentID)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	last := events[len(events)-1]
	assert.Equal(t, "operator_attention", last.Kind)
	assert.Equal(t, "execute_bead_child_create_rejected", last.Summary)
	assert.Contains(t, last.Body, `"attempted_title":"Sample: lint cleanup"`)
	assert.Contains(t, last.Body, `"parent":""`)
	assert.Contains(t, last.Body, `"reason":"sample_fixture_title"`)
}

func TestExecuteBeadDecompositionRequiresCurrentParent(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	run := func(args ...string) (string, error) {
		return executeCommand(factory.NewRootCommand(), args...)
	}

	parentOut, err := run("bead", "create", "Parent with inherited metadata",
		"--type", "task",
		"--labels", "phase:build,area:work,kind:bug",
		"--set", "spec-id=IP-2026-06-13-server-managed-workers",
	)
	require.NoError(t, err)
	parentID := strings.TrimSpace(parentOut)
	require.NotEmpty(t, parentID)

	t.Setenv(agent.DDXModeEnvKey, agent.DDXModeBeadExecution)
	t.Setenv(agent.DDXBeadIDEnvKey, parentID)
	t.Setenv(agent.DDXAttemptIDEnvKey, "attempt-valid-child")

	childOut, err := run("bead", "create", "Implement bounded child slice",
		"--type", "task",
		"--parent", parentID,
		"--labels", "phase:build,area:work,kind:bug,child",
		"--set", "spec-id=IP-2026-06-13-server-managed-workers",
	)
	require.NoError(t, err)
	childID := strings.TrimSpace(childOut)
	require.NotEmpty(t, childID)

	store := factory.beadStore()
	child, err := store.Get(context.Background(), childID)
	require.NoError(t, err)
	require.NotNil(t, child)
	assert.Equal(t, parentID, child.Parent)
	assert.ElementsMatch(t, []string{"phase:build", "area:work", "kind:bug", "child"}, child.Labels)
	assert.Equal(t, "IP-2026-06-13-server-managed-workers", child.Extra["spec-id"])

	_, err = run("bead", "create", "Missing parent child",
		"--type", "task",
		"--labels", "phase:build,area:work,kind:bug",
		"--set", "spec-id=IP-2026-06-13-server-managed-workers",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing_current_parent")

	_, err = run("bead", "create", "Missing inherited label child",
		"--type", "task",
		"--parent", parentID,
		"--labels", "phase:build,area:work",
		"--set", "spec-id=IP-2026-06-13-server-managed-workers",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing_parent_labels:kind:bug")
}

func TestWorkStatusDropsStaleRunStateWhenWorkerPidGone(t *testing.T) {
	projectRoot := t.TempDir()
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{ID: "ddx-dead-runstate", Title: "Dead run-state"}))

	now := time.Now().UTC()
	require.NoError(t, agent.WriteRunState(projectRoot, agent.RunState{
		BeadID:      "ddx-dead-runstate",
		AttemptID:   "attempt-dead-runstate",
		PID:         definitelyDeadPID(),
		StartedAt:   now.Add(-time.Minute),
		RefreshedAt: now,
		ExpiresAt:   now.Add(time.Minute),
	}))

	factory := NewCommandFactory(projectRoot)
	factory.workerScannerOverride = fixedScanner{workers: nil}
	out, err := executeCommand(factory.NewRootCommand(), "work", "status", "--project", projectRoot, "--json")
	require.NoError(t, err)

	var report WorkStatusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.Zero(t, report.ActiveWork.Count)
	assert.NotContains(t, report.ActiveWork.BeadIDs, "ddx-dead-runstate")
}

func definitelyDeadPID() int {
	pid := os.Getpid() + 1000000
	for processAlive(pid) {
		pid++
	}
	return pid
}
