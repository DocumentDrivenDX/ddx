package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/require"
)

// TestVocabConsistency_AttemptIDOnly asserts that no old run-id Go identifiers
// remain in cli/internal/agent/ or cli/internal/bead/. The canonical term is
// attemptID (camelCase) / AttemptID (PascalCase) / attempt_id (JSON).
func TestVocabConsistency_AttemptIDOnly(t *testing.T) {
	t.Parallel()
	// Split strings to avoid this file self-matching.
	forbidden := []string{
		"run" + "ID", // would be runID
		"Run" + "ID", // would be RunID
	}

	dirs := []string{".", filepath.Join("..", "bead")}

	var violations []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
				continue
			}
			if e.Name() == "vocab_consistency_test.go" {
				continue // skip self
			}
			path := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile %s: %v", path, err)
			}
			text := string(data)
			for _, pat := range forbidden {
				if strings.Contains(text, pat) {
					violations = append(violations, fmt.Sprintf("%s: contains %q", path, pat))
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Errorf("old run-id Go identifiers found (rename to attemptID/AttemptID):\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// TestExecutionDirs_AcceptBothNames verifies that the execution directory
// reader handles dirs with both legacy naming (historically called run-id)
// and new canonical naming (attempt-id). Both use the same timestamp-hex
// format; the test confirms no format-specific filtering is applied.
func TestExecutionDirs_AcceptBothNames(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	// one dir named in the old "run-id" convention, one in the new "attempt-id" convention
	legacyName := "20260101T120000-aabb1122"
	newName := "20260511T012426-c8d9f9d4"

	for _, name := range []string{legacyName, newName} {
		dir := filepath.Join(projectRoot, ExecuteBeadArtifactDir, name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "manifest.json"),
			[]byte(`{"attempt_id":"`+name+`"}`),
			0o644,
		))
	}

	var summary ExecutionCleanupSummary
	count := scanCompleteEvidenceDirs(projectRoot, ExecuteBeadArtifactDir, "manifest.json", "result.json", &summary)

	require.Equal(t, 2, count, "reader must enumerate both legacy and new dir names")
	require.Equal(t, 2, summary.CompleteEvidenceDirs, "both dirs must be recognized as complete evidence")
}
