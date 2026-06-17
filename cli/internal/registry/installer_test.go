package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	iofs "io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/registry/defaultplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyInstallPackageHonorsSkillSource(t *testing.T) {
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

func TestLegacyInstallPackagePreservesUnrelatedProjectSkills(t *testing.T) {
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

func TestLegacyInstallPackageRecordsManifestSkillTargets(t *testing.T) {
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

// TestLegacyInstallPackageFromFSHonorsManifestMappings proves that the
// compatibility copy installer reads package.yaml and applies both the root
// mapping and the declared skill mappings when fed an embedded fs.FS.
func TestLegacyInstallPackageFromFSHonorsManifestMappings(t *testing.T) {
	projectRoot := t.TempDir()
	pkg := sampleInstallPackage("https://example.com/sample-plugin")

	entry, err := InstallPackageFromFS(pkg, samplePackageFS(), projectRoot)
	require.NoError(t, err)

	// Legacy root mapping: package.yaml from the embedded FS lands under the
	// manifest target. Marketplace installs use the cache/sync path instead.
	rootManifest := filepath.Join(projectRoot, ddxroot.DirName, "plugins", "sample-plugin", "package.yaml")
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

// TestLegacyInstallPackageFromFSInstallsDefaultDDxPackageOffline proves the
// embedded default ddx package remains usable through the compatibility copy
// installer without invoking network download code.
func TestLegacyInstallPackageFromFSInstallsDefaultDDxPackageOffline(t *testing.T) {
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
		filepath.Join(".agents", "skills", "ddx", "SKILL.md"),
		filepath.Join(".claude", "skills", "ddx", "SKILL.md"),
	} {
		path := filepath.Join(projectRoot, rel)
		_, statErr := os.Stat(path)
		require.NoError(t, statErr, "expected embedded default package to install %s", rel)
	}
	assert.NoDirExists(t, filepath.Join(projectRoot, ddxroot.DirName, "plugins", "ddx"),
		"embedded default package install must not create a project payload root")

	// The recorded file list must include the skill targets so the install
	// state can be uninstalled/reverified later.
	for _, rel := range []string{
		filepath.Join(".agents", "skills", "ddx", "SKILL.md"),
		filepath.Join(".claude", "skills", "ddx", "SKILL.md"),
	} {
		assert.Contains(t, entry.Files, rel)
	}
}

// TestLegacyInstallPackageFromRemoteUsesSharedCore proves the remote and
// embedded-FS compatibility entrypoints share the same copy behavior for a
// fixture package: same recorded files, same on-disk layout.
func TestLegacyInstallPackageFromRemoteUsesSharedCore(t *testing.T) {
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

// TestDefaultDDxPackageManifestUsesPackageLocalSkillSource proves the
// canonical library/package.yaml declares package-local skill sources
// (i.e. source: skills/) so the embedded default-package install copies
// the DDx skill out of library/skills/ rather than out of a checked-in
// .agents/skills mirror. It must not declare a root payload target; the
// built-in DDx package is cache-only plus generated adapters.
func TestDefaultDDxPackageManifestUsesPackageLocalSkillSource(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "..", "library", "package.yaml")
	pkg, issues, err := LoadPackageManifest(filepath.Dir(manifestPath))
	require.NoError(t, err, "library/package.yaml must load cleanly")
	require.Empty(t, issues, "library/package.yaml must have no schema issues")
	require.NotNil(t, pkg)

	assert.Nil(t, pkg.Install.Root, "library/package.yaml must not advertise a DDx project payload root")
	require.NotEmpty(t, pkg.Install.Skills, "library/package.yaml must declare install.skills mappings")
	for _, m := range pkg.Install.Skills {
		assert.Equal(t, "skills/", m.Source,
			"library/package.yaml install.skills[*].source must be package-local 'skills/', got %q", m.Source)
	}
}

func TestInstallResourceIsRetired(t *testing.T) {
	entry, err := InstallResource("persona/strict-code-reviewer")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "individual resource installs are retired")
	assert.Empty(t, entry.Files)
}

// TestDefaultPackageEmbedCopyIncludesDDxSkill proves the embedded
// default-package tree used by the binary actually carries the canonical
// library/skills/ddx/SKILL.md so offline `ddx init` installs a real skill.
func TestDefaultPackageEmbedCopyIncludesDDxSkill(t *testing.T) {
	data, err := iofs.ReadFile(defaultplugin.FS(), "skills/ddx/SKILL.md")
	require.NoError(t, err, "embedded default package must include skills/ddx/SKILL.md")
	require.NotEmpty(t, data, "embedded skills/ddx/SKILL.md must not be empty")

	canonical, err := os.ReadFile(filepath.Join("..", "..", "..", "library", "skills", "ddx", "SKILL.md"))
	require.NoError(t, err, "canonical library/skills/ddx/SKILL.md must exist on disk")
	assert.Equal(t, string(canonical), string(data),
		"embedded skills/ddx/SKILL.md must match the canonical library/skills/ddx/SKILL.md (run `make copy-skills` to sync)")
}

func TestBuiltinDDxCacheReadyRequiresOnlyBootstrapSkill(t *testing.T) {
	cachePath := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(cachePath, "skills", "ddx"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "package.yaml"), []byte("name: ddx\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "skills", "ddx", "SKILL.md"), []byte("---\nname: ddx\n---\n"), 0o644))

	require.True(t, BuiltinDDxCacheReady(cachePath), "minimal bootstrap package should make built-in ddx cache ready")
	require.NoDirExists(t, filepath.Join(cachePath, "personas"))
	require.NoDirExists(t, filepath.Join(cachePath, "prompts"))
	require.NoDirExists(t, filepath.Join(cachePath, "templates"))
}

// offlineTransport fails any HTTP attempt so tests can prove an install path
// did not reach the network.
type offlineTransport struct{ t *testing.T }

func (o *offlineTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	o.t.Helper()
	o.t.Fatalf("unexpected HTTP request during offline install: %s %s", req.Method, req.URL.String())
	return nil, nil
}
