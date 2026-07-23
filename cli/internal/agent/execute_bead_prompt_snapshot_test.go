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

// TestPrompts_ClaudeVariant_ByteIdentical is the byte-for-byte snapshot gate
// for the execute-bead instruction block used by Claude-style harnesses.
func TestPrompts_ClaudeVariant_ByteIdentical(t *testing.T) {
	assertPromptSnapshot(t, "claude", executeBeadInstructionsText)
}

// TestPrompts_AgentVariant_ByteIdentical is the byte-for-byte snapshot gate
// for the execute-bead instruction block used by Agent-style harnesses.
func TestPrompts_AgentVariant_ByteIdentical(t *testing.T) {
	assertPromptSnapshot(t, "agent", executeBeadInstructionsText)
}

// TestPrompts_HarnessNeutral_ByteIdentical is retained as a compatibility
// alias for older snapshot tooling and now delegates to the Claude variant.
func TestPrompts_HarnessNeutral_ByteIdentical(t *testing.T) {
	TestPrompts_ClaudeVariant_ByteIdentical(t)
}
