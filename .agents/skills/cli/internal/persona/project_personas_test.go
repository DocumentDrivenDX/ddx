package persona

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoader_ProjectOverridesLibrary covers the precedence rule in AC #2:
// when a project persona shares a name with a library persona, the project
// persona wins for list/load/find operations.
func TestLoader_ProjectOverridesLibrary(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	libDir := filepath.Join(tmp, "library")
	projDir := filepath.Join(tmp, "project")
	require.NoError(t, os.MkdirAll(libDir, 0o755))
	require.NoError(t, os.MkdirAll(projDir, 0o755))

	libBody := `---
name: code-reviewer
roles: [code-reviewer]
description: Library reviewer
tags: []
---

# Library Reviewer
`
	projBody := `---
name: code-reviewer
roles: [code-reviewer]
description: Project reviewer
tags: []
---

# Project Reviewer
`
	require.NoError(t, os.WriteFile(filepath.Join(libDir, "code-reviewer.md"), []byte(libBody), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "code-reviewer.md"), []byte(projBody), 0o644))

	loader := NewPersonaLoaderWithDirs(libDir, projDir)

	loaded, err := loader.LoadPersona("code-reviewer")
	require.NoError(t, err)
	assert.Equal(t, SourceProject, loaded.Source, "project persona should win on name collision")
	assert.Equal(t, "Project reviewer", loaded.Description)

	all, err := loader.ListPersonas()
	require.NoError(t, err)
	require.Len(t, all, 1, "duplicate names collapse; project wins")
	assert.Equal(t, SourceProject, all[0].Source)
}

// TestLoader_SourceBadges covers AC #2: list distinguishes library vs
// project personas with correct source labels.
func TestLoader_SourceBadges(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	libDir := filepath.Join(tmp, "library")
	projDir := filepath.Join(tmp, "project")
	require.NoError(t, os.MkdirAll(libDir, 0o755))
	require.NoError(t, os.MkdirAll(projDir, 0o755))

	writeFixture(t, libDir, "architect", "Library architect")
	writeFixture(t, projDir, "our-reviewer", "Project reviewer")

	loader := NewPersonaLoaderWithDirs(libDir, projDir)
	all, err := loader.ListPersonas()
	require.NoError(t, err)

	got := map[string]string{}
	for _, p := range all {
		got[p.Name] = p.Source
	}
	assert.Equal(t, SourceLibrary, got["architect"])
	assert.Equal(t, SourceProject, got["our-reviewer"])
}

// TestProjectPersonaWriter covers AC #2/#3: project-local CRUD succeeds,
// library personas are read-only, and fork produces a project file.
func TestProjectPersonaWriter(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	libPersonasDir := filepath.Join(workingDir, "lib", "personas")
	require.NoError(t, os.MkdirAll(libPersonasDir, 0o755))
	writeFixture(t, libPersonasDir, "architect", "Library architect")

	// Point resolveLibraryPersonasDir at libDir by synthesizing a config
	// file that the loader will pick up.
	configPath := filepath.Join(workingDir, ".ddx", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(
		"version: \"2.0\"\nlibrary:\n  path: lib\n"), 0o644))

	writer := NewProjectPersonaWriter(workingDir)

	// Create a project-local persona.
	body := `---
name: ignored
roles: [code-reviewer]
description: Our reviewer
tags: []
---

# Our Reviewer
`
	created, err := writer.Create("our-reviewer", body)
	require.NoError(t, err)
	assert.Equal(t, SourceProject, created.Source)
	assert.Equal(t, "our-reviewer", created.Name, "name is forced to match filename")

	// Library personas cannot be updated or deleted directly.
	_, err = writer.Update("architect", body)
	require.Error(t, err)
	perr, ok := err.(*PersonaError)
	require.True(t, ok)
	assert.Equal(t, ErrorReadOnlyLibrary, perr.Type)

	err = writer.Delete("architect")
	require.Error(t, err)
	perr, ok = err.(*PersonaError)
	require.True(t, ok)
	assert.Equal(t, ErrorReadOnlyLibrary, perr.Type)

	// Fork copies the library persona into the project dir.
	forked, err := writer.Fork("architect", "architect-local")
	require.NoError(t, err)
	assert.Equal(t, SourceProject, forked.Source)
	assert.Equal(t, "architect-local", forked.Name)

	// Update then delete the project persona succeeds.
	_, err = writer.Update("our-reviewer", body)
	require.NoError(t, err)
	require.NoError(t, writer.Delete("our-reviewer"))
	_, err = os.Stat(filepath.Join(workingDir, ".ddx", "personas", "our-reviewer.md"))
	assert.True(t, os.IsNotExist(err))
}

func writeFixture(t *testing.T, dir, name, description string) {
	t.Helper()
	body := "---\nname: " + name + "\nroles: [code-reviewer]\ndescription: " + description + "\ntags: []\n---\n\n# " + name + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644))
}
