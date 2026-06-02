package cmd

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/DocumentDrivenDX/ddx/internal/skills"
	"github.com/DocumentDrivenDX/ddx/internal/update"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// UpdateOptions represents update command configuration
type UpdateOptions struct {
	Check        bool
	Force        bool
	Reset        bool
	Sync         bool
	Strategy     string
	Backup       bool
	Interactive  bool
	Abort        bool
	DryRun       bool
	Resource     string // selective update resource
	DiscardLocal bool   // discard local changes when overwriting
	Global       bool   // update global plugin tree instead of project tree
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

	// --global updates only the machine-wide plugin tree.
	if opts.Global {
		result, err := performGlobalUpdate(opts)
		if err != nil {
			return err
		}
		return displayUpdateResult(cmd, result, opts)
	}

	// Call pure business logic
	result, err := performUpdate(f.WorkingDir, opts)
	if err != nil {
		return err
	}

	// Handle output formatting
	return displayUpdateResult(cmd, result, opts)
}

// isUpdateTargetDirty returns true when filePath exists, is tracked by git,
// and has uncommitted changes (staged or unstaged). Untracked files return false.
// Returns false on any error (git unavailable, not a repo, etc.).
func isUpdateTargetDirty(workingDir, filePath string) bool {
	abs := filePath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(workingDir, filePath)
	}
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		return false
	}

	rel, err := filepath.Rel(workingDir, abs)
	if err != nil {
		rel = filePath
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := gitpkg.Command(ctx, workingDir, "status", "--porcelain", "--", rel)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if len(line) < 2 {
			continue
		}
		// XY: X=index status, Y=worktree status. "??" = untracked — skip.
		if line[:2] != "??" {
			return true
		}
	}
	return false
}

// enumerateSkillUpdateTargets returns the destination paths that
// refreshShippedSkills would write to (files from the embedded ddx skill).
func enumerateSkillUpdateTargets(workingDir string) []string {
	var result []string
	_ = fs.WalkDir(skills.SkillFiles, "ddx", func(p string, d fs.DirEntry, _ error) error {
		if d == nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel("ddx", filepath.FromSlash(p))
		if err != nil {
			return nil
		}
		for _, base := range []string{
			filepath.Join(workingDir, ".agents", "skills", "ddx"),
			filepath.Join(workingDir, ".claude", "skills", "ddx"),
		} {
			result = append(result, filepath.Join(base, rel))
		}
		return nil
	})
	return result
}

// collectDirtyUpdateTargets returns absolute paths of files that would be
// overwritten by ddx update and currently have uncommitted git changes.
// Returns nil, nil when the working dir is not a git repository.
func collectDirtyUpdateTargets(workingDir string) ([]string, error) {
	if !gitpkg.IsRepository(workingDir) {
		return nil, nil
	}

	var candidates []string
	candidates = append(candidates, enumerateSkillUpdateTargets(workingDir)...)

	state, err := registry.LoadState()
	if err == nil {
		for _, entry := range state.Installed {
			for _, f := range entry.Files {
				expanded := registry.ExpandHome(f)
				if !filepath.IsAbs(expanded) {
					expanded = filepath.Join(workingDir, expanded)
				}
				candidates = append(candidates, expanded)
			}
		}
	}

	var dirty []string
	for _, c := range candidates {
		if isUpdateTargetDirty(workingDir, c) {
			dirty = append(dirty, c)
		}
	}
	return dirty, nil
}

