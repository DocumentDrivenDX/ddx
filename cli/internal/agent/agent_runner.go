package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/agent/prompt"
	"github.com/DocumentDrivenDX/agent/provider/anthropic"
	oai "github.com/DocumentDrivenDX/agent/provider/openai"
	"github.com/DocumentDrivenDX/agent/provider/virtual"
	"github.com/DocumentDrivenDX/agent/session"
	"github.com/DocumentDrivenDX/agent/tool"
)

// AgentRunConfig holds resolved configuration for one agent invocation.
type AgentRunConfig struct {
	Provider      string
	BaseURL       string
	APIKey        string
	Model         string
	Preset        string
	MaxIterations int
}

// roundRobinCounter is shared across all agent runs for endpoint rotation.
var roundRobinCounter uint64

// RunAgent executes a prompt using the embedded agent library.
// This runs in-process — no subprocess, no binary lookup.
func (r *Runner) RunAgent(opts RunOptions) (*Result, error) {
	promptText, err := r.resolvePrompt(opts)
	if err != nil {
		return nil, err
	}

	model := r.resolveModel(opts, "agent")
	timeout := r.resolveTimeout(opts)

	// Resolve working directory
	wd := opts.WorkDir
	if wd == "" {
		wd, _ = os.Getwd()
	}

	// Resolve agent configuration (config.yaml → env vars → opts)
	agentCfg := r.resolveAgentConfig(model)

	// Use injected provider (testing) or build from resolved config.
	var provider agentlib.Provider
	if r.AgentProvider != nil {
		provider = r.AgentProvider.(agentlib.Provider)
	} else {
		var err error
		provider, err = buildAgentProvider(agentCfg)
		if err != nil {
			return nil, fmt.Errorf("agent: provider: %w", err)
		}
	}

	maxIter := agentCfg.MaxIterations
	if maxIter == 0 {
		maxIter = 20
	}

	// Build tools
	tools := []agentlib.Tool{
		&tool.ReadTool{WorkDir: wd},
		&tool.WriteTool{WorkDir: wd},
		&tool.EditTool{WorkDir: wd},
		&tool.BashTool{WorkDir: wd},
	}

	// Build system prompt using presets.
	sysPrompt := prompt.NewFromPreset(agentCfg.Preset).
		WithTools(tools).
		WithContextFiles(prompt.LoadContextFiles(wd)).
		WithWorkDir(wd).
		Build()

	// Session logger
	logDir := r.Config.SessionLogDir
	if logDir == "" {
		logDir = DefaultLogDir
	}
	sessionID := fmt.Sprintf("agent-%d", time.Now().UnixNano())
	logger := session.NewLogger(logDir, sessionID)
	defer logger.Close() //nolint:errcheck

	req := agentlib.Request{
		Prompt:        promptText,
		SystemPrompt:  sysPrompt,
		Provider:      provider,
		Tools:         tools,
		MaxIterations: maxIter,
		WorkDir:       wd,
		Callback:      logger.Callback(),
		Metadata:      opts.Correlation,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	agentResult, err := agentlib.Run(ctx, req)
	elapsed := time.Since(start)

	// Map agent tool calls to DDx ToolCallEntry
	var toolCalls []ToolCallEntry
	for _, tc := range agentResult.ToolCalls {
		entry := ToolCallEntry{
			Tool:     tc.Tool,
			Input:    string(tc.Input),
			Output:   tc.Output,
			Duration: int(tc.Duration.Milliseconds()),
			Error:    tc.Error,
		}
		toolCalls = append(toolCalls, entry)
	}

	result := &Result{
		Harness:        "agent",
		Model:          agentResult.Model,
		Output:         agentResult.Output,
		InputTokens:    agentResult.Tokens.Input,
		OutputTokens:   agentResult.Tokens.Output,
		Tokens:         agentResult.Tokens.Total,
		DurationMS:     int(elapsed.Milliseconds()),
		ToolCalls:      toolCalls,
		AgentSessionID: agentResult.SessionID,
	}

	if agentResult.CostUSD >= 0 {
		result.CostUSD = agentResult.CostUSD
	}

	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 1
	} else if agentResult.Status != agentlib.StatusSuccess {
		result.ExitCode = 1
		result.Error = string(agentResult.Status)
	}

	// Log session
	promptSource := opts.PromptSource
	if promptSource == "" {
		if opts.PromptFile != "" {
			promptSource = opts.PromptFile
		} else {
			promptSource = "inline"
		}
	}
	r.logSession(result, len(promptText), promptText, promptSource, opts.Correlation)
	r.recordRoutingOutcome(result, elapsed, opts)
	return result, nil
}

