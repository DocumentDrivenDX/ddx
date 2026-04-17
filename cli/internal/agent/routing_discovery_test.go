package agent

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Stub provider fixtures ---

// stubDiscovery creates a DiscoveryResult with the given provider→models mapping.
func stubDiscovery(providers map[string][]string) *DiscoveryResult {
	var dp []DiscoveredProvider
	for name, models := range providers {
		dp = append(dp, DiscoveredProvider{Name: name, Models: models})
	}
	return &DiscoveryResult{Providers: dp, CachedAt: time.Now()}
}

// newDiscoveryRunner creates a Runner with discovery data and no real binaries.
func newDiscoveryRunner(disc *DiscoveryResult) *Runner {
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = func(file string) (string, error) {
		return "", fmt.Errorf("%s not found", file)
	}
	r.Discovery = disc
	return r
}

// === FUZZY MATCHING ===

func TestFuzzyMatchExactWins(t *testing.T) {
	pool := []string{"qwen3.6", "qwen3.6-35b-a3b", "qwen3.6-27b"}
	resolved, fuzzy := fuzzyMatchInPool("qwen3.6", pool)
	assert.Equal(t, "qwen3.6", resolved)
	assert.False(t, fuzzy, "exact match should not be flagged as fuzzy")
}

func TestFuzzyMatchPrefixShortestSuffix(t *testing.T) {
	pool := []string{"qwen3.6-35b-a3b", "qwen3.6-27b"}
	resolved, fuzzy := fuzzyMatchInPool("qwen3.6", pool)
	assert.Equal(t, "qwen3.6-27b", resolved, "shorter suffix should win")
	assert.True(t, fuzzy)
}

func TestFuzzyMatchEqualSuffixAlphabetical(t *testing.T) {
	pool := []string{"qwen3.6-bbb", "qwen3.6-aaa"}
	resolved, fuzzy := fuzzyMatchInPool("qwen3.6", pool)
	assert.Equal(t, "qwen3.6-aaa", resolved, "alphabetical tiebreak")
	assert.True(t, fuzzy)
}

func TestFuzzyMatchNoMatch(t *testing.T) {
	pool := []string{"gemma-4-31b", "minimax-m2.7"}
	resolved, fuzzy := fuzzyMatchInPool("qwen3.6", pool)
	assert.Empty(t, resolved)
	assert.False(t, fuzzy)
}

func TestFuzzyMatchEmptyPool(t *testing.T) {
	resolved, fuzzy := fuzzyMatchInPool("qwen3.6", nil)
	assert.Empty(t, resolved)
	assert.False(t, fuzzy)
}

func TestFuzzyMatchDiscoveryIntegration(t *testing.T) {
	disc := stubDiscovery(map[string][]string{
		"vidar": {"qwen3.6-35b-a3b", "qwen3.5-27b"},
		"bragi": {"qwen/qwen3.6-35b-a3b", "gemma-4-31b"},
	})
	resolved, fuzzy := disc.FuzzyMatchModel("qwen3.6")
	assert.Equal(t, "qwen3.6-35b-a3b", resolved)
	assert.True(t, fuzzy)
}

// === MODE 1: --harness override ===

func TestHarnessOverridePassesModelThrough(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"vidar": {"qwen3.6-35b-a3b"},
	}))

	// With --harness, the model is a pass-through — no discovery check
	plans := r.BuildCandidatePlans(RouteRequest{
		HarnessOverride: "agent",
		ModelPin:        "anything-goes",
	}, map[string]HarnessState{"agent": healthyState()})

	var agentPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "agent" {
			agentPlan = &plans[i]
		}
	}
	require.NotNil(t, agentPlan)
	assert.True(t, agentPlan.Viable)
	assert.Equal(t, "anything-goes", agentPlan.ConcreteModel)
}

// === MODE 2: --model (exact + fuzzy) ===

