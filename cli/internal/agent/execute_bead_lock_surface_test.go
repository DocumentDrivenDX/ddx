package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/require"
)

type writeFileAgentRunner struct{}

func (writeFileAgentRunner) Run(opts RunArgs) (*Result, error) {
	beadID := strings.TrimSpace(opts.Correlation["bead_id"])
	if beadID == "" {
		beadID = "ddx-test-worker"
	}
	path := filepath.Join(opts.WorkDir, beadID+".txt")
	if err := os.WriteFile(path, []byte("content for "+beadID+"\n"), 0o644); err != nil {
		return nil, err
	}
	return &Result{ExitCode: 0}, nil
}

type lockSurfacePrepareBackend struct {
	inner AttemptBackend
	delay time.Duration
}

var _ AttemptBackend = (*lockSurfacePrepareBackend)(nil)

func (b *lockSurfacePrepareBackend) Name() string { return b.inner.Name() }

func (b *lockSurfacePrepareBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	if b.delay > 0 {
		time.Sleep(b.delay)
	}
	if err := withMainGitLock(req.ProjectRoot, "prepare_lock_surface_probe", func() error { return nil }); err != nil {
		return nil, fmt.Errorf("prepare ran under main-git lock: %w", err)
	}
	return b.inner.Prepare(ctx, req)
}

func (b *lockSurfacePrepareBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	return b.inner.Run(ctx, req)
}

