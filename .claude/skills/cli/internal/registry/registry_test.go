package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltinRegistry(t *testing.T) {
	r := BuiltinRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}

	pkg, err := r.Find("helix")
	if err != nil {
		t.Fatalf("expected helix package: %v", err)
	}

	if pkg.Name != "helix" {
		t.Errorf("expected name=helix, got %q", pkg.Name)
	}
	if pkg.Type != PackageTypeWorkflow {
		t.Errorf("expected type=workflow, got %q", pkg.Type)
	}
	if pkg.Version == "" {
		t.Error("expected non-empty version")
	}
	if pkg.Description == "" {
		t.Error("expected non-empty description")
	}
	if pkg.Source == "" {
		t.Error("expected non-empty source")
	}
	if pkg.Install.Skills == nil {
		t.Error("expected install.skills to be set")
	}
}

func TestBuiltinRegistry_DDxPackage(t *testing.T) {
	r := BuiltinRegistry()

	pkg, err := r.Find("ddx")
	if err != nil {
		t.Fatalf("expected ddx package: %v", err)
	}

	if pkg.Name != "ddx" {
		t.Errorf("expected name=ddx, got %q", pkg.Name)
	}
	if pkg.Type != PackageTypePlugin {
		t.Errorf("expected type=plugin, got %q", pkg.Type)
	}
	if pkg.Install.Root == nil {
		t.Fatal("expected install.root to be set")
	}
	if pkg.Install.Root.Source != "library" {
		t.Errorf("expected root source=library, got %q", pkg.Install.Root.Source)
	}
	if pkg.Install.Root.Target != ".ddx/plugins/ddx" {
		t.Errorf("expected root target=.ddx/plugins/ddx, got %q", pkg.Install.Root.Target)
	}
	// ddx plugin ships skills to project-local and global skill dirs.
	if len(pkg.Install.Skills) != 4 {
		t.Errorf("expected 4 skill mappings, got %d", len(pkg.Install.Skills))
	}
	if pkg.Install.Scripts != nil {
		t.Error("expected no scripts")
	}
}

func TestFind(t *testing.T) {
	r := BuiltinRegistry()

	pkg, err := r.Find("helix")
	if err != nil {
		t.Fatalf("expected to find helix: %v", err)
	}
	if pkg.Name != "helix" {
		t.Errorf("expected helix, got %q", pkg.Name)
	}

	ddxPkg, err := r.Find("ddx")
	if err != nil {
		t.Fatalf("expected to find ddx: %v", err)
	}
	if ddxPkg.Name != "ddx" {
		t.Errorf("expected ddx, got %q", ddxPkg.Name)
	}

	_, err = r.Find("nonexistent-package")
	if err == nil {
		t.Error("expected error for nonexistent package")
	}
	if !strings.Contains(err.Error(), "nonexistent-package") {
		t.Errorf("expected error to mention package name, got: %v", err)
	}
}

func TestSearch(t *testing.T) {
	r := BuiltinRegistry()

	// Match by name
	results := r.Search("helix")
	if len(results) == 0 {
		t.Error("expected results for 'helix' name search")
	}
	found := false
	for _, p := range results {
		if p.Name == "helix" {
			found = true
		}
	}
	if !found {
		t.Error("expected helix in name search results")
	}

	// Match by description
	results = r.Search("workflow")
	if len(results) == 0 {
		t.Error("expected results for 'workflow' description/type search")
	}

	// Match by keyword
	results = r.Search("methodology")
	if len(results) == 0 {
		t.Error("expected results for 'methodology' keyword search")
	}

	// No match
	results = r.Search("zzz-does-not-exist-zzz")
	if len(results) != 0 {
		t.Errorf("expected no results for non-matching query, got %d", len(results))
	}
}

