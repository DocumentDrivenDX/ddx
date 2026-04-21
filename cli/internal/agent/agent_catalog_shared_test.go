package agent

import (
	"context"
	"fmt"
	"testing"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/stretchr/testify/assert"
)

// catalogStubAgent is a minimal DdxAgent stub for catalog tests.
type catalogStubAgent struct {
	profiles []agentlib.ProfileInfo
	resolved map[string]*agentlib.ResolvedProfile
}

func (s *catalogStubAgent) ListProfiles(_ context.Context) ([]agentlib.ProfileInfo, error) {
	return s.profiles, nil
}

func (s *catalogStubAgent) ResolveProfile(_ context.Context, name string) (*agentlib.ResolvedProfile, error) {
	r, ok := s.resolved[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", name)
	}
	return r, nil
}

func (s *catalogStubAgent) ProfileAliases(_ context.Context) (map[string]string, error) {
	return nil, nil
}

func (s *catalogStubAgent) Execute(_ context.Context, _ agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *catalogStubAgent) TailSessionLog(_ context.Context, _ string) (<-chan agentlib.ServiceEvent, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *catalogStubAgent) ListHarnesses(_ context.Context) ([]agentlib.HarnessInfo, error) {
	return nil, nil
}
func (s *catalogStubAgent) ListProviders(_ context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}
func (s *catalogStubAgent) ListModels(_ context.Context, _ agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	return nil, nil
}
func (s *catalogStubAgent) HealthCheck(_ context.Context, _ agentlib.HealthTarget) error {
	return fmt.Errorf("not implemented")
}
func (s *catalogStubAgent) ResolveRoute(_ context.Context, _ agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *catalogStubAgent) RecordRouteAttempt(_ context.Context, _ agentlib.RouteAttempt) error {
	return nil
}
func (s *catalogStubAgent) RouteStatus(_ context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}

// testSurfaces builds a []ProfileSurface using the surface names the agentlib service emits.
func codeHighSurfaces() []agentlib.ProfileSurface {
	return []agentlib.ProfileSurface{
		{Name: "native-openai", Harness: "agent", Model: "gpt-5.4"},
		{Name: "native-anthropic", Harness: "agent", Model: "opus-4.6"},
		{Name: "claude", Harness: "claude", Model: "opus-4.6"},
		{Name: "codex", Harness: "codex", Model: "gpt-5.4"},
	}
}

func codeMediumSurfaces() []agentlib.ProfileSurface {
	return []agentlib.ProfileSurface{
		{Name: "native-openai", Harness: "agent", Model: "gpt-5.4-mini"},
		{Name: "native-anthropic", Harness: "agent", Model: "sonnet-4.6"},
		{Name: "claude", Harness: "claude", Model: "sonnet-4.6"},
		{Name: "codex", Harness: "codex", Model: "gpt-5.4-mini"},
	}
}

func codeEconomySurfaces() []agentlib.ProfileSurface {
	return []agentlib.ProfileSurface{
		{Name: "native-openai", Harness: "agent", Model: "qwen3.5-27b"},
		{Name: "native-anthropic", Harness: "agent", Model: "haiku-5.5"},
		{Name: "claude", Harness: "claude", Model: "haiku-5.5"},
	}
}

// newFullStub returns a stub with all tier profiles, aliases, and a deprecated entry.
func newFullStub() *catalogStubAgent {
	stub := &catalogStubAgent{
		profiles: []agentlib.ProfileInfo{
			{Name: "code-high", Target: "code-high"},
			{Name: "smart", Target: "code-high", AliasOf: "code-high"},
			{Name: "code-medium", Target: "code-medium"},
			{Name: "standard", Target: "code-medium", AliasOf: "code-medium"},
			{Name: "code-economy", Target: "code-economy"},
			{Name: "cheap", Target: "code-economy", AliasOf: "code-economy"},
			{Name: "high", Target: "code-high", AliasOf: "code-high"},
			{Name: "medium", Target: "code-medium", AliasOf: "code-medium"},
			{Name: "economy", Target: "code-economy", AliasOf: "code-economy"},
			{Name: "claude-sonnet-4", Target: "code-medium", AliasOf: "code-medium", Deprecated: true, Replacement: "code-medium"},
		},
		resolved: map[string]*agentlib.ResolvedProfile{
			"code-high":    {Name: "code-high", Target: "code-high", Surfaces: codeHighSurfaces()},
			"smart":        {Name: "smart", Target: "code-high", Surfaces: codeHighSurfaces()},
			"code-medium":  {Name: "code-medium", Target: "code-medium", Surfaces: codeMediumSurfaces()},
			"standard":     {Name: "standard", Target: "code-medium", Surfaces: codeMediumSurfaces()},
			"code-economy": {Name: "code-economy", Target: "code-economy", Surfaces: codeEconomySurfaces()},
			"cheap":        {Name: "cheap", Target: "code-economy", Surfaces: codeEconomySurfaces()},
			"high":         {Name: "high", Target: "code-high", Surfaces: codeHighSurfaces()},
			"medium":       {Name: "medium", Target: "code-medium", Surfaces: codeMediumSurfaces()},
			"economy":      {Name: "economy", Target: "code-economy", Surfaces: codeEconomySurfaces()},
			"claude-sonnet-4": {
				Name:        "claude-sonnet-4",
				Target:      "code-medium",
				Deprecated:  true,
				Replacement: "code-medium",
				Surfaces: []agentlib.ProfileSurface{
					{Name: "native-openai", Harness: "agent", Model: "anthropic/claude-sonnet-4"},
					{Name: "claude", Harness: "claude", Model: "claude-sonnet-4-20250514"},
				},
			},
		},
	}
	return stub
}

