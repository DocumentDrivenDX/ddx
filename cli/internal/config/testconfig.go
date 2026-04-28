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

// TestRunConfigOpts names every durable knob the agent run path reads
// via ResolvedConfig (per SD-024 §RunArgs → AgentRunRuntime field
// classification). Tests must specify each field explicitly; there are
// no zero-value defaults that silently bypass real config.
//
// See SD-024 / TD-024 §Test config constructors and §Stage 2 bead 17.
type TestRunConfigOpts struct {
	Harness       string
	Model         string
	Provider      string
	ModelRef      string
	Effort        string
	Permissions   string
	Timeout       time.Duration
	WallClock     time.Duration
	SessionLogDir string
}

// NewTestConfigForRun returns a *Config that, when Resolve()d with the
// matching CLIOverrides, produces a ResolvedConfig whose run-path
// accessors return the values supplied in opts. Pure CLI-override
// fields (Provider, ModelRef, Effort) have no durable home on
// AgentConfig and must be applied at Resolve time via CLIOverrides.
func NewTestConfigForRun(opts TestRunConfigOpts) *Config {
	return &Config{
		Version: "1.0",
		Agent: &AgentConfig{
			Harness:       opts.Harness,
			Model:         opts.Model,
			TimeoutMS:     int(opts.Timeout / time.Millisecond),
			SessionLogDir: opts.SessionLogDir,
			Permissions:   opts.Permissions,
		},
	}
}

// TestBeadConfigOpts names every durable knob the execute-bead worker
// reads via ResolvedConfig (per SD-024 §ExecuteBeadOptions →
// ExecuteBeadRuntime field classification). Tests must specify each
// field explicitly; there are no zero-value defaults that silently
// bypass real config.
//
// See SD-024 / TD-024 §Test config constructors and §Stage 3.
type TestBeadConfigOpts struct {
	Harness  string
	Model    string
	Provider string // CLI-only override, no durable home on AgentConfig
	ModelRef string // CLI-only override, no durable home on AgentConfig
	Effort   string // CLI-only override, no durable home on AgentConfig
	Mirror   *ExecutionsMirrorConfig
}

// NewTestConfigForBead returns a *Config that, when Resolve()d with
// the matching CLIOverrides, produces a ResolvedConfig whose
// execute-bead-relevant accessors return the values supplied in opts.
// Pure CLI-override fields (Provider, ModelRef, Effort) have no
// durable home on AgentConfig and must be applied at Resolve time via
// CLIOverrides.
func NewTestConfigForBead(opts TestBeadConfigOpts) *Config {
	cfg := &Config{
		Version: "1.0",
		Agent: &AgentConfig{
			Harness: opts.Harness,
			Model:   opts.Model,
		},
	}
	if opts.Mirror != nil {
		cfg.Executions = &ExecutionsConfig{Mirror: opts.Mirror}
	}
	return cfg
}

// TestBeadOverrides returns the CLIOverrides that, combined with the
// *Config produced by NewTestConfigForBead(opts), drive a Resolve call
// to a ResolvedConfig matching opts. Pure-override fields (Provider,
// ModelRef, Effort) have no durable home on *Config; they are applied
// at Resolve time only.
func TestBeadOverrides(opts TestBeadConfigOpts) CLIOverrides {
	return CLIOverrides{
		Provider: opts.Provider,
		ModelRef: opts.ModelRef,
		Effort:   opts.Effort,
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
