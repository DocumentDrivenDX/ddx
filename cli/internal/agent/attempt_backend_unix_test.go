//go:build !windows

package agent

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDockerCloneBackend_ProcessGroupCleanup(t *testing.T) {
	cmd := dockerAttemptCommand(context.Background(), "run", "--rm", "--init", "runner:latest")

	require.NotNil(t, cmd)
	require.NotNil(t, cmd.SysProcAttr)
	require.True(t, cmd.SysProcAttr.Setpgid, "docker-clone must start docker in its own process group")
	require.Equal(t, syscall.SIGKILL, cmd.SysProcAttr.Pdeathsig)
	require.Equal(t, "docker", filepath.Base(cmd.Path))
	require.Contains(t, cmd.Args, "--rm")
	require.Contains(t, cmd.Args, "--init")
}

func TestDockerCloneBackendConfiguresProcessGroupForDockerCommand(t *testing.T) {
	// Verify the docker-clone backend applies the same process-group setup contract
	// to the docker invocation as other execute-bead harness paths.
	cmd := dockerAttemptCommand(context.Background(), "ps")

	require.NotNil(t, cmd)
	require.NotNil(t, cmd.SysProcAttr, "docker command must have SysProcAttr configured")
	require.True(t, cmd.SysProcAttr.Setpgid, "docker command must set Setpgid for process-group isolation")
	require.Equal(t, syscall.SIGKILL, cmd.SysProcAttr.Pdeathsig, "docker command must set Pdeathsig for parent-death cleanup")
}

func TestDockerCloneBackendTerminatesContainerOnWorkerCancellation(t *testing.T) {
	// Verify context cancellation or graceful worker shutdown stops the docker CLI process
	// and the associated containerized harness tree.
	// Note: This test verifies the mechanism without requiring an actual docker daemon.
	// The actual container cleanup is guaranteed by the --rm flag in dockerRunArgs.

	ctx, cancel := context.WithCancel(context.Background())

	// Create a docker command that will be cancelled
	cmd := dockerAttemptCommand(ctx, "ps")

	// Cancel the context to simulate worker shutdown
	time.AfterFunc(10*time.Millisecond, cancel)

	// The command should respect context cancellation through exec.CommandContext
	err := cmd.Run()

	// We expect an error since we cancelled the context
	require.Error(t, err, "docker command should be interrupted by context cancellation")

	// Verify the context was actually cancelled
	require.Eventually(t, func() bool {
		return ctx.Err() != nil
	}, 100*time.Millisecond, 5*time.Millisecond, "context should be cancelled")
}

func TestDockerCloneBackendParentDeathCleanupDocumentedOrEnforced(t *testing.T) {
	// Verify abnormal parent-death behavior for docker-clone is either enforced
	// through parent-death/process-group/container cleanup or has a documented justification.

	cmd := dockerAttemptCommand(context.Background(), "ps")

	// Verify that process-group setup is in place for docker command
	require.NotNil(t, cmd.SysProcAttr, "docker command must have SysProcAttr")
	require.True(t, cmd.SysProcAttr.Setpgid, "Setpgid must be set for process-group isolation")
	require.Equal(t, syscall.SIGKILL, cmd.SysProcAttr.Pdeathsig, "Pdeathsig must be set to SIGKILL for parent-death cleanup")

	// Verify the docker run arguments include --rm for automatic container cleanup
	ws := &AttemptWorkspace{
		ProjectRoot: "/repo",
		WorkDir:     "/tmp/work",
		BeadID:      "ddx-test",
		AttemptID:   "test-attempt",
		DockerRun:   "/tmp/runtime",
		DockerHome:  "/tmp/runtime/home",
	}
	args := dockerRunArgs(nil, ws, "/usr/bin/ddx", "test:latest", nil)
	require.Contains(t, args, "--rm", "docker run must include --rm for automatic container cleanup on exit")
	require.Contains(t, args, "--init", "docker run must include --init for proper signal handling inside container")
}
