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

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// Stable refusal codes returned when a review turn cannot proceed. These are
// part of the external contract — operators triage on these literal strings.
const (
	RefusalCodePromptBudgetExceeded = "PROMPT_BUDGET_EXCEEDED"
	RefusalCodeCostCapExceeded      = "COST_CAP_EXCEEDED"
	RefusalCodeReviewerUnavailable  = "REVIEWER_UNAVAILABLE"
)

// ReviewRefusalBody is the structured JSON body attached to all review turn
// refusals. Code is one of the RefusalCode* constants above.
type ReviewRefusalBody struct {
	Code           string `json:"code"`
	Message        string `json:"message"`
	Retryable      bool   `json:"retryable"`
	OperatorAction string `json:"operator_action"`
}

// MarshalRefusalJSON returns the JSON encoding of a ReviewRefusalBody.
func MarshalRefusalJSON(code, message, operatorAction string, retryable bool) (string, error) {
	b := ReviewRefusalBody{
		Code:           code,
		Message:        message,
		Retryable:      retryable,
		OperatorAction: operatorAction,
	}
	data, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ReviewCostCapExceededError is returned by AppendTurn when adding the turn's
// cost to the session's accumulated cost would exceed MaxBillableUSD.
type ReviewCostCapExceededError struct {
	SessionID   string
	CurrentCost float64
	TurnCost    float64
	MaxUSD      float64
}

func (e *ReviewCostCapExceededError) Error() string {
	return fmt.Sprintf(
		"COST_CAP_EXCEEDED: session %s current cost $%.4f + turn cost $%.4f exceeds max_billable_usd $%.4f",
		e.SessionID, e.CurrentCost, e.TurnCost, e.MaxUSD,
	)
}

// RefusalBody returns the structured JSON refusal body for this error.
func (e *ReviewCostCapExceededError) RefusalBody() ReviewRefusalBody {
	return ReviewRefusalBody{
		Code:           RefusalCodeCostCapExceeded,
		Message:        e.Error(),
		Retryable:      false,
		OperatorAction: "raise max_billable_usd for the review session or create a new session",
	}
}

// ReviewerUnavailableError is returned when a review turn is attempted on a
// session that is in a terminal state and cannot accept new reviewer turns.
type ReviewerUnavailableError struct {
	SessionID string
	Status    string
}

func (e *ReviewerUnavailableError) Error() string {
	return fmt.Sprintf(
		"REVIEWER_UNAVAILABLE: session %s has status %q and cannot accept new review turns",
		e.SessionID, e.Status,
	)
}

// RefusalBody returns the structured JSON refusal body for this error.
func (e *ReviewerUnavailableError) RefusalBody() ReviewRefusalBody {
	return ReviewRefusalBody{
		Code:           RefusalCodeReviewerUnavailable,
		Message:        e.Error(),
		Retryable:      false,
		OperatorAction: "open a new review session to continue",
	}
}

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

// AppendTurn appends a single transcript entry to turns.jsonl. It enforces
// MaxBillableUSD: if the session has a non-zero budget cap and the accumulated
// cost plus this turn's cost would exceed it, AppendTurn returns a
// *ReviewCostCapExceededError without writing the turn. On success the
// manifest's CostUSD accumulator is updated to reflect the new total.
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

	// Load the manifest to check the cost cap and read the current accumulated
	// cost. If the manifest does not exist yet (session created without Create),
	// proceed without enforcement.
	manifestPath := filepath.Join(root, reviewManifestName)
	var manifest *reviewSessionManifest
	if data, readErr := os.ReadFile(manifestPath); readErr == nil {
		var m reviewSessionManifest
		if jsonErr := json.Unmarshal(data, &m); jsonErr == nil {
			manifest = &m
		}
	}

	if manifest != nil && manifest.MaxBillableUSD > 0 {
		if manifest.CostUSD+turn.CostUSD > manifest.MaxBillableUSD {
			return &ReviewCostCapExceededError{
				SessionID:   sessionID,
				CurrentCost: manifest.CostUSD,
				TurnCost:    turn.CostUSD,
				MaxUSD:      manifest.MaxBillableUSD,
			}
		}
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

	// Update the manifest's CostUSD accumulator so the next AppendTurn sees
	// the correct running total.
	if manifest != nil {
		manifest.CostUSD += turn.CostUSD
		_ = writeJSONFile(manifestPath, *manifest)
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
	return ddxroot.JoinProject(s.projectRoot, reviewSessionsDirName, sessionID), nil
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
