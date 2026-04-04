package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Contract validation tests verify that CLI commands conform to their API contracts
// as defined in docs/02-design/contracts/CLI-001-core-commands.md

// Helper function to create a fresh root command for tests
func getContractTestRootCommand() *cobra.Command {
	factory := NewCommandFactory("/tmp")
	return factory.NewRootCommand()
}

// TestInitCommand_Contract validates init command against CLI-001 contract
func TestInitCommand_Contract(t *testing.T) {
	tests := []struct {
		name           string
		description    string
		args           []string
		setup          func(t *testing.T) string
		expectCode     int // Expected exit code per contract
		validateOutput func(t *testing.T, output string)
	}{
		{
			name:        "contract_exit_code_0_success",
			description: "Exit code 0: Success",
			args:        []string{"init", "--no-git"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()
				// Initialize git repository for init command in the correct directory
				gitInit := exec.Command("git", "init")
				gitInit.Dir = workDir
				require.NoError(t, gitInit.Run())

				gitEmail := exec.Command("git", "config", "user.email", "test@example.com")
				gitEmail.Dir = workDir
				require.NoError(t, gitEmail.Run())

				gitName := exec.Command("git", "config", "user.name", "Test User")
				gitName.Dir = workDir
				require.NoError(t, gitName.Run())

				return workDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Contract specifies these output elements
				assert.Contains(t, output, "Initialized DDx")
			},
		},
		{
			name:        "contract_exit_code_2_exists",
			description: "Exit code 2: Configuration already exists",
			args:        []string{"init", "--no-git"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()
				// Initialize git repository for init command
				gitInit := exec.Command("git", "init")
				gitInit.Dir = workDir
				require.NoError(t, gitInit.Run())

				gitConfigEmail := exec.Command("git", "config", "user.email", "test@example.com")
				gitConfigEmail.Dir = workDir
				require.NoError(t, gitConfigEmail.Run())

				gitConfigName := exec.Command("git", "config", "user.name", "Test User")
				gitConfigName.Dir = workDir
				require.NoError(t, gitConfigName.Run())
				// Create existing config in new format
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte("version: \"1.0\"\nlibrary:\n  path: \".ddx/library\"\n  repository:\n    url: \"https://github.com/easel/ddx-library\"\n    branch: \"main\"\npersona_bindings: {}"),
					0644,
				))
				return workDir
			},
			expectCode: 2,
			validateOutput: func(t *testing.T, output string) {
				// Should indicate config exists
			},
		},
		{
			name:        "contract_force_flag",
			description: "--force flag overwrites existing config without backup",
			args:        []string{"init", "--force", "--no-git"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()
				// Initialize git repository for init command in the correct directory
				gitInit := exec.Command("git", "init")
				gitInit.Dir = workDir
				require.NoError(t, gitInit.Run())

				gitEmail := exec.Command("git", "config", "user.email", "test@example.com")
				gitEmail.Dir = workDir
				require.NoError(t, gitEmail.Run())

				gitName := exec.Command("git", "config", "user.name", "Test User")
				gitName.Dir = workDir
				require.NoError(t, gitName.Run())
				// Create existing config in new format
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte("version: \"0.9\"\nlibrary:\n  path: \".ddx/library\"\n  repository:\n    url: \"https://github.com/easel/ddx-library\"\n    branch: \"main\"\npersona_bindings: {}"),
					0644,
				))
				return workDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should succeed with force and not mention backup
				assert.NotContains(t, output, "backup", "Should not create or mention backup")
			},
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

			// Create a fresh root command with the test working directory
			var rootCmd *cobra.Command
			if workDir != "" {
				factory := NewCommandFactory(workDir)
				rootCmd = factory.NewRootCommand()
			} else {
				rootCmd = getContractTestRootCommand()
			}

			output, err := executeContractCommand(rootCmd, tt.args...)

			// Validate exit code matches contract
			if tt.expectCode == 0 {
				assert.NoError(t, err, "Contract specifies exit code 0 for: %s", tt.description)
			} else {
				assert.Error(t, err, "Contract specifies non-zero exit code for: %s", tt.description)
				// Note: Cobra doesn't expose exact exit codes in tests,
				// but we validate error presence
			}

			if tt.validateOutput != nil {
				tt.validateOutput(t, output)
			}
		})
	}
}

