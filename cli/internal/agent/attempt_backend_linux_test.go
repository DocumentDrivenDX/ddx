//go:build linux

package agent

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"

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
