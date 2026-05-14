package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DocumentDrivenDX/ddx/internal/registry/defaultplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallPackageHonorsSkillSource(t *testing.T) {
	projectRoot := t.TempDir()
	server := newPackageArchiveServer(t, samplePackageTarball(t))
	defer server.Close()

	pkg := sampleInstallPackage(server.URL)
	entry, err := InstallPackage(pkg, projectRoot)
	require.NoError(t, err)

	for _, rel := range []string{
		filepath.Join(".agents", "skills", "ddx", "SKILL.md"),
		filepath.Join(".claude", "skills", "ddx", "SKILL.md"),
	} {
		path := filepath.Join(projectRoot, rel)
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.Contains(t, string(data), "ddx")
	}

	for _, rel := range []string{
		filepath.Join(".agents", "skills", "ddx", "SKILL.md"),
		filepath.Join(".claude", "skills", "ddx", "SKILL.md"),
	} {
		assert.Contains(t, entry.Files, rel)
	}
}

func TestInstallPackagePreservesUnrelatedProjectSkills(t *testing.T) {
	projectRoot := t.TempDir()
	server := newPackageArchiveServer(t, samplePackageTarball(t))
	defer server.Close()

	for _, rel := range []string{
		filepath.Join(".agents", "skills", "existing-skill", "SKILL.md"),
		filepath.Join(".claude", "skills", "existing-skill", "SKILL.md"),
	} {
		path := filepath.Join(projectRoot, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("keep me"), 0o644))
	}

	_, err := InstallPackage(sampleInstallPackage(server.URL), projectRoot)
	require.NoError(t, err)

	for _, rel := range []string{
		filepath.Join(".agents", "skills", "existing-skill", "SKILL.md"),
		filepath.Join(".claude", "skills", "existing-skill", "SKILL.md"),
	} {
		path := filepath.Join(projectRoot, rel)
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.Equal(t, "keep me", string(data))
	}
}

func TestInstallPackageRecordsManifestSkillTargets(t *testing.T) {
	projectRoot := t.TempDir()
	server := newPackageArchiveServer(t, samplePackageTarball(t))
	defer server.Close()

	entry, err := InstallPackage(sampleInstallPackage(server.URL), projectRoot)
	require.NoError(t, err)

	for _, rel := range []string{
		filepath.Join(".agents", "skills", "ddx", "SKILL.md"),
		filepath.Join(".claude", "skills", "ddx", "SKILL.md"),
	} {
		assert.Contains(t, entry.Files, rel)
	}
}

func sampleInstallPackage(source string) *Package {
	return &Package{
		Name:    "sample-plugin",
		Version: "1.0.0",
		Type:    PackageTypePlugin,
		Source:  source,
		Install: PackageInstall{
			Root: &InstallMapping{
				Source: ".",
				Target: ".ddx/plugins/sample-plugin",
			},
			Skills: []InstallMapping{
				{Source: "skills/", Target: ".agents/skills/"},
				{Source: "skills/", Target: ".claude/skills/"},
			},
		},
	}
}

func samplePackageTarball(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	topDir := "sample-plugin-1.0.0"
	writeTarDir(t, tw, topDir)
	writeTarFile(t, tw, filepath.Join(topDir, "package.yaml"), []byte(`name: sample-plugin
version: 1.0.0
description: Sample package
type: plugin
source: https://example.com/sample-plugin
api_version: 1
install:
  root:
    source: .
    target: .ddx/plugins/sample-plugin
  skills:
    - source: skills/
      target: .agents/skills/
    - source: skills/
      target: .claude/skills/
`))
	writeTarDir(t, tw, filepath.Join(topDir, "skills"))
	writeTarDir(t, tw, filepath.Join(topDir, "skills", "ddx"))
	writeTarFile(t, tw, filepath.Join(topDir, "skills", "ddx", "SKILL.md"), []byte("---\nname: ddx\ndescription: sample\n---\n# ddx\n"))

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func newPackageArchiveServer(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/archive/refs/tags/v1.0.0.tar.gz") {
			t.Errorf("unexpected archive path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(payload)
	}))
}

func writeTarDir(t *testing.T, tw *tar.Writer, name string) {
	t.Helper()

	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     filepath.ToSlash(name) + "/",
		Mode:     0o755,
		Typeflag: tar.TypeDir,
	}))
}

func writeTarFile(t *testing.T, tw *tar.Writer, name string, body []byte) {
	t.Helper()

	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     filepath.ToSlash(name),
		Mode:     0o644,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write(body)
	require.NoError(t, err)
}

// samplePackageFS returns an in-memory fs.FS rooted at the package layout that
// the remote tarball produces, so both entrypoints exercise the same manifest
// and mappings.
func samplePackageFS() fstest.MapFS {
	return fstest.MapFS{
		"package.yaml": &fstest.MapFile{Data: []byte(`name: sample-plugin
version: 1.0.0
description: Sample package
type: plugin
source: https://example.com/sample-plugin
api_version: "1"
install:
  root:
    source: .
    target: .ddx/plugins/sample-plugin
  skills:
    - source: skills/
      target: .agents/skills/
    - source: skills/
      target: .claude/skills/
`)},
		"skills/ddx/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: ddx\ndescription: sample\n---\n# ddx\n")},
	}
}

