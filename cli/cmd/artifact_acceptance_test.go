package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeArtifactDoc(t *testing.T, workingDir, artifactID string) {
	t.Helper()
	path := filepath.Join(workingDir, "docs", artifactID+".md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("---\nddx:\n  id: "+artifactID+"\n---\n# "+artifactID+"\n"), 0o644))
}

func TestArtifactCommandRegistered(t *testing.T) {
	rootCmd := NewCommandFactory(t.TempDir()).NewRootCommand()

	out, err := executeCommand(rootCmd, "artifact", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "Inspect and regenerate graph artifacts")
	assert.Contains(t, out, "regenerate")
}

func TestArtifactRegenerateMissingGeneratorReturnsClearError(t *testing.T) {
	workingDir := t.TempDir()
	writeArtifactDoc(t, workingDir, "ART-001")
	rootCmd := NewCommandFactory(workingDir).NewRootCommand()

	_, err := executeCommand(rootCmd, "artifact", "regenerate", "ART-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `artifact "ART-001" has no generator definition or generated_by provenance; cannot regenerate`)
}

func TestArtifactRegenerateRecordsProducedArtifact(t *testing.T) {
	workingDir := t.TempDir()
	writeArtifactDoc(t, workingDir, "ART-001")
	store := ddxexec.NewStore(workingDir)
	require.NoError(t, store.SaveDefinition(ddxexec.Definition{
		ID:          "generate-art-001",
		ArtifactIDs: []string{"ART-001"},
		Executor: ddxexec.ExecutorSpec{
			Kind:    ddxexec.ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf 'generated\\n'"},
		},
		Active:    true,
		CreatedAt: mustExecAcceptanceTime(t, "2026-04-30T15:00:00Z"),
	}))

	rootCmd := NewCommandFactory(workingDir).NewRootCommand()
	out, err := executeCommand(rootCmd, "artifact", "regenerate", "ART-001", "--json")
	require.NoError(t, err)

	var rec ddxexec.RunRecord
	require.NoError(t, json.Unmarshal([]byte(out), &rec))
	assert.Equal(t, "ART-001", rec.ProducesArtifact)
	assert.Equal(t, ddxexec.StatusSuccess, rec.Status)
	assert.Equal(t, "generated", rec.Result.Stdout)
}
