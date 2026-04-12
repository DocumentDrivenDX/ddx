package agent

// ModelTier represents a quality/cost tier for model selection.
type ModelTier string

const (
	TierSmart    ModelTier = "smart"    // top-tier foundation models; hard/broad tasks, interactive sessions, HELIX alignment
	TierStandard ModelTier = "standard" // default for most builds; strong capability at reasonable cost
	TierCheap    ModelTier = "cheap"    // mechanical tasks: extraction, formatting, simple transforms; minimize cost
)

// DefaultModelTiers maps harness → tier → model.
// These are the current production defaults as of 2026-04.
var DefaultModelTiers = map[string]map[ModelTier]string{
	"codex": {
		TierSmart:    "gpt-5.4",
		TierStandard: "gpt-5.4", // codex routes internally; gpt-5.4 is the available smart/standard model
		TierCheap:    "gpt-5.4-mini",
	},
	"claude": {
		TierSmart:    "claude-opus-4-6",
		TierStandard: "claude-sonnet-4-6",
		TierCheap:    "claude-haiku-4-5",
	},
	"agent": {
		TierSmart:    "minimax/minimax-m2.7",
		TierStandard: "minimax/minimax-m2.7",
		TierCheap:    "qwen3.5-27b",
	},
	"opencode": {
		TierSmart:    "anthropic/claude-opus-4-6",
		TierStandard: "anthropic/claude-sonnet-4-6",
		TierCheap:    "anthropic/claude-haiku-4-5",
	},
}

// ResolveModelTier returns the model for a given harness and tier.
// Falls back to the harness DefaultModel if the tier is unknown.
func ResolveModelTier(harness string, tier ModelTier) string {
	if tiers, ok := DefaultModelTiers[harness]; ok {
		if model, ok := tiers[tier]; ok {
			return model
		}
	}
	return ""
}

// BenchmarkArm defines one arm in a benchmark run.
type BenchmarkArm struct {
	Label   string    `json:"label"`
	Harness string    `json:"harness"`
	Tier    ModelTier `json:"tier"`
	Model   string    `json:"model,omitempty"` // explicit override; empty = resolve from tier
}

// DefaultBenchmarkArms returns the standard set of arms for a full comparison.
func DefaultBenchmarkArms() []BenchmarkArm {
	return []BenchmarkArm{
		{Label: "agent-smart", Harness: "agent", Tier: TierSmart},
		{Label: "agent-standard", Harness: "agent", Tier: TierStandard},
		{Label: "agent-cheap", Harness: "agent", Tier: TierCheap},
		{Label: "codex-smart", Harness: "codex", Tier: TierSmart},
		{Label: "codex-standard", Harness: "codex", Tier: TierStandard},
		{Label: "codex-cheap", Harness: "codex", Tier: TierCheap},
		{Label: "claude-smart", Harness: "claude", Tier: TierSmart},
		{Label: "claude-standard", Harness: "claude", Tier: TierStandard},
		{Label: "claude-cheap", Harness: "claude", Tier: TierCheap},
	}
}

// ResolveArm fills in the model from the tier if not explicitly set.
func (a *BenchmarkArm) ResolveArm() {
	if a.Model == "" {
		a.Model = ResolveModelTier(a.Harness, a.Tier)
	}
}

// BenchmarkArmsToCompare converts a slice of BenchmarkArms into CompareOptions fields.
func BenchmarkArmsToCompare(arms []BenchmarkArm, baseOpts RunOptions) CompareOptions {
	harnesses := make([]string, len(arms))
	armModels := make(map[int]string, len(arms))
	armLabels := make(map[int]string, len(arms))

	for i, arm := range arms {
		arm.ResolveArm()
		harnesses[i] = arm.Harness
		if arm.Model != "" {
			armModels[i] = arm.Model
		}
		armLabels[i] = arm.Label
	}

	return CompareOptions{
		RunOptions: baseOpts,
		Harnesses:  harnesses,
		ArmModels:  armModels,
		ArmLabels:  armLabels,
	}
}
