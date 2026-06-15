package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestDefaultConfig validates the default configuration values
func TestDefaultConfig_Basic(t *testing.T) {
	t.Parallel()
	config := DefaultNewConfig()

	assert.Equal(t, "1.0", config.Version)
	assert.Equal(t, ".ddx/plugins/ddx", config.Library.Path)
	assert.Equal(t, "https://github.com/DocumentDrivenDX/ddx-library", config.Library.Repository.URL)
	assert.Equal(t, "main", config.Library.Repository.Branch)
	assert.Empty(t, config.PersonaBindings)
}

// TestLoadConfig_DefaultOnly tests loading when no config files exist
func TestLoadConfig_DefaultOnly_Basic(t *testing.T) {
	// Create temp directory without config files
	tempDir := t.TempDir()

	// Isolate from global config by setting temporary HOME
	t.Setenv("HOME", tempDir)

	config, err := LoadWithWorkingDir(tempDir)

	require.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, DefaultConfig.Version, config.Version)
	assert.Equal(t, DefaultConfig.Library.Repository.URL, config.Library.Repository.URL)
}

func TestLoadWithWorkingDir_MergesGlobalExecutionConfig(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	globalDir := filepath.Join(homeDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(globalDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(`version: "1.0"
executions:
  temp_worktree_root: /Users/erik/Projects/.ddx-exec-wt
  docker:
    image: ddx-attempt-runner:dev
    project_dockerfile: .ddx/attempt-runner.Dockerfile
    project_context: .
    memory: 8g
    cpus: "4"
`), 0o644))

	projectDirDDX := filepath.Join(projectDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(projectDirDDX, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDirDDX, "config.yaml"), []byte(`version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/project/repo
    branch: main
executions:
  docker:
    memory: 4g
`), 0o644))

	cfg, err := LoadWithWorkingDir(projectDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Executions)
	require.NotNil(t, cfg.Executions.Docker)
	assert.Equal(t, "/Users/erik/Projects/.ddx-exec-wt", cfg.Executions.TempWorktreeRoot)
	assert.Equal(t, "ddx-attempt-runner:dev", cfg.Executions.Docker.Image)
	assert.Equal(t, ".ddx/attempt-runner.Dockerfile", cfg.Executions.Docker.ProjectDockerfile)
	assert.Equal(t, ".", cfg.Executions.Docker.ProjectContext)
	assert.Equal(t, "4g", cfg.Executions.Docker.Memory, "project config must override global execution fields")
	assert.Equal(t, "4", cfg.Executions.Docker.CPUs)
}

func TestLoadWithWorkingDir_InheritsGlobalAgentEndpointsWhenProjectHasNone(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	globalDir := filepath.Join(homeDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(globalDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(`version: "1.0"
agent:
  endpoints:
    - type: lmstudio
      host: 127.0.0.1
      port: 1234
      api_key: lmstudio
`), 0o644))

	projectDirDDX := filepath.Join(projectDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(projectDirDDX, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDirDDX, "config.yaml"), []byte(`version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/project/repo
    branch: main
`), 0o644))

	cfg, err := LoadWithWorkingDir(projectDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Agent)
	require.Len(t, cfg.Agent.Endpoints, 1)
	assert.Equal(t, "lmstudio", cfg.Agent.Endpoints[0].Type)
	assert.Equal(t, "127.0.0.1", cfg.Agent.Endpoints[0].Host)
	assert.Equal(t, 1234, cfg.Agent.Endpoints[0].Port)
}

func TestLoadWithWorkingDir_ProjectAgentEndpointsOverrideGlobal(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	globalDir := filepath.Join(homeDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(globalDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(`version: "1.0"
agent:
  endpoints:
    - type: lmstudio
      host: 127.0.0.1
      port: 1234
`), 0o644))

	projectDirDDX := filepath.Join(projectDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(projectDirDDX, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDirDDX, "config.yaml"), []byte(`version: "1.0"
agent:
  endpoints:
    - type: omlx
      host: 127.0.0.1
      port: 1235
`), 0o644))

	cfg, err := LoadWithWorkingDir(projectDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Agent)
	require.Len(t, cfg.Agent.Endpoints, 1)
	assert.Equal(t, "omlx", cfg.Agent.Endpoints[0].Type)
	assert.Equal(t, 1235, cfg.Agent.Endpoints[0].Port)
}

