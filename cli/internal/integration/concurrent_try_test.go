package integration

// concurrent_try_test.go — subprocess integration tests for concurrent ddx try
// invocations as required by TP-021-multi-worker-try-reliability.md.
//
// Four test functions are provided (the first three satisfy the TP-021
// integration ACs and the fourth guards the hermetic provider boundary):
//
//   TestIntegration_ConcurrentTryDistinctBeads_LocalClone
//   TestIntegration_ConcurrentTrySameBead_OneClaimWins
//   TestIntegration_ConcurrentTryPreserveRefsUnique
//   TestIntegration_ConcurrentTryFizeauSeamLaunchesNoLiveProvider
//
// All four use a ddx binary compiled with Fizeau's public testseam tag and a
// deterministic FakeProvider with isolated HOME/XDG_DATA_HOME. None require
// network access or external agent CLIs.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
)

const fizeauTestPlanEnv = "DDX_FIZEAU_TEST_PLAN"

type concurrentTryFizeauPlan struct {
	SleepMS          int      `json:"sleep_ms,omitempty"`
	WritePath        string   `json:"write_path"`
	WriteContent     string   `json:"write_content"`
	CommitMessage    string   `json:"commit_message"`
	SeamLog          string   `json:"seam_log,omitempty"`
	TripwireBinDir   string   `json:"tripwire_bin_dir,omitempty"`
	TripwireNames    []string `json:"tripwire_names,omitempty"`
	TripwireSentinel string   `json:"tripwire_sentinel,omitempty"`
}

func withFizeauTestPlan(t *testing.T, env []string, plan concurrentTryFizeauPlan) ([]string, string) {
	t.Helper()
	dir := t.TempDir()
	plan.SeamLog = filepath.Join(dir, "fizeau-testseam.log")
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal Fizeau test plan: %v", err)
	}
	planPath := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(planPath, raw, 0o600); err != nil {
		t.Fatalf("write Fizeau test plan: %v", err)
	}
	return append(env, fizeauTestPlanEnv+"="+planPath), plan.SeamLog
}

func requireFizeauTestSeamExecutions(t *testing.T, logPath string, want int) {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read Fizeau test seam log: %v", err)
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.Contains(line, "mode=bead_execution") {
			count++
			if !strings.Contains(line, "harness=fiz") {
				t.Errorf("test-seam execution did not use Fizeau native harness: %s", line)
			}
		}
	}
	if count < want {
		t.Errorf("Fizeau test-seam execution count=%d, want at least %d; log:\n%s", count, want, raw)
	}
}

func setSubprocessEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i := range env {
		if strings.HasPrefix(env[i], prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

// subprocessEnvFor builds the minimal isolated subprocess environment used by
// all concurrent-try tests. HOME and XDG_DATA_HOME are set to fresh temp dirs
// so workers do not read real developer config or register with the local
// ddx-server. PATH is stripped to git + standard system directories so agent
// CLIs (codex, claude, gemini) cannot be discovered and daemonised.
func subprocessEnvFor(t *testing.T, bin string) (env []string, home, xdg string) {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("subprocessEnvFor: locate git binary: %v", err)
	}
	minimalPATH := strings.Join(
		[]string{filepath.Dir(gitPath), filepath.Dir(bin), "/usr/bin", "/bin", "/usr/sbin", "/sbin"},
		string(os.PathListSeparator),
	)
	home = t.TempDir()
	xdg = t.TempDir()
	env = []string{
		"HOME=" + home,
		"XDG_DATA_HOME=" + xdg,
		"PATH=" + minimalPATH,
		"DDX_DISABLE_UPDATE_CHECK=1",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
	}
	return env, home, xdg
}

// spawnTry launches a single `ddx try <beadID>` subprocess and returns the
// started *exec.Cmd with stdout/stderr captured in buf. Callers must call
// cmd.Wait() to collect the exit code and release resources. extraFlags are
// appended to the base flags (e.g. "--from <sha>").
func spawnTry(bin, proj, beadID string, env []string, extraFlags ...string) (*exec.Cmd, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	args := []string{"try", beadID,
		"--harness", "fiz",
		"--attempt-backend", "local-clone",
		"--no-review", "--no-review-i-know-what-im-doing",
		"--project", proj,
	}
	args = append(args, extraFlags...)
	cmd := exec.Command(bin, args...)
	cmd.Dir = proj
	cmd.Env = env
	cmd.Stdout = buf
	cmd.Stderr = buf
	return cmd, buf
}

// isLandCoordinationRetryOutput reports whether a `ddx try` subprocess exited
// non-zero solely because the land stage lost a compare-and-swap on the shared
// target branch to a sibling worker (status: land_retry). The attempt's work is
// preserved on a refs/ddx/iterations/... ref and the bead stays claimable, so
// under deliberate concurrency this is expected coordination, not a failure.
func isLandCoordinationRetryOutput(output string) bool {
	return strings.Contains(output, "status: "+agent.ExecuteBeadStatusLandRetry)
}

// cloneAttemptDirs returns the paths of any lingering .execute-bead-clone-*
// directories under the subprocess's cache area (HOME/.cache/ddx/exec-wt/).
// A subprocess that exits cleanly should remove these; they are only present
// during an active attempt or if cleanup failed.
func cloneAttemptDirs(home string) ([]string, error) {
	cloneRoot := filepath.Join(home, ".cache", "ddx", "exec-wt")
	entries, err := os.ReadDir(cloneRoot)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var found []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".execute-bead-clone-") {
			found = append(found, filepath.Join(cloneRoot, e.Name()))
		}
	}
	return found, nil
}

