package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeExecutionCleanupRunner struct {
	calls int
	err   error
}

func (f *fakeExecutionCleanupRunner) Cleanup(ctx context.Context) (ExecutionCleanupSummary, error) {
	_ = ctx
	f.calls++
	return ExecutionCleanupSummary{ProjectRoot: "fake", TempRoot: "fake"}, f.err
}

func TestTryResourcePreflight_ChecksEvidenceAndTempRoots(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".ddx"), 0o755))
	tempRoot := t.TempDir()
	t.Setenv("DDX_EXEC_WT_DIR", tempRoot)

	checker := NewExecutionResourceChecker(projectRoot, &executionCleanupTestGitOps{})
	result, err := checker.Check(context.Background())
	require.NoError(t, err)

	assert.Equal(t, tempRoot, result.TempRoot)
	assert.ElementsMatch(t, []string{
		filepath.Join(projectRoot, ExecuteBeadArtifactDir),
		filepath.Join(projectRoot, ".ddx", "runs"),
	}, result.EvidenceRoots)

	require.Len(t, result.RootChecks, 3)
	assert.Equal(t, tempRoot, result.RootChecks[0].Path)
	assert.Equal(t, filepath.Join(projectRoot, ExecuteBeadArtifactDir), result.RootChecks[1].Path)
	assert.Equal(t, filepath.Join(projectRoot, ".ddx", "runs"), result.RootChecks[2].Path)
}

func TestTryResourcePreflight_RechecksAfterCleanup(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".ddx"), 0o755))
	tempRoot := t.TempDir()

	healthy := false
	checker := &ExecutionResourcePreflight{
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
		EvidenceRoots: []string{
			filepath.Join(projectRoot, ExecuteBeadArtifactDir),
		},
		CleanupRunner: &fakeExecutionCleanupRunner{},
		RootProbe: func(path string) (ExecutionResourceRootCheck, error) {
			check := ExecutionResourceRootCheck{
				Path:       path,
				Writable:   true,
				BytesFree:  executionResourceMinFreeBytes - 1,
				InodesFree: executionResourceMinFreeInodes - 1,
			}
			if healthy {
				check.BytesFree = executionResourceMinFreeBytes + 1
				check.InodesFree = executionResourceMinFreeInodes + 1
			}
			return check, nil
		},
	}

	runner := checker.CleanupRunner.(*fakeExecutionCleanupRunner)
	runner.err = nil
	checker.CleanupRunner = &cleanupTogglingRunner{
		inner: runner,
		onCleanup: func() {
			healthy = true
		},
	}

	result, err := checker.Check(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, runner.calls)
}

type cleanupTogglingRunner struct {
	inner     *fakeExecutionCleanupRunner
	onCleanup func()
}

func (c *cleanupTogglingRunner) Cleanup(ctx context.Context) (ExecutionCleanupSummary, error) {
	if c.onCleanup != nil {
		c.onCleanup()
	}
	return c.inner.Cleanup(ctx)
}
