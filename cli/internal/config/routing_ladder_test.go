package config

import (
	"io"
	"os"
	"path/filepath"
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
`
	var cfg AgentConfig
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))

	require.NotNil(t, cfg.Routing)

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

func TestAgentConfigParsesEndpointBlocks(t *testing.T) {
	raw := `
harness: claude
endpoints:
  - type: lmstudio
    host: vidar
    port: 1234
    api_key: lmstudio
  - type: omlx
    base_url: http://vidar:1235/v1
`
	var cfg AgentConfig
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))

	require.Len(t, cfg.Endpoints, 2)
	assert.Equal(t, "lmstudio", cfg.Endpoints[0].Type)
	assert.Equal(t, "vidar", cfg.Endpoints[0].Host)
	assert.Equal(t, 1234, cfg.Endpoints[0].Port)
	assert.Equal(t, "lmstudio", cfg.Endpoints[0].APIKey)
	assert.Equal(t, "omlx", cfg.Endpoints[1].Type)
	assert.Equal(t, "http://vidar:1235/v1", cfg.Endpoints[1].BaseURL)
}

func TestLoadConfigWarnsForLegacyProfilePriority(t *testing.T) {
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
agent:
  routing:
    profile_priority: [cheap, standard]
`), 0644))

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	_, loadErr := LoadWithWorkingDir(tempDir)
	require.NoError(t, w.Close())
	os.Stderr = oldStderr
	require.NoError(t, loadErr)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Contains(t, string(out), "agent.routing.profile_priority is deprecated")
}

// TestRoutingConfigResolvedLadder captures the precedence rule:
// ProfileLadders wins when a profile-specific entry exists; legacy
// ProfilePriority is the default-profile fallback; the shipped FEAT-006
// defaults are used when neither config form is set.
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
			name: "legacy only aliases default",
			cfg: &RoutingConfig{
				ProfilePriority: []string{"cheap"},
			},
			profile: "default",
			want:    []string{"cheap"},
		},
		{
			name: "legacy only does not override named shipped profile",
			cfg: &RoutingConfig{
				ProfilePriority: []string{"cheap"},
			},
			profile: "smart",
			want:    []string{"smart"},
		},
		{
			name:    "nil routing",
			cfg:     nil,
			profile: "default",
			want:    []string{"cheap", "standard", "smart"},
		},
		{
			name: "empty everywhere uses shipped default",
			cfg: &RoutingConfig{
				ProfileLadders:  map[string][]string{},
				ProfilePriority: nil,
			},
			profile: "default",
			want:    []string{"cheap", "standard", "smart"},
		},
		{
			name:    "shipped fast profile",
			cfg:     nil,
			profile: "fast",
			want:    []string{"fast", "smart"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.cfg.ResolvedLadder(tc.profile))
		})
	}
}

func TestDefaultProfileLaddersReturnsCopy(t *testing.T) {
	got := DefaultProfileLadders()
	require.Equal(t, []string{"cheap", "standard", "smart"}, got["default"])
	got["default"][0] = "mutated"
	assert.Equal(t, "cheap", DefaultProfileLadders()["default"][0], "caller mutation must not leak into defaults")
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
