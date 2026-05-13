package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/metaprompt"
	"github.com/DocumentDrivenDX/ddx/internal/metric"
	"github.com/DocumentDrivenDX/ddx/internal/persona"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/DocumentDrivenDX/ddx/internal/update"
	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const versionTemplate = `DDx {{.Version}}
Commit: {{index .Annotations "commit"}}
Built: {{index .Annotations "date"}}
`

// CommandFactory creates fresh command instances without global state
type CommandFactory struct {
	// Configuration options
	Version string
	Commit  string
	Date    string

	// Working directory (injected once at startup)
	WorkingDir string

	// AgentRunnerOverride overrides the agent runner used by the
	// execute-bead path (legacy agent.AgentRunner interface). For testing.
	AgentRunnerOverride agent.AgentRunner

	// ExecAgentRunnerOverride overrides the agent runner used by execStore.
	// SD-024 Stage 2: distinct from AgentRunnerOverride because the exec
	// AgentRunner interface has migrated to (ctx, ResolvedConfig, runtime),
	// while agent.AgentRunner still uses the legacy RunArgs signature.
	// For testing.
	ExecAgentRunnerOverride ddxexec.AgentRunner

	// executeBeadGitOverride overrides git operations in execute-bead worker (for testing).
	executeBeadGitOverride agent.GitOps

	// executeBeadOrchestratorGitOverride overrides git operations in the
	// execute-bead orchestrator (LandBeadResult) for testing.
	executeBeadOrchestratorGitOverride agent.OrchestratorGitOps

	// executeBeadLandingGitOverride overrides the LandingGitOps used by the
	// single-bead CLI and the queue-work LandCoordinator for testing.
	executeBeadLandingGitOverride agent.LandingGitOps

	// executeBeadLandingAdvancerOverride, when non-nil, replaces the default
	// Land() wrapper used by the interactive single-bead CLI with a custom
	// callback. Tests inject this to assert on the number of land calls
	// without needing a real git repo.
	executeBeadLandingAdvancerOverride func(res *agent.ExecuteBeadResult) (*agent.LandResult, error)

	// syncGitRunnerOverride replaces the real git runner in 'ddx sync' (for testing).
	syncGitRunnerOverride syncGitRunner

	// tryExecutorOverride, when non-nil, replaces the real executor used by
	// "ddx try". Injected by tests to observe claim/execute/outcome without
	// shelling out to a real agent harness.
	tryExecutorOverride agent.ExecuteBeadExecutor

	// resourceCheckerOverride, when non-nil, replaces the default execution
	// resource preflight used by ddx try and ddx work.
	resourceCheckerOverride agent.ExecutionResourceChecker

	// preflightWarnOnce ensures the project-local skill layout warning fires at
	// most once per process for long-running entrypoints (work, try).
	preflightWarnOnce sync.Once

	// serverListenAndServeOverride, when non-nil, replaces srv.ListenAndServeTLS
	// in the server RunE. Used by tests to verify server startup behavior without
	// actually binding to a port.
	serverListenAndServeOverride func(string, string) error

	// Custom viper instance for isolation
	viperInstance *viper.Viper

	// Update checker instance (stores check result for PostRunE)
	updateChecker *update.Checker
	updateDone    chan struct{}
	updateMu      sync.Mutex
}

// NewCommandFactory creates a new command factory with default settings
func NewCommandFactory(workingDir string) *CommandFactory {
	return &CommandFactory{
		Version:       Version,
		Commit:        Commit,
		Date:          Date,
		WorkingDir:    workingDir,
		viperInstance: viper.New(),
	}
}

