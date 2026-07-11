package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	ProjectRoot        string                 `json:"project_root"`
	Clean              bool                   `json:"clean"`
	DDXStateCheckpoint *doctorUnjamCheckpoint `json:"ddx_state_checkpoint,omitempty"`
	PrunableWorktrees  []doctorUnjamWorktree  `json:"prunable_worktrees"`
	RemovedWorktrees   []doctorUnjamWorktree  `json:"removed_worktrees"`
	PrunedWorktrees    int                    `json:"pruned_worktrees"`
	Actions            []doctorUnjamAction    `json:"actions"`
	BeadDoctorRepair   *doctorUnjamRepairView `json:"bead_doctor_repair,omitempty"`
	ReleasedClaims     []string               `json:"released_claims,omitempty"`
	PreservedClaims    []string               `json:"preserved_claims,omitempty"`
}

type doctorUnjamRepairView struct {
	Path               string   `json:"path"`
	Clean              bool     `json:"clean"`
	FindingsCount      int      `json:"findings_count"`
	FixedFindingsCount int      `json:"fixed_findings_count"`
	FixedBeadIDs       []string `json:"fixed_bead_ids,omitempty"`
	BackupPath         string   `json:"backup_path,omitempty"`
	RepairArtifacts    []string `json:"repair_artifacts,omitempty"`
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

func TestDoctorUnjam_CheckpointsDDXOwnedState(t *testing.T) {
	projectRoot := setupDoctorUnjamRepo(t)
	execPath := filepath.Join(projectRoot, ddxroot.DirName, "executions", "20260710T000000-deadbeef", "result.json")
	metricsPath := filepath.Join(projectRoot, ddxroot.DirName, "metrics", "attempts.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(execPath), 0o755))
	require.NoError(t, os.WriteFile(execPath, []byte(`{"status":"done"}`+"\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Dir(metricsPath), 0o755))
	require.NoError(t, os.WriteFile(metricsPath, []byte(`{"attempt_id":"test"}`+"\n"), 0o644))

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)

	report := decodeDoctorUnjamReport(t, output)
	require.NotNil(t, report.DDXStateCheckpoint)
	assert.NotEmpty(t, report.DDXStateCheckpoint.CommitSHA)

	head := runGitCapture(t, projectRoot, "rev-parse", "HEAD")
	assert.Equal(t, head, report.DDXStateCheckpoint.CommitSHA)

	subject := runGitCapture(t, projectRoot, "log", "-1", "--format=%s")
	assert.Equal(t, ddxStateCheckpointCommitMessage, subject)

	status := runGitCapture(t, projectRoot, "status", "--porcelain", "--", ".ddx/executions", ".ddx/metrics")
	assert.Empty(t, status, "checkpoint must leave .ddx/executions and .ddx/metrics clean")

	secondOutput, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)
	secondReport := decodeDoctorUnjamReport(t, secondOutput)
	assert.Nil(t, secondReport.DDXStateCheckpoint, "rerun with nothing new dirty must not create another checkpoint")

	secondHead := runGitCapture(t, projectRoot, "rev-parse", "HEAD")
	assert.Equal(t, head, secondHead, "rerun must not add a duplicate checkpoint commit")
}

func TestDoctorUnjam_CheckpointSummaryListsCommittedPaths(t *testing.T) {
	projectRoot := setupDoctorUnjamRepo(t)
	execPath := filepath.Join(projectRoot, ddxroot.DirName, "executions", "20260710T000000-deadbeef", "result.json")
	metricsPath := filepath.Join(projectRoot, ddxroot.DirName, "metrics", "attempts.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(execPath), 0o755))
	require.NoError(t, os.WriteFile(execPath, []byte(`{"status":"done"}`+"\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Dir(metricsPath), 0o755))
	require.NoError(t, os.WriteFile(metricsPath, []byte(`{"attempt_id":"test"}`+"\n"), 0o644))

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)

	report := decodeDoctorUnjamReport(t, output)
	require.NotNil(t, report.DDXStateCheckpoint)
	assert.ElementsMatch(t, []string{
		".ddx/executions/20260710T000000-deadbeef/result.json",
		".ddx/metrics/attempts.jsonl",
	}, report.DDXStateCheckpoint.CommittedPaths)

	require.NotEmpty(t, report.Actions)
	assert.Equal(t, "ddx_state_checkpoint", report.Actions[0].Kind)
	assert.Equal(t, 2, report.Actions[0].Count)
}

func TestDoctorUnjam_StashesPreserveDerivedPaths(t *testing.T) {
	projectRoot := setupDoctorUnjamRepo(t)
	preserveRef, dirtyRel := seedDoctorUnjamPreserveRefDirtyPath(t, projectRoot)

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)

	report := decodeDoctorUnjamReport(t, output)
	action := findDoctorUnjamAction(report.Actions, doctorUnjamPreserveRefStashActionKind)
	require.NotNil(t, action, "expected a preserve-ref stash action in the JSON summary")
	assert.Equal(t, preserveRef, action.PreserveRef)
	assert.Equal(t, 1, action.Count)
	assert.Equal(t, dirtyRel, action.Path)

	stashList := runGitCapture(t, projectRoot, "stash", "list")
	assert.Contains(t, stashList, preserveRef)

	secondOutput, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)
	secondReport := decodeDoctorUnjamReport(t, secondOutput)
	assert.Nil(t, findDoctorUnjamAction(secondReport.Actions, doctorUnjamPreserveRefStashActionKind))

	stashList = runGitCapture(t, projectRoot, "stash", "list")
	assert.Equal(t, 1, strings.Count(stashList, preserveRef), "rerunning doctor --unjam must not create a duplicate preserve-ref stash")
}

func TestDoctorUnjam_StashCleansPreserveDerivedPath(t *testing.T) {
	projectRoot := setupDoctorUnjamRepo(t)
	_, dirtyRel := seedDoctorUnjamPreserveRefDirtyPath(t, projectRoot)

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)

	report := decodeDoctorUnjamReport(t, output)
	require.NotNil(t, findDoctorUnjamAction(report.Actions, doctorUnjamPreserveRefStashActionKind))

	status := runGitCapture(t, projectRoot, "status", "--porcelain", "--", dirtyRel)
	assert.Empty(t, status, "doctor --unjam must leave the leaked preserve-derived path clean")
}

func runGitCapture(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
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

func TestDoctorUnjam_PrunesPhantomDeps(t *testing.T) {
	projectRoot := setupDoctorUnjamRepo(t)
	activePath := filepath.Join(projectRoot, ddxroot.DirName, "beads.jsonl")
	realTargetID := "ddx-real-target"
	referrerID := "ddx-referrer"
	writeTrackerRows(t, activePath, []string{
		mustBeadJSON(t, map[string]any{"id": realTargetID, "title": "real target", "status": "open", "priority": 1, "issue_type": bead.IssueTypeOperatorPrompt}),
		mustBeadJSON(t, map[string]any{
			"id":         referrerID,
			"title":      "referrer",
			"status":     "open",
			"priority":   1,
			"issue_type": bead.IssueTypeOperatorPrompt,
			"dependencies": []map[string]any{
				{"issue_id": referrerID, "depends_on_id": "ddx-missing-target", "type": "blocks"},
				{"issue_id": referrerID, "depends_on_id": realTargetID, "type": "blocks"},
			},
		}),
	})

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)

	report := decodeDoctorUnjamReport(t, output)
	require.NotNil(t, report.BeadDoctorRepair)
	assert.False(t, report.BeadDoctorRepair.Clean)
	assert.Greater(t, report.BeadDoctorRepair.FixedFindingsCount, 0)
	assert.Contains(t, report.BeadDoctorRepair.FixedBeadIDs, referrerID)
	assert.NotEmpty(t, report.BeadDoctorRepair.BackupPath)
	require.NotEmpty(t, report.Actions)
	assert.Equal(t, "bead_doctor_fix", report.Actions[0].Kind)

	rewritten := readTrackerRows(t, activePath)
	require.Len(t, rewritten, 2)
	referrer := decodeBeadRecord(t, rewritten[1])
	require.Len(t, referrer.Dependencies, 1)
	assert.Equal(t, realTargetID, referrer.Dependencies[0].DependsOnID)
}

func TestDoctorUnjam_PreservesArchivedDependencyTargets(t *testing.T) {
	projectRoot := setupDoctorUnjamRepo(t)
	activePath := filepath.Join(projectRoot, ddxroot.DirName, "beads.jsonl")
	archivePath := filepath.Join(projectRoot, ddxroot.DirName, "beads-archive.jsonl")
	archivedID := "ddx-archived-target"
	referrerID := "ddx-archive-referrer"
	writeTrackerRows(t, archivePath, []string{
		mustBeadJSON(t, map[string]any{"id": archivedID, "title": "archived target", "status": "closed", "priority": 1, "issue_type": bead.IssueTypeOperatorPrompt}),
	})
	writeTrackerRows(t, activePath, []string{
		mustBeadJSON(t, map[string]any{
			"id":         referrerID,
			"title":      "archive referrer",
			"status":     "open",
			"priority":   1,
			"issue_type": bead.IssueTypeOperatorPrompt,
			"dependencies": []map[string]any{
				{"issue_id": referrerID, "depends_on_id": archivedID, "type": "blocks"},
			},
		}),
	})

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)

	report := decodeDoctorUnjamReport(t, output)
	require.NotNil(t, report.BeadDoctorRepair)
	assert.True(t, report.BeadDoctorRepair.Clean)
	assert.Zero(t, report.BeadDoctorRepair.FixedFindingsCount)
	assert.Empty(t, report.BeadDoctorRepair.BackupPath)

	rewritten := readTrackerRows(t, activePath)
	require.Len(t, rewritten, 1)
	referrer := decodeBeadRecord(t, rewritten[0])
	require.Len(t, referrer.Dependencies, 1)
	assert.Equal(t, archivedID, referrer.Dependencies[0].DependsOnID)
}

func TestDoctorUnjam_BeadDoctorRepairSummaryAndIdempotency(t *testing.T) {
	projectRoot := setupDoctorUnjamRepo(t)
	activePath := filepath.Join(projectRoot, ddxroot.DirName, "beads.jsonl")
	referrerID := "ddx-summary-referrer"
	writeTrackerRows(t, activePath, []string{
		mustBeadJSON(t, map[string]any{
			"id":         referrerID,
			"title":      "summary referrer",
			"status":     "open",
			"priority":   1,
			"issue_type": bead.IssueTypeOperatorPrompt,
			"dependencies": []map[string]any{
				{"issue_id": referrerID, "depends_on_id": "ddx-summary-missing", "type": "blocks"},
			},
		}),
	})

	factory := NewCommandFactory(projectRoot)
	firstOutput, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)
	firstReport := decodeDoctorUnjamReport(t, firstOutput)
	require.NotNil(t, firstReport.BeadDoctorRepair)
	assert.False(t, firstReport.BeadDoctorRepair.Clean)
	assert.Equal(t, 1, firstReport.BeadDoctorRepair.FixedFindingsCount)
	assert.Contains(t, firstReport.BeadDoctorRepair.FixedBeadIDs, referrerID)
	assert.NotEmpty(t, firstReport.BeadDoctorRepair.BackupPath)
	require.NotEmpty(t, firstReport.Actions)
	assert.Equal(t, "bead_doctor_fix", firstReport.Actions[0].Kind)
	assert.Equal(t, 1, firstReport.Actions[0].Count)

	secondOutput, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err)
	secondReport := decodeDoctorUnjamReport(t, secondOutput)
	require.NotNil(t, secondReport.BeadDoctorRepair)
	assert.True(t, secondReport.BeadDoctorRepair.Clean)
	assert.Zero(t, secondReport.BeadDoctorRepair.FixedFindingsCount)
	assert.Empty(t, secondReport.BeadDoctorRepair.BackupPath)
	assert.Empty(t, secondReport.Actions, "clean second run must not report repair actions")
}

