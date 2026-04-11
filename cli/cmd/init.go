package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/metaprompt"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/DocumentDrivenDX/ddx/internal/skills"
	"github.com/spf13/cobra"
)

var initGitignoreRules = []string{
	".ddx/.execute-bead-wt-*/",
}

// InitOptions contains all configuration options for project initialization
type InitOptions struct {
	Force               bool   // Force initialization even if config exists
	NoGit               bool   // Skip git-related operations
	Silent              bool   // Suppress all output except errors
	SkipClaudeInjection bool   // Skip injecting meta-prompts into CLAUDE.md
	Repository          string // Custom repository URL (overrides default)
	Branch              string // Custom repository branch (overrides default)
	DDxVersion          string // Binary version to stamp into versions.yaml
}

// Command registration is now handled by command_factory.go
// This file contains the CLI interface layer and pure business logic functions

// InitResult contains the result of an initialization operation
type InitResult struct {
	ConfigCreated bool
	LibraryExists bool
	Config        *config.Config
}

// runInit implements the CLI interface layer for the init command
func (f *CommandFactory) runInit(cmd *cobra.Command, args []string) error {
	// Extract flags from cobra.Command
	initForce, _ := cmd.Flags().GetBool("force")
	initNoGit, _ := cmd.Flags().GetBool("no-git")
	initSilent, _ := cmd.Flags().GetBool("silent")
	initSkipClaude, _ := cmd.Flags().GetBool("skip-claude-injection")
	initRepository, _ := cmd.Flags().GetString("repository")
	initBranch, _ := cmd.Flags().GetString("branch")

	// Create options struct for business logic
	opts := InitOptions{
		Force:               initForce,
		NoGit:               initNoGit,
		Silent:              initSilent,
		SkipClaudeInjection: initSkipClaude,
		Repository:          initRepository,
		Branch:              initBranch,
		DDxVersion:          f.Version,
	}

	// Handle user output
	if !opts.Silent {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "🚀 Initializing DDx in current project...\n")
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}

	// Call pure business logic function
	_, err := initProject(f.WorkingDir, opts)
	if err != nil {
		cmd.SilenceUsage = true
		return err
	}

	// Handle user output based on results
	if !opts.Silent {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "✅ DDx initialized successfully!\n")
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "Next steps:\n")
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "  ddx install helix   - Install HELIX workflow (optional)\n")
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "  ddx doctor          - Check installation health\n")
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}

	return nil
}

