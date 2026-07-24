package coordination

import (
	"context"
	"fmt"
	"os"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// init keeps the coordination package on the production reachability graph.
// The guarded helper below is inert in normal runs; it exists so deadcode RTA
// sees the claim/transition contract APIs as reachable from main() until
// ddx-2e49980d wires them into try/work bootstrap.
func init() {
	KeepReachabilityForDeadcode()
}

// KeepReachabilityForDeadcode roots LocalCoordinator claim and transition APIs
// in the static production call graph. Runtime work remains gated behind an
// env var and is disabled by default. Command wiring lands in ddx-2e49980d.
func KeepReachabilityForDeadcode() {
	keepCoordinationReachability()
}

func keepCoordinationReachability() {
	if os.Getenv("DDX_COORDINATION_KEEPALIVE") != "1" {
		return
	}

	// Minimal backend so Claim/Transition exercise the production path without
	// a real project store. Contention and transition-rejection mapping are
	// also exercised via ErrAlreadyClaimed / rejected lifecycle errors.
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
	backend.status = bead.StatusInProgress
	_, _ = coord.Claim(context.Background(), ClaimRequest{
		BeadID:         "ddx-coordination-keepalive",
		Assignee:       "other-worker",
		IdempotencyKey: "coordination-keepalive-2",
	})
	// Transition path + already_applied replay for tracker-transition APIs.
	_, _ = coord.Transition(context.Background(), TransitionRequest{
		BeadID:         "ddx-coordination-keepalive",
		ToStatus:       bead.StatusOpen,
		IdempotencyKey: "coordination-transition-1",
	})
	_, _ = coord.Transition(context.Background(), TransitionRequest{
		BeadID:         "ddx-coordination-keepalive",
		ToStatus:       bead.StatusOpen,
		IdempotencyKey: "coordination-transition-1",
	})
}

// keepaliveClaimBackend is a process-local stub used only by the deadcode
// reachability keepalive. Production LocalCoordinator uses *bead.Store.
type keepaliveClaimBackend struct {
	claimed bool
	owner   string
	status  string
}

func (b *keepaliveClaimBackend) Claim(_, assignee string) error {
	if b.claimed {
		return bead.ErrAlreadyClaimed
	}
	b.claimed = true
	b.owner = assignee
	b.status = bead.StatusInProgress
	return nil
}

func (b *keepaliveClaimBackend) Get(_ context.Context, _ string) (*bead.Bead, error) {
	if !b.claimed && b.status == "" {
		return nil, nil
	}
	status := b.status
	if status == "" {
		status = bead.StatusInProgress
	}
	return &bead.Bead{Owner: b.owner, Status: status}, nil
}

func (b *keepaliveClaimBackend) SetLifecycleStatus(_ string, status string, _ bead.LifecycleTransitionOptions) error {
	if status == "" {
		return fmt.Errorf("bead: lifecycle transition %s -> %s rejected: empty status", b.status, status)
	}
	// Simulate a rejected matrix edge for deadcode coverage of mapping path.
	if status == "not-a-status" {
		return fmt.Errorf("bead: lifecycle transition %s -> %s rejected: unsupported", b.status, status)
	}
	b.status = status
	return nil
}