// NewRootCommand creates a fresh root command with all subcommands
func (f *CommandFactory) NewRootCommand() *cobra.Command {
	// Local flag variables scoped to this command instance
	var cfgFile string
	var verbose bool
	var libraryPath string

	// Create fresh root command
	rootCmd := &cobra.Command{
		Use:   "ddx",
		Short: "Document-Driven Development eXperience - AI development toolkit",
		Long: color.New(color.FgCyan).Sprint(banner) + `
DDx is a toolkit for AI-assisted development that helps you:

• Share templates, prompts, and patterns across projects
• Maintain consistent development practices
• Integrate AI tooling seamlessly
• Install personas, prompts, and templates from the library

Get started:
  ddx init          Initialize DDx in your project
  ddx list          See available resources
  ddx doctor        Check installation and diagnose issues
  ddx cleanup       Inspect and clean stale DDx execution resources

More information:
  Documentation: https://github.com/DocumentDrivenDX/ddx
  Issues & Support: https://github.com/DocumentDrivenDX/ddx/issues`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if verbose {
				fmt.Printf("DDx %s (commit: %s, built: %s)\n", f.Version, f.Commit, f.Date)
			}
		},
	}

	rootCmd.Version = formattedVersion(f.Version)
	rootCmd.Annotations = map[string]string{
		"commit": f.Commit,
		"date":   f.Date,
	}
	rootCmd.SetVersionTemplate(versionTemplate)

	// Setup flags - these are now local to this command instance
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ddx.yml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVar(&libraryPath, "library-base-path", "", "override path for DDx library location")

	// Store flag values in command context for access by subcommands
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Initialize config with the local viper instance
		f.initConfig(cfgFile, libraryPath)

		// Version gate: block old binary from running in newer project
		if err := f.checkVersionGate(cmd); err != nil {
			return err
		}

		// Check for updates (synchronous, once per 24h)
		f.checkForUpdates(cmd)

		// Call the original PersistentPreRun if it exists
		if rootCmd.PersistentPreRun != nil {
			rootCmd.PersistentPreRun(cmd, args)
		}
		return nil
	}

	// Display update notification and staleness hints after command completes
	rootCmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		if err := f.displayUpdateNotification(cmd); err != nil {
			return err
		}
		f.displayStalenessHints(cmd)
		return nil
	}

	// Add all subcommands
	f.registerSubcommands(rootCmd)

	// Keep the persona APIs in the static production graph. Cobra dispatch
	// does not always show up in deadcode RTA, so this explicit hook keeps the
	// real CLI implementations visible without changing runtime behavior.
	persona.KeepReachabilityForDeadcode()
	metaprompt.KeepReachabilityForDeadcode()
	metric.KeepReachabilityForDeadcode()

	return rootCmd
}

func formattedVersion(version string) string {
	if version == "dev" {
		return "v0.0.1-dev"
	}
	if !strings.HasPrefix(version, "v") {
		return "v" + version
	}
	return version
}

func (f *CommandFactory) versionOutput() string {
	return fmt.Sprintf("DDx %s\nCommit: %s\nBuilt: %s\n", formattedVersion(f.Version), f.Commit, f.Date)
}

