//go:build !windows

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

func startFakeProviderChild(t *testing.T, dir, provider string) int {
	t.Helper()
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	bin := filepath.Join(dir, provider)
	if err := os.Symlink(sleepPath, bin); err != nil {
		t.Fatalf("symlink fake %s: %v", provider, err)
	}
	cmd := exec.Command(bin, "120")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake %s: %v", provider, err)
	}
	pid := cmd.Process.Pid
	go func() { _, _ = cmd.Process.Wait() }()
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	})
	return pid
}

func TestAttemptProviderChildrenAreRouteScoped(t *testing.T) {
	dir := t.TempDir()
	claudePID := startFakeProviderChild(t, dir, "claude")
	codexPID := startFakeProviderChild(t, dir, "codex")
	geminiPID := startFakeProviderChild(t, dir, "gemini")
	waitForProviderChildren(t, os.Getpid(), claudePID, codexPID, geminiPID)

	reaped, survivors := reapSupersededProviderChildren(context.Background(), os.Getpid(), "", "claude/sonnet", "", time.Now().UTC())

	assertProcessGone(t, codexPID)
	assertProcessGone(t, geminiPID)
	if !signalProcessAlive(claudePID) {
		t.Fatalf("active-route claude child %d was reaped", claudePID)
	}
	reapedProviders := map[string]bool{}
	for _, r := range reaped {
		reapedProviders[r.Provider] = true
	}
	if !reapedProviders["codex"] || !reapedProviders["gemini"] || reapedProviders["claude"] {
		t.Fatalf("unexpected reaped providers: %+v", reaped)
	}
	var sawClaude bool
	for _, s := range survivors {
		if s.Provider == "claude" {
			sawClaude = true
		}
		if s.Provider == "codex" || s.Provider == "gemini" {
			t.Fatalf("survivor list contains superseded provider: %+v", survivors)
		}
	}
	if !sawClaude {
		t.Fatalf("survivors missing claude child: %+v", survivors)
	}
}

