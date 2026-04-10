package registry

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/skills"
)

// AuditInstalledEntry checks one installed package entry for plugin issues.
func AuditInstalledEntry(entry InstalledEntry, fallback *Package) []ValidationIssue {
	var issues []ValidationIssue
	root := installedRootPath(entry)
	if root == "" {
		issues = append(issues, ValidationIssue{Message: fmt.Sprintf("installed entry %q has no recorded plugin root", entry.Name)})
		return issues
	}

	issues = append(issues, auditRecordedFiles(entry)...)

	manifest, manifestMissing, manifestIssues := loadPackageDefinitionForAudit(root, fallback)
	issues = append(issues, manifestIssues...)

	if manifestMissing {
		issues = append(issues, ValidationIssue{
			Path:    filepath.Join(root, "package.yaml"),
			Message: "missing package.yaml",
		})
	}

	if manifest == nil {
		manifest = &Package{}
	}

	issues = append(issues, ValidatePackageStructure(root, manifest)...)
	return issues
}

// ValidatePackageStructure checks the skill, symlink, and executable layout
// for a package root before install or after install-state recovery.
func ValidatePackageStructure(root string, pkg *Package) []ValidationIssue {
	if pkg == nil {
		return []ValidationIssue{{Path: filepath.Join(root, "package.yaml"), Message: "missing package definition"}}
	}
	return auditPluginRoot(root, pkg)
}

func loadPackageDefinitionForAudit(root string, fallback *Package) (*Package, bool, []ValidationIssue) {
	manifest, manifestMissing, issues, err := LoadPackageManifestWithFallback(root, fallback)
	if err == nil || os.IsNotExist(err) {
		return manifest, manifestMissing, nil
	}

	if len(issues) == 0 {
		issues = append(issues, ValidationIssue{Path: filepath.Join(root, "package.yaml"), Message: err.Error()})
	}
	return manifest, false, issues
}

func auditPluginRoot(root string, pkg *Package) []ValidationIssue {
	var issues []ValidationIssue

	if pkg.APIVersion != "" && pkg.APIVersion != SupportedPackageAPIVersion {
		issues = append(issues, ValidationIssue{
			Path:    filepath.Join(root, "package.yaml"),
			Message: fmt.Sprintf("unsupported `api_version` %q (supported: %s)", pkg.APIVersion, SupportedPackageAPIVersion),
		})
	}

	for _, symlinkIssue := range auditBrokenSymlinks(root) {
		issues = append(issues, symlinkIssue)
	}

	for _, skillRoot := range collectSkillRoots(root, pkg) {
		issues = append(issues, auditSkillRoot(skillRoot)...)
	}

	for _, rel := range pkg.Install.Executable {
		issues = append(issues, auditExecutable(root, rel)...)
	}

	return issues
}

func auditRecordedFiles(entry InstalledEntry) []ValidationIssue {
	var issues []ValidationIssue
	seen := map[string]bool{}
	recordedPaths := append([]string(nil), entry.Files...)
	if root := installedRootPath(entry); root != "" {
		recordedPaths = append(recordedPaths, root)
	}
	for _, recorded := range recordedPaths {
		path := ExpandHome(recorded)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				issues = append(issues, ValidationIssue{Path: path, Message: "missing recorded file or symlink"})
				continue
			}
			issues = append(issues, ValidationIssue{Path: path, Message: err.Error()})
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}

		target, err := os.Readlink(path)
		if err != nil {
			issues = append(issues, ValidationIssue{Path: path, Message: fmt.Sprintf("broken symlink: %v", err)})
			continue
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		if _, err := os.Stat(target); err != nil {
			issues = append(issues, ValidationIssue{Path: path, Message: fmt.Sprintf("broken symlink target %q: %v", target, err)})
		}
	}
	return issues
}

