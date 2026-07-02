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

func writeCheckpointTestFile(t *testing.T, projectRoot, rel, content string) string {
	t.Helper()
	path := filepath.Join(projectRoot, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return filepath.ToSlash(rel)
}

func requireHeadMissingPath(t *testing.T, projectRoot, rel string) {
	t.Helper()
	_, err := runGitIntegOutput(projectRoot, "cat-file", "-e", "HEAD:"+filepath.ToSlash(rel))
	require.Error(t, err, "%s must not be tracked in HEAD", rel)
}

func requireUntrackedOrIgnoredStatusEntries(t *testing.T, projectRoot string, rels ...string) {
	t.Helper()
	args := []string{"status", "--porcelain=v1", "--ignored=matching", "--untracked-files=all", "--"}
	args = append(args, rels...)
	status := runGitInteg(t, projectRoot, args...)
	for _, rel := range rels {
		path := filepath.ToSlash(rel)
		assert.True(t,
			strings.Contains(status, "!! "+path) || strings.Contains(status, "?? "+path),
			"expected %s to remain untracked or ignored in git status, got:\n%s", path, status,
		)
	}
}

// TestExecuteBead_DirtyParentTree_CheckpointCommitted seeds a repo with
// allowed DDx bookkeeping dirt, runs ExecuteBead, and asserts that the
// checkpoint lands as a real commit on the current branch.
func TestExecuteBead_DirtyParentTree_CheckpointCommitted(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"
	metricsRel := writeCheckpointTestFile(t, projectRoot, filepath.Join(ddxroot.DirName, "metrics", "attempts.jsonl"),
		`{"attempt_id":"checkpoint-test","outcome":"success"}`+"\n")

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
	metricsAtHead := runGitInteg(t, projectRoot, "show", "HEAD:"+metricsRel)
	assert.Contains(t, metricsAtHead, "checkpoint-test",
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
	metricsRel := writeCheckpointTestFile(t, projectRoot, filepath.Join(ddxroot.DirName, "metrics", "attempts.jsonl"),
		`{"attempt_id":"hook-test","outcome":"success"}`+"\n")

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
	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:"+metricsRel), "hook-test")
}

func TestCheckpointPreDispatchDirtIgnoresRunStateFiles(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const attemptID = "20260515T000001-runstate"
	runStateRootRel := filepath.Join(ddxroot.DirName, "run-state.json")
	runStateAttemptRel := filepath.Join(ddxroot.DirName, "run-state", attemptID+".json")
	metricsRel := filepath.Join(ddxroot.DirName, "metrics", "attempts.jsonl")

	writeCheckpointTestFile(t, projectRoot, runStateRootRel, `{"attempt_id":"`+attemptID+`","kind":"legacy"}`+"\n")
	writeCheckpointTestFile(t, projectRoot, runStateAttemptRel, `{"attempt_id":"`+attemptID+`","kind":"attempt"}`+"\n")
	writeCheckpointTestFile(t, projectRoot, metricsRel, `{"attempt_id":"`+attemptID+`","outcome":"success"}`+"\n")

	paths, err := preDispatchCheckpointDirtyPaths(projectRoot)
	require.NoError(t, err)
	assert.NotContains(t, paths, filepath.ToSlash(runStateRootRel))
	assert.NotContains(t, paths, filepath.ToSlash(runStateAttemptRel))
	assert.Contains(t, paths, filepath.ToSlash(metricsRel))

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	committed, err := checkpointPreDispatchDirt(projectRoot, attemptID)
	require.NoError(t, err)
	require.True(t, committed, "durable metrics dirt should still checkpoint")
	assert.NotEqual(t, headBefore, runGitInteg(t, projectRoot, "rev-parse", "HEAD"))

	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:"+filepath.ToSlash(metricsRel)), attemptID)
	requireHeadMissingPath(t, projectRoot, runStateRootRel)
	requireHeadMissingPath(t, projectRoot, runStateAttemptRel)
	requireUntrackedOrIgnoredStatusEntries(t, projectRoot, runStateRootRel, runStateAttemptRel)
}

func TestCheckpointPreDispatchDirtIgnoresEmbeddedExecutionPrivateFiles(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const attemptID = "20260515T000002-embedded"
	manifestRel := filepath.Join(ddxroot.DirName, "executions", attemptID, "manifest.json")
	embeddedRel := filepath.Join(ddxroot.DirName, "executions", attemptID, "embedded", "git-bin", "git")

	writeCheckpointTestFile(t, projectRoot, manifestRel, `{"attempt_id":"`+attemptID+`"}`+"\n")
	writeCheckpointTestFile(t, projectRoot, embeddedRel, "private git runtime\n")

	paths, err := preDispatchCheckpointDirtyPaths(projectRoot)
	require.NoError(t, err)
	assert.Contains(t, paths, filepath.ToSlash(manifestRel))
	assert.NotContains(t, paths, filepath.ToSlash(embeddedRel))

	committed, err := checkpointPreDispatchDirt(projectRoot, attemptID)
	require.NoError(t, err)
	require.True(t, committed, "durable execution evidence should still checkpoint")

	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:"+filepath.ToSlash(manifestRel)), attemptID)
	requireHeadMissingPath(t, projectRoot, embeddedRel)
	_, statErr := os.Stat(filepath.Join(projectRoot, embeddedRel))
	require.NoError(t, statErr, "embedded private file should remain on disk but stay untracked")
}

func TestCheckpointPreDispatchDirtAllowsTrackerAndEvidencePaths(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const attemptID = "20260513T000001-allow000"
	beadsRel := filepath.Join(ddxroot.DirName, "beads.jsonl")
	beadsPath := filepath.Join(projectRoot, beadsRel)
	beadsBefore, err := os.ReadFile(beadsPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(beadsPath, append(beadsBefore, '\n'), 0o644))
	manifestRel := writeCheckpointTestFile(t, projectRoot, filepath.Join(ddxroot.DirName, "executions", attemptID, "manifest.json"),
		`{"attempt_id":"`+attemptID+`","artifact":"manifest"}`+"\n")
	promptRel := writeCheckpointTestFile(t, projectRoot, filepath.Join(ddxroot.DirName, "executions", attemptID, "prompt.md"),
		"# Prompt\n\nattempt "+attemptID+"\n")
	usageRel := writeCheckpointTestFile(t, projectRoot, filepath.Join(ddxroot.DirName, "executions", attemptID, "usage.json"),
		`{"attempt_id":"`+attemptID+`","tokens":42}`+"\n")
	resultRel := writeCheckpointTestFile(t, projectRoot, filepath.Join(ddxroot.DirName, "executions", attemptID, "result.json"),
		`{"attempt_id":"`+attemptID+`","status":"success"}`+"\n")
	metricsRel := writeCheckpointTestFile(t, projectRoot, filepath.Join(ddxroot.DirName, "metrics", "attempts.jsonl"),
		`{"attempt_id":"`+attemptID+`","outcome":"success"}`+"\n")

	paths, err := preDispatchCheckpointDirtyPaths(projectRoot)
	require.NoError(t, err)
	for _, rel := range []string{beadsRel, manifestRel, promptRel, usageRel, resultRel, metricsRel} {
		assert.Contains(t, paths, filepath.ToSlash(rel))
	}

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	committed, err := checkpointPreDispatchDirt(projectRoot, attemptID)
	require.NoError(t, err)
	require.True(t, committed, "tracker/evidence dirt should checkpoint")

	headAfter := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	assert.NotEqual(t, headBefore, headAfter, "HEAD must advance for checkpointed bookkeeping")
	committedPaths := runGitInteg(t, projectRoot, "show", "--pretty=format:", "--name-only", "HEAD")
	for _, rel := range []string{beadsRel, manifestRel, promptRel, usageRel, resultRel, metricsRel} {
		assert.Contains(t, committedPaths, filepath.ToSlash(rel))
	}
	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:"+manifestRel), attemptID)
	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:"+promptRel), attemptID)
	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:"+usageRel), attemptID)
	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:"+resultRel), attemptID)
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

func TestPreservePreDispatchDirtyPathsCreatesRecoverableRef(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)

	implRel := filepath.Join("cli", "internal", "agent", "dirty_impl.go")
	implPath := filepath.Join(projectRoot, implRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(implPath), 0o755))
	require.NoError(t, os.WriteFile(implPath, []byte("package agent\n"), 0o644))

	preserved, err := preservePreDispatchDirtyPaths(projectRoot, []string{filepath.ToSlash(implRel)})
	require.NoError(t, err)
	require.NotNil(t, preserved)
	assert.Equal(t, []string{filepath.ToSlash(implRel)}, preserved.DirtyPaths)
	assert.Contains(t, preserved.PreserveRef, preDispatchDirtyPreserveRefPrefix)
	assert.Equal(t, "git stash apply "+preserved.PreserveRef, preserved.RecoverCommand)
	assert.NotEmpty(t, runGitInteg(t, projectRoot, "rev-parse", preserved.PreserveRef))
	assert.Empty(t, runGitInteg(t, projectRoot, "status", "--short", "--", filepath.ToSlash(implRel)))

	_, showErr := runGitIntegOutput(projectRoot, "show", "HEAD:"+filepath.ToSlash(implRel))
	require.Error(t, showErr, "the preserved file must be restored to HEAD before redispatch")

	out, applyErr := runGitIntegOutput(projectRoot, "stash", "apply", preserved.PreserveRef)
	require.NoError(t, applyErr, out)
	assert.Contains(t, runGitInteg(t, projectRoot, "status", "--short", "--", filepath.ToSlash(implRel)), "?? "+filepath.ToSlash(implRel))
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

	writeCheckpointTestFile(t, projectRoot, filepath.Join(ddxroot.DirName, "metrics", "attempts.jsonl"),
		`{"attempt_id":"overlay","outcome":"success"}`+"\n")
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

// TestPreDispatchCheckpoint_MaterializedSkillSymlinkIsNotDirt verifies that a
// tracked skill symlink materialized into a directory by an external tool is
// not reported as an uncommitted implementation change. This handles the case
// where tool-managed skill symlinks (.crush/skills/*, .claude/skills/*,
// .agents/skills/*) are converted from symlinks to directories.
func TestPreDispatchCheckpoint_MaterializedSkillSymlinkIsNotDirt(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)

	skillDirs := []string{".crush/skills", ".claude/skills", ".agents/skills"}
	for _, skillDir := range skillDirs {
		skillPath := filepath.Join(projectRoot, filepath.FromSlash(skillDir), "test-skill")
		require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))

		// Create a target directory for the symlink
		targetDir := filepath.Join(projectRoot, "skill-targets", "test-skill")
		require.NoError(t, os.MkdirAll(targetDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte("# Test Skill\n"), 0o644))

		// Create and commit a symlink
		require.NoError(t, os.Symlink(filepath.Join("..", "..", "skill-targets", "test-skill"), skillPath))
		runGitInteg(t, projectRoot, "add", filepath.ToSlash(filepath.Join(skillDir, "test-skill")))
		runGitInteg(t, projectRoot, "commit", "-m", "test: add skill symlink")

		// Remove the symlink and materialize it as a directory
		require.NoError(t, os.Remove(skillPath))
		require.NoError(t, os.MkdirAll(skillPath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte("# Test Skill\n"), 0o644))

		// Verify that the materialized symlink is not reported as dirty
		paths, err := preDispatchCheckpointDirtyPaths(projectRoot)
		require.NoError(t, err)
		assert.NotContains(t, paths, filepath.ToSlash(filepath.Join(skillDir, "test-skill")),
			"materialized skill symlink should not be reported as dirty in %s", skillDir)

		// Clean up for next iteration
		runGitInteg(t, projectRoot, "reset", "--hard", "HEAD")
		os.RemoveAll(filepath.Join(projectRoot, filepath.FromSlash(skillDir)))
		os.RemoveAll(filepath.Join(projectRoot, "skill-targets"))
	}
}

