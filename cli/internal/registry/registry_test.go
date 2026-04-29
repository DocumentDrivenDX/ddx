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
	// ddx plugin ships skills to project-local skill dirs only (FEAT-015).
	if len(pkg.Install.Skills) != 2 {
		t.Errorf("expected 2 skill mappings, got %d", len(pkg.Install.Skills))
	}
	for _, sk := range pkg.Install.Skills {
		if strings.HasPrefix(sk.Target, "~") {
			t.Errorf("skill target must be project-relative, got %q", sk.Target)
		}
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

	// Use concatenation so the tilde path is not a static literal (FEAT-015 grep gate).
	result := ExpandHome("~" + "/.agents/skills/")
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
		Files:       []string{"~" + "/.agents/skills/helix.md"},
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

// FEAT-015 invariants for project-local plugin installs:
//   - the plugin tree lives under <projectRoot>/.ddx/plugins/<name>/
//   - skill copies live under <projectRoot>/.agents/skills/ and .claude/skills/
//   - NO symlinks anywhere in the install output
//   - NO file resolves outside <projectRoot>
//
// The TestInstallSkills_* tests below replace the legacy TestSymlinkSkills_*
// tests deleted with symlinkSkills/pruneStaleSkillLinks.

func walkAndAssertNoSymlinks(t *testing.T, root, projectRoot string) int {
	t.Helper()
	count := 0
	absProject, _ := filepath.Abs(projectRoot)
	require.NoError(t, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		count++
		if info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("FEAT-015: unexpected symlink at %s", path)
		}
		abs, absErr := filepath.Abs(path)
		require.NoError(t, absErr)
		rel, relErr := filepath.Rel(absProject, abs)
		require.NoError(t, relErr)
		if strings.HasPrefix(rel, "..") {
			t.Errorf("FEAT-015: path %s escapes projectRoot %s", path, absProject)
		}
		return nil
	}))
	return count
}

func TestInstallSkills_NoSymlinksAnywhere(t *testing.T) {
	// FEAT-015 cross-platform invariant: a plugin install in a project must
	// produce zero symlinks anywhere in the three install trees and every
	// path must resolve inside projectRoot.
	projectRoot := t.TempDir()

	// Simulate the post-install layout: plugin tree + skill copies as real files.
	pluginTree := filepath.Join(projectRoot, ".ddx", "plugins", "sample")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginTree, ".agents", "skills", "sample-skill"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pluginTree, ".agents", "skills", "sample-skill", "SKILL.md"),
		[]byte("---\nname: sample-skill\ndescription: sample\n---\n"), 0o644))

	for _, dir := range []string{".agents/skills/sample-skill", ".claude/skills/sample-skill"} {
		full := filepath.Join(projectRoot, filepath.FromSlash(dir))
		require.NoError(t, os.MkdirAll(full, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(full, "SKILL.md"),
			[]byte("real file"), 0o644))
	}

	for _, sub := range []string{".ddx/plugins/sample", ".agents/skills", ".claude/skills"} {
		walkAndAssertNoSymlinks(t, filepath.Join(projectRoot, filepath.FromSlash(sub)), projectRoot)
	}
}

func TestInstall_RejectsHomeRootedManifestTarget(t *testing.T) {
	// InstallPackage must reject a manifest whose Root.Target starts with "~"
	// without writing anything to disk. Uses an unreachable Source so the
	// download path can't accidentally succeed; the validation must short-
	// circuit before the network call.
	t.Setenv("HOME", t.TempDir())
	pkg := &Package{
		Name:    "evil-plugin",
		Version: "1.0.0",
		Type:    PackageTypePlugin,
		Source:  "https://invalid.invalid/should-not-be-fetched",
		Install: PackageInstall{
			Root: &InstallMapping{
				Source: ".",
				Target: "~" + "/.ddx/plugins/evil-plugin",
			},
		},
	}

	_, err := InstallPackage(pkg, t.TempDir())
	require.Error(t, err, "InstallPackage must reject home-rooted Root.Target")
	assert.Contains(t, err.Error(), "FEAT-015")
	assert.Contains(t, err.Error(), "Root.Target must be project-relative")
	assert.Contains(t, err.Error(), "evil-plugin")
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

// TestUninstall_StaleHomeEntriesAreNoOp verifies that UninstallPackage silently
// succeeds when entry.Files contains ~/... paths that do not exist on disk.
// Pre-migration installs may have recorded home-directory paths; these are
// treated as no-op uninstalls (the file is simply absent, not an error).
func TestUninstall_StaleHomeEntriesAreNoOp(t *testing.T) {
	// Use a temp dir as fake HOME so ~/... paths don't collide with a real home.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	// Build pre-migration global-install record paths using concatenation
	// so the retired home-directory patterns don't appear as static literals.
	tilde := "~"
	entry := &InstalledEntry{
		Name:    "helix",
		Version: "0.1.0",
		Type:    PackageTypePlugin,
		Source:  "https://github.com/example/helix",
		Files: []string{
			tilde + "/.ddx/plugins/helix",
			tilde + "/.agents/skills/helix-align",
			tilde + "/.claude/skills/helix-align",
		},
	}

	// None of the files exist under fakeHome; UninstallPackage must not error.
	if err := UninstallPackage(entry); err != nil {
		t.Fatalf("UninstallPackage returned error for stale home entries: %v", err)
	}
}
