package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedScanner struct {
	workers []workerstatus.LiveWorker
}

func (s fixedScanner) Scan(_ context.Context) ([]workerstatus.LiveWorker, error) {
	return s.workers, nil
}

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

func TestWorkStatusEnrichesFreshSamePIDLivenessDespiteStartSkew(t *testing.T) {
	projectRoot := t.TempDir()
	procStartedAt := time.Now().Add(-2 * time.Minute).UTC()
	const pid = 4343

	scannerWorkers := []workerstatus.LiveWorker{
		{
			PID:         pid,
			Command:     "ddx work --watch --project " + projectRoot,
			ProjectRoot: projectRoot,
			StartedAt:   procStartedAt,
			Age:         "2m",
			AgeSeconds:  120,
		},
	}
	// The sidecar's started_at is skewed >10s from the process scanner's start
	// time. Previously matchingLivenessRecord discarded the fresh same-PID
	// sidecar over this skew, erasing the operator-facing bead/attempt fields
	// (ddx-f9b41107). A single fresh same-PID sidecar must now be honored.
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, "worker-skew", workerstatus.LivenessRecord{
		WorkerID:       "worker-skew",
		ProjectRoot:    projectRoot,
		CurrentBead:    "ddx-skew0001",
		AttemptID:      "20260518T015704-18b08637",
		Phase:          "running",
		PID:            pid,
		StartedAt:      procStartedAt.Add(-18 * time.Second),
		LastActivityAt: time.Now().UTC(),
	}))

	factory := NewCommandFactory(projectRoot)
	factory.workerScannerOverride = fixedScanner{workers: scannerWorkers}
	root := factory.NewRootCommand()

	textOut, err := executeCommand(root, "work", "status", "--project", projectRoot)
	require.NoError(t, err)

	assert.NotContains(t, textOut, "bead=-")
	assert.NotContains(t, textOut, "attempt=-")
	assert.Contains(t, textOut, "bead=ddx-skew0001")
	assert.Contains(t, textOut, "attempt=20260518T015704-18b08637")
}

func TestWorkStatusUsesRunStateWhenLivenessAttemptIsMissingOrStale(t *testing.T) {
	projectRoot := t.TempDir()
	procStartedAt := time.Now().Add(-2 * time.Minute).UTC()
	const pid = 5454
	worktree := filepath.Join(t.TempDir(), ".execute-bead-wt-20260518T015727-6716231e")

	scannerWorkers := []workerstatus.LiveWorker{
		{
			PID:         pid,
			Command:     "ddx work --watch --project " + projectRoot,
			ProjectRoot: projectRoot,
			StartedAt:   procStartedAt,
			Age:         "2m",
			AgeSeconds:  120,
		},
	}
	// No liveness sidecar is written, so liveness enrichment supplies nothing.
	// A fresh per-attempt run-state record for the same PID must fill
	// bead/attempt/worktree (ddx-f9b41107).
	require.NoError(t, agent.WriteRunState(projectRoot, agent.RunState{
		BeadID:       "ddx-runstate01",
		AttemptID:    "20260518T015727-6716231e",
		WorktreePath: worktree,
		PID:          pid,
		StartedAt:    procStartedAt,
		RefreshedAt:  time.Now().UTC(),
		ExpiresAt:    time.Now().Add(2 * time.Minute).UTC(),
	}))

	factory := NewCommandFactory(projectRoot)
	factory.workerScannerOverride = fixedScanner{workers: scannerWorkers}
	root := factory.NewRootCommand()

	jsonOut, err := executeCommand(root, "work", "status", "--project", projectRoot, "--json")
	require.NoError(t, err)

	var report WorkStatusReport
	require.NoError(t, json.Unmarshal([]byte(jsonOut), &report))
	require.Len(t, report.Workers, 1)
	assert.Equal(t, "ddx-runstate01", report.Workers[0].BeadID)
	assert.Equal(t, "20260518T015727-6716231e", report.Workers[0].AttemptID)
	assert.Equal(t, worktree, report.Workers[0].ExecutionWorktree)
}

