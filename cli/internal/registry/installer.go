package registry

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InstallPackage downloads the source release tarball and copies declared install mappings.
// It records installed files in the returned InstalledEntry.
func InstallPackage(pkg *Package) (InstalledEntry, error) {
	entry := InstalledEntry{
		Name:        pkg.Name,
		Version:     pkg.Version,
		Type:        pkg.Type,
		Source:      pkg.Source,
		InstalledAt: time.Now(),
	}

	// Download and extract the release tarball to a temp directory.
	tmpDir, err := os.MkdirTemp("", "ddx-install-"+pkg.Name+"-*")
	if err != nil {
		return entry, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tarballURL := githubTarballURL(pkg.Source, pkg.Version)
	extractedDir, err := downloadAndExtract(tarballURL, tmpDir)
	if err != nil {
		return entry, fmt.Errorf("downloading %s: %w", tarballURL, err)
	}

	// Process Root mapping first - copy the entire plugin to central location.
	var installedRoot string
	if pkg.Install.Root != nil {
		files, err := copyMapping(extractedDir, pkg.Install.Root)
		if err != nil {
			return entry, fmt.Errorf("installing plugin root: %w", err)
		}
		entry.Files = append(entry.Files, files...)
		installedRoot = ExpandHome(pkg.Install.Root.Target)

		// Ensure declared executables have the execute bit set.
		for _, rel := range pkg.Install.Executable {
			p := filepath.Join(installedRoot, filepath.FromSlash(rel))
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				_ = os.Chmod(p, info.Mode()|0111)
			}
		}
	}

	// Create project-local symlink to global plugin root so that project-relative
	// paths (e.g. .ddx/plugins/helix/workflows/) resolve correctly.
	// Only when the root target is a global (~/...) path — skip for project-local targets.
	if installedRoot != "" && pkg.Install.Root != nil && strings.HasPrefix(pkg.Install.Root.Target, "~") {
		localPluginDir := filepath.Join(".ddx", "plugins", pkg.Name)
		if err := os.MkdirAll(filepath.Dir(localPluginDir), 0755); err == nil {
			// Remove existing file/symlink/directory.
			if _, err := os.Lstat(localPluginDir); err == nil {
				_ = os.RemoveAll(localPluginDir)
			}
			_ = os.Symlink(installedRoot, localPluginDir)
		}
	}

	// Process Skills mappings — symlink from installed root when available,
	// otherwise fall back to copying from the extracted tarball.
	for i := range pkg.Install.Skills {
		skill := &pkg.Install.Skills[i]
		if installedRoot != "" {
			files, err := symlinkSkills(installedRoot, skill)
			if err != nil {
				return entry, fmt.Errorf("symlinking skills: %w", err)
			}
			entry.Files = append(entry.Files, files...)
		} else {
			files, err := copyMapping(extractedDir, skill)
			if err != nil {
				return entry, fmt.Errorf("installing skills: %w", err)
			}
			entry.Files = append(entry.Files, files...)
		}
	}

	// Process Scripts mapping — copy the script to the target path.
	// If the target is already a symlink, the developer is managing it
	// themselves (e.g. symlink to their git checkout) — skip with a notice.
	if pkg.Install.Scripts != nil {
		dst := ExpandHome(pkg.Install.Scripts.Target)
		if li, err := os.Lstat(dst); err == nil && li.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(dst)
			fmt.Fprintf(os.Stderr, "notice: %s is a symlink → %s (developer mode, skipping copy)\n", dst, target)
			entry.Files = append(entry.Files, dst)
		} else {
			srcDir := extractedDir
			if installedRoot != "" {
				srcDir = installedRoot
			}
			files, err := copyMapping(srcDir, pkg.Install.Scripts)
			if err != nil {
				return entry, fmt.Errorf("installing scripts: %w", err)
			}
			entry.Files = append(entry.Files, files...)

			// Ensure the installed script is executable.
			if info, err := os.Stat(dst); err == nil && !info.IsDir() {
				_ = os.Chmod(dst, info.Mode()|0111)
			}
		}
	}

	// Process symlinks.
	for _, sym := range pkg.Install.Symlinks {
		src := ExpandHome(sym.Source)
		dst := ExpandHome(sym.Target)

		// Create parent dir if needed.
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return entry, fmt.Errorf("creating symlink dir %s: %w", filepath.Dir(dst), err)
		}

		// Remove existing symlink/file if present.
		if _, err := os.Lstat(dst); err == nil {
			if err := os.RemoveAll(dst); err != nil {
				return entry, fmt.Errorf("removing existing %s: %w", dst, err)
			}
		}

		if err := os.Symlink(src, dst); err != nil {
			return entry, fmt.Errorf("creating symlink %s -> %s: %w", dst, src, err)
		}
		entry.Files = append(entry.Files, dst)
	}

	return entry, nil
}

