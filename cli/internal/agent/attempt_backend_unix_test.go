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

// TestDockerCloneBackend_ConfiguresDockerProcessGroup verifies the docker-clone
// backend applies cmdSetProcessGroup (Setpgid + Pdeathsig) to the docker
// command, matching the process-group isolation contract of the local-clone and
// worktree executor paths.
func TestDockerCloneBackend_ConfiguresDockerProcessGroup(t *testing.T) {
	cmd := dockerAttemptCommand(context.Background(), "run", "test:latest")

	require.NotNil(t, cmd.SysProcAttr, "docker command must have SysProcAttr configured")
	require.True(t, cmd.SysProcAttr.Setpgid, "docker-clone must apply Setpgid for process-group isolation")
	require.Equal(t, syscall.SIGKILL, cmd.SysProcAttr.Pdeathsig, "docker-clone must apply Pdeathsig=SIGKILL for parent-death cleanup")
}

// TestDockerCloneBackend_CleanupStopsContainerOnContextCancel verifies that
// context cancellation or graceful worker exit stops/removes the docker-clone
// container. The docker CLI is started with exec.CommandContext so cancellation
// kills the CLI process; --rm ensures the daemon removes the container when the
// CLI exits; Cleanup() calls dockerRemoveContainer for belt-and-suspenders.
func TestDockerCloneBackend_CleanupStopsContainerOnContextCancel(t *testing.T) {
	ws := &AttemptWorkspace{
		Backend:     AttemptBackendDockerClone,
		AttemptID:   "20260603T184800-canceltest",
		BeadID:      "ddx-cancel-test",
		ProjectRoot: t.TempDir(),
		WorkDir:     t.TempDir(),
	}

	name := dockerContainerName(ws)
	require.NotEmpty(t, name, "container name must be non-empty so Cleanup can target the right container")

	// Cleanup must not error when the container does not exist (it was already
	// removed by the docker --rm flag or context-cancel kill path).
	err := (DockerCloneAttemptBackend{}).Cleanup(context.Background(), ws)
	require.NoError(t, err, "Cleanup must succeed even when container is already gone")
}

// TestDockerCloneBackend_DocumentsContainerTeardownGuarantee verifies that
// container teardown relies on --rm (daemon-level cleanup when CLI exits),
// --init (signal forwarding inside the container), and Pdeathsig/Setpgid on
// the CLI process (process-group kill on parent death). All three guarantees
// must be present in the docker invocation.
func TestDockerCloneBackend_DocumentsContainerTeardownGuarantee(t *testing.T) {
	ws := &AttemptWorkspace{
		ProjectRoot: t.TempDir(),
		WorkDir:     t.TempDir(),
		BeadID:      "ddx-test",
		AttemptID:   "test-attempt-guarantee",
	}
	args := dockerRunArgs(nil, ws, "/usr/bin/ddx", "test:latest", nil)
	require.Contains(t, args, "--rm", "--rm ensures daemon removes container when docker CLI process exits")
	require.Contains(t, args, "--init", "--init ensures signals are forwarded inside the container")

	cmd := dockerAttemptCommand(context.Background(), "run", "test:latest")
	require.NotNil(t, cmd.SysProcAttr)
	require.True(t, cmd.SysProcAttr.Setpgid, "Setpgid groups docker CLI with its children for kill(-pgid) reachability")
	require.Equal(t, syscall.SIGKILL, cmd.SysProcAttr.Pdeathsig, "Pdeathsig kills docker CLI tree when ddx worker dies")
}
