package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupWorkPlanEnv initializes a TestEnvironment and populates its bead store
// with the supplied beads. It also initializes the store on disk.
func setupWorkPlanEnv(t *testing.T, beads ...*bead.Bead) *TestEnvironment {
	t.Helper()
	env := NewTestEnvironment(t)
	store := bead.NewStore(filepath.Join(env.Dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	for _, b := range beads {
		require.NoError(t, store.Create(context.Background(), b))
	}
	return env
}

func setupConventionWorkPlanProject(t *testing.T, beads ...*bead.Bead) string {
	t.Helper()
	projectRoot := minimalProjectDir(t)
	store := bead.NewStore(ddxroot.Path(context.Background(), projectRoot))
	require.NoError(t, store.Init(context.Background()))
	for _, b := range beads {
		require.NoError(t, store.Create(context.Background(), b))
	}
	return projectRoot
}

// TestWorkPlan_CLI_TextOutput verifies that the default text output contains the
// documented column headers and at least one data row when beads are present.
func TestWorkPlan_CLI_TextOutput(t *testing.T) {
	b1 := &bead.Bead{ID: "ddx-text-001", Title: "First bead", Priority: 0, Extra: map[string]any{"queue-rank": 3}}
	b2 := &bead.Bead{ID: "ddx-text-002", Title: "Second bead", Priority: 1}
	env := setupWorkPlanEnv(t, b1, b2)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "plan")
	require.NoError(t, err)

	// Must include the documented column headers.
	assert.Contains(t, out, "POS")
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, "TITLE")
	assert.Contains(t, out, "PRI")
	assert.Contains(t, out, "RANK")
	assert.Contains(t, out, "UPDATED")
	assert.Contains(t, out, "STATUS")
	assert.Contains(t, out, "DECISION")
	assert.Contains(t, out, "WHY")

	// Both bead IDs must appear.
	assert.Contains(t, out, "ddx-text-001")
	assert.Contains(t, out, "ddx-text-002")
	assert.Contains(t, out, "First bead")
	assert.Contains(t, out, "Second bead")
	assert.Regexp(t, regexp.MustCompile(`ddx-text-001.*\b3\b`), out)
}

// TestWorkPlan_CLI_JSONOutput verifies that --json emits valid JSON parseable
// as an array of queue entries with expected fields.
func TestWorkPlan_CLI_JSONOutput(t *testing.T) {
	b := &bead.Bead{ID: "ddx-json-001", Title: "JSON bead", Priority: 0, Extra: map[string]any{"queue-rank": 11}}
	env := setupWorkPlanEnv(t, b)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "plan", "--json")
	require.NoError(t, err)

	var entries []agent.QueueEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries), "output must be valid JSON")
	require.Len(t, entries, 1)
	assert.Equal(t, "ddx-json-001", entries[0].BeadID)
	assert.Equal(t, agent.FilterDecisionNext, entries[0].FilterDecision)
	require.NotNil(t, entries[0].QueueRank)
	assert.Equal(t, 11, *entries[0].QueueRank)
}

// TestWorkPlan_Subcommand_Registered verifies that "ddx work plan" is wired as
// a subcommand of "ddx work" and that --help is accessible.
func TestWorkPlan_Subcommand_Registered(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	planCmd, _, err := root.Find([]string{"work", "plan"})
	require.NoError(t, err, "ddx work plan must be registered")
	require.NotNil(t, planCmd)
	assert.Equal(t, "plan", planCmd.Use)
}

// TestWorkPlan_LabelFilter verifies that --label-filter narrows results
// identically to the worker's label intersection.
func TestWorkPlan_LabelFilter(t *testing.T) {
	bMatch := &bead.Bead{ID: "ddx-lf-match", Title: "Matching", Priority: 0, Labels: []string{"area:agent"}}
	bNoMatch := &bead.Bead{ID: "ddx-lf-nomatch", Title: "No match", Priority: 0, Labels: []string{"area:cli"}}
	env := setupWorkPlanEnv(t, bMatch, bNoMatch)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "plan", "--json", "--label-filter=area:agent")
	require.NoError(t, err)

	var entries []agent.QueueEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))

	// Two entries expected: one eligible, one skipped.
	require.Len(t, entries, 2)
	for _, e := range entries {
		if e.BeadID == "ddx-lf-match" {
			assert.Equal(t, agent.FilterDecisionNext, e.FilterDecision)
		}
		if e.BeadID == "ddx-lf-nomatch" {
			assert.Equal(t, agent.FilterDecisionSkipped, e.FilterDecision)
			assert.Contains(t, e.Why, "label_filter mismatch")
		}
	}
}

