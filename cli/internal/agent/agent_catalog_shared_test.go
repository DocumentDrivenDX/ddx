package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSharedManifestV3 is a representative version-3 ddx-agent catalog manifest.
const testSharedManifestV3 = `
version: 3
generated_at: 2026-04-12T00:00:00Z
catalog_version: 2026-04-12.2

profiles:
  code-high:
    target: code-high
  smart:
    target: code-high
  code-medium:
    target: code-medium
  standard:
    target: code-medium
  code-economy:
    target: code-economy
  cheap:
    target: code-economy

targets:
  code-high:
    family: coding-tier
    aliases:
      - high
    status: active
    surfaces:
      agent.anthropic: opus-4.6
      claude-code: opus-4.6
      agent.openai: gpt-5.4
      codex: gpt-5.4

  code-medium:
    family: coding-tier
    aliases:
      - medium
    status: active
    surfaces:
      agent.anthropic: sonnet-4.6
      claude-code: sonnet-4.6
      agent.openai: gpt-5.4-mini
      codex: gpt-5.4-mini

  code-economy:
    family: coding-tier
    aliases:
      - economy
    status: active
    surfaces:
      agent.anthropic: haiku-5.5
      claude-code: haiku-5.5
      agent.openai: qwen3.5-27b

  claude-sonnet-4:
    family: claude-sonnet
    status: deprecated
    replacement: code-medium
    surfaces:
      agent.anthropic: claude-sonnet-4-20250514
      agent.openai: anthropic/claude-sonnet-4
`

// testSharedManifestV2 is a minimal version-2 manifest (earlier schema).
const testSharedManifestV2 = `
version: 2
generated_at: 2026-04-10T00:00:00Z
catalog_version: 2026-04-11.1
profiles:
  code-high:
    target: code-high
targets:
  code-high:
    family: coding-tier
    status: active
    surfaces:
      agent.openai: gpt-5.4
`

// writeSharedManifest writes content to a temp file and returns its path.
func writeSharedManifest(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "models.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// --- LoadSharedCatalog ---

func TestLoadSharedCatalogMissingFile(t *testing.T) {
	m, err := LoadSharedCatalog("/tmp/nonexistent-shared-catalog-xyz-abc.yaml")
	require.NoError(t, err)
	assert.Nil(t, m, "missing file must return nil with no error")
}

func TestLoadSharedCatalogValidV3(t *testing.T) {
	path := writeSharedManifest(t, testSharedManifestV3)
	m, err := LoadSharedCatalog(path)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, 3, m.Version)
	assert.Equal(t, "2026-04-12.2", m.CatalogVersion)
	assert.Contains(t, m.Profiles, "code-high")
	assert.Contains(t, m.Profiles, "smart")
	assert.Contains(t, m.Targets, "code-high")
}

func TestLoadSharedCatalogValidV2(t *testing.T) {
	path := writeSharedManifest(t, testSharedManifestV2)
	m, err := LoadSharedCatalog(path)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, 2, m.Version)
}