// InstallResource installs a single resource file (e.g. "persona/strict-code-reviewer")
// from the ddx-library GitHub repo into the local .ddx/plugins/ddx/<type>/ directory.
func InstallResource(resourcePath string) (InstalledEntry, error) {
	entry := InstalledEntry{
		Name:        resourcePath,
		Version:     "latest",
		Type:        PackageTypeResource,
		Source:      "https://github.com/DocumentDrivenDX/ddx-library",
		InstalledAt: time.Now(),
	}

	// resourcePath is like "persona/strict-code-reviewer"
	parts := strings.SplitN(resourcePath, "/", 2)
	if len(parts) != 2 {
		return entry, fmt.Errorf("invalid resource path %q: expected <type>/<name>", resourcePath)
	}
	resourceType, resourceName := parts[0], parts[1]

	// Determine target directory relative to cwd.
	target := filepath.Join(".ddx", "plugins", "ddx", resourceType+"s")
	if err := os.MkdirAll(target, 0755); err != nil {
		return entry, fmt.Errorf("creating target directory %s: %w", target, err)
	}

	// Fetch raw file from GitHub.
	rawURL := fmt.Sprintf(
		"https://raw.githubusercontent.com/easel/ddx-library/main/%ss/%s.md",
		resourceType, resourceName,
	)

	destFile := filepath.Join(target, resourceName+".md")
	if err := downloadFile(rawURL, destFile); err != nil {
		return entry, fmt.Errorf("downloading %s: %w", rawURL, err)
	}

	entry.Files = append(entry.Files, destFile)
	return entry, nil
}

// UninstallPackage removes files recorded in the entry.
func UninstallPackage(entry *InstalledEntry) error {
	var errs []string
	for _, f := range entry.Files {
		expanded := ExpandHome(f)
		if err := os.Remove(expanded); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("removing %s: %v", f, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("uninstall errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// githubTarballURL builds a GitHub release tarball URL from a repo URL and version tag.
// e.g. "https://github.com/owner/repo" + "1.0.0" →
//
//	"https://github.com/owner/repo/archive/refs/tags/v1.0.0.tar.gz"
func githubTarballURL(repoURL, version string) string {
	tag := version
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	return strings.TrimRight(repoURL, "/") + "/archive/refs/tags/" + tag + ".tar.gz"
}

// downloadAndExtract downloads a .tar.gz from url into destDir and returns
// the path of the single top-level directory extracted from the archive.
func downloadAndExtract(url, destDir string) (string, error) {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: HTTP %s", url, resp.Status)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	var topDir string

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar: %w", err)
		}

		// Sanitize path to prevent directory traversal.
		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "..") {
			continue
		}

		dest := filepath.Join(destDir, clean)

		// Skip PAX global headers (GitHub tarballs include these).
		if hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}

		// Track the top-level directory name.
		parts := strings.SplitN(clean, string(filepath.Separator), 2)
		if topDir == "" && parts[0] != "" && parts[0] != "." {
			topDir = parts[0]
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0755); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return "", err
			}
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode())
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return "", err
			}
			_ = f.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return "", err
			}
			_ = os.Remove(dest)
			if err := os.Symlink(hdr.Linkname, dest); err != nil {
				return "", err
			}
		}
	}

	if topDir == "" {
		return destDir, nil
	}
	return filepath.Join(destDir, topDir), nil
}

