package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestInitRegistersSkills verifies that ddx init copies bootstrap skills to project-local directories.
func TestInitRegistersSkills(t *testing.T) {
	te := NewTestEnvironment(t, WithGitInit(false))
	_, err := te.RunCommand("init", "--no-git")
	require.NoError(t, err)

	// Bootstrap skills should be copied as real files to all three project directories
	bootstrapSkills := []string{"ddx-doctor", "ddx-run"}
	targetDirs := []string{
		filepath.Join(te.Dir, ".ddx", "skills"),
		filepath.Join(te.Dir, ".agents", "skills"),
		filepath.Join(te.Dir, ".claude", "skills"),
	}

	for _, dir := range targetDirs {
		for _, name := range bootstrapSkills {
			skillFile := filepath.Join(dir, name, "SKILL.md")
			assert.FileExists(t, skillFile, "skill file should exist at %s", skillFile)
		}
	}
}

// TestCleanupBootstrapSkills_RemovesStaleSkills verifies stale ddx-* skills are removed.
func TestCleanupBootstrapSkills_RemovesStaleSkills(t *testing.T) {
	targetDir := t.TempDir()

	// Create a stale ddx-* skill (no longer in bootstrap list)
	staleDir := filepath.Join(targetDir, "ddx-stale")
	require.NoError(t, os.MkdirAll(staleDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staleDir, "SKILL.md"), []byte("# Stale"), 0o644))

	// Create a kept skill
	keepDir := filepath.Join(targetDir, "ddx-doctor")
	require.NoError(t, os.MkdirAll(keepDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(keepDir, "SKILL.md"), []byte("# Doctor"), 0o644))

	cleanupBootstrapSkills(targetDir, []string{"ddx-doctor"})

	_, err := os.Stat(staleDir)
	assert.True(t, os.IsNotExist(err), "stale ddx-* skill should be removed")
	assert.DirExists(t, keepDir, "kept ddx-* skill should remain")
}

// TestCleanupBootstrapSkills_PreservesNonDdxDirs verifies non-ddx- dirs are untouched.
func TestCleanupBootstrapSkills_PreservesNonDdxDirs(t *testing.T) {
	targetDir := t.TempDir()

	otherDir := filepath.Join(targetDir, "helix-align")
	require.NoError(t, os.MkdirAll(otherDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(otherDir, "SKILL.md"), []byte("# Helix"), 0o644))

	cleanupBootstrapSkills(targetDir, []string{"ddx-doctor"})

	assert.DirExists(t, otherDir, "non-ddx- skill should not be removed")
}

// TestCleanupBootstrapSkills_SkipsDirsWithoutSKILLMD verifies dirs without SKILL.md are untouched.
func TestCleanupBootstrapSkills_SkipsDirsWithoutSKILLMD(t *testing.T) {
	targetDir := t.TempDir()

	noSkillDir := filepath.Join(targetDir, "ddx-no-skill")
	require.NoError(t, os.MkdirAll(noSkillDir, 0o755))
	// no SKILL.md written

	cleanupBootstrapSkills(targetDir, []string{"ddx-doctor"})

	assert.DirExists(t, noSkillDir, "ddx-* dir without SKILL.md should not be removed")
}

// TestRegisterProjectSkills_CleansUpStaleBootstrapSkills verifies stale bootstrap skills
// are removed when registerProjectSkills is called with an updated bootstrap list.
func TestRegisterProjectSkills_CleansUpStaleBootstrapSkills(t *testing.T) {
	workingDir := t.TempDir()

	// Manually plant a stale bootstrap skill in all three target directories
	targetDirs := []string{
		filepath.Join(workingDir, ".ddx", "skills"),
		filepath.Join(workingDir, ".agents", "skills"),
		filepath.Join(workingDir, ".claude", "skills"),
	}
	for _, dir := range targetDirs {
		staleDir := filepath.Join(dir, "ddx-stale-old")
		require.NoError(t, os.MkdirAll(staleDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(staleDir, "SKILL.md"), []byte("# Old"), 0o644))
	}

	// registerProjectSkills uses the current bootstrap list which does not include ddx-stale-old
	registerProjectSkills(workingDir, false)

	// Stale skill must be cleaned up in every target directory
	for _, dir := range targetDirs {
		staleDir := filepath.Join(dir, "ddx-stale-old")
		_, err := os.Stat(staleDir)
		assert.True(t, os.IsNotExist(err), "stale skill ddx-stale-old should be removed from %s", dir)
	}

	// Current bootstrap skills must be present in every target directory
	for _, dir := range targetDirs {
		for _, skill := range []string{"ddx-doctor", "ddx-run"} {
			skillFile := filepath.Join(dir, skill, "SKILL.md")
			assert.FileExists(t, skillFile, "bootstrap skill %s should exist in %s", skill, dir)
		}
	}
}

