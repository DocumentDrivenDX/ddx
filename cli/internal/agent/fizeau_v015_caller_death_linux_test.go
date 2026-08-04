//go:build linux

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/require"
)

const (
	fizeauCallerDeathHelperEnv     = "DDX_FIZEAU_CALLER_DEATH_HELPER"
	fizeauCallerDeathWorkDirEnv    = "DDX_FIZEAU_CALLER_DEATH_WORKDIR"
	fizeauCallerDeathSessionLogEnv = "DDX_FIZEAU_CALLER_DEATH_SESSION_LOG_DIR"
	fizeauCallerDeathTargetPIDEnv  = "DDX_FIZEAU_CALLER_DEATH_TARGET_PID_FILE"
	fizeauCallerDeathGrandPIDEnv   = "DDX_FIZEAU_CALLER_DEATH_GRANDCHILD_PID_FILE"
	fizeauCallerDeathStubDirEnv    = "DDX_FIZEAU_CALLER_DEATH_STUB_DIR"
)

// TestFizeauCallerDeathTerminatesWrappedProcessTree proves the public Execute
// contract tears down the wrapped harness process tree when the embedding DDx
// worker dies abnormally.
func TestFizeauCallerDeathTerminatesWrappedProcessTree(t *testing.T) {
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