// --- ApplyCatalogFromService: nil safety ---

func TestApplyCatalogFromServiceNilSafe(t *testing.T) {
	ApplyCatalogFromService(context.Background(), nil, nil)
	ApplyCatalogFromService(context.Background(), NewCatalog(nil), nil)
}

// --- profile/alias/target population ---

func TestApplyCatalogFromServiceProfilesAdded(t *testing.T) {
	cat := NewCatalog(nil)
	ApplyCatalogFromService(context.Background(), cat, newFullStub())

	for _, name := range []string{"code-high", "smart", "code-medium", "standard", "code-economy", "cheap"} {
		assert.True(t, cat.KnownOnAnySurface(name), "profile %q must be in catalog", name)
	}
}

func TestApplyCatalogFromServiceAliasesAdded(t *testing.T) {
	cat := NewCatalog(nil)
	ApplyCatalogFromService(context.Background(), cat, newFullStub())

	for _, alias := range []string{"high", "medium", "economy"} {
		assert.True(t, cat.KnownOnAnySurface(alias), "alias %q must be in catalog", alias)
	}
}

// --- surface translation ---

func TestApplyCatalogFromServiceSurfaceTranslation(t *testing.T) {
	cat := NewCatalog(nil)
	ApplyCatalogFromService(context.Background(), cat, newFullStub())

	// claude surface
	model, ok := cat.Resolve("code-high", "claude")
	assert.True(t, ok)
	assert.Equal(t, "opus-4.6", model)

	// native-openai → embedded-openai (wins over native-anthropic)
	model, ok = cat.Resolve("code-high", "embedded-openai")
	assert.True(t, ok)
	assert.Equal(t, "gpt-5.4", model, "native-openai must map to embedded-openai")

	// codex surface
	model, ok = cat.Resolve("code-high", "codex")
	assert.True(t, ok)
	assert.Equal(t, "gpt-5.4", model)
}

func TestApplyCatalogFromServiceNativeAnthropicNotMappedToEmbeddedOpenAI(t *testing.T) {
	// native-anthropic must not map to embedded-openai; native-openai wins.
	cat := NewCatalog(nil)
	ApplyCatalogFromService(context.Background(), cat, newFullStub())

	model, ok := cat.Resolve("code-high", "embedded-openai")
	assert.True(t, ok)
	assert.Equal(t, "gpt-5.4", model)
	assert.NotEqual(t, "opus-4.6", model, "native-anthropic must not set embedded-openai")
}

// --- tier coverage ---

func TestApplyCatalogFromServiceCodeHighMediumEconomy(t *testing.T) {
	cat := NewCatalog(nil)
	ApplyCatalogFromService(context.Background(), cat, newFullStub())

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

func TestApplyCatalogFromServiceProfileMatchesTier(t *testing.T) {
	cat := NewCatalog(nil)
	ApplyCatalogFromService(context.Background(), cat, newFullStub())

	codeHigh, _ := cat.Resolve("code-high", "claude")
	smart, ok := cat.Resolve("smart", "claude")
	assert.True(t, ok)
	assert.Equal(t, codeHigh, smart)

	codeMedium, _ := cat.Resolve("code-medium", "embedded-openai")
	standard, ok := cat.Resolve("standard", "embedded-openai")
	assert.True(t, ok)
	assert.Equal(t, codeMedium, standard)

	codeEconomy, _ := cat.Resolve("code-economy", "embedded-openai")
	cheap, ok := cat.Resolve("cheap", "embedded-openai")
	assert.True(t, ok)
	assert.Equal(t, codeEconomy, cheap)
}

// --- deprecated handling ---

func TestApplyCatalogFromServiceDeprecatedEntryAdded(t *testing.T) {
	cat := NewCatalog(nil)
	ApplyCatalogFromService(context.Background(), cat, newFullStub())

	entry, ok := cat.Entry("claude-sonnet-4")
	assert.True(t, ok, "deprecated entry must be in catalog")
	assert.True(t, entry.Deprecated)
	assert.Equal(t, "code-medium", entry.ReplacedBy)
}
