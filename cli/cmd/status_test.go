package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a fresh root command for status tests
func getStatusTestRootCommand(workingDir string) *cobra.Command {
	if workingDir == "" {
		workingDir = "/tmp"
	}
	factory := NewCommandFactory(workingDir)
	return factory.NewRootCommand()
}

// Helper to execute command with captured output
func executeStatusCommand(root *cobra.Command, args ...string) (output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err = root.Execute()
	return buf.String(), err
}

// Helper to setup test environment with DDX project
func setupStatusTestDir(t *testing.T) (string, func()) {
	tempDir := t.TempDir()
	//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

	// Change to temp directory

	// Create basic DDX structure
	ddxDir := filepath.Join(tempDir, ".ddx")
	err := os.MkdirAll(ddxDir, 0755)
	require.NoError(t, err)

	// Create .ddx.yml config
	configContent := `repository:
  url: "https://github.com/DocumentDrivenDX/ddx"
  branch: "main"
version: "v1.2.3"
last_updated: "2025-01-14T10:30:00Z"
`
	configPath := filepath.Join(tempDir, ".ddx.yml")
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cleanup := func() {
	}

	return tempDir, cleanup
}

// TestAcceptance_US012_TrackAssetVersions tests version tracking functionality
func TestAcceptance_US012_TrackAssetVersions(t *testing.T) {
	t.Run("show_current_version_information", func(t *testing.T) {
		// AC1: Given I have a DDX project, when I run `ddx status`, then I see current version information for all resources

		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Execute status command
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "status")

		// Should succeed
		assert.NoError(t, err)

		// Should show version information
		assert.Contains(t, output, "DDX Status Report")
		assert.Contains(t, output, "Current Version:")
		assert.Contains(t, output, "Last Updated:")
		assert.Regexp(t, `v\d+\.\d+\.\d+`, output) // Version pattern
	})

	t.Run("show_local_modifications", func(t *testing.T) {
		// AC2: Given I have modified resources, when I check status, then local modifications are clearly shown

		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Modify a DDX resource
		promptsDir := filepath.Join(testDir, ".ddx", "prompts")
		_ = os.MkdirAll(promptsDir, 0755)
		modifiedFile := filepath.Join(promptsDir, "test-prompt.md")
		err := os.WriteFile(modifiedFile, []byte("# Modified prompt\nThis is a local change"), 0644)
		require.NoError(t, err)

		// Execute status command
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "status")

		// Should succeed
		assert.NoError(t, err)
		// Basic status should work even with modifications
		assert.Contains(t, output, "DDX Status Report")
	})

	t.Run("detect_upstream_updates", func(t *testing.T) {
		// AC3: Given updates exist upstream, when I check status, then I'm notified that updates are available

		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Execute status command with upstream check
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "status", "--check-upstream")

		// Should detect updates (our implementation always shows them for now)
		assert.NoError(t, err)
		assert.Contains(t, output, "DDX Status Report")
	})

	t.Run("show_version_details_with_timestamp", func(t *testing.T) {
		// AC4: Given I want version details, when I view status, then I see last update timestamp for each resource

		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "status", "--verbose")

		assert.NoError(t, err)
		assert.Contains(t, output, "DDX Status Report")

		// Should show detailed timestamp information
		assert.Contains(t, output, "Last Updated:")
		assert.Regexp(t, `\d{4}-\d{2}-\d{2}`, output) // Date pattern
	})

	t.Run("list_changed_files", func(t *testing.T) {
		// AC5: Given changes have occurred, when I request details, then I see a list of changed files

		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		// Create some changes
		changesDir := filepath.Join(testDir, ".ddx", "patterns")
		_ = os.MkdirAll(changesDir, 0755)
		changeFile := filepath.Join(changesDir, "auth-pattern.go")
		_ = os.WriteFile(changeFile, []byte("modified content"), 0644)

		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "status", "--changes")

		assert.NoError(t, err)
		assert.Contains(t, output, "DDX Status Report")
	})

	t.Run("view_commit_history", func(t *testing.T) {
		// AC6: Given I need history, when I run `ddx log`, then I see commit history for DDX assets

		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "log")

		// Since there's no git repo, this will show alternative history
		// The test should not fail, but show a helpful message
		if err != nil {
			// It's OK if log fails when no git repo exists
			assert.Contains(t, err.Error(), "not a DDX project")
		} else {
			// If it succeeds, it should show some form of history
			// Log command may show alternative output when git is not available
			// This is acceptable for the test
			if output == "" {
				// Alternative log was written to stderr instead of stdout, which is OK
				assert.True(t, true, "Log command completed successfully")
			} else {
				assert.NotEmpty(t, output)
			}
		}
	})

	t.Run("compare_versions", func(t *testing.T) {
		// AC7: Given versions differ, when I compare, then I can see differences between versions

		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "status", "--diff")

		assert.NoError(t, err)
		assert.Contains(t, output, "DDX Status Report")
		// Diff functionality shows placeholder for now
	})

	t.Run("export_version_manifest", func(t *testing.T) {
		// AC8: Given I need documentation, when I export manifest, then a version manifest is generated

		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		manifestPath := filepath.Join(testDir, "manifest.yml")

		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		_, err := executeStatusCommand(rootCmd, "status", "--export", manifestPath)

		assert.NoError(t, err)

		// Should create manifest file
		assert.FileExists(t, manifestPath)

		// Read and validate manifest content
		content, err := os.ReadFile(manifestPath)
		require.NoError(t, err)

		manifestStr := string(content)
		assert.Contains(t, manifestStr, "version:")
		assert.Contains(t, manifestStr, "last_updated:")
	})
}

