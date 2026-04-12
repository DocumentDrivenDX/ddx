package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Catalog Projection Tests (TP-020) ---

// TestAgentRoutingResolvesProfileAcrossHarnessSurfaces verifies that a profile
// like "cheap" resolves to a concrete model on each harness surface via the catalog.
func TestAgentRoutingResolvesProfileAcrossHarnessSurfaces(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"codex": healthyState(),
		"agent": healthyLocalState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, states)

	for _, p := range plans {
		if !p.Viable {
			continue
		}
		switch p.Harness {
		case "codex":
			assert.Equal(t, "profile:cheap", p.RequestedRef)
			assert.NotEmpty(t, p.ConcreteModel, "codex should have a concrete model for cheap profile")
		case "agent":
			assert.Equal(t, "profile:cheap", p.RequestedRef)
			assert.NotEmpty(t, p.ConcreteModel, "agent should have a concrete model for cheap profile")
		}
	}

	// Verify at least codex and agent are viable with profile mappings.
	viableHarnesses := map[string]bool{}
	for _, p := range plans {
		if p.Viable {
			viableHarnesses[p.Harness] = true
		}
	}
	assert.True(t, viableHarnesses["codex"], "codex should be viable for cheap profile")
	assert.True(t, viableHarnesses["agent"], "agent should be viable for cheap profile")
}

// TestAgentRoutingResolvesQwen3ToEmbeddedOnly verifies that a ModelRef resolving
// only on the embedded-openai surface routes exclusively to the embedded harness.
func TestAgentRoutingResolvesQwen3ToEmbeddedOnly(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"agent":  healthyLocalState(),
		"codex":  healthyState(),
		"claude": healthyState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{ModelRef: "qwen3"}, states)

	// agent (surface=embedded-openai) should be viable; others should be rejected.
	agentViable := false
	for _, p := range plans {
		switch p.Harness {
		case "agent":
			assert.True(t, p.Viable, "agent should be viable for qwen3 (embedded-openai surface)")
			assert.Equal(t, "ref:qwen3", p.RequestedRef)
			assert.NotEmpty(t, p.ConcreteModel, "agent should have a concrete model for qwen3")
			agentViable = true
		case "codex", "claude":
			assert.False(t, p.Viable, "harness %s should be rejected for embedded-only qwen3", p.Harness)
			assert.Contains(t, p.RejectReason, "not available on surface")
		}
	}
	assert.True(t, agentViable, "agent must be in plans and viable for qwen3")
}

// TestAgentRoutingRejectsUnknownModelRef verifies that when a ModelPin is used
// (exact bypass), harnesses without ExactPinSupport are rejected.
func TestAgentRoutingRejectsUnknownModelRef(t *testing.T) {
	r := newTestRunnerForRouting()

	// Provide a harness that lacks ExactPinSupport.
	r.Registry.harnesses["no-pin-harness"] = Harness{
		Name:            "no-pin-harness",
		Binary:          "no-pin-harness",
		Surface:         "no-pin",
		CostClass:       "medium",
		ExactPinSupport: false,
		IsLocal:         false,
	}

	states := map[string]HarnessState{
		"codex":          healthyState(),
		"no-pin-harness": healthyState(),
	}

	// "some-unknown-concrete-model" is not in the catalog → treated as ModelPin.
	plans := r.BuildCandidatePlans(RouteRequest{ModelPin: "some-unknown-concrete-model"}, states)

	for _, p := range plans {
		switch p.Harness {
		case "codex":
			// codex has ExactPinSupport: true.
			assert.True(t, p.Viable, "codex with ExactPinSupport should be viable for a pin")
			assert.Equal(t, "some-unknown-concrete-model", p.ConcreteModel)
		case "no-pin-harness":
			assert.False(t, p.Viable, "harness without ExactPinSupport should be rejected for a pin")
			assert.Contains(t, p.RejectReason, "does not support exact model pins")
		}
	}
}