// parsePreserveRef extracts the preserve_ref value from ddx try output.
func parsePreserveRef(output string) string {
	const prefix = "preserve_ref: "
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return ""
}

// gitRunIn runs a git command in dir and returns its output, fatally failing t
// on error. env defaults to the current process environment.
func gitRunIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// An "empty commit" is not a test failure.
		if strings.Contains(string(out), "nothing to commit") {
			return string(out)
		}
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestIntegration_ConcurrentTryDistinctBeads_LocalClone satisfies TP-021 AC1.
//
// Seeds 8 independent beads and runs 4 concurrent `ddx try <id>` subprocesses
// (each targeting a distinct bead) through Fizeau's FakeProvider seam and
// --attempt-backend local-clone. Asserts:
//
//   - all 4 subprocesses exit (success or preserved)
//   - attempt IDs (derived from evidence bundle names) are unique
//   - no lingering .execute-bead-clone-* directories remain after all
//     subprocesses exit
func TestIntegration_ConcurrentTryDistinctBeads_LocalClone(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: spawns ddx try subprocesses")
	}

	bin := testutils.BuildDDxFizeauTestSeamBinary(t)
	t.Setenv("DDX_BIN", bin)
	proj := testutils.NewFixtureRepo(t, "minimal")
	env, home, _ := subprocessEnvFor(t, bin)

	// Suppress auto-materialized paths so the initial tree stays clean.
	if err := appendGitignore(proj, ".agents/", ".claude/", ".ddx/lifecycle-schema.json"); err != nil {
		t.Fatalf("append gitignore: %v", err)
	}
	if err := gitCommitAll(proj, "test: ignore auto-materialized paths"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Seed 8 independent beads so there is no queue contention.
	const totalBeads = 8
	const concurrentWorkers = 4
	beadIDs, err := createBeads(bin, proj, env, totalBeads, "Concurrent try distinct bead")
	if err != nil {
		t.Fatalf("seed beads: %v", err)
	}
	if err := gitCommitAll(proj, "test: seed distinct beads"); err != nil {
		t.Fatalf("commit beads: %v", err)
	}

	env, seamLog := withFizeauTestPlan(t, env, concurrentTryFizeauPlan{
		SleepMS:       300,
		WritePath:     "out-${DDX_BEAD_ID}.txt",
		WriteContent:  "done\n",
		CommitMessage: "feat: ${DDX_BEAD_ID} done",
	})

	// Run concurrentWorkers concurrent ddx try subprocesses against distinct beads.
	type result struct {
		beadID string
		output string
		code   int
	}
	results := make([]result, concurrentWorkers)
	var wg sync.WaitGroup
	for i := 0; i < concurrentWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			bid := beadIDs[idx]
			cmd, buf := spawnTry(bin, proj, bid, env)
			if err := cmd.Start(); err != nil {
				t.Errorf("start try for bead %s: %v", bid, err)
				return
			}
			_ = cmd.Wait()
			code := 0
			if cmd.ProcessState != nil {
				code = cmd.ProcessState.ExitCode()
			}
			results[idx] = result{beadID: bid, output: buf.String(), code: code}
		}(i)
	}
	wg.Wait()

	// All 4 subprocesses must have exited (code 0=success, 1=preserved, 2=other).
	// We accept both 0 and 1 because a concurrent land race can preserve a
	// result (another worker on a shared fixture may have advanced the branch).
	//
	// The same shared-branch contention also surfaces as the retryable
	// land_retry status: the `git update-ref` compare-and-swap on the target
	// branch loses to a sibling worker that advanced it mid-merge. `ddx try` is
	// single-shot, so it reports that with exit code 2 even though the work is
	// preserved on a refs/ddx/iterations/... ref and the bead stays claimable.
	// That belongs with codes 0 and 1, not with genuine execution failures;
	// race-detector overhead makes contention heavy enough to hit it regularly.
	// Every other exit-code-2 failure is still an error.
	for i, r := range results {
		if r.code == 2 && !isLandCoordinationRetryOutput(r.output) {
			t.Errorf("worker %d (bead %s) exited with code 2 (unexpected failure):\n%s", i, r.beadID, r.output)
		}
	}

	// No lingering clone attempt directories after all subprocesses exit.
	lingering, err := cloneAttemptDirs(home)
	if err != nil {
		t.Fatalf("reading clone attempt dirs: %v", err)
	}
	if len(lingering) > 0 {
		t.Errorf("lingering attempt directories after all subprocesses exited:\n%s",
			strings.Join(lingering, "\n"))
	}

	// Attempt IDs must be unique. Collect them from the evidence bundle
	// directory names under .ddx/executions/.
	executionsDir := filepath.Join(ddxroot.JoinProject(proj), agent.ExecuteBeadArtifactDir[len(".ddx/"):])
	entries, err := os.ReadDir(executionsDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("reading executions dir: %v", err)
	}
	seen := map[string]bool{}
	var attemptIDs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Evidence bundle dirs are named with the attempt ID (a timestamp-hex string).
		if name == "reviewer-streams" || name == "mirror-index.jsonl" {
			continue
		}
		if seen[name] {
			t.Errorf("duplicate attempt ID in evidence dirs: %s", name)
		}
		seen[name] = true
		attemptIDs = append(attemptIDs, name)
	}
	// At least concurrentWorkers evidence entries must exist (one per attempt).
	if len(attemptIDs) < concurrentWorkers {
		t.Errorf("expected at least %d evidence bundle dirs, found %d", concurrentWorkers, len(attemptIDs))
	}
	requireFizeauTestSeamExecutions(t, seamLog, concurrentWorkers)
}

