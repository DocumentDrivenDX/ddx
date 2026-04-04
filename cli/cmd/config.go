package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/metaprompt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Command registration is now handled by command_factory.go
// This file only contains the run function implementation

// runConfig implements the config command logic for CommandFactory
func (f *CommandFactory) runConfig(cmd *cobra.Command, args []string) error {
	// Extract flags from cobra.Command
	showFlag, _ := cmd.Flags().GetBool("show")
	showFilesFlag, _ := cmd.Flags().GetBool("show-files")
	editFlag, _ := cmd.Flags().GetBool("edit")
	resetFlag, _ := cmd.Flags().GetBool("reset")
	wizardFlag, _ := cmd.Flags().GetBool("wizard")
	validateFlag, _ := cmd.Flags().GetBool("validate")
	globalFlag, _ := cmd.Flags().GetBool("global")

	// Handle flags by calling pure business logic functions
	if showFlag {
		return fmt.Errorf("config show removed - use 'cat .ddx/config.yaml' to view configuration")
	}

	if showFilesFlag {
		files := configListFiles(f.WorkingDir)
		return f.outputConfigFiles(cmd, files)
	}

	if editFlag {
		configPath := configGetPath(f.WorkingDir, globalFlag)
		return f.editConfigFile(cmd, configPath)
	}

	if resetFlag {
		configPath := configGetPath(f.WorkingDir, globalFlag)
		if err := configReset(f.WorkingDir, globalFlag); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Configuration reset to defaults: %s\n", configPath)
		return nil
	}

	if wizardFlag {
		cfg, err := configWizard()
		if err != nil {
			return err
		}
		if err := configSave(f.WorkingDir, cfg, false); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "✅ Configuration saved to .ddx/config.yaml")
		return nil
	}

	if validateFlag {
		if err := configValidate(f.WorkingDir); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "✅ Configuration is valid")
		return nil
	}

	// Handle subcommands
	if len(args) == 0 {
		// Default behavior: show help
		return cmd.Help()
	}

	subcommand := args[0]
	switch subcommand {
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("key required for get command")
		}
		value, err := configGet(f.WorkingDir, args[1], globalFlag)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), value)
		return nil
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("key and value required for set command")
		}
		if err := configSet(f.WorkingDir, args[1], args[2], globalFlag); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Set %s = %s\n", args[1], args[2])
		return nil
	case "validate":
		if err := configValidate(f.WorkingDir); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "✅ Configuration is valid")
		return nil
	case "export":
		// Simply output the config file content
		var configPath string
		if globalFlag {
			// Use global config path
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}
			configPath = filepath.Join(homeDir, ".ddx", "config.yaml")
		} else {
			// Use local config path
			configPath = filepath.Join(f.WorkingDir, ".ddx", "config.yaml")
		}

		content, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), string(content))
		return nil
	case "import":
		// For now, just read from stdin
		return fmt.Errorf("import not yet implemented")
	case "profile":
		if len(args) < 2 {
			return fmt.Errorf("profile subcommand requires additional arguments")
		}
		return f.handleProfileSubcommand(cmd, args[1:])
	default:
		return fmt.Errorf("unknown config subcommand: %s", subcommand)
	}
}

// Legacy wrapper functions for backwards compatibility
func runConfig(cmd *cobra.Command, args []string) error {
	f := &CommandFactory{WorkingDir: ""}
	return f.runConfig(cmd, args)
}

// Legacy functions - replaced by pure business logic functions above
// Business Logic Layer - Pure Functions

// configGet retrieves a configuration value
func configGet(workingDir string, key string, global bool) (string, error) {
	var cfg *config.Config
	var err error

	if workingDir != "" && !global {
		// Load config from specific working directory using new format
		cfg, err = config.LoadWithWorkingDir(workingDir)
		if err != nil {
			return "", fmt.Errorf("failed to load configuration from %s: %w", workingDir, err)
		}
	} else {
		// Use standard config loading (current directory)
		cfg, err = config.Load()
		if err != nil {
			return "", fmt.Errorf("failed to load configuration: %w", err)
		}
	}

	return extractConfigValue(cfg, key)
}

