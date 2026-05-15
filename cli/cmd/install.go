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

// newInstallCommand creates the "ddx install <name>" command.
func (f *CommandFactory) newInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a package or resource",
		Long: `Install a package or resource from the DDx registry into the
current project under .ddx/plugins/<name>/.

Examples:
  ddx install helix                        # Install HELIX workflow
  ddx install helix --force                # Reinstall even if already up to date
  ddx plugin install helix --local ../helix --force
  ddx install persona/strict-code-reviewer # Install a single persona`,
		Args: cobra.ExactArgs(1),
		RunE: f.runInstall,
	}
	cmd.Flags().BoolP("force", "f", false, "Reinstall even if already at the latest version")
	cmd.Flags().String("local", "", "Install from a local directory instead of the registry")
	return cmd
}

func (f *CommandFactory) runInstall(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	force, _ := cmd.Flags().GetBool("force")
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
	var oldFiles []string
	if state != nil {
		if old := state.FindInstalled(name); old != nil {
			oldFiles = append([]string{}, old.Files...)
		}
	}

	fmt.Fprintf(out, "Installing %s %s from %s...\n", pkg.Name, pkg.Version, pkg.Source)

	entry, err := registry.InstallPackage(pkg, f.WorkingDir)
	if err != nil {
		return fmt.Errorf("install package: %w", err)
	}

	removed := removeStaleFilesFromInstall(oldFiles, entry.Files)
	if removed > 0 {
		fmt.Fprintf(out, "Removed %d stale file(s)\n", removed)
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

func (f *CommandFactory) newPluginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "plugin",
		Aliases: []string{"plugins"},
		Short:   "Manage DDx plugins",
	}
	installCmd := f.newInstallCommand()
	installCmd.Use = "install <name>"
	installCmd.Short = "Install a plugin"
	cmd.AddCommand(installCmd)
	return cmd
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
func commitPluginChanges(name, version string) {
	// Stage the project-local plugin tree and the skill copies.
	paths := []string{ddxroot.JoinRelative("plugins", name) + string(filepath.Separator), ".agents/skills/",
		".claude/skills/",
	}
	ctx := context.Background()
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			gitAdd := internalgit.Command(ctx, "", "add", p)
			_ = gitAdd.Run()
		}
	}

	// Check if there's anything to commit.
	status := internalgit.Command(ctx, "", "diff", "--cached", "--quiet")
	if status.Run() == nil {
		return // nothing staged
	}

	msg := fmt.Sprintf("chore: install %s %s", name, version)
	gitCommit := internalgit.Command(ctx, "", "commit", "-m", msg)
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

// installLocal installs a plugin from a local directory into the current
// checkout as a developer overlay. It intentionally does not update installed
// state and does not auto-commit: the recorded plugin pin remains whatever the
// project already declares, while this machine can test a local plugin tree.
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

	pluginDir := pkg.Install.Root.Target

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
	entry.Files = append(entry.Files, pluginDir)

	// Install local skills as symlinks into each supported harness surface.
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
	for _, issue := range issues {
		if rel, ok := localInstallIssueRel(roots, issue.Path); ok && shouldSkipLocalInstallIssue(rel) {
			continue
		}
		filtered = append(filtered, issue)
	}
	return filtered
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