// TestInstallPackageFromFSHonorsManifestMappings proves that an embedded
// fs.FS rooted at a package directory reads package.yaml and applies both
// the root mapping and the declared skill mappings.
func TestInstallPackageFromFSHonorsManifestMappings(t *testing.T) {
	projectRoot := t.TempDir()
	pkg := sampleInstallPackage("https://example.com/sample-plugin")

	entry, err := InstallPackageFromFS(pkg, samplePackageFS(), projectRoot)
	require.NoError(t, err)

	// Root mapping: package.yaml from the embedded FS lands under .ddx/plugins/<name>/.
	rootManifest := filepath.Join(projectRoot, ".ddx", "plugins", "sample-plugin", "package.yaml")
	_, statErr := os.Stat(rootManifest)
	require.NoError(t, statErr, "package.yaml should be installed via root mapping")

	// Skill mappings: each declared target gets the skill directory under it.
	for _, rel := range []string{
		filepath.Join(".agents", "skills", "ddx", "SKILL.md"),
		filepath.Join(".claude", "skills", "ddx", "SKILL.md"),
	} {
		path := filepath.Join(projectRoot, rel)
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr, "skill file should exist via manifest skill mapping: %s", rel)
		assert.Contains(t, string(data), "ddx")
		assert.Contains(t, entry.Files, rel)
	}
}

// TestInstallPackageFromFSInstallsDefaultDDxPackageOffline proves the embedded
// default ddx package can install .ddx/plugins/ddx, .agents/skills/ddx, and
// .claude/skills/ddx without invoking any network download code.
func TestInstallPackageFromFSInstallsDefaultDDxPackageOffline(t *testing.T) {
	projectRoot := t.TempDir()

	// Block network: any HTTP attempt should fail this test loudly.
	origTransport := http.DefaultTransport
	http.DefaultTransport = &offlineTransport{t: t}
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	pkg := &Package{
		Name:    "ddx",
		Version: "0.0.0",
		Type:    PackageTypePlugin,
		Source:  "https://example.com/ddx",
	}

	entry, err := InstallPackageFromFS(pkg, defaultplugin.FS(), projectRoot)
	require.NoError(t, err)

	for _, rel := range []string{
		filepath.Join(".ddx", "plugins", "ddx", "package.yaml"),
		filepath.Join(".agents", "skills", "ddx", "SKILL.md"),
		filepath.Join(".claude", "skills", "ddx", "SKILL.md"),
	} {
		path := filepath.Join(projectRoot, rel)
		_, statErr := os.Stat(path)
		require.NoError(t, statErr, "expected embedded default package to install %s", rel)
	}

	// The recorded file list must include the skill targets so the install
	// state can be uninstalled/reverified later.
	for _, rel := range []string{
		filepath.Join(".agents", "skills", "ddx", "SKILL.md"),
		filepath.Join(".claude", "skills", "ddx", "SKILL.md"),
	} {
		assert.Contains(t, entry.Files, rel)
	}
}

// TestInstallPackageFromRemoteUsesSharedCore proves the remote entrypoint
// and embedded-FS entrypoint share the same skill mapping behavior for a
// fixture package — same recorded files, same on-disk layout.
func TestInstallPackageFromRemoteUsesSharedCore(t *testing.T) {
	remoteRoot := t.TempDir()
	fsRoot := t.TempDir()

	server := newPackageArchiveServer(t, samplePackageTarball(t))
	defer server.Close()

	remoteEntry, err := InstallPackageFromRemote(sampleInstallPackage(server.URL), remoteRoot)
	require.NoError(t, err)

	fsEntry, err := InstallPackageFromFS(sampleInstallPackage(server.URL), samplePackageFS(), fsRoot)
	require.NoError(t, err)

	// Both entrypoints record the same project-relative install paths.
	remoteFiles := append([]string(nil), remoteEntry.Files...)
	fsFiles := append([]string(nil), fsEntry.Files...)
	sort.Strings(remoteFiles)
	sort.Strings(fsFiles)
	assert.Equal(t, remoteFiles, fsFiles, "remote and FS entrypoints should record identical install file lists")

	// And the same skill files actually exist under each project root.
	for _, rel := range []string{
		filepath.Join(".agents", "skills", "ddx", "SKILL.md"),
		filepath.Join(".claude", "skills", "ddx", "SKILL.md"),
	} {
		_, err := os.Stat(filepath.Join(remoteRoot, rel))
		require.NoError(t, err, "remote install missing %s", rel)
		_, err = os.Stat(filepath.Join(fsRoot, rel))
		require.NoError(t, err, "fs install missing %s", rel)
	}
}

// offlineTransport fails any HTTP attempt so tests can prove an install path
// did not reach the network.
type offlineTransport struct{ t *testing.T }

func (o *offlineTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	o.t.Helper()
	o.t.Fatalf("unexpected HTTP request during offline install: %s %s", req.Method, req.URL.String())
	return nil, nil
}
