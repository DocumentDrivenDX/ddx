package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestInitDoesNotCreateBootstrapDDxSkillMirror verifies the legacy
// .ddx/skills/ddx bootstrap mirror is no longer created. Skill outputs only
// live at the harness-visible paths .agents/skills/ddx and .claude/skills/ddx.
func TestInitDoesNotCreateBootstrapDDxSkillMirror(t *testing.T) {
	te := NewTestEnvironment(t, WithGitInit(false))
	_, err := te.RunCommand("init", "--no-git")
	require.NoError(t, err)

	// The bootstrap mirror path must not exist after init.
	bootstrapMirror := filepath.Join(te.Dir, ddxroot.DirName, "skills", "ddx")
	_, statErr := os.Stat(bootstrapMirror)
	assert.True(t, os.IsNotExist(statErr),
		".ddx/skills/ddx must not be created as a separate bootstrap mirror; got stat err=%v", statErr)
}

// TestInitProject_DoesNotInstallPlugins verifies that plain `ddx init` creates
// <project>/.ddx/ and the manifest but does not write .ddx/plugins/ddx,
// .agents/skills/ddx, .claude/skills/ddx, or any project-local plugins/ tree.
func TestInitProject_DoesNotInstallPlugins(t *testing.T) {
	te := NewTestEnvironment(t, WithGitInit(false))
	_, err := te.RunCommand("init", "--no-git")
	require.NoError(t, err)

	// .ddx/ directory and config.yaml must exist.
	assert.DirExists(t, filepath.Join(te.Dir, ddxroot.DirName))
	assert.FileExists(t, filepath.Join(te.Dir, ddxroot.DirName, "config.yaml"))

	// No project-local plugins/ tree must be created.
	pluginsDir := filepath.Join(te.Dir, ddxroot.DirName, "plugins")
	_, statErr := os.Stat(pluginsDir)
	assert.True(t, os.IsNotExist(statErr),
		".ddx/plugins/ must not be created by plain ddx init; got stat err=%v", statErr)

	// No agent-tier skill links must be installed.
	for _, surface := range []string{".agents/skills/ddx", ".claude/skills/ddx"} {
		skillDir := filepath.Join(te.Dir, surface)
		_, statErr := os.Stat(skillDir)
		assert.True(t, os.IsNotExist(statErr),
			"%s must not be created by plain ddx init; got stat err=%v", surface, statErr)
	}
}

// TestInitProject_LeavesPluginsForLazyResolution verifies that after plain
// `ddx init`, registry.ResolvePlugin("ddx") falls through to the baked-in
// layer because no project-local or global plugin was installed.
func TestInitProject_LeavesPluginsForLazyResolution(t *testing.T) {
	// Isolate global DDx dir so a pre-existing global install doesn't interfere.
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)

	te := NewTestEnvironment(t, WithGitInit(false))
	_, err := te.RunCommand("init", "--no-git")
	require.NoError(t, err)

	_, layer, resolveErr := registry.ResolvePlugin(context.Background(), te.Dir, "ddx")
	require.NoError(t, resolveErr)
	assert.Equal(t, "baked-in", layer,
		"ResolvePlugin must fall through to baked-in after plain init (no project-local or global install)")
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

