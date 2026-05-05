package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewSessionPromptAssemblyPinnedRolling(t *testing.T) {
	session := ReviewSession{
		ID:             "review-123",
		ArtifactID:     "artifact-1",
		ArtifactSHA:    "sha-abc123",
		ArtifactGitRev: "gitrev-def456",
		SystemRubric:   "apply the review rubric carefully",
		TemplateRef:    "template://review/story-17",
		PromptRef:      "prompt://review/story-17",
		Status:         "open",
		Turns: []ReviewTurn{
			{Actor: "user", Content: "first user intent"},
			{Actor: "assistant", Content: "ack"},
			{Actor: "user", Content: "explicit decision one"},
		},
	}

	res, err := RenderReviewPrompt(ReviewPromptRenderInput{
		Session:              session,
		SessionMemorySummary: "session memory summary",
		LastVerbatimTurns:    2,
		UnresolvedFindings:   []ReviewPromptFinding{{Location: "file.go:12", Summary: "follow up"}},
		MaxPromptBytes:       1 << 20,
	})
	require.NoError(t, err)
	require.False(t, res.Overflow)
	require.Len(t, res.Sections, 3)
	assert.Equal(t, "pinned", res.Sections[0].Name)
	assert.Equal(t, "rolling", res.Sections[1].Name)
	assert.Equal(t, "unresolved-findings", res.Sections[2].Name)

	pinnedIdx := strings.Index(res.Prompt, "<pinned>")
	rollingIdx := strings.Index(res.Prompt, "<rolling>")
	unresolvedIdx := strings.Index(res.Prompt, "<unresolved-findings>")
	require.GreaterOrEqual(t, pinnedIdx, 0)
	require.GreaterOrEqual(t, rollingIdx, 0)
	require.GreaterOrEqual(t, unresolvedIdx, 0)
	assert.Less(t, pinnedIdx, rollingIdx)
	assert.Less(t, rollingIdx, unresolvedIdx)
	assert.Contains(t, res.Prompt, "<first-user-intent>")
	assert.Contains(t, res.Prompt, "first user intent")
	assert.Contains(t, res.Prompt, "<decision>explicit decision one</decision>")
	assert.Contains(t, res.Prompt, "<session-memory-summary>")
	assert.Contains(t, res.Prompt, "session memory summary")
	assert.Contains(t, res.Prompt, "<turn actor=\"user\">")
	assert.Contains(t, res.Prompt, "follow up")
}

func TestReviewSessionPromptAssemblyBudgetRefusal(t *testing.T) {
	session := ReviewSession{
		ID:             "review-oversize",
		ArtifactID:     strings.Repeat("a", 1024),
		ArtifactSHA:    strings.Repeat("b", 1024),
		ArtifactGitRev: strings.Repeat("c", 1024),
		SystemRubric:   strings.Repeat("r", 4096),
		TemplateRef:    "template://review/story-17",
		PromptRef:      "prompt://review/story-17",
		Status:         "open",
	}

	_, err := RenderReviewPrompt(ReviewPromptRenderInput{
		Session:        session,
		MaxPromptBytes: 256,
	})
	require.Error(t, err)
	var budgetErr *PromptBudgetExceededError
	require.ErrorAs(t, err, &budgetErr)
	assert.Contains(t, err.Error(), "PROMPT_BUDGET_EXCEEDED")
	assert.Greater(t, budgetErr.ObservedBytes, budgetErr.CapBytes)
}
