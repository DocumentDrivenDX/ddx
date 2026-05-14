package graphql

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Review session lifecycle status values exposed via the GraphQL ReviewSession
// type. Stored as the manifest status on disk and broadcast verbatim on
// reviewSessionEvents.
const (
	ReviewSessionStatusActive    = "active"
	ReviewSessionStatusCancelled = "cancelled"
	ReviewSessionStatusCompleted = "completed"
)

// Review event kinds emitted on the reviewSessionEvents subscription. Delta
// chunks (kind=delta) are optional; the final-message event (kind=final) is
// required so subscribers can reconstruct the reviewer turn even when no
// upstream streaming is available.
const (
	ReviewSessionEventKindDelta = "delta"
	ReviewSessionEventKindFinal = "final"
)

// ReviewSessionService is the production interface satisfied by the server
// package's review session store + dispatcher pair. The resolver depends only
// on this interface so tests can inject a fake implementation without spinning
// up the on-disk store.
//
// When the resolver is constructed with a nil ReviewSessions service the
// in-memory default (NewInMemoryReviewSessionService) is used. The default
// records turns and broadcasts a synthetic reviewer reply on Respond so the
// reviewSessionEvents subscription delivers the required final-message event
// without depending on the (separately-tracked) dispatcher bead.
type ReviewSessionService interface {
	Start(ctx context.Context, input ReviewSessionStartInput) (*ReviewSession, error)
	Respond(ctx context.Context, sessionID string, turn ReviewTurnInput) (*ReviewSession, error)
	Cancel(ctx context.Context, sessionID string) (bool, error)
	Subscribe(sessionID string) (<-chan *ReviewSessionEvent, func())
}

// inMemoryReviewSessionService is the default ReviewSessionService used when
// the server has not wired a production implementation. It keeps sessions in
// memory and produces a deterministic synthetic reviewer turn on each
// Respond call so the subscription path is end-to-end testable from the
// GraphQL layer alone.
type inMemoryReviewSessionService struct {
	mu       sync.Mutex
	sessions map[string]*ReviewSession
	subs     map[string][]chan *ReviewSessionEvent
}

// NewInMemoryReviewSessionService returns the default service backing the
// review session mutations and subscription when the resolver has no wired
// production implementation.
func NewInMemoryReviewSessionService() ReviewSessionService {
	return &inMemoryReviewSessionService{
		sessions: map[string]*ReviewSession{},
		subs:     map[string][]chan *ReviewSessionEvent{},
	}
}

