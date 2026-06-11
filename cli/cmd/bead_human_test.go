package cmd

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupBeadHumanEnv(t *testing.T, beads ...*bead.Bead) (string, *CommandFactory, *bead.Store) {
	t.Helper()
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	store := bead.NewStore(filepath.Join(workingDir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	for _, b := range beads {
		require.NoError(t, store.Create(context.Background(), b))
	}
	return workingDir, factory, store
}

func TestBeadNeedsHumanCommand_JSON(t *testing.T) {
	meta := bead.NeedsHumanMeta{
		Reason:          "review blocked",
		Since:           "2026-05-09T00:00:00Z",
		Source:          "pre-close review",
		SuggestedAction: "retry",
		Summary:         "review found missing tests",
	}
	nh := &bead.Bead{
		ID:       "ddx-needs-human-json",
		Title:    "Needs operator",
		Priority: 0,
		Status:   bead.StatusProposed,
		Labels:   []string{"area:cli"},
	}
	plain := &bead.Bead{ID: "ddx-plain-json", Title: "Plain"}
	_, factory, store := setupBeadHumanEnv(t, nh, plain)
	require.NoError(t, store.Update(context.Background(), nh.ID, func(b *bead.Bead) {
		bead.SetNeedsHumanMeta(b, meta)
	}))

	out, err := executeCommand(factory.NewRootCommand(), "bead", "needs-human", "--json")
	require.NoError(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, nh.ID, rows[0]["id"])
	assert.Equal(t, float64(0), rows[0]["priority"])
	assert.Equal(t, nh.Title, rows[0]["title"])
	assert.Equal(t, meta.Reason, rows[0]["reason"])
	assert.Equal(t, meta.Since, rows[0]["since"])
	assert.Equal(t, meta.Source, rows[0]["source"])
	assert.Equal(t, meta.SuggestedAction, rows[0]["suggested_action"])
	assert.Equal(t, meta.Summary, rows[0]["summary"])
	assert.NotContains(t, rows[0]["labels"], bead.LabelNeedsHuman)
}

func TestBeadNeedsHumanCommand_Text(t *testing.T) {
	nh := &bead.Bead{
		ID:       "ddx-needs-human-text",
		Title:    "Needs text output",
		Priority: 1,
		Status:   bead.StatusProposed,
	}
	_, factory, store := setupBeadHumanEnv(t, nh)
	require.NoError(t, store.Update(context.Background(), nh.ID, func(b *bead.Bead) {
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{Reason: "operator decision required"})
	}))

	out, err := executeCommand(factory.NewRootCommand(), "bead", "needs-human")
	require.NoError(t, err)
	assert.Contains(t, out, nh.ID)
	assert.Contains(t, out, "P1")
	assert.Contains(t, out, "Needs text output")
	assert.Contains(t, out, "operator decision required")
}

func TestBeadReadyExcludesProposedByDefault(t *testing.T) {
	ready := &bead.Bead{ID: "ddx-ready-normal", Title: "Normal ready"}
	proposed := &bead.Bead{ID: "ddx-ready-human", Title: "Human ready", Status: bead.StatusProposed}
	_, factory, _ := setupBeadHumanEnv(t, ready, proposed)

	out, err := executeCommand(factory.NewRootCommand(), "bead", "ready", "--json")
	require.NoError(t, err)
	assertReadyIDs(t, out, []string{ready.ID})

	out, err = executeCommand(factory.NewRootCommand(), "bead", "ready", "--include-human", "--json")
	require.NoError(t, err)
	assertReadyIDs(t, out, []string{ready.ID})
}

func TestBeadStatusIncludesSixLifecycleStates(t *testing.T) {
	ready := &bead.Bead{ID: "ddx-status-ready", Title: "Ready"}
	dependencyRoot := &bead.Bead{ID: "ddx-status-dep-root", Title: "Dependency root"}
	dependencyWaiting := &bead.Bead{ID: "ddx-status-dep-wait", Title: "Dependency waiting"}
	dependencyWaiting.AddDep(dependencyRoot.ID, "blocks")
	inProgress := &bead.Bead{ID: "ddx-status-in-progress", Title: "In progress"}
	externalBlocked := &bead.Bead{ID: "ddx-status-blocked", Title: "Blocked"}
	proposed := &bead.Bead{ID: "ddx-status-proposed", Title: "Proposed", Status: bead.StatusProposed}
	closed := &bead.Bead{ID: "ddx-status-closed", Title: "Closed", Status: bead.StatusClosed}
	cancelled := &bead.Bead{ID: "ddx-status-cancelled", Title: "Cancelled", Status: bead.StatusCancelled}
	_, factory, store := setupBeadHumanEnv(t, ready, dependencyRoot, dependencyWaiting, inProgress, externalBlocked, proposed, closed, cancelled)
	require.NoError(t, store.Claim(inProgress.ID, "worker"))
	require.NoError(t, store.UpdateWithLifecycleStatus(externalBlocked.ID, bead.StatusBlocked, bead.LifecycleTransitionOptions{
		ExternalBlockerReason: "waiting on upstream release",
		Reason:                "test external blocker",
		Source:                "test",
	}, nil))

	text, err := executeCommand(factory.NewRootCommand(), "bead", "status")
	require.NoError(t, err)
	assert.Contains(t, text, "Open:")
	assert.Contains(t, text, "Lifecycle in progress:")
	assert.Contains(t, text, "Closed:")
	assert.Contains(t, text, "Blocked:")
	assert.Contains(t, text, "Proposed:")
	assert.Contains(t, text, "Cancelled:")
	assert.Contains(t, text, "Operator attention:")
	assert.Contains(t, text, "Worker ready:")
	assert.Contains(t, text, "Active workers:     1")

	out, err := executeCommand(factory.NewRootCommand(), "bead", "status", "--json")
	require.NoError(t, err)
	var counts BeadStatusReport
	require.NoError(t, json.Unmarshal([]byte(out), &counts))
	assert.Equal(t, 8, counts.Total)
	assert.Equal(t, 3, counts.Open)
	assert.Equal(t, 1, counts.InProgress)
	assert.Equal(t, 1, counts.Closed)
	assert.Equal(t, 1, counts.Blocked)
	assert.Equal(t, 1, counts.Proposed)
	assert.Equal(t, 1, counts.Cancelled)
	assert.Equal(t, 1, counts.NeedsHuman)
	assert.Equal(t, 1, counts.OperatorAttention)
	assert.Equal(t, 2, counts.Ready)
	assert.Equal(t, 2, counts.WorkerReady)
	assert.Equal(t, 1, counts.DependencyWaiting)
	assert.Equal(t, 1, counts.ExternalBlocked)
	assert.Equal(t, 1, counts.ActiveWork.Count)
	assert.Contains(t, counts.ActiveWork.BeadIDs, inProgress.ID)
}

func TestBeadStatusCountsFreshActiveWorkers(t *testing.T) {
	workingDir, factory, _ := setupBeadHumanEnv(t,
		&bead.Bead{ID: "ddx-status-active-a", Title: "Active A"},
		&bead.Bead{ID: "ddx-status-active-b", Title: "Active B"},
	)
	projectRoot := workingDir

	require.NoError(t, workerstatus.WriteLiveness(projectRoot, "worker-active-a", workerstatus.LivenessRecord{
		WorkerID:       "worker-active-a",
		ProjectRoot:    projectRoot,
		CurrentBead:    "ddx-status-active-a",
		AttemptID:      "att-active-a",
		Phase:          "running",
		LastActivityAt: time.Now().UTC(),
	}))
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, "worker-active-b", workerstatus.LivenessRecord{
		WorkerID:       "worker-active-b",
		ProjectRoot:    projectRoot,
		CurrentBead:    "ddx-status-active-b",
		AttemptID:      "att-active-b",
		Phase:          "running",
		LastActivityAt: time.Now().UTC(),
	}))

	text, err := executeCommand(factory.NewRootCommand(), "bead", "status")
	require.NoError(t, err)
	assert.Contains(t, text, "Lifecycle in progress: 0")
	assert.Contains(t, text, "Active workers:     2")
	assert.Contains(t, text, "ddx-status-active-a")
	assert.Contains(t, text, "ddx-status-active-b")
}

