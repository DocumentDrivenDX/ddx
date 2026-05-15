package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkAttemptMetricsAutoCommitted(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	projectRoot, store, beadIDs, head := newWorkAuditRepo(t, 1)
	factory := NewCommandFactory(projectRoot)
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			BeadID:           beadID,
			AttemptID:        "20260515T101828-work-metrics",
			Status:           agent.ExecuteBeadStatusSuccess,
			SessionID:        "sess-work-metrics",
			BaseRev:          head,
			ResultRev:        head,
			ProjectRoot:      projectRoot,
			RequestedProfile: "smart",
		}, nil
	})

	_, err := executeCommand(
		factory.NewRootCommand(),
		"work",
		"--once",
		"--project", projectRoot,
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err)

	rows, err := attemptmetrics.LoadRows(projectRoot)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "20260515T101828-work-metrics", rows[0].AttemptID)

	status := runGitWorkAudit(t, projectRoot, "status", "--short", "--", ".ddx/beads.jsonl", ".ddx/metrics/attempts.jsonl", ".ddx/attachments")
	assert.Empty(t, status)

	got, err := store.Get(context.Background(), beadIDs[0])
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

func TestWorkOutcomeTrackerMutationAutoCommitted(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	projectRoot, store, beadIDs, head := newWorkAuditRepo(t, 1)
	factory := NewCommandFactory(projectRoot)
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			BeadID:           beadID,
			AttemptID:        "20260515T101828-work-outcome",
			Status:           agent.ExecuteBeadStatusExecutionFailed,
			Detail:           "implementation failed",
			BaseRev:          head,
			ResultRev:        head,
			SessionID:        "sess-work-outcome",
			ProjectRoot:      projectRoot,
			RequestedProfile: "smart",
		}, nil
	})

	_, err := executeCommand(
		factory.NewRootCommand(),
		"work",
		"--once",
		"--project", projectRoot,
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err)

	status := runGitWorkAudit(t, projectRoot, "status", "--short", "--", ".ddx/beads.jsonl", ".ddx/metrics/attempts.jsonl", ".ddx/attachments")
	assert.Empty(t, status)

	got, err := store.Get(context.Background(), beadIDs[0])
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, got.Extra["work-last-status"])
	_, hasRetry := got.Extra["work-retry-after"]
	assert.True(t, hasRetry)
}

func TestWorkStopsWhenDurableAuditCommitFails(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	projectRoot, store, beadIDs, head := newWorkAuditRepo(t, 2)
	var attempted []string

	factory := NewCommandFactory(projectRoot)
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		attempted = append(attempted, beadID)
		return agent.ExecuteBeadReport{
			BeadID:           beadID,
			AttemptID:        "20260515T101828-work-fail-" + beadID,
			Status:           agent.ExecuteBeadStatusExecutionFailed,
			Detail:           "implementation failed",
			BaseRev:          head,
			ResultRev:        head,
			SessionID:        "sess-work-fail",
			ProjectRoot:      projectRoot,
			RequestedProfile: "smart",
		}, nil
	})
	factory.durableAuditFinalizeOverride = func(report agent.ExecuteBeadReport) error {
		require.NoError(t, attemptmetrics.AppendRow(projectRoot, attemptmetrics.AttemptRow{
			SchemaVersion: attemptmetrics.SchemaVersion,
			AttemptID:     report.AttemptID,
			BeadID:        report.BeadID,
			Outcome:       report.Status,
		}))
		return errors.New("git commit failed: forced durable audit failure")
	}

	_, err := executeCommand(
		factory.NewRootCommand(),
		"work",
		"--watch",
		"--idle-interval", "1s",
		"--project", projectRoot,
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err)

	require.Equal(t, []string{beadIDs[0]}, attempted)

	first, err := store.Get(context.Background(), beadIDs[0])
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, first.Status)
	assert.Empty(t, first.Owner)
	_, hasRetry := first.Extra["work-retry-after"]
	assert.True(t, hasRetry)

	second, err := store.Get(context.Background(), beadIDs[1])
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, second.Status)
	assert.Empty(t, second.Owner)

	status := runGitWorkAudit(t, projectRoot, "status", "--short", "--", ".ddx/beads.jsonl", ".ddx/metrics/attempts.jsonl")
	assert.NotEmpty(t, status)
}

func newWorkAuditRepo(t *testing.T, beadCount int) (string, *bead.Store, []string, string) {
	t.Helper()

	projectRoot := minimalProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# test\n"), 0o644))
	runGitWorkAudit(t, projectRoot, "init", "-b", "main")
	runGitWorkAudit(t, projectRoot, "config", "user.email", "test@ddx.test")
	runGitWorkAudit(t, projectRoot, "config", "user.name", "DDx Test")

	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	require.NoError(t, store.Init())

	var beadIDs []string
	for i := 0; i < beadCount; i++ {
		id := beadIDForIndex(i)
		beadIDs = append(beadIDs, id)
		require.NoError(t, store.Create(&bead.Bead{
			ID:       id,
			Title:    "Audit bead " + id,
			Priority: i,
		}))
	}

	runGitWorkAudit(t, projectRoot, "add", ".")
	runGitWorkAudit(t, projectRoot, "commit", "-m", "chore: seed repo")
	head := runGitWorkAudit(t, projectRoot, "rev-parse", "HEAD")
	return projectRoot, store, beadIDs, head
}

func beadIDForIndex(i int) string {
	return fmt.Sprintf("ddx-work-audit-%04d", i+1)
}

func runGitWorkAudit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = scrubbedWorkAuditGitEnv()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, strings.TrimSpace(string(out)))
	return strings.TrimSpace(string(out))
}

func scrubbedWorkAuditGitEnv() []string {
	parent := os.Environ()
	env := make([]string, 0, len(parent))
	for _, kv := range parent {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	return env
}
