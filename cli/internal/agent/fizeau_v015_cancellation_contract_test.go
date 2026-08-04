package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/require"
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

const (
	fizeauCallerDeathHelperEnv     = "DDX_FIZEAU_CALLER_DEATH_HELPER"
	fizeauCallerDeathWorkDirEnv    = "DDX_FIZEAU_CALLER_DEATH_WORKDIR"
	fizeauCallerDeathSessionLogEnv = "DDX_FIZEAU_CALLER_DEATH_SESSION_LOG_DIR"
	fizeauCallerDeathTargetPIDEnv  = "DDX_FIZEAU_CALLER_DEATH_TARGET_PID_FILE"
	fizeauCallerDeathGrandPIDEnv   = "DDX_FIZEAU_CALLER_DEATH_GRANDCHILD_PID_FILE"
	fizeauCallerDeathStubDirEnv    = "DDX_FIZEAU_CALLER_DEATH_STUB_DIR"
)

// TestFizeauExecuteCancellationTerminatesWrappedProcessTree proves the public
// Execute contract: cancelling the Execute context does not complete until the
// wrapped harness process and its spawned child/grandchild have exited. Uses
// only exported Fizeau package APIs.
func TestFizeauExecuteCancellationTerminatesWrappedProcessTree(t *testing.T) {
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

// TestFizeauCallerDeathTerminatesWrappedProcessTree proves the public Execute
// contract tears down the wrapped harness process tree when the embedding DDx
// worker dies abnormally.
func TestFizeauCallerDeathTerminatesWrappedProcessTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrapped process-tree fixture requires POSIX shell and process-group semantics")
	}
	if testing.Short() {
		t.Skip("wrapped process-tree fixture is integration-heavy")
	}

	tmp := t.TempDir()
	workDir := filepath.Join(tmp, "work")
	sessionLogDir := filepath.Join(workDir, "sessions")
	if err := os.MkdirAll(sessionLogDir, 0o700); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

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

	fakeHome := t.TempDir()
	fakeXDGConfig := filepath.Join(fakeHome, ".config")
	fakeXDGData := filepath.Join(fakeHome, ".local", "share")

	helperCmd := exec.Command(os.Args[0], "-test.run=^TestFizeauCallerDeathTerminatesWrappedProcessTreeHelper$")
	helperCmd.Dir = workDir
	helperEnv := append([]string{}, os.Environ()...)
	helperEnv = replaceEnv(helperEnv, "HOME", fakeHome)
	helperEnv = replaceEnv(helperEnv, "XDG_CONFIG_HOME", fakeXDGConfig)
	helperEnv = replaceEnv(helperEnv, "XDG_DATA_HOME", fakeXDGData)
	helperEnv = replaceEnv(helperEnv, fizeauCallerDeathHelperEnv, "1")
	helperEnv = replaceEnv(helperEnv, fizeauCallerDeathWorkDirEnv, workDir)
	helperEnv = replaceEnv(helperEnv, fizeauCallerDeathSessionLogEnv, sessionLogDir)
	helperEnv = replaceEnv(helperEnv, fizeauCallerDeathTargetPIDEnv, "lifecycle-target.pid")
	helperEnv = replaceEnv(helperEnv, fizeauCallerDeathGrandPIDEnv, "lifecycle-grandchild.pid")
	helperEnv = replaceEnv(helperEnv, fizeauCallerDeathStubDirEnv, binDir)
	helperEnv = replaceEnv(helperEnv, "PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	helperCmd.Env = helperEnv
	helperCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	helperCmd.Stdout = os.Stdout
	helperCmd.Stderr = os.Stderr
	require.NoError(t, helperCmd.Start())
	t.Cleanup(func() {
		if helperCmd.Process != nil {
			_ = syscall.Kill(-helperCmd.Process.Pid, syscall.SIGKILL)
		}
	})

	targetPID := waitForCompletePIDFile(t, filepath.Join(workDir, "lifecycle-target.pid"), 10*time.Second)
	grandchildPID := waitForCompletePIDFile(t, filepath.Join(workDir, "lifecycle-grandchild.pid"), 10*time.Second)
	if targetPID == grandchildPID {
		t.Fatalf("target and grandchild PIDs must differ; both %d", targetPID)
	}
	assertWrappedTreeProcessAlive(t, targetPID, "target")
	assertWrappedTreeProcessAlive(t, grandchildPID, "grandchild")

	done := make(chan error, 1)
	go func() {
		done <- helperCmd.Wait()
	}()
	require.NoError(t, syscall.Kill(helperCmd.Process.Pid, syscall.SIGKILL))
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("helper process exited cleanly after SIGKILL, want signal termination")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper process did not exit after SIGKILL")
	}

	deadline := fizeauCallerDeathReapDeadline()
	var targetState, grandchildState string
	require.Eventually(t, func() bool {
		targetState = processDeadOrZombieStatus(targetPID)
		grandchildState = processDeadOrZombieStatus(grandchildPID)
		return processDeadOrZombie(targetPID) && processDeadOrZombie(grandchildPID)
	}, deadline, 25*time.Millisecond, "caller-death cleanup must reap the wrapped tree (target state=%s grandchild state=%s)", procStateSnapshot{&targetState}, procStateSnapshot{&grandchildState})
}

// TestFizeauCallerDeathTerminatesWrappedProcessTreeHelper is invoked as a
// subprocess by TestFizeauCallerDeathTerminatesWrappedProcessTree.
func TestFizeauCallerDeathTerminatesWrappedProcessTreeHelper(t *testing.T) {
	if os.Getenv(fizeauCallerDeathHelperEnv) != "1" {
		return
	}

	workDir := os.Getenv(fizeauCallerDeathWorkDirEnv)
	sessionLogDir := os.Getenv(fizeauCallerDeathSessionLogEnv)
	targetPIDFile := os.Getenv(fizeauCallerDeathTargetPIDEnv)
	grandchildPIDFile := os.Getenv(fizeauCallerDeathGrandPIDEnv)
	stubDir := os.Getenv(fizeauCallerDeathStubDirEnv)
	if workDir == "" || sessionLogDir == "" || targetPIDFile == "" || grandchildPIDFile == "" || stubDir == "" {
		t.Fatal("caller-death helper missing required environment")
	}
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

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
		StaleHarnessReaperGrace: time.Hour,
	})
	if err != nil {
		t.Fatalf("agentlib.New: %v", err)
	}

	events, err := svc.Execute(context.Background(), agentlib.ServiceExecuteRequest{
		Prompt:        "fizeau-caller-death-process-tree",
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

	go func() {
		for range events {
		}
	}()

	targetPID := waitForCompletePIDFile(t, filepath.Join(workDir, filepath.Base(targetPIDFile)), 10*time.Second)
	grandchildPID := waitForCompletePIDFile(t, filepath.Join(workDir, filepath.Base(grandchildPIDFile)), 10*time.Second)
	if targetPID == grandchildPID {
		t.Fatalf("target and grandchild PIDs must differ; both %d", targetPID)
	}
	assertWrappedTreeProcessAlive(t, targetPID, "target")
	assertWrappedTreeProcessAlive(t, grandchildPID, "grandchild")

	select {}
}

func replaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			if !replaced {
				out = append(out, prefix+value)
				replaced = true
			}
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

func fizeauCallerDeathReapDeadline() time.Duration {
	if os.Getenv("CI") != "" {
		return 20 * time.Second
	}
	return 5 * time.Second
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