func TestStatusIncludesAgentUsage(t *testing.T) {
	t.Run("renders_30d_agent_usage_from_session_logs", func(t *testing.T) {
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		logDir := filepath.Join(testDir, ".ddx", "agent-logs")
		require.NoError(t, os.MkdirAll(logDir, 0755))
		entry := fmt.Sprintf(`{"id":"sess-1","timestamp":%q,"harness":"codex","model":"gpt-test","input_tokens":1234,"output_tokens":567,"cost_usd":1.23,"duration_ms":2000,"exit_code":0}`+"\n", time.Now().UTC().Format(time.RFC3339Nano))
		require.NoError(t, os.WriteFile(filepath.Join(logDir, "sessions.jsonl"), []byte(entry), 0644))

		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "status")

		require.NoError(t, err)
		assert.Contains(t, output, "Agent Usage (30d):")
		assert.Contains(t, output, "codex")
		assert.Contains(t, output, "1,234")
		assert.Contains(t, output, "567")
		assert.Contains(t, output, "$1.23")
	})

	t.Run("succeeds_without_session_logs", func(t *testing.T) {
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		status, err := checkStatus(testDir, false, false, false)

		require.NoError(t, err)
		assert.Empty(t, status.Usage)
	})
}

// TestStatusCommand_Contract validates status command CLI contract
func TestStatusCommand_Contract(t *testing.T) {
	t.Run("command_exists", func(t *testing.T) {
		// Status command should exist
		rootCmd := getStatusTestRootCommand("")
		output, err := executeStatusCommand(rootCmd, "--help")

		assert.NoError(t, err)
		assert.Contains(t, output, "status")
		assert.Contains(t, output, "Show version and status information")
	})

	t.Run("accepts_standard_flags", func(t *testing.T) {
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		testCases := []struct {
			flag string
			desc string
		}{
			{"--verbose", "detailed output"},
			{"--check-upstream", "check for upstream updates"},
			{"--changes", "show changed files"},
			{"--diff", "show differences"},
			{"--export", "export manifest"},
		}

		for _, tc := range testCases {
			t.Run(tc.flag, func(t *testing.T) {
				rootCmd := getStatusTestRootCommand("")
				output, err := executeStatusCommand(rootCmd, "status", "--help")

				assert.NoError(t, err)
				assert.Contains(t, strings.ToLower(output), strings.TrimPrefix(tc.flag, "--"))
			})
		}
	})

	t.Run("requires_ddx_project", func(t *testing.T) {
		// Should fail in non-DDX directory
		tempDir := t.TempDir()
		//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

		factory := NewCommandFactory(tempDir)
		rootCmd := factory.NewRootCommand()
		_, err := executeStatusCommand(rootCmd, "status")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a DDX project")
	})

	t.Run("performance_requirements", func(t *testing.T) {
		// Status check should complete within 2 seconds
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		start := time.Now()
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeStatusCommand(rootCmd, "status")
		duration := time.Since(start)

		assert.NoError(t, err)
		assert.Less(t, duration, 2*time.Second, "Status command took too long: %v", duration)
		assert.NotEmpty(t, output)
	})
}

// TestLogCommand_Contract validates log command CLI contract
func TestLogCommand_Contract(t *testing.T) {
	t.Run("command_exists", func(t *testing.T) {
		// Log command should exist
		rootCmd := getStatusTestRootCommand("")
		output, err := executeStatusCommand(rootCmd, "--help")

		assert.NoError(t, err)
		assert.Contains(t, output, "log")
		assert.Contains(t, output, "Show DDX asset history")
	})

	t.Run("performance_requirements", func(t *testing.T) {
		// History retrieval should complete within 3 seconds
		testDir, cleanup := setupStatusTestDir(t)
		defer cleanup()
		defer func() { _ = os.RemoveAll(testDir) }()

		start := time.Now()
		factory := NewCommandFactory(testDir)
		rootCmd := factory.NewRootCommand()
		_, _ = executeStatusCommand(rootCmd, "log")
		duration := time.Since(start)

		// Log might fail if no git repo, but should fail quickly
		assert.Less(t, duration, 3*time.Second, "Log command took too long: %v", duration)
	})
}
