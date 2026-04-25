package config

import "time"

// TestLoopConfigOpts names every durable knob the execute-bead loop
// reads via ResolvedConfig. Tests must specify each field explicitly;
// there are no zero-value defaults that silently bypass real config.
//
// See SD-024 / TD-024 §Test config constructors.
type TestLoopConfigOpts struct {
	Assignee                string
	ReviewMaxRetries        int
	NoProgressCooldown      time.Duration
	MaxNoChangesBeforeClose int
	HeartbeatInterval       time.Duration
	Harness                 string
	Model                   string
	Profile                 string
	MinTier                 string
	MaxTier                 string
	EvidenceCaps            EvidenceCapsConfig
}

// NewTestConfigForLoop returns a *Config that, when Resolve()d with the
// matching CLIOverrides, produces a ResolvedConfig whose loop-relevant
// accessors return exactly the values supplied in opts.
//
// Tests must obtain ResolvedConfig via Resolve on the returned *Config
// — there is no shortcut to construct a ResolvedConfig directly. Pure
// CLI-override fields (Assignee, Profile, MinTier, MaxTier) are passed
// through CLIOverrides at the call site; the constructor stores every
// other field on *Config so it round-trips through Resolve.
func NewTestConfigForLoop(opts TestLoopConfigOpts) *Config {
	reviewMaxRetries := opts.ReviewMaxRetries
	maxNoChanges := opts.MaxNoChangesBeforeClose

	caps := opts.EvidenceCaps

	return &Config{
		Version: "1.0",
		Agent: &AgentConfig{
			Harness: opts.Harness,
			Model:   opts.Model,
		},
		Workers: &WorkersConfig{
			NoProgressCooldown:      opts.NoProgressCooldown.String(),
			MaxNoChangesBeforeClose: &maxNoChanges,
			HeartbeatInterval:       opts.HeartbeatInterval.String(),
		},
		ReviewMaxRetries: &reviewMaxRetries,
		EvidenceCaps:     &caps,
	}
}

// TestLoopOverrides returns the CLIOverrides that, combined with the
// *Config produced by NewTestConfigForLoop(opts), drive a Resolve call
// to a ResolvedConfig matching opts. Pure-override fields (Assignee,
// Profile, MinTier, MaxTier) have no durable home on *Config; they are
// applied at Resolve time only.
func TestLoopOverrides(opts TestLoopConfigOpts) CLIOverrides {
	return CLIOverrides{
		Assignee: opts.Assignee,
		Profile:  opts.Profile,
		MinTier:  opts.MinTier,
		MaxTier:  opts.MaxTier,
	}
}