func TestLoadSharedCatalogUnsupportedVersion(t *testing.T) {
	content := "version: 99\nprofiles: {}\ntargets:\n  x:\n    family: f\n    status: active\n    surfaces:\n      agent.openai: m\n"
	path := writeSharedManifest(t, content)
	_, err := LoadSharedCatalog(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported schema version")
}

func TestLoadSharedCatalogMalformedYAML(t *testing.T) {
	path := writeSharedManifest(t, "not: valid: yaml: {{{{{")
	_, err := LoadSharedCatalog(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

// --- ApplySharedCatalog: profile/target/alias population ---

func TestApplySharedCatalogProfilesAdded(t *testing.T) {
	path := writeSharedManifest(t, testSharedManifestV3)
	m, err := LoadSharedCatalog(path)
	require.NoError(t, err)

	cat := NewCatalog(nil)
	ApplySharedCatalog(cat, m)

	for _, name := range []string{"code-high", "smart", "code-medium", "standard", "code-economy", "cheap"} {
		assert.True(t, cat.KnownOnAnySurface(name), "profile %q must be in catalog", name)
	}
}

func TestApplySharedCatalogTargetIDsAdded(t *testing.T) {
	path := writeSharedManifest(t, testSharedManifestV3)
	m, err := LoadSharedCatalog(path)
	require.NoError(t, err)

	cat := NewCatalog(nil)
	ApplySharedCatalog(cat, m)

	for _, name := range []string{"code-high", "code-medium", "code-economy"} {
		assert.True(t, cat.KnownOnAnySurface(name), "target %q must be in catalog", name)
	}
}

func TestApplySharedCatalogAliasesAdded(t *testing.T) {
	path := writeSharedManifest(t, testSharedManifestV3)
	m, err := LoadSharedCatalog(path)
	require.NoError(t, err)

	cat := NewCatalog(nil)
	ApplySharedCatalog(cat, m)

	for _, alias := range []string{"high", "medium", "economy"} {
		assert.True(t, cat.KnownOnAnySurface(alias), "alias %q must be in catalog", alias)
	}
}

// --- ApplySharedCatalog: surface translation ---

func TestApplySharedCatalogSurfaceTranslation(t *testing.T) {
	path := writeSharedManifest(t, testSharedManifestV3)
	m, err := LoadSharedCatalog(path)
	require.NoError(t, err)

	cat := NewCatalog(nil)
	ApplySharedCatalog(cat, m)

	// claude-code → claude
	model, ok := cat.Resolve("code-high", "claude")
	assert.True(t, ok)
	assert.Equal(t, "opus-4.6", model)

	// agent.openai → embedded-openai (wins over agent.anthropic)
	model, ok = cat.Resolve("code-high", "embedded-openai")
	assert.True(t, ok)
	assert.Equal(t, "gpt-5.4", model, "agent.openai must win for embedded-openai surface")

	// codex → codex
	model, ok = cat.Resolve("code-high", "codex")
	assert.True(t, ok)
	assert.Equal(t, "gpt-5.4", model)
}

// --- Acceptance criteria: code-high/code-medium/code-economy tier coverage ---

func TestApplySharedCatalogCodeHighMediumEconomy(t *testing.T) {
	path := writeSharedManifest(t, testSharedManifestV3)
	m, err := LoadSharedCatalog(path)
	require.NoError(t, err)

	cat := NewCatalog(nil)
	ApplySharedCatalog(cat, m)

	cases := []struct {
		tier    string
		surface string
		want    string
	}{
		{"code-high", "embedded-openai", "gpt-5.4"},
		{"code-high", "claude", "opus-4.6"},
		{"code-high", "codex", "gpt-5.4"},
		{"code-medium", "embedded-openai", "gpt-5.4-mini"},
		{"code-medium", "claude", "sonnet-4.6"},
		{"code-medium", "codex", "gpt-5.4-mini"},
		{"code-economy", "embedded-openai", "qwen3.5-27b"},
		{"code-economy", "claude", "haiku-5.5"},
	}
	for _, tc := range cases {
		model, ok := cat.Resolve(tc.tier, tc.surface)
		assert.True(t, ok, "must resolve %s on %s", tc.tier, tc.surface)
		assert.Equal(t, tc.want, model, "tier=%s surface=%s", tc.tier, tc.surface)
	}
}

func TestApplySharedCatalogProfileMatchesTier(t *testing.T) {
	path := writeSharedManifest(t, testSharedManifestV3)
	m, err := LoadSharedCatalog(path)
	require.NoError(t, err)

	cat := NewCatalog(nil)
	ApplySharedCatalog(cat, m)

	// "smart" → code-high — same surface mappings.
	codeHigh, _ := cat.Resolve("code-high", "claude")
	smart, ok := cat.Resolve("smart", "claude")
	assert.True(t, ok, "smart profile must be resolvable")
	assert.Equal(t, codeHigh, smart, "smart must resolve same as code-high")

	// "standard" → code-medium
	codeMedium, _ := cat.Resolve("code-medium", "embedded-openai")
	standard, ok := cat.Resolve("standard", "embedded-openai")
	assert.True(t, ok, "standard profile must be resolvable")
	assert.Equal(t, codeMedium, standard, "standard must resolve same as code-medium")

	// "cheap" → code-economy
	codeEconomy, _ := cat.Resolve("code-economy", "embedded-openai")
	cheap, ok := cat.Resolve("cheap", "embedded-openai")
	assert.True(t, ok, "cheap profile must be resolvable")
	assert.Equal(t, codeEconomy, cheap, "cheap must resolve same as code-economy")
}

// --- Deprecated target handling ---

func TestApplySharedCatalogDeprecatedTargetAdded(t *testing.T) {
	path := writeSharedManifest(t, testSharedManifestV3)
	m, err := LoadSharedCatalog(path)
	require.NoError(t, err)

	cat := NewCatalog(nil)
	ApplySharedCatalog(cat, m)

	entry, ok := cat.Entry("claude-sonnet-4")
	require.True(t, ok, "deprecated target must be in catalog")
	assert.True(t, entry.Deprecated)
	assert.Equal(t, "code-medium", entry.ReplacedBy)
}

// --- agent.anthropic boundary ---

func TestApplySharedCatalogAgentAnthropicNotMappedToOwnSurface(t *testing.T) {
	// agent.anthropic is excluded from DDx surface translation. The embedded
	// agent harness uses "embedded-openai" surface in DDx; agent.openai wins.
	path := writeSharedManifest(t, testSharedManifestV3)
	m, err := LoadSharedCatalog(path)
	require.NoError(t, err)

	cat := NewCatalog(nil)
	ApplySharedCatalog(cat, m)

	// code-high → embedded-openai must be gpt-5.4 (agent.openai), not opus-4.6 (agent.anthropic).
	model, ok := cat.Resolve("code-high", "embedded-openai")
	assert.True(t, ok)
	assert.Equal(t, "gpt-5.4", model)
	assert.NotEqual(t, "opus-4.6", model, "agent.anthropic must not override embedded-openai surface")
}

// --- Nil safety ---

func TestApplySharedCatalogNilSafe(t *testing.T) {
	// Must not panic on nil inputs.
	ApplySharedCatalog(nil, nil)
	ApplySharedCatalog(NewCatalog(nil), nil)
}

// --- Integration: routing with shared catalog ---

func TestSharedCatalogAllProfilesRoutable(t *testing.T) {
	// All tier profiles from the shared manifest must produce at least one viable
	// candidate when the catalog is applied to a runner.
	path := writeSharedManifest(t, testSharedManifestV3)
	m, err := LoadSharedCatalog(path)
	require.NoError(t, err)

	cat := buildBuiltinCatalog()
	ApplySharedCatalog(cat, m)

	r := newTestRunnerForRouting()
	r.Catalog = cat

	states := map[string]HarnessState{
		"codex":  healthyState(),
		"agent":  healthyLocalState(),
		"claude": healthyState(),
	}

	for _, profile := range []string{"code-high", "code-medium", "code-economy", "smart", "standard", "cheap"} {
		plans := r.BuildCandidatePlans(RouteRequest{Profile: profile}, states)
		viable := false
		for _, p := range plans {
			if p.Viable {
				viable = true
				break
			}
		}
		assert.True(t, viable, "profile %q must have at least one viable candidate", profile)
	}
}

func TestSharedCatalogCodeHighRoutesToClaudeAndAgent(t *testing.T) {
	// code-high must route to both claude (via claude-code surface) and agent
	// (via agent.openai surface) when both are healthy.
	path := writeSharedManifest(t, testSharedManifestV3)
	m, err := LoadSharedCatalog(path)
	require.NoError(t, err)

	cat := buildBuiltinCatalog()
	ApplySharedCatalog(cat, m)

	r := newTestRunnerForRouting()
	r.Catalog = cat

	states := map[string]HarnessState{
		"claude": healthyState(),
		"agent":  healthyLocalState(),
		"codex":  healthyState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "code-high"}, states)

	viableHarnesses := map[string]bool{}
	for _, p := range plans {
		if p.Viable {
			viableHarnesses[p.Harness] = true
		}
	}
	assert.True(t, viableHarnesses["claude"], "claude must be viable for code-high")
	assert.True(t, viableHarnesses["agent"], "agent must be viable for code-high")
	assert.True(t, viableHarnesses["codex"], "codex must be viable for code-high")
}