func TestIsResourcePath(t *testing.T) {
	if !IsResourcePath("persona/foo") {
		t.Error("expected persona/foo to be a resource path")
	}
	if !IsResourcePath("template/my-template") {
		t.Error("expected template/my-template to be a resource path")
	}
	if IsResourcePath("helix") {
		t.Error("expected helix to NOT be a resource path")
	}
	if IsResourcePath("some-package") {
		t.Error("expected some-package to NOT be a resource path")
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}

	result := ExpandHome("~/.agents/skills/")
	if !strings.HasPrefix(result, home) {
		t.Errorf("expected expanded path to start with %q, got %q", home, result)
	}
	if strings.HasPrefix(result, "~") {
		t.Error("expected ~ to be expanded")
	}

	// Non-~ path should be returned unchanged
	plain := "/absolute/path"
	if ExpandHome(plain) != plain {
		t.Errorf("expected unchanged absolute path, got %q", ExpandHome(plain))
	}

	relative := "relative/path"
	if ExpandHome(relative) != relative {
		t.Errorf("expected unchanged relative path, got %q", ExpandHome(relative))
	}
}

func TestLoadSaveState(t *testing.T) {
	tmpDir := t.TempDir()

	// Override HOME so LoadState/SaveState use our temp dir.
	orig := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", orig) }()

	entry := InstalledEntry{
		Name:        "helix",
		Version:     "0.1.0",
		Type:        PackageTypeWorkflow,
		Source:      "https://github.com/easel/helix",
		InstalledAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Files:       []string{"~/.agents/skills/helix.md"},
	}

	state := &InstalledState{
		Installed: []InstalledEntry{entry},
	}

	if err := SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if len(loaded.Installed) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(loaded.Installed))
	}

	got := loaded.Installed[0]
	if got.Name != entry.Name {
		t.Errorf("expected name=%q, got %q", entry.Name, got.Name)
	}
	if got.Version != entry.Version {
		t.Errorf("expected version=%q, got %q", entry.Version, got.Version)
	}
	if got.Type != entry.Type {
		t.Errorf("expected type=%q, got %q", entry.Type, got.Type)
	}
	if got.Source != entry.Source {
		t.Errorf("expected source=%q, got %q", entry.Source, got.Source)
	}
	if len(got.Files) != 1 || got.Files[0] != entry.Files[0] {
		t.Errorf("expected files=%v, got %v", entry.Files, got.Files)
	}

	// Verify round-trip of time (truncated to second precision by YAML)
	if got.InstalledAt.IsZero() {
		t.Error("expected non-zero InstalledAt")
	}
}

func TestSymlinkSkills_BrokenTarballSymlinks(t *testing.T) {
	// Simulate the GitHub tarball bug: .agents/skills/ contains symlinks
	// with absolute paths from the build machine (e.g. /home/erik/Projects/helix/skills/...)
	// that don't exist on the installing machine.
	root := t.TempDir()

	// Create the real skill directories (as they exist after Root copy)
	realSkillDir := filepath.Join(root, "skills", "helix-test")
	require.NoError(t, os.MkdirAll(realSkillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(realSkillDir, "SKILL.md"), []byte("test"), 0o644))

	// Create broken symlinks (as tarball would produce)
	agentsSkillDir := filepath.Join(root, ".agents", "skills")
	require.NoError(t, os.MkdirAll(agentsSkillDir, 0o755))
	// This symlink points to an absolute path that doesn't exist
	require.NoError(t, os.Symlink("/nonexistent/build-machine/skills/helix-test",
		filepath.Join(agentsSkillDir, "helix-test")))

	// symlinkSkills should recover by finding skills/helix-test in the root
	dstDir := filepath.Join(t.TempDir(), ".claude", "skills")
	written, err := symlinkSkills(root, &InstallMapping{
		Source: ".agents/skills/",
		Target: dstDir,
	})
	require.NoError(t, err)
	require.Len(t, written, 1, "should have created 1 symlink despite broken source symlink")

	// The created symlink should point to the real skill dir
	target, err := os.Readlink(filepath.Join(dstDir, "helix-test"))
	require.NoError(t, err)
	assert.Contains(t, target, "skills/helix-test")

	// And the target should actually exist
	_, err = os.Stat(filepath.Join(dstDir, "helix-test", "SKILL.md"))
	assert.NoError(t, err, "skill content should be accessible through symlink")
}