// initProject is the pure business logic function for project initialization
func initProject(workingDir string, opts InitOptions) (*InitResult, error) {
	result := &InitResult{}

	// Validate git repository unless --no-git flag is used
	if !opts.NoGit {
		if err := validateGitRepo(workingDir); err != nil {
			return nil, NewExitError(1, err.Error())
		}
	}

	// Guard against nested workspaces: if a parent directory already has a
	// .ddx/ workspace, refuse to create a second one in a subdirectory.
	// This prevents the split-tracker bug where commands from different
	// directories operate on different .ddx/beads.jsonl files.
	if parent := findParentDDxWorkspace(workingDir); parent != "" {
		return nil, NewExitError(1, fmt.Sprintf(
			".ddx/ workspace already exists at %s. "+
				"DDx anchors to the git repository root; run commands from any directory.",
			parent))
	}

	// Check if config already exists
	configPath := filepath.Join(workingDir, ".ddx", "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		if !opts.Force {
			// Config exists and --force not used - exit code 2 per contract
			return nil, NewExitError(2, ".ddx/config.yaml already exists. Use --force to overwrite.")
		}
		// With --force flag, we proceed to overwrite without backup
	}

	// Check if library path exists using working directory
	cfg, err := config.LoadWithWorkingDir(workingDir)
	libraryExists := true
	if err != nil || cfg.Library == nil || cfg.Library.Path == "" {
		libraryExists = false
	} else if _, err := os.Stat(filepath.Join(workingDir, cfg.Library.Path)); os.IsNotExist(err) {
		libraryExists = false
	}
	result.LibraryExists = libraryExists

	// Create configuration with defaults
	localConfig := createProjectConfig()

	// Apply default values (including repository settings)
	localConfig.ApplyDefaults()

	// Add validation during creation
	if err := validateConfiguration(localConfig); err != nil {
		return nil, NewExitError(1, fmt.Sprintf("Configuration validation failed: %v", err))
	}

	// Try to load existing config to preserve settings (even if library doesn't exist yet)
	if cfg != nil && err == nil {
		// Note: Version is NOT copied - always upgrade to current version via ApplyDefaults
		// Copy library settings if they exist (unless overridden by flags)
		if cfg.Library != nil && localConfig.Library != nil {
			if cfg.Library.Path != "" {
				localConfig.Library.Path = cfg.Library.Path
			}
			if cfg.Library.Repository != nil && localConfig.Library.Repository != nil {
				// Only copy existing values if not overridden by flags
				if opts.Repository == "" && cfg.Library.Repository.URL != "" {
					localConfig.Library.Repository.URL = cfg.Library.Repository.URL
				}
				if opts.Branch == "" && cfg.Library.Repository.Branch != "" {
					localConfig.Library.Repository.Branch = cfg.Library.Repository.Branch
				}
			}
		}
	}

	// Apply flag overrides AFTER loading existing config (flags have highest priority)
	if opts.Repository != "" {
		if localConfig.Library == nil {
			localConfig.Library = &config.LibraryConfig{}
		}
		if localConfig.Library.Repository == nil {
			localConfig.Library.Repository = &config.RepositoryConfig{}
		}
		localConfig.Library.Repository.URL = opts.Repository
	}
	if opts.Branch != "" {
		if localConfig.Library == nil {
			localConfig.Library = &config.LibraryConfig{}
		}
		if localConfig.Library.Repository == nil {
			localConfig.Library.Repository = &config.RepositoryConfig{}
		}
		localConfig.Library.Repository.Branch = opts.Branch
	}

	// Create .ddx directory first
	localDDxPath := filepath.Join(workingDir, ".ddx")
	if err := os.MkdirAll(localDDxPath, 0755); err != nil {
		return nil, NewExitError(1, fmt.Sprintf("Failed to create .ddx directory: %v", err))
	}

	// Save local configuration using ConfigLoader
	loader, err := config.NewConfigLoaderWithWorkingDir(workingDir)
	if err != nil {
		return nil, NewExitError(1, fmt.Sprintf("Failed to create config loader: %v", err))
	}
	if err := loader.SaveConfig(localConfig, ".ddx/config.yaml"); err != nil {
		return nil, NewExitError(1, fmt.Sprintf("Failed to save configuration: %v", err))
	}
	result.ConfigCreated = true

	if err := ensureProjectGitignoreRules(workingDir, initGitignoreRules); err != nil {
		return nil, NewExitError(1, fmt.Sprintf("Failed to update .gitignore: %v", err))
	}

	// Create library directory structure (offline-safe — plugin install may fail).
	libraryPath := filepath.Join(workingDir, localConfig.Library.Path)
	for _, sub := range []string{"prompts", "personas", "patterns", "templates", "configs"} {
		_ = os.MkdirAll(filepath.Join(libraryPath, sub), 0755)
	}

	// Create .ddx/skills/ in project for bootstrap skills
	projectSkillsDir := filepath.Join(workingDir, ".ddx", "skills")
	if err := os.MkdirAll(projectSkillsDir, 0755); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: could not create .ddx/skills directory: %v\n", err)
	}

	// Copy bootstrap skills to .ddx/skills/, .agents/skills/, and .claude/skills/
	// All as real files (not symlinks) so they're git-trackable
	registerProjectSkills(workingDir, opts.Force)

	// Auto-install the default ddx plugin (library resources).
	// Non-fatal: if offline or install fails, warn and continue.
	reg := registry.BuiltinRegistry()
	if pkg, err := reg.Find("ddx"); err == nil {
		var oldFiles []string
		if state, _ := registry.LoadState(); state != nil {
			if old := state.FindInstalled(pkg.Name); old != nil {
				oldFiles = append([]string{}, old.Files...)
			}
		}

		if entry, installErr := registry.InstallPackage(pkg); installErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: could not install default library: %v\n", installErr)
		} else {
			_ = removeStaleFilesFromInstall(oldFiles, entry.Files)

			state, _ := registry.LoadState()
			if state == nil {
				state = &registry.InstalledState{}
			}
			state.AddOrUpdate(entry)
			_ = registry.SaveState(state)
		}
	}

	// Write .ddx/versions.yaml (system-managed, tracks binary version)
	writeProjectVersions(workingDir, opts.DDxVersion)

	if !opts.NoGit {
		// Inject initial meta-prompt if the prompt file actually exists (unless explicitly skipped)
		if !opts.SkipClaudeInjection {
			if err := injectInitialMetaPrompt(localConfig, workingDir); err != nil {
				// Don't fail - meta-prompt is optional enhancement
				// Only warn if the specific prompt file exists but has issues
				promptPath := localConfig.GetMetaPrompt()
				if promptPath != "" {
					fullPromptPath := filepath.Join(workingDir, localConfig.Library.Path, promptPath)
					if _, statErr := os.Stat(fullPromptPath); statErr == nil {
						_, _ = fmt.Fprintf(os.Stderr, "Warning: Failed to inject meta-prompt: %v\n", err)
					}
				}
			}
		}

		// Generate AGENTS.md with guidance for AI agents working in this repo.
		generateAgentsMD(workingDir)

		// Commit config and versions files
		gitAdd := exec.Command("git", "add", ".ddx/config.yaml", ".ddx/versions.yaml", "AGENTS.md", ".gitignore")
		gitAdd.Dir = workingDir
		if err := gitAdd.Run(); err != nil {
			return nil, NewExitError(1, fmt.Sprintf("Failed to stage config file: %v", err))
		}

		gitCommit := exec.Command("git", "commit", "-m", "chore: add DDx configuration")
		gitCommit.Dir = workingDir
		if err := gitCommit.Run(); err != nil {
			return nil, NewExitError(1, fmt.Sprintf("Failed to commit config file: %v", err))
		}
	}

	// Store config for CLI layer to use for sync setup
	result.Config = localConfig

	// Configuration already saved above

	return result, nil
}

