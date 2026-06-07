package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Git helpers
// ---------------------------------------------------------------------------

// scrubbedGitEnvInteg returns the current environment with all GIT_* variables
// removed, ensuring test-local git subprocesses don't inherit parent repo state.
func scrubbedGitEnvInteg() []string {
	parent := os.Environ()
	env := make([]string, 0, len(parent))
	for _, kv := range parent {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// runGitInteg runs a git command in dir with scrubbed GIT_* env.
// Fails the test on non-zero exit.
func runGitInteg(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = scrubbedGitEnvInteg()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// runGitIntegOutput runs a git command and returns (output, error) — for cases
// where failure is expected or handled by the caller.
func runGitIntegOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = scrubbedGitEnvInteg()
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ---------------------------------------------------------------------------
// Repo setup
// ---------------------------------------------------------------------------

// newScriptHarnessRepo creates a temp git repo with an initial seed commit on
// main, seeds .ddx/beads.jsonl with beadCount open beads (IDs like
// ddx-int-0001, ddx-int-0002 …), and returns (projectRoot, initialSHA).
// The initialSHA is the SHA immediately after the "chore: initial seed" commit,
// BEFORE the seed-beads commit, so callers can measure commits added beyond it.
func newScriptHarnessRepo(t *testing.T, beadCount int) (string, string) {
	t.Helper()

	setExecutionWorktreeRootForTest(t)
	root := t.TempDir()

	runGitInteg(t, root, "init", "-b", "main")
	runGitInteg(t, root, "config", "user.email", "test@ddx.test")
	runGitInteg(t, root, "config", "user.name", "DDx Test")

	// Create initial seed file and commit so the repo has a HEAD.
	seedFile := filepath.Join(root, "seed.txt")
	require.NoError(t, os.WriteFile(seedFile, []byte("seed\n"), 0644))
	runGitInteg(t, root, "add", ".")
	runGitInteg(t, root, "commit", "-m", "chore: initial seed")

	initialSHA := runGitInteg(t, root, "rev-parse", "HEAD")

	// Set up .ddx dir and bead store.
	ddxDir := testutils.MakeInitializedDDxRoot(t, root)
	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init(context.Background()))

	for i := 0; i < beadCount; i++ {
		id := fmt.Sprintf("ddx-int-%04d", i+1)
		b := &bead.Bead{
			ID:        id,
			Title:     fmt.Sprintf("Integration test bead %d", i+1),
			IssueType: "task",
			Priority:  i,
		}
		require.NoError(t, store.Create(context.Background(

		// Commit beads.jsonl so the initial worktree contains it.
		), b))
	}

	runGitInteg(t, root, "add", ".ddx/beads.jsonl", ".ddx/config.yaml")
	runGitInteg(t, root, "commit", "-m", "chore: seed beads")

	return root, initialSHA
}

func setExecutionWorktreeRootForTest(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), ExecuteBeadTmpSubdir)
	require.NoError(t, os.MkdirAll(root, 0o755))
	t.Setenv(config.ExecutionWorktreeRootEnv, root)
	return root
}

// ---------------------------------------------------------------------------
// Directive file helper
// ---------------------------------------------------------------------------

