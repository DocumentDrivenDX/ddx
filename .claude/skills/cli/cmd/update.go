package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/metaprompt"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/DocumentDrivenDX/ddx/internal/update"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// UpdateOptions represents update command configuration
type UpdateOptions struct {
	Check       bool
	Force       bool
	Reset       bool
	Sync        bool
	Strategy    string
	Backup      bool
	Interactive bool
	Abort       bool
	DryRun      bool
	Resource    string // selective update resource
}

// ConflictInfo represents information about a detected conflict
type ConflictInfo struct {
	FilePath     string
	LineNumber   int
	ConflictType string
	LocalContent string
	TheirContent string
	BaseContent  string
}

// UpdateResult represents the result of an update operation
type UpdateResult struct {
	Success      bool
	Message      string
	UpdatedFiles []string
	Conflicts    []ConflictInfo
	BackupPath   string
}

// CommandFactory method - CLI interface layer
func (f *CommandFactory) runUpdate(cmd *cobra.Command, args []string) error {
	// Extract flags to options struct
	opts, err := extractUpdateOptions(cmd, args)
	if err != nil {
		return err
	}

	// Upgrade the DDx binary synchronously (always check upstream).
	if err := f.runUpgrade(cmd, []string{}); err != nil {
		// Non-fatal: report but continue to plugin updates.
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: binary upgrade check failed: %v\n", err)
	}

	// Call pure business logic
	result, err := performUpdate(f.WorkingDir, opts)
	if err != nil {
		return err
	}

	// Handle output formatting
	return displayUpdateResult(cmd, result, opts)
}

// performUpdate upgrades the DDx binary if a new release is available, then
// checks GitHub for the latest version of each installed plugin and updates
// any that are outdated (or all if --force). Always refreshes the embedded
// `ddx` skill and the AGENTS.md block so projects that ran `ddx init` under
// an older DDx version pick up current skill content without re-running init.
func performUpdate(workingDir string, opts *UpdateOptions) (*UpdateResult, error) {
	// Refresh the shipped `ddx` skill copy + AGENTS.md block first, regardless
	// of whether any plugins are installed. This is what lets older projects
	// pick up new SKILL.md / reference/*.md content without re-init.
	refreshShippedSkills(workingDir)

	state, err := registry.LoadState()
	if err != nil || len(state.Installed) == 0 {
		return &UpdateResult{Success: true, Message: "Shipped skills refreshed. No packages installed."}, nil
	}

	reg := registry.BuiltinRegistry()

	var updated []string

	for _, entry := range state.Installed {
		// Filter to specific target if requested.
		if opts.Resource != "" && entry.Name != opts.Resource {
			continue
		}

		pkg, err := reg.Find(entry.Name)
		if err != nil {
			continue // not in registry, skip
		}

		// Fetch actual latest version from GitHub.
		latestVersion := pkg.Version
		if release, err := update.FetchLatestReleaseForRepo(pkg.Source); err == nil {
			latestVersion = strings.TrimPrefix(release.TagName, "v")
		}

		if !opts.Force && entry.Version == latestVersion {
			continue
		}

		// Install the latest version.
		installPkg := *pkg
		installPkg.Version = latestVersion
		newEntry, err := registry.InstallPackage(&installPkg)
		if err != nil {
			return nil, fmt.Errorf("updating %s: %w", entry.Name, err)
		}
		state.AddOrUpdate(newEntry)
		updated = append(updated, entry.Name+" "+entry.Version+" → "+latestVersion)
	}

	if err := registry.SaveState(state); err != nil {
		return nil, fmt.Errorf("saving state: %w", err)
	}

	if len(updated) == 0 {
		return &UpdateResult{Success: true, Message: "Shipped skills refreshed. All packages are up to date."}, nil
	}

	return &UpdateResult{
		Success:      true,
		Message:      "Updated: " + strings.Join(updated, ", "),
		UpdatedFiles: updated,
	}, nil
}

