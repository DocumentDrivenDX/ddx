//go:build !windows

package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// startSelfMatchingMonitorShell starts a real sh process whose command line
// contains `pgrep -f <tag>` where <tag> also appears in the shell's command,
// making pgrep match the shell itself. The shell loops until killed.
func startSelfMatchingMonitorShell(t *testing.T, tag string) int {
	t.Helper()
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh not available: %v", err)
	}
	// The tag appears in the shell script itself (as the pgrep pattern),
	// so `pgrep -f <tag>` will always find this shell → eternal loop.
	script := `while kill -0 $(pgrep -f "` + tag + `") 2>/dev/null; do sleep 0.1; done`
	cmd := exec.Command(shPath, "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start self-matching monitor shell: %v", err)
	}
	pid := cmd.Process.Pid
	go func() { _, _ = cmd.Process.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	return pid
}

// waitForMonitorShellInScanner polls until the scanner returns the shell PID.
func waitForMonitorShellInScanner(t *testing.T, rootPID, shellPID int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		procs, err := monitorShellScanner(context.Background(), rootPID, time.Now().UTC())
		if err == nil {
			for _, p := range procs {
				if p.PID == shellPID {
					return
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("scanner did not observe monitor shell %d under pid %d", shellPID, rootPID)
}

// TestWorkDetectsAndCancelsSelfMatchingMonitorShells proves that after a
// self-matching pgrep-based monitor shell is detected, the guard terminates it
// and the sweep returns normally so the attempt can continue unblocked.
func TestWorkDetectsAndCancelsSelfMatchingMonitorShells(t *testing.T) {
	const tag = "ddx-monitor-selfmatch-test-unique"
	shellPID := startSelfMatchingMonitorShell(t, tag)
	waitForMonitorShellInScanner(t, os.Getpid(), shellPID)

	// Build the guard and run one manual tick.
	projectRoot := t.TempDir()
	g := newMonitorShellGuard(projectRoot, "ddx-test-bead", "test-attempt-id", os.Getpid())
	g.tick(context.Background(), time.Now().UTC())

	// The monitor shell must be gone.
	assertProcessGone(t, shellPID)

	// Guard records the cleanup and the attempt can continue (function returned).
	if g.CleanedCount() == 0 {
		t.Fatal("guard cleaned count must be > 0 after terminating self-matching monitor shell")
	}
}

// TestAttemptBackgroundMonitorEvidenceRecordsCommandAndAge proves that after
// a self-matching monitor shell is terminated, the evidence artifact written
// under .ddx/executions/<attempt-id>/ contains PID, command, age_seconds,
// action, and reason fields as required by the bead acceptance criteria.
func TestAttemptBackgroundMonitorEvidenceRecordsCommandAndAge(t *testing.T) {
	const tag = "ddx-monitor-evidence-test-unique"
	shellPID := startSelfMatchingMonitorShell(t, tag)
	waitForMonitorShellInScanner(t, os.Getpid(), shellPID)

	projectRoot := t.TempDir()
	const beadID = "ddx-ev000001"
	const attemptID = "20260614T000000-evtest01"

	g := newMonitorShellGuard(projectRoot, beadID, attemptID, os.Getpid())
	g.tick(context.Background(), time.Now().UTC())

	// Read the evidence artifact.
	artifactPath := filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID, monitorShellCleanupArtifact)
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read monitor-shell-cleanup.json: %v", err)
	}
	var report monitorShellCleanupReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse monitor-shell-cleanup.json: %v", err)
	}

	if report.AttemptID != attemptID {
		t.Fatalf("attempt_id = %q, want %q", report.AttemptID, attemptID)
	}
	if report.BeadID != beadID {
		t.Fatalf("bead_id = %q, want %q", report.BeadID, beadID)
	}
	if len(report.Cleaned) == 0 {
		t.Fatal("cleaned list must be non-empty")
	}
	var found bool
	for _, rec := range report.Cleaned {
		if rec.PID != shellPID {
			continue
		}
		found = true
		if rec.Command == "" {
			t.Error("command must be non-empty")
		}
		if !strings.Contains(rec.Command, "pgrep -f") {
			t.Errorf("command %q must contain pgrep -f", rec.Command)
		}
		if rec.AgeSeconds < 0 {
			t.Errorf("age_seconds must be >= 0, got %v", rec.AgeSeconds)
		}
		if rec.Action != monitorShellActionTerminated {
			t.Errorf("action = %q, want %q", rec.Action, monitorShellActionTerminated)
		}
		if rec.Reason != reasonSelfMatchingMonitor {
			t.Errorf("reason = %q, want %q", rec.Reason, reasonSelfMatchingMonitor)
		}
		if rec.Pattern == "" {
			t.Error("pattern must be non-empty for self-matching monitor")
		}
	}
	if !found {
		t.Fatalf("evidence does not include a record for pid %d; records: %+v", shellPID, report.Cleaned)
	}
}

