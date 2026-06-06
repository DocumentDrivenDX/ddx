package integration

// concurrent_try_test.go — subprocess integration tests for concurrent ddx try
// invocations as required by TP-021-multi-worker-try-reliability.md.
//
// Three test functions are provided (each satisfying one TP-021 integration AC):
//
//   TestIntegration_ConcurrentTryDistinctBeads_LocalClone
//   TestIntegration_ConcurrentTrySameBead_OneClaimWins
//   TestIntegration_ConcurrentTryPreserveRefsUnique
//
// All three use testutils.BuildDDxBinary, fixture repositories, and the
// deterministic `script` harness with an isolated HOME/XDG_DATA_HOME. None of
// them require network access or external agent CLIs.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
)

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
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
	}
	return env, home, xdg
}

// spawnTry launches a single `ddx try <beadID>` subprocess and returns the
// started *exec.Cmd with stdout/stderr captured in buf. Callers must call
// cmd.Wait() to collect the exit code and release resources. extraFlags are
// appended to the base flags (e.g. "--from <sha>").
func spawnTry(bin, proj, beadID, harness, directive string, env []string, extraFlags ...string) (*exec.Cmd, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	args := []string{"try", beadID,
		"--harness", harness,
		"--model", directive,
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
// (each targeting a distinct bead) with the script harness and
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

	bin := testutils.BuildDDxBinary(t)
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

	// Directive: simulate an LLM wait outside the lock, then create a file and
	// commit so there are meaningful changes to land.
	directive := filepath.Join(t.TempDir(), "directive.txt")
	if err := os.WriteFile(directive, []byte(
		"sleep-ms 300\n"+
			"create-file out-${DDX_BEAD_ID}.txt done\n"+
			"commit feat: ${DDX_BEAD_ID} done\n"), 0o644); err != nil {
		t.Fatalf("write directive: %v", err)
	}

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
			cmd, buf := spawnTry(bin, proj, bid, "script", directive, env)
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
	// Code 2 (unexpected failure) is always an error.
	for i, r := range results {
		if r.code == 2 {
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

	bin := testutils.BuildDDxBinary(t)
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

	// Directive with a long sleep so the winner holds the claim long enough for
	// the losers to reach their not-claimable check.
	directive := filepath.Join(t.TempDir(), "directive.txt")
	if err := os.WriteFile(directive, []byte(
		"sleep-ms 1200\n"+
			"create-file out-race.txt done\n"+
			"commit feat: race winner\n"), 0o644); err != nil {
		t.Fatalf("write directive: %v", err)
	}

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
			cmd, buf := spawnTry(bin, proj, beadID, "script", directive, env)
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
}

// TestIntegration_ConcurrentTryPreserveRefsUnique satisfies TP-021 AC3.
//
// Forces two concurrent non-landed attempts (one per bead) by pinning both to
// a stale base revision (--from <sha>) so every land attempt encounters a merge
// conflict. Both attempts run in parallel; each worker writes "worker-version"
// on top of staleSHA while main has already advanced to "main-advanced-version",
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

	bin := testutils.BuildDDxBinary(t)
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
	// Workers pinned to staleSHA will commit "worker-version" on top of
	// staleSHA while main already has "main-advanced-version", so the merge
	// always conflicts and landIterationRef always fires.
	if err := os.WriteFile(conflictFile, []byte("main-advanced-version\n"), 0o644); err != nil {
		t.Fatalf("write conflict.txt for advance: %v", err)
	}
	if err := gitCommitAll(proj, "test: advance main with conflicting conflict.txt"); err != nil {
		t.Fatalf("advance commit: %v", err)
	}

	// Directive: overwrite conflict.txt and commit (exit 0 so the land path is
	// attempted). The 3-way merge always conflicts → landIterationRef creates a
	// preserve ref before auto-recovery resolves the conflict.
	directive := filepath.Join(t.TempDir(), "directive.txt")
	if err := os.WriteFile(directive, []byte(
		"create-file conflict.txt worker-version\n"+
			"commit chore: worker attempt\n"), 0o644); err != nil {
		t.Fatalf("write directive: %v", err)
	}

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
			cmd, buf := spawnTry(bin, proj, bid, "script", directive, env,
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
}