func TestWorkStatusUsesFreshLivenessSidecarForActiveAttempt(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process cwd inspection for sidecar child pid is linux-only")
	}

	projectRoot := t.TempDir()
	worktree := filepath.Join(t.TempDir(), ".execute-bead-wt-ddx-aabbccdd-20260515T120840-2c0aa694")
	require.NoError(t, os.MkdirAll(worktree, 0o755))

	child := exec.Command("sleep", "30")
	child.Dir = worktree
	require.NoError(t, child.Start())
	t.Cleanup(func() {
		if child.Process != nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	})

	startedAt := time.Now().Add(-2 * time.Minute).UTC()
	parentPID := 4242
	scannerWorkers := []workerstatus.LiveWorker{
		{
			PID:         parentPID,
			Command:     "ddx work --watch --project " + projectRoot,
			ProjectRoot: projectRoot,
			StartedAt:   startedAt,
			Age:         "2m",
			AgeSeconds:  120,
		},
	}
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, "worker-active", workerstatus.LivenessRecord{
		WorkerID:       "worker-active",
		ProjectRoot:    projectRoot,
		CurrentBead:    "ddx-aabbccdd",
		AttemptID:      "20260515T120840-2c0aa694",
		Phase:          "running",
		PID:            parentPID,
		ChildPID:       child.Process.Pid,
		StartedAt:      startedAt,
		LastActivityAt: time.Now().UTC(),
	}))

	factory := NewCommandFactory(projectRoot)
	factory.workerScannerOverride = fixedScanner{workers: scannerWorkers}
	root := factory.NewRootCommand()

	textOut, err := executeCommand(root, "work", "status", "--project", projectRoot)
	require.NoError(t, err)

	assert.Contains(t, textOut, "bead=ddx-aabbccdd")
	assert.Contains(t, textOut, "attempt=20260515T120840-2c0aa694")
	assert.Contains(t, textOut, "worktree="+worktree)
	assert.NotContains(t, textOut, "bead=-")

	jsonOut, err := executeCommand(factory.NewRootCommand(), "work", "status", "--project", projectRoot, "--json")
	require.NoError(t, err)

	var report WorkStatusReport
	require.NoError(t, json.Unmarshal([]byte(jsonOut), &report))
	require.Len(t, report.Workers, 1)
	assert.Equal(t, "ddx-aabbccdd", report.Workers[0].BeadID)
	assert.Equal(t, "20260515T120840-2c0aa694", report.Workers[0].AttemptID)
	assert.Equal(t, "running", report.Workers[0].Phase)
	assert.Equal(t, child.Process.Pid, report.Workers[0].ChildPID)
	assert.Equal(t, worktree, report.Workers[0].ExecutionWorktree)
	assert.False(t, report.Workers[0].LastActivityAt.IsZero())
}

func TestWorkStatusAllProjectsEnrichesActiveBeads(t *testing.T) {
	projectA := t.TempDir()
	projectB := t.TempDir()
	now := time.Now().UTC()

	scannerWorkers := []workerstatus.LiveWorker{
		{
			PID:         1001,
			Command:     "ddx work --watch --project " + projectA,
			ProjectRoot: projectA,
			StartedAt:   now.Add(-5 * time.Minute),
			Age:         "5m",
			AgeSeconds:  300,
		},
		{
			PID:         2002,
			Command:     "ddx work --watch --project " + projectB,
			ProjectRoot: projectB,
			StartedAt:   now.Add(-3 * time.Minute),
			Age:         "3m",
			AgeSeconds:  180,
		},
	}

	require.NoError(t, workerstatus.WriteLiveness(projectA, "worker-a", workerstatus.LivenessRecord{
		WorkerID:       "worker-a",
		ProjectRoot:    projectA,
		CurrentBead:    "ddx-aaaabbbb",
		AttemptID:      "20260515T120840-a1",
		Phase:          "running",
		PID:            1001,
		StartedAt:      scannerWorkers[0].StartedAt,
		LastActivityAt: now,
	}))
	require.NoError(t, workerstatus.WriteLiveness(projectB, "worker-b", workerstatus.LivenessRecord{
		WorkerID:       "worker-b",
		ProjectRoot:    projectB,
		CurrentBead:    "ddx-ccccdddd",
		AttemptID:      "20260515T121500-b2",
		Phase:          "running",
		PID:            2002,
		StartedAt:      scannerWorkers[1].StartedAt,
		LastActivityAt: now,
	}))

	factory := NewCommandFactory(projectA)
	factory.workerScannerOverride = fixedScanner{workers: scannerWorkers}
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "work", "status", "--project", projectA, "--all-projects", "--json")
	require.NoError(t, err)

	var report WorkStatusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.Equal(t, "all-projects", report.Scope)
	require.Len(t, report.Workers, 2)

	byProject := make(map[string]workerstatus.LiveWorker, len(report.Workers))
	for _, w := range report.Workers {
		byProject[w.ProjectRoot] = w
	}
	require.Contains(t, byProject, projectA)
	require.Contains(t, byProject, projectB)
	assert.Equal(t, "ddx-aaaabbbb", byProject[projectA].BeadID)
	assert.Equal(t, "20260515T120840-a1", byProject[projectA].AttemptID)
	assert.Equal(t, "ddx-ccccdddd", byProject[projectB].BeadID)
	assert.Equal(t, "20260515T121500-b2", byProject[projectB].AttemptID)
}

