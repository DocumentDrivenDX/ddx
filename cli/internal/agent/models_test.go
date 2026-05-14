package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBenchmarkArmsToCompare(t *testing.T) {
	arms := []BenchmarkArm{
		{Label: "a", Harness: "agent", PowerClass: "smart"},
		{Label: "b", Harness: "claude", PowerClass: "cheap", Model: "claude-haiku-4-5"},
	}
	runtime := BenchmarkArmsToCompare(arms, AgentRunRuntime{Prompt: "test"})
	assert.Equal(t, []string{"agent", "claude"}, runtime.Harnesses)
	assert.Empty(t, runtime.ArmModels[0])
	assert.Equal(t, "claude-haiku-4-5", runtime.ArmModels[1])
	assert.Equal(t, "a", runtime.ArmLabels[0])
	assert.Equal(t, "b", runtime.ArmLabels[1])
	assert.Equal(t, "test", runtime.Prompt)
}
