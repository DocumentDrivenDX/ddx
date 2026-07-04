package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFakeBackendTool(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nexit 0\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
}

func writeStoreConfig(t *testing.T, dir string, backend string) {
	t.Helper()
	ddxDir := filepath.Join(dir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	content := fmt.Sprintf(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
bead:
  backend: %s
`, backend)
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(content), 0o644))
}

func TestNewStore_DefaultsToJSONL(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, s.Init(context.Background()))

	assert.Equal(t, DefaultCollection, s.Collection)
	_, err := os.Stat(s.File)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, ddxroot.DirName, "axon"))
	assert.Error(t, err)
}

func TestNewStore_SelectsAxonFromConfig(t *testing.T) {
	tempDir := t.TempDir()
	writeStoreConfig(t, tempDir, BackendAxon)

	s := NewStore(filepath.Join(tempDir, ddxroot.DirName))
	require.NoError(t, s.Init(context.Background()))

	_, err := os.Stat(filepath.Join(tempDir, ddxroot.DirName, "axon"))
	require.NoError(t, err)
}

func TestExternalBackendCarriesLogicalCollectionName(t *testing.T) {
	toolDir := t.TempDir()
	writeFakeBackendTool(t, toolDir, "bd")
	writeFakeBackendTool(t, toolDir, "br")
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, tc := range []struct {
		name       string
		backend    string
		collection string
	}{
		{name: "default-bd", backend: "bd", collection: DefaultCollection},
		{name: "exec-runs-bd", backend: "bd", collection: "exec-runs"},
		{name: "agent-sessions-br", backend: "br", collection: "agent-sessions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DDX_BEAD_BACKEND", tc.backend)
			s := NewStore(filepath.Join(t.TempDir(), ddxroot.DirName), WithCollection(tc.collection))
			require.Equal(t, tc.collection, s.Collection)
			require.NoError(t, s.Init(context.Background()))
			assert.Contains(t, s.File, filepath.Join(ddxroot.DirName, tc.collection+".jsonl"))
		})
	}
}

func TestExternalBackendOpensBeadsArchiveWithFallback(t *testing.T) {
	toolDir := t.TempDir()
	failScript := "#!/bin/sh\necho 'fake bd should not be called for non-default collections' 1>&2\nexit 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(toolDir, "bd"), []byte(failScript), 0o755))
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DDX_BEAD_BACKEND", "bd")

	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ddxroot.DirName)
	s := NewStoreWithCollection(ddxDir, BeadsArchiveCollection)
	require.NoError(t, s.Init(context.Background()))

	want := Bead{ID: "ddx-arch-1", Title: "archived item", Status: StatusClosed, IssueType: "task", Priority: 2}
	require.NoError(t, s.WriteAll([]Bead{want}))

	got, err := s.ReadAll(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, want.ID, got[0].ID)
	assert.Equal(t, want.Title, got[0].Title)
}
