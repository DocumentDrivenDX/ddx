package config

import (
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// CLIOverrides carries per-invocation flag values that override
// project config. Zero values mean "no override; use config".
// Pointer fields distinguish "explicit value" from "not set".
type CLIOverrides struct {
	Harness     string
	Model       string
	Provider    string
	ModelRef    string
	Profile     string
	Effort      string
	Permissions string
	MinTier     string
	MaxTier     string
	Timeout     *time.Duration
	NoReview    *bool
	Assignee    string
}

// Resolve produces a sealed ResolvedConfig by layering overrides onto
// cfg. Overrides are applied as last-write-wins per field. The
// returned ResolvedConfig does not alias cfg's storage — every map
// and slice is deep-cloned via per-type Clone methods before being
// captured.
//
// Resolve accepts a nil cfg and returns a ResolvedConfig populated
// from package defaults, sealed normally.
func (c *NewConfig) Resolve(overrides CLIOverrides) ResolvedConfig {
	r := ResolvedConfig{sealed: true}

	r.assignee = overrides.Assignee
	r.reviewMaxRetries = c.ResolveReviewMaxRetries()

	var agent *AgentConfig
	if c != nil {
		agent = c.Agent.Clone()
	}

	r.harness = overrides.Harness
	if r.harness == "" && agent != nil {
		r.harness = agent.Harness
	}

	r.model = overrides.Model
	if r.model == "" && agent != nil {
		r.model = agent.Model
	}

	r.provider = overrides.Provider
	r.modelRef = overrides.ModelRef
	r.profile = overrides.Profile
	r.minTier = overrides.MinTier
	r.maxTier = overrides.MaxTier
	r.effort = overrides.Effort

	r.permissions = overrides.Permissions
	if r.permissions == "" && agent != nil {
		r.permissions = agent.Permissions
	}

	if overrides.Timeout != nil {
		r.timeout = *overrides.Timeout
	} else if agent != nil && agent.TimeoutMS > 0 {
		r.timeout = time.Duration(agent.TimeoutMS) * time.Millisecond
	}

	r.evidenceCaps = c.ResolveEvidenceCaps(r.harness)

	if agent != nil {
		r.sessionLogDir = agent.SessionLogDir
		if agent.ReasoningLevels != nil {
			r.reasoningLevels = make(map[string][]string, len(agent.ReasoningLevels))
			for k, v := range agent.ReasoningLevels {
				r.reasoningLevels[k] = append([]string(nil), v...)
			}
		}
		if agent.Routing != nil {
			r.resolvedLadder = map[string][]string{
				r.profile: agent.Routing.ResolvedLadder(r.profile),
			}
		}
	}
	if r.resolvedLadder == nil {
		r.resolvedLadder = map[string][]string{
			r.profile: (*RoutingConfig)(nil).ResolvedLadder(r.profile),
		}
	}

	if c != nil && c.Executions != nil && c.Executions.Mirror != nil {
		r.mirrorConfig = c.Executions.Mirror.Clone()
	}

	return r
}

// LoadAndResolve is the canonical production entry point that produces
// a sealed ResolvedConfig. It loads the project's .ddx/config.yaml from
// projectRoot, layers overrides on top, and returns the immutable
// ResolvedConfig.
//
// On load error, LoadAndResolve still returns a usable, sealed
// ResolvedConfig populated from package defaults plus the supplied
// overrides, alongside the underlying error. Callers decide whether to
// surface the error or proceed with the defaults-backed config.
//
// If projectRoot is empty, the process working directory is used.
func LoadAndResolve(projectRoot string, overrides CLIOverrides) (ResolvedConfig, error) {
	cfg, err := LoadWithWorkingDir(projectRoot)
	if err != nil {
		return DefaultNewConfig().Resolve(overrides), err
	}
	return cfg.Resolve(overrides), nil
}

// ResolvedConfig is the loop/runner/reviewer's view of merged project
// config plus per-invocation overrides. It is constructed only by
// (*Config).Resolve / config.LoadAndResolve and is safe to share across
// goroutines (no method mutates it).
//
// Sealed-construction: every public accessor calls requireSealed on
// entry. A zero-value ResolvedConfig{} or `var r ResolvedConfig` is a
// valid Go expression but fails loudly on first read with a panic
// naming LoadAndResolve. See SD-024 / TD-024.
type ResolvedConfig struct {
	sealed bool

	assignee                string
	reviewMaxRetries        int
	noProgressCooldown      time.Duration
	maxNoChangesBeforeClose int
	heartbeatInterval       time.Duration
	harness                 string
	model                   string
	provider                string
	modelRef                string
	profile                 string
	minTier                 string
	maxTier                 string
	effort                  string
	permissions             string
	timeout                 time.Duration
	wallClock               time.Duration
	contextBudget           string
	evidenceCaps            evidence.Caps
	sessionLogDir           string
	mirrorConfig            *ExecutionsMirrorConfig
	resolvedLadder          map[string][]string
	reasoningLevels         map[string][]string
}

// requireSealed panics if r was not produced by Resolve / LoadAndResolve.
// Called as the first statement of every public accessor.
func (r ResolvedConfig) requireSealed() {
	if !r.sealed {
		panic("config: ResolvedConfig used without going through " +
			"(*Config).Resolve or config.LoadAndResolve. " +
			"Production callers must obtain a ResolvedConfig from " +
			"LoadAndResolve; tests must use NewTestConfigFor*.")
	}
}

func (r ResolvedConfig) Assignee() string {
	r.requireSealed()
	return r.assignee
}

func (r ResolvedConfig) ReviewMaxRetries() int {
	r.requireSealed()
	return r.reviewMaxRetries
}

func (r ResolvedConfig) NoProgressCooldown() time.Duration {
	r.requireSealed()
	return r.noProgressCooldown
}

func (r ResolvedConfig) MaxNoChangesBeforeClose() int {
	r.requireSealed()
	return r.maxNoChangesBeforeClose
}

func (r ResolvedConfig) HeartbeatInterval() time.Duration {
	r.requireSealed()
	return r.heartbeatInterval
}

func (r ResolvedConfig) Harness() string {
	r.requireSealed()
	return r.harness
}

func (r ResolvedConfig) Model() string {
	r.requireSealed()
	return r.model
}

func (r ResolvedConfig) Provider() string {
	r.requireSealed()
	return r.provider
}

func (r ResolvedConfig) ModelRef() string {
	r.requireSealed()
	return r.modelRef
}

func (r ResolvedConfig) Profile() string {
	r.requireSealed()
	return r.profile
}

func (r ResolvedConfig) MinTier() string {
	r.requireSealed()
	return r.minTier
}

func (r ResolvedConfig) MaxTier() string {
	r.requireSealed()
	return r.maxTier
}

func (r ResolvedConfig) Effort() string {
	r.requireSealed()
	return r.effort
}

func (r ResolvedConfig) Permissions() string {
	r.requireSealed()
	return r.permissions
}

func (r ResolvedConfig) Timeout() time.Duration {
	r.requireSealed()
	return r.timeout
}

func (r ResolvedConfig) WallClock() time.Duration {
	r.requireSealed()
	return r.wallClock
}

func (r ResolvedConfig) ContextBudget() string {
	r.requireSealed()
	return r.contextBudget
}

func (r ResolvedConfig) EvidenceCaps() evidence.Caps {
	r.requireSealed()
	return r.evidenceCaps
}

func (r ResolvedConfig) SessionLogDir() string {
	r.requireSealed()
	return r.sessionLogDir
}

// MirrorConfig returns the executions mirror config snapshot, or nil if
// mirroring is not configured. Callers must treat the returned pointer
// as read-only.
func (r ResolvedConfig) MirrorConfig() *ExecutionsMirrorConfig {
	r.requireSealed()
	return r.mirrorConfig
}

// ResolvedLadder returns a defensive copy of the resolved profile→tier
// ladder map. Mutating the returned map does not affect the receiver.
func (r ResolvedConfig) ResolvedLadder() map[string][]string {
	r.requireSealed()
	return cloneStringSliceMap(r.resolvedLadder)
}

// ReasoningLevels returns a defensive copy of the reasoning-level map.
// Mutating the returned map does not affect the receiver.
func (r ResolvedConfig) ReasoningLevels() map[string][]string {
	r.requireSealed()
	return cloneStringSliceMap(r.reasoningLevels)
}

func cloneStringSliceMap(m map[string][]string) map[string][]string {
	if m == nil {
		return nil
	}
	out := make(map[string][]string, len(m))
	for k, v := range m {
		out[k] = append([]string(nil), v...)
	}
	return out
}
