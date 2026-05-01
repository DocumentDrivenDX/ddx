package server

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gateRepoFixture builds a real git repo seeded with:
//   - docs/specs/<specID>.md           (governing artifact doc)
//   - docs/exec/exec.<specID>.smoke.md (required execution gate, only when withGate=true)
//
// The gate's command is `sh -c "exit <gateExitCode>"`. When withGate=false the
// gate doc is omitted so gate evaluation finds nothing for the spec.
//
// Returns (root, initialMainSHA).
func gateRepoFixture(t *testing.T, specID string, withGate bool, gateExitCode int) (string, string) {
	t.Helper()
	root := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# test\n"), 0o644))

	// Governing spec doc.
	specDir := filepath.Join(root, "docs", "specs")
	require.NoError(t, os.MkdirAll(specDir, 0o755))
	specBody := "---\nddx:\n  id: " + specID + "\n---\n# " + specID + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(specDir, specID+".md"), []byte(specBody), 0o644))

	// Optional required execution gate.
	if withGate {
		execDir := filepath.Join(root, "docs", "exec")
		require.NoError(t, os.MkdirAll(execDir, 0o755))
		execID := "exec." + specID + ".smoke"
		gateBody := "---\nddx:\n  id: " + execID + "\n  depends_on:\n    - " + specID +
			"\n  execution:\n    kind: command\n    required: true\n    command:\n" +
			"      - sh\n      - -c\n      - exit " + intToStr(gateExitCode) + "\n---\n# " + execID + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(execDir, execID+".md"), []byte(gateBody), 0o644))
	}

	runCmd(t, root, "git", "init", "-b", "main")
	runCmd(t, root, "git", "config", "user.name", "Test")
	runCmd(t, root, "git", "config", "user.email", "test@test.local")
	runCmd(t, root, "git", "add", "-A")
	runCmd(t, root, "git", "commit", "-m", "init")

	tipOut, err := exec.Command("git", "-C", root, "rev-parse", "refs/heads/main").Output()
	require.NoError(t, err)
	return root, strings.TrimSpace(string(tipOut))
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// writeGateManifest writes a minimal manifest under
// .ddx/executions/<attempt>/manifest.json at projectRoot, declaring the given
// governing IDs. Returns the bundle-relative manifest path the way ExecuteBead's
// worker would set it on the result.
func writeGateManifest(t *testing.T, projectRoot, beadID, attemptID string, governingIDs []string) string {
	t.Helper()
	dirRel := filepath.Join(".ddx", "executions", attemptID)
	dirAbs := filepath.Join(projectRoot, dirRel)
	require.NoError(t, os.MkdirAll(dirAbs, 0o755))

	type govRef struct {
		ID string `json:"id"`
	}
	body := map[string]any{
		"attempt_id": attemptID,
		"bead_id":    beadID,
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

// commitWorkerChange creates a real commit on top of refs/heads/main in a
// throwaway worktree, returning the commit SHA. This simulates the output of
// ExecuteBead without spinning up a harness.
func commitWorkerChange(t *testing.T, root, fileName, fileContent string) string {
	t.Helper()
	wt, err := os.MkdirTemp("", "gate-wt-*")
	require.NoError(t, err)
	_ = os.RemoveAll(wt)
	runCmd(t, root, "git", "worktree", "add", "--detach", wt, "refs/heads/main")
	defer func() { runCmd(t, root, "git", "worktree", "remove", "--force", wt) }()

	require.NoError(t, os.WriteFile(filepath.Join(wt, fileName), []byte(fileContent), 0o644))
	runCmd(t, wt, "git", "add", "-A")
	runCmd(t, wt, "git", "-c", "user.name=Worker", "-c", "user.email=worker@test.local",
		"commit", "-m", "feat: worker change")
	headOut, err := exec.Command("git", "-C", wt, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(headOut))
}

// recordingSubmitter is a gateLandSubmitter that records every Submit call
// and forwards to a real LandCoordinator. Used to assert whether the
// preserve path bypassed the coordinator.
type recordingSubmitter struct {
	inner *LandCoordinator
	calls []agent.LandRequest
}

func (r *recordingSubmitter) Submit(req agent.LandRequest) (*agent.LandResult, error) {
	r.calls = append(r.calls, req)
	return r.inner.Submit(req)
}

// TestWorker_RequiredGatePass_LandsViaCoordinator: gate exits 0 → main
// advances to ResultRev (via coordinator), checks.json exists with PASS.
func TestWorker_RequiredGatePass_LandsViaCoordinator(t *testing.T) {
	const specID = "FEAT-WSGATE-PASS"
	const beadID = "ddx-gates-pass"
	const attemptID = "20260418T070000-gates-pass"

	root, initialTip := gateRepoFixture(t, specID, true, 0)
	manifestRel := writeGateManifest(t, root, beadID, attemptID, []string{specID})
	resultRev := commitWorkerChange(t, root, "worker-out.txt", "worker out\n")

	res := &agent.ExecuteBeadResult{
		BeadID:       beadID,
		AttemptID:    attemptID,
		BaseRev:      initialTip,
		ResultRev:    resultRev,
		ExitCode:     0,
		Outcome:      agent.ExecuteBeadOutcomeTaskSucceeded,
		ExecutionDir: filepath.Join(".ddx", "executions", attemptID),
		ManifestFile: manifestRel,
	}

	m := NewWorkerManager(root)
	t.Cleanup(func() { m.LandCoordinators.StopAll() })
	rec := &recordingSubmitter{inner: m.LandCoordinators.Get(root)}

	var logBuf bytes.Buffer
	require.NoError(t, evaluateGatesAndSubmit(root, res, &agent.RealGitOps{}, rec, &logBuf))

	// Coordinator MUST have been called (gates passed).
	assert.Len(t, rec.calls, 1, "coordinator must be called when required gates pass")

	// Target ref MUST have advanced.
	tipAfter := mustResolveRef(t, root, "refs/heads/main")
	assert.NotEqual(t, initialTip, tipAfter, "main must advance when required gate passes")

	// checks.json MUST exist with summary=pass.
	checksAbs := filepath.Join(root, ".ddx", "executions", attemptID, "checks.json")
	require.FileExists(t, checksAbs)
	raw, err := os.ReadFile(checksAbs)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "\"summary\": \"pass\"")

	// res state: outcome=merged, no preserve ref.
	assert.Equal(t, "merged", res.Outcome)
	assert.Empty(t, res.PreserveRef)
}

// TestWorker_RequiredGateFail_PreservesWithoutCoordinator: gate exits 1 →
// main UNCHANGED, refs/ddx/iterations/<bead>/<attempt> created at ResultRev,
// checks.json has FAIL, res.Outcome=preserved, coordinator NOT called.
func TestWorker_RequiredGateFail_PreservesWithoutCoordinator(t *testing.T) {
	const specID = "FEAT-WSGATE-FAIL"
	const beadID = "ddx-gates-fail"
	const attemptID = "20260418T070100-gates-fail"

	root, initialTip := gateRepoFixture(t, specID, true, 1) // gate exits 1
	manifestRel := writeGateManifest(t, root, beadID, attemptID, []string{specID})
	resultRev := commitWorkerChange(t, root, "worker-out.txt", "worker out\n")

	res := &agent.ExecuteBeadResult{
		BeadID:       beadID,
		AttemptID:    attemptID,
		BaseRev:      initialTip,
		ResultRev:    resultRev,
		ExitCode:     0,
		Outcome:      agent.ExecuteBeadOutcomeTaskSucceeded,
		ExecutionDir: filepath.Join(".ddx", "executions", attemptID),
		ManifestFile: manifestRel,
	}

	m := NewWorkerManager(root)
	t.Cleanup(func() { m.LandCoordinators.StopAll() })
	rec := &recordingSubmitter{inner: m.LandCoordinators.Get(root)}

	var logBuf bytes.Buffer
	require.NoError(t, evaluateGatesAndSubmit(root, res, &agent.RealGitOps{}, rec, &logBuf))

	// Coordinator MUST NOT have been called (gate failed → preserve directly).
	assert.Empty(t, rec.calls, "coordinator must NOT be called when required gate fails")

	// Target ref MUST be unchanged.
	tipAfter := mustResolveRef(t, root, "refs/heads/main")
	assert.Equal(t, initialTip, tipAfter, "main must NOT advance when required gate fails")

	// Iteration ref MUST point at ResultRev. Format mirrors LandBeadResult:
	// refs/ddx/iterations/<beadID>/<timestamp>-<shortBaseSHA> (PreserveRef helper).
	iterRef := agent.PreserveRef(beadID, initialTip)
	iterSHA := mustResolveRef(t, root, iterRef)
	assert.Equal(t, resultRev, iterSHA, "iteration ref must point at ResultRev")

	// checks.json MUST exist with summary=fail.
	checksAbs := filepath.Join(root, ".ddx", "executions", attemptID, "checks.json")
	require.FileExists(t, checksAbs)
	raw, err := os.ReadFile(checksAbs)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "\"summary\": \"fail\"")

	// res state: outcome=preserved, reason="post-run checks failed",
	// PreserveRef set, status=post_run_check_failed. These mirror what
	// LandBeadResult would set on the same scenario:
	//   - Outcome=preserved, Reason=post-run checks failed
	//     (execute_bead_orchestrator.go:246-251)
	//   - Status from ClassifyExecuteBeadStatus(preserved, 0, "post-run checks failed")
	//     → ExecuteBeadStatusPostRunCheckFailed (execute_bead_status.go:236-237)
	assert.Equal(t, "preserved", res.Outcome)
	assert.Equal(t, "post-run checks failed", res.Reason)
	assert.Equal(t, iterRef, res.PreserveRef)
	assert.Equal(t, agent.ExecuteBeadStatusPostRunCheckFailed, res.Status)
}

