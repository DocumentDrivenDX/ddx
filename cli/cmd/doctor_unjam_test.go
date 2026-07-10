package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type doctorUnjamTestReport struct {
	ProjectRoot       string                `json:"project_root"`
	Clean             bool                  `json:"clean"`
	PrunableWorktrees []doctorUnjamWorktree `json:"prunable_worktrees"`
	RemovedWorktrees  []doctorUnjamWorktree `json:"removed_worktrees"`
	PrunedWorktrees   int                   `json:"pruned_worktrees"`
	Actions           []doctorUnjamAction   `json:"actions"`
	ReleasedClaims    []string              `json:"released_claims,omitempty"`
	PreservedClaims   []string              `json:"preserved_claims,omitempty"`
}

func TestDoctorUnjam_PrunesStaleWorktrees(t *testing.T) {
	projectRoot := setupDoctorUnjamRepo(t)
	worktreePath := seedStaleExecuteBeadWorktree(t, projectRoot)

	cmd := exec.Command("git", "worktree", "add", "--detach", worktreePath, "HEAD")
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	require.Error(t, err, string(out))

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)

	report := decodeDoctorUnjamReport(t, output)
	require.True(t, report.Clean)
	require.Len(t, report.PrunableWorktrees, 1)
	assert.Equal(t, worktreePath, report.PrunableWorktrees[0].Path)
	require.Len(t, report.Actions, 2)
	assert.Equal(t, "worktree_remove", report.Actions[0].Kind)
	assert.Equal(t, worktreePath, report.Actions[0].Path)
	assert.Equal(t, "worktree_prune", report.Actions[1].Kind)
	assert.Equal(t, 1, report.PrunedWorktrees)

	runGit(t, projectRoot, "worktree", "add", "--detach", worktreePath, "HEAD")
}

func TestDoctorUnjam_Idempotent(t *testing.T) {
	projectRoot := setupDoctorUnjamRepo(t)
	seedStaleExecuteBeadWorktree(t, projectRoot)

	factory := NewCommandFactory(projectRoot)

	firstOutput, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)
	firstReport := decodeDoctorUnjamReport(t, firstOutput)
	require.True(t, firstReport.Clean)
	require.Len(t, firstReport.Actions, 2)
	assert.Equal(t, 1, firstReport.PrunedWorktrees)

	secondOutput, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)
	secondReport := decodeDoctorUnjamReport(t, secondOutput)
	require.True(t, secondReport.Clean)
	assert.Empty(t, secondReport.PrunableWorktrees)
	assert.Empty(t, secondReport.RemovedWorktrees)
	assert.Empty(t, secondReport.Actions)
	assert.Zero(t, secondReport.PrunedWorktrees)
}

func TestDoctorUnjam_ReleasesStaleClaimLease(t *testing.T) {
	projectRoot := setupDoctorUnjamRepo(t)
	beadID := seedStaleClaimedBead(t, projectRoot)

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)

	report := decodeDoctorUnjamReport(t, output)
	require.True(t, report.Clean)
	assert.Contains(t, report.ReleasedClaims, beadID)
	assert.NotContains(t, report.PreservedClaims, beadID)

	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	_, found, err := store.ClaimLease(beadID)
	require.NoError(t, err)
	assert.False(t, found)

	events := readBeadEvents(t, projectRoot, beadID)
	require.NotEmpty(t, events)
	assert.Equal(t, "bead.stopped", events[len(events)-1].Kind)
}

func TestDoctorUnjam_PreservesFreshLiveClaim(t *testing.T) {
	projectRoot := setupDoctorUnjamRepo(t)
	beadID := seedFreshClaimedBead(t, projectRoot)

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)

	report := decodeDoctorUnjamReport(t, output)
	require.True(t, report.Clean)
	assert.Contains(t, report.PreservedClaims, beadID)
	assert.NotContains(t, report.ReleasedClaims, beadID)

	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	_, found, err := store.ClaimLease(beadID)
	require.NoError(t, err)
	assert.True(t, found)
}

