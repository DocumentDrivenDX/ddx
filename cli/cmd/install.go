package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

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
	fmt.Fprintln(w, "NAME\tVERSION\tTYPE\tINSTALLED")
	for _, e := range state.Installed {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			e.Name,
			e.Version,
			string(e.Type),
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
