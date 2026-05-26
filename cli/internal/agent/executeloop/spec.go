// Package executeloop defines the canonical spec type for work workers.
// It is import-safe: no dependency on cobra, server, or transport packages.
package executeloop

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/escalation"
)

// SpecCurrentVersion is the single spec version supported by this client and
// server. Forward-compat stance: a client MUST set SpecVersion = SpecCurrentVersion;
// a server receiving an unknown version SHOULD reject with an error rather than
// silently misinterpreting fields. There is no backwards-compat shim — upgrade
// client and server together.
const SpecCurrentVersion = 1

// Mode controls how the work terminates.
type Mode string

const (
	// ModeOnce runs one bead (or none if the queue is empty) then exits.
	ModeOnce Mode = "once"
	// ModeDrain processes all ready beads then exits.
	ModeDrain Mode = "drain"
	// ModeWatch processes all ready beads, then sleeps for IdleInterval and
	// rechecks, looping indefinitely until interrupted.
	ModeWatch Mode = "watch"
)

// Duration wraps time.Duration with JSON support that marshals as a string
// (e.g., "30s") and unmarshals both strings ("30s") and numeric nanoseconds.
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	// Try string first.
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("executeloop: invalid duration string %q: %w", s, err)
		}
		d.Duration = parsed
		return nil
	}
	// Try numeric nanoseconds.
	var ns int64
	if err := json.Unmarshal(b, &ns); err == nil {
		d.Duration = time.Duration(ns)
		return nil
	}
	return fmt.Errorf("executeloop: cannot unmarshal %s as duration (want string or nanosecond integer)", strconv.Quote(string(b)))
}

// ExecuteLoopSpec is the canonical, transport-agnostic specification for an
// work worker. All transports (cobra, REST, GraphQL) map into this type
// before persisting or dispatching.
type ExecuteLoopSpec struct {
	ProjectRoot string `json:"project_root,omitempty"`
	Harness     string `json:"harness,omitempty"`
	Model       string `json:"model,omitempty"`
	Profile     string `json:"profile,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Effort      string `json:"effort,omitempty"`
	LabelFilter string `json:"label_filter,omitempty"`
	// AttemptBackend selects the execute-bead workspace/transport backend for
	// each attempt. Empty uses executions.attempt_backend or the binary default.
	AttemptBackend string `json:"attempt_backend,omitempty"`

	// Mode controls loop termination. Defaults to ModeDrain via ApplyDefaults.
	Mode Mode `json:"mode,omitempty"`

	// IdleInterval is the sleep duration between queue polls when Mode=watch
	// and no execution-ready work is found.
	IdleInterval Duration `json:"idle_interval,omitempty"`

	NoReview               bool   `json:"no_review,omitempty"`
	ReviewHarness          string `json:"review_harness,omitempty"`
	ReviewModel            string `json:"review_model,omitempty"`
	IgnoreCooldown         bool   `json:"ignore_cooldown,omitempty"`
	CooldownOverrideReason string `json:"cooldown_override_reason,omitempty"`

	// OpaquePassthrough skips DDx-side route validation and config
	// harness/model injection (CONTRACT-003 / FEAT-010).
	OpaquePassthrough bool `json:"opaque_passthrough,omitempty"`

	MaxCostUSD         float64  `json:"max_cost_usd,omitempty"`
	MaxBeadCostUSD     float64  `json:"max_bead_cost_usd,omitempty"`
	MaxRecoveryCostUSD float64  `json:"max_recovery_cost_usd,omitempty"`
	PreClaimTimeout    Duration `json:"preclaim_timeout,omitempty"`
	RequestTimeout     Duration `json:"request_timeout,omitempty"`
	RateLimitMaxWait   Duration `json:"rate_limit_max_wait,omitempty"`
	// RouteResolutionTimeout bounds routing preflight and the resolveRoute
	// viability check so a hung resolver cannot wedge the worker. Zero uses the
	// binary default (agent.DefaultRouteResolutionTimeout, 60s).
	RouteResolutionTimeout Duration `json:"route_resolution_timeout,omitempty"`
	MinPower               int      `json:"min_power,omitempty"`
	MaxPower               int      `json:"max_power,omitempty"`

	// FromRev, if set, narrows execution to beads introduced after this git revision.
	FromRev string `json:"from_rev,omitempty"`

	// SpecVersion must equal SpecCurrentVersion. See SpecCurrentVersion for the
	// client/server forward-compat stance.
	SpecVersion int `json:"spec_version,omitempty"`
}

// DispatchOptions carries control-plane fields used at dispatch time. It is not
// persisted into the worker record.
type DispatchOptions struct {
	// Local, when true, runs the worker in-process rather than via the server API.
	Local bool `json:"local,omitempty"`
	// JSON is the raw JSON payload to forward when dispatching via REST/GraphQL.
	// Callers that build the request body manually may set this instead of
	// populating an ExecuteLoopSpec.
	JSON string `json:"json,omitempty"`
}

// ApplyDefaults fills in zero-value fields with their canonical defaults.
// Safe to call on a zero-value spec.
func (s *ExecuteLoopSpec) ApplyDefaults() {
	if s.Mode == "" {
		s.Mode = ModeDrain
	}
	if s.Mode == ModeWatch && s.IdleInterval.Duration == 0 {
		s.IdleInterval = Duration{30 * time.Second}
	}
	if s.SpecVersion == 0 {
		s.SpecVersion = SpecCurrentVersion
	}
	if s.MaxRecoveryCostUSD == 0 {
		s.MaxRecoveryCostUSD = escalation.DefaultMaxRecoveryCostUSD
	}
}

// Validate checks that the spec is well-formed after ApplyDefaults has run.
func (s *ExecuteLoopSpec) Validate() error {
	switch s.Mode {
	case ModeOnce, ModeDrain, ModeWatch:
	default:
		return fmt.Errorf("executeloop: unknown mode %q (want once, drain, or watch)", s.Mode)
	}
	if s.Mode != ModeWatch && s.IdleInterval.Duration != 0 {
		return fmt.Errorf("executeloop: idle_interval is only valid when mode=watch")
	}
	if s.SpecVersion != 0 && s.SpecVersion != SpecCurrentVersion {
		return fmt.Errorf("executeloop: unsupported spec_version %d (want %d)", s.SpecVersion, SpecCurrentVersion)
	}
	return nil
}
