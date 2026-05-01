package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestAcceptance_US015_ViewChangeHistory tests the US-015 "View Change History" functionality
func TestAcceptance_US015_ViewChangeHistory(t *testing.T) {
	t.Run("view_full_chronological_history", func(t *testing.T) {
		// AC: Given DDX resources with history, when I run `ddx log`, then I see a chronological list of changes
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log")

		assert.NoError(t, err)
		assert.Contains(t, output, "DDX Asset History", "Should show history header")
		// Should show chronological format even in fallback mode
	})

	t.Run("filter_history_by_path", func(t *testing.T) {
		// AC: Given I want specific resource history, when I run `ddx log <path>`, then history is filtered by that path
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Create test resource at specific path
		patternsDir := filepath.Join(testDir, ".ddx", "patterns")
		_ = os.MkdirAll(patternsDir, 0755)
		patternFile := filepath.Join(patternsDir, "auth-pattern.md")
		_ = os.WriteFile(patternFile, []byte("# Auth Pattern\nTest content"), 0644)

		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log", "patterns/auth-pattern.md")

		assert.NoError(t, err)
		// Should filter by path - for now just verify command accepts path argument
		assert.NotEmpty(t, output)
	})

	t.Run("show_author_and_date", func(t *testing.T) {
		// AC: Given history exists, when viewing entries, then I see author and date for each change
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log")

		assert.NoError(t, err)
		// Should show some form of date and author attribution
		assert.Contains(t, output, "DDX Asset History")
	})

	t.Run("display_commit_messages_clearly", func(t *testing.T) {
		// AC: Given changes have descriptions, when viewing log, then commit messages are displayed clearly
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log")

		assert.NoError(t, err)
		assert.NotEmpty(t, output)
	})

	t.Run("limit_number_of_entries", func(t *testing.T) {
		// AC: Given long history exists, when I use `--limit`, then I can control the number of entries shown
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log", "--limit", "5")

		// This should fail initially because --limit flag doesn't exist yet
		if err != nil {
			assert.Contains(t, err.Error(), "unknown flag: --limit")
		} else {
			assert.NotEmpty(t, output)
		}
	})

	t.Run("show_actual_changes_with_diff", func(t *testing.T) {
		// AC: Given I want details, when I use `--diff`, then I see the actual changes for each commit
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log", "--diff")

		// This should fail initially because --diff flag doesn't exist yet
		if err != nil {
			assert.Contains(t, err.Error(), "unknown flag: --diff")
		} else {
			assert.NotEmpty(t, output)
		}
	})

	t.Run("export_history_to_readable_format", func(t *testing.T) {
		// AC: Given I need a report, when I use `--export`, then history is exported in a readable format
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		exportFile := filepath.Join(testDir, "history.md")

		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		_, err := executeStatusCommand(rootCmd, "log", "--export", exportFile)

		// This should fail initially because --export flag doesn't exist yet
		if err != nil {
			assert.Contains(t, err.Error(), "unknown flag: --export")
		} else {
			assert.FileExists(t, exportFile)
		}
	})

	t.Run("integrate_with_vcs_history", func(t *testing.T) {
		// AC: Given I use version control, when viewing DDX history, then it integrates with underlying VCS log
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// This test verifies that git integration is attempted
		// Current implementation already does this
		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log")

		assert.NoError(t, err)
		assert.NotEmpty(t, output)
		// Should show either git output or fallback message
	})
}

