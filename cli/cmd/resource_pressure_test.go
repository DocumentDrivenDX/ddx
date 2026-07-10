package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeResourcePressureChecker struct {
	report agent.ResourcePressureReport
	err    error
}

func (f *fakeResourcePressureChecker) Check(ctx context.Context) (agent.ResourcePressureReport, error) {
	_ = ctx
	return f.report, f.err
}

// TestResourcePressure_ServerStartupEmitsDiagnostics verifies that ddx server
// startup surfaces the same resource pressure fields (fd_used, fd_limit,
// worker subprocess count, temp worktree count, stale execution dir count)
// that worker pre-claim reports, without blocking server construction.
func TestResourcePressure_ServerStartupEmitsDiagnostics(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	projectRoot := t.TempDir()

	factory := NewCommandFactory(projectRoot)
	factory.serverListenAndServeOverride = func(cert, key string) error { return nil }
	factory.resourcePressureCheckerOverride = &fakeResourcePressureChecker{
		report: agent.ResourcePressureReport{
			ResourcePressureCheck:  agent.CheckFDUsage(92, 100),
			WorkerSubprocessCount:  4,
			TempWorktreeCount:      1,
			StaleExecutionDirCount: 7,
		},
	}

	root := factory.NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"server", "--tsnet=false"})
	err := root.Execute()
	require.NoError(t, err, "ddx server must not fail because of resource pressure diagnostics")

	out := buf.String()
	assert.Contains(t, out, "resource pressure", "startup diagnostics must mention resource pressure")
	assert.Contains(t, out, string(agent.ResourcePressureOperatorAttention))
	assert.Contains(t, out, "fd_used=92")
	assert.Contains(t, out, "fd_limit=100")
	assert.Contains(t, out, "worker_subprocess_count=4")
	assert.Contains(t, out, "temp_worktree_count=1")
	assert.Contains(t, out, "stale_execution_dir_count=7")
}

// TestResourcePressure_ServerStartupSkipsHealthyDiagnostics verifies that
// server startup stays silent about resource pressure when the checker
// reports OK severity, so healthy runs don't accumulate noisy output.
func TestResourcePressure_ServerStartupSkipsHealthyDiagnostics(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	projectRoot := t.TempDir()

	factory := NewCommandFactory(projectRoot)
	factory.serverListenAndServeOverride = func(cert, key string) error { return nil }
	factory.resourcePressureCheckerOverride = &fakeResourcePressureChecker{
		report: agent.ResourcePressureReport{
			ResourcePressureCheck: agent.CheckFDUsage(1, 100),
		},
	}

	root := factory.NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"server", "--tsnet=false"})
	require.NoError(t, root.Execute())

	assert.NotContains(t, buf.String(), "resource pressure")
}