// TestWorker_NoGoverningIDs_LandsViaCoordinator: manifest declares no
// governing IDs → no gate eval, target ref advanced via coordinator,
// NO checks.json, NO gate-pin ref left behind.
func TestWorker_NoGoverningIDs_LandsViaCoordinator(t *testing.T) {
	const specID = "FEAT-WSGATE-NONE"
	const beadID = "ddx-gates-none"
	const attemptID = "20260418T070200-gates-none"

	// withGate=false; we still write the spec doc just for parity with the
	// other tests. The manifest below declares NO governing IDs so gate eval
	// never even runs.
	root, initialTip := gateRepoFixture(t, specID, false, 0)
	manifestRel := writeGateManifest(t, root, beadID, attemptID, nil) // empty governing
	resultRev := commitWorkerChange(t, root, "worker-out.txt", "worker out\n")

	res := &agent.ExecuteBeadResult{
		BeadID:       beadID,
		AttemptID:    attemptID,
		BaseRev:      initialTip,
		ResultRev:    resultRev,
		ExitCode:     0,
		Outcome:      agent.ExecuteBeadOutcomeTaskSucceeded,
		ExecutionDir: filepath.Join(".ddx", "executions", attemptID),
		ManifestFile: manifestRel,
	}

	m := NewWorkerManager(root)
	t.Cleanup(func() { m.LandCoordinators.StopAll() })
	rec := &recordingSubmitter{inner: m.LandCoordinators.Get(root)}

	var logBuf bytes.Buffer
	require.NoError(t, evaluateGatesAndSubmit(root, res, &agent.RealGitOps{}, rec, &logBuf))

	// Coordinator MUST have been called (no gates → submit path).
	assert.Len(t, rec.calls, 1, "coordinator must be called when no governing IDs")

	// Target ref MUST have advanced.
	tipAfter := mustResolveRef(t, root, "refs/heads/main")
	assert.NotEqual(t, initialTip, tipAfter, "main must advance when no governing IDs")

	// NO checks.json should exist (gate eval did not run).
	checksAbs := filepath.Join(root, ".ddx", "executions", attemptID, "checks.json")
	_, statErr := os.Stat(checksAbs)
	assert.True(t, os.IsNotExist(statErr), "checks.json must not exist when no governing IDs")

	// NO gate-pin ref should remain. BuildLandingGateContext returns
	// ("", nil, noop, nil) when governing IDs are empty, so no pin is ever
	// created — the cleanup is the noop.
	pinRef := "refs/ddx/gate-pins/" + beadID + "/" + attemptID
	out, pinErr := exec.Command("git", "-C", root, "rev-parse", "--verify", pinRef).Output()
	assert.Error(t, pinErr, "gate-pin ref must not exist (output=%q)", string(out))

	// res state: outcome=merged.
	assert.Equal(t, "merged", res.Outcome)
}

// mustResolveRef returns the SHA the ref points at, failing the test on error.
func mustResolveRef(t *testing.T, root, ref string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "rev-parse", "--verify", ref).Output()
	require.NoError(t, err, "rev-parse %s", ref)
	return strings.TrimSpace(string(out))
}