// TestBackgroundMonitorCleanupDoesNotKillProviderProcess proves that the
// monitor shell cleanup only terminates shell processes with self-matching
// pgrep patterns. Provider processes (non-shell binaries) and unrelated
// same-user processes are not touched.
func TestBackgroundMonitorCleanupDoesNotKillProviderProcess(t *testing.T) {
	// Start a fake provider process (sleep binary symlinked to "codex") that
	// must survive — the scanner only returns shells with pgrep -f.
	dir := t.TempDir()
	providerPID := startFakeProviderChild(t, dir, "codex")
	waitForProviderChildren(t, os.Getpid(), providerPID)

	// Start a self-matching monitor shell that SHOULD be terminated.
	const tag = "ddx-monitor-notkill-test-unique"
	shellPID := startSelfMatchingMonitorShell(t, tag)
	waitForMonitorShellInScanner(t, os.Getpid(), shellPID)

	// Inject a terminate spy to verify only the shell PID is targeted.
	restoreTerminate := terminateMonitorShell
	t.Cleanup(func() { terminateMonitorShell = restoreTerminate })

	var mu sync.Mutex
	var killed []int
	terminateMonitorShell = func(pid int) {
		mu.Lock()
		killed = append(killed, pid)
		mu.Unlock()
		// Still actually terminate so the shell goes away.
		restoreTerminate(pid)
	}

	projectRoot := t.TempDir()
	g := newMonitorShellGuard(projectRoot, "ddx-test-bead", "test-attempt-nk", os.Getpid())
	g.tick(context.Background(), time.Now().UTC())

	// The monitor shell must be gone; the provider (codex) must survive.
	assertProcessGone(t, shellPID)
	if !signalProcessAlive(providerPID) {
		t.Fatalf("provider process pid %d was killed by monitor shell cleanup", providerPID)
	}

	// The terminate spy must only have been called with the shell's PID, never
	// with the provider's PID.
	mu.Lock()
	defer mu.Unlock()
	for _, pid := range killed {
		if pid == providerPID {
			t.Fatalf("terminate was called with provider pid %d — monitor cleanup must not kill provider processes", providerPID)
		}
	}
}

// TestIsSelfMatchingMonitorPatterns is a table-driven unit test for the
// isSelfMatchingMonitor detection logic. Command lines use the form ps shows
// on Linux (argv joined by spaces, no outer shell quoting on the -c argument).
func TestIsSelfMatchingMonitorPatterns(t *testing.T) {
	cases := []struct {
		cmdline string
		wantOK  bool
		wantPat string
	}{
		{
			// ps format: outer single-quote is part of the script text
			cmdline: `sh -c until ! kill -0 $(pgrep -f "lefthook run pre-commit") 2>/dev/null; do sleep 1; done`,
			wantOK:  true,
			wantPat: "lefthook run pre-commit",
		},
		{
			cmdline: `bash -c while pgrep -f "go test ./..." > /dev/null; do sleep 0.5; done`,
			wantOK:  true,
			wantPat: "go test ./...",
		},
		{
			cmdline: `sh -c sleep 120`,
			wantOK:  false,
		},
		{
			cmdline: `sh -c pgrep -f`,
			wantOK:  false, // no pattern argument
		},
		{
			cmdline: `claude --print --model sonnet`,
			wantOK:  false, // not a shell command
		},
		{
			cmdline: `sh -c while pgrep -f myprocess > /dev/null; do sleep 1; done`,
			wantOK:  true,
			wantPat: "myprocess",
		},
	}
	for _, tc := range cases {
		pat, ok := isSelfMatchingMonitor(tc.cmdline)
		if ok != tc.wantOK {
			t.Errorf("isSelfMatchingMonitor(%q) ok=%v want %v", tc.cmdline, ok, tc.wantOK)
			continue
		}
		if tc.wantOK && pat != tc.wantPat {
			t.Errorf("isSelfMatchingMonitor(%q) pattern=%q want %q", tc.cmdline, pat, tc.wantPat)
		}
	}
}

// TestExtractPgrepPattern verifies that pgrep -f patterns are correctly
// extracted from various command forms. Input strings use the ps display format
// (argv joined by spaces, no outer shell quoting on arguments).
func TestExtractPgrepPattern(t *testing.T) {
	cases := []struct {
		cmdline string
		want    string
	}{
		// Double-quoted pattern (double quotes appear in the argv value)
		{`sh -c pgrep -f "lefthook run pre-commit"`, "lefthook run pre-commit"},
		// Single-quoted pattern
		{`sh -c pgrep -f 'go test'`, "go test"},
		// Unquoted pattern followed by shell syntax
		{`sh -c while pgrep -f myservice > /dev/null; do sleep 1; done`, "myservice"},
		// No pattern argument
		{`sh -c pgrep -f`, ""},
		// No pgrep at all
		{`sh -c sleep 10`, ""},
	}
	for _, tc := range cases {
		got := extractPgrepPattern(tc.cmdline)
		if got != tc.want {
			t.Errorf("extractPgrepPattern(%q) = %q, want %q", tc.cmdline, got, tc.want)
		}
	}
}
