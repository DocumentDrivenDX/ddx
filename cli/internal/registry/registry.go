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

// InstallMapping describes a source→target copy during installation.
type InstallMapping struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// PackageInstall describes what to copy during installation.
type PackageInstall struct {
	Skills  *InstallMapping `yaml:"skills,omitempty"`
	Scripts *InstallMapping `yaml:"scripts,omitempty"`
}

// Package describes a single installable package.
type Package struct {
	Name        string         `yaml:"name"`
	Version     string         `yaml:"version"`
	Description string         `yaml:"description"`
	Type        PackageType    `yaml:"type"`
	Source      string         `yaml:"source"`
	Install     PackageInstall `yaml:"install"`
	Keywords    []string       `yaml:"keywords,omitempty"`
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
				Name:        "helix",
				Version:     "0.1.0",
				Description: "Structured development workflow with AI-assisted collaboration",
				Type:        PackageTypeWorkflow,
				Source:      "https://github.com/easel/helix",
				Install: PackageInstall{
					Skills: &InstallMapping{
						Source: ".agents/skills/",
						Target: "~/.agents/skills/",
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
