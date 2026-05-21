package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDDXVersionCommit(t *testing.T) {
	output := "DDx v0.6.2-alpha48\nCommit: 8274fc381\nBuilt: 2026-05-12T18:37:34Z\n"

	assert.Equal(t, "8274fc381", parseDDXVersionCommit(output))
}

func TestShouldRefreshDDXBinary(t *testing.T) {
	assert.True(t, shouldRefreshDDXBinary("528f77644", "8274fc381"))
	assert.False(t, shouldRefreshDDXBinary("8274fc381", "8274fc381"))
	assert.False(t, shouldRefreshDDXBinary("8274fc381abcdef", "8274fc381"))
	assert.False(t, shouldRefreshDDXBinary("dev", "8274fc381"))
	assert.False(t, shouldRefreshDDXBinary("8274fc381", "unknown"))
}

func TestWorkBinaryRefreshEnabledOnlyForInstalledDDX(t *testing.T) {
	assert.True(t, workBinaryRefreshEnabled([]string{"/home/erik/.local/bin/ddx", "work"}))
	assert.False(t, workBinaryRefreshEnabled([]string{"/tmp/go-build123/cmd.test", "-test.run=TestWork"}))
	assert.False(t, workBinaryRefreshEnabled(nil))
}

// TestBinarySnapshotSurvivesBeadCommitsDetectsReinstall verifies the signal the
// worker now uses to decide whether to hand off: a bead-only commit never
// rewrites the binary (snapshot unchanged → worker stays put), while a real
// reinstall rewrites it (snapshot differs → worker hands off). See
// ddx-65d3ba51.
func TestBinarySnapshotSurvivesBeadCommitsDetectsReinstall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ddx")
	require.NoError(t, os.WriteFile(path, []byte("ddx-binary-v1"), 0o755))

	base, ok := snapshotBinary(path)
	require.True(t, ok)

	// A `ddx bead create` advances the ddx repo HEAD but does not touch the
	// worker's binary: the snapshot is unchanged, so the worker must not exit.
	unchanged, ok := snapshotBinary(path)
	require.True(t, ok)
	assert.True(t, base.equal(unchanged), "unchanged binary must compare equal so the worker survives bead commits")

	// An intentional reinstall rewrites the binary: the snapshot differs, so the
	// worker still hands off to the new build.
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, os.WriteFile(path, []byte("ddx-binary-v2-reinstalled"), 0o755))
	reinstalled, ok := snapshotBinary(path)
	require.True(t, ok)
	assert.False(t, base.equal(reinstalled), "a replaced binary must compare not-equal so the worker upgrades")

	// A missing binary yields no snapshot; the worker stays put rather than dying.
	_, ok = snapshotBinary(filepath.Join(dir, "absent"))
	assert.False(t, ok)
}
