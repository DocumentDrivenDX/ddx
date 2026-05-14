package agent

import (
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
)

// BenchmarkArm defines one arm in a benchmark run.
type BenchmarkArm struct {
	Label      string                `json:"label"`
	Harness    string                `json:"harness"`
	PowerClass escalation.PowerClass `json:"power_class"`
	Model      string                `json:"model,omitempty"` // explicit override; empty = no DDx-side model resolution
}

// ResolveArm no longer resolves powerClass to a DDx-side concrete model.
// Benchmark callers must set Model explicitly when they want a pin.
func (a *BenchmarkArm) ResolveArm() {
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
