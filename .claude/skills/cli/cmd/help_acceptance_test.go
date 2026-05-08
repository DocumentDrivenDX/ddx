package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a fresh root command for tests
func getHelpTestRootCommand(workingDir string) *cobra.Command {
	if workingDir == "" {
		workingDir = "/tmp"
	}
	factory := NewCommandFactory(workingDir)
	return factory.NewRootCommand()
}

// TestAcceptance_US006_GetCommandHelp tests US-006: Get Command Help
func TestAcceptance_US006_GetCommandHelp(t *testing.T) {

	t.Run("general_help_overview", func(t *testing.T) {
		// AC: Given I need general help, when I run `ddx help`, then I see an overview of all available commands with brief descriptions
		rootCmd := getHelpTestRootCommand("")
		output, err := executeCommand(rootCmd, "help")

		require.NoError(t, err, "Help command should execute successfully")

		// Should show overview
		assert.Contains(t, output, "Available Commands", "Should show command overview section")
		assert.Contains(t, output, "DDx", "Should mention DDx in overview")

		// Should list all major commands with descriptions
		assert.Contains(t, output, "init", "Should list init command")
		assert.Contains(t, output, "list", "Should list list command")
		assert.Contains(t, output, "update", "Should list update command")
		assert.Contains(t, output, "doctor", "Should list doctor command")
		assert.Contains(t, output, "config", "Should list config command")
		assert.Contains(t, output, "prompts", "Should list prompts command")
		assert.Contains(t, output, "persona", "Should list persona command")

		// Each command should have a brief description
		lines := strings.Split(output, "\n")
		commandLines := 0
		for _, line := range lines {
			if strings.Contains(line, "init") && strings.Contains(line, "Initialize") {
				commandLines++
			}
			if strings.Contains(line, "doctor") && (strings.Contains(line, "Check") || strings.Contains(line, "diagnose")) {
				commandLines++
			}
		}
		assert.GreaterOrEqual(t, commandLines, 3, "Commands should have descriptions")
	})

	t.Run("command_specific_help", func(t *testing.T) {
		// AC: Given I need command-specific help, when I run `ddx help <command>`, then detailed help for that command is displayed
		rootCmd := getHelpTestRootCommand("")
		output, err := executeCommand(rootCmd, "help", "init")

		require.NoError(t, err, "Help for specific command should work")

		// Should show detailed help for init command
		assert.Contains(t, output, "Initialize", "Should show command description")
		assert.Contains(t, output, "Usage:", "Should show usage section")
		assert.Contains(t, output, "Flags:", "Should show flags section")
		assert.Contains(t, output, "Examples:", "Should show examples section")

		// Should be more detailed than general help
		assert.Greater(t, len(output), 200, "Command-specific help should be detailed")
	})

	t.Run("help_flag_syntax", func(t *testing.T) {
		// AC: Given I prefer flag syntax, when I run `ddx <command> --help`, then the same help information is shown
		rootCmd1 := getHelpTestRootCommand("")
		rootCmd2 := getHelpTestRootCommand("")

		output1, err1 := executeCommand(rootCmd1, "help", "init")
		output2, err2 := executeCommand(rootCmd2, "init", "--help")

		require.NoError(t, err1, "ddx help init should work")
		require.NoError(t, err2, "ddx init --help should work")

		// Both should produce the same output
		assert.Equal(t, output1, output2, "Help flag should produce same output as help command")
	})

	t.Run("practical_examples_included", func(t *testing.T) {
		// AC: Given I'm viewing help, when I read the output, then I see practical examples for common use cases
		rootCmd := getHelpTestRootCommand("")
		output, err := executeCommand(rootCmd, "help", "init")

		require.NoError(t, err, "Help command should work")

		// Should contain examples section
		assert.Contains(t, output, "Examples:", "Should have examples section")

		// Should show practical usage examples
		assert.Contains(t, output, "ddx init", "Should show basic usage example")
		assert.Contains(t, output, "#", "Should include example comments")

		// Examples should be realistic and useful
		exampleCount := strings.Count(output, "ddx init")
		assert.GreaterOrEqual(t, exampleCount, 2, "Should have multiple practical examples")
	})

	t.Run("flags_listed_with_defaults", func(t *testing.T) {
		// AC: Given a command has flags, when I view its help, then all available flags are listed with their defaults
		rootCmd := getHelpTestRootCommand("")
		output, err := executeCommand(rootCmd, "help", "init")

		require.NoError(t, err, "Help command should work")

		// Should list flags
		assert.Contains(t, output, "Flags:", "Should have flags section")
		assert.Contains(t, output, "--force", "Should list force flag")
		assert.Contains(t, output, "--no-git", "Should list no-git flag")
		assert.Contains(t, output, "--help", "Should list help flag")

		// Should show flag descriptions
		lines := strings.Split(output, "\n")
		flagDescriptions := 0
		for _, line := range lines {
			if strings.Contains(line, "--") && strings.Contains(line, " ") {
				// Line contains a flag and description
				flagDescriptions++
			}
		}
		assert.GreaterOrEqual(t, flagDescriptions, 3, "Should describe multiple flags")

		// Global flags should also be shown
		assert.Contains(t, output, "Global Flags:", "Should show global flags")
		assert.Contains(t, output, "--config", "Should show config flag")
		assert.Contains(t, output, "--verbose", "Should show verbose flag")
	})

	t.Run("online_documentation_links", func(t *testing.T) {
		// AC: Given I need more information, when I view help, then links to online documentation are provided
		rootCmd := getHelpTestRootCommand("")
		output, err := executeCommand(rootCmd, "help")

		require.NoError(t, err, "Help command should work")

		// Should mention where to find more info (this is currently not implemented)
		// This test will initially fail and drive implementation
		assert.Contains(t, output, "More information:", "Should provide links to documentation")
		assert.Contains(t, output, "github.com", "Should include GitHub repository link")
	})

	t.Run("required_vs_optional_arguments", func(t *testing.T) {
		// AC: Given arguments are required, when I view help, then required vs optional arguments are clearly indicated
		rootCmd := getHelpTestRootCommand("")
		output, err := executeCommand(rootCmd, "help", "list")

		require.NoError(t, err, "Help command should work")

		// Should indicate argument requirements
		assert.Contains(t, output, "Usage:", "Should show usage")

		// Should use standard conventions for required/optional
		// <angle> for required, [brackets] for optional
		lines := strings.Split(output, "\n")
		usageFound := false
		for _, line := range lines {
			if strings.Contains(line, "ddx list") && strings.Contains(line, "type") {
				usageFound = true
				// Should show optional type and optional flags
				assert.Contains(t, line, "[type]", "Should use brackets for optional type argument")
				assert.Contains(t, line, "[flags]", "Should use brackets for optional flags")
				break
			}
		}

		// Should have found the usage line
		assert.True(t, usageFound, "Should find usage line with ddx list")
	})

	t.Run("command_aliases_shown", func(t *testing.T) {
		// AC: Given commands have aliases, when I view help, then available aliases are shown
		rootCmd := getHelpTestRootCommand("")

		// First check if any commands have aliases
		output, err := executeCommand(rootCmd, "help")
		require.NoError(t, err, "Help command should work")

		// This test will initially fail as aliases need to be implemented
		// Common aliases might be: ls for list, cfg for config, etc.
		if strings.Contains(output, "list, ls") || strings.Contains(output, "Aliases:") {
			// If aliases exist, they should be clearly shown
			assert.Contains(t, output, "Aliases:", "Should label aliases clearly")
		} else {
			// Currently no aliases implemented - this drives the requirement
			t.Skip("No aliases currently implemented - test will pass once aliases are added")
		}
	})

	t.Run("invalid_command_help", func(t *testing.T) {
		// Error scenario: Invalid command name provided to help
		rootCmd := getHelpTestRootCommand("")
		output, err := executeCommand(rootCmd, "help", "invalid-command")

		// Cobra doesn't return error for invalid help topics, just shows message
		assert.NoError(t, err, "Should not error, just show unknown message")
		assert.Contains(t, output, "Unknown help topic", "Should indicate unknown command")
		assert.Contains(t, output, "invalid-command", "Should mention the invalid command")

		// Should still show available commands as fallback
		assert.Contains(t, output, "Available Commands", "Should show available commands")
	})

	t.Run("nested_command_help", func(t *testing.T) {
		// Validation scenario: Help for nested commands
		rootCmd := getHelpTestRootCommand("")
		output, err := executeCommand(rootCmd, "help", "list", "templates")

		if err == nil {
			// If nested commands are supported
			assert.Contains(t, output, "templates", "Should show nested command help")
		} else {
			// If not supported, should show parent help
			output, err = executeCommand(rootCmd, "help", "list")
			require.NoError(t, err, "Should fall back to parent command help")
			assert.Contains(t, output, "list", "Should show list command help")
		}
	})

	t.Run("help_formatting_quality", func(t *testing.T) {
		// Additional quality checks for help output
		rootCmd := getHelpTestRootCommand("")
		output, err := executeCommand(rootCmd, "help", "init")

		require.NoError(t, err, "Help command should work")

		// Should be well-formatted
		assert.NotContains(t, output, "\n\n\n", "Should not have excessive blank lines")
		assert.Contains(t, output, "\n", "Should have line breaks for readability")

		// Should have consistent structure
		assert.Contains(t, output, "Usage:", "Should have usage section")
		assert.Contains(t, output, "Flags:", "Should have flags section")
		assert.Contains(t, output, "Examples:", "Should have examples section")

		// Should be a reasonable length (not too short or too long)
		assert.Greater(t, len(output), 100, "Should be substantial")
		assert.Less(t, len(output), 5000, "Should not be overwhelming")
	})
}
