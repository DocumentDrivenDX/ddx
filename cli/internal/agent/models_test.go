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
