package config

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllConfigLoadEntrypointsHardErrorOnLegacyAgentProviderFields(t *testing.T) {
	fieldValues := map[string][]string{
		"model":            {"opaque-model", `""`, "null"},
		"models":           {"{codex: opaque-model}", "{}", "null"},
		"reasoning_levels": {"{codex: [high]}", "{}", "null"},
		"endpoints":        {"[{type: lmstudio, base_url: 'http://example.invalid/v1'}]", "[]", "null"},
	}

	for field, values := range fieldValues {
		for _, value := range values {
			field, value := field, value
			t.Run(field+"/"+strings.ReplaceAll(value, "/", "_"), func(t *testing.T) {
				root := t.TempDir()
				t.Chdir(root)
				configDir := filepath.Join(root, ddxroot.DirName)
				require.NoError(t, os.MkdirAll(configDir, 0o755))
				configPath := filepath.Join(configDir, "config.yaml")
				contents := "version: \"1.0\"\n" +
					"library:\n  path: ./library\n  repository:\n    url: https://example.invalid/library\n    branch: main\n" +
					"agent:\n  " + field + ": " + value + "\n"
				require.NoError(t, os.WriteFile(configPath, []byte(contents), 0o600))

				loader, err := NewConfigLoaderWithWorkingDir(root)
				require.NoError(t, err)
				entrypoints := map[string]func() error{
					"LoadConfig": func() error {
						_, err := loader.LoadConfig()
						return err
					},
					"LoadConfigFromPath": func() error {
						_, err := loader.LoadConfigFromPath(filepath.Join(ddxroot.DirName, "config.yaml"))
						return err
					},
					"LoadFromFile": func() error {
						_, err := LoadFromFile(filepath.Join(ddxroot.DirName, "config.yaml"))
						return err
					},
					"LoadFromFileAbsolute": func() error {
						_, err := LoadFromFile(configPath)
						return err
					},
					"LoadWithWorkingDir": func() error {
						_, err := LoadWithWorkingDir(root)
						return err
					},
				}
				for name, load := range entrypoints {
					t.Run(name, func(t *testing.T) {
						err := load()
						require.Error(t, err)
						var migrationErr *AgentConfigMigrationError
						require.ErrorAs(t, err, &migrationErr, "expected typed migration error, got %T: %v", err, err)
						assert.Equal(t, "agent."+field, migrationErr.Field)
						assert.Equal(t, configPath, migrationErr.Path)
						assert.True(t, filepath.IsAbs(migrationErr.Path))
						assert.Contains(t, err.Error(), "Fizeau")
						assert.Contains(t, err.Error(), "docs/migrations/routing-config.md")
					})
				}
			})
		}
	}

	for field, values := range fieldValues {
		for _, value := range values {
			field, value := field, value
			t.Run("global/"+field+"/"+strings.ReplaceAll(value, "/", "_"), func(t *testing.T) {
				root := t.TempDir()
				projectConfigDir := filepath.Join(root, ddxroot.DirName)
				require.NoError(t, os.MkdirAll(projectConfigDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(projectConfigDir, "config.yaml"), []byte(`version: "1.0"
library:
  path: ./library
  repository:
    url: https://example.invalid/library
    branch: main
`), 0o600))

				home := t.TempDir()
				t.Setenv("HOME", home)
				globalConfigDir := filepath.Join(home, ddxroot.DirName)
				require.NoError(t, os.MkdirAll(globalConfigDir, 0o755))
				globalConfigPath := filepath.Join(globalConfigDir, "config.yaml")
				contents := "version: \"1.0\"\nagent:\n  " + field + ": " + value + "\n"
				require.NoError(t, os.WriteFile(globalConfigPath, []byte(contents), 0o600))

				_, err := LoadWithWorkingDir(root)
				require.Error(t, err)
				var migrationErr *AgentConfigMigrationError
				require.ErrorAs(t, err, &migrationErr, "expected typed global migration error, got %T: %v", err, err)
				assert.Equal(t, "agent."+field, migrationErr.Field)
				assert.Equal(t, globalConfigPath, migrationErr.Path)
				assert.True(t, filepath.IsAbs(migrationErr.Path))
				assert.Contains(t, err.Error(), "Fizeau")
				assert.Contains(t, err.Error(), "docs/migrations/routing-config.md")
			})
		}
	}
}

