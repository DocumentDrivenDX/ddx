package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNoChangesRationale(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want ParsedNoChangesRationale
	}{
		{
			name: "empty",
			in:   "",
			want: ParsedNoChangesRationale{Kind: NoChangesKindUnjustified},
		},
		{
			name: "bare",
			in:   "nothing to do here",
			want: ParsedNoChangesRationale{Kind: NoChangesKindUnjustified},
		},
		{
			name: "verification_command basic",
			in:   "verification_command: go test ./foo -run TestX\noutput: PASS",
			want: ParsedNoChangesRationale{Kind: NoChangesKindVerified, VerificationCommand: "go test ./foo -run TestX"},
		},
		{
			name: "open with reason and suggested action",
			in:   "status: open\nreason: autonomous work remains possible\nmore detail line\nsuggested_action: retry with smart agent",
			want: ParsedNoChangesRationale{
				Kind:            NoChangesKindLifecycleStatus,
				LifecycleStatus: "open",
				Reason:          "autonomous work remains possible more detail line",
				SuggestedAction: "retry with smart agent",
			},
		},
		{
			name: "legacy needs_investigation rejected",
			in:   "status: needs_investigation\nreason: provider quota unknown\nmore detail line",
			want: ParsedNoChangesRationale{
				Kind:            NoChangesKindRejectedLegacyStatus,
				LifecycleStatus: "needs_investigation",
				Reason:          "provider quota unknown more detail line",
				RejectionReason: "status: needs_investigation is no longer accepted; use status: open, status: proposed, or status: blocked for new no_changes output, and run `ddx bead migrate --lifecycle` for stored legacy rows",
			},
		},
		{
			name: "verification_command takes precedence over status",
			in:   "status: needs_investigation\nverification_command: true",
			want: ParsedNoChangesRationale{Kind: NoChangesKindVerified, VerificationCommand: "true"},
		},
		{
			name: "case insensitive markers",
			in:   "Verification_Command: ls\noutput: x",
			want: ParsedNoChangesRationale{Kind: NoChangesKindVerified, VerificationCommand: "ls"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseNoChangesRationale(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDefaultVerificationCommandRunner(t *testing.T) {
	t.Run("exit 0", func(t *testing.T) {
		code, _, err := DefaultVerificationCommandRunner(context.Background(), "", "true")
		assert.NoError(t, err)
		assert.Equal(t, 0, code)
	})
	t.Run("exit non-zero", func(t *testing.T) {
		code, _, err := DefaultVerificationCommandRunner(context.Background(), "", "false")
		assert.NoError(t, err)
		assert.NotEqual(t, 0, code)
	})
}

func TestDefaultVerificationCommandRunnerTimeoutKillsProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group assertions are unix-specific")
	}

	projectRoot := t.TempDir()
	shellPIDFile := filepath.Join(projectRoot, "inner-shell.pid")
	childPIDFile := filepath.Join(projectRoot, "sleep.pid")
	command := nestedPIDCaptureCommand(shellPIDFile, childPIDFile, "sleep 30")

	code, _, err := DefaultVerificationCommandRunnerWithTimeout(100*time.Millisecond)(context.Background(), projectRoot, command)
	require.Error(t, err)
	assert.Equal(t, -1, code)
	assert.Contains(t, err.Error(), "timed out after")

	shellPID := readPIDFile(t, shellPIDFile)
	childPID := readPIDFile(t, childPIDFile)
	require.Eventually(t, func() bool {
		return !processExists(shellPID) && !processExists(childPID)
	}, time.Second, 20*time.Millisecond)
}

func TestDefaultVerificationCommandRunnerAllowsConfiguredLongGate(t *testing.T) {
	command := "sh -lc 'sleep 0.12'"

	shortRunner := DefaultVerificationCommandRunnerWithTimeout(50 * time.Millisecond)
	shortCode, _, shortErr := shortRunner(context.Background(), "", command)
	require.Error(t, shortErr)
	assert.Equal(t, -1, shortCode)
	assert.Contains(t, shortErr.Error(), "timed out after")

	longRunner := DefaultVerificationCommandRunnerWithTimeout(250 * time.Millisecond)
	longCode, _, longErr := longRunner(context.Background(), "", command)
	require.NoError(t, longErr)
	assert.Equal(t, 0, longCode)
}

func nestedPIDCaptureCommand(shellPIDFile, childPIDFile, longRunningCommand string) string {
	return fmt.Sprintf(
		"sh -lc \"echo \\$\\$ > %s; %s & echo \\$! > %s; wait\"",
		shSingleQuote(shellPIDFile),
		longRunningCommand,
		shSingleQuote(childPIDFile),
	)
}

func shSingleQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

func readPIDFile(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	return pid
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
