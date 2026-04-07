package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/DocumentDrivenDX/ddx/internal/update"
	"github.com/spf13/cobra"
)

// newInstallCommand creates the "ddx install <name>" command.
func (f *CommandFactory) newInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a package or resource",
		Long: `Install a package or resource from the DDx registry.

Examples:
  ddx install helix                        # Install HELIX workflow
  ddx install helix --force                # Reinstall even if already up to date
  ddx install persona/strict-code-reviewer # Install a single persona`,
		Args: cobra.ExactArgs(1),
		RunE: f.runInstall,
	}
	cmd.Flags().BoolP("force", "f", false, "Reinstall even if already at the latest version")
	cmd.Flags().String("local", "", "Install from a local directory instead of the registry")
	return cmd
}

func (f *CommandFactory) runInstall(cmd *cobra.Command, args []string) error {
	name := args[0]
	out := cmd.OutOrStdout()
	force, _ := cmd.Flags().GetBool("force")

	// Ensure install operations resolve relative paths against the project
	// root (WorkingDir), not the caller's cwd. This prevents creating stale
	// .ddx/ workspaces in subdirectories.
	if f.WorkingDir != "" {
		origDir, _ := os.Getwd()
		if origDir != f.WorkingDir {
			if err := os.Chdir(f.WorkingDir); err == nil {
				defer func() { _ = os.Chdir(origDir) }()
			}
		}
	}

	// Handle --local: install from a local directory instead of registry.
	if localPath, _ := cmd.Flags().GetString("local"); localPath != "" {
		return f.installLocal(name, localPath, force, out)
	}

	if registry.IsResourcePath(name) {
		// Individual resource install (e.g. "persona/strict-code-reviewer")
		fmt.Fprintf(out, "Installing resource %s...\n", name)
		entry, err := registry.InstallResource(name)
		if err != nil {
			return fmt.Errorf("install resource: %w", err)
		}

		state, err := registry.LoadState()
		if err != nil {
			return fmt.Errorf("loading state: %w", err)
		}
		state.AddOrUpdate(entry)
		if err := registry.SaveState(state); err != nil {
			return fmt.Errorf("saving state: %w", err)
		}

		fmt.Fprintf(out, "Installed %s\n", name)
		return nil
	}

	// Package install (e.g. "helix")
	reg := registry.BuiltinRegistry()
	pkg, err := reg.Find(name)
	if err != nil {
		return err
	}

	// Fetch actual latest version from GitHub (overrides hardcoded registry version).
	if release, err := update.FetchLatestReleaseForRepo(pkg.Source); err == nil {
		pkg.Version = strings.TrimPrefix(release.TagName, "v")
	}

	// Check if already installed at the latest version.
	state, err := registry.LoadState()
	if err == nil {
		for _, e := range state.Installed {
			if e.Name == name {
				if e.Version == pkg.Version {
					if !force {
						fmt.Fprintf(out, "%s %s is already up to date\n", e.Name, e.Version)
						return nil
					}
					fmt.Fprintf(out, "Reinstalling %s %s...\n", e.Name, e.Version)
				} else {
					fmt.Fprintf(out, "Updating %s from %s to %s...\n", e.Name, e.Version, pkg.Version)
				}
			}
		}
	}

	// Capture old file list before installing so we can remove stale files.
	var oldFiles map[string]bool
	if state != nil {
		if old := state.FindInstalled(name); old != nil {
			oldFiles = make(map[string]bool, len(old.Files))
			for _, f := range old.Files {
				oldFiles[f] = true
			}
		}
	}

	fmt.Fprintf(out, "Installing %s %s from %s...\n", pkg.Name, pkg.Version, pkg.Source)

	entry, err := registry.InstallPackage(pkg)
	if err != nil {
		return fmt.Errorf("install package: %w", err)
	}

	// Remove files from the old version that are no longer in the new version.
	if oldFiles != nil {
		newFiles := make(map[string]bool, len(entry.Files))
		for _, f := range entry.Files {
			newFiles[f] = true
		}
		var removed int
		for f := range oldFiles {
			if !newFiles[f] {
				expanded := registry.ExpandHome(f)
				if err := os.Remove(expanded); err == nil {
					removed++
				}
			}
		}
		if removed > 0 {
			fmt.Fprintf(out, "Removed %d stale file(s)\n", removed)
		}
	}

	state, stateErr := registry.LoadState()
	if stateErr != nil {
		return fmt.Errorf("loading state: %w", stateErr)
	}
	state.AddOrUpdate(entry)
	if err := registry.SaveState(state); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	fmt.Fprintf(out, "Installed %s %s (%d file(s))\n", pkg.Name, pkg.Version, len(entry.Files))

	// Auto-commit skill symlinks and other trackable changes.
	commitPluginChanges(name, pkg.Version)

	return nil
}

