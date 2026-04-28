package agent

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
)

func TestResolveModelTier(t *testing.T) {
	assert.Equal(t, "gpt-5.4", ResolveModelTier("codex", escalation.TierSmart))
	assert.Equal(t, "gpt-5.4-mini", ResolveModelTier("codex", escalation.TierCheap))
	assert.Equal(t, "claude-opus-4-6", ResolveModelTier("claude", escalation.TierSmart))
	assert.Equal(t, "claude-haiku-4-5", ResolveModelTier("claude", escalation.TierCheap))
	assert.Equal(t, "minimax/minimax-m2.7", ResolveModelTier("agent", escalation.TierSmart))
	assert.Equal(t, "qwen3.5-27b", ResolveModelTier("agent", escalation.TierCheap))
	assert.Equal(t, "", ResolveModelTier("unknown", escalation.TierSmart))
}

func TestDefaultBenchmarkArms(t *testing.T) {
	arms := DefaultBenchmarkArms()
	assert.Len(t, arms, 9)
	labels := map[string]bool{}
	for _, a := range arms {
		labels[a.Label] = true
		a.ResolveArm()
		assert.NotEmpty(t, a.Model, "arm %s should resolve a model", a.Label)
	}
	assert.True(t, labels["agent-smart"])
	assert.True(t, labels["agent-standard"])
	assert.True(t, labels["agent-cheap"])
	assert.True(t, labels["codex-smart"])
	assert.True(t, labels["codex-standard"])
	assert.True(t, labels["codex-cheap"])
	assert.True(t, labels["claude-smart"])
	assert.True(t, labels["claude-standard"])
	assert.True(t, labels["claude-cheap"])
}

func TestBenchmarkArmsToCompare(t *testing.T) {
	arms := []BenchmarkArm{
		{Label: "a", Harness: "agent", Tier: escalation.TierSmart},
		{Label: "b", Harness: "claude", Tier: escalation.TierCheap},
	}
	runtime := BenchmarkArmsToCompare(arms, AgentRunRuntime{Prompt: "test"})
	assert.Equal(t, []string{"agent", "claude"}, runtime.Harnesses)
	assert.Equal(t, "minimax/minimax-m2.7", runtime.ArmModels[0])
	assert.Equal(t, "claude-haiku-4-5", runtime.ArmModels[1])
	assert.Equal(t, "a", runtime.ArmLabels[0])
	assert.Equal(t, "b", runtime.ArmLabels[1])
	assert.Equal(t, "test", runtime.Prompt)
}
