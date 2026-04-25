package agent

// compare_adapter.go provides comparison, quorum, and benchmark execution
// helpers for DDx. The compare/quorum/benchmark execution logic accepts a
// run-function closure so production callers can drive arms via the agent
// service (RunViaService) while legacy Runner-bound tests can keep using
// Runner.Run.
//
// The Runner.RunCompare/Runner.RunQuorum/Runner.RunBenchmark wrappers below
// remain in place for test scaffolding only; production code should call
// RunCompareViaService, RunQuorumViaService, and RunBenchmarkViaService.
//
// Phase 5h migration (ddx-bfff4bc7): Runner shim methods deleted; production
// calls use ViaService variants directly. Tests use RunCompareWith/RunQuorumWith
// with an injected RunFunc.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// BenchmarkPrompt is a single test case in a benchmark suite.
type BenchmarkPrompt struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Prompt      string   `json:"prompt"`
	PromptFile  string   `json:"prompt_file,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
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
	Timeout     string            `json:"timeout,omitempty"`
}

// BenchmarkResult is the output of running a full benchmark suite.
type BenchmarkResult struct {
	Suite       string             `json:"suite"`
	Version     string             `json:"version"`
	Timestamp   time.Time          `json:"timestamp"`
	Arms        []BenchmarkArm     `json:"arms"`
	Comparisons []ComparisonRecord `json:"comparisons"`
	Summary     BenchmarkSummary   `json:"summary"`
}

// BenchmarkArmSummary aggregates stats for one arm across all prompts.
type BenchmarkArmSummary struct {
	Label         string  `json:"label"`
	Completed     int     `json:"completed"`
	Failed        int     `json:"failed"`
	TotalTokens   int     `json:"total_tokens"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	AvgDurationMS int     `json:"avg_duration_ms"`
	AvgScore      float64 `json:"avg_score,omitempty"`
}

// BenchmarkSummary aggregates stats across all arms and prompts.
type BenchmarkSummary struct {
	TotalPrompts int                   `json:"total_prompts"`
	Arms         []BenchmarkArmSummary `json:"arms"`
}

// RunFunc executes a single agent invocation and returns the Result. Used by
// the comparison helpers so the same orchestration logic can be driven by
// either the legacy Runner or the agent service.
type RunFunc func(opts RunOptions) (*Result, error)

// RunCompareWith dispatches the same prompt to multiple harnesses, executing
// each arm via run, and returns a ComparisonRecord. Production callers should
// use RunCompareViaService; tests may pass Runner.Run as the closure.
func RunCompareWith(run RunFunc, opts CompareOptions, resolvePrompt func(RunOptions) (string, error), cleanupSandbox func(repoDir, compareID string)) (*ComparisonRecord, error) {
	if len(opts.Harnesses) == 0 {
		return nil, fmt.Errorf("agent: compare requires at least one harness")
	}
	if resolvePrompt == nil {
		resolvePrompt = defaultResolvePromptForCompare
	}

	prompt, err := resolvePrompt(opts.RunOptions)
	if err != nil {
		return nil, err
	}

	id := genCompareID()
	record := &ComparisonRecord{
		ID:        id,
		Timestamp: time.Now().UTC(),
		Prompt:    prompt,
		Arms:      make([]ComparisonArm, len(opts.Harnesses)),
	}

	baseDir := opts.WorkDir
	if baseDir == "" {
		baseDir, _ = os.Getwd()
	}

	worktrees := make([]string, len(opts.Harnesses))
	if opts.Sandbox {
		for i, harness := range opts.Harnesses {
			label := harness
			if l, ok := opts.ArmLabels[i]; ok {
				label = l
			}
			wt, err := createCompareWorktree(baseDir, id, label)
			if err != nil {
				record.Arms[i] = ComparisonArm{
					Harness:  label,
					ExitCode: 1,
					Error:    fmt.Sprintf("worktree: %s", err),
				}
				continue
			}
			worktrees[i] = wt
		}
	}

	var wg sync.WaitGroup
	for i, harness := range opts.Harnesses {
		if opts.Sandbox && worktrees[i] == "" && record.Arms[i].Error != "" {
			continue
		}
		wg.Add(1)
		go func(idx int, harnessName string) {
			defer wg.Done()
			record.Arms[idx] = runCompareArmWith(run, opts, idx, harnessName, baseDir, prompt, worktrees[idx])
		}(i, harness)
	}
	wg.Wait()

	if opts.Sandbox && !opts.KeepSandbox {
		if cleanupSandbox != nil {
			cleanupSandbox(baseDir, id)
		} else {
			cleanupCompareWorktrees(baseDir, id)
		}
	}

	return record, nil
}

