package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveModelTier(t *testing.T) {
	assert.Equal(t, "gpt-5.4", ResolveModelTier("codex", TierSmart))
	assert.Equal(t, "gpt-5.4-mini", ResolveModelTier("codex", TierFast))
	assert.Equal(t, "claude-opus-4-6", ResolveModelTier("claude", TierSmart))
	assert.Equal(t, "claude-sonnet-4-6", ResolveModelTier("claude", TierFast))
	assert.Equal(t, "qwen/qwen3-coder-next", ResolveModelTier("forge", TierSmart))
	assert.Equal(t, "qwen3.5-27b", ResolveModelTier("forge", TierFast))
	assert.Equal(t, "", ResolveModelTier("unknown", TierSmart))
}

func TestDefaultBenchmarkArms(t *testing.T) {
	arms := DefaultBenchmarkArms()
	assert.Len(t, arms, 6)
	labels := map[string]bool{}
	for _, a := range arms {
		labels[a.Label] = true
		a.ResolveArm()
		assert.NotEmpty(t, a.Model, "arm %s should resolve a model", a.Label)
	}
	assert.True(t, labels["forge-smart"])
	assert.True(t, labels["forge-fast"])
	assert.True(t, labels["codex-smart"])
	assert.True(t, labels["codex-fast"])
	assert.True(t, labels["claude-smart"])
	assert.True(t, labels["claude-fast"])
}

func TestBenchmarkArmsToCompare(t *testing.T) {
	arms := []BenchmarkArm{
		{Label: "a", Harness: "forge", Tier: TierSmart},
		{Label: "b", Harness: "claude", Tier: TierFast},
	}
	opts := BenchmarkArmsToCompare(arms, RunOptions{Prompt: "test"})
	assert.Equal(t, []string{"forge", "claude"}, opts.Harnesses)
	assert.Equal(t, "qwen/qwen3-coder-next", opts.ArmModels[0])
	assert.Equal(t, "claude-sonnet-4-6", opts.ArmModels[1])
	assert.Equal(t, "a", opts.ArmLabels[0])
	assert.Equal(t, "b", opts.ArmLabels[1])
}