func TestSupersededProviderChildrenAreReapedWithEvidence(t *testing.T) {
	now := time.Now().UTC()
	const supersededPID = 424242
	restoreScanner := providerChildScanner
	restoreTerminate := terminateProviderChild
	t.Cleanup(func() {
		providerChildScanner = restoreScanner
		terminateProviderChild = restoreTerminate
	})

	providerChildScanner = func(context.Context, int, time.Time) ([]providerChildProcess, error) {
		return []providerChildProcess{{
			PID:       supersededPID,
			Provider:  "codex",
			Command:   "/usr/local/bin/codex --no-alt-screen",
			StartedAt: now.Add(-90 * time.Second),
		}}, nil
	}
	var mu sync.Mutex
	var killed []int
	terminateProviderChild = func(pid int) {
		mu.Lock()
		killed = append(killed, pid)
		mu.Unlock()
	}

	reaped, survivors := reapSupersededProviderChildren(context.Background(), os.Getpid(), "", "claude/sonnet", "", now)

	if len(survivors) != 0 {
		t.Fatalf("expected no survivors; got %+v", survivors)
	}
	if len(reaped) != 1 {
		t.Fatalf("expected one reaped record; got %+v", reaped)
	}
	rec := reaped[0]
	if rec.PID != supersededPID || rec.Provider != "codex" || rec.Action != providerChildActionTerminated || rec.Reason != reasonSupersededProviderChild {
		t.Fatalf("bad reap evidence: %+v", rec)
	}
	if rec.AgeSeconds < 89 || rec.AgeSeconds > 91 {
		t.Fatalf("age_seconds = %v, want ~90", rec.AgeSeconds)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(killed) != 1 || killed[0] != supersededPID {
		t.Fatalf("expected terminate(%d), got %v", supersededPID, killed)
	}
}

func TestSupersededProviderChildrenScopeIgnoresSiblingWorkerProviders(t *testing.T) {
	now := time.Now().UTC()
	scopeDir := filepath.Join(t.TempDir(), "attempt")
	insideDir := filepath.Join(scopeDir, "cli")
	outsideDir := filepath.Join(t.TempDir(), "other-attempt")
	if err := os.MkdirAll(insideDir, 0o755); err != nil {
		t.Fatalf("mkdir inside dir: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside dir: %v", err)
	}

	const insidePID = 424243
	const siblingPID = 424244
	restoreScanner := providerChildScanner
	restoreTerminate := terminateProviderChild
	t.Cleanup(func() {
		providerChildScanner = restoreScanner
		terminateProviderChild = restoreTerminate
	})

	providerChildScanner = func(context.Context, int, time.Time) ([]providerChildProcess, error) {
		return []providerChildProcess{
			{
				PID:       insidePID,
				Provider:  "codex",
				Command:   "/usr/local/bin/codex --no-alt-screen",
				CWD:       insideDir,
				StartedAt: now.Add(-90 * time.Second),
			},
			{
				PID:       siblingPID,
				Provider:  "gemini",
				Command:   "/usr/local/bin/gemini --yolo",
				CWD:       outsideDir,
				StartedAt: now.Add(-90 * time.Second),
			},
		}, nil
	}
	var killed []int
	terminateProviderChild = func(pid int) {
		killed = append(killed, pid)
	}

	reaped, survivors := reapSupersededProviderChildren(context.Background(), os.Getpid(), scopeDir, "claude/sonnet", "", now)

	if len(survivors) != 0 {
		t.Fatalf("expected no survivors; got %+v", survivors)
	}
	if len(reaped) != 1 || reaped[0].PID != insidePID {
		t.Fatalf("expected only in-scope provider to be reaped; got %+v", reaped)
	}
	if len(killed) != 1 || killed[0] != insidePID {
		t.Fatalf("expected only terminate(%d), got %v", insidePID, killed)
	}
}

func TestSupersededProviderChildrenWithoutRouteIdentityDoesNotReap(t *testing.T) {
	now := time.Now().UTC()
	const activePID = 525252
	restoreScanner := providerChildScanner
	restoreTerminate := terminateProviderChild
	t.Cleanup(func() {
		providerChildScanner = restoreScanner
		terminateProviderChild = restoreTerminate
	})

	providerChildScanner = func(context.Context, int, time.Time) ([]providerChildProcess, error) {
		return []providerChildProcess{{
			PID:       activePID,
			Provider:  "claude",
			Command:   "/usr/local/bin/claude --print --model sonnet",
			StartedAt: now,
		}}, nil
	}
	var killed []int
	terminateProviderChild = func(pid int) {
		killed = append(killed, pid)
	}

	reaped, survivors := reapSupersededProviderChildren(context.Background(), os.Getpid(), "", "", "", now)

	if len(reaped) != 0 {
		t.Fatalf("expected no reaped children without route identity; got %+v", reaped)
	}
	if len(survivors) != 0 {
		t.Fatalf("expected no survivor report when cleanup is skipped; got %+v", survivors)
	}
	if len(killed) != 0 {
		t.Fatalf("active provider child was terminated without route identity: %v", killed)
	}
}

func TestSupersededProviderChildrenUsesHarnessAsOwnerWhenProviderBlank(t *testing.T) {
	now := time.Now().UTC()
	const activePID = 626262
	restoreScanner := providerChildScanner
	restoreTerminate := terminateProviderChild
	t.Cleanup(func() {
		providerChildScanner = restoreScanner
		terminateProviderChild = restoreTerminate
	})

	providerChildScanner = func(context.Context, int, time.Time) ([]providerChildProcess, error) {
		return []providerChildProcess{{
			PID:       activePID,
			Provider:  "claude",
			Command:   "/usr/local/bin/claude --print --model sonnet",
			StartedAt: now,
		}}, nil
	}
	var killed []int
	terminateProviderChild = func(pid int) {
		killed = append(killed, pid)
	}

	reaped, survivors := reapSupersededProviderChildren(context.Background(), os.Getpid(), "", "", "claude", now)

	if len(reaped) != 0 {
		t.Fatalf("expected active harness provider to survive; got reaped %+v", reaped)
	}
	if len(survivors) != 1 || survivors[0].Provider != "claude" {
		t.Fatalf("expected claude survivor; got %+v", survivors)
	}
	if len(killed) != 0 {
		t.Fatalf("active provider child was terminated despite harness ownership: %v", killed)
	}
}

func TestSupersededProviderChildrenTreatsClaudeTUIRouteAsClaudeOwner(t *testing.T) {
	now := time.Now().UTC()
	const activePID = 727272
	restoreScanner := providerChildScanner
	restoreTerminate := terminateProviderChild
	t.Cleanup(func() {
		providerChildScanner = restoreScanner
		terminateProviderChild = restoreTerminate
	})

	providerChildScanner = func(context.Context, int, time.Time) ([]providerChildProcess, error) {
		return []providerChildProcess{{
			PID:       activePID,
			Provider:  "claude",
			Command:   "/home/erik/.local/bin/claude --permission-mode bypassPermissions --model sonnet",
			StartedAt: now,
		}}, nil
	}
	var killed []int
	terminateProviderChild = func(pid int) {
		killed = append(killed, pid)
	}

	reaped, survivors := reapSupersededProviderChildren(context.Background(), os.Getpid(), "", "claude-tui/sonnet-4.6", "claude-tui", now)

	if len(reaped) != 0 {
		t.Fatalf("expected active claude-tui child to survive; got reaped %+v", reaped)
	}
	if len(survivors) != 1 || survivors[0].Provider != "claude" {
		t.Fatalf("expected claude survivor for claude-tui route; got %+v", survivors)
	}
	if len(killed) != 0 {
		t.Fatalf("active claude-tui provider child was terminated: %v", killed)
	}
}

func TestDefunctRouteOwnedProviderChildIsReaped(t *testing.T) {
	now := time.Now().UTC()
	const defunctPID = 737373
	restoreScanner := providerChildScanner
	restoreTerminate := terminateProviderChild
	t.Cleanup(func() {
		providerChildScanner = restoreScanner
		terminateProviderChild = restoreTerminate
	})

	providerChildScanner = func(context.Context, int, time.Time) ([]providerChildProcess, error) {
		return []providerChildProcess{{
			PID:       defunctPID,
			Provider:  "claude",
			Command:   "[claude] <defunct>",
			StartedAt: now.Add(-2 * time.Minute),
			Defunct:   true,
		}}, nil
	}
	var killed []int
	terminateProviderChild = func(pid int) {
		killed = append(killed, pid)
	}

	reaped, survivors := reapSupersededProviderChildren(context.Background(), os.Getpid(), "", "claude-tui/sonnet-4.6", "claude-tui", now)

	if len(survivors) != 0 {
		t.Fatalf("defunct child must not survive even when route-owned; got %+v", survivors)
	}
	if len(reaped) != 1 || reaped[0].PID != defunctPID || reaped[0].Reason != reasonDefunctProviderChild {
		t.Fatalf("expected defunct child reap evidence; got %+v", reaped)
	}
	if len(killed) != 1 || killed[0] != defunctPID {
		t.Fatalf("expected terminate(%d), got %v", defunctPID, killed)
	}
}

func TestRunningProviderGuardReapsNonRouteProviderChildren(t *testing.T) {
	dir := t.TempDir()
	claudePID := startFakeProviderChild(t, dir, "claude")
	codexPID := startFakeProviderChild(t, dir, "codex")
	geminiPID := startFakeProviderChild(t, dir, "gemini")
	waitForProviderChildren(t, os.Getpid(), claudePID, codexPID, geminiPID)

	children, reaped := runningProviderChildGuard(context.Background(), os.Getpid(), "", "claude/sonnet", "", "running", time.Now().UTC())

	assertProcessGone(t, codexPID)
	assertProcessGone(t, geminiPID)
	if !signalProcessAlive(claudePID) {
		t.Fatalf("active-route claude child %d was reaped by running guard", claudePID)
	}

	reapedProviders := map[string]bool{}
	for _, r := range reaped {
		reapedProviders[r.Provider] = true
		if r.Reason != reasonRunningPhaseGuard {
			t.Fatalf("unexpected reap reason for %s: %q", r.Provider, r.Reason)
		}
	}
	if !reapedProviders["codex"] || !reapedProviders["gemini"] || reapedProviders["claude"] {
		t.Fatalf("running guard must reap codex+gemini but not claude; got %+v", reaped)
	}

	byProvider := map[string]workerstatus.ProviderChild{}
	for _, c := range children {
		byProvider[c.Provider] = c
	}
	if claude, ok := byProvider["claude"]; !ok || claude.RouteOwner == "" || claude.NonRoute {
		t.Fatalf("claude child must report as route-owned, not non-route: %+v", byProvider["claude"])
	}
	for _, name := range []string{"codex", "gemini"} {
		child, ok := byProvider[name]
		if !ok {
			t.Fatalf("status view missing non-route child %s: %+v", name, children)
		}
		if !child.NonRoute || child.RouteOwner != "" || child.Diagnostic == "" {
			t.Fatalf("%s must report as non-route with a diagnostic: %+v", name, child)
		}
	}
}

func TestRunningProviderGuardDoesNotReapNestedProviderChildren(t *testing.T) {
	now := time.Now().UTC()
	scopeDir := filepath.Join(t.TempDir(), "attempt")
	if err := os.MkdirAll(scopeDir, 0o755); err != nil {
		t.Fatalf("mkdir scope dir: %v", err)
	}
	const (
		rootPID        = 111111
		claudePID      = 111112
		nestedCodexPID = 111113
	)
	restoreScanner := providerChildScanner
	restoreTerminate := terminateProviderChild
	t.Cleanup(func() {
		providerChildScanner = restoreScanner
		terminateProviderChild = restoreTerminate
	})

	providerChildScanner = func(context.Context, int, time.Time) ([]providerChildProcess, error) {
		return []providerChildProcess{
			{
				PID:       claudePID,
				PPID:      rootPID,
				Provider:  "claude",
				Command:   "/home/erik/.local/bin/claude --model opus",
				CWD:       scopeDir,
				StartedAt: now.Add(-30 * time.Second),
			},
			{
				PID:       nestedCodexPID,
				PPID:      claudePID,
				Provider:  "codex",
				Command:   "/home/linuxbrew/.linuxbrew/bin/codex exec --json",
				CWD:       scopeDir,
				StartedAt: now.Add(-5 * time.Second),
			},
		}, nil
	}
	var killed []int
	terminateProviderChild = func(pid int) {
		killed = append(killed, pid)
	}

	children, reaped := runningProviderChildGuard(context.Background(), rootPID, scopeDir, "claude/opus", "", "running", now)

	if len(reaped) != 0 {
		t.Fatalf("nested provider child must not be reaped; got %+v", reaped)
	}
	if len(killed) != 0 {
		t.Fatalf("nested provider child was terminated: %v", killed)
	}
	byPID := map[int]workerstatus.ProviderChild{}
	for _, child := range children {
		byPID[child.PID] = child
	}
	nested, ok := byPID[nestedCodexPID]
	if !ok {
		t.Fatalf("status view missing nested provider child: %+v", children)
	}
	if !nested.NonRoute || !strings.Contains(nested.Diagnostic, "not terminated") {
		t.Fatalf("nested child should be reported as observed but not terminated: %+v", nested)
	}
}

func TestRunningProviderGuardReapsProviderGrandchildrenByProcessGroup(t *testing.T) {
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh not available: %v", err)
	}
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	dir := t.TempDir()
	nodeBin := filepath.Join(dir, "node")
	if err := os.Symlink(sleepPath, nodeBin); err != nil {
		t.Fatalf("symlink fake node: %v", err)
	}
	geminiBin := filepath.Join(dir, "gemini")
	if err := os.Symlink(shPath, geminiBin); err != nil {
		t.Fatalf("symlink fake gemini: %v", err)
	}
	pidFile := filepath.Join(dir, "node.pid")

	// argv[0] is the gemini symlink path so the scanner classifies the parent as
	// a "gemini" provider; the backgrounded fake node inherits gemini's process
	// group (non-interactive sh keeps no separate group for background jobs), so
	// it is a grandchild reachable only by killing the group.
	cmd := exec.Command(geminiBin, "-c", `"$1" 120 & echo $! > "$2"; wait`, "gemini", nodeBin, pidFile)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake gemini: %v", err)
	}
	geminiPID := cmd.Process.Pid
	go func() { _, _ = cmd.Process.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-geminiPID, syscall.SIGKILL) })

	waitForProviderChildren(t, os.Getpid(), geminiPID)
	nodePID := waitForPIDFile(t, pidFile)

	_, reaped := runningProviderChildGuard(context.Background(), os.Getpid(), "", "claude/sonnet", "", "running", time.Now().UTC())

	assertProcessGone(t, geminiPID)
	assertProcessGone(t, nodePID)

	var sawGemini bool
	for _, r := range reaped {
		if r.Provider == "gemini" {
			sawGemini = true
		}
	}
	if !sawGemini {
		t.Fatalf("running guard must reap the gemini parent; got %+v", reaped)
	}
}