// TestInitSkillsNoOverwrite verifies that existing skill files are not overwritten.
func TestInitSkillsNoOverwrite(t *testing.T) {
	te := NewTestEnvironment(t, WithGitInit(false))

	// Pre-create a skill file with custom content
	skillDir := filepath.Join(te.Dir, ".agents", "skills", "ddx-doctor")
	require.NoError(t, os.MkdirAll(skillDir, 0755))
	existingContent := "# custom content"
	skillFile := filepath.Join(skillDir, "SKILL.md")
	require.NoError(t, os.WriteFile(skillFile, []byte(existingContent), 0644))

	_, err := te.RunCommand("init", "--no-git")
	require.NoError(t, err)

	// Existing file must not be overwritten
	data, err := os.ReadFile(skillFile)
	require.NoError(t, err)
	assert.Equal(t, existingContent, string(data), "existing skill file should not be overwritten")
}

// TestInitCommand tests the init command
func TestInitCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		envOptions  []TestEnvOption
		setup       func(t *testing.T, te *TestEnvironment)
		validate    func(t *testing.T, te *TestEnvironment, output string, err error)
		expectError bool
	}{
		{
			name:       "basic initialization",
			args:       []string{"init", "--no-git"},
			envOptions: []TestEnvOption{WithGitInit(false)},
			validate: func(t *testing.T, te *TestEnvironment, output string, cmdErr error) {
				// Check .ddx/config.yaml was created
				assert.FileExists(t, te.ConfigPath)

				// Verify config content
				data, err := os.ReadFile(te.ConfigPath)
				require.NoError(t, err)

				var config map[string]interface{}
				err = yaml.Unmarshal(data, &config)
				require.NoError(t, err)

				assert.Contains(t, config, "version")
				assert.Contains(t, config, "library")
				if library, ok := config["library"].(map[string]interface{}); ok {
					assert.Contains(t, library, "repository")
				}
			},
			expectError: false,
		},
		{
			name:       "init with force flag when config exists",
			args:       []string{"init", "--force", "--no-git"},
			envOptions: []TestEnvOption{WithGitInit(false)},
			setup: func(t *testing.T, te *TestEnvironment) {
				// Create existing config in new format
				existingConfig := `version: "0.9"
library:
  path: "./library"
  repository:
    url: "https://old.repo"
    branch: "main"
    subtree: "library"
persona_bindings: {}
`
				te.CreateConfig(existingConfig)
			},
			validate: func(t *testing.T, te *TestEnvironment, output string, cmdErr error) {
				// Config should be overwritten
				data, err := os.ReadFile(te.ConfigPath)
				require.NoError(t, err)

				var config map[string]interface{}
				err = yaml.Unmarshal(data, &config)
				require.NoError(t, err)

				// With --force flag, creates new config with default version
				assert.Equal(t, "1.0", config["version"])
			},
			expectError: false,
		},
		{
			name:       "init without force when config exists",
			args:       []string{"init", "--no-git"},
			envOptions: []TestEnvOption{WithGitInit(false)},
			setup: func(t *testing.T, te *TestEnvironment) {
				// Create existing config
				te.CreateConfig("version: \"1.0\"")
			},
			validate: func(t *testing.T, te *TestEnvironment, output string, err error) {
				// Should fail
				assert.Error(t, err)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := NewTestEnvironment(t, tt.envOptions...)

			if tt.setup != nil {
				tt.setup(t, te)
			}

			output, err := te.RunCommand(tt.args...)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.validate != nil {
				tt.validate(t, te, output, err)
			}
		})
	}
}

// TestInitCommand_Help tests the help output
func TestInitCommand_Help(t *testing.T) {
	te := NewTestEnvironment(t)
	output, err := te.RunCommand("init", "--help")

	assert.NoError(t, err)
	assert.Contains(t, output, "Initialize DDx")
	assert.Contains(t, output, "--force")
	assert.Contains(t, output, "--no-git")
}

