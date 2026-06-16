package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
)

// TestMaterializeWorktreeSkills_RemovesBrokenLinks simulates an execute-bead
// worktree whose `.agents/skills/` and `.claude/skills/` directories contain
// project-local symlinks whose targets do not exist. It asserts that after
// materializeWorktreeSkills runs, no broken symlinks remain in those
// directories (so the harness cannot emit "failed to stat" errors).
func TestMaterializeWorktreeSkills_RemovesBrokenLinks(t *testing.T) {
	setWorktreeSkillTestCache(t)
	wt := t.TempDir()

	for _, rel := range []string{".agents/skills", ".claude/skills"} {
		dir := filepath.Join(wt, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		// Simulate a build-machine-specific absolute target that does not
		// exist on this host.
		brokenTarget := "/nonexistent/home/demo/.ddx/plugins/helix/.agents/skills/helix-align"
		if err := os.Symlink(brokenTarget, filepath.Join(dir, "helix-align")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	}

	if err := materializeWorktreeSkills("", wt); err != nil {
		t.Fatalf("materializeWorktreeSkills: %v", err)
	}

	// After repair, no broken symlinks should remain. os.Stat follows the
	// link, so a broken link reports a non-IsNotExist error.
	for _, rel := range []string{".agents/skills", ".claude/skills"} {
		dir := filepath.Join(wt, rel)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if _, err := os.Stat(p); err != nil && os.IsNotExist(err) {
				t.Errorf("broken symlink remains at %s after materialize", p)
			}
		}
	}
}

// TestMaterializeWorktreeSkills_PreservesValidLinks ensures that symlinks
// whose targets do resolve (e.g. correctly re-materialized by install) are
// left untouched.
func TestMaterializeWorktreeSkills_PreservesValidLinks(t *testing.T) {
	setWorktreeSkillTestCache(t)
	wt := t.TempDir()

	// Create a real target and link to it.
	realDir := filepath.Join(wt, "real", "skills", "helix-align")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	linkParent := filepath.Join(wt, ".claude", "skills")
	if err := os.MkdirAll(linkParent, 0o755); err != nil {
		t.Fatalf("mkdir link parent: %v", err)
	}
	linkPath := filepath.Join(linkParent, "helix-align")
	if err := os.Symlink(realDir, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := materializeWorktreeSkills("", wt); err != nil {
		t.Fatalf("materializeWorktreeSkills: %v", err)
	}

	if _, err := os.Stat(linkPath); err != nil {
		t.Errorf("valid symlink was removed: %v", err)
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != realDir {
		t.Errorf("symlink target changed: got %s, want %s", target, realDir)
	}
}

// TestExecuteBead_WorktreeFromPreMigrationCommit verifies that a worktree
// checked out from a pre-FEAT-015 commit (where skill dirs contained
// symlinks into an old global path that no longer exists) is cleaned up
// gracefully: broken symlinks are removed and no error is returned. This
// confirms that resolveBrokenSkillLink (deleted in FEAT-015) is not needed.
func TestExecuteBead_WorktreeFromPreMigrationCommit(t *testing.T) {
	setWorktreeSkillTestCache(t)
	wt := t.TempDir()

	// Simulate pre-migration state: symlinks in .agents/skills/ and
	// .claude/skills/ pointing to an old absolute path that no longer exists.
	for _, rel := range []string{".agents/skills", ".claude/skills"} {
		dir := filepath.Join(wt, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		// Target encodes the pre-migration global plugin layout.
		oldGlobalTarget := "/home/olduser/old-ddx-plugins/helix/skills/helix-align"
		if err := os.Symlink(oldGlobalTarget, filepath.Join(dir, "helix-align")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	}

	// Simulate a real file alongside the broken symlink (should be preserved).
	realSkillDir := filepath.Join(wt, ".agents", "skills", "ddx")
	if err := os.MkdirAll(realSkillDir, 0o755); err != nil {
		t.Fatalf("mkdir real skill: %v", err)
	}

	if err := materializeWorktreeSkills("", wt); err != nil {
		t.Fatalf("materializeWorktreeSkills: %v", err)
	}

	// Broken symlinks must be gone.
	for _, rel := range []string{".agents/skills", ".claude/skills"} {
		broken := filepath.Join(wt, rel, "helix-align")
		if _, err := os.Lstat(broken); err == nil {
			t.Errorf("broken symlink was not removed: %s", broken)
		}
	}

	// Real file must be preserved.
	if _, err := os.Stat(realSkillDir); err != nil {
		t.Errorf("real skill directory was removed: %v", err)
	}
}

func TestMaterializeWorktreeSkills_RecreatesBuiltinDDxAdapters(t *testing.T) {
	setWorktreeSkillTestCache(t)
	wt := t.TempDir()

	if err := materializeWorktreeSkills("", wt); err != nil {
		t.Fatalf("materializeWorktreeSkills: %v", err)
	}

	for _, rel := range []string{".agents/skills/ddx", ".claude/skills/ddx"} {
		skillPath := filepath.Join(wt, rel, "bead-lifecycle", "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			t.Fatalf("expected built-in ddx skill adapter %s: %v", skillPath, err)
		}
	}
}

func TestMaterializeWorktreeSkills_RecreatesLockedPluginAdapters(t *testing.T) {
	setWorktreeSkillTestCache(t)
	projectRoot := t.TempDir()
	wt := t.TempDir()
	cachePath := filepath.Join(t.TempDir(), "cache", "plugins", "example", "1.0.0")
	skillDir := filepath.Join(cachePath, "skills", "example-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: example-skill\ndescription: Example skill.\n---\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.MkdirAll(ddxroot.InTree(projectRoot), 0o755); err != nil {
		t.Fatalf("mkdir project ddx: %v", err)
	}
	lock := &registry.PluginLock{Plugins: []registry.PluginLockEntry{{
		Name:      "example",
		Version:   "1.0.0",
		Type:      registry.PackageTypePlugin,
		Source:    "https://example.invalid/example",
		CachePath: cachePath,
	}}}
	if err := registry.SaveProjectPluginLock(testContext(t), projectRoot, lock); err != nil {
		t.Fatalf("save plugin lock: %v", err)
	}

	if err := materializeWorktreeSkills(projectRoot, wt); err != nil {
		t.Fatalf("materializeWorktreeSkills: %v", err)
	}

	for _, rel := range []string{".agents/skills/example-skill", ".claude/skills/example-skill"} {
		skillPath := filepath.Join(wt, rel, "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			t.Fatalf("expected locked plugin skill adapter %s: %v", skillPath, err)
		}
	}
}

func setWorktreeSkillTestCache(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg-data"))
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}
