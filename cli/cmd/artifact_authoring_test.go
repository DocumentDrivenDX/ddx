package cmd

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixtureSlideXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld><p:spTree><p:sp><p:txBody>
    <a:p><a:r><a:t>Prebill</a:t></a:r><a:r><a:t> review</a:t></a:r></a:p>
    <a:p><a:r><a:t>Tier one firms only</a:t></a:r></a:p>
  </p:txBody></p:sp></p:spTree></p:cSld>
</p:sld>`

// writeFixtureDeck writes a one-slide .pptx so the check-in path can be
// exercised end to end without a committed binary fixture.
func writeFixtureDeck(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	file, err := os.Create(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()

	archive := zip.NewWriter(file)
	writer, err := archive.Create("ppt/slides/slide1.xml")
	require.NoError(t, err)
	_, err = writer.Write([]byte(fixtureSlideXML))
	require.NoError(t, err)
	require.NoError(t, archive.Close())
}

// writeExternalToolArtifact writes an artifact document with an external-tool
// authoring block and returns the path to its Markdown file.
func writeExternalToolArtifact(t *testing.T, workingDir, artifactID, export string) string {
	t.Helper()

	front := "---\nddx:\n  id: " + artifactID + "\n  type: solution-design\n  status: draft\n" +
		"  authoring:\n    home: external-tool\n    state: checked-out\n    tool: google-slides\n" +
		"    origin: https://docs.google.com/presentation/d/1AbC/edit\n"
	if export != "" {
		front += "    export: " + export + "\n"
	}
	front += "---\n# " + artifactID + "\n\nIdentity and description only; the deck is in the tool.\n"

	path := filepath.Join(workingDir, "docs", artifactID+".md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(front), 0o644))
	return path
}

func TestArtifactAuthoringSubcommandsRegistered(t *testing.T) {
	rootCmd := NewCommandFactory(t.TempDir()).NewRootCommand()

	out, err := executeCommand(rootCmd, "artifact", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "check-in")
	assert.Contains(t, out, "check-out")
}

func TestArtifactCheckInLandsExportInBody(t *testing.T) {
	workingDir := t.TempDir()
	export := "docs/assets/SD-004.pptx"
	docPath := writeExternalToolArtifact(t, workingDir, "SD-004", export)
	writeFixtureDeck(t, filepath.Join(workingDir, filepath.FromSlash(export)))

	rootCmd := NewCommandFactory(workingDir).NewRootCommand()
	out, err := executeCommand(rootCmd, "artifact", "check-in", "SD-004")
	require.NoError(t, err)
	assert.Contains(t, out, "checked in SD-004 from docs/assets/SD-004.pptx")

	updated, err := os.ReadFile(docPath)
	require.NoError(t, err)
	text := string(updated)

	// Frontmatter is preserved; only the authoring state and digest change.
	assert.Contains(t, text, "id: SD-004")
	assert.Contains(t, text, "type: solution-design")
	assert.Contains(t, text, "tool: google-slides")
	assert.Contains(t, text, "origin: https://docs.google.com/presentation/d/1AbC/edit")
	assert.Contains(t, text, "export: docs/assets/SD-004.pptx")
	assert.Contains(t, text, "state: checked-in")
	assert.NotContains(t, text, "state: checked-out")

	// export_sha256 is the digest of the export file, so "the body matches" is
	// checkable after the fact.
	raw, err := os.ReadFile(filepath.Join(workingDir, filepath.FromSlash(export)))
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	assert.Contains(t, text, "export_sha256: "+hex.EncodeToString(sum[:]))

	// The body is the extracted deck, and the placeholder body is gone.
	assert.Contains(t, text, "## Slide 1")
	assert.Contains(t, text, "- Prebill review")
	assert.Contains(t, text, "- Tier one firms only")
	assert.NotContains(t, text, "Identity and description only")
}

// The round trip is the contract: check-in lands content, check-out marks it
// undependable without discarding it, and a second check-in of the same export
// reproduces the first byte for byte.
func TestArtifactCheckInCheckOutRoundTrip(t *testing.T) {
	workingDir := t.TempDir()
	export := "docs/assets/SD-004.pptx"
	docPath := writeExternalToolArtifact(t, workingDir, "SD-004", export)
	writeFixtureDeck(t, filepath.Join(workingDir, filepath.FromSlash(export)))

	rootCmd := NewCommandFactory(workingDir).NewRootCommand()
	_, err := executeCommand(rootCmd, "artifact", "check-in", "SD-004")
	require.NoError(t, err)
	afterFirstCheckIn, err := os.ReadFile(docPath)
	require.NoError(t, err)

	rootCmd = NewCommandFactory(workingDir).NewRootCommand()
	out, err := executeCommand(rootCmd, "artifact", "check-out", "SD-004")
	require.NoError(t, err)
	assert.Contains(t, out, "checked out SD-004")

	afterCheckOut, err := os.ReadFile(docPath)
	require.NoError(t, err)
	checkedOutText := string(afterCheckOut)
	assert.Contains(t, checkedOutText, "state: checked-out")
	// Checking out never deletes content the repository already has.
	assert.Contains(t, checkedOutText, "- Prebill review")
	assert.Contains(t, checkedOutText, "export: docs/assets/SD-004.pptx")

	rootCmd = NewCommandFactory(workingDir).NewRootCommand()
	_, err = executeCommand(rootCmd, "artifact", "check-in", "SD-004")
	require.NoError(t, err)
	afterSecondCheckIn, err := os.ReadFile(docPath)
	require.NoError(t, err)
	assert.Equal(t, string(afterFirstCheckIn), string(afterSecondCheckIn),
		"re-checking in an unchanged export must be byte-identical")
}

func TestArtifactCheckInJSONOutput(t *testing.T) {
	workingDir := t.TempDir()
	export := "docs/assets/SD-004.pptx"
	writeExternalToolArtifact(t, workingDir, "SD-004", export)
	writeFixtureDeck(t, filepath.Join(workingDir, filepath.FromSlash(export)))

	rootCmd := NewCommandFactory(workingDir).NewRootCommand()
	out, err := executeCommand(rootCmd, "artifact", "check-in", "SD-004", "--json")
	require.NoError(t, err)

	var result artifactAuthoringResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, "SD-004", result.ID)
	assert.Equal(t, "checked-in", result.State)
	assert.Equal(t, "google-slides", result.Tool)
	assert.Equal(t, export, result.Export)
	assert.Equal(t, filepath.ToSlash(filepath.Join("docs", "SD-004.md")), filepath.ToSlash(result.Path))
	assert.Len(t, result.ExportSHA256, 64)
}

func TestArtifactCheckInRejectsRepoAuthoredArtifact(t *testing.T) {
	workingDir := t.TempDir()
	path := filepath.Join(workingDir, "docs", "TD-022.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path,
		[]byte("---\nddx:\n  id: TD-022\n  authoring:\n    home: repo\n---\n# TD-022\n"), 0o644))

	rootCmd := NewCommandFactory(workingDir).NewRootCommand()
	_, err := executeCommand(rootCmd, "artifact", "check-in", "TD-022")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `artifact "TD-022" has ddx.authoring.home: repo`)
	assert.Contains(t, err.Error(), "external-tool")
}

func TestArtifactCheckOutRejectsArtifactWithoutAuthoringBlock(t *testing.T) {
	workingDir := t.TempDir()
	writeArtifactDoc(t, workingDir, "ART-001")

	rootCmd := NewCommandFactory(workingDir).NewRootCommand()
	_, err := executeCommand(rootCmd, "artifact", "check-out", "ART-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declares no ddx.authoring.home")
}

func TestArtifactCheckInRequiresExport(t *testing.T) {
	workingDir := t.TempDir()
	writeExternalToolArtifact(t, workingDir, "SD-004", "")

	rootCmd := NewCommandFactory(workingDir).NewRootCommand()
	_, err := executeCommand(rootCmd, "artifact", "check-in", "SD-004")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declares no ddx.authoring.export")
	assert.Contains(t, err.Error(), "google-slides")
}

func TestArtifactCheckInReportsMissingExportFile(t *testing.T) {
	workingDir := t.TempDir()
	writeExternalToolArtifact(t, workingDir, "SD-004", "docs/assets/SD-004.pptx")

	rootCmd := NewCommandFactory(workingDir).NewRootCommand()
	_, err := executeCommand(rootCmd, "artifact", "check-in", "SD-004")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `declares ddx.authoring.export "docs/assets/SD-004.pptx"`)
	assert.Contains(t, err.Error(), "cannot be read")
}

func TestArtifactCheckInReportsUnsupportedExportFormat(t *testing.T) {
	workingDir := t.TempDir()
	export := "docs/assets/SD-004.key"
	writeExternalToolArtifact(t, workingDir, "SD-004", export)
	exportPath := filepath.Join(workingDir, filepath.FromSlash(export))
	require.NoError(t, os.MkdirAll(filepath.Dir(exportPath), 0o755))
	require.NoError(t, os.WriteFile(exportPath, []byte("keynote"), 0o644))

	rootCmd := NewCommandFactory(workingDir).NewRootCommand()
	_, err := executeCommand(rootCmd, "artifact", "check-in", "SD-004")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no converter for .key files")
}

// check-in copies whatever export points at into a committed file, so an
// export that is absolute or escapes the repository root is refused.
func TestArtifactCheckInRejectsExportOutsideRepository(t *testing.T) {
	for name, export := range map[string]string{
		"escaping": "../outside/SD-004.pptx",
		"absolute": "/etc/SD-004.pptx",
	} {
		t.Run(name, func(t *testing.T) {
			workingDir := t.TempDir()
			writeExternalToolArtifact(t, workingDir, "SD-004", export)

			rootCmd := NewCommandFactory(workingDir).NewRootCommand()
			_, err := executeCommand(rootCmd, "artifact", "check-in", "SD-004")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "repository")
		})
	}
}

func TestArtifactCheckInUnknownArtifact(t *testing.T) {
	rootCmd := NewCommandFactory(t.TempDir()).NewRootCommand()

	_, err := executeCommand(rootCmd, "artifact", "check-in", "SD-999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `artifact "SD-999" not found in document graph`)
}

func TestArtifactCheckOutBeforeFirstCheckInNeedsNoExport(t *testing.T) {
	workingDir := t.TempDir()
	docPath := writeExternalToolArtifact(t, workingDir, "SD-004", "")

	rootCmd := NewCommandFactory(workingDir).NewRootCommand()
	out, err := executeCommand(rootCmd, "artifact", "check-out", "SD-004")
	require.NoError(t, err)
	assert.Contains(t, out, "author at https://docs.google.com/presentation/d/1AbC/edit")

	updated, err := os.ReadFile(docPath)
	require.NoError(t, err)
	assert.Contains(t, string(updated), "state: checked-out")
	assert.Contains(t, string(updated), "Identity and description only")
	assert.False(t, strings.Contains(string(updated), "export:"))
}
