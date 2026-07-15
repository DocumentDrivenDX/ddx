package agent

import (
	"context"
	"path/filepath"

	agentlib "github.com/easel/fizeau"
	// Import the configinit package for its init() side-effect: it triggers
	// agent's internal/config init which registers the config loader into
	// agentlib so that agentlib.New(ServiceOptions{ConfigPath:…}) can resolve
	// provider configuration without a separate adapter. configinit is the
	// public marker package exposed for this purpose after agent v0.5.0
	// moved internal/config out of the public surface.
	_ "github.com/easel/fizeau/configinit"
)

// NewServiceFromWorkDir constructs the normal execution FizeauService for a DDx
// project. Fizeau owns provider config, provider discovery, include-by-default
// semantics, and policy routing. DDx does not synthesize a second provider
// registry from project configuration.
//
// NOTE on goroutine lifecycle: the returned service spawns background probe /
// quota-refresh / aliveness goroutines that are tied to
// ServiceOptions.QuotaRefreshContext (default context.Background). Short-lived
// callers (HTTP handlers, one-off CLI subcommands, tests) MUST prefer
// NewServiceFromWorkDirCtx and pass a request-scoped context so those
// goroutines are cancelled when the caller is done. Otherwise the goroutines
// leak for the lifetime of the process and accumulate under repeated calls
// (see bead ddx-server-fizeau-leak).
func NewServiceFromWorkDir(workDir string) (agentlib.FizeauService, error) {
	return agentlib.New(agentlib.ServiceOptions{
		ConfigPath: filepath.Join(workDir, "config.yaml"),
	})
}

// NewServiceFromWorkDirCtx is the context-scoped variant of
// NewServiceFromWorkDir. Cancelling ctx terminates the background quota
// refresh, quota recovery probe, and aliveness probe goroutines spawned by
// the returned service. Callers that issue requests at HTTP / RPC granularity
// SHOULD use this form with their request context to avoid goroutine leaks.
func NewServiceFromWorkDirCtx(ctx context.Context, workDir string) (agentlib.FizeauService, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return agentlib.New(agentlib.ServiceOptions{
		ConfigPath:          filepath.Join(workDir, "config.yaml"),
		QuotaRefreshContext: ctx,
	})
}

// NewPreflightServiceFromWorkDir constructs the short-lived service used by
// execution and capability queries. It disables Fizeau's background
// quota/aliveness/probe loops at construction time; callers still pass live
// request contexts to foreground service methods.
func NewPreflightServiceFromWorkDir(workDir string) (agentlib.FizeauService, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return NewServiceFromWorkDirCtx(ctx, workDir)
}