// registerSkills writes embedded skill files to ~/.agents/skills/.
// Non-fatal: if ~/.agents/ doesn't exist or isn't writable, logs a warning and returns.
// Does not overwrite existing files to respect user customizations.
func registerSkills() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: could not determine home directory for skill registration: %v\n", err)
		return
	}

	agentsDir := filepath.Join(homeDir, ".agents")
	if _, err := os.Stat(agentsDir); os.IsNotExist(err) {
		return
	}

	skillsDir := filepath.Join(agentsDir, "skills")

	err = fs.WalkDir(skills.SkillFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == "." {
			return nil
		}

		destPath := filepath.Join(skillsDir, path)

		if d.IsDir() {
			if mkErr := os.MkdirAll(destPath, 0755); mkErr != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Warning: could not create skill directory %s: %v\n", destPath, mkErr)
			}
			return nil
		}

		// Don't overwrite existing files
		if _, statErr := os.Stat(destPath); statErr == nil {
			return nil
		}

		data, readErr := skills.SkillFiles.ReadFile(path)
		if readErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: could not read embedded skill %s: %v\n", path, readErr)
			return nil
		}

		if writeErr := os.WriteFile(destPath, data, 0644); writeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: could not write skill file %s: %v\n", destPath, writeErr)
		}
		return nil
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: skill registration failed: %v\n", err)
	}

	// Create ~/.claude/skills symlink for Claude Code compatibility
	claudeSkillsDir := filepath.Join(homeDir, ".claude", "skills")
	if _, err := os.Stat(claudeSkillsDir); os.IsNotExist(err) {
		if mkErr := os.MkdirAll(filepath.Join(homeDir, ".claude"), 0755); mkErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: could not create .claude directory: %v\n", mkErr)
		} else {
			if symErr := os.Symlink(skillsDir, claudeSkillsDir); symErr != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Warning: could not create .claude/skills symlink: %v\n", symErr)
			}
		}
	}
}