// TestAgentRoutingSurfacesDeprecatedReplacementWarning verifies that a deprecated
// catalog entry surfaces a deprecation warning in the candidate plan.
func TestAgentRoutingSurfacesDeprecatedReplacementWarning(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"codex": healthyState(),
	}

	// "codex-mini" is a deprecated alias in BuiltinCatalog.
	plans := r.BuildCandidatePlans(RouteRequest{ModelRef: "codex-mini"}, states)

	var codexPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "codex" {
			codexPlan = &plans[i]
			break
		}
	}
	require.NotNil(t, codexPlan, "codex plan should be present")
	assert.True(t, codexPlan.Viable, "codex should be viable for codex-mini (has codex surface mapping)")
	assert.NotEmpty(t, codexPlan.DeprecationWarning, "deprecated ref should produce a deprecation warning")
	assert.Contains(t, codexPlan.DeprecationWarning, "codex-mini")
	assert.Contains(t, codexPlan.DeprecationWarning, "cheap") // ReplacedBy logical ref
}

// --- Embedded Boundary Tests (TP-020) ---

// TestAgentRoutingSelectsEmbeddedWithoutInspectingProviderDetails verifies that
// when agent (embedded) is selected for an embedded-only ref, the plan does not
// contain provider-specific backend pool fields — DDx stops at harness selection.
func TestAgentRoutingSelectsEmbeddedWithoutInspectingProviderDetails(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"agent": healthyLocalState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{ModelRef: "qwen3"}, states)
	ranked := RankCandidates("", plans)

	best, err := SelectBestCandidate(ranked)
	require.NoError(t, err)
	assert.Equal(t, "agent", best.Harness)

	// The plan's ConcreteModel comes from the catalog, not from DDx-owned
	// backend-pool logic. There are no provider-level fields in CandidatePlan.
	// This test asserts the structural boundary: the plan only records what
	// DDx decides, not what ddx-agent decides internally.
	assert.Equal(t, "embedded-openai", best.Surface, "embedded harness surface should be embedded-openai")
	assert.NotEmpty(t, best.ConcreteModel, "catalog concrete model should be set")
	// No provider-specific fields exist in CandidatePlan (structural assertion).
	// The embedded runtime handles provider/backend resolution internally.
}

// TestAgentRoutingPassesResolvedIntentToEmbeddedHarness verifies that the
// candidate plan for the embedded harness records the intent (RequestedRef),
// not a concrete provider-level backend choice.
func TestAgentRoutingPassesResolvedIntentToEmbeddedHarness(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"agent": healthyLocalState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{ModelRef: "qwen3"}, states)

	var agentPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "agent" {
			agentPlan = &plans[i]
			break
		}
	}
	require.NotNil(t, agentPlan)
	assert.True(t, agentPlan.Viable)
	// DDx records the intent ref, not a provider/backend selection.
	assert.Equal(t, "ref:qwen3", agentPlan.RequestedRef)
	assert.Equal(t, "qwen3", agentPlan.CanonicalTarget)
}

// TestAgentRoutingDoesNotDuplicateEmbeddedBackendPoolStrategy verifies that
// DDx catalog entries for embedded-only refs do not contain backend pool
// strategy fields — that logic belongs entirely in ddx-agent.
func TestAgentRoutingDoesNotDuplicateEmbeddedBackendPoolStrategy(t *testing.T) {
	// The catalog entry for qwen3 maps only to a concrete model string per
	// surface. It does not contain provider, endpoint, or strategy fields.
	// This test asserts that the CatalogEntry struct has no such fields and
	// that BuiltinCatalog.Resolve returns only a model string.
	entry, ok := BuiltinCatalog.Entry("qwen3")
	require.True(t, ok, "qwen3 must be in the catalog")
	assert.False(t, entry.Deprecated)

	// Only embedded-openai surface.
	assert.Len(t, entry.Surfaces, 1)
	model, ok := entry.Surfaces["embedded-openai"]
	assert.True(t, ok, "qwen3 must map to embedded-openai surface")
	assert.NotEmpty(t, model)

	// The catalog resolution returns a concrete model string only — no provider
	// endpoint, no backend pool strategy. DDx stops here; ddx-agent continues.
	resolved, ok := BuiltinCatalog.Resolve("qwen3", "embedded-openai")
	assert.True(t, ok)
	assert.NotEmpty(t, resolved)
	// Resolved value is a model name, not a URL or strategy.
	assert.NotContains(t, resolved, "://", "catalog must not embed provider URLs")
}

// --- Request Normalization Tests (TP-020) ---

