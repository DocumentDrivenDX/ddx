package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
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

// postCleanupHookRunner wraps a real executionCleanupRunner and invokes after
// once the wrapped Cleanup call returns, so tests can flip a RootProbe from
// unhealthy to healthy exactly when the real cleanup pass has run.
type postCleanupHookRunner struct {
	inner executionCleanupRunner
	after func()
}

func (h *postCleanupHookRunner) Cleanup(ctx context.Context) (ExecutionCleanupSummary, error) {
	summary, err := h.inner.Cleanup(ctx)
	if h.after != nil {
		h.after()
	}
	return summary, err
}

// TestResourcePreflightReportsClaimLivenessReclaimedInodes proves that when
// resource preflight runs a cleanup pass (the real ExecutionCleanupManager,
// not a fake), stale claim-liveness "*.tmp-*" sidecars are reclaimed and the
// reclaimed file/inode counts surface on the CleanupSummary that feeds
// resource.preflight events, while the live (non-tmp) heartbeat file is left
// in place.
//
// Fixture roots come from newExecuteLoopTestStore so config-derived DDx/cache
// paths stay private to this test and cannot enumerate a host-global scratch
// tree or module cache.
func TestResourcePreflightReportsClaimLivenessReclaimedInodes(t *testing.T) {
	store, _, _ := newExecuteLoopTestStore(t)
	fixtureRoot := filepath.Dir(store.Dir)
	projectRoot := filepath.Join(fixtureRoot, "project")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	tempRoot := os.Getenv(config.ExecutionWorktreeRootEnv)
	require.NotEmpty(t, tempRoot, "newExecuteLoopTestStore must pin DDX_EXEC_WT_DIR")
	require.NoError(t, os.MkdirAll(tempRoot, 0o755))
	// Fixture roots may nest under the process temp dir via t.TempDir, but must
	// not be the host temp root that production cleanup enumerates by default.
	require.NotEqual(t, filepath.Clean(os.TempDir()), filepath.Clean(tempRoot))
	require.NotEqual(t, filepath.Clean(os.TempDir()), filepath.Clean(projectRoot))
	// Store/project/temp all share the private fixture allocated by the helper.
	assertPathUnder(t, store.Dir, fixtureRoot)
	assertPathUnder(t, projectRoot, fixtureRoot)
	assertPathUnder(t, tempRoot, fixtureRoot)

	claimLivenessRoot := bead.ClaimLivenessRoot(ddxroot.JoinProject(projectRoot))
	require.NoError(t, os.MkdirAll(claimLivenessRoot, 0o755))

	staleTmp := filepath.Join(claimLivenessRoot, "ddx-stale111.json.tmp-555")
	require.NoError(t, os.WriteFile(staleTmp, []byte("{}"), 0o644))
	staleTime := time.Now().Add(-1 * time.Hour)
	require.NoError(t, os.Chtimes(staleTmp, staleTime, staleTime))

	liveHeartbeat := filepath.Join(claimLivenessRoot, "ddx-live222.json")
	require.NoError(t, os.WriteFile(liveHeartbeat, []byte("{}"), 0o644))
	require.NoError(t, os.Chtimes(liveHeartbeat, staleTime, staleTime))

	checker := NewExecutionResourceChecker(projectRoot, &executionCleanupTestGitOps{})
	checker.TempRoot = tempRoot
	checker.SoftMinFreeInodes = 100
	checker.CleanupRunner = newHermeticExecutionCleanupTestManager(
		t, projectRoot, tempRoot, &executionCleanupTestGitOps{},
	)

	healthy := false
	checker.RootProbe = func(path string) (ExecutionResourceRootCheck, error) {
		check := ExecutionResourceRootCheck{
			Path:       path,
			Writable:   true,
			BytesFree:  executionResourceMinFreeBytes + 1,
			InodesFree: executionResourceMinFreeInodes + 1,
		}
		if path == claimLivenessRoot && !healthy {
			check.InodesFree = 1
		}
		return check, nil
	}
	checker.CleanupRunner = &postCleanupHookRunner{
		inner: checker.CleanupRunner,
		after: func() { healthy = true },
	}

	result, err := checker.Check(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(1), result.CleanupSummary.RemovedClaimLivenessTmpFiles)
	assert.Equal(t, int64(1), result.CleanupSummary.ClaimLivenessInodesReclaimed)
	assert.Equal(t, int64(len("{}")), result.CleanupSummary.ClaimLivenessBytesReclaimed)

	_, statErr := os.Stat(staleTmp)
	assert.True(t, os.IsNotExist(statErr), "expected stale claim-liveness tmp file to be reclaimed")
	_, statErr = os.Stat(liveHeartbeat)
	assert.NoError(t, statErr, "expected live claim-liveness heartbeat file to be preserved")

	// Config-derived roots remain the private pin after the cleanup pass.
	require.Equal(t, filepath.Clean(tempRoot), filepath.Clean(config.ExecutionTempRoot(projectRoot)))
	require.NotEqual(t, filepath.Clean(os.TempDir()), filepath.Clean(config.ExecutionScratchRoot(projectRoot)))
	assertPathUnder(t, config.ExecutionTempRoot(projectRoot), fixtureRoot)
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

// TestResourceTopInodeConsumerScanReportsPathCountSizeAgeAndCleanupPrefix
// proves the bounded scanner reports child path, entry count, byte size when
// available, age/mtime, and DDx cleanup-prefix match for children including
// ddx-home-* and ddx-claim-heartbeats-style names.
func TestResourceTopInodeConsumerScanReportsPathCountSizeAgeAndCleanupPrefix(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	old := now.Add(-2 * time.Hour)

	home := filepath.Join(root, "ddx-home-scan-abc")
	require.NoError(t, os.Mkdir(home, 0o755))
	payload := []byte("xx")
	require.NoError(t, os.WriteFile(filepath.Join(home, "a"), payload, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(home, "b"), payload, 0o644))
	require.NoError(t, os.Chtimes(home, old, old))

	claimHB := filepath.Join(root, "ddx-claim-heartbeats")
	require.NoError(t, os.Mkdir(claimHB, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(claimHB, "lease.json"), []byte("{}"), 0o644))
	require.NoError(t, os.Chtimes(claimHB, old, old))

	other := filepath.Join(root, "other-app-cache")
	require.NoError(t, os.Mkdir(other, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(other, "blob"), []byte("z"), 0o644))

	consumers, truncated, err := scanTopInodeConsumers(root, 8, 4096, now)
	require.NoError(t, err)
	assert.False(t, truncated)

	byBase := make(map[string]ExecutionTopInodeConsumer, len(consumers))
	for _, c := range consumers {
		byBase[filepath.Base(c.Path)] = c
	}

	homeC, ok := byBase["ddx-home-scan-abc"]
	require.True(t, ok, "expected ddx-home-* consumer")
	assert.Equal(t, home, homeC.Path)
	assert.GreaterOrEqual(t, homeC.EntryCount, int64(3), "dir + two files")
	assert.GreaterOrEqual(t, homeC.Bytes, int64(len(payload)*2))
	assert.False(t, homeC.ModTime.IsZero())
	assert.GreaterOrEqual(t, homeC.AgeSeconds, int64(2*time.Hour/time.Second-5))
	assert.True(t, homeC.MatchesCleanup)
	assert.Equal(t, "ddx-home-", homeC.CleanupPrefix)

	hbC, ok := byBase["ddx-claim-heartbeats"]
	require.True(t, ok, "expected ddx-claim-heartbeats consumer")
	assert.Equal(t, claimHB, hbC.Path)
	assert.GreaterOrEqual(t, hbC.EntryCount, int64(2), "dir + lease file")
	assert.True(t, hbC.MatchesCleanup)
	assert.Equal(t, "ddx-claim-heartbeats", hbC.CleanupPrefix)
	assert.GreaterOrEqual(t, hbC.AgeSeconds, int64(2*time.Hour/time.Second-5))

	otherC, ok := byBase["other-app-cache"]
	require.True(t, ok, "expected non-DDx consumer")
	assert.False(t, otherC.MatchesCleanup)
	assert.Empty(t, otherC.CleanupPrefix)

	// Diagnostic type is JSON-ready for later root-check attachment.
	check := ExecutionResourceRootCheck{
		Path:                       root,
		TopInodeConsumers:          consumers,
		TopInodeConsumersTruncated: truncated,
	}
	raw, err := json.Marshal(check)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "top_inode_consumers")
	assert.Contains(t, string(raw), "ddx-home-")
}

