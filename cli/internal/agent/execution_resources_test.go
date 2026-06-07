package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
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
	testutils.MakeInitializedDDxRoot(t, projectRoot)
	tempRoot := t.TempDir()
	t.Setenv("DDX_EXEC_WT_DIR", tempRoot)

	checker := NewExecutionResourceChecker(projectRoot, &executionCleanupTestGitOps{})
	result, err := checker.Check(context.Background())
	require.NoError(t, err)

	assert.Equal(t, tempRoot, result.TempRoot)
	assert.ElementsMatch(t, []string{
		filepath.Join(projectRoot, ExecuteBeadArtifactDir),
		filepath.Join(projectRoot, ddxroot.DirName, "runs"),
	}, result.EvidenceRoots)

	require.Len(t, result.RootChecks, 3)
	assert.Equal(t, tempRoot, result.RootChecks[0].Path)
	assert.Equal(t, filepath.Join(projectRoot, ExecuteBeadArtifactDir), result.RootChecks[1].Path)
	assert.Equal(t, filepath.Join(projectRoot, ddxroot.DirName, "runs"), result.RootChecks[2].Path)
}

func TestTryResourcePreflight_RechecksAfterCleanup(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)
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

func TestWorkResourcePreflight_RunsCleanupBelowSoftFloor(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)
	tempRoot := filepath.Join(t.TempDir(), "ddx-exec-wt")

	runner := &fakeExecutionCleanupRunner{}
	checker := &ExecutionResourcePreflight{
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
		EvidenceRoots: []string{
			filepath.Join(projectRoot, ExecuteBeadArtifactDir),
		},
		SoftMinFreeBytes:  100,
		SoftMinFreeInodes: 100,
		HardMinFreeBytes:  10,
		HardMinFreeInodes: 10,
		CleanupRunner:     runner,
		RootProbe: func(path string) (ExecutionResourceRootCheck, error) {
			return ExecutionResourceRootCheck{
				Path:       path,
				Writable:   true,
				BytesFree:  50,
				InodesFree: 50,
			}, nil
		},
	}

	result, err := checker.Check(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, runner.calls)
	require.NotEmpty(t, result.BeforeRootChecks)
	require.NotEmpty(t, result.RootChecks)
	assert.Contains(t, result.BeforeRootChecks[0].Notes, "free bytes 50 < soft cleanup threshold 100")
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
