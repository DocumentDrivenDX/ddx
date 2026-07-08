package agent

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