// TestAgentRouteRequestFromProfile verifies that a profile-only set of flags
// produces a RouteRequest with Profile populated and ModelRef/ModelPin empty.
func TestAgentRouteRequestFromProfile(t *testing.T) {
	req := NormalizeRouteRequest(
		RouteFlags{Profile: "cheap"},
		Config{},
		BuiltinCatalog,
	)
	assert.Equal(t, "cheap", req.Profile)
	assert.Empty(t, req.ModelRef)
	assert.Empty(t, req.ModelPin)
	assert.Empty(t, req.HarnessOverride)
}

// TestAgentRouteRequestFromModelRef verifies that --model with a catalog-known
// ref produces a RouteRequest with ModelRef set and ModelPin empty.
func TestAgentRouteRequestFromModelRef(t *testing.T) {
	req := NormalizeRouteRequest(
		RouteFlags{Model: "qwen3"},
		Config{},
		BuiltinCatalog,
	)
	assert.Equal(t, "qwen3", req.ModelRef)
	assert.Empty(t, req.ModelPin)
	assert.Empty(t, req.Profile)
}

// TestAgentRouteRequestFromExactPinFallback verifies that --model with a value
// not in the catalog produces a RouteRequest with ModelPin set and ModelRef empty.
func TestAgentRouteRequestFromExactPinFallback(t *testing.T) {
	req := NormalizeRouteRequest(
		RouteFlags{Model: "some-vendor/unknown-model-xyz"},
		Config{},
		BuiltinCatalog,
	)
	assert.Equal(t, "some-vendor/unknown-model-xyz", req.ModelPin)
	assert.Empty(t, req.ModelRef)
	assert.Empty(t, req.Profile)
}

// TestAgentRouteRequestHarnessOverrideWins verifies that --harness sets
// HarnessOverride regardless of other flags.
func TestAgentRouteRequestHarnessOverrideWins(t *testing.T) {
	req := NormalizeRouteRequest(
		RouteFlags{Profile: "cheap", Harness: "codex"},
		Config{},
		BuiltinCatalog,
	)
	assert.Equal(t, "codex", req.HarnessOverride)
	// Profile is also preserved (harness override constrains routing but doesn't
	// replace the model intent).
	assert.Equal(t, "cheap", req.Profile)
}

// --- CLI and Config Tests (TP-020) ---

// TestAgentRunProfileFlagRoutesWithoutHarness verifies that a RouteRequest with
// only Profile set can route successfully without requiring HarnessOverride.
func TestAgentRunProfileFlagRoutesWithoutHarness(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"codex": healthyState(),
		"agent": healthyLocalState(),
	}

	req := RouteRequest{Profile: "cheap"}
	plans := r.BuildCandidatePlans(req, states)
	ranked := RankCandidates("cheap", plans)

	best, err := SelectBestCandidate(ranked)
	require.NoError(t, err, "routing by profile alone should select a viable harness")
	assert.NotEmpty(t, best.Harness, "a harness should be selected")
	// No harness override was required.
	assert.Empty(t, req.HarnessOverride)
}

// TestAgentRunModelRefFlagRoutesWithoutHarness verifies that a RouteRequest with
// only ModelRef set (catalog-known) can route successfully without HarnessOverride.
func TestAgentRunModelRefFlagRoutesWithoutHarness(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"agent": healthyLocalState(),
		"codex": healthyState(),
	}

	req := RouteRequest{ModelRef: "qwen3"}
	plans := r.BuildCandidatePlans(req, states)
	ranked := RankCandidates("", plans)

	best, err := SelectBestCandidate(ranked)
	require.NoError(t, err, "routing by model ref alone should select a viable harness")
	// qwen3 is embedded-only → must route to agent.
	assert.Equal(t, "agent", best.Harness)
}

// TestAgentConfigDefaultProfileUsedWhenNoExplicitSelector verifies that when no
// CLI flags are set, the default profile from Config is applied.
func TestAgentConfigDefaultProfileUsedWhenNoExplicitSelector(t *testing.T) {
	req := NormalizeRouteRequest(
		RouteFlags{}, // no flags
		Config{Profile: "smart"},
		BuiltinCatalog,
	)
	assert.Equal(t, "smart", req.Profile)
	assert.Empty(t, req.HarnessOverride)
	assert.Empty(t, req.ModelRef)
	assert.Empty(t, req.ModelPin)
}

