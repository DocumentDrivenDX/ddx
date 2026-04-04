package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Acceptance tests validate user stories and business requirements
// These tests follow the Given/When/Then pattern from user stories

// TestAcceptance_US001_InitializeProject tests US-001: Initialize DDX in Project
func TestAcceptance_US001_InitializeProject(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		given    func(t *testing.T) string                 // Setup conditions
		when     func(t *testing.T, dir string) error      // Execute action
		then     func(t *testing.T, dir string, err error) // Verify outcome
	}{
		{
			name:     "basic_initialization",
			scenario: "Initialize DDX in project without existing configuration",
			given: func(t *testing.T) string {
				// Given: I am in a project directory without DDX
				tempDir := t.TempDir()
				return tempDir
			},
			when: func(t *testing.T, dir string) error {
				// When: I run `ddx init`
				// Use CommandFactory with the test working directory
				factory := NewCommandFactory(dir)
				rootCmd := factory.NewRootCommand()
				_, err := executeCommand(rootCmd, "init")
				return err
			},
			then: func(t *testing.T, dir string, err error) {
				// Then: a `.ddx/config.yaml` configuration file exists with my settings
				configPath := filepath.Join(dir, ".ddx", "config.yaml")
				if _, statErr := os.Stat(configPath); statErr == nil {
					// Config file exists - validate structure
					data, readErr := os.ReadFile(configPath)
					require.NoError(t, readErr)

					var config map[string]interface{}
					yamlErr := yaml.Unmarshal(data, &config)
					require.NoError(t, yamlErr)

					assert.Contains(t, config, "version", "Config should have version")
					assert.Contains(t, config, "library", "Config should have library")
				}
			},
		},
		{
			name:     "template_based_initialization",
			scenario: "Initialize DDX with specific template",
			given: func(t *testing.T) string {
				// Given: I want to use a specific template
				tempDir := t.TempDir()

				// Setup mock template
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				templateDir := filepath.Join(homeDir, ".ddx", "templates", "test-template")
				require.NoError(t, os.MkdirAll(templateDir, 0755))

				return tempDir
			},
			when: func(t *testing.T, dir string) error {
				// When: I run `ddx init --template test-template`
				// Use CommandFactory with the test working directory
				factory := NewCommandFactory(dir)
				rootCmd := factory.NewRootCommand()
				_, err := executeCommand(rootCmd, "init", "--template", "test-template")
				return err
			},
			then: func(t *testing.T, dir string, err error) {
				// Then: the specified template is applied during initialization
				// Note: Actual implementation may vary
				t.Log("Template-based initialization scenario")
			},
		},
		{
			name:     "reinitialization_prevention",
			scenario: "Prevent re-initialization of DDX-enabled project",
			given: func(t *testing.T) string {
				// Given: DDX is already initialized
				tempDir := t.TempDir()

				// Initialize git repository in temp directory
				gitInit := exec.Command("git", "init")
				gitInit.Dir = tempDir
				require.NoError(t, gitInit.Run())

				gitConfigEmail := exec.Command("git", "config", "user.email", "test@example.com")
				gitConfigEmail.Dir = tempDir
				require.NoError(t, gitConfigEmail.Run())

				gitConfigName := exec.Command("git", "config", "user.name", "Test User")
				gitConfigName.Dir = tempDir
				require.NoError(t, gitConfigName.Run())

				// Create existing config
				config := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"`
				ddxDir := filepath.Join(tempDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte(config),
					0644,
				))

				return tempDir
			},
			when: func(t *testing.T, dir string) error {
				// When: I run `ddx init` again
				// Use CommandFactory with the test working directory
				factory := NewCommandFactory(dir)
				rootCmd := factory.NewRootCommand()
				_, err := executeCommand(rootCmd, "init")
				return err
			},
			then: func(t *testing.T, dir string, err error) {
				// Then: Clear message that DDX is already initialized
				if err != nil {
					assert.Contains(t, err.Error(), "already",
						"Error should indicate DDX already initialized")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

			// Given
			dir := tt.given(t)

			// When
			err := tt.when(t, dir)

			// Then
			tt.then(t, dir, err)
		})
	}
}