// initConfig initializes configuration for this command instance
func (f *CommandFactory) initConfig(cfgFile, libPath string) {
	// Store library path override if provided
	if libPath != "" {
		_ = os.Setenv("DDX_LIBRARY_BASE_PATH", libPath)
	}

	if cfgFile != "" {
		// Use config file from the flag
		f.viperInstance.SetConfigFile(cfgFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		if err == nil {
			// Search for config in home directory with name ".ddx" (without extension)
			f.viperInstance.AddConfigPath(home)
			f.viperInstance.AddConfigPath(".")
			f.viperInstance.SetConfigType("yaml")
			f.viperInstance.SetConfigName(".ddx")
		}
	}

	f.viperInstance.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in
	if err := f.viperInstance.ReadInConfig(); err == nil {
		if verbose := f.viperInstance.GetBool("verbose"); verbose {
			_, _ = fmt.Fprintln(os.Stderr, "Using config file:", f.viperInstance.ConfigFileUsed())
		}
	}
}

// checkForUpdates performs automatic update check (synchronous, once per 24h)
func (f *CommandFactory) checkForUpdates(cmd *cobra.Command) {
	// Check if disabled via env var
	if os.Getenv("DDX_DISABLE_UPDATE_CHECK") == "1" {
		return
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		// Silent failure - use defaults
		cfg = config.DefaultNewConfig()
	}

	// Check if disabled via config
	if cfg.UpdateCheck != nil && !cfg.UpdateCheck.Enabled {
		return
	}

	// Create checker and perform check (synchronous)
	checker := update.NewChecker(f.Version, cfg)
	f.updateMu.Lock()
	f.updateChecker = checker
	f.updateDone = make(chan struct{})
	done := f.updateDone
	f.updateMu.Unlock()

	// Run the check asynchronously so command startup is never blocked.
	go func() {
		defer close(done)
		_, _ = checker.CheckForUpdate(context.Background())
		// Also refresh plugin version cache while we have a network call in flight.
		f.refreshPluginVersionCache()
	}()
}

// displayUpdateNotification shows update notification if available
func (f *CommandFactory) displayUpdateNotification(cmd *cobra.Command) error {
	// Skip if disabled via environment variable
	if os.Getenv("DDX_DISABLE_UPDATE_CHECK") == "1" {
		return nil
	}

	f.updateMu.Lock()
	checker := f.updateChecker
	done := f.updateDone
	f.updateMu.Unlock()

	if checker == nil {
		return nil
	}

	if done != nil {
		select {
		case <-done:
		default:
			return nil
		}
	}

	// Skip update notification for help commands
	if cmd.Name() == "help" || cmd.Parent() != nil && cmd.Parent().Name() == "help" {
		return nil
	}

	// Skip update notification for machine-readable output formats
	jsonFlag, _ := cmd.Flags().GetBool("json")
	if jsonFlag {
		return nil
	}

	// Skip update notification if silent flag is set
	silentFlag, _ := cmd.Flags().GetBool("silent")
	if silentFlag {
		return nil
	}

	// Skip update notification if --no-check flag is set (for version command)
	noCheck, _ := cmd.Flags().GetBool("no-check")
	if noCheck {
		return nil
	}

	// Skip when stdout is not a terminal (pipes, scripts, CI)
	if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		return nil
	}

	available, version, err := checker.IsUpdateAvailable()
	if err != nil || !available {
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: Could not check for updates: %v\n", err)
		}
		return nil
	}

	// Show update notification with changelog for version command
	// Always write to stderr so it doesn't corrupt JSON pipelines
	isVersionCmd := cmd.Use == "version"
	if isVersionCmd {
		_, _ = fmt.Fprintf(os.Stderr,
			"\n⬆️  Update available: %s (run 'ddx upgrade' to install)\n\nWhat's new:\n  • Performance improvements\n  • Bug fixes\n  • New features\n",
			version)
	} else {
		_, _ = fmt.Fprintf(os.Stderr,
			"\n⬆️  Update available: %s (run 'ddx upgrade' to install)\n",
			version)
	}

	return nil
}

// checkVersionGate blocks execution if the binary is older than the project requires.
// Returns an error for non-exempt commands when binary version < project's ddx_version.
func (f *CommandFactory) checkVersionGate(cmd *cobra.Command) error {
	// Dev builds bypass the gate
	if f.Version == "" || f.Version == "dev" {
		return nil
	}

	// Read project versions
	projectVersion := readProjectVersions(f.WorkingDir)
	if projectVersion == "" || projectVersion == "dev" {
		return nil // No versions.yaml or dev project — skip
	}

	// Exempt commands that must work even when binary is too old
	switch cmd.Name() {
	case "upgrade", "version", "doctor", "init", "help", "completion":
		return nil
	}

	// Compare: is the binary older than what the project requires?
	needsUpgrade, err := update.NeedsUpgrade(f.Version, projectVersion)
	if err != nil {
		return nil // Parse error — don't block
	}
	if needsUpgrade {
		return fmt.Errorf("this project requires DDx v%s or newer (you have %s).\nRun 'ddx upgrade' to update your DDx binary",
			strings.TrimPrefix(projectVersion, "v"),
			f.Version)
	}

	return nil
}

// displayStalenessHints shows soft hints when project skills or plugins are outdated.
func (f *CommandFactory) displayStalenessHints(cmd *cobra.Command) {
	// Skip for machine-readable output
	if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
		return
	}
	if silentFlag, _ := cmd.Flags().GetBool("silent"); silentFlag {
		return
	}
	// Skip when stdout is not a terminal (pipes, scripts, CI)
	if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		return
	}

	// Dev builds don't hint
	if f.Version == "" || f.Version == "dev" {
		return
	}

	// Check project staleness
	projectVersion := readProjectVersions(f.WorkingDir)
	if projectVersion != "" && projectVersion != "dev" {
		// Is the binary newer than the project?
		projectNeedsUpgrade, err := update.NeedsUpgrade(projectVersion, f.Version)
		if err == nil && projectNeedsUpgrade {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"\n💡 Project skills from DDx v%s (you have %s). Run 'ddx init --force' to update.\n",
				strings.TrimPrefix(projectVersion, "v"),
				f.Version)
		}
	}

	// Check plugin staleness (reuse ddx outdated logic)
	f.displayPluginStalenessHints(cmd)
}