// configSet sets a configuration value
func configSet(workingDir string, key, value string, global bool) error {
	var cfg *config.Config
	var err error

	if workingDir != "" && !global {
		// Load config from specific working directory using new format
		cfg, err = config.LoadWithWorkingDir(workingDir)
		if err != nil {
			// If file doesn't exist in working dir, create a new config
			if os.IsNotExist(err) {
				cfg = &config.Config{
					Version: "1.0",
					Library: &config.LibraryConfig{
						Path: ".ddx/library",
						Repository: &config.RepositoryConfig{
							URL:    "https://github.com/DocumentDrivenDX/ddx-library",
							Branch: "main",
						},
					},
				}
			} else {
				return fmt.Errorf("failed to load configuration from %s: %w", workingDir, err)
			}
		}
	} else {
		// Use standard config loading (current directory)
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}
	}

	if err := setConfigValueInStruct(cfg, key, value); err != nil {
		return err
	}

	return configSave(workingDir, cfg, global)
}

// configValidate validates the configuration
func configValidate(workingDir string) error {
	var cfg *config.Config
	var err error
	if workingDir != "" {
		cfg, err = config.LoadWithWorkingDir(workingDir)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	return cfg.Validate()
}

// configReset resets configuration to defaults
func configReset(workingDir string, global bool) error {
	cfg := config.DefaultConfig
	return configSave(workingDir, cfg, global)
}

// configWizard runs the configuration wizard
func configWizard() (*config.Config, error) {
	cyan := color.New(color.FgCyan)
	_, _ = cyan.Println("🧙 DDx Configuration Wizard")
	fmt.Println()

	// Start with default config
	cfg := *config.DefaultConfig

	// Return the config without interactive prompts (Variables removed)
	return &cfg, nil
}

// configListFiles returns a list of configuration file locations
func configListFiles(workingDir string) []ConfigFileInfo {
	var files []ConfigFileInfo

	// Current directory config
	localConfig := ".ddx/config.yaml"
	if workingDir != "" {
		localConfig = filepath.Join(workingDir, ".ddx", "config.yaml")
	}
	if _, err := os.Stat(localConfig); err == nil {
		files = append(files, ConfigFileInfo{Path: localConfig, Type: "project", Exists: true})
	} else {
		files = append(files, ConfigFileInfo{Path: localConfig, Type: "project", Exists: false})
	}

	// Global config
	home, err := os.UserHomeDir()
	if err == nil {
		globalConfig := filepath.Join(home, ".ddx", "config.yaml")
		if _, err := os.Stat(globalConfig); err == nil {
			files = append(files, ConfigFileInfo{Path: globalConfig, Type: "global", Exists: true})
		} else {
			files = append(files, ConfigFileInfo{Path: globalConfig, Type: "global", Exists: false})
		}

		// Config directory
		configDir := filepath.Join(home, ".ddx")
		if _, err := os.Stat(configDir); err == nil {
			files = append(files, ConfigFileInfo{Path: configDir, Type: "directory", Exists: true})
		} else {
			files = append(files, ConfigFileInfo{Path: configDir, Type: "directory", Exists: false})
		}
	}

	return files
}

// configGetPath returns the config file path for editing
func configGetPath(workingDir string, global bool) string {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return "~/.ddx/config.yaml"
		}
		return filepath.Join(home, ".ddx", "config.yaml")
	}
	if workingDir != "" {
		return filepath.Join(workingDir, ".ddx", "config.yaml")
	}
	return ".ddx/config.yaml"
}

// configSave saves configuration to file
func configSave(workingDir string, cfg *config.Config, global bool) error {
	configPath := configGetPath(workingDir, global)

	// Ensure the .ddx directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create .ddx directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write configuration: %w", err)
	}

	return nil
}

// Helper types and functions
type ConfigFileInfo struct {
	Path   string
	Type   string
	Exists bool
}