func (b *lockSurfacePrepareBackend) PublishResult(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error {
	return b.inner.PublishResult(ctx, ws, res)
}

func (b *lockSurfacePrepareBackend) Cleanup(ctx context.Context, ws *AttemptWorkspace) error {
	return b.inner.Cleanup(ctx, ws)
}

type lockSurfaceLandingGitOps struct {
	real  RealLandingGitOps
	delay time.Duration

	mu                      sync.Mutex
	resolveCalls            int
	resolveProbeOK          bool
	resolveReverifyBlocked  bool
	countProbeOK            bool
	updateProbeBlocked      bool
	updateProbeUnexpectedOK bool
}

var _ LandingGitOps = (*lockSurfaceLandingGitOps)(nil)

func (g *lockSurfaceLandingGitOps) CurrentBranch(dir string) (string, error) {
	return g.real.CurrentBranch(dir)
}

func (g *lockSurfaceLandingGitOps) ResolveRef(dir, ref string) (string, error) {
	g.mu.Lock()
	g.resolveCalls++
	callNum := g.resolveCalls
	g.mu.Unlock()
	if g.delay > 0 {
		time.Sleep(g.delay)
	}
	err := withMainGitLock(dir, "land_read_prep_probe", func() error { return nil })
	g.mu.Lock()
	switch {
	case callNum%2 == 1 && err == nil:
		g.resolveProbeOK = true
	case callNum%2 == 0 && err != nil:
		g.resolveReverifyBlocked = true
	case callNum%2 == 0 && err == nil:
		g.resolveReverifyBlocked = false
	}
	g.mu.Unlock()
	if callNum%2 == 1 {
		if err != nil {
			return "", fmt.Errorf("resolve ref ran under main-git lock: %w", err)
		}
	} else if err == nil {
		return "", fmt.Errorf("re-resolving target tip unexpectedly acquired main-git lock")
	}
	return g.real.ResolveRef(dir, ref)
}

func (g *lockSurfaceLandingGitOps) UpdateRefTo(dir, ref, sha, oldSHA string) error {
	err := withMainGitLock(dir, "land_update_probe", func() error { return nil })
	g.mu.Lock()
	if err != nil {
		g.updateProbeBlocked = true
	} else {
		g.updateProbeUnexpectedOK = true
	}
	g.mu.Unlock()
	if err == nil {
		return fmt.Errorf("update-ref unexpectedly acquired main-git lock")
	}
	return g.real.UpdateRefTo(dir, ref, sha, oldSHA)
}

func (g *lockSurfaceLandingGitOps) SyncWorkTreeToHead(dir, fromRev string) error {
	return g.real.SyncWorkTreeToHead(dir, fromRev)
}

func (g *lockSurfaceLandingGitOps) AddWorktree(dir, path, rev string) error {
	return g.real.AddWorktree(dir, path, rev)
}

func (g *lockSurfaceLandingGitOps) AddBranchWorktree(dir, path, branch string) error {
	return g.real.AddBranchWorktree(dir, path, branch)
}

func (g *lockSurfaceLandingGitOps) RemoveWorktree(dir, path string) error {
	return g.real.RemoveWorktree(dir, path)
}

func (g *lockSurfaceLandingGitOps) MergeInto(wtDir, srcRev, msg string) error {
	return g.real.MergeInto(wtDir, srcRev, msg)
}

func (g *lockSurfaceLandingGitOps) HeadRevAt(dir string) (string, error) {
	return g.real.HeadRevAt(dir)
}

func (g *lockSurfaceLandingGitOps) CountCommits(dir, base, tip string) int {
	if g.delay > 0 {
		time.Sleep(g.delay)
	}
	if err := withMainGitLock(dir, "land_read_prep_probe", func() error { return nil }); err != nil {
		g.mu.Lock()
		g.countProbeOK = false
		g.mu.Unlock()
		return g.real.CountCommits(dir, base, tip)
	}
	g.mu.Lock()
	g.countProbeOK = true
	g.mu.Unlock()
	return g.real.CountCommits(dir, base, tip)
}

func (g *lockSurfaceLandingGitOps) StageDir(dir, relPath string) error {
	return g.real.StageDir(dir, relPath)
}

func (g *lockSurfaceLandingGitOps) CommitStaged(dir, msg string) (string, error) {
	return g.real.CommitStaged(dir, msg)
}

func (g *lockSurfaceLandingGitOps) DiffNumstat(dir, base, tip string) (string, error) {
	return g.real.DiffNumstat(dir, base, tip)
}

func (g *lockSurfaceLandingGitOps) DiffNameOnly(dir, base, tip string) ([]string, error) {
	return g.real.DiffNameOnly(dir, base, tip)
}

type sleepLandingGitOps struct {
	real  RealLandingGitOps
	delay time.Duration
}

var _ LandingGitOps = (*sleepLandingGitOps)(nil)

func (g *sleepLandingGitOps) CurrentBranch(dir string) (string, error) {
	return g.real.CurrentBranch(dir)
}

func (g *sleepLandingGitOps) ResolveRef(dir, ref string) (string, error) {
	if g.delay > 0 {
		time.Sleep(g.delay)
	}
	return g.real.ResolveRef(dir, ref)
}

func (g *sleepLandingGitOps) UpdateRefTo(dir, ref, sha, oldSHA string) error {
	return g.real.UpdateRefTo(dir, ref, sha, oldSHA)
}

func (g *sleepLandingGitOps) SyncWorkTreeToHead(dir, fromRev string) error {
	return g.real.SyncWorkTreeToHead(dir, fromRev)
}

func (g *sleepLandingGitOps) AddWorktree(dir, path, rev string) error {
	return g.real.AddWorktree(dir, path, rev)
}

func (g *sleepLandingGitOps) AddBranchWorktree(dir, path, branch string) error {
	return g.real.AddBranchWorktree(dir, path, branch)
}

func (g *sleepLandingGitOps) RemoveWorktree(dir, path string) error {
	return g.real.RemoveWorktree(dir, path)
}

func (g *sleepLandingGitOps) MergeInto(wtDir, srcRev, msg string) error {
	return g.real.MergeInto(wtDir, srcRev, msg)
}

func (g *sleepLandingGitOps) HeadRevAt(dir string) (string, error) {
	return g.real.HeadRevAt(dir)
}

func (g *sleepLandingGitOps) CountCommits(dir, base, tip string) int {
	if g.delay > 0 {
		time.Sleep(g.delay)
	}
	return g.real.CountCommits(dir, base, tip)
}

func (g *sleepLandingGitOps) StageDir(dir, relPath string) error {
	return g.real.StageDir(dir, relPath)
}

func (g *sleepLandingGitOps) CommitStaged(dir, msg string) (string, error) {
	return g.real.CommitStaged(dir, msg)
}

func (g *sleepLandingGitOps) DiffNumstat(dir, base, tip string) (string, error) {
	return g.real.DiffNumstat(dir, base, tip)
}

func (g *sleepLandingGitOps) DiffNameOnly(dir, base, tip string) ([]string, error) {
	return g.real.DiffNameOnly(dir, base, tip)
}

func TestPreDispatchLockSurface_PrepareRunsOutsideMainGitLock(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName, "metrics"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ddxroot.DirName, "metrics", "attempts.jsonl"), []byte(`{"seed":"pre-dispatch"}`+"\n"), 0o644))

	prevPolicy := trackerLockPolicy
	trackerLockPolicy = LockRetryPolicy{
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
		Multiplier:     2.0,
		MaxRetries:     50,
		MaxElapsed:     200 * time.Millisecond,
	}
	t.Cleanup(func() { trackerLockPolicy = prevPolicy })

	backend := &lockSurfacePrepareBackend{
		inner: WorktreeAttemptBackend{},
	}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{Harness: "script"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
		AgentRunner:    writeFileAgentRunner{},
		AttemptBackend: backend,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotEmpty(t, res.ResultRev)
}

