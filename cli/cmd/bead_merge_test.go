package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBeadMergeCommandReadsGitStages(t *testing.T) {
	workingDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, ".ddx"), 0o755))
	runGit(t, workingDir, "init")

	base := `{"id":"ddx-base","title":"Base","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-30T00:00:00Z","updated_at":"2026-04-30T00:00:00Z"}`
	ours := base + "\n" + `{"id":"ddx-ours","title":"Ours","status":"open","priority":1,"issue_type":"task","created_at":"2026-04-30T01:00:00Z","updated_at":"2026-04-30T01:00:00Z"}`
	theirs := base + "\n" + `{"id":"ddx-theirs","title":"Theirs","status":"open","priority":1,"issue_type":"task","created_at":"2026-04-30T02:00:00Z","updated_at":"2026-04-30T02:00:00Z"}`

	baseBlob := gitHashObject(t, workingDir, base+"\n")
	oursBlob := gitHashObject(t, workingDir, ours+"\n")
	theirsBlob := gitHashObject(t, workingDir, theirs+"\n")
	stageBeadConflict(t, workingDir, baseBlob, oursBlob, theirsBlob)

	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()
	output, err := executeCommand(rootCmd, "bead", "merge")
	require.NoError(t, err)
	require.Contains(t, output, "Merged .ddx/beads.jsonl: 3 records")

	data, err := os.ReadFile(filepath.Join(workingDir, ".ddx", "beads.jsonl"))
	require.NoError(t, err)
	records := readCommandMergeRecords(t, data)
	require.Contains(t, records, "ddx-base")
	require.Contains(t, records, "ddx-ours")
	require.Contains(t, records, "ddx-theirs")
}

func TestBeadMergeHelpDescribesSupportedEscapeHatch(t *testing.T) {
	factory := newBeadTestRoot(t, t.TempDir())
	rootCmd := factory.NewRootCommand()

	output, err := executeCommand(rootCmd, "bead", "merge", "--help")
	require.NoError(t, err)
	require.Contains(t, output, "supported escape hatch")
	require.Contains(t, output, "not a general hand-edit workflow")
}

func gitHashObject(t *testing.T, dir string, content string) string {
	t.Helper()
	cmd := exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(content)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func stageBeadConflict(t *testing.T, dir, base, ours, theirs string) {
	t.Helper()
	var input bytes.Buffer
	input.WriteString("100644 " + base + " 1\t.ddx/beads.jsonl\n")
	input.WriteString("100644 " + ours + " 2\t.ddx/beads.jsonl\n")
	input.WriteString("100644 " + theirs + " 3\t.ddx/beads.jsonl\n")
	cmd := exec.Command("git", "update-index", "--index-info")
	cmd.Dir = dir
	cmd.Stdin = &input
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func readCommandMergeRecords(t *testing.T, data []byte) map[string]map[string]any {
	t.Helper()
	out := make(map[string]map[string]any)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		id, _ := rec["id"].(string)
		out[id] = rec
	}
	return out
}
