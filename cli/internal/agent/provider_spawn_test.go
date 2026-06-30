//go:build linux

package agent

import (
	"context"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExecutor_ProviderSpawnSetsPdeathsigAndSetpgid verifies that
// BuildProviderLaunchCmd — the provider-launch seam used to wrap codex/claude
// subprocesses spawned by the Fizeau service — applies cmdSetProcessGroup
// semantics on Linux. Setpgid keeps SIGKILL reachable for the entire process
// group via kill(-pgid, …), and Pdeathsig=SIGKILL is the kernel parent-death
// signal that reaps the wrapper when the worker dies abnormally. Because
// Pdeathsig is preserved across execve(2) for non-setuid/non-capability
// binaries (per prctl(2)), the codex/claude process inherits the death-signal
// once the wrapper hands control off via syscall.Exec.
//
// See ddx-01b89378 for the production incident (28 codex orphans, ppid=1)
// that motivated this seam.
func TestExecutor_ProviderSpawnSetsPdeathsigAndSetpgid(t *testing.T) {
	cmd := BuildProviderLaunchCmd(context.Background(), "/usr/bin/codex", "exec", "--json")

	require.NotNil(t, cmd, "BuildProviderLaunchCmd must return a non-nil *exec.Cmd")
	require.NotNil(t, cmd.SysProcAttr, "SysProcAttr must be set so the kernel reaps the wrapper on parent death")
	require.True(t, cmd.SysProcAttr.Setpgid, "Setpgid must be true so cmdKillProcessGroup can reach descendants via kill(-pgid)")
	require.Equal(t, syscall.SIGKILL, cmd.SysProcAttr.Pdeathsig, "Pdeathsig must be SIGKILL to prevent orphan codex/claude processes when the worker dies")
}
