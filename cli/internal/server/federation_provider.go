package server

import (
	"context"
	"io"
	"strings"
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

// ForwardMutation routes a single owner-targeted mutation to the spoke that
// owns the target project. It preserves the request metadata as headers so the
// spoke can enforce origin and idempotency constraints.
func (p *hubFederationProvider) ForwardMutation(ctx context.Context, req *federation.ForwardMutationRequest) (*federation.ForwardMutationResponse, error) {
	if p.server.hub == nil {
		return nil, federation.ErrForwardMutationOffline
	}
	if req == nil {
		return nil, federation.ErrForwardMutationBroadcastLike
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	p.server.hub.mu.Lock()
	target := p.server.hub.registry.FindSpoke(req.TargetNodeID)
	if target == nil {
		p.server.hub.mu.Unlock()
		return nil, federation.ErrForwardMutationMissingOwner
	}
	targetCopy := *target
	p.server.hub.mu.Unlock()

	if targetCopy.Status == federation.StatusOffline {
		return nil, federation.ErrForwardMutationOffline
	}
	if targetCopy.Status == federation.StatusStale {
		return nil, federation.ErrForwardMutationStale
	}
	if !spokeHasCapability(targetCopy.Capabilities, "write") {
		return nil, federation.ErrForwardMutationReadOnly
	}

	ownsProject := false
	for _, owned := range targetCopy.ProjectIDs {
		if owned == req.TargetProjectID {
			ownsProject = true
			break
		}
	}
	if !ownsProject {
		return nil, federation.ErrForwardMutationMissingOwner
	}

	headers := make(map[string]string, len(req.Headers)+5)
	for k, v := range req.Headers {
		headers[k] = v
	}
	if req.OriginIdentity != "" {
		headers[federationOriginIdentityHeader] = req.OriginIdentity
	}
	if coordinatorIdentity := strings.TrimSpace(p.server.federationSelfIdentity()); coordinatorIdentity != "" {
		headers[federationCoordinatorIdentityHeader] = coordinatorIdentity
	}
	if len(req.ForwardingPath) > 0 {
		headers["X-DDx-Forwarding-Path"] = strings.Join(req.ForwardingPath, " -> ")
	}
	if req.RequestID != "" {
		headers["X-DDx-Request-ID"] = req.RequestID
	}
	if req.IdempotencyKey != "" {
		headers["X-DDx-Idempotency-Key"] = req.IdempotencyKey
	}
	if req.TargetProjectID != "" {
		headers["X-DDx-Target-Project-ID"] = req.TargetProjectID
	}
	if req.ExpectedVersion != nil && *req.ExpectedVersion != "" {
		headers["X-DDx-Expected-Version"] = *req.ExpectedVersion
	}

	resp, err := federation.NewFanOutClient().ForwardMutation(ctx, &targetCopy, req.Body, headers)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &federation.ForwardMutationResponse{
		OriginIdentity:       req.OriginIdentity,
		ForwardingPath:       append([]string(nil), req.ForwardingPath...),
		RequestID:            req.RequestID,
		IdempotencyKey:       req.IdempotencyKey,
		TargetNodeID:         req.TargetNodeID,
		TargetProjectID:      req.TargetProjectID,
		ExpectedVersion:      req.ExpectedVersion,
		RequiredCapabilities: append([]string(nil), req.RequiredCapabilities...),
		StatusCode:           resp.StatusCode,
		Headers:              resp.Header.Clone(),
		Body:                 body,
	}, nil
}

func spokeHasCapability(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

// Compile-time assertion that hubFederationProvider satisfies the resolver
// contract.
var _ ddxgraphql.FederationProvider = (*hubFederationProvider)(nil)
