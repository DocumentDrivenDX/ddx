package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTriesCommandHelp(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	dir := testutils.NewFixtureRepo(t, "minimal")

	factory := NewCommandFactory(dir)
	rootCmd := factory.NewRootCommand()

	out, err := executeCommand(rootCmd, "tries", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "worktree", "tries --help should mention worktree records")
	assert.Contains(t, out, "merge", "tries --help should mention merge outcomes")
	assert.Contains(t, out, "preserve", "tries --help should mention preserve outcomes")
}

func TestTriesCommandRecords(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_BEAD_DIR", "")

	t.Run("empty_returns_no_records_message", func(t *testing.T) {
		dir := testutils.NewFixtureRepo(t, "minimal")
		factory := NewCommandFactory(dir)
		rootCmd := factory.NewRootCommand()

		out, err := executeCommand(rootCmd, "tries")
		require.NoError(t, err)
		assert.Contains(t, out, "no layer-2 worktree records found")
	})

	t.Run("json_empty_returns_empty_array", func(t *testing.T) {
		dir := testutils.NewFixtureRepo(t, "minimal")
		factory := NewCommandFactory(dir)
		rootCmd := factory.NewRootCommand()

		out, err := executeCommand(rootCmd, "tries", "--json")
		require.NoError(t, err)
		trimmed := strings.TrimSpace(out)
		assert.Equal(t, "[]", trimmed, "empty tries --json should return []")
	})

	t.Run("surfaces_worktree_records_from_executions", func(t *testing.T) {
		dir := testutils.NewFixtureRepo(t, "minimal")
		execRoot := filepath.Join(dir, ddxroot.DirName, "executions")

		// A layer-2 record with worktree_path set (worktree start/end + merge outcome).
		writeExecResult(t, execRoot, "20260620T100000-try0001", map[string]any{
			"attempt_id":    "try0001",
			"bead_id":       "ddx-abc1",
			"worktree_path": "/tmp/ddx-wt-try0001",
			"started_at":    "2026-06-20T10:00:00Z",
			"finished_at":   "2026-06-20T10:05:00Z",
			"outcome":       "merged",
			"status":        "task_succeeded",
			"base_rev":      "abc123",
			"result_rev":    "def456",
		})

		// A layer-2 record with preserve outcome.
		writeExecResult(t, execRoot, "20260620T110000-try0002", map[string]any{
			"attempt_id":    "try0002",
			"bead_id":       "ddx-abc2",
			"worktree_path": "/tmp/ddx-wt-try0002",
			"started_at":    "2026-06-20T11:00:00Z",
			"finished_at":   "2026-06-20T11:03:00Z",
			"outcome":       "preserved",
			"status":        "preserved_for_review",
			"preserve_ref":  "refs/ddx/preserved/try0002",
			"base_rev":      "abc123",
			"result_rev":    "ghi789",
		})

		// A layer-1 record (no worktree_path) — should be excluded.
		writeExecResult(t, execRoot, "20260620T120000-run0001", map[string]any{
			"attempt_id": "run0001",
			"bead_id":    "ddx-xyz1",
			"outcome":    "task_succeeded",
			"status":     "task_succeeded",
		})

		factory := NewCommandFactory(dir)
		rootCmd := factory.NewRootCommand()

		out, err := executeCommand(rootCmd, "tries", "--json")
		require.NoError(t, err)

		var records []worktreeRecord
		require.NoError(t, json.Unmarshal([]byte(out), &records))
		require.Len(t, records, 2, "should return exactly 2 layer-2 records (layer-1 excluded)")

		byAttempt := map[string]worktreeRecord{}
		for _, r := range records {
			byAttempt[r.AttemptID] = r
		}

		r1, ok := byAttempt["try0001"]
		require.True(t, ok, "try0001 should be present")
		assert.Equal(t, "ddx-abc1", r1.BeadID)
		assert.Equal(t, "merged", r1.Outcome)
		assert.Equal(t, "task_succeeded", r1.Status)
		assert.Equal(t, "/tmp/ddx-wt-try0001", r1.WorktreePath)
		assert.False(t, r1.StartedAt.IsZero(), "started_at should be populated")
		assert.False(t, r1.FinishedAt.IsZero(), "finished_at should be populated")

		r2, ok := byAttempt["try0002"]
		require.True(t, ok, "try0002 should be present")
		assert.Equal(t, "ddx-abc2", r2.BeadID)
		assert.Equal(t, "preserved", r2.Outcome)
		assert.Equal(t, "preserved_for_review", r2.Status)
		assert.Equal(t, "refs/ddx/preserved/try0002", r2.PreserveRef)
	})

	t.Run("table_output_has_header_columns", func(t *testing.T) {
		dir := testutils.NewFixtureRepo(t, "minimal")
		execRoot := filepath.Join(dir, ddxroot.DirName, "executions")

		writeExecResult(t, execRoot, "20260620T130000-try0003", map[string]any{
			"attempt_id":    "try0003",
			"bead_id":       "ddx-tbl1",
			"worktree_path": "/tmp/ddx-wt-try0003",
			"started_at":    "2026-06-20T13:00:00Z",
			"finished_at":   "2026-06-20T13:01:00Z",
			"outcome":       "merged",
			"status":        "task_succeeded",
		})

		factory := NewCommandFactory(dir)
		rootCmd := factory.NewRootCommand()

		out, err := executeCommand(rootCmd, "tries")
		require.NoError(t, err)
		header := strings.SplitN(out, "\n", 2)[0]
		for _, col := range []string{"ATTEMPT_ID", "BEAD_ID", "STARTED_AT", "FINISHED_AT", "OUTCOME", "STATUS"} {
			assert.Contains(t, header, col)
		}
	})
}
