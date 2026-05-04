package cmd

import (
	"context"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// reviewerFunc is a test-local functional adapter implementing agent.BeadReviewer.
type reviewerFunc func(ctx context.Context, beadID, resultRev string, impl agent.ImplementerRouting) (*agent.ReviewResult, error)

func (f reviewerFunc) ReviewBead(ctx context.Context, beadID, resultRev string, impl agent.ImplementerRouting) (*agent.ReviewResult, error) {
	return f(ctx, beadID, resultRev, impl)
}