func (s *inMemoryReviewSessionService) Start(_ context.Context, input ReviewSessionStartInput) (*ReviewSession, error) {
	if input.ArtifactID == "" {
		return nil, fmt.Errorf("reviewSessionStart: artifactId is required")
	}
	if input.ArtifactSha == "" {
		return nil, fmt.Errorf("reviewSessionStart: artifactSha is required")
	}

	id := ""
	if input.SessionID != nil {
		id = *input.SessionID
	}
	if id == "" {
		var err error
		id, err = newReviewSessionID()
		if err != nil {
			return nil, err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[id]; exists {
		return nil, fmt.Errorf("reviewSessionStart: session %q already exists", id)
	}

	session := &ReviewSession{
		ID:          id,
		ArtifactID:  input.ArtifactID,
		ArtifactSha: input.ArtifactSha,
		TemplateRef: input.TemplateRef,
		PromptRef:   input.PromptRef,
		Status:      ReviewSessionStatusActive,
		Turns:       []*ReviewTurn{},
	}
	if input.MaxBillableUsd != nil {
		session.MaxBillableUsd = *input.MaxBillableUsd
	}
	s.sessions[id] = session
	return cloneReviewSession(session), nil
}

func (s *inMemoryReviewSessionService) Respond(_ context.Context, sessionID string, turn ReviewTurnInput) (*ReviewSession, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("reviewSessionRespond: sessionId is required")
	}
	if turn.Content == "" {
		return nil, fmt.Errorf("reviewSessionRespond: turn content is required")
	}

	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("reviewSessionRespond: session %q not found", sessionID)
	}
	if session.Status != ReviewSessionStatusActive {
		s.mu.Unlock()
		return nil, fmt.Errorf("reviewSessionRespond: session %q has status %q", sessionID, session.Status)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	userTurn := &ReviewTurn{
		Actor:     "user",
		Content:   turn.Content,
		CostUsd:   0,
		CreatedAt: now,
	}
	reviewerTurn := &ReviewTurn{
		Actor:     "reviewer",
		Content:   "review pending dispatcher integration",
		CostUsd:   0,
		CreatedAt: now,
	}
	session.Turns = append(session.Turns, userTurn, reviewerTurn)
	snapshot := cloneReviewSession(session)
	s.mu.Unlock()

	s.publish(sessionID, &ReviewSessionEvent{
		SessionID: sessionID,
		Kind:      ReviewSessionEventKindFinal,
		Content:   reviewerTurn.Content,
		CostUsd:   reviewerTurn.CostUsd,
		Timestamp: now,
	})

	return snapshot, nil
}

func (s *inMemoryReviewSessionService) Cancel(_ context.Context, sessionID string) (bool, error) {
	if sessionID == "" {
		return false, fmt.Errorf("reviewSessionCancel: sessionId is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return false, fmt.Errorf("reviewSessionCancel: session %q not found", sessionID)
	}
	session.Status = ReviewSessionStatusCancelled
	return true, nil
}

func (s *inMemoryReviewSessionService) Subscribe(sessionID string) (<-chan *ReviewSessionEvent, func()) {
	ch := make(chan *ReviewSessionEvent, 16)
	s.mu.Lock()
	s.subs[sessionID] = append(s.subs[sessionID], ch)
	s.mu.Unlock()

	unsub := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		subs := s.subs[sessionID]
		for i, c := range subs {
			if c == ch {
				s.subs[sessionID] = append(subs[:i], subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, unsub
}

func (s *inMemoryReviewSessionService) publish(sessionID string, event *ReviewSessionEvent) {
	s.mu.Lock()
	subs := append([]chan *ReviewSessionEvent(nil), s.subs[sessionID]...)
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func cloneReviewSession(in *ReviewSession) *ReviewSession {
	if in == nil {
		return nil
	}
	out := *in
	if in.Turns != nil {
		out.Turns = make([]*ReviewTurn, len(in.Turns))
		for i, t := range in.Turns {
			tc := *t
			out.Turns[i] = &tc
		}
	}
	return &out
}

func newReviewSessionID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("review session id: %w", err)
	}
	return "rev-" + hex.EncodeToString(buf), nil
}

// reviewSessionService returns the resolver's wired ReviewSessions service,
// lazily allocating the in-memory default when none was wired so the resolver
// is usable without external configuration.
func (r *Resolver) reviewSessionService() ReviewSessionService {
	r.reviewMu.Lock()
	defer r.reviewMu.Unlock()
	if r.ReviewSessions == nil {
		r.ReviewSessions = NewInMemoryReviewSessionService()
	}
	return r.ReviewSessions
}

// ReviewSessionStart is the resolver for the reviewSessionStart mutation.
func (r *mutationResolver) ReviewSessionStart(ctx context.Context, input ReviewSessionStartInput) (*ReviewSession, error) {
	return r.reviewSessionService().Start(ctx, input)
}

// ReviewSessionRespond is the resolver for the reviewSessionRespond mutation.
func (r *mutationResolver) ReviewSessionRespond(ctx context.Context, sessionID string, turn ReviewTurnInput) (*ReviewSession, error) {
	return r.reviewSessionService().Respond(ctx, sessionID, turn)
}

// ReviewSessionCancel is the resolver for the reviewSessionCancel mutation.
func (r *mutationResolver) ReviewSessionCancel(ctx context.Context, sessionID string) (bool, error) {
	return r.reviewSessionService().Cancel(ctx, sessionID)
}

// ReviewSessionEvents is the resolver for the reviewSessionEvents subscription.
//
// The subscription guarantees at least one final-message event per reviewer
// turn. Streaming deltas (kind=delta) are optional — subscribers that want to
// render incremental output may consume them but are not required to.
func (r *subscriptionResolver) ReviewSessionEvents(ctx context.Context, sessionID string) (<-chan *ReviewSessionEvent, error) {
	src, unsub := r.reviewSessionService().Subscribe(sessionID)
	out := make(chan *ReviewSessionEvent, 16)
	go func() {
		defer unsub()
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-src:
				if !ok {
					return
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}