// TestResourcePreflightTopInodeConsumerScanIsBounded proves the diagnostic
// stops at the configured or hard-coded safe limit, marks truncated results,
// and does not recursively walk unbounded trees.
func TestResourcePreflightTopInodeConsumerScanIsBounded(t *testing.T) {
	root := t.TempDir()
	now := time.Now()

	for i := 0; i < 20; i++ {
		dir := filepath.Join(root, fmt.Sprintf("ddx-home-%02d", i))
		require.NoError(t, os.Mkdir(dir, 0o755))
		for j := 0; j < i+1; j++ {
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, fmt.Sprintf("f%d", j)),
				[]byte("x"),
				0o644,
			))
		}
	}

	const maxConsumers = 3
	consumers, truncated, err := scanTopInodeConsumers(root, maxConsumers, 4096, now)
	require.NoError(t, err)
	assert.True(t, truncated, "more than maxConsumers children must mark report truncation")
	require.Len(t, consumers, maxConsumers)
	assert.Equal(t, "ddx-home-19", filepath.Base(consumers[0].Path))
	assert.Equal(t, "ddx-home-18", filepath.Base(consumers[1].Path))
	assert.Equal(t, "ddx-home-17", filepath.Base(consumers[2].Path))
	// Defaults apply when limits are non-positive.
	defaulted, defaultTruncated, err := scanTopInodeConsumers(root, 0, 0, now)
	require.NoError(t, err)
	assert.True(t, defaultTruncated)
	assert.Len(t, defaulted, defaultTopInodeConsumerLimit)

	// Wide child: entry sampling stops at maxEntriesPerChild.
	wideRoot := t.TempDir()
	wide := filepath.Join(wideRoot, "ddx-home-wide")
	require.NoError(t, os.Mkdir(wide, 0o755))
	const maxEntries = 5
	for i := 0; i < 100; i++ {
		require.NoError(t, os.WriteFile(
			filepath.Join(wide, fmt.Sprintf("e%03d", i)),
			[]byte("y"),
			0o644,
		))
	}
	wideConsumers, _, err := scanTopInodeConsumers(wideRoot, 8, maxEntries, now)
	require.NoError(t, err)
	require.NotEmpty(t, wideConsumers)
	assert.True(t, wideConsumers[0].EntriesTruncated)
	assert.Equal(t, int64(1+maxEntries), wideConsumers[0].EntryCount,
		"directory inode + capped immediate children")

	// Deep nested tree: only the first-level child under the top consumer is
	// counted; nested files must not inflate the entry count.
	deepRoot := t.TempDir()
	deepChild := filepath.Join(deepRoot, "ddx-home-deep")
	nested := deepChild
	for i := 0; i < 40; i++ {
		nested = filepath.Join(nested, fmt.Sprintf("n%d", i))
	}
	require.NoError(t, os.MkdirAll(nested, 0o755))
	for i := 0; i < 100; i++ {
		require.NoError(t, os.WriteFile(
			filepath.Join(nested, fmt.Sprintf("leaf%d", i)),
			[]byte("nested-secret"),
			0o644,
		))
	}
	deepConsumers, deepTruncated, err := scanTopInodeConsumers(deepRoot, 8, 4096, now)
	require.NoError(t, err)
	assert.False(t, deepTruncated)
	require.Len(t, deepConsumers, 1)
	// Immediate children of ddx-home-deep: only n0 → entry count = dir + 1.
	assert.Equal(t, int64(2), deepConsumers[0].EntryCount)
	assert.False(t, deepConsumers[0].EntriesTruncated)
	assert.Less(t, deepConsumers[0].EntryCount, int64(50),
		"must not recursively walk nested trees")
}

// TestResourceTopInodeConsumerScanOmitsSensitiveContents proves the diagnostic
// includes paths and counts only and never reads or reports file contents.
func TestResourceTopInodeConsumerScanOmitsSensitiveContents(t *testing.T) {
	root := t.TempDir()
	const secret = "SUPER_SECRET_PAYLOAD_do_not_leak_42"
	child := filepath.Join(root, "ddx-home-secret")
	require.NoError(t, os.Mkdir(child, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(child, "credentials.txt"), []byte(secret), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(child, "token.bin"), []byte(secret+secret), 0o644))

	consumers, _, err := scanTopInodeConsumers(root, 8, 4096, time.Now())
	require.NoError(t, err)
	require.NotEmpty(t, consumers)

	raw, err := json.Marshal(consumers)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), secret)

	check := ExecutionResourceRootCheck{
		Path:              root,
		TopInodeConsumers: consumers,
	}
	checkRaw, err := json.Marshal(check)
	require.NoError(t, err)
	assert.NotContains(t, string(checkRaw), secret)

	for _, c := range consumers {
		assert.NotContains(t, c.Path, secret)
		assert.NotContains(t, c.CleanupPrefix, secret)
		assert.Greater(t, c.EntryCount, int64(0))
	}
}
