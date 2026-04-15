package agent

import (
	"context"
	"encoding/json"
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

// maxConsecutiveCompactionFailures is the number of consecutive no-op compaction
// events (success=false, no_compaction=true) with no write-tool progress that
// triggers the compaction-stuck circuit breaker. The counter resets when a
// write-capable tool (bash/write/edit) fires or compaction succeeds.
// Each agent iteration emits at most 2 compaction events (pre + mid-turn), so
// this threshold allows ~25 iterations of read-only work before bailing.
const maxConsecutiveCompactionFailures = 50

// isNoopCompactionEndEvent reports whether e is a compaction.end event that
// indicates no compaction occurred (success=false, no_compaction=true). These
// events fire every iteration when the context is below the compaction threshold
// or the re-compaction guard blocks a second pass. They represent no real
// progress and should not reset the inactivity timer or the stuck counter.
func isNoopCompactionEndEvent(e agentlib.Event) bool {
	if e.Type != agentlib.EventCompactionEnd {
		return false
	}
	var data struct {
		Success      bool `json:"success"`
		NoCompaction bool `json:"no_compaction"`
	}
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return false
	}
	return !data.Success && data.NoCompaction
}

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

	// Resolve agent configuration (native config → env vars → built-in defaults).
	// Use injected provider (testing) or build from resolved config.
	var agentCfg AgentRunConfig
	var provider agentlib.Provider
	var runAgentRouteReason string
	if r.AgentProvider != nil {
		agentCfg, err = r.resolveAgentConfig(model)
		if err != nil {
			return nil, err
		}
		provider = r.AgentProvider.(agentlib.Provider)
		runAgentRouteReason = "direct-override"
	} else {
		resolved, err := r.resolveEmbeddedAgentProvider(wd, model)
		if err != nil {
			return nil, err
		}
		agentCfg = resolved.Config
		provider = resolved.Provider
		runAgentRouteReason = resolved.RouteReason
	}

	maxIter := agentCfg.MaxIterations
	if maxIter == 0 {
		maxIter = 100
	}

	// Stall detection: if the agent hasn't made any write/edit calls after
	// this many consecutive read-only tool calls, cancel execution.
	// When stall detection is active, allow more iterations — the stall
	// detector will catch a stuck agent faster than iteration counting.
	stallThreshold := 30 // enough read-only calls to read many files but not infinite
	if stallThreshold > 0 && maxIter < 200 {
		maxIter = 200 // Let stall detection be the primary circuit breaker
	}
	tools := []agentlib.Tool{
		&tool.ReadTool{WorkDir: wd},
		&tool.WriteTool{WorkDir: wd},
		&tool.EditTool{WorkDir: wd},
		&tool.BashTool{WorkDir: wd},
		&tool.LsTool{WorkDir: wd},
		&tool.GrepTool{WorkDir: wd},
		&findTool{Glob: tool.GlobTool{WorkDir: wd}},
	}

	// Build system prompt using presets.
	sysPrompt := prompt.NewFromPreset(agentCfg.Preset).
		WithTools(tools).
		WithContextFiles(prompt.LoadContextFiles(wd)).
		WithWorkDir(wd).
		Build()

	// Session logger. Per-run override (opts.SessionLogDir) takes precedence over
	// the runner-wide Config.SessionLogDir so managed execute-bead invocations can
	// redirect embedded-agent runtime state into a DDx-owned execution bundle
	// instead of the runner cwd.
	logDir := opts.SessionLogDir
	if logDir == "" {
		logDir = r.Config.SessionLogDir
	}
	if logDir == "" {
		logDir = DefaultLogDir
	}
	sessionID := fmt.Sprintf("agent-%d", time.Now().UnixNano())
	logger := session.NewLogger(logDir, sessionID)
	defer logger.Close() //nolint:errcheck

	compactor := compaction.NewCompactor(embeddedCompactionConfig(agentCfg.Model))
	if r.CompactorOverride != nil {
		compactor = r.CompactorOverride
	}

	req := agentlib.Request{
		Prompt:        promptText,
		SystemPrompt:  sysPrompt,
		Provider:      provider,
		Tools:         tools,
		MaxIterations: maxIter,
		WorkDir:       wd,
		Metadata:      opts.Correlation,
		Compactor:     compactor,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var timedOut atomic.Bool
	var stalled atomic.Bool
	var compactionStuck atomic.Bool
	var readOnlyCount atomic.Int32
	var consecutiveCompFails atomic.Int32

	stuckThreshold := maxConsecutiveCompactionFailures
	if r.CompactionStuckThreshold > 0 {
		stuckThreshold = r.CompactionStuckThreshold
	}

	// Stall detection: track write activity in the callback.
	// If no write/edit tool calls after stallThreshold read-only calls, cancel.
	// Write-capable tools: bash, write, edit. bash is classified as write-only
	// because the dedicated ls/find/grep tools now cover all read-only shell
	// use cases — any bash invocation is assumed to be a mutation.
	// Read-only tools: read, ls, find, grep.
	stallCallback := func(event agentlib.Event) {
		if event.Type == agentlib.EventToolCall {
			var data struct {
				Tool string `json:"tool"`
			}
			if err := json.Unmarshal(event.Data, &data); err == nil {
				if isWriteCapableTool(data.Tool) {
					readOnlyCount.Store(0)
				} else {
					consecutive := readOnlyCount.Add(1)
					if consecutive >= int32(stallThreshold) {
						stalled.Store(true)
						cancel()
					}
				}
			}
		}
	}

	// compactionCallback implements the compaction-stuck circuit breaker.
	//
	// Every agentlib iteration emits compaction.start / compaction.end pairs
	// regardless of whether compaction actually runs (the compactor is called and
	// returns no-op when ShouldCompact is false or the re-compaction guard fires).
	// These no-op events represent zero real progress. If they accumulate without
	// any write-tool activity or successful compaction, the agent is stuck.
	//
	// Counter behaviour:
	//   - Increment on: compaction.end with no_compaction=true, success=false
	//   - Reset on:     write-capable tool call (bash/write/edit) OR
	//                   compaction.end with success=true
	//   - Fire when:    consecutive count >= stuckThreshold
	//
	// The counter is NOT reset by LLM events or read-only tool calls so that an
	// agent looping with git/grep commands (write-capable bash resets it) but
	// also an agent stuck in pure read loops is caught eventually.
	compactionCallback := func(event agentlib.Event) {
		if isNoopCompactionEndEvent(event) {
			count := consecutiveCompFails.Add(1)
			if count >= int32(stuckThreshold) {
				compactionStuck.Store(true)
				cancel()
			}
			return
		}
		// Reset on successful compaction.
		if event.Type == agentlib.EventCompactionEnd {
			consecutiveCompFails.Store(0)
			return
		}
		// Reset on write-capable tool calls (same classification as stall detection).
		if event.Type == agentlib.EventToolCall {
			var data struct {
				Tool string `json:"tool"`
			}
			if err := json.Unmarshal(event.Data, &data); err == nil && isWriteCapableTool(data.Tool) {
				consecutiveCompFails.Store(0)
			}
		}
	}

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
			// No-op compaction events are not real progress: suppress the
			// inactivity-timer pulse so the timer can fire when the agent is
			// stuck generating only compaction events. compaction.start is also
			// suppressed because it is always immediately followed by its end.
			if event.Type != agentlib.EventCompactionStart && !isNoopCompactionEndEvent(event) {
				pulse()
			}
			compactionCallback(event)
			stallCallback(event)
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
		callback := logger.Callback()
		req.Callback = func(event agentlib.Event) {
			compactionCallback(event)
			stallCallback(event)
			callback(event)
		}
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
		Harness:         "agent",
		Provider:        agentResult.SelectedProvider,
		Model:           agentResult.Model,
		Output:          agentResult.Output,
		InputTokens:     agentResult.Tokens.Input,
		OutputTokens:    agentResult.Tokens.Output,
		Tokens:          agentResult.Tokens.Total,
		DurationMS:      int(elapsed.Milliseconds()),
		ToolCalls:       toolCalls,
		AgentSessionID:  agentResult.SessionID,
		RouteReason:     runAgentRouteReason,
		ResolvedBaseURL: agentCfg.BaseURL,
	}

	if agentResult.CostUSD >= 0 {
		result.CostUSD = agentResult.CostUSD
	}

	if timedOut.Load() {
		result.Error = fmt.Sprintf("timeout after %v", timeout.Round(time.Second))
		result.ExitCode = 1
	} else if compactionStuck.Load() {
		result.Error = fmt.Sprintf("compaction stuck: %d consecutive failed compaction attempts with no write progress", stuckThreshold)
		result.ExitCode = 1
	} else if stalled.Load() {
		result.Error = fmt.Sprintf("stalled: no write activity after %d consecutive read-only tool calls", stallThreshold)
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
	Config      AgentRunConfig
	Provider    agentlib.Provider
	RouteReason string // how the provider was selected (catalog-match, direct-override, first-available)
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

// resolveAgentConfig builds an AgentRunConfig from built-in defaults, env vars, and opts.
// Priority: opts (model arg) > env vars > built-in defaults.
// Used for the testing path (when AgentProvider is injected) and as the fallback when no
// native .agent/config.yaml is present.
func (r *Runner) resolveAgentConfig(model string) (AgentRunConfig, error) {
	cfg := AgentRunConfig{
		Provider:      "openai-compat",
		BaseURL:       "http://localhost:1234/v1",
		Preset:        "agent",
		MaxIterations: 100,
	}

	// Layer 1: environment variables override built-in defaults
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

	// Layer 2: opts.Model overrides everything
	if model != "" {
		cfg.Model = model
	}

	if !containsString(prompt.PresetNames(), cfg.Preset) {
		return AgentRunConfig{}, fmt.Errorf("agent: unsupported preset %q; supported presets: %s", cfg.Preset, strings.Join(prompt.PresetNames(), ", "))
	}

	return cfg, nil
}

func (r *Runner) resolveEmbeddedAgentProvider(workDir, model string) (*embeddedAgentProviderResolution, error) {
	// Try native agent config first (from ~/.config/agent/config.yaml or .agent/config.yaml).
	// This handles all provider routing including OpenRouter when configured.
	if resolved, err := r.resolveNativeAgentProvider(workDir, model); err != nil {
		return nil, err
	} else if resolved != nil {
		return resolved, nil
	}

	// No native config found. Use built-in defaults + env vars.
	cfg, err := r.resolveAgentConfig(model)
	if err != nil {
		return nil, err
	}
	provider, err := buildAgentProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("agent: provider: %w", err)
	}
	fallbackRouteReason := "first-available"
	if model != "" {
		if modelRef, _ := BuiltinCatalog.NormalizeModelRef(model); modelRef != "" {
			fallbackRouteReason = "catalog-match"
		} else {
			fallbackRouteReason = "direct-override"
		}
	}
	return &embeddedAgentProviderResolution{Config: cfg, Provider: provider, RouteReason: fallbackRouteReason}, nil
}

