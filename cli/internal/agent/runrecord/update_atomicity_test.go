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

// withPublishTestHooks installs h for Publish until the returned cleanup runs.
// Test-only: lives in *_test.go so deadcode does not treat it as production
// dead. Tests must call cleanup when finished and must not install hooks from
// concurrent t.Parallel cases.
func withPublishTestHooks(h *publishHooks) (cleanup func()) {
	publishTestHooksMu.Lock()
	prev := publishTestHooks
	publishTestHooks = h
	publishTestHooksMu.Unlock()
	return func() {
		publishTestHooksMu.Lock()
		publishTestHooks = prev
		publishTestHooksMu.Unlock()
	}
}

// TestRunRecordUpdateAtomicitySurvivesKill injects interruption during running
// and terminal record updates and observes either the prior complete record or
// the next complete record — never partial JSON. It also proves those phase
// transitions use the same Publish atomic writer contract as the pre-dispatch
// path (Phase 3 WB-2 / ddx-0340d669).
func TestRunRecordUpdateAtomicitySurvivesKill(t *testing.T) {
	// Serial: installs package-level Publish hooks shared with production writers.
	killPhases := []publishFaultPhase{
		faultAfterTempCreate,
		faultAfterPartialWrite,
		faultAfterFullWrite,
		faultAfterFsyncFile,
		faultAfterRename,
		faultAfterFsyncDir,
	}
	disciplineOrder := []publishFaultPhase{
		faultAfterTempCreate,
		faultAfterFullWrite,
		faultAfterFsyncFile,
		faultAfterRename,
		faultAfterFsyncDir,
	}

	t.Run("running_update", func(t *testing.T) {
		// AC1: interrupt during dispatching → running; prior=dispatching or next=running.
		const runID = "run_atomic_running_update"
		const beadID = "ddx-0340d669"

		// AC3: happy-path observation — TransitionToRunning walks the same
		// temp → write → fsync → rename → fsync-dir discipline as Publish.
		{
			root := t.TempDir()
			prior := NewDispatching(runID, beadID, []EvidenceLink{
				{Name: "prompt", Path: "prompt.md", MediaType: "text/markdown"},
			})
			// Stable timestamps for prior-vs-next comparison after kill.
			prior.StartedAt = time.Date(2026, 7, 26, 7, 0, 0, 0, time.UTC)
			prior.UpdatedAt = prior.StartedAt
			if err := Publish(root, prior); err != nil {
				t.Fatalf("seed dispatching: %v", err)
			}
			assertValidRecordAt(t, root, runID, prior)

			var observed []publishFaultPhase
			cleanup := withPublishTestHooks(&publishHooks{
				onPhase: func(phase publishFaultPhase, recordPath, tmpPath string) {
					observed = append(observed, phase)
					if phase != faultAfterRename && phase != faultAfterFsyncDir {
						if filepath.Base(tmpPath) == RecordFileName {
							t.Errorf("phase %v: tmp path is record.json: %s", phase, tmpPath)
						}
						if !strings.Contains(tmpPath, ".tmp") {
							t.Errorf("phase %v: expected temp path, got %s", phase, tmpPath)
						}
						if filepath.Dir(tmpPath) != filepath.Dir(recordPath) {
							t.Errorf("phase %v: temp not in target dir", phase)
						}
					}
					if recordPath != RecordPath(root, runID) {
						t.Errorf("phase %v: recordPath=%q", phase, recordPath)
					}
				},
			})
			err := TransitionToRunning(root, runID, &FizeauPublicResult{
				PublicSessionRef: "sess-running-atomic",
				SessionLogPath:   "/var/fizeau/sessions/running.jsonl",
			})
			cleanup() // must clear before kill subtests install their own hooks
			if err != nil {
				t.Fatalf("TransitionToRunning with observation: %v", err)
			}
			if !reflect.DeepEqual(observed, disciplineOrder) {
				t.Fatalf("running update discipline phases=%v, want %v (same contract as pre-dispatch Publish)",
					observed, disciplineOrder)
			}
			got, err := Read(root, runID)
			if err != nil || got == nil {
				t.Fatalf("Read after running: %v %#v", err, got)
			}
			if got.Phase != PhaseRunning {
				t.Fatalf("phase=%q, want running", got.Phase)
			}
			assertNoForbiddenFieldsInFile(t, RecordPath(root, runID))
		}

		for _, phase := range killPhases {
			phase := phase
			t.Run(phaseName(phase), func(t *testing.T) {
				caseRoot := t.TempDir()
				prior := NewDispatching(runID, beadID, []EvidenceLink{
					{Name: "prompt", Path: "prompt.md"},
				})
				prior.StartedAt = time.Date(2026, 7, 26, 7, 0, 0, 0, time.UTC)
				prior.UpdatedAt = prior.StartedAt
				if err := Publish(caseRoot, prior); err != nil {
					t.Fatalf("seed: %v", err)
				}
				assertValidRecordAt(t, caseRoot, runID, prior)

				fault := phase
				cleanup := withPublishTestHooks(&publishHooks{faultAt: &fault})
				err := TransitionToRunning(caseRoot, runID, &FizeauPublicResult{
					PublicSessionRef: "sess-running-atomic",
					SessionLogPath:   "/var/fizeau/sessions/running.jsonl",
				})
				cleanup()
				// Transition wraps publish errors with %w; accept either form.
				if err == nil || (!errors.Is(err, errPublishKilled) && !strings.Contains(err.Error(), errPublishKilled.Error())) {
					t.Fatalf("expected kill during running update, got %v", err)
				}

				assertKillSurvivedCompleteRecord(t, caseRoot, runID, phase, prior, func(next Record) bool {
					return next.Phase == PhaseRunning &&
						next.RunID == runID &&
						next.BeadID == beadID &&
						next.Fizeau != nil &&
						next.Fizeau.PublicSessionRef == "sess-running-atomic"
				}, PhaseRunning)
			})
		}
	})

	t.Run("terminal_update_from_running", func(t *testing.T) {
		// AC2: interrupt during running → terminal; prior=running or next=terminal.
		const runID = "run_atomic_terminal_from_running"
		const beadID = "ddx-0340d669"

		{
			root := t.TempDir()
			if err := Publish(root, NewDispatching(runID, beadID, []EvidenceLink{
				{Name: "prompt", Path: "prompt.md"},
			})); err != nil {
				t.Fatalf("seed dispatching: %v", err)
			}
			if err := TransitionToRunning(root, runID, &FizeauPublicResult{
				PublicSessionRef: "sess-term-atomic",
			}); err != nil {
				t.Fatalf("seed running: %v", err)
			}
			prior, err := Read(root, runID)
			if err != nil || prior == nil || prior.Phase != PhaseRunning {
				t.Fatalf("prior running: %v %#v", err, prior)
			}

			var observed []publishFaultPhase
			cleanup := withPublishTestHooks(&publishHooks{
				onPhase: func(phase publishFaultPhase, recordPath, tmpPath string) {
					observed = append(observed, phase)
				},
			})
			exit := 0
			err = TransitionToTerminal(root, runID, TerminalInput{
				Outcome: Outcome{
					Status:          "success",
					Reason:          "public_final_ok",
					EvidenceVerdict: "public_final_only",
				},
				Public: &FizeauPublicResult{
					FinalStatus:    "success",
					FinalExitCode:  &exit,
					SessionLogPath: "/var/fizeau/sessions/term.jsonl",
				},
			})
			cleanup() // must clear before kill subtests install their own hooks
			if err != nil {
				t.Fatalf("TransitionToTerminal with observation: %v", err)
			}
			if !reflect.DeepEqual(observed, disciplineOrder) {
				t.Fatalf("terminal update discipline phases=%v, want %v (same contract as pre-dispatch Publish)",
					observed, disciplineOrder)
			}
			got, err := Read(root, runID)
			if err != nil || got == nil {
				t.Fatalf("Read after terminal: %v %#v", err, got)
			}
			if got.Phase != PhaseTerminal {
				t.Fatalf("phase=%q, want terminal", got.Phase)
			}
			assertNoForbiddenFieldsInFile(t, RecordPath(root, runID))
		}

		for _, phase := range killPhases {
			phase := phase
			t.Run(phaseName(phase), func(t *testing.T) {
				caseRoot := t.TempDir()
				if err := Publish(caseRoot, NewDispatching(runID, beadID, []EvidenceLink{
					{Name: "prompt", Path: "prompt.md"},
				})); err != nil {
					t.Fatalf("seed dispatching: %v", err)
				}
				if err := TransitionToRunning(caseRoot, runID, &FizeauPublicResult{
					PublicSessionRef: "sess-term-atomic",
				}); err != nil {
					t.Fatalf("seed running: %v", err)
				}
				prior, err := Read(caseRoot, runID)
				if err != nil || prior == nil {
					t.Fatalf("read prior: %v %#v", err, prior)
				}
				if prior.Phase != PhaseRunning {
					t.Fatalf("prior phase=%q, want running", prior.Phase)
				}

				fault := phase
				cleanup := withPublishTestHooks(&publishHooks{faultAt: &fault})
				exit := 0
				err = TransitionToTerminal(caseRoot, runID, TerminalInput{
					Outcome: Outcome{
						Status:          "success",
						Reason:          "public_final_ok",
						EvidenceVerdict: "public_final_only",
					},
					Public: &FizeauPublicResult{
						FinalStatus:    "success",
						FinalExitCode:  &exit,
						SessionLogPath: "/var/fizeau/sessions/term.jsonl",
					},
				})
				cleanup()
				if err == nil || (!errors.Is(err, errPublishKilled) && !strings.Contains(err.Error(), errPublishKilled.Error())) {
					t.Fatalf("expected kill during terminal update, got %v", err)
				}

				assertKillSurvivedCompleteRecord(t, caseRoot, runID, phase, *prior, func(next Record) bool {
					return next.Phase == PhaseTerminal &&
						next.RunID == runID &&
						next.Outcome != nil &&
						next.Outcome.Status == "success" &&
						next.FinishedAt != nil
				}, PhaseTerminal)
			})
		}
	})

	t.Run("terminal_update_from_dispatching", func(t *testing.T) {
		// AC2 also allows prior=dispatching when terminal is written from an
		// immediate-error path without a running intermediate.
		const runID = "run_atomic_terminal_from_dispatching"
		const beadID = "ddx-0340d669"

		for _, phase := range killPhases {
			phase := phase
			t.Run(phaseName(phase), func(t *testing.T) {
				caseRoot := t.TempDir()
				prior := NewDispatching(runID, beadID, []EvidenceLink{
					{Name: "prompt", Path: "prompt.md"},
				})
				prior.StartedAt = time.Date(2026, 7, 26, 7, 10, 0, 0, time.UTC)
				prior.UpdatedAt = prior.StartedAt
				if err := Publish(caseRoot, prior); err != nil {
					t.Fatalf("seed: %v", err)
				}

				fault := phase
				cleanup := withPublishTestHooks(&publishHooks{faultAt: &fault})
				err := TransitionToTerminal(caseRoot, runID, TerminalInput{
					Outcome: Outcome{
						Status:          "failure",
						Reason:          "provider_model_unavailable",
						EvidenceVerdict: "immediate_error",
					},
					Public: &FizeauPublicResult{
						ImmediateError: "provider_model_unavailable",
					},
				})
				cleanup()
				if err == nil || (!errors.Is(err, errPublishKilled) && !strings.Contains(err.Error(), errPublishKilled.Error())) {
					t.Fatalf("expected kill during terminal-from-dispatching update, got %v", err)
				}

				assertKillSurvivedCompleteRecord(t, caseRoot, runID, phase, prior, func(next Record) bool {
					return next.Phase == PhaseTerminal &&
						next.Outcome != nil &&
						next.Outcome.Status == "failure" &&
						next.Fizeau != nil &&
						next.Fizeau.ImmediateError == "provider_model_unavailable"
				}, PhaseTerminal)
			})
		}
	})
}

