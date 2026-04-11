package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/agent/compaction"
	agentconfig "github.com/DocumentDrivenDX/agent/config"
	"github.com/DocumentDrivenDX/agent/prompt"
	"github.com/DocumentDrivenDX/agent/provider/anthropic"
	oai "github.com/DocumentDrivenDX/agent/provider/openai"
	"github.com/DocumentDrivenDX/agent/provider/virtual"
	"github.com/DocumentDrivenDX/agent/session"
	"github.com/DocumentDrivenDX/agent/tool"
	"gopkg.in/yaml.v3"
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
	// Use injected provider (testing) or build from resolved config.
	var agentCfg AgentRunConfig
	var provider agentlib.Provider
	if r.AgentProvider != nil {
		agentCfg, err = r.resolveAgentConfig(model)
		if err != nil {
			return nil, err
		}
		provider = r.AgentProvider.(agentlib.Provider)
	} else {
		resolved, err := r.resolveEmbeddedAgentProvider(wd, model)
		if err != nil {
			return nil, err
		}
		agentCfg = resolved.Config
		provider = resolved.Provider
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
		Metadata:      opts.Correlation,
		Compactor:     compaction.NewCompactor(embeddedCompactionConfig(agentCfg.Model)),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var timedOut atomic.Bool
	if timeout > 0 {
		activity := make(chan struct{}, 1)
		pulse := func() {
			select {
			case activity <- struct{}{}:
			default:
			}
		}
		callback := logger.Callback()
		req.Callback = func(event agentlib.Event) {
			pulse()
			callback(event)
		}
		go func() {
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-activity:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(timeout)
				case <-timer.C:
					timedOut.Store(true)
					cancel()
					return
				}
			}
		}()
	} else {
		req.Callback = logger.Callback()
	}

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
		Provider:       agentResult.SelectedProvider,
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

	if timedOut.Load() {
		result.Error = fmt.Sprintf("timeout after %v", timeout.Round(time.Second))
		result.ExitCode = 1
	} else if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			result.Error = fmt.Sprintf("timeout after %v", timeout.Round(time.Second))
		} else {
			result.Error = err.Error()
		}
		result.ExitCode = 1
	} else if agentResult.Status != agentlib.StatusSuccess {
		if agentResult.Status == agentlib.StatusCancelled {
			result.Error = string(agentlib.StatusCancelled)
		} else {
			result.Error = string(agentResult.Status)
		}
		result.ExitCode = 1
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

type embeddedAgentProviderResolution struct {
	Config   AgentRunConfig
	Provider agentlib.Provider
}

func embeddedCompactionConfig(model string) compaction.Config {
	cfg := compaction.DefaultConfig()
	// DDx does not yet have reliable per-model context-window metadata for
	// embedded providers. Use a conservative default that actually fires under
	// long-running multi-turn planning/doc loops instead of inheriting the
	// upstream 8k/8k defaults, which never trigger compaction.
	cfg.ContextWindow = 32_000
	cfg.ReserveTokens = 4_000
	cfg.KeepRecentTokens = 8_000
	return cfg
}

// resolveAgentConfig builds an AgentRunConfig from .ddx/config.yaml, env vars, and opts.
// Priority: opts > env vars > config > built-in defaults.
// If model resolves to a named preset in agent.models, the preset's endpoint and model are applied.
func (r *Runner) resolveAgentConfig(model string) (AgentRunConfig, error) {
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

	if !containsString(prompt.PresetNames(), cfg.Preset) {
		return AgentRunConfig{}, fmt.Errorf("agent: unsupported preset %q; supported presets: %s", cfg.Preset, strings.Join(prompt.PresetNames(), ", "))
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
	} else if isOpenRouterModel(cfg.Model) {
		// Layer 5: unknown model with vendor/ prefix → fall back to OpenRouter
		cfg.Provider = "openai-compat"
		cfg.BaseURL = "https://openrouter.ai/api/v1"
		if cfg.APIKey == "" {
			cfg.APIKey = os.Getenv("OPENROUTER_API_KEY")
		}
	}

	return cfg, nil
}

func (r *Runner) resolveEmbeddedAgentProvider(workDir, model string) (*embeddedAgentProviderResolution, error) {
	// If the model looks like an OpenRouter model (vendor/name format), skip
	// native config and legacy preset resolution — route to OpenRouter directly.
	if isOpenRouterModel(model) {
		return r.resolveOpenRouterProvider(model)
	}

	if resolved, err := r.resolveNativeAgentProvider(workDir, model); err != nil {
		return nil, err
	} else if resolved != nil {
		return resolved, nil
	}

	cfg, err := r.resolveAgentConfig(model)
	if err != nil {
		return nil, err
	}
	provider, err := buildAgentProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("agent: provider: %w", err)
	}
	return &embeddedAgentProviderResolution{Config: cfg, Provider: provider}, nil
}

