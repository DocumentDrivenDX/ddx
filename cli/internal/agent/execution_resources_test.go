package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
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

	claimLivenessRoot := bead.ClaimLivenessRoot(ddxroot.JoinProject(projectRoot))

	assert.Equal(t, tempRoot, result.TempRoot)
	assert.ElementsMatch(t, []string{
		filepath.Join(projectRoot, ExecuteBeadArtifactDir),
		filepath.Join(projectRoot, ddxroot.DirName, "runs"),
		claimLivenessRoot,
	}, result.EvidenceRoots)

	require.Len(t, result.RootChecks, 4)
	assert.Equal(t, tempRoot, result.RootChecks[0].Path)
	assert.Equal(t, filepath.Join(projectRoot, ExecuteBeadArtifactDir), result.RootChecks[1].Path)
	assert.Equal(t, filepath.Join(projectRoot, ddxroot.DirName, "runs"), result.RootChecks[2].Path)
	assert.Equal(t, claimLivenessRoot, result.RootChecks[3].Path)
}

// TestResourcePreflightIncludesClaimLivenessRoot proves the default execution
// resource preflight checks the same claim-liveness heartbeat root that bead
// claim writes use, rather than reconstructing (and potentially drifting
// from) that path independently. See ddx-c054124f: a claim-liveness root
// excluded from preflight let /tmp inode exhaustion surface only as
// picker.claim_race loops instead of an upfront resource.preflight failure.
func TestResourcePreflightIncludesClaimLivenessRoot(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	checker := NewExecutionResourceChecker(projectRoot, &executionCleanupTestGitOps{})
	wantRoot := bead.ClaimLivenessRoot(ddxroot.JoinProject(projectRoot))

	assert.Contains(t, checker.EvidenceRoots, wantRoot)

	result, err := checker.Check(context.Background())
	require.NoError(t, err)

	var sawClaimLivenessCheck bool
	for _, check := range result.RootChecks {
		if check.Path == wantRoot {
			sawClaimLivenessCheck = true
			assert.True(t, check.Writable)
		}
	}
	assert.True(t, sawClaimLivenessCheck, "expected a root check for the claim-liveness root %s", wantRoot)
}

// TestResourcePreflightFailsWhenClaimLivenessRootBelowHardInodeMinimum proves
// preflight surfaces a ResourceExhaustedError when the claim-liveness root
// specifically (not just the temp/evidence roots) drops below the hard inode
// floor, matching the /tmp-exhaustion scenario from ddx-c054124f.
func TestResourcePreflightFailsWhenClaimLivenessRootBelowHardInodeMinimum(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)
	tempRoot := t.TempDir()
	claimLivenessRoot := bead.ClaimLivenessRoot(ddxroot.JoinProject(projectRoot))

	checker := &ExecutionResourcePreflight{
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
		EvidenceRoots: []string{
			filepath.Join(projectRoot, ExecuteBeadArtifactDir),
			claimLivenessRoot,
		},
		CleanupRunner: &fakeExecutionCleanupRunner{},
		RootProbe: func(path string) (ExecutionResourceRootCheck, error) {
			check := ExecutionResourceRootCheck{
				Path:       path,
				Writable:   true,
				BytesFree:  executionResourceMinFreeBytes + 1,
				InodesFree: executionResourceMinFreeInodes + 1,
			}
			if path == claimLivenessRoot {
				check.InodesFree = executionResourceMinFreeInodes - 1
			}
			return check, nil
		},
	}

	result, err := checker.Check(context.Background())
	require.Error(t, err)

	var resourceErr *ResourceExhaustedError
	require.ErrorAs(t, err, &resourceErr)
	assert.Contains(t, resourceErr.Detail, claimLivenessRoot)
	assert.NotNil(t, result)
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

// TestResourcePreflightClassifiesTooManyOpenFiles proves EMFILE from the
// writability probe is classified as fd exhaustion (not an ordinary
// unwritable root) and carries fd_count/fd_limit diagnostics. It injects the
// EMFILE failure via createWritabilityProbeFile rather than actually
// exhausting the test process's file descriptors, since lowering
// RLIMIT_NOFILE process-wide is flaky and can crash the Go runtime (e.g. the
// netpoller's epoll_create) if it hasn't initialized yet.
func TestResourcePreflightClassifiesTooManyOpenFiles(t *testing.T) {
	original := createWritabilityProbeFile
	t.Cleanup(func() { createWritabilityProbeFile = original })
	createWritabilityProbeFile = func(dir, pattern string) (*os.File, error) {
		return nil, &os.PathError{Op: "open", Path: dir, Err: unix.EMFILE}
	}

	root := t.TempDir()
	p := &ExecutionResourcePreflight{}
	check, err := p.checkRoot(root)
	require.Error(t, err)

	assert.False(t, check.Writable)
	assert.True(t, check.FDExhausted)
	assert.Greater(t, check.FDSoftLimit, uint64(0))
	assert.Greater(t, check.FDHardLimit, uint64(0))
	if runtime.GOOS == "linux" {
		assert.Greater(t, check.FDCount, 0)
	}
}

// TestResourcePreflightPreservesOrdinaryUnwritableRoot proves non-EMFILE
// writability failures still report an unwritable root without claiming fd
// exhaustion.
func TestResourcePreflightPreservesOrdinaryUnwritableRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks are ineffective when running as root")
	}

	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o555))
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	p := &ExecutionResourcePreflight{}
	check, err := p.checkRoot(root)
	require.Error(t, err)

	assert.False(t, check.Writable)
	assert.False(t, check.FDExhausted)
	assert.Zero(t, check.FDCount)
	assert.NotEmpty(t, check.WritableReason)
}
