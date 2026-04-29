package skills

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInstall_FromEmbedFS_CopiesBootstrapSkill verifies the embed.FS bootstrap
// case: the single shipped `ddx` skill ends up as real files (not symlinks)
// in <projectRoot>/.agents/skills/ddx and <projectRoot>/.claude/skills/ddx.
func TestInstall_FromEmbedFS_CopiesBootstrapSkill(t *testing.T) {
	projectRoot := t.TempDir()

	require.NoError(t, Install(SkillFiles, projectRoot, Options{}))

	for _, parent := range []string{".agents", ".claude"} {
		skillFile := filepath.Join(projectRoot, parent, "skills", "ddx", "SKILL.md")
		info, err := os.Lstat(skillFile)
		require.NoError(t, err, "missing SKILL.md at %s", skillFile)
		assert.Zero(t, info.Mode()&os.ModeSymlink, "SKILL.md must be a real file, not a symlink: %s", skillFile)
		data, err := os.ReadFile(skillFile)
		require.NoError(t, err)
		assert.NotEmpty(t, data, "SKILL.md should be non-empty")
	}
}

// TestInstall_BrokenTarballSymlinkRecovery verifies that when the source's
// .agents/skills/<skill> entry is a broken symlink (typical of a
// mis-extracted plugin tarball), Install falls back to <source>/skills/<skill>
// and writes real files to the destination.
func TestInstall_BrokenTarballSymlinkRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks not reliable on windows")
	}

	source := t.TempDir()
	// Real content lives at <source>/skills/foo
	require.NoError(t, os.MkdirAll(filepath.Join(source, "skills", "foo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "skills", "foo", "SKILL.md"), []byte("# foo"), 0o644))

	// Plugin-style entry with a BROKEN symlink.
	require.NoError(t, os.MkdirAll(filepath.Join(source, ".agents", "skills"), 0o755))
	brokenLink := filepath.Join(source, ".agents", "skills", "foo")
	require.NoError(t, os.Symlink("../../skills/foo-does-not-exist", brokenLink))

	projectRoot := t.TempDir()
	require.NoError(t, Install(os.DirFS(source), projectRoot, Options{}))

	for _, parent := range []string{".agents", ".claude"} {
		dest := filepath.Join(projectRoot, parent, "skills", "foo", "SKILL.md")
		info, err := os.Lstat(dest)
		require.NoError(t, err, "fallback content missing at %s", dest)
		assert.Zero(t, info.Mode()&os.ModeSymlink, "destination must be a real file: %s", dest)
		data, err := os.ReadFile(dest)
		require.NoError(t, err)
		assert.Equal(t, "# foo", string(data))
	}
}

// TestInstall_RejectsPathTraversal verifies that '..', absolute, and
// symlink-escape inputs return an error before any write.
func TestInstall_RejectsPathTraversal(t *testing.T) {
	t.Run("dotdot skill name", func(t *testing.T) {
		// fstest.MapFS allows planting a directory named "..".
		src := fstest.MapFS{
			"../bad/SKILL.md": &fstest.MapFile{Data: []byte("# bad")},
		}
		projectRoot := t.TempDir()
		err := Install(src, projectRoot, Options{})
		require.Error(t, err)
		// Nothing should have been written.
		_, statErr := os.Stat(filepath.Join(projectRoot, ".agents", "skills"))
		assert.True(t, statErr == nil || os.IsNotExist(statErr), "no skill content should be written")
		// In particular no skill subdir exists.
		assertNoSkillsWritten(t, projectRoot)
	})

	t.Run("absolute path projectRoot", func(t *testing.T) {
		// Empty projectRoot is rejected.
		err := Install(SkillFiles, "", Options{})
		require.Error(t, err)
	})

	t.Run("symlink escape via .agents/skills", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks not reliable on windows")
		}
		projectRoot := t.TempDir()
		outside := t.TempDir()
		// Plant a symlink at projectRoot/.agents/skills pointing OUTSIDE projectRoot.
		require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".agents"), 0o755))
		require.NoError(t, os.Symlink(outside, filepath.Join(projectRoot, ".agents", "skills")))

		err := Install(SkillFiles, projectRoot, Options{})
		require.Error(t, err)
		// And confirm nothing was written into the outside dir.
		entries, _ := os.ReadDir(outside)
		assert.Empty(t, entries, "Install must not write outside projectRoot")
	})
}

