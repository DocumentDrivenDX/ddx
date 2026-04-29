package agent

import (
	"os"
	"path/filepath"
)

// skillLinkDirs lists the project-local skill directories that an execute-bead
// worktree must clean before the agent runs. In the post-FEAT-015 model these
// contain real files; pre-migration worktrees may still have symlinks whose
// targets no longer exist (the old global plugin tree is gone). Broken
// symlinks are removed silently so the harness does not emit "failed to stat"
// errors on startup.
var skillLinkDirs = []string{
	filepath.Join(".agents", "skills"),
	filepath.Join(".claude", "skills"),
}

// materializeWorktreeSkills cleans up broken symlinks in the project-local
// skill directories inside an execute-bead worktree. Real files and valid
// symlinks are left untouched. Broken symlinks (pre-migration remnants) are
// removed so the harness does not log stat errors.
func materializeWorktreeSkills(wtPath string) error {
	for _, rel := range skillLinkDirs {
		dir := filepath.Join(wtPath, rel)
		if err := repairSkillLinkDir(dir); err != nil {
			return err
		}
	}
	return nil
}

// repairSkillLinkDir walks a single skill directory and removes broken
// symlinks. It is a no-op when dir does not exist.
func repairSkillLinkDir(dir string) error {
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
		// Broken symlink — remove so the harness stops logging stat errors.
		_ = os.Remove(entryPath)
	}
	return nil
}
