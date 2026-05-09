package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

func TestAgentRunRESTDispatchSpecParity(t *testing.T) {
	workDir := t.TempDir()
	body := `{
		"text":"hello from rest",
		"harness":"codex",
		"model":"gpt-5.4-mini",
		"profile":" smart ",
		"effort":"high",
		"permissions":"unrestricted",
		"timeout":"45s",
		"prompt_source":"rest-test"
	}`
	var spec agent.AgentRunDispatchSpec
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		t.Fatalf("decode REST dispatch spec: %v", err)
	}

	prepared, err := agent.PrepareAgentRunDispatch(workDir, spec)
	if err != nil {
		t.Fatalf("prepare REST dispatch spec: %v", err)
	}
	assertPreparedAgentRunDispatch(t, prepared, expectedPreparedAgentRunDispatch{
		workDir:      workDir,
		prompt:       "hello from rest",
		promptSource: "rest-test",
		harness:      "codex",
		model:        "gpt-5.4-mini",
		profile:      "smart",
		effort:       "high",
		permissions:  "unrestricted",
		timeout:      45 * time.Second,
	})
}

func TestAgentRunMCPDispatchSpecParity(t *testing.T) {
	workDir := t.TempDir()
	spec, err := agentRunDispatchSpecFromMCPArguments(map[string]any{
		"project":     "ignored-after-working-dir-resolution",
		"text":        "hello from mcp",
		"harness":     "claude",
		"model":       "sonnet",
		"profile":     " default ",
		"effort":      "medium",
		"permissions": "safe",
		"timeout":     "30s",
	})
	if err != nil {
		t.Fatalf("decode MCP dispatch spec: %v", err)
	}

	prepared, err := agent.PrepareAgentRunDispatch(workDir, spec)
	if err != nil {
		t.Fatalf("prepare MCP dispatch spec: %v", err)
	}
	assertPreparedAgentRunDispatch(t, prepared, expectedPreparedAgentRunDispatch{
		workDir:      workDir,
		prompt:       "hello from mcp",
		promptSource: "inline",
		harness:      "claude",
		model:        "sonnet",
		profile:      "default",
		effort:       "medium",
		permissions:  "safe",
		timeout:      30 * time.Second,
	})

	_, err = agentRunDispatchSpecFromMCPArguments(map[string]any{
		"prompt":  "hello",
		"unknown": "ignored before",
	})
	if err == nil || !strings.Contains(err.Error(), `unsupported agent run dispatch field "unknown"`) {
		t.Fatalf("unknown MCP field error = %v", err)
	}
}

func TestAgentRunDispatchUnsupportedFieldsFailExplicitly(t *testing.T) {
	_, err := agent.PrepareAgentRunDispatch(t.TempDir(), agent.AgentRunDispatchSpec{
		Prompt:    "hello",
		Compare:   true,
		Record:    true,
		Output:    "text",
		Harnesses: "codex,claude",
	})
	if err == nil {
		t.Fatal("expected unsupported field error")
	}
	for _, want := range []string{"compare", "harnesses", "output", "record"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("unsupported field error %q missing %q", err.Error(), want)
		}
	}
}

type expectedPreparedAgentRunDispatch struct {
	workDir      string
	prompt       string
	promptSource string
	harness      string
	model        string
	profile      string
	effort       string
	permissions  string
	timeout      time.Duration
}

func assertPreparedAgentRunDispatch(t *testing.T, got agent.PreparedAgentRunDispatch, want expectedPreparedAgentRunDispatch) {
	t.Helper()
	if got.Runtime.WorkDir != want.workDir {
		t.Fatalf("runtime WorkDir = %q, want %q", got.Runtime.WorkDir, want.workDir)
	}
	if got.Runtime.Prompt != want.prompt {
		t.Fatalf("runtime Prompt = %q, want %q", got.Runtime.Prompt, want.prompt)
	}
	if got.Runtime.PromptSource != want.promptSource {
		t.Fatalf("runtime PromptSource = %q, want %q", got.Runtime.PromptSource, want.promptSource)
	}
	if got.Overrides.Harness != want.harness {
		t.Fatalf("override Harness = %q, want %q", got.Overrides.Harness, want.harness)
	}
	if got.Overrides.Model != want.model {
		t.Fatalf("override Model = %q, want %q", got.Overrides.Model, want.model)
	}
	if got.Overrides.Profile != want.profile {
		t.Fatalf("override Profile = %q, want %q", got.Overrides.Profile, want.profile)
	}
	if got.Overrides.Effort != want.effort {
		t.Fatalf("override Effort = %q, want %q", got.Overrides.Effort, want.effort)
	}
	if got.Overrides.Permissions != want.permissions {
		t.Fatalf("override Permissions = %q, want %q", got.Overrides.Permissions, want.permissions)
	}
	if got.Overrides.Timeout == nil {
		t.Fatalf("override Timeout is nil, want %v", want.timeout)
	}
	if *got.Overrides.Timeout != want.timeout {
		t.Fatalf("override Timeout = %v, want %v", *got.Overrides.Timeout, want.timeout)
	}
}