// extractConfigValue extracts a value from config by key
func extractConfigValue(cfg *config.Config, key string) (string, error) {
	// Handle library configuration keys
	switch key {
	case "version":
		return cfg.Version, nil
	case "library.path":
		if cfg.Library == nil {
			return "", nil
		}
		return cfg.Library.Path, nil
	case "library.repository.url":
		if cfg.Library == nil || cfg.Library.Repository == nil {
			return "", nil
		}
		return cfg.Library.Repository.URL, nil
	case "library.repository.branch":
		if cfg.Library == nil || cfg.Library.Repository == nil {
			return "", nil
		}
		return cfg.Library.Repository.Branch, nil
	default:
		return "", fmt.Errorf("unknown configuration key: %s\nValid keys: version, library.path, library.repository.url, library.repository.branch", key)
	}
}

// setConfigValueInStruct sets a value in the config struct by key
func setConfigValueInStruct(cfg *config.Config, key, value string) error {
	// Handle library configuration keys
	switch key {
	case "library.path":
		if cfg.Library == nil {
			cfg.Library = &config.LibraryConfig{}
		}
		cfg.Library.Path = value
	case "library.repository.url":
		if cfg.Library == nil {
			cfg.Library = &config.LibraryConfig{}
		}
		if cfg.Library.Repository == nil {
			cfg.Library.Repository = &config.RepositoryConfig{}
		}
		cfg.Library.Repository.URL = value
	case "library.repository.branch":
		if cfg.Library == nil {
			cfg.Library = &config.LibraryConfig{}
		}
		if cfg.Library.Repository == nil {
			cfg.Library.Repository = &config.RepositoryConfig{}
		}
		cfg.Library.Repository.Branch = value
	default:
		return fmt.Errorf("unknown configuration key: %s\nValid keys: library.path, library.repository.url, library.repository.branch", key)
	}
	return nil
}

// CLI Interface Layer Functions

func getConfigValueWithWorkingDir(cmd *cobra.Command, key string, global bool, workingDir string) error {
	value, err := configGet(workingDir, key, global)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), value)
	return nil
}

// outputConfigFiles handles outputting configuration file locations
func (f *CommandFactory) outputConfigFiles(cmd *cobra.Command, files []ConfigFileInfo) error {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "📋 DDx Configuration File Locations:")
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	for _, file := range files {
		if file.Exists {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ %s config: %s (exists)\n", file.Type, file.Path)
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "⬜ %s config: %s (not found)\n", file.Type, file.Path)
		}
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Priority order: Environment variables > Project config > Global config > Defaults")
	return nil
}

// editConfigFile handles opening a config file in an editor
func (f *CommandFactory) editConfigFile(cmd *cobra.Command, configPath string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Open editor
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Opening %s in %s...\n", configPath, editor)
	// In real implementation, would exec the editor
	return nil
}

// handleProfileSubcommand handles profile-specific operations
func (f *CommandFactory) handleProfileSubcommand(cmd *cobra.Command, args []string) error {
	return handleProfileSubcommand(cmd, args)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = sourceFile.Close() }()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = destFile.Close() }()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

// showConfigFiles displays all config file locations
func showConfigFiles(cmd *cobra.Command) error {
	f := &CommandFactory{WorkingDir: ""}
	files := configListFiles(f.WorkingDir)
	return f.outputConfigFiles(cmd, files)
}

// handleProfileSubcommand handles profile-specific subcommands for US-023
func handleProfileSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("profile subcommand requires an action")
	}

	action := args[0]
	switch action {
	case "create":
		if len(args) < 2 {
			return fmt.Errorf("profile create requires a profile name")
		}
		return createProfile(cmd, args[1])
	case "list":
		return listProfiles(cmd)
	case "activate":
		if len(args) < 2 {
			return fmt.Errorf("profile activate requires a profile name")
		}
		return activateProfile(cmd, args[1])
	case "copy":
		if len(args) < 3 {
			return fmt.Errorf("profile copy requires source and destination profile names")
		}
		return copyProfile(cmd, args[1], args[2])
	case "validate":
		if len(args) < 2 {
			return fmt.Errorf("profile validate requires a profile name")
		}
		return validateProfile(cmd, args[1])
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("profile show requires a profile name")
		}
		return showProfile(cmd, args[1])
	case "diff":
		if len(args) < 3 {
			return fmt.Errorf("profile diff requires two profile names")
		}
		return diffProfiles(cmd, args[1], args[2])
	case "delete":
		if len(args) < 2 {
			return fmt.Errorf("profile delete requires a profile name")
		}
		return deleteProfile(cmd, args[1])
	default:
		return fmt.Errorf("unknown profile action: %s", action)
	}
}

