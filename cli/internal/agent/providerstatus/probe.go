// Package providerstatus provides a unified provider liveness probe shared by
// the providers, check, models, and route-status commands.
package providerstatus

import (
	"context"
	"fmt"
	"strings"

	agentconfig "github.com/DocumentDrivenDX/agent/config"
	oai "github.com/DocumentDrivenDX/agent/provider/openai"
)

// Result is the outcome of a single provider probe.
type Result struct {
	// Reachable is true when the provider responded (or has a configured API
	// key for Anthropic providers that do not expose a listing endpoint).
	Reachable bool
	// Models contains the discovered model IDs for OAI-compat providers.
	// It is nil for Anthropic providers (no /v1/models endpoint).
	Models []string
	// Message is a short human-readable status summary.
	Message string
}

// Probe checks a single provider's liveness and returns a Result.
//
// For Anthropic providers it checks for a non-empty API key. For
// OAI-compatible providers it calls the /v1/models endpoint using the
// deadline already set on ctx. Callers must wrap ctx with
// context.WithTimeout when a specific probe timeout is required.
func Probe(ctx context.Context, pc agentconfig.ProviderConfig) Result {
	if pc.Type == "anthropic" {
		if pc.APIKey == "" {
			return Result{Reachable: false, Message: "missing API key"}
		}
		return Result{Reachable: true, Models: nil, Message: "api key configured"}
	}
	if strings.TrimSpace(pc.BaseURL) == "" {
		return Result{Reachable: false, Message: "no URL configured"}
	}
	ids, err := oai.DiscoverModels(ctx, pc.BaseURL, pc.APIKey)
	if err != nil {
		return Result{Reachable: false, Message: err.Error()}
	}
	return Result{Reachable: true, Models: ids, Message: fmt.Sprintf("connected (%d models)", len(ids))}
}