func TestWorkStatusIgnoresStaleLivenessSidecar(t *testing.T) {
	projectRoot := t.TempDir()
	startedAt := time.Now().Add(-2 * time.Minute).UTC()

	scannerWorkers := []workerstatus.LiveWorker{
		{
			PID:         5151,
			Command:     "ddx work --watch --project " + projectRoot,
			ProjectRoot: projectRoot,
			StartedAt:   startedAt,
			Age:         "2m",
			AgeSeconds:  120,
		},
	}
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, "worker-stale", workerstatus.LivenessRecord{
		WorkerID:       "worker-stale",
		ProjectRoot:    projectRoot,
		CurrentBead:    "ddx-deadbeef",
		AttemptID:      "20260515T115959-stale",
		Phase:          "running",
		PID:            5151,
		StartedAt:      startedAt,
		LastActivityAt: time.Now().Add(-workerstatus.LivenessTTL - time.Second).UTC(),
	}))

	factory := NewCommandFactory(projectRoot)
	factory.workerScannerOverride = fixedScanner{workers: scannerWorkers}
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "work", "status", "--project", projectRoot, "--json")
	require.NoError(t, err)

	var report WorkStatusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	require.Len(t, report.Workers, 1)
	assert.Empty(t, report.Workers[0].BeadID, "stale sidecars must not make an idle parent look active")
	assert.Empty(t, report.Workers[0].AttemptID, "stale sidecars must not contribute attempt metadata")
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

func TestWorkStatusReportsServerUnavailableState(t *testing.T) {
	projectRoot := t.TempDir()
	scannerWorkers := []workerstatus.LiveWorker{
		{
			PID:         9090,
			Command:     "ddx work --watch --project " + projectRoot,
			ProjectRoot: projectRoot,
			StartedAt:   time.Now().Add(-90 * time.Second).UTC(),
			Age:         "1m30s",
			AgeSeconds:  90,
			Phase:       "server.unavailable",
			Message:     "server unreachable: holding queue until /api/health returns",
		},
	}

	factory := NewCommandFactory(projectRoot)
	factory.workerScannerOverride = fixedScanner{workers: scannerWorkers}
	root := factory.NewRootCommand()

	textOut, err := executeCommand(root, "work", "status", "--project", projectRoot)
	require.NoError(t, err)
	assert.Contains(t, textOut, "phase=server.unavailable")
	assert.Contains(t, textOut, `message="server unreachable: holding queue until /api/health returns"`)

	jsonOut, err := executeCommand(root, "work", "status", "--project", projectRoot, "--json")
	require.NoError(t, err)
	var report WorkStatusReport
	require.NoError(t, json.Unmarshal([]byte(jsonOut), &report))
	require.Len(t, report.Workers, 1)
	assert.Equal(t, "server.unavailable", report.Workers[0].Phase)
	assert.Equal(t, "server unreachable: holding queue until /api/health returns", report.Workers[0].Message)
}

