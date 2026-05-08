package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMaterializeWorktreeSkills_RemovesBrokenLinks simulates an execute-bead
// worktree whose `.agents/skills/` and `.claude/skills/` directories contain
// project-local symlinks whose targets do not exist. It asserts that after
// materializeWorktreeSkills runs, no broken symlinks remain in those
// directories (so the harness cannot emit "failed to stat" errors).
func TestMaterializeWorktreeSkills_RemovesBrokenLinks(t *testing.T) {
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

	if err := materializeWorktreeSkills(wt); err != nil {
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

	if err := materializeWorktreeSkills(wt); err != nil {
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

// TestMaterializeWorktreeSkills_RewritesToGlobalPlugin covers the happy
// path of approach (A): a broken link whose target encodes a plugin name
// is rewritten to point at the user's global plugin install when that
// install exists. We simulate a fake HOME containing the expected plugin
// layout so the test does not depend on the real host.
func TestMaterializeWorktreeSkills_RewritesToGlobalPlugin(t *testing.T) {
	fakeHome := t.TempDir()
	// Lay out ~/.ddx/plugins/helix/.agents/skills/helix-align
	skillDir := filepath.Join(fakeHome, ".ddx", "plugins", "helix", ".agents", "skills", "helix-align")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skillDir: %v", err)
	}

	// Create a worktree with a broken symlink whose target path embeds
	// .ddx/plugins/helix/...
	wt := t.TempDir()
	linkParent := filepath.Join(wt, ".claude", "skills")
	if err := os.MkdirAll(linkParent, 0o755); err != nil {
		t.Fatalf("mkdir linkParent: %v", err)
	}
	brokenTarget := "/home/demo/.ddx/plugins/helix/.agents/skills/helix-align"
	linkPath := filepath.Join(linkParent, "helix-align")
	if err := os.Symlink(brokenTarget, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// Redirect HOME so resolveBrokenSkillLink finds our fake install.
	t.Setenv("HOME", fakeHome)

	if err := materializeWorktreeSkills(wt); err != nil {
		t.Fatalf("materializeWorktreeSkills: %v", err)
	}

	// Expect the symlink to now resolve to the fake home install.
	resolved, err := os.Stat(linkPath)
	if err != nil {
		t.Fatalf("stat linkPath: %v", err)
	}
	if !resolved.IsDir() {
		t.Errorf("resolved link is not a directory")
	}
	newTarget, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if newTarget != skillDir {
		t.Errorf("symlink not rewritten: got %s, want %s", newTarget, skillDir)
	}
}

func TestPluginFromSkillLinkTarget(t *testing.T) {
	cases := map[string]string{
		"/home/demo/.ddx/plugins/helix/.agents/skills/helix-align": "helix",
		"../../.ddx/plugins/ddx/.agents/skills/ddx-run":            "ddx",
		"/no/plugin/here": "",
		"":                "",
	}
	for in, want := range cases {
		got := pluginFromSkillLinkTarget(in)
		if got != want {
			t.Errorf("pluginFromSkillLinkTarget(%q) = %q, want %q", in, got, want)
		}
	}
}
