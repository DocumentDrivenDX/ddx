package agent

import (
	"path/filepath"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	// Import the configinit package for its init() side-effect: it triggers
	// agent's internal/config init which registers the config loader into
	// agentlib so that agentlib.New(ServiceOptions{ConfigPath:…}) can resolve
	// provider configuration without a separate adapter. configinit is the
	// public marker package exposed for this purpose after agent v0.5.0
	// moved internal/config out of the public surface.
	_ "github.com/DocumentDrivenDX/agent/configinit"
)

// DefaultProviderRequestTimeout bounds a single Chat / ChatStream call.
// Defeats RC4 of ddx-0a651925: a stalled TCP socket that has delivered
// headers but stopped emitting body bytes would otherwise pin a goroutine
// until the outer wall-clock (3h) frees it.
const DefaultProviderRequestTimeout = 15 * time.Minute

// DefaultProviderIdleReadTimeout bounds the maximum idle gap between stream
// deltas. Used by service callers to bound idle reads on streaming providers.
const DefaultProviderIdleReadTimeout = 5 * time.Minute

// NewServiceFromWorkDir constructs a DdxAgent using the agent config found at
// <workDir>/.agent/config.yaml. ConfigPath is set to a file inside workDir so
// that filepath.Dir(ConfigPath) == workDir, and the agent library calls
// config.Load(workDir) which reads <workDir>/.agent/config.yaml.
func NewServiceFromWorkDir(workDir string) (agentlib.DdxAgent, error) {
	return agentlib.New(agentlib.ServiceOptions{
		ConfigPath: filepath.Join(workDir, "config.yaml"),
	})
}