func TestWorkStatusActiveWorkerSummaryMatchesBeadStatus(t *testing.T) {
	projectRoot := t.TempDir()
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))

	sidecarBead := &bead.Bead{ID: "ddx-work-status-sidecar", Title: "Sidecar active bead"}
	runStateBead := &bead.Bead{ID: "ddx-work-status-runstate", Title: "Run-state active bead"}
	require.NoError(t, store.Create(context.Background(), sidecarBead))
	require.NoError(t, store.Create(context.Background(), runStateBead))

	now := time.Now().UTC()
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, "worker-status-sidecar", workerstatus.LivenessRecord{
		WorkerID:       "worker-status-sidecar",
		ProjectRoot:    projectRoot,
		CurrentBead:    sidecarBead.ID,
		AttemptID:      "att-status-sidecar",
		Phase:          "running",
		PID:            os.Getpid(),
		LastActivityAt: now,
	}))
	require.NoError(t, agent.WriteRunState(projectRoot, agent.RunState{
		BeadID:      runStateBead.ID,
		AttemptID:   "att-status-runstate",
		PID:         os.Getpid(),
		StartedAt:   now.Add(-time.Minute),
		RefreshedAt: now,
		ExpiresAt:   now.Add(time.Minute),
	}))

	factory := NewCommandFactory(projectRoot)
	factory.workerScannerOverride = fixedScanner{workers: nil}
	root := factory.NewRootCommand()

	workStatusOut, err := executeCommand(root, "work", "status", "--project", projectRoot, "--json")
	require.NoError(t, err)

	var workStatus WorkStatusReport
	require.NoError(t, json.Unmarshal([]byte(workStatusOut), &workStatus))
	require.Equal(t, 2, workStatus.ActiveWork.Count)
	assert.Contains(t, workStatus.ActiveWork.BeadIDs, sidecarBead.ID)
	assert.Contains(t, workStatus.ActiveWork.BeadIDs, runStateBead.ID)

	beadStatusOut, err := executeCommand(NewCommandFactory(projectRoot).NewRootCommand(), "bead", "status", "--json")
	require.NoError(t, err)

	var beadStatus BeadStatusReport
	require.NoError(t, json.Unmarshal([]byte(beadStatusOut), &beadStatus))
	require.Equal(t, 2, beadStatus.ActiveWork.Count)
	assert.Contains(t, beadStatus.ActiveWork.BeadIDs, sidecarBead.ID)
	assert.Contains(t, beadStatus.ActiveWork.BeadIDs, runStateBead.ID)
	assert.Equal(t, workStatus.ActiveWork.Count, beadStatus.ActiveWork.Count)
	assert.ElementsMatch(t, workStatus.ActiveWork.BeadIDs, beadStatus.ActiveWork.BeadIDs)
}

func TestWorkerStatusReportsProviderChildren(t *testing.T) {
	projectRoot := t.TempDir()
	procStartedAt := time.Now().Add(-2 * time.Minute).UTC()
	const pid = 7676

	scannerWorkers := []workerstatus.LiveWorker{{
		PID:         pid,
		Command:     "ddx work --watch --project " + projectRoot,
		ProjectRoot: projectRoot,
		StartedAt:   procStartedAt,
		Age:         "2m",
		AgeSeconds:  120,
	}}
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, "worker-pc", workerstatus.LivenessRecord{
		WorkerID:    "worker-pc",
		ProjectRoot: projectRoot,
		CurrentBead: "ddx-pc000001",
		AttemptID:   "20260613T202003-18b8bd7e",
		Phase:       "running",
		Route:       "claude/sonnet",
		Harness:     "claude",
		PID:         pid,
		StartedAt:   procStartedAt,
		ProviderChildren: []workerstatus.ProviderChild{
			{PID: 1882389, Provider: "claude", Harness: "claude", RouteOwner: "claude/sonnet", Phase: "running", AgeSeconds: 42},
		},
		LastActivityAt: time.Now().UTC(),
	}))

	factory := NewCommandFactory(projectRoot)
	factory.workerScannerOverride = fixedScanner{workers: scannerWorkers}
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "work", "status", "--project", projectRoot, "--json")
	require.NoError(t, err)

	var report WorkStatusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	require.Len(t, report.Workers, 1)
	children := report.Workers[0].ProviderChildren
	require.Len(t, children, 1, "provider_children must be surfaced; got %s", out)
	child := children[0]
	assert.Equal(t, 1882389, child.PID)
	assert.Equal(t, "claude", child.Provider)
	assert.Equal(t, "claude", child.Harness)
	assert.Equal(t, "claude/sonnet", child.RouteOwner)
	assert.Equal(t, "running", child.Phase)
	assert.Equal(t, float64(42), child.AgeSeconds)
	assert.Contains(t, out, "provider_children")
}
