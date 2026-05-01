package cmd

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/skills"
)

// installGlobal implements `ddx install --global`: extracts the embedded
// DDx skill tree into the user's home directory and chains relative
// symlinks from `~/.agents/skills/` and `~/.claude/skills/` so every
// skills-aware agent runtime sees the same canonical files in
// `~/.ddx/skills/`.
//
// Layout after success (per FEAT-015 AC-002):
//
//	~/.ddx/skills/<skill>/...            # real files
//	~/.agents/skills/<skill>  → ../../.ddx/skills/<skill>
//	~/.claude/skills/<skill>  → ../../.agents/skills/<skill>
//
// Idempotent: re-running updates files in place (force mirrors every
// existing file regardless of timestamp; without --force, existing files
// are left alone to respect user edits). Symlinks are always replaced to
// point at the new canonical target.
func (f *CommandFactory) installGlobal(force bool, out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("install --global: resolve home directory: %w", err)
	}

	ddxSkillsRoot := filepath.Join(home, ".ddx", "skills")
	agentsSkillsRoot := filepath.Join(home, ".agents", "skills")
	claudeSkillsRoot := filepath.Join(home, ".claude", "skills")

	// Enumerate top-level skill directories from the embedded tree. The
	// embedded FS root contains one entry per skill (today just "ddx"),
	// each with its own SKILL.md + reference/ layout.
	topLevel, err := fs.ReadDir(skills.SkillFiles, ".")
	if err != nil {
		return fmt.Errorf("install --global: read embedded skill tree: %w", err)
	}
	var skillNames []string
	for _, entry := range topLevel {
		if entry.IsDir() {
			skillNames = append(skillNames, entry.Name())
		}
	}
	if len(skillNames) == 0 {
		return fmt.Errorf("install --global: embedded skill tree is empty")
	}

	// 1. Extract each skill into ~/.ddx/skills/<name>/.
	if err := os.MkdirAll(ddxSkillsRoot, 0o755); err != nil {
		return fmt.Errorf("install --global: create %s: %w", ddxSkillsRoot, err)
	}
	for _, name := range skillNames {
		if err := extractSkillTree(skills.SkillFiles, name, filepath.Join(ddxSkillsRoot, name), force); err != nil {
			return fmt.Errorf("install --global: extract %s: %w", name, err)
		}
	}

	// 2. Symlink from ~/.agents/skills/<name> → ../../.ddx/skills/<name>.
	if err := os.MkdirAll(agentsSkillsRoot, 0o755); err != nil {
		return fmt.Errorf("install --global: create %s: %w", agentsSkillsRoot, err)
	}
	for _, name := range skillNames {
		target := filepath.Join("..", "..", ".ddx", "skills", name)
		if err := replaceRelativeSymlink(filepath.Join(agentsSkillsRoot, name), target); err != nil {
			return fmt.Errorf("install --global: symlink %s: %w", name, err)
		}
	}

	// 3. Symlink from ~/.claude/skills/<name> → ../../.agents/skills/<name>.
	// Claude Code reads ~/.claude/skills/; chaining through ~/.agents/
	// keeps one canonical anchor while every runtime finds the skill.
	if err := os.MkdirAll(claudeSkillsRoot, 0o755); err != nil {
		return fmt.Errorf("install --global: create %s: %w", claudeSkillsRoot, err)
	}
	for _, name := range skillNames {
		target := filepath.Join("..", "..", ".agents", "skills", name)
		if err := replaceRelativeSymlink(filepath.Join(claudeSkillsRoot, name), target); err != nil {
			return fmt.Errorf("install --global: claude symlink %s: %w", name, err)
		}
	}

	fmt.Fprintf(out, "Extracted %d skill(s) to %s\n", len(skillNames), ddxSkillsRoot)
	for _, name := range skillNames {
		fmt.Fprintf(out, "  %s\n", name)
	}
	fmt.Fprintf(out, "Linked ~/.agents/skills and ~/.claude/skills to ~/.ddx/skills\n")
	return nil
}

// extractSkillTree walks skillName in srcFS and writes every file under
// destRoot, preserving the directory structure. When force is false,
// existing files are left alone so user edits survive re-extraction.
func extractSkillTree(srcFS fs.FS, skillName, destRoot string, force bool) error {
	return fs.WalkDir(srcFS, skillName, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(skillName, path)
		if err != nil {
			return err
		}
		// rel == "." for the skill root itself — handled by MkdirAll.
		destPath := filepath.Join(destRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}

		if !force {
			if _, err := os.Stat(destPath); err == nil {
				// Respect user edits on re-run without --force.
				return nil
			}
		}

		data, err := fs.ReadFile(srcFS, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(destPath), err)
		}
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", destPath, err)
		}
		return nil
	})
}

// replaceRelativeSymlink points linkPath at target (relative). If
// linkPath already exists — as a real directory, a file, or another
// symlink — it is removed first so re-running converges on the desired
// target. A real directory at linkPath is an install-time data-loss
// risk, so it is refused explicitly rather than silently deleted.
func replaceRelativeSymlink(linkPath, relativeTarget string) error {
	info, err := os.Lstat(linkPath)
	switch {
	case os.IsNotExist(err):
		// Nothing to replace — fall through to create.
	case err != nil:
		return fmt.Errorf("inspect %s: %w", linkPath, err)
	case info.Mode()&os.ModeSymlink != 0:
		// Existing symlink — always replace so the target is fresh.
		if rmErr := os.Remove(linkPath); rmErr != nil {
			return fmt.Errorf("remove existing symlink %s: %w", linkPath, rmErr)
		}
	case info.IsDir():
		// Real directory: refuse. A `.agents/skills/ddx` folder with
		// real files is not something the global installer should wipe.
		return fmt.Errorf("%s exists as a real directory; refusing to replace with a symlink. Remove it manually or pass --force to a future surface that handles this explicitly", linkPath)
	default:
		// Regular file — also unsafe to silently clobber.
		return fmt.Errorf("%s exists as a regular file; refusing to replace", linkPath)
	}
	return os.Symlink(relativeTarget, linkPath)
}