// displayPluginStalenessHints reads the plugin version cache and shows hints for
// any installed plugins that have a newer version available.
func (f *CommandFactory) displayPluginStalenessHints(cmd *cobra.Command) {
	state, err := registry.LoadState()
	if err != nil || len(state.Installed) == 0 {
		return
	}

	cache := update.NewPluginCache()
	if err := cache.Load(); err != nil {
		return // no cache yet — check runs in background, hints appear next time
	}

	for _, entry := range state.Installed {
		latestVersion, ok := cache.Data.Plugins[entry.Name]
		if !ok {
			continue
		}
		needsUpgrade, err := update.NeedsUpgrade(entry.Version, latestVersion)
		if err == nil && needsUpgrade {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"\n💡 %s %s installed, %s available. Run 'ddx install %s' to update.\n",
				entry.Name, entry.Version, latestVersion, entry.Name)
		}
	}
}

// refreshPluginVersionCache fetches the latest version for each installed plugin
// from GitHub and writes the result to the plugin cache. Called from the
// background update goroutine — failures are silent.
func (f *CommandFactory) refreshPluginVersionCache() {
	cache := update.NewPluginCache()
	_ = cache.Load() // ignore missing-file error

	if !cache.IsExpired() {
		return
	}

	state, err := registry.LoadState()
	if err != nil || len(state.Installed) == 0 {
		return
	}

	reg := registry.BuiltinRegistry()
	updated := false
	for _, entry := range state.Installed {
		pkg, err := reg.Find(entry.Name)
		if err != nil {
			continue
		}
		release, err := update.FetchLatestReleaseForRepo(pkg.Source)
		if err != nil {
			continue
		}
		latestVersion := strings.TrimPrefix(release.TagName, "v")
		cache.Data.Plugins[entry.Name] = latestVersion
		updated = true
	}

	if updated {
		cache.Data.LastCheck = time.Now()
		_ = cache.Save()
	}
}

