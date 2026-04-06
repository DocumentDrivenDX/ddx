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

	// Process Root mapping first - copy the entire plugin to central location
	if pkg.Install.Root != nil {
		files, err := copyMapping(extractedDir, pkg.Install.Root)
		if err != nil {
			return entry, fmt.Errorf("installing plugin root: %w", err)
		}
		entry.Files = append(entry.Files, files...)
	}

	// Process Skills mappings (one per target directory)
	for i := range pkg.Install.Skills {
		files, err := copyMapping(extractedDir, &pkg.Install.Skills[i])
		if err != nil {
			return entry, fmt.Errorf("installing skills: %w", err)
		}
		entry.Files = append(entry.Files, files...)
	}

	// Process Scripts mapping (for CLI binaries)
	if pkg.Install.Scripts != nil {
		files, err := copyMapping(extractedDir, pkg.Install.Scripts)
		if err != nil {
			return entry, fmt.Errorf("installing scripts: %w", err)
		}
		entry.Files = append(entry.Files, files...)
	}

	// Process symlinks.
	for _, sym := range pkg.Install.Symlinks {
		src := expandHome(sym.Source)
		dst := expandHome(sym.Target)

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
// from the ddx-library GitHub repo into the local .ddx/library/<type>/ directory.
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
	target := filepath.Join(".ddx", "library", resourceType+"s")
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
		expanded := expandHome(f)
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

// copyMapping copies files from srcDir/<mapping.Source> to expandHome(mapping.Target).
// If the source is a single file and the target does not end with a path
// separator, the target is treated as the exact destination file path.
// If the source is a directory (or target ends with /), files are copied
// into the target directory.
// Returns the list of destination files written.
func copyMapping(srcDir string, mapping *InstallMapping) ([]string, error) {
	src := filepath.Join(srcDir, filepath.FromSlash(mapping.Source))
	dst := expandHome(mapping.Target)

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
// If dst already exists (file, symlink, or directory), it is removed first.
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

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
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

// expandHome expands a leading ~ to the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
