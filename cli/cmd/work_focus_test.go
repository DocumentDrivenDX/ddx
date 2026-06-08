package cmd

import (
	"context"
	"encoding/json"
	"path/filepath"
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

// setupWorkFocusEnv initializes a test environment and populates its bead store.
func setupWorkFocusEnv(t *testing.T, beads ...*bead.Bead) *TestEnvironment {
	t.Helper()
	env := NewTestEnvironment(t)
	store := bead.NewStore(filepath.Join(env.Dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	for _, b := range beads {
		require.NoError(t, store.Create(context.Background(), b))
	}
	return env
}

// TestWorkFocusReportsInterventionQueue verifies that proposed/operator-attention
// beads appear in human_required, and dependency-waiting beads appear
// in blocked_or_planning.
func TestWorkFocusReportsInterventionQueue(t *testing.T) {
	blocker := &bead.Bead{
		ID:    "ddx-focus-blocker",
		Title: "Blocker bead",
	}
	// Proposed bead (operator attention required).
	nhBead := &bead.Bead{
		ID:     "ddx-focus-nh",
		Title:  "Needs human attention",
		Status: bead.StatusProposed,
		Labels: []string{bead.LabelNeedsHuman},
		Extra: map[string]any{
			bead.ExtraNeedsHumanReason: "review returned terminal BLOCK",
		},
	}
	// Dependency-waiting bead (planning required before work can proceed).
	depBlocked := &bead.Bead{
		ID:    "ddx-focus-dep",
		Title: "Dep-blocked planning bead",
	}
	depBlocked.AddDep(blocker.ID, "blocks")

	env := setupWorkFocusEnv(t, blocker, nhBead, depBlocked)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "focus", "--json")
	require.NoError(t, err)

	var report WorkFocusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	// Proposed bead must appear in human_required.
	var foundNH bool
	for _, item := range report.HumanRequired {
		if item.ID == nhBead.ID {
			foundNH = true
			assert.Equal(t, "review returned terminal BLOCK", item.Reason)
		}
	}
	assert.True(t, foundNH, "proposed bead must appear in human_required")

	// Proposed bead must NOT appear in blocked_or_planning.
	for _, item := range report.BlockedOrPlanning {
		assert.NotEqual(t, nhBead.ID, item.ID, "proposed bead must not appear in blocked_or_planning")
	}

	// Dependency-waiting bead must appear in blocked_or_planning.
	var foundDep bool
	for _, item := range report.BlockedOrPlanning {
		if item.ID == depBlocked.ID {
			foundDep = true
			assert.Equal(t, bead.BlockerKindDependency, item.BlockerKind)
		}
	}
	assert.True(t, foundDep, "dependency-waiting bead must appear in blocked_or_planning")
}

// TestWorkFocusOmitsWorkerReadyBeadsByDefault verifies that worker-ready
// (execution-eligible) beads are NOT listed as primary intervention items.
// They must appear only as a depth count in ready_summary.
func TestWorkFocusOmitsWorkerReadyBeadsByDefault(t *testing.T) {
	readyBead := &bead.Bead{
		ID:    "ddx-focus-ready",
		Title: "Ready for worker",
	}
	nhBead := &bead.Bead{
		ID:     "ddx-focus-nh2",
		Title:  "Needs human",
		Status: bead.StatusProposed,
		Labels: []string{bead.LabelNeedsHuman},
	}

	env := setupWorkFocusEnv(t, readyBead, nhBead)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "focus", "--json")
	require.NoError(t, err)

	var report WorkFocusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	// Worker-ready bead must NOT appear in human_required.
	for _, item := range report.HumanRequired {
		assert.NotEqual(t, readyBead.ID, item.ID, "worker-ready bead must not be in human_required")
	}
	// Worker-ready bead must NOT appear in blocked_or_planning.
	for _, item := range report.BlockedOrPlanning {
		assert.NotEqual(t, readyBead.ID, item.ID, "worker-ready bead must not be in blocked_or_planning")
	}
	// The ready_summary must reflect the ready bead.
	assert.Equal(t, 1, report.ReadySummary.Count)
	assert.NotEmpty(t, report.ReadySummary.Depth)

	// The proposed bead must appear in human_required.
	var foundNH bool
	for _, item := range report.HumanRequired {
		if item.ID == nhBead.ID {
			foundNH = true
		}
	}
	assert.True(t, foundNH, "proposed bead must still appear in human_required")
}

// TestWorkFocusJSONIncludesUnknownHazards verifies that the JSON output's
// unknowns field surfaces capacity signals when in_progress beads are present.
func TestWorkFocusJSONIncludesUnknownHazards(t *testing.T) {
	// Create an in_progress bead by creating it as open then claiming it.
	b := &bead.Bead{
		ID:    "ddx-focus-ip",
		Title: "In-progress bead",
	}
	env := setupWorkFocusEnv(t, b)

	// Claim the bead to move it to in_progress.
	store := bead.NewStore(filepath.Join(env.Dir, ddxroot.DirName))
	require.NoError(t, store.Claim(b.ID, "test-worker"))

	root := NewCommandFactory(env.Dir).NewRootCommand()
	out, err := executeCommand(root, "work", "focus", "--json")
	require.NoError(t, err)

	var report WorkFocusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	// unknowns must be present and mention worker process liveness.
	require.NotEmpty(t, report.Unknowns, "unknowns must not be empty when in_progress beads exist")
	var foundLiveness bool
	for _, u := range report.Unknowns {
		if strings.Contains(u, "worker process liveness") || strings.Contains(u, "in_progress") {
			foundLiveness = true
		}
	}
	assert.True(t, foundLiveness, "unknowns must mention worker process liveness or in_progress count")
}

// TestWorkFocusRecommendsWorkerForDeepReadyQueue verifies the conservative
// worker-start suggestion when ready depth is high (>= 3) and no active
// workers are detected.
func TestWorkFocusRecommendsWorkerForDeepReadyQueue(t *testing.T) {
	var beads []*bead.Bead
	for i := 0; i < 4; i++ {
		beads = append(beads, &bead.Bead{
			ID:    "ddx-focus-deep-" + string(rune('a'+i)),
			Title: "Deep queue bead",
		})
	}

	env := setupWorkFocusEnv(t, beads...)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "focus", "--json")
	require.NoError(t, err)

	var report WorkFocusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	assert.Equal(t, 4, report.ReadySummary.Count)
	assert.Equal(t, "deep", report.ReadySummary.Depth)
	// Worker recommendation must suggest running ddx work.
	assert.Contains(t, report.WorkerRecommendation, "ddx work",
		"worker recommendation must suggest 'ddx work' for deep ready queue")
}

