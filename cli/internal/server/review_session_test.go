package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReviewSession_CreatePersistsManifest(t *testing.T) {
	workDir := t.TempDir()
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

	manifestPath := filepath.Join(workDir, ".ddx", reviewSessionsDirName, session.ID, reviewManifestName)
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
	workDir := t.TempDir()
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

	turnsPath := filepath.Join(workDir, ".ddx", reviewSessionsDirName, sessionID, reviewTurnsName)
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
	workDir := t.TempDir()
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

	got, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, &want) {
		t.Fatalf("round-trip session = %+v, want %+v", got, want)
	}
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
