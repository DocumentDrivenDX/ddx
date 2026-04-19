package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const SupportedPackageAPIVersion = "1"

// ValidationIssue represents one package or plugin validation problem.
type ValidationIssue struct {
	Path    string
	Message string
}

func (i ValidationIssue) Error() string {
	if i.Path == "" {
		return i.Message
	}
	return fmt.Sprintf("%s: %s", i.Path, i.Message)
}

type packageManifestRaw struct {
	Name        string         `yaml:"name"`
	Version     string         `yaml:"version"`
	Description string         `yaml:"description"`
	Type        string         `yaml:"type"`
	Source      string         `yaml:"source"`
	APIVersion  any            `yaml:"api_version"`
	Install     PackageInstall `yaml:"install"`
	Keywords    []string       `yaml:"keywords"`
}

// LoadPackageManifest reads and validates package.yaml from root.
func LoadPackageManifest(root string) (*Package, []ValidationIssue, error) {
	manifestPath := filepath.Join(root, "package.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, nil, err
	}

	var raw packageManifestRaw
	if err := yaml.Unmarshal(data, &raw); err != nil {
		issue := ValidationIssue{
			Path:    manifestPath,
			Message: fmt.Sprintf("invalid YAML: %v", err),
		}
		return nil, []ValidationIssue{issue}, fmt.Errorf("%s: invalid YAML: %w", manifestPath, err)
	}

	pkg, issues := validatePackageManifest(manifestPath, raw)
	if len(issues) > 0 && pkg == nil {
		return nil, issues, fmt.Errorf("%s: %s", manifestPath, JoinValidationIssues(issues))
	}
	// Even when validation issues exist, return the partial package so callers
	// can perform structural audits (symlinks, SKILL.md files) independently.
	// However, if no package was returned at all, return an error so callers
	// know something is fundamentally wrong.
	if pkg == nil {
		return nil, issues, fmt.Errorf("%s: %s", manifestPath, JoinValidationIssues(issues))
	}
	return pkg, issues, nil
}

// LoadPackageManifestWithFallback reads package.yaml and falls back to the
// provided package when the manifest is missing.
func LoadPackageManifestWithFallback(root string, fallback *Package) (*Package, bool, []ValidationIssue, error) {
	pkg, issues, err := LoadPackageManifest(root)
	if err == nil {
		// Return issues separately even when package is valid, so callers can
		// detect schema problems while still using the package for structural audits.
		if len(issues) > 0 {
			return pkg, false, issues, nil
		}
		return pkg, false, nil, nil
	}

	// If a partial package was returned (YAML parsed but validation issues exist),
	// return it with the issues so callers can still use its structure.
	if pkg != nil {
		return pkg, false, issues, nil
	}

	if os.IsNotExist(err) {
		return fallback, true, nil, err
	}

	if fallback != nil {
		return fallback, false, issues, err
	}
	return nil, false, issues, err
}

// validatePackageManifest validates required fields and returns a partial package
// even when validation issues are found. This ensures that structural audits
// (symlinks, SKILL.md files) can proceed independently of manifest schema issues.
func validatePackageManifest(path string, raw packageManifestRaw) (*Package, []ValidationIssue) {
	var issues []ValidationIssue

	name := strings.TrimSpace(raw.Name)
	if name == "" {
		issues = append(issues, ValidationIssue{Path: path, Message: "missing required field `name`"})
	}
	version := strings.TrimSpace(raw.Version)
	if version == "" {
		issues = append(issues, ValidationIssue{Path: path, Message: "missing required field `version`"})
	}
	description := strings.TrimSpace(raw.Description)
	if description == "" {
		issues = append(issues, ValidationIssue{Path: path, Message: "missing required field `description`"})
	}
	pkgType, ok := parseManifestPackageType(raw.Type)
	if !ok {
		issues = append(issues, ValidationIssue{Path: path, Message: fmt.Sprintf("unsupported `type` %q", strings.TrimSpace(raw.Type))})
	}
	source := strings.TrimSpace(raw.Source)
	if source == "" {
		issues = append(issues, ValidationIssue{Path: path, Message: "missing required field `source`"})
	}

	apiVersion, apiIssue := parseAPIVersion(raw.APIVersion)
	if apiIssue != "" {
		issues = append(issues, ValidationIssue{Path: path, Message: apiIssue})
	}

	// Always construct a partial package so structural audits can proceed.
	// Validation issues are still collected and returned separately.
	pkg := &Package{
		Name:        name,
		Version:     version,
		Description: description,
		Type:        pkgType,
		Source:      source,
		APIVersion:  apiVersion,
		Install:     raw.Install,
		Keywords:    append([]string(nil), raw.Keywords...),
	}
	return pkg, issues
}

func parseManifestPackageType(rawType string) (PackageType, bool) {
	switch PackageType(strings.TrimSpace(rawType)) {
	case PackageTypeWorkflow, PackageTypePlugin, PackageTypePersonaPack, PackageTypeTemplatePack:
		return PackageType(strings.TrimSpace(rawType)), true
	default:
		return "", false
	}
}

func parseAPIVersion(raw any) (string, string) {
	if raw == nil {
		return "", "missing required field `api_version`"
	}

	value := strings.TrimSpace(fmt.Sprint(raw))
	if value == "" {
		return "", "missing required field `api_version`"
	}
	if value != SupportedPackageAPIVersion {
		return value, fmt.Sprintf("unsupported `api_version` %q (supported: %s)", value, SupportedPackageAPIVersion)
	}
	return value, ""
}

// JoinValidationIssues renders a slice of validation issues into a single
// semicolon-separated error string.
func JoinValidationIssues(issues []ValidationIssue) string {
	msgs := make([]string, 0, len(issues))
	for _, issue := range issues {
		msgs = append(msgs, issue.Error())
	}
	return strings.Join(msgs, "; ")
}

func defaultPackageRootTarget(name string) string {
	return fmt.Sprintf(".ddx/plugins/%s", name)
}