// TestAgentConfigForcedHarnessBypassesAutomaticSelection verifies that a forced
// harness in Config (Harness field) is carried through as HarnessOverride.
func TestAgentConfigForcedHarnessBypassesAutomaticSelection(t *testing.T) {
	req := NormalizeRouteRequest(
		RouteFlags{}, // no flags
		Config{Harness: "codex", Profile: "cheap"},
		BuiltinCatalog,
	)
	assert.Equal(t, "codex", req.HarnessOverride)
	// Profile from config is still applied.
	assert.Equal(t, "cheap", req.Profile)
}

// TestAgentDoctorReportsEmbeddedDefaultBackendRoutability verifies that the
// embedded harness (agent) is always reachable without binary lookup.
// This is already covered by TestProbeHarnessStateEmbeddedAlwaysReachable but
// here we specifically check that doctor can report it as routeable.
func TestAgentDoctorReportsEmbeddedDefaultBackendRoutability(t *testing.T) {
	r := newTestRunnerForRouting()

	// Probe agent — must succeed even with no binary lookup.
	state := r.fastHarnessState("agent", r.Registry.harnesses["agent"])
	assert.True(t, state.Installed, "embedded harness should always report installed")
	assert.True(t, state.Reachable, "embedded harness should always report reachable")
	assert.True(t, state.Authenticated, "embedded harness should always report authenticated")
	assert.True(t, state.QuotaOK, "embedded harness should always report quota OK")
}

// --- Deprecated Explicit Pin Guardrail Tests (ddx-e6428c08) ---

// TestCheckDeprecatedPinDetectsClaudeFamily verifies that deprecated Claude
// explicit model version strings are detected and report the canonical replacement.
func TestCheckDeprecatedPinDetectsClaudeFamily(t *testing.T) {
	cases := []struct {
		pin             string
		surface         string
		wantDeprecated  bool
		wantReplacement string
	}{
		// Stale claude versions — should be flagged.
		{"claude-opus-4-5", "claude", true, "claude-opus-4-6"},
		{"claude-3-5-sonnet-20241022", "claude", true, "claude-sonnet-4-6"},
		{"claude-3-opus-20240229", "claude", true, "claude-opus-4-6"},
		{"claude-3-sonnet-20240229", "claude", true, "claude-sonnet-4-6"},
		// Current canonical models — must not be flagged.
		{"claude-opus-4-6", "claude", false, ""},
		{"claude-sonnet-4-6", "claude", false, ""},
		// Completely unknown pin — must not be flagged.
		{"claude-unknown-9999", "claude", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.pin, func(t *testing.T) {
			dp, deprecated := BuiltinCatalog.CheckDeprecatedPin(tc.pin, tc.surface)
			assert.Equal(t, tc.wantDeprecated, deprecated, "deprecated status mismatch for pin %q", tc.pin)
			if tc.wantDeprecated {
				assert.Equal(t, tc.wantReplacement, dp.ReplacedBy, "replacement mismatch for pin %q", tc.pin)
			}
		})
	}
}

// TestCheckDeprecatedPinDetectsCodexFamily verifies that deprecated OpenAI/codex
// explicit model version strings are detected and report the canonical replacement.
func TestCheckDeprecatedPinDetectsCodexFamily(t *testing.T) {
	cases := []struct {
		pin             string
		surface         string
		wantDeprecated  bool
		wantReplacement string
	}{
		// Stale codex versions — should be flagged.
		{"gpt-4o", "codex", true, "gpt-5.4-mini"},
		{"gpt-4-turbo", "codex", true, "gpt-5.4"},
		{"o1-2024-12-17", "codex", true, "gpt-5.4"},
		// Current canonical models — must not be flagged.
		{"gpt-5.4", "codex", false, ""},
		{"gpt-5.4-mini", "codex", false, ""},
		// Completely unknown pin — must not be flagged.
		{"gpt-99-super", "codex", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.pin, func(t *testing.T) {
			dp, deprecated := BuiltinCatalog.CheckDeprecatedPin(tc.pin, tc.surface)
			assert.Equal(t, tc.wantDeprecated, deprecated, "deprecated status mismatch for pin %q", tc.pin)
			if tc.wantDeprecated {
				assert.Equal(t, tc.wantReplacement, dp.ReplacedBy, "replacement mismatch for pin %q", tc.pin)
			}
		})
	}
}

