package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	}
}

// Run invokes a single agent harness and returns the result.
func (r *Runner) Run(opts RunOptions) (*Result, error) {
	// Resolve harness
	harnessName := opts.Harness
	if harnessName == "" {
		harnessName = r.Config.Harness
	}
	harness, ok := r.Registry.Get(harnessName)
	if !ok {
		return nil, fmt.Errorf("agent: unknown harness: %s", harnessName)
	}

	// Check availability
	if _, err := exec.LookPath(harness.Binary); err != nil {
		return nil, fmt.Errorf("agent: harness %s not available: %s not found in PATH", harnessName, harness.Binary)
	}

	// Resolve prompt
	prompt := opts.Prompt
	if opts.PromptFile != "" {
		data, err := os.ReadFile(opts.PromptFile)
		if err != nil {
			return nil, fmt.Errorf("agent: read prompt file: %w", err)
		}
		prompt = string(data)
	}
	if prompt == "" {
		return nil, fmt.Errorf("agent: prompt is required")
	}

	// Resolve model
	model := opts.Model
	if model == "" {
		if m, ok := r.Config.Models[harnessName]; ok {
			model = m
		} else {
			model = r.Config.Model
		}
	}

	// Resolve timeout
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = time.Duration(r.Config.TimeoutMS) * time.Millisecond
	}

	// Build command
	args := append([]string{}, harness.Args...)

	// Add working directory if supported
	if opts.WorkDir != "" && harness.WorkDirFlag != "" {
		args = append(args, harness.WorkDirFlag, opts.WorkDir)
	}

	// Add model flag
	if model != "" && harness.ModelFlag != "" {
		args = append(args, harness.ModelFlag, model)
	}

	// Add effort flag
	if opts.Effort != "" && harness.EffortFlag != "" {
		if harnessName == "codex" {
			// codex uses -c reasoning.effort=<value>
			args = append(args, harness.EffortFlag, "reasoning.effort="+opts.Effort)
		} else {
			args = append(args, harness.EffortFlag, opts.Effort)
		}
	}

	// Add prompt based on mode
	switch harness.PromptMode {
	case "arg":
		args = append(args, prompt)
	}

	// Execute
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, harness.Binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// For stdin mode, pipe the prompt
	if harness.PromptMode == "stdin" {
		cmd.Stdin = strings.NewReader(prompt)
	}

	err := cmd.Run()
	elapsed := time.Since(start)

	result := &Result{
		Harness:    harnessName,
		Model:      model,
		DurationMS: int(elapsed.Milliseconds()),
		Output:     stdout.String(),
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = fmt.Sprintf("timeout after %s", timeout)
			result.ExitCode = -1
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Error = stderr.String()
		} else {
			result.Error = err.Error()
			result.ExitCode = -1
		}
	}

	// Extract token count from output
	result.Tokens = extractTokens(result.Output, harnessName)

	// Log session
	r.logSession(result, len(prompt))

	return result, nil
}

// extractTokens attempts to parse token usage from agent output.
func extractTokens(output, harness string) int {
	switch harness {
	case "codex":
		// Codex outputs "tokens used\nNNN" at the end
		re := regexp.MustCompile(`tokens used\n([0-9,]+)`)
		matches := re.FindStringSubmatch(output)
		if len(matches) > 1 {
			cleaned := strings.ReplaceAll(matches[1], ",", "")
			n, _ := strconv.Atoi(cleaned)
			return n
		}
	case "claude":
		// Claude may output token info in various formats
		re := regexp.MustCompile(`(?i)tokens?[:\s]+([0-9,]+)`)
		matches := re.FindStringSubmatch(output)
		if len(matches) > 1 {
			cleaned := strings.ReplaceAll(matches[1], ",", "")
			n, _ := strconv.Atoi(cleaned)
			return n
		}
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