// backupUpdateFile copies filePath to backupBase/<path-relative-to-workingDir>.
func backupUpdateFile(workingDir, filePath, backupBase string) error {
	abs := filePath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(workingDir, filePath)
	}
	rel, err := filepath.Rel(workingDir, abs)
	if err != nil {
		rel = filepath.Base(abs)
	}
	dst := filepath.Join(backupBase, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// performUpdate checks GitHub for the latest version of each installed plugin
// and updates any that are outdated (or all if --force). Always refreshes the
// embedded `ddx` skill and the AGENTS.md block so projects that ran `ddx init`
// under an older DDx version pick up current skill content without re-running
// init. Binary updates are intentionally explicit via `ddx upgrade`; `ddx
// update` must not replace a locally-built dogfood binary with the latest
// public release.
func performUpdate(workingDir string, opts *UpdateOptions) (*UpdateResult, error) {
	// Pre-check: detect dirty update targets before writing anything (atomic
	// refuse — no file is mutated if any target is dirty without --discard-local).
	dirtyFiles, _ := collectDirtyUpdateTargets(workingDir)
	if len(dirtyFiles) > 0 && !opts.DiscardLocal {
		var sb strings.Builder
		sb.WriteString("ddx update: uncommitted changes in files that would be overwritten:\n")
		for _, f := range dirtyFiles {
			sb.WriteString("  ")
			if rel, err := filepath.Rel(workingDir, f); err == nil {
				sb.WriteString(rel)
			} else {
				sb.WriteString(f)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\nCommit or stash these changes first, or use --force --discard-local to overwrite anyway.")
		return nil, fmt.Errorf("%s", sb.String())
	}

	// Backup dirty files before overwriting when --discard-local is set.
	var backupPath string
	if len(dirtyFiles) > 0 && opts.DiscardLocal {
		backupPath = ddxroot.JoinProject(workingDir, "update-backup",
			time.Now().UTC().Format("20060102T150405"))
		for _, f := range dirtyFiles {
			_ = backupUpdateFile(workingDir, f, backupPath)
		}
	}

	// Refresh the shipped `ddx` skill copy + AGENTS.md block first, regardless
	// of whether any plugins are installed. This is what lets older projects
	// pick up new SKILL.md / reference/*.md content without re-init.
	refreshShippedSkills(workingDir)

	state, err := registry.LoadState()
	if err != nil || len(state.Installed) == 0 {
		return &UpdateResult{Success: true, Message: "Shipped skills refreshed. No packages installed.", BackupPath: backupPath}, nil
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
		newEntry, err := registry.InstallPackage(&installPkg, workingDir)
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
		return &UpdateResult{Success: true, Message: "Shipped skills refreshed. All packages are up to date.", BackupPath: backupPath}, nil
	}

	return &UpdateResult{
		Success:      true,
		Message:      "Updated: " + strings.Join(updated, ", "),
		UpdatedFiles: updated,
		BackupPath:   backupPath,
	}, nil
}

// performGlobalUpdate checks for newer versions of globally installed plugins
// and reinstalls any that are outdated (or all if --force). It operates
// exclusively on the global plugin tree (${XDG_DATA_HOME}/ddx/global/) and
// never touches the project tree.
func performGlobalUpdate(opts *UpdateOptions) (*UpdateResult, error) {
	state, err := registry.LoadGlobalState()
	if err != nil || len(state.Installed) == 0 {
		return &UpdateResult{Success: true, Message: "No globally installed packages."}, nil
	}

	reg := registry.BuiltinRegistry()
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home dir: %w", err)
	}

	var updated []string
	for _, entry := range state.Installed {
		if opts.Resource != "" && entry.Name != opts.Resource {
			continue
		}
		pkg, err := reg.Find(entry.Name)
		if err != nil {
			continue
		}

		latestVersion := pkg.Version
		if release, err := update.FetchLatestReleaseForRepo(pkg.Source); err == nil {
			latestVersion = strings.TrimPrefix(release.TagName, "v")
		}

		if !opts.Force && entry.Version == latestVersion {
			continue
		}

		installPkg := *pkg
		installPkg.Version = latestVersion
		adjustedPkg := adjustInstallTargets(&installPkg, entry.Name,
			filepath.Join(home, ".agents", "skills"),
			filepath.Join(home, ".claude", "skills"))

		newEntry, err := registry.InstallPackage(adjustedPkg, ddxroot.GlobalDir())
		if err != nil {
			return nil, fmt.Errorf("updating global %s: %w", entry.Name, err)
		}
		state.AddOrUpdate(newEntry)
		updated = append(updated, entry.Name+" "+entry.Version+" → "+latestVersion)
	}

	if err := registry.SaveGlobalState(state); err != nil {
		return nil, fmt.Errorf("saving global state: %w", err)
	}

	if len(updated) == 0 {
		return &UpdateResult{Success: true, Message: "Global packages are up to date."}, nil
	}
	return &UpdateResult{
		Success:      true,
		Message:      "Updated globally: " + strings.Join(updated, ", "),
		UpdatedFiles: updated,
	}, nil
}

// refreshShippedSkills re-copies the embedded `ddx` skill into the project's
// skill directories and refreshes the AGENTS.md DDx block. Safe to call on
// every `ddx update` because skills.Install with Force=true handles the
// "existing files should be updated" case, and generateAgentsMD's
// marker-based merge is idempotent. Stale pre-consolidation skill dirs
// (ddx-bead, ddx-run, etc.) are swept by cleanupBootstrapSkills.
func refreshShippedSkills(workingDir string) {
	bootstrapSkillNames := []string{"ddx"}
	for _, dir := range []string{
		filepath.Join(workingDir, ".agents", "skills"),
		filepath.Join(workingDir, ".claude", "skills"),
	} {
		_ = os.MkdirAll(dir, 0755)
		cleanupBootstrapSkills(dir, bootstrapSkillNames)
	}
	if err := skills.Install(skills.SkillFiles, workingDir, skills.Options{Force: true}); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: skill install failed: %v\n", err)
	}
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
	opts.DiscardLocal, _ = cmd.Flags().GetBool("discard-local")
	opts.Global, _ = cmd.Flags().GetBool("global")

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
