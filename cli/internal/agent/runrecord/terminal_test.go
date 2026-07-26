package runrecord

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestTransitionToTerminal_FromDispatchingImmediateError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const runID = "20260726T120000-term-imm"

	pre := NewDispatching(runID, "ddx-bead", []EvidenceLink{
		{Name: "prompt", Path: "prompt.md"},
	})
	if err := Publish(root, pre); err != nil {
		t.Fatalf("publish dispatching: %v", err)
	}
	started := pre.StartedAt
	time.Sleep(2 * time.Millisecond)

	err := TransitionToTerminal(root, runID, TerminalInput{
		Outcome: Outcome{
			Status:          "failure",
			Reason:          "provider_model_unavailable",
			EvidenceVerdict: "immediate_error",
		},
		Public: &FizeauPublicResult{
			ImmediateError: "provider_model_unavailable",
		},
	})
	if err != nil {
		t.Fatalf("TransitionToTerminal: %v", err)
	}

	got, err := Read(root, runID)
	if err != nil || got == nil {
		t.Fatalf("Read: %v %#v", err, got)
	}
	if got.Phase != PhaseTerminal {
		t.Fatalf("phase=%q, want terminal", got.Phase)
	}
	if !got.StartedAt.Equal(started) {
		t.Fatalf("started_at rewritten: got %v want %v", got.StartedAt, started)
	}
	if got.FinishedAt == nil || got.FinishedAt.IsZero() {
		t.Fatal("finished_at must be set on terminal")
	}
	if got.Outcome == nil || got.Outcome.Status != "failure" || got.Outcome.Reason != "provider_model_unavailable" {
		t.Fatalf("outcome=%+v", got.Outcome)
	}
	if got.Fizeau == nil || got.Fizeau.ImmediateError != "provider_model_unavailable" {
		t.Fatalf("fizeau=%+v", got.Fizeau)
	}
	if evidencePathByName(got.Evidence, "prompt") != "prompt.md" {
		t.Fatalf("prompt evidence lost: %+v", got.Evidence)
	}

	raw, err := os.ReadFile(RecordPath(root, runID))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatal("record.json not valid JSON")
	}
}

func TestTransitionToTerminal_FromRunningWithCostAndEvidence(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const runID = "20260726T120000-term-final"

	if err := Publish(root, NewDispatching(runID, "ddx-bead", []EvidenceLink{
		{Name: "bundle", Path: ".ddx/executions/" + runID},
	})); err != nil {
		t.Fatal(err)
	}
	if err := TransitionToRunning(root, runID, &FizeauPublicResult{
		PublicSessionRef: "sess-1",
	}); err != nil {
		t.Fatal(err)
	}

	exit := 0
	cost := 0.042
	inTok, outTok, total := 100, 50, 150
	err := TransitionToTerminal(root, runID, TerminalInput{
		Outcome: Outcome{
			Status:          "success",
			Reason:          "public_final_ok",
			EvidenceVerdict: "result_artifact_present",
		},
		Public: &FizeauPublicResult{
			SessionLogPath: "/sessions/1.jsonl",
			FinalStatus:    "success",
			FinalExitCode:  &exit,
			CostUSD:        &cost,
			InputTokens:    &inTok,
			OutputTokens:   &outTok,
			TotalTokens:    &total,
		},
		AdditionalEvidence: []EvidenceLink{
			{Name: "result", Path: ".ddx/executions/" + runID + "/result.json", MediaType: "application/json"},
		},
	})
	if err != nil {
		t.Fatalf("TransitionToTerminal: %v", err)
	}

	got, err := Read(root, runID)
	if err != nil || got == nil {
		t.Fatalf("Read: %v %#v", err, got)
	}
	if got.Phase != PhaseTerminal {
		t.Fatalf("phase=%q", got.Phase)
	}
	if got.Fizeau == nil {
		t.Fatal("fizeau nil")
	}
	if got.Fizeau.PublicSessionRef != "sess-1" {
		t.Fatalf("public_session_ref not preserved: %q", got.Fizeau.PublicSessionRef)
	}
	if got.Fizeau.CostUSD == nil || *got.Fizeau.CostUSD != cost {
		t.Fatalf("cost_usd=%v", got.Fizeau.CostUSD)
	}
	if got.Fizeau.InputTokens == nil || *got.Fizeau.InputTokens != inTok {
		t.Fatalf("input_tokens=%v", got.Fizeau.InputTokens)
	}
	if evidencePathByName(got.Evidence, "result") == "" {
		t.Fatalf("result evidence missing: %+v", got.Evidence)
	}
	if evidencePathByName(got.Evidence, "bundle") == "" {
		t.Fatalf("bundle evidence lost: %+v", got.Evidence)
	}
}

func TestTransitionToTerminal_IdempotentWhenAlreadyTerminal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const runID = "run-term-idem"
	if err := Publish(root, NewDispatching(runID, "b", nil)); err != nil {
		t.Fatal(err)
	}
	if err := TransitionToTerminal(root, runID, TerminalInput{
		Outcome: Outcome{Status: "failure", Reason: "x"},
		Public:  &FizeauPublicResult{ImmediateError: "x"},
	}); err != nil {
		t.Fatal(err)
	}
	first, _ := Read(root, runID)
	if err := TransitionToTerminal(root, runID, TerminalInput{
		Outcome: Outcome{Status: "success"},
	}); err != nil {
		t.Fatalf("second: %v", err)
	}
	second, _ := Read(root, runID)
	if second.Phase != PhaseTerminal {
		t.Fatalf("phase=%q", second.Phase)
	}
	if second.Outcome == nil || second.Outcome.Status != first.Outcome.Status {
		t.Fatalf("idempotent path rewrote outcome: %+v vs %+v", second.Outcome, first.Outcome)
	}
}

func TestTransitionToTerminal_MissingRecord(t *testing.T) {
	t.Parallel()
	err := TransitionToTerminal(t.TempDir(), "missing", TerminalInput{
		Outcome: Outcome{Status: "failure"},
	})
	if err == nil {
		t.Fatal("expected error for missing record")
	}
}

func evidencePathByName(evidence []EvidenceLink, name string) string {
	for _, e := range evidence {
		if e.Name == name {
			return e.Path
		}
	}
	return ""
}