// TestIntegration_ConcurrentTrySameBead_OneClaimWins satisfies TP-021 AC2.
//
// Runs 3 concurrent `ddx try <same-id>` subprocesses against a single bead.
// Asserts:
//
//   - at most one subprocess claims/runs the bead (exits with code 0)
//   - losing subprocesses exit through the not-claimable path (exit code != 0)
//     and emit "not execution-ready" or "not claimable" on stderr
//   - losers do not create evidence bundle result.json files
func TestIntegration_ConcurrentTrySameBead_OneClaimWins(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: spawns ddx try subprocesses")
	}

	bin := testutils.BuildDDxFizeauTestSeamBinary(t)
	t.Setenv("DDX_BIN", bin)
	proj := testutils.NewFixtureRepo(t, "minimal")
	env, _, _ := subprocessEnvFor(t, bin)

	if err := appendGitignore(proj, ".agents/", ".claude/", ".ddx/lifecycle-schema.json"); err != nil {
		t.Fatalf("append gitignore: %v", err)
	}
	if err := gitCommitAll(proj, "test: ignore auto-materialized paths"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	beadIDs, err := createBeads(bin, proj, env, 1, "Same bead claim race")
	if err != nil {
		t.Fatalf("seed bead: %v", err)
	}
	if err := gitCommitAll(proj, "test: seed bead"); err != nil {
		t.Fatalf("commit bead: %v", err)
	}
	beadID := beadIDs[0]

	env, seamLog := withFizeauTestPlan(t, env, concurrentTryFizeauPlan{
		SleepMS:       1200,
		WritePath:     "out-race.txt",
		WriteContent:  "done\n",
		CommitMessage: "feat: race winner",
	})

	const concurrentAttempts = 3
	type result struct {
		output string
		code   int
	}
	results := make([]result, concurrentAttempts)
	var wg sync.WaitGroup
	for i := 0; i < concurrentAttempts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cmd, buf := spawnTry(bin, proj, beadID, env)
			if err := cmd.Start(); err != nil {
				t.Errorf("start attempt %d: %v", idx, err)
				return
			}
			_ = cmd.Wait()
			code := 0
			if cmd.ProcessState != nil {
				code = cmd.ProcessState.ExitCode()
			}
			results[idx] = result{output: buf.String(), code: code}
		}(i)
	}
	wg.Wait()

	// Count successful (code 0) and losing (non-zero) exits.
	successes := 0
	var loserOutputs []string
	for _, r := range results {
		if r.code == 0 {
			successes++
		} else {
			loserOutputs = append(loserOutputs, r.output)
		}
	}

	// At most one claim must win. (Zero wins can happen if the land fails, which
	// is fine — the test proves no MULTI-claim scenario.)
	if successes > 1 {
		t.Errorf("expected at most 1 successful claim, got %d", successes)
	}

	// All losers must have exited through the not-claimable path.
	for i, out := range loserOutputs {
		lower := strings.ToLower(out)
		if !strings.Contains(lower, "not execution-ready") &&
			!strings.Contains(lower, "not claimable") &&
			!strings.Contains(lower, "already claimed") {
			t.Errorf("loser %d output does not contain a claim-failure message:\n%s", i, out)
		}
	}

	// Losers must not have created result.json evidence bundles. The winner's
	// bundle is expected; loser bundles would indicate an unexpected execution.
	executionsDir := filepath.Join(ddxroot.JoinProject(proj), agent.ExecuteBeadArtifactDir[len(".ddx/"):])
	entries, _ := os.ReadDir(executionsDir)
	bundleCount := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "reviewer-streams" {
			continue
		}
		resultJSON := filepath.Join(executionsDir, name, "result.json")
		if _, statErr := os.Stat(resultJSON); statErr == nil {
			bundleCount++
		}
	}
	// At most 1 result.json (from the winner). Losers exit before executing.
	if bundleCount > 1 {
		t.Errorf("expected at most 1 result.json evidence bundle, found %d (losers created bundles)", bundleCount)
	}
	requireFizeauTestSeamExecutions(t, seamLog, 1)
}

