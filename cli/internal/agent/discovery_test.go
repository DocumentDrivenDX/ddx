package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryProvidersForModel(t *testing.T) {
	disc := &DiscoveryResult{
		Providers: []DiscoveredProvider{
			{Name: "vidar", Models: []string{"qwen3.6-35b-a3b", "qwen3.5-27b"}},
			{Name: "bragi", Models: []string{"qwen3.6-35b-a3b", "gemma-4-31b"}},
			{Name: "grendel", Models: []string{"gemma-4-31b"}},
		},
	}

	// Exact model match returns matching providers
	providers := disc.ProvidersForModel("qwen3.6-35b-a3b")
	require.Len(t, providers, 2)
	assert.Equal(t, "vidar", providers[0].Name)
	assert.Equal(t, "bragi", providers[1].Name)

	// Model only on one provider
	providers = disc.ProvidersForModel("qwen3.5-27b")
	require.Len(t, providers, 1)
	assert.Equal(t, "vidar", providers[0].Name)

	// Unknown model returns empty
	providers = disc.ProvidersForModel("nonexistent-model")
	assert.Empty(t, providers)

	// Nil discovery returns nil
	var nilDisc *DiscoveryResult
	assert.Nil(t, nilDisc.ProvidersForModel("anything"))
}

// TestDiscoveredModelRoutesToEmbeddedHarness verifies that when a ModelPin
// is not in the catalog but IS discovered on a provider, the agent/lmstudio
// harness gets a viable candidate plan (T1).
func TestDiscoveredModelRoutesToEmbeddedHarness(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = mockLookPath
	r.Discovery = &DiscoveryResult{
		Providers: []DiscoveredProvider{
			{Name: "vidar", Models: []string{"qwen3.6-35b-a3b"}},
		},
	}

	plans := r.BuildCandidatePlans(RouteRequest{ModelPin: "qwen3.6-35b-a3b"}, nil)

	// agent harness should be viable (embedded-openai surface, ExactPinSupport)
	var agentPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "agent" {
			agentPlan = &plans[i]
		}
	}

	require.NotNil(t, agentPlan, "agent candidate must exist")
	assert.True(t, agentPlan.Viable, "agent should be viable; got reject: %s", agentPlan.RejectReason)
	assert.Equal(t, "qwen3.6-35b-a3b", agentPlan.ConcreteModel)
	assert.Equal(t, "vidar", agentPlan.Provider)

	// Cloud harnesses (claude, codex) should be rejected for this uncataloged model
	for _, p := range plans {
		if p.Harness == "claude" || p.Harness == "codex" {
			assert.False(t, p.Viable, "%s should NOT be viable for discovered-only model", p.Harness)
			assert.Contains(t, p.RejectReason, "discovered on other providers")
		}
	}
}

// TestUndiscoveredUncatalogedModelErrors verifies that a model not in the
// catalog AND not discovered on any provider produces a clear error (T2).
func TestUndiscoveredUncatalogedModelErrors(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = mockLookPath
	r.Discovery = &DiscoveryResult{
		Providers: []DiscoveredProvider{
			{Name: "vidar", Models: []string{"some-other-model"}},
		},
	}

	err := r.ValidateOrphanModel(RunOptions{Model: "nonexistent-xyz"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the catalog and no discovered provider")
}

// TestProfileRoutingIgnoresDiscoveredModels verifies that profile routing
// (cheap/standard/smart) does NOT auto-select uncataloged discovered models (T3).
func TestProfileRoutingIgnoresDiscoveredModels(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = mockLookPath
	r.Discovery = &DiscoveryResult{
		Providers: []DiscoveredProvider{
			{Name: "vidar", Models: []string{"qwen3.6-35b-a3b"}},
		},
	}

	// Profile routing should go through catalog, not discovery
	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, nil)

	for _, p := range plans {
		if p.Viable && p.ConcreteModel == "qwen3.6-35b-a3b" {
			t.Errorf("profile routing should not select discovered-only model qwen3.6-35b-a3b on harness %s", p.Harness)
		}
	}
}

// TestPartialDiscoveryFailure verifies that when one provider is unreachable,
// routing succeeds with the responding provider (T4).
func TestPartialDiscoveryFailure(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = mockLookPath
	// Simulate: grendel failed to respond, vidar succeeded
	r.Discovery = &DiscoveryResult{
		Providers: []DiscoveredProvider{
			{Name: "vidar", Models: []string{"qwen3.6-35b-a3b"}},
			// grendel is absent — it timed out during discovery
		},
	}

	plans := r.BuildCandidatePlans(RouteRequest{ModelPin: "qwen3.6-35b-a3b"}, nil)

	var agentPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "agent" {
			agentPlan = &plans[i]
			break
		}
	}

	require.NotNil(t, agentPlan)
	assert.True(t, agentPlan.Viable, "should route to vidar despite grendel failure")
	assert.Equal(t, "vidar", agentPlan.Provider)
}

// TestDiscoveryCacheReuse verifies that the cache returns results within TTL (T5).
func TestDiscoveryCacheReuse(t *testing.T) {
	InvalidateDiscoveryCache()

	disc1 := &DiscoveryResult{
		Providers: []DiscoveredProvider{
			{Name: "vidar", Models: []string{"model-a"}},
		},
	}

	// Manually populate cache
	discoveryCache.mu.Lock()
	discoveryCache.result = disc1
	disc1.CachedAt = timeNow()
	discoveryCache.mu.Unlock()

	// Cache hit should return same result without probing
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = mockLookPath
	r.WorkDir = t.TempDir() // won't have agent config, but cache should hit first

	// Trigger discovery via ProbeAndBuildCandidatePlans with a ModelPin
	_ = r.ProbeAndBuildCandidatePlans(RouteRequest{ModelPin: "model-a"}, 5)

	assert.NotNil(t, r.Discovery)
	require.Len(t, r.Discovery.Providers, 1)
	assert.Equal(t, "vidar", r.Discovery.Providers[0].Name)

	InvalidateDiscoveryCache()
}

func timeNow() time.Time {
	return time.Now()
}
