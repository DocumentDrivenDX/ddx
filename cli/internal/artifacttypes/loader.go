package artifacttypes

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"gopkg.in/yaml.v3"
)

// Loader discovers artifact types under an installed plugin root and caches
// the normalized result until the on-disk mtimes change.
type Loader struct {
	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	index     *Index
	snapshots []snapshot
}

type snapshot struct {
	path    string
	exists  bool
	modTime time.Time
}

type metaDoc struct {
	Artifact struct {
		Name     string `yaml:"name"`
		ID       string `yaml:"id"`
		Type     string `yaml:"type"`
		Prefix   string `yaml:"prefix"`
		Phase    string `yaml:"phase"`
		Optional bool   `yaml:"optional"`
	} `yaml:"artifact"`
	Description string `yaml:"description"`
	Output      struct {
		Location string `yaml:"location"`
		Format   string `yaml:"format"`
		Naming   string `yaml:"naming"`
	} `yaml:"output"`
	Prompts struct {
		Generation string `yaml:"generation"`
		Review     string `yaml:"review"`
	} `yaml:"prompts"`
	Template struct {
		File string `yaml:"file"`
	} `yaml:"template"`
	Examples []struct {
		File        string `yaml:"file"`
		Description string `yaml:"description"`
	} `yaml:"examples"`
	IDFormat struct {
		Prefix      string `yaml:"prefix"`
		Pattern     string `yaml:"pattern"`
		Example     string `yaml:"example"`
		Description string `yaml:"description"`
	} `yaml:"id_format"`
	Name   string `yaml:"name"`
	ID     string `yaml:"id"`
	Prefix string `yaml:"prefix"`
	Phase  string `yaml:"phase"`
}

// NewLoader creates an empty loader with no cached roots.
func NewLoader() *Loader {
	return &Loader{cache: make(map[string]cacheEntry)}
}

// Load discovers artifact types for pluginRoot.
func (l *Loader) Load(pluginRoot string) (*Index, error) {
	if strings.TrimSpace(pluginRoot) == "" {
		return nil, fmt.Errorf("artifacttypes: plugin root is empty")
	}

	rootCanonical, err := canonicalPath(pluginRoot)
	if err != nil {
		return nil, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if entry, ok := l.cache[rootCanonical]; ok {
		if ok, err := entry.valid(); err == nil && ok {
			return entry.index.Clone(), nil
		}
	}

	pluginName, roots, snapshots, err := loadPluginRoots(rootCanonical)
	if err != nil {
		return nil, err
	}

	types, scannedSnapshots, err := scanPlugin(rootCanonical, pluginName, roots)
	if err != nil {
		return nil, err
	}

	snapshots = append(snapshots, scannedSnapshots...)
	index, err := buildIndex(types)
	if err != nil {
		return nil, err
	}
	l.cache[rootCanonical] = cacheEntry{index: index.Clone(), snapshots: snapshots}
	return index.Clone(), nil
}

func (e cacheEntry) valid() (bool, error) {
	for _, snap := range e.snapshots {
		info, err := os.Stat(snap.path)
		if err != nil {
			if os.IsNotExist(err) {
				return !snap.exists, nil
			}
			return false, err
		}
		if !snap.exists {
			return false, nil
		}
		if !info.ModTime().Equal(snap.modTime) {
			return false, nil
		}
	}
	return true, nil
}

func loadPluginRoots(rootCanonical string) (string, []string, []snapshot, error) {
	snapshots := []snapshot{recordSnapshot(rootCanonical)}
	manifest, _, err := registry.LoadPackageManifest(rootCanonical)
	if err != nil {
		if os.IsNotExist(err) {
			return filepath.Base(rootCanonical), []string{"workflows"}, snapshots, nil
		}
		// If the manifest parsed but had issues, still use the partial package
		// so artifact roots can be resolved from the preserved Extra map.
		if manifest == nil {
			return "", nil, snapshots, err
		}
	}
	if manifest == nil {
		return filepath.Base(rootCanonical), []string{"workflows"}, snapshots, nil
	}

	roots, err := artifactTypeRoots(manifest)
	if err != nil {
		return "", nil, snapshots, err
	}
	pluginName := strings.TrimSpace(manifest.Name)
	if pluginName == "" {
		pluginName = filepath.Base(rootCanonical)
	}
	snapshots = append(snapshots, recordSnapshot(filepath.Join(rootCanonical, "package.yaml")))
	return pluginName, roots, snapshots, nil
}

func artifactTypeRoots(pkg *registry.Package) ([]string, error) {
	if pkg == nil {
		return []string{"workflows"}, nil
	}
	extra, ok := pkg.Extra["artifact_type_roots"]
	if !ok {
		return []string{"workflows"}, nil
	}

	roots, err := stringsSlice(extra)
	if err != nil {
		return nil, fmt.Errorf("artifact_type_roots: %w", err)
	}
	if len(roots) == 0 {
		return []string{}, nil
	}
	return roots, nil
}

func stringsSlice(raw any) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...), nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string entries, got %T", item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected list, got %T", raw)
	}
}

