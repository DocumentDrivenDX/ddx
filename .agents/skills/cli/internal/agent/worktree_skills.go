package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// skillLinkDirs lists the project-local skill link farms that an execute-bead
// worktree must materialize before the agent runs. Git tracks these as
// symlinks, but their targets point to installed plugin roots that may not
// exist inside the fresh worktree (e.g. build-machine-specific absolute paths
// left over from a tarball extraction). When the Claude Code harness walks
// these directories at startup it emits repeated "failed to stat" errors,
// polluting stderr and breaking skill discovery.
var skillLinkDirs = []string{
	filepath.Join(".agents", "skills"),
	filepath.Join(".claude", "skills"),
}

// materializeWorktreeSkills repairs project-local skill symlinks inside an
// execute-bead worktree so the harness can resolve them without noisy errors.
//
// Behavior:
//  1. Walks each skill link directory under wtPath.
//  2. For every symlink whose target does not exist, attempts to resolve the
//     intended skill by inferring the plugin name from the link target's
//     path segments (`.ddx/plugins/<plugin>/.agents/skills/<skill>`) and
//     searching the user's global plugin directory
//     (`~/.ddx/plugins/<plugin>/.agents/skills/<skill>`) for a match.
//  3. When a replacement is found, rewrites the symlink to the absolute path.
//  4. When no replacement is found, removes the broken symlink silently so
//     the harness does not log stat errors.
//
// Valid symlinks (whose targets resolve) are left untouched. Regular files
// and directories are left untouched.
func materializeWorktreeSkills(wtPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	for _, rel := range skillLinkDirs {
		dir := filepath.Join(wtPath, rel)
		if err := repairSkillLinkDir(dir, home); err != nil {
			return err
		}
	}
	return nil
}

// repairSkillLinkDir walks a single skill link directory and repairs broken
// symlinks. It is a no-op when dir does not exist.
func repairSkillLinkDir(dir, homeDir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		entryPath := filepath.Join(dir, e.Name())
		info, err := os.Lstat(entryPath)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		// Resolved successfully? Leave it alone.
		if _, err := os.Stat(entryPath); err == nil {
			continue
		}

		linkTarget, readErr := os.Readlink(entryPath)
		if readErr != nil {
			_ = os.Remove(entryPath)
			continue
		}

		replacement := resolveBrokenSkillLink(e.Name(), linkTarget, homeDir)
		if replacement != "" {
			if _, statErr := os.Stat(replacement); statErr == nil {
				_ = os.Remove(entryPath)
				if err := os.Symlink(replacement, entryPath); err == nil {
					continue
				}
			}
		}

		// Could not recover — remove so the harness stops logging stat errors.
		_ = os.Remove(entryPath)
	}
	return nil
}

// resolveBrokenSkillLink attempts to compute a valid absolute path for a
// broken project-local skill symlink. It returns an empty string when the
// intended target cannot be recovered.
//
// The symlink target is expected to look like
// `.../.ddx/plugins/<plugin>/.agents/skills/<skill>`. We extract <plugin> from
// the path segments and look the skill up inside the user's global plugin
// directory (`~/.ddx/plugins/<plugin>/.agents/skills/<skill>`), which is the
// canonical install location managed by `ddx install`.
func resolveBrokenSkillLink(skillName, linkTarget, homeDir string) string {
	if homeDir == "" {
		return ""
	}
	plugin := pluginFromSkillLinkTarget(linkTarget)
	if plugin == "" {
		return ""
	}
	return filepath.Join(homeDir, ".ddx", "plugins", plugin, ".agents", "skills", skillName)
}

// pluginFromSkillLinkTarget extracts the plugin name from a skill symlink
// target of the form `.../.ddx/plugins/<plugin>/.agents/skills/<skill>`.
// It returns an empty string when the expected segments are not present.
func pluginFromSkillLinkTarget(linkTarget string) string {
	// Normalize separators so we handle both posix-style recorded targets
	// and any future Windows paths consistently.
	segs := strings.Split(filepath.ToSlash(linkTarget), "/")
	for i := 0; i+3 < len(segs); i++ {
		if segs[i] == ".ddx" && segs[i+1] == "plugins" && segs[i+2] != "" {
			return segs[i+2]
		}
	}
	return ""
}
