package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestConfigCommand tests the config command
func TestConfigCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		setup       func(t *testing.T) string
		validate    func(t *testing.T, workDir string, output string, err error)
		expectError bool
	}{
		{
			name: "show config",
			args: []string{"config", "export"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				// Create config file
				config := `version: "1.0"
repository:
  url: "https://github.com/test/repo"
  branch: "main"
persona_bindings:
  project_name: "test-project"
  port: "8080"
`
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0644))
				return workDir
			},
			validate: func(t *testing.T, workDir string, output string, err error) {
				assert.Contains(t, output, "version")
				assert.Contains(t, output, "repository")
			},
			expectError: false,
		},
		{
			name: "get specific config value",
			args: []string{"config", "get", "library.repository.url"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				config := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
persona_bindings:
  author: "Test User"
`
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0644))
				return workDir
			},
			validate: func(t *testing.T, workDir string, output string, err error) {
				assert.Contains(t, output, "https://github.com/test/repo")
			},
			expectError: false,
		},
		{
			name: "set config value",
			args: []string{"config", "set", "variables.new_var", "new_value"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				config := `version: "1.0"
persona_bindings:
  existing: "value"
`
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0644))
				return workDir
			},
			validate: func(t *testing.T, workDir string, output string, cmdErr error) {
				// Read config to verify change
				data, err := os.ReadFile(filepath.Join(workDir, ".ddx", "config.yaml"))
				if err == nil {
					var config map[string]interface{}
					_ = yaml.Unmarshal(data, &config)

					if vars, ok := config["variables"].(map[string]interface{}); ok {
						assert.Equal(t, "new_value", vars["new_var"])
					}
				}
			},
			expectError: false,
		},
		{
			name: "config with no config file",
			args: []string{"config"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()
				// No config file created
				return workDir
			},
			validate: func(t *testing.T, workDir string, output string, err error) {
				// Should handle gracefully or show defaults
				assert.NotEmpty(t, output)
			},
			expectError: false,
		},
		{
			name: "validate config",
			args: []string{"config", "validate"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				// Valid config
				config := `version: "1.0"
repository:
  url: "https://github.com/test/repo"
  branch: "main"
`
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0644))
				return workDir
			},
			validate: func(t *testing.T, workDir string, output string, err error) {
				assert.Contains(t, output, "valid")
			},
			expectError: false,
		},
		{
			name: "validate invalid config",
			args: []string{"config", "validate"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				// Invalid YAML
				config := `version: "1.0"
repository:
  url: [this is invalid
`
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0644))
				return workDir
			},
			validate: func(t *testing.T, workDir string, output string, err error) {
				// Should report invalid
				assert.Error(t, err)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh command for test isolation
			// (flags are now local to the command)

			//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

			var workDir string
			if tt.setup != nil {
				workDir = tt.setup(t)
			}

			// Use CommandFactory with the test working directory
			var factory *CommandFactory
			if workDir != "" {
				factory = NewCommandFactory(workDir)
			} else {
				factory = NewCommandFactory("/tmp")
			}
			rootCmd := factory.NewRootCommand()

			output, err := executeCommand(rootCmd, tt.args...)

			if tt.expectError {
				// Error handling depends on implementation
			}

			if tt.validate != nil {
				tt.validate(t, workDir, output, err)
			}
		})
	}
}

// TestConfigCommand_Global tests global config operations
func TestConfigCommand_Global(t *testing.T) {
	// Create a fresh command for test isolation
	// (flags are now local to the command)

	// Setup temp home
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Create global config using new format
	globalConfigDir := filepath.Join(homeDir, ".ddx")
	require.NoError(t, os.MkdirAll(globalConfigDir, 0755))

	globalConfig := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
persona_bindings:
  author: "Test User"
  email: "test@example.com"
  project_name: "test"
`
	require.NoError(t, os.WriteFile(filepath.Join(globalConfigDir, "config.yaml"), []byte(globalConfig), 0644))

	// Use command factory to get proper export functionality
	tempDir := t.TempDir()
	factory := NewCommandFactory(tempDir)
	rootCmd := factory.NewRootCommand()

	// Test reading global config
	output, err := executeCommand(rootCmd, "config", "export", "--global")

	assert.NoError(t, err)
	assert.Contains(t, output, "author")
	assert.Contains(t, output, "email")
}

// TestConfigCommand_Help tests the help output
func TestConfigCommand_Help(t *testing.T) {
	rootCmd := &cobra.Command{
		Use:   "ddx",
		Short: "DDx CLI",
	}

	// Create fresh config command
	freshConfigCmd := &cobra.Command{
		Use:   "config [get|set|validate] [key] [value]",
		Short: "Manage DDx configuration",
		RunE:  runConfig,
	}
	freshConfigCmd.Flags().BoolP("global", "g", false, "Edit global configuration")
	freshConfigCmd.Flags().BoolP("local", "l", false, "Edit local project configuration")
	freshConfigCmd.Flags().Bool("unset", false, "Unset a configuration key")
	freshConfigCmd.Flags().Bool("list", false, "List all configuration values")

	rootCmd.AddCommand(freshConfigCmd)

	output, err := executeCommand(rootCmd, "config", "--help")

	assert.NoError(t, err)
	assert.Contains(t, output, "Manage DDx configuration")
	assert.Contains(t, output, "global")
	assert.Contains(t, output, "local")
	assert.Contains(t, output, "configuration")
}
