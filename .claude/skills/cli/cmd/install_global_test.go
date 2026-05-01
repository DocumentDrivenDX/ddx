package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each test isolates HOME into a temp dir so the install lands in the
// fixture, not the running user's home.
func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestInstallGlobalExtractsSkillsAndChainsSymlinks(t *testing.T) {
	home := isolatedHome(t)

	factory := NewCommandFactory(t.TempDir())
	var out bytes.Buffer
	require.NoError(t, factory.installGlobal(false, &out))

	// ~/.ddx/skills/ddx/SKILL.md is a real file (not a symlink) with non-empty content.
	skillPath := filepath.Join(home, ".ddx", "skills", "ddx", "SKILL.md")
	info, err := os.Lstat(skillPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0), info.Mode()&os.ModeSymlink, "SKILL.md must be a real file, not a symlink")
	body, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	assert.Greater(t, len(body), 0, "embedded SKILL.md must not be empty")

	// ~/.agents/skills/ddx must be a symlink to ../../.ddx/skills/ddx.
	agentsLink := filepath.Join(home, ".agents", "skills", "ddx")
	agentsTarget, err := os.Readlink(agentsLink)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("..", "..", ".ddx", "skills", "ddx"), agentsTarget)

	// ~/.claude/skills/ddx must be a symlink to ../../.agents/skills/ddx.
	claudeLink := filepath.Join(home, ".claude", "skills", "ddx")
	claudeTarget, err := os.Readlink(claudeLink)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("..", "..", ".agents", "skills", "ddx"), claudeTarget)

	// Chain must resolve end-to-end — Claude runtimes traversing
	// claudeLink should land on the real SKILL.md.
	resolved, err := filepath.EvalSymlinks(filepath.Join(claudeLink, "SKILL.md"))
	require.NoError(t, err)
	assert.FileExists(t, resolved)
}

func TestInstallGlobalIsIdempotent(t *testing.T) {
	home := isolatedHome(t)
	factory := NewCommandFactory(t.TempDir())

	var out1 bytes.Buffer
	require.NoError(t, factory.installGlobal(false, &out1))
	skillPath := filepath.Join(home, ".ddx", "skills", "ddx", "SKILL.md")
	first, err := os.ReadFile(skillPath)
	require.NoError(t, err)

	// User edits the extracted skill. Without --force, a second run must
	// preserve the edit.
	edited := append(first, []byte("\n\nlocal edit\n")...)
	require.NoError(t, os.WriteFile(skillPath, edited, 0o644))

	var out2 bytes.Buffer
	require.NoError(t, factory.installGlobal(false, &out2))
	after, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	assert.Equal(t, edited, after, "second install (no --force) must preserve user edits")

	// --force restores the pristine content.
	var out3 bytes.Buffer
	require.NoError(t, factory.installGlobal(true, &out3))
	restored, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	assert.Equal(t, first, restored, "--force must overwrite user edits with embedded content")
}

func TestInstallGlobalReplacesStaleSymlinkTarget(t *testing.T) {
	home := isolatedHome(t)

	// Pre-seed a stale symlink pointing somewhere irrelevant. Global
	// install must replace it rather than fail or leave it dangling.
	agentsDir := filepath.Join(home, ".agents", "skills")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	stalePath := filepath.Join(agentsDir, "ddx")
	require.NoError(t, os.Symlink("/nonexistent/old-target", stalePath))

	factory := NewCommandFactory(t.TempDir())
	var out bytes.Buffer
	require.NoError(t, factory.installGlobal(false, &out))

	got, err := os.Readlink(stalePath)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("..", "..", ".ddx", "skills", "ddx"), got, "stale symlink must be replaced with the canonical relative target")
}

func TestInstallGlobalRefusesToClobberRealDirectoryAtAgentsLink(t *testing.T) {
	home := isolatedHome(t)

	// Pre-existing REAL directory where the symlink needs to live —
	// likely user data or a project-local install bleed-through. The
	// installer must refuse rather than silently deleting it.
	agentsPath := filepath.Join(home, ".agents", "skills", "ddx")
	require.NoError(t, os.MkdirAll(agentsPath, 0o755))
	guarded := filepath.Join(agentsPath, "user-data.md")
	require.NoError(t, os.WriteFile(guarded, []byte("do not delete"), 0o644))

	factory := NewCommandFactory(t.TempDir())
	var out bytes.Buffer
	err := factory.installGlobal(false, &out)
	require.Error(t, err, "global install must refuse to replace a real directory")
	assert.Contains(t, err.Error(), "real directory")

	// User data must still be on disk.
	body, err := os.ReadFile(guarded)
	require.NoError(t, err)
	assert.Equal(t, "do not delete", string(body))
}

func TestInstallGlobalRefusesToClobberRealFileAtAgentsLink(t *testing.T) {
	home := isolatedHome(t)

	agentsParent := filepath.Join(home, ".agents", "skills")
	require.NoError(t, os.MkdirAll(agentsParent, 0o755))
	realFile := filepath.Join(agentsParent, "ddx")
	require.NoError(t, os.WriteFile(realFile, []byte("someone left a file here"), 0o644))

	factory := NewCommandFactory(t.TempDir())
	var out bytes.Buffer
	err := factory.installGlobal(false, &out)
	require.Error(t, err, "global install must refuse to replace a regular file")
	assert.Contains(t, err.Error(), "regular file")
}

func TestReplaceRelativeSymlinkCreatesWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "child")
	require.NoError(t, replaceRelativeSymlink(link, "../target"))

	got, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, "../target", got)
}

func TestReplaceRelativeSymlinkReplacesExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "child")
	require.NoError(t, os.Symlink("old-target", link))

	require.NoError(t, replaceRelativeSymlink(link, "new-target"))
	got, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, "new-target", got)
}
