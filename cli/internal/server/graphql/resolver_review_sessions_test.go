package graphql

import (
	"context"
	"testing"
	"time"
)

func newReviewTestResolver() *Resolver {
	return &Resolver{
		ReviewSessions: NewInMemoryReviewSessionService(),
	}
}

func TestGraphQL_ReviewSessionStart(t *testing.T) {
	r := newReviewTestResolver()
	mr := &mutationResolver{r}

	sid := "rev-start-1"
	tpl := "templates/code-review.md"
	pr := "prompts/code-review.md"
	maxUSD := 1.50
	input := ReviewSessionStartInput{
		SessionID:      &sid,
		ArtifactID:     "doc/architecture.md",
		ArtifactSha:    "deadbeef",
		TemplateRef:    &tpl,
		PromptRef:      &pr,
		MaxBillableUsd: &maxUSD,
	}

	session, err := mr.ReviewSessionStart(context.Background(), input)
	if err != nil {
		t.Fatalf("ReviewSessionStart: %v", err)
	}
	if session == nil {
		t.Fatal("expected non-nil session")
	}
	if session.ID != sid {
		t.Errorf("ID = %q, want %q", session.ID, sid)
	}
	if session.ArtifactID != "doc/architecture.md" {
		t.Errorf("ArtifactID = %q", session.ArtifactID)
	}
	if session.ArtifactSha != "deadbeef" {
		t.Errorf("ArtifactSha = %q", session.ArtifactSha)
	}
	if session.Status != ReviewSessionStatusActive {
		t.Errorf("Status = %q, want %q", session.Status, ReviewSessionStatusActive)
	}
	if session.MaxBillableUsd != 1.50 {
		t.Errorf("MaxBillableUsd = %v, want 1.50", session.MaxBillableUsd)
	}
	if session.TemplateRef == nil || *session.TemplateRef != tpl {
		t.Errorf("TemplateRef mismatch: %v", session.TemplateRef)
	}
	if session.PromptRef == nil || *session.PromptRef != pr {
		t.Errorf("PromptRef mismatch: %v", session.PromptRef)
	}
	if len(session.Turns) != 0 {
		t.Errorf("Turns = %d, want 0", len(session.Turns))
	}
}

func TestGraphQL_ReviewSessionStart_RequiresArtifact(t *testing.T) {
	r := newReviewTestResolver()
	mr := &mutationResolver{r}
	if _, err := mr.ReviewSessionStart(context.Background(), ReviewSessionStartInput{ArtifactSha: "abc"}); err == nil {
		t.Fatal("expected error when ArtifactID is empty")
	}
	if _, err := mr.ReviewSessionStart(context.Background(), ReviewSessionStartInput{ArtifactID: "a"}); err == nil {
		t.Fatal("expected error when ArtifactSha is empty")
	}
}

func TestGraphQL_ReviewSessionRespond(t *testing.T) {
	r := newReviewTestResolver()
	mr := &mutationResolver{r}

	sid := "rev-respond-1"
	_, err := mr.ReviewSessionStart(context.Background(), ReviewSessionStartInput{
		SessionID:   &sid,
		ArtifactID:  "doc/a.md",
		ArtifactSha: "sha",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	updated, err := mr.ReviewSessionRespond(context.Background(), sid, ReviewTurnInput{Content: "please review"})
	if err != nil {
		t.Fatalf("ReviewSessionRespond: %v", err)
	}
	if updated == nil {
		t.Fatal("expected non-nil updated session")
	}
	if len(updated.Turns) < 1 {
		t.Fatalf("expected at least one turn appended, got %d", len(updated.Turns))
	}

	var userFound bool
	for _, turn := range updated.Turns {
		if turn.Actor == "user" && turn.Content == "please review" {
			userFound = true
			break
		}
	}
	if !userFound {
		t.Fatalf("expected user turn with content 'please review'; got %+v", updated.Turns)
	}
}

func TestGraphQL_ReviewSessionCancel(t *testing.T) {
	r := newReviewTestResolver()
	mr := &mutationResolver{r}

	sid := "rev-cancel-1"
	_, err := mr.ReviewSessionStart(context.Background(), ReviewSessionStartInput{
		SessionID:   &sid,
		ArtifactID:  "doc/a.md",
		ArtifactSha: "sha",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	ok, err := mr.ReviewSessionCancel(context.Background(), sid)
	if err != nil {
		t.Fatalf("ReviewSessionCancel: %v", err)
	}
	if !ok {
		t.Fatal("ReviewSessionCancel returned false")
	}

	svc := r.reviewSessionService().(*inMemoryReviewSessionService)
	svc.mu.Lock()
	got := svc.sessions[sid].Status
	svc.mu.Unlock()
	if got != ReviewSessionStatusCancelled {
		t.Fatalf("session status = %q, want %q", got, ReviewSessionStatusCancelled)
	}
}

func TestGraphQL_ReviewSessionEvents_FinalMessage(t *testing.T) {
	r := newReviewTestResolver()
	mr := &mutationResolver{r}
	sr := &subscriptionResolver{r}

	sid := "rev-events-1"
	_, err := mr.ReviewSessionStart(context.Background(), ReviewSessionStartInput{
		SessionID:   &sid,
		ArtifactID:  "doc/a.md",
		ArtifactSha: "sha",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events, err := sr.ReviewSessionEvents(ctx, sid)
	if err != nil {
		t.Fatalf("ReviewSessionEvents: %v", err)
	}

	// Trigger a reviewer turn after the subscription is wired so the final
	// event is delivered to the subscriber.
	go func() {
		time.Sleep(20 * time.Millisecond)
		_, _ = mr.ReviewSessionRespond(context.Background(), sid, ReviewTurnInput{Content: "please review"})
	}()

	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("event channel closed before final event")
		}
		if ev.SessionID != sid {
			t.Errorf("event SessionID = %q, want %q", ev.SessionID, sid)
		}
		if ev.Kind != ReviewSessionEventKindFinal {
			t.Errorf("event Kind = %q, want %q", ev.Kind, ReviewSessionEventKindFinal)
		}
		if ev.Content == "" {
			t.Error("event Content is empty; expected reviewer reply body")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for final review event")
	}
}
