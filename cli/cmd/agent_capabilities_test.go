package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentCapabilitiesCommandJSON(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	config := `version: "1.0"
library:
  path: ".ddx/library"
  repository:
    url: "https://example.com/lib"
    branch: "main"
agent:
  harness: codex
  model: o3-mini
  reasoning_levels:
    codex:
      - low
      - medium
      - high
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0o644))

	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	codexPath := filepath.Join(binDir, "codex")
	require.NoError(t, os.WriteFile(codexPath, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "capabilities", "--json")
	require.NoError(t, err)

	var caps struct {
		Harness         string   `json:"harness"`
		Available       bool     `json:"available"`
		Binary          string   `json:"binary"`
		Model           string   `json:"model"`
		Models          []string `json:"models"`
		ReasoningLevels []string `json:"reasoning_levels"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &caps))
	require.Equal(t, "codex", caps.Harness)
	require.True(t, caps.Available)
	require.Equal(t, "codex", caps.Binary)
	require.Equal(t, "o3-mini", caps.Model)
	require.Equal(t, []string{"o3-mini"}, caps.Models)
	require.Equal(t, []string{"low", "medium", "high"}, caps.ReasoningLevels)
}

func TestAgentCapabilitiesCommandUnknownHarness(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	config := `version: "1.0"
library:
  path: ".ddx/library"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0o644))

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(rootCmd, "agent", "capabilities", "nonexistent")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "unknown harness"))
}
