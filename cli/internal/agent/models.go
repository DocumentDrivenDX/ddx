package agent

import (
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
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
func ResolveModelTier(harness string, tier escalation.ModelTier) string {
	surface, ok := harnessToSurface[harness]
	if !ok {
		return ""
	}
	model, _ := BuiltinCatalog.Resolve(string(tier), surface)
	return model
}

// BenchmarkArm defines one arm in a benchmark run.
type BenchmarkArm struct {
	Label   string               `json:"label"`
	Harness string               `json:"harness"`
	Tier    escalation.ModelTier `json:"tier"`
	Model   string               `json:"model,omitempty"` // explicit override; empty = resolve from tier
}

// ResolveArm fills in the model from the tier if not explicitly set.
func (a *BenchmarkArm) ResolveArm() {
	if a.Model == "" {
		a.Model = ResolveModelTier(a.Harness, a.Tier)
	}
}

// BenchmarkArmsToCompare converts a slice of BenchmarkArms into a
// CompareRuntime carrying per-arm harness/model/label data plus the
// caller-provided base AgentRunRuntime (Prompt, PromptFile, WorkDir, ...).
func BenchmarkArmsToCompare(arms []BenchmarkArm, base AgentRunRuntime) CompareRuntime {
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

	return CompareRuntime{
		AgentRunRuntime: base,
		Harnesses:       harnesses,
		ArmModels:       armModels,
		ArmLabels:       armLabels,
	}
}