func (r *Runner) resolveNativeAgentProvider(workDir, model string) (*embeddedAgentProviderResolution, error) {
	cfg, err := agentconfig.Load(workDir)
	if err != nil {
		return nil, fmt.Errorf("agent: native config: %w", err)
	}
	if cfg == nil || len(cfg.ProviderNames()) == 0 {
		return nil, nil
	}
	applyNativeDefaultProviderCompatibility(cfg, workDir)

	overrides := agentconfig.ProviderOverrides{}
	routeReason := "first-available"
	if model != "" {
		if modelRef, modelPin := BuiltinCatalog.NormalizeModelRef(model); modelRef != "" {
			overrides.ModelRef = modelRef
			routeReason = "catalog-match"
		} else {
			overrides.Model = modelPin
			routeReason = "direct-override"
		}
	}
	providerName := cfg.DefaultName()

	// If the model looks like a vendor/model (e.g. "anthropic/claude-sonnet-4.6",
	// "qwen/qwen3-coder-next"), prefer the "openrouter" provider if it exists
	// in the config. These models are only available through OpenRouter; local
	// providers will reject them.
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
		runCfg.MaxIterations = 100
	}

	// Apply AGENT_* env vars as overrides — same precedence as the .ddx fallback path.
	if v := os.Getenv("AGENT_PROVIDER"); v != "" {
		runCfg.Provider = v
	}
	if v := os.Getenv("AGENT_BASE_URL"); v != "" {
		runCfg.BaseURL = v
	}
	if v := os.Getenv("AGENT_API_KEY"); v != "" {
		runCfg.APIKey = v
	}
	if v := os.Getenv("AGENT_MODEL"); v != "" {
		runCfg.Model = v
	}
	if v := os.Getenv("AGENT_PRESET"); v != "" {
		runCfg.Preset = v
	}

	if !containsString(prompt.PresetNames(), runCfg.Preset) {
		return nil, fmt.Errorf("agent: unsupported preset %q; supported presets: %s", runCfg.Preset, strings.Join(prompt.PresetNames(), ", "))
	}

	return &embeddedAgentProviderResolution{
		Config:      runCfg,
		Provider:    provider,
		RouteReason: routeReason,
	}, nil
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

