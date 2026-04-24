package cmd

import (
	"github.com/spf13/cobra"
)

// newInitCommand creates a fresh init command
func (f *CommandFactory) newInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize DDx in current project",
		Long: `Initialize DDx in the current project.

This command:
• Creates a .ddx/config.yaml configuration file
• Installs the default DDx library plugin
• Commits the config file to git

Examples:
  ddx init                  # Initialize DDx in current project
  ddx init --force          # Reinitialize existing project
  ddx init --no-git         # Skip git operations`,
		Args: cobra.NoArgs,
		RunE: f.runInit,
	}

	cmd.Flags().BoolP("force", "f", false, "Force initialization even if DDx already exists")
	cmd.Flags().Bool("no-git", false, "Skip git operations")
	cmd.Flags().Bool("silent", false, "Suppress all output except errors")
	cmd.Flags().Bool("skip-claude-injection", false, "Skip injecting meta-prompts into CLAUDE.md")
	cmd.Flags().String("repository", "", "Library repository URL (default: https://github.com/DocumentDrivenDX/ddx-library)")
	cmd.Flags().String("branch", "", "Library repository branch (default: main)")

	return cmd
}

// newListCommand creates a fresh list command
func (f *CommandFactory) newListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list [type]",
		Short:   "List available DDx resources",
		Aliases: []string{"ls"},
		Long: `List available DDx resources.

Resources include:
• Templates - Complete project setups
• Patterns - Reusable code patterns
• Prompts - AI interaction prompts
• Scripts - Automation scripts
• Configs - Tool configurations

Examples:
  ddx list              # List all resources
  ddx list templates    # List only templates
  ddx list patterns     # List only patterns`,
		Args: cobra.MaximumNArgs(1),
		RunE: f.runList,
	}

	cmd.Flags().BoolP("detailed", "d", false, "Show detailed information")
	cmd.Flags().StringP("filter", "f", "", "Filter resources by name")
	cmd.Flags().Bool("json", false, "Output results as JSON")
	cmd.Flags().Bool("tree", false, "Display resources in tree format")

	return cmd
}

// newDoctorCommand creates a fresh doctor command
func (f *CommandFactory) newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check installation health and diagnose issues",
		Long: `Doctor checks your DDx installation and environment.

This command verifies:
• DDx binary and PATH configuration
• Git installation and availability
• File system permissions
• Network connectivity
• Library path accessibility
• Project structure and configuration
• Development tool setup
• AI integration readiness

The doctor helps identify and resolve:
• Installation issues
• Configuration problems
• Missing dependencies
• Environment setup issues`,
		Args: cobra.NoArgs,
		RunE: f.runDoctor,
	}

	cmd.Flags().BoolP("verbose", "v", false, "Show detailed diagnostic output")
	cmd.Flags().Bool("plugins", false, "Audit installed plugins for manifest and skill issues")
	cmd.Flags().Bool("fix", false, "Automatically remediate detected git-repo corruption (core.bare/core.worktree)")

	return cmd
}

// newUpdateCommand creates a fresh update command
func (f *CommandFactory) newUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [target]",
		Short: "Update DDx CLI or installed plugins",
		Long: `Update DDx CLI or installed plugins to their latest versions.

Targets:
  ddx        - Update DDx CLI to latest release
  helix      - Update helix plugin to latest version
  all        - Update everything (default)

Examples:
  ddx update           # Update all installed packages
  ddx update ddx       # Update DDx CLI only
  ddx update helix    # Update helix plugin only
  ddx update --check   # Check for updates without applying`,
		Args: cobra.MaximumNArgs(1),
		RunE: f.runUpdate,
	}

	cmd.Flags().Bool("check", false, "Check for updates without applying")
	cmd.Flags().Bool("force", false, "Force update even if already latest")

	return cmd
}

// newUpgradeCommand creates a fresh upgrade command
func (f *CommandFactory) newUpgradeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade DDx to the latest version",
		Long: `Upgrade DDx binary to the latest release version.

This command:
• Checks for the latest DDx release on GitHub
• Downloads and executes the installation script
• Replaces the current binary with the latest version
• Preserves all your project configurations

Examples:
  ddx upgrade              # Upgrade to latest version
  ddx upgrade --check      # Only check for updates
  ddx upgrade --force      # Force upgrade even if already latest`,
		Args: cobra.NoArgs,
		RunE: f.runUpgrade,
	}

	cmd.Flags().Bool("check", false, "Check for updates without upgrading")
	cmd.Flags().Bool("force", false, "Force upgrade even if already on latest version")

	return cmd
}

