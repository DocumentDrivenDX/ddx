package agent

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type concurrentLandOutcome struct {
	index  int
	result *LandResult
	err    error
}

func seedSharedLandFile(t *testing.T, r *landTestRepo, content string) string {
	t.Helper()
	r.writeFile("shared.txt", content)
	r.runGit("add", "shared.txt")
	r.runGit("commit", "-m", "chore: seed shared land file")
	return r.resolveRef("refs/heads/main")
}

func runConcurrentLandBatch(t *testing.T, projectRoot string, reqs []LandRequest, gitOps LandingGitOps) []concurrentLandOutcome {
	t.Helper()

	ready := make(chan struct{}, len(reqs))
	start := make(chan struct{})
	outcomes := make(chan concurrentLandOutcome, len(reqs))

	var wg sync.WaitGroup
	for i, req := range reqs {
		i, req := i, req
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			result, err := Land(projectRoot, req, gitOps)
			outcomes <- concurrentLandOutcome{index: i, result: result, err: err}
		}()
	}

	for i := 0; i < len(reqs); i++ {
		<-ready
	}
	close(start)
	wg.Wait()
	close(outcomes)

	results := make([]concurrentLandOutcome, len(reqs))
	for outcome := range outcomes {
		results[outcome.index] = outcome
	}
	return results
}

func TestConcurrentLand_NonOverlappingChanges_BothLandClean(t *testing.T) {
	r := newLandTestRepo(t)
	ops := RealLandingGitOps{}

	const workers = 3
	baseSHA := r.baseSHA
	workerSHAs := make([]string, workers)
	reqs := make([]LandRequest, 0, workers)
	for i := 0; i < workers; i++ {
		workerSHAs[i] = r.commitOn(
			baseSHA,
			fmt.Sprintf("worker-%d.txt", i),
			fmt.Sprintf("unique worker %d\n", i),
			fmt.Sprintf("feat: worker %d", i),
		)
		reqs = append(reqs, LandRequest{
			WorktreeDir:  r.dir,
			BaseRev:      baseSHA,
			ResultRev:    workerSHAs[i],
			BeadID:       fmt.Sprintf("ddx-land-nonoverlap-%d", i),
			AttemptID:    fmt.Sprintf("20260511T0100%02d-nonoverlap", i),
			TargetBranch: "main",
		})
	}

	outcomes := runConcurrentLandBatch(t, r.dir, reqs, ops)

	ffCount := 0
	mergedCount := 0
	for i, outcome := range outcomes {
		require.NoError(t, outcome.err, "worker %d: Land must succeed", i)
		require.NotNil(t, outcome.result, "worker %d: Land must return a result", i)
		require.Equal(t, "landed", outcome.result.Status, "worker %d: expected landed", i)
		require.Empty(t, outcome.result.PreserveRef, "worker %d: non-overlap should not preserve", i)
		if outcome.result.Merged {
			mergedCount++
		} else {
			ffCount++
		}
	}

	require.Equal(t, 1, ffCount, "exactly one worker should take the fast-forward path")
	require.Equal(t, workers-1, mergedCount, "the remaining workers should merge cleanly")

	for i, sha := range workerSHAs {
		require.Truef(t, r.shaReachable("refs/heads/main", sha), "worker %d commit %s must be reachable from main", i, sha)
		parents := r.commitParents(sha)
		require.Lenf(t, parents, 1, "worker %d commit parent count", i)
		require.Equalf(t, baseSHA, parents[0], "worker %d commit parent", i)
	}

	require.Equal(t, workers-1, r.mergeCommitCount("refs/heads/main"), "clean concurrent lands should produce merge commits for the later workers")
}