// RunCompareViaService is the production replacement for Runner.RunCompare.
// It resolves prompts from inline text or PromptFile and drives each arm
// through the agent service (RunViaService).
func RunCompareViaService(ctx context.Context, workDir string, opts CompareOptions) (*ComparisonRecord, error) {
	run := func(runOpts RunOptions) (*Result, error) {
		return RunViaService(ctx, workDir, runOpts)
	}
	return RunCompareWith(run, opts, defaultResolvePromptForCompare, cleanupCompareWorktrees)
}

// RunCompareWithAgent calls RunCompareViaService using a pre-built DdxAgent.
// Suitable for callers that want to reuse a service instance.
func RunCompareWithAgent(ctx context.Context, agent agentlib.DdxAgent, workDir string, opts CompareOptions) (*ComparisonRecord, error) {
	run := func(runOpts RunOptions) (*Result, error) {
		return RunViaServiceWith(ctx, agent, workDir, runOpts)
	}
	return RunCompareWith(run, opts, defaultResolvePromptForCompare, cleanupCompareWorktrees)
}

// defaultResolvePromptForCompare reads inline Prompt or PromptFile.
func defaultResolvePromptForCompare(opts RunOptions) (string, error) {
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

func runCompareArmWith(run RunFunc, opts CompareOptions, armIdx int, harnessName, baseDir, prompt, worktreePath string) ComparisonArm {
	label := harnessName
	if l, ok := opts.ArmLabels[armIdx]; ok {
		label = l
	}
	arm := ComparisonArm{Harness: label}

	workDir := baseDir
	if worktreePath != "" {
		workDir = worktreePath
	}

	model := opts.Model
	if m, ok := opts.ArmModels[armIdx]; ok {
		model = m
	}

	runOpts := RunOptions{
		Harness:     harnessName,
		Prompt:      prompt,
		Model:       model,
		Effort:      opts.Effort,
		Timeout:     opts.Timeout,
		WorkDir:     workDir,
		Permissions: opts.Permissions,
		Correlation: opts.Correlation,
	}

	result, err := run(runOpts)
	if err != nil {
		arm.ExitCode = 1
		arm.Error = err.Error()
	} else {
		arm.Model = result.Model
		arm.Output = result.Output
		arm.ToolCalls = result.ToolCalls
		arm.Tokens = result.Tokens
		arm.InputTokens = result.InputTokens
		arm.OutputTokens = result.OutputTokens
		arm.CostUSD = result.CostUSD
		arm.DurationMS = result.DurationMS
		arm.ExitCode = result.ExitCode
		arm.Error = result.Error
	}

	if worktreePath != "" {
		arm.Diff = captureGitDiff(worktreePath)
	}

	if opts.PostRun != "" && workDir != "" {
		out, ok := runPostCommand(workDir, opts.PostRun)
		arm.PostRunOut = out
		arm.PostRunOK = &ok
	}

	return arm
}

func createCompareWorktree(workDir, compareID, harnessName string) (string, error) {
	gitRoot, err := resolveGitRoot(workDir)
	if err != nil {
		return "", fmt.Errorf("resolving git root: %w", err)
	}
	wtDir := filepath.Join(gitRoot, ".worktrees", fmt.Sprintf("%s-%s", compareID, harnessName))
	cmd := internalgit.Command(context.Background(), gitRoot, "worktree", "add", "--detach", wtDir, "HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %s\n%s", err, string(out))
	}
	return wtDir, nil
}

func resolveGitRoot(dir string) (string, error) {
	cmd := internalgit.Command(context.Background(), dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %s", dir)
	}
	return strings.TrimSpace(string(out)), nil
}

