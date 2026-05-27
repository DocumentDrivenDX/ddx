package integration

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/lockmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
)

// This file is the wired-in reliability proof for ddx-6650fdc9 / its parent
// scope-locks-to-mutation-window: 5 concurrent `ddx work --watch` workers must
// be able to drain a shared queue while an operator shell issues bead
// create/update/close commands, WITHOUT either side holding .git/index.lock or
// .ddx/.git-tracker.lock long enough to time out the other. The locks are
// scoped to the mutation window only — the harness "LLM wait" (here a script
// `sleep-ms`) happens entirely outside any lock — so p99 hold times stay far
// under the configured caps (DefaultIndexLockCap=10s, DefaultTrackerLockCap=30s
// in internal/lockmetrics/lockcap.go).
//
// The expensive scenario (build a ddx binary, spawn 5 watcher subprocesses and
// 20 operator subprocesses against a fixture repo) runs exactly once via a
// process-wide sync.Once; the four AC-named test functions assert disjoint
// properties of the single captured result.

const (
	workerCount   = 5
	operatorCount = 20
	// workerBeadCount matches the contractual minimum: 5 independent ready
	// beads for 5 workers (the bead's PROPOSED FIX). sleep-ms in the directive
	// keeps each execution's lock-free window open long enough to overlap with
	// the concurrent operator commands.
	workerBeadCount = 5
)

// contentionResult captures everything the four assertions need, so the
// scenario can run once and the fixture can be torn down before later
// assertions execute.
type contentionResult struct {
	indexHoldMS    []float64
	trackerHoldMS  []float64
	operatorOutput []operatorCommand
	workersSpawned int
}

type operatorCommand struct {
	args   string
	output string
	err    error
}

// timedOut reports whether this operator command failed because it could not
// acquire the tracker lock in time (the "tracker lock timeout" error string is
// emitted by lockTimeoutError in internal/agent/tracker_lock.go).
func (c operatorCommand) timedOut() bool {
	return strings.Contains(c.output, "tracker lock timeout")
}

var (
	scenarioOnce   sync.Once
	scenarioResult *contentionResult
	scenarioErr    error
)

// contentionScenario runs the multi-worker scenario at most once per process
// and returns the captured result. Heavy integration work is skipped under
// `go test -short`.
func contentionScenario(t *testing.T) *contentionResult {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: spawns ddx work --watch subprocesses")
	}
	scenarioOnce.Do(func() {
		scenarioResult, scenarioErr = runContentionScenario(t)
	})
	if scenarioErr != nil {
		t.Fatalf("multi-worker lock-contention scenario failed: %v", scenarioErr)
	}
	return scenarioResult
}

// TestIntegration_MultiWorkerLockContention_5Workers proves the scenario runs
// to completion: 5 `ddx work --watch` worker processes were spawned and all 20
// operator bead commands executed.
func TestIntegration_MultiWorkerLockContention_5Workers(t *testing.T) {
	res := contentionScenario(t)
	if res.workersSpawned != workerCount {
		t.Fatalf("expected %d ddx work --watch worker processes, spawned %d", workerCount, res.workersSpawned)
	}
	if len(res.operatorOutput) != operatorCount {
		t.Fatalf("expected %d operator bead commands to run, got %d", operatorCount, len(res.operatorOutput))
	}
	// At least one lock acquisition must have been observed, otherwise the p99
	// assertions below would be vacuously true.
	if len(res.indexHoldMS) == 0 && len(res.trackerHoldMS) == 0 {
		t.Fatal("no lock acquisitions recorded; workers did not exercise the instrumented lock seams")
	}
}

