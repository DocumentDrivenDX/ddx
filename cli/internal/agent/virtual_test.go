package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptHash(t *testing.T) {
	h1 := PromptHash("hello world")
	h2 := PromptHash("hello world")
	h3 := PromptHash("different prompt")

	assert.Equal(t, h1, h2, "same prompt should produce same hash")
	assert.NotEqual(t, h1, h3, "different prompts should produce different hashes")
	assert.Len(t, h1, 16, "hash should be 16 hex characters")
}

func TestRecordAndLookup(t *testing.T) {
	dir := t.TempDir()

	entry := &VirtualEntry{
		Prompt:       "Create a hello world program",
		Response:     "Here is a hello world program...",
		Harness:      "claude",
		Model:        "claude-sonnet-4-20250514",
		DelayMS:      2000,
		InputTokens:  100,
		OutputTokens: 50,
	}

	err := RecordEntry(dir, entry)
	require.NoError(t, err)

	// Verify file was created with hash-based name.
	hash := PromptHash("Create a hello world program")
	path := filepath.Join(dir, hash+".json")
	assert.FileExists(t, path)

	// Lookup should find it.
	found, err := LookupEntry(dir, "Create a hello world program")
	require.NoError(t, err)
	assert.Equal(t, "Here is a hello world program...", found.Response)
	assert.Equal(t, "claude", found.Harness)
	assert.Equal(t, 2000, found.DelayMS)
	assert.Equal(t, 100, found.InputTokens)

	// Lookup with different prompt should fail.
	_, err = LookupEntry(dir, "different prompt")
	assert.Error(t, err)
}

func TestRunVirtual(t *testing.T) {
	dir := t.TempDir()
	dictDir := filepath.Join(dir, ".ddx", "agent-dictionary")

	// Record a response.
	entry := &VirtualEntry{
		Prompt:       "test prompt",
		Response:     "test response output",
		Harness:      "claude",
		DelayMS:      0, // no delay for tests
		InputTokens:  50,
		OutputTokens: 25,
	}
	require.NoError(t, RecordEntry(dictDir, entry))

	// Create runner with virtual harness.
	logDir := filepath.Join(dir, ".ddx", "agent-logs")
	require.NoError(t, os.MkdirAll(logDir, 0755))

	runner := NewRunner(Config{
		Harness:       "virtual",
		SessionLogDir: logDir,
	})

	// Override dictionary lookup path by ensuring VirtualDictionaryDir is checked.
	// For the test, we need the runner to find our dict dir.
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origWd) }()

	result, err := runner.RunVirtual(RunOptions{
		Harness: "virtual",
		Prompt:  "test prompt",
	})
	require.NoError(t, err)
	assert.Equal(t, "test response output", result.Output)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, 50, result.InputTokens)
	assert.Equal(t, 25, result.OutputTokens)
	assert.Equal(t, "virtual", result.Harness)
}
