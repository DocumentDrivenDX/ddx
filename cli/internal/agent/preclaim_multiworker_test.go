package agent

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/require"
)

// TestPreClaim_MultiWorkerParallelClaimUnderTrackerChurn covers ddx-df77e668
// AC #5: with several workers sharing one project, every bead is claimed in
// parallel even while .ddx/beads.jsonl is continuously staged by concurrent
// tracker writes. Before the fix, the pre-claim worktree-cleanliness check
// bounced with "landing worktree has staged changes" whenever a tracker file
// was staged, wedging the queue. The fix exempts tracker files, so pre-claim
// always succeeds under tracker churn and the workers drain the queue.
func TestPreClaim_MultiWorkerParallelClaimUnderTrackerChurn(t *testing.T) {
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s: %v", strings.Join(args, " "), string(out), err)
		}
	}
	runGit("init", "-b", "main")
	runGit("config", "user.name", "Test")
	runGit("config", "user.email", "test@test.local")

	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))

	const nBeads = 6
	for i := 0; i < nBeads; i++ {
		require.NoError(t, store.Create(context.Background(), &bead.Bead{
			ID:       fmt.Sprintf("ddx-mw-%02d", i),
			Title:    fmt.Sprintf("multiworker bead %02d", i),
			Priority: 0,
		}))
	}
	// Commit a beads.jsonl baseline so HEAD differs from the worktree as soon
	// as a worker claims and the store rewrites the file — the exact condition
	// that makes a staged beads.jsonl block pre-claim.
	runGit("add", "-A")
	runGit("commit", "-m", "seed beads")

	ops := RealLandingGitOps{}
	preClaim := func() error {
		_, err := ops.LocalAncestryCheck(dir, "main")
		return err
	}

	// Background tracker churn: keep staging the store-rewritten beads.jsonl to
	// reproduce the continuous multi-worker tracker race.
	stop := make(chan struct{})
	var churnWG sync.WaitGroup
	churnWG.Add(1)
	go func() {
		defer churnWG.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = exec.Command("git", "-C", dir, "add", ".ddx/beads.jsonl").Run()
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()

	const workers = 4
	var (
		claimedBy    sync.Map // beadID -> assignee (detects double claims)
		claimCount   int32
		preClaimErrs = make(chan string, 512)
		wg           sync.WaitGroup
	)
	deadline := time.Now().Add(10 * time.Second)
	for w := 0; w < workers; w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			assignee := fmt.Sprintf("worker-%d", w)
			for time.Now().Before(deadline) && atomic.LoadInt32(&claimCount) < nBeads {
				if err := preClaim(); err != nil {
					// A staged-changes bounce is the bug under test. Other
					// transient errors (e.g. a momentary index.lock) are
					// retried on the next cycle and are not the failure here.
					if strings.Contains(err.Error(), "staged changes") {
						preClaimErrs <- err.Error()
					}
					continue
				}
				ready, err := store.ReadyExecution()
				if err != nil || len(ready) == 0 {
					continue
				}
				cand := ready[0]
				if err := store.Claim(cand.ID, assignee); err != nil {
					continue // lost the claim race to another worker
				}
				if prev, dup := claimedBy.LoadOrStore(cand.ID, assignee); dup {
					t.Errorf("bead %s claimed twice (%v and %s)", cand.ID, prev, assignee)
				}
				atomic.AddInt32(&claimCount, 1)
			}
		}()
	}
	wg.Wait()
	close(stop)
	churnWG.Wait()
	close(preClaimErrs)

	for msg := range preClaimErrs {
		t.Errorf("pre-claim wedged on staged tracker files under concurrent writes: %s", msg)
	}
	require.Equal(t, int32(nBeads), atomic.LoadInt32(&claimCount),
		"all beads must be claimed in parallel despite continuous tracker churn")
}
