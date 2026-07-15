package server

import (
	"context"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	ddxconfig "github.com/DocumentDrivenDX/ddx/internal/config"
)

// ReviewDispatcher dispatches review-session turns to the reviewer harness
// and records each resulting turn in the session's turns.jsonl.
type ReviewDispatcher struct {
	ProjectRoot string
	Store       *ReviewSessionStore
	// Runner, when non-nil, replaces the production dispatch path. Inject in
	// tests to return canned results without spinning up a real agent service.
	Runner agent.AgentRunner
}

// DispatchReviewTurn assembles a prompt from the session state and the
// incoming user turn, invokes the reviewer harness in no-tool mode, records
// the resulting reviewer turn in turns.jsonl, and returns that turn.
//
// The readonly permissions constraint is forwarded unchanged to Fizeau, which
// owns harness selection and capability enforcement. DDx does not inspect or
// reject a concrete harness based on a local capability catalog.
func (d *ReviewDispatcher) DispatchReviewTurn(ctx context.Context, session ReviewSession, userTurn ReviewTurn) (ReviewTurn, error) {
	// Include the incoming user turn before rendering so the reviewer sees
	// the full turn history including the latest message.
	sessionWithTurn := session
	sessionWithTurn.Turns = append(append([]ReviewTurn(nil), session.Turns...), userTurn)

	rendered, err := RenderReviewPrompt(ReviewPromptRenderInput{
		Session: sessionWithTurn,
	})
	if err != nil {
		return ReviewTurn{}, fmt.Errorf("review dispatcher: assemble prompt: %w", err)
	}

	var result *agent.Result
	if d.Runner != nil {
		// Test injection path: pass permissions directly in RunArgs so tests
		// can verify that PermissionsReadOnlyReviewer was set.
		result, err = d.Runner.Run(agent.RunArgs{
			Context:     ctx,
			Prompt:      rendered.Prompt,
			WorkDir:     d.ProjectRoot,
			Permissions: agent.PermissionsReadOnlyReviewer,
			Role:        "reviewer",
		})
	} else {
		rcfg, _ := ddxconfig.LoadAndResolve(d.ProjectRoot, ddxconfig.CLIOverrides{})
		runtime := agent.AgentRunRuntime{
			Prompt:              rendered.Prompt,
			WorkDir:             d.ProjectRoot,
			PermissionsOverride: agent.PermissionsReadOnlyReviewer,
			ClearRoutingPins:    true,
			ClearProfile:        true,
			Role:                "reviewer",
		}
		result, err = agent.RunWithConfigViaService(ctx, d.ProjectRoot, rcfg, runtime)
	}
	if err != nil {
		return ReviewTurn{}, fmt.Errorf("review dispatcher: dispatch: %w", err)
	}

	output := ""
	costUSD := 0.0
	if result != nil {
		output = result.Output
		costUSD = result.CostUSD
	}

	reviewerTurn := ReviewTurn{
		Actor:     "reviewer",
		Content:   output,
		CostUSD:   costUSD,
		CreatedAt: time.Now().UTC(),
	}
	if d.Store != nil {
		if appendErr := d.Store.AppendTurn(session.ID, reviewerTurn); appendErr != nil {
			return ReviewTurn{}, fmt.Errorf("review dispatcher: record turn: %w", appendErr)
		}
	}

	return reviewerTurn, nil
}
