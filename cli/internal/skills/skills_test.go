package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRepoSkillsHaveValidMetadata(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	skillGlobs := []string{
		filepath.Join(repoRoot, "library", "skills", "*", "SKILL.md"),
		filepath.Join(repoRoot, "cli", "internal", "registry", "defaultplugin", "library", "skills", "*", "SKILL.md"),
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

// TestHumanWritingSupportSkillContent verifies every tracked copy of the
// human-writing-support skill includes the required workflow command and
// preservation rules. Equivalent to the docs-edit-runs-check and
// preserves-technical-structure evals from the prose-quality plan.
func TestHumanWritingSupportSkillContent(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))

	candidates := []string{
		filepath.Join(repoRoot, ".agents", "skills", "human-writing-support", "SKILL.md"),
		filepath.Join(repoRoot, ".claude", "skills", "human-writing-support", "SKILL.md"),
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

	var checked int
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Errorf("read %s: %v", p, err)
			continue
		}
		checked++
		text := string(data)
		for _, want := range required {
			if !strings.Contains(text, want) {
				t.Errorf("%s: missing required substring %q", p, want)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no human-writing-support skill copies found")
	}
}