// TestInitDoesNotTouchProjectSkillDirs verifies that plain `ddx init` does not
// create, modify, or delete any project-local agent-tier skill directories
// (.agents/skills/, .claude/skills/). Plugin install is deferred to lazy
// resolution; init is not allowed to touch the skills surface.
func TestInitDoesNotTouchProjectSkillDirs(t *testing.T) {
	te := NewTestEnvironment(t, WithGitInit(false))

	preExistingSkills := []string{"ddx-bead", "helix-align", "some-skill"}

	targetDirs := []string{
		filepath.Join(te.Dir, ".agents", "skills"),
		filepath.Join(te.Dir, ".claude", "skills"),
	}
	for _, dir := range targetDirs {
		for _, skill := range preExistingSkills {
			skillDir := filepath.Join(dir, skill)
			require.NoError(t, os.MkdirAll(skillDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# "+skill), 0o644))
		}
	}

	_, err := te.RunCommand("init", "--no-git")
	require.NoError(t, err)

	// All pre-existing skills must remain untouched.
	for _, dir := range targetDirs {
		for _, skill := range preExistingSkills {
			skillFile := filepath.Join(dir, skill, "SKILL.md")
			assert.FileExists(t, skillFile, "pre-existing skill %s must not be removed from %s by plain init", skill, dir)
		}
	}

	// The ddx skill must NOT be installed by plain init.
	for _, dir := range targetDirs {
		ddxSkill := filepath.Join(dir, "ddx")
		_, statErr := os.Stat(ddxSkill)
		assert.True(t, os.IsNotExist(statErr),
			"plain init must not install ddx skill at %s (lazy resolution handles this)", ddxSkill)
	}
}

// TestGenerateAgentsMD_IncludesInteractiveStewardGuidance verifies that the
// generated AGENTS.md block includes interactive-steward mode, DDX_MODE
// precedence, mutation policy, and the tracker/merge/safety carve-out.
func TestGenerateAgentsMD_IncludesInteractiveStewardGuidance(t *testing.T) {
	workingDir := t.TempDir()

	generateAgentsMD(workingDir)

	agentsPath := filepath.Join(workingDir, "AGENTS.md")
	data, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	content := string(data)

	// AC1: Default Interactive Mode section naming all four modes
	assert.Contains(t, content, "interactive-steward", "interactive-steward mode missing")
	assert.Contains(t, content, "queue_steward", "queue_steward mode missing")
	assert.Contains(t, content, "bead_execution", "bead_execution mode missing")
	assert.Contains(t, content, "direct_user_implementation", "direct_user_implementation mode missing")
	assert.Contains(t, content, "review", "review mode missing")

	// AC2: DDX_MODE=bead_execution overrides only interactive default; never overrides policy
	assert.Contains(t, content, "DDX_MODE=bead_execution", "DDX_MODE=bead_execution precedence statement missing")
	assert.Contains(t, content, "never", "bead_execution override carve-out missing")
	assert.Contains(t, content, "tracker", "tracker carve-out missing")
	assert.Contains(t, content, "merge", "merge policy carve-out missing")
	assert.Contains(t, content, "safety", "safety carve-out missing")

	// AC3: mutation policy — non-mutating phases and explicit-verb requirement
	assert.Contains(t, content, "non-mutating", "non-mutating policy statement missing")
	assert.Contains(t, content, "Tracker mutation", "tracker mutation explicit-verb requirement missing")
	assert.Contains(t, content, "Code edits", "code edits explicit-intent requirement missing")
}

// TestGenerateAgentsMD_MergesWithMarkers verifies AGENTS.md injection preserves
// user content outside the DDx markers and updates content between them.
func TestGenerateAgentsMD_MergesWithMarkers(t *testing.T) {
	workingDir := t.TempDir()
	agentsPath := filepath.Join(workingDir, "AGENTS.md")

	// Pre-seed AGENTS.md with user content both before and after the DDx block
	userBefore := "# My Project\n\nUser content before the DDx block.\n\n"
	oldDdxBlock := agentsMarkerStart + "\nold ddx content\n" + agentsMarkerEnd + "\n"
	userAfter := "\n## More User Content\n\nUser content after the DDx block.\n"
	require.NoError(t, os.WriteFile(agentsPath, []byte(userBefore+oldDdxBlock+userAfter), 0644))

	generateAgentsMD(workingDir)

	data, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	content := string(data)

	// User content outside markers must survive
	assert.Contains(t, content, "User content before the DDx block.", "pre-block user content lost")
	assert.Contains(t, content, "User content after the DDx block.", "post-block user content lost")
	// New DDx block content must be present
	assert.Contains(t, content, "This project uses [DDx]", "new DDx block content not injected")
	// Block markers must still exist (exactly one pair)
	assert.Equal(t, 1, countOccurrences(content, agentsMarkerStart), "should have exactly one start marker")
	assert.Equal(t, 1, countOccurrences(content, agentsMarkerEnd), "should have exactly one end marker")

	// Running generateAgentsMD again must not duplicate the block
	generateAgentsMD(workingDir)
	data2, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	content2 := string(data2)
	assert.Equal(t, 1, countOccurrences(content2, agentsMarkerStart), "re-run duplicated start marker")
	assert.Equal(t, 1, countOccurrences(content2, agentsMarkerEnd), "re-run duplicated end marker")
}

// TestGenerateAgentsMD_CreatesWhenMissing verifies AGENTS.md is created if absent.
func TestGenerateAgentsMD_CreatesWhenMissing(t *testing.T) {
	workingDir := t.TempDir()
	agentsPath := filepath.Join(workingDir, "AGENTS.md")

	generateAgentsMD(workingDir)

	data, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, agentsMarkerStart, "start marker missing")
	assert.Contains(t, content, agentsMarkerEnd, "end marker missing")
	assert.Contains(t, content, "This project uses [DDx]", "DDx content missing")
}

// TestGenerateAgentsMD_AppendsWhenMarkersAbsent verifies the block is appended
// when AGENTS.md exists but has no markers (user had AGENTS.md from another tool).
func TestGenerateAgentsMD_AppendsWhenMarkersAbsent(t *testing.T) {
	workingDir := t.TempDir()
	agentsPath := filepath.Join(workingDir, "AGENTS.md")

	userContent := "# My Project\n\nExisting AGENTS.md from another tool.\n"
	require.NoError(t, os.WriteFile(agentsPath, []byte(userContent), 0644))

	generateAgentsMD(workingDir)

	data, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "Existing AGENTS.md from another tool.", "existing content lost")
	assert.Contains(t, content, agentsMarkerStart, "start marker missing after append")
	assert.Contains(t, content, agentsMarkerEnd, "end marker missing after append")
}

