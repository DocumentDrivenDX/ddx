package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/DocumentDrivenDX/ddx/internal/update"
	"github.com/spf13/cobra"
)

// newInstallCommand creates the retired top-level "ddx install <name>" shim.
func (f *CommandFactory) newInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Deprecated: use 'ddx plugin install'",
		Long: `The top-level ddx install command has been retired.

Use 'ddx plugin install <name>' so plugin lifecycle operations stay under
the plugin noun. Registry installs write project lock metadata, resolve
payloads into the XDG cache, and generate agent adapter shims for the current
project.`,
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := "<name>"
			if len(args) > 0 {
				name = args[0]
			}
			return fmt.Errorf("ddx install has been retired; use 'ddx plugin install %s' instead", name)
		},
	}
	cmd.Flags().BoolP("force", "f", false, "Reinstall even if already at the latest version")
	cmd.Flags().String("local", "", "Install from a local directory instead of the registry")
	cmd.Flags().Bool("global", false, "deprecated compatibility flag")
	cmd.Flags().Bool("silent", false, "Suppress all output except errors")
	return cmd
}

// newPluginInstallCommand creates the canonical "ddx plugin install <name>" command.
func (f *CommandFactory) newPluginInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a plugin",
		Long: `Install a plugin from the DDx registry.

Registry installs behave like an npx-style dependency:
  - Project metadata: <ddx-root>/plugins.lock.yaml
  - Payload cache: ${XDG_DATA_HOME}/ddx/cache/plugins/<name>/<version>/
  - Generated adapters: <project>/.agents/skills/ and <project>/.claude/skills/

Generated adapters can be recreated with 'ddx plugin sync' and should not be
treated as source payload copies.

Examples:
  ddx plugin install ddx --force
  ddx plugin install helix
  ddx plugin install helix --force
  ddx plugin install helix --local ../helix --force`,
		Args: cobra.ExactArgs(1),
		RunE: f.runInstall,
	}
	cmd.Flags().BoolP("force", "f", false, "Reinstall even if already at the latest version")
	cmd.Flags().String("local", "", "Install from a local directory instead of the registry")
	cmd.Flags().Bool("global", false, "deprecated compatibility flag")
	_ = cmd.Flags().MarkHidden("global")
	cmd.Flags().Bool("silent", false, "Suppress all output except errors")
	return cmd
}

func (f *CommandFactory) runInstall(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	force, _ := cmd.Flags().GetBool("force")
	global, _ := cmd.Flags().GetBool("global")
	if global {
		cmd.SilenceUsage = true
		return errGlobalPluginInstallRetired()
	}
	if silent, _ := cmd.Flags().GetBool("silent"); silent {
		out = io.Discard
	}
	name := args[0]

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
		cmd.SilenceUsage = true
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

	if name == "ddx" {
		pkg, err := registry.BuiltinRegistry().Find("ddx")
		if err != nil {
			return err
		}
		files, err := syncBuiltinDDxSkillAdapters(f.WorkingDir, force)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Installed %s %s from built-in cache (%d adapter(s))\n", pkg.Name, pkg.Version, len(files))
		return nil
	}

	reg := f.pluginRegistry()
	pkg, err := reg.Find(name)
	if err != nil {
		return err
	}

	// Fetch actual latest version from GitHub (overrides hardcoded registry version).
	if release, err := update.FetchLatestReleaseForRepo(pkg.Source); err == nil {
		pkg.Version = strings.TrimPrefix(release.TagName, "v")
	}

	return f.installProjectRegistryPlugin(cmd.Context(), out, pkg, force)
}

func errGlobalPluginInstallRetired() error {
	return fmt.Errorf("global plugin installs are retired; use 'ddx plugin install <name>' per project, with payloads shared through ${XDG_DATA_HOME}/ddx/cache/plugins and adapters recreated by 'ddx plugin sync'")
}

func (f *CommandFactory) pluginRegistry() *registry.Registry {
	if f.registryOverride != nil {
		return f.registryOverride
	}
	return registry.BuiltinRegistry()
}

func (f *CommandFactory) installProjectRegistryPlugin(ctx context.Context, out io.Writer, pkg *registry.Package, force bool) error {
	lock, err := registry.LoadProjectPluginLock(ctx, f.WorkingDir)
	if err != nil {
		return err
	}
	if existing := lock.Find(pkg.Name); existing != nil {
		if existing.Version == pkg.Version {
			if !force {
				fmt.Fprintf(out, "%s %s is already up to date\n", existing.Name, existing.Version)
				return nil
			}
			fmt.Fprintf(out, "Reinstalling %s %s...\n", existing.Name, existing.Version)
		} else {
			fmt.Fprintf(out, "Updating %s from %s to %s...\n", existing.Name, existing.Version, pkg.Version)
		}
	}

	fmt.Fprintf(out, "Installing %s %s from %s...\n", pkg.Name, pkg.Version, pkg.Source)
	entry, err := registry.CachePackageFromRemote(pkg)
	if err != nil {
		return fmt.Errorf("install package: %w", err)
	}
	result, err := registry.SyncProjectPlugin(ctx, f.WorkingDir, entry, force)
	if err != nil {
		return fmt.Errorf("sync plugin shims: %w", err)
	}
	entry.GeneratedFiles = result.GeneratedFiles
	lock.AddOrUpdate(entry)
	if err := registry.SaveProjectPluginLock(ctx, f.WorkingDir, lock); err != nil {
		return fmt.Errorf("saving plugin lock: %w", err)
	}
	fmt.Fprintf(out, "Installed %s %s (cache: %s, shims: %d)\n", entry.Name, entry.Version, entry.CachePath, len(entry.GeneratedFiles))
	commitPluginChanges(f.WorkingDir, entry.Name, entry.Version)
	return nil
}

