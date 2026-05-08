package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
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
	ddxDir := filepath.Join(root, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init())

	for i := 0; i < beadCount; i++ {
		id := fmt.Sprintf("ddx-int-%04d", i+1)
		b := &bead.Bead{
			ID:        id,
			Title:     fmt.Sprintf("Integration test bead %d", i+1),
			IssueType: "task",
			Priority:  i,
		}
		require.NoError(t, store.Create(b))
	}

	// Commit beads.jsonl so the initial worktree contains it.
	runGitInteg(t, root, "add", ".ddx/beads.jsonl")
	runGitInteg(t, root, "commit", "-m", "chore: seed beads")

	return root, initialSHA
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
	// calls do not race on BuiltinCatalog (NewRunner is not goroutine-safe
	// during catalog construction).
	runner := NewRunner(Config{})
	gitOps := &RealGitOps{}
	orchGitOps := &RealOrchestratorGitOps{}
	// Per-projectRoot mutex serializes git operations so concurrent workers
	// don't race on the git index or on Land()'s CAS UpdateRefTo.
	repoMu := landMutexFor(projectRoot)

	return ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		repoMu.Lock()
		defer repoMu.Unlock()

		res, err := ExecuteBead(ctx, projectRoot, beadID, ExecuteBeadOptions{
			Harness:     "script",
			Model:       directivePath,
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
	}
}

// ---------------------------------------------------------------------------
// Git assertion helpers
// ---------------------------------------------------------------------------

// gitCommitCount returns the number of commits reachable from ref (plus any
// additional git rev-list args like "--not", "SHA").
func gitCommitCount(t *testing.T, projectRoot string, refAndArgs ...string) int {
	t.Helper()
	args := append([]string{"rev-list", "--count"}, refAndArgs...)
	out := runGitInteg(t, projectRoot, args...)
	n, err := strconv.Atoi(out)
	require.NoError(t, err, "git rev-list --count %v", refAndArgs)
	return n
}

// gitHasMergeCommits returns true when any merge commits exist in ref's history.
func gitHasMergeCommits(t *testing.T, projectRoot, ref string) bool {
	t.Helper()
	out := runGitInteg(t, projectRoot, "log", "--merges", "--format=%H", ref)
	return out != ""
}

// refExists returns true when the given git ref is present in the repo.
func refExists(t *testing.T, projectRoot, ref string) bool {
	t.Helper()
	_, err := runGitIntegOutput(projectRoot, "rev-parse", "--verify", ref)
	return err == nil
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
