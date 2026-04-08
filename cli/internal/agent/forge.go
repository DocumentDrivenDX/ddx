package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/DocumentDrivenDX/forge"
	"github.com/DocumentDrivenDX/forge/prompt"
	"github.com/DocumentDrivenDX/forge/provider/anthropic"
	oai "github.com/DocumentDrivenDX/forge/provider/openai"
	"github.com/DocumentDrivenDX/forge/provider/virtual"
	"github.com/DocumentDrivenDX/forge/session"
	"github.com/DocumentDrivenDX/forge/tool"
)

// ForgeRunConfig holds resolved configuration for one forge invocation.
type ForgeRunConfig struct {
	Provider      string
	BaseURL       string
	APIKey        string
	Model         string
	Preset        string
	MaxIterations int
}

// roundRobinCounter is shared across all forge runs for endpoint rotation.
var roundRobinCounter uint64

// RunForge executes a prompt using the embedded forge library.
// This runs in-process — no subprocess, no binary lookup.
func (r *Runner) RunForge(opts RunOptions) (*Result, error) {
	promptText, err := r.resolvePrompt(opts)
	if err != nil {
		return nil, err
	}

	model := r.resolveModel(opts, "forge")
	timeout := r.resolveTimeout(opts)

	// Resolve working directory
	wd := opts.WorkDir
	if wd == "" {
		wd, _ = os.Getwd()
	}

	// Resolve forge configuration (config.yaml → env vars → opts)
	forgeCfg := r.resolveForgeConfig(model)

	// Use injected provider (testing) or build from resolved config.
	var provider forge.Provider
	if r.ForgeProvider != nil {
		provider = r.ForgeProvider.(forge.Provider)
	} else {
		var err error
		provider, err = buildForgeProvider(forgeCfg)
		if err != nil {
			return nil, fmt.Errorf("agent: forge provider: %w", err)
		}
	}

	maxIter := forgeCfg.MaxIterations
	if maxIter == 0 {
		maxIter = 20
	}

	// Build tools
	tools := []forge.Tool{
		&tool.ReadTool{WorkDir: wd},
		&tool.WriteTool{WorkDir: wd},
		&tool.EditTool{WorkDir: wd},
		&tool.BashTool{WorkDir: wd},
	}

	// Build system prompt using forge presets.
	sysPrompt := prompt.NewFromPreset(forgeCfg.Preset).
		WithTools(tools).
		WithContextFiles(prompt.LoadContextFiles(wd)).
		WithWorkDir(wd).
		Build()

	// Session logger
	logDir := r.Config.SessionLogDir
	if logDir == "" {
		logDir = DefaultLogDir
	}
	sessionID := fmt.Sprintf("forge-%d", time.Now().UnixNano())
	logger := session.NewLogger(logDir, sessionID)
	defer logger.Close()

	req := forge.Request{
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
	forgeResult, err := forge.Run(ctx, req)
	elapsed := time.Since(start)

	// Map forge tool calls to DDx ToolCallEntry
	var toolCalls []ToolCallEntry
	for _, tc := range forgeResult.ToolCalls {
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
		Harness:        "forge",
		Model:          forgeResult.Model,
		Output:         forgeResult.Output,
		InputTokens:    forgeResult.Tokens.Input,
		OutputTokens:   forgeResult.Tokens.Output,
		Tokens:         forgeResult.Tokens.Total,
		DurationMS:     int(elapsed.Milliseconds()),
		ToolCalls:      toolCalls,
		ForgeSessionID: forgeResult.SessionID,
	}

	if forgeResult.CostUSD >= 0 {
		result.CostUSD = forgeResult.CostUSD
	}

	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 1
	} else if forgeResult.Status != forge.StatusSuccess {
		result.ExitCode = 1
		result.Error = string(forgeResult.Status)
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
	return result, nil
}

// resolveForgeConfig builds a ForgeRunConfig from .ddx/config.yaml, env vars, and opts.
// Priority: opts > env vars > config > built-in defaults.
// If model resolves to a named preset in forge.models, the preset's endpoint and model are applied.
func (r *Runner) resolveForgeConfig(model string) ForgeRunConfig {
	cfg := ForgeRunConfig{
		Provider:      "openai-compat",
		BaseURL:       "http://localhost:1234/v1",
		Preset:        "forge",
		MaxIterations: 20,
	}

	var yamlModels map[string]*LLMPresetYAML

	// Layer 1: .ddx/config.yaml (if ForgeConfigLoader is set)
	if r.ForgeConfigLoader != nil {
		if fc := r.ForgeConfigLoader(); fc != nil {
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
	if v := os.Getenv("FORGE_PROVIDER"); v != "" {
		cfg.Provider = v
	}
	if v := os.Getenv("FORGE_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("FORGE_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("FORGE_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("FORGE_PRESET"); v != "" {
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

// buildForgeProvider creates a forge.Provider from resolved config.
func buildForgeProvider(cfg ForgeRunConfig) (forge.Provider, error) {
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
		dictDir := filepath.Join(".forge", "dictionary")
		return virtual.New(virtual.Config{DictDir: dictDir}), nil
	default:
		return nil, fmt.Errorf("unknown forge provider %q (use openai-compat, anthropic, or virtual)", cfg.Provider)
	}
}