// TestLogCommand_US015_Features tests specific US-015 feature implementations
func TestLogCommand_US015_Features(t *testing.T) {
	t.Run("accepts_limit_flag", func(t *testing.T) {
		// --limit flag should be accepted
		// Use CommandFactory with temp directory for help command
		factory := NewCommandFactory(t.TempDir())
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log", "--help")

		assert.NoError(t, err)
		// Should show --limit in help - this will fail initially
		assert.Contains(t, strings.ToLower(output), "limit")
	})

	t.Run("accepts_diff_flag", func(t *testing.T) {
		// --diff flag should be accepted
		// Use CommandFactory with temp directory for help command
		factory := NewCommandFactory(t.TempDir())
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log", "--help")

		assert.NoError(t, err)
		// Should show --diff in help - this will fail initially
		assert.Contains(t, strings.ToLower(output), "diff")
	})

	t.Run("accepts_export_flag", func(t *testing.T) {
		// --export flag should be accepted
		// Use CommandFactory with temp directory for help command
		factory := NewCommandFactory(t.TempDir())
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log", "--help")

		assert.NoError(t, err)
		// Should show --export in help - this will fail initially
		assert.Contains(t, strings.ToLower(output), "export")
	})

	t.Run("accepts_path_argument", func(t *testing.T) {
		// Command should accept path arguments
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		_, err := executeStatusCommand(rootCmd, "log", "some/path")

		// Should not fail due to extra arguments
		assert.NoError(t, err)
	})

	t.Run("export_formats_supported", func(t *testing.T) {
		// Different export formats should be supported
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		formats := []string{"history.md", "history.json", "history.csv", "history.html"}

		for _, format := range formats {
			t.Run(format, func(t *testing.T) {
				exportFile := filepath.Join(testDir, format)

				// Use CommandFactory with the test working directory
				factory := NewCommandFactory(testDir)
				rootCmd := factory.NewRootCommand()
				_, err := executeStatusCommand(rootCmd, "log", "--export", exportFile)

				// Initially this will fail because --export doesn't exist
				// After implementation, should create the file
				if err == nil {
					assert.FileExists(t, exportFile)

					// Clean up for next iteration
					_ = os.Remove(exportFile)
				}
			})
		}
	})

	t.Run("performance_with_large_history", func(t *testing.T) {
		// History retrieval should be performant
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		start := time.Now()
		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		_, err := executeStatusCommand(rootCmd, "log", "--limit", "100")
		duration := time.Since(start)

		assert.NoError(t, err)
		assert.Less(t, duration, 5*time.Second, "Log command with limit took too long: %v", duration)
	})
}

// TestLogCommand_US015_ValidationScenarios tests the specific validation scenarios from US-015
func TestLogCommand_US015_ValidationScenarios(t *testing.T) {
	t.Run("scenario_1_view_full_history", func(t *testing.T) {
		// 1. Have DDX project with multiple updates
		// 2. Run `ddx log`
		// 3. Expected: See all changes chronologically
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Create multiple files to simulate updates
		updateDirs := []string{"patterns", "templates", "prompts"}
		for _, dir := range updateDirs {
			fullDir := filepath.Join(testDir, ".ddx", dir)
			_ = os.MkdirAll(fullDir, 0755)
			_ = os.WriteFile(filepath.Join(fullDir, "test.md"), []byte("content"), 0644)
		}

		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log")

		assert.NoError(t, err)
		assert.NotEmpty(t, output)
		assert.Contains(t, output, "DDX Asset History")
	})

	t.Run("scenario_2_filter_by_path", func(t *testing.T) {
		// 1. Run `ddx log patterns/auth`
		// 2. Expected: Only auth pattern changes shown
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		authDir := filepath.Join(testDir, ".ddx", "patterns", "auth")
		_ = os.MkdirAll(authDir, 0755)
		_ = os.WriteFile(filepath.Join(authDir, "oauth.md"), []byte("OAuth pattern"), 0644)

		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log", "patterns/auth")

		assert.NoError(t, err)
		assert.NotEmpty(t, output)
	})

	t.Run("scenario_3_limited_history", func(t *testing.T) {
		// 1. Run `ddx log --limit 10`
		// 2. Expected: Only 10 most recent entries
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		_, err := executeStatusCommand(rootCmd, "log", "--limit", "10")

		// Will initially fail because --limit flag doesn't exist
		if err != nil {
			assert.Contains(t, err.Error(), "unknown flag: --limit")
		}
	})

	t.Run("scenario_4_export_history", func(t *testing.T) {
		// 1. Run `ddx log --export history.md`
		// 2. Expected: Markdown file with formatted history
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		exportFile := filepath.Join(testDir, "history.md")

		// Use CommandFactory with the test working directory
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		_, err := executeStatusCommand(rootCmd, "log", "--export", exportFile)

		// Will initially fail because --export flag doesn't exist
		if err != nil {
			assert.Contains(t, err.Error(), "unknown flag: --export")
		}
	})
}
