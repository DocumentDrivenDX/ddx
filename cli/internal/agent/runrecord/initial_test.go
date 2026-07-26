package runrecord

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRunRecordWriterRejectsRoutingDecisionInInitialRecord verifies the initial
// writer rejects concrete harness, provider, model, route, immediate result,
// final result, cost, provider PID/process metadata, and raw provider output
// before public route/result data exists (AC1 / ddx-a841737a).
func TestRunRecordWriterRejectsRoutingDecisionInInitialRecord(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2026, 7, 26, 8, 26, 28, 0, time.UTC)
	base := func() Record {
		return Record{
			Version:   SchemaVersion,
			RunID:     "run_reject_preroute",
			BeadID:    "ddx-a841737a",
			AttemptID: "20260726T082628-00265763",
			Phase:     PhaseDispatching,
			CreatedAt: fixed,
			StartedAt: fixed,
			UpdatedAt: fixed,
		}
	}

	cost := 0.42
	exit := 1
	cases := []struct {
		name    string
		mutate  func(*Record)
		wantSub string
	}{
		{
			name: "harness_in_correlation",
			mutate: func(r *Record) {
				r.Correlation = map[string]string{"harness": "claude-tui"}
			},
			wantSub: "pre-route field",
		},
		{
			name: "provider_in_correlation",
			mutate: func(r *Record) {
				r.Correlation = map[string]string{"provider": "anthropic"}
			},
			wantSub: "pre-route field",
		},
		{
			name: "model_in_correlation",
			mutate: func(r *Record) {
				r.Correlation = map[string]string{"model": "sonnet-4.6"}
			},
			wantSub: "pre-route field",
		},
		{
			name: "route_in_correlation",
			mutate: func(r *Record) {
				r.Correlation = map[string]string{"route": "policy=default"}
			},
			wantSub: "pre-route field",
		},
		{
			name: "immediate_result_via_fizeau",
			mutate: func(r *Record) {
				r.Fizeau = &FizeauPublicResult{ImmediateError: "spawn_failed"}
			},
			wantSub: "Fizeau route/result",
		},
		{
			name: "final_result_via_fizeau",
			mutate: func(r *Record) {
				r.Fizeau = &FizeauPublicResult{FinalStatus: "completed", FinalExitCode: &exit}
			},
			wantSub: "Fizeau route/result",
		},
		{
			name: "cost_via_fizeau",
			mutate: func(r *Record) {
				r.Fizeau = &FizeauPublicResult{CostUSD: &cost}
			},
			wantSub: "Fizeau route/result",
		},
		{
			name: "cost_in_correlation",
			mutate: func(r *Record) {
				r.Correlation = map[string]string{"cost_usd": "0.42"}
			},
			wantSub: "pre-route field",
		},
		{
			name: "provider_pid_in_correlation",
			mutate: func(r *Record) {
				r.Correlation = map[string]string{"provider_pid": "4242"}
			},
			wantSub: "pre-route field",
		},
		{
			name: "process_tree_in_correlation",
			mutate: func(r *Record) {
				r.Correlation = map[string]string{"process_tree": "1-2-3"}
			},
			wantSub: "pre-route field",
		},
		{
			name: "raw_provider_output_in_correlation",
			mutate: func(r *Record) {
				r.Correlation = map[string]string{"raw_output": "provider stream dump"}
			},
			wantSub: "pre-route field",
		},
		{
			name: "outcome_result_block",
			mutate: func(r *Record) {
				r.Outcome = &Outcome{Status: "success", Reason: "too_early"}
			},
			wantSub: "outcome/result",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			rec := base()
			// Unique run id per case so parallel cases never share a path.
			rec.RunID = "run_reject_" + tc.name
			tc.mutate(&rec)

			err := PublishInitial(root, rec)
			if err == nil {
				t.Fatalf("PublishInitial succeeded for %s, want rejection", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error=%q, want substring %q", err.Error(), tc.wantSub)
			}

			// Defense in depth: general Publish must reject the same dirty
			// dispatching record.
			if err2 := Publish(root, rec); err2 == nil {
				t.Fatalf("Publish also succeeded for dirty dispatching record %s", tc.name)
			}

			// No durable record.json may exist after rejection.
			if _, err := os.Stat(RecordPath(root, rec.RunID)); !os.IsNotExist(err) {
				t.Errorf("record.json written despite rejection (stat err=%v)", err)
			}
			// And no .ddx/runs substrate for this run id.
			if _, err := os.Stat(RunDir(root, rec.RunID)); !os.IsNotExist(err) {
				// Mkdir may happen after validation — ensure validation runs first.
				// publish validates before MkdirAll; directory must not exist.
				t.Errorf("run dir created despite rejection (stat err=%v)", err)
			}
		})
	}
}