func setupDoctorUnjamRepo(t *testing.T) string {
	t.Helper()

	projectRoot := t.TempDir()
	initGitRepo(t, projectRoot)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ddxroot.DirName, "beads.jsonl"), nil, 0o644))
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

func writeTrackerRows(t *testing.T, path string, rows []string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	content := strings.Join(rows, "\n") + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func mustBeadJSON(t *testing.T, beadData map[string]any) string {
	t.Helper()

	data, err := json.Marshal(beadData)
	require.NoError(t, err)
	return string(data)
}

func readTrackerRows(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	rows := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(rows) == 1 && rows[0] == "" {
		return nil
	}
	return rows
}

func decodeBeadRecord(t *testing.T, row string) bead.Bead {
	t.Helper()

	var b bead.Bead
	require.NoError(t, json.Unmarshal([]byte(row), &b))
	return b
}

func seedDoctorUnjamPreserveRefDirtyPath(t *testing.T, projectRoot string) (string, string) {
	t.Helper()

	dirtyRel := filepath.ToSlash(filepath.Join("preserve", "leaked.txt"))
	dirtyPath := filepath.Join(projectRoot, filepath.FromSlash(dirtyRel))
	require.NoError(t, os.MkdirAll(filepath.Dir(dirtyPath), 0o755))
	require.NoError(t, os.WriteFile(dirtyPath, []byte("preserve-ref v1\n"), 0o644))

	runGit(t, projectRoot, "add", "--", dirtyRel)
	runGit(t, projectRoot, "commit", "-m", "seed preserve ref source")

	preserveRef := "refs/ddx/iterations/ddx-unjam-preserve/20260711T040733Z-deadbeefcafe"
	runGit(t, projectRoot, "update-ref", preserveRef, "HEAD")

	require.NoError(t, os.WriteFile(dirtyPath, []byte("preserve-ref v2\n"), 0o644))
	return preserveRef, dirtyRel
}

func findDoctorUnjamAction(actions []doctorUnjamAction, kind string) *doctorUnjamAction {
	for i := range actions {
		if actions[i].Kind == kind {
			return &actions[i]
		}
	}
	return nil
}
