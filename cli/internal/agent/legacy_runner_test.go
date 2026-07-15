package agent

// This file intentionally contains the retired DDx-owned concrete harness
// runner used by legacy unit tests. Production dispatch is service-only: it
// sends an opaque Fizeau request and never selects or invokes a concrete
// harness/model itself. Keeping this adapter in a _test.go file preserves
// focused argv/parser coverage without exposing a second production router.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

const DefaultHarness = "codex"

// Config is the retired DDx-owned runner configuration. It remains test-local
// solely for legacy argv/parser fixtures; production routing configuration is
// represented by config.ResolvedConfig and interpreted by Fizeau.
type Config struct {
	Profile         string
	Harness         string
	Model           string
	Models          map[string]string
	ReasoningLevels map[string][]string
	TimeoutMS       int
	WallClockMS     int
	SessionLogDir   string
	Permissions     string
}

type Runner struct {
	registry *harnessRegistry
	Config   Config
	Executor Executor
	LookPath LookPathFunc
	WorkDir  string
}

func NewRunner(cfg Config) *Runner {
	if cfg.Harness == "" {
		cfg.Harness = DefaultHarness
	}
	if cfg.TimeoutMS == 0 {
		cfg.TimeoutMS = DefaultTimeoutMS
	}
	if cfg.WallClockMS == 0 {
		cfg.WallClockMS = DefaultWallClockMS
	}
	if cfg.SessionLogDir == "" {
		cfg.SessionLogDir = DefaultLogDir
	}
	r := &Runner{
		registry: newHarnessRegistry(),
		Config:   cfg,
		Executor: &OSExecutor{},
		LookPath: DefaultLookPath,
	}
	r.registry.LookPath = func(file string) (string, error) {
		if r.LookPath != nil {
			return r.LookPath(file)
		}
		return DefaultLookPath(file)
	}
	return r
}

func (r *Runner) Run(opts RunArgs) (*Result, error) {
	harness, harnessName, err := r.resolveHarness(opts)
	if err != nil {
		return nil, err
	}
	if harnessName == "agent" || harness.IsHTTPProvider {
		return nil, fmt.Errorf("legacy test runner cannot dispatch service-managed harness %q", harnessName)
	}

	prompt, err := r.resolvePrompt(opts)
	if err != nil {
		return nil, err
	}
	model := r.resolveModel(opts, harnessName)
	timeout := r.resolveTimeout(opts)
	wallClock := r.resolveWallClock(opts)
	resolvedOpts := opts
	resolvedOpts.Prompt = prompt
	resolvedOpts.Permissions = resolvePermissions(r.Config.Permissions, opts.Permissions)
	args := BuildArgs(harness, buildArgsInputFromRunArgs(resolvedOpts), model)
	stdin := ""
	if harness.PromptMode == "stdin" {
		stdin = prompt
	}

	parentCtx := opts.Context
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()
	ctx = withExecutionEnv(ctx, opts.Env)
	ctx = withExecutionTimeout(ctx, timeout)
	ctx = withExecutionWallClock(ctx, wallClock)

	execDir := ""
	if opts.WorkDir != "" && harness.WorkDirFlag == "" {
		execDir = opts.WorkDir
	}
	start := time.Now()
	execResult, execErr := r.Executor.ExecuteInDir(ctx, harness.Binary, args, stdin, execDir)
	elapsed := time.Since(start)
	result := r.processResult(harnessName, model, harness, execResult, execErr, elapsed, ctx)
	promptSource := opts.PromptSource
	if promptSource == "" {
		if opts.PromptFile != "" {
			promptSource = opts.PromptFile
		} else {
			promptSource = "inline"
		}
	}
	r.logSession(result, len(prompt), prompt, promptSource, opts.Correlation)
	return result, nil
}

func (r *Runner) resolveHarness(opts RunArgs) (harnessConfig, string, error) {
	name := opts.Harness
	if name == "" && opts.Provider != "" {
		resolved := resolveHarnessAlias(opts.Provider)
		if r.registry.Has(resolved) {
			name = resolved
		}
	}
	if name == "" {
		name = r.Config.Harness
	}
	harness, ok := r.registry.Get(name)
	if !ok {
		return harnessConfig{}, "", fmt.Errorf("agent: unknown harness: %s", name)
	}
	if name != "virtual" && name != "agent" && name != "script" && !harness.IsHTTPProvider {
		if _, err := r.LookPath(harness.Binary); err != nil {
			return harnessConfig{}, "", fmt.Errorf("agent: harness %s not available: %s not found in PATH", name, harness.Binary)
		}
	}
	return harness, name, nil
}

func (r *Runner) resolvePrompt(opts RunArgs) (string, error) {
	prompt := opts.Prompt
	if opts.PromptFile != "" {
		data, err := readPromptFileBounded(opts.PromptFile)
		if err != nil {
			return "", fmt.Errorf("agent: read prompt file: %w", err)
		}
		prompt = string(data)
	}
	if prompt == "" {
		return "", fmt.Errorf("agent: prompt is required")
	}
	return prompt, nil
}