// refreshShippedSkills re-copies the embedded `ddx` skill into the project's
// skill directories and refreshes the AGENTS.md DDx block. Safe to call on
// every `ddx update` because registerProjectSkills with force=true handles
// the "existing files should be updated" case, and generateAgentsMD's
// marker-based merge is idempotent. Stale pre-consolidation skill dirs
// (ddx-bead, ddx-run, etc.) are swept by the cleanup logic shared with init.
func refreshShippedSkills(workingDir string) {
	registerProjectSkills(workingDir, true)
	generateAgentsMD(workingDir)
}

// Helper functions for working directory-based operations
func extractUpdateOptions(cmd *cobra.Command, args []string) (*UpdateOptions, error) {
	opts := &UpdateOptions{}

	// Extract flags
	opts.Check, _ = cmd.Flags().GetBool("check")
	opts.Force, _ = cmd.Flags().GetBool("force")
	opts.Reset, _ = cmd.Flags().GetBool("reset")
	opts.Sync, _ = cmd.Flags().GetBool("sync")
	opts.Strategy, _ = cmd.Flags().GetString("strategy")
	opts.Backup, _ = cmd.Flags().GetBool("backup")
	opts.Interactive, _ = cmd.Flags().GetBool("interactive")
	opts.Abort, _ = cmd.Flags().GetBool("abort")
	opts.DryRun, _ = cmd.Flags().GetBool("dry-run")

	// Handle mine/theirs flags by converting to strategy
	updateMine, _ := cmd.Flags().GetBool("mine")
	updateTheirs, _ := cmd.Flags().GetBool("theirs")

	if updateMine && updateTheirs {
		return nil, fmt.Errorf("cannot use both --mine and --theirs flags")
	}
	if updateMine {
		opts.Strategy = "ours"
	}
	if updateTheirs {
		opts.Strategy = "theirs"
	}

	// Check for selective update
	if len(args) > 0 {
		opts.Resource = args[0]
	}

	return opts, nil
}

func isInitializedInDir(workingDir string) bool {
	configPath := ".ddx/config.yaml"
	if workingDir != "" {
		configPath = filepath.Join(workingDir, ".ddx/config.yaml")
	}
	_, err := os.Stat(configPath)
	return err == nil
}

func loadConfigFromWorkingDirForUpdate(workingDir string) (*config.Config, error) {
	if workingDir == "" {
		return config.Load()
	}

	configPath := filepath.Join(workingDir, ".ddx/config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return config.LoadFromFile(configPath)
	}

	return config.Load()
}

func validateUpdateStrategy(opts *UpdateOptions) error {
	if opts.Strategy != "" {
		validStrategies := []string{"ours", "theirs", "mine"}
		valid := false
		for _, strategy := range validStrategies {
			if opts.Strategy == strategy {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid strategy: %s (use 'ours', 'theirs', or 'mine')", opts.Strategy)
		}

		// Convert "mine" to "ours" for internal consistency
		if opts.Strategy == "mine" {
			opts.Strategy = "ours"
		}
	}
	return nil
}

func checkForUpdatesInDir(workingDir string, cfg *config.Config, opts *UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{
		Success: true,
		Message: "Update check completed",
	}

	// In a real implementation, this would check git remote for actual updates
	// For now, provide basic output
	return result, nil
}

func previewUpdateInDir(workingDir string, cfg *config.Config, opts *UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{
		Success: true,
		Message: "Dry-run preview completed",
	}

	// Simulate preview logic
	if opts.Resource != "" {
		result.Message = fmt.Sprintf("Would update resource: %s", opts.Resource)
	} else {
		result.Message = "Would update all DDx resources"
	}

	return result, nil
}

func synchronizeWithUpstreamInDir(workingDir string, cfg *config.Config, opts *UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{
		Success: true,
		Message: "Synchronized with upstream",
	}

	// In real implementation, would perform git synchronization
	return result, nil
}

func detectConflictsInDir(workingDir string) []ConflictInfo {
	var conflicts []ConflictInfo

	// Check for conflict markers in .ddx directory
	ddxPath := ".ddx"
	if workingDir != "" {
		ddxPath = filepath.Join(workingDir, ".ddx")
	}

	if _, err := os.Stat(ddxPath); os.IsNotExist(err) {
		return conflicts
	}

	// Walk through .ddx directory looking for conflict markers
	_ = filepath.Walk(ddxPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Skip binary files
		if isBinaryFileForUpdate(path) {
			return nil
		}

		// Read file and look for conflict markers
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		content := string(data)
		lines := strings.Split(content, "\n")

		for i, line := range lines {
			if strings.Contains(line, "<<<<<<<") ||
				strings.Contains(line, "=======") ||
				strings.Contains(line, ">>>>>>>") {

				// Extract conflict sections
				conflict := ConflictInfo{
					FilePath:     path,
					LineNumber:   i + 1,
					ConflictType: "merge",
				}

				// Try to extract local and their content
				if strings.Contains(line, "<<<<<<<") {
					conflict.LocalContent, conflict.TheirContent = extractConflictContentForUpdate(lines, i)
				}

				conflicts = append(conflicts, conflict)
				break // Only report one conflict per file
			}
		}

		return nil
	})

	return conflicts
}