// writeDirectiveFile writes a directive file at path with the given lines.
func writeDirectiveFile(t *testing.T, path string, lines []string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	content := strings.Join(lines, "\n") + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

// ---------------------------------------------------------------------------
// Executor helpers
// ---------------------------------------------------------------------------

// landSerializerMu guards landSerializerMap.
var (
	landSerializerMu  sync.Mutex
	landSerializerMap = map[string]*sync.Mutex{}
)

// landMutexFor returns the per-projectRoot mutex that serializes Land() calls.
// This mimics the production per-project LandCoordinator goroutine
// (cli/internal/server/workers.go). Tests that spawn concurrent workers share
// this mutex so the CAS-based UpdateRefTo in Land() never races.
func landMutexFor(projectRoot string) *sync.Mutex {
	landSerializerMu.Lock()
	defer landSerializerMu.Unlock()
	m, ok := landSerializerMap[projectRoot]
	if !ok {
		m = &sync.Mutex{}
		landSerializerMap[projectRoot] = m
	}
	return m
}

// scriptHarnessExecutor builds an ExecuteBeadExecutorFunc that:
//  1. Runs ExecuteBead with the script harness (directive file = directivePath).
//  2. Passes the result through LandBeadResult with a LandingAdvancer that
//     calls Land() — serialized via landMutexFor(projectRoot).
//
// Both ExecuteBead and Land() are serialized per projectRoot. This is required
// because ExecuteBead calls CommitTracker (which acquires the git index lock)
// and because Land() uses CAS-based UpdateRefTo. In production, the server
// dispatches one bead per worker and serializes Land() via the LandCoordinator
// goroutine; in integration tests, the same mutex handles both.
//
// Failed exits (exit code != 0) with commits are preserved under
// refs/ddx/iterations/ by LandBeadResult before Land() is invoked.
func scriptHarnessExecutor(t *testing.T, projectRoot, directivePath string) ExecuteBeadExecutorFunc {
	t.Helper()
	// Build Runner once at executor-construction time so concurrent goroutine
	// calls share a stable test runner instance.
	runner := NewRunner(Config{})
	gitOps := &RealGitOps{}
	orchGitOps := &RealGitOps{}
	// Per-projectRoot mutex serializes git operations so concurrent workers
	// don't race on the git index or on Land()'s CAS UpdateRefTo.
	repoMu := landMutexFor(projectRoot)

	cfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Model: directivePath,
	})
	rcfg := cfg.Resolve(config.CLIOverrides{Harness: "script"})

	return ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		repoMu.Lock()
		defer repoMu.Unlock()

		res, err := ExecuteBeadWithConfig(ctx, projectRoot, beadID, rcfg, ExecuteBeadRuntime{
			AgentRunner: runner,
		}, gitOps)
		if err != nil {
			if res != nil {
				return executeBeadResultToReport(res), nil
			}
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: err.Error(),
			}, nil
		}

		landFromRev := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
		landing, landErr := LandBeadResult(projectRoot, res, orchGitOps, BeadLandingOptions{
			LandingAdvancer: func(r *ExecuteBeadResult) (*LandResult, error) {
				// landMutexFor is already held (repoMu == landMutexFor(projectRoot)).
				req := BuildLandRequestFromResult(projectRoot, r)
				return Land(projectRoot, req, RealLandingGitOps{})
			},
		})
		if landErr != nil {
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: landErr.Error(),
			}, nil
		}
		ApplyLandingToResult(res, landing)
		if syncErr := (RealLandingGitOps{}).SyncWorkTreeToHead(projectRoot, landFromRev); syncErr != nil {
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: syncErr.Error(),
			}, nil
		}
		return executeBeadResultToReport(res), nil
	})
}

// executeBeadResultToReport converts ExecuteBeadResult to ExecuteBeadReport.
func executeBeadResultToReport(res *ExecuteBeadResult) ExecuteBeadReport {
	return ExecuteBeadReport{
		BeadID:             res.BeadID,
		AttemptID:          res.AttemptID,
		WorkerID:           res.WorkerID,
		Harness:            res.Harness,
		Model:              res.Model,
		Status:             res.Status,
		Detail:             res.Detail,
		SessionID:          res.SessionID,
		BaseRev:            res.BaseRev,
		ResultRev:          res.ResultRev,
		PreserveRef:        res.PreserveRef,
		NoChangesRationale: res.NoChangesRationale,
		Error:              res.Error,
		Stderr:             res.Stderr,
		RateLimitBudget:    res.RateLimitBudget,
	}
}

// ---------------------------------------------------------------------------
// Git assertion helpers
// ---------------------------------------------------------------------------

// gitCommitCount returns the number of commits reachable from ref (plus any
// additional git rev-list args like "--not", "SHA").
func gitCommitCount(t *testing.T, projectRoot string, refAndArgs ...string) int {
	t.Helper()
	n, err := gitCommitCountOutput(projectRoot, refAndArgs...)
	require.NoError(t, err, "git rev-list --count %v", refAndArgs)
	return n
}

