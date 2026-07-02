package server

import (
	"context"
	"sync"

	"github.com/DocumentDrivenDX/ddx/internal/federation"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// hubFederationProvider adapts a *federationHub to the
// ddxgraphql.FederationProvider interface so federationNodes /
// federated{Beads,Runs,Projects} resolvers can read live registry state and
// drive fan-out against registered spokes.
//
// The provider applies StatusUpdates from each fan-out result back to the
// registry on the same lock the hub uses, so subsequent /federation page
// loads (or fan-out queries) observe the propagated transport-level
// status (offline) without an out-of-band reconcile loop.
type hubFederationProvider struct {
	server *Server
	once   sync.Once
	client *federation.FanOutClient
}

func newHubFederationProvider(s *Server) *hubFederationProvider {
	return &hubFederationProvider{server: s}
}

// fanOutClient is lazy-built so tests that swap s.hub mid-run see a fresh
// client wired to the latest hub config.
func (p *hubFederationProvider) fanOutClient() *federation.FanOutClient {
	p.once.Do(func() {
		c := federation.NewFanOutClient()
		c.HubDDxVersion = p.server.hub.hubDDxVersion
		c.HubSchemaVersion = p.server.hub.hubSchemaVer
		p.client = c
	})
	return p.client
}

// Spokes returns a copy of the current registry's spokes.
func (p *hubFederationProvider) Spokes() []federation.SpokeRecord {
	if p.server.hub == nil {
		return nil
	}
	p.server.hub.mu.Lock()
	defer p.server.hub.mu.Unlock()
	out := make([]federation.SpokeRecord, len(p.server.hub.registry.Spokes))
	copy(out, p.server.hub.registry.Spokes)
	return out
}

// FanOut runs req against every registered spoke and applies any returned
// StatusUpdates back to the registry (so transport-level offline transitions
// surface on the next /federation load).
func (p *hubFederationProvider) FanOut(ctx context.Context, req *federation.FanOutRequest) (*federation.FanOutResult, error) {
	if p.server.hub == nil {
		return &federation.FanOutResult{}, nil
	}
	spokes := p.Spokes()
	res, err := p.fanOutClient().Execute(ctx, spokes, req)
	if err != nil || res == nil {
		return res, err
	}
	if len(res.StatusUpdates) > 0 {
		p.server.hub.mu.Lock()
		for nodeID, st := range res.StatusUpdates {
			_ = p.server.hub.registry.SetStatus(nodeID, st)
		}
		p.server.persistFederationLocked()
		p.server.hub.mu.Unlock()
	}
	return res, nil
}

// Compile-time assertion that hubFederationProvider satisfies the resolver
// contract.
var _ ddxgraphql.FederationProvider = (*hubFederationProvider)(nil)
