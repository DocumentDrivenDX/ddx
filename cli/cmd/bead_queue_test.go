package cmd

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedQueueBeads(t *testing.T, workingDir string, beads []bead.Bead) {
	t.Helper()
	store := bead.NewStore(filepath.Join(workingDir, ddxroot.DirName))
	require.NoError(t, store.Init())
	require.NoError(t, store.WriteAll(beads))
}

func readBeadJSON(t *testing.T, root *CommandFactory, id string) map[string]any {
	t.Helper()
	out, err := executeCommand(root.NewRootCommand(), "bead", "show", id, "--json")
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	return got
}

func readReadyIDs(t *testing.T, root *CommandFactory) []string {
	t.Helper()
	out, err := executeCommand(root.NewRootCommand(), "bead", "ready", "--execution", "--json")
	require.NoError(t, err)

	var ready []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &ready))

	ids := make([]string, 0, len(ready))
	for _, row := range ready {
		ids = append(ids, row["id"].(string))
	}
	return ids
}

func TestBeadQueueTopSetsRankWithoutChangingPriority(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	seedQueueBeads(t, workingDir, []bead.Bead{
		{ID: "ddx-queue-a", Title: "A", Status: bead.StatusOpen, Priority: 2, IssueType: "task", CreatedAt: now, UpdatedAt: now},
		{ID: "ddx-queue-b", Title: "B", Status: bead.StatusOpen, Priority: 2, IssueType: "task", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
	})

	_, err := executeCommand(factory.NewRootCommand(), "bead", "queue", "top", "ddx-queue-b")
	require.NoError(t, err)

	got := readBeadJSON(t, factory, "ddx-queue-b")
	assert.Equal(t, float64(2), got["priority"])
	rank, ok := got["queue-rank"]
	require.True(t, ok, "queue-rank should be persisted")
	_, ok = rank.(float64)
	assert.True(t, ok, "queue-rank should serialize as a JSON number")

	ids := readReadyIDs(t, factory)
	require.Len(t, ids, 2)
	assert.Equal(t, "ddx-queue-b", ids[0])
}

func TestBeadQueueMoveBeforeAfterUsesSparseRanks(t *testing.T) {
	t.Run("before", func(t *testing.T) {
		workingDir := t.TempDir()
		factory := newBeadTestRoot(t, workingDir)
		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		seedQueueBeads(t, workingDir, []bead.Bead{
			{ID: "ddx-p0", Title: "P0", Status: bead.StatusOpen, Priority: 0, IssueType: "task", CreatedAt: now, UpdatedAt: now},
			{ID: "ddx-p1-a", Title: "P1 A", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now, UpdatedAt: now},
			{ID: "ddx-p1-b", Title: "P1 B", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
			{ID: "ddx-p1-c", Title: "P1 C", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(2 * time.Minute)},
		})

		_, err := executeCommand(factory.NewRootCommand(), "bead", "queue", "move", "ddx-p1-c", "--before", "ddx-p1-b")
		require.NoError(t, err)

		ids := readReadyIDs(t, factory)
		require.Equal(t, []string{"ddx-p0", "ddx-p1-a", "ddx-p1-c", "ddx-p1-b"}, ids)

		p0 := readBeadJSON(t, factory, "ddx-p0")
		_, ok := p0["queue-rank"]
		assert.False(t, ok, "unrelated priority bucket should not gain queue-rank")
	})

	t.Run("after", func(t *testing.T) {
		workingDir := t.TempDir()
		factory := newBeadTestRoot(t, workingDir)
		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		seedQueueBeads(t, workingDir, []bead.Bead{
			{ID: "ddx-p0", Title: "P0", Status: bead.StatusOpen, Priority: 0, IssueType: "task", CreatedAt: now, UpdatedAt: now},
			{ID: "ddx-p1-a", Title: "P1 A", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now, UpdatedAt: now},
			{ID: "ddx-p1-b", Title: "P1 B", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
			{ID: "ddx-p1-c", Title: "P1 C", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(2 * time.Minute)},
		})

		_, err := executeCommand(factory.NewRootCommand(), "bead", "queue", "move", "ddx-p1-a", "--after", "ddx-p1-c")
		require.NoError(t, err)

		ids := readReadyIDs(t, factory)
		require.Equal(t, []string{"ddx-p0", "ddx-p1-b", "ddx-p1-c", "ddx-p1-a"}, ids)

		p0 := readBeadJSON(t, factory, "ddx-p0")
		_, ok := p0["queue-rank"]
		assert.False(t, ok, "unrelated priority bucket should not gain queue-rank")
	})
}

func TestBeadQueueMoveRejectsCrossPriority(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	seedQueueBeads(t, workingDir, []bead.Bead{
		{ID: "ddx-p0", Title: "P0", Status: bead.StatusOpen, Priority: 0, IssueType: "task", CreatedAt: now, UpdatedAt: now},
		{ID: "ddx-p1", Title: "P1", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
	})

	_, err := executeCommand(factory.NewRootCommand(), "bead", "queue", "move", "ddx-p1", "--before", "ddx-p0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "one priority bucket")

	p0 := readBeadJSON(t, factory, "ddx-p0")
	_, ok := p0["queue-rank"]
	assert.False(t, ok, "failed move must not mutate the target bucket")
	p1 := readBeadJSON(t, factory, "ddx-p1")
	_, ok = p1["queue-rank"]
	assert.False(t, ok, "failed move must not mutate either bead")
}

func TestBeadQueueClearRemovesQueueRank(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	seedQueueBeads(t, workingDir, []bead.Bead{
		{ID: "ddx-a", Title: "A", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now, UpdatedAt: now},
		{ID: "ddx-b", Title: "B", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now, UpdatedAt: now},
	})

	_, err := executeCommand(factory.NewRootCommand(), "bead", "queue", "top", "ddx-b")
	require.NoError(t, err)
	_, err = executeCommand(factory.NewRootCommand(), "bead", "queue", "clear", "ddx-b")
	require.NoError(t, err)

	got := readBeadJSON(t, factory, "ddx-b")
	_, ok := got["queue-rank"]
	assert.False(t, ok, "queue-rank should be removed")

	ids := readReadyIDs(t, factory)
	require.Equal(t, []string{"ddx-a", "ddx-b"}, ids)
}

func TestBeadQueueRenormalizesOnlyPriorityBucket(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	seedQueueBeads(t, workingDir, []bead.Bead{
		{ID: "ddx-p0", Title: "P0", Status: bead.StatusOpen, Priority: 0, IssueType: "task", CreatedAt: now, UpdatedAt: now},
		{ID: "ddx-p1-a", Title: "P1 A", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now, UpdatedAt: now, Extra: map[string]any{"queue-rank": 0}},
		{ID: "ddx-p1-b", Title: "P1 B", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute), Extra: map[string]any{"queue-rank": 1}},
		{ID: "ddx-p1-c", Title: "P1 C", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(2 * time.Minute)},
	})

	_, err := executeCommand(factory.NewRootCommand(), "bead", "queue", "move", "ddx-p1-c", "--before", "ddx-p1-b")
	require.NoError(t, err)

	ids := readReadyIDs(t, factory)
	require.Equal(t, []string{"ddx-p0", "ddx-p1-a", "ddx-p1-c", "ddx-p1-b"}, ids)

	p0 := readBeadJSON(t, factory, "ddx-p0")
	_, ok := p0["queue-rank"]
	assert.False(t, ok, "renormalization must not touch other priority buckets")

	p1a := readBeadJSON(t, factory, "ddx-p1-a")
	p1b := readBeadJSON(t, factory, "ddx-p1-b")
	p1c := readBeadJSON(t, factory, "ddx-p1-c")
	assert.Equal(t, float64(0), p1a["queue-rank"])
	assert.Equal(t, float64(10), p1c["queue-rank"])
	assert.Equal(t, float64(20), p1b["queue-rank"])
}

func TestBeadQueueHelpListsTopMoveAndClear(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)

	out, err := executeCommand(factory.NewRootCommand(), "bead", "queue", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "top")
	assert.Contains(t, out, "move")
	assert.Contains(t, out, "clear")
}
