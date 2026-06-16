package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"gopkg.in/yaml.v3"
)

const ProjectPluginLockFile = "plugins.lock.yaml"

// PluginLock records project plugin intent. Payloads live in the shared cache;
// project trees keep only this lock plus generated harness skill shims.
type PluginLock struct {
	Plugins []PluginLockEntry `yaml:"plugins"`
}

type PluginLockEntry struct {
	Name           string      `yaml:"name"`
	Version        string      `yaml:"version"`
	Type           PackageType `yaml:"type"`
	Source         string      `yaml:"source"`
	CachePath      string      `yaml:"cache_path"`
	InstalledAt    time.Time   `yaml:"installed_at"`
	GeneratedFiles []string    `yaml:"generated_files,omitempty"`
}

type PluginSyncResult struct {
	Name           string
	Version        string
	CachePresent   bool
	LocalOverlay   bool
	GeneratedFiles []string
}

func ProjectPluginLockPath(ctx context.Context, projectRoot string) string {
	return filepath.Join(ddxroot.Path(ctx, projectRoot), ProjectPluginLockFile)
}

func LoadProjectPluginLock(ctx context.Context, projectRoot string) (*PluginLock, error) {
	path := ProjectPluginLockPath(ctx, projectRoot)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &PluginLock{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading plugin lock: %w", err)
	}
	var lock PluginLock
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parsing plugin lock: %w", err)
	}
	return &lock, nil
}

func SaveProjectPluginLock(ctx context.Context, projectRoot string, lock *PluginLock) error {
	path := ProjectPluginLockPath(ctx, projectRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating plugin lock directory: %w", err)
	}
	data, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("marshaling plugin lock: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing plugin lock: %w", err)
	}
	return nil
}

func (l *PluginLock) Find(name string) *PluginLockEntry {
	if l == nil {
		return nil
	}
	for i := range l.Plugins {
		if l.Plugins[i].Name == name {
			return &l.Plugins[i]
		}
	}
	return nil
}

func (l *PluginLock) AddOrUpdate(entry PluginLockEntry) {
	for i := range l.Plugins {
		if l.Plugins[i].Name == entry.Name {
			l.Plugins[i] = entry
			sort.Slice(l.Plugins, func(i, j int) bool { return l.Plugins[i].Name < l.Plugins[j].Name })
			return
		}
	}
	l.Plugins = append(l.Plugins, entry)
	sort.Slice(l.Plugins, func(i, j int) bool { return l.Plugins[i].Name < l.Plugins[j].Name })
}

func (l *PluginLock) Remove(name string) bool {
	if l == nil {
		return false
	}
	for i := range l.Plugins {
		if l.Plugins[i].Name == name {
			l.Plugins = append(l.Plugins[:i], l.Plugins[i+1:]...)
			return true
		}
	}
	return false
}

func PluginCacheDir(name, version string) string {
	return filepath.Join(filepath.Dir(ddxroot.GlobalDir()), "cache", "plugins", cleanCacheSegment(name), cleanCacheSegment(version))
}

func cleanCacheSegment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "_"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "..", "_")
	return replacer.Replace(s)
}