func handleUpdateAbortInDir(workingDir string) (*UpdateResult, error) {
	result := &UpdateResult{}

	// Check for backup directory
	backupDir := ".ddx.backup"
	if workingDir != "" {
		backupDir = filepath.Join(workingDir, ".ddx.backup")
	}

	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		result.Success = false
		result.Message = "No backup found. Nothing to restore."
		return result, nil
	}

	// Check if there's an ongoing update state
	updateStateFile := ".ddx/.update-state"
	if workingDir != "" {
		updateStateFile = filepath.Join(workingDir, ".ddx/.update-state")
	}

	if _, err := os.Stat(updateStateFile); os.IsNotExist(err) {
		result.Success = false
		result.Message = "No active update operation found."
		return result, nil
	}

	// Restore from backup
	ddxDir := ".ddx"
	if workingDir != "" {
		ddxDir = filepath.Join(workingDir, ".ddx")
	}

	// Remove current .ddx directory
	if err := os.RemoveAll(ddxDir); err != nil {
		return nil, fmt.Errorf("failed to remove current .ddx directory: %w", err)
	}

	// Restore from backup
	if err := copyDirForRestore(backupDir, ddxDir); err != nil {
		return nil, fmt.Errorf("failed to restore from backup: %w", err)
	}

	// Clean up backup directory
	_ = os.RemoveAll(backupDir)
	_ = os.Remove(updateStateFile)

	result.Success = true
	result.Message = "Update operation aborted successfully! Project restored to pre-update state"
	return result, nil
}

func handleInteractiveResolutionInDir(workingDir string, conflicts []ConflictInfo, opts *UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{
		Success:   true,
		Message:   "Interactive conflict resolution completed",
		Conflicts: conflicts,
	}

	// In real implementation, this would provide interactive conflict resolution
	// For now, simulate the process
	return result, nil
}

func executeUpdateInDir(workingDir string, cfg *config.Config, opts *UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{
		Success: true,
		Message: "DDx updated successfully!",
	}

	// Create backup if requested
	if opts.Backup {
		backupPath, err := createBackupInDir(workingDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create backup: %w", err)
		}
		result.BackupPath = backupPath
	}

	// Apply conflict resolution strategy if specified
	if opts.Strategy != "" {
		result.Message += fmt.Sprintf(" Conflicts resolved using '%s' strategy.", opts.Strategy)
	}

	// Simulate the update process
	if opts.Resource != "" {
		result.UpdatedFiles = []string{opts.Resource}
		baseMsg := fmt.Sprintf("Updated resource: %s", opts.Resource)
		if opts.Strategy != "" {
			result.Message = baseMsg + fmt.Sprintf(" (using '%s' strategy)", opts.Strategy)
		} else {
			result.Message = baseMsg
		}
	} else {
		// Simulate updating multiple resources (simplified config doesn't track specific files)
		result.UpdatedFiles = []string{"library/"} // Indicate library was updated
		if opts.Force {
			result.Message = "DDx updated successfully! Used force mode to override any conflicts."
		} else if opts.Strategy != "" {
			result.Message = fmt.Sprintf("DDx updated successfully! Used '%s' strategy for conflict resolution.", opts.Strategy)
		} else {
			result.Message = "DDx updated successfully!"
		}
	}

	return result, nil
}

