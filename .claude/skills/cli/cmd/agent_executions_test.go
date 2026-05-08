package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// TestAgentExecutionsFetch verifies that `ddx agent executions fetch <id>`
// resolves the mirror entry and copies the bundle into the project's
// .ddx/executions/<id>/ directory.
func TestAgentExecutionsFetch(t *testing.T) {
	projectRoot := t.TempDir()
	mirrorRoot := t.TempDir()

	const attemptID = "20260418T061717-fetch01"
	const beadID = "ddx-fetch-test"

	mirrorDir := filepath.Join(mirrorRoot, attemptID)
	if err := os.MkdirAll(mirrorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mirrorDir, "manifest.json"), []byte(`{"attempt_id":"`+attemptID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mirrorDir, "result.json"), []byte(`{"status":"success"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := agent.AppendMirrorIndex(projectRoot, &agent.MirrorIndexEntry{
		AttemptID:  attemptID,
		BeadID:     beadID,
		MirrorURI:  mirrorDir,
		UploadedAt: time.Now().UTC(),
		ByteSize:   42,
		Kind:       "local",
	}); err != nil {
		t.Fatal(err)
	}

	f := NewCommandFactory(projectRoot)
	cmd := f.newAgentExecutionsCommand()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"fetch", "--project", projectRoot, attemptID})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\noutput=%s", err, out.String())
	}

	dest := filepath.Join(projectRoot, ".ddx", "executions", attemptID)
	for _, name := range []string{"manifest.json", "result.json"} {
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Errorf("expected %s after fetch: %v", name, err)
		}
	}
	if !strings.Contains(out.String(), attemptID) {
		t.Errorf("output missing attempt id: %q", out.String())
	}
}

// TestAgentExecutionsFetch_MissingEntry verifies fetch returns a clear
// error when the attempt id is not present in mirror-index.jsonl.
func TestAgentExecutionsFetch_MissingEntry(t *testing.T) {
	projectRoot := t.TempDir()
	f := NewCommandFactory(projectRoot)
	cmd := f.newAgentExecutionsCommand()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"fetch", "--project", projectRoot, "20260101T000000-missing"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for missing attempt; output=%s", out.String())
	}
	if !strings.Contains(err.Error(), "no mirror entry") {
		t.Errorf("unexpected error: %v", err)
	}
}