// resolveAgentConfig builds an AgentRunConfig from .ddx/config.yaml, env vars, and opts.
// Priority: opts > env vars > config > built-in defaults.
// If model resolves to a named preset in agent.models, the preset's endpoint and model are applied.
func (r *Runner) resolveAgentConfig(model string) AgentRunConfig {
	cfg := AgentRunConfig{
		Provider:      "openai-compat",
		BaseURL:       "http://localhost:1234/v1",
		Preset:        "agent",
		MaxIterations: 20,
	}

	var yamlModels map[string]*LLMPresetYAML

	// Layer 1: .ddx/config.yaml (if AgentConfigLoader is set)
	if r.AgentConfigLoader != nil {
		if fc := r.AgentConfigLoader(); fc != nil {
			if fc.Provider != "" {
				cfg.Provider = fc.Provider
			}
			if fc.BaseURL != "" {
				cfg.BaseURL = fc.BaseURL
			}
			if fc.APIKey != "" {
				cfg.APIKey = fc.APIKey
			}
			if fc.Model != "" {
				cfg.Model = fc.Model
			}
			if fc.Preset != "" {
				cfg.Preset = fc.Preset
			}
			if fc.MaxIterations > 0 {
				cfg.MaxIterations = fc.MaxIterations
			}
			yamlModels = fc.Models
		}
	}

	// Layer 2: environment variables override config
	if v := os.Getenv("AGENT_PROVIDER"); v != "" {
		cfg.Provider = v
	}
	if v := os.Getenv("AGENT_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("AGENT_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("AGENT_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("AGENT_PRESET"); v != "" {
		cfg.Preset = v
	}

	// Layer 3: opts.Model overrides everything
	if model != "" {
		cfg.Model = model
	}

	// Layer 4: if cfg.Model names a preset, resolve it to endpoint + model.
	if preset, ok := yamlModels[cfg.Model]; ok {
		cfg.Model = preset.Model
		if preset.Provider != "" {
			cfg.Provider = preset.Provider
		}
		if preset.APIKey != "" {
			cfg.APIKey = preset.APIKey
		}
		if len(preset.Endpoints) > 0 {
			cfg.BaseURL = selectEndpoint(preset.Endpoints, preset.Strategy)
		}
	}

	return cfg
}

// selectEndpoint picks one endpoint from the list using the specified strategy.
// Strategies: "round-robin" (default), "first-available" (first entry, no rotation).
func selectEndpoint(endpoints []string, strategy string) string {
	if len(endpoints) == 1 {
		return endpoints[0]
	}
	if strategy == "first-available" {
		return endpoints[0]
	}
	// Default: round-robin using a global atomic counter.
	idx := atomic.AddUint64(&roundRobinCounter, 1) - 1
	return endpoints[idx%uint64(len(endpoints))]
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// buildAgentProvider creates an agentlib.Provider from resolved config.
func buildAgentProvider(cfg AgentRunConfig) (agentlib.Provider, error) {
	switch cfg.Provider {
	case "openai-compat", "openai":
		return oai.New(oai.Config{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
		}), nil
	case "anthropic":
		return anthropic.New(anthropic.Config{
			APIKey: cfg.APIKey,
			Model:  cfg.Model,
		}), nil
	case "virtual":
		dictDir := filepath.Join(".ddx", "agent", "dictionary")
		return virtual.New(virtual.Config{DictDir: dictDir}), nil
	default:
		return nil, fmt.Errorf("unknown agent provider %q (use openai-compat, anthropic, or virtual)", cfg.Provider)
	}
}