// TestIntegration_MultiWorkerLockContention_P99IndexLockUnderCap asserts the
// p99 .git/index.lock hold time across all acquisitions is under the cap.
func TestIntegration_MultiWorkerLockContention_P99IndexLockUnderCap(t *testing.T) {
	res := contentionScenario(t)
	if len(res.indexHoldMS) == 0 {
		t.Fatal("no index.lock acquisitions recorded")
	}
	p99 := percentile(res.indexHoldMS, 0.99)
	capMS := float64(lockmetrics.DefaultIndexLockCap.Milliseconds())
	if p99 >= capMS {
		t.Fatalf("p99 index.lock hold %.0fms >= cap %.0fms (n=%d, max=%.0fms)",
			p99, capMS, len(res.indexHoldMS), maxFloat(res.indexHoldMS))
	}
}

// TestIntegration_MultiWorkerLockContention_P99TrackerLockUnderCap asserts the
// p99 .ddx/.git-tracker.lock hold time across all acquisitions is under the cap.
func TestIntegration_MultiWorkerLockContention_P99TrackerLockUnderCap(t *testing.T) {
	res := contentionScenario(t)
	if len(res.trackerHoldMS) == 0 {
		t.Fatal("no tracker.lock acquisitions recorded")
	}
	p99 := percentile(res.trackerHoldMS, 0.99)
	capMS := float64(lockmetrics.DefaultTrackerLockCap.Milliseconds())
	if p99 >= capMS {
		t.Fatalf("p99 tracker.lock hold %.0fms >= cap %.0fms (n=%d, max=%.0fms)",
			p99, capMS, len(res.trackerHoldMS), maxFloat(res.trackerHoldMS))
	}
}

// TestIntegration_MultiWorkerLockContention_NoOperatorTimeouts asserts that no
// operator-issued bead command failed with a tracker-lock timeout.
func TestIntegration_MultiWorkerLockContention_NoOperatorTimeouts(t *testing.T) {
	res := contentionScenario(t)
	var timedOut []string
	for _, c := range res.operatorOutput {
		if c.timedOut() {
			timedOut = append(timedOut, fmt.Sprintf("`ddx bead %s`: %s", c.args, strings.TrimSpace(c.output)))
		}
	}
	if len(timedOut) > 0 {
		t.Fatalf("%d operator command(s) failed with tracker lock timeout:\n%s",
			len(timedOut), strings.Join(timedOut, "\n"))
	}
}

