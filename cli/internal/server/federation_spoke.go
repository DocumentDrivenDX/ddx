package server

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/federation"
)

// federationSpoke holds the per-server state for spoke-mode federation: the
// running spoke lifecycle agent and its config (so tests can introspect what
// was advertised to the hub).
type federationSpoke struct {
	agent   *federation.Spoke
	hubURL  string
	selfURL string
}

// EnableSpokeMode launches the spoke-side lifecycle: register-on-start,
// jittered heartbeat, URL-change detection across restarts, and best-effort
// deregister-on-shutdown. Idempotent: a second call is a no-op.
//
// hubURL is the federation hub base URL (e.g. "https://hub:7743").
// selfURL is this server's externally-reachable URL advertised to the hub
// (e.g. "https://node-a:7743"). When empty, a default is constructed from
// the bind address.
//
// The returned error reflects the initial registration outcome — if the hub
// rejects the registration (version mismatch, identity conflict, etc.) the
// spoke does not start.
//
// Both EnableHubMode and EnableSpokeMode may be called on the same server
// (hub_spoke role).
func (s *Server) EnableSpokeMode(ctx context.Context, hubURL, selfURL string, opts ...SpokeOption) error {
	if s.spoke != nil {
		return nil
	}
	if strings.TrimSpace(hubURL) == "" {
		return fmt.Errorf("federation: --hub-address URL required for spoke mode")
	}
	if strings.TrimSpace(selfURL) == "" {
		// Best-effort default: scheme is https + bind address. Operators can
		// override via the explicit selfURL arg.
		selfURL = "https://" + s.Addr
	}

	cfg := federation.SpokeConfig{
		NodeID:        s.state.Node.ID,
		Name:          s.state.Node.Name,
		URL:           selfURL,
		HubURL:        hubURL,
		DDxVersion:    HubDefaultDDxVersion,
		SchemaVersion: federation.CurrentSchemaVersion,
		Logger:        func(format string, args ...any) { log.Printf("WARN: "+format, args...) },
	}
	for _, proj := range s.state.GetProjects() {
		if proj.ID != "" {
			cfg.ProjectIDs = append(cfg.ProjectIDs, proj.ID)
		}
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	agent, err := federation.NewSpoke(cfg)
	if err != nil {
		return err
	}

	if agent.URLChanged() {
		log.Printf("federation: spoke URL changed from %q to %q; will re-register with new URL",
			agent.PreviousURL(), selfURL)
	}

	if err := agent.Start(ctx); err != nil {
		return fmt.Errorf("federation: spoke register: %w", err)
	}

	s.spoke = &federationSpoke{agent: agent, hubURL: hubURL, selfURL: selfURL}
	s.SpokeMode = true
	return nil
}

// SpokeOption customises the SpokeConfig used by EnableSpokeMode.
type SpokeOption func(*federation.SpokeConfig)

// SpokeAgent exposes the underlying federation.Spoke for tests and operator
// introspection. Returns nil if spoke mode is not enabled.
func (s *Server) SpokeAgent() *federation.Spoke {
	if s.spoke == nil {
		return nil
	}
	return s.spoke.agent
}

// ShutdownSpoke stops the spoke lifecycle (heartbeat + best-effort deregister).
// Safe to call when spoke mode is not enabled — returns nil. Called from
// graceful shutdown paths so the hub registry does not show a stale spoke.
func (s *Server) ShutdownSpoke(ctx context.Context) error {
	if s.spoke == nil {
		return nil
	}
	return s.spoke.agent.Shutdown(ctx)
}
