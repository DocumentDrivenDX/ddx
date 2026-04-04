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

// Runner executes agent invocations.
type Runner struct {
	Registry *Registry
	Config   Config
	Executor Executor   // injected; defaults to OSExecutor
	LookPath LookPathFunc // injected; defaults to exec.LookPath
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

	prompt, err := r.resolvePrompt(opts)
	if err != nil {
		return nil, err
	}

	model := r.resolveModel(opts, harnessName)
	timeout := r.resolveTimeout(opts)

	// Build args with the resolved prompt (may have come from file)
	resolvedOpts := opts
	resolvedOpts.Prompt = prompt
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
	r.logSession(result, len(prompt))
	return result, nil
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
	if _, err := r.LookPath(harness.Binary); err != nil {
		return Harness{}, "", fmt.Errorf("agent: harness %s not available: %s not found in PATH", name, harness.Binary)
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
	return r.Config.Model
}

// resolveTimeout picks the timeout from opts or config.
func (r *Runner) resolveTimeout(opts RunOptions) time.Duration {
	if opts.Timeout > 0 {
		return opts.Timeout
	}
	return time.Duration(r.Config.TimeoutMS) * time.Millisecond
}

// BuildArgs constructs the argument array for a harness invocation.
// Exported for testing.
func BuildArgs(h Harness, opts RunOptions, model string) []string {
	args := append([]string{}, h.Args...)

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
	return result
}

// ExtractTokens parses token usage from agent output using the harness's pattern.
func ExtractTokens(output string, harness Harness) int {
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
func (r *Runner) logSession(result *Result, promptLen int) {
	dir := r.Config.SessionLogDir
	if dir == "" {
		return
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return // best effort
	}

	id := genSessionID()
	entry := SessionEntry{
		ID:        id,
		Timestamp: time.Now().UTC(),
		Harness:   result.Harness,
		Model:     result.Model,
		PromptLen: promptLen,
		Tokens:    result.Tokens,
		Duration:  result.DurationMS,
		ExitCode:  result.ExitCode,
		Error:     result.Error,
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
	rand.Read(b)
	return "as-" + hex.EncodeToString(b)
}
