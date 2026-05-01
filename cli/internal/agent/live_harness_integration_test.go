//go:build live_harness

// Live harness integration tests.
//
// These tests invoke real third-party harness binaries (codex, claude,
// opencode, pi, gemini) and are NOT part of the default DDx test suite.
// Harness execution now lives in Fizeau; DDx only retains these as opt-in
// smoke tests for the DDx→harness boundary.
//
// To run them explicitly:
//
//	go test -tags=live_harness ./internal/agent -run TestIntegration_
//
// Each test additionally skips if the binary or its credentials are not
// available, so even with the build tag enabled the suite remains safe to
// run on hosts that don't have every harness installed.
//
// This is the ONLY file in DDx allowed to exec real harness binaries from
// tests. Unit-level coverage uses mockExecutor + mockLookPath (see
// agent_test.go) and Fizeau-boundary stubs.

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_CodexEcho(t *testing.T) {
	if _, err := DefaultLookPath("codex"); err != nil {
		t.Skip("codex not available")
	}
	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Harness: "codex",
		Timeout: 30 * time.Second,
	}).Resolve(config.CLIOverrides{})
	result, err := r.RunWithConfig(context.Background(), rcfg, AgentRunRuntime{
		Prompt: `print("hello from codex integration test")`,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0, error: %s", result.Error)
	assert.NotEmpty(t, result.Output, "should have output")
}

func TestIntegration_ClaudeEcho(t *testing.T) {
	if _, err := DefaultLookPath("claude"); err != nil {
		t.Skip("claude not available")
	}
	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Harness: "claude",
		Timeout: 60 * time.Second,
	}).Resolve(config.CLIOverrides{})
	result, err := r.RunWithConfig(context.Background(), rcfg, AgentRunRuntime{
		Prompt: "Respond with exactly: INTEGRATION_TEST_OK",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0, error: %s", result.Error)
	assert.NotEmpty(t, result.Output, "should have output")
}

func TestIntegration_OpencodeEcho(t *testing.T) {
	if _, err := DefaultLookPath("opencode"); err != nil {
		t.Skip("opencode not available")
	}
	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Harness: "opencode",
		Timeout: 60 * time.Second,
	}).Resolve(config.CLIOverrides{})
	result, err := r.RunWithConfig(context.Background(), rcfg, AgentRunRuntime{
		Prompt: "Respond with exactly: INTEGRATION_TEST_OK",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0, error: %s", result.Error)
	assert.NotEmpty(t, result.Output, "should have output")
}

func TestIntegration_PiEcho(t *testing.T) {
	if _, err := DefaultLookPath("pi"); err != nil {
		t.Skip("pi not available")
	}
	// Skip if no API key is configured for pi (avoids hanging until timeout).
	piKeys := []string{
		"ANTHROPIC_API_KEY", "ANTHROPIC_OAUTH_TOKEN",
		"OPENAI_API_KEY", "GEMINI_API_KEY",
		"GROQ_API_KEY", "XAI_API_KEY", "OPENROUTER_API_KEY",
	}
	hasKey := false
	for _, k := range piKeys {
		if os.Getenv(k) != "" {
			hasKey = true
			break
		}
	}
	if !hasKey {
		t.Skip("pi API credentials not configured")
	}
	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Harness: "pi",
		Timeout: 60 * time.Second,
	}).Resolve(config.CLIOverrides{})
	result, err := r.RunWithConfig(context.Background(), rcfg, AgentRunRuntime{
		Prompt: "Respond with exactly: INTEGRATION_TEST_OK",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0, error: %s", result.Error)
	assert.NotEmpty(t, result.Output, "should have output")
}

func TestIntegration_GeminiEcho(t *testing.T) {
	if _, err := DefaultLookPath("gemini"); err != nil {
		t.Skip("gemini not available")
	}
	// Skip if gemini credentials are not configured (avoids hanging until timeout).
	// gemini CLI stores credentials in ~/.gemini/ or uses GEMINI_API_KEY.
	if os.Getenv("GEMINI_API_KEY") == "" {
		homeDir, _ := os.UserHomeDir()
		credPath := filepath.Join(homeDir, ".gemini", "credentials.json")
		if _, err := os.Stat(credPath); os.IsNotExist(err) {
			t.Skip("gemini credentials not configured (set GEMINI_API_KEY or provide ~/.gemini/credentials.json)")
		}
	}
	// Gemini has slow initialization (skill loading), so use a longer timeout
	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Harness: "gemini",
		Timeout: 180 * time.Second,
	}).Resolve(config.CLIOverrides{})
	result, err := r.RunWithConfig(context.Background(), rcfg, AgentRunRuntime{
		Prompt: "Respond with exactly: INTEGRATION_TEST_OK",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0, error: %s", result.Error)
	assert.NotEmpty(t, result.Output, "should have output")
}