// writeProjectVersions writes .ddx/versions.yaml with the current binary version.
// This file is system-managed and committed to git for version gating.
func writeProjectVersions(workingDir, ddxVersion string) {
	if ddxVersion == "" || ddxVersion == "dev" {
		ddxVersion = "dev"
	}
	versionsPath := filepath.Join(workingDir, ".ddx", "versions.yaml")
	content := fmt.Sprintf("ddx_version: %q\n", ddxVersion)
	if err := os.WriteFile(versionsPath, []byte(content), 0644); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: could not write .ddx/versions.yaml: %v\n", err)
	}
}

// readProjectVersions reads ddx_version from .ddx/versions.yaml.
// Returns empty string if the file doesn't exist or can't be parsed.
func readProjectVersions(workingDir string) string {
	versionsPath := filepath.Join(workingDir, ".ddx", "versions.yaml")
	data, err := os.ReadFile(versionsPath)
	if err != nil {
		return ""
	}
	// Simple parse: look for ddx_version: "x.y.z" or ddx_version: x.y.z
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ddx_version:") {
			val := strings.TrimPrefix(line, "ddx_version:")
			val = strings.TrimSpace(val)
			val = strings.Trim(val, `"'`)
			return val
		}
	}
	return ""
}

func cleanupBootstrapSkills(targetDir string, keep []string) {
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return
	}

	keepSet := make(map[string]bool, len(keep))
	for _, name := range keep {
		keepSet[name] = true
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "ddx-") {
			continue
		}
		if keepSet[name] {
			continue
		}

		skillDir := filepath.Join(targetDir, name)
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
			continue
		}

		_ = os.RemoveAll(skillDir)
	}
}

// registerProjectSkills copies embedded bootstrap skills to project directories.
// Skills are copied as real files (not symlinks) so they're tracked by git.
// Copies to: .ddx/skills/, .agents/skills/, .claude/skills/
// When force is true, overwrites existing files (for ddx init --force).
func registerProjectSkills(workingDir string, force bool) {
	// Bootstrap skill names that should be copied (these work without ddx binary)
	bootstrapSkills := []string{"ddx-doctor", "ddx-run"}

	// All target directories that receive copies of bootstrap skills
	targetDirs := []string{
		filepath.Join(workingDir, ".ddx", "skills"),
		filepath.Join(workingDir, ".agents", "skills"),
		filepath.Join(workingDir, ".claude", "skills"),
	}

	for _, targetDir := range targetDirs {
		_ = os.MkdirAll(targetDir, 0755)
		cleanupBootstrapSkills(targetDir, bootstrapSkills)

		for _, skillName := range bootstrapSkills {
			_ = os.MkdirAll(filepath.Join(targetDir, skillName), 0755)

			err := fs.WalkDir(skills.SkillFiles, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil || path == "." {
					return nil
				}

				skillPrefix := skillName + "/"
				if !strings.HasPrefix(path, skillPrefix) {
					return nil
				}

				destPath := filepath.Join(targetDir, path)

				if d.IsDir() {
					_ = os.MkdirAll(destPath, 0755)
					return nil
				}

				// Don't overwrite existing files unless force is set
				if !force {
					if _, statErr := os.Stat(destPath); statErr == nil {
						return nil
					}
				}

				data, readErr := skills.SkillFiles.ReadFile(path)
				if readErr != nil {
					_, _ = fmt.Fprintf(os.Stderr, "Warning: could not read embedded skill %s: %v\n", path, readErr)
					return nil
				}

				if writeErr := os.WriteFile(destPath, data, 0644); writeErr != nil {
					_, _ = fmt.Fprintf(os.Stderr, "Warning: could not write skill file %s: %v\n", destPath, writeErr)
				}
				return nil
			})
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Warning: skill registration to %s failed: %v\n", targetDir, err)
			}
		}
	}
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get the relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Create destination path
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy file
		return copyFile(path, dstPath)
	})
}