func TestSymlinkSkills_WorkingSymlinks(t *testing.T) {
	// Normal case: .agents/skills/ has relative symlinks that resolve correctly
	root := t.TempDir()

	realSkillDir := filepath.Join(root, "skills", "helix-test")
	require.NoError(t, os.MkdirAll(realSkillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(realSkillDir, "SKILL.md"), []byte("test"), 0o644))

	agentsSkillDir := filepath.Join(root, ".agents", "skills")
	require.NoError(t, os.MkdirAll(agentsSkillDir, 0o755))
	// Relative symlink that works
	require.NoError(t, os.Symlink("../../skills/helix-test",
		filepath.Join(agentsSkillDir, "helix-test")))

	dstDir := filepath.Join(t.TempDir(), ".claude", "skills")
	written, err := symlinkSkills(root, &InstallMapping{
		Source: ".agents/skills/",
		Target: dstDir,
	})
	require.NoError(t, err)
	require.Len(t, written, 1)

	_, err = os.Stat(filepath.Join(dstDir, "helix-test", "SKILL.md"))
	assert.NoError(t, err)
}

func TestSymlinkSkills_RealDirectories(t *testing.T) {
	// Case: .agents/skills/ has real directories (not symlinks)
	// This happens after a clean ddx install (copyMapping resolves symlinks)
	root := t.TempDir()

	skillDir := filepath.Join(root, ".agents", "skills", "helix-test")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("test"), 0o644))

	dstDir := filepath.Join(t.TempDir(), ".claude", "skills")
	written, err := symlinkSkills(root, &InstallMapping{
		Source: ".agents/skills/",
		Target: dstDir,
	})
	require.NoError(t, err)
	require.Len(t, written, 1)

	_, err = os.Stat(filepath.Join(dstDir, "helix-test", "SKILL.md"))
	assert.NoError(t, err)
}

func TestSymlinkSkills_NoPreExistingTargetDir(t *testing.T) {
	// Case: target .claude/skills/ doesn't exist yet (fresh user)
	root := t.TempDir()

	skillDir := filepath.Join(root, ".agents", "skills", "helix-test")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("test"), 0o644))

	// Target directory does NOT exist — should be created
	dstDir := filepath.Join(t.TempDir(), "fresh-user", ".claude", "skills")
	written, err := symlinkSkills(root, &InstallMapping{
		Source: ".agents/skills/",
		Target: dstDir,
	})
	require.NoError(t, err)
	require.Len(t, written, 1)

	_, err = os.Stat(filepath.Join(dstDir, "helix-test", "SKILL.md"))
	assert.NoError(t, err, "should create target dir and symlink even when .claude/ doesn't exist")
}

func TestVerifyFiles_AllMissing(t *testing.T) {
	entry := InstalledEntry{
		Name:  "phantom",
		Files: []string{"/nonexistent/path/a", "/nonexistent/path/b"},
	}
	if entry.VerifyFiles() {
		t.Error("expected VerifyFiles to return false when all files missing")
	}
}

func TestVerifyFiles_NoFiles(t *testing.T) {
	entry := InstalledEntry{Name: "empty"}
	if entry.VerifyFiles() {
		t.Error("expected VerifyFiles to return false when no files recorded")
	}
}

func TestVerifyFiles_SomeExist(t *testing.T) {
	tmpDir := t.TempDir()
	realFile := tmpDir + "/exists.txt"
	_ = os.WriteFile(realFile, []byte("x"), 0644)

	entry := InstalledEntry{
		Name:  "partial",
		Files: []string{"/nonexistent/file", realFile},
	}
	if !entry.VerifyFiles() {
		t.Error("expected VerifyFiles to return true when at least one file exists")
	}
}

func TestPruneStaleSkillLinks_RemovesStaleLinksWithinRoot(t *testing.T) {
	root := t.TempDir()
	dstDir := t.TempDir()

	// Create a real skill directory inside root
	realSkill := filepath.Join(root, "skills", "helix-old")
	require.NoError(t, os.MkdirAll(realSkill, 0o755))

	// Create a symlink in dstDir pointing INTO root (stale — not in allowed)
	staleLink := filepath.Join(dstDir, "helix-old")
	require.NoError(t, os.Symlink(realSkill, staleLink))

	// allowed map does NOT include "helix-old"
	allowed := map[string]bool{"helix-new": true}

	require.NoError(t, pruneStaleSkillLinks(root, dstDir, allowed))

	_, err := os.Lstat(staleLink)
	assert.True(t, os.IsNotExist(err), "stale symlink within root should be removed")
}