// TestLoadConfig_LocalConfig tests loading with local .ddx.yml
func TestLoadConfig_LocalConfig_Basic(t *testing.T) {
	tempDir := t.TempDir()

	// Create local config
	localConfig := &Config{
		Version: "2.0",
		Library: &LibraryConfig{
			Path: "./custom-library",
			Repository: &RepositoryConfig{
				URL:    "https://github.com/custom/repo",
				Branch: "develop",
			},
		},
		PersonaBindings: map[string]string{
			"test-role": "test-persona",
		},
	}

	configData, err := yaml.Marshal(localConfig)
	require.NoError(t, err)

	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	configPath := filepath.Join(ddxDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, configData, 0644))

	// Load config
	config, err := LoadWithWorkingDir(tempDir)

	require.NoError(t, err)
	assert.Equal(t, "2.0", config.Version)
	assert.Equal(t, "https://github.com/custom/repo", config.Library.Repository.URL)
	assert.Equal(t, "develop", config.Library.Repository.Branch)
	assert.Contains(t, config.PersonaBindings, "test-role")
}

// TestLoadLocal tests LoadLocal function
func TestLoadLocal_Basic(t *testing.T) {
	tempDir := t.TempDir()

	// Create local config
	localConfig := &Config{
		Version: "1.5",
		Library: &LibraryConfig{
			Path: "./library",
			Repository: &RepositoryConfig{
				URL:    "https://github.com/local/repo",
				Branch: "feature",
			},
		},
		PersonaBindings: map[string]string{
			"test_var": "test_value",
		},
	}

	configData, err := yaml.Marshal(localConfig)
	require.NoError(t, err)

	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	configPath := filepath.Join(ddxDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, configData, 0644))

	// Load local config
	config, err := LoadWithWorkingDir(tempDir)

	require.NoError(t, err)
	assert.Equal(t, "1.5", config.Version)
	assert.Equal(t, "https://github.com/local/repo", config.Library.Repository.URL)
	assert.Equal(t, "test_value", config.PersonaBindings["test_var"])
}

func TestLoadWithWorkingDir_ProseConfig(t *testing.T) {
	tempDir := t.TempDir()

	configData := []byte(`version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/test/repo
    branch: main
persona_bindings: {}
prose:
  mode: planning
  severity: advisory
  policy: blocking
  runner: vale
  includes:
    - docs/helix/**
  excludes:
    - "**/*.generated.md"
  vocabulary:
    accept:
      - Quartz
    reject:
      - system
`)

	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), configData, 0644))

	cfg, err := LoadWithWorkingDir(tempDir)
	require.NoError(t, err)
	if cfg.Prose == nil {
		t.Fatal("expected prose config to load")
	}
	assert.Equal(t, "planning", cfg.Prose.Mode)
	assert.Equal(t, "advisory", cfg.Prose.Severity)
	assert.Equal(t, "blocking", cfg.Prose.Policy)
	assert.Equal(t, "vale", cfg.Prose.Runner)
	assert.Equal(t, []string{"docs/helix/**"}, cfg.Prose.Includes)
	assert.Equal(t, []string{"**/*.generated.md"}, cfg.Prose.Excludes)
	require.NotNil(t, cfg.Prose.Vocabulary)
	assert.Contains(t, cfg.Prose.Vocabulary.Accept, "Quartz")
	assert.Contains(t, cfg.Prose.Vocabulary.Reject, "system")
}

func TestLoadWithWorkingDir_ConventionRoot(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "demo-project")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	initConfigTestRepo(t, projectRoot)

	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	conventionRoot := ddxroot.Path(context.Background(), projectRoot)
	configData := []byte(`version: "1.0"
library:
  path: "./convention-library"
  repository:
    url: "https://github.com/convention/repo"
    branch: "main"
persona_bindings:
  author: "Convention User"
`)
	require.NoError(t, os.WriteFile(filepath.Join(conventionRoot, "config.yaml"), configData, 0o644))

	subdir := filepath.Join(projectRoot, "nested", "dir")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	cfg, err := LoadWithWorkingDir(subdir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Library)
	assert.Equal(t, "./convention-library", cfg.Library.Path)
	assert.Equal(t, "Convention User", cfg.PersonaBindings["author"])

	loader, err := NewConfigLoaderWithWorkingDir(subdir)
	require.NoError(t, err)
	format, configPath, err := loader.DetectConfigFormat()
	require.NoError(t, err)
	assert.Equal(t, "new", format)
	assert.Equal(t, filepath.Join(conventionRoot, "config.yaml"), configPath)
}