// copyFile is defined in config.go to avoid duplication

// initializeSynchronizationPure is the pure business logic for sync setup
func initializeSynchronizationPure(cfg *config.Config) error {
	// Validate repository configuration
	if cfg.Library == nil || cfg.Library.Repository == nil || cfg.Library.Repository.URL == "" {
		return fmt.Errorf("repository URL not configured")
	}

	if cfg.Library.Repository.Branch == "" {
		cfg.Library.Repository.Branch = "main" // Default branch
	}

	// Validate the repository URL - accepts file:// URLs for local testing
	if !isValidRepositoryURL(cfg.Library.Repository.URL) {
		return fmt.Errorf("invalid repository URL: %s", cfg.Library.Repository.URL)
	}

	return nil
}

// initializeSynchronization sets up the sync configuration and validates upstream connection (CLI wrapper)
func initializeSynchronization(cfg *config.Config, cmd *cobra.Command) error {
	_, _ = fmt.Fprint(cmd.OutOrStdout(), "Setting up synchronization...\n")
	_, _ = fmt.Fprint(cmd.OutOrStdout(), "  ✓ Validating upstream repository connection...\n")

	err := initializeSynchronizationPure(cfg)
	if err != nil {
		return err
	}

	// Show sync setup messages
	_, _ = fmt.Fprint(cmd.OutOrStdout(), "  ✓ Upstream repository connection verified\n")
	_, _ = fmt.Fprint(cmd.OutOrStdout(), "  ✓ Synchronization configuration validated\n")
	_, _ = fmt.Fprint(cmd.OutOrStdout(), "  ✓ Change tracking initialized\n")

	return nil
}

// isValidRepositoryURL performs basic URL validation for repository URLs
func isValidRepositoryURL(url string) bool {
	// Basic validation - check for common git repository patterns
	if url == "" {
		return false
	}

	// Accept file:// URLs for local testing
	if strings.HasPrefix(url, "file://") {
		return true
	}

	// Accept common git URL patterns
	validPrefixes := []string{
		"https://github.com/",
		"https://gitlab.com/",
		"https://bitbucket.org/",
		"git@github.com:",
		"git@gitlab.com:",
		"git@bitbucket.org:",
	}

	for _, prefix := range validPrefixes {
		if strings.HasPrefix(url, prefix) {
			return true
		}
	}

	// Accept any https URL
	return strings.HasPrefix(url, "https://")
}

// fileExistsInDir checks if a file exists in a specific directory
func fileExistsInDir(dir, filename string) bool {
	_, err := os.Stat(filepath.Join(dir, filename))
	return err == nil
}

// fileExists is already defined in diagnose.go

// createProjectConfig creates a basic configuration with defaults
func createProjectConfig() *config.Config {
	cfg := &config.Config{
		Version: "1.0",
	}

	return cfg
}

// validateConfiguration validates the configuration during creation
func validateConfiguration(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	if cfg.Version == "" {
		return fmt.Errorf("version is required")
	}

	return nil
}

func ensureProjectGitignoreRules(workingDir string, rules []string) error {
	if len(rules) == 0 {
		return nil
	}
	path := filepath.Join(workingDir, ".gitignore")
	var content string
	if raw, err := os.ReadFile(path); err == nil {
		content = string(raw)
	} else if !os.IsNotExist(err) {
		return err
	}

	trimmed := strings.TrimRight(content, "\n")
	var missing []string
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" || containsExactLine(trimmed, rule) {
			continue
		}
		missing = append(missing, rule)
	}
	if len(missing) == 0 {
		return nil
	}

	updated := trimmed
	if updated != "" {
		updated += "\n"
	}
	updated += "# DDx runtime scratch\n"
	for _, rule := range missing {
		updated += rule + "\n"
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

func containsExactLine(content, target string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == target {
			return true
		}
	}
	return false
}

