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

// TestAgentConfigAcceptsProfilePriority verifies the flat profile_priority list still parses.
func TestAgentConfigAcceptsProfilePriority(t *testing.T) {
	raw := `
routing:
  profile_priority: [cheap, standard]
`
	var cfg AgentConfig
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))
	require.NotNil(t, cfg.Routing)
	assert.Equal(t, []string{"cheap", "standard"}, cfg.Routing.ProfilePriority)
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
