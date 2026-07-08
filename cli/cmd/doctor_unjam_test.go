package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type doctorUnjamTestSummary struct {
	ProjectRoot string `json:"project_root"`
	Checkpoint  []struct {
		Commit string   `json:"commit"`
		Paths  []string `json:"paths"`
	} `json:"checkpoint"`
	Stashes []struct {
		Ref     string   `json:"ref"`
		Message string   `json:"message"`
		Paths   []string `json:"paths"`
		TopRef  string   `json:"top_ref"`
	} `json:"stashes"`
	Reported []struct {
		Path   string `json:"path"`
		Status string `json:"status"`
		Ref    string `json:"ref"`
	} `json:"reported"`
	Remaining []struct {
		Path   string `json:"path"`
		Status string `json:"status"`
		Ref    string `json:"ref"`
	} `json:"remaining"`
	Clean bool `json:"clean"`
}

func TestDoctorUnjam_CheckpointsDdxOwnedDirt(t *testing.T) {
	workDir := t.TempDir()
	initTestRepo(t, workDir)

	writeDirty := func(rel, contents string) {
		t.Helper()
		abs := filepath.Join(workDir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(contents), 0o644))
	}

	writeDirty(".ddx/executions/20260708T072902Z-test/result.json", `{"ok":true}`)
	writeDirty(".ddx/metrics/attempts.jsonl", `{"attempt":"ddx-b23c2a68"}`)

	factory := NewCommandFactory(workDir)
	out, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err, "doctor --unjam should succeed:\n%s", out)

	var summary doctorUnjamTestSummary
	require.NoError(t, json.Unmarshal([]byte(out), &summary))
	assert.True(t, summary.Clean, "checkpointed DDx dirt should leave the tree clean")
	require.Len(t, summary.Checkpoint, 1)
	assert.NotEmpty(t, summary.Checkpoint[0].Commit)
	assert.ElementsMatch(t, []string{
		".ddx/executions/20260708T072902Z-test/result.json",
		".ddx/metrics/attempts.jsonl",
	}, summary.Checkpoint[0].Paths)
	assert.Empty(t, summary.Stashes)
	assert.Empty(t, summary.Reported)
	assert.Empty(t, summary.Remaining)

	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = workDir
	statusCmd.Env = gitpkg.CleanEnv()
	statusOut, statusErr := statusCmd.CombinedOutput()
	require.NoError(t, statusErr, string(statusOut))
	assert.Empty(t, strings.TrimSpace(string(statusOut)), "tree must be clean after checkpoint commit")
}

func TestDoctorUnjam_StashesPreservedAttemptDirt(t *testing.T) {
	workDir := t.TempDir()
	initTestRepo(t, workDir)

	preservePath := "preserved.txt"
	operatorPath := "operator.txt"

	require.NoError(t, os.WriteFile(filepath.Join(workDir, preservePath), []byte("preserved payload\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, operatorPath), []byte("operator payload\n"), 0o644))

	add := exec.Command("git", "add", preservePath)
	add.Dir = workDir
	add.Env = gitpkg.CleanEnv()
	require.NoError(t, add.Run())

	commit := exec.Command("git", "commit", "-m", "preserve candidate")
	commit.Dir = workDir
	commit.Env = gitpkg.CleanEnv()
	require.NoError(t, commit.Run())

	preserveRef := "refs/ddx/iterations/ddx-b23c2a68/20260708T072902Z-deadbeef"
	headSHA := resolveGitHead(t, workDir)
	updateRef := exec.Command("git", "update-ref", preserveRef, headSHA)
	updateRef.Dir = workDir
	updateRef.Env = gitpkg.CleanEnv()
	require.NoError(t, updateRef.Run())

	reset := exec.Command("git", "reset", "--hard", "HEAD~1")
	reset.Dir = workDir
	reset.Env = gitpkg.CleanEnv()
	require.NoError(t, reset.Run())

	require.NoError(t, os.WriteFile(filepath.Join(workDir, preservePath), []byte("preserved payload\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, operatorPath), []byte("operator payload\n"), 0o644))

	factory := NewCommandFactory(workDir)
	out, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--unjam")
	require.NoError(t, err, "doctor --unjam should succeed:\n%s", out)

	var summary doctorUnjamTestSummary
	require.NoError(t, json.Unmarshal([]byte(out), &summary))
	assert.False(t, summary.Clean, "operator-authored dirt should remain after unjam")
	require.Len(t, summary.Stashes, 1)
	assert.Equal(t, preserveRef, summary.Stashes[0].Ref)
	assert.Contains(t, summary.Stashes[0].Message, preserveRef)
	assert.ElementsMatch(t, []string{preservePath}, summary.Stashes[0].Paths)
	assert.Empty(t, summary.Checkpoint)
	require.Len(t, summary.Reported, 1)
	assert.Equal(t, operatorPath, summary.Reported[0].Path)
	assert.Equal(t, operatorPath, summary.Remaining[0].Path)

	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = workDir
	statusCmd.Env = gitpkg.CleanEnv()
	statusOut, statusErr := statusCmd.CombinedOutput()
	require.NoError(t, statusErr, string(statusOut))
	assert.Contains(t, string(statusOut), "?? "+operatorPath)
	assert.NotContains(t, string(statusOut), preservePath)
}

func resolveGitHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	cmd.Env = gitpkg.CleanEnv()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}
