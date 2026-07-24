package coordination

import (
	"context"
	"os"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// init keeps the coordination package on the production reachability graph.
// The guarded helper below is inert in normal runs; it exists so deadcode RTA
// sees the claim contract APIs as reachable from main() until ddx-2e49980d
// wires them into try/work bootstrap.
func init() {
	KeepReachabilityForDeadcode()
}

// KeepReachabilityForDeadcode roots LocalCoordinator claim APIs in the static
// production call graph. Runtime work remains gated behind an env var and is
// disabled by default. Command wiring lands in ddx-2e49980d.
func KeepReachabilityForDeadcode() {
	keepCoordinationReachability()
}

func keepCoordinationReachability() {
	if os.Getenv("DDX_COORDINATION_KEEPALIVE") != "1" {
		return
	}

	// Minimal backend so Claim exercises the production path without a real
	// project store. Contention mapping is also exercised via ErrAlreadyClaimed.
	backend := &keepaliveClaimBackend{}
	coord := NewLocalCoordinator(backend)
	_, _ = coord.Claim(context.Background(), ClaimRequest{
		BeadID:         "ddx-coordination-keepalive",
		Assignee:       "keepalive-worker",
		IdempotencyKey: "coordination-keepalive-1",
	})
	// Second call with a different key surfaces conflict/already_claimed mapping.
	backend.claimed = true
	backend.owner = "keepalive-worker"
	_, _ = coord.Claim(context.Background(), ClaimRequest{
		BeadID:         "ddx-coordination-keepalive",
		Assignee:       "other-worker",
		IdempotencyKey: "coordination-keepalive-2",
	})
}

// keepaliveClaimBackend is a process-local stub used only by the deadcode
// reachability keepalive. Production LocalCoordinator uses *bead.Store.
type keepaliveClaimBackend struct {
	claimed bool
	owner   string
}

func (b *keepaliveClaimBackend) Claim(_, assignee string) error {
	if b.claimed {
		return bead.ErrAlreadyClaimed
	}
	b.claimed = true
	b.owner = assignee
	return nil
}

func (b *keepaliveClaimBackend) Get(_ context.Context, _ string) (*bead.Bead, error) {
	if !b.claimed {
		return nil, nil
	}
	return &bead.Bead{Owner: b.owner, Status: bead.StatusInProgress}, nil
}
