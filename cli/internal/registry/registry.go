package registry

import (
	"fmt"
	"strings"
)

// PackageType represents the type of a registry package.
type PackageType string

const (
	PackageTypeWorkflow     PackageType = "workflow"
	PackageTypePlugin       PackageType = "plugin"
	PackageTypePersonaPack  PackageType = "persona-pack"
	PackageTypeTemplatePack PackageType = "template-pack"
	PackageTypeResource     PackageType = "resource"
)

// InstallMapping describes a source→target mapping from a plugin package.
// Marketplace installs use these mappings to select cache contents and
// generated adapter sources; only legacy compatibility installs copy them
// directly into a project worktree.
type InstallMapping struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// PackageInstall describes the plugin package materialization contract.
type PackageInstall struct {
	Root       *InstallMapping  `yaml:"root,omitempty"`       // package root; registry installs cache this payload, local overlays may link it
	Skills     []InstallMapping `yaml:"skills,omitempty"`     // skill source directories used to generate agent adapter shims
	Scripts    *InstallMapping  `yaml:"scripts,omitempty"`    // scripts/binaries
	Symlinks   []SymlinkMapping `yaml:"symlinks,omitempty"`   // post-install symlinks
	Executable []string         `yaml:"executable,omitempty"` // paths (relative to root) that must be executable
}

// SymlinkMapping describes a symlink to create during installation.
type SymlinkMapping struct {
	Source string `yaml:"source"` // the source path (relative to install root)
	Target string `yaml:"target"` // the target path (where symlink points)
}

// Package describes a single installable package.
type Package struct {
	Name        string         `yaml:"name"`
	Version     string         `yaml:"version"`
	Description string         `yaml:"description"`
	Type        PackageType    `yaml:"type"`
	Source      string         `yaml:"source"`
	APIVersion  string         `yaml:"api_version,omitempty"`
	Install     PackageInstall `yaml:"install"`
	Keywords    []string       `yaml:"keywords,omitempty"`
	// Extra captures top-level keys not recognized by this DDx version so
	// newer manifests can keep their unknown fields in memory without
	// silently dropping them. See SD-018 "Manifest Versioning". Not
	// marshaled via `yaml:"-"` so callers must preserve it explicitly.
	Extra map[string]any `yaml:"-"`
}

// Registry holds the list of known packages.
type Registry struct {
	Packages []Package
}

// BuiltinRegistry returns the built-in registry of known packages.
func BuiltinRegistry() *Registry {
	return &Registry{
		Packages: []Package{
			{
				Name:        "ddx",
				Version:     "0.4.7",
				Description: "DDx bootstrap skill package for offline init and worker discovery",
				Type:        PackageTypePlugin,
				Source:      "https://github.com/DocumentDrivenDX/ddx",
				Install: PackageInstall{
					Skills: []InstallMapping{
						{Source: "skills/", Target: ".agents/skills/"},
						{Source: "skills/", Target: ".claude/skills/"},
					},
				},
				Keywords: []string{"bootstrap", "default", "skills", "offline"},
			},
			{
				Name:        "helix",
				Version:     "0.3.2",
				Description: "Supervisory autopilot for AI-assisted software delivery",
				Type:        PackageTypeWorkflow,
				Source:      "https://github.com/DocumentDrivenDX/helix",
				Install: PackageInstall{
					Root: &InstallMapping{
						Source: ".",
						// Compatibility manifest target. Marketplace installs
						// cache HELIX under XDG; .ddx/plugins/helix is reserved
						// for an explicit local checkout overlay.
						Target: ".ddx/plugins/helix",
					},
					// Registry installs generate project-local adapter shims
					// from cached skill sources.
					Skills: []InstallMapping{
						{Source: ".agents/skills/", Target: ".agents/skills/"},
						{Source: ".agents/skills/", Target: ".claude/skills/"},
					},
				},
				Keywords: []string{"workflow", "methodology", "ai", "development"},
			},
		},
	}
}

// Find returns the package with the given name, or an error if not found.
func (r *Registry) Find(name string) (*Package, error) {
	for i := range r.Packages {
		if r.Packages[i].Name == name {
			return &r.Packages[i], nil
		}
	}
	return nil, fmt.Errorf("package %q not found in registry", name)
}

// Search returns all packages whose name, description, or keywords contain the query.
func (r *Registry) Search(query string) []Package {
	q := strings.ToLower(query)
	var results []Package
	for _, pkg := range r.Packages {
		if matchesQuery(pkg, q) {
			results = append(results, pkg)
		}
	}
	return results
}

func matchesQuery(pkg Package, query string) bool {
	if strings.Contains(strings.ToLower(pkg.Name), query) {
		return true
	}
	if strings.Contains(strings.ToLower(pkg.Description), query) {
		return true
	}
	if strings.Contains(strings.ToLower(string(pkg.Type)), query) {
		return true
	}
	for _, kw := range pkg.Keywords {
		if strings.Contains(strings.ToLower(kw), query) {
			return true
		}
	}
	return false
}

// IsResourcePath returns true if name looks like a resource path (e.g. "persona/foo").
func IsResourcePath(name string) bool {
	return strings.Contains(name, "/")
}
