package artifacttypes

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoaderLoad_NewShapeScansWorkflows(t *testing.T) {
	root := t.TempDir()
	writePackageManifest(t, root, `
name: sample-plugin
version: 1.0.0
description: Sample plugin
type: plugin
source: file://sample-plugin
api_version: "1"
`)

	typeDir := filepath.Join(root, "workflows", "phases", "02-design", "artifacts", "solution-design")
	require.NoError(t, os.MkdirAll(typeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(typeDir, "meta.yml"), []byte(`
artifact:
  name: Solution Design
  id: solution-design
  type: document
  prefix: SD
  phase: design
description: |
  Feature-level solution design.
output:
  location: docs/helix/02-design/solution-designs/
  format: markdown
  naming: SD-{feature-number}-{feature-name}.md
prompts:
  generation: prompt.md
template:
  file: template.md
examples:
  - file: example.md
    description: Worked example
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(typeDir, "template.md"), []byte("# template\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(typeDir, "prompt.md"), []byte("# prompt\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(typeDir, "example.md"), []byte("# example\n"), 0o644))

	loader := NewLoader()
	index, err := loader.Load(root)
	require.NoError(t, err)
	require.Len(t, index.Types, 1)

	typ := index.Types[0]
	require.Equal(t, "sample-plugin", typ.Plugin)
	require.Equal(t, "solution-design", typ.TypeID)
	require.Equal(t, "Solution Design", typ.Name)
	require.Equal(t, "Feature-level solution design.", typ.Description)
	require.Equal(t, "SD", typ.Prefix)
	require.Equal(t, "SD-{feature-number}-{feature-name}.md", typ.Pattern)
	require.Equal(t, "design", typ.Phase)
	require.Equal(t, filepath.ToSlash(filepath.Join("workflows", "phases", "02-design", "artifacts", "solution-design", "template.md")), typ.TemplatePath)
	require.Equal(t, filepath.ToSlash(filepath.Join("workflows", "phases", "02-design", "artifacts", "solution-design", "prompt.md")), typ.PromptPath)
	require.Equal(t, filepath.ToSlash(filepath.Join("workflows", "phases", "02-design", "artifacts", "solution-design", "meta.yml")), typ.SourceMetaPath)
	require.Len(t, typ.Examples, 1)
	require.Equal(t, filepath.ToSlash(filepath.Join("workflows", "phases", "02-design", "artifacts", "solution-design", "example.md")), typ.Examples[0].Path)
	require.Equal(t, "Worked example", typ.Examples[0].Description)
}

func TestLoaderLoad_LegacyShapeRespectsArtifactTypeRoots(t *testing.T) {
	root := t.TempDir()
	writePackageManifest(t, root, `
name: legacy-plugin
version: 2.0.0
description: Legacy plugin
type: plugin
source: file://legacy-plugin
api_version: "1"
artifact_type_roots:
  - extras/types
`)

	typeDir := filepath.Join(root, "extras", "types", "adr")
	require.NoError(t, os.MkdirAll(typeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(typeDir, "meta.yml"), []byte(`
artifact:
  name: Architecture Decision Record
  id: adr
  type: document
  phase: design
description: |
  Legacy HELIX metadata shape with extra workflow fields.
id_format:
  prefix: ADR
  pattern: "ADR-[0-9]{3}"
output:
  location: docs/helix/02-design/adrs/
  format: markdown
  naming: ADR-{number}-{decision-name}.md
validation:
  required_sections:
    - context
    - decision
prompts:
  generation: prompt.md
template:
  file: template.md
examples:
  - file: example.md
    description: Legacy example
workflow:
  creation_order: 1
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(typeDir, "template.md"), []byte("# template\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(typeDir, "prompt.md"), []byte("# prompt\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(typeDir, "example.md"), []byte("# example\n"), 0o644))

	loader := NewLoader()
	index, err := loader.Load(root)
	require.NoError(t, err)
	require.Len(t, index.Types, 1)

	typ := index.Types[0]
	require.Equal(t, "legacy-plugin", typ.Plugin)
	require.Equal(t, "adr", typ.TypeID)
	require.Equal(t, "Architecture Decision Record", typ.Name)
	require.Equal(t, "ADR", typ.Prefix)
	require.Equal(t, "ADR-{number}-{decision-name}.md", typ.Pattern)
	require.Equal(t, "design", typ.Phase)
	require.Equal(t, filepath.ToSlash(filepath.Join("extras", "types", "adr", "template.md")), typ.TemplatePath)
	require.Equal(t, filepath.ToSlash(filepath.Join("extras", "types", "adr", "prompt.md")), typ.PromptPath)
	require.Equal(t, filepath.ToSlash(filepath.Join("extras", "types", "adr", "meta.yml")), typ.SourceMetaPath)
}

func TestLoaderLoad_InvalidateOnMetaMtimeChange(t *testing.T) {
	root := t.TempDir()
	writePackageManifest(t, root, `
name: cache-plugin
version: 1.0.0
description: Cache plugin
type: plugin
source: file://cache-plugin
api_version: "1"
`)

	typeDir := filepath.Join(root, "workflows", "phases", "01-frame", "artifacts", "principles")
	require.NoError(t, os.MkdirAll(typeDir, 0o755))
	metaPath := filepath.Join(typeDir, "meta.yml")
	require.NoError(t, os.WriteFile(metaPath, []byte(`
artifact:
  name: Project Principles
  id: principles
  type: document
  prefix: PRD
  phase: frame
description: First version
output:
  location: docs/helix/01-frame/principles.md
  format: markdown
  naming: principles.md
prompts:
  generation: prompt.md
template:
  file: template.md
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(typeDir, "template.md"), []byte("# template\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(typeDir, "prompt.md"), []byte("# prompt\n"), 0o644))

	loader := NewLoader()
	index1, err := loader.Load(root)
	require.NoError(t, err)
	require.Len(t, index1.Types, 1)
	typ1 := index1.Types[0]
	require.Equal(t, "First version", typ1.Description)

	updatedAt := time.Now().Add(2 * time.Minute)
	require.NoError(t, os.WriteFile(metaPath, []byte(`
artifact:
  name: Project Principles
  id: principles
  type: document
  prefix: PRD
  phase: frame
description: Second version
output:
  location: docs/helix/01-frame/principles.md
  format: markdown
  naming: principles.md
prompts:
  generation: prompt.md
template:
  file: template.md
`), 0o644))
	require.NoError(t, os.Chtimes(metaPath, updatedAt, updatedAt))

	index2, err := loader.Load(root)
	require.NoError(t, err)
	require.Len(t, index2.Types, 1)
	typ2 := index2.Types[0]
	require.Equal(t, "Second version", typ2.Description)
	require.NotEqual(t, typ1.Description, typ2.Description)
}

func TestLoaderLoad_RejectsMetaOutsidePluginTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not reliable on windows test runners")
	}

	root := t.TempDir()
	writePackageManifest(t, root, `
name: escape-plugin
version: 1.0.0
description: Escape plugin
type: plugin
source: file://escape-plugin
api_version: "1"
`)

	typeDir := filepath.Join(root, "workflows", "phases", "02-design", "artifacts", "escape")
	require.NoError(t, os.MkdirAll(typeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(typeDir, "template.md"), []byte("# template\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(typeDir, "prompt.md"), []byte("# prompt\n"), 0o644))

	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "meta.yml"), []byte(`
artifact:
  name: Escape Hatch
  id: escape
  type: document
  prefix: ESC
description: Should be rejected
output:
  format: markdown
  naming: escape.md
prompts:
  generation: prompt.md
template:
  file: template.md
`), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "meta.yml"), filepath.Join(typeDir, "meta.yml")))

	loader := NewLoader()
	_, err := loader.Load(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes plugin root")
}

func writePackageManifest(t *testing.T, root, body string) {
	t.Helper()
	content := "name: sample\nversion: 1.0.0\ndescription: sample\ntype: plugin\nsource: file://sample\napi_version: \"1\"\n"
	if strings.TrimSpace(body) != "" {
		content = strings.TrimSpace(body) + "\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "package.yaml"), []byte(content), 0o644))
}
