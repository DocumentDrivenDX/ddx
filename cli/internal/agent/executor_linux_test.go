//go:build linux

package agent

import (
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

	"github.com/stretchr/testify/require"
)

const (
	executorPdeathsigHelperEnv       = "DDX_EXECUTOR_PDEATHSIG_HELPER"
	executorPdeathsigChildPIDFileEnv = "DDX_EXECUTOR_PDEATHSIG_CHILD_PID_FILE"
	executorPdeathsigGrandPIDFileEnv = "DDX_EXECUTOR_PDEATHSIG_GRANDCHILD_PID_FILE"
)

func executorReadPIDFile(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	return pid
}

func executorProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}

func executorProcessNotOrphaned(pid int) bool {
	if pid <= 0 {
		return true
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return errors.Is(err, syscall.ESRCH)
	}
	ppid, ok := readOrphanHarnessParentPID(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if !ok {
		return false
	}
	return ppid != 1
}

// TestCmdSetProcessGroup_LinuxConfiguresPdeathsigAndSetpgid verifies that
// cmdSetProcessGroup sets both Setpgid (process-group isolation) and Pdeathsig
// (kernel parent-death signal) on Linux.
func TestCmdSetProcessGroup_LinuxConfiguresPdeathsigAndSetpgid(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	cmdSetProcessGroup(cmd)

	require.NotNil(t, cmd.SysProcAttr)
	require.True(t, cmd.SysProcAttr.Setpgid, "expected process-group isolation")
	require.Equal(t, syscall.SIGKILL, cmd.SysProcAttr.Pdeathsig, "expected Linux parent-death signal")
}

// TestOSExecutorExecuteInDir_LinuxPdeathsigKillsHarnessOnParentSIGKILL starts
// an execute-bead executor through a helper process, runs a fake long-lived
// harness, SIGKILLs the helper process, and asserts the direct harness child
// does not survive as a PID-1 orphan within a bounded grace on Linux.
func TestOSExecutorExecuteInDir_LinuxPdeathsigKillsHarnessOnParentSIGKILL(t *testing.T) {
	dir := t.TempDir()
	childPIDFile := filepath.Join(dir, "child.pid")
	grandPIDFile := filepath.Join(dir, "grandchild.pid")

	ctx := withExecutionEnv(context.Background(), map[string]string{
		executorPdeathsigHelperEnv:       "1",
		executorPdeathsigChildPIDFileEnv: childPIDFile,
		executorPdeathsigGrandPIDFileEnv: grandPIDFile,
	})

	executor := &OSExecutor{}
	result, err := executor.ExecuteInDir(
		ctx,
		os.Args[0],
		[]string{"-test.run=^TestOSExecutorExecuteInDir_LinuxPdeathsigKillsHarnessHelper$"},
		"",
		dir,
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	childPID := executorReadPIDFile(t, childPIDFile)
	grandPID := executorReadPIDFile(t, grandPIDFile)

	require.Eventually(t, func() bool {
		return !executorProcessAlive(childPID) && !executorProcessAlive(grandPID)
	}, 5*time.Second, 25*time.Millisecond, "parent-death cleanup must reap the harness tree")

	require.Eventually(t, func() bool {
		return executorProcessNotOrphaned(grandPID)
	}, 5*time.Second, 25*time.Millisecond, "grandchild must not stick around as a PID 1 orphan")
}

// TestOSExecutorExecuteInDir_LinuxPdeathsigKillsHarnessHelper is a subprocess
// helper invoked by TestOSExecutorExecuteInDir_LinuxPdeathsigKillsHarnessOnParentSIGKILL.
// When DDX_EXECUTOR_PDEATHSIG_HELPER is not set it returns immediately so that
// normal test runs skip it silently.
func TestOSExecutorExecuteInDir_LinuxPdeathsigKillsHarnessHelper(t *testing.T) {
	if os.Getenv(executorPdeathsigHelperEnv) != "1" {
		return
	}

	childPIDFile := os.Getenv(executorPdeathsigChildPIDFileEnv)
	grandPIDFile := os.Getenv(executorPdeathsigGrandPIDFileEnv)
	if childPIDFile == "" || grandPIDFile == "" {
		t.Fatal("helper pid file env vars are required")
	}

	grandchild := exec.Command("sleep", "300")
	cmdSetProcessGroup(grandchild)
	require.NoError(t, grandchild.Start())

	require.NoError(t, os.WriteFile(childPIDFile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644))
	require.NoError(t, os.WriteFile(grandPIDFile, []byte(strconv.Itoa(grandchild.Process.Pid)+"\n"), 0o644))

	// Give the outer test time to observe the spawned tree before we
	// simulate a worker crash with SIGKILL.
	time.Sleep(100 * time.Millisecond)
	_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
}

// TestOSExecutorExecuteInDir_ParentDeathSignalThreadGuard verifies that
// executor.go locks the OS thread around cmd.Start() to ensure Pdeathsig is
// reliably associated with the creating thread. Go's runtime may migrate a
// goroutine to a different OS thread; LockOSThread prevents that migration
// during the critical exec window so the child inherits the parent-death signal
// from the correct thread (golang.org/issue/27505).
func TestOSExecutorExecuteInDir_ParentDeathSignalThreadGuard(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)

	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "executor.go"))
	require.NoError(t, err)

	src := string(data)
	require.Contains(t, src, "runtime.LockOSThread()", "start path must lock the OS thread before cmd.Start()")
	require.Contains(t, src, "cmd.Start()", "start path must still start the child process under the lock")
}

// TestOSExecutorExecuteInDir_GracefulCancellationStillKillsProcessGroup is a
// regression guard: ctx cancellation or SIGTERM-style graceful shutdown must
// still terminate the entire harness process group, not just the direct child.
func TestOSExecutorExecuteInDir_GracefulCancellationStillKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	childPIDFile := filepath.Join(dir, "child.pid")
	grandPIDFile := filepath.Join(dir, "grandchild.pid")
	scriptPath := filepath.Join(dir, "harness.sh")
	script := `#!/bin/sh
set -eu
sleep 300 &
echo $$ > "` + childPIDFile + `"
echo $! > "` + grandPIDFile + `"
wait
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	executor := &OSExecutor{}
	result, err := executor.ExecuteInDir(ctx, scriptPath, nil, "", dir)
	require.Error(t, err, "graceful cancellation should stop the harness tree")
	require.NotNil(t, result)

	childPID := executorReadPIDFile(t, childPIDFile)
	grandPID := executorReadPIDFile(t, grandPIDFile)
	require.Eventually(t, func() bool {
		return !executorProcessAlive(childPID) && !executorProcessAlive(grandPID)
	}, 5*time.Second, 25*time.Millisecond, "graceful cancellation must kill the entire harness tree")
}
