package skills

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type skillFrontmatter struct {
	Name         string `yaml:"name"`
	Description  string `yaml:"description"`
	ArgumentHint string `yaml:"argument-hint,omitempty"`
	Skill        any    `yaml:"skill,omitempty"`
}

// Issue represents one skill validation problem.
type Issue struct {
	Path    string
	Message string
}

func (i Issue) Error() string {
	if i.Path == "" {
		return i.Message
	}
	return fmt.Sprintf("%s: %s", i.Path, i.Message)
}

// ValidateContent checks one SKILL.md payload against the DDx skill metadata contract.
func ValidateContent(name string, data []byte) []Issue {
	frontmatter, body, ok := splitSkillFrontmatter(data)
	if !ok {
		return []Issue{{Path: name, Message: "missing YAML frontmatter delimited by ---"}}
	}

	var meta skillFrontmatter
	if err := yaml.Unmarshal(frontmatter, &meta); err != nil {
		return []Issue{{Path: name, Message: fmt.Sprintf("invalid YAML frontmatter: %v", err)}}
	}

	var issues []Issue
	if meta.Skill != nil {
		issues = append(issues, Issue{Path: name, Message: "nested `skill:` frontmatter is not allowed; use top-level `name` and `description`"})
	}
	if strings.TrimSpace(meta.Name) == "" {
		issues = append(issues, Issue{Path: name, Message: "missing required top-level `name` field"})
	}
	if strings.TrimSpace(meta.Description) == "" {
		issues = append(issues, Issue{Path: name, Message: "missing required top-level `description` field"})
	}
	if strings.TrimSpace(string(body)) == "" {
		issues = append(issues, Issue{Path: name, Message: "markdown body is empty"})
	}
	if strings.TrimSpace(meta.ArgumentHint) == "" && bytes.Contains(frontmatter, []byte("argument-hint:")) {
		issues = append(issues, Issue{Path: name, Message: "`argument-hint` must not be empty when present"})
	}
	return issues
}

// ValidatePaths checks one or more files or directories recursively for SKILL.md files.
func ValidatePaths(paths []string) ([]string, []Issue) {
	seen := map[string]bool{}
	var files []string
	var issues []Issue

	for _, root := range paths {
		if strings.TrimSpace(root) == "" {
			continue
		}

		info, err := os.Stat(root)
		if err != nil {
			issues = append(issues, Issue{Path: root, Message: err.Error()})
			continue
		}

		if info.IsDir() {
			err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					issues = append(issues, Issue{Path: path, Message: err.Error()})
					return nil
				}
				if d.IsDir() {
					return nil
				}
				if filepath.Base(path) != "SKILL.md" {
					return nil
				}
				files = appendIfNew(files, seen, path)
				return nil
			})
			if err != nil {
				issues = append(issues, Issue{Path: root, Message: err.Error()})
			}
			continue
		}

		if filepath.Base(root) != "SKILL.md" {
			issues = append(issues, Issue{Path: root, Message: "expected a SKILL.md file or a directory containing skills"})
			continue
		}
		files = appendIfNew(files, seen, root)
	}

	sort.Strings(files)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			issues = append(issues, Issue{Path: path, Message: err.Error()})
			continue
		}
		issues = append(issues, ValidateContent(path, data)...)
	}

	if len(files) == 0 && len(issues) == 0 {
		issues = append(issues, Issue{Message: "no SKILL.md files found"})
	}

	return files, issues
}

func appendIfNew(files []string, seen map[string]bool, path string) []string {
	if seen[path] {
		return files
	}
	seen[path] = true
	return append(files, path)
}

func splitSkillFrontmatter(data []byte) ([]byte, []byte, bool) {
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) < 3 || string(lines[0]) != "---" {
		return nil, nil, false
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if string(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, nil, false
	}

	frontmatter := bytes.Join(lines[1:end], []byte("\n"))
	body := bytes.Join(lines[end+1:], []byte("\n"))
	return frontmatter, body, true
}