// TestIntegration_ConcurrentTryPreserveRefsUnique satisfies TP-021 AC3.
//
// Forces two concurrent non-landed attempts (one per bead) by pinning both to
// a stale base revision (--from <sha>) so every land attempt encounters a merge
// conflict. Both attempts run in parallel; each worker writes a bead-specific
// worker version on top of staleSHA while main has advanced the same line,
// producing a 3-way merge conflict. The land path calls landIterationRef to
// create a preserve ref before attempting auto-recovery, so both attempts
// produce a durable refs/ddx/iterations/<bead-id>/<attemptID>-<sha> ref even
// when auto-recovery later succeeds.
//
// Proves that refs under refs/ddx/iterations/ are unique even when attempts
// start within the same second. Uniqueness is guaranteed because landIterationRef
// uses attemptID (timestamp+4-byte-random) in the ref name, not a
// second-precision timestamp alone.
func TestIntegration_ConcurrentTryPreserveRefsUnique(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: spawns ddx try subprocesses")
	}

	bin := testutils.BuildDDxFizeauTestSeamBinary(t)
	t.Setenv("DDX_BIN", bin)
	proj := testutils.NewFixtureRepo(t, "minimal")
	env, _, _ := subprocessEnvFor(t, bin)

	if err := appendGitignore(proj, ".agents/", ".claude/", ".ddx/lifecycle-schema.json"); err != nil {
		t.Fatalf("append gitignore: %v", err)
	}

	// Step 1: create conflict.txt.
	conflictFile := filepath.Join(proj, "conflict.txt")
	if err := os.WriteFile(conflictFile, []byte("initial\n"), 0o644); err != nil {
		t.Fatalf("write conflict.txt: %v", err)
	}
	if err := gitCommitAll(proj, "test: add conflict.txt and gitignore"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Step 2: create and commit two beads (both must exist at staleSHA so their
	// local-clone workers can load them from .ddx/beads.jsonl).
	const numBeads = 2
	beadIDs, err := createBeads(bin, proj, env, numBeads, "Preserve ref uniqueness")
	if err != nil {
		t.Fatalf("seed beads: %v", err)
	}
	if err := gitCommitAll(proj, "test: seed beads"); err != nil {
		t.Fatalf("commit beads: %v", err)
	}

	// Capture staleSHA AFTER the bead commit so each clone at staleSHA can find
	// the beads in .ddx/beads.jsonl and conflict.txt is still "initial\n".
	staleSHA := gitRunIn(t, proj, "rev-parse", "HEAD")

	// Step 3: advance main with a conflicting change to conflict.txt.
	// Workers pinned to staleSHA will commit a bead-specific worker version on top of
	// staleSHA while main already has "main-advanced-version", so the merge
	// always conflicts and landIterationRef always fires.
	if err := os.WriteFile(conflictFile, []byte("main-advanced-version\n"), 0o644); err != nil {
		t.Fatalf("write conflict.txt for advance: %v", err)
	}
	if err := gitCommitAll(proj, "test: advance main with conflicting conflict.txt"); err != nil {
		t.Fatalf("advance commit: %v", err)
	}

	env, seamLog := withFizeauTestPlan(t, env, concurrentTryFizeauPlan{
		WritePath:     "conflict.txt",
		WriteContent:  "worker-${DDX_BEAD_ID}\n",
		CommitMessage: "chore: worker attempt ${DDX_BEAD_ID}",
	})

	// Run numBeads concurrent ddx try subprocesses, one per bead, all pinned
	// to staleSHA. Both workers experience a merge conflict on landing and each
	// records a unique preserve ref under refs/ddx/iterations/<bead-id>/.
	type tryResult struct {
		beadID string
		output string
	}
	results := make([]tryResult, numBeads)
	var wg sync.WaitGroup
	for i := 0; i < numBeads; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			bid := beadIDs[idx]
			cmd, buf := spawnTry(bin, proj, bid, env,
				"--from", staleSHA,
			)
			if startErr := cmd.Start(); startErr != nil {
				t.Errorf("start try for bead %s: %v", bid, startErr)
				return
			}
			_ = cmd.Wait()
			results[idx] = tryResult{beadID: bid, output: buf.String()}
		}(i)
	}
	wg.Wait()

	// Log all attempt outputs for diagnosis.
	for i, r := range results {
		t.Logf("attempt %d (bead %s) output:\n%s", i, r.beadID, r.output)
	}

	// Collect preserve refs from all attempts. Every attempt must produce one
	// because the merge always conflicts and landIterationRef fires before
	// auto-recovery.
	var allPreserveRefs []string
	for i, r := range results {
		ref := parsePreserveRef(r.output)
		if ref == "" {
			t.Errorf("attempt %d (bead %s): no preserve_ref found in output; "+
				"land may have succeeded without conflict or the attempt errored before landing",
				i, r.beadID)
			continue
		}
		allPreserveRefs = append(allPreserveRefs, ref)
	}
	if t.Failed() {
		return
	}

	// Verify all refs are under the expected iterations namespace.
	for _, ref := range allPreserveRefs {
		if !strings.HasPrefix(ref, "refs/ddx/iterations/") {
			t.Errorf("preserve ref %q does not start with refs/ddx/iterations/", ref)
		}
	}

	// Verify refs are unique (uniqueness proof for same-second scenarios).
	// landIterationRef embeds attemptID = timestamp+4-byte-random, so two
	// concurrent attempts within the same second still produce distinct refs.
	seen := map[string]bool{}
	for _, ref := range allPreserveRefs {
		if seen[ref] {
			t.Errorf("duplicate preserve ref detected: %q (refs must be unique even within the same second)", ref)
		}
		seen[ref] = true
	}

	// Verify each ref exists and resolves in git.
	for _, ref := range allPreserveRefs {
		sha := gitRunIn(t, proj, "rev-parse", "--verify", ref)
		if sha == "" {
			t.Errorf("preserve ref %q not resolvable in git repo", ref)
		}
	}
	requireFizeauTestSeamExecutions(t, seamLog, numBeads)
}

