package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Options controls Install behavior.
type Options struct {
	// Force overwrites pre-existing real-file destinations. When false, existing
	// real-file skill directories are preserved (per-skill skip). Pre-existing
	// symlink destinations are ALWAYS removed regardless of Force.
	Force bool
}

// Install copies skills from src into <projectRoot>/.agents/skills/<name>/ and
// <projectRoot>/.claude/skills/<name>/ as real files. Never creates symlinks.
//
// Source layouts:
//   - Top-level skill dirs (embed.FS bootstrap): src has "<skillName>/..."
//   - Plugin layout (os.DirFS over plugin root): src has ".agents/skills/<skillName>";
//     when that path is a broken symlink (tarball authored with symlinks),
//     falls back to "skills/<skillName>".
func Install(src fs.FS, projectRoot string, opts Options) error {
	if projectRoot == "" {
		return errors.New("skills.Install: projectRoot is empty")
	}

	skillNames, sources, err := discoverSkills(src)
	if err != nil {
		return fmt.Errorf("discover skills: %w", err)
	}

	for _, name := range skillNames {
		if err := validateSkillName(name); err != nil {
			return err
		}
	}

	targets := []string{
		filepath.Join(projectRoot, ".agents", "skills"),
		filepath.Join(projectRoot, ".claude", "skills"),
	}

	// Pre-flight: validate every destination resolves inside projectRoot.
	// Done before any write so traversal/escape inputs error out cleanly.
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("resolve projectRoot: %w", err)
	}
	for _, target := range targets {
		for _, name := range skillNames {
			destDir := filepath.Join(target, name)
			// Symlinks are always removed and replaced with real files; skip the
			// within-root check for them (the replacement write will be inside
			// the root). This handles pre-migration symlinks pointing to
			// locations outside the project root.
			if info, err := os.Lstat(destDir); err == nil && info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if err := assertWithinRoot(destDir, absRoot); err != nil {
				return err
			}
		}
	}

	// Ensure target parents exist after validation.
	for _, t := range targets {
		if err := os.MkdirAll(t, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", t, err)
		}
		// Reject if target itself is a symlink escaping projectRoot.
		if err := assertWithinRoot(t, absRoot); err != nil {
			return err
		}
	}

	for i, name := range skillNames {
		srcSub := sources[i]
		for _, target := range targets {
			destDir := filepath.Join(target, name)
			info, lerr := os.Lstat(destDir)
			if lerr == nil {
				if info.Mode()&os.ModeSymlink != 0 {
					// Symlink: always remove and replace, regardless of Force.
					if err := os.Remove(destDir); err != nil {
						return fmt.Errorf("remove pre-existing symlink %s: %w", destDir, err)
					}
				} else if !opts.Force {
					// Real-file destination + !Force: skip per-skill (whole dir).
					continue
				} else {
					// Real-file destination + Force: overwrite by removing first.
					if err := os.RemoveAll(destDir); err != nil {
						return fmt.Errorf("remove existing dir %s: %w", destDir, err)
					}
				}
			}
			if err := copyFromFS(srcSub, destDir); err != nil {
				return fmt.Errorf("copy skill %q: %w", name, err)
			}
		}
	}

	return nil
}

// discoverSkills inspects src and returns parallel slices of skill names and
// per-skill sub-FSes rooted at the skill content. Handles both bootstrap
// (top-level) and plugin (.agents/skills) layouts.
func discoverSkills(src fs.FS) ([]string, []fs.FS, error) {
	if info, err := fs.Stat(src, ".agents/skills"); err == nil && info.IsDir() {
		entries, err := fs.ReadDir(src, ".agents/skills")
		if err != nil {
			return nil, nil, err
		}
		var names []string
		var subs []fs.FS
		for _, e := range entries {
			name := e.Name()
			primary := ".agents/skills/" + name
			// Try the canonical path first. fs.Stat follows symlinks under os.DirFS,
			// so a broken symlink yields an error here.
			if _, statErr := fs.Stat(src, primary); statErr == nil {
				sub, err := fs.Sub(src, primary)
				if err != nil {
					return nil, nil, err
				}
				names = append(names, name)
				subs = append(subs, sub)
				continue
			}
			// Broken symlink (tarball case) — fall back to skills/<name>.
			fallback := "skills/" + name
			if _, statErr := fs.Stat(src, fallback); statErr != nil {
				continue
			}
			sub, err := fs.Sub(src, fallback)
			if err != nil {
				return nil, nil, err
			}
			names = append(names, name)
			subs = append(subs, sub)
		}
		return names, subs, nil
	}

	// Default layout: top-level entries are skill directories.
	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		return nil, nil, err
	}
	var names []string
	var subs []fs.FS
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, err := fs.Sub(src, e.Name())
		if err != nil {
			return nil, nil, err
		}
		names = append(names, e.Name())
		subs = append(subs, sub)
	}
	return names, subs, nil
}

// validateSkillName rejects names that could escape the destination tree.
func validateSkillName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("skills.Install: invalid skill name %q", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("skills.Install: skill name contains path separator: %q", name)
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("skills.Install: skill name is absolute: %q", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("skills.Install: skill name contains parent reference: %q", name)
	}
	return nil
}

// assertWithinRoot resolves dest (and the existing portion of its path) and
// returns an error if it escapes absRoot via symlinks or absolute paths.
func assertWithinRoot(dest, absRoot string) error {
	abs, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dest, err)
	}
	// Walk upward to find an existing ancestor and EvalSymlinks on it; this
	// catches symlink-escapes even when the leaf does not yet exist.
	check := abs
	for {
		if _, err := os.Lstat(check); err == nil {
			resolved, err := filepath.EvalSymlinks(check)
			if err != nil {
				return fmt.Errorf("resolve symlinks for %s: %w", check, err)
			}
			resolvedRoot, rerr := filepath.EvalSymlinks(absRoot)
			if rerr != nil {
				// projectRoot may not exist yet — fall back to lexical compare.
				resolvedRoot = absRoot
			}
			if !pathHasPrefix(resolved, resolvedRoot) {
				return fmt.Errorf("skills.Install: path %s escapes project root %s", dest, absRoot)
			}
			break
		}
		parent := filepath.Dir(check)
		if parent == check {
			break
		}
		check = parent
	}
	if !pathHasPrefix(abs, absRoot) {
		return fmt.Errorf("skills.Install: path %s escapes project root %s", dest, absRoot)
	}
	return nil
}

func pathHasPrefix(p, prefix string) bool {
	if p == prefix {
		return true
	}
	sep := string(filepath.Separator)
	if !strings.HasSuffix(prefix, sep) {
		prefix += sep
	}
	return strings.HasPrefix(p, prefix)
}

// copyFromFS recursively copies src (rooted at ".") into destDir as real files.
// Symlinks in the source are skipped (never materialized) — see FEAT-015
// "Cross-platform invariant".
func copyFromFS(src fs.FS, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		if strings.Contains(p, "..") {
			return fmt.Errorf("invalid path in source: %s", p)
		}
		target := filepath.Join(destDir, p)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			// Skip plugin-source symlinks — never materialize.
			return nil
		}
		data, rerr := fs.ReadFile(src, p)
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(target, data, 0o644)
	})
}
