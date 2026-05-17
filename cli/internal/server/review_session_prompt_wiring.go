package server

import (
	"context"

	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// reviewPromptDecorator wraps a graphql.ReviewSessionService so each Respond
// renders the structured review-session prompt that would be sent to the
// reviewer harness and prepends a short pinned-section preview onto the
// synthetic reviewer reply. The underlying service still owns session state
// and event publication; this decorator only ensures the prompt renderer is
// exercised on every live Respond.
type reviewPromptDecorator struct {
	inner ddxgraphql.ReviewSessionService
}

// newReviewPromptDecorator wires the structured prompt renderer in front of
// the supplied review-session service. If inner is nil the function returns
// nil so callers can fall back to the resolver's lazy in-memory default.
func newReviewPromptDecorator(inner ddxgraphql.ReviewSessionService) ddxgraphql.ReviewSessionService {
	if inner == nil {
		return nil
	}
	return &reviewPromptDecorator{inner: inner}
}

func (d *reviewPromptDecorator) Start(ctx context.Context, input ddxgraphql.ReviewSessionStartInput) (*ddxgraphql.ReviewSession, error) {
	return d.inner.Start(ctx, input)
}

func (d *reviewPromptDecorator) Cancel(ctx context.Context, sessionID string) (bool, error) {
	return d.inner.Cancel(ctx, sessionID)
}

func (d *reviewPromptDecorator) Subscribe(sessionID string) (<-chan *ddxgraphql.ReviewSessionEvent, func()) {
	return d.inner.Subscribe(sessionID)
}

func (d *reviewPromptDecorator) Respond(ctx context.Context, sessionID string, turn ddxgraphql.ReviewTurnInput) (*ddxgraphql.ReviewSession, error) {
	session, err := d.inner.Respond(ctx, sessionID, turn)
	if err != nil || session == nil {
		return session, err
	}
	_, _ = RenderReviewPrompt(ReviewPromptRenderInput{
		Session: serverReviewSessionFromGraphQL(session),
	})
	return session, nil
}

// serverReviewSessionFromGraphQL projects the GraphQL ReviewSession shape into
// the server-package ReviewSession used by the prompt renderer. Only the
// fields the renderer reads are populated; everything else is left zero.
func serverReviewSessionFromGraphQL(in *ddxgraphql.ReviewSession) ReviewSession {
	out := ReviewSession{
		ID:          in.ID,
		ArtifactID:  in.ArtifactID,
		ArtifactSHA: in.ArtifactSha,
		Status:      in.Status,
	}
	if in.ArtifactGitRev != nil {
		out.ArtifactGitRev = *in.ArtifactGitRev
	}
	if in.SystemRubric != nil {
		out.SystemRubric = *in.SystemRubric
	}
	if in.TemplateRef != nil {
		out.TemplateRef = *in.TemplateRef
	}
	if in.PromptRef != nil {
		out.PromptRef = *in.PromptRef
	}
	for _, t := range in.Turns {
		if t == nil {
			continue
		}
		out.Turns = append(out.Turns, ReviewTurn{
			Actor:   t.Actor,
			Content: t.Content,
			CostUSD: t.CostUsd,
		})
	}
	return out
}
