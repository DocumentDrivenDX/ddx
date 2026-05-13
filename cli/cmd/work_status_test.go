package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkStatusDefaultsToCurrentProjectOnly proves the default scope of
// `ddx work status --json` is the requested project root, not every live
// ddx worker on the host. This is the regression test for the originating
// incident: a global `ps | grep ddx work` scan answered "is project A's
// worker running?" with a worker that actually belonged to project B.
func TestWorkStatusDefaultsToCurrentProjectOnly(t *testing.T) {
	projectA := t.TempDir()
	projectB := t.TempDir()

	scannerWorkers := []workerstatus.LiveWorker{
		{
			PID:         101,
			Command:     "ddx work --once --project " + projectA,
			ProjectRoot: projectA,
			StartedAt:   time.Now().Add(-10 * time.Minute).UTC(),
			Age:         "10m",
			AgeSeconds:  600,
		},
		{
			PID:         202,
			Command:     "ddx work --once --project " + projectB,
			ProjectRoot: projectB,
			StartedAt:   time.Now().Add(-3 * time.Minute).UTC(),
			Age:         "3m",
			AgeSeconds:  180,
		},
	}

	factory := NewCommandFactory(projectA)
	factory.workerScannerOverride = fixedScanner{workers: scannerWorkers}
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "work", "status", "--project", projectA, "--json")
	require.NoError(t, err)

	var report WorkStatusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	assert.Equal(t, "project", report.Scope)
	assert.Equal(t, projectA, report.ProjectRoot)
	require.Len(t, report.Workers, 1, "default scope must filter to requested project only; got %s", out)
	assert.Equal(t, 101, report.Workers[0].PID)
	assert.Equal(t, projectA, report.Workers[0].ProjectRoot)
}

// TestWorkStatusAllProjectsShowsExplicitGlobalView proves --all-projects
// returns every live worker (labelled with their own project_root) so an
// operator can explicitly opt into the cross-project view.
func TestWorkStatusAllProjectsShowsExplicitGlobalView(t *testing.T) {
	projectA := t.TempDir()
	projectB := t.TempDir()

	scannerWorkers := []workerstatus.LiveWorker{
		{PID: 11, Command: "ddx work", ProjectRoot: projectA, StartedAt: time.Now().Add(-1 * time.Minute)},
		{PID: 22, Command: "ddx try ddx-abc12345", ProjectRoot: projectB, StartedAt: time.Now().Add(-2 * time.Minute)},
	}

	factory := NewCommandFactory(projectA)
	factory.workerScannerOverride = fixedScanner{workers: scannerWorkers}
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "work", "status", "--project", projectA, "--all-projects", "--json")
	require.NoError(t, err)

	var report WorkStatusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	assert.Equal(t, "all-projects", report.Scope)
	require.Len(t, report.Workers, 2)
	projects := make([]string, 0, 2)
	for _, w := range report.Workers {
		projects = append(projects, w.ProjectRoot)
		assert.NotEmpty(t, w.ProjectRoot, "every worker entry must carry its own project_root")
	}
	assert.Contains(t, projects, projectA)
	assert.Contains(t, projects, projectB)
}

// TestWorkStatusNoLiveWorkersNamesProjectRoot proves the empty state
// names the resolved project root rather than falling back to silence or
// to unrelated processes. This is the exact failure surface the bead
// originated from: an empty answer must not be answered with another
// project's process.
func TestWorkStatusNoLiveWorkersNamesProjectRoot(t *testing.T) {
	projectA := t.TempDir()
	projectB := t.TempDir()

	scannerWorkers := []workerstatus.LiveWorker{
		{PID: 999, Command: "ddx work --project " + projectB, ProjectRoot: projectB, StartedAt: time.Now()},
	}

	factory := NewCommandFactory(projectA)
	factory.workerScannerOverride = fixedScanner{workers: scannerWorkers}
	root := factory.NewRootCommand()

	// Text output
	textOut, err := executeCommand(root, "work", "status", "--project", projectA)
	require.NoError(t, err)
	assert.Contains(t, textOut, "No live ddx workers for project")
	assert.Contains(t, textOut, projectA,
		"empty state must name the resolved project root, not fall back to unrelated workers")
	assert.NotContains(t, textOut, projectB,
		"empty state must not surface another project's worker as evidence")

	// JSON output for the same condition
	jsonOut, err := executeCommand(root, "work", "status", "--project", projectA, "--json")
	require.NoError(t, err)
	var report WorkStatusReport
	require.NoError(t, json.Unmarshal([]byte(jsonOut), &report))
	assert.Equal(t, projectA, report.ProjectRoot)
	assert.Empty(t, report.Workers)
}

// TestWorkStatusInfersBeadAndExecutionWorktree proves a live inline
// `ddx work` / `ddx try` process exposes pid, age, command, project_root,
// and the active bead ID when the command line or execution worktree
// contains one.
func TestWorkStatusInfersBeadAndExecutionWorktree(t *testing.T) {
	projectA := t.TempDir()

	worktree := "/tmp/ddx-exec-wt/.execute-bead-wt-ddx-c3219628-20260513T231309-0e6f776b"
	cmdline := "ddx work --once --project " + projectA
	beadID, inferredWorktree := workerstatus.InferBead(cmdline, worktree)
	require.Equal(t, "ddx-c3219628", beadID,
		"InferBead must extract the bead id from an execute-bead worktree path")
	require.Equal(t, worktree, inferredWorktree)

	scannerWorkers := []workerstatus.LiveWorker{
		{
			PID:               12345,
			Command:           cmdline,
			ProjectRoot:       projectA,
			StartedAt:         time.Now().Add(-90 * time.Second).UTC(),
			Age:               "1m",
			AgeSeconds:        90,
			BeadID:            beadID,
			ExecutionWorktree: inferredWorktree,
			Cwd:               worktree,
		},
	}

	factory := NewCommandFactory(projectA)
	factory.workerScannerOverride = fixedScanner{workers: scannerWorkers}
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "work", "status", "--project", projectA, "--json")
	require.NoError(t, err)

	var report WorkStatusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	require.Len(t, report.Workers, 1)
	w := report.Workers[0]
	assert.Equal(t, 12345, w.PID)
	assert.NotZero(t, w.AgeSeconds, "age must be exposed")
	assert.NotEmpty(t, w.Age, "human-readable age must be set")
	assert.Equal(t, cmdline, w.Command)
	assert.Equal(t, projectA, w.ProjectRoot)
	assert.Equal(t, "ddx-c3219628", w.BeadID)
	assert.Equal(t, worktree, w.ExecutionWorktree)
}

// TestWorkStatus_EmptyAllProjectsMessage verifies the all-projects empty
// state speaks about the host rather than naming a project root (which
// would be misleading when the user opted into the cross-project view).
func TestWorkStatus_EmptyAllProjectsMessage(t *testing.T) {
	factory := NewCommandFactory(t.TempDir())
	factory.workerScannerOverride = fixedScanner{workers: nil}
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "work", "status", "--all-projects")
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, "No live ddx workers found on this host."),
		"expected host-scoped empty message, got %q", out)
}

// TestWorkStatusFlagsArePresent guards the public flag surface.
func TestWorkStatusFlagsArePresent(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	statusCmd, _, err := root.Find([]string{"work", "status"})
	require.NoError(t, err, "ddx work status must exist")
	require.NotNil(t, statusCmd.Flags().Lookup("project"))
	require.NotNil(t, statusCmd.Flags().Lookup("all-projects"))
	require.NotNil(t, statusCmd.Flags().Lookup("json"))
}
