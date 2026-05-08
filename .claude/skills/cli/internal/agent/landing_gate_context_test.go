package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newGateCtxRepo initializes a real git repo and writes a single tracked file
// so HEAD has a real SHA the helper can pin / check out.
func newGateCtxRepo(t *testing.T) (root, headSHA string) {
	t.Helper()
	root = t.TempDir()
	runGitInteg(t, root, "init", "-b", "main")
	runGitInteg(t, root, "config", "user.email", "test@ddx.test")
	runGitInteg(t, root, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0644))
	runGitInteg(t, root, "add", ".")
	runGitInteg(t, root, "commit", "-m", "chore: initial seed")
	headSHA = runGitInteg(t, root, "rev-parse", "HEAD")
	return root, headSHA
}

// writeManifest serializes a minimal manifest with the given governing IDs to
// projectRoot/.ddx/executions/<attempt>/manifest.json and returns the
// bundle-relative manifest path the way ExecuteBead would set it on the result.
func writeManifest(t *testing.T, projectRoot, attemptID string, governingIDs []string) string {
	t.Helper()
	dirRel := filepath.Join(".ddx", "executions", attemptID)
	dirAbs := filepath.Join(projectRoot, dirRel)
	require.NoError(t, os.MkdirAll(dirAbs, 0o755))

	type govRef struct {
		ID string `json:"id"`
	}
	body := map[string]any{
		"attempt_id": attemptID,
		"bead_id":    "ddx-gatectx-bead",
	}
	if len(governingIDs) > 0 {
		refs := make([]govRef, 0, len(governingIDs))
		for _, id := range governingIDs {
			refs = append(refs, govRef{ID: id})
		}
		body["governing"] = refs
	}
	raw, err := json.MarshalIndent(body, "", "  ")
	require.NoError(t, err)
	manifestRel := filepath.Join(dirRel, "manifest.json")
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, manifestRel), raw, 0o644))
	return manifestRel
}

