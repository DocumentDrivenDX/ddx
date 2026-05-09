package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupWorkFocusEnv initializes a test environment and populates its bead store.
func setupWorkFocusEnv(t *testing.T, beads ...*bead.Bead) *TestEnvironment {
	t.Helper()
	env := NewTestEnvironment(t)
	store := bead.NewStore(filepath.Join(env.Dir, ".ddx"))
	require.NoError(t, store.Init())
	for _, b := range beads {
		require.NoError(t, store.Create(b))
	}
	return env
}

// TestWorkFocusReportsInterventionQueue verifies that proposed/operator-attention and
// review-blocked beads appear in human_required, and dep-blocked beads appear
// in blocked_or_planning.
func TestWorkFocusReportsInterventionQueue(t *testing.T) {
	blocker := &bead.Bead{
		ID:    "ddx-focus-blocker",
		Title: "Blocker bead",
	}
	// needs_human bead (operator attention required).
	nhBead := &bead.Bead{
		ID:     "ddx-focus-nh",
		Title:  "Needs human attention",
		Status: bead.StatusProposed,
		Labels: []string{bead.LabelNeedsHuman},
		Extra: map[string]any{
			bead.ExtraNeedsHumanReason: "review returned terminal BLOCK",
		},
	}
	// Dep-blocked bead (planning required before work can proceed).
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

	// needs_human bead must appear in human_required.
	var foundNH bool
	for _, item := range report.HumanRequired {
		if item.ID == nhBead.ID {
			foundNH = true
			assert.Equal(t, "review returned terminal BLOCK", item.Reason)
		}
	}
	assert.True(t, foundNH, "needs_human bead must appear in human_required")

	// needs_human bead must NOT appear in blocked_or_planning.
	for _, item := range report.BlockedOrPlanning {
		assert.NotEqual(t, nhBead.ID, item.ID, "needs_human bead must not appear in blocked_or_planning")
	}

	// Dep-blocked bead must appear in blocked_or_planning.
	var foundDep bool
	for _, item := range report.BlockedOrPlanning {
		if item.ID == depBlocked.ID {
			foundDep = true
			assert.Equal(t, bead.BlockerKindDependency, item.BlockerKind)
		}
	}
	assert.True(t, foundDep, "dep-blocked bead must appear in blocked_or_planning")
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

	// The needs_human bead must appear in human_required.
	var foundNH bool
	for _, item := range report.HumanRequired {
		if item.ID == nhBead.ID {
			foundNH = true
		}
	}
	assert.True(t, foundNH, "needs_human bead must still appear in human_required")
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
	store := bead.NewStore(filepath.Join(env.Dir, ".ddx"))
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
	assert.Contains(t, out, "Requires human")
	assert.Contains(t, out, "Blocked / planning")
	assert.Contains(t, out, "Worker-ready summary")
	assert.Contains(t, out, "Worker recommendation")
	assert.Contains(t, out, "Unknowns")
}

// TestWorkFocusJSONStableKeys verifies AC3: --json returns the five stable keys.
func TestWorkFocusJSONStableKeys(t *testing.T) {
	env := setupWorkFocusEnv(t)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "focus", "--json")
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(out), &raw))

	for _, key := range []string{"human_required", "blocked_or_planning", "ready_summary", "worker_recommendation", "unknowns"} {
		assert.Contains(t, raw, key, "JSON output must contain stable key: %s", key)
	}
}