func TestConcurrentLand_OverlappingChanges_OneLandsRestPreserve(t *testing.T) {
	r := newLandTestRepo(t)
	baseSHA := seedSharedLandFile(t, r, "base shared line\n")
	ops := RealLandingGitOps{}

	const workers = 3
	workerSHAs := make([]string, workers)
	reqs := make([]LandRequest, 0, workers)
	for i := 0; i < workers; i++ {
		workerSHAs[i] = r.commitOn(
			baseSHA,
			"shared.txt",
			fmt.Sprintf("conflicting worker %d\n", i),
			fmt.Sprintf("feat: shared worker %d", i),
		)
		reqs = append(reqs, LandRequest{
			WorktreeDir:  r.dir,
			BaseRev:      baseSHA,
			ResultRev:    workerSHAs[i],
			BeadID:       fmt.Sprintf("ddx-land-overlap-%d", i),
			AttemptID:    fmt.Sprintf("20260511T0200%02d-overlap", i),
			TargetBranch: "main",
		})
	}

	outcomes := runConcurrentLandBatch(t, r.dir, reqs, ops)

	landed := 0
	preserved := 0
	var landedSHA string
	for i, outcome := range outcomes {
		require.NoError(t, outcome.err, "worker %d: Land must return cleanly", i)
		require.NotNil(t, outcome.result, "worker %d: Land must return a result", i)
		switch outcome.result.Status {
		case "landed":
			landed++
			landedSHA = outcome.result.NewTip
			require.Empty(t, outcome.result.PreserveRef, "worker %d: landed attempts must not preserve", i)
		case "preserved":
			preserved++
			require.NotEmpty(t, outcome.result.PreserveRef, "worker %d: preserved attempts need a preserve ref", i)
			require.Truef(t, r.shaReachable(outcome.result.PreserveRef, workerSHAs[i]), "worker %d preserve ref must retain the worker commit", i)
			require.Equalf(t, workerSHAs[i], r.resolveRef(outcome.result.PreserveRef), "worker %d preserve ref must point at the worker commit", i)
		default:
			t.Fatalf("worker %d: unexpected land status %q", i, outcome.result.Status)
		}
	}

	require.Equal(t, 1, landed, "exactly one conflicting attempt should land")
	require.Equal(t, workers-1, preserved, "the other conflicting attempts should preserve")
	require.NotEmpty(t, landedSHA, "the landed worker must produce a tip")
	require.Equal(t, landedSHA, r.resolveRef("refs/heads/main"), "main must advance to the landed tip")
	require.Equal(t, 0, r.mergeCommitCount("refs/heads/main"), "conflicting lands should not create merge commits")

	for i, sha := range workerSHAs {
		parents := r.commitParents(sha)
		require.Lenf(t, parents, 1, "worker %d commit parent count", i)
		require.Equalf(t, baseSHA, parents[0], "worker %d commit parent", i)
	}
}

func TestConcurrentLand_PreservationRateUnderLoadIsBounded(t *testing.T) {
	r := newLandTestRepo(t)
	_ = seedSharedLandFile(t, r, "base shared line\n")
	ops := RealLandingGitOps{}

	const (
		workers = 4
		cycles  = 10
	)

	rng := rand.New(rand.NewSource(20260511))
	totalAttempts := 0
	totalPreserved := 0

	for cycle := 0; cycle < cycles; cycle++ {
		cycleBase := r.resolveRef("refs/heads/main")
		sharedCount := 1 + rng.Intn(2) // 1 or 2 overlapping attempts per cycle.
		sharedSlots := map[int]struct{}{}
		for len(sharedSlots) < sharedCount {
			sharedSlots[rng.Intn(workers)] = struct{}{}
		}

		reqs := make([]LandRequest, 0, workers)
		workerSHAs := make([]string, workers)
		for i := 0; i < workers; i++ {
			var path, content, msg string
			if _, ok := sharedSlots[i]; ok {
				path = "shared.txt"
				content = fmt.Sprintf("cycle %d shared worker %d\n", cycle, i)
				msg = fmt.Sprintf("feat: shared cycle %d worker %d", cycle, i)
			} else {
				path = fmt.Sprintf("cycle-%02d-worker-%d.txt", cycle, i)
				content = fmt.Sprintf("cycle %d unique worker %d\n", cycle, i)
				msg = fmt.Sprintf("feat: unique cycle %d worker %d", cycle, i)
			}
			workerSHAs[i] = r.commitOn(cycleBase, path, content, msg)
			reqs = append(reqs, LandRequest{
				WorktreeDir:  r.dir,
				BaseRev:      cycleBase,
				ResultRev:    workerSHAs[i],
				BeadID:       fmt.Sprintf("ddx-land-load-%02d-%d", cycle, i),
				AttemptID:    fmt.Sprintf("20260511T0300%02d-%02d", cycle, i),
				TargetBranch: "main",
			})
		}

		outcomes := runConcurrentLandBatch(t, r.dir, reqs, ops)
		for i, outcome := range outcomes {
			require.NoError(t, outcome.err, "cycle %d worker %d: Land must succeed", cycle, i)
			require.NotNil(t, outcome.result, "cycle %d worker %d: Land must return a result", cycle, i)
			switch outcome.result.Status {
			case "landed":
				require.Empty(t, outcome.result.PreserveRef, "cycle %d worker %d: landed result should not preserve", cycle, i)
			case "preserved":
				totalPreserved++
				require.NotEmpty(t, outcome.result.PreserveRef, "cycle %d worker %d: preserved result needs a ref", cycle, i)
				require.Equalf(t, workerSHAs[i], r.resolveRef(outcome.result.PreserveRef), "cycle %d worker %d: preserve ref must point at the worker commit", cycle, i)
				require.Truef(t, r.shaReachable(outcome.result.PreserveRef, workerSHAs[i]), "cycle %d worker %d: preserve ref must retain the worker commit", cycle, i)
			default:
				t.Fatalf("cycle %d worker %d: unexpected land status %q", cycle, i, outcome.result.Status)
			}
		}

		totalAttempts += workers
		for i, sha := range workerSHAs {
			parents := r.commitParents(sha)
			require.Lenf(t, parents, 1, "cycle %d worker %d: commit parent count", cycle, i)
			require.Equalf(t, cycleBase, parents[0], "cycle %d worker %d: commit parent", cycle, i)
		}
	}

	require.Lessf(t, float64(totalPreserved)/float64(totalAttempts), 0.5, "preservation rate must stay below 50%%")
}