func captureGitDiff(worktreePath string) string {
	cmd := internalgit.Command(context.Background(), worktreePath, "diff", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	diff := string(out)

	cmd3 := internalgit.Command(context.Background(), worktreePath, "ls-files", "--others", "--exclude-standard")
	untrackedOut, _ := cmd3.Output()
	untracked := strings.TrimSpace(string(untrackedOut))
	if untracked != "" {
		for _, f := range strings.Split(untracked, "\n") {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			content, err := os.ReadFile(filepath.Join(worktreePath, f))
			if err != nil {
				continue
			}
			diff += fmt.Sprintf("\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1 @@\n", f)
			for _, line := range strings.Split(string(content), "\n") {
				if line != "" || len(content) > 0 {
					diff += "+" + line + "\n"
				}
			}
		}
	}
	return strings.TrimSpace(diff)
}

func runPostCommand(dir, command string) (string, bool) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

// cleanupCompareWorktrees removes the worktrees created for a compare run.
// Free function; both RunCompareViaService and RunCompareWithAgent route through
// this helper.
func cleanupCompareWorktrees(repoDir, compareID string) {
	if root, err := resolveGitRoot(repoDir); err == nil {
		repoDir = root
	}
	wtBase := filepath.Join(repoDir, ".worktrees")
	entries, err := os.ReadDir(wtBase)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), compareID) {
			wtPath := filepath.Join(wtBase, e.Name())
			cmd := internalgit.Command(context.Background(), repoDir, "worktree", "remove", "--force", wtPath)
			_ = cmd.Run()
		}
	}
	cmd := internalgit.Command(context.Background(), repoDir, "worktree", "prune")
	_ = cmd.Run()
}

func genCompareID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "cmp-" + hex.EncodeToString(b)
}

// RunQuorumWith invokes multiple harnesses concurrently and returns each
// result. Production callers should use RunQuorumViaService; tests may pass
// Runner.Run.
func RunQuorumWith(run RunFunc, opts QuorumOptions) ([]*Result, error) {
	if len(opts.Harnesses) == 0 {
		return nil, fmt.Errorf("agent: quorum requires at least one harness")
	}

	threshold := effectiveThreshold(opts.Strategy, opts.Threshold, len(opts.Harnesses))
	if threshold < 1 || threshold > len(opts.Harnesses) {
		return nil, fmt.Errorf("agent: invalid quorum threshold %d for %d harnesses", threshold, len(opts.Harnesses))
	}

	results := make([]*Result, len(opts.Harnesses))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, name := range opts.Harnesses {
		wg.Add(1)
		go func(idx int, harness string) {
			defer wg.Done()
			runOpts := opts.RunOptions
			runOpts.Harness = harness
			result, err := run(runOpts)
			mu.Lock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			results[idx] = result
			mu.Unlock()
		}(i, name)
	}
	wg.Wait()

	return results, firstErr
}

// RunQuorumViaService is the production replacement for Runner.RunQuorum.
func RunQuorumViaService(ctx context.Context, workDir string, opts QuorumOptions) ([]*Result, error) {
	run := func(runOpts RunOptions) (*Result, error) {
		return RunViaService(ctx, workDir, runOpts)
	}
	return RunQuorumWith(run, opts)
}

// RunQuorumWithAgent calls RunQuorumViaService using a pre-built DdxAgent.
func RunQuorumWithAgent(ctx context.Context, agent agentlib.DdxAgent, workDir string, opts QuorumOptions) ([]*Result, error) {
	run := func(runOpts RunOptions) (*Result, error) {
		return RunViaServiceWith(ctx, agent, workDir, runOpts)
	}
	return RunQuorumWith(run, opts)
}

// QuorumMet returns true if enough results succeeded.
func QuorumMet(strategy string, threshold int, results []*Result) bool {
	total := len(results)
	eff := effectiveThreshold(strategy, threshold, total)
	successes := 0
	for _, r := range results {
		if r != nil && r.ExitCode == 0 {
			successes++
		}
	}
	return successes >= eff
}

func effectiveThreshold(strategy string, threshold, total int) int {
	switch strategy {
	case "any":
		return 1
	case "majority":
		return (total / 2) + 1
	case "unanimous":
		return total
	default:
		if threshold > 0 {
			return threshold
		}
		return 1
	}
}