func TestBeadStatusJSONReportsActiveWorkers(t *testing.T) {
	workingDir, factory, _ := setupBeadHumanEnv(t,
		&bead.Bead{ID: "ddx-status-json-a", Title: "JSON A"},
		&bead.Bead{ID: "ddx-status-json-b", Title: "JSON B"},
	)
	projectRoot := workingDir

	require.NoError(t, workerstatus.WriteLiveness(projectRoot, "worker-json-a", workerstatus.LivenessRecord{
		WorkerID:       "worker-json-a",
		ProjectRoot:    projectRoot,
		CurrentBead:    "ddx-status-json-a",
		AttemptID:      "att-json-a",
		Phase:          "running",
		LastActivityAt: time.Now().UTC(),
	}))
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, "worker-json-b", workerstatus.LivenessRecord{
		WorkerID:       "worker-json-b",
		ProjectRoot:    projectRoot,
		CurrentBead:    "ddx-status-json-b",
		AttemptID:      "att-json-b",
		Phase:          "running",
		LastActivityAt: time.Now().UTC(),
	}))

	out, err := executeCommand(factory.NewRootCommand(), "bead", "status", "--json")
	require.NoError(t, err)

	var report BeadStatusReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.Equal(t, 2, report.ActiveWork.Count)
	assert.ElementsMatch(t, []string{"ddx-status-json-a", "ddx-status-json-b"}, report.ActiveWork.BeadIDs)
	assert.Equal(t, 2, report.Total)
	assert.Equal(t, 0, report.InProgress)
}