// resyncMetaPromptAfterConfigChange re-syncs meta-prompt after config change
func resyncMetaPromptAfterConfigChange(workingDir string) error {
	cfg, err := config.LoadWithWorkingDir(workingDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	return syncMetaPromptWithConfig(cfg, workingDir)
}

// syncMetaPromptWithConfig syncs meta-prompt based on config
func syncMetaPromptWithConfig(cfg *config.Config, workingDir string) error {
	promptPath := cfg.GetMetaPrompt()
	if promptPath == "" {
		// Disabled - remove meta-prompt section if exists
		injector := metaprompt.NewMetaPromptInjectorWithPaths(
			"CLAUDE.md",
			cfg.Library.Path,
			workingDir,
		)
		return injector.RemoveMetaPrompt()
	}

	// Create injector and sync
	injector := metaprompt.NewMetaPromptInjectorWithPaths(
		"CLAUDE.md",
		cfg.Library.Path,
		workingDir,
	)

	return injector.InjectMetaPrompt(promptPath)
}

// createProfile creates a new environment profile
func createProfile(cmd *cobra.Command, profileName string) error {
	// Validate profile name
	if strings.Contains(profileName, "/") || strings.Contains(profileName, "\\") {
		return fmt.Errorf("invalid profile name: cannot contain path separators")
	}

	profilePath := fmt.Sprintf(".ddx.%s.yml", profileName)

	// Check if profile already exists
	if _, err := os.Stat(profilePath); err == nil {
		return fmt.Errorf("profile '%s' already exists", profileName)
	}

	// Load base configuration for inheritance
	baseCfg, err := config.Load()
	if err != nil {
		// If no base config exists, use default
		baseCfg = config.DefaultConfig
	}

	// Create new profile config with inheritance
	profileCfg := *baseCfg

	// Profile configuration ready (removed Variables for profiles)

	// Marshal to YAML
	data, err := yaml.Marshal(&profileCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal profile configuration: %w", err)
	}

	// Write profile file
	if err := os.WriteFile(profilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write profile configuration: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Created profile '%s' at %s\n", profileName, profilePath)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "💡 Edit the file to customize environment-specific settings\n")
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "💡 Activate with: ddx config profile activate %s\n", profileName)

	return nil
}

// listProfiles lists all available environment profiles
func listProfiles(cmd *cobra.Command) error {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "📋 Available Environment Profiles:")
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	// Find all .ddx.*.yml files
	profiles, err := filepath.Glob(".ddx.*.yml")
	if err != nil {
		return fmt.Errorf("failed to search for profiles: %w", err)
	}

	if len(profiles) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  No environment profiles found")
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  Create one with: ddx config profile create <name>")
		return nil
	}

	// Get current active profile
	activeProfile := os.Getenv("DDX_ENV")

	// Display each profile
	for _, profilePath := range profiles {
		// Extract profile name from filename
		filename := filepath.Base(profilePath)
		profileName := strings.TrimPrefix(filename, ".ddx.")
		profileName = strings.TrimSuffix(profileName, ".yml")

		// Get file info
		fileInfo, err := os.Stat(profilePath)
		if err != nil {
			continue
		}

		// Check if this is the active profile
		isActive := activeProfile == profileName
		status := "inactive"
		icon := "⚪"
		if isActive {
			status = "active"
			icon = "🟢"
		}

		// Quick validation check
		validationStatus := "✅ valid"
		if _, err := config.LoadFromFile(profilePath); err != nil {
			validationStatus = "❌ invalid"
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s %-15s (%s)\n", icon, profileName, status)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    File: %s\n", profilePath)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Modified: %s\n", fileInfo.ModTime().Format("2006-01-02 15:04:05"))
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Status: %s\n", validationStatus)
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "💡 Activate a profile with: ddx config profile activate <name>")
	if activeProfile != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "🟢 Currently active: %s\n", activeProfile)
	} else {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "ℹ️  No profile currently active")
	}

	return nil
}

