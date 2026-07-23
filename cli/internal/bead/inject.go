package bead

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// InjectPayload is the base interface for all injectable payload types.
type InjectPayload interface {
	Kind() string
	Validate() error
}

// ReviewFindingPayload holds the data for a review-finding bead.
type ReviewFindingPayload struct {
	Verdict    string `json:"verdict"`            // APPROVE, REQUEST_CHANGES, BLOCK
	Findings   string `json:"findings,omitempty"` // review findings
	ResultRev  string `json:"result_rev"`         // commit SHA being reviewed
	ReviewedBy string `json:"reviewed_by"`        // reviewer identity
}

func (p *ReviewFindingPayload) Kind() string {
	return IssueTypeReviewFinding
}

func (p *ReviewFindingPayload) Validate() error {
	if p == nil {
		return fmt.Errorf("review-finding payload: payload cannot be nil")
	}
	if p.Verdict == "" {
		return fmt.Errorf("review-finding payload: verdict is required")
	}
	if p.ResultRev == "" {
		return fmt.Errorf("review-finding payload: result_rev is required")
	}
	return nil
}

// AlignmentReviewPayload holds the data for an alignment-review bead.
type AlignmentReviewPayload struct {
	Document  string `json:"document"`   // document path being aligned
	Alignment string `json:"alignment"`  // alignment findings
	UpdatedBy string `json:"updated_by"` // who performed alignment
}

func (p *AlignmentReviewPayload) Kind() string {
	return IssueTypeAlignmentReview
}

func (p *AlignmentReviewPayload) Validate() error {
	if p == nil {
		return fmt.Errorf("alignment-review payload: payload cannot be nil")
	}
	if p.Document == "" {
		return fmt.Errorf("alignment-review payload: document is required")
	}
	return nil
}

// payloadHash computes a content hash of the payload for idempotency.
func payloadHash(payload InjectPayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("payload hash: marshal: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// InjectOptions configures Inject behavior.
type InjectOptions struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	Labels      []string       `json:"labels,omitempty"`
	Priority    int            `json:"priority,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
}

// Inject creates or returns an idempotent bead of the specified kind.
// The bead is created with status=open if it doesn't already exist.
// Idempotency is based on (parent, kind, payload-hash).
// Parent must be a valid bead ID (will be validated against the store).
func (s *Store) Inject(ctx context.Context, parent string, payload InjectPayload, opts InjectOptions) (string, error) {
	if parent == "" {
		return "", fmt.Errorf("inject: parent bead ID required")
	}
	if payload == nil {
		return "", fmt.Errorf("inject: payload required")
	}
	kind := payload.Kind()
	if kind != IssueTypeReviewFinding && kind != IssueTypeAlignmentReview {
		return "", fmt.Errorf("inject: unknown kind %q", kind)
	}
	if err := payload.Validate(); err != nil {
		return "", fmt.Errorf("inject: %w", err)
	}

	var foundID string
	err := s.WithLock(func() error {
		// Verify parent exists.
		parentBead, err := s.Get(ctx, parent)
		if err != nil {
			return fmt.Errorf("inject: parent bead lookup failed: %w", err)
		}
		if parentBead == nil {
			return fmt.Errorf("inject: parent bead not found: %s", parent)
		}

		// Compute payload hash for idempotency.
		hash, err := payloadHash(payload)
		if err != nil {
			return fmt.Errorf("inject: %w", err)
		}

		// Check if an identical bead already exists.
		allBeads, _, err := s.readAllLatestRaw()
		if err != nil {
			return fmt.Errorf("inject: read all beads failed: %w", err)
		}

		for _, b := range allBeads {
			if b.IssueType != kind || b.Parent != parent {
				continue
			}
			// Check if the payload hash matches.
			if existing, ok := b.Extra["payload_hash"]; ok && existing == hash {
				foundID = b.ID
				return nil
			}
		}

		// Create a new bead.
		now := time.Now().UTC()
		newID, err := s.GenID(context.Background())
		if err != nil {
			return fmt.Errorf("inject: generate id: %w", err)
		}

		title := opts.Title
		if title == "" {
			title = fmt.Sprintf("%s: %s", kind, parent)
		}

		priority := opts.Priority
		if priority < 0 {
			priority = 0
		}
		if priority > MaxPriority {
			priority = MaxPriority
		}

		newBead := &Bead{
			ID:            newID,
			Title:         title,
			IssueType:     kind,
			Status:        StatusOpen,
			Priority:      priority,
			SchemaVersion: CurrentSchemaVersion,
			CreatedAt:     now,
			UpdatedAt:     now,
			Parent:        parent,
			Description:   opts.Description,
			Labels:        opts.Labels,
			Extra: map[string]any{
				"payload_hash": hash,
				"payload":      payload,
			},
		}

		// Merge in any additional extra fields.
		if opts.Extra != nil {
			for k, v := range opts.Extra {
				newBead.Extra[k] = v
			}
		}

		allBeads = append(allBeads, *newBead)
		if err := s.writeAllLocked(allBeads); err != nil {
			return fmt.Errorf("inject: write all: %w", err)
		}

		foundID = newID
		return nil
	})
	if err != nil {
		return "", err
	}
	return foundID, nil
}

// InjectReviewFinding is a convenience wrapper for injecting review-finding beads.
func (s *Store) InjectReviewFinding(ctx context.Context, parent string, verdict, resultRev, reviewedBy string, opts InjectOptions) (string, error) {
	payload := &ReviewFindingPayload{
		Verdict:    verdict,
		ResultRev:  resultRev,
		ReviewedBy: reviewedBy,
	}
	return s.Inject(ctx, parent, payload, opts)
}

// InjectAlignmentReview is a convenience wrapper for injecting alignment-review beads.
func (s *Store) InjectAlignmentReview(ctx context.Context, parent, document, updatedBy string, opts InjectOptions) (string, error) {
	payload := &AlignmentReviewPayload{
		Document:  document,
		UpdatedBy: updatedBy,
	}
	return s.Inject(ctx, parent, payload, opts)
}
