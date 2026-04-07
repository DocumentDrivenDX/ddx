package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DocumentDrivenDX/forge"
	"github.com/DocumentDrivenDX/forge/prompt"
	"github.com/DocumentDrivenDX/forge/provider/anthropic"
	oai "github.com/DocumentDrivenDX/forge/provider/openai"
	"github.com/DocumentDrivenDX/forge/provider/virtual"
	"github.com/DocumentDrivenDX/forge/session"
	"github.com/DocumentDrivenDX/forge/tool"
)

// ForgeConfig holds configuration for the embedded forge executor.
type ForgeConfig struct {
	Provider      string `yaml:"provider"`       // "openai-compat" or "anthropic"
	BaseURL       string `yaml:"base_url"`       // provider endpoint
	APIKey        string `yaml:"api_key"`        // API key
	Model         string `yaml:"model"`          // model name
	MaxIterations int    `yaml:"max_iterations"` // max tool-call rounds
}

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

	// Use injected provider (testing) or build from config/env.
	var provider forge.Provider
	maxIter := 20
	if r.ForgeProvider != nil {
		provider = r.ForgeProvider.(forge.Provider)
	} else {
		forgeCfg := r.resolveForgeConfig(model)
		var err error
		provider, err = buildForgeProvider(forgeCfg)
		if err != nil {
			return nil, fmt.Errorf("agent: forge provider: %w", err)
		}
		if forgeCfg.MaxIterations > 0 {
			maxIter = forgeCfg.MaxIterations
		}
	}

	// Build tools
	tools := []forge.Tool{
		&tool.ReadTool{WorkDir: wd},
		&tool.WriteTool{WorkDir: wd},
		&tool.EditTool{WorkDir: wd},
		&tool.BashTool{WorkDir: wd},
	}

	// Build system prompt
	sysPrompt := prompt.New("You are an expert coding assistant. You help users by reading files, executing commands, editing code, and writing new files. Use the available tools to complete tasks.").
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

	result := &Result{
		Harness:      "forge",
		Model:        forgeResult.Model,
		Output:       forgeResult.Output,
		InputTokens:  forgeResult.Tokens.Input,
		OutputTokens: forgeResult.Tokens.Output,
		Tokens:       forgeResult.Tokens.Total,
		DurationMS:   int(elapsed.Milliseconds()),
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

// resolveForgeConfig builds a ForgeConfig from environment and runner config.
func (r *Runner) resolveForgeConfig(model string) ForgeConfig {
	cfg := ForgeConfig{
		Provider:      envOrDefault("FORGE_PROVIDER", "openai-compat"),
		BaseURL:       envOrDefault("FORGE_BASE_URL", "http://localhost:1234/v1"),
		APIKey:        os.Getenv("FORGE_API_KEY"),
		Model:         model,
		MaxIterations: 20,
	}
	if v := os.Getenv("FORGE_MODEL"); v != "" && cfg.Model == "" {
		cfg.Model = v
	}
	return cfg
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// buildForgeProvider creates a forge.Provider from config.
func buildForgeProvider(cfg ForgeConfig) (forge.Provider, error) {
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
