package config

import (
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

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
