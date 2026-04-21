package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestAgentConfigParsesProfileLaddersAndOverrides covers the
// ddx-7955af22 / ddx-bbb65768 acceptance: config schema supports
// profile_ladders (per-profile ordered tier lists) + model_overrides.
func TestAgentConfigParsesProfileLaddersAndOverrides(t *testing.T) {
	raw := `
harness: claude
routing:
  profile_ladders:
    default: [cheap, standard, smart]
    cheap: [cheap]
    fast: [fast, smart]
    smart: [smart]
  model_overrides:
    cheap: qwen/qwen3.6
    fast: kimi/k2.5
    standard: codex/gpt-5.4
    smart: minimax/minimax-m2.7
  default_harness: agent
`
	var cfg AgentConfig
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))

	require.NotNil(t, cfg.Routing)
	assert.Equal(t, "agent", cfg.Routing.DefaultHarness)

	require.Contains(t, cfg.Routing.ProfileLadders, "default")
	assert.Equal(t, []string{"cheap", "standard", "smart"}, cfg.Routing.ProfileLadders["default"])
	require.Contains(t, cfg.Routing.ProfileLadders, "cheap")
	assert.Equal(t, []string{"cheap"}, cfg.Routing.ProfileLadders["cheap"])
	require.Contains(t, cfg.Routing.ProfileLadders, "fast")
	assert.Equal(t, []string{"fast", "smart"}, cfg.Routing.ProfileLadders["fast"])
	require.Contains(t, cfg.Routing.ProfileLadders, "smart")
	assert.Equal(t, []string{"smart"}, cfg.Routing.ProfileLadders["smart"])

	assert.Equal(t, "qwen/qwen3.6", cfg.Routing.ModelOverrides["cheap"])
	assert.Equal(t, "codex/gpt-5.4", cfg.Routing.ModelOverrides["standard"])
	assert.Equal(t, "minimax/minimax-m2.7", cfg.Routing.ModelOverrides["smart"])
}

// TestAgentConfigAcceptsLegacyProfilePriority keeps backward compatibility
// with the flat profile_priority list while profile_ladders is rolled out.
func TestAgentConfigAcceptsLegacyProfilePriority(t *testing.T) {
	raw := `
routing:
  profile_priority: [cheap, standard]
  model_overrides:
    cheap: qwen/qwen3.6
`
	var cfg AgentConfig
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))
	require.NotNil(t, cfg.Routing)
	assert.Equal(t, []string{"cheap", "standard"}, cfg.Routing.ProfilePriority)
	assert.Nil(t, cfg.Routing.ProfileLadders)
}

// TestRoutingConfigResolvedLadder captures the precedence rule:
// ProfileLadders wins when a profile-specific entry exists; legacy
// ProfilePriority is the whole-config fallback for every profile; nil
// for unknown profiles without legacy fallback.
func TestRoutingConfigResolvedLadder(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *RoutingConfig
		profile string
		want    []string
	}{
		{
			name: "profile_ladders wins for matched profile",
			cfg: &RoutingConfig{
				ProfileLadders: map[string][]string{
					"default": {"cheap", "smart"},
				},
				ProfilePriority: []string{"fast"}, // should be ignored
			},
			profile: "default",
			want:    []string{"cheap", "smart"},
		},
		{
			name: "falls through to legacy profile_priority when no ladder entry",
			cfg: &RoutingConfig{
				ProfileLadders: map[string][]string{
					"cheap": {"cheap"},
				},
				ProfilePriority: []string{"cheap", "standard"},
			},
			profile: "default",
			want:    []string{"cheap", "standard"},
		},
		{
			name: "legacy only",
			cfg: &RoutingConfig{
				ProfilePriority: []string{"cheap"},
			},
			profile: "anything",
			want:    []string{"cheap"},
		},
		{
			name:    "nil routing",
			cfg:     nil,
			profile: "default",
			want:    nil,
		},
		{
			name: "empty everywhere",
			cfg: &RoutingConfig{
				ProfileLadders:  map[string][]string{},
				ProfilePriority: nil,
			},
			profile: "default",
			want:    nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.cfg.ResolvedLadder(tc.profile))
		})
	}
}

// TestResolvedLadderReturnsCopy guarantees callers cannot mutate the
// underlying config by modifying the returned slice.
func TestResolvedLadderReturnsCopy(t *testing.T) {
	cfg := &RoutingConfig{
		ProfileLadders: map[string][]string{
			"default": {"cheap", "smart"},
		},
	}
	got := cfg.ResolvedLadder("default")
	require.Len(t, got, 2)
	got[0] = "mutated"
	assert.Equal(t, "cheap", cfg.ProfileLadders["default"][0], "caller mutation must not leak into config")
}