// symlinkSkills creates symlinks in the target skill directory pointing to the
// corresponding entries in the installed plugin root. This keeps skills in sync
// with the plugin rather than creating independent copies.
//
// The source entries may themselves be symlinks (e.g. HELIX's
// .agents/skills/helix-align -> ../../skills/helix-align). We resolve through
// all symlinks using filepath.EvalSymlinks so the output symlinks point to
// real directories, not to intermediate symlinks that may contain stale or
// Docker-only paths.
func symlinkSkills(installedRoot string, skill *InstallMapping) ([]string, error) {
	srcDir := filepath.Join(installedRoot, filepath.FromSlash(skill.Source))
	dstDir := ExpandHome(skill.Target)

	entries, err := os.ReadDir(srcDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading skills dir %s: %w", srcDir, err)
	}

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return nil, fmt.Errorf("creating skills dir %s: %w", dstDir, err)
	}

	allowed := make(map[string]bool, len(entries))
	for _, e := range entries {
		allowed[e.Name()] = true
	}
	if err := pruneStaleSkillLinks(installedRoot, dstDir, allowed); err != nil {
		return nil, err
	}

	var written []string
	for _, e := range entries {
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(dstDir, e.Name())

		// Resolve through all symlinks to get the real, absolute target
		// path. This prevents broken symlinks when:
		//  - the source entry is itself a relative symlink
		//  - the target directory is outside the project (e.g. ~/.claude/skills/)
		//
		// GitHub tarballs resolve relative symlinks to absolute paths from
		// the build machine (e.g. ../../skills/helix-align becomes
		// /home/user/Projects/helix/skills/helix-align). These are broken
		// on any other machine. When EvalSymlinks fails, read the link
		// target and try to resolve it relative to the installed root.
		realSrc, err := filepath.EvalSymlinks(src)
		if err != nil {
			// Try to recover broken symlinks from tarballs.
			linkTarget, readErr := os.Readlink(src)
			if readErr != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("resolving symlinks for %s: %w", src, err)
			}

			// The link target may be absolute (broken from tarball) or relative.
			// For relative targets like ../../skills/helix-align, resolve
			// against the directory containing the symlink.
			if !filepath.IsAbs(linkTarget) {
				resolved := filepath.Join(filepath.Dir(src), linkTarget)
				resolved = filepath.Clean(resolved)
				if info, statErr := os.Stat(resolved); statErr == nil && info.IsDir() {
					realSrc = resolved
				}
			}
			// If still unresolved, try matching the basename in the installed
			// root's skills/ directory (the real location).
			if realSrc == "" {
				candidate := filepath.Join(installedRoot, "skills", e.Name())
				if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
					realSrc = candidate
				}
			}
			if realSrc == "" {
				// Truly broken — skip this entry.
				continue
			}
		}
		realSrc, err = filepath.Abs(realSrc)
		if err != nil {
			return nil, fmt.Errorf("resolving absolute path %s: %w", realSrc, err)
		}

		// Remove existing file/symlink/directory.
		if _, err := os.Lstat(dst); err == nil {
			if err := os.RemoveAll(dst); err != nil {
				return nil, fmt.Errorf("removing existing %s: %w", dst, err)
			}
		}

		if err := os.Symlink(realSrc, dst); err != nil {
			return nil, fmt.Errorf("symlinking %s -> %s: %w", dst, realSrc, err)
		}
		written = append(written, dst)
	}
	return written, nil
}

func pruneStaleSkillLinks(installedRoot, dstDir string, allowed map[string]bool) error {
	if installedRoot == "" {
		return nil
	}

	entries, err := os.ReadDir(dstDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading skills dir %s: %w", dstDir, err)
	}

	absRoot, err := filepath.Abs(installedRoot)
	if err != nil {
		absRoot = installedRoot
	}

	for _, e := range entries {
		name := e.Name()
		if allowed[name] {
			continue
		}

		dstPath := filepath.Join(dstDir, name)
		info, err := os.Lstat(dstPath)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}

		target, err := resolveSymlinkTarget(dstPath)
		if err != nil {
			continue
		}
		if !isWithinRoot(target, absRoot) {
			continue
		}

		if err := os.RemoveAll(dstPath); err != nil {
			return fmt.Errorf("removing stale skill link %s: %w", dstPath, err)
		}
	}
	return nil
}

func resolveSymlinkTarget(path string) (string, error) {
	target, err := filepath.EvalSymlinks(path)
	if err == nil {
		if abs, absErr := filepath.Abs(target); absErr == nil {
			return abs, nil
		}
		return target, nil
	}

	linkTarget, readErr := os.Readlink(path)
	if readErr != nil {
		return "", err
	}

	if !filepath.IsAbs(linkTarget) {
		linkTarget = filepath.Join(filepath.Dir(path), linkTarget)
	}
	linkTarget = filepath.Clean(linkTarget)
	if abs, absErr := filepath.Abs(linkTarget); absErr == nil {
		linkTarget = abs
	}
	return linkTarget, nil
}

