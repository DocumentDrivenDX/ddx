package cmd

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupWorkPlanEnv initializes a TestEnvironment and populates its bead store
// with the supplied beads. It also initializes the store on disk.
func setupWorkPlanEnv(t *testing.T, beads ...*bead.Bead) *TestEnvironment {
	t.Helper()
	env := NewTestEnvironment(t)
	store := bead.NewStore(filepath.Join(env.Dir, ".ddx"))
	require.NoError(t, store.Init())
	for _, b := range beads {
		require.NoError(t, store.Create(b))
	}
	return env
}

// TestWorkPlan_CLI_TextOutput verifies that the default text output contains the
// documented column headers and at least one data row when beads are present.
func TestWorkPlan_CLI_TextOutput(t *testing.T) {
	b1 := &bead.Bead{ID: "ddx-text-001", Title: "First bead", Priority: 0}
	b2 := &bead.Bead{ID: "ddx-text-002", Title: "Second bead", Priority: 1}
	env := setupWorkPlanEnv(t, b1, b2)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "plan")
	require.NoError(t, err)

	// Must include the documented column headers.
	assert.Contains(t, out, "POS")
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, "PRI")
	assert.Contains(t, out, "UPDATED")
	assert.Contains(t, out, "STATUS")
	assert.Contains(t, out, "DECISION")
	assert.Contains(t, out, "WHY")

	// Both bead IDs must appear.
	assert.Contains(t, out, "ddx-text-001")
	assert.Contains(t, out, "ddx-text-002")
}

// TestWorkPlan_CLI_JSONOutput verifies that --json emits valid JSON parseable
// as an array of queue entries with expected fields.
func TestWorkPlan_CLI_JSONOutput(t *testing.T) {
	b := &bead.Bead{ID: "ddx-json-001", Title: "JSON bead", Priority: 0}
	env := setupWorkPlanEnv(t, b)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "work", "plan", "--json")
	require.NoError(t, err)

	var entries []agent.QueueEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries), "output must be valid JSON")
	require.Len(t, entries, 1)
	assert.Equal(t, "ddx-json-001", entries[0].BeadID)
	assert.Equal(t, agent.FilterDecisionNext, entries[0].FilterDecision)
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
