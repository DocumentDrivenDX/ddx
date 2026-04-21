package agent

import (
	"encoding/json"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// stubBeadEventAppender is an in-memory appender. It is a stub, not a mock:
// assertions read back the stored events rather than asserting call order.
type stubBeadEventAppender struct {
	events []struct {
		BeadID string
		Event  bead.BeadEvent
	}
}

func (s *stubBeadEventAppender) AppendEvent(id string, event bead.BeadEvent) error {
	s.events = append(s.events, struct {
		BeadID string
		Event  bead.BeadEvent
	}{id, event})
	return nil
}

func TestAppendBeadCostEvidenceRecordsPerAttemptCost(t *testing.T) {
	app := &stubBeadEventAppender{}
	body := costEventBody{
		Harness:      "claude",
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-6",
		InputTokens:  12345,
		OutputTokens: 6789,
		CostUSD:      0.1234,
		DurationMS:   42000,
		ExitCode:     0,
	}
	appendBeadCostEvidence(app, "ddx-0001", "20260421T120000-abcdef12", body)

	if len(app.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(app.events))
	}
	got := app.events[0]
	if got.BeadID != "ddx-0001" {
		t.Fatalf("bead id: got %q", got.BeadID)
	}
	if got.Event.Kind != "cost" {
		t.Fatalf("kind: got %q, want cost", got.Event.Kind)
	}
	if got.Event.Actor != "ddx" {
		t.Fatalf("actor: got %q", got.Event.Actor)
	}
	if got.Event.Source != "ddx agent execute-bead" {
		t.Fatalf("source: got %q", got.Event.Source)
	}

	var parsed costEventBody
	if err := json.Unmarshal([]byte(got.Event.Body), &parsed); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, got.Event.Body)
	}
	if parsed.AttemptID != "20260421T120000-abcdef12" {
		t.Fatalf("attempt_id: got %q", parsed.AttemptID)
	}
	if parsed.InputTokens != 12345 || parsed.OutputTokens != 6789 {
		t.Fatalf("tokens: got in=%d out=%d", parsed.InputTokens, parsed.OutputTokens)
	}
	// TotalTokens must be populated even when caller didn't set it.
	if parsed.TotalTokens != 12345+6789 {
		t.Fatalf("total_tokens: got %d, want %d", parsed.TotalTokens, 12345+6789)
	}
	if parsed.CostUSD != 0.1234 {
		t.Fatalf("cost_usd: got %v", parsed.CostUSD)
	}
	if parsed.Model != "claude-sonnet-4-6" {
		t.Fatalf("model: got %q", parsed.Model)
	}
	if got.Event.Summary == "" {
		t.Fatalf("summary should not be empty")
	}
}

func TestAppendBeadCostEvidenceHonorsExplicitTotalTokens(t *testing.T) {
	app := &stubBeadEventAppender{}
	body := costEventBody{
		InputTokens:  0,
		OutputTokens: 0,
		TotalTokens:  5000, // caller provided a pre-aggregated value
		CostUSD:      0.01,
	}
	appendBeadCostEvidence(app, "ddx-0002", "att", body)
	if len(app.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(app.events))
	}
	var parsed costEventBody
	_ = json.Unmarshal([]byte(app.events[0].Event.Body), &parsed)
	if parsed.TotalTokens != 5000 {
		t.Fatalf("total_tokens should be preserved when caller set it: got %d", parsed.TotalTokens)
	}
}

func TestAppendBeadCostEvidenceSkipsWhenNoCost(t *testing.T) {
	app := &stubBeadEventAppender{}
	// All zero — nothing to record. Caller got a no-changes outcome or a
	// dry-run with no provider call.
	appendBeadCostEvidence(app, "ddx-0003", "att", costEventBody{})
	if len(app.events) != 0 {
		t.Fatalf("expected 0 events for all-zero body, got %d", len(app.events))
	}
}

func TestAppendBeadCostEvidenceSkipsWhenAppenderNil(t *testing.T) {
	// Must not panic.
	appendBeadCostEvidence(nil, "ddx-0004", "att", costEventBody{InputTokens: 1})
}

func TestAppendBeadCostEvidenceSkipsWhenBeadIDEmpty(t *testing.T) {
	app := &stubBeadEventAppender{}
	appendBeadCostEvidence(app, "", "att", costEventBody{InputTokens: 1})
	if len(app.events) != 0 {
		t.Fatalf("expected 0 events when beadID empty, got %d", len(app.events))
	}
}