// TestWorkFocusEmptyQueueExitsSuccessfully verifies AC1: ddx work focus is
// read-only and exits successfully on an empty queue.
func TestWorkFocusEmptyQueueExitsSuccessfully(t *testing.T) {
	env := setupWorkFocusEnv(t) // no beads
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "focus")
	require.NoError(t, err)
	assert.Contains(t, out, "Operator attention")
	assert.Contains(t, out, "Blocked / planning")
	assert.Contains(t, out, "Worker-ready summary")
	assert.Contains(t, out, "Worker recommendation")
	assert.Contains(t, out, "Unknowns")
}

// TestWorkFocusReportsActiveLongRunningWorkerFromWorkerStatus verifies AC #5:
// a fresh worker-status sidecar (last_activity_at within LivenessTTL) is
// reported as an active worker by `ddx work focus`, even when the bead
// tracker's claim timestamp has not moved. Without the sidecar, the same
// bead would appear merely as in_progress + an "unknown liveness" hazard.
func TestWorkFocusReportsActiveLongRunningWorkerFromWorkerStatus(t *testing.T) {
	b := &bead.Bead{
		ID:    "ddx-focus-active-worker",
		Title: "In-progress bead with live worker",
	}
	env := setupWorkFocusEnv(t, b)

	store := bead.NewStore(filepath.Join(env.Dir, ddxroot.DirName))
	require.NoError(t, store.Claim(b.ID, "test-worker"))

	// Write a fresh worker status sidecar so the focus report can detect
	// the live worker without consulting the (stale-by-design) tracker
	// claim timestamp.
	workerID := "wkr-focus-active"
	require.NoError(t, workerstatus.WriteLiveness(env.Dir, workerID, workerstatus.LivenessRecord{
		WorkerID:       workerID,
		ProjectRoot:    env.Dir,
		CurrentBead:    b.ID,
		AttemptID:      "att-focus-001",
		Phase:          "running",
		Harness:        "claude",
		Model:          "opus",
		PID:            4242,
		LastActivityAt: time.Now().UTC(),
	}))

	root := NewCommandFactory(env.Dir).NewRootCommand()
	out, err := executeCommand(root, "work", "focus", "--json")
	require.NoError(t, err)

	var report WorkFocusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	require.Len(t, report.ActiveWorkers, 1,
		"focus must surface the live worker sidecar even when the tracker claim timestamp has not changed")
	assert.Equal(t, workerID, report.ActiveWorkers[0].WorkerID)
	assert.Equal(t, b.ID, report.ActiveWorkers[0].CurrentBead)
	assert.Equal(t, "att-focus-001", report.ActiveWorkers[0].AttemptID)
	assert.Equal(t, "running", report.ActiveWorkers[0].Phase)

	// The in_progress liveness "unknown" hazard must NOT be added when an
	// active sidecar is present — otherwise the focus report would
	// contradict itself.
	for _, u := range report.Unknowns {
		assert.NotContains(t, u, "worker process liveness",
			"active sidecar must suppress the in_progress liveness unknown")
	}
}

