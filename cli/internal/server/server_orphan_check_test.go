package server

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerWarnsAboutOrphanHomeServerState(t *testing.T) {
	workingDir := t.TempDir()
	fakeHome := t.TempDir()

	// Create in-tree .ddx so active state root is workingDir/.ddx, not fakeHome/.ddx.
	require.NoError(t, os.MkdirAll(ddxroot.InTree(workingDir), 0o755))
	// Create the orphaned home server state directory.
	orphanPath := ddxroot.JoinHome(fakeHome, "server")
	require.NoError(t, os.MkdirAll(orphanPath, 0o755))

	prevDir := orphanHomeStateDirFn
	orphanHomeStateDirFn = func() (string, error) { return fakeHome, nil }
	t.Cleanup(func() { orphanHomeStateDirFn = prevDir })

	var buf bytes.Buffer
	prevWriter := orphanHomeStateWarnWriter
	orphanHomeStateWarnWriter = &buf
	t.Cleanup(func() { orphanHomeStateWarnWriter = prevWriter })

	checkOrphanHomeState(workingDir)

	warn := buf.String()
	assert.True(t, strings.Contains(warn, orphanPath),
		"warning should name orphaned path %s; got: %s", orphanPath, warn)
	activeRoot := ddxroot.InTree(workingDir)
	assert.True(t, strings.Contains(warn, activeRoot),
		"warning should name active state root %s; got: %s", activeRoot, warn)
}

func TestServerWarnsAboutOrphanHomeTSNetState(t *testing.T) {
	workingDir := t.TempDir()
	fakeHome := t.TempDir()

	// Create in-tree .ddx so active state root is workingDir/.ddx, not fakeHome/.ddx.
	require.NoError(t, os.MkdirAll(ddxroot.InTree(workingDir), 0o755))
	// Create the orphaned home tsnet state directory.
	orphanPath := ddxroot.JoinHome(fakeHome, "tsnet")
	require.NoError(t, os.MkdirAll(orphanPath, 0o755))

	prevDir := orphanHomeStateDirFn
	orphanHomeStateDirFn = func() (string, error) { return fakeHome, nil }
	t.Cleanup(func() { orphanHomeStateDirFn = prevDir })

	var buf bytes.Buffer
	prevWriter := orphanHomeStateWarnWriter
	orphanHomeStateWarnWriter = &buf
	t.Cleanup(func() { orphanHomeStateWarnWriter = prevWriter })

	checkOrphanHomeState(workingDir)

	warn := buf.String()
	assert.True(t, strings.Contains(warn, orphanPath),
		"warning should name orphaned path %s; got: %s", orphanPath, warn)
	activeRoot := ddxroot.InTree(workingDir)
	assert.True(t, strings.Contains(warn, activeRoot),
		"warning should name active state root %s; got: %s", activeRoot, warn)
}

func TestServerDoesNotWarnWhenNoOrphanHomeState(t *testing.T) {
	workingDir := t.TempDir()
	fakeHome := t.TempDir()

	require.NoError(t, os.MkdirAll(ddxroot.InTree(workingDir), 0o755))
	// No ~/.ddx/server or ~/.ddx/tsnet created.

	prevDir := orphanHomeStateDirFn
	orphanHomeStateDirFn = func() (string, error) { return fakeHome, nil }
	t.Cleanup(func() { orphanHomeStateDirFn = prevDir })

	var buf bytes.Buffer
	prevWriter := orphanHomeStateWarnWriter
	orphanHomeStateWarnWriter = &buf
	t.Cleanup(func() { orphanHomeStateWarnWriter = prevWriter })

	checkOrphanHomeState(workingDir)

	assert.Empty(t, buf.String())
}