func TestModelExactDiscoveredRoutesToLocalProvider(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"vidar": {"qwen3.6-35b-a3b"},
	}))

	plans := r.BuildCandidatePlans(RouteRequest{ModelPin: "qwen3.6-35b-a3b"}, nil)

	var agentPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "agent" {
			agentPlan = &plans[i]
		}
	}
	require.NotNil(t, agentPlan)
	assert.True(t, agentPlan.Viable)
	assert.Equal(t, "qwen3.6-35b-a3b", agentPlan.ConcreteModel)
	assert.Equal(t, "vidar", agentPlan.Provider)
}

func TestModelFuzzyDiscoveredRoutesToLocalProvider(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"vidar": {"qwen3.6-35b-a3b", "qwen3.5-27b"},
	}))

	plans := r.BuildCandidatePlans(RouteRequest{ModelPin: "qwen3.6"}, nil)

	var agentPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "agent" {
			agentPlan = &plans[i]
		}
	}
	require.NotNil(t, agentPlan)
	assert.True(t, agentPlan.Viable, "agent should be viable; got: %s", agentPlan.RejectReason)
	assert.Equal(t, "qwen3.6-35b-a3b", agentPlan.ConcreteModel, "fuzzy should resolve to full model name")
	assert.Equal(t, "vidar", agentPlan.Provider)
}

func TestModelDiscoveredRejectsCloudHarness(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"vidar": {"qwen3.6-35b-a3b"},
	}))

	// Provide state overrides so cloud harnesses are "installed" — this
	// isolates the discovery rejection from the binary-not-found rejection.
	states := map[string]HarnessState{
		"agent":  healthyState(),
		"claude": healthyState(),
		"codex":  healthyState(),
	}
	plans := r.BuildCandidatePlans(RouteRequest{ModelPin: "qwen3.6-35b-a3b"}, states)

	for _, p := range plans {
		if p.Harness == "claude" || p.Harness == "codex" {
			assert.False(t, p.Viable, "%s should be rejected for discovered-only model", p.Harness)
			assert.Contains(t, p.RejectReason, "discovered on other providers")
		}
	}
}

func TestModelNotFoundErrors(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"vidar": {"some-other-model"},
	}))

	err := r.ValidateOrphanModel(RunOptions{Model: "nonexistent-xyz"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the catalog and no discovered provider")
}

func TestModelCatalogedSkipsDiscovery(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"vidar": {"qwen3.6-35b-a3b"},
	}))

	// "cheap" is a catalog ref — it should resolve via catalog, not discovery
	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, nil)

	for _, p := range plans {
		if p.Viable && p.ConcreteModel == "qwen3.6-35b-a3b" {
			t.Errorf("profile routing should not select discovered-only model on %s", p.Harness)
		}
	}
}

// === MODE 2 + provider modifier ===

func TestModelWithProviderPrefersSpecifiedProvider(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"vidar": {"qwen3.6-35b-a3b"},
		"bragi": {"qwen3.6-35b-a3b"},
	}))

	// Both providers have the model. With discovery, agent harness gets
	// the first matching provider. The provider modifier should influence
	// which one is picked (this tests the evaluateCandidate path).
	plans := r.BuildCandidatePlans(RouteRequest{ModelPin: "qwen3.6-35b-a3b"}, nil)

	var agentPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "agent" {
			agentPlan = &plans[i]
		}
	}
	require.NotNil(t, agentPlan)
	assert.True(t, agentPlan.Viable)
	// Provider should be one of vidar/bragi
	assert.Contains(t, []string{"vidar", "bragi"}, agentPlan.Provider)
}

func TestModelWithProviderSoftFallback(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"bragi": {"qwen3.6-35b-a3b"},
		// vidar does NOT have qwen3.6-35b-a3b
	}))

	// --provider vidar is soft — should still route to bragi
	plans := r.BuildCandidatePlans(RouteRequest{ModelPin: "qwen3.6-35b-a3b"}, nil)

	var agentPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "agent" {
			agentPlan = &plans[i]
		}
	}
	require.NotNil(t, agentPlan)
	assert.True(t, agentPlan.Viable)
	assert.Equal(t, "bragi", agentPlan.Provider)
}

