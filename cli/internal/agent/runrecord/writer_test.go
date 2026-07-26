package runrecord

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestRunRecordWriterCreatesDispatchingRecordAtomically proves the writer
// creates .ddx/runs/<run-id>/record.json with lifecycle phase dispatching
// using temp-file, fsync, rename, and directory fsync semantics (AC1).
func TestRunRecordWriterCreatesDispatchingRecordAtomically(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	const runID = "run_writer_dispatching_atomic"
	const beadID = "ddx-67fe97c1"

	rec := NewDispatching(runID, beadID, []EvidenceLink{
		{Name: "prompt", Path: "prompt.md", MediaType: "text/markdown"},
	})
	if rec.Phase != PhaseDispatching {
		t.Fatalf("NewDispatching phase=%q, want %q", rec.Phase, PhaseDispatching)
	}

	var observed []publishFaultPhase
	hooks := &publishHooks{
		onPhase: func(phase publishFaultPhase, recordPath, tmpPath string) {
			observed = append(observed, phase)
			wantRecord := RecordPath(root, runID)
			if recordPath != wantRecord {
				t.Errorf("phase %v: recordPath=%q want %q", phase, recordPath, wantRecord)
			}
			// Pre-rename steps must write only through a temp file in the run dir.
			if phase != faultAfterRename && phase != faultAfterFsyncDir {
				if filepath.Base(tmpPath) == RecordFileName {
					t.Errorf("phase %v: tmp path is record.json itself: %s", phase, tmpPath)
				}
				if !strings.Contains(tmpPath, ".tmp") {
					t.Errorf("phase %v: expected temp path, got %s", phase, tmpPath)
				}
				if filepath.Dir(tmpPath) != filepath.Dir(recordPath) {
					t.Errorf("phase %v: temp not in target dir: tmp=%s record=%s", phase, tmpPath, recordPath)
				}
				// Final path must not yet be the sole durable source mid-flight.
				if phase == faultAfterTempCreate || phase == faultAfterFullWrite || phase == faultAfterFsyncFile {
					if _, err := os.Stat(recordPath); !os.IsNotExist(err) {
						// First publish: record.json must not appear before rename.
						t.Errorf("phase %v: record.json exists before rename: %v", phase, err)
					}
				}
			}
		},
	}

	if err := publish(root, rec, hooks); err != nil {
		t.Fatalf("publish dispatching record: %v", err)
	}

	wantOrder := []publishFaultPhase{
		faultAfterTempCreate,
		faultAfterFullWrite,
		faultAfterFsyncFile,
		faultAfterRename,
		faultAfterFsyncDir,
	}
	if !reflect.DeepEqual(observed, wantOrder) {
		t.Fatalf("discipline phases=%v, want %v (temp → write → fsync file → rename → fsync dir)", observed, wantOrder)
	}

	path := RecordPath(root, runID)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !json.Valid(raw) {
		t.Fatalf("record.json is not valid JSON: %q", raw)
	}

	var got Record
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Phase != PhaseDispatching {
		t.Errorf("phase=%q, want %q", got.Phase, PhaseDispatching)
	}
	if got.RunID != runID {
		t.Errorf("run_id=%q, want %q", got.RunID, runID)
	}
	if got.BeadID != beadID {
		t.Errorf("bead_id=%q, want %q", got.BeadID, beadID)
	}
	if got.Version != SchemaVersion {
		t.Errorf("version=%d, want %d", got.Version, SchemaVersion)
	}
	if got.Fizeau != nil {
		t.Errorf("dispatching record must not carry Fizeau public fields, got %+v", got.Fizeau)
	}

	// Public Publish entry point must yield the same durable path/shape.
	loaded, err := Read(root, runID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if loaded == nil {
		t.Fatal("Read returned nil after successful publish")
	}
	if loaded.Phase != PhaseDispatching {
		t.Errorf("Read phase=%q, want %q", loaded.Phase, PhaseDispatching)
	}
	assertNoForbiddenFieldsInBytes(t, raw)
}

// TestRunRecordWriterCreatesRunDirectory verifies a missing
// .ddx/runs/<run-id>/ directory is created without affecting sibling run
// directories (AC2).
func TestRunRecordWriterCreatesRunDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	const siblingID = "run_sibling_existing"
	const newID = "run_new_missing_dir"

	// Seed a sibling run directory + record so isolation can be checked.
	sibling := NewDispatching(siblingID, "ddx-sibling", nil)
	sibling.StartedAt = time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	sibling.UpdatedAt = sibling.StartedAt
	if err := Publish(root, sibling); err != nil {
		t.Fatalf("seed sibling: %v", err)
	}
	siblingPath := RecordPath(root, siblingID)
	siblingBefore, err := os.ReadFile(siblingPath)
	if err != nil {
		t.Fatalf("read sibling before: %v", err)
	}
	siblingInfoBefore, err := os.Stat(RunDir(root, siblingID))
	if err != nil {
		t.Fatalf("stat sibling dir before: %v", err)
	}

	// Ensure the new run directory does not exist yet.
	newDir := RunDir(root, newID)
	if _, err := os.Stat(newDir); !os.IsNotExist(err) {
		t.Fatalf("new run dir already exists: %v", err)
	}
	// Parent .ddx/runs may exist from the sibling; that is fine.

	rec := NewDispatching(newID, "ddx-67fe97c1", nil)
	if err := Publish(root, rec); err != nil {
		t.Fatalf("Publish new run: %v", err)
	}

	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatalf("expected run directory %s: %v", newDir, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", newDir)
	}
	if _, err := os.Stat(RecordPath(root, newID)); err != nil {
		t.Fatalf("expected record.json under new run dir: %v", err)
	}

	// Sibling must be byte-identical and still a directory.
	siblingAfter, err := os.ReadFile(siblingPath)
	if err != nil {
		t.Fatalf("read sibling after: %v", err)
	}
	if !reflect.DeepEqual(siblingBefore, siblingAfter) {
		t.Fatalf("sibling record changed:\nbefore=%s\nafter=%s", siblingBefore, siblingAfter)
	}
	siblingInfoAfter, err := os.Stat(RunDir(root, siblingID))
	if err != nil {
		t.Fatalf("stat sibling dir after: %v", err)
	}
	if !siblingInfoAfter.IsDir() {
		t.Fatal("sibling run dir is no longer a directory")
	}
	// ModTime may change on some FS when parent is rewritten; content isolation
	// is the load-bearing check. Keep inode-ish identity where available via name.
	_ = siblingInfoBefore

	// Exactly two run directories under .ddx/runs.
	entries, err := os.ReadDir(filepath.Join(root, StoreDir))
	if err != nil {
		t.Fatalf("readdir runs: %v", err)
	}
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names[e.Name()] = true
		}
	}
	if !names[siblingID] || !names[newID] {
		t.Fatalf("run dirs=%v, want both %q and %q", names, siblingID, newID)
	}
	if len(names) != 2 {
		t.Fatalf("run dir count=%d, want 2 (no extras/clobber): %v", len(names), names)
	}
}

