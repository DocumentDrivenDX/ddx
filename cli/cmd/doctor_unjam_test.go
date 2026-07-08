package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
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

func decodeDoctorUnjamReport(t *testing.T, output string) doctorUnjamTestReport {
	t.Helper()

	var report doctorUnjamTestReport
	require.NoError(t, json.Unmarshal([]byte(output), &report), output)
	return report
}
