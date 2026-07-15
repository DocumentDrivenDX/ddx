package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/require"
)

// TestSynthesizeCommit_GitignoredDirsDoNotFail covers ddx-feb1d4a5:
// RealGitOps.SynthesizeCommit must not fail with "staging changes: exit
// status 1" when .ddx/agent-logs/, .ddx/workers/, or .ddx/executions/
// exist as untracked gitignored directories. Previously the :(exclude)
// pathspecs for these paths caused `git add` to report them as
// explicitly-ignored and exit non-zero.
func TestSynthesizeCommit_GitignoredDirsDoNotFail(t *testing.T) {
	root, _ := newScriptHarnessRepo(t, 0)

	gitignore := filepath.Join(root, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore,
		[]byte(".ddx/agent-logs/\n.ddx/workers/\n.ddx/executions/\n"), 0644))
	runGitInteg(t, root, "add", ".gitignore")
	runGitInteg(t, root, "commit", "-m", "chore: add gitignore")

	logsDir := filepath.Join(root, ddxroot.DirName, "agent-logs")
	require.NoError(t, os.MkdirAll(logsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(logsDir, "log.jsonl"),
		[]byte(`{"ts":1}`), 0644))

	workersDir := filepath.Join(root, ddxroot.DirName, "workers")
	require.NoError(t, os.MkdirAll(workersDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workersDir, "w.json"),
		[]byte(`{}`), 0644))

	executionsDir := filepath.Join(root, ddxroot.DirName, "executions", "attempt", "embedded")
	require.NoError(t, os.MkdirAll(executionsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(executionsDir, "session.jsonl"),
		[]byte(`{"ts":1}`), 0644))

	ops := &RealGitOps{}

	committed, err := ops.SynthesizeCommit(root, "chore: test checkpoint")
	require.NoError(t, err, "SynthesizeCommit must succeed when only untracked changes are in gitignored dirs")
	require.False(t, committed, "no commit expected when the only 'changes' are gitignored")

	realFile := filepath.Join(root, "feature.txt")
	require.NoError(t, os.WriteFile(realFile, []byte("feature\n"), 0644))

	committed, err = ops.SynthesizeCommit(root, "chore: test real change")
	require.NoError(t, err, "SynthesizeCommit must succeed with a real change alongside gitignored dirs")
	require.True(t, committed, "commit expected when a real tracked-or-untracked file changes")

	trackedOut := runGitInteg(t, root, "ls-tree", "-r", "--name-only", "HEAD")
	require.Contains(t, trackedOut, "feature.txt", "real change must land in the commit")
	require.NotContains(t, trackedOut, ".ddx/agent-logs", "gitignored path must not be committed")
	require.NotContains(t, trackedOut, ".ddx/workers", "gitignored path must not be committed")
	require.NotContains(t, trackedOut, ".ddx/executions", "gitignored path must not be committed")
}

func TestSynthesizeCommitExcludesEntireExecutionEvidenceTree(t *testing.T) {
	root, _ := newScriptHarnessRepo(t, 0)
	// Simulate a stale project whose versioned ignore file does not protect
	// execution evidence. Also pre-stage the report to exercise the reset
	// defence, not just the add pathspec.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), nil, 0o644))
	runGitInteg(t, root, "add", ".gitignore")
	runGitInteg(t, root, "commit", "-m", "remove execution ignore coverage")

	const evidenceRel = ".ddx/executions/attempt/custom-report.md"
	evidenceAbs := filepath.Join(root, filepath.FromSlash(evidenceRel))
	require.NoError(t, os.MkdirAll(filepath.Dir(evidenceAbs), 0o755))
	wantEvidence := []byte("local report\n")
	require.NoError(t, os.WriteFile(evidenceAbs, wantEvidence, 0o644))
	runGitInteg(t, root, "add", "-f", "--", evidenceRel)
	require.NoError(t, os.WriteFile(filepath.Join(root, "feature.txt"), []byte("feature\n"), 0o644))

	committed, err := (&RealGitOps{}).SynthesizeCommit(root, "feat: synthesize without evidence")
	require.NoError(t, err)
	require.True(t, committed)
	require.Contains(t, runGitInteg(t, root, "ls-tree", "-r", "--name-only", "HEAD"), "feature.txt")
	require.NotContains(t, runGitInteg(t, root, "ls-tree", "-r", "--name-only", "HEAD"), ".ddx/executions/")
	require.Empty(t, runGitInteg(t, root, "diff", "--cached", "--name-only", "--", ".ddx/executions"))
	gotEvidence, readErr := os.ReadFile(evidenceAbs)
	require.NoError(t, readErr)
	require.Equal(t, wantEvidence, gotEvidence)
}
