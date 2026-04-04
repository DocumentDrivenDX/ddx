package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Registry tests ---

func TestRegistryBuiltinHarnesses(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"codex", "claude", "gemini", "opencode", "pi", "cursor"} {
		assert.True(t, r.Has(name), "should have builtin harness: %s", name)
	}
	assert.False(t, r.Has("nonexistent"))
}

func TestRegistryGet(t *testing.T) {
	r := NewRegistry()
	h, ok := r.Get("codex")
	require.True(t, ok)
	assert.Equal(t, "codex", h.Name)
	assert.Equal(t, "codex", h.Binary)
	assert.Equal(t, "arg", h.PromptMode)
	assert.Equal(t, "-m", h.ModelFlag)
	assert.Equal(t, "-C", h.WorkDirFlag)
}

func TestRegistryNamesPreferenceOrder(t *testing.T) {
	r := NewRegistry()
	names := r.Names()
	// First 6 should be in preference order
	require.Len(t, names, 6)
	assert.Equal(t, "codex", names[0])
	assert.Equal(t, "claude", names[1])
	assert.Equal(t, "gemini", names[2])
}

func TestRegistryDiscover(t *testing.T) {
	r := NewRegistry()
	statuses := r.Discover()
	assert.Len(t, statuses, 6)
	for _, s := range statuses {
		assert.NotEmpty(t, s.Name)
		assert.NotEmpty(t, s.Binary)
		// Available depends on system — just check it's a bool
	}
}

// --- Quorum tests ---

func TestEffectiveThreshold(t *testing.T) {
	tests := []struct {
		strategy  string
		threshold int
		total     int
		expected  int
	}{
		{"any", 0, 3, 1},
		{"majority", 0, 3, 2},
		{"majority", 0, 5, 3},
		{"unanimous", 0, 3, 3},
		{"", 2, 3, 2},  // numeric threshold
		{"", 0, 3, 1},  // default
	}

	for _, tt := range tests {
		got := effectiveThreshold(tt.strategy, tt.threshold, tt.total)
		assert.Equal(t, tt.expected, got,
			"strategy=%s threshold=%d total=%d", tt.strategy, tt.threshold, tt.total)
	}
}

func TestQuorumMet(t *testing.T) {
	pass := &Result{ExitCode: 0}
	fail := &Result{ExitCode: 1}

	assert.True(t, QuorumMet("any", 0, []*Result{pass, fail, fail}))
	assert.False(t, QuorumMet("any", 0, []*Result{fail, fail, fail}))

	assert.True(t, QuorumMet("majority", 0, []*Result{pass, pass, fail}))
	assert.False(t, QuorumMet("majority", 0, []*Result{pass, fail, fail}))

	assert.True(t, QuorumMet("unanimous", 0, []*Result{pass, pass, pass}))
	assert.False(t, QuorumMet("unanimous", 0, []*Result{pass, pass, fail}))

	// nil results count as failures
	assert.False(t, QuorumMet("unanimous", 0, []*Result{pass, nil, pass}))
}

// --- Token extraction tests ---

func TestExtractTokensCodex(t *testing.T) {
	output := "some output\ntokens used\n1,234\n"
	assert.Equal(t, 1234, extractTokens(output, "codex"))
}

func TestExtractTokensCodexNoMatch(t *testing.T) {
	assert.Equal(t, 0, extractTokens("no token info here", "codex"))
}

func TestExtractTokensUnknownHarness(t *testing.T) {
	assert.Equal(t, 0, extractTokens("tokens: 500", "gemini"))
}

// --- Harness field tests ---

func TestHarnessFieldsPopulated(t *testing.T) {
	r := NewRegistry()

	codex, _ := r.Get("codex")
	assert.Equal(t, "-m", codex.ModelFlag)
	assert.Equal(t, "-C", codex.WorkDirFlag)
	assert.Equal(t, "-c", codex.EffortFlag)

	claude, _ := r.Get("claude")
	assert.Equal(t, "--model", claude.ModelFlag)
	assert.Equal(t, "--cwd", claude.WorkDirFlag)
	assert.Equal(t, "--effort", claude.EffortFlag)

	// gemini has no flags
	gemini, _ := r.Get("gemini")
	assert.Empty(t, gemini.ModelFlag)
	assert.Empty(t, gemini.WorkDirFlag)
}

// --- Runner defaults ---

func TestNewRunnerDefaults(t *testing.T) {
	r := NewRunner(Config{})
	assert.Equal(t, DefaultHarness, r.Config.Harness)
	assert.Equal(t, DefaultTimeoutMS, r.Config.TimeoutMS)
	assert.Equal(t, DefaultLogDir, r.Config.SessionLogDir)
}

func TestNewRunnerPreservesConfig(t *testing.T) {
	r := NewRunner(Config{
		Harness:       "claude",
		TimeoutMS:     60000,
		SessionLogDir: "/tmp/logs",
	})
	assert.Equal(t, "claude", r.Config.Harness)
	assert.Equal(t, 60000, r.Config.TimeoutMS)
	assert.Equal(t, "/tmp/logs", r.Config.SessionLogDir)
}

func TestRunUnknownHarness(t *testing.T) {
	r := NewRunner(Config{})
	_, err := r.Run(RunOptions{Harness: "nonexistent", Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown harness")
}

func TestRunEmptyPrompt(t *testing.T) {
	r := NewRunner(Config{})
	_, err := r.Run(RunOptions{Harness: "codex"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prompt is required")
}
