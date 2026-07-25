package runrecord

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestRunRecordAtomicPublishSurvivesKill injects interruption at each write
// phase and observes either the prior valid record or the next complete
// record, never partial JSON (AC1, AC2). Also asserts the publisher does not
// persist forbidden provider-process fields (AC3).
func TestRunRecordAtomicPublishSurvivesKill(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	const runID = "run_atomic_kill_probe"

	prior := Record{
		Version:   SchemaVersion,
		RunID:     runID,
		BeadID:    "ddx-128c35f5",
		AttemptID: "attempt-prior",
		Phase:     PhaseDispatching,
		StartedAt: time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC),
	}
	next := Record{
		Version:   SchemaVersion,
		RunID:     runID,
		BeadID:    "ddx-128c35f5",
		AttemptID: "attempt-next",
		Phase:     PhaseRunning,
		StartedAt: prior.StartedAt,
		UpdatedAt: time.Date(2026, 7, 25, 10, 5, 0, 0, time.UTC),
		Fizeau: &FizeauPublicResult{
			SessionLogPath:   "/var/ddx/sessions/opaque.jsonl",
			PublicSessionRef: "sess-public",
			FinalStatus:      "running",
		},
		Evidence: []EvidenceLink{
			{Name: "prompt", Path: "prompt.md", MediaType: "text/markdown"},
		},
	}

	// Happy path: publish prior, then observe full discipline order for next.
	if err := Publish(root, prior); err != nil {
		t.Fatalf("seed prior Publish: %v", err)
	}
	assertValidRecordAt(t, root, runID, prior)

	// AC2: successful publish walks temp → write → fsync file → rename → fsync dir.
	// Re-seed prior so the observed publish is an in-place update.
	if err := Publish(root, prior); err != nil {
		t.Fatalf("re-seed prior: %v", err)
	}
	var observed []publishFaultPhase
	hooks := &publishHooks{
		onPhase: func(phase publishFaultPhase, recordPath, tmpPath string) {
			observed = append(observed, phase)
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
			}
			wantRecord := RecordPath(root, runID)
			if recordPath != wantRecord {
				t.Errorf("phase %v: recordPath=%q want %q", phase, recordPath, wantRecord)
			}
		},
	}
	if err := publish(root, next, hooks); err != nil {
		t.Fatalf("publish next with observation: %v", err)
	}
	wantOrder := []publishFaultPhase{
		faultAfterTempCreate,
		faultAfterFullWrite,
		faultAfterFsyncFile,
		faultAfterRename,
		faultAfterFsyncDir,
	}
	if !reflect.DeepEqual(observed, wantOrder) {
		t.Fatalf("discipline phases=%v, want %v", observed, wantOrder)
	}
	assertValidRecordAt(t, root, runID, next)
	assertNoForbiddenFieldsInFile(t, RecordPath(root, runID))

	// AC1: inject kill at each phase; visible record is prior or next, never partial.
	killPhases := []publishFaultPhase{
		faultAfterTempCreate,
		faultAfterPartialWrite,
		faultAfterFullWrite,
		faultAfterFsyncFile,
		faultAfterRename,
		faultAfterFsyncDir,
	}
	for _, phase := range killPhases {
		phase := phase
		t.Run(phaseName(phase), func(t *testing.T) {
			// Isolate each kill case under its own project root.
			caseRoot := t.TempDir()
			if err := Publish(caseRoot, prior); err != nil {
				t.Fatalf("seed prior: %v", err)
			}
			assertValidRecordAt(t, caseRoot, runID, prior)

			fault := phase
			err := publish(caseRoot, next, &publishHooks{faultAt: &fault})
			// Kill phases before successful completion return errPublishKilled.
			// faultAfterFsyncDir fires after the durable publish finishes, so
			// the next record is fully published and the kill is post-commit.
			if phase == faultAfterFsyncDir {
				if !errors.Is(err, errPublishKilled) {
					t.Fatalf("expected kill after fsync dir, got %v", err)
				}
			} else if phase == faultAfterRename {
				// After rename the next record is already visible; kill skips dir fsync.
				if !errors.Is(err, errPublishKilled) {
					t.Fatalf("expected kill after rename, got %v", err)
				}
			} else {
				if !errors.Is(err, errPublishKilled) {
					t.Fatalf("expected errPublishKilled, got %v", err)
				}
			}

			path := RecordPath(caseRoot, runID)
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				// Pre-rename kill with no prior would mean missing file; we always
				// seeded prior, so missing file is a bug.
				t.Fatalf("record.json missing after kill at %s: %v", phaseName(phase), readErr)
			}
			if !json.Valid(raw) {
				t.Fatalf("partial/torn JSON after kill at %s: %q", phaseName(phase), string(raw))
			}

			var got Record
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("unmarshal after kill at %s: %v\nraw=%s", phaseName(phase), err, raw)
			}

			// Before rename (and partial write), the prior complete record must
			// still be visible. After rename (and after dir fsync), the next
			// complete record is visible.
			switch phase {
			case faultAfterTempCreate, faultAfterPartialWrite, faultAfterFullWrite, faultAfterFsyncFile:
				assertRecordEqual(t, got, prior)
			case faultAfterRename, faultAfterFsyncDir:
				assertRecordEqual(t, got, next)
			}
			assertNoForbiddenFieldsInBytes(t, raw)

			// Temp debris after pre-rename kill is allowed; it must not be
			// named record.json and must not be what Read returns.
			entries, _ := os.ReadDir(RunDir(caseRoot, runID))
			for _, e := range entries {
				if e.Name() == RecordFileName {
					continue
				}
				// Any leftover temp must not parse as the sole source of truth;
				// Read must still return a complete prior/next record.
				if strings.Contains(e.Name(), ".tmp") {
					tmpRaw, err := os.ReadFile(filepath.Join(RunDir(caseRoot, runID), e.Name()))
					if err != nil {
						t.Fatalf("read temp debris: %v", err)
					}
					// Partial write intentionally leaves invalid JSON on temp only.
					if phase == faultAfterPartialWrite && !json.Valid(tmpRaw) {
						continue
					}
				}
			}
			// Read API must agree with the on-disk complete record.
			loaded, err := Read(caseRoot, runID)
			if err != nil {
				t.Fatalf("Read after kill: %v", err)
			}
			if loaded == nil {
				t.Fatal("Read returned nil after kill with seeded prior")
			}
			switch phase {
			case faultAfterTempCreate, faultAfterPartialWrite, faultAfterFullWrite, faultAfterFsyncFile:
				assertRecordEqual(t, *loaded, prior)
			case faultAfterRename, faultAfterFsyncDir:
				assertRecordEqual(t, *loaded, next)
			}
		})
	}

	// First-publish kill before rename: no record.json (or no partial).
	t.Run("first_publish_kill_before_rename", func(t *testing.T) {
		caseRoot := t.TempDir()
		first := prior
		first.RunID = "run_first_publish"
		for _, phase := range []publishFaultPhase{
			faultAfterTempCreate,
			faultAfterPartialWrite,
			faultAfterFullWrite,
			faultAfterFsyncFile,
		} {
			phase := phase
			// Fresh dir each sub-phase via unique run id.
			rec := first
			rec.RunID = "run_first_" + phaseName(phase)
			fault := phase
			err := publish(caseRoot, rec, &publishHooks{faultAt: &fault})
			if !errors.Is(err, errPublishKilled) {
				t.Fatalf("phase %s: want kill, got %v", phaseName(phase), err)
			}
			path := RecordPath(caseRoot, rec.RunID)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				// Must not leave a partial record.json when there was no prior.
				raw, readErr := os.ReadFile(path)
				if readErr == nil && !json.Valid(raw) {
					t.Fatalf("phase %s: partial record.json with no prior: %q", phaseName(phase), raw)
				}
				if readErr == nil {
					t.Fatalf("phase %s: record.json exists before rename with no prior (raw=%s)", phaseName(phase), raw)
				}
			}
			loaded, err := Read(caseRoot, rec.RunID)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if loaded != nil {
				t.Fatalf("phase %s: Read returned a record before rename: %+v", phaseName(phase), loaded)
			}
		}
	})
}

