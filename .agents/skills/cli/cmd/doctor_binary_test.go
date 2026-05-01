package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckBinaryInstallLocationDetectsStaleCopy verifies that a fake ddx binary
// shadowed earlier on PATH (with different content/SHA-256) is reported as stale.
func TestCheckBinaryInstallLocationDetectsStaleCopy(t *testing.T) {
	runDir := t.TempDir()
	staleDir := t.TempDir()

	// "Running" binary.
	runningBin := filepath.Join(runDir, "ddx")
	require.NoError(t, os.WriteFile(runningBin, []byte("#!/bin/sh\necho running\n"), 0o755))

	// Stale binary placed earlier on PATH with different content → different SHA-256.
	staleBin := filepath.Join(staleDir, "ddx")
	require.NoError(t, os.WriteFile(staleBin, []byte("#!/bin/sh\necho stale\n"), 0o755))

	// Isolate PATH to only the test dirs so real ddx copies on the host don't interfere.
	t.Setenv("PATH", staleDir+string(os.PathListSeparator)+runDir)

	issues := checkBinaryInstallLocation(runningBin)

	var mismatch []DiagnosticIssue
	for _, issue := range issues {
		if issue.Type == "binary_sha_mismatch" {
			mismatch = append(mismatch, issue)
		}
	}
	require.Len(t, mismatch, 1, "expected exactly one sha_mismatch issue")
	assert.Contains(t, mismatch[0].Description, staleBin)
	assert.Contains(t, mismatch[0].Remediation[0], "rm")
	assert.Contains(t, mismatch[0].Remediation[0], staleBin)
	assert.Contains(t, mismatch[0].Remediation[0], runningBin)
}

// TestCheckBinaryInstallLocationNoFalsePositiveForSameSHA verifies that a copy of
// the running binary with an identical SHA-256 is not flagged as stale.
func TestCheckBinaryInstallLocationNoFalsePositiveForSameSHA(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	content := []byte("#!/bin/sh\necho ddx\n")
	bin1 := filepath.Join(dir1, "ddx")
	bin2 := filepath.Join(dir2, "ddx")
	require.NoError(t, os.WriteFile(bin1, content, 0o755))
	require.NoError(t, os.WriteFile(bin2, content, 0o755))

	t.Setenv("PATH", dir1+string(os.PathListSeparator)+dir2)

	issues := checkBinaryInstallLocation(bin1)

	for _, issue := range issues {
		assert.NotEqual(t, "binary_sha_mismatch", issue.Type,
			"same-SHA copy should not be flagged as stale")
	}
}

// TestCheckBinaryInstallLocationCanonicalPath verifies that a binary at
// $HOME/.local/bin/ddx does not produce a binary_not_canonical issue.
func TestCheckBinaryInstallLocationCanonicalPath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	canonicalDir := filepath.Join(homeDir, ".local", "bin")
	require.NoError(t, os.MkdirAll(canonicalDir, 0o755))
	canonicalBin := filepath.Join(canonicalDir, "ddx")
	require.NoError(t, os.WriteFile(canonicalBin, []byte("#!/bin/sh\necho ddx\n"), 0o755))

	t.Setenv("PATH", canonicalDir)

	issues := checkBinaryInstallLocation(canonicalBin)

	for _, issue := range issues {
		assert.NotEqual(t, "binary_not_canonical", issue.Type,
			"binary at canonical location should not produce a not_canonical issue")
	}
}

// TestCheckBinaryInstallLocationNonCanonicalWarns verifies that running from a
// non-canonical path produces a binary_not_canonical issue.
func TestCheckBinaryInstallLocationNonCanonicalWarns(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	otherDir := t.TempDir()
	otherBin := filepath.Join(otherDir, "ddx")
	require.NoError(t, os.WriteFile(otherBin, []byte("#!/bin/sh\necho ddx\n"), 0o755))

	t.Setenv("PATH", otherDir)

	issues := checkBinaryInstallLocation(otherBin)

	var notCanonical []DiagnosticIssue
	for _, issue := range issues {
		if issue.Type == "binary_not_canonical" {
			notCanonical = append(notCanonical, issue)
		}
	}
	require.Len(t, notCanonical, 1, "expected one binary_not_canonical issue")
	assert.Contains(t, notCanonical[0].Description, otherBin)
}