// countOccurrences is a small test helper; we avoid strings.Count import noise here.
func countOccurrences(s, sub string) int {
	count := 0
	start := 0
	for {
		idx := indexFrom(s, sub, start)
		if idx == -1 {
			return count
		}
		count++
		start = idx + len(sub)
	}
}

func indexFrom(s, sub string, start int) int {
	if start > len(s) {
		return -1
	}
	rest := s[start:]
	for i := 0; i+len(sub) <= len(rest); i++ {
		if rest[i:i+len(sub)] == sub {
			return start + i
		}
	}
	return -1
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
				backupFiles, _ := filepath.Glob(filepath.Join(te.Dir, ddxroot.DirName, "config.yaml.backup.*"))
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

// TestInitGitignoreRules verifies that ddx init writes the correct .gitignore rules
// for the tracked/ignored split: runtime scratch is ignored, execution evidence is tracked.
func TestInitGitignoreRules(t *testing.T) {
	te := NewTestEnvironment(t, WithGitInit(false))
	_, err := te.RunCommand("init", "--no-git")
	require.NoError(t, err)

	gitignorePath := filepath.Join(te.Dir, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	content := string(data)

	// Runtime scratch must be ignored
	assert.Contains(t, content, ".ddx/agent-logs/", ".ddx/agent-logs/ must be ignored")
	assert.Contains(t, content, ".ddx/attachments/", ".ddx/attachments/ must be ignored")
	assert.Contains(t, content, ".ddx/workers/", ".ddx/workers/ must be ignored")
	assert.Contains(t, content, ".ddx/exec-runs.d/", ".ddx/exec-runs.d/ must be ignored")
	assert.Contains(t, content, ".ddx/server.env", ".ddx/server.env must be ignored")
	assert.Contains(t, content, ".ddx/server/", ".ddx/server/ must be ignored")
	assert.Contains(t, content, ".ddx/run-state.json", ".ddx/run-state.json must be ignored")
	assert.Contains(t, content, ".ddx/run-state/", ".ddx/run-state/ must be ignored")
	assert.Contains(t, content, ".ddx/executions/*/embedded/", "embedded runtime state must be ignored")

	// Execution evidence must be explicitly un-ignored
	assert.Contains(t, content, "!.ddx/executions/", "executions/ directory must be un-ignored")
	assert.Contains(t, content, "!.ddx/executions/*/prompt.md", "prompt.md must be un-ignored")
	assert.Contains(t, content, "!.ddx/executions/*/manifest.json", "manifest.json must be un-ignored")
	assert.Contains(t, content, "!.ddx/executions/*/result.json", "result.json must be un-ignored")
	assert.Contains(t, content, "!.ddx/executions/*/usage.json", "usage.json must be un-ignored")

	// Verify with git check-ignore that a concrete evidence file is NOT ignored
	// Set up a minimal git repo to run check-ignore
	gitInit := exec.Command("git", "init", "-q")
	gitInit.Dir = te.Dir
	require.NoError(t, gitInit.Run())

	gitConfig1 := exec.Command("git", "config", "user.email", "test@test.com")
	gitConfig1.Dir = te.Dir
	require.NoError(t, gitConfig1.Run())
	gitConfig2 := exec.Command("git", "config", "user.name", "Test")
	gitConfig2.Dir = te.Dir
	require.NoError(t, gitConfig2.Run())

	// git check-ignore exits 0 if ignored, 1 if not ignored
	checkIgnore := exec.Command("git", "check-ignore", "-q", ".ddx/executions/abc123/prompt.md")
	checkIgnore.Dir = te.Dir
	err = checkIgnore.Run()
	// exit code 1 means NOT ignored — that's what we want
	assert.Error(t, err, ".ddx/executions/abc123/prompt.md must NOT be ignored by git")

	checkIgnoreUsage := exec.Command("git", "check-ignore", "-q", ".ddx/executions/abc123/usage.json")
	checkIgnoreUsage.Dir = te.Dir
	err = checkIgnoreUsage.Run()
	// exit code 1 means NOT ignored — that's what we want
	assert.Error(t, err, ".ddx/executions/abc123/usage.json must NOT be ignored by git")
}

func TestInitGitignoreRunStateMigration(t *testing.T) {
	te := NewTestEnvironment(t, WithGitInit(false))
	runCleanupCommandGit(t, te.Dir, "init", "-b", "main")
	runCleanupCommandGit(t, te.Dir, "config", "user.email", "test@test.com")
	runCleanupCommandGit(t, te.Dir, "config", "user.name", "Test")

	ddxDir := filepath.Join(te.Dir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(filepath.Join(ddxDir, "run-state"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "run-state.json"), []byte(`{"attempt_id":"legacy-root"}`+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "run-state", "legacy-attempt.json"), []byte(`{"attempt_id":"legacy-attempt"}`+"\n"), 0o644))
	runCleanupCommandGit(t, te.Dir, "add", ".ddx/run-state.json", ".ddx/run-state/legacy-attempt.json")
	runCleanupCommandGit(t, te.Dir, "commit", "-m", "test: track legacy run-state")

	_, err := te.RunCommand("init")
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(ddxDir, "run-state.json"))
	assert.FileExists(t, filepath.Join(ddxDir, "run-state", "legacy-attempt.json"))
	assert.Empty(t, runCleanupCommandGit(t, te.Dir, "ls-files", "--", ".ddx/run-state.json", ".ddx/run-state"),
		"legacy run-state files must be removed from the git index by init migration")
	assert.Empty(t, runCleanupCommandGit(t, te.Dir, "status", "--short", "--", ".ddx/run-state.json", ".ddx/run-state"),
		"init migration must not leave run-state deletions or modifications behind")

	data, err := os.ReadFile(filepath.Join(te.Dir, ".gitignore"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, ".ddx/run-state.json")
	assert.Contains(t, content, ".ddx/run-state/")
}

// TestInitGlobal_CreatesAgentTierLinks verifies that `ddx init --global` creates
// ~/.claude/skills/ddx and ~/.agents/skills/ddx symlinks pointing into
// ${XDG_DATA_HOME}/ddx/global/plugins/ddx/.
func TestInitGlobal_CreatesAgentTierLinks(t *testing.T) {
	xdgDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("HOME", homeDir)

	te := NewTestEnvironment(t, WithGitInit(false))
	_, err := te.RunCommand("init", "--global")
	require.NoError(t, err)

	globalPluginDir := filepath.Join(xdgDir, "ddx", "global", "plugins", "ddx")
	expectedTarget := filepath.Join(globalPluginDir, "skills", "ddx")

	for _, surface := range []string{".claude/skills", ".agents/skills"} {
		link := filepath.Join(homeDir, surface, "ddx")
		info, statErr := os.Lstat(link)
		require.NoError(t, statErr, "skill link %s must exist", link)
		assert.True(t, info.Mode()&os.ModeSymlink != 0, "%s must be a symlink", link)
		target, readErr := os.Readlink(link)
		require.NoError(t, readErr)
		assert.Equal(t, expectedTarget, target, "%s must point into global plugin dir", link)
	}
}

// TestInitGlobal_WritesGlobalConfig verifies that `ddx init --global` writes
// ${XDG_DATA_HOME}/ddx/global/config.yaml with convention defaults.
func TestInitGlobal_WritesGlobalConfig(t *testing.T) {
	xdgDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("HOME", homeDir)

	te := NewTestEnvironment(t, WithGitInit(false))
	_, err := te.RunCommand("init", "--global")
	require.NoError(t, err)

	configPath := filepath.Join(xdgDir, "ddx", "global", "config.yaml")
	assert.FileExists(t, configPath, "global config.yaml must be written by ddx init --global")

	data, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	content := string(data)
	assert.Contains(t, content, "version:", "global config.yaml must contain version field")
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