// TestSaveLocal tests SaveLocal function
func TestSaveLocal_Basic(t *testing.T) {
	tempDir := t.TempDir()

	config := &Config{
		Version: "1.0",
		Library: &LibraryConfig{
			Repository: &RepositoryConfig{
				URL:    "https://github.com/test/repo",
				Branch: "main",
			},
		},
		PersonaBindings: map[string]string{
			"key1": "value1",
		},
	}

	// Save config locally in new format
	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	configPath := filepath.Join(ddxDir, "config.yaml")
	configData, err := yaml.Marshal(config)
	require.NoError(t, err)
	err = os.WriteFile(configPath, configData, 0644)
	require.NoError(t, err)

	// Verify file was created
	assert.FileExists(t, configPath)

	// Load and verify
	loadedConfig, err := LoadWithWorkingDir(tempDir)
	require.NoError(t, err)

	assert.Equal(t, config.Version, loadedConfig.Version)
	assert.Equal(t, config.Library.Repository.URL, loadedConfig.Library.Repository.URL)
	assert.Equal(t, "value1", loadedConfig.PersonaBindings["key1"])
}

// TestLoadConfig_InvalidYAML tests handling of invalid YAML
func TestLoadConfig_InvalidYAML_Basic(t *testing.T) {
	tempDir := t.TempDir()

	// Create invalid YAML file in new format location
	invalidYAML := `
version: 1.0
repository:
  url: https://github.com/test
  branch: [this is invalid
`
	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	configPath := filepath.Join(ddxDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(invalidYAML), 0644))

	// Should return error
	config, err := LoadWithWorkingDir(tempDir)

	assert.Error(t, err)
	assert.Nil(t, config)
}

func TestLoadConfig_AgentCapabilitiesFields(t *testing.T) {
	tempDir := t.TempDir()

	content := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
agent:
  model: o3-mini
  models:
    claude: claude-sonnet-4-20250514
  reasoning_levels:
    codex:
      - low
      - medium
      - high
`

	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(content), 0644))

	cfg, err := LoadWithWorkingDir(tempDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Agent)
	assert.Equal(t, "o3-mini", cfg.Agent.Model)
	assert.Equal(t, "claude-sonnet-4-20250514", cfg.Agent.Models["claude"])
	assert.Equal(t, []string{"low", "medium", "high"}, cfg.Agent.ReasoningLevels["codex"])
}

func TestSchemaValidation_AgentVirtualNormalize(t *testing.T) {
	t.Parallel()
	validator, err := NewValidator()
	require.NoError(t, err)

	content := []byte(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
agent:
  virtual:
    normalize:
      - pattern: "foo.*bar"
        replace: "baz"
      - pattern: "\\d{4}-\\d{2}-\\d{2}"
        replace: "<date>"
`)
	err = validator.Validate(content)
	assert.NoError(t, err, "config with agent.virtual.normalize should pass schema validation")
}

func TestSchemaValidation_AgentHarnessRejected(t *testing.T) {
	t.Parallel()
	validator, err := NewValidator()
	require.NoError(t, err)

	content := []byte(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
agent:
  harness: codex
`)
	err = validator.Validate(content)
	assert.Error(t, err, "agent.harness is not durable project config")
	assert.Contains(t, err.Error(), "harness")
}

func TestSchemaValidation_ServerSection(t *testing.T) {
	t.Parallel()
	validator, err := NewValidator()
	require.NoError(t, err)

	content := []byte(`version: "1.0"
server:
  addr: ":8080"
  tsnet:
    enabled: true
    hostname: "ddx-server"
    auth_key: "tskey-auth-xxx"
    state_dir: "/var/lib/ddx/tsnet"
`)
	err = validator.Validate(content)
	assert.NoError(t, err, "config with server.addr and server.tsnet fields should pass schema validation")
}

func TestLoadConfig_BeadPrefixField(t *testing.T) {
	tempDir := t.TempDir()

	content := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
bead:
  id_prefix: "nif"
`

	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(content), 0o644))

	cfg, err := LoadWithWorkingDir(tempDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Bead)
	assert.Equal(t, "nif", cfg.Bead.IDPrefix)
}

func TestLoadConfig_BeadBackendField(t *testing.T) {
	tempDir := t.TempDir()

	content := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
bead:
  backend: axon
`

	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(content), 0o644))

	cfg, err := LoadWithWorkingDir(tempDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Bead)
	assert.Equal(t, "axon", cfg.Bead.Backend)
}

func initConfigTestRepo(t *testing.T, dir string) {
	t.Helper()
	out, err := gitpkg.Command(context.Background(), dir, "init").CombinedOutput()
	require.NoError(t, err, "git init failed: %s", out)
}
