package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
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
		profile:      " smart ",
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
		profile:      " default ",
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

func TestUnpinnedEntryPointsIgnoreConfiguredProjectModel(t *testing.T) {
	cfg := config.NewTestConfigForRun(config.TestRunConfigOpts{Model: "configured-project-model"})

	t.Run("REST", func(t *testing.T) {
		svc := installResolveRouteFailingService(t)
		prepared, err := agent.PrepareAgentRunDispatch(t.TempDir(), agent.AgentRunDispatchSpec{Prompt: "hello"})
		if err != nil {
			t.Fatalf("prepare REST dispatch: %v", err)
		}
		rcfg := cfg.Resolve(prepared.Overrides)
		if got := rcfg.Model(); got != "" {
			t.Fatalf("unconfigured REST dispatch inherited project agent.model %q", got)
		}
		if _, err := agent.RunWithConfigViaService(context.Background(), prepared.Runtime.WorkDir, rcfg, prepared.Runtime); err != nil {
			t.Fatalf("execute REST dispatch: %v", err)
		}
		assertOpaqueUnpinnedExecuteRequest(t, svc.lastExecuteReq)
	})

	t.Run("MCP", func(t *testing.T) {
		svc := installResolveRouteFailingService(t)
		spec, err := agentRunDispatchSpecFromMCPArguments(map[string]any{"text": "hello"})
		if err != nil {
			t.Fatalf("prepare MCP arguments: %v", err)
		}
		prepared, err := agent.PrepareAgentRunDispatch(t.TempDir(), spec)
		if err != nil {
			t.Fatalf("prepare MCP dispatch: %v", err)
		}
		rcfg := cfg.Resolve(prepared.Overrides)
		if got := rcfg.Model(); got != "" {
			t.Fatalf("unconfigured MCP dispatch inherited project agent.model %q", got)
		}
		if _, err := agent.RunWithConfigViaService(context.Background(), prepared.Runtime.WorkDir, rcfg, prepared.Runtime); err != nil {
			t.Fatalf("execute MCP dispatch: %v", err)
		}
		assertOpaqueUnpinnedExecuteRequest(t, svc.lastExecuteReq)
	})

	t.Run("server-managed", func(t *testing.T) {
		assertServerManagedUnpinnedExecute(t)
	})
}

func assertOpaqueUnpinnedExecuteRequest(t *testing.T, req agentlib.ServiceExecuteRequest) {
	t.Helper()
	if req.Harness != "" || req.Provider != "" || req.Model != "" || req.Policy != "" {
		t.Fatalf("unpinned Execute request leaked a concrete route: harness=%q provider=%q model=%q policy=%q",
			req.Harness, req.Provider, req.Model, req.Policy)
	}
	if req.Reasoning != "" || req.ProviderTimeout != 0 {
		t.Fatalf("unpinned Execute request leaked DDx reasoning/timeout policy: reasoning=%q provider_timeout=%s",
			req.Reasoning, req.ProviderTimeout)
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
	if !got.Overrides.OpaquePassthrough {
		t.Fatal("REST/MCP dispatch must suppress project routing defaults")
	}
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