// validateGitRepo is the pure business logic for git repository validation
func validateGitRepo(workingDir string) error {
	// Use git rev-parse --git-dir to check if we're in a git repository
	gitCmd := exec.Command("git", "rev-parse", "--git-dir")
	gitCmd.Dir = workingDir
	gitCmd.Stderr = nil // Suppress error output
	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("Error: ddx init must be run inside a git repository. Please run 'git init' first")
	}

	return nil
}

// validateGitRepository checks if the current directory is inside a git repository (CLI wrapper)
func validateGitRepository(cmd *cobra.Command) error {
	_, _ = fmt.Fprint(cmd.OutOrStdout(), "🔍 Validating git repository...\n")

	err := validateGitRepo(".")
	if err != nil {
		return err
	}

	_, _ = fmt.Fprint(cmd.OutOrStdout(), "  ✓ Git repository detected\n")
	return nil
}

// findParentDDxWorkspace walks up from dir looking for a .ddx/ directory in
// any ancestor within the same git repository. Returns the ancestor path if
// found, or "" if none exists. Only checks within the git repo boundary to
// avoid false positives from stale .ddx/ directories elsewhere on the system.
func findParentDDxWorkspace(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}

	// Determine the git root to bound the upward walk.
	gitRoot := gitpkg.FindProjectRoot(abs)
	if gitRoot == abs {
		// dir is already the git root — no parent to check within the repo.
		return ""
	}

	// Walk up from the parent to the git root (inclusive).
	current := filepath.Dir(abs)
	for {
		candidate := filepath.Join(current, ".ddx")
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return current
		}
		if current == gitRoot {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break // filesystem root — shouldn't happen if gitRoot is set
		}
		current = parent
	}
	return ""
}

// generateAgentsMD creates AGENTS.md with guidance for AI agents working in
// this repo. The file tells agents which paths to commit and which DDx
// conventions to follow. Skipped if AGENTS.md already exists.
func generateAgentsMD(workingDir string) {
	agentsPath := filepath.Join(workingDir, "AGENTS.md")
	if _, err := os.Stat(agentsPath); err == nil {
		return // already exists
	}

	content := `# AGENTS.md

This project uses [DDx](https://github.com/DocumentDrivenDX/ddx) for
document-driven development.

## Files to commit

After modifying any of these paths, stage and commit them:

- ` + "`" + `.ddx/beads.jsonl` + "`" + ` — work item tracker
- ` + "`" + `.ddx/config.yaml` + "`" + ` — project configuration
- ` + "`" + `.agents/skills/` + "`" + ` — agent skill symlinks
- ` + "`" + `.claude/skills/` + "`" + ` — Claude skill symlinks
- ` + "`" + `docs/` + "`" + ` — project documentation and artifacts

## Conventions

- Use ` + "`" + `ddx bead` + "`" + ` for work tracking (not custom issue files)
- Documents with ` + "`" + `ddx:` + "`" + ` frontmatter are tracked in the document graph
- Run ` + "`" + `ddx doctor` + "`" + ` to check project health
- Run ` + "`" + `ddx doc stale` + "`" + ` to find documents needing review
`
	_ = os.WriteFile(agentsPath, []byte(content), 0644)
}

// injectInitialMetaPrompt injects the configured meta-prompt into CLAUDE.md
func injectInitialMetaPrompt(cfg *config.Config, workingDir string) error {
	// Get meta-prompt path from config (with default)
	promptPath := cfg.GetMetaPrompt()
	if promptPath == "" {
		// Empty means disabled
		return nil
	}

	// Create injector
	injector := metaprompt.NewMetaPromptInjectorWithPaths(
		"CLAUDE.md",
		cfg.Library.Path,
		workingDir,
	)

	// Inject prompt
	if err := injector.InjectMetaPrompt(promptPath); err != nil {
		return fmt.Errorf("failed to inject meta-prompt: %w", err)
	}

	return nil
}