// runContentionScenario builds the fixture, spawns the workers and operators,
// drives them to completion, and returns the captured lock metrics + operator
// outcomes.
func runContentionScenario(t *testing.T) (*contentionResult, error) {
	t.Helper()

	// Build ddx from the current source tree (not a possibly-stale `ddx` on
	// PATH) and pin it so the fixture seeding and the spawned workers/operators
	// all exercise the lock instrumentation under test.
	bin := testutils.BuildDDxBinary(t)
	t.Setenv("DDX_BIN", bin)
	proj := testutils.NewFixtureRepo(t, "minimal")
	ddxDir := ddxroot.JoinProject(proj)

	// The first `ddx work` would otherwise be blocked by auto-materialized
	// project-local skill/schema files appearing as dirty implementation
	// changes; ignore them so the worktree stays clean under the dirty guard.
	if err := appendGitignore(proj, ".agents/", ".claude/", ".ddx/lifecycle-schema.json"); err != nil {
		return nil, err
	}
	if err := gitCommitAll(proj, "test: ignore auto-materialized paths"); err != nil {
		return nil, err
	}

	// Subprocesses run with a minimal, isolated environment: an isolated HOME
	// and XDG_DATA_HOME (so workers neither read the developer's ~/.ddx config
	// nor register with this box's real ddx server), and a PATH restricted to
	// git plus the standard system dirs. Restricting PATH is essential: with the
	// full developer PATH, `ddx work`'s harness discovery probes whatever agent
	// CLIs are installed (codex/claude/gemini), and at least one (gemini)
	// daemonizes — a detached child that survives the worker and keeps writing
	// under HOME, racing t.TempDir cleanup. The built-in `script` harness needs
	// only git + sh, so excluding the agent CLIs keeps the run deterministic.
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("locate git: %w", err)
	}
	minimalPATH := strings.Join([]string{filepath.Dir(gitPath), "/usr/bin", "/bin", "/usr/sbin", "/sbin"}, string(os.PathListSeparator))
	home := t.TempDir()
	xdg := t.TempDir()
	subprocessEnv := []string{
		"HOME=" + home,
		"XDG_DATA_HOME=" + xdg,
		"PATH=" + minimalPATH,
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
	}

	// Seed the worker queue with workerBeadCount independent ready beads.
	workerBeads, err := createBeads(bin, proj, subprocessEnv, workerBeadCount, "Worker bead")
	if err != nil {
		return nil, fmt.Errorf("seed worker beads: %w", err)
	}
	if err := gitCommitAll(proj, "test: seed worker beads"); err != nil {
		return nil, err
	}

	// Directive run by the stubbed `script` harness: the sleep-ms mimics the
	// LLM wait and happens OUTSIDE any lock; the create-file + commit are the
	// only mutations, so the worker holds the index/tracker locks only briefly.
	directive := filepath.Join(t.TempDir(), "directive.txt")
	if err := os.WriteFile(directive, []byte(
		"sleep-ms 400\n"+
			"create-file out-${DDX_BEAD_ID}.txt done\n"+
			"commit feat: ${DDX_BEAD_ID} work\n"), 0o644); err != nil {
		return nil, fmt.Errorf("write directive: %w", err)
	}

	// Spawn the 5 watcher processes.
	workers := make([]*exec.Cmd, 0, workerCount)
	for i := 0; i < workerCount; i++ {
		cmd := exec.Command(bin, "work", "--watch",
			"--idle-interval", "500ms",
			"--harness", "script",
			"--model", directive,
			"--attempt-backend", "local-clone",
			"--no-review", "--no-review-i-know-what-im-doing",
			"--project", proj,
		)
		cmd.Dir = proj
		cmd.Env = subprocessEnv
		if err := cmd.Start(); err != nil {
			stopWorkers(workers)
			return nil, fmt.Errorf("start worker %d: %w", i, err)
		}
		workers = append(workers, cmd)
	}
	// Guarantee the watchers are reaped even if the scenario aborts midway.
	defer stopWorkers(workers)

	// Concurrently hammer the tracker with 20 operator bead commands.
	opDone := make(chan []operatorCommand, 1)
	go func() {
		opDone <- runOperatorCommands(bin, proj, subprocessEnv, operatorCount)
	}()

	ops := <-opDone

	// Give the watchers a bounded window to drain the worker queue. The
	// assertions do not require every worker bead to close (cross-process
	// landing races can leave a bead retryable), but draining maximises the
	// overlap window that produced the lock metrics.
	waitForWorkerBeadsToDrain(ddxDir, workerBeads, 60*time.Second)

	// Stop the watchers and let their final metric writes flush.
	stopWorkers(workers)

	events, err := lockmetrics.Load(proj)
	if err != nil {
		return nil, fmt.Errorf("load lock metrics: %w", err)
	}
	index, tracker := holdDurations(events)

	return &contentionResult{
		indexHoldMS:    index,
		trackerHoldMS:  tracker,
		operatorOutput: ops,
		workersSpawned: len(workers),
	}, nil
}

// runOperatorCommands issues `count` operator bead commands (create/update/
// close cycles on operator-owned beads) and returns each command's outcome.
func runOperatorCommands(bin, proj string, env []string, count int) []operatorCommand {
	cmds := make([]operatorCommand, 0, count)
	run := func(args ...string) operatorCommand {
		c := exec.Command(bin, append([]string{"bead"}, args...)...)
		c.Dir = proj
		c.Env = env
		out, err := c.CombinedOutput()
		return operatorCommand{args: strings.Join(args, " "), output: string(out), err: err}
	}
	for len(cmds) < count {
		create := run("create", fmt.Sprintf("Operator bead %d", len(cmds)),
			"--priority", "2", "--description", "operator-issued work", "--acceptance", "ok")
		cmds = append(cmds, create)
		id := lastLine(create.output)
		if create.err != nil || id == "" {
			continue
		}
		if len(cmds) < count {
			cmds = append(cmds, run("update", id, "--notes", "operator touch"))
		}
		if len(cmds) < count {
			cmds = append(cmds, run("close", id))
		}
	}
	return cmds
}

