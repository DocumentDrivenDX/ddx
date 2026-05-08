package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/persona"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPersonaCLI_Lifecycle verifies the new CLI subcommands — new, edit,
// fork, delete — against AC #4 (CLI parity): project-local files get
// created/updated/deleted, and library personas are protected.
func TestPersonaCLI_Lifecycle(t *testing.T) {
	workDir := t.TempDir()
	libraryRoot := filepath.Join(workDir, "library")
	libPersonasDir := filepath.Join(libraryRoot, "personas")
	require.NoError(t, os.MkdirAll(libPersonasDir, 0o755))

	libPersona := `---
name: architect
roles: [architect]
description: Library architect
tags: []
---

# Architect
`
	require.NoError(t, os.WriteFile(filepath.Join(libPersonasDir, "architect.md"), []byte(libPersona), 0o644))

	cfg := "version: \"2.0\"\nlibrary:\n  path: library\n"
	ddxDir := filepath.Join(workDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644))

	bodyFile := filepath.Join(workDir, "body.md")
	bodyContent := `---
name: ignored
roles: [code-reviewer]
description: Our reviewer
tags: []
---

# Our Reviewer
`
	require.NoError(t, os.WriteFile(bodyFile, []byte(bodyContent), 0o644))

	run := func(args ...string) (string, error) {
		rootCmd := getPersonaIntegrationTestRootCommand(workDir)
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		rootCmd.SetArgs(args)
		err := rootCmd.Execute()
		return out.String(), err
	}

	// new: creates a project-local persona.
	out, err := run("persona", "new", "our-reviewer", "--body", bodyFile)
	require.NoError(t, err, out)
	projectFile := filepath.Join(workDir, ".ddx", "personas", "our-reviewer.md")
	assert.FileExists(t, projectFile)

	// edit: updates via --body.
	out, err = run("persona", "edit", "our-reviewer", "--body", bodyFile)
	require.NoError(t, err, out)

	// fork: copies the library persona into the project dir.
	out, err = run("persona", "fork", "architect", "--as", "architect-local")
	require.NoError(t, err, out)
	assert.FileExists(t, filepath.Join(workDir, ".ddx", "personas", "architect-local.md"))

	// edit: library persona is rejected.
	_, err = run("persona", "edit", "architect", "--body", bodyFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "library")

	// delete: library persona is rejected.
	_, err = run("persona", "delete", "architect")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "library")

	// delete: project-local succeeds.
	out, err = run("persona", "delete", "our-reviewer")
	require.NoError(t, err, out)
	_, err = os.Stat(projectFile)
	assert.True(t, os.IsNotExist(err))
}

// TestPersonaCLI_GraphQLParity covers AC #4: a persona created via `ddx
// persona new` is visible to the persona package's loader, proving that
// the CLI and GraphQL paths share the same storage.
func TestPersonaCLI_GraphQLParity(t *testing.T) {
	workDir := t.TempDir()
	libraryRoot := filepath.Join(workDir, "library")
	libPersonasDir := filepath.Join(libraryRoot, "personas")
	require.NoError(t, os.MkdirAll(libPersonasDir, 0o755))
	libPersona := "---\nname: architect\nroles: [architect]\ndescription: Library architect\ntags: []\n---\n\n# Architect\n"
	require.NoError(t, os.WriteFile(filepath.Join(libPersonasDir, "architect.md"), []byte(libPersona), 0o644))

	cfg := "version: \"2.0\"\nlibrary:\n  path: library\n"
	ddxDir := filepath.Join(workDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644))

	bodyFile := filepath.Join(workDir, "body.md")
	body := "---\nname: ignored\nroles: [code-reviewer]\ndescription: Shared reviewer\ntags: []\n---\n\n# Shared\n"
	require.NoError(t, os.WriteFile(bodyFile, []byte(body), 0o644))

	rootCmd := getPersonaIntegrationTestRootCommand(workDir)
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"persona", "new", "shared-reviewer", "--body", bodyFile})
	require.NoError(t, rootCmd.Execute(), buf.String())

	loader := persona.NewPersonaLoader(workDir)
	all, err := loader.ListPersonas()
	require.NoError(t, err)

	names := map[string]string{}
	for _, p := range all {
		names[p.Name] = p.Source
	}
	assert.Equal(t, persona.SourceProject, names["shared-reviewer"],
		"CLI-created persona should show up via the persona loader as a project persona")
	assert.Equal(t, persona.SourceLibrary, names["architect"],
		"library architect still visible with library source")
}

func TestPersonaCLI_LoadUsesProjectOverride(t *testing.T) {
	workDir := t.TempDir()
	libraryRoot := filepath.Join(workDir, "library")
	libPersonasDir := filepath.Join(libraryRoot, "personas")
	projectPersonasDir := filepath.Join(workDir, ".ddx", "personas")
	require.NoError(t, os.MkdirAll(libPersonasDir, 0o755))
	require.NoError(t, os.MkdirAll(projectPersonasDir, 0o755))

	cfg := `version: "2.0"
library:
  path: library
persona_bindings:
  code-reviewer: code-reviewer
`
	require.NoError(t, os.WriteFile(filepath.Join(workDir, ".ddx", "config.yaml"), []byte(cfg), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(libPersonasDir, "code-reviewer.md"), []byte(`---
name: code-reviewer
roles: [code-reviewer]
description: Library reviewer
tags: []
---

# Library Reviewer
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectPersonasDir, "code-reviewer.md"), []byte(`---
name: code-reviewer
roles: [code-reviewer]
description: Project reviewer
tags: []
---

# Project Reviewer
`), 0o644))

	rootCmd := getPersonaIntegrationTestRootCommand(workDir)
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"persona", "load"})
	require.NoError(t, rootCmd.Execute(), buf.String())

	claude, err := os.ReadFile(filepath.Join(workDir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(claude), "Project Reviewer")
	assert.NotContains(t, string(claude), "Library Reviewer")
}