func TestAttemptEndCleanupStillReapsAllProviderChildren(t *testing.T) {
	dir := t.TempDir()
	claudePID := startFakeProviderChild(t, dir, "claude")
	codexPID := startFakeProviderChild(t, dir, "codex")
	waitForProviderChildren(t, os.Getpid(), claudePID, codexPID)

	reaped := reapAllProviderChildren(context.Background(), os.Getpid(), "", time.Now().UTC())

	// The attempt-end backstop reaps every provider child regardless of route,
	// including the active-route claude child the running guard would preserve.
	assertProcessGone(t, claudePID)
	assertProcessGone(t, codexPID)

	reapedProviders := map[string]bool{}
	for _, r := range reaped {
		reapedProviders[r.Provider] = true
		if r.Reason != reasonAttemptEnded {
			t.Fatalf("attempt-end reap reason for %s = %q, want %q", r.Provider, r.Reason, reasonAttemptEnded)
		}
	}
	if !reapedProviders["claude"] || !reapedProviders["codex"] {
		t.Fatalf("attempt-end backstop must reap all provider children; got %+v", reaped)
	}
}

func TestAttemptEndCleanupScopeIgnoresSiblingAndUnknownCWDProviders(t *testing.T) {
	now := time.Now().UTC()
	scopeDir := filepath.Join(t.TempDir(), "attempt")
	insideDir := filepath.Join(scopeDir, "cli")
	outsideDir := filepath.Join(t.TempDir(), "other-attempt")
	if err := os.MkdirAll(insideDir, 0o755); err != nil {
		t.Fatalf("mkdir inside dir: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside dir: %v", err)
	}

	const insidePID = 757575
	const siblingPID = 757576
	const unknownCWDPID = 757577
	restoreScanner := providerChildScanner
	restoreTerminate := terminateProviderChild
	t.Cleanup(func() {
		providerChildScanner = restoreScanner
		terminateProviderChild = restoreTerminate
	})

	providerChildScanner = func(context.Context, int, time.Time) ([]providerChildProcess, error) {
		return []providerChildProcess{
			{
				PID:       insidePID,
				Provider:  "claude",
				Command:   "/usr/local/bin/claude --model sonnet",
				CWD:       insideDir,
				StartedAt: now.Add(-30 * time.Second),
			},
			{
				PID:       siblingPID,
				Provider:  "claude",
				Command:   "[claude] <defunct>",
				CWD:       outsideDir,
				StartedAt: now.Add(-30 * time.Second),
				Defunct:   true,
			},
			{
				PID:       unknownCWDPID,
				Provider:  "codex",
				Command:   "/usr/local/bin/codex exec",
				StartedAt: now.Add(-30 * time.Second),
			},
		}, nil
	}
	var killed []int
	terminateProviderChild = func(pid int) {
		killed = append(killed, pid)
	}

	reaped := reapAllProviderChildren(context.Background(), os.Getpid(), scopeDir, now)

	if len(reaped) != 1 || reaped[0].PID != insidePID || reaped[0].Reason != reasonAttemptEnded {
		t.Fatalf("expected only in-scope provider to be reaped at attempt end; got %+v", reaped)
	}
	if len(killed) != 1 || killed[0] != insidePID {
		t.Fatalf("expected only terminate(%d), got %v", insidePID, killed)
	}
}