// TestListCommand_Contract validates list command against CLI-001 contract
func TestListCommand_Contract(t *testing.T) {
	tests := []struct {
		name           string
		description    string
		args           []string
		setup          func(t *testing.T) string
		expectCode     int
		validateOutput func(t *testing.T, output string)
	}{
		{
			name:        "contract_output_format",
			description: "Output format matches contract specification",
			args:        []string{"list"},
			setup: func(t *testing.T) string {
				testDir := t.TempDir()

				// Initialize DDx properly
				factory := NewCommandFactory(testDir)
				initCmd := factory.NewRootCommand()
				initCmd.SetArgs([]string{"init", "--no-git", "--silent"})
				var initOut bytes.Buffer
				initCmd.SetOut(&initOut)
				initCmd.SetErr(&initOut)
				require.NoError(t, initCmd.Execute())

				// Create test resources in the library
				libraryDir := filepath.Join(testDir, ".ddx", "library")

				promptsDir := filepath.Join(libraryDir, "prompts")
				require.NoError(t, os.MkdirAll(filepath.Join(promptsDir, "claude"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "claude", "prompt.md"), []byte("# Prompt"), 0644))

				templatesDir := filepath.Join(libraryDir, "templates")
				require.NoError(t, os.MkdirAll(filepath.Join(templatesDir, "nextjs"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(templatesDir, "nextjs", "template.yml"), []byte("name: nextjs"), 0644))

				return testDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Contract specifies section headers
				assert.Contains(t, output, "Templates")
				assert.Contains(t, output, "Prompts")
			},
		},
		{
			name:        "contract_filter_argument",
			description: "Filter argument works as specified",
			args:        []string{"list", "prompts"},
			setup: func(t *testing.T) string {
				testDir := t.TempDir()

				// Initialize DDx properly
				factory := NewCommandFactory(testDir)
				initCmd := factory.NewRootCommand()
				initCmd.SetArgs([]string{"init", "--no-git", "--silent"})
				var initOut bytes.Buffer
				initCmd.SetOut(&initOut)
				initCmd.SetErr(&initOut)
				require.NoError(t, initCmd.Execute())

				// Create test resources in the library
				libraryDir := filepath.Join(testDir, ".ddx", "library")

				promptsDir := filepath.Join(libraryDir, "prompts")
				require.NoError(t, os.MkdirAll(filepath.Join(promptsDir, "claude"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "claude", "prompt.md"), []byte("# Prompt"), 0644))

				templatesDir := filepath.Join(libraryDir, "templates")
				require.NoError(t, os.MkdirAll(filepath.Join(templatesDir, "nextjs"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(templatesDir, "nextjs", "template.yml"), []byte("name: nextjs"), 0644))

				return testDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should only show prompts
				assert.Contains(t, output, "Prompts")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh command for test isolation
			// (flags are now local to the command)

			var workDir string
			if tt.setup != nil {
				workDir = tt.setup(t)
			}

			// Create a fresh list command to avoid state pollution
			var factory *CommandFactory
			if workDir != "" {
				factory = NewCommandFactory(workDir)
			} else {
				factory = NewCommandFactory("/tmp")
			}
			freshListCmd := &cobra.Command{
				Use:   "list",
				Short: "List available resources",
				RunE:  factory.runList,
			}
			freshListCmd.Flags().StringP("filter", "f", "", "Filter resources by name")
			freshListCmd.Flags().Bool("json", false, "Output results as JSON")
			freshListCmd.Flags().Bool("tree", false, "Display resources in tree format")

			rootCmd := &cobra.Command{
				Use:   "ddx",
				Short: "DDx CLI",
			}
			rootCmd.AddCommand(freshListCmd)

			output, err := executeContractCommand(rootCmd, tt.args...)

			if tt.expectCode == 0 {
				assert.NoError(t, err, "Contract specifies exit code 0 for: %s", tt.description)
			} else {
				assert.Error(t, err, "Contract specifies non-zero exit code for: %s", tt.description)
			}

			if tt.validateOutput != nil {
				tt.validateOutput(t, output)
			}
		})
	}
}

// TestConfigCommand_Contract validates config command output format
func TestConfigCommand_Contract(t *testing.T) {
	tests := []struct {
		name           string
		description    string
		args           []string
		setup          func(t *testing.T) string
		validateOutput func(t *testing.T, output string)
	}{
		{
			name:        "contract_yaml_output",
			description: "Config output is valid YAML as per contract",
			args:        []string{"config", "export"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				config := `version: "1.0"
repository:
  url: "https://github.com/test/repo"
persona_bindings:
  test: "value"`
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0644))
				return workDir
			},
			validateOutput: func(t *testing.T, output string) {
				// Output should contain YAML structure
				assert.Contains(t, output, "version")
				assert.Contains(t, output, "repository")
			},
		},
		{
			name:        "contract_get_operation",
			description: "Get operation returns specific value",
			args:        []string{"config", "get", "version"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				config := `version: "1.0"`
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0644))
				return workDir
			},
			validateOutput: func(t *testing.T, output string) {
				// Should return just the value
				assert.Contains(t, strings.TrimSpace(output), "1.0")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			output, err := executeContractCommand(rootCmd, tt.args...)
			assert.NoError(t, err)

			if tt.validateOutput != nil {
				tt.validateOutput(t, output)
			}
		})
	}
}

// Helper to execute command for contract testing
func executeContractCommand(root *cobra.Command, args ...string) (output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err = root.Execute()
	return buf.String(), err
}
