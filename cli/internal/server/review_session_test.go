package server

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func newReviewSessionTestRoot(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	return t.TempDir()
}

func TestReviewSession_CreatePersistsManifest(t *testing.T) {
	workDir := newReviewSessionTestRoot(t)
	store := NewReviewSessionStore(workDir)
	session := ReviewSession{
		ID:             "review-001",
		ArtifactID:     "art-123",
		ArtifactSHA:    "sha-abc",
		ArtifactGitRev: "gitrev-def",
		SystemRubric:   "rubric body",
		TemplateRef:    "template://review/default",
		PromptRef:      "prompt://review/default",
		Status:         "open",
		CostUSD:        1.25,
		MaxBillableUSD: 5.00,
	}

	if err := store.Create(session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	manifestPath := ddxroot.JoinProject(workDir, reviewSessionsDirName, session.ID, reviewManifestName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("reading manifest.json: %v", err)
	}
	var got reviewSessionManifest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing manifest.json: %v", err)
	}

	want := reviewSessionManifestFrom(session)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("manifest.json = %+v, want %+v", got, want)
	}
}

func TestReviewSession_AppendTurn_TurnsJsonl(t *testing.T) {
	workDir := newReviewSessionTestRoot(t)
	store := NewReviewSessionStore(workDir)
	sessionID := "review-append-001"
	if err := store.Create(ReviewSession{ID: sessionID, Status: "open"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	turn := ReviewTurn{
		Actor:     "reviewer",
		Content:   "looks good",
		CostUSD:   0.12,
		CreatedAt: time.Date(2026, 5, 5, 6, 0, 0, 0, time.UTC),
	}
	if err := store.AppendTurn(sessionID, turn); err != nil {
		t.Fatalf("AppendTurn() error = %v", err)
	}

	turnsPath := ddxroot.JoinProject(workDir, reviewSessionsDirName, sessionID, reviewTurnsName)
	data, err := os.ReadFile(turnsPath)
	if err != nil {
		t.Fatalf("reading turns.jsonl: %v", err)
	}
	lines := splitNonEmptyLines(string(data))
	if len(lines) != 1 {
		t.Fatalf("turns.jsonl line count = %d, want 1", len(lines))
	}
	var got ReviewTurn
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("parsing turns.jsonl: %v", err)
	}
	if !reflect.DeepEqual(got, turn) {
		t.Fatalf("turn = %+v, want %+v", got, turn)
	}
}

func TestReviewSession_RoundTrip(t *testing.T) {
	workDir := newReviewSessionTestRoot(t)
	store := NewReviewSessionStore(workDir)
	sessionID := "review-roundtrip-001"
	want := ReviewSession{
		ID:             sessionID,
		ArtifactID:     "art-321",
		ArtifactSHA:    "sha-654",
		ArtifactGitRev: "gitrev-987",
		SystemRubric:   "system rubric",
		TemplateRef:    "template://review/strict",
		PromptRef:      "prompt://review/strict",
		Status:         "in_progress",
		CostUSD:        2.50,
		MaxBillableUSD: 8.00,
	}
	if err := store.Create(want); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	turns := []ReviewTurn{
		{
			Actor:     "reviewer",
			Content:   "first pass",
			CostUSD:   0.50,
			CreatedAt: time.Date(2026, 5, 5, 6, 30, 0, 0, time.UTC),
		},
		{
			Actor:     "reviewer",
			Content:   "second pass",
			CostUSD:   0.75,
			CreatedAt: time.Date(2026, 5, 5, 6, 45, 0, 0, time.UTC),
		},
	}
	for _, turn := range turns {
		if err := store.AppendTurn(sessionID, turn); err != nil {
			t.Fatalf("AppendTurn() error = %v", err)
		}
	}
	want.Turns = turns
	// AppendTurn accumulates each turn's cost into the manifest's CostUSD so
	// that subsequent cap-enforcement checks see the correct running total.
	// After appending 0.50 + 0.75 the manifest reflects 2.50 + 0.50 + 0.75.
	want.CostUSD = 3.75

	got, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, &want) {
		t.Fatalf("round-trip session = %+v, want %+v", got, want)
	}
}