func (f *CommandFactory) newPluginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "plugin",
		Aliases: []string{"plugins"},
		Short:   "Manage DDx plugins",
	}
	cmd.AddCommand(f.newPluginInstallCommand())
	cmd.AddCommand(f.newPluginListCommand())
	cmd.AddCommand(f.newPluginShowCommand())
	cmd.AddCommand(f.newPluginSyncCommand())
	cmd.AddCommand(f.newPluginUpgradeCommand())
	cmd.AddCommand(f.newPluginUninstallCommand())
	return cmd
}

func (f *CommandFactory) newPluginListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		Args:  cobra.NoArgs,
		RunE:  f.runPluginList,
	}
	cmd.Flags().Bool("global", false, "deprecated compatibility flag")
	_ = cmd.Flags().MarkHidden("global")
	cmd.Flags().Bool("json", false, "Output project plugin state as JSON")
	return cmd
}

type projectPluginListEntry struct {
	Name           string    `json:"name"`
	Version        string    `json:"version"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	Source         string    `json:"source,omitempty"`
	CachePath      string    `json:"cache_path,omitempty"`
	Path           string    `json:"path,omitempty"`
	LocalOverlay   bool      `json:"local_overlay"`
	GeneratedFiles []string  `json:"generated_files,omitempty"`
	InstalledAt    time.Time `json:"installed_at,omitempty"`
}

func (f *CommandFactory) runPluginList(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	global, _ := cmd.Flags().GetBool("global")
	if global {
		cmd.SilenceUsage = true
		return errGlobalPluginInstallRetired()
	}
	jsonOut, _ := cmd.Flags().GetBool("json")

	entries, err := f.projectPluginListEntries(cmd.Context())
	if err != nil {
		return err
	}
	if jsonOut {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}
	if len(entries) == 0 {
		fmt.Fprintln(out, "No plugins installed.")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tTYPE\tSTATUS\tSOURCE")
	for _, e := range entries {
		source := e.Source
		if e.LocalOverlay {
			source = e.Path
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.Name, e.Version, e.Type, e.Status, source)
	}
	return w.Flush()
}

func (f *CommandFactory) projectPluginListEntries(ctx context.Context) ([]projectPluginListEntry, error) {
	lock, err := registry.LoadProjectPluginLock(ctx, f.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("loading plugin lock: %w", err)
	}
	entries := make([]projectPluginListEntry, 0, len(lock.Plugins)+1)
	seen := map[string]bool{}
	if entry, err := builtinDDxListEntry(ctx, f.WorkingDir); err != nil {
		return nil, err
	} else {
		entries = append(entries, entry)
		seen[entry.Name] = true
	}
	for _, e := range lock.Plugins {
		if seen[e.Name] {
			continue
		}
		status := projectPluginLockStatus(ctx, f.WorkingDir, e)
		cachePath := e.CachePath
		if cachePath == "" {
			cachePath = registry.PluginCacheDir(e.Name, e.Version)
		}
		localPath, local := projectPluginLocalOverlayPath(ctx, f.WorkingDir, e.Name)
		entry := projectPluginListEntry{
			Name:           e.Name,
			Version:        e.Version,
			Type:           string(e.Type),
			Status:         status,
			Source:         e.Source,
			CachePath:      cachePath,
			LocalOverlay:   local,
			GeneratedFiles: append([]string(nil), e.GeneratedFiles...),
			InstalledAt:    e.InstalledAt,
		}
		if local {
			entry.Path = localPath
		}
		entries = append(entries, entry)
		seen[e.Name] = true
	}

	overlayRoot := filepath.Join(ddxroot.Path(ctx, f.WorkingDir), "plugins")
	children, readErr := os.ReadDir(overlayRoot)
	if readErr != nil && !os.IsNotExist(readErr) {
		return nil, fmt.Errorf("reading local plugin overlays: %w", readErr)
	}
	for _, child := range children {
		name := child.Name()
		if seen[name] {
			continue
		}
		path := filepath.Join(overlayRoot, name)
		info, statErr := os.Lstat(path)
		if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, _ := os.Readlink(path)
		if target != "" && !filepath.IsAbs(target) {
			target = filepath.Clean(filepath.Join(filepath.Dir(path), target))
		}
		entries = append(entries, projectPluginListEntry{
			Name:         name,
			Version:      "local",
			Type:         string(registry.PackageTypePlugin),
			Status:       "local-overlay",
			Source:       target,
			Path:         target,
			LocalOverlay: true,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func (f *CommandFactory) newPluginShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show details about an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE:  f.runPluginShow,
	}
	cmd.Flags().Bool("global", false, "deprecated compatibility flag")
	_ = cmd.Flags().MarkHidden("global")
	return cmd
}

func (f *CommandFactory) runPluginShow(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	name := args[0]
	global, _ := cmd.Flags().GetBool("global")
	if global {
		cmd.SilenceUsage = true
		return errGlobalPluginInstallRetired()
	}

	lock, err := registry.LoadProjectPluginLock(cmd.Context(), f.WorkingDir)
	if err != nil {
		return fmt.Errorf("loading plugin lock: %w", err)
	}
	entry := lock.Find(name)
	if entry == nil {
		if name == "ddx" {
			return f.runBuiltinDDxPluginShow(cmd.Context(), out)
		}
		return fmt.Errorf("plugin %q is not installed", name)
	}
	result, err := registry.SyncProjectPlugin(cmd.Context(), f.WorkingDir, *entry, false)
	if err != nil {
		return err
	}
	if !sameStringSlices(entry.GeneratedFiles, result.GeneratedFiles) {
		entry.GeneratedFiles = result.GeneratedFiles
		if err := registry.SaveProjectPluginLock(cmd.Context(), f.WorkingDir, lock); err != nil {
			return fmt.Errorf("saving plugin lock: %w", err)
		}
	}
	fmt.Fprintf(out, "Name:         %s\n", entry.Name)
	fmt.Fprintf(out, "Version:      %s\n", entry.Version)
	fmt.Fprintf(out, "Type:         %s\n", string(entry.Type))
	fmt.Fprintf(out, "Source:       %s\n", entry.Source)
	fmt.Fprintf(out, "Cache:        %s\n", entry.CachePath)
	fmt.Fprintf(out, "Installed at: %s\n", entry.InstalledAt.Format("2006-01-02"))
	fmt.Fprintf(out, "Shims:        %d\n", len(entry.GeneratedFiles))
	return nil
}

func builtinDDxListEntry(ctx context.Context, projectRoot string) (projectPluginListEntry, error) {
	pkg, err := registry.BuiltinRegistry().Find("ddx")
	if err != nil {
		return projectPluginListEntry{}, err
	}
	cachePath := registry.PluginCacheDir(pkg.Name, pkg.Version)
	generatedFiles := []string{
		filepath.Join(".agents", "skills", "ddx"),
		filepath.Join(".claude", "skills", "ddx"),
	}
	entry := projectPluginListEntry{
		Name:           pkg.Name,
		Version:        pkg.Version,
		Type:           string(pkg.Type),
		Status:         projectBuiltinDDxStatus(projectRoot, cachePath, generatedFiles),
		Source:         pkg.Source,
		CachePath:      cachePath,
		GeneratedFiles: generatedFiles,
	}
	if localPath, local := projectPluginLocalOverlayPath(ctx, projectRoot, pkg.Name); local {
		entry.Version = "local"
		entry.Status = "local-overlay"
		entry.Source = localPath
		entry.Path = localPath
		entry.LocalOverlay = true
	}
	return entry, nil
}

func projectBuiltinDDxStatus(projectRoot, cachePath string, generatedFiles []string) string {
	if info, err := os.Stat(cachePath); err != nil || !info.IsDir() {
		return "cache-missing"
	}
	for _, generated := range generatedFiles {
		path := filepath.Join(projectRoot, filepath.FromSlash(generated))
		if _, err := os.Stat(path); err != nil {
			return "shims-missing"
		}
	}
	return "ok"
}

func (f *CommandFactory) runBuiltinDDxPluginShow(ctx context.Context, out io.Writer) error {
	pkg, err := registry.BuiltinRegistry().Find("ddx")
	if err != nil {
		return err
	}
	files, err := syncBuiltinDDxSkillAdapters(f.WorkingDir, false)
	if err != nil {
		return err
	}
	cachePath := registry.PluginCacheDir(pkg.Name, pkg.Version)
	fmt.Fprintf(out, "Name:         %s\n", pkg.Name)
	fmt.Fprintf(out, "Version:      %s\n", pkg.Version)
	fmt.Fprintf(out, "Type:         %s\n", string(pkg.Type))
	if localPath, local := projectPluginLocalOverlayPath(ctx, f.WorkingDir, pkg.Name); local {
		fmt.Fprintf(out, "Source:       %s\n", localPath)
		fmt.Fprintf(out, "Status:       local-overlay\n")
	} else {
		fmt.Fprintf(out, "Source:       %s\n", pkg.Source)
		fmt.Fprintf(out, "Status:       built-in\n")
	}
	fmt.Fprintf(out, "Cache:        %s\n", cachePath)
	fmt.Fprintf(out, "Installed at: built-in\n")
	fmt.Fprintf(out, "Shims:        %d\n", len(files))
	return nil
}

func sameStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (f *CommandFactory) newPluginSyncCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Recreate generated plugin shims from the project lock",
		Args:  cobra.NoArgs,
		RunE:  f.runPluginSync,
	}
	cmd.Flags().BoolP("force", "f", false, "Replace existing generated skill directories")
	return cmd
}

func (f *CommandFactory) runPluginSync(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	force, _ := cmd.Flags().GetBool("force")
	builtinFiles, err := syncBuiltinDDxSkillAdapters(f.WorkingDir, force)
	if err != nil {
		return err
	}
	results, err := registry.SyncProjectPlugins(cmd.Context(), f.WorkingDir, force)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "ddx builtin: ok (%d adapter(s))\n", len(builtinFiles))
	for _, result := range results {
		status := "ok"
		if result.LocalOverlay {
			status = "local-overlay"
		} else if !result.CachePresent {
			status = "cache-missing"
		}
		fmt.Fprintf(out, "%s %s: %s (%d shim(s))\n", result.Name, result.Version, status, len(result.GeneratedFiles))
	}
	return nil
}

func (f *CommandFactory) newPluginUpgradeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade [name]",
		Short: "Upgrade registry-installed plugins",
		Long: `Upgrade one or all registry-installed project plugins.

Local overlays are machine-local developer state and are skipped. Registry
plugins remain lock/cache based: the project lock is updated, payloads resolve
into the shared XDG cache, and generated adapters are recreated.`,
		Args: cobra.MaximumNArgs(1),
		RunE: f.runPluginUpgrade,
	}
	cmd.Flags().BoolP("force", "f", false, "Reinstall even when the locked version is current")
	cmd.Flags().Bool("silent", false, "Suppress all output except errors")
	return cmd
}

func (f *CommandFactory) runPluginUpgrade(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	if silent, _ := cmd.Flags().GetBool("silent"); silent {
		out = io.Discard
	}
	force, _ := cmd.Flags().GetBool("force")

	lock, err := registry.LoadProjectPluginLock(cmd.Context(), f.WorkingDir)
	if err != nil {
		return fmt.Errorf("loading plugin lock: %w", err)
	}
	var targets []registry.PluginLockEntry
	if len(args) == 1 {
		name := args[0]
		if projectPluginLocalOverlay(cmd.Context(), f.WorkingDir, name) {
			fmt.Fprintf(out, "%s is local-linked; skipped\n", name)
			return nil
		}
		entry := lock.Find(name)
		if entry == nil {
			return fmt.Errorf("plugin %q is not installed", name)
		}
		targets = append(targets, *entry)
	} else {
		targets = append(targets, lock.Plugins...)
		entries, err := f.projectPluginListEntries(cmd.Context())
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.LocalOverlay && lock.Find(entry.Name) == nil {
				fmt.Fprintf(out, "%s is local-linked; skipped\n", entry.Name)
			}
		}
	}
	if len(targets) == 0 {
		fmt.Fprintln(out, "No registry plugins installed.")
		return nil
	}

	reg := f.pluginRegistry()
	for _, locked := range targets {
		if projectPluginLocalOverlay(cmd.Context(), f.WorkingDir, locked.Name) {
			fmt.Fprintf(out, "%s is local-linked; skipped\n", locked.Name)
			continue
		}
		pkg, findErr := reg.Find(locked.Name)
		if findErr != nil {
			pkg = &registry.Package{
				Name:        locked.Name,
				Version:     locked.Version,
				Description: locked.Name + " plugin",
				Type:        locked.Type,
				Source:      locked.Source,
			}
		}
		if pkg.Source == "" {
			pkg.Source = locked.Source
		}
		if pkg.Version == "" {
			pkg.Version = locked.Version
		}
		if release, err := update.FetchLatestReleaseForRepo(pkg.Source); err == nil {
			pkg.Version = strings.TrimPrefix(release.TagName, "v")
		}
		if pkg.Version == locked.Version && !force {
			fmt.Fprintf(out, "%s %s is already up to date\n", locked.Name, locked.Version)
			continue
		}
		if err := f.installProjectRegistryPlugin(cmd.Context(), out, pkg, force); err != nil {
			return err
		}
	}
	return nil
}

func (f *CommandFactory) newPluginUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Remove a project plugin or local overlay",
		Long: `Remove a project plugin.

For registry-installed plugins, this removes generated adapter files and the
project lock entry. For local overlays, this removes only project symlinks and
leaves both the checkout target and any registry version pin untouched.`,
		Args: cobra.ExactArgs(1),
		RunE: f.runPluginUninstall,
	}
}

func (f *CommandFactory) runPluginUninstall(cmd *cobra.Command, args []string) error {
	name := args[0]
	out := cmd.OutOrStdout()

	if overlayPath, local := projectPluginLocalOverlayPath(cmd.Context(), f.WorkingDir, name); local {
		removed, err := removeLocalPluginOverlay(cmd.Context(), f.WorkingDir, name, overlayPath)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Uninstalled local overlay %s (%d symlink(s) removed)\n", name, removed)
		return nil
	}

	lock, err := registry.LoadProjectPluginLock(cmd.Context(), f.WorkingDir)
	if err != nil {
		return fmt.Errorf("loading plugin lock: %w", err)
	}
	entry := lock.Find(name)
	if entry == nil {
		return fmt.Errorf("plugin %q is not installed", name)
	}
	removed := 0
	for _, generated := range entry.GeneratedFiles {
		path := filepath.Join(f.WorkingDir, filepath.FromSlash(generated))
		if err := os.RemoveAll(path); err == nil {
			removed++
		}
	}
	lock.Remove(name)
	if err := registry.SaveProjectPluginLock(cmd.Context(), f.WorkingDir, lock); err != nil {
		return fmt.Errorf("saving plugin lock: %w", err)
	}
	commitPluginChanges(f.WorkingDir, entry.Name, entry.Version)
	fmt.Fprintf(out, "Uninstalled %s (%d generated adapter(s) removed)\n", name, removed)
	return nil
}

func projectPluginLockStatus(ctx context.Context, projectRoot string, entry registry.PluginLockEntry) string {
	if projectPluginLocalOverlay(ctx, projectRoot, entry.Name) {
		return "local-overlay"
	}
	cachePath := entry.CachePath
	if cachePath == "" {
		cachePath = registry.PluginCacheDir(entry.Name, entry.Version)
	}
	if info, err := os.Stat(cachePath); err != nil || !info.IsDir() {
		return "cache-missing"
	}
	for _, generated := range entry.GeneratedFiles {
		path := filepath.Join(projectRoot, filepath.FromSlash(generated))
		if _, err := os.Stat(path); err != nil {
			return "shims-missing"
		}
	}
	return "ok"
}

func projectPluginLocalOverlay(ctx context.Context, projectRoot, name string) bool {
	_, ok := projectPluginLocalOverlayPath(ctx, projectRoot, name)
	return ok
}

func projectPluginLocalOverlayPath(ctx context.Context, projectRoot, name string) (string, bool) {
	path := filepath.Join(ddxroot.Path(ctx, projectRoot), "plugins", name)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return "", false
	}
	target, err := os.Readlink(path)
	if err != nil {
		return path, true
	}
	if target != "" && !filepath.IsAbs(target) {
		target = filepath.Clean(filepath.Join(filepath.Dir(path), target))
	}
	return target, true
}

func removeLocalPluginOverlay(ctx context.Context, projectRoot, name, targetRoot string) (int, error) {
	removed := 0
	overlayPath := filepath.Join(ddxroot.Path(ctx, projectRoot), "plugins", name)
	if info, err := os.Lstat(overlayPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(overlayPath); err != nil {
			return removed, fmt.Errorf("removing local plugin overlay %s: %w", overlayPath, err)
		}
		removed++
	}

	if targetRoot == "" {
		return removed, nil
	}
	if abs, err := filepath.Abs(targetRoot); err == nil {
		targetRoot = abs
	}
	for _, surface := range []string{".agents/skills", ".claude/skills"} {
		dir := filepath.Join(projectRoot, filepath.FromSlash(surface))
		children, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return removed, fmt.Errorf("reading generated skills %s: %w", dir, err)
		}
		for _, child := range children {
			path := filepath.Join(dir, child.Name())
			info, err := os.Lstat(path)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			dest, err := filepath.EvalSymlinks(path)
			if err != nil {
				continue
			}
			if localPathHasPrefix(dest, targetRoot) {
				if err := os.Remove(path); err != nil {
					return removed, fmt.Errorf("removing local skill overlay %s: %w", path, err)
				}
				removed++
			}
		}
	}
	return removed, nil
}

func localPathHasPrefix(path, prefix string) bool {
	path = filepath.Clean(path)
	prefix = filepath.Clean(prefix)
	if path == prefix {
		return true
	}
	rel, err := filepath.Rel(prefix, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

func removeStaleFilesFromInstall(oldFiles []string, newFiles []string) int {
	if len(oldFiles) == 0 {
		return 0
	}

	newSet := make(map[string]bool, len(newFiles))
	for _, f := range newFiles {
		newSet[f] = true
	}

	removed := 0
	for _, f := range oldFiles {
		if newSet[f] {
			continue
		}
		if installPathContainsAny(f, newFiles) {
			continue
		}
		expanded := registry.ExpandHome(f)
		if err := os.RemoveAll(expanded); err == nil {
			removed++
		}
	}
	return removed
}

func installPathContainsAny(parent string, files []string) bool {
	parent = filepath.ToSlash(strings.TrimSuffix(parent, string(filepath.Separator)))
	parent = strings.TrimSuffix(parent, "/")
	if parent == "" {
		return false
	}
	prefix := parent + "/"
	for _, f := range files {
		f = filepath.ToSlash(f)
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

// commitPluginChanges stages and commits plugin-related changes in the working tree.
// Non-fatal: if git operations fail (not a repo, nothing to commit), it's silently skipped.
func commitPluginChanges(projectRoot, name, version string) {
	// Stage project plugin intent. Skill shims are generated adapter state.
	lockPath := ddxroot.JoinRelative(registry.ProjectPluginLockFile)
	ctx := context.Background()
	if projectRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			projectRoot = cwd
		}
	}
	if _, err := os.Stat(filepath.Join(projectRoot, lockPath)); err == nil {
		gitAdd := internalgit.Command(ctx, projectRoot, "add", lockPath)
		_ = gitAdd.Run()
	}

	// Check if there's anything to commit.
	status := internalgit.Command(ctx, projectRoot, "diff", "--cached", "--quiet", "--", lockPath)
	if status.Run() == nil {
		return // nothing staged
	}

	msg := fmt.Sprintf("chore: install %s %s", name, version)
	gitCommit := internalgit.Command(ctx, projectRoot, "commit", "-m", msg, "--only", "--", lockPath)
	_ = gitCommit.Run()
}

func prepareSymlinkTarget(target string, force bool) error {
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || force {
			if err := os.RemoveAll(target); err != nil {
				return fmt.Errorf("removing existing %s: %w", target, err)
			}
			return nil
		}
		return fmt.Errorf("%s already exists (use --force to replace)", target)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking existing %s: %w", target, err)
	}
	return nil
}

// installLocal installs a plugin from a local directory as a project developer
// overlay (in-tree mode: <project>/.ddx/plugins/<name>; convention mode: XDG
// project path).
// Local installs intentionally do not update the recorded installed state and
// do not auto-commit so the recorded plugin pin remains whatever the project
// already declares.
func (f *CommandFactory) installLocal(name, localPath string, force bool, out io.Writer) error {
	// Resolve to absolute path.
	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("resolving path %s: %w", localPath, err)
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("local path does not exist: %s", absPath)
	}

	// Load package.yaml when present so local installs honor the manifest
	// contract. If package.yaml is absent, use the plugin manifest rather than
	// assuming a repository layout from the package name.
	reg := registry.BuiltinRegistry()
	fallbackPkg, _ := reg.Find(name)

	pkg, manifestMissing, manifestIssues, manifestErr := registry.LoadPackageManifestWithFallback(absPath, fallbackPkg)
	if manifestMissing {
		pluginPkg, ok, pluginErr := loadLocalPluginManifestPackage(absPath, name)
		if pluginErr != nil {
			return pluginErr
		}
		if ok {
			pkg = pluginPkg
		} else {
			pkg = &registry.Package{
				Name:        name,
				Version:     "local",
				Description: "local plugin package",
				Type:        registry.PackageTypePlugin,
				Source:      absPath,
			}
		}
	}
	if manifestErr == nil {
		if strings.TrimSpace(pkg.Name) != "" && pkg.Name != name {
			return fmt.Errorf("local package name %q does not match package.yaml name %q", name, pkg.Name)
		}
	} else {
		if os.IsNotExist(manifestErr) && manifestMissing {
			// Already handled through package.yaml absence fallback above.
		} else if len(manifestIssues) > 0 {
			return fmt.Errorf("validating package manifest: %w", manifestErr)
		} else {
			return fmt.Errorf("loading package manifest: %w", manifestErr)
		}
	}

	if pkg == nil {
		pkg = &registry.Package{
			Name:        name,
			Version:     "local",
			Description: "local plugin package",
			Type:        registry.PackageTypePlugin,
			Source:      absPath,
		}
	}

	if pkg.Install.Root == nil {
		pkg.Install.Root = &registry.InstallMapping{
			Source: ".",
			Target: fmt.Sprintf(".ddx/plugins/%s", pkg.Name),
		}
	}

	if strings.HasPrefix(pkg.Install.Root.Target, "~") {
		return fmt.Errorf("FEAT-015: Root.Target must be project-relative; got %s in package %s; update the manifest to use a relative path", pkg.Install.Root.Target, pkg.Name)
	}

	projectRoot := f.WorkingDir
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}
	var oldFiles []string
	if state, err := registry.LoadState(); err == nil {
		if old := state.FindInstalled(pkg.Name); old != nil {
			oldFiles = append([]string{}, old.Files...)
			_, _ = markLocalInstallOverlay(projectRoot, pkg.Name, oldFiles)
		}
	}

	if issues := filterLocalInstallValidationIssues(absPath, registry.ValidatePackageStructure(absPath, pkg)); len(issues) > 0 {
		return fmt.Errorf("validating package structure: %s", registry.JoinValidationIssues(issues))
	}

	// Determine the project plugin directory.
	// In-tree: link into <project>/.ddx/plugins/<name>.
	// Project convention: link into the XDG convention root plugins/<name>.
	// The FEAT-015 check above already validates that pkg.Install.Root.Target
	// does not start with "~"; we now override the actual destination path.
	ddxRootDir := resolveBeadStoreRoot(f.WorkingDir)
	pluginDir := filepath.Join(ddxRootDir, "plugins", name)

	if err := prepareSymlinkTarget(pluginDir, force); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(pluginDir), 0755); err != nil {
		return fmt.Errorf("creating plugins dir: %w", err)
	}

	if err := os.Symlink(absPath, pluginDir); err != nil {
		return fmt.Errorf("linking plugin into %s: %w", pluginDir, err)
	}

	fmt.Fprintf(out, "Linked %s -> %s\n", pluginDir, absPath)

	entry := registry.InstalledEntry{
		Name:    pkg.Name,
		Version: pkg.Version,
		Type:    pkg.Type,
		Source:  absPath,
	}
	// Record entry.Files as project-relative paths when the plugin lives inside
	// the working directory (in-tree mode). Convention-root absolute paths are
	// recorded as-is. This keeps removeStaleFilesFromInstall compatible with
	// relative paths recorded by prior runs.
	fileToRecord := pluginDir
	if f.WorkingDir != "" {
		if rel, relErr := filepath.Rel(f.WorkingDir, pluginDir); relErr == nil && !strings.HasPrefix(rel, "..") {
			fileToRecord = rel
		}
	}
	entry.Files = append(entry.Files, fileToRecord)

	// Install local skills as project-local symlinks into each supported
	// harness surface.
	if len(pkg.Install.Skills) > 0 {
		absProject := f.WorkingDir
		if absProject == "" {
			absProject, _ = os.Getwd()
		}
		if !filepath.IsAbs(absProject) {
			absProject, _ = filepath.Abs(absProject)
		}
		writtenSkills, err := installLocalSkillSymlinks(absPath, absProject, pkg.Install.Skills, force)
		if err != nil {
			return fmt.Errorf("installing skills: %w", err)
		}
		entry.Files = append(entry.Files, writtenSkills...)
	}

	// Copy CLI script if defined (skip if target is a developer symlink).
	if pkg.Install.Scripts != nil {
		dst := registry.ExpandHome(pkg.Install.Scripts.Target)
		if li, lErr := os.Lstat(dst); lErr == nil && li.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(dst)
			fmt.Fprintf(out, "notice: %s is a symlink → %s (developer mode, skipping copy)\n", dst, target)
			entry.Files = append(entry.Files, dst)
		} else {
			copied, err := registry.CopyScriptFromRoot(pluginDir, pkg.Install.Scripts)
			if err != nil {
				fmt.Fprintf(out, "Warning: script copy error: %v\n", err)
			} else {
				entry.Files = append(entry.Files, copied)
			}
		}
	}

	// Set execute bits on installed plugin files.
	for _, rel := range pkg.Install.Executable {
		p := filepath.Join(pluginDir, filepath.FromSlash(rel))
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			_ = os.Chmod(p, info.Mode()|0111)
		}
	}

	entry.InstalledAt = time.Now()

	removed := removeStaleFilesFromInstall(oldFiles, entry.Files)
	if removed > 0 {
		fmt.Fprintf(out, "Removed %d stale file(s) from previous install\n", removed)
	}

	if hidden, err := markLocalInstallOverlay(projectRoot, entry.Name, entry.Files); err != nil {
		fmt.Fprintf(out, "Warning: local install is active but git overlay hiding failed: %v\n", err)
	} else if hidden > 0 {
		fmt.Fprintf(out, "Marked %d tracked local overlay file(s) skip-worktree\n", hidden)
	}

	fmt.Fprintf(out, "Installed %s from local path %s — %d file(s); recorded plugin pin unchanged\n", entry.Name, absPath, len(entry.Files))
	return nil
}

func markLocalInstallOverlay(projectRoot, pluginName string, files []string) (int, error) {
	if projectRoot == "" {
		return 0, nil
	}

	ctx := context.Background()
	topOut, err := internalgit.Command(ctx, projectRoot, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return 0, nil
	}
	repoRoot := strings.TrimSpace(string(topOut))
	if repoRoot == "" {
		return 0, nil
	}

	paths := localOverlayPaths(pluginName, files)
	if len(paths) == 0 {
		return 0, nil
	}
	if err := addLocalOverlayIgnores(repoRoot, paths); err != nil {
		return 0, err
	}

	args := append([]string{"ls-files", "-z", "--"}, paths...)
	trackedOut, err := internalgit.Command(ctx, repoRoot, args...).Output()
	if err != nil {
		return 0, err
	}
	var tracked []string
	for _, p := range strings.Split(string(trackedOut), "\x00") {
		if p != "" {
			tracked = append(tracked, p)
		}
	}
	for start := 0; start < len(tracked); start += 100 {
		end := start + 100
		if end > len(tracked) {
			end = len(tracked)
		}
		args := append([]string{"update-index", "--skip-worktree", "--"}, tracked[start:end]...)
		if err := internalgit.Command(ctx, repoRoot, args...).Run(); err != nil {
			return 0, err
		}
	}
	return len(tracked), nil
}

func localOverlayPaths(pluginName string, files []string) []string {
	seen := map[string]bool{}
	add := func(path string) {
		path = filepath.ToSlash(strings.TrimSpace(path))
		path = strings.TrimPrefix(path, "./")
		if path == "" || strings.HasPrefix(path, "~") || filepath.IsAbs(path) {
			return
		}
		if !seen[path] {
			seen[path] = true
		}
	}

	add(filepath.ToSlash(ddxroot.JoinRelative("plugins", pluginName)))
	for _, file := range files {
		p := filepath.ToSlash(file)
		for _, prefix := range []string{".agents/skills/", ".claude/skills/"} {
			if strings.HasPrefix(p, prefix) {
				rest := strings.TrimPrefix(p, prefix)
				name := strings.Split(rest, "/")[0]
				if name != "" {
					add(prefix + name)
				}
			}
		}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

func installLocalSkillSymlinks(pluginRoot, projectRoot string, mappings []registry.InstallMapping, force bool) ([]string, error) {
	sourceRoots, err := localSkillSourceRoots(pluginRoot, mappings)
	if err != nil {
		return nil, err
	}
	if len(sourceRoots) == 0 {
		return nil, nil
	}

	var written []string
	seenSkills := map[string]bool{}
	for _, sourceRoot := range sourceRoots {
		entries, err := os.ReadDir(sourceRoot)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			name := entry.Name()
			if seenSkills[name] {
				continue
			}
			src := filepath.Join(sourceRoot, name)
			if _, err := os.Stat(filepath.Join(src, "SKILL.md")); err != nil {
				return nil, fmt.Errorf("%s: missing SKILL.md: %w", src, err)
			}
			for _, surface := range []string{".agents/skills", ".claude/skills"} {
				dst := filepath.Join(projectRoot, filepath.FromSlash(surface), name)
				if err := prepareSymlinkTarget(dst, force); err != nil {
					return nil, err
				}
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
					return nil, fmt.Errorf("creating skills dir: %w", err)
				}
				target, err := filepath.Rel(filepath.Dir(dst), src)
				if err != nil {
					target = src
				}
				if err := os.Symlink(target, dst); err != nil {
					return nil, fmt.Errorf("linking skill %s -> %s: %w", dst, target, err)
				}
				written = append(written, filepath.Join(filepath.FromSlash(surface), name))
			}
			seenSkills[name] = true
		}
	}
	return written, nil
}

func localSkillSourceRoots(pluginRoot string, mappings []registry.InstallMapping) ([]string, error) {
	var roots []string
	seen := map[string]bool{}
	for _, mapping := range mappings {
		source := strings.TrimSpace(mapping.Source)
		if source == "" {
			continue
		}
		candidate := filepath.Clean(filepath.Join(pluginRoot, filepath.FromSlash(source)))
		info, err := os.Stat(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s: skill source is not a directory", candidate)
		}
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		roots = append(roots, candidate)
	}
	return roots, nil
}

type localPluginJSONManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Repository  string `json:"repository"`
	Skills      any    `json:"skills"`
}

func loadLocalPluginManifestPackage(root, expectedName string) (*registry.Package, bool, error) {
	for _, rel := range []string{
		filepath.Join(".codex-plugin", "plugin.json"),
		filepath.Join(".claude-plugin", "plugin.json"),
	} {
		path := filepath.Join(root, rel)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, false, fmt.Errorf("loading plugin manifest %s: %w", path, err)
		}
		var manifest localPluginJSONManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, false, fmt.Errorf("loading plugin manifest %s: invalid JSON: %w", path, err)
		}
		name := strings.TrimSpace(manifest.Name)
		if name == "" {
			name = expectedName
		}
		if name != expectedName {
			return nil, false, fmt.Errorf("local package name %q does not match plugin manifest name %q", expectedName, name)
		}
		sources, err := pluginManifestSkillSources(manifest.Skills)
		if err != nil {
			return nil, false, fmt.Errorf("loading plugin manifest %s: %w", path, err)
		}
		pkg := &registry.Package{
			Name:        name,
			Version:     defaultString(strings.TrimSpace(manifest.Version), "local"),
			Description: defaultString(strings.TrimSpace(manifest.Description), "local plugin package"),
			Type:        registry.PackageTypePlugin,
			Source:      defaultString(strings.TrimSpace(manifest.Repository), root),
			Install: registry.PackageInstall{
				Root: &registry.InstallMapping{
					Source: ".",
					Target: fmt.Sprintf(".ddx/plugins/%s", name),
				},
			},
		}
		for _, source := range sources {
			pkg.Install.Skills = append(pkg.Install.Skills, registry.InstallMapping{
				Source: source,
				Target: ".agents/skills/",
			})
		}
		return pkg, true, nil
	}
	return nil, false, nil
}

func pluginManifestSkillSources(raw any) ([]string, error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case string:
		source := cleanPluginManifestPath(v)
		if source == "" {
			return nil, nil
		}
		return []string{source}, nil
	case []any:
		var sources []string
		for _, item := range v {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("skills entries must be strings")
			}
			source := cleanPluginManifestPath(text)
			if source != "" {
				sources = append(sources, source)
			}
		}
		return sources, nil
	default:
		return nil, fmt.Errorf("skills must be a string or array of strings")
	}
}

func cleanPluginManifestPath(raw string) string {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "./")
	if cleaned == "" || cleaned == "." {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(cleaned)))
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func addLocalOverlayIgnores(repoRoot string, paths []string) error {
	excludePath := filepath.Join(repoRoot, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}
	existing, _ := os.ReadFile(excludePath)
	text := string(existing)

	var additions []string
	for _, p := range paths {
		pattern := filepath.ToSlash(p)
		candidates := []string{pattern}
		if !strings.HasSuffix(pattern, "/") {
			candidates = append(candidates, pattern+"/")
		}
		for _, candidate := range candidates {
			if !strings.Contains(text, "\n"+candidate+"\n") && !strings.HasPrefix(text, candidate+"\n") {
				additions = append(additions, candidate)
			}
		}
	}
	if len(additions) == 0 {
		return nil
	}

	var sb strings.Builder
	if len(existing) > 0 && !strings.HasSuffix(text, "\n") {
		sb.WriteString("\n")
	}
	if !strings.Contains(text, "# DDx local plugin overlays") {
		sb.WriteString("\n# DDx local plugin overlays\n")
	}
	for _, pattern := range additions {
		sb.WriteString(pattern)
		sb.WriteString("\n")
	}
	return os.WriteFile(excludePath, append(existing, []byte(sb.String())...), 0o644)
}

func filterLocalInstallValidationIssues(root string, issues []registry.ValidationIssue) []registry.ValidationIssue {
	filtered := issues[:0]
	roots := localInstallRootAliases(root)
	ignoreCache := map[string]bool{}
	for _, issue := range issues {
		if rel, ok := localInstallIssueRel(roots, issue.Path); ok &&
			(shouldSkipLocalInstallIssue(rel) || localInstallPathIsGitIgnored(root, rel, ignoreCache)) {
			continue
		}
		filtered = append(filtered, issue)
	}
	return filtered
}

// localInstallPathIsGitIgnored reports whether rel (a slash path relative to
// the local install source root) is gitignored in the source repository.
// ddx install --local must not fail validation on broken symlinks that live
// under gitignored scratch/tmp paths (e.g. doctor/home/.codex/tmp/...) the
// operator never intended to ship. Returns false when the source is not a git
// repo or git is unavailable, leaving the well-known-prefix skip list as the
// fallback.
func localInstallPathIsGitIgnored(root, rel string, cache map[string]bool) bool {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return false
	}
	if cached, ok := cache[rel]; ok {
		return cached
	}
	// git check-ignore exits 0 when the path is ignored, 1 when it is not, and
	// 128 when the source is not a git repository — treat anything but 0 as
	// "not ignored" so non-git sources fall through to the prefix skip list.
	cmd := internalgit.Command(context.Background(), root, "check-ignore", "-q", "--", filepath.FromSlash(rel))
	ignored := cmd.Run() == nil
	cache[rel] = ignored
	return ignored
}

func localInstallRootAliases(root string) []string {
	seen := map[string]bool{}
	var roots []string
	add := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		if !seen[abs] {
			seen[abs] = true
			roots = append(roots, abs)
		}
		if resolved, err := filepath.EvalSymlinks(abs); err == nil && resolved != "" && !seen[resolved] {
			seen[resolved] = true
			roots = append(roots, resolved)
		}
	}
	add(root)
	return roots
}

func localInstallIssueRel(roots []string, path string) (string, bool) {
	for _, candidate := range localInstallPathAliases(path) {
		for _, root := range roots {
			rel, err := filepath.Rel(root, candidate)
			if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
				continue
			}
			return filepath.ToSlash(rel), true
		}
	}
	return "", false
}

func localInstallPathAliases(path string) []string {
	seen := map[string]bool{}
	var aliases []string
	add := func(candidate string) {
		if strings.TrimSpace(candidate) == "" {
			return
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			abs = candidate
		}
		if !seen[abs] {
			seen[abs] = true
			aliases = append(aliases, abs)
		}
	}
	add(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != "" {
		add(resolved)
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if resolvedDir, err := filepath.EvalSymlinks(dir); err == nil && resolvedDir != "" {
		add(filepath.Join(resolvedDir, base))
	}
	return aliases
}

func shouldSkipLocalInstallIssue(rel string) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	for _, prefix := range []string{
		".git/",
		".ddx/plugins/",
		".ddx/agent-logs/",
		".ddx/executions/",
		".ddx/exec-runs.d/",
		".ddx/server/",
		".ddx/tsnet/",
		".ddx/workers/",
		"node_modules/",
		"tmp/",
		"website/node_modules/",
		"website/public/",
		"website/resources/",
		"website/test-results/",
		"website/playwright-report/",
	} {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

// newInstalledCommand creates the "ddx installed" command.
func (f *CommandFactory) newInstalledCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "installed",
		Short:  "Deprecated: use 'ddx plugin list'",
		Long:   `The top-level ddx installed command has been retired. Use 'ddx plugin list' instead.`,
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("ddx installed has been retired; use 'ddx plugin list' instead")
		},
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
		Use:    "uninstall <name>",
		Short:  "Deprecated: plugin lifecycle moved under 'ddx plugin'",
		Long:   `The top-level ddx uninstall command has been retired. Use 'ddx plugin list' and 'ddx plugin sync' for project plugin state.`,
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("ddx uninstall has been retired; plugin lifecycle is managed under 'ddx plugin'")
		},
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
		Use:    "verify",
		Short:  "Deprecated: use 'ddx doctor --plugins'",
		Long:   `The top-level ddx verify command has been retired. Use 'ddx doctor --plugins' and 'ddx plugin sync' instead.`,
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("ddx verify has been retired; use 'ddx doctor --plugins' or 'ddx plugin sync' instead")
		},
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
		return fmt.Errorf("%d integrity issue(s) found — run 'ddx plugin install <name> --force' to repair", totalIssues)
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
		Use:    "outdated",
		Short:  "Deprecated: plugin lifecycle moved under 'ddx plugin'",
		Long:   `The top-level ddx outdated command has been retired. Use 'ddx plugin list' and 'ddx doctor --plugins' instead.`,
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("ddx outdated has been retired; use 'ddx plugin list' and 'ddx doctor --plugins' instead")
		},
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
