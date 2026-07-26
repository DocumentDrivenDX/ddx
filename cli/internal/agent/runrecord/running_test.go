package runrecord

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTransitionToRunning_FromDispatching(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const runID = "20260726T120000-running"

	pre := NewDispatching(runID, "ddx-bead", []EvidenceLink{
		{Name: "prompt", Path: "prompt.md"},
	})
	if err := Publish(root, pre); err != nil {
		t.Fatalf("publish dispatching: %v", err)
	}
	started := pre.StartedAt

	// Ensure clock advances so UpdatedAt differs on slow/fast CI.
	time.Sleep(2 * time.Millisecond)

	public := &FizeauPublicResult{
		PublicSessionRef: "sess-public-1",
		SessionLogPath:   "/sessions/1.jsonl",
	}
	if err := TransitionToRunning(root, runID, public); err != nil {
		t.Fatalf("TransitionToRunning: %v", err)
	}

	got, err := Read(root, runID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got == nil {
		t.Fatal("record missing after transition")
	}
	if got.Phase != PhaseRunning {
		t.Fatalf("phase=%q, want running", got.Phase)
	}
	if !got.StartedAt.Equal(started) {
		t.Fatalf("started_at rewritten: got %v want %v", got.StartedAt, started)
	}
	if got.Fizeau == nil || got.Fizeau.PublicSessionRef != "sess-public-1" {
		t.Fatalf("fizeau=%+v", got.Fizeau)
	}
	if got.Fizeau.SessionLogPath != "/sessions/1.jsonl" {
		t.Fatalf("session_log_path=%q", got.Fizeau.SessionLogPath)
	}
	if got.FinishedAt != nil {
		t.Fatalf("finished_at should be nil for running: %v", got.FinishedAt)
	}

	raw, err := os.ReadFile(RecordPath(root, runID))
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatal("record.json not valid JSON")
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, forbidden := range []string{"harness", "provider", "model", "pid", "raw_output"} {
		if _, ok := m[forbidden]; ok {
			t.Errorf("forbidden top-level field %q present", forbidden)
		}
	}
}

func TestTransitionToRunning_IdempotentWhenAlreadyRunning(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const runID = "run-idempotent"
	if err := Publish(root, NewDispatching(runID, "b", nil)); err != nil {
		t.Fatal(err)
	}
	if err := TransitionToRunning(root, runID, &FizeauPublicResult{PublicSessionRef: "s1"}); err != nil {
		t.Fatal(err)
	}
	first, err := Read(root, runID)
	if err != nil || first == nil {
		t.Fatalf("read first: %v %#v", err, first)
	}
	// Second call must not error and must leave phase running.
	if err := TransitionToRunning(root, runID, &FizeauPublicResult{PublicSessionRef: "s2"}); err != nil {
		t.Fatalf("second transition: %v", err)
	}
	second, err := Read(root, runID)
	if err != nil || second == nil {
		t.Fatalf("read second: %v %#v", err, second)
	}
	if second.Phase != PhaseRunning {
		t.Fatalf("phase=%q", second.Phase)
	}
	// Idempotent path does not re-publish; first public session ref remains.
	if second.Fizeau == nil || second.Fizeau.PublicSessionRef != "s1" {
		t.Fatalf("fizeau=%+v want public_session_ref=s1", second.Fizeau)
	}
}

func TestTransitionToRunning_MissingRecord(t *testing.T) {
	t.Parallel()
	err := TransitionToRunning(t.TempDir(), "missing", nil)
	if err == nil {
		t.Fatal("expected error for missing record")
	}
}

func TestTransitionToRunning_EmptyPublicStillRunning(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const runID = "run-empty-public"
	if err := Publish(root, NewDispatching(runID, "b", nil)); err != nil {
		t.Fatal(err)
	}
	if err := TransitionToRunning(root, runID, nil); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root, runID)
	if err != nil || got == nil {
		t.Fatalf("read: %v %#v", err, got)
	}
	if got.Phase != PhaseRunning {
		t.Fatalf("phase=%q", got.Phase)
	}
	if got.Fizeau != nil {
		t.Fatalf("fizeau should stay nil for empty public: %+v", got.Fizeau)
	}
	// Directory exists under StoreDir.
	if _, err := os.Stat(filepath.Join(root, StoreDir, runID, RecordFileName)); err != nil {
		t.Fatal(err)
	}
}