// TestReview_CostCap_ReturnsCostCapExceeded verifies that AppendTurn returns a
// *ReviewCostCapExceededError (not nil, not a different error) when the turn's
// cost would push the session past MaxBillableUSD, and that the turn was NOT
// written to turns.jsonl.
func TestReview_CostCap_ReturnsCostCapExceeded(t *testing.T) {
	workDir := newReviewSessionTestRoot(t)
	store := NewReviewSessionStore(workDir)
	sessionID := "review-cap-001"
	// Session with a tight $0.50 cap; seed with $0.40 already spent.
	if err := store.Create(ReviewSession{
		ID:             sessionID,
		Status:         "open",
		CostUSD:        0.40,
		MaxBillableUSD: 0.50,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// A turn that would cost $0.20 — pushing total to $0.60, over the $0.50 cap.
	turn := ReviewTurn{
		Actor:   "reviewer",
		Content: "review output",
		CostUSD: 0.20,
	}
	err := store.AppendTurn(sessionID, turn)
	if err == nil {
		t.Fatal("AppendTurn() should have returned an error, got nil")
	}
	var capErr *ReviewCostCapExceededError
	if !errors.As(err, &capErr) {
		t.Fatalf("AppendTurn() error type = %T, want *ReviewCostCapExceededError", err)
	}
	if capErr.SessionID != sessionID {
		t.Errorf("ReviewCostCapExceededError.SessionID = %q, want %q", capErr.SessionID, sessionID)
	}
	if capErr.MaxUSD != 0.50 {
		t.Errorf("ReviewCostCapExceededError.MaxUSD = %v, want 0.50", capErr.MaxUSD)
	}

	// The turn must NOT have been written.
	turnsPath := ddxroot.JoinProject(workDir, reviewSessionsDirName, sessionID, reviewTurnsName)
	data, err := os.ReadFile(turnsPath)
	if err != nil {
		t.Fatalf("reading turns.jsonl: %v", err)
	}
	if lines := splitNonEmptyLines(string(data)); len(lines) != 0 {
		t.Fatalf("turns.jsonl should be empty after cap refusal, got %d line(s)", len(lines))
	}
}

// TestReview_RefusalCodes_HaveStructuredBody verifies that all three stable
// refusal codes produce ReviewRefusalBody values with the required fields.
func TestReview_RefusalCodes_HaveStructuredBody(t *testing.T) {
	t.Run("PROMPT_BUDGET_EXCEEDED", func(t *testing.T) {
		body := ReviewRefusalBody{
			Code:           RefusalCodePromptBudgetExceeded,
			Message:        "PROMPT_BUDGET_EXCEEDED: pinned floor observed 512 bytes exceeds cap 256 bytes",
			Retryable:      false,
			OperatorAction: "reduce prompt size or raise max_prompt_bytes",
		}
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal(ReviewRefusalBody) error = %v", err)
		}
		var got ReviewRefusalBody
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("json.Unmarshal error = %v", err)
		}
		if got.Code != RefusalCodePromptBudgetExceeded {
			t.Errorf("code = %q, want %q", got.Code, RefusalCodePromptBudgetExceeded)
		}
		if got.Message == "" {
			t.Error("message must not be empty")
		}
		if got.OperatorAction == "" {
			t.Error("operator_action must not be empty")
		}
	})

	t.Run("COST_CAP_EXCEEDED", func(t *testing.T) {
		capErr := &ReviewCostCapExceededError{
			SessionID:   "sess-1",
			CurrentCost: 0.40,
			TurnCost:    0.20,
			MaxUSD:      0.50,
		}
		body := capErr.RefusalBody()
		if body.Code != RefusalCodeCostCapExceeded {
			t.Errorf("code = %q, want %q", body.Code, RefusalCodeCostCapExceeded)
		}
		if body.Message == "" {
			t.Error("message must not be empty")
		}
		if body.OperatorAction == "" {
			t.Error("operator_action must not be empty")
		}
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal error = %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("json.Unmarshal error = %v", err)
		}
		for _, field := range []string{"code", "message", "retryable", "operator_action"} {
			if _, ok := got[field]; !ok {
				t.Errorf("refusal body missing field %q", field)
			}
		}
	})

	t.Run("REVIEWER_UNAVAILABLE", func(t *testing.T) {
		unavailErr := &ReviewerUnavailableError{
			SessionID: "sess-2",
			Status:    "closed",
		}
		body := unavailErr.RefusalBody()
		if body.Code != RefusalCodeReviewerUnavailable {
			t.Errorf("code = %q, want %q", body.Code, RefusalCodeReviewerUnavailable)
		}
		if body.Message == "" {
			t.Error("message must not be empty")
		}
		if body.OperatorAction == "" {
			t.Error("operator_action must not be empty")
		}
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal error = %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("json.Unmarshal error = %v", err)
		}
		for _, field := range []string{"code", "message", "retryable", "operator_action"} {
			if _, ok := got[field]; !ok {
				t.Errorf("refusal body missing field %q", field)
			}
		}
	})
}

func splitNonEmptyLines(s string) []string {
	lines := make([]string, 0)
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
