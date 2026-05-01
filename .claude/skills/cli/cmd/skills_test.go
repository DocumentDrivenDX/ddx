package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillsCheckDefaultPaths(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "skills", "plugin-foo", "SKILL.md"), `---
name: plugin-foo
description: Valid test skill.
---

# Plugin Foo
`)

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "skills", "check")
	require.NoError(t, err)
	assert.Contains(t, output, "validated 1 skill files")
}

func TestSkillsCheckExplicitPath(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "plugin", "skills", "plugin-bar", "SKILL.md"), `---
name: plugin-bar
description: Explicit path skill.
argument-hint: "[scope]"
---

# Plugin Bar
`)

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "skills", "check", filepath.Join(dir, "plugin", "skills"))
	require.NoError(t, err)
	assert.Contains(t, output, "validated 1 skill files")
}

func TestSkillsCheckRejectsNestedSkillFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "skills", "broken-skill", "SKILL.md"), `---
skill:
  name: broken-skill
  description: Nested metadata is invalid.
---

# Broken
`)

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(rootCmd, "skills", "check", filepath.Join(dir, "skills"))
	require.Error(t, err)

	exitErr, ok := err.(*ExitError)
	require.True(t, ok)
	assert.Equal(t, ExitCodeGeneralError, exitErr.Code)
	assert.Contains(t, exitErr.Message, "skill validation failed")
}

func writeSkillFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