// TestWorkPlan_ShowsCompletedEpicClosureCandidate verifies that the text
// output distinguishes closure-ready epics from structural epics that still
// have open direct children.
func TestWorkPlan_ShowsCompletedEpicClosureCandidate(t *testing.T) {
	task := &bead.Bead{ID: "ddx-work-task", Title: "Task work", Priority: 0}
	completedEpic := &bead.Bead{ID: "ddx-work-epic-closed", Title: "Completed epic", IssueType: "epic", Priority: 1}
	activeEpic := &bead.Bead{ID: "ddx-work-epic-open", Title: "Active epic", IssueType: "epic", Priority: 2}
	closedChildOne := &bead.Bead{ID: "ddx-work-epic-closed-child-1", Title: "Closed child one", Parent: completedEpic.ID, Status: bead.StatusClosed}
	closedChildTwo := &bead.Bead{ID: "ddx-work-epic-closed-child-2", Title: "Closed child two", Parent: completedEpic.ID, Status: bead.StatusClosed}
	openChild := &bead.Bead{ID: "ddx-work-epic-open-child", Title: "Open child", Parent: activeEpic.ID, Status: bead.StatusBlocked}
	env := setupWorkPlanEnv(t, task, completedEpic, activeEpic, closedChildOne, closedChildTwo, openChild)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "plan")
	require.NoError(t, err)
	assert.Contains(t, out, "ddx-work-task")
	assert.Contains(t, out, "completed epic closure candidate(s)")
	assert.Contains(t, out, completedEpic.ID)
	assert.Contains(t, out, "skipped 1 ready epic(s) with open children")
	assert.Contains(t, out, activeEpic.ID)
}

// TestWorkPlan_Limit verifies that --limit caps the number of returned entries.
func TestWorkPlan_Limit(t *testing.T) {
	now := time.Now().UTC()
	var beads []*bead.Bead
	for i := 0; i < 5; i++ {
		beads = append(beads, &bead.Bead{
			ID:        "ddx-limit-" + string(rune('a'+i)),
			Title:     "Limit test bead",
			Priority:  0,
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		})
	}
	env := setupWorkPlanEnv(t, beads...)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "plan", "--json", "--limit=3")
	require.NoError(t, err)

	var entries []agent.QueueEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	assert.Len(t, entries, 3)
}

// TestWorkPlan_LimitZero verifies that --limit=0 returns all eligible beads.
func TestWorkPlan_LimitZero(t *testing.T) {
	now := time.Now().UTC()
	var beads []*bead.Bead
	for i := 0; i < 5; i++ {
		beads = append(beads, &bead.Bead{
			ID:        "ddx-all-" + string(rune('a'+i)),
			Title:     "Full queue bead",
			Priority:  0,
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		})
	}
	env := setupWorkPlanEnv(t, beads...)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "plan", "--json", "--limit=0")
	require.NoError(t, err)

	var entries []agent.QueueEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	assert.Len(t, entries, 5)
}

// TestWorkPlan_EmptyQueue verifies that an empty queue produces a readable
// message and does not error.
func TestWorkPlan_EmptyQueue(t *testing.T) {
	env := setupWorkPlanEnv(t) // no beads
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "plan")
	require.NoError(t, err)
	assert.Contains(t, out, "No execution-eligible beads")
}

func TestWorkPlanUsesDDxRootPath(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	projectRoot := setupConventionWorkPlanProject(t, &bead.Bead{
		ID:       "ddx-convention-plan",
		Title:    "Convention queue bead",
		Priority: 0,
	})

	_, statErr := os.Stat(filepath.Join(projectRoot, ddxroot.DirName))
	require.True(t, os.IsNotExist(statErr), "project root must stay in convention mode for this test")

	out, err := executeCommand(NewCommandFactory(projectRoot).NewRootCommand(), "work", "plan", "--json")
	require.NoError(t, err)

	var entries []agent.QueueEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "ddx-convention-plan", entries[0].BeadID)
}