func scanPlugin(rootCanonical, pluginName string, patterns []string) ([]Type, []snapshot, error) {
	if len(patterns) == 0 {
		return []Type{}, []snapshot{recordSnapshot(rootCanonical)}, nil
	}

	concreteRoots, err := resolveRoots(rootCanonical, patterns)
	if err != nil {
		return nil, nil, err
	}

	var (
		types     []Type
		snapshots []snapshot
	)
	for _, root := range concreteRoots {
		snapshots = append(snapshots, recordSnapshot(root))
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			snapshots = append(snapshots, recordSnapshot(p))
			if !d.IsDir() {
				if filepath.Base(p) != "meta.yml" {
					return nil
				}
				typ, extraSnaps, err := parseTypeMeta(rootCanonical, pluginName, p)
				if err != nil {
					return err
				}
				types = append(types, typ)
				snapshots = append(snapshots, extraSnaps...)
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return types, snapshots, nil
}

func parseTypeMeta(rootCanonical, pluginName, metaPath string) (Type, []snapshot, error) {
	metaCanonical, err := canonicalPath(metaPath)
	if err != nil {
		return Type{}, nil, err
	}
	if err := ensureWithinRoot(rootCanonical, metaCanonical); err != nil {
		return Type{}, nil, err
	}

	baseDir := filepath.Dir(metaCanonical)
	templatePath, err := resolveRelativeFile(rootCanonical, baseDir, "template.md")
	if err != nil {
		return Type{}, nil, fmt.Errorf("%s: template.md: %w", metaPath, err)
	}
	promptPath, err := resolveRelativeFile(rootCanonical, baseDir, "prompt.md")
	if err != nil {
		return Type{}, nil, fmt.Errorf("%s: prompt.md: %w", metaPath, err)
	}

	raw, err := os.ReadFile(metaCanonical)
	if err != nil {
		return Type{}, nil, fmt.Errorf("read %s: %w", metaPath, err)
	}
	var doc metaDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Type{}, nil, fmt.Errorf("parse %s: %w", metaPath, err)
	}

	typeID := firstNonEmpty(doc.Artifact.ID, doc.ID, filepath.Base(baseDir))
	name := firstNonEmpty(doc.Artifact.Name, doc.Name, humanizeTypeID(typeID))
	pattern := firstNonEmpty(doc.Output.Naming, doc.IDFormat.Pattern, doc.Output.Location)
	explicitPrefix := firstNonEmpty(doc.Artifact.Prefix, doc.IDFormat.Prefix, doc.Prefix)
	prefix := firstNonEmpty(explicitPrefix, inferPrefix(pattern), inferPrefixFromName(name), strings.ToUpper(typeID))
	prefixExplicit := explicitPrefix != ""
	phase := firstNonEmpty(doc.Artifact.Phase, doc.Phase, inferPhase(metaCanonical, rootCanonical))
	description := strings.TrimSpace(doc.Description)

	examples := make([]Example, 0, len(doc.Examples))
	if len(doc.Examples) > 0 {
		for _, ex := range doc.Examples {
			exPath, err := resolveRelativeFile(rootCanonical, baseDir, ex.File)
			if err != nil {
				return Type{}, nil, fmt.Errorf("%s: examples[%q]: %w", metaPath, ex.File, err)
			}
			rel, err := filepath.Rel(rootCanonical, exPath)
			if err != nil {
				return Type{}, nil, fmt.Errorf("%s: examples[%q]: %w", metaPath, ex.File, err)
			}
			examples = append(examples, Example{
				Path:        filepath.ToSlash(rel),
				Description: strings.TrimSpace(ex.Description),
			})
		}
	} else {
		examplePath, err := resolveRelativeFile(rootCanonical, baseDir, "example.md")
		if err == nil {
			rel, relErr := filepath.Rel(rootCanonical, examplePath)
			if relErr != nil {
				return Type{}, nil, relErr
			}
			examples = append(examples, Example{Path: filepath.ToSlash(rel)})
		}
	}

	metaRel, err := filepath.Rel(rootCanonical, metaCanonical)
	if err != nil {
		return Type{}, nil, err
	}
	templateRel, err := filepath.Rel(rootCanonical, templatePath)
	if err != nil {
		return Type{}, nil, err
	}
	promptRel, err := filepath.Rel(rootCanonical, promptPath)
	if err != nil {
		return Type{}, nil, err
	}

	typ := Type{
		Plugin:         pluginName,
		TypeID:         typeID,
		Name:           name,
		Description:    description,
		Prefix:         prefix,
		PrefixExplicit: prefixExplicit,
		Pattern:        pattern,
		Phase:          phase,
		TemplatePath:   filepath.ToSlash(templateRel),
		PromptPath:     filepath.ToSlash(promptRel),
		Examples:       examples,
		SourceMetaPath: filepath.ToSlash(metaRel),
	}
	return typ, []snapshot{recordSnapshot(metaCanonical)}, nil
}

func resolveRoots(rootCanonical string, patterns []string) ([]string, error) {
	var roots []string
	for _, raw := range patterns {
		pattern := strings.TrimSpace(filepath.ToSlash(raw))
		if pattern == "" {
			continue
		}
		if err := validateRelativePattern(pattern); err != nil {
			return nil, err
		}
		if hasGlob(pattern) {
			matches, err := globDirs(rootCanonical, pattern)
			if err != nil {
				return nil, err
			}
			roots = append(roots, matches...)
			continue
		}
		candidate := filepath.Join(rootCanonical, filepath.FromSlash(pattern))
		canonical, err := canonicalPath(candidate)
		if err != nil {
			return nil, err
		}
		if err := ensureWithinRoot(rootCanonical, canonical); err != nil {
			return nil, err
		}
		info, err := os.Stat(canonical)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		roots = append(roots, canonical)
	}
	roots = dedupeSortedPaths(roots)
	return roots, nil
}

func globDirs(rootCanonical, pattern string) ([]string, error) {
	var matches []string
	err := filepath.WalkDir(rootCanonical, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(rootCanonical, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}
		if matchGlobPath(pattern, rel) {
			matches = append(matches, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dedupeSortedPaths(matches), nil
}

func dedupeSortedPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)
	out := paths[:0]
	var prev string
	for i, p := range paths {
		if i == 0 || p != prev {
			out = append(out, p)
			prev = p
		}
	}
	return append([]string(nil), out...)
}

func matchGlobPath(pattern, candidate string) bool {
	pp := splitPath(pattern)
	cp := splitPath(candidate)
	return matchGlobSegments(pp, cp)
}

func matchGlobSegments(pattern, candidate []string) bool {
	if len(pattern) == 0 {
		return len(candidate) == 0
	}
	if pattern[0] == "**" {
		if len(pattern) == 1 {
			return true
		}
		for i := 0; i <= len(candidate); i++ {
			if matchGlobSegments(pattern[1:], candidate[i:]) {
				return true
			}
		}
		return false
	}
	if len(candidate) == 0 {
		return false
	}
	ok, err := path.Match(pattern[0], candidate[0])
	if err != nil || !ok {
		return false
	}
	return matchGlobSegments(pattern[1:], candidate[1:])
}

func splitPath(s string) []string {
	parts := strings.Split(filepath.ToSlash(strings.TrimSpace(s)), "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	return out
}

func validateRelativePattern(pattern string) error {
	if filepath.IsAbs(pattern) {
		return fmt.Errorf("artifact_type_roots: absolute paths are not allowed: %q", pattern)
	}
	for _, part := range splitPath(pattern) {
		if part == ".." {
			return fmt.Errorf("artifact_type_roots: path escapes plugin root: %q", pattern)
		}
	}
	return nil
}

func hasGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

func recordSnapshot(path string) snapshot {
	info, err := os.Stat(path)
	if err != nil {
		return snapshot{path: path}
	}
	return snapshot{path: path, exists: true, modTime: info.ModTime()}
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return abs, nil
		}
		return "", err
	}
	return resolved, nil
}

func ensureWithinRoot(rootCanonical, candidateCanonical string) error {
	rel, err := filepath.Rel(rootCanonical, candidateCanonical)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %s escapes plugin root %s", candidateCanonical, rootCanonical)
	}
	return nil
}