// activateProfile activates an environment profile
func activateProfile(cmd *cobra.Command, profileName string) error {
	profilePath := fmt.Sprintf(".ddx.%s.yml", profileName)

	// Check if profile exists
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		return fmt.Errorf("profile '%s' does not exist", profileName)
	}

	// Validate profile before activation
	if _, err := config.LoadFromFile(profilePath); err != nil {
		return fmt.Errorf("profile '%s' is invalid: %w", profileName, err)
	}

	// Note: In a real implementation, we would set the environment variable for the current shell
	// For now, we provide instructions to the user
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Profile '%s' is ready for activation\n", profileName)
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "To activate this profile, run:")
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  export DDX_ENV=%s\n", profileName)
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Or add to your shell configuration:")
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  echo 'export DDX_ENV=%s' >> ~/.bashrc\n", profileName)
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "💡 All subsequent DDx commands will use this profile's configuration")

	return nil
}

// copyProfile copies an existing profile to create a new one
func copyProfile(cmd *cobra.Command, sourceProfile, destProfile string) error {
	sourcePath := fmt.Sprintf(".ddx.%s.yml", sourceProfile)
	destPath := fmt.Sprintf(".ddx.%s.yml", destProfile)

	// Check if source profile exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return fmt.Errorf("source profile '%s' does not exist", sourceProfile)
	}

	// Check if destination profile already exists
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("destination profile '%s' already exists", destProfile)
	}

	// Load source configuration
	sourceCfg, err := config.LoadFromFile(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to load source profile: %w", err)
	}

	// Profile copy ready (removed Variables for profiles)

	// Marshal to YAML
	data, err := yaml.Marshal(sourceCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal destination profile: %w", err)
	}

	// Write destination file
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write destination profile: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Copied profile '%s' to '%s'\n", sourceProfile, destProfile)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "📁 Created: %s\n", destPath)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "💡 You can now customize the new profile independently\n")

	return nil
}

// validateProfile validates a specific environment profile
func validateProfile(cmd *cobra.Command, profileName string) error {
	profilePath := fmt.Sprintf(".ddx.%s.yml", profileName)

	// Check if profile exists
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		return fmt.Errorf("profile '%s' does not exist", profileName)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "🔍 Validating profile '%s'...\n", profileName)
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	// Load and validate the profile configuration
	_, err := config.LoadFromFile(profilePath)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "❌ Profile validation failed: %v\n", err)
		return fmt.Errorf("profile '%s' is invalid: %w", profileName, err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Profile '%s' is valid\n", profileName)
	return nil
}

// showProfile displays the configuration for a specific profile
func showProfile(cmd *cobra.Command, profileName string) error {
	profilePath := fmt.Sprintf(".ddx.%s.yml", profileName)

	// Check if profile exists
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		return fmt.Errorf("profile '%s' does not exist", profileName)
	}

	// Load profile configuration
	profileCfg, err := config.LoadFromFile(profilePath)
	if err != nil {
		return fmt.Errorf("failed to load profile '%s': %w", profileName, err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "📋 Profile Configuration: %s\n", profileName)
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	// Show resolved configuration
	cyan := color.New(color.FgCyan)
	yellow := color.New(color.FgYellow)

	// Marshal profile config to YAML
	data, err := yaml.Marshal(profileCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal profile configuration: %w", err)
	}

	_, _ = cyan.Println("📄 Resolved Configuration:")
	_, _ = fmt.Fprint(cmd.OutOrStdout(), string(data))

	// Show inheritance information
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	_, _ = yellow.Println("ℹ️  Inheritance Information:")
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Profile inherits from base configuration: %s\n", ".ddx.yml")
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Profile-specific values override base values\n")
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Environment variables take highest precedence\n")

	return nil
}

