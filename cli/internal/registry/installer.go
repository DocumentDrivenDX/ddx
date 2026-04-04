package registry

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// InstallPackage clones the source repo and copies declared install mappings.
// It records installed files in the returned InstalledEntry.
func InstallPackage(pkg *Package) (InstalledEntry, error) {
	entry := InstalledEntry{
		Name:        pkg.Name,
		Version:     pkg.Version,
		Type:        pkg.Type,
		Source:      pkg.Source,
		InstalledAt: time.Now(),
	}

	// Shallow-clone the source repo to a temp directory.
	tmpDir, err := os.MkdirTemp("", "ddx-install-"+pkg.Name+"-*")
	if err != nil {
		return entry, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := shallowClone(pkg.Source, tmpDir); err != nil {
		return entry, fmt.Errorf("cloning %s: %w", pkg.Source, err)
	}

	// Process install mappings.
	if pkg.Install.Skills != nil {
		files, err := copyMapping(tmpDir, pkg.Install.Skills)
		if err != nil {
			return entry, fmt.Errorf("installing skills: %w", err)
		}
		entry.Files = append(entry.Files, files...)
	}

	if pkg.Install.Scripts != nil {
		files, err := copyMapping(tmpDir, pkg.Install.Scripts)
		if err != nil {
			return entry, fmt.Errorf("installing scripts: %w", err)
		}
		entry.Files = append(entry.Files, files...)
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

// shallowClone performs a shallow git clone of url into dir.
func shallowClone(url, dir string) error {
	cmd := exec.Command("git", "clone", "--depth=1", url, dir)
	cmd.Stdout = os.Stderr // progress output to stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// copyMapping copies files from srcDir/<mapping.Source> to expandHome(mapping.Target).
// Returns the list of destination files written.
func copyMapping(srcDir string, mapping *InstallMapping) ([]string, error) {
	src := filepath.Join(srcDir, filepath.FromSlash(mapping.Source))
	dst := expandHome(mapping.Target)

	if err := os.MkdirAll(dst, 0755); err != nil {
		return nil, fmt.Errorf("creating target dir %s: %w", dst, err)
	}

	var written []string

	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		// Source path doesn't exist in this repo — skip silently.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		// Copy directory tree. HELIX skills use symlinks, so we resolve each
		// entry via os.Stat (follows symlinks) rather than os.Lstat.
		entries, err := os.ReadDir(src)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			srcPath := filepath.Join(src, e.Name())
			dstPath := filepath.Join(dst, e.Name())

			// Stat follows symlinks, giving us the real target info.
			fi, err := os.Stat(srcPath)
			if err != nil {
				continue // skip broken symlinks
			}

			if fi.IsDir() {
				// Recurse into directory (or symlink-to-directory).
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
		// Source is a single file.
		dstFile := filepath.Join(dst, filepath.Base(src))
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
	// Use curl or wget as a simple approach; avoids importing net/http for now.
	cmd := exec.Command("curl", "-fsSL", "-o", dest, url)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("curl %s: %w", url, err)
	}
	return nil
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