// newConfigCommand creates a fresh config command
func (f *CommandFactory) newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configure DDx settings",
		Long: `Configure DDx settings and preferences.

This command allows you to:
• View current configuration
• Modify settings interactively
• Set individual configuration values
• Reset to defaults

Examples:
  ddx config                    # Show help
  ddx config set key value      # Set specific value
  ddx config get key            # Get specific value
  ddx config edit               # Edit config in $EDITOR
  cat .ddx/config.yaml          # View current config`,
		RunE: f.runConfig,
	}

	cmd.Flags().Bool("show-files", false, "Display all config file locations")
	cmd.Flags().Bool("edit", false, "Edit configuration file directly")
	cmd.Flags().Bool("reset", false, "Reset to default configuration")
	cmd.Flags().Bool("wizard", false, "Run configuration wizard")
	cmd.Flags().Bool("validate", false, "Validate configuration")
	cmd.Flags().Bool("global", false, "Use global configuration")

	// Enhanced validation flags for US-022
	cmd.Flags().String("file", "", "Validate specific configuration file")
	cmd.Flags().Bool("verbose", false, "Detailed validation output")
	cmd.Flags().Bool("offline", false, "Skip network checks during validation")

	return cmd
}

// newPersonaCommand creates a fresh persona command
func (f *CommandFactory) newPersonaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "persona",
		Short: "Manage AI personas for consistent interactions",
		Long: `Manage AI personas for consistent role-based interactions.

Personas provide:
• Consistent AI behavior across team members
• Specialized expertise for different roles
• Reusable personality templates
• Project-specific persona bindings

Examples:
  ddx persona --list              # List available personas
  ddx persona --show reviewer     # Show persona details
  ddx persona --bind strict-reviewer --role code-reviewer`,
		RunE: f.runPersona,
	}

	cmd.Flags().Bool("list", false, "List available personas")
	cmd.Flags().String("show", "", "Show details of a specific persona")
	cmd.Flags().String("bind", "", "Bind a persona to a role")
	cmd.Flags().String("role", "", "Role to bind persona to or filter by")
	cmd.Flags().String("tag", "", "Filter personas by tag")
	cmd.Flags().String("body", "", "Path to a markdown file with the persona body (new/edit)")
	cmd.Flags().String("as", "", "Optional new name when forking a library persona")

	return cmd
}

// newPromptsListCommand creates the prompts list subcommand
func (f *CommandFactory) newPromptsListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available prompts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.runPromptsList(cmd, args)
		},
	}
	cmd.Flags().String("search", "", "Search for prompts containing this text")
	return cmd
}

// newPromptsShowCommand creates the prompts show subcommand
func (f *CommandFactory) newPromptsShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <prompt-name>",
		Short: "Show a specific prompt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromptsShow(cmd, args)
		},
	}
}

// newStatusCommand creates a fresh status command
func (f *CommandFactory) newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show version and status information",
		Long: `Show comprehensive version and status information for your DDX project.

This command displays:
- Current DDX version and commit hash
- Last update timestamp
- Local modifications to DDX resources
- Available upstream updates
- Change history and differences

Examples:
  ddx status                          # Show basic status
  ddx status --verbose                # Show detailed information
  ddx status --check-upstream         # Check for updates
  ddx status --changes                # List changed files
  ddx status --diff                   # Show differences
  ddx status --export manifest.yml    # Export version manifest`,
		RunE: f.runStatus,
	}

	cmd.Flags().Bool("check-upstream", false, "Check for upstream updates")
	cmd.Flags().Bool("changes", false, "Show list of changed files")
	cmd.Flags().Bool("diff", false, "Show differences between versions")
	cmd.Flags().String("export", "", "Export version manifest to file")

	return cmd
}

// newLogCommand creates a fresh log command
func (f *CommandFactory) newLogCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show DDX asset history",
		Long: `Show commit history for DDX assets and resources.

This command displays the git log for your DDX resources, helping you
track changes, updates, and the evolution of your project setup.

Examples:
  ddx log                    # Show recent commit history
  ddx log -n 10              # Show last 10 commits
  ddx log --oneline          # Show compact format
  ddx log --since="1 week ago" # Show commits from last week`,
		RunE: f.runLog,
	}

	cmd.Flags().IntP("number", "n", 20, "Number of commits to show")
	cmd.Flags().Int("limit", 20, "Limit number of commits to show (same as --number)")
	cmd.Flags().Bool("oneline", false, "Show compact one-line format")
	cmd.Flags().Bool("diff", false, "Show changes in each commit")
	cmd.Flags().String("export", "", "Export history to file (format: .md, .json, .csv, .html)")
	cmd.Flags().String("since", "", "Show commits since date (e.g., '1 week ago')")
	cmd.Flags().String("author", "", "Filter by author")
	cmd.Flags().String("grep", "", "Filter by commit message")

	return cmd
}