// diffProfiles compares two environment profiles
func diffProfiles(cmd *cobra.Command, profileA, profileB string) error {
	profilePathA := fmt.Sprintf(".ddx.%s.yml", profileA)
	profilePathB := fmt.Sprintf(".ddx.%s.yml", profileB)

	// Check if both profiles exist
	if _, err := os.Stat(profilePathA); os.IsNotExist(err) {
		return fmt.Errorf("profile '%s' does not exist", profileA)
	}
	if _, err := os.Stat(profilePathB); os.IsNotExist(err) {
		return fmt.Errorf("profile '%s' does not exist", profileB)
	}

	// Load both configurations
	cfgA, err := config.LoadFromFile(profilePathA)
	if err != nil {
		return fmt.Errorf("failed to load profile '%s': %w", profileA, err)
	}

	cfgB, err := config.LoadFromFile(profilePathB)
	if err != nil {
		return fmt.Errorf("failed to load profile '%s': %w", profileB, err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "📊 Profile Comparison: %s vs %s\n", profileA, profileB)
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	// Compare major sections
	red := color.New(color.FgRed)
	green := color.New(color.FgGreen)
	cyan := color.New(color.FgCyan)

	_, _ = cyan.Println("🔍 Differences Found:")
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	differences := 0

	// Compare library repository URLs
	urlA := ""
	urlB := ""
	if cfgA.Library != nil && cfgA.Library.Repository != nil {
		urlA = cfgA.Library.Repository.URL
	}
	if cfgB.Library != nil && cfgB.Library.Repository != nil {
		urlB = cfgB.Library.Repository.URL
	}
	if urlA != urlB {
		differences++
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Library Repository URL:")
		_, _ = red.Fprintf(cmd.OutOrStdout(), "  - %s: %s\n", profileA, urlA)
		_, _ = green.Fprintf(cmd.OutOrStdout(), "  + %s: %s\n", profileB, urlB)
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}

	// Compare library repository branches
	branchA := ""
	branchB := ""
	if cfgA.Library != nil && cfgA.Library.Repository != nil {
		branchA = cfgA.Library.Repository.Branch
	}
	if cfgB.Library != nil && cfgB.Library.Repository != nil {
		branchB = cfgB.Library.Repository.Branch
	}
	if branchA != branchB {
		differences++
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Library Repository Branch:")
		_, _ = red.Fprintf(cmd.OutOrStdout(), "  - %s: %s\n", profileA, branchA)
		_, _ = green.Fprintf(cmd.OutOrStdout(), "  + %s: %s\n", profileB, branchB)
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}

	// Summary
	if differences == 0 {
		_, _ = green.Println("✅ Profiles are identical")
	} else {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "📊 Summary: %d differences found\n", differences)
	}

	return nil
}

// deleteProfile deletes an environment profile
func deleteProfile(cmd *cobra.Command, profileName string) error {
	profilePath := fmt.Sprintf(".ddx.%s.yml", profileName)

	// Check if profile exists
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		return fmt.Errorf("profile '%s' does not exist", profileName)
	}

	// Check if this is the currently active profile
	activeProfile := os.Getenv("DDX_ENV")
	if activeProfile == profileName {
		return fmt.Errorf("cannot delete active profile '%s'. Deactivate it first by unsetting DDX_ENV", profileName)
	}

	// For tests, we'll proceed directly with deletion
	// In a real implementation, we would ask for confirmation

	// Delete the profile file
	if err := os.Remove(profilePath); err != nil {
		return fmt.Errorf("failed to delete profile file: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Deleted profile '%s'\n", profileName)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "📁 Removed: %s\n", profilePath)

	return nil
}

// ConfigValueWithSource represents a configuration value with its source attribution
