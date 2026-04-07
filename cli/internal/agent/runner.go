package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// Runner executes agent invocations.
type Runner struct {
	Registry      *Registry
	Config        Config
	Executor      Executor     // injected; defaults to OSExecutor
	LookPath      LookPathFunc // injected; defaults to exec.LookPath
	ForgeProvider interface{}  // injected forge.Provider for testing; nil = resolve from config
}

// NewRunner creates a runner with defaults.
func NewRunner(cfg Config) *Runner {
	if cfg.Harness == "" {
		cfg.Harness = DefaultHarness
	}
	if cfg.TimeoutMS == 0 {
		cfg.TimeoutMS = DefaultTimeoutMS
	}
	if cfg.SessionLogDir == "" {
		cfg.SessionLogDir = DefaultLogDir
	}
	return &Runner{
		Registry: NewRegistry(),
		Config:   cfg,
		Executor: &OSExecutor{},
		LookPath: DefaultLookPath,
	}
}

// Run invokes a single agent harness and returns the result.
func (r *Runner) Run(opts RunOptions) (*Result, error) {
	harness, harnessName, err := r.resolveHarness(opts)
	if err != nil {
		return nil, err
	}

	// Virtual harness: replay from dictionary instead of executing a binary.
	if harnessName == "virtual" {
		return r.RunVirtual(opts)
	}

	// Forge harness: run in-process via the forge library.
	if harnessName == "forge" {
		return r.RunForge(opts)
	}

	prompt, err := r.resolvePrompt(opts)
	if err != nil {
		return nil, err
	}

	model := r.resolveModel(opts, harnessName)

	// Warn on unknown model
	if model != "" && len(harness.Models) > 0 && !containsString(harness.Models, model) {
		fmt.Fprintf(os.Stderr, "agent: model %q is not a known model for harness %q; available models: %s\n",
			model, harnessName, strings.Join(harness.Models, ", "))
	}

	// Warn on unknown effort
	if opts.Effort != "" {
		levels := r.resolveReasoningLevels(harnessName, harness)
		if len(levels) > 0 && !containsString(levels, opts.Effort) {
			fmt.Fprintf(os.Stderr, "agent: effort %q is not a known reasoning level for harness %q; available levels: %s\n",
				opts.Effort, harnessName, strings.Join(levels, ", "))
		}
	}

	timeout := r.resolveTimeout(opts)

	// Build args with the resolved prompt (may have come from file)
	resolvedOpts := opts
	resolvedOpts.Prompt = prompt
	resolvedOpts.Permissions = resolvePermissions(r.Config.Permissions, opts.Permissions)
	args := BuildArgs(harness, resolvedOpts, model)
	stdin := ""
	if harness.PromptMode == "stdin" {
		stdin = prompt
	}

	// Execute
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	execResult, execErr := r.Executor.Execute(ctx, harness.Binary, args, stdin)
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

// Capabilities reports the model and reasoning options for a harness.
func (r *Runner) Capabilities(name string) (*HarnessCapabilities, error) {
	harness, harnessName, err := r.resolveHarness(RunOptions{Harness: name})
	if err != nil {
		return nil, err
	}

	caps := &HarnessCapabilities{
		Harness:         harnessName,
		Available:       true,
		Binary:          harness.Binary,
		ReasoningLevels: r.resolveReasoningLevels(harnessName, harness),
	}
	if path, err := r.LookPath(harness.Binary); err == nil {
		caps.Path = path
	}

	model := r.resolveModel(RunOptions{}, harnessName)
	if model == "" {
		model = harness.DefaultModel
	}
	if model != "" {
		caps.Model = model
		caps.Models = []string{model}
	}

	return caps, nil
}

// resolveHarness looks up the harness by name and checks availability.
func (r *Runner) resolveHarness(opts RunOptions) (Harness, string, error) {
	name := opts.Harness
	if name == "" {
		name = r.Config.Harness
	}
	harness, ok := r.Registry.Get(name)
	if !ok {
		return Harness{}, "", fmt.Errorf("agent: unknown harness: %s", name)
	}
	// Embedded harnesses don't need a binary in PATH.
	if name != "virtual" && name != "forge" {
		if _, err := r.LookPath(harness.Binary); err != nil {
			return Harness{}, "", fmt.Errorf("agent: harness %s not available: %s not found in PATH", name, harness.Binary)
		}
	}
	return harness, name, nil
}

// resolvePrompt reads the prompt from text or file.
func (r *Runner) resolvePrompt(opts RunOptions) (string, error) {
	prompt := opts.Prompt
	if opts.PromptFile != "" {
		data, err := os.ReadFile(opts.PromptFile)
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

// resolveModel picks the model from opts, per-harness config, or global config.
func (r *Runner) resolveModel(opts RunOptions, harnessName string) string {
	if opts.Model != "" {
		return opts.Model
	}
	if m, ok := r.Config.Models[harnessName]; ok {
		return m
	}
	if r.Config.Model != "" {
		return r.Config.Model
	}
	return ""
}

func (r *Runner) resolveReasoningLevels(harnessName string, harness Harness) []string {
	if r.Config.ReasoningLevels != nil {
		if levels, ok := r.Config.ReasoningLevels[harnessName]; ok && len(levels) > 0 {
			return append([]string{}, levels...)
		}
	}
	if len(harness.ReasoningLevels) > 0 {
		return append([]string{}, harness.ReasoningLevels...)
	}
	return []string{}
}

// resolveTimeout picks the timeout from opts or config.
func (r *Runner) resolveTimeout(opts RunOptions) time.Duration {
	if opts.Timeout > 0 {
		return opts.Timeout
	}
	return time.Duration(r.Config.TimeoutMS) * time.Millisecond
}

// resolvePermissions returns the effective permission level, defaulting to "safe".
func resolvePermissions(cfgPerms, optsPerms string) string {
	if optsPerms != "" {
		return optsPerms
	}
	if cfgPerms != "" {
		return cfgPerms
	}
	return "safe"
}

// BuildArgs constructs the argument array for a harness invocation.
// Exported for testing.
func BuildArgs(h Harness, opts RunOptions, model string) []string {
	// Use BaseArgs if set, fall back to legacy Args for compatibility.
	base := h.BaseArgs
	if base == nil {
		base = h.Args
	}
	args := append([]string{}, base...)

	// Append permission-specific args.
	if h.PermissionArgs != nil {
		level := opts.Permissions
		if level == "" {
			level = "safe"
		}
		if extra, ok := h.PermissionArgs[level]; ok {
			args = append(args, extra...)
		}
	}

	if opts.WorkDir != "" && h.WorkDirFlag != "" {
		args = append(args, h.WorkDirFlag, opts.WorkDir)
	}
	if model != "" && h.ModelFlag != "" {
		args = append(args, h.ModelFlag, model)
	}
	if opts.Effort != "" && h.EffortFlag != "" {
		if h.EffortFormat != "" {
			args = append(args, h.EffortFlag, fmt.Sprintf(h.EffortFormat, opts.Effort))
		} else {
			args = append(args, h.EffortFlag, opts.Effort)
		}
	}
	if h.PromptMode == "arg" {
		args = append(args, opts.Prompt)
	}
	return args
}

// processResult converts execution output to a Result.
func (r *Runner) processResult(harnessName, model string, harness Harness, execResult *ExecResult, execErr error, elapsed time.Duration, ctx context.Context) *Result {
	result := &Result{
		Harness:    harnessName,
		Model:      model,
		DurationMS: int(elapsed.Milliseconds()),
	}

	if execResult != nil {
		result.Output = execResult.Stdout
		result.CondensedOutput = CondenseOutput(result.Output, "")
		result.Stderr = execResult.Stderr
		result.ExitCode = execResult.ExitCode
	}

	if execErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
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
	result.OutputTokens = usage.OutputTokens
	result.CostUSD = usage.CostUSD
	return result
}

// UsageData holds structured token usage from a structured agent output.
type UsageData struct {
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// ExtractUsage parses structured token usage from agent output.
// For codex, it scans JSONL output for a turn.completed event and reads the usage object.
// For claude, it parses the --output-format json envelope (whole output or last non-empty line).
// Returns zero-value UsageData if parsing fails or the harness is unsupported.
func ExtractUsage(harnessName string, output string) UsageData {
	switch harnessName {
	case "codex":
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || !strings.Contains(line, `"turn.completed"`) {
				continue
			}
			var event struct {
				Type  string `json:"type"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			if event.Type == "turn.completed" {
				return UsageData{
					InputTokens:  event.Usage.InputTokens,
					OutputTokens: event.Usage.OutputTokens,
				}
			}
		}
		return UsageData{}
	case "claude":
		var envelope struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			TotalCostUSD float64 `json:"total_cost_usd"`
		}
		// Try whole output first, then fall back to last non-empty line.
		if err := json.Unmarshal([]byte(output), &envelope); err != nil {
			lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
			last := ""
			for i := len(lines) - 1; i >= 0; i-- {
				if strings.TrimSpace(lines[i]) != "" {
					last = lines[i]
					break
				}
			}
			if last == "" {
				return UsageData{}
			}
			if err2 := json.Unmarshal([]byte(last), &envelope); err2 != nil {
				return UsageData{}
			}
		}
		if envelope.Usage.InputTokens == 0 && envelope.Usage.OutputTokens == 0 && envelope.TotalCostUSD == 0 {
			return UsageData{}
		}
		return UsageData{
			InputTokens:  envelope.Usage.InputTokens,
			OutputTokens: envelope.Usage.OutputTokens,
			CostUSD:      envelope.TotalCostUSD,
		}
	case "opencode":
		// opencode -f json emits a JSON object; parse usage fields if present.
		var envelope struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			TotalCostUSD float64 `json:"total_cost_usd"`
		}
		if err := json.Unmarshal([]byte(output), &envelope); err != nil {
			// Try last non-empty line (in case of preamble).
			lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
			last := ""
			for i := len(lines) - 1; i >= 0; i-- {
				if strings.TrimSpace(lines[i]) != "" {
					last = lines[i]
					break
				}
			}
			if last == "" {
				return UsageData{}
			}
			if err2 := json.Unmarshal([]byte(last), &envelope); err2 != nil {
				return UsageData{}
			}
		}
		if envelope.Usage.InputTokens == 0 && envelope.Usage.OutputTokens == 0 && envelope.TotalCostUSD == 0 {
			return UsageData{}
		}
		return UsageData{
			InputTokens:  envelope.Usage.InputTokens,
			OutputTokens: envelope.Usage.OutputTokens,
			CostUSD:      envelope.TotalCostUSD,
		}
	default:
		return UsageData{}
	}
}

// ExtractTokens parses token usage from agent output using the harness's pattern.
// For codex, it delegates to ExtractUsage and returns total tokens (input + output).
func ExtractTokens(output string, harness Harness) int {
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
	if len(matches) > 1 {
		cleaned := strings.ReplaceAll(matches[1], ",", "")
		n, _ := strconv.Atoi(cleaned)
		return n
	}
	return 0
}

// logSession writes a session entry to the log directory.
func (r *Runner) logSession(result *Result, promptLen int, prompt, promptSource string, correlation map[string]string) {
	dir := r.Config.SessionLogDir
	if dir == "" {
		return
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return // best effort
	}

	id := genSessionID()
	entry := SessionEntry{
		ID:           id,
		Timestamp:    time.Now().UTC(),
		Harness:      result.Harness,
		Model:        result.Model,
		PromptLen:    promptLen,
		Prompt:       prompt,
		PromptSource: promptSource,
		Response:     result.Output,
		Correlation:  correlation,
		Stderr:       result.Stderr,
		Tokens:       result.Tokens,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		CostUSD:      result.CostUSD,
		Duration:     result.DurationMS,
		ExitCode:     result.ExitCode,
		Error:        result.Error,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	logFile := filepath.Join(dir, "sessions.jsonl")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\n", data)
}

func genSessionID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "as-" + hex.EncodeToString(b)
}