func TestLandLockSurface_ReadPrepRunsOutsideMainGitLock(t *testing.T) {
	repo := newLandTestRepo(t)
	workerSHA := repo.commitOn(repo.baseSHA, "feature.txt", "feature\n", "feat: worker")

	prevPolicy := trackerLockPolicy
	trackerLockPolicy = LockRetryPolicy{
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
		Multiplier:     2.0,
		MaxRetries:     50,
		MaxElapsed:     200 * time.Millisecond,
	}
	t.Cleanup(func() { trackerLockPolicy = prevPolicy })

	ops := &lockSurfaceLandingGitOps{real: RealLandingGitOps{}}
	land, err := Land(repo.dir, LandRequest{
		WorktreeDir:  repo.dir,
		BaseRev:      repo.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-land-surface",
		AttemptID:    "20260605T000000-surface",
		TargetBranch: "main",
	}, ops)
	require.NoError(t, err)
	require.NotNil(t, land)
	require.Equal(t, "landed", land.Status)

	ops.mu.Lock()
	resolveOK := ops.resolveProbeOK
	countOK := ops.countProbeOK
	updateBlocked := ops.updateProbeBlocked
	updateUnexpected := ops.updateProbeUnexpectedOK
	ops.mu.Unlock()

	require.True(t, resolveOK, "ResolveRef should run without holding the main-git lock")
	require.True(t, countOK, "CountCommits should run without holding the main-git lock")
	require.True(t, updateBlocked, "UpdateRefTo should be inside the main-git lock")
	require.False(t, updateUnexpected, "UpdateRefTo unexpectedly acquired the lock from inside the ref-update phase")
}

func TestConcurrentPrepareNoLock(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 2)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName, "metrics"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ddxroot.DirName, "metrics", "attempts.jsonl"), []byte(`{"seed":"concurrent-prepare"}`+"\n"), 0o644))

	prevPolicy := trackerLockPolicy
	trackerLockPolicy = LockRetryPolicy{
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
		Multiplier:     2.0,
		MaxRetries:     200,
		MaxElapsed:     5 * time.Second,
	}
	t.Cleanup(func() { trackerLockPolicy = prevPolicy })

	backend := &lockSurfacePrepareBackend{
		inner: WorktreeAttemptBackend{},
		delay: 250 * time.Millisecond,
	}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{Harness: "script"})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	results := make([]*ExecuteBeadResult, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			beadID := fmt.Sprintf("ddx-int-%04d", i+1)
			results[i], errs[i] = ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
				AgentRunner:    writeFileAgentRunner{},
				AttemptBackend: backend,
			}, &RealGitOps{})
		}()
	}
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "worker %d should not time out in pre-dispatch", i)
		require.NotNilf(t, results[i], "worker %d should produce a result", i)
		require.NotEmptyf(t, results[i].ResultRev, "worker %d should commit a result", i)
	}
}

func TestConcurrentLandRefRace(t *testing.T) {
	repo := newLandTestRepo(t)
	workerSHAs := []string{
		repo.commitOn(repo.baseSHA, "worker-1.txt", "worker 1\n", "feat: worker 1"),
		repo.commitOn(repo.baseSHA, "worker-2.txt", "worker 2\n", "feat: worker 2"),
	}

	prevPolicy := trackerLockPolicy
	trackerLockPolicy = LockRetryPolicy{
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
		Multiplier:     2.0,
		MaxRetries:     200,
		MaxElapsed:     5 * time.Second,
	}
	t.Cleanup(func() { trackerLockPolicy = prevPolicy })

	var wg sync.WaitGroup
	errs := make([]error, 2)
	results := make([]*LandResult, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			landOps := &sleepLandingGitOps{
				real:  RealLandingGitOps{},
				delay: 250 * time.Millisecond,
			}
			workerSHA := workerSHAs[i]
			res, err := Land(repo.dir, LandRequest{
				WorktreeDir:  repo.dir,
				BaseRev:      repo.baseSHA,
				ResultRev:    workerSHA,
				BeadID:       fmt.Sprintf("ddx-land-%d", i+1),
				AttemptID:    fmt.Sprintf("20260605T00000%d-race", i+1),
				TargetBranch: "main",
			}, landOps)
			results[i] = res
			if err != nil {
				errs[i] = err
			} else {
				errs[i] = nil
			}
		}()
	}
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "worker %d should complete pre-dispatch + land without timing out", i)
		require.NotNilf(t, results[i], "worker %d should produce a result", i)
	}
}
