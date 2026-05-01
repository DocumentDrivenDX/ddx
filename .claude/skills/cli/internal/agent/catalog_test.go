package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestCatalogCloneIsIndependent(t *testing.T) {
	cat := NewCatalogWithPins([]CatalogEntry{
		{Ref: "cheap", Surfaces: map[string]string{"codex": "gpt-5.4-mini"}},
	}, []DeprecatedPin{
		{Pin: "gpt-4o", Surface: "codex", ReplacedBy: "gpt-5.4-mini"},
	})
	cat.AddBlockedModelID("blocked-model")

	clone := cat.Clone()
	clone.AddOrReplace(CatalogEntry{Ref: "cheap", Surfaces: map[string]string{"claude": "haiku-5.5"}})
	clone.deprecatedPins["gpt-4o"] = DeprecatedPin{Pin: "gpt-4o", Surface: "codex", ReplacedBy: "gpt-5.4"}
	clone.AddBlockedModelID("new-blocked-model")

	model, ok := cat.Resolve("cheap", "codex")
	assert.True(t, ok)
	assert.Equal(t, "gpt-5.4-mini", model)

	_, ok = cat.Resolve("cheap", "claude")
	assert.False(t, ok)

	pin, ok := cat.CheckDeprecatedPin("gpt-4o", "codex")
	assert.True(t, ok)
	assert.Equal(t, "gpt-5.4-mini", pin.ReplacedBy)

	assert.True(t, cat.IsBlockedModelID("blocked-model"))
	assert.False(t, cat.IsBlockedModelID("new-blocked-model"))
}
