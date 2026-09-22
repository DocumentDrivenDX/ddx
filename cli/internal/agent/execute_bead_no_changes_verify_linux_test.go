//go:build linux

package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultVerificationCommandRunnerTimeoutKillsProcessGroup(t *testing.T) {
	projectRoot := t.TempDir()
	shellPIDFile := filepath.Join(projectRoot, "inner-shell.pid")
	childPIDFile := filepath.Join(projectRoot, "sleep.pid")
	command := nestedPIDCaptureCommand(shellPIDFile, childPIDFile, "sleep 30")

	code, _, err := DefaultVerificationCommandRunnerWithTimeout(time.Second)(context.Background(), projectRoot, command)
	require.Error(t, err)
	assert.Equal(t, -1, code)
	assert.Contains(t, err.Error(), "timed out after")

	shellPID := readPIDFile(t, shellPIDFile)
	childPID := readPIDFile(t, childPIDFile)
	var shellState, childState string
	require.Eventually(t, func() bool {
		shellState = processDeadOrZombieStatus(shellPID)
		childState = processDeadOrZombieStatus(childPID)
		return processDeadOrZombie(shellPID) && processDeadOrZombie(childPID)
	}, time.Second, 20*time.Millisecond, "shell proc state=%s child proc state=%s", procStateSnapshot{&shellState}, procStateSnapshot{&childState})
}
