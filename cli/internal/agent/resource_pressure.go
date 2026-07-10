package agent

import "context"

// ResourcePressureSeverity classifies how close a resource is to exhaustion.
type ResourcePressureSeverity string

const (
	// ResourcePressureOK means the resource is within normal operating range.
	ResourcePressureOK ResourcePressureSeverity = "ok"
	// ResourcePressureWarn means the resource is approaching exhaustion but
	// has not yet crossed the operator-attention threshold.
	ResourcePressureWarn ResourcePressureSeverity = "warn"
	// ResourcePressureOperatorAttention means the resource is close enough to
	// exhaustion that an operator should be notified before it fails outright.
	ResourcePressureOperatorAttention ResourcePressureSeverity = "operator_attention"
)

const (
	// fdPressureWarnRatio is the fd_used/fd_limit ratio at which FD usage is
	// classified as warn.
	fdPressureWarnRatio = 0.80
	// fdPressureOperatorAttentionRatio is the fd_used/fd_limit ratio at which
	// FD usage is classified as operator_attention.
	fdPressureOperatorAttentionRatio = 0.90
)

// ResourcePressureCheck captures a single resource-pressure classification.
type ResourcePressureCheck struct {
	FDUsed   int                      `json:"fd_used"`
	FDLimit  uint64                   `json:"fd_limit"`
	FDRatio  float64                  `json:"fd_ratio"`
	Severity ResourcePressureSeverity `json:"severity"`
}

// CheckFDUsage classifies file-descriptor pressure from fdUsed/fdLimit. It is
// a pure, side-effect-free function so callers (and tests) can inject
// arbitrary fd counts without probing the real process or host limits.
func CheckFDUsage(fdUsed int, fdLimit uint64) ResourcePressureCheck {
	check := ResourcePressureCheck{FDUsed: fdUsed, FDLimit: fdLimit}
	if fdLimit == 0 {
		check.Severity = ResourcePressureOK
		return check
	}

	check.FDRatio = float64(fdUsed) / float64(fdLimit)
	switch {
	case check.FDRatio >= fdPressureOperatorAttentionRatio:
		check.Severity = ResourcePressureOperatorAttention
	case check.FDRatio >= fdPressureWarnRatio:
		check.Severity = ResourcePressureWarn
	default:
		check.Severity = ResourcePressureOK
	}
	return check
}

// ResourcePressureReport bundles current-process FD pressure with
// non-destructive resource observation counts so operators can see resource
// trend data before any threshold trips a hard stop.
type ResourcePressureReport struct {
	ResourcePressureCheck
	WorkerSubprocessCount  int64 `json:"worker_subprocess_count"`
	TempWorktreeCount      int64 `json:"temp_worktree_count"`
	StaleExecutionDirCount int64 `json:"stale_execution_dir_count"`
}

// ResourcePressureChecker reports current resource pressure without
// performing cleanup or claim-blocking side effects. Implementations must be
// non-destructive: Check must not remove files or kill processes.
type ResourcePressureChecker interface {
	Check(ctx context.Context) (ResourcePressureReport, error)
}

// CheckProcessFDUsage classifies the current process's open-file-descriptor
// pressure against its RLIMIT_NOFILE soft limit.
func CheckProcessFDUsage() ResourcePressureCheck {
	diag := collectFDDiagnostics()
	return CheckFDUsage(diag.Count, diag.SoftLimit)
}