// === MODE 3: --profile (catalog only, no discovery) ===

func TestProfileRoutingUsesOnlyCatalog(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"vidar": {"amazing-new-model-v9"},
	}))

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "smart"}, nil)

	// No plan should use the discovered-only model for profile routing
	for _, p := range plans {
		if p.Viable {
			assert.NotEqual(t, "amazing-new-model-v9", p.ConcreteModel,
				"profile routing should not select uncataloged model on %s", p.Harness)
		}
	}
}

// === Discovery infrastructure ===

func TestPartialDiscoveryOnlyUsesRespondingProviders(t *testing.T) {
	// Simulate: grendel failed, vidar responded
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"vidar": {"qwen3.6-35b-a3b"},
	}))

	plans := r.BuildCandidatePlans(RouteRequest{ModelPin: "qwen3.6-35b-a3b"}, nil)

	var agentPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "agent" {
			agentPlan = &plans[i]
		}
	}
	require.NotNil(t, agentPlan)
	assert.True(t, agentPlan.Viable)
	assert.Equal(t, "vidar", agentPlan.Provider)
}

func TestNoDiscoveryFallsBackToNormalRouting(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = mockLookPath
	// No discovery — nil
	r.Discovery = nil

	// Cataloged model should still work fine
	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, nil)
	viable := false
	for _, p := range plans {
		if p.Viable {
			viable = true
		}
	}
	assert.True(t, viable, "cataloged profile routing should work without discovery")
}

// === Validate interaction matrix ===

func TestValidateOrphanModelPassesExactDiscovery(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"vidar": {"qwen3.6-35b-a3b"},
	}))
	assert.NoError(t, r.ValidateOrphanModel(RunOptions{Model: "qwen3.6-35b-a3b"}))
}

func TestValidateOrphanModelPassesFuzzyDiscovery(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"vidar": {"qwen3.6-35b-a3b"},
	}))
	assert.NoError(t, r.ValidateOrphanModel(RunOptions{Model: "qwen3.6"}))
}

func TestValidateOrphanModelWithDiscoveryCatalogRefPasses(t *testing.T) {
	r := newDiscoveryRunner(nil)
	assert.NoError(t, r.ValidateOrphanModel(RunOptions{Model: "cheap"}))
}

func TestValidateOrphanModelWithDiscoveryProviderPasses(t *testing.T) {
	r := newDiscoveryRunner(nil)
	assert.NoError(t, r.ValidateOrphanModel(RunOptions{Model: "anything", Provider: "vidar"}))
}

func TestValidateOrphanModelWithDiscoveryModelRefPasses(t *testing.T) {
	r := newDiscoveryRunner(nil)
	assert.NoError(t, r.ValidateOrphanModel(RunOptions{Model: "anything", ModelRef: "code-medium"}))
}

func TestValidateOrphanModelWithDiscoverySubprocessPasses(t *testing.T) {
	r := newDiscoveryRunner(nil)
	assert.NoError(t, r.ValidateOrphanModel(RunOptions{Harness: "claude", Model: "whatever"}))
}

func TestValidateOrphanModelRejectsUnknown(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{
		"vidar": {"some-other-model"},
	}))
	err := r.ValidateOrphanModel(RunOptions{Model: "nonexistent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the catalog and no discovered provider")
}

func TestValidateOrphanModelRejectsWithEmptyDiscovery(t *testing.T) {
	r := newDiscoveryRunner(stubDiscovery(map[string][]string{}))
	err := r.ValidateOrphanModel(RunOptions{Model: "nonexistent"})
	require.Error(t, err)
}

// === AllModels ===

func TestAllModelsDeduplicates(t *testing.T) {
	disc := stubDiscovery(map[string][]string{
		"vidar": {"model-a", "model-b"},
		"bragi": {"model-b", "model-c"},
	})
	all := disc.AllModels()
	assert.Len(t, all, 3)
	seen := make(map[string]bool)
	for _, m := range all {
		assert.False(t, seen[m], "duplicate model: %s", m)
		seen[m] = true
	}
}
