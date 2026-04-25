package exec

// store_oversize_prompt_env_test.go covers FEAT-022 §9 / Stage D2: an exec
// definition whose DDX_AGENT_PROMPT env var exceeds the configured cap
// must hard-fail at definition load with an actionable error naming the
// definition, the env var, observed size, and cap.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecStoreOversizePromptEnv(t *testing.T) {
	const testCap = 1024
	prev := execPromptCapBytes
	execPromptCapBytes = testCap
	t.Cleanup(func() { execPromptCapBytes = prev })

	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-OVR")

	huge := strings.Repeat("x", testCap*2)
	writeExecDefinition(t, wd, Definition{
		ID:          "exec-oversize-prompt@1",
		ArtifactIDs: []string{"MET-OVR"},
		Executor: ExecutorSpec{
			Kind: ExecutorKindAgent,
			Env: map[string]string{
				"DDX_AGENT_PROMPT":  huge,
				"DDX_AGENT_HARNESS": "claude",
			},
		},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-25T09:14:41Z"),
	})

	store := NewStore(wd)
	_, err := store.ListDefinitions("")
	require.Error(t, err)
	msg := err.Error()
	for _, want := range []string{
		"exec-oversize-prompt@1",
		"DDX_AGENT_PROMPT",
		"2048",
		"1024",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing required substring %q", msg, want)
		}
	}
}