func TestTerminateProviderChildReapsDirectChild(t *testing.T) {
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	cmd := exec.Command(sleepPath, "120")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
	})

	terminateProviderChildImpl(pid)

	assertProcessGone(t, pid)
	var status syscall.WaitStatus
	waited, waitErr := syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
	if waitErr != syscall.ECHILD {
		t.Fatalf("expected child pid %d to be reaped (ECHILD), got waited=%d err=%v", pid, waited, waitErr)
	}
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("pid file %s was not written", path)
	return 0
}

func TestAttemptEndReapsAllProviderChildren(t *testing.T) {
	for _, mode := range []string{"success", "failure", "interrupt"} {
		t.Run(mode, func(t *testing.T) {
			beadID := "ddx-provider-end-" + mode
			projectRoot, gitOps := setupProcessCleanupAttempt(t, beadID)
			binDir := t.TempDir()
			runner := newProviderChildRunner(t, binDir, "codex", mode)

			ctx := context.Background()
			var cancel context.CancelFunc
			if mode == "interrupt" {
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
			}

			done := make(chan struct{})
			go func() {
				_, _ = ExecuteBeadWithConfig(ctx, projectRoot, beadID, processCleanupConfig(), ExecuteBeadRuntime{
					AgentRunner: runner,
				}, gitOps)
				close(done)
			}()

			pid := <-runner.started
			if mode == "interrupt" {
				cancel()
			}
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("ExecuteBeadWithConfig did not return")
			}
			assertProcessGone(t, pid)
		})
	}
}

