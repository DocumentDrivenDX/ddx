package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestResolveMaxFamilyExpansion_DefaultWhenUnsetAndNonPositive(t *testing.T) {
	t.Parallel()

	// Unset: nil config / empty agent.triage.
	assert.Equal(t, DefaultMaxFamilyExpansion, (*NewConfig)(nil).ResolveMaxFamilyExpansion())
	assert.Equal(t, DefaultMaxFamilyExpansion, (&NewConfig{}).ResolveMaxFamilyExpansion())
	assert.Equal(t, DefaultMaxFamilyExpansion, (&NewConfig{Agent: &AgentConfig{}}).ResolveMaxFamilyExpansion())
	assert.Equal(t, DefaultMaxFamilyExpansion, (&NewConfig{
		Agent: &AgentConfig{Triage: &TriageConfig{}},
	}).ResolveMaxFamilyExpansion())

	// Non-positive values resolve to the conservative default.
	for _, v := range []int{0, -1, -99} {
		v := v
		cfg := &NewConfig{
			Agent: &AgentConfig{Triage: &TriageConfig{MaxFamilyExpansion: &v}},
		}
		assert.Equal(t, DefaultMaxFamilyExpansion, cfg.ResolveMaxFamilyExpansion(),
			"non-positive %d must resolve to default %d", v, DefaultMaxFamilyExpansion)
	}

	// Explicit positive value is honored.
	eight := 12
	cfg := &NewConfig{
		Agent: &AgentConfig{Triage: &TriageConfig{MaxFamilyExpansion: &eight}},
	}
	assert.Equal(t, 12, cfg.ResolveMaxFamilyExpansion())
}

func TestResolvedConfig_MaxFamilyExpansionAndDecompositionPolicy(t *testing.T) {
	t.Parallel()

	// Defaults via Resolve.
	rcfg := (&NewConfig{}).Resolve(CLIOverrides{})
	assert.Equal(t, DefaultMaxFamilyExpansion, rcfg.MaxFamilyExpansion())
	assert.Equal(t, DefaultMaxDecompositionDepth, rcfg.MaxDecompositionDepth())
	policy := rcfg.DecompositionPolicy()
	assert.Equal(t, DefaultMaxFamilyExpansion, policy.MaxFamilyExpansion)
	assert.Equal(t, DefaultMaxDecompositionDepth, policy.MaxDepth)

	// Explicit values flow through Resolve and DecompositionPolicy together.
	depth := 2
	budget := 5
	cfg := &NewConfig{
		Agent: &AgentConfig{
			Triage: &TriageConfig{
				MaxDecompositionDepth: &depth,
				MaxFamilyExpansion:    &budget,
			},
		},
	}
	rcfg = cfg.Resolve(CLIOverrides{})
	assert.Equal(t, 2, rcfg.MaxDecompositionDepth())
	assert.Equal(t, 5, rcfg.MaxFamilyExpansion())
	assert.Equal(t, DecompositionPolicy{MaxDepth: 2, MaxFamilyExpansion: 5}, rcfg.DecompositionPolicy())
}

func TestNewConfigParsesMaxFamilyExpansion(t *testing.T) {
	t.Parallel()

	raw := `
version: "1.0"
agent:
  triage:
    max_decomposition_depth: 2
    max_family_expansion: 6
`
	var cfg NewConfig
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))
	require.NotNil(t, cfg.Agent)
	require.NotNil(t, cfg.Agent.Triage)
	require.NotNil(t, cfg.Agent.Triage.MaxFamilyExpansion)
	assert.Equal(t, 6, *cfg.Agent.Triage.MaxFamilyExpansion)
	assert.Equal(t, 6, cfg.ResolveMaxFamilyExpansion())
	assert.Equal(t, 2, cfg.ResolveMaxDecompositionDepth())
}

func TestSchemaAcceptsMaxFamilyExpansion(t *testing.T) {
	t.Parallel()

	v, err := NewValidator()
	require.NoError(t, err)
	require.NoError(t, v.Validate([]byte(`version: "1.0"
agent:
  triage:
    max_decomposition_depth: 2
    max_family_expansion: 8
`)))
}
