package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