// CondenseOutput filters raw agent output to keep only progress-relevant lines.
// The canonical implementation lives in agent/internal/comparison.CondenseOutput.
func CondenseOutput(input, namespacePrefix string) string {
	var kept []string
	skippingTokens := false
	skippingDiff := false
	blankRun := 0
	lastWasKept := false
	keepNextResult := false

	lines := strings.Split(input, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "Commands run:") {
			continue
		}
		if line == "tokens used" {
			skippingTokens = true
			continue
		}
		if skippingTokens {
			skippingTokens = false
			continue
		}
		if condenseIsDiffHeader(line) {
			skippingDiff = true
			continue
		}
		if skippingDiff {
			if condenseIsDiffHeader(line) {
				continue
			}
			if len(line) > 0 && (line[0] == '+' || line[0] == '-' || line[0] == ' ') {
				continue
			}
			skippingDiff = false
		}
		if line == "" {
			blankRun++
			continue
		}
		keep := false
		if keepNextResult {
			keep = true
			keepNextResult = false
		}
		if namespacePrefix != "" && strings.HasPrefix(line, namespacePrefix) {
			keep = true
		}
		if strings.HasPrefix(line, "$ ") {
			keep = true
			keepNextResult = true
		}
		for _, kw := range []string{"error", "Error", "ERROR", "warning", "Warning", "WARN", "FAIL", "fail", "panic"} {
			if strings.Contains(line, kw) {
				keep = true
				break
			}
		}
		for _, kw := range []string{"hx-", "helix-", "FEAT-", "US-", "COMPLETE", "BLOCKED", "CLOSED", "closed", "commit "} {
			if strings.Contains(line, kw) {
				keep = true
				break
			}
		}
		if len(line) > 0 && condenseIsAlphaNumUnderscore(rune(line[0])) && strings.Contains(line, ":") {
			keep = true
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") || strings.HasPrefix(line, " |") || strings.HasPrefix(line, "**") {
			keep = true
		}
		if strings.HasPrefix(line, "Phase") || strings.HasPrefix(line, "Step") || strings.HasPrefix(line, "---") {
			keep = true
		}
		if keep {
			if lastWasKept && blankRun > 0 {
				kept = append(kept, "")
			}
			blankRun = 0
			lastWasKept = true
			kept = append(kept, line)
		}
	}

	if len(kept) == 0 {
		return ""
	}
	result := strings.Join(kept, "\n")
	return condenseTrimBlankLines(result)
}

func condenseIsDiffHeader(line string) bool {
	return strings.HasPrefix(line, "diff --git ") ||
		strings.HasPrefix(line, "index ") ||
		strings.HasPrefix(line, "--- a/") ||
		strings.HasPrefix(line, "+++ b/") ||
		strings.HasPrefix(line, "@@ ")
}

func condenseIsAlphaNumUnderscore(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func condenseTrimBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

// LoadBenchmarkSuite reads a benchmark suite from a JSON file.
// The canonical implementation lives in agent/internal/comparison.
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

// RunBenchmarkWith executes all prompts in a suite against all arms via the
// runCompare closure. Production callers should use RunBenchmarkViaService.
func RunBenchmarkWith(runCompare func(CompareOptions) (*ComparisonRecord, error), suite *BenchmarkSuite) (*BenchmarkResult, error) {
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
		if promptText == "" && prompt.PromptFile != "" {
			data, err := os.ReadFile(prompt.PromptFile)
			if err != nil {
				return nil, fmt.Errorf("reading prompt file %s: %w", prompt.PromptFile, err)
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

		record, err := runCompare(compareOpts)
		if err != nil {
			return nil, fmt.Errorf("prompt %s: %w", prompt.ID, err)
		}

		result.Comparisons = append(result.Comparisons, *record)
	}

	result.Summary = benchSummarize(result)
	return result, nil
}

// RunBenchmarkViaService is the production replacement for Runner.RunBenchmark.
func RunBenchmarkViaService(ctx context.Context, workDir string, suite *BenchmarkSuite) (*BenchmarkResult, error) {
	return RunBenchmarkWith(func(opts CompareOptions) (*ComparisonRecord, error) {
		return RunCompareViaService(ctx, workDir, opts)
	}, suite)
}

// RunBenchmarkWithAgent calls RunBenchmarkViaService using a pre-built DdxAgent.
func RunBenchmarkWithAgent(ctx context.Context, agent agentlib.DdxAgent, workDir string, suite *BenchmarkSuite) (*BenchmarkResult, error) {
	return RunBenchmarkWith(func(opts CompareOptions) (*ComparisonRecord, error) {
		return RunCompareWithAgent(ctx, agent, workDir, opts)
	}, suite)
}

func benchSummarize(result *BenchmarkResult) BenchmarkSummary {
	summary := BenchmarkSummary{
		TotalPrompts: len(result.Comparisons),
	}
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