// TestInitCommand_US017_InitializeConfiguration tests US-017 Initialize Configuration
func TestInitCommand_US017_InitializeConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		envOptions     []TestEnvOption
		setup          func(t *testing.T, te *TestEnvironment)
		args           []string
		validateOutput func(t *testing.T, te *TestEnvironment, output string, err error)
		expectError    bool
	}{
		{
			name:       "creates_initial_config_with_sensible_defaults",
			args:       []string{"init", "--no-git"},
			envOptions: []TestEnvOption{WithGitInit(false)},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				// Should create .ddx/config.yaml with sensible defaults
				assert.FileExists(t, te.ConfigPath, "Should create .ddx/config.yaml")

				data, err := os.ReadFile(te.ConfigPath)
				require.NoError(t, err)

				var config map[string]interface{}
				err = yaml.Unmarshal(data, &config)
				require.NoError(t, err)

				assert.Contains(t, config, "version")
				assert.Contains(t, config, "library")
				if library, ok := config["library"].(map[string]interface{}); ok {
					assert.Contains(t, library, "repository")
				}
			},
			expectError: false,
		},
		{
			name:       "detects_project_type_javascript",
			args:       []string{"init", "--no-git"},
			envOptions: []TestEnvOption{WithGitInit(false)},
			setup: func(t *testing.T, te *TestEnvironment) {
				// Create package.json to simulate JavaScript project
				te.CreateFile("package.json", `{"name": "test"}`)
			},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				data, err := os.ReadFile(te.ConfigPath)
				require.NoError(t, err)

				var config map[string]interface{}
				err = yaml.Unmarshal(data, &config)
				require.NoError(t, err)

				// Project type detection has been removed - init just creates basic config
				assert.Contains(t, config, "version")
				assert.Contains(t, config, "library")
			},
			expectError: false,
		},
		{
			name:       "detects_project_type_go",
			args:       []string{"init", "--no-git"},
			envOptions: []TestEnvOption{WithGitInit(false)},
			setup: func(t *testing.T, te *TestEnvironment) {
				// Create go.mod to simulate Go project
				te.CreateFile("go.mod", "module test")
			},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				data, err := os.ReadFile(te.ConfigPath)
				require.NoError(t, err)

				var config map[string]interface{}
				err = yaml.Unmarshal(data, &config)
				require.NoError(t, err)

				// Project type detection has been removed - init just creates basic config
				assert.Contains(t, config, "version")
				assert.Contains(t, config, "library")
			},
			expectError: false,
		},
		{
			name:       "validates_configuration_during_creation",
			args:       []string{"init", "--no-git"},
			envOptions: []TestEnvOption{WithGitInit(false)},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				// Should pass validation without error
				assert.NoError(t, err, "Configuration validation should pass")
				assert.Contains(t, output, "✅ DDx initialized successfully!")
			},
			expectError: false,
		},
		{
			name: "force_overwrites_without_backup",
			args: []string{"init", "--force"},
			setup: func(t *testing.T, te *TestEnvironment) {
				// Create existing config with repository URL
				existingConfig := fmt.Sprintf(`version: "0.9"
library:
  path: .ddx/plugins/ddx
  repository:
    url: %s
    branch: master
`, te.TestLibraryURL)
				te.CreateConfig(existingConfig)
				te.CreateFile("README.md", "# Test Project")

				gitAdd := exec.Command("git", "add", ".")
				gitAdd.Dir = te.Dir
				require.NoError(t, gitAdd.Run())

				gitCommit := exec.Command("git", "commit", "-m", "Initial commit")
				gitCommit.Dir = te.Dir
				require.NoError(t, gitCommit.Run())
			},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				// Should NOT create backup or show backup message
				assert.NotContains(t, output, "💾 Created backup of existing config")
				assert.NotContains(t, output, "backup")

				// Should NOT have backup file
				backupFiles, _ := filepath.Glob(filepath.Join(te.Dir, ".ddx", "config.yaml.backup.*"))
				assert.Equal(t, 0, len(backupFiles), "Should not create backup file")

				// Should successfully overwrite config
				assert.Contains(t, output, "✅ DDx initialized successfully!")
			},
			expectError: false,
		},
		{
			name:       "no_git_flag_functionality",
			args:       []string{"init", "--no-git"},
			envOptions: []TestEnvOption{WithGitInit(true)},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				// Should create config successfully without git operations
				assert.FileExists(t, te.ConfigPath, "Should create config with --no-git flag")
			},
			expectError: false,
		},
		{
			name: "includes_example_variable_definitions",
			args: []string{"init", "--silent"},
			setup: func(t *testing.T, te *TestEnvironment) {
				// Create initial commit
				te.CreateFile("README.md", "# Test Project")
				gitAdd := exec.Command("git", "add", ".")
				gitAdd.Dir = te.Dir
				require.NoError(t, gitAdd.Run())
				gitCommit := exec.Command("git", "commit", "-m", "Initial commit")
				gitCommit.Dir = te.Dir
				require.NoError(t, gitCommit.Run())
			},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				data, err := os.ReadFile(te.ConfigPath)
				require.NoError(t, err)

				var config map[string]interface{}
				err = yaml.Unmarshal(data, &config)
				require.NoError(t, err)

				// Variable definitions have been removed - init creates minimal config
				assert.Contains(t, config, "version")
				assert.Contains(t, config, "library")
			},
			expectError: false,
		},
		{
			name: "commits_config_file_to_git",
			args: []string{"init", "--silent"},
			setup: func(t *testing.T, te *TestEnvironment) {
				// Create initial commit
				te.CreateFile("README.md", "# Test Project")

				gitAdd := exec.Command("git", "add", "README.md")
				gitAdd.Dir = te.Dir
				require.NoError(t, gitAdd.Run())

				gitCommit := exec.Command("git", "commit", "-m", "Initial commit")
				gitCommit.Dir = te.Dir
				require.NoError(t, gitCommit.Run())
			},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				// Config file should be created
				assert.FileExists(t, te.ConfigPath, "Config file should exist")

				// Check that config file is tracked in git
				gitLsFiles := exec.Command("git", "ls-files", ".ddx/config.yaml")
				gitLsFiles.Dir = te.Dir
				lsOutput, err := gitLsFiles.CombinedOutput()
				require.NoError(t, err, "Should be able to check git ls-files")

				assert.Contains(t, string(lsOutput), ".ddx/config.yaml", "Config file should be tracked in git")

				// Verify library directory structure exists (init creates it even if sync fails)
				assert.DirExists(t, filepath.Join(te.Dir, ".ddx", "plugins", "ddx"), "Library directory should exist")
				assert.DirExists(t, filepath.Join(te.Dir, ".ddx", "plugins", "ddx", "prompts"), "Prompts directory should exist")
			},
			expectError: false,
		},
		{
			name:       "skips_commit_with_no_git_flag",
			args:       []string{"init", "--no-git"},
			envOptions: []TestEnvOption{WithGitInit(true)},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				// Config file should be created
				assert.FileExists(t, te.ConfigPath, "Config file should exist")

				// Git log should not have config commit (--no-git skips commits)
				gitLog := exec.Command("git", "log", "--oneline", "--all")
				gitLog.Dir = te.Dir
				logOutput, _ := gitLog.CombinedOutput()
				logStr := string(logOutput)

				// With --no-git, no commits should be made
				assert.Empty(t, logStr, "Should have no commits with --no-git flag")
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := NewTestEnvironment(t, tt.envOptions...)

			if tt.setup != nil {
				tt.setup(t, te)
			}

			output, err := te.RunCommand(tt.args...)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.validateOutput != nil {
				tt.validateOutput(t, te, output, err)
			}
		})
	}
}

