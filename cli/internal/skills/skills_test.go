package skills

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"testing"
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
		filepath.Join(repoRoot, "skills", "*", "SKILL.md"),
		filepath.Join(repoRoot, "cli", "internal", "skills", "*", "SKILL.md"),
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