// assertKillSurvivedCompleteRecord checks that after a kill at phase, record.json
// is valid JSON and equals prior (pre-rename) or a complete next record (post-rename).
func assertKillSurvivedCompleteRecord(
	t *testing.T,
	root, runID string,
	phase publishFaultPhase,
	prior Record,
	nextOK func(Record) bool,
	wantNextPhase LifecyclePhase,
) {
	t.Helper()
	path := RecordPath(root, runID)
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("record.json missing after kill at %s: %v", phaseName(phase), readErr)
	}
	if !json.Valid(raw) {
		t.Fatalf("partial/torn JSON after kill at %s: %q", phaseName(phase), string(raw))
	}

	var got Record
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal after kill at %s: %v\nraw=%s", phaseName(phase), err, raw)
	}
	assertNoForbiddenFieldsInBytes(t, raw)

	switch phase {
	case faultAfterTempCreate, faultAfterPartialWrite, faultAfterFullWrite, faultAfterFsyncFile:
		if got.Phase != prior.Phase {
			t.Fatalf("pre-rename kill at %s: phase=%q want prior %q", phaseName(phase), got.Phase, prior.Phase)
		}
		if got.RunID != prior.RunID || got.AttemptID != prior.AttemptID {
			t.Fatalf("pre-rename kill rewrote identity: got=%+v prior=%+v", got, prior)
		}
		assertRecordEqual(t, got, prior)
	case faultAfterRename, faultAfterFsyncDir:
		if got.Phase != wantNextPhase {
			t.Fatalf("post-rename kill at %s: phase=%q want %q", phaseName(phase), got.Phase, wantNextPhase)
		}
		if !nextOK(got) {
			t.Fatalf("post-rename kill at %s: incomplete next record: %+v", phaseName(phase), got)
		}
	}

	loaded, err := Read(root, runID)
	if err != nil {
		t.Fatalf("Read after kill: %v", err)
	}
	if loaded == nil {
		t.Fatal("Read returned nil after kill with seeded prior")
	}
	switch phase {
	case faultAfterTempCreate, faultAfterPartialWrite, faultAfterFullWrite, faultAfterFsyncFile:
		if loaded.Phase != prior.Phase {
			t.Fatalf("Read pre-rename: phase=%q want %q", loaded.Phase, prior.Phase)
		}
	case faultAfterRename, faultAfterFsyncDir:
		if loaded.Phase != wantNextPhase {
			t.Fatalf("Read post-rename: phase=%q want %q", loaded.Phase, wantNextPhase)
		}
	}

	// Temp debris is allowed; it must not be the sole source of truth.
	entries, _ := os.ReadDir(RunDir(root, runID))
	for _, e := range entries {
		if e.Name() == RecordFileName {
			continue
		}
		if strings.Contains(e.Name(), ".tmp") && phase == faultAfterPartialWrite {
			tmpRaw, err := os.ReadFile(filepath.Join(RunDir(root, runID), e.Name()))
			if err == nil && !json.Valid(tmpRaw) {
				// Partial write intentionally leaves invalid JSON on temp only.
				continue
			}
		}
	}
}