type providerChildRunner struct {
	bin     string
	started chan int
	mode    string
}

func newProviderChildRunner(t *testing.T, dir, provider, mode string) *providerChildRunner {
	t.Helper()
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	bin := filepath.Join(dir, provider)
	if err := os.Symlink(sleepPath, bin); err != nil {
		t.Fatalf("symlink fake %s: %v", provider, err)
	}
	return &providerChildRunner{bin: bin, started: make(chan int, 1), mode: mode}
}

func (r *providerChildRunner) Run(opts RunArgs) (*Result, error) {
	cmd := exec.Command(r.bin, "120")
	cmd.Dir = opts.WorkDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return &Result{ExitCode: 1, Error: err.Error()}, nil
	}
	pid := cmd.Process.Pid
	go func() { _, _ = cmd.Process.Wait() }()
	r.started <- pid
	switch r.mode {
	case "interrupt":
		ctx := opts.Context
		if ctx == nil {
			ctx = context.Background()
		}
		<-ctx.Done()
		return &Result{ExitCode: 1, Error: ctx.Err().Error()}, nil
	case "failure":
		return &Result{ExitCode: 1, Error: "synthetic failure"}, nil
	default:
		return &Result{ExitCode: 0}, nil
	}
}

func waitForProviderChildren(t *testing.T, rootPID int, pids ...int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		procs, err := providerChildScanner(context.Background(), rootPID, time.Now().UTC())
		if err == nil {
			seen := map[int]bool{}
			for _, p := range procs {
				seen[p.PID] = true
			}
			all := true
			for _, pid := range pids {
				if !seen[pid] {
					all = false
					break
				}
			}
			if all {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("scanner did not observe all provider children %v under pid %d", pids, rootPID)
}

// TestProviderForCommandDetectsNodeWrappedGemini proves that
// providerForCommand correctly classifies "node /path/to/gemini ..." as
// provider "gemini" while ordinary non-provider Node invocations return empty.
func TestProviderForCommandDetectsNodeWrappedGemini(t *testing.T) {
	cases := []struct {
		cmdline  string
		expected string
	}{
		{"node /home/linuxbrew/.linuxbrew/bin/gemini --interactive", "gemini"},
		{"node /usr/local/bin/gemini", "gemini"},
		{"/usr/bin/node /opt/homebrew/bin/gemini --no-sandbox", "gemini"},
		{"/usr/bin/node /usr/local/bin/codex", "codex"},
		{"node /some/other/script.js", ""},
		{"node script.js", ""},
		{"node", ""},
		{"node -e 'console.log(1)'", ""},
		{"python /usr/local/bin/gemini", ""},
		{"gemini --interactive", "gemini"},
		{"[claude] <defunct>", "claude"},
	}
	for _, tc := range cases {
		got := providerForCommand(tc.cmdline)
		if got != tc.expected {
			t.Errorf("providerForCommand(%q) = %q, want %q", tc.cmdline, got, tc.expected)
		}
	}
}

// TestRunningProviderGuardReapsNodeWrappedGeminiProcessGroup starts a fake
// Node wrapper process whose argv includes a gemini path and proves the running
// guard reports and reaps that Gemini process group while preserving the
// active-route Claude child.
func TestRunningProviderGuardReapsNodeWrappedGeminiProcessGroup(t *testing.T) {
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh not available: %v", err)
	}
	dir := t.TempDir()

	// geminiScript is the script that sh will interpret; sh's own process
	// retains the original argv (nodeBin, geminiScript) visible in ps.
	geminiScript := filepath.Join(dir, "gemini")
	if err := os.WriteFile(geminiScript, []byte("#!/bin/sh\nsleep 120\n"), 0755); err != nil {
		t.Fatalf("write gemini script: %v", err)
	}

	// nodeBin is the fake "node" path used as argv[0]; the actual binary run
	// is sh so that the process blocks while ps shows "node /tmp/.../gemini".
	nodeBin := filepath.Join(dir, "node")

	// Run sh with custom argv: [nodeBin, geminiScript].  ps reports the command
	// as "/tmp/.../node /tmp/.../gemini" — the real-world Node-wrapper shape.
	cmd := &exec.Cmd{
		Path:        shPath,
		Args:        []string{nodeBin, geminiScript},
		SysProcAttr: &syscall.SysProcAttr{Setpgid: true},
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake node-wrapped gemini: %v", err)
	}
	nodeGeminiPID := cmd.Process.Pid
	go func() { _, _ = cmd.Process.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-nodeGeminiPID, syscall.SIGKILL) })

	claudePID := startFakeProviderChild(t, dir, "claude")
	waitForProviderChildren(t, os.Getpid(), claudePID, nodeGeminiPID)

	children, reaped := runningProviderChildGuard(context.Background(), os.Getpid(), "", "claude/sonnet", "", "running", time.Now().UTC())

	assertProcessGone(t, nodeGeminiPID)
	if !signalProcessAlive(claudePID) {
		t.Fatalf("active-route claude child %d was reaped by running guard", claudePID)
	}

	var sawGeminiReaped bool
	for _, r := range reaped {
		if r.Provider == "gemini" {
			sawGeminiReaped = true
			if r.Reason != reasonRunningPhaseGuard {
				t.Fatalf("node-wrapped gemini reap reason = %q, want %q", r.Reason, reasonRunningPhaseGuard)
			}
		}
	}
	if !sawGeminiReaped {
		t.Fatalf("running guard must reap node-wrapped gemini; reaped=%+v", reaped)
	}

	byProvider := map[string]workerstatus.ProviderChild{}
	for _, c := range children {
		byProvider[c.Provider] = c
	}
	if claude, ok := byProvider["claude"]; !ok || claude.RouteOwner == "" || claude.NonRoute {
		t.Fatalf("claude must be route-owned: %+v", byProvider["claude"])
	}
	gemini, ok := byProvider["gemini"]
	if !ok {
		t.Fatalf("gemini must appear in status view: %+v", children)
	}
	if !gemini.NonRoute || gemini.RouteOwner != "" || gemini.Diagnostic == "" {
		t.Fatalf("node-wrapped gemini must be non-route with diagnostic: %+v", gemini)
	}
}