// commitPluginChanges stages and commits plugin-related changes in the working tree.
// Non-fatal: if git operations fail (not a repo, nothing to commit), it's silently skipped.
func commitPluginChanges(name, version string) {
	// Stage skill symlinks and any other trackable plugin artifacts.
	paths := []string{".agents/skills/", ".claude/skills/"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			gitAdd := exec.Command("git", "add", p)
			_ = gitAdd.Run()
		}
	}

	// Check if there's anything to commit.
	status := exec.Command("git", "diff", "--cached", "--quiet")
	if status.Run() == nil {
		return // nothing staged
	}

	msg := fmt.Sprintf("chore: install %s %s", name, version)
	gitCommit := exec.Command("git", "commit", "-m", msg)
	_ = gitCommit.Run()
}

// installLocal installs a plugin from a local directory. It creates a symlink
// from .ddx/plugins/<name> to the local path, then discovers and symlinks
// skills the same way a registry install does.
func (f *CommandFactory) installLocal(name, localPath string, force bool, out io.Writer) error {
	// Resolve to absolute path.
	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("resolving path %s: %w", localPath, err)
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("local path does not exist: %s", absPath)
	}

	// Create .ddx/plugins/ directory.
	pluginDir := filepath.Join(".ddx", "plugins", name)
	if err := os.MkdirAll(filepath.Dir(pluginDir), 0755); err != nil {
		return fmt.Errorf("creating plugins dir: %w", err)
	}

	// Remove existing plugin (symlink or directory).
	if info, err := os.Lstat(pluginDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || force {
			if err := os.RemoveAll(pluginDir); err != nil {
				return fmt.Errorf("removing existing %s: %w", pluginDir, err)
			}
		} else {
			if !force {
				return fmt.Errorf("%s already exists (use --force to replace)", pluginDir)
			}
		}
	}

	// Create symlink to local path.
	if err := os.Symlink(absPath, pluginDir); err != nil {
		return fmt.Errorf("creating symlink %s -> %s: %w", pluginDir, absPath, err)
	}

	fmt.Fprintf(out, "Linked %s -> %s\n", pluginDir, absPath)

	// Look up the package in registry for skill mappings, or use defaults.
	reg := registry.BuiltinRegistry()
	pkg, _ := reg.Find(name)

	entry := registry.InstalledEntry{
		Name:    name,
		Version: "local",
		Type:    registry.PackageTypeWorkflow,
		Source:  absPath,
	}
	entry.Files = append(entry.Files, pluginDir)

	// Discover and symlink skills using the same logic as registry install.
	if pkg != nil {
		for i := range pkg.Install.Skills {
			skill := &pkg.Install.Skills[i]
			files, err := registry.SymlinkSkillsFromRoot(absPath, skill)
			if err != nil {
				fmt.Fprintf(out, "Warning: skill symlink error: %v\n", err)
				continue
			}
			entry.Files = append(entry.Files, files...)
		}

		// Copy CLI script if defined (skip if target is a developer symlink).
		if pkg.Install.Scripts != nil {
			dst := registry.ExpandHome(pkg.Install.Scripts.Target)
			if li, lErr := os.Lstat(dst); lErr == nil && li.Mode()&os.ModeSymlink != 0 {
				target, _ := os.Readlink(dst)
				fmt.Fprintf(out, "notice: %s is a symlink → %s (developer mode, skipping copy)\n", dst, target)
				entry.Files = append(entry.Files, dst)
			} else {
				copied, err := registry.CopyScriptFromRoot(absPath, pkg.Install.Scripts)
				if err != nil {
					fmt.Fprintf(out, "Warning: script copy error: %v\n", err)
				} else {
					entry.Files = append(entry.Files, copied)
				}
			}
		}

		// Set execute bits.
		for _, rel := range pkg.Install.Executable {
			p := filepath.Join(absPath, filepath.FromSlash(rel))
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				_ = os.Chmod(p, info.Mode()|0111)
			}
		}
	}

	// Save install state.
	state, _ := registry.LoadState()
	if state == nil {
		state = &registry.InstalledState{}
	}
	entry.InstalledAt = time.Now()
	state.AddOrUpdate(entry)
	if err := registry.SaveState(state); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	fmt.Fprintf(out, "Installed %s (local) — %d file(s)\n", name, len(entry.Files))
	commitPluginChanges(name, "local")
	return nil
}

// newInstalledCommand creates the "ddx installed" command.
func (f *CommandFactory) newInstalledCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "installed",
		Short: "List installed packages",
		Long:  `List all packages and resources installed via ddx install.`,
		Args:  cobra.NoArgs,
		RunE:  f.runInstalled,
	}
}

func (f *CommandFactory) runInstalled(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	state, err := registry.LoadState()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	if len(state.Installed) == 0 {
		fmt.Fprintln(out, "No packages installed.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tTYPE\tSTATUS\tINSTALLED")
	for _, e := range state.Installed {
		status := "ok"
		if !e.VerifyFiles() {
			status = "BROKEN"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			e.Name,
			e.Version,
			string(e.Type),
			status,
			e.InstalledAt.Format("2006-01-02"),
		)
	}
	return w.Flush()
}

// newUninstallCommand creates the "ddx uninstall <name>" command.
func (f *CommandFactory) newUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Remove an installed package",
		Long:  `Remove a package or resource installed via ddx install.`,
		Args:  cobra.ExactArgs(1),
		RunE:  f.runUninstall,
	}
}

