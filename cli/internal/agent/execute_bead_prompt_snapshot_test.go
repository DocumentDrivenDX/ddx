package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const promptSnapshotDir = "testdata/execute_bead_prompt_snapshots"

func updatePromptSnapshots() bool {
	return os.Getenv("UPDATE_PROMPT_SNAPSHOTS") == "1"
}

func assertPromptSnapshot(t *testing.T, name string, got string) {
	t.Helper()

	path := filepath.Join(promptSnapshotDir, name+".txt")
	if updatePromptSnapshots() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir snapshot dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write snapshot %s: %v", path, err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot %s: %v (run UPDATE_PROMPT_SNAPSHOTS=1 go test ./internal/agent -run 'TestPrompts_.*ByteIdentical')", path, err)
	}
	if !bytes.Equal([]byte(got), want) {
		t.Fatalf("prompt snapshot %s drifted", path)
	}
}

// TestPrompts_HarnessNeutral_ByteIdentical is the byte-for-byte snapshot gate
// for the single execute-bead instruction block. The legacy snapshot filename
// is retained to avoid obscuring the substantive prompt change with a rename.
func TestPrompts_HarnessNeutral_ByteIdentical(t *testing.T) {
	assertPromptSnapshot(t, "claude", executeBeadInstructionsText)
}