func TestDoctorUnjam_ClaimLeaseReleaseIsIdempotent(t *testing.T) {
	projectRoot := setupDoctorUnjamRepo(t)
	beadID := seedStaleClaimedBead(t, projectRoot)

	factory := NewCommandFactory(projectRoot)
	firstOutput, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)
	firstReport := decodeDoctorUnjamReport(t, firstOutput)
	require.Contains(t, firstReport.ReleasedClaims, beadID)

	secondOutput, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)
	secondReport := decodeDoctorUnjamReport(t, secondOutput)
	assert.NotContains(t, secondReport.ReleasedClaims, beadID)
	assert.NotContains(t, secondReport.PreservedClaims, beadID)
	assert.Empty(t, readBeadEvents(t, projectRoot, beadID)[1:])
}

func setupDoctorUnjamRepo(t *testing.T) string {
	t.Helper()

	projectRoot := t.TempDir()
	initGitRepo(t, projectRoot)
	return projectRoot
}

func seedStaleExecuteBeadWorktree(t *testing.T, projectRoot string) string {
	t.Helper()

	tempRoot := filepath.Join(t.TempDir(), ".ddx-exec-wt")
	worktreePath := filepath.Join(tempRoot, agent.ExecuteBeadWtPrefix+"ddx-unjam-20260708T072228-deadbeef")
	require.NoError(t, os.MkdirAll(tempRoot, 0o755))
	runGit(t, projectRoot, "worktree", "add", "--detach", worktreePath, "HEAD")
	require.NoError(t, os.RemoveAll(worktreePath))
	return worktreePath
}

func seedStaleClaimedBead(t *testing.T, projectRoot string) string {
	t.Helper()

	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	b := seedReadyBead(t, store)
	require.NoError(t, store.Claim(b.ID, "worker-stale"))
	staleLeaseDir := bead.ClaimLivenessRoot(ddxroot.JoinProject(projectRoot))
	require.NoError(t, os.MkdirAll(staleLeaseDir, 0o755))
	staleLease := bead.ClaimLeaseRecord{
		BeadID:    b.ID,
		Owner:     "ddx",
		Machine:   "stale-machine",
		StartedAt: time.Now().Add(-4 * time.Hour),
		UpdatedAt: time.Now().Add(-4 * time.Hour),
		PID:       43210,
	}
	data, err := json.MarshalIndent(staleLease, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(staleLeaseDir, b.ID+".json"), data, 0o644))
	writeWorkerRecord(t, projectRoot, "w-stale", server.WorkerRecord{
		ID:          "w-stale",
		Kind:        "work",
		State:       "running",
		ProjectRoot: projectRoot,
		CurrentBead: b.ID,
		PID:         43210,
	})
	return b.ID
}

func seedFreshClaimedBead(t *testing.T, projectRoot string) string {
	t.Helper()

	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	b := seedReadyBead(t, store)
	require.NoError(t, store.Claim(b.ID, "worker-fresh"))
	lease, found, err := store.ClaimLease(b.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, lease.UpdatedAt.IsZero())
	return b.ID
}

func seedReadyBead(t *testing.T, store *bead.Store) *bead.Bead {
	t.Helper()

	b := &bead.Bead{
		Title:     "unjam test",
		Status:    bead.StatusOpen,
		Priority:  1,
		IssueType: bead.IssueTypeOperatorPrompt,
	}
	err := store.Create(context.Background(), b)
	require.NoError(t, err)
	return b
}

func writeWorkerRecord(t *testing.T, projectRoot string, id string, rec server.WorkerRecord) {
	t.Helper()

	dir := filepath.Join(ddxroot.JoinProject(projectRoot), "workers", id)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	data, err := json.MarshalIndent(rec, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.json"), data, 0o644))
}

func readBeadEvents(t *testing.T, projectRoot, beadID string) []bead.BeadEvent {
	t.Helper()

	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	events, err := store.Events(beadID)
	require.NoError(t, err)
	return events
}

func decodeDoctorUnjamReport(t *testing.T, output string) doctorUnjamTestReport {
	t.Helper()

	var report doctorUnjamTestReport
	require.NoError(t, json.Unmarshal([]byte(output), &report), output)
	return report
}
