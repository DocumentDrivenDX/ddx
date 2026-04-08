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

// ForgeConfigFunc returns forge config from .ddx/config.yaml. Nil return = no config.
type ForgeConfigFunc func() *ForgeYAMLConfig

// ForgeYAMLConfig mirrors config.ForgeConfig without importing the config package.
type ForgeYAMLConfig struct {
	Provider      string
	BaseURL       string
	APIKey        string
	Model         string
	Preset        string
	MaxIterations int
}

// Runner executes agent invocations.
type Runner struct {
	Registry          *Registry
	Config            Config
	Executor          Executor         // injected; defaults to OSExecutor
	LookPath          LookPathFunc     // injected; defaults to exec.LookPath
	ForgeProvider     interface{}      // injected forge.Provider for testing; nil = resolve from config
	ForgeConfigLoader ForgeConfigFunc  // injected; loads forge config from .ddx/config.yaml
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

	// Use ExecuteInDir when WorkDir is set — this handles harnesses like
	// claude that have no --cwd flag by setting cmd.Dir on the subprocess.
	execDir := ""
	if opts.WorkDir != "" && harness.WorkDirFlag == "" {
		execDir = opts.WorkDir
	}
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

// ResolvePreset resolves a harness + preset to a complete runnable config.
func (r *Runner) ResolvePreset(harness string, preset Preset) (*PresetDefinition, error) {
	h, _, err := r.resolveHarness(RunOptions{Harness: harness})
	if err != nil {
		return nil, err
	}
	def, err := ResolvePreset(harness, preset)
	if err != nil {
		return nil, err
	}
	def.Harness = harness
	def = ResolvePresetWithOverrides(def, r.Config)
	if def.Effort != "" {
		levels := r.resolveReasoningLevels(harness, h)
		if len(levels) > 0 && !containsString(levels, def.Effort) {
			def.Effort = levels[len(levels)-1]
		}
	}
	return &def, nil
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
	case "pi":
		// pi outputs JSONL - cost is in intermediate events, summary at end has no cost
		// Scan backwards to find the last line with cost data
		lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
		var inputTokens, outputTokens int
		var costUSD float64
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			// Try to parse as pi event with usage/cost
			// Format: {"type":"text_end","message":{"usage":{"input":N,"output":M,"cost":{"total":X}}}}
			var event struct {
				Type    string `json:"type"`
				Message struct {
					Usage struct {
						Input  int `json:"input"`
						Output int `json:"output"`
						Cost   struct {
							Total float64 `json:"total"`
						} `json:"cost"`
					} `json:"usage"`
				} `json:"message"`
				Partial struct {
					Usage struct {
						Input  int `json:"input"`
						Output int `json:"output"`
						Cost   struct {
							Total float64 `json:"total"`
						} `json:"cost"`
					} `json:"usage"`
				} `json:"partial"`
			}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			// Check message.usage first, then partial.usage
			if event.Message.Usage.Input > 0 || event.Message.Usage.Output > 0 {
				inputTokens = event.Message.Usage.Input
				outputTokens = event.Message.Usage.Output
				costUSD = event.Message.Usage.Cost.Total
				break
			}
			if event.Partial.Usage.Input > 0 || event.Partial.Usage.Output > 0 {
				inputTokens = event.Partial.Usage.Input
				outputTokens = event.Partial.Usage.Output
				costUSD = event.Partial.Usage.Cost.Total
				break
			}
		}
		return UsageData{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			CostUSD:      costUSD,
		}
	case "gemini":
		// gemini outputs single JSON with stats.models[].tokens (no cost in JSON)
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
		var envelope struct {
			Stats struct {
				Models map[string]struct {
					Tokens struct {
						Input int `json:"input"`
						Total int `json:"total"`
					} `json:"tokens"`
				} `json:"models"`
			} `json:"stats"`
		}
		if err := json.Unmarshal([]byte(last), &envelope); err != nil {
			return UsageData{}
		}
		inputTokens := 0
		outputTokens := 0
		for _, model := range envelope.Stats.Models {
			inputTokens += model.Tokens.Input
			outputTokens += model.Tokens.Total - model.Tokens.Input
		}
		// Gemini JSON output doesn't include cost
		return UsageData{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			CostUSD:      0,
		}
	default:
		return UsageData{}
	}
}

// ExtractOutput extracts clean text from raw agent output based on harness format.
// For codex: scans JSONL for type=item.completed with item.type=agent_message, returns item.text
// For claude: parses JSON envelope and returns the 'result' field
// For forge/opencode: returns output as-is (no transformation needed)
// For unknown harnesses or malformed input: returns output as-is
func ExtractOutput(harnessName string, rawOutput string) string {
	switch harnessName {
	case "codex":
		return extractOutputCodex(rawOutput)
	case "claude":
		return extractOutputClaude(rawOutput)
	case "forge", "opencode":
		// forge and opencode return clean text directly
		return rawOutput
	case "pi", "gemini":
		return extractOutputPiGemini(rawOutput)
	default:
		// Unknown harnesses return output as-is
		return rawOutput
	}
}

func extractOutputPiGemini(rawOutput string) string {
	// pi outputs JSONL (last line has summary), gemini outputs single JSON
	// Try to parse the last non-empty line for the response field
	lines := strings.Split(strings.TrimRight(rawOutput, "\n"), "\n")
	last := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			last = lines[i]
			break
		}
	}
	if last == "" {
		return rawOutput
	}
	var envelope struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal([]byte(last), &envelope); err != nil {
		return rawOutput
	}
	return envelope.Response
}

func extractOutputCodex(rawOutput string) string {
	for _, line := range strings.Split(rawOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item struct {
			Type string `json:"type"`
			Item struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		if item.Type == "output" && item.Item.Type == "agent_message" {
			return item.Item.Text
		}
	}
	return rawOutput
}

func extractOutputClaude(rawOutput string) string {
	var envelope struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(rawOutput), &envelope); err == nil {
		return envelope.Result
	}
	// Try last non-empty line (in case of preamble)
	lines := strings.Split(strings.TrimRight(rawOutput, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			var envelope struct {
				Result string `json:"result"`
			}
			if err := json.Unmarshal([]byte(line), &envelope); err == nil {
				return envelope.Result
			}
		}
	}
	return rawOutput
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