func phaseName(p publishFaultPhase) string {
	switch p {
	case faultAfterTempCreate:
		return "after_temp_create"
	case faultAfterPartialWrite:
		return "after_partial_write"
	case faultAfterFullWrite:
		return "after_full_write"
	case faultAfterFsyncFile:
		return "after_fsync_file"
	case faultAfterRename:
		return "after_rename"
	case faultAfterFsyncDir:
		return "after_fsync_dir"
	default:
		return "unknown"
	}
}

func assertValidRecordAt(t *testing.T, root, runID string, want Record) {
	t.Helper()
	path := RecordPath(root, runID)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !json.Valid(raw) {
		t.Fatalf("invalid JSON at %s: %q", path, raw)
	}
	var got Record
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	assertRecordEqual(t, got, want)
	assertNoForbiddenFieldsInBytes(t, raw)
}

func assertRecordEqual(t *testing.T, got, want Record) {
	t.Helper()
	if got.Version != want.Version {
		t.Errorf("version=%d want %d", got.Version, want.Version)
	}
	if got.RunID != want.RunID {
		t.Errorf("run_id=%q want %q", got.RunID, want.RunID)
	}
	if got.BeadID != want.BeadID {
		t.Errorf("bead_id=%q want %q", got.BeadID, want.BeadID)
	}
	if got.AttemptID != want.AttemptID {
		t.Errorf("attempt_id=%q want %q", got.AttemptID, want.AttemptID)
	}
	if got.Phase != want.Phase {
		t.Errorf("phase=%q want %q", got.Phase, want.Phase)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("started_at=%v want %v", got.StartedAt, want.StartedAt)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("updated_at=%v want %v", got.UpdatedAt, want.UpdatedAt)
	}
	if (got.Fizeau == nil) != (want.Fizeau == nil) {
		t.Errorf("fizeau presence got=%v want=%v", got.Fizeau != nil, want.Fizeau != nil)
	}
	if got.Fizeau != nil && want.Fizeau != nil {
		if got.Fizeau.SessionLogPath != want.Fizeau.SessionLogPath {
			t.Errorf("session_log_path=%q want %q", got.Fizeau.SessionLogPath, want.Fizeau.SessionLogPath)
		}
		if got.Fizeau.PublicSessionRef != want.Fizeau.PublicSessionRef {
			t.Errorf("public_session_ref=%q want %q", got.Fizeau.PublicSessionRef, want.Fizeau.PublicSessionRef)
		}
	}
	if got.AttemptID == want.AttemptID && got.Phase == want.Phase {
		// core identity matches — enough to distinguish prior vs next
		return
	}
}

func assertNoForbiddenFieldsInFile(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	assertNoForbiddenFieldsInBytes(t, raw)
}

func assertNoForbiddenFieldsInBytes(t *testing.T, raw []byte) {
	t.Helper()
	forbidden := []string{
		"raw_output",
		"provider_output",
		"output_excerpt",
		"stdout",
		"stderr",
		"provider_pid",
		"process_tree",
		"provider_process_tree",
		"children_pids",
		"process_tree_metadata",
		"session_canonical_state",
		"provider_session_state",
		"provider_session_canonical_state",
		"canonical_state",
	}
	s := string(raw)
	for _, tag := range forbidden {
		needle := `"` + tag + `"`
		if strings.Contains(s, needle) {
			t.Errorf("published record contains forbidden field %q: %s", tag, s)
		}
	}
	// Bare "pid" as a JSON key (not as a substring of another word).
	if strings.Contains(s, `"pid"`) {
		t.Errorf("published record contains forbidden field %q: %s", "pid", s)
	}
}