func TestBeadHumanResolveRetryRequiresNote(t *testing.T) {
	nh := &bead.Bead{
		ID:     "ddx-human-retry",
		Title:  "Retry human bead",
		Status: bead.StatusProposed,
	}
	_, factory, store := setupBeadHumanEnv(t, nh)
	require.NoError(t, store.Update(context.Background(), nh.ID, func(b *bead.Bead) {
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{Reason: "blocked"})
	}))

	_, err := executeCommand(factory.NewRootCommand(), "bead", "human", "resolve", nh.ID, "--action", "retry")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--note is required")

	_, err = executeCommand(factory.NewRootCommand(), "bead", "human", "resolve", nh.ID, "--action", "retry", "--note", "operator approved retry")
	require.NoError(t, err)

	got, err := store.Get(context.Background(), nh.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.False(t, hasTestLabel(got.Labels, bead.LabelNeedsHuman))
	assert.Empty(t, bead.GetNeedsHumanMeta(*got).Reason)

	events, err := store.Events(nh.ID)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	assert.Equal(t, "needs_human_resolved", events[len(events)-1].Kind)
	assert.Equal(t, "action=retry", events[len(events)-1].Summary)
	assert.Contains(t, events[len(events)-1].Body, "operator approved retry")
}

func TestBeadHumanResolveSplitObsoleteDefer(t *testing.T) {
	t.Run("split requires children and links them", func(t *testing.T) {
		parent := &bead.Bead{ID: "ddx-human-split", Title: "Split", Status: bead.StatusProposed}
		child := &bead.Bead{ID: "ddx-human-child", Title: "Child"}
		_, factory, store := setupBeadHumanEnv(t, parent, child)

		_, err := executeCommand(factory.NewRootCommand(), "bead", "human", "resolve", parent.ID, "--action", "split", "--note", "split manually")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--children is required")

		_, err = executeCommand(factory.NewRootCommand(), "bead", "human", "resolve", parent.ID, "--action", "split", "--children", child.ID, "--note", "split manually")
		require.NoError(t, err)

		got, err := store.Get(context.Background(), parent.ID)
		require.NoError(t, err)
		assert.False(t, hasTestLabel(got.Labels, bead.LabelNeedsHuman))
		assert.Contains(t, got.DepIDs(), child.ID)
	})

	t.Run("obsolete closes with evidence", func(t *testing.T) {
		nh := &bead.Bead{ID: "ddx-human-obsolete", Title: "Obsolete", Status: bead.StatusProposed}
		_, factory, store := setupBeadHumanEnv(t, nh)

		_, err := executeCommand(factory.NewRootCommand(), "bead", "human", "resolve", nh.ID, "--action", "obsolete", "--note", "superseded")
		require.NoError(t, err)

		got, err := store.Get(context.Background(), nh.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusClosed, got.Status)
		events, err := store.Events(nh.ID)
		require.NoError(t, err)
		require.NotEmpty(t, events)
		assert.Equal(t, "needs_human_resolved", events[len(events)-1].Kind)
		assert.Equal(t, "action=obsolete", events[len(events)-1].Summary)
	})

	t.Run("defer preserves operator attention with note event", func(t *testing.T) {
		nh := &bead.Bead{ID: "ddx-human-defer", Title: "Defer", Status: bead.StatusProposed}
		_, factory, store := setupBeadHumanEnv(t, nh)

		_, err := executeCommand(factory.NewRootCommand(), "bead", "human", "resolve", nh.ID, "--action", "defer", "--note", "wait for operator window")
		require.NoError(t, err)

		got, err := store.Get(context.Background(), nh.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusProposed, got.Status)
		assert.False(t, hasTestLabel(got.Labels, bead.LabelNeedsHuman))
		events, err := store.Events(nh.ID)
		require.NoError(t, err)
		require.NotEmpty(t, events)
		assert.Equal(t, "action=defer", events[len(events)-1].Summary)
		assert.Contains(t, events[len(events)-1].Body, "wait for operator window")
	})
}

func assertReadyIDs(t *testing.T, out string, want []string) {
	t.Helper()
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, row["id"].(string))
	}
	assert.Equal(t, want, got)
}

func hasTestLabel(labels []string, label string) bool {
	return strings.Contains(","+strings.Join(labels, ",")+",", ","+label+",")
}
