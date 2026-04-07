package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// BenchmarkPrompt is a single test case in a benchmark suite.
type BenchmarkPrompt struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Prompt      string   `json:"prompt"`               // inline prompt text
	PromptFile  string   `json:"prompt_file,omitempty"` // or path to prompt file
	Tags        []string `json:"tags,omitempty"`        // e.g. ["read-comprehension", "coding", "analysis"]
	MaxTokens   int      `json:"max_tokens,omitempty"`  // expected max output tokens (for budget)
}

// BenchmarkSuite defines a repeatable set of comparison runs.
type BenchmarkSuite struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Version     string            `json:"version"`
	Arms        []BenchmarkArm    `json:"arms"`
	Prompts     []BenchmarkPrompt `json:"prompts"`
	Sandbox     bool              `json:"sandbox,omitempty"`
	PostRun     string            `json:"post_run,omitempty"`
	Timeout     string            `json:"timeout,omitempty"` // e.g. "5m"
}

// BenchmarkResult is the output of running a full benchmark suite.
type BenchmarkResult struct {
	Suite       string              `json:"suite"`
	Version     string              `json:"version"`
	Timestamp   time.Time           `json:"timestamp"`
	Arms        []BenchmarkArm      `json:"arms"`
	Comparisons []ComparisonRecord  `json:"comparisons"`
	Summary     BenchmarkSummary    `json:"summary"`
}

// BenchmarkArmSummary aggregates stats for one arm across all prompts.
type BenchmarkArmSummary struct {
	Label        string  `json:"label"`
	Completed    int     `json:"completed"`
	Failed       int     `json:"failed"`
	TotalTokens  int     `json:"total_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	AvgDurationMS int    `json:"avg_duration_ms"`
	AvgScore     float64 `json:"avg_score,omitempty"` // populated after grading
}

// BenchmarkSummary aggregates stats across all arms and prompts.
type BenchmarkSummary struct {
	TotalPrompts int                   `json:"total_prompts"`
	Arms         []BenchmarkArmSummary `json:"arms"`
}

// LoadBenchmarkSuite reads a benchmark suite from a JSON file.
func LoadBenchmarkSuite(path string) (*BenchmarkSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading benchmark suite: %w", err)
	}
	var suite BenchmarkSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("parsing benchmark suite: %w", err)
	}
	return &suite, nil
}

// RunBenchmark executes all prompts in a suite against all arms.
func (r *Runner) RunBenchmark(suite *BenchmarkSuite) (*BenchmarkResult, error) {
	result := &BenchmarkResult{
		Suite:     suite.Name,
		Version:   suite.Version,
		Timestamp: time.Now().UTC(),
		Arms:      suite.Arms,
	}

	var timeout time.Duration
	if suite.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(suite.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout %q: %w", suite.Timeout, err)
		}
	}

	for _, prompt := range suite.Prompts {
		promptText := prompt.Prompt
		promptFile := prompt.PromptFile
		if promptText == "" && promptFile != "" {
			data, err := os.ReadFile(promptFile)
			if err != nil {
				return nil, fmt.Errorf("reading prompt file %s: %w", promptFile, err)
			}
			promptText = string(data)
		}

		baseOpts := RunOptions{
			Prompt:  promptText,
			Timeout: timeout,
		}

		compareOpts := BenchmarkArmsToCompare(suite.Arms, baseOpts)
		compareOpts.Sandbox = suite.Sandbox
		compareOpts.PostRun = suite.PostRun

		record, err := r.RunCompare(compareOpts)
		if err != nil {
			return nil, fmt.Errorf("prompt %s: %w", prompt.ID, err)
		}

		result.Comparisons = append(result.Comparisons, *record)
	}

	result.Summary = summarizeBenchmark(result)
	return result, nil
}

// summarizeBenchmark computes per-arm aggregates.
func summarizeBenchmark(result *BenchmarkResult) BenchmarkSummary {
	summary := BenchmarkSummary{
		TotalPrompts: len(result.Comparisons),
	}

	// Index arms by label
	armStats := make(map[string]*BenchmarkArmSummary)
	armOrder := make([]string, len(result.Arms))
	for i, arm := range result.Arms {
		label := arm.Label
		armOrder[i] = label
		armStats[label] = &BenchmarkArmSummary{Label: label}
	}

	for _, cmp := range result.Comparisons {
		for _, arm := range cmp.Arms {
			stats, ok := armStats[arm.Harness]
			if !ok {
				continue
			}
			if arm.ExitCode == 0 {
				stats.Completed++
			} else {
				stats.Failed++
			}
			stats.TotalTokens += arm.Tokens
			stats.TotalCostUSD += arm.CostUSD
			stats.AvgDurationMS += arm.DurationMS
		}
	}

	for _, label := range armOrder {
		stats := armStats[label]
		total := stats.Completed + stats.Failed
		if total > 0 {
			stats.AvgDurationMS = stats.AvgDurationMS / total
		}
		summary.Arms = append(summary.Arms, *stats)
	}

	return summary
}

// SaveBenchmarkResult writes a benchmark result to a JSON file.
func SaveBenchmarkResult(path string, result *BenchmarkResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling result: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