// TestIntegration_ConcurrentTryFizeauSeamLaunchesNoLiveProvider proves the
// tagged subprocess path terminates through Fizeau's FakeProvider without
// launching a subscription harness CLI or opening an HTTP provider
// connection. Executable tripwires fail loudly if a known harness is spawned;
// a local proxy listener counts any attempted network-provider connection.
func TestIntegration_ConcurrentTryFizeauSeamLaunchesNoLiveProvider(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: spawns ddx try subprocess")
	}

	bin := testutils.BuildDDxFizeauTestSeamBinary(t)
	t.Setenv("DDX_BIN", bin)
	proj := testutils.NewFixtureRepo(t, "minimal")
	env, _, _ := subprocessEnvFor(t, bin)

	if err := appendGitignore(proj, ".agents/", ".claude/", ".ddx/lifecycle-schema.json"); err != nil {
		t.Fatalf("append gitignore: %v", err)
	}
	if err := gitCommitAll(proj, "test: ignore auto-materialized paths"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}
	beadIDs, err := createBeads(bin, proj, env, 1, "Fizeau seam provider tripwire")
	if err != nil {
		t.Fatalf("seed bead: %v", err)
	}
	if err := gitCommitAll(proj, "test: seed provider tripwire bead"); err != nil {
		t.Fatalf("commit bead: %v", err)
	}

	tripwireDir := t.TempDir()
	tripwireLog := filepath.Join(tripwireDir, "provider-launches.log")
	const tripwireSentinel = "ddx-fizeau-tripwire-sentinel"
	providerNames := []string{"claude", "claude-tui", "codex", "gemini", "opencode", "pi"}
	shim := []byte("#!/bin/sh\nprintf '%s %s\\n' \"$0\" \"$*\" >> \"$DDX_PROVIDER_TRIPWIRE\"\nexit 0\n")
	for _, name := range append(append([]string(nil), providerNames...), tripwireSentinel) {
		if err := os.WriteFile(filepath.Join(tripwireDir, name), shim, 0o700); err != nil {
			t.Fatalf("write provider tripwire %s: %v", name, err)
		}
	}
	env = setSubprocessEnv(env, "DDX_PROVIDER_TRIPWIRE", tripwireLog)
	env, seamLog := withFizeauTestPlan(t, env, concurrentTryFizeauPlan{
		WritePath:        "fizeau-seam-tripwire.txt",
		WriteContent:     "done\n",
		CommitMessage:    "test: Fizeau seam tripwire",
		TripwireBinDir:   tripwireDir,
		TripwireNames:    providerNames,
		TripwireSentinel: tripwireSentinel,
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for HTTP provider tripwire: %v", err)
	}
	var networkConnections atomic.Int32
	var networkRequestsMu sync.Mutex
	var networkRequests []string
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			networkConnections.Add(1)
			_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
			buf := make([]byte, 4096)
			if n, _ := conn.Read(buf); n > 0 {
				networkRequestsMu.Lock()
				networkRequests = append(networkRequests, string(buf[:n]))
				networkRequestsMu.Unlock()
			}
			_ = conn.Close()
		}
	}()
	proxyURL := fmt.Sprintf("http://%s", listener.Addr())
	env = setSubprocessEnv(env, "HTTP_PROXY", proxyURL)
	env = setSubprocessEnv(env, "HTTPS_PROXY", proxyURL)
	env = setSubprocessEnv(env, "NO_PROXY", "")

	cmd, buf := spawnTry(bin, proj, beadIDs[0], env)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start tripwire try: %v", err)
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		t.Fatalf("tripwire try failed: %v\n%s", waitErr, buf.String())
	}
	_ = listener.Close()
	select {
	case <-acceptDone:
	case <-time.After(time.Second):
		t.Fatal("HTTP provider tripwire listener did not stop")
	}

	requireFizeauTestSeamExecutions(t, seamLog, 1)
	raw, readErr := os.ReadFile(tripwireLog)
	if readErr != nil {
		t.Fatalf("read provider launch tripwire: %v", readErr)
	}
	var sentinelObserved bool
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		launched := filepath.Base(fields[0])
		if launched == tripwireSentinel {
			sentinelObserved = true
			continue
		}
		for _, providerName := range providerNames {
			if launched == providerName {
				t.Errorf("live provider CLI launched despite Fizeau FakeProvider seam: %s", line)
			}
		}
	}
	if !sentinelObserved {
		t.Errorf("provider launch observer did not record positive-control sentinel; log:\n%s", raw)
	}
	restrictedPaths, err := filepath.Glob(filepath.Join(filepath.Dir(seamLog), ".fizeau-tools-*"))
	if err != nil || len(restrictedPaths) != 1 {
		t.Fatalf("locate restricted Fizeau test-seam PATH: paths=%v err=%v", restrictedPaths, err)
	}
	for _, name := range append(append([]string(nil), providerNames...), tripwireSentinel) {
		if _, err := os.Lstat(filepath.Join(restrictedPaths[0], name)); err != nil {
			t.Errorf("tripwire %s was not installed in restricted Fizeau PATH: %v", name, err)
		}
	}
	if got := networkConnections.Load(); got != 0 {
		networkRequestsMu.Lock()
		requests := strings.Join(networkRequests, "\n---\n")
		networkRequestsMu.Unlock()
		t.Errorf("network provider connections=%d, want 0; requests:\n%s", got, requests)
	}
}