// setupLinkedWorktreeWithConflictingBeads creates a primary git repo and a
// linked git worktree, seeds the primary's canonical .ddx/ with a bead in
// the given canonicalStatus, and seeds the linked worktree's stale .ddx/ with
// the same bead left open. Returns (primary, linked) directory paths.
func setupLinkedWorktreeWithConflictingBeads(t *testing.T, beadID, canonicalStatus string) (string, string) {
	t.Helper()

	primary := t.TempDir()
	runLinkedWorktreeGit(t, primary, "init")
	runLinkedWorktreeGit(t, primary, "config", "user.email", "test@example.com")
	runLinkedWorktreeGit(t, primary, "config", "user.name", "Test User")
	runLinkedWorktreeGit(t, primary, "config", "commit.gpgsign", "false")

	// Initial commit is required for git worktree add.
	require.NoError(t, os.WriteFile(filepath.Join(primary, "README.md"), []byte("# test"), 0o644))
	runLinkedWorktreeGit(t, primary, "add", "README.md")
	runLinkedWorktreeGit(t, primary, "commit", "-m", "init")

	// Create the linked worktree as a sibling of primary.
	linked := filepath.Join(t.TempDir(), ".execute-bead-wt-test")
	runLinkedWorktreeGit(t, primary, "worktree", "add", "-b", "exec-branch", linked)

	ctx := context.Background()

	// Seed the CANONICAL store (in primary) with the bead.
	primaryDDx := filepath.Join(primary, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(primaryDDx, 0o755))
	canonicalStore := bead.NewStore(primaryDDx)
	require.NoError(t, canonicalStore.Init(ctx))
	b := &bead.Bead{ID: beadID, Title: "Store unification test bead", Priority: 0}
	require.NoError(t, canonicalStore.Create(ctx, b))
	if canonicalStatus == "closed" {
		require.NoError(t, canonicalStore.Close(ctx, beadID))
	}

	// Seed the STALE local snapshot (in linked) with the bead left open.
	linkedDDx := filepath.Join(linked, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(linkedDDx, 0o755))
	staleStore := bead.NewStore(linkedDDx)
	require.NoError(t, staleStore.Init(ctx))
	stale := &bead.Bead{ID: beadID, Title: "Store unification test bead", Priority: 0}
	require.NoError(t, staleStore.Create(ctx, stale))
	// Leave stale as "open" — simulates a snapshot taken before the canonical close.

	return primary, linked
}

func runLinkedWorktreeGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitpkg.CleanEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// TestBeadAndWorkPlanUseSameStoreInLinkedWorktree verifies that from inside a
// git linked worktree both "ddx bead show" and "ddx work plan" resolve to the
// canonical (primary) bead store rather than the linked worktree's stale local
// .ddx/ snapshot. This is the regression test for ddx-d94f1857.
func TestBeadAndWorkPlanUseSameStoreInLinkedWorktree(t *testing.T) {
	const beadID = "ddx-wt-unify-001"
	_, linked := setupLinkedWorktreeWithConflictingBeads(t, beadID, "closed")

	factory := NewCommandFactory(linked)

	// bead show must read the canonical closed status.
	showOut, err := executeCommand(factory.NewRootCommand(), "bead", "show", beadID, "--json")
	require.NoError(t, err)
	var shown map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &shown))
	assert.Equal(t, "closed", shown["status"],
		"bead show must use the canonical (primary) store, not the stale linked snapshot")

	// work plan must also read the canonical store and not return the closed bead as next claim.
	planOut, err := executeCommand(factory.NewRootCommand(), "work", "plan", "--json", "--limit=0")
	require.NoError(t, err)
	var entries []agent.QueueEntry
	require.NoError(t, json.Unmarshal([]byte(planOut), &entries))
	for _, e := range entries {
		if e.BeadID == beadID {
			assert.NotEqual(t, agent.FilterDecisionNext, e.FilterDecision,
				"closed bead must not appear as 'next claim' in work plan")
		}
	}
}

// TestWorkPlanDoesNotPreferStaleLocalSnapshotWhenCanonicalStoreExists covers
// the observed ddx-d94f1857 failure mode: a bead closed in the canonical
// primary store must not be returned as next claim from a stale local .ddx/
// snapshot in a linked worktree.
func TestWorkPlanDoesNotPreferStaleLocalSnapshotWhenCanonicalStoreExists(t *testing.T) {
	const beadID = "ddx-wt-stale-002"
	_, linked := setupLinkedWorktreeWithConflictingBeads(t, beadID, "closed")

	planOut, err := executeCommand(NewCommandFactory(linked).NewRootCommand(), "work", "plan", "--json", "--limit=0")
	require.NoError(t, err)

	var entries []agent.QueueEntry
	require.NoError(t, json.Unmarshal([]byte(planOut), &entries))

	for _, e := range entries {
		require.NotEqual(t, agent.FilterDecisionNext, e.FilterDecision,
			"canonical closed bead %s must not appear as next claim (stale local snapshot must be ignored)", beadID)
	}
}