// waitForWorkerBeadsToDrain polls until every worker bead is closed or the
// deadline elapses. It tolerates transient read errors from the concurrently
// mutated store.
func waitForWorkerBeadsToDrain(ddxDir string, ids []string, timeout time.Duration) {
	store := bead.NewStore(ddxDir)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		closed := 0
		for _, id := range ids {
			if b, err := store.Get(context.Background(), id); err == nil && b != nil && b.Status == bead.StatusClosed {
				closed++
			}
		}
		if closed == len(ids) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// holdDurations splits release-event hold durations (milliseconds) by lock name.
func holdDurations(events []lockmetrics.Event) (index, tracker []float64) {
	for _, ev := range events {
		if ev.Event != "release" {
			continue
		}
		switch ev.LockName {
		case "index.lock":
			index = append(index, float64(ev.DurationMS))
		case "tracker.lock":
			tracker = append(tracker, float64(ev.DurationMS))
		}
	}
	return index, tracker
}

// percentile returns the nearest-rank percentile (p in [0,1]) of values.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func maxFloat(values []float64) float64 {
	m := 0.0
	for _, v := range values {
		if v > m {
			m = v
		}
	}
	return m
}

// createBeads creates n beads via `ddx bead create` and returns their IDs.
func createBeads(bin, proj string, env []string, n int, titlePrefix string) ([]string, error) {
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		cmd := exec.Command(bin, "bead", "create", fmt.Sprintf("%s %d", titlePrefix, i+1),
			"--priority", "1", "--description", "do work", "--acceptance", "done")
		cmd.Dir = proj
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("bead create %d: %v\n%s", i, err, out)
		}
		id := lastLine(string(out))
		if id == "" {
			return nil, fmt.Errorf("bead create %d produced no id:\n%s", i, out)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// stopWorkers interrupts each still-running worker and waits for it to exit,
// killing any that do not stop promptly.
func stopWorkers(workers []*exec.Cmd) {
	for _, cmd := range workers {
		if cmd.Process == nil || (cmd.ProcessState != nil && cmd.ProcessState.Exited()) {
			continue
		}
		_ = cmd.Process.Signal(os.Interrupt)
	}
	for _, cmd := range workers {
		if cmd.Process == nil {
			continue
		}
		done := make(chan struct{})
		go func(c *exec.Cmd) {
			_ = c.Wait()
			close(done)
		}(cmd)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
}

// appendGitignore appends paths to the project's .gitignore.
func appendGitignore(proj string, paths ...string) error {
	f, err := os.OpenFile(filepath.Join(proj, ".gitignore"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open .gitignore: %w", err)
	}
	defer f.Close() //nolint:errcheck
	for _, p := range paths {
		if _, err := fmt.Fprintln(f, p); err != nil {
			return fmt.Errorf("write .gitignore: %w", err)
		}
	}
	return nil
}

// gitCommitAll stages everything and commits with signing disabled.
func gitCommitAll(proj, msg string) error {
	for _, args := range [][]string{
		{"add", "-A"},
		{"-c", "commit.gpgsign=false", "commit", "-q", "-m", msg},
	} {
		cmd := exec.Command("git", append([]string{"-C", proj}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			// An empty commit (nothing staged) is not a failure for our setup.
			if strings.Contains(string(out), "nothing to commit") {
				continue
			}
			return fmt.Errorf("git %v: %v\n%s", args, err, out)
		}
	}
	return nil
}

// lastLine returns the last non-empty trimmed line of s (the bead id printed by
// `ddx bead create`).
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if v := strings.TrimSpace(lines[i]); v != "" {
			return v
		}
	}
	return ""
}