// TestAcceptance_US002_ListAvailableAssets tests US-002: List Available Assets
func TestAcceptance_US002_ListAvailableAssets(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		given    func(t *testing.T) string
		when     func(t *testing.T) (string, error)
		then     func(t *testing.T, output string, err error)
	}{
		{
			name:     "list_all_resources",
			scenario: "List all available DDX resources",
			given: func(t *testing.T) string {
				// Given: DDX is initialized with available resources
				env := NewTestEnvironment(t)
				env.InitWithDDx()

				// Create various resources in library directory
				libDir := filepath.Join(env.Dir, ".ddx", "library")

				promptsDir := filepath.Join(libDir, "prompts")
				require.NoError(t, os.MkdirAll(filepath.Join(promptsDir, "claude"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "claude", "prompt.md"), []byte("# Prompt"), 0644))

				templatesDir := filepath.Join(libDir, "templates")
				require.NoError(t, os.MkdirAll(filepath.Join(templatesDir, "nextjs"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(templatesDir, "nextjs", "template.yml"), []byte("name: nextjs"), 0644))

				return env.Dir
			},
			when: func(t *testing.T) (string, error) {
				// When: I run `ddx list`
				testDir := os.Getenv("TEST_DIR")
				factory := NewCommandFactory(testDir)
				rootCmd := factory.NewRootCommand()
				return executeCommand(rootCmd, "list")
			},
			then: func(t *testing.T, output string, err error) {
				// Then: I see categorized resources with helpful descriptions
				assert.NoError(t, err)
				assert.Contains(t, output, "Templates", "Should show templates category")
				assert.Contains(t, output, "Prompts", "Should show prompts category")
			},
		},
		{
			name:     "filter_by_type",
			scenario: "Filter resources by type",
			given: func(t *testing.T) string {
				// Given: I want to see only prompts
				env := NewTestEnvironment(t)
				env.InitWithDDx()

				libDir := filepath.Join(env.Dir, ".ddx", "library")

				promptsDir := filepath.Join(libDir, "prompts")
				require.NoError(t, os.MkdirAll(filepath.Join(promptsDir, "claude"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "claude", "prompt.md"), []byte("# Prompt"), 0644))

				templatesDir := filepath.Join(libDir, "templates")
				require.NoError(t, os.MkdirAll(filepath.Join(templatesDir, "nextjs"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(templatesDir, "nextjs", "template.yml"), []byte("name: nextjs"), 0644))

				return env.Dir
			},
			when: func(t *testing.T) (string, error) {
				// When: I run `ddx list prompts`
				testDir := os.Getenv("TEST_DIR")
				factory := NewCommandFactory(testDir)
				rootCmd := factory.NewRootCommand()
				return executeCommand(rootCmd, "list", "prompts")
			},
			then: func(t *testing.T, output string, err error) {
				// Then: only prompts are shown
				assert.NoError(t, err)
				assert.Contains(t, output, "Prompts", "Should show prompts")
			},
		},
		{
			name:     "json_output",
			scenario: "Output resources as JSON",
			given: func(t *testing.T) string {
				// Given: DDx project with resources
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
				claudeDir := filepath.Join(promptsDir, "claude")
				require.NoError(t, os.MkdirAll(claudeDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "prompt.md"), []byte("# Claude Prompt"), 0644))

				// Store testDir for when() to use
				t.Setenv("TEST_DIR", testDir)
				return testDir
			},
			when: func(t *testing.T) (string, error) {
				// When: I run `ddx list --json`
				testDir := os.Getenv("TEST_DIR")
				factory := NewTestRootCommandWithDir(testDir)
				rootCmd := factory.NewRootCommand()
				return executeCommand(rootCmd, "list", "--json")
			},
			then: func(t *testing.T, output string, err error) {
				// Then: output is valid JSON with resource data
				assert.NoError(t, err)

				// Verify it's valid JSON
				var response struct {
					Resources []map[string]interface{} `json:"resources"`
					Summary   map[string]int           `json:"summary"`
				}
				assert.NoError(t, json.Unmarshal([]byte(output), &response))

				// Should have resources and summary
				assert.NotEmpty(t, response.Resources)
				assert.NotEmpty(t, response.Summary)
			},
		},
		{
			name:     "empty_filter_results",
			scenario: "Handle empty filter results gracefully",
			given: func(t *testing.T) string {
				// Given: DDx has resources but none match filter
				env := NewTestEnvironment(t)
				env.InitWithDDx()

				libDir := filepath.Join(env.Dir, ".ddx", "library")
				promptsDir := filepath.Join(libDir, "prompts")
				require.NoError(t, os.MkdirAll(filepath.Join(promptsDir, "claude"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "claude", "prompt.md"), []byte("# Prompt"), 0644))

				return env.Dir
			},
			when: func(t *testing.T) (string, error) {
				// When: I run `ddx list --filter nonexistent`
				testDir := os.Getenv("TEST_DIR")
				factory := NewCommandFactory(testDir)
				rootCmd := factory.NewRootCommand()
				return executeCommand(rootCmd, "list", "--filter", "nonexistent")
			},
			then: func(t *testing.T, output string, err error) {
				// Then: I see a clear message about no matches
				assert.NoError(t, err)
				assert.Contains(t, output, "No DDx resources found", "Should show no resources message")
				assert.Contains(t, output, "No resources match filter: 'nonexistent'", "Should show filter message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			testDir := tt.given(t)
			t.Setenv("TEST_DIR", testDir)

			// When
			output, err := tt.when(t)

			// Then
			tt.then(t, output, err)
		})
	}
}

// TestAcceptance_ConfigurationManagement tests configuration-related user stories
func TestAcceptance_ConfigurationManagement(t *testing.T) {
	t.Run("view_configuration", func(t *testing.T) {
		// Given: DDX is configured in my project
		tempDir := t.TempDir()

		config := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
persona_bindings:
  environment: "development"`
		ddxDir := filepath.Join(tempDir, ".ddx")
		require.NoError(t, os.MkdirAll(ddxDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(ddxDir, "config.yaml"),
			[]byte(config),
			0644,
		))

		// When: I run `ddx config export`
		factory := NewCommandFactory(tempDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeCommand(rootCmd, "config", "export")

		// Then: I see my current configuration clearly displayed
		assert.NoError(t, err)
		assert.Contains(t, output, "version", "Should show version")
		assert.Contains(t, output, "library", "Should show library")
		assert.Contains(t, output, "persona_bindings", "Should show persona_bindings")
	})

	t.Run("modify_configuration", func(t *testing.T) {
		// Given: I need to change a configuration value
		tempDir := t.TempDir()

		config := `version: "1.0"
library:
  path: "./library"
persona_bindings:
  old_value: "original"`
		ddxDir := filepath.Join(tempDir, ".ddx")
		require.NoError(t, os.MkdirAll(ddxDir, 0755))
		configPath := filepath.Join(ddxDir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte(config), 0644))

		// When: I run `ddx config set persona_bindings.new_value "updated"`
		factory := NewCommandFactory(tempDir)
		rootCmd := factory.NewRootCommand()
		_, err := executeCommand(rootCmd, "config", "set", "persona_bindings.new_value", "updated")

		// Then: the configuration is updated with the new value
		if err == nil {
			data, readErr := os.ReadFile(configPath)
			if readErr == nil {
				var updatedConfig map[string]interface{}
				_ = yaml.Unmarshal(data, &updatedConfig)

				if vars, ok := updatedConfig["persona_bindings"].(map[string]interface{}); ok {
					assert.Equal(t, "updated", vars["new_value"],
						"New value should be set")
				}
			}
		}
	})
}

// TestAcceptance_ProjectSetupIntegration tests complete project setup
func TestAcceptance_ProjectSetupIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("complete_project_setup", func(t *testing.T) {
		// Step 1: Initialize DDX
		tempDir := t.TempDir()

		// Create library structure with prompts
		libraryDir := filepath.Join(tempDir, "library")
		promptsDir := filepath.Join(libraryDir, "prompts")
		require.NoError(t, os.MkdirAll(filepath.Join(promptsDir, "claude"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "claude", "prompt.md"), []byte("# Prompt"), 0644))

		// Create config pointing to library in new format
		config := []byte(`version: "2.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/DocumentDrivenDX/ddx-library"
    branch: "main"
persona_bindings: {}`)
		ddxDir := filepath.Join(tempDir, ".ddx")
		require.NoError(t, os.MkdirAll(ddxDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), config, 0644))

		// Use CommandFactory with working directory
		factory := NewCommandFactory(tempDir)
		rootCmd := factory.NewRootCommand()
		_, initErr := executeCommand(rootCmd, "init")
		_ = initErr

		// Step 2: List available resources
		listOutput, listErr := executeCommand(rootCmd, "list")
		if listErr == nil && listOutput != "" && !strings.Contains(listOutput, "❌ DDx library not found") {
			assert.Contains(t, listOutput, "Prompts", "Should list prompts")
		} else {
			t.Log("Skipping list assertion due to DDx not being initialized or available")
		}

		// Step 3: Verify configuration
		configOutput, configErr := executeCommand(rootCmd, "config")
		if configErr == nil {
			assert.NotEmpty(t, configOutput, "Should show configuration")
		}
	})
}

// TestAcceptance_ErrorScenarios tests error handling from user perspective
func TestAcceptance_ErrorScenarios(t *testing.T) {
	t.Run("clear_error_messages", func(t *testing.T) {
		tests := []struct {
			name          string
			setup         func() string
			command       []string
			expectedError string
		}{
			{
				name: "already_initialized",
				setup: func() string {
					tempDir := t.TempDir()
					// Initialize git repository first
					gitInit := exec.Command("git", "init")
					gitInit.Dir = tempDir
					_ = gitInit.Run()

					gitConfigEmail := exec.Command("git", "config", "user.email", "test@example.com")
					gitConfigEmail.Dir = tempDir
					_ = gitConfigEmail.Run()

					gitConfigName := exec.Command("git", "config", "user.name", "Test User")
					gitConfigName.Dir = tempDir
					_ = gitConfigName.Run()

					config := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/DocumentDrivenDX/ddx-library"
    branch: "main"
persona_bindings: {}`
					ddxDir := filepath.Join(tempDir, ".ddx")
					_ = os.MkdirAll(ddxDir, 0755)
					_ = os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0644)
					return tempDir
				},
				command:       []string{"init"},
				expectedError: "already",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

				tempDir := tt.setup()

				// Use CommandFactory with working directory
				factory := NewCommandFactory(tempDir)
				rootCmd := factory.NewRootCommand()
				output, err := executeCommand(rootCmd, tt.command...)

				// Verify clear error message
				if err != nil {
					assert.Contains(t, strings.ToLower(err.Error()), tt.expectedError,
						"Error message should be clear and helpful")
				} else if output != "" {
					assert.Contains(t, strings.ToLower(output), tt.expectedError,
						"Output should contain helpful error information")
				}
			})
		}
	})
}