// CachePackageFromRemote downloads a registry package and materializes only the
// declared plugin root into the shared XDG cache.
func CachePackageFromRemote(pkg *Package) (PluginLockEntry, error) {
	entry := PluginLockEntry{
		Name:        pkg.Name,
		Version:     pkg.Version,
		Type:        pkg.Type,
		Source:      pkg.Source,
		InstalledAt: time.Now(),
	}
	if pkg.Install.Root != nil && strings.HasPrefix(pkg.Install.Root.Target, "~") {
		return entry, fmt.Errorf("FEAT-015: Root.Target must be project-relative; got %s in package %s; update the manifest to use a relative path", pkg.Install.Root.Target, pkg.Name)
	}

	tmpDir, err := os.MkdirTemp("", "ddx-plugin-cache-"+pkg.Name+"-*")
	if err != nil {
		return entry, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tarballURL := githubTarballURL(pkg.Source, pkg.Version)
	extractedDir, err := downloadAndExtract(tarballURL, tmpDir)
	if err != nil {
		return entry, fmt.Errorf("downloading %s: %w", tarballURL, err)
	}
	return cachePackageFromDir(pkg, extractedDir, entry)
}

func CachePackageFromDirectory(pkg *Package, sourceDir string) (PluginLockEntry, error) {
	entry := PluginLockEntry{
		Name:        pkg.Name,
		Version:     pkg.Version,
		Type:        pkg.Type,
		Source:      pkg.Source,
		InstalledAt: time.Now(),
	}
	return cachePackageFromDir(pkg, sourceDir, entry)
}

func cachePackageFromDir(pkg *Package, sourceDir string, entry PluginLockEntry) (PluginLockEntry, error) {
	manifestPkg, manifestMissing, manifestIssues, manifestErr := LoadPackageManifestWithFallback(sourceDir, pkg)
	if manifestErr == nil || (os.IsNotExist(manifestErr) && manifestMissing) {
		if manifestPkg != nil {
			pkg = manifestPkg
		}
	} else {
		if len(manifestIssues) > 0 {
			return entry, fmt.Errorf("validating package manifest: %s", JoinValidationIssues(manifestIssues))
		}
		return entry, fmt.Errorf("loading package manifest: %w", manifestErr)
	}
	if pkg.Install.Root == nil {
		pkg.Install.Root = &InstallMapping{Source: ".", Target: defaultPackageRootTarget(pkg.Name)}
	}
	if strings.HasPrefix(pkg.Install.Root.Target, "~") {
		return entry, fmt.Errorf("FEAT-015: Root.Target must be project-relative; got %s in package %s; update the manifest to use a relative path", pkg.Install.Root.Target, pkg.Name)
	}
	rootSource := strings.TrimSpace(pkg.Install.Root.Source)
	if rootSource == "" {
		rootSource = "."
	}
	packageRoot := filepath.Join(sourceDir, filepath.FromSlash(rootSource))
	if issues := ValidatePackageStructure(packageRoot, pkg); len(issues) > 0 {
		return entry, fmt.Errorf("validating package structure: %s", JoinValidationIssues(issues))
	}

	cacheDir := PluginCacheDir(pkg.Name, pkg.Version)
	if err := os.RemoveAll(cacheDir); err != nil {
		return entry, fmt.Errorf("clearing plugin cache %s: %w", cacheDir, err)
	}
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
		return entry, fmt.Errorf("creating plugin cache root: %w", err)
	}
	if _, err := copyMapping(sourceDir, &InstallMapping{Source: rootSource, Target: cacheDir}); err != nil {
		return entry, fmt.Errorf("caching plugin root: %w", err)
	}

	entry.Name = pkg.Name
	entry.Version = pkg.Version
	entry.Type = pkg.Type
	entry.Source = pkg.Source
	entry.CachePath = cacheDir
	return entry, nil
}

func SyncProjectPlugins(ctx context.Context, projectRoot string, force bool) ([]PluginSyncResult, error) {
	lock, err := LoadProjectPluginLock(ctx, projectRoot)
	if err != nil {
		return nil, err
	}
	var results []PluginSyncResult
	changed := false
	for _, entry := range lock.Plugins {
		result, err := SyncProjectPlugin(ctx, projectRoot, entry, force)
		if err != nil {
			return results, err
		}
		results = append(results, result)
		if locked := lock.Find(entry.Name); locked != nil && !sameStrings(locked.GeneratedFiles, result.GeneratedFiles) {
			locked.GeneratedFiles = result.GeneratedFiles
			changed = true
		}
	}
	if changed {
		if err := SaveProjectPluginLock(ctx, projectRoot, lock); err != nil {
			return results, err
		}
	}
	return results, nil
}