func resolveRelativeFile(rootCanonical, baseDir, rel string) (string, error) {
	trimmed := strings.TrimSpace(rel)
	if trimmed == "" {
		return "", fmt.Errorf("missing relative path")
	}
	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("absolute paths are not allowed: %q", trimmed)
	}
	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes type directory: %q", trimmed)
	}
	candidate := filepath.Join(baseDir, cleaned)
	info, err := os.Stat(candidate)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", candidate)
	}
	canonical, err := canonicalPath(candidate)
	if err != nil {
		return "", err
	}
	if err := ensureWithinRoot(rootCanonical, canonical); err != nil {
		return "", err
	}
	return canonical, nil
}

func inferPhase(metaCanonical, rootCanonical string) string {
	rel, err := filepath.Rel(rootCanonical, metaCanonical)
	if err != nil {
		return ""
	}
	parts := splitPath(rel)
	for i := 0; i < len(parts); i++ {
		if parts[i] != "phases" || i+1 >= len(parts) {
			continue
		}
		phase := parts[i+1]
		if idx := strings.Index(phase, "-"); idx >= 0 && idx+1 < len(phase) {
			return phase[idx+1:]
		}
		return phase
	}
	return ""
}

func inferPrefix(pattern string) string {
	if pattern == "" {
		return ""
	}
	base := filepath.Base(pattern)
	if idx := strings.IndexAny(base, "-_."); idx > 0 {
		base = base[:idx]
	}
	if base == "" {
		return ""
	}
	if isPrefixLike(base) {
		return base
	}
	return ""
}

func isPrefixLike(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r == '-' || r == ' ' {
			return false
		}
	}
	return strings.ToUpper(s) == s
}

func inferPrefixFromName(name string) string {
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, part := range strings.FieldsFunc(name, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '/'
	}) {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