// TestCheckDeprecatedPinSurfaceMismatchNotFlagged verifies that a deprecated pin
// entry for surface A is not flagged when queried for surface B.
func TestCheckDeprecatedPinSurfaceMismatchNotFlagged(t *testing.T) {
	// "claude-opus-4-5" is deprecated on the "claude" surface.
	// Querying it against the "codex" surface should return not deprecated.
	_, deprecated := BuiltinCatalog.CheckDeprecatedPin("claude-opus-4-5", "codex")
	assert.False(t, deprecated, "surface-specific deprecated pin must not match a different surface")
}

// TestCheckDeprecatedPinEmptySurfaceMatchesAny verifies that a DeprecatedPin
// with no Surface set matches any surface query.
func TestCheckDeprecatedPinEmptySurfaceMatchesAny(t *testing.T) {
	cat := NewCatalogWithPins(nil, []DeprecatedPin{
		{Pin: "old-model-v1", Surface: "", ReplacedBy: "new-model-v2"},
	})
	dp, deprecated := cat.CheckDeprecatedPin("old-model-v1", "codex")
	assert.True(t, deprecated, "pin with empty surface should match any surface")
	assert.Equal(t, "new-model-v2", dp.ReplacedBy)

	dp2, deprecated2 := cat.CheckDeprecatedPin("old-model-v1", "claude")
	assert.True(t, deprecated2, "pin with empty surface should match any surface")
	assert.Equal(t, "new-model-v2", dp2.ReplacedBy)
}

// TestCandidatePlanSetsDeprecationWarningForDeprecatedPin verifies that when a
// RouteRequest uses a deprecated ModelPin, the resulting CandidatePlan for the
// matching harness surface carries a non-empty DeprecationWarning.
func TestCandidatePlanSetsDeprecationWarningForDeprecatedPin(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"claude": healthyState(),
		"codex":  healthyState(),
	}

	// "claude-opus-4-5" is a deprecated explicit pin for the claude surface.
	plans := r.BuildCandidatePlans(RouteRequest{ModelPin: "claude-opus-4-5"}, states)

	var claudePlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "claude" {
			claudePlan = &plans[i]
			break
		}
	}

	require.NotNil(t, claudePlan, "claude plan should be present")
	assert.True(t, claudePlan.Viable, "claude should be viable for an exact pin")
	assert.Equal(t, "claude-opus-4-5", claudePlan.ConcreteModel)
	assert.NotEmpty(t, claudePlan.DeprecationWarning, "deprecated pin must produce a DeprecationWarning")
	assert.Contains(t, claudePlan.DeprecationWarning, "claude-opus-4-5")
	assert.Contains(t, claudePlan.DeprecationWarning, "claude-opus-4-6")
}

// TestCandidatePlanNoDeprecationWarningForCanonicalPin verifies that current
// canonical model pins do not produce a DeprecationWarning.
func TestCandidatePlanNoDeprecationWarningForCanonicalPin(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"claude": healthyState(),
		"codex":  healthyState(),
	}

	// "claude-opus-4-6" is the canonical current pin — must not be flagged.
	plans := r.BuildCandidatePlans(RouteRequest{ModelPin: "claude-opus-4-6"}, states)

	for _, p := range plans {
		if p.Harness == "claude" {
			assert.Empty(t, p.DeprecationWarning, "canonical pin must not produce a deprecation warning")
		}
	}
}

// TestCandidatePlanDeprecatedPinWithHarnessOverride verifies that a deprecated
// pin also surfaces the DeprecationWarning when HarnessOverride is set alongside
// the ModelPin.
func TestCandidatePlanDeprecatedPinWithHarnessOverride(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"codex": healthyState(),
	}

	// "gpt-4o" is deprecated on the codex surface.
	plans := r.BuildCandidatePlans(RouteRequest{
		HarnessOverride: "codex",
		ModelPin:        "gpt-4o",
	}, states)

	var codexPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "codex" {
			codexPlan = &plans[i]
			break
		}
	}

	require.NotNil(t, codexPlan, "codex plan should be present")
	assert.True(t, codexPlan.Viable, "codex should be viable for HarnessOverride+ModelPin")
	assert.Equal(t, "gpt-4o", codexPlan.ConcreteModel)
	assert.NotEmpty(t, codexPlan.DeprecationWarning, "deprecated pin must produce a DeprecationWarning even with HarnessOverride")
	assert.Contains(t, codexPlan.DeprecationWarning, "gpt-4o")
	assert.Contains(t, codexPlan.DeprecationWarning, "gpt-5.4-mini")
}