func TestWorkFocusActiveWorkerSummaryUsesSharedSnapshot(t *testing.T) {
	projectRoot := t.TempDir()
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))

	freshSidecar := &bead.Bead{ID: "ddx-focus-summary-sidecar", Title: "Sidecar bead"}
	freshRunState := &bead.Bead{ID: "ddx-focus-summary-runstate", Title: "Run-state bead"}
	require.NoError(t, store.Create(context.Background(), freshSidecar))
	require.NoError(t, store.Create(context.Background(), freshRunState))

	now := time.Now().UTC()
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, "worker-focus-summary", workerstatus.LivenessRecord{
		WorkerID:       "worker-focus-summary",
		ProjectRoot:    projectRoot,
		CurrentBead:    freshSidecar.ID,
		AttemptID:      "att-focus-summary-sidecar",
		Phase:          "running",
		LastActivityAt: now,
	}))
	require.NoError(t, agent.WriteRunState(projectRoot, agent.RunState{
		BeadID:      freshRunState.ID,
		AttemptID:   "att-focus-summary-runstate",
		PID:         4243,
		StartedAt:   now.Add(-time.Minute),
		RefreshedAt: now,
		ExpiresAt:   now.Add(time.Minute),
	}))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "work", "focus", "--json")
	require.NoError(t, err)

	var report WorkFocusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	require.Equal(t, 2, report.ActiveWork.Count, "shared snapshot must include fresh sidecars and run-state entries")
	assert.Contains(t, report.ActiveWork.BeadIDs, freshSidecar.ID)
	assert.Contains(t, report.ActiveWork.BeadIDs, freshRunState.ID)
	require.Len(t, report.ActiveWorkers, 2)
}

// TestWorkFocusJSONStableKeys verifies AC3: --json returns the stable keys.
func TestWorkFocusJSONStableKeys(t *testing.T) {
	env := setupWorkFocusEnv(t)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "focus", "--json")
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(out), &raw))

	for _, key := range []string{"human_required", "blocked_or_planning", "ready_summary", "active_work", "worker_recommendation", "unknowns"} {
		assert.Contains(t, raw, key, "JSON output must contain stable key: %s", key)
	}
}
