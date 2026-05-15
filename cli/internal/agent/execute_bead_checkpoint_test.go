package agent

// execute_bead_checkpoint_test.go — PowerClass-2 integration tests for the
// pre-dispatch checkpoint guard. DDx must capture allowed bookkeeping changes
// as a real commit on the current branch, but it must reject ordinary
// implementation files so they stay in the bead's substantive [ddx-<id>]
// commit instead of being absorbed into the checkpoint.
//
// Clean parent worktrees must NOT spawn a redundant checkpoint commit.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runNoopExecuteBeadForCheckpoint(t *testing.T, projectRoot, beadID string) *ExecuteBeadResult {
	t.Helper()

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{})

	runner := NewRunner(Config{})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: dirFile}).Resolve(config.CLIOverrides{Harness: "script"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: runner,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)
	return res
}

// TestExecuteBead_DirtyParentTree_CheckpointCommitted seeds a repo with
// allowed DDx bookkeeping dirt, runs ExecuteBead, and asserts that the
// checkpoint lands as a real commit on the current branch.
func TestExecuteBead_DirtyParentTree_CheckpointCommitted(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ddxroot.DirName, "run-state.json"),
		[]byte(`{"attempt_id":"checkpoint-test"}`+"\n"), 0o644))

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	commitsBefore := gitCommitCount(t, projectRoot, "HEAD")

	res := runNoopExecuteBeadForCheckpoint(t, projectRoot, beadID)

	headAfter := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	commitsAfter := gitCommitCount(t, projectRoot, "HEAD")

	assert.NotEqual(t, headBefore, headAfter,
		"HEAD must advance to capture the bookkeeping checkpoint commit")
	assert.GreaterOrEqual(t, commitsAfter-commitsBefore, 1,
		"at least one commit (the checkpoint) must land on the parent branch")

	// The allowed bookkeeping file must be reachable in HEAD.
	runStateAtHead := runGitInteg(t, projectRoot, "show", "HEAD:.ddx/run-state.json")
	assert.Contains(t, runStateAtHead, "checkpoint-test",
		"allowed bookkeeping content must be present in HEAD after checkpoint")

	// BaseRev recorded on the result must be the post-checkpoint HEAD.
	assert.NotEqual(t, headBefore, res.BaseRev,
		"BaseRev must be the post-checkpoint HEAD, not the pre-checkpoint HEAD")
}

// TestExecuteBead_CleanParentTree_NoSpuriousCheckpoint runs ExecuteBead against
// a clean parent worktree and asserts that no extra checkpoint commit is made
// beyond what other steps (CommitTracker) might add. The acceptance criterion
// is: clean parent trees do not create redundant checkpoint artifacts.
func TestExecuteBead_CleanParentTree_NoSpuriousCheckpoint(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	// Confirm we start clean.
	status := runGitInteg(t, projectRoot, "status", "--porcelain")
	require.Empty(t, status, "test setup invariant: parent must be clean")

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{})

	runner := NewRunner(Config{})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: dirFile}).Resolve(config.CLIOverrides{Harness: "script"})
	_, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: runner,
	}, &RealGitOps{})
	require.NoError(t, err)

	headAfter := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	// CommitTracker may have added a tracker commit if .ddx/beads.jsonl
	// changed during the attempt (e.g. claim metadata). What must NOT happen:
	// a separate "checkpoint pre-execute-bead" commit when nothing was dirty.
	if headBefore != headAfter {
		// Inspect the diff: only beads.jsonl (or other tracker files) may
		// differ. No checkpoint commit message in the log between these refs.
		log := runGitInteg(t, projectRoot, "log", "--format=%s", headBefore+".."+headAfter)
		assert.NotContains(t, log, "checkpoint pre-execute-bead",
			"clean parent tree must not produce a checkpoint commit")
	}
}

func TestExecuteBead_PreDispatchCheckpointBypassesHooks(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ddxroot.DirName, "run-state.json"), []byte(`{"attempt_id":"hook-test"}`+"\n"), 0o644))

	markerPath := filepath.Join(projectRoot, "hook-invoked.txt")
	hookPath := filepath.Join(projectRoot, ".git", "hooks", "pre-commit")
	hook := "#!/bin/sh\nprintf invoked > " + markerPath + "\nexit 42\n"
	require.NoError(t, os.MkdirAll(filepath.Dir(hookPath), 0o755))
	require.NoError(t, os.WriteFile(hookPath, []byte(hook), 0o755))

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	res := runNoopExecuteBeadForCheckpoint(t, projectRoot, beadID)
	require.NotEqual(t, headBefore, res.BaseRev, "checkpointed bookkeeping should still become the worker base")

	_, statErr := os.Stat(markerPath)
	require.True(t, os.IsNotExist(statErr), "pre-dispatch checkpoint must not invoke parent pre-commit hook")
	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:.ddx/run-state.json"), "hook-test")
}

