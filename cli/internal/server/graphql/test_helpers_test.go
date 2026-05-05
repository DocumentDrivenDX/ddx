package graphql

import (
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// NewResolver is a test/helper constructor for resolvers that need the
// production validation logic without exposing the seam to deadcode scans.
func NewResolver(state StateProvider, workingDir string) (*Resolver, error) {
	if state == nil {
		return nil, fmt.Errorf("resolver: state provider is required")
	}
	if workingDir == "" {
		return nil, fmt.Errorf("resolver: working directory is required")
	}
	return &Resolver{
		State:      state,
		WorkingDir: workingDir,
	}, nil
}

func resetProviderModelsCacheForTest() {
	providerModelsCache.Lock()
	providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
	providerModelsCache.inFlight = make(map[string]chan struct{})
	providerModelsCache.Unlock()
}

// RecordHarnessRateLimit stores the latest parsed rate-limit signal for a
// harness invocation.
func RecordHarnessRateLimit(name string, sig agent.RateLimitSignal) {
	name = strings.TrimSpace(name)
	if name == "" || !sig.HasAny() {
		return
	}
	harnessRateLimitCache.Lock()
	harnessRateLimitCache.byName[name] = sig
	harnessRateLimitCache.Unlock()
}

func resetHarnessRateLimitCache() {
	harnessRateLimitCache.Lock()
	harnessRateLimitCache.byName = make(map[string]agent.RateLimitSignal)
	harnessRateLimitCache.Unlock()
}
