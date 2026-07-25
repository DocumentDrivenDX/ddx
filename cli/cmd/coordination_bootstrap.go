package cmd

// coordination_bootstrap.go — shared reconnecting coordination client for
// ddx try, manual ddx work, and ddx work --server-managed (ADR-022 rev 6).

import (
	"context"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
)

// bootstrapCoordinationClient constructs the single reconnecting coordination
// client used by try/work claim, tracker-transition, and landing paths.
// Callers must Close the client on exit. Returns nil client when store is nil.
func bootstrapCoordinationClient(projectRoot string, store *bead.Store) (*agent.CoordinationClient, error) {
	if store == nil {
		return nil, nil
	}
	client, err := agent.NewCoordinationClient(projectRoot, agent.CoordinationClientConfig{
		WorkerID:         resolveClaimAssignee(),
		AddrFunc:         serverpkg.ReadServerAddr,
		HTTPClient:       newLocalServerClientTimeout(15 * time.Second),
		DiscoverInterval: agent.DefaultCoordinationDiscoverInterval,
		LandGitOps:       agent.RealLandingGitOps{},
		Store:            store,
	})
	if err != nil {
		return nil, err
	}
	client.Start(context.Background())
	return client, nil
}

// coordinationLandSubmit returns the land submit callback used by try/work.
// Prefer the shared coordination client; fall back to the production Land path
// only when the client could not be constructed.
func coordinationLandSubmit(projectRoot string, client *agent.CoordinationClient) func(agent.LandRequest) (*agent.LandResult, error) {
	if client != nil {
		return client.SubmitLand
	}
	return func(req agent.LandRequest) (*agent.LandResult, error) {
		return agent.Land(projectRoot, req, agent.RealLandingGitOps{})
	}
}