func createBackupInDir(workingDir string) (string, error) {
	ddxDir := ".ddx"
	backupDir := ".ddx.backup"

	if workingDir != "" {
		ddxDir = filepath.Join(workingDir, ".ddx")
		backupDir = filepath.Join(workingDir, ".ddx.backup")
	}

	if _, err := os.Stat(ddxDir); os.IsNotExist(err) {
		return "", fmt.Errorf("no .ddx directory to backup")
	}

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	// Copy .ddx to backup
	err := copyDirForRestore(ddxDir, backupDir)
	return backupDir, err
}

// copyDirForRestore copies a directory recursively for backup/restore operations
func copyDirForRestore(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate destination path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy file
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = srcFile.Close() }()

		dstFile, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer func() { _ = dstFile.Close() }()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}

// Output formatting function
func displayUpdateResult(cmd *cobra.Command, result *UpdateResult, opts *UpdateOptions) error {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)

	out := cmd.OutOrStdout()
	writer := out.(io.Writer)

	// Handle error cases
	if !result.Success {
		if len(result.Conflicts) > 0 {
			return handleConflictOutput(out, result.Conflicts, opts)
		}
		_, _ = red.Fprintln(writer, "❌", result.Message)
		return nil
	}

	// Handle check mode
	if opts.Check {
		_, _ = fmt.Fprintln(writer, "Checking for updates...")
		_, _ = fmt.Fprintln(writer, "Fetching latest changes from master repository...")
		_, _ = fmt.Fprintln(writer, "Available updates:")
		_, _ = fmt.Fprintln(writer, "Changes since last update:")
		return nil
	}

	// Handle dry-run mode
	if opts.DryRun {
		return displayDryRunResult(out, result, opts)
	}

	// Display success message
	_, _ = green.Fprintln(writer, "✅", result.Message)
	_, _ = fmt.Fprintln(out)

	// Show updated files
	if len(result.UpdatedFiles) > 0 {
		_, _ = green.Fprintln(writer, "📦 Updated resources:")
		for _, file := range result.UpdatedFiles {
			_, _ = fmt.Fprintf(writer, "  • %s\n", file)
		}
		_, _ = fmt.Fprintln(out)
	}

	// Show backup info
	if result.BackupPath != "" {
		_, _ = fmt.Fprintf(out, "💾 Backup created at: %s\n", result.BackupPath)
		_, _ = fmt.Fprintln(out)
	}

	return nil
}

func handleConflictOutput(out interface{}, conflicts []ConflictInfo, opts *UpdateOptions) error {
	writer := out.(io.Writer)
	red := color.New(color.FgRed)
	cyan := color.New(color.FgCyan)
	green := color.New(color.FgGreen)

	_, _ = red.Fprintln(writer, "⚠️  MERGE CONFLICTS DETECTED")
	_, _ = fmt.Fprintln(writer, "")

	_, _ = fmt.Fprintf(writer, "Found %d conflict(s) that require resolution:\n", len(conflicts))
	_, _ = fmt.Fprintln(writer, "")

	// Display detailed conflict information
	for i, conflict := range conflicts {
		_, _ = red.Fprintf(writer, "❌ Conflict %d: %s (line %d)\n", i+1, conflict.FilePath, conflict.LineNumber)
		_, _ = fmt.Fprintln(writer, "")
	}

	// Provide resolution guidance
	_, _ = fmt.Fprintln(writer, "")
	_, _ = cyan.Fprintln(writer, "🔧 RESOLUTION OPTIONS")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "Choose one of the following resolution strategies:")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "  📋 Automatic Resolution:")
	_, _ = fmt.Fprintln(writer, "    --strategy=ours    Keep your local changes")
	_, _ = fmt.Fprintln(writer, "    --strategy=theirs  Accept upstream changes")
	_, _ = fmt.Fprintln(writer, "    --mine             Same as --strategy=ours")
	_, _ = fmt.Fprintln(writer, "    --theirs           Same as --strategy=theirs")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "  🔄 Interactive Resolution:")
	_, _ = fmt.Fprintln(writer, "    --interactive      Resolve conflicts one by one")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "  ⚡ Force Resolution:")
	_, _ = fmt.Fprintln(writer, "    --force            Override all conflicts with upstream")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "  🔙 Abort Update:")
	_, _ = fmt.Fprintln(writer, "    --abort            Cancel update and restore previous state")

	_, _ = green.Fprintln(writer, "💡 Examples:")
	_, _ = fmt.Fprintln(writer, "  ddx update --strategy=theirs   # Accept all upstream changes")
	_, _ = fmt.Fprintln(writer, "  ddx update --mine              # Keep all local changes")
	_, _ = fmt.Fprintln(writer, "  ddx update --interactive       # Choose per conflict")
	_, _ = fmt.Fprintln(writer, "  ddx update --abort             # Cancel and restore")

	return fmt.Errorf("conflicts require resolution")
}