func (r *Runner) resolveModel(opts RunArgs, harnessName string) string {
	if opts.Model != "" {
		return opts.Model
	}
	if m, ok := r.Config.Models[harnessName]; ok {
		return m
	}
	return r.Config.Model
}

func (r *Runner) resolveReasoningLevels(harnessName string, harness harnessConfig) []string {
	if levels, ok := r.Config.ReasoningLevels[harnessName]; ok && len(levels) > 0 {
		return append([]string{}, levels...)
	}
	return append([]string{}, harness.ReasoningLevels...)
}

func (r *Runner) resolveTimeout(opts RunArgs) time.Duration {
	if opts.Timeout > 0 {
		return opts.Timeout
	}
	return time.Duration(r.Config.TimeoutMS) * time.Millisecond
}

func (r *Runner) resolveWallClock(opts RunArgs) time.Duration {
	if opts.WallClock > 0 {
		return opts.WallClock
	}
	return time.Duration(r.Config.WallClockMS) * time.Millisecond
}

func resolvePermissions(cfgPerms, optsPerms string) string {
	if optsPerms != "" {
		return optsPerms
	}
	if cfgPerms != "" {
		return cfgPerms
	}
	return "safe"
}

type buildArgsInput struct {
	Prompt      string
	WorkDir     string
	Permissions string
	Effort      string
}

func buildArgsInputFromRunArgs(opts RunArgs) buildArgsInput {
	return buildArgsInput{
		Prompt:      opts.Prompt,
		WorkDir:     opts.WorkDir,
		Permissions: opts.Permissions,
		Effort:      opts.Effort,
	}
}

func BuildArgs(h harnessConfig, in buildArgsInput, model string) []string {
	base := h.BaseArgs
	if base == nil {
		base = h.Args
	}
	args := append([]string{}, base...)
	if h.PermissionArgs != nil {
		level := in.Permissions
		if level == "" {
			level = "safe"
		}
		if extra, ok := h.PermissionArgs[level]; ok {
			args = append(args, extra...)
		}
	}
	if in.WorkDir != "" && h.WorkDirFlag != "" {
		args = append(args, h.WorkDirFlag, in.WorkDir)
	}
	if model != "" && h.ModelFlag != "" {
		args = append(args, h.ModelFlag, model)
	}
	if in.Effort != "" && h.EffortFlag != "" {
		if h.EffortFormat != "" {
			args = append(args, h.EffortFlag, fmt.Sprintf(h.EffortFormat, in.Effort))
		} else {
			args = append(args, h.EffortFlag, in.Effort)
		}
	}
	if h.PromptMode == "arg" {
		args = append(args, in.Prompt)
	}
	return args
}

func (r *Runner) processResult(harnessName, model string, harness harnessConfig, execResult *ExecResult, execErr error, elapsed time.Duration, ctx context.Context) *Result {
	result := &Result{Harness: harnessName, Model: model, DurationMS: int(elapsed.Milliseconds())}
	if execResult != nil {
		result.Output = execResult.Stdout
		result.Stderr = execResult.Stderr
		result.ExitCode = execResult.ExitCode
	}
	if execResult != nil && execResult.EarlyCancel {
		result.Error = fmt.Sprintf("cancelled: auth/rate-limit detected (%s)", execResult.CancelReason)
		result.ExitCode = -1
	} else if execResult != nil && execResult.WallClockTimeout {
		reportedElapsed := execResult.WallClockElapsed
		if reportedElapsed == 0 {
			reportedElapsed = elapsed
		}
		result.Error = fmt.Sprintf("wall-clock deadline exceeded after %v", reportedElapsed.Round(time.Second))
		result.ExitCode = -1
	} else if execErr != nil {
		if errors.Is(execErr, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Error = fmt.Sprintf("timeout after %v", elapsed.Round(time.Second))
			result.ExitCode = -1
		} else {
			result.Error = execErr.Error()
			if result.ExitCode == 0 {
				result.ExitCode = -1
			}
		}
	} else if execResult != nil && execResult.ExitCode != 0 {
		result.Error = execResult.Stderr
	}
	result.Tokens = ExtractTokens(result.Output, harness)
	usage := ExtractUsage(harnessName, result.Output)
	result.InputTokens = usage.InputTokens
	result.CachedTokens = usage.CachedTokens
	result.OutputTokens = usage.OutputTokens
	result.CostUSD = usage.CostUSD
	return result
}

