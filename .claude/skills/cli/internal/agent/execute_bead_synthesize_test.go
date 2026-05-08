package agent

import (
	"os"
	"path/filepath"
	"testing"

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

	logsDir := filepath.Join(root, ".ddx", "agent-logs")
	require.NoError(t, os.MkdirAll(logsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(logsDir, "log.jsonl"),
		[]byte(`{"ts":1}`), 0644))

	workersDir := filepath.Join(root, ".ddx", "workers")
	require.NoError(t, os.MkdirAll(workersDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workersDir, "w.json"),
		[]byte(`{}`), 0644))

	executionsDir := filepath.Join(root, ".ddx", "executions", "attempt", "embedded")
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