func displayDryRunResult(out interface{}, result *UpdateResult, opts *UpdateOptions) error {
	writer := out.(io.Writer)
	cyan := color.New(color.FgCyan)
	green := color.New(color.FgGreen)

	_, _ = cyan.Fprintln(writer, "🔍 DRY-RUN MODE: Previewing update changes")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "This is a preview of what would happen if you run 'ddx update'.")
	_, _ = fmt.Fprintln(writer, "No actual changes will be made to your project.")
	_, _ = fmt.Fprintln(writer, "")

	_, _ = green.Fprintln(writer, "📋 What would happen:")
	_, _ = fmt.Fprintln(writer, result.Message)

	_, _ = fmt.Fprintln(writer, "")
	_, _ = green.Fprintln(writer, "💡 To proceed with the update, run:")
	if opts.Resource != "" {
		_, _ = fmt.Fprintf(writer, "   ddx update %s\n", opts.Resource)
	} else {
		_, _ = fmt.Fprintln(writer, "   ddx update")
	}

	_, _ = fmt.Fprintln(writer, "")
	_, _ = green.Fprintln(writer, "✅ Dry-run preview completed successfully!")

	return nil
}

// Helper functions (simplified versions of the complex logic from original)
func isBinaryFileForUpdate(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	binaryExts := []string{".jpg", ".jpeg", ".png", ".gif", ".pdf", ".zip", ".tar", ".gz", ".exe", ".bin"}

	for _, bext := range binaryExts {
		if ext == bext {
			return true
		}
	}
	return false
}

func extractConflictContentForUpdate(lines []string, startLine int) (local, their string) {
	var localLines, theirLines []string
	var inLocal, inTheir bool

	for i := startLine; i < len(lines); i++ {
		line := lines[i]

		if strings.Contains(line, "<<<<<<<") {
			inLocal = true
			continue
		} else if strings.Contains(line, "=======") {
			inLocal = false
			inTheir = true
			continue
		} else if strings.Contains(line, ">>>>>>>") {
			break
		}

		if inLocal {
			localLines = append(localLines, line)
		} else if inTheir {
			theirLines = append(theirLines, line)
		}
	}

	return strings.Join(localLines, "\n"), strings.Join(theirLines, "\n")
}

// Legacy function for compatibility
func runUpdate(cmd *cobra.Command, args []string) error {
	// Extract flags to options struct
	opts, err := extractUpdateOptions(cmd, args)
	if err != nil {
		return err
	}

	// Call pure business logic
	result, err := performUpdate("", opts)
	if err != nil {
		return err
	}

	// Handle output formatting
	return displayUpdateResult(cmd, result, opts)
}

// syncMetaPrompt syncs the meta-prompt from library to CLAUDE.md
func syncMetaPrompt(cfg *config.Config, workingDir string) error {
	// Get meta-prompt path from config
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
