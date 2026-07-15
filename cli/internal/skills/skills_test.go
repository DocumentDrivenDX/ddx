package skills

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/registry/defaultplugin"
)

func TestEmbeddedSkillsHaveValidMetadata(t *testing.T) {
	var skillFiles []string
	err := fs.WalkDir(SkillFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Base(path) == "SKILL.md" {
			skillFiles = append(skillFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded skills: %v", err)
	}
	if len(skillFiles) == 0 {
		t.Fatal("no embedded SKILL.md files found")
	}

	for _, path := range skillFiles {
		data, err := SkillFiles.ReadFile(path)
		if err != nil {
			t.Fatalf("read embedded skill %s: %v", path, err)
		}
		if issues := ValidateContent(path, data); len(issues) > 0 {
			t.Fatalf("embedded skill validation failed: %v", issues[0])
		}
	}
}

func TestRepoSkillsHaveValidMetadata(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	skillGlobs := []string{
		filepath.Join(repoRoot, "library", "skills", "*", "SKILL.md"),
		filepath.Join(repoRoot, "cli", "internal", "skills", "*", "SKILL.md"),
		filepath.Join(repoRoot, "cli", "internal", "registry", "defaultplugin", "library", "skills", "*", "SKILL.md"),
		filepath.Join(repoRoot, ".agents", "skills", "*", "SKILL.md"),
		filepath.Join(repoRoot, ".claude", "skills", "*", "SKILL.md"),
	}

	var matches []string
	for _, pattern := range skillGlobs {
		found, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		matches = append(matches, found...)
	}
	if len(matches) == 0 {
		t.Fatal("no repo SKILL.md files found")
	}

	files, issues := ValidatePaths(matches)
	if len(files) != len(matches) {
		t.Fatalf("validated %d skill files, want %d", len(files), len(matches))
	}
	if len(issues) > 0 {
		t.Fatalf("repo skill validation failed: %v", issues[0])
	}
}

// TestHumanWritingSupportSkillContent verifies the canonical
// human-writing-support skill is embedded in the default plugin and that the
// embedded package installs byte-identical agent and Claude copies into an
// otherwise empty project. Equivalent to the docs-edit-runs-check and
// preserves-technical-structure evals from the prose-quality plan.
func TestHumanWritingSupportSkillContent(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))

	canonicalPath := filepath.Join(repoRoot, "library", "skills", "human-writing-support", "SKILL.md")
	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical human-writing-support skill: %v", err)
	}
	if issues := ValidateContent(canonicalPath, canonical); len(issues) > 0 {
		t.Fatalf("canonical human-writing-support skill validation failed: %v", issues[0])
	}

	embeddedPath := "skills/human-writing-support/SKILL.md"
	embedded, err := fs.ReadFile(defaultplugin.FS(), embeddedPath)
	if err != nil {
		t.Fatalf("read embedded human-writing-support skill: %v", err)
	}
	if string(embedded) != string(canonical) {
		t.Fatalf("embedded %s differs from canonical %s", embeddedPath, canonicalPath)
	}

	embeddedSkills, err := fs.Sub(defaultplugin.FS(), "skills")
	if err != nil {
		t.Fatalf("open embedded default-plugin skills: %v", err)
	}
	projectRoot := t.TempDir()
	if err := Install(embeddedSkills, projectRoot, Options{}); err != nil {
		t.Fatalf("install embedded default-plugin skills: %v", err)
	}

	installedPaths := []string{
		filepath.Join(projectRoot, ".agents", "skills", "human-writing-support", "SKILL.md"),
		filepath.Join(projectRoot, ".claude", "skills", "human-writing-support", "SKILL.md"),
	}

	// Required substrings: workflow command (AC3) and preservation rule
	// surface area (AC4: paths, commands, tables, IDs, acceptance criteria).
	required := []string{
		"ddx doc prose --changed",
		"Preservation Rules",
		"Paths",
		"commands",
		"table",
		"IDs",
		"acceptance criteria",
	}

	for _, p := range installedPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read installed skill %s: %v", p, err)
		}
		if string(data) != string(canonical) {
			t.Errorf("installed skill %s differs from canonical %s", p, canonicalPath)
		}
		text := string(data)
		for _, want := range required {
			if !strings.Contains(text, want) {
				t.Errorf("%s: missing required substring %q", p, want)
			}
		}
	}
}
