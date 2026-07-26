package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
)

// emptyFizeauServiceConfig is a public ServiceConfig implementation with no
// providers. Pinned wrapped-harness Execute still resolves via harness + model
// pins without consulting provider entries (Fizeau public harness-dispatch
// contract).
type emptyFizeauServiceConfig struct {
	workDir       string
	sessionLogDir string
}

func (c emptyFizeauServiceConfig) ProviderNames() []string { return nil }
func (c emptyFizeauServiceConfig) DefaultProviderName() string {
	return ""
}
func (c emptyFizeauServiceConfig) Provider(string) (agentlib.ServiceProviderEntry, bool) {
	return agentlib.ServiceProviderEntry{}, false
}
func (c emptyFizeauServiceConfig) HealthCooldown() time.Duration { return 0 }
func (c emptyFizeauServiceConfig) WorkDir() string               { return c.workDir }
func (c emptyFizeauServiceConfig) SessionLogDir() string         { return c.sessionLogDir }

// TestFizeauV015ExecuteCancellationWaitsForWrappedTree proves the released
// Fizeau v0.15 public Execute contract: cancelling the Execute context does not
// complete until the wrapped harness process and its spawned child/grandchild
// have exited. Uses only exported Fizeau package APIs.
func TestFizeauV015ExecuteCancellationWaitsForWrappedTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrapped process-tree fixture requires POSIX shell and process-group semantics")
	}

	// Isolate Fizeau from the operator HOME/XDG config so only the in-test
	// ServiceConfig and PATH stub participate.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(fakeHome, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(fakeHome, ".local", "share"))

	workDir := t.TempDir()
	sessionLogDir := filepath.Join(workDir, "sessions")
	if err := os.MkdirAll(sessionLogDir, 0o700); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	// Stub codex on PATH: write direct-child PID, spawn TERM-immune grandchild,
	// wait. Fizeau must still reap the full tree on Execute cancellation.
	const (
		targetPIDFile     = "lifecycle-target.pid"
		grandchildPIDFile = "lifecycle-grandchild.pid"
	)
	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	script := `#!/bin/sh
trap '' TERM
printf '%s\n' "$$" > lifecycle-target.pid
sh -c 'trap "" TERM; exec sleep 300' &
child=$!
printf '%s\n' "$child" > lifecycle-grandchild.pid
wait "$child"
`
	if err := os.WriteFile(codexPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write codex stub: %v", err)
	}
	// Keep system PATH so sh/sleep remain available inside the fixture.
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	refreshCtx, refreshCancel := context.WithCancel(context.Background())
	refreshCancel()
	svc, err := agentlib.New(agentlib.ServiceOptions{
		ServiceConfig: emptyFizeauServiceConfig{
			workDir:       workDir,
			sessionLogDir: sessionLogDir,
		},
		SessionLogDir:           sessionLogDir,
		QuotaRefreshContext:     refreshCtx,
		HarnessCleanupTimeout:   15 * time.Second,
		StaleHarnessReaperGrace: time.Hour, // avoid reaping live records mid-test
	})
	if err != nil {
		t.Fatalf("agentlib.New: %v", err)
	}

	execCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := svc.Execute(execCtx, agentlib.ServiceExecuteRequest{
		Prompt:        "cancellation-process-tree-conformance",
		Harness:       "codex",
		Model:         "gpt-5.4",
		WorkDir:       workDir,
		SessionLogDir: sessionLogDir,
		Permissions:   "safe",
		Reasoning:     agentlib.ReasoningLow,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	targetPID := waitForCompletePIDFile(t, filepath.Join(workDir, targetPIDFile), 10*time.Second)
	grandchildPID := waitForCompletePIDFile(t, filepath.Join(workDir, grandchildPIDFile), 10*time.Second)
	if targetPID == grandchildPID {
		t.Fatalf("target and grandchild PIDs must differ; both %d", targetPID)
	}
	assertWrappedTreeProcessAlive(t, targetPID, "target")
	assertWrappedTreeProcessAlive(t, grandchildPID, "grandchild")

	// Cancel Execute. The public contract requires the event stream to stay open
	// until the wrapped tree (direct child + grandchild) has exited.
	cancel()

	// Drain with an independent context: the cancelled Execute context must not
	// short-circuit observation of stream close after cleanup.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer drainCancel()
	var sawFinal bool
	for {
		select {
		case <-drainCtx.Done():
			t.Fatalf("timed out waiting for Execute event stream to close after cancel: %v", drainCtx.Err())
		case ev, ok := <-events:
			if !ok {
				// Stream closed == Execute finished. Both PIDs must already be gone.
				// Check immediately — do not wait after return (that would weaken
				// the "returns only after tree exit" proof).
				assertWrappedTreeProcessGoneNow(t, targetPID, "target")
				assertWrappedTreeProcessGoneNow(t, grandchildPID, "grandchild")
				if !sawFinal {
					t.Fatal("Execute closed without a final event after cancellation")
				}
				return
			}
			decoded, decodeErr := agentlib.DecodeServiceEvent(ev)
			if decodeErr != nil {
				t.Fatalf("DecodeServiceEvent: %v", decodeErr)
			}
			if decoded.Final != nil {
				sawFinal = true
				// Cancellation should surface as cancelled / context_cancelled when
				// cleanup succeeds. Accept cleanup_failed primary-cancel as long as
				// process-tree containment (the AC) still holds at stream close.
				if decoded.Final.Outcome != agentlib.SessionOutcomeCancelled &&
					decoded.Final.PrimaryOutcome != agentlib.SessionOutcomeCancelled {
					t.Fatalf("final outcome = %q primary=%q, want cancelled (status=%q cause=%q)",
						decoded.Final.Outcome, decoded.Final.PrimaryOutcome,
						decoded.Final.Status, decoded.Final.Cause)
				}
			}
		}
	}
}

func waitForCompletePIDFile(t *testing.T, path string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastData []byte
	var lastErr error
	for {
		data, err := os.ReadFile(path)
		lastData, lastErr = data, err
		// Require trailing newline so we do not parse a partial write.
		if err == nil && bytes.HasSuffix(data, []byte{'\n'}) {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr == nil && pid > 0 {
				return pid
			}
			lastErr = parseErr
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for complete pid file %s (data %q): %v", path, lastData, lastErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assertWrappedTreeProcessAlive(t *testing.T, pid int, label string) {
	t.Helper()
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("%s pid %d is not alive before cancel: %v", label, pid, err)
	}
}

// assertWrappedTreeProcessGoneNow fails if pid is still killable. Unlike the
// package-level assertProcessGone helper (which waits), this is an immediate
// check used at Execute stream close.
func assertWrappedTreeProcessGoneNow(t *testing.T, pid int, label string) {
	t.Helper()
	err := syscall.Kill(pid, 0)
	if err == nil {
		t.Fatalf("%s pid %d still alive after Execute returned", label, pid)
	}
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
		return
	}
	// Some Go versions surface process-not-found via wrapped syscall errors.
	if strings.Contains(err.Error(), "no such process") {
		return
	}
	t.Fatalf("%s pid %d unexpected kill(0) after Execute returned: %v", label, pid, err)
}