// TestInitCommand_US014_SynchronizationSetup tests US-014 synchronization initialization
func TestInitCommand_US014_SynchronizationSetup(t *testing.T) {
	tests := []struct {
		name           string
		envOptions     []TestEnvOption
		setup          func(t *testing.T, te *TestEnvironment)
		args           []string
		validateOutput func(t *testing.T, te *TestEnvironment, output string, err error)
		expectError    bool
	}{
		{
			name: "basic_sync_initialization",
			args: []string{"init", "--silent"},
			setup: func(t *testing.T, te *TestEnvironment) {
				// Create initial commit
				te.CreateFile("README.md", "# Test")
				gitAdd := exec.Command("git", "add", ".")
				gitAdd.Dir = te.Dir
				require.NoError(t, gitAdd.Run())
				gitCommit := exec.Command("git", "commit", "-m", "Initial commit")
				gitCommit.Dir = te.Dir
				require.NoError(t, gitCommit.Run())
			},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				// Verify config is created
				assert.FileExists(t, te.ConfigPath, "Should create config file")
			},
			expectError: false,
		},
		{
			name: "sync_initialization_with_custom_repository",
			args: []string{"init", "--force", "--silent"},
			setup: func(t *testing.T, te *TestEnvironment) {
				// Create existing config with repository URL
				existingConfig := fmt.Sprintf(`version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: %s
    branch: master
`, te.TestLibraryURL)
				te.CreateConfig(existingConfig)
				te.CreateFile("README.md", "# Test")
				gitAdd := exec.Command("git", "add", ".")
				gitAdd.Dir = te.Dir
				require.NoError(t, gitAdd.Run())
				gitCommit := exec.Command("git", "commit", "-m", "Initial commit")
				gitCommit.Dir = te.Dir
				require.NoError(t, gitCommit.Run())
			},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				// Should handle custom repository successfully
				assert.NotContains(t, output, "backup", "Should not create or mention backup")
			},
			expectError: false,
		},
		{
			name: "sync_initialization_fresh_project",
			args: []string{"init", "--silent"},
			setup: func(t *testing.T, te *TestEnvironment) {
				// Create initial commit
				te.CreateFile("README.md", "# Test")
				gitAdd := exec.Command("git", "add", ".")
				gitAdd.Dir = te.Dir
				require.NoError(t, gitAdd.Run())
				gitCommit := exec.Command("git", "commit", "-m", "Initial commit")
				gitCommit.Dir = te.Dir
				require.NoError(t, gitCommit.Run())
			},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				// Check .ddx/config.yaml was created with sync config
				assert.FileExists(t, te.ConfigPath)

				data, err := os.ReadFile(te.ConfigPath)
				require.NoError(t, err)

				var config map[string]interface{}
				err = yaml.Unmarshal(data, &config)
				require.NoError(t, err)

				assert.Contains(t, config, "library")
				if library, ok := config["library"].(map[string]interface{}); ok {
					assert.Contains(t, library, "repository")
					if repo, ok := library["repository"].(map[string]interface{}); ok {
						assert.Contains(t, repo, "url")
						assert.Contains(t, repo, "branch")
					}
				}
			},
			expectError: false,
		},
		{
			name: "sync_initialization_existing_project",
			args: []string{"init", "--force", "--silent"},
			setup: func(t *testing.T, te *TestEnvironment) {
				// Create existing project files
				te.CreateFile("README.md", "# Existing Project")
				te.CreateFile("package.json", `{"name": "test"}`)
				gitAdd := exec.Command("git", "add", ".")
				gitAdd.Dir = te.Dir
				require.NoError(t, gitAdd.Run())
				gitCommit := exec.Command("git", "commit", "-m", "Initial commit")
				gitCommit.Dir = te.Dir
				require.NoError(t, gitCommit.Run())
			},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				// Existing files should remain untouched
				assert.FileExists(t, filepath.Join(te.Dir, "README.md"))
				assert.FileExists(t, filepath.Join(te.Dir, "package.json"))
			},
			expectError: false,
		},
		{
			name: "sync_initialization_validation_success",
			args: []string{"init", "--silent"},
			setup: func(t *testing.T, te *TestEnvironment) {
				te.CreateFile("README.md", "# Test")
				gitAdd := exec.Command("git", "add", ".")
				gitAdd.Dir = te.Dir
				require.NoError(t, gitAdd.Run())
				gitCommit := exec.Command("git", "commit", "-m", "Initial commit")
				gitCommit.Dir = te.Dir
				require.NoError(t, gitCommit.Run())
			},
			validateOutput: func(t *testing.T, te *TestEnvironment, output string, err error) {
				// Verify config file exists with proper structure
				assert.FileExists(t, te.ConfigPath, "Should create config file")
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := NewTestEnvironment(t, tt.envOptions...)

			if tt.setup != nil {
				tt.setup(t, te)
			}

			output, err := te.RunCommand(tt.args...)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.validateOutput != nil {
				tt.validateOutput(t, te, output, err)
			}
		})
	}
}