func TestAgentConfigContainsOnlyGenericExecutionControls(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	_, err = LoadFromFile(filepath.Join(repoRoot, ddxroot.DirName, "config.yaml"))
	require.NoError(t, err, "the repository's own DDx config must be migrated")

	root := t.TempDir()
	configDir := filepath.Join(root, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	configPath := filepath.Join(configDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`version: "1.0"
library:
  path: ./library
  repository:
    url: https://example.invalid/library
    branch: main
agent:
  timeout_ms: 1234
  wall_clock_ms: 5678
  session_log_dir: .ddx/test-agent-logs
  permissions: unrestricted
  routing: {}
  virtual:
    normalize:
      - pattern: opaque
        replace: replacement
  triage:
    max_decomposition_depth: 4
`), 0o600))

	loader, err := NewConfigLoaderWithWorkingDir(root)
	require.NoError(t, err)
	cfg, err := loader.LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg.Agent)
	assert.Equal(t, 1234, cfg.Agent.TimeoutMS)
	assert.Equal(t, 5678, cfg.Agent.WallClockMS)
	assert.Equal(t, ".ddx/test-agent-logs", cfg.Agent.SessionLogDir)
	assert.Equal(t, "unrestricted", cfg.Agent.Permissions)
	require.NotNil(t, cfg.Agent.Routing)
	require.NotNil(t, cfg.Agent.Virtual)
	require.NotNil(t, cfg.Agent.Triage)

	idle := 9*time.Second + 7*time.Millisecond
	rcfg := cfg.Resolve(CLIOverrides{Timeout: &idle})
	assert.Equal(t, idle, rcfg.Timeout())
	assert.Equal(t, 5678*time.Millisecond, rcfg.WallClock())
	assert.Equal(t, "unrestricted", rcfg.Permissions())
	assert.Equal(t, ".ddx/test-agent-logs", rcfg.SessionLogDir())
	assert.Equal(t, 4, rcfg.MaxDecompositionDepth())

	allowedFields := map[string]bool{
		"TimeoutMS": true, "WallClockMS": true, "SessionLogDir": true,
		"Permissions": true, "Routing": true, "Virtual": true, "Triage": true,
	}
	agentType := reflect.TypeOf(AgentConfig{})
	for i := 0; i < agentType.NumField(); i++ {
		field := agentType.Field(i)
		assert.True(t, allowedFields[field.Name], "AgentConfig has non-generic field %s", field.Name)
	}
	assert.Len(t, allowedFields, agentType.NumField())
}

func TestNoLegacyProviderConfigurationTypesRemain(t *testing.T) {
	fset := token.NewFileSet()
	typesFile, err := parser.ParseFile(fset, "types.go", nil, 0)
	require.NoError(t, err)
	declaredTypes := make(map[string]bool)
	ast.Inspect(typesFile, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if ok {
			declaredTypes[typeSpec.Name.Name] = true
		}
		return true
	})
	for _, name := range []string{"AgentEndpoint", "AgentRunnerConfig", "LLMPresetConfig", "ProfileModels"} {
		assert.False(t, declaredTypes[name], "retired provider type %s remains", name)
	}

	schemaBytes, err := os.ReadFile(filepath.Join("schema", "config.schema.json"))
	require.NoError(t, err)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(schemaBytes, &schema))
	properties := schema["properties"].(map[string]any)
	agentSchema := properties["agent"].(map[string]any)
	agentProperties := agentSchema["properties"].(map[string]any)
	for _, name := range []string{"model", "models", "reasoning_levels", "endpoints"} {
		_, exists := agentProperties[name]
		assert.False(t, exists, "retired agent.%s remains in schema", name)
	}

	for _, path := range []string{"types.go", "clone.go", "resolved.go"} {
		contents, err := os.ReadFile(path)
		require.NoError(t, err)
		for _, token := range []string{"AgentEndpoint", "AgentRunnerConfig", "LLMPresetConfig", "ProfileModels", ".Models", ".ReasoningLevels", ".Endpoints"} {
			assert.NotContains(t, string(contents), token, "%s retains %s", path, token)
		}
	}
}