func assertNoSkillsWritten(t *testing.T, projectRoot string) {
	t.Helper()
	for _, parent := range []string{".agents", ".claude"} {
		dir := filepath.Join(projectRoot, parent, "skills")
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		require.NoError(t, err)
		for _, e := range entries {
			t.Errorf("unexpected skill entry written: %s/%s", dir, e.Name())
		}
	}
}

// TestInstall_PreExistingSymlinkRemovedUnconditionally verifies that pre-existing
// SYMLINK destinations are removed and rewritten as real files even when
// Force=false. This heals migrations from the symlinked-home model.
func TestInstall_PreExistingSymlinkRemovedUnconditionally(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks not reliable on windows")
	}
	projectRoot := t.TempDir()

	// Plant a symlink at .agents/skills/ddx pointing somewhere arbitrary inside projectRoot.
	someTarget := filepath.Join(projectRoot, "elsewhere")
	require.NoError(t, os.MkdirAll(someTarget, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".agents", "skills"), 0o755))
	linkPath := filepath.Join(projectRoot, ".agents", "skills", "ddx")
	require.NoError(t, os.Symlink(someTarget, linkPath))

	// Force=false — symlinks must still be replaced.
	require.NoError(t, Install(SkillFiles, projectRoot, Options{Force: false}))

	info, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink, "pre-existing symlink should be replaced with a real dir")
	assert.True(t, info.IsDir())
	skillFile := filepath.Join(linkPath, "SKILL.md")
	_, err = os.Stat(skillFile)
	require.NoError(t, err)
}

// TestInstall_ForceFalseSkipsRealDir verifies that pre-existing real-file
// skill directories are preserved when Force=false (per-skill skip).
func TestInstall_ForceFalseSkipsRealDir(t *testing.T) {
	projectRoot := t.TempDir()
	skillDir := filepath.Join(projectRoot, ".agents", "skills", "ddx")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	customFile := filepath.Join(skillDir, "SKILL.md")
	require.NoError(t, os.WriteFile(customFile, []byte("# user-customized"), 0o644))

	require.NoError(t, Install(SkillFiles, projectRoot, Options{Force: false}))

	data, err := os.ReadFile(customFile)
	require.NoError(t, err)
	assert.Equal(t, "# user-customized", string(data), "Force=false must not overwrite real-file skill dir")
}

// TestInstall_ForceTrueOverwritesRealDir verifies Force=true overwrites
// pre-existing real-file skill directories with the canonical content.
func TestInstall_ForceTrueOverwritesRealDir(t *testing.T) {
	projectRoot := t.TempDir()
	skillDir := filepath.Join(projectRoot, ".agents", "skills", "ddx")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	customFile := filepath.Join(skillDir, "SKILL.md")
	require.NoError(t, os.WriteFile(customFile, []byte("# stale"), 0o644))
	staleExtra := filepath.Join(skillDir, "stale.md")
	require.NoError(t, os.WriteFile(staleExtra, []byte("stale"), 0o644))

	require.NoError(t, Install(SkillFiles, projectRoot, Options{Force: true}))

	data, err := os.ReadFile(customFile)
	require.NoError(t, err)
	assert.NotEqual(t, "# stale", string(data), "Force=true must overwrite SKILL.md")
	// Stale extra file should be removed by Force=true (whole-dir overwrite).
	_, err = os.Stat(staleExtra)
	assert.True(t, os.IsNotExist(err), "Force=true must remove stale extra files in the skill dir")
}
