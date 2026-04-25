// Package evidence provides shared primitives for assembling LLM prompts
// from bounded evidence sources. Every prompt-building call site in DDx
// must use these primitives so that no prompt has unbounded size.
//
// FEAT-022 §1 / §1a. Stage A1.
package evidence

// Built-in default caps. These are intentionally permissive on land
// (FEAT-022 Non-Functional / Backward compatibility) so existing
// behavior does not change. Stage F telemetry informs later tightening.
const (
	DefaultMaxPromptBytes       = 4 * 1024 * 1024 // 4 MiB
	DefaultMaxInlinedFileBytes  = 512 * 1024      // 512 KiB
	DefaultMaxDiffBytes         = 2 * 1024 * 1024 // 2 MiB
	DefaultMaxGoverningDocBytes = 256 * 1024      // 256 KiB
)

// Caps is the resolved byte-cap set used by an evidence-assembly call site.
// All caps are byte-based (FEAT-022 Non-Functional / Byte-based enforcement).
type Caps struct {
	MaxPromptBytes       int
	MaxInlinedFileBytes  int
	MaxDiffBytes         int
	MaxGoverningDocBytes int
}

// DefaultCaps returns the built-in conservative defaults.
func DefaultCaps() Caps {
	return Caps{
		MaxPromptBytes:       DefaultMaxPromptBytes,
		MaxInlinedFileBytes:  DefaultMaxInlinedFileBytes,
		MaxDiffBytes:         DefaultMaxDiffBytes,
		MaxGoverningDocBytes: DefaultMaxGoverningDocBytes,
	}
}

// CapsOverride is the partial-override shape that .ddx/config.yaml
// expresses. A nil pointer means "fall through to the next layer".
type CapsOverride struct {
	MaxPromptBytes       *int
	MaxInlinedFileBytes  *int
	MaxDiffBytes         *int
	MaxGoverningDocBytes *int
}

// Apply layers an override onto a Caps value, returning the resolved Caps.
// Nil fields in the override leave the corresponding cap unchanged.
func (c Caps) Apply(o CapsOverride) Caps {
	if o.MaxPromptBytes != nil {
		c.MaxPromptBytes = *o.MaxPromptBytes
	}
	if o.MaxInlinedFileBytes != nil {
		c.MaxInlinedFileBytes = *o.MaxInlinedFileBytes
	}
	if o.MaxDiffBytes != nil {
		c.MaxDiffBytes = *o.MaxDiffBytes
	}
	if o.MaxGoverningDocBytes != nil {
		c.MaxGoverningDocBytes = *o.MaxGoverningDocBytes
	}
	return c
}

// ResolveCaps resolves the effective caps for an invocation given a
// project-wide override and an optional per-harness override. Precedence
// (low → high): defaults < project-wide < per-harness.
//
// harness == "" or absence in perHarness means no per-harness layer.
// A missing project override is expressed as a zero-value CapsOverride.
func ResolveCaps(project CapsOverride, perHarness map[string]CapsOverride, harness string) Caps {
	caps := DefaultCaps().Apply(project)
	if harness != "" && perHarness != nil {
		if h, ok := perHarness[harness]; ok {
			caps = caps.Apply(h)
		}
	}
	return caps
}
