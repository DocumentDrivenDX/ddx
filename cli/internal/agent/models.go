package agent

// ModelTier represents a quality/cost tier for model selection.
type ModelTier string

const (
	TierSmart    ModelTier = "smart"    // top-tier foundation models; hard/broad tasks, interactive sessions, HELIX alignment
	TierStandard ModelTier = "standard" // default for most builds; strong capability at reasonable cost
	TierCheap    ModelTier = "cheap"    // mechanical tasks: extraction, formatting, simple transforms; minimize cost
)

// harnessToSurface maps harness names to their catalog surface identifier.
// This is the only place harness→surface is declared; all model resolution
// goes through BuiltinCatalog using the surface name.
var harnessToSurface = map[string]string{
	"codex":    "codex",
	"claude":   "claude",
	"agent":    "embedded-openai",
	"opencode": "claude",
}

// ResolveModelTier returns the concrete model for a given harness and tier
// by looking up the tier profile in BuiltinCatalog for the harness's surface.
func ResolveModelTier(harness string, tier ModelTier) string {
	surface, ok := harnessToSurface[harness]
	if !ok {
		return ""
	}
	model, _ := BuiltinCatalog.Resolve(string(tier), surface)
	return model
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
