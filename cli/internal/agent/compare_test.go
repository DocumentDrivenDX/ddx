package agent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/agent/provider/virtual"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRepo creates a minimal git repo in a temp dir and returns its path.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "seed")
	return dir
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// Strip git hook env vars (GIT_INDEX_FILE, GIT_DIR, etc.) that leak from the
	// parent process when tests run inside a git pre-commit hook.
	cmd.Env = append(cleanGitEnv(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, string(out))
}

// C-01: RunCompare creates a ComparisonRecord with one arm per harness.
func TestCompareBasic(t *testing.T) {
	repo := setupTestRepo(t)

	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "/./",
			Response: agentlib.Response{
				Content: "done",
				Model:   "test-model",
				Usage:   agentlib.TokenUsage{Input: 100, Output: 20, Total: 120},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	record, err := r.RunCompare(CompareOptions{
		RunOptions: RunOptions{Prompt: "do something", WorkDir: repo},
		Harnesses:  []string{"agent", "virtual"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, record.ID)
	assert.Equal(t, "do something", record.Prompt)
	assert.Len(t, record.Arms, 2)

	// Verify each arm is labeled with its harness
	harnesses := map[string]bool{}
	for _, arm := range record.Arms {
		harnesses[arm.Harness] = true
	}
	assert.True(t, harnesses["agent"])
	assert.True(t, harnesses["virtual"])
}

// C-02: Arms in sandbox mode are isolated — changes in one don't appear in another.
func TestCompareSandboxIsolation(t *testing.T) {
	repo := setupTestRepo(t)

	// Agent provider writes a file; virtual provider doesn't.
	agentProvider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "/./",
			Response: agentlib.Response{
				Content: "wrote a file",
				Model:   "test-model",
				Usage:   agentlib.TokenUsage{Input: 50, Output: 10, Total: 60},
				ToolCalls: []agentlib.ToolCall{{
					ID:        "tc-1",
					Name:      "write",
					Arguments: json.RawMessage(`{"path":"new_file.txt","content":"from agent"}`),
				}},
			},
		}, {
			// After tool execution, return final response
			PromptMatch: "/./",
			Response: agentlib.Response{
				Content: "wrote new_file.txt",
				Model:   "test-model",
				Usage:   agentlib.TokenUsage{Input: 80, Output: 15, Total: 95},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = agentProvider

	// Set up inline virtual responses
	t.Setenv("DDX_VIRTUAL_RESPONSES", `[{"prompt_match":"/.*/","response":"virtual done","exit_code":0}]`)

	record, err := r.RunCompare(CompareOptions{
		RunOptions: RunOptions{Prompt: "create new_file.txt", WorkDir: repo},
		Harnesses:  []string{"agent"},
		Sandbox:    true,
	})
	require.NoError(t, err)
	require.Len(t, record.Arms, 1)

	// Original repo should be unchanged
	_, err = os.Stat(filepath.Join(repo, "new_file.txt"))
	assert.True(t, os.IsNotExist(err), "sandbox should not modify original repo")
}

// C-03: Side-effect diff is captured when agent modifies files.
func TestCompareCapturesDiff(t *testing.T) {
	repo := setupTestRepo(t)

	// Use an agent provider that writes a file via tool call
	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "add readme",
			Response: agentlib.Response{
				ToolCalls: []agentlib.ToolCall{{
					ID:        "tc-1",
					Name:      "write",
					Arguments: json.RawMessage(`{"path":"README.md","content":"# Hello\n"}`),
				}},
				Model: "test-model",
				Usage: agentlib.TokenUsage{Input: 50, Output: 10, Total: 60},
			},
		}, {
			PromptMatch: "/./",
			Response: agentlib.Response{
				Content: "created README.md",
				Model:   "test-model",
				Usage:   agentlib.TokenUsage{Input: 80, Output: 15, Total: 95},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	record, err := r.RunCompare(CompareOptions{
		RunOptions: RunOptions{Prompt: "add readme", WorkDir: repo},
		Harnesses:  []string{"agent"},
		Sandbox:    true,
	})
	require.NoError(t, err)
	require.Len(t, record.Arms, 1)

	// The diff should mention README.md
	assert.Contains(t, record.Arms[0].Diff, "README.md")
}

// C-04: Arm with no file changes records empty diff.
func TestCompareEmptyDiff(t *testing.T) {
	repo := setupTestRepo(t)

	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "/./",
			Response: agentlib.Response{
				Content: "nothing to change",
				Model:   "test-model",
				Usage:   agentlib.TokenUsage{Input: 50, Output: 10, Total: 60},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	record, err := r.RunCompare(CompareOptions{
		RunOptions: RunOptions{Prompt: "review the code", WorkDir: repo},
		Harnesses:  []string{"agent"},
		Sandbox:    true,
	})
	require.NoError(t, err)
	require.Len(t, record.Arms, 1)
	assert.Empty(t, record.Arms[0].Diff)
}

// C-05: Worktrees are cleaned up by default.
func TestCompareCleansUpWorktrees(t *testing.T) {
	repo := setupTestRepo(t)

	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "/./",
			Response: agentlib.Response{
				Content: "ok",
				Model:   "test-model",
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	record, err := r.RunCompare(CompareOptions{
		RunOptions: RunOptions{Prompt: "test", WorkDir: repo},
		Harnesses:  []string{"agent"},
		Sandbox:    true,
	})
	require.NoError(t, err)

	// Worktrees directory should be empty or gone
	wtDir := filepath.Join(repo, ".worktrees")
	entries, _ := os.ReadDir(wtDir)
	// Filter for entries matching this comparison ID
	for _, e := range entries {
		assert.NotContains(t, e.Name(), record.ID,
			"worktree should be cleaned up")
	}
}

// C-06: --keep-sandbox preserves worktrees.
func TestCompareKeepSandbox(t *testing.T) {
	repo := setupTestRepo(t)

	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "/./",
			Response: agentlib.Response{
				Content: "ok",
				Model:   "test-model",
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	record, err := r.RunCompare(CompareOptions{
		RunOptions:  RunOptions{Prompt: "test", WorkDir: repo},
		Harnesses:   []string{"agent"},
		Sandbox:     true,
		KeepSandbox: true,
	})
	require.NoError(t, err)

	// At least one worktree should still exist
	wtDir := filepath.Join(repo, ".worktrees")
	entries, err := os.ReadDir(wtDir)
	require.NoError(t, err)
	found := false
	for _, e := range entries {
		if e.IsDir() {
			found = true
			break
		}
	}
	assert.True(t, found, "worktree should be preserved with --keep-sandbox")

	// Cleanup for test hygiene
	_ = exec.Command("git", "-C", repo, "worktree", "prune").Run()
	_ = os.RemoveAll(wtDir)
	_ = record // used above
}

// C-08: ComparisonRecord has expected schema.
func TestCompareRecordSchema(t *testing.T) {
	repo := setupTestRepo(t)

	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "/./",
			Response: agentlib.Response{
				Content: "done",
				Model:   "schema-model",
				Usage:   agentlib.TokenUsage{Input: 200, Output: 40, Total: 240},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	record, err := r.RunCompare(CompareOptions{
		RunOptions: RunOptions{Prompt: "schema test", WorkDir: repo},
		Harnesses:  []string{"agent"},
	})
	require.NoError(t, err)

	// Verify JSON serialization round-trips
	data, err := json.Marshal(record)
	require.NoError(t, err)
	var decoded ComparisonRecord
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.NotEmpty(t, decoded.ID)
	assert.False(t, decoded.Timestamp.IsZero())
	assert.Equal(t, "schema test", decoded.Prompt)
	require.Len(t, decoded.Arms, 1)
	assert.Equal(t, "agent", decoded.Arms[0].Harness)
	assert.Equal(t, "schema-model", decoded.Arms[0].Model)
	assert.Equal(t, 200, decoded.Arms[0].InputTokens)
	assert.Equal(t, 40, decoded.Arms[0].OutputTokens)
}

// C-09: If one arm fails, comparison still completes.
func TestCompareArmFailure(t *testing.T) {
	repo := setupTestRepo(t)

	// Agent provider succeeds
	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "/./",
			Response: agentlib.Response{
				Content: "ok",
				Model:   "test-model",
			},
		}},
	})

	// Mock executor for a subprocess harness that fails
	mock := &mockExecutor{output: "error output", exitCode: 1}

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.Executor = mock
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	record, err := r.RunCompare(CompareOptions{
		RunOptions: RunOptions{Prompt: "partial failure", WorkDir: repo},
		Harnesses:  []string{"agent", "codex"},
	})
	require.NoError(t, err)
	assert.Len(t, record.Arms, 2)

	// One should succeed, one should fail
	var agentArm, codexArm *ComparisonArm
	for i := range record.Arms {
		switch record.Arms[i].Harness {
		case "agent":
			agentArm = &record.Arms[i]
		case "codex":
			codexArm = &record.Arms[i]
		}
	}
	require.NotNil(t, agentArm)
	require.NotNil(t, codexArm)
	assert.Equal(t, 0, agentArm.ExitCode)
	assert.Equal(t, 1, codexArm.ExitCode)
}
