//go:build !windows

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
