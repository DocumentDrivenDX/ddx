package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// cliResourcePressureChecker adapts the non-destructive startup housekeeping
// scan (worker liveness dirs, temp worktrees, stale execution dirs) plus
// current-process FD pressure into agent.ResourcePressureChecker, so worker
// pre-claim and server startup surface the same resource pressure fields
// (ddx-e9182ba1). Check never removes files or kills processes.
type cliResourcePressureChecker struct {
	runner *startupHousekeepingRunner
}

// buildCLIResourcePressureChecker constructs the default resource pressure
// checker for projectRoot, or returns override when non-nil (test injection).
func buildCLIResourcePressureChecker(projectRoot string, override agent.ResourcePressureChecker) agent.ResourcePressureChecker {
	if override != nil {
		return override
	}
	return &cliResourcePressureChecker{runner: newStartupHousekeepingRunner(projectRoot)}
}

func (c *cliResourcePressureChecker) Check(ctx context.Context) (agent.ResourcePressureReport, error) {
	report := agent.ResourcePressureReport{ResourcePressureCheck: agent.CheckProcessFDUsage()}
	if c == nil || c.runner == nil {
		return report, nil
	}
	scan, err := c.runner.scan(ctx, false)
	if err != nil {
		return report, err
	}
	report.WorkerSubprocessCount = scan.WorkerSubprocessCount
	report.TempWorktreeCount = scan.TempWorktreeCount
	report.StaleExecutionDirCount = scan.StaleExecutionDirs
	return report, nil
}

// emitServerResourcePressureDiagnostics writes non-blocking resource pressure
// diagnostics to w when report carries warn or operator_attention severity.
// Called by the server RunE before ListenAndServeTLS, mirroring
// emitServerPreflightDiagnostics.
func emitServerResourcePressureDiagnostics(w io.Writer, report agent.ResourcePressureReport) {
	if report.Severity != agent.ResourcePressureWarn && report.Severity != agent.ResourcePressureOperatorAttention {
		return
	}
	fmt.Fprintf(w, "DDx server: resource pressure %s (fd_used=%d fd_limit=%d fd_ratio=%.2f)\n",
		report.Severity, report.FDUsed, report.FDLimit, report.FDRatio)
	fmt.Fprintf(w, "  worker_subprocess_count=%d temp_worktree_count=%d stale_execution_dir_count=%d\n",
		report.WorkerSubprocessCount, report.TempWorktreeCount, report.StaleExecutionDirCount)
}