// resolveRef returns the SHA the ref points at, or "" when missing.
func resolveRef(t *testing.T, root, ref string) string {
	t.Helper()
	out, err := runGitIntegOutput(root, "rev-parse", "--verify", ref)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func TestBuildLandingGateContext_NoGoverningIDs(t *testing.T) {
	root, head := newGateCtxRepo(t)
	manifestRel := writeManifest(t, root, "20260418T010101-empty", nil) // empty governing list

	res := &ExecuteBeadResult{
		BeadID:       "ddx-gatectx-bead",
		AttemptID:    "20260418T010101-empty",
		BaseRev:      head,
		ResultRev:    head,
		ManifestFile: manifestRel,
	}

	wt, ids, cleanup, err := BuildLandingGateContext(root, res, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	assert.Empty(t, wt, "wtPath must be empty when manifest declares no governing IDs")
	assert.Empty(t, ids, "govern IDs must be empty when manifest declares none")

	// pin ref must NOT have been created
	assert.Empty(t, resolveRef(t, root, "refs/ddx/gate-pins/ddx-gatectx-bead/20260418T010101-empty"))

	// cleanup must be safe to call (noop)
	cleanup()
}

func TestBuildLandingGateContext_PinAndCheckout(t *testing.T) {
	root, head := newGateCtxRepo(t)
	manifestRel := writeManifest(t, root, "20260418T020202-ok", []string{"FEAT-CTX"})

	res := &ExecuteBeadResult{
		BeadID:       "ddx-gatectx-bead",
		AttemptID:    "20260418T020202-ok",
		BaseRev:      head,
		ResultRev:    head,
		ManifestFile: manifestRel,
	}

	wt, ids, cleanup, err := BuildLandingGateContext(root, res, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	require.Equal(t, []string{"FEAT-CTX"}, ids)
	require.NotEmpty(t, wt, "wtPath must be set when governing IDs exist")
	assert.True(t, strings.Contains(filepath.Base(wt), "ddx-gate-wt-"),
		"temp worktree should be created with the documented prefix; got %s", wt)

	// pin ref must point at ResultRev
	pinSHA := resolveRef(t, root, "refs/ddx/gate-pins/ddx-gatectx-bead/20260418T020202-ok")
	assert.Equal(t, head, pinSHA, "pin ref must point at ResultRev")

	// temp worktree directory must exist on disk
	info, statErr := os.Stat(wt)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestBuildLandingGateContext_CleanupRollsBack(t *testing.T) {
	root, head := newGateCtxRepo(t)
	manifestRel := writeManifest(t, root, "20260418T030303-cleanup", []string{"FEAT-CLEANUP"})

	res := &ExecuteBeadResult{
		BeadID:       "ddx-gatectx-bead",
		AttemptID:    "20260418T030303-cleanup",
		BaseRev:      head,
		ResultRev:    head,
		ManifestFile: manifestRel,
	}

	wt, ids, cleanup, err := BuildLandingGateContext(root, res, &RealGitOps{})
	require.NoError(t, err)
	require.NotEmpty(t, wt)
	require.NotEmpty(t, ids)

	// Pin and worktree exist before cleanup.
	pinRef := "refs/ddx/gate-pins/ddx-gatectx-bead/20260418T030303-cleanup"
	require.Equal(t, head, resolveRef(t, root, pinRef))
	_, statBefore := os.Stat(wt)
	require.NoError(t, statBefore)

	cleanup()

	// Pin ref gone.
	assert.Empty(t, resolveRef(t, root, pinRef), "cleanup must delete the pin ref")
	// Temp worktree gone.
	_, statAfter := os.Stat(wt)
	assert.True(t, os.IsNotExist(statAfter), "cleanup must remove the temp worktree dir; statErr=%v", statAfter)
}

func TestBuildLandingGateContext_MissingManifestFile(t *testing.T) {
	root, head := newGateCtxRepo(t)

	res := &ExecuteBeadResult{
		BeadID:       "ddx-gatectx-bead",
		AttemptID:    "20260418T040404-missing",
		BaseRev:      head,
		ResultRev:    head,
		ManifestFile: ".ddx/executions/does-not-exist/manifest.json",
	}

	wt, ids, cleanup, err := BuildLandingGateContext(root, res, &RealGitOps{})
	require.NoError(t, err, "missing manifest must soft-skip without error")
	assert.Empty(t, wt)
	assert.Empty(t, ids)
	require.NotNil(t, cleanup)
	cleanup() // must be a noop

	// pin ref must NOT have been created
	assert.Empty(t, resolveRef(t, root, "refs/ddx/gate-pins/ddx-gatectx-bead/20260418T040404-missing"))
}

func TestBuildLandingGateContext_NoResultRev(t *testing.T) {
	root, head := newGateCtxRepo(t)
	manifestRel := writeManifest(t, root, "20260418T050505-norev", []string{"FEAT-NOREV"})

	res := &ExecuteBeadResult{
		BeadID:       "ddx-gatectx-bead",
		AttemptID:    "20260418T050505-norev",
		BaseRev:      head,
		ResultRev:    "",
		ManifestFile: manifestRel,
	}

	wt, ids, cleanup, err := BuildLandingGateContext(root, res, &RealGitOps{})
	require.Error(t, err, "empty ResultRev must return error when governing IDs exist")
	assert.Empty(t, wt)
	assert.Empty(t, ids)
	require.NotNil(t, cleanup)
	cleanup() // safe noop on the returned closure
}

// TestEvaluateRequiredGatesForResult_WritesChecksFile is the direct unit test
// for the canonical helper extracted from LandBeadResult: when gate eval
// produces results and a checks-artifact path is provided, the helper writes
// checks.json and records the relative path on the result.
func TestEvaluateRequiredGatesForResult_WritesChecksFile(t *testing.T) {
	wtPath := t.TempDir()
	const specID = "FEAT-CHECKSWRITE"
	writeArtifactDoc(t, wtPath, specID)
	writeGateDoc(t, wtPath, "exec."+specID+".smoke", specID, true, []string{"sh", "-c", "exit 0"})

	bundleDir := t.TempDir()
	checksAbs := filepath.Join(bundleDir, "checks.json")
	checksRel := "checks.json"

	res := &ExecuteBeadResult{
		BeadID:    "ddx-checks-bead",
		AttemptID: "attempt-checks-1",
	}

	failed, ratchet, err := EvaluateRequiredGatesForResult(wtPath, []string{specID}, res, "", checksAbs, checksRel)
	require.NoError(t, err)
	assert.False(t, failed)
	assert.False(t, ratchet)

	require.Len(t, res.GateResults, 1, "expected one gate result")
	assert.Equal(t, "pass", res.GateResults[0].Status)
	assert.Equal(t, "pass", res.RequiredExecSummary)
	assert.Equal(t, checksRel, res.ChecksFile, "ChecksFile must be the bundle-relative path")
	assert.FileExists(t, checksAbs)

	// Sanity: checks.json must contain the gate result.
	raw, err := os.ReadFile(checksAbs)
	require.NoError(t, err)
	assert.Contains(t, string(raw), specID)
	assert.Contains(t, string(raw), "\"summary\": \"pass\"")
}
