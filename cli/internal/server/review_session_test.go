package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	reviewSessionsDirName = "reviews"
	reviewManifestName    = "manifest.json"
	reviewTurnsName       = "turns.jsonl"
	reviewAttachmentsDir  = "attachments"
)

type ReviewSession struct {
	ID             string       `json:"id"`
	ArtifactID     string       `json:"artifact_id"`
	ArtifactSHA    string       `json:"artifact_sha"`
	ArtifactGitRev string       `json:"artifact_git_rev"`
	SystemRubric   string       `json:"system_rubric"`
	TemplateRef    string       `json:"template_ref"`
	PromptRef      string       `json:"prompt_ref"`
	Status         string       `json:"status"`
	Turns          []ReviewTurn `json:"turns,omitempty"`
	CostUSD        float64      `json:"cost_usd"`
	MaxBillableUSD float64      `json:"max_billable_usd"`
}

type ReviewTurn struct {
	Actor     string    `json:"actor"`
	Content   string    `json:"content"`
	CostUSD   float64   `json:"cost_usd"`
	CreatedAt time.Time `json:"created_at"`
}

type reviewSessionManifest struct {
	ID             string  `json:"id"`
	ArtifactID     string  `json:"artifact_id"`
	ArtifactSHA    string  `json:"artifact_sha"`
	ArtifactGitRev string  `json:"artifact_git_rev"`
	SystemRubric   string  `json:"system_rubric"`
	TemplateRef    string  `json:"template_ref"`
	PromptRef      string  `json:"prompt_ref"`
	Status         string  `json:"status"`
	CostUSD        float64 `json:"cost_usd"`
	MaxBillableUSD float64 `json:"max_billable_usd"`
}

type ReviewSessionStore struct {
	projectRoot string
	mu          sync.Mutex
}

func NewReviewSessionStore(projectRoot string) *ReviewSessionStore {
	return &ReviewSessionStore{projectRoot: projectRoot}
}

func (s *ReviewSessionStore) Create(session ReviewSession) error {
	if s == nil {
		return errors.New("review session store: nil store")
	}
	root, err := s.sessionRoot(session.ID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Join(root, reviewAttachmentsDir), 0o755); err != nil {
		return fmt.Errorf("review session: mkdir attachments: %w", err)
	}
	if err := writeJSONFile(filepath.Join(root, reviewManifestName), reviewSessionManifestFrom(session)); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, reviewTurnsName), nil, 0o644); err != nil {
		return fmt.Errorf("review session: seed turns: %w", err)
	}
	return nil
}

func (s *ReviewSessionStore) AppendTurn(sessionID string, turn ReviewTurn) error {
	if s == nil {
		return errors.New("review session store: nil store")
	}
	root, err := s.sessionRoot(sessionID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("review session: mkdir session: %w", err)
	}
	if turn.CreatedAt.IsZero() {
		turn.CreatedAt = time.Now().UTC()
	}
	path := filepath.Join(root, reviewTurnsName)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("review session: open turns: %w", err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(turn); err != nil {
		return fmt.Errorf("review session: encode turn: %w", err)
	}
	return nil
}

func (s *ReviewSessionStore) Load(sessionID string) (*ReviewSession, error) {
	if s == nil {
		return nil, errors.New("review session store: nil store")
	}
	root, err := s.sessionRoot(sessionID)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(root, reviewManifestName))
	if err != nil {
		return nil, fmt.Errorf("review session: read manifest: %w", err)
	}
	var manifest reviewSessionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("review session: parse manifest: %w", err)
	}
	session := &ReviewSession{
		ID:             manifest.ID,
		ArtifactID:     manifest.ArtifactID,
		ArtifactSHA:    manifest.ArtifactSHA,
		ArtifactGitRev: manifest.ArtifactGitRev,
		SystemRubric:   manifest.SystemRubric,
		TemplateRef:    manifest.TemplateRef,
		PromptRef:      manifest.PromptRef,
		Status:         manifest.Status,
		CostUSD:        manifest.CostUSD,
		MaxBillableUSD: manifest.MaxBillableUSD,
	}

	turnsPath := filepath.Join(root, reviewTurnsName)
	f, err := os.Open(turnsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return session, nil
		}
		return nil, fmt.Errorf("review session: read turns: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var turn ReviewTurn
		if err := json.Unmarshal([]byte(line), &turn); err != nil {
			return nil, fmt.Errorf("review session: parse turn: %w", err)
		}
		session.Turns = append(session.Turns, turn)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("review session: scan turns: %w", err)
	}
	return session, nil
}

func (s *ReviewSessionStore) sessionRoot(sessionID string) (string, error) {
	if s == nil {
		return "", errors.New("review session store: nil store")
	}
	if s.projectRoot == "" {
		return "", errors.New("review session store: empty project root")
	}
	if sessionID == "" {
		return "", errors.New("review session store: empty session id")
	}
	if sessionID == "." || sessionID == ".." || strings.ContainsAny(sessionID, "/\\") {
		return "", fmt.Errorf("review session store: invalid session id %q", sessionID)
	}
	return filepath.Join(s.projectRoot, ".ddx", reviewSessionsDirName, sessionID), nil
}

func reviewSessionManifestFrom(session ReviewSession) reviewSessionManifest {
	return reviewSessionManifest{
		ID:             session.ID,
		ArtifactID:     session.ArtifactID,
		ArtifactSHA:    session.ArtifactSHA,
		ArtifactGitRev: session.ArtifactGitRev,
		SystemRubric:   session.SystemRubric,
		TemplateRef:    session.TemplateRef,
		PromptRef:      session.PromptRef,
		Status:         session.Status,
		CostUSD:        session.CostUSD,
		MaxBillableUSD: session.MaxBillableUSD,
	}
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("review session: marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("review session: write %s: %w", path, err)
	}
	return nil
}

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