// isOpenRouterModel returns true if the model name looks like a vendor/model
// format (e.g. "qwen/qwen3-coder-next", "anthropic/claude-sonnet-4.6"). These
// should be configured in .ddx/config.yaml under agent_runner.models or the
// native agent config (~/.config/agent/config.yaml).
func isOpenRouterModel(model string) bool {
	if model == "" {
		return false
	}
	return strings.Contains(model, "/")
}

// writeCapableTools lists tools that count as write activity for stall
// detection. bash is included because the dedicated ls/find/grep tools now
// cover read-only shell use cases — bash is assumed to be a mutation.
var writeCapableTools = map[string]bool{
	"bash":  true,
	"write": true,
	"edit":  true,
}

// isWriteCapableTool reports whether the named tool counts as a write action
// for stall detection purposes.
func isWriteCapableTool(name string) bool {
	return writeCapableTools[name]
}

// findTool is a thin wrapper around tool.GlobTool that exposes the tool as
// "find" to the model, so agents have an intuitive name for locating files by
// pattern instead of shelling out through bash.
type findTool struct {
	Glob tool.GlobTool
}

func (t *findTool) Name() string { return "find" }
func (t *findTool) Description() string {
	return "Find files matching a glob pattern. Use instead of the find/ls shell commands to locate files by name."
}
func (t *findTool) Schema() json.RawMessage { return t.Glob.Schema() }
func (t *findTool) Parallel() bool          { return t.Glob.Parallel() }
func (t *findTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	return t.Glob.Execute(ctx, params)
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
