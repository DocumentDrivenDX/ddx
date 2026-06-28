package cmd

import (
	"context"
	"fmt"
	"strings"

	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

type attemptBeadReader interface {
	Get(ctx context.Context, id string) (*bead.Bead, error)
}

// resolveAttemptBead prefers the current tracker state, then refreshes the
// store once, and only falls back to the in-memory claimed bead snapshot if the
// reload path still cannot find the bead.
func resolveAttemptBead(ctx context.Context, beadID string, primary attemptBeadReader, refresh func() attemptBeadReader, fallback *bead.Bead) (*bead.Bead, error) {
	if primary != nil {
		if loaded, err := primary.Get(ctx, beadID); err == nil {
			return loaded, nil
		} else if !isBeadNotFoundError(err, beadID) {
			return nil, err
		}
	}

	if refresh != nil {
		if refreshed := refresh(); refreshed != nil {
			if loaded, err := refreshed.Get(ctx, beadID); err == nil {
				return loaded, nil
			} else if !isBeadNotFoundError(err, beadID) {
				return nil, err
			}
		}
	}

	if claimed, ok := agenttry.BeadFromContext(ctx); ok && claimed != nil && claimed.ID == beadID {
		return claimed, nil
	}
	if fallback != nil && fallback.ID == beadID {
		return fallback, nil
	}
	return nil, fmt.Errorf("bead: not found: %s", beadID)
}

func isBeadNotFoundError(err error, beadID string) bool {
	if err == nil {
		return false
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return false
	}
	return msg == fmt.Sprintf("bead: not found: %s", beadID) || strings.HasPrefix(msg, "bead: not found:")
}
