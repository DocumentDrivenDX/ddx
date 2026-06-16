//go:build linux

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmdTestsDoNotSpawnRealProviderCLIs(t *testing.T) {
	for _, testName := range []string{
		"TestWorkResourceExhaustionEndToEnd_StopsBeforeNextClaim",
		"TestWorkDoesNotSpawnProviderAfterUnderSpecifiedRouting",
	} {
		t.Run(testName, func(t *testing.T) {
			runCmdTestAndAssertNoProviderDescendants(t, testName)
		})
	}
}

func runCmdTestAndAssertNoProviderDescendants(t *testing.T, testName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0],
		"-test.run", "^"+regexp.QuoteMeta(testName)+"$",
		"-test.count=1",
		"-test.v",
	)
	cmd.Env = append(os.Environ(), "DDX_CMD_PROVIDER_LEAK_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	require.NoError(t, cmd.Start())

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	require.NoError(t, err)
	t.Cleanup(func() {
		killProcessGroup(pgid, syscall.SIGKILL)
	})

	err = cmd.Wait()
	if ctx.Err() != nil {
		killProcessGroup(pgid, syscall.SIGKILL)
		t.Fatalf("%s timed out:\n%s", testName, out.String())
	}
	require.NoErrorf(t, err, "%s failed:\n%s", testName, out.String())

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		leaks := providerProcessesInGroup(pgid)
		require.Empty(c, leaks, "provider CLI descendants still alive after %s:\n%s\nchild output:\n%s", testName, strings.Join(leaks, "\n"), out.String())
	}, 3*time.Second, 50*time.Millisecond)
}

func providerProcessesInGroup(pgid int) []string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var leaks []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		procPGID, ok := readProcGroup(pid)
		if !ok || procPGID != pgid {
			continue
		}
		name := readProcName(pid)
		if !isProviderProcessName(name) {
			continue
		}
		leaks = append(leaks, fmt.Sprintf("pid=%d pgid=%d name=%s cmd=%q", pid, pgid, name, readProcCmdline(pid)))
	}
	return leaks
}

func readProcGroup(pid int) (int, bool) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, false
	}
	stat := string(data)
	endComm := strings.LastIndex(stat, ")")
	if endComm < 0 || endComm+2 >= len(stat) {
		return 0, false
	}
	fields := strings.Fields(stat[endComm+2:])
	if len(fields) < 3 {
		return 0, false
	}
	pgid, err := strconv.Atoi(fields[2])
	return pgid, err == nil
}

func readProcName(pid int) string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
	if err == nil {
		return strings.TrimSpace(string(data))
	}
	cmdline := readProcCmdline(pid)
	if cmdline == "" {
		return ""
	}
	return filepath.Base(strings.Fields(cmdline)[0])
}

func readProcCmdline(pid int) string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil || len(data) == 0 {
		return ""
	}
	data = bytes.TrimRight(data, "\x00")
	return strings.ReplaceAll(string(bytes.ReplaceAll(data, []byte{0}, []byte{' '})), "\n", " ")
}

func isProviderProcessName(name string) bool {
	switch filepath.Base(name) {
	case "codex", "claude", "gemini":
		return true
	default:
		return false
	}
}

func killProcessGroup(pgid int, sig syscall.Signal) {
	if pgid > 0 {
		_ = syscall.Kill(-pgid, sig)
	}
}
