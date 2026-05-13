package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeExecResult(t *testing.T, execRoot, sessionID string, payload map[string]any) {
	t.Helper()

	dir := filepath.Join(execRoot, sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir exec result dir: %v", err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal exec result: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), raw, 0o644); err != nil {
		t.Fatalf("write exec result: %v", err)
	}
}