// TestRunRecordInitialJSONOmitsRouteAndResultFields verifies a valid
// dispatching record serializes without harness, provider, model, route,
// result, or raw provider stream fields (AC2 / ddx-a841737a).
func TestRunRecordInitialJSONOmitsRouteAndResultFields(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2026, 7, 26, 8, 26, 28, 0, time.UTC)
	rec := NewDispatchingRecord(DispatchingParams{
		RunID:      "run_json_omits_preroute",
		BeadID:     "ddx-a841737a",
		AttemptID:  "20260726T082628-json-omit",
		WorkerID:   "worker-1",
		BaseRev:    "c9359ddecad3a4fa99bd54a55873e8b691c3cf14",
		PromptPath: ".ddx/executions/20260726T082628-00265763/prompt.md",
		Correlation: map[string]string{
			"session_id":  "eb-unit",
			"bundle_path": ".ddx/executions/20260726T082628-00265763",
		},
		Now: fixed,
	})

	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal map: %v", err)
	}

	// Top-level keys that must never appear on a valid initial record.
	for _, forbidden := range []string{
		"harness", "provider", "model", "route", "route_reason", "routing_policy",
		"result", "immediate_result", "final_result",
		"raw_output", "provider_output", "stdout", "stderr",
		"pid", "provider_pid", "process_tree",
		"cost", "cost_usd",
		// Typed blocks that hold post-route data must be omitted (omitempty).
		"fizeau", "outcome", "finished_at",
	} {
		if _, ok := asMap[forbidden]; ok {
			t.Errorf("initial JSON includes forbidden top-level field %q: %s", forbidden, raw)
		}
	}

	// Nested string scan for forbidden JSON object keys.
	rawStr := string(raw)
	for _, needle := range []string{
		`"harness"`, `"provider"`, `"model"`, `"route"`,
		`"raw_output"`, `"provider_pid"`, `"process_tree"`,
		`"cost_usd"`, `"final_status"`, `"immediate_error"`,
	} {
		if strings.Contains(rawStr, needle) {
			t.Errorf("initial JSON contains forbidden key %s: %s", needle, rawStr)
		}
	}

	// Durable publish must match the same omission contract.
	root := t.TempDir()
	if err := PublishInitial(root, rec); err != nil {
		t.Fatalf("PublishInitial: %v", err)
	}
	onDisk, err := os.ReadFile(RecordPath(root, rec.RunID))
	if err != nil {
		t.Fatalf("read on-disk: %v", err)
	}
	var diskMap map[string]json.RawMessage
	if err := json.Unmarshal(onDisk, &diskMap); err != nil {
		t.Fatalf("unmarshal on-disk: %v", err)
	}
	for _, forbidden := range []string{"harness", "provider", "model", "route", "fizeau", "outcome", "raw_output"} {
		if _, ok := diskMap[forbidden]; ok {
			t.Errorf("on-disk record includes forbidden field %q: %s", forbidden, onDisk)
		}
	}

	// Allowed DDx correlation keys remain present.
	for _, required := range []string{"version", "run_id", "phase", "correlation", "evidence"} {
		if _, ok := diskMap[required]; !ok {
			t.Errorf("on-disk record missing required field %q", required)
		}
	}
	if string(diskMap["phase"]) != `"dispatching"` {
		t.Errorf("phase JSON=%s, want \"dispatching\"", diskMap["phase"])
	}
}

// TestRunRecordInitialLifecyclePhaseDispatching verifies the only lifecycle
// phase accepted by the initial writer is dispatching (AC3 / ddx-a841737a).
func TestRunRecordInitialLifecyclePhaseDispatching(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2026, 7, 26, 8, 26, 28, 0, time.UTC)
	phases := []LifecyclePhase{
		PhaseRunning,
		PhaseTerminal,
		PhaseInterrupted,
		LifecyclePhase(""),
		LifecyclePhase("unknown"),
	}

	for _, phase := range phases {
		phase := phase
		t.Run(string(phase)+"_rejected", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			name := string(phase)
			if name == "" {
				name = "empty"
			}
			rec := Record{
				Version:   SchemaVersion,
				RunID:     "run_phase_" + name,
				BeadID:    "ddx-a841737a",
				AttemptID: "attempt-phase-" + name,
				Phase:     phase,
				StartedAt: fixed,
				UpdatedAt: fixed,
			}
			err := PublishInitial(root, rec)
			if err == nil {
				t.Fatalf("PublishInitial(phase=%q) succeeded, want rejection", phase)
			}
			if !strings.Contains(err.Error(), "initial writer accepts only phase") {
				t.Errorf("error=%q, want initial writer phase rejection", err.Error())
			}
			if !strings.Contains(err.Error(), string(PhaseDispatching)) {
				t.Errorf("error=%q, want mention of %q", err.Error(), PhaseDispatching)
			}
			if _, err := os.Stat(RecordPath(root, rec.RunID)); !os.IsNotExist(err) {
				t.Errorf("record written for rejected phase %q", phase)
			}
		})
	}

	// Happy path: only dispatching is accepted.
	t.Run("dispatching_accepted", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rec := NewDispatchingRecord(DispatchingParams{
			RunID:     "run_phase_dispatching_ok",
			AttemptID: "attempt-dispatching-ok",
			BeadID:    "ddx-a841737a",
			Now:       fixed,
		})
		if rec.Phase != PhaseDispatching {
			t.Fatalf("NewDispatchingRecord phase=%q", rec.Phase)
		}
		if err := PublishInitial(root, rec); err != nil {
			t.Fatalf("PublishInitial(dispatching): %v", err)
		}
		loaded, err := Read(root, rec.RunID)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if loaded == nil {
			t.Fatal("Read returned nil")
		}
		if loaded.Phase != PhaseDispatching {
			t.Errorf("loaded phase=%q, want %q", loaded.Phase, PhaseDispatching)
		}
		// Exactly one record under .ddx/runs.
		entries, err := os.ReadDir(filepath.Join(root, StoreDir))
		if err != nil {
			t.Fatalf("readdir: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("run dirs=%d, want 1", len(entries))
		}
	})
}