func TestPruneStaleSkillLinks_PreservesLinksOutsideRoot(t *testing.T) {
	root := t.TempDir()
	dstDir := t.TempDir()
	outsideDir := t.TempDir()

	// Symlink pointing OUTSIDE root (e.g. user-installed third-party skill)
	externalLink := filepath.Join(dstDir, "external-skill")
	require.NoError(t, os.Symlink(outsideDir, externalLink))

	// external-skill is NOT in allowed, but target is outside root — must be preserved
	allowed := map[string]bool{}

	require.NoError(t, pruneStaleSkillLinks(root, dstDir, allowed))

	_, err := os.Lstat(externalLink)
	assert.NoError(t, err, "symlink pointing outside root should be preserved")
}

func TestPruneStaleSkillLinks_PreservesAllowedLinks(t *testing.T) {
	root := t.TempDir()
	dstDir := t.TempDir()

	realSkill := filepath.Join(root, "skills", "helix-keep")
	require.NoError(t, os.MkdirAll(realSkill, 0o755))

	keepLink := filepath.Join(dstDir, "helix-keep")
	require.NoError(t, os.Symlink(realSkill, keepLink))

	// helix-keep IS in allowed — must not be removed
	allowed := map[string]bool{"helix-keep": true}

	require.NoError(t, pruneStaleSkillLinks(root, dstDir, allowed))

	_, err := os.Lstat(keepLink)
	assert.NoError(t, err, "allowed symlink should not be removed")
}

func TestPruneStaleSkillLinks_PreservesRealDirectories(t *testing.T) {
	root := t.TempDir()
	dstDir := t.TempDir()

	// A real directory (not a symlink) must never be removed by pruneStaleSkillLinks
	realDir := filepath.Join(dstDir, "ddx-doctor")
	require.NoError(t, os.MkdirAll(realDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(realDir, "SKILL.md"), []byte("# Doctor"), 0o644))

	allowed := map[string]bool{} // not in allowed

	require.NoError(t, pruneStaleSkillLinks(root, dstDir, allowed))

	assert.DirExists(t, realDir, "real directory (not a symlink) should not be removed")
}

func TestPruneStaleSkillLinks_EmptyInstalledRoot(t *testing.T) {
	// When installedRoot is empty, pruneStaleSkillLinks must be a no-op
	dstDir := t.TempDir()

	staleLink := filepath.Join(dstDir, "some-skill")
	someDir := t.TempDir()
	require.NoError(t, os.Symlink(someDir, staleLink))

	require.NoError(t, pruneStaleSkillLinks("", dstDir, map[string]bool{}))

	_, err := os.Lstat(staleLink)
	assert.NoError(t, err, "nothing should be removed when installedRoot is empty")
}

// FEAT-015 §5 / AC-004: plugin skill symlinks must be RELATIVE so they
// survive clones, home-directory moves, and tarball rebuilds on a
// different machine. This test asserts the link target is a relative
// path and that it resolves back to the real skill content.
func TestSymlinkSkills_WritesRelativeSymlinks(t *testing.T) {
	projectRoot := t.TempDir()

	// Simulate a plugin installed at $project/.ddx/plugins/helix/ with
	// a skill at .agents/skills/helix-align (real directory).
	pluginRoot := filepath.Join(projectRoot, ".ddx", "plugins", "helix")
	realSkillDir := filepath.Join(pluginRoot, ".agents", "skills", "helix-align")
	require.NoError(t, os.MkdirAll(realSkillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(realSkillDir, "SKILL.md"), []byte("# helix-align\n"), 0o644))

	// Install target is $project/.agents/skills/ — a sibling of the
	// plugin root that shares a common ancestor, so filepath.Rel
	// produces a short relative path.
	dstDir := filepath.Join(projectRoot, ".agents", "skills")

	written, err := symlinkSkills(pluginRoot, &InstallMapping{
		Source: ".agents/skills",
		Target: dstDir,
	})
	require.NoError(t, err)
	require.Len(t, written, 1)

	linkPath := filepath.Join(dstDir, "helix-align")
	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	assert.False(t, filepath.IsAbs(target),
		"plugin skill symlink must be relative (got absolute: %s) — FEAT-015 §5 relative-symlinks rule",
		target)

	// Sanity: relative link resolves to the real SKILL.md.
	content, err := os.ReadFile(filepath.Join(linkPath, "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "# helix-align\n", string(content))
}