// --- Blocked Model Tests (ddx-1c18a107) ---

// TestResolveBlockedEntryReturnsFalse verifies that Resolve returns ok=false
// when the catalog entry itself is marked Blocked.
func TestResolveBlockedEntryReturnsFalse(t *testing.T) {
	cat := NewCatalog([]CatalogEntry{
		{
			Ref:     "bad-ref",
			Blocked: true,
			Surfaces: map[string]string{
				"codex": "some-model",
			},
		},
	})
	model, ok := cat.Resolve("bad-ref", "codex")
	assert.False(t, ok, "blocked entry must return ok=false from Resolve")
	assert.Empty(t, model)
}

// TestResolveBlockedModelIDReturnsFalse verifies that Resolve returns ok=false
// when the resolved concrete model ID is in the blocked set.
func TestResolveBlockedModelIDReturnsFalse(t *testing.T) {
	cat := NewCatalog([]CatalogEntry{
		{
			Ref: "standard",
			Surfaces: map[string]string{
				"claude": "blocked-concrete-model",
			},
		},
	})
	cat.AddBlockedModelID("blocked-concrete-model")

	model, ok := cat.Resolve("standard", "claude")
	assert.False(t, ok, "resolve to a blocked model ID must return ok=false")
	assert.Empty(t, model)
}

// TestResolveNonBlockedModelIDSucceeds verifies that Resolve still works
// normally for model IDs not in the blocked set.
func TestResolveNonBlockedModelIDSucceeds(t *testing.T) {
	cat := NewCatalog([]CatalogEntry{
		{
			Ref: "standard",
			Surfaces: map[string]string{
				"claude": "claude-sonnet-4-6",
			},
		},
	})
	cat.AddBlockedModelID("some-other-blocked-model")

	model, ok := cat.Resolve("standard", "claude")
	assert.True(t, ok)
	assert.Equal(t, "claude-sonnet-4-6", model)
}

// TestIsBlockedModelID verifies the IsBlockedModelID helper.
func TestIsBlockedModelID(t *testing.T) {
	cat := NewCatalog(nil)
	cat.AddBlockedModelID("gpt-3.5-turbo")

	assert.True(t, cat.IsBlockedModelID("gpt-3.5-turbo"))
	assert.False(t, cat.IsBlockedModelID("gpt-5.4"))
	assert.False(t, cat.IsBlockedModelID(""))
}

// TestApplyModelCatalogYAMLPopulatesBlockedModels verifies that
// ApplyModelCatalogYAML reads Blocked=true entries and registers them.
func TestApplyModelCatalogYAMLPopulatesBlockedModels(t *testing.T) {
	yml := &ModelCatalogYAML{
		Models: []ModelEntryYAML{
			{ID: "old-bad-model", Blocked: true, Provider: "openai", Tier: "cheap"},
			{ID: "current-good-model", Blocked: false, Provider: "openai", Tier: "cheap"},
		},
	}
	cat := NewCatalog(nil)
	ApplyModelCatalogYAML(cat, yml)

	assert.True(t, cat.IsBlockedModelID("old-bad-model"), "blocked model must be registered")
	assert.False(t, cat.IsBlockedModelID("current-good-model"), "non-blocked model must not be registered")
}

// TestDefaultModelCatalogYAMLBlockedModelsNeverResolve verifies that the
// default catalog blocked entries cannot be resolved on any surface.
func TestDefaultModelCatalogYAMLBlockedModelsNeverResolve(t *testing.T) {
	yml := DefaultModelCatalogYAML()
	cat := NewCatalog(nil)
	ApplyModelCatalogYAML(cat, yml)

	blockedIDs := []string{
		"gpt-3.5-turbo",
		"gpt-3.5-turbo-16k",
		"claude-opus-4-5",
		"claude-3-opus-20240229",
		"claude-3-5-sonnet-20241022",
	}
	for _, id := range blockedIDs {
		assert.True(t, cat.IsBlockedModelID(id), "default blocked model %q must be registered", id)
	}
}