func gitCommitCountOutput(projectRoot string, refAndArgs ...string) (int, error) {
	args := append([]string{"rev-list", "--count"}, refAndArgs...)
	out, err := runGitIntegOutput(projectRoot, args...)
	if err != nil {
		return 0, fmt.Errorf("git %v in %s: %w (%s)", args, projectRoot, err, out)
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("parse git rev-list count %q: %w", out, err)
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Store helpers
// ---------------------------------------------------------------------------

// makeLoopStore creates an ExecuteBeadLoopStore backed by a bead.Store rooted
// in ddxDir. The store is already initialised by newScriptHarnessRepo.
func makeLoopStore(t *testing.T, ddxDir string) ExecuteBeadLoopStore {
	t.Helper()
	return bead.NewStore(ddxDir)
}

// countClosedBeads counts how many beads in ddxDir have status "closed".
func countClosedBeads(t *testing.T, ddxDir string) int {
	t.Helper()
	store := bead.NewStore(ddxDir)
	all, err := store.List("", "", nil)
	require.NoError(t, err)
	n := 0
	for _, b := range all {
		if b.Status == bead.StatusClosed {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Shell quoting helper
// ---------------------------------------------------------------------------

// escapeForShell escapes a string for embedding in a single-quoted shell argument.
func escapeForShell(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

func requireConcurrentScriptHarnessLandingInvariant(
	t *testing.T,
	projectRoot, ddxDir, initialSHA string,
	results []*ExecuteBeadLoopResult,
	errs []error,
	beadCount int,
) {
	t.Helper()

	failures := make([]string, 0)
	totalAttempts := 0
	totalSuccesses := 0
	executedByBead := make(map[string]int, beadCount)

	if len(results) != beadCount {
		failures = append(failures, fmt.Sprintf("worker result count=%d, want %d", len(results), beadCount))
	}
	if len(errs) != beadCount {
		failures = append(failures, fmt.Sprintf("worker error count=%d, want %d", len(errs), beadCount))
	}

	for i, err := range errs {
		if err != nil {
			failures = append(failures, fmt.Sprintf("worker %d returned error: %v", i, err))
		}
	}

	for i, result := range results {
		if result == nil {
			failures = append(failures, fmt.Sprintf("worker %d returned nil loop result", i))
			continue
		}
		totalAttempts += result.Attempts
		totalSuccesses += result.Successes
		for _, report := range result.Results {
			executedByBead[report.BeadID]++
			if report.Status != ExecuteBeadStatusSuccess {
				failures = append(failures,
					fmt.Sprintf("worker %d reported bead %s status=%s detail=%q", i, report.BeadID, report.Status, report.Detail))
			}
		}
	}

	if totalAttempts != beadCount {
		failures = append(failures, fmt.Sprintf("total attempts=%d, want %d", totalAttempts, beadCount))
	}
	if totalSuccesses != beadCount {
		failures = append(failures, fmt.Sprintf("total successes=%d, want %d", totalSuccesses, beadCount))
	}

	for i := 1; i <= beadCount; i++ {
		beadID := fmt.Sprintf("ddx-int-%04d", i)
		if got := executedByBead[beadID]; got != 1 {
			failures = append(failures, fmt.Sprintf("bead %s executed %d times, want exactly once", beadID, got))
		}
	}

	store := bead.NewStore(ddxDir)
	all, err := store.List("", "", nil)
	if err != nil {
		failures = append(failures, fmt.Sprintf("list beads: %v", err))
	} else {
		byID := make(map[string]bead.Bead, len(all))
		closedCount := 0
		for _, item := range all {
			byID[item.ID] = item
			if item.Status == bead.StatusClosed {
				closedCount++
			}
		}
		if len(all) != beadCount {
			failures = append(failures, fmt.Sprintf("bead count=%d, want %d", len(all), beadCount))
		}
		if closedCount != beadCount {
			failures = append(failures, fmt.Sprintf("closed bead count=%d, want %d", closedCount, beadCount))
		}
		for i := 1; i <= beadCount; i++ {
			beadID := fmt.Sprintf("ddx-int-%04d", i)
			item, ok := byID[beadID]
			if !ok {
				failures = append(failures, fmt.Sprintf("missing bead row for %s", beadID))
				continue
			}
			if item.Status != bead.StatusClosed {
				failures = append(failures, fmt.Sprintf("bead %s status=%s, want closed", beadID, item.Status))
			}
			if closingSHA := extraString(item.Extra, "closing_commit_sha"); closingSHA == "" {
				failures = append(failures, fmt.Sprintf("bead %s missing closing_commit_sha", beadID))
			}
		}
	}

	commitsOnMain, err := gitCommitCountOutput(projectRoot, "HEAD", "--not", initialSHA)
	if err != nil {
		failures = append(failures, fmt.Sprintf("count commits on main: %v", err))
	} else if commitsOnMain < beadCount {
		failures = append(failures,
			fmt.Sprintf("commits on main since seed=%d, want at least %d", commitsOnMain, beadCount))
	}

	for i := 1; i <= beadCount; i++ {
		fileName := fmt.Sprintf("bead-%04d.txt", i)
		out, err := runGitIntegOutput(projectRoot, "show", "HEAD:"+fileName)
		if err != nil {
			failures = append(failures,
				fmt.Sprintf("HEAD:%s not reachable: %v (%s)", fileName, err, strings.TrimSpace(out)))
		}
	}

	if len(failures) == 0 {
		return
	}

	t.Fatalf("concurrent script-harness landing invariant failed:\n- %s\n\nDiagnostics:\n%s",
		strings.Join(failures, "\n- "),
		formatConcurrentLandingDiagnostics(projectRoot, ddxDir, results, errs),
	)
}

func formatConcurrentLandingDiagnostics(projectRoot, ddxDir string, results []*ExecuteBeadLoopResult, errs []error) string {
	var b strings.Builder

	b.WriteString("workers:\n")
	for i := 0; i < maxLandingDiagInt(len(results), len(errs)); i++ {
		var (
			result *ExecuteBeadLoopResult
			err    error
		)
		if i < len(results) {
			result = results[i]
		}
		if i < len(errs) {
			err = errs[i]
		}
		b.WriteString(fmt.Sprintf("  worker-%d err=%v", i, err))
		if result == nil {
			b.WriteString(" result=<nil>\n")
			continue
		}
		b.WriteString(fmt.Sprintf(
			" attempts=%d successes=%d failures=%d stop=%q exit=%q no_ready=%t no_ready_detail=%+v\n",
			result.Attempts,
			result.Successes,
			result.Failures,
			result.StopCondition,
			result.ExitReason,
			result.NoReadyWork,
			result.NoReadyWorkDetail,
		))
		for _, report := range result.Results {
			b.WriteString(fmt.Sprintf(
				"    bead=%s status=%s detail=%q attempt=%s result_rev=%s landed_rev=%s preserve_ref=%s\n",
				report.BeadID,
				report.Status,
				report.Detail,
				report.AttemptID,
				report.ResultRev,
				report.LandedRev,
				report.PreserveRef,
			))
		}
	}

	store := bead.NewStore(ddxDir)
	all, err := store.List("", "", nil)
	if err != nil {
		b.WriteString(fmt.Sprintf("beads: list error: %v\n", err))
	} else {
		sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
		closedCount := 0
		b.WriteString("bead lifecycle summary:\n")
		for _, item := range all {
			if item.Status == bead.StatusClosed {
				closedCount++
			}
			events, eventsErr := store.Events(item.ID)
			lastKinds := ""
			if eventsErr != nil {
				lastKinds = "events-error:" + eventsErr.Error()
			} else {
				lastKinds = tailEventKinds(events, 3)
			}
			b.WriteString(fmt.Sprintf(
				"  %s status=%s owner=%s closing_commit_sha=%s work_last_status=%s events=%d last_events=%s\n",
				item.ID,
				item.Status,
				item.Owner,
				extraString(item.Extra, "closing_commit_sha"),
				extraString(item.Extra, "work-last-status"),
				len(events),
				lastKinds,
			))
		}
		b.WriteString(fmt.Sprintf("  closed_count=%d total=%d\n", closedCount, len(all)))
		if breakdown, breakdownErr := store.ReadyExecutionBreakdown(); breakdownErr != nil {
			b.WriteString(fmt.Sprintf("queue breakdown error: %v\n", breakdownErr))
		} else {
			b.WriteString(fmt.Sprintf(
				"queue breakdown: ready=%v dependency_waiting=%v proposed=%v retry_cooldown=%v external_blocked=%v not_eligible=%v superseded=%v epics=%v epic_closure_candidates=%v next_retry_after=%s human_review_blocked_total=%d\n",
				breakdown.ExecutionReady,
				breakdown.DependencyWaiting,
				breakdown.ProposedOperatorAttention,
				breakdown.RetryCooldown,
				breakdown.ExternalBlocked,
				breakdown.NotEligible,
				breakdown.Superseded,
				breakdown.Epics,
				breakdown.EpicClosureCandidates,
				breakdown.NextRetryAfter,
				breakdown.HumanReviewBlockedTotal,
			))
		}
	}

	b.WriteString("git refs:\n")
	head, headErr := runGitIntegOutput(projectRoot, "rev-parse", "HEAD")
	b.WriteString(fmt.Sprintf("  HEAD=%s err=%v\n", strings.TrimSpace(head), headErr))
	refs, refsErr := runGitIntegOutput(projectRoot, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads", "refs/ddx/iterations")
	if refsErr != nil {
		b.WriteString(fmt.Sprintf("  for-each-ref error: %v\n", refsErr))
	} else if strings.TrimSpace(refs) == "" {
		b.WriteString("  (no refs)\n")
	} else {
		for _, line := range strings.Split(strings.TrimSpace(refs), "\n") {
			b.WriteString("  " + line + "\n")
		}
	}

	return b.String()
}

func extraString(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	raw, ok := extra[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func tailEventKinds(events []bead.BeadEvent, n int) string {
	if len(events) == 0 {
		return "(none)"
	}
	if n > len(events) {
		n = len(events)
	}
	kinds := make([]string, 0, n)
	for _, event := range events[len(events)-n:] {
		kinds = append(kinds, event.Kind)
	}
	return strings.Join(kinds, ",")
}

func maxLandingDiagInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