func (r *Runner) resolveNativeAgentProvider(workDir, model string) (*embeddedAgentProviderResolution, error) {
	if model != "" && r.legacyPresetExists(model) {
		return nil, nil
	}

	cfg, err := agentconfig.Load(workDir)
	if err != nil {
		return nil, fmt.Errorf("agent: native config: %w", err)
	}
	if cfg == nil || len(cfg.ProviderNames()) == 0 {
		return nil, nil
	}
	applyNativeDefaultProviderCompatibility(cfg, workDir)

	overrides := agentconfig.ProviderOverrides{}
	if model != "" {
		overrides.Model = model
	}
	providerName := cfg.DefaultName()

	// If the model looks like a vendor/model (e.g. "z-ai/glm-5.1", "qwen/qwen3.6"),
	// prefer the "openrouter" provider if it exists in the config. These models
	// are only available through OpenRouter; local providers will reject them.
	if isOpenRouterModel(model) {
		if _, ok := cfg.GetProvider("openrouter"); ok {
			providerName = "openrouter"
		}
	}
	provider, pc, _, err := cfg.BuildProviderWithOverrides(providerName, overrides)
	if err != nil {
		return nil, fmt.Errorf("agent: native config provider %q: %w", providerName, err)
	}

	runCfg := AgentRunConfig{
		Provider:      pc.Type,
		BaseURL:       pc.BaseURL,
		APIKey:        pc.APIKey,
		Model:         pc.Model,
		Preset:        cfg.Preset,
		MaxIterations: cfg.MaxIterations,
	}
	if runCfg.Preset == "" {
		runCfg.Preset = "agent"
	}
	if runCfg.MaxIterations == 0 {
		runCfg.MaxIterations = 20
	}
	if !containsString(prompt.PresetNames(), runCfg.Preset) {
		return nil, fmt.Errorf("agent: unsupported preset %q; supported presets: %s", runCfg.Preset, strings.Join(prompt.PresetNames(), ", "))
	}

	return &embeddedAgentProviderResolution{
		Config:   runCfg,
		Provider: provider,
	}, nil
}

func (r *Runner) legacyPresetExists(model string) bool {
	if model == "" || r.AgentConfigLoader == nil {
		return false
	}
	if fc := r.AgentConfigLoader(); fc != nil && fc.Models != nil {
		_, ok := fc.Models[model]
		return ok
	}
	return false
}

func applyNativeDefaultProviderCompatibility(cfg *agentconfig.Config, workDir string) {
	if cfg == nil || cfg.Default != "" || len(cfg.Providers) == 0 {
		return
	}
	if alias := nativeDefaultProviderAlias(workDir); alias != "" {
		if _, ok := cfg.Providers[alias]; ok {
			cfg.Default = alias
		}
	}
}

func nativeDefaultProviderAlias(workDir string) string {
	paths := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "agent", "config.yaml"))
	}
	paths = append(paths, filepath.Join(workDir, ".agent", "config.yaml"))

	alias := ""
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		expanded := os.Expand(string(data), func(key string) string {
			return os.Getenv(key)
		})
		var parsed struct {
			DefaultProvider string `yaml:"default_provider"`
		}
		if err := yaml.Unmarshal([]byte(expanded), &parsed); err != nil {
			continue
		}
		if parsed.DefaultProvider != "" {
			alias = parsed.DefaultProvider
		}
	}
	return alias
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

// resolveOpenRouterProvider builds a provider config for OpenRouter.
// Models with vendor/name format (e.g. "z-ai/glm-5.1", "qwen/qwen3.6") are
// routed to OpenRouter unless they match a local preset.
func (r *Runner) resolveOpenRouterProvider(model string) (*embeddedAgentProviderResolution, error) {
	cfg := AgentRunConfig{
		Provider:      "openai-compat",
		BaseURL:       "https://openrouter.ai/api/v1",
		APIKey:        os.Getenv("OPENROUTER_API_KEY"),
		Model:         model,
		Preset:        "agent",
		MaxIterations: 20,
	}

	// Allow config to override defaults
	if r.AgentConfigLoader != nil {
		if fc := r.AgentConfigLoader(); fc != nil {
			if fc.MaxIterations > 0 {
				cfg.MaxIterations = fc.MaxIterations
			}
		}
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("agent: OPENROUTER_API_KEY not set; required for remote model %q", model)
	}

	provider, err := buildAgentProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("agent: openrouter provider: %w", err)
	}
	return &embeddedAgentProviderResolution{Config: cfg, Provider: provider}, nil
}

// isOpenRouterModel returns true if the model name looks like an OpenRouter
// model identifier (vendor/model format like "qwen/qwen3.6", "z-ai/glm-5.1").
// Local preset names never contain "/".
func isOpenRouterModel(model string) bool {
	if model == "" {
		return false
	}
	return strings.Contains(model, "/")
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