// registerSubcommands adds all subcommands to the root command
func (f *CommandFactory) registerSubcommands(rootCmd *cobra.Command) {
	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			_, _ = fmt.Fprint(cmd.OutOrStdout(), f.versionOutput())
			f.warnIfInstalledBinaryBehindSource(cmd)

			// Check for --no-check flag
			noCheck, _ := cmd.Flags().GetBool("no-check")
			_ = noCheck // TODO: Implement update checking when this flag is used
			// For now, we don't check for updates
		},
	}
	versionCmd.Flags().Bool("no-check", false, "Skip checking for updates")
	rootCmd.AddCommand(versionCmd)

	// Completion command
	completionCmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate completion script",
		Long: `To configure your shell to load completions:

Bash:
  echo 'source <(ddx completion bash)' >> ~/.bashrc

Zsh:
  echo 'source <(ddx completion zsh)' >> ~/.zshrc

Fish:
  ddx completion fish | source

PowerShell:
  ddx completion powershell | Out-String | Invoke-Expression
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Run: func(cmd *cobra.Command, args []string) {
			switch args[0] {
			case "bash":
				_ = rootCmd.GenBashCompletion(os.Stdout)
			case "zsh":
				_ = rootCmd.GenZshCompletion(os.Stdout)
			case "fish":
				_ = rootCmd.GenFishCompletion(os.Stdout, true)
			case "powershell":
				_ = rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
			}
		},
	}
	rootCmd.AddCommand(completionCmd)

	// Register all other commands
	rootCmd.AddCommand(f.newInitCommand())
	rootCmd.AddCommand(f.newListCommand())
	rootCmd.AddCommand(f.newDoctorCommand())
	rootCmd.AddCommand(f.newCleanupCommand())
	rootCmd.AddCommand(f.newUpdateCommand())
	rootCmd.AddCommand(f.newUpgradeCommand())
	rootCmd.AddCommand(f.newConfigCommand())
	rootCmd.AddCommand(f.newPersonaCommand())
	rootCmd.AddCommand(f.newStatusCommand())
	rootCmd.AddCommand(f.newLogCommand())
	rootCmd.AddCommand(f.newBeadCommand())
	rootCmd.AddCommand(f.newArtifactCommand())
	rootCmd.AddCommand(f.newExecCommand())
	rootCmd.AddCommand(f.newMetricCommand())
	rootCmd.AddCommand(f.newMetricsCommand())
	rootCmd.AddCommand(f.newAgentCommand())
	rootCmd.AddCommand(f.newAcCommand())
	rootCmd.AddCommand(f.newDocCommand())
	rootCmd.AddCommand(f.newCheckpointCommand())
	rootCmd.AddCommand(f.newServerCommand())
	rootCmd.AddCommand(f.newSkillsCommand())
	rootCmd.AddCommand(f.newPluginCommand())
	rootCmd.AddCommand(f.newInstallCommand())
	rootCmd.AddCommand(f.newInstalledCommand())
	rootCmd.AddCommand(f.newUninstallCommand())
	rootCmd.AddCommand(f.newSearchCommand())
	rootCmd.AddCommand(f.newOutdatedCommand())
	rootCmd.AddCommand(f.newVerifyCommand())
	rootCmd.AddCommand(f.newJqCommand())
	rootCmd.AddCommand(f.newRunCommand())
	rootCmd.AddCommand(f.newWorkCommand())
	rootCmd.AddCommand(f.newTryCommand())
	rootCmd.AddCommand(f.newSyncCommand())

	// Add prompts command group
	promptsCmd := &cobra.Command{
		Use:     "prompts",
		Short:   "Manage AI prompts",
		Aliases: []string{"prompt"},
	}
	promptsCmd.AddCommand(f.newPromptsListCommand())
	promptsCmd.AddCommand(f.newPromptsShowCommand())
	rootCmd.AddCommand(promptsCmd)
}

func (f *CommandFactory) warnIfInstalledBinaryBehindSource(cmd *cobra.Command) {
	if f.Version == "" || f.Version == "dev" {
		return
	}
	if f.Commit == "" || f.Commit == "unknown" {
		return
	}

	repoRoot := gitpkg.FindProjectRoot(f.WorkingDir)
	if repoRoot == "" || !gitpkg.IsRepository(repoRoot) {
		return
	}

	originURL, err := gitCommandOutput(repoRoot, "remote", "get-url", "origin")
	if err != nil || !isDDXOriginURL(originURL) {
		return
	}

	buildSHA, err := gitResolveCommit(repoRoot, f.Commit)
	if err != nil {
		return
	}
	headSHA, err := gitCommandOutput(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return
	}

	if err := gitpkg.Command(context.Background(), repoRoot, "merge-base", "--is-ancestor", buildSHA, headSHA).Run(); err != nil {
		return
	}

	aheadCount, err := gitCommandOutput(repoRoot, "rev-list", "--count", buildSHA+".."+headSHA)
	if err != nil || aheadCount == "0" {
		return
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"WARNING: installed ddx is built from %s; source tree HEAD is %s (%s commits ahead). Run \"make install\" to refresh.\n",
		buildSHA, headSHA, aheadCount)
}

func gitCommandOutput(dir string, args ...string) (string, error) {
	out, err := gitpkg.Command(context.Background(), dir, args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitResolveCommit(dir, commit string) (string, error) {
	return gitCommandOutput(dir, "rev-parse", "--verify", commit+"^{commit}")
}

func isDDXOriginURL(url string) bool {
	return strings.Contains(strings.ToLower(url), "documentdrivendx/ddx")
}

// resolveAgentSession looks up a session by ID from the agent session log.
// Returns nil if the session is not found or cannot be read.
func (f *CommandFactory) resolveAgentSession(sessionID string) *agent.SessionEntry {
	if sessionID == "" {
		return nil
	}

	cfg, err := config.LoadWithWorkingDir(f.WorkingDir)
	if err != nil {
		return nil
	}

	sessionLogDir := ".ddx/agent-logs"
	if cfg.Agent != nil && cfg.Agent.SessionLogDir != "" {
		sessionLogDir = cfg.Agent.SessionLogDir
	}

	logDir := agent.ResolveLogDir(f.WorkingDir, sessionLogDir)
	idx, ok, err := agent.FindSessionIndex(logDir, sessionID)
	if err != nil {
		return nil
	}
	if ok {
		entry := agent.SessionIndexEntryToLegacy(idx)
		bodies := agent.LoadSessionBodies(f.WorkingDir, idx)
		entry.Prompt = bodies.Prompt
		entry.Response = bodies.Response
		entry.Stderr = bodies.Stderr
		return &entry
	}
	return nil
}
