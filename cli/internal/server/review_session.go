package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	reviewSessionsDirName = "reviews"
	reviewManifestName    = "manifest.json"
	reviewTurnsName       = "turns.jsonl"
	reviewAttachmentsDir  = "attachments"
)

// ReviewSession is the server-side review state machine persisted under
// .ddx/reviews/<session-id>/ for Story 16 and review mutations.
//
// Story 16 on-disk contract:
//   - manifest.json stores the session metadata only: id, artifact refs,
//     rubric/template/prompt refs, status, and billing limits.
//   - turns.jsonl is append-only JSONL. Each non-empty line is one ReviewTurn
//     record with actor, content, cost_usd, and created_at.
//   - attachments/ stores any binary sidecar files for the session.
//
// The manifest is written when a session is created. Turns are appended
// without rewriting prior lines so the review transcript remains durable and
// streamable.
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

// ReviewTurn is one append-only transcript entry in turns.jsonl.
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

// ReviewSessionStore persists review sessions under .ddx/reviews/.
type ReviewSessionStore struct {
	projectRoot string
	mu          sync.Mutex
}

// NewReviewSessionStore constructs a store rooted at projectRoot.
func NewReviewSessionStore(projectRoot string) *ReviewSessionStore {
	return &ReviewSessionStore{projectRoot: projectRoot}
}

// Create initializes the session directory, writes manifest.json, seeds
// turns.jsonl, and creates attachments/.
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

// AppendTurn appends a single transcript entry to turns.jsonl.
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

// Load reads the manifest plus append-only transcript back into memory.
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