func TestCheckpointPreDispatchDirtAllowsTrackerAndEvidencePaths(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const attemptID = "20260513T000001-allow000"

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ddxroot.DirName, "run-state.json"), []byte(`{"attempt_id":"allow"}`+"\n"), 0o644))
	evidenceRel := filepath.Join(ddxroot.DirName, "executions", attemptID, "manifest.json")
	evidencePath := filepath.Join(projectRoot, evidenceRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(evidencePath), 0o755))
	require.NoError(t, os.WriteFile(evidencePath, []byte(`{"attempt_id":"`+attemptID+`"}`+"\n"), 0o644))
	metricsRel := filepath.Join(ddxroot.DirName, "metrics", "attempts.jsonl")
	metricsPath := filepath.Join(projectRoot, metricsRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(metricsPath), 0o755))
	require.NoError(t, os.WriteFile(metricsPath, []byte(`{"attempt_id":"`+attemptID+`","outcome":"success"}`+"\n"), 0o644))

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	committed, err := checkpointPreDispatchDirt(projectRoot, attemptID)
	require.NoError(t, err)
	require.True(t, committed, "tracker/evidence dirt should checkpoint")

	headAfter := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	assert.NotEqual(t, headBefore, headAfter, "HEAD must advance for checkpointed bookkeeping")
	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:.ddx/run-state.json"), "allow")
	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:"+evidenceRel), attemptID)
	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:"+metricsRel), attemptID)
}

func TestPreDispatchCheckpointRejectsImplementationDirtyPaths(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const attemptID = "20260513T000002-block000"

	implPath := filepath.Join(projectRoot, "cli", "internal", "agent", "dirty_impl.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(implPath), 0o755))
	require.NoError(t, os.WriteFile(implPath, []byte("package agent\n"), 0o644))

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	committed, err := checkpointPreDispatchDirt(projectRoot, attemptID)
	require.Error(t, err)
	require.False(t, committed)
	assert.Contains(t, err.Error(), "cli/internal/agent/dirty_impl.go")
	assert.Contains(t, err.Error(), "commit or clean")
	assert.Contains(t, err.Error(), "[ddx-<id>]")
	assert.Equal(t, headBefore, runGitInteg(t, projectRoot, "rev-parse", "HEAD"))
}

func TestCheckpointPreDispatchDirtIgnoresGitIgnoredGeneratedPaths(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const attemptID = "20260513T000003-ignore000"

	ignore := strings.Join([]string{
		"cli/build/",
		"website/public/",
		".ddx/agent-logs/",
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".gitignore"), []byte(ignore), 0o644))
	runGitInteg(t, projectRoot, "add", ".gitignore")
	runGitInteg(t, projectRoot, "commit", "-m", "test: ignore generated paths")

	ignoredFiles := map[string]string{
		filepath.Join("cli", "build", "ddx"):                      "binary",
		filepath.Join("website", "public", "index.html"):          "<html></html>",
		filepath.Join(ddxroot.DirName, "agent-logs", "log.jsonl"): "{}\n",
	}
	for rel, content := range ignoredFiles {
		path := filepath.Join(projectRoot, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	committed, err := checkpointPreDispatchDirt(projectRoot, attemptID)
	require.NoError(t, err)
	assert.False(t, committed, "ignored generated paths should not create a checkpoint")
	assert.Equal(t, headBefore, runGitInteg(t, projectRoot, "rev-parse", "HEAD"))
}

func TestCheckpointPreDispatchDirtPreservesSkipWorktreeLocalOverlay(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const attemptID = "20260514T000001-overlay"

	pluginFileRel := filepath.Join(ddxroot.DirName, "plugins", "helix", "README.md")
	pluginFile := filepath.Join(projectRoot, pluginFileRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(pluginFile), 0o755))
	require.NoError(t, os.WriteFile(pluginFile, []byte("tracked plugin copy\n"), 0o644))
	runGitInteg(t, projectRoot, "add", pluginFileRel)
	runGitInteg(t, projectRoot, "commit", "-m", "test: track plugin copy")
	runGitInteg(t, projectRoot, "update-index", "--skip-worktree", "--", pluginFileRel)
	require.Contains(t, runGitInteg(t, projectRoot, "ls-files", "-t", "--", pluginFileRel), "S "+filepath.ToSlash(pluginFileRel))

	require.NoError(t, os.Remove(pluginFile))
	require.Empty(t, runGitInteg(t, projectRoot, "status", "--short", "--", pluginFileRel),
		"skip-worktree local overlay changes must start hidden")

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ddxroot.DirName, "run-state.json"), []byte(`{"attempt_id":"overlay"}`+"\n"), 0o644))
	committed, err := checkpointPreDispatchDirt(projectRoot, attemptID)
	require.NoError(t, err)
	require.True(t, committed, "allowed DDx bookkeeping should still checkpoint")

	assert.Contains(t, runGitInteg(t, projectRoot, "ls-files", "-t", "--", pluginFileRel), "S "+filepath.ToSlash(pluginFileRel),
		"checkpoint index sync must not drop local overlay skip-worktree bits")
	assert.Empty(t, runGitInteg(t, projectRoot, "status", "--short", "--", pluginFileRel),
		"hidden local overlay changes must remain hidden after checkpoint")
}