// TestRunRecordWriterRejectsEmptyRunID verifies the writer fails before writing
// anything when run ID is empty or path-unsafe (AC3).
func TestRunRecordWriterRejectsEmptyRunID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		runID string
	}{
		{name: "empty", runID: ""},
		{name: "whitespace", runID: "   "},
		{name: "dot", runID: "."},
		{name: "dotdot", runID: ".."},
		{name: "slash_prefix", runID: "/abs/run"},
		{name: "relative_escape", runID: "../escape"},
		{name: "nested_segment", runID: "a/b"},
		{name: "backslash", runID: `a\b`},
		{name: "leading_space", runID: " run"},
		{name: "trailing_space", runID: "run "},
		{name: "null_byte", runID: "run\x00id"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()

			// Marker that must remain the only content under the project root
			// if the writer correctly fails before any runs substrate write.
			marker := filepath.Join(root, "keep-me.txt")
			if err := os.WriteFile(marker, []byte("untouched"), 0o644); err != nil {
				t.Fatalf("write marker: %v", err)
			}

			rec := Record{
				Version:   SchemaVersion,
				RunID:     tc.runID,
				Phase:     PhaseDispatching,
				StartedAt: time.Date(2026, 7, 26, 7, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 7, 26, 7, 0, 0, 0, time.UTC),
			}
			err := Publish(root, rec)
			if err == nil {
				t.Fatalf("Publish(%q) succeeded, want error", tc.runID)
			}
			errMsg := err.Error()
			if tc.runID == "" || strings.TrimSpace(tc.runID) == "" {
				if !strings.Contains(errMsg, "empty run_id") {
					t.Errorf("error=%q, want empty run_id", errMsg)
				}
			} else if !strings.Contains(errMsg, "path-unsafe") && !strings.Contains(errMsg, "empty run_id") {
				t.Errorf("error=%q, want path-unsafe or empty run_id", errMsg)
			}

			// No .ddx/runs substrate and no record.json anywhere under root.
			if _, err := os.Stat(filepath.Join(root, StoreDir)); !os.IsNotExist(err) {
				t.Errorf(".ddx/runs created despite invalid run_id %q (stat err=%v)", tc.runID, err)
				_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
					if err == nil && !info.IsDir() && filepath.Base(path) == RecordFileName {
						t.Errorf("found record.json after rejected publish: %s", path)
					}
					return nil
				})
			}
			// Escape attempts must not create sibling dirs outside .ddx/runs either.
			if strings.Contains(tc.runID, "..") {
				if _, err := os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
					t.Errorf("path escape created %s/escape", root)
				}
			}

			raw, err := os.ReadFile(marker)
			if err != nil || string(raw) != "untouched" {
				t.Errorf("marker file disturbed: err=%v raw=%q", err, raw)
			}
		})
	}
}