// TestPreDispatchCheckpoint_MaterializedSymlinkWithImplementationChanges is a
// regression test to ensure that genuine uncommitted implementation changes are
// still detected even when a symlink has been materialized. This guards against
// overly broad filtering.
func TestPreDispatchCheckpoint_MaterializedSymlinkWithImplementationChanges(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)

	// Create and commit a skill symlink
	skillPath := filepath.Join(projectRoot, ".crush", "skills", "test-skill")
	targetDir := filepath.Join(projectRoot, "skill-targets", "test-skill")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte("# Test Skill\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
	require.NoError(t, os.Symlink(filepath.Join("..", "..", "skill-targets", "test-skill"), skillPath))
	runGitInteg(t, projectRoot, "add", filepath.ToSlash(filepath.Join(".crush", "skills", "test-skill")))
	runGitInteg(t, projectRoot, "commit", "-m", "test: add skill symlink")

	// Materialize the symlink into a directory
	require.NoError(t, os.Remove(skillPath))
	require.NoError(t, os.MkdirAll(skillPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte("# Test Skill\n"), 0o644))

	// Add a genuine implementation file
	implPath := filepath.Join(projectRoot, "feature.txt")
	require.NoError(t, os.WriteFile(implPath, []byte("real implementation\n"), 0o644))

	// Verify that both the materialized symlink and the implementation change are detected,
	// but the implementation change is the one that matters
	paths, err := preDispatchCheckpointDirtyPaths(projectRoot)
	require.NoError(t, err)

	// The materialized symlink should NOT be in the dirty paths
	assert.NotContains(t, paths, ".crush/skills/test-skill")

	// But the implementation file should still be caught
	assert.Contains(t, paths, "feature.txt")
}