func TestCheckpointPreDispatchDirtyPathsExcludeIgnored(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)

	ignore := strings.Join([]string{
		"cli/build/",
		"website/public/",
		".ddx/agent-logs/",
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".gitignore"), []byte(ignore), 0o644))
	runGitInteg(t, projectRoot, "add", ".gitignore")
	runGitInteg(t, projectRoot, "commit", "-m", "test: ignore generated paths")

	ignoredPath := filepath.Join(projectRoot, "cli", "build", "ddx")
	require.NoError(t, os.MkdirAll(filepath.Dir(ignoredPath), 0o755))
	require.NoError(t, os.WriteFile(ignoredPath, []byte("binary"), 0o644))

	implPath := filepath.Join(projectRoot, "cli", "internal", "agent", "dirty_impl.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(implPath), 0o755))
	require.NoError(t, os.WriteFile(implPath, []byte("package agent\n"), 0o644))

	paths, err := preDispatchCheckpointDirtyPaths(projectRoot)
	require.NoError(t, err)
	assert.Contains(t, paths, "cli/internal/agent/dirty_impl.go")
	assert.NotContains(t, paths, "cli/build/ddx")
}

// TestPreDispatchCheckpoint_IgnoresWorkerSidecarsWithoutGitignore guards the
// fix for old projects whose .gitignore predates the .ddx/workers/ rule. The
// pre-dispatch checkpoint must treat .ddx/workers/<id>/status.json as DDx
// runtime scratch regardless of project gitignore contents — otherwise stale
// worker liveness sidecars block the next ddx work attempt.
func TestPreDispatchCheckpoint_IgnoresWorkerSidecarsWithoutGitignore(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const attemptID = "20260514T000002-workers"

	// No .gitignore rule for .ddx/workers/ — simulate an older repo.
	workerRel := filepath.Join(ddxroot.DirName, "workers", "agent-loop-test", "status.json")
	workerPath := filepath.Join(projectRoot, workerRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(workerPath), 0o755))
	require.NoError(t, os.WriteFile(workerPath, []byte(`{"alive":true}`+"\n"), 0o644))

	paths, err := preDispatchCheckpointDirtyPaths(projectRoot)
	require.NoError(t, err)
	assert.NotContains(t, paths, ".ddx/workers/agent-loop-test/status.json",
		"worker liveness sidecars must not appear in pre-dispatch checkpoint dirt")

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	committed, err := checkpointPreDispatchDirt(projectRoot, attemptID)
	require.NoError(t, err)
	assert.False(t, committed,
		"checkpoint must be a no-op when only worker sidecars are dirty")
	assert.Equal(t, headBefore, runGitInteg(t, projectRoot, "rev-parse", "HEAD"),
		"HEAD must not advance when only worker sidecars are dirty")

	// Negative assertion: an ordinary untracked file must still appear and
	// still trigger the existing checkpoint refusal.
	implPath := filepath.Join(projectRoot, "feature.txt")
	require.NoError(t, os.WriteFile(implPath, []byte("real implementation\n"), 0o644))

	paths, err = preDispatchCheckpointDirtyPaths(projectRoot)
	require.NoError(t, err)
	assert.Contains(t, paths, "feature.txt",
		"ordinary untracked files must still surface as checkpoint dirt")
	assert.NotContains(t, paths, ".ddx/workers/agent-loop-test/status.json")

	committed, err = checkpointPreDispatchDirt(projectRoot, attemptID)
	require.Error(t, err)
	require.False(t, committed)
	assert.Contains(t, err.Error(), "checkpoint refused to absorb implementation changes outside DDx bookkeeping")
	assert.Contains(t, err.Error(), "feature.txt")
}

func TestExecuteBeadCheckpointDoesNotAbsorbSubstantiveWork(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ddxroot.DirName, "run-state.json"), []byte(`{"attempt_id":"exec-test"}`+"\n"), 0o644))
	implPath := filepath.Join(projectRoot, "cli", "internal", "agent", "dirty_impl.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(implPath), 0o755))
	require.NoError(t, os.WriteFile(implPath, []byte("package agent\n"), 0o644))

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{})
	runner := NewRunner(Config{})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: dirFile}).Resolve(config.CLIOverrides{Harness: "script"})
	_, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: runner,
	}, &RealGitOps{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkpoint refused to absorb implementation changes")
	assert.Contains(t, err.Error(), "cli/internal/agent/dirty_impl.go")
	assert.Equal(t, headBefore, runGitInteg(t, projectRoot, "rev-parse", "HEAD"))
}