func ExtractTokens(output string, harness harnessConfig) int {
	if harness.Name == "codex" {
		usage := ExtractUsage("codex", output)
		if usage.InputTokens > 0 || usage.OutputTokens > 0 {
			return usage.InputTokens + usage.OutputTokens
		}
	}
	if harness.TokenPattern == "" {
		return 0
	}
	re, err := regexp.Compile(harness.TokenPattern)
	if err != nil {
		return 0
	}
	matches := re.FindStringSubmatch(output)
	if len(matches) <= 1 {
		return 0
	}
	n, _ := strconv.Atoi(strings.ReplaceAll(matches[1], ",", ""))
	return n
}

func (r *Runner) logSession(result *Result, promptLen int, prompt, promptSource string, correlation map[string]string) {
	dir := r.Config.SessionLogDir
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	canonicalTarget := result.Model
	surface := ""
	if harness, ok := r.registry.Get(result.Harness); ok {
		surface = harness.Surface
		if canonicalTarget == "" {
			canonicalTarget = harness.DefaultModel
		}
	}
	if canonicalTarget == "" {
		canonicalTarget = result.Harness
	}
	entry := SessionEntry{
		ID: genSessionID(), Timestamp: time.Now().UTC(), Harness: result.Harness,
		Provider: result.Provider, Surface: surface, CanonicalTarget: canonicalTarget,
		BaseURL: result.ResolvedBaseURL, BillingMode: billingModeFor(result.Harness, surface, result.ResolvedBaseURL),
		Model: result.Model, PromptLen: promptLen, Prompt: prompt, PromptSource: promptSource,
		Response: result.Output, Correlation: correlation, NativeSessionID: result.AgentSessionID,
		Stderr: result.Stderr, Tokens: result.Tokens, InputTokens: result.InputTokens,
		OutputTokens: result.OutputTokens, TotalTokens: result.InputTokens + result.OutputTokens,
		CostUSD: result.CostUSD, Duration: result.DurationMS, ExitCode: result.ExitCode,
		Error: result.Error, ToolCalls: append([]ToolCallEntry(nil), result.ToolCalls...),
	}
	if entry.NativeSessionID == "" && correlation != nil {
		entry.NativeSessionID = correlation["native_session_id"]
	}
	if correlation != nil {
		entry.NativeLogRef = correlation["native_log_ref"]
		entry.TraceID = correlation["trace_id"]
		entry.SpanID = correlation["span_id"]
	}
	_ = AppendSessionIndex(dir, SessionIndexEntryFromLegacy("", entry), entry.Timestamp)
}

func (r *Runner) runInternal(args RunArgs) (*Result, error) {
	return r.Run(args)
}

func runVirtualFn(r *Runner, opts RunArgs) (*Result, error) {
	prompt, err := r.resolvePrompt(opts)
	if err != nil {
		return nil, err
	}
	if envResponses := os.Getenv("DDX_VIRTUAL_RESPONSES"); envResponses != "" {
		var responses []InlineResponse
		if err := json.Unmarshal([]byte(envResponses), &responses); err != nil {
			return nil, fmt.Errorf("parsing DDX_VIRTUAL_RESPONSES: %w", err)
		}
		if ir, ok := LookupInline(responses, prompt); ok {
			return buildVirtualResultFn(r, ir.Response, ir.ExitCode, ir.Model, ir.DelayMS, 0, 0, 0, prompt, opts), nil
		}
	}
	dictDir := filepath.Join(r.Config.SessionLogDir, "..", "agent-dictionary")
	if _, err := os.Stat(VirtualDictionaryDir); err == nil {
		dictDir = VirtualDictionaryDir
	}
	var patterns []config.NormalizePattern
	if cfg, err := config.Load(); err == nil && cfg.Agent != nil && cfg.Agent.Virtual != nil {
		patterns = cfg.Agent.Virtual.Normalize
	}
	entry, err := LookupEntry(dictDir, prompt, patterns...)
	if err != nil {
		return nil, err
	}
	return buildVirtualResultFn(r, entry.Response, entry.ExitCode, entry.Model, entry.DelayMS,
		entry.InputTokens, entry.OutputTokens, entry.CostUSD, prompt, opts), nil
}

func buildVirtualResultFn(r *Runner, response string, exitCode int, model string, delayMS int,
	inputTokens, outputTokens int, costUSD float64, prompt string, opts RunArgs) *Result {
	if delayMS > 0 {
		time.Sleep(time.Duration(delayMS) * time.Millisecond)
	}
	result := &Result{
		Harness: "virtual", Model: model, Output: response, ExitCode: exitCode,
		DurationMS: delayMS, InputTokens: inputTokens, OutputTokens: outputTokens,
		Tokens: inputTokens + outputTokens, CostUSD: costUSD,
	}
	promptSource := opts.PromptSource
	if promptSource == "" {
		if opts.PromptFile != "" {
			promptSource = opts.PromptFile
		} else {
			promptSource = "inline"
		}
	}
	r.logSession(result, len(prompt), prompt, promptSource, opts.Correlation)
	return result
}