func SyncProjectPlugin(ctx context.Context, projectRoot string, entry PluginLockEntry, force bool) (PluginSyncResult, error) {
	result := PluginSyncResult{Name: entry.Name, Version: entry.Version}
	if isLocalOverlay(ctx, projectRoot, entry.Name) {
		result.LocalOverlay = true
		return result, nil
	}
	cachePath := entry.CachePath
	if cachePath == "" {
		cachePath = PluginCacheDir(entry.Name, entry.Version)
	}
	if info, err := os.Stat(cachePath); err != nil || !info.IsDir() {
		result.CachePresent = false
		return result, fmt.Errorf("plugin %s %s cache missing at %s; run ddx plugin install %s", entry.Name, entry.Version, cachePath, entry.Name)
	}
	result.CachePresent = true

	fallback := &Package{
		Name:    entry.Name,
		Version: entry.Version,
		Type:    entry.Type,
		Source:  entry.Source,
	}
	pkg, _, issues, err := LoadPackageManifestWithFallback(cachePath, fallback)
	if err != nil && len(issues) > 0 {
		return result, fmt.Errorf("validating cached plugin manifest: %s", JoinValidationIssues(issues))
	}
	if err != nil && pkg == nil {
		return result, fmt.Errorf("loading cached plugin manifest: %w", err)
	}
	if pkg == nil {
		pkg = fallback
	}
	files, err := materializeSkillShims(cachePath, projectRoot, pkg.Install.Skills, force)
	if err != nil {
		return result, err
	}
	result.GeneratedFiles = files
	return result, nil
}

func isLocalOverlay(ctx context.Context, projectRoot, name string) bool {
	path := filepath.Join(ddxroot.Path(ctx, projectRoot), "plugins", name)
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func materializeSkillShims(cachePath, projectRoot string, mappings []InstallMapping, force bool) ([]string, error) {
	sourceRoots, err := skillSourceRoots(cachePath, mappings)
	if err != nil {
		return nil, err
	}
	var written []string
	seen := map[string]bool{}
	for _, sourceRoot := range sourceRoots {
		entries, err := os.ReadDir(sourceRoot)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			name := entry.Name()
			if seen[name] {
				continue
			}
			src := filepath.Join(sourceRoot, name)
			if _, err := os.Stat(filepath.Join(src, "SKILL.md")); err != nil {
				return nil, fmt.Errorf("%s: missing SKILL.md: %w", src, err)
			}
			for _, surface := range []string{".agents/skills", ".claude/skills"} {
				dst := filepath.Join(projectRoot, filepath.FromSlash(surface), name)
				if err := ensureSkillShim(src, dst, force); err != nil {
					return nil, err
				}
				written = append(written, filepath.Join(filepath.FromSlash(surface), name))
			}
			seen[name] = true
		}
	}
	sort.Strings(written)
	return written, nil
}

func skillSourceRoots(cachePath string, mappings []InstallMapping) ([]string, error) {
	seen := map[string]bool{}
	var roots []string
	if len(mappings) == 0 {
		mappings = []InstallMapping{{Source: "skills/"}}
	}
	for _, mapping := range mappings {
		source := strings.TrimSpace(mapping.Source)
		if source == "" {
			continue
		}
		candidate := filepath.Clean(filepath.Join(cachePath, filepath.FromSlash(source)))
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
		if !seen[candidate] {
			seen[candidate] = true
			roots = append(roots, candidate)
		}
	}
	return roots, nil
}

func ensureSkillShim(src, dst string, force bool) error {
	if info, err := os.Lstat(dst); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(dst)
			if readErr == nil {
				resolved := target
				if !filepath.IsAbs(resolved) {
					resolved = filepath.Join(filepath.Dir(dst), resolved)
				}
				if samePath(resolved, src) {
					return nil
				}
			}
			if err := os.RemoveAll(dst); err != nil {
				return fmt.Errorf("replacing skill shim %s: %w", dst, err)
			}
		} else if force {
			if err := os.RemoveAll(dst); err != nil {
				return fmt.Errorf("replacing skill directory %s: %w", dst, err)
			}
		} else {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking skill shim %s: %w", dst, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("creating skill shim directory: %w", err)
	}
	target, err := filepath.Rel(filepath.Dir(dst), src)
	if err != nil {
		target = src
	}
	if err := os.Symlink(target, dst); err != nil {
		return fmt.Errorf("linking skill shim %s -> %s: %w", dst, target, err)
	}
	return nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.EvalSymlinks(a)
	bb, errB := filepath.EvalSymlinks(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	return errA == nil && errB == nil && absA == absB
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}
