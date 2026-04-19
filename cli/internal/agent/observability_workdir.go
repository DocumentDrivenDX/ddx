package agent

// observability_workdir.go provides workDir-based free-function wrappers around
// the Runner-attached observability helpers (ProbeHarnessState,
// LoadRoutingSignalSnapshot, RefreshClaudeQuotaViaTmux, RefreshCodexQuotaViaTmux).
// These exist so DDx callers in cmd/ and internal/server/ do not need to
// construct an *agent.Runner just to invoke these helpers — the constructors
// f.agentRunner() and m.buildAgentRunner have been retired in favor of going
// through the agentlib.DdxAgent service for actual execution paths.
//
// The underlying implementations remain on *Runner because routing internals
// in this package still use them; these wrappers build a minimal Runner from
// workDir to drive them.

import (
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// minimalRunnerForWorkDir builds an *agent.Runner sized only for observability
// helpers that depend on Config.SessionLogDir. Mirrors the surface of the
// retired f.agentRunner() / m.buildAgentRunner constructors.
func minimalRunnerForWorkDir(workDir string) *Runner {
	cfg := Config{}
	if c, err := config.LoadWithWorkingDir(workDir); err == nil && c != nil && c.Agent != nil {
		cfg.Harness = c.Agent.Harness
		cfg.Model = c.Agent.Model
		cfg.Models = c.Agent.Models
		cfg.ReasoningLevels = c.Agent.ReasoningLevels
		cfg.TimeoutMS = c.Agent.TimeoutMS
		cfg.SessionLogDir = c.Agent.SessionLogDir
		cfg.Permissions = c.Agent.Permissions
	}
	cfg.SessionLogDir = ResolveLogDir(workDir, cfg.SessionLogDir)
	r := NewRunner(cfg)
	r.WorkDir = workDir
	return r
}

// SessionLogDirForWorkDir returns the resolved session-log directory for a
// project root, applying the same precedence rules used by the retired
// f.agentRunner() constructor: project config when present, otherwise the
// default log dir resolved against workDir.
func SessionLogDirForWorkDir(workDir string) string {
	cfg := ""
	if c, err := config.LoadWithWorkingDir(workDir); err == nil && c != nil && c.Agent != nil {
		cfg = c.Agent.SessionLogDir
	}
	return ResolveLogDir(workDir, cfg)
}

// ProbeHarnessStateForWorkDir is a workDir-based wrapper around
// (*Runner).ProbeHarnessState for callers that previously used
// f.agentRunner().ProbeHarnessState.
func ProbeHarnessStateForWorkDir(workDir, harnessName string, timeout time.Duration) HarnessState {
	return minimalRunnerForWorkDir(workDir).ProbeHarnessState(harnessName, timeout)
}

// LoadRoutingSignalSnapshotForWorkDir is a workDir-based wrapper around
// (*Runner).LoadRoutingSignalSnapshot for callers that previously used
// f.agentRunner().LoadRoutingSignalSnapshot.
func LoadRoutingSignalSnapshotForWorkDir(workDir, harnessName string, now time.Time) RoutingSignalSnapshot {
	return minimalRunnerForWorkDir(workDir).LoadRoutingSignalSnapshot(harnessName, now)
}

// RefreshClaudeQuotaViaTmuxForWorkDir is a workDir-based wrapper around
// (*Runner).RefreshClaudeQuotaViaTmux for callers that previously used
// f.agentRunner().RefreshClaudeQuotaViaTmux.
func RefreshClaudeQuotaViaTmuxForWorkDir(workDir string, now time.Time, maxAge time.Duration) error {
	return minimalRunnerForWorkDir(workDir).RefreshClaudeQuotaViaTmux(now, maxAge)
}

// RefreshCodexQuotaViaTmuxForWorkDir is a workDir-based wrapper around
// (*Runner).RefreshCodexQuotaViaTmux for callers that previously used
// f.agentRunner().RefreshCodexQuotaViaTmux.
func RefreshCodexQuotaViaTmuxForWorkDir(workDir string, now time.Time, maxAge time.Duration) error {
	return minimalRunnerForWorkDir(workDir).RefreshCodexQuotaViaTmux(now, maxAge)
}