func (f *CommandFactory) runUninstall(cmd *cobra.Command, args []string) error {
	name := args[0]
	out := cmd.OutOrStdout()

	state, err := registry.LoadState()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	entry := state.FindInstalled(name)
	if entry == nil {
		return fmt.Errorf("package %q is not installed", name)
	}

	if err := registry.UninstallPackage(entry); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}

	state.Remove(name)
	if err := registry.SaveState(state); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	fmt.Fprintf(out, "Uninstalled %s\n", name)
	return nil
}

// newVerifyCommand creates the "ddx verify" command.
func (f *CommandFactory) newVerifyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Verify installed package integrity",
		Long:  `Check that all files recorded for installed packages still exist and symlinks resolve correctly.`,
		Args:  cobra.NoArgs,
		RunE:  f.runVerify,
	}
}

func (f *CommandFactory) runVerify(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	state, err := registry.LoadState()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	if len(state.Installed) == 0 {
		fmt.Fprintln(out, "No packages installed.")
		return nil
	}

	var totalIssues int
	for _, entry := range state.Installed {
		var issues []string
		for _, f := range entry.Files {
			expanded := registry.ExpandHome(f)
			info, err := os.Lstat(expanded)
			if os.IsNotExist(err) {
				issues = append(issues, fmt.Sprintf("  missing: %s", f))
				continue
			}
			if err != nil {
				issues = append(issues, fmt.Sprintf("  error: %s: %v", f, err))
				continue
			}
			// Check symlinks resolve.
			if info.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(expanded)
				if err != nil {
					issues = append(issues, fmt.Sprintf("  broken symlink: %s", f))
					continue
				}
				// Resolve relative symlinks against their parent directory.
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(expanded), target)
				}
				if _, err := os.Stat(target); os.IsNotExist(err) {
					issues = append(issues, fmt.Sprintf("  broken symlink: %s -> %s", f, target))
				}
			}
		}

		if len(issues) == 0 {
			fmt.Fprintf(out, "%s %s: OK (%d files)\n", entry.Name, entry.Version, len(entry.Files))
		} else {
			fmt.Fprintf(out, "%s %s: %d issue(s)\n", entry.Name, entry.Version, len(issues))
			for _, issue := range issues {
				fmt.Fprintln(out, issue)
			}
			totalIssues += len(issues)
		}
	}

	if totalIssues > 0 {
		return fmt.Errorf("%d integrity issue(s) found — run 'ddx install <name> --force' to repair", totalIssues)
	}
	return nil
}

// newSearchCommand creates the "ddx search <query>" command.
func (f *CommandFactory) newSearchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search available packages",
		Long:  `Search the DDx registry for packages matching the given query.`,
		Args:  cobra.ExactArgs(1),
		RunE:  f.runSearch,
	}
}

func (f *CommandFactory) runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	out := cmd.OutOrStdout()

	reg := registry.BuiltinRegistry()
	results := reg.Search(query)

	if len(results) == 0 {
		fmt.Fprintf(out, "No packages found matching %q\n", query)
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tTYPE\tDESCRIPTION")
	for _, pkg := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			pkg.Name,
			pkg.Version,
			string(pkg.Type),
			pkg.Description,
		)
	}
	return w.Flush()
}

func (f *CommandFactory) newOutdatedCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "outdated",
		Short: "List installed packages with available updates",
		Long:  `Check installed packages against the registry and list those with newer versions available.`,
		Args:  cobra.NoArgs,
		RunE:  f.runOutdated,
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func (f *CommandFactory) runOutdated(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	asJSON, _ := cmd.Flags().GetBool("json")

	state, err := registry.LoadState()
	if err != nil {
		return fmt.Errorf("reading installed state: %w", err)
	}
	if len(state.Installed) == 0 {
		fmt.Fprintln(out, "No packages installed.")
		return nil
	}

	reg := registry.BuiltinRegistry()

	type outdatedEntry struct {
		Name             string `json:"name"`
		InstalledVersion string `json:"installed_version"`
		LatestVersion    string `json:"latest_version"`
	}

	var outdated []outdatedEntry
	for _, entry := range state.Installed {
		pkg, err := reg.Find(entry.Name)
		if err != nil {
			continue // not in registry, skip
		}
		if pkg.Version != entry.Version {
			outdated = append(outdated, outdatedEntry{
				Name:             entry.Name,
				InstalledVersion: entry.Version,
				LatestVersion:    pkg.Version,
			})
		}
	}

	if len(outdated) == 0 {
		fmt.Fprintln(out, "All installed packages are up to date.")
		return nil
	}

	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(outdated)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tINSTALLED\tLATEST")
	for _, o := range outdated {
		fmt.Fprintf(w, "%s\t%s\t%s\n", o.Name, o.InstalledVersion, o.LatestVersion)
	}
	return w.Flush()
}
