package escalation

// ModelTier represents a quality/cost tier for model selection.
type ModelTier string

const (
	TierSmart    ModelTier = "smart"    // top-tier foundation models; hard/broad tasks, interactive sessions, HELIX alignment
	TierStandard ModelTier = "standard" // default for most builds; strong capability at reasonable cost
	TierCheap    ModelTier = "cheap"    // mechanical tasks: extraction, formatting, simple transforms; minimize cost
)