func isWithinRoot(target, root string) bool {
	if root == "" || target == "" {
		return false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// SymlinkSkillsFromRoot creates skill symlinks from an installed plugin root.
// Exported for use by local install path.
func SymlinkSkillsFromRoot(installedRoot string, skill *InstallMapping) ([]string, error) {
	return symlinkSkills(installedRoot, skill)
}

// CopyScriptFromRoot copies a script from an installed plugin root to the
// target path. Exported for use by local install path.
func CopyScriptFromRoot(installedRoot string, mapping *InstallMapping) (string, error) {
	files, err := copyMapping(installedRoot, mapping)
	if err != nil {
		return "", err
	}
	// Ensure executable.
	dst := ExpandHome(mapping.Target)
	if info, err := os.Stat(dst); err == nil && !info.IsDir() {
		_ = os.Chmod(dst, info.Mode()|0111)
	}
	if len(files) > 0 {
		return files[0], nil
	}
	return dst, nil
}

// copyMapping copies files from srcDir/<mapping.Source> to ExpandHome(mapping.Target).
// If the source is a single file and the target does not end with a path
// separator, the target is treated as the exact destination file path.
// If the source is a directory (or target ends with /), files are copied
// into the target directory.
// Returns the list of destination files written.
func copyMapping(srcDir string, mapping *InstallMapping) ([]string, error) {
	src := filepath.Join(srcDir, filepath.FromSlash(mapping.Source))
	dst := ExpandHome(mapping.Target)

	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		// Source path doesn't exist in this repo — skip silently.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var written []string

	if info.IsDir() {
		// If dst is an existing symlink (e.g. from a prior --local install),
		// remove it before creating the real directory.
		if li, err := os.Lstat(dst); err == nil && li.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(dst); err != nil {
				return nil, fmt.Errorf("removing symlink at %s: %w", dst, err)
			}
		}

		// Copy directory tree into dst (create dst as a directory).
		if err := os.MkdirAll(dst, 0755); err != nil {
			return nil, fmt.Errorf("creating target dir %s: %w", dst, err)
		}

		// HELIX skills use symlinks, so resolve each entry via os.Stat.
		entries, err := os.ReadDir(src)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			srcPath := filepath.Join(src, e.Name())
			dstPath := filepath.Join(dst, e.Name())

			fi, err := os.Stat(srcPath)
			if err != nil {
				continue // skip broken symlinks
			}

			if fi.IsDir() {
				subFiles, subErr := copyMapping(srcPath, &InstallMapping{Source: ".", Target: dstPath})
				if subErr != nil {
					return nil, subErr
				}
				written = append(written, subFiles...)
			} else if fi.Mode().IsRegular() {
				if err := copyFile(srcPath, dstPath); err != nil {
					return nil, err
				}
				written = append(written, dstPath)
			}
		}
	} else {
		// Source is a single file. If target ends with /, copy into that
		// directory using the source filename; otherwise treat target as
		// the exact destination file path.
		var dstFile string
		if strings.HasSuffix(mapping.Target, "/") {
			if err := os.MkdirAll(dst, 0755); err != nil {
				return nil, fmt.Errorf("creating target dir %s: %w", dst, err)
			}
			dstFile = filepath.Join(dst, filepath.Base(src))
		} else {
			dstFile = dst
		}
		if err := copyFile(src, dstFile); err != nil {
			return nil, err
		}
		written = append(written, dstFile)
	}

	return written, nil
}

// copyFile copies src to dst, creating parent directories as needed.
// Preserves the source file's permissions. If dst already exists (file,
// symlink, or directory), it is removed first.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// Remove any existing file/symlink/directory at dst.
	if _, err := os.Lstat(dst); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("removing existing %s: %w", dst, err)
		}
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

// downloadFile fetches url and writes it to dest.
func downloadFile(url, dest string) error {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("fetching %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: HTTP %s", url, resp.Status)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, resp.Body)
	return err
}

// ExpandHome expands a leading ~ to the user's home directory.
func ExpandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