func auditBrokenSymlinks(root string) []ValidationIssue {
	var issues []ValidationIssue
	walkRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil && resolved != "" {
		walkRoot = resolved
	}

	_ = filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			issues = append(issues, ValidationIssue{Path: path, Message: err.Error()})
			return nil
		}
		if path == walkRoot {
			return nil
		}

		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				issues = append(issues, ValidationIssue{Path: path, Message: "missing installed file"})
				return nil
			}
			issues = append(issues, ValidationIssue{Path: path, Message: err.Error()})
			return nil
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return nil
		}

		target, err := os.Readlink(path)
		if err != nil {
			issues = append(issues, ValidationIssue{Path: path, Message: fmt.Sprintf("broken symlink: %v", err)})
			return nil
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		if _, err := os.Stat(target); err != nil {
			issues = append(issues, ValidationIssue{Path: path, Message: fmt.Sprintf("broken symlink target %q: %v", target, err)})
		}
		return nil
	})

	return issues
}

func collectSkillRoots(root string, pkg *Package) []string {
	seen := map[string]bool{}
	var roots []string

	add := func(candidate string) {
		if strings.TrimSpace(candidate) == "" {
			return
		}
		abs := candidate
		if resolved, err := filepath.EvalSymlinks(candidate); err == nil && resolved != "" {
			abs = resolved
		} else if resolved, err := filepath.Abs(candidate); err == nil && resolved != "" {
			abs = resolved
		}
		if seen[abs] {
			return
		}
		if _, err := os.Stat(candidate); err != nil {
			return
		}
		seen[abs] = true
		roots = append(roots, candidate)
	}

	for _, mapping := range pkg.Install.Skills {
		add(filepath.Join(root, filepath.FromSlash(mapping.Source)))
	}

	add(filepath.Join(root, "skills"))
	add(filepath.Join(root, ".agents", "skills"))
	add(filepath.Join(root, ".claude", "skills"))

	return roots
}

func auditSkillRoot(root string) []ValidationIssue {
	var issues []ValidationIssue

	info, err := os.Stat(root)
	if err != nil {
		issues = append(issues, ValidationIssue{Path: root, Message: err.Error()})
		return issues
	}
	if !info.IsDir() {
		issues = append(issues, ValidationIssue{Path: root, Message: "expected a skill directory"})
		return issues
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		issues = append(issues, ValidationIssue{Path: root, Message: err.Error()})
		return issues
	}

	for _, entry := range entries {
		skillPath := filepath.Join(root, entry.Name())
		skillInfo, err := os.Lstat(skillPath)
		if err != nil {
			issues = append(issues, ValidationIssue{Path: skillPath, Message: err.Error()})
			continue
		}
		if skillInfo.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Stat(skillPath); err != nil {
				issues = append(issues, ValidationIssue{Path: skillPath, Message: fmt.Sprintf("broken symlink: %v", err)})
				continue
			}
		}
		if !skillInfo.IsDir() && skillInfo.Mode()&os.ModeSymlink == 0 {
			continue
		}

		skillFile := filepath.Join(skillPath, "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			if os.IsNotExist(err) {
				issues = append(issues, ValidationIssue{Path: skillPath, Message: "missing SKILL.md"})
				continue
			}
			issues = append(issues, ValidationIssue{Path: skillFile, Message: err.Error()})
			continue
		}

		for _, issue := range skills.ValidateContent(skillFile, data) {
			issues = append(issues, ValidationIssue{Path: issue.Path, Message: issue.Message})
		}
	}

	return issues
}

func auditExecutable(root, rel string) []ValidationIssue {
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(path)
	if err != nil {
		return []ValidationIssue{{Path: path, Message: err.Error()}}
	}
	if info.IsDir() {
		return []ValidationIssue{{Path: path, Message: "expected executable file, found directory"}}
	}
	if info.Mode()&0111 == 0 {
		return []ValidationIssue{{Path: path, Message: "file lost execute permission"}}
	}
	return nil
}

func installedRootPath(entry InstalledEntry) string {
	if len(entry.Files) > 0 {
		return ExpandHome(entry.Files[0])
	}
	if strings.TrimSpace(entry.Source) != "" {
		return ExpandHome(entry.Source)
	}
	return ""
}
