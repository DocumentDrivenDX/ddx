//go:build omlx_smoke

// Opt-in live smoke test for the ddx work → ddx-agent → vidar-omlx consumer
// path. Unlike the fixture-replay test in omlx_e2e_test.go (which ships in CI
// and uses a recorded test double), this test hits a real omlx endpoint and is
// intended for manual release verification against a live server. It is gated
// by the `omlx_smoke` build tag so the default `go test ./...` never runs it.
//
// Run:
//
//	OMLX_SMOKE_URL=http://vidar:1235/v1\
//	OMLX_SMOKE_MODEL=Qwen3.6-35B-A3B-4bit\
//	go test -tags omlx_smoke -run TestOMLXLiveSmoke -v ./cli/internal/agent
//
// This covers the same happy-path shape the CI fixture does, but against a
// real server — useful for catching provider-side regressions (the omlx
// server ships a breaking change, HTTP stack differences, headers, etc.) that
// a recorded fixture cannot detect.

package agent

import (
	"os"
	"testing"

	"github.com/DocumentDrivenDX/agent/provider/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOMLXLiveSmoke(t *testing.T) {
	baseURL := os.Getenv("OMLX_SMOKE_URL")
	if baseURL == "" {
		t.Skip("OMLX_SMOKE_URL not set — live smoke requires an omlx endpoint")
	}
	model := os.Getenv("OMLX_SMOKE_MODEL")
	if model == "" {
		t.Fatal("OMLX_SMOKE_MODEL is required when OMLX_SMOKE_URL is set")
	}

	provider := openai.New(openai.Config{
		BaseURL: baseURL,
		APIKey:  os.Getenv("OMLX_SMOKE_API_KEY"),
		Model:   model,
		Flavor:  "omlx",
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir(), TimeoutMS: 120000})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "Reply with the single word 'ready' and nothing else.",
		WorkDir: t.TempDir(),
	})
	require.NoError(t, err)

	// The regression signature is the precise error v0.3.13 surfaced when
	// omlx emitted keep-alive SSE comment frames during reasoning warmup.
	// Any bump that reintroduces it must be caught here before it ships.
	assert.NotContains(t, result.Error, "unexpected end of JSON input",
		"live omlx produced the v0.3.13 SSE comment-frame regression signature")
	assert.Equal(t, 0, result.ExitCode, "live omlx smoke must close cleanly; error=%q", result.Error)
	assert.NotEmpty(t, result.Output, "live omlx must return some content")
}
