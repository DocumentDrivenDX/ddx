package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newAgentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Invoke AI agents with harness dispatch, quorum, and session logging",
		Long: `Unified interface for dispatching work to AI coding agents.

Supports multiple harnesses (codex, claude, gemini, etc.) with output capture,
token tracking, session logging, and multi-agent quorum.

Examples:
  ddx agent run --harness codex --prompt task.md
  ddx agent run --quorum majority --harnesses codex,claude --prompt task.md
  ddx agent list
  ddx agent capabilities codex
  ddx agent doctor
  ddx agent log`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newAgentRunCommand())
	cmd.AddCommand(f.newAgentCondenseCommand())
	cmd.AddCommand(f.newAgentListCommand())
	cmd.AddCommand(f.newAgentCapabilitiesCommand())
	cmd.AddCommand(f.newAgentPresetsCommand())
	cmd.AddCommand(f.newAgentResolveCommand())
	cmd.AddCommand(f.newAgentDoctorCommand())
	cmd.AddCommand(f.newAgentLogCommand())
	cmd.AddCommand(f.newAgentBenchmarkCommand())
	cmd.AddCommand(f.newAgentUsageCommand())

	return cmd
}

func (f *CommandFactory) agentRunner() *agent.Runner {
	cfg, err := config.LoadWithWorkingDir(f.WorkingDir)
	if err != nil || cfg.Agent == nil {
		return agent.NewRunner(agent.Config{})
	}
	r := agent.NewRunner(agent.Config{
		Harness:         cfg.Agent.Harness,
		Model:           cfg.Agent.Model,
		Models:          cfg.Agent.Models,
		ReasoningLevels: cfg.Agent.ReasoningLevels,
		TimeoutMS:       cfg.Agent.TimeoutMS,
		SessionLogDir:   cfg.Agent.SessionLogDir,
		Permissions:     cfg.Agent.Permissions,
	})

	// Wire forge config loader — reads from .ddx/config.yaml on each invocation.
	r.ForgeConfigLoader = func() *agent.ForgeYAMLConfig {
		c, err := config.LoadWithWorkingDir(f.WorkingDir)
		if err != nil || c.Agent == nil || c.Agent.Forge == nil {
			return nil
		}
		fc := c.Agent.Forge
		return &agent.ForgeYAMLConfig{
			Provider:      fc.Provider,
			BaseURL:       fc.BaseURL,
			APIKey:        fc.APIKey,
			Model:         fc.Model,
			Preset:        fc.Preset,
			MaxIterations: fc.MaxIterations,
		}
	}

	return r
}

func (f *CommandFactory) newAgentRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Invoke an agent with a prompt",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := f.agentRunner()

			promptFile, _ := cmd.Flags().GetString("prompt")
			promptText, _ := cmd.Flags().GetString("text")
			harness, _ := cmd.Flags().GetString("harness")
			preset, _ := cmd.Flags().GetString("preset")
			model, _ := cmd.Flags().GetString("model")
			effort, _ := cmd.Flags().GetString("effort")
			timeoutStr, _ := cmd.Flags().GetString("timeout")
			quorum, _ := cmd.Flags().GetString("quorum")
			harnesses, _ := cmd.Flags().GetString("harnesses")
			asJSON, _ := cmd.Flags().GetBool("json")
			worktreeName, _ := cmd.Flags().GetString("worktree")
			permissions, _ := cmd.Flags().GetString("permissions")
			compare, _ := cmd.Flags().GetBool("compare")
			sandbox, _ := cmd.Flags().GetBool("sandbox")
			keepSandbox, _ := cmd.Flags().GetBool("keep-sandbox")
			postRun, _ := cmd.Flags().GetString("post-run")
			arms, _ := cmd.Flags().GetStringArray("arm")

			// Resolve preset: infer harness if needed, apply preset config.
			if preset != "" {
				if harness == "" {
					harness = r.Config.Harness
				}
				if harness == "" {
					harness = agent.DefaultHarness
				}
				presDef, err := r.ResolvePreset(harness, agent.Preset(preset))
				if err != nil {
					return err
				}
				if model == "" {
					model = presDef.Model
				}
				if effort == "" {
					effort = presDef.Effort
				}
				if timeoutStr == "" && presDef.TimeoutMS > 0 {
					timeoutStr = fmt.Sprintf("%ds", presDef.TimeoutMS/1000)
				}
				if permissions == "" {
					permissions = presDef.Permissions
				}
				r.Config.Harness = harness
			}

			var timeout time.Duration
			if timeoutStr != "" {
				var err error
				timeout, err = time.ParseDuration(timeoutStr)
				if err != nil {
					return fmt.Errorf("invalid timeout: %w", err)
				}
			}

			// Resolve worktree if requested
			workDir := f.WorkingDir
			if worktreeName != "" {
				wtPath, err := resolveWorktree(workDir, worktreeName)
				if err != nil {
					return fmt.Errorf("worktree: %w", err)
				}
				workDir = wtPath
			}

			// Read prompt from stdin if neither file nor text provided
			prompt := promptText
			promptSource := "inline"
			if prompt == "" && promptFile == "" {
				// Check if stdin has data
				stat, _ := os.Stdin.Stat()
				if (stat.Mode() & os.ModeCharDevice) == 0 {
					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						return fmt.Errorf("reading stdin: %w", err)
					}
					prompt = string(data)
					promptSource = "stdin"
				}
			} else if promptFile != "" {
				promptSource = promptFile
			}

			// Comparison mode
			if compare {
				var harnessNames []string
				armModels := map[int]string{}
				armLabels := map[int]string{}

				// Parse --arm flags: "harness:model:label" or "harness:model" or "harness"
				if len(arms) > 0 {
					for i, arm := range arms {
						parts := strings.SplitN(arm, ":", 3)
						harnessNames = append(harnessNames, parts[0])
						if len(parts) >= 2 && parts[1] != "" {
							armModels[i] = parts[1]
						}
						if len(parts) >= 3 {
							armLabels[i] = parts[2]
						} else if len(parts) >= 2 && parts[1] != "" {
							armLabels[i] = parts[0] + "/" + parts[1]
						}
					}
				} else if harnesses != "" {
					harnessNames = strings.Split(harnesses, ",")
				} else {
					return fmt.Errorf("--arm or --harnesses required for --compare mode")
				}

				opts := agent.CompareOptions{
					RunOptions: agent.RunOptions{
						Prompt:       prompt,
						PromptFile:   promptFile,
						PromptSource: promptSource,
						Model:        model,
						Effort:       effort,
						Timeout:      timeout,
						WorkDir:      workDir,
						Permissions:  permissions,
					},
					Harnesses:   harnessNames,
					ArmModels:   armModels,
					ArmLabels:   armLabels,
					Sandbox:     sandbox,
					KeepSandbox: keepSandbox,
					PostRun:     postRun,
				}
				record, err := r.RunCompare(opts)
				if err != nil {
					return err
				}
				if asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(record)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Comparison %s (%d arms)\n", record.ID, len(record.Arms))
				for _, arm := range record.Arms {
					status := "OK"
					if arm.ExitCode != 0 {
						status = fmt.Sprintf("FAIL (rc=%d)", arm.ExitCode)
					}
					cost := ""
					if arm.CostUSD > 0 {
						cost = fmt.Sprintf(" cost=$%.4f", arm.CostUSD)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  %-12s %s  tokens=%d  duration=%dms%s\n",
						arm.Harness, status, arm.Tokens, arm.DurationMS, cost)
					if arm.Diff != "" {
						lines := strings.Count(arm.Diff, "\n")
						fmt.Fprintf(cmd.OutOrStdout(), "             diff: %d lines\n", lines)
					}
				}
				return nil
			}

			// Quorum mode
			if quorum != "" || harnesses != "" {
				harnessNames := strings.Split(harnesses, ",")
				if len(harnessNames) == 0 || (len(harnessNames) == 1 && harnessNames[0] == "") {
					return fmt.Errorf("--harnesses required for quorum mode")
				}
				opts := agent.QuorumOptions{
					RunOptions: agent.RunOptions{
						Prompt:       prompt,
						PromptFile:   promptFile,
						PromptSource: promptSource,
						Model:        model,
						Effort:       effort,
						Timeout:      timeout,
						WorkDir:      workDir,
						Permissions:  permissions,
					},
					Harnesses: harnessNames,
					Strategy:  quorum,
				}
				results, err := r.RunQuorum(opts)
				if err != nil {
					return err
				}
				met := agent.QuorumMet(quorum, 0, results)
				if asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]any{
						"quorum_met": met,
						"strategy":   quorum,
						"results":    results,
					})
				}
				for _, result := range results {
					if result == nil {
						continue
					}
					fmt.Fprintf(cmd.OutOrStdout(), "[%s] rc=%d tokens=%d duration=%dms\n",
						result.Harness, result.ExitCode, result.Tokens, result.DurationMS)
					if result.CondensedOutput != "" {
						fmt.Fprintln(cmd.OutOrStdout(), result.CondensedOutput)
					} else if result.Output != "" {
						fmt.Fprintln(cmd.OutOrStdout(), result.Output)
					}
				}
				if met {
					fmt.Fprintf(cmd.OutOrStdout(), "Quorum: MET (%s)\n", quorum)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Quorum: NOT MET (%s)\n", quorum)
					return fmt.Errorf("quorum not met")
				}
				return nil
			}

			// Single harness mode
			opts := agent.RunOptions{
				Harness:      harness,
				Prompt:       prompt,
				PromptFile:   promptFile,
				PromptSource: promptSource,
				Model:        model,
				Effort:       effort,
				Timeout:      timeout,
				WorkDir:      workDir,
				Permissions:  permissions,
			}
			result, err := r.Run(opts)
			if err != nil {
				return err
			}

			// Record the prompt→response pair for virtual harness replay.
			if record, _ := cmd.Flags().GetBool("record"); record && result.ExitCode == 0 {
				resolvedPrompt := prompt
				if resolvedPrompt == "" && promptFile != "" {
					data, _ := os.ReadFile(promptFile)
					resolvedPrompt = string(data)
				}
				entry := &agent.VirtualEntry{
					Prompt:       resolvedPrompt,
					Response:     result.Output,
					Harness:      result.Harness,
					Model:        result.Model,
					DelayMS:      result.DurationMS,
					InputTokens:  result.InputTokens,
					OutputTokens: result.OutputTokens,
					CostUSD:      result.CostUSD,
				}
				dictDir := agent.VirtualDictionaryDir
				// Load normalization patterns from config.
				var patterns []config.NormalizePattern
				if cfg, cfgErr := config.Load(); cfgErr == nil && cfg.Agent != nil && cfg.Agent.Virtual != nil {
					patterns = cfg.Agent.Virtual.Normalize
				}
				if err := agent.RecordEntry(dictDir, entry, patterns...); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to record response: %v\n", err)
				} else {
					normalized := agent.NormalizePrompt(resolvedPrompt, patterns)
					fmt.Fprintf(os.Stderr, "Recorded response → %s/%s.json\n", dictDir, agent.PromptHash(normalized))
				}
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			// Print output
			if result.CondensedOutput != "" {
				fmt.Fprint(cmd.OutOrStdout(), result.CondensedOutput)
			} else if result.Output != "" {
				fmt.Fprint(cmd.OutOrStdout(), result.Output)
			}
			if result.ExitCode != 0 {
				return fmt.Errorf("agent exited with code %d", result.ExitCode)
			}
			return nil
		},
	}

	cmd.Flags().String("prompt", "", "Path to prompt file")
	cmd.Flags().String("text", "", "Inline prompt text")
	cmd.Flags().String("harness", "", "Harness name (default from config)")
	cmd.Flags().String("preset", "", "Preset name (fast, smart, reasoning)")
	cmd.Flags().String("model", "", "Model override")
	cmd.Flags().String("effort", "", "Reasoning effort level")
	cmd.Flags().String("timeout", "", "Timeout duration (e.g. 30s, 5m)")
	cmd.Flags().String("quorum", "", "Quorum strategy: any, majority, unanimous")
	cmd.Flags().String("harnesses", "", "Comma-separated harnesses for quorum")
	cmd.Flags().Bool("json", false, "Output as JSON")
	cmd.Flags().String("worktree", "", "Create/reuse a git worktree for the run")
	cmd.Flags().String("permissions", "", "Permission level: safe, supervised, unrestricted (overrides config)")
	cmd.Flags().Bool("record", false, "Record prompt→response pair for virtual harness replay")
	cmd.Flags().Bool("compare", false, "Compare harnesses on the same prompt")
	cmd.Flags().Bool("sandbox", false, "Run each comparison arm in an isolated git worktree")
	cmd.Flags().Bool("keep-sandbox", false, "Preserve worktrees after comparison")
	cmd.Flags().String("post-run", "", "Command to run in each worktree after the agent completes")
	cmd.Flags().StringArray("arm", nil, "Comparison arm: harness:model:label (repeatable)")

	return cmd
}

func (f *CommandFactory) newAgentPresetsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "presets",
		Short: "List available presets",
		RunE: func(cmd *cobra.Command, args []string) error {
			presets := agent.PresetConfigs()
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(presets)
			}
			for _, p := range presets {
				fmt.Fprintln(cmd.OutOrStdout(), p)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func (f *CommandFactory) newAgentResolveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <harness> <preset>",
		Short: "Resolve a harness + preset to a runnable configuration",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := f.agentRunner()
			harness := args[0]
			preset := agent.Preset(args[1])

			def, err := r.ResolvePreset(harness, preset)
			if err != nil {
				return err
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(def)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Preset:    %s\n", def.Preset)
			fmt.Fprintf(cmd.OutOrStdout(), "Harness:   %s\n", def.Harness)
			if def.Model != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Model:     %s\n", def.Model)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Effort:    %s\n", def.Effort)
			if def.TimeoutMS > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Timeout:   %ds\n", def.TimeoutMS/1000)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Perms:     %s\n", def.Permissions)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func (f *CommandFactory) newAgentCondenseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "condense",
		Short: "Filter raw agent output to progress-relevant lines",
		Long: `Read raw agent output from stdin and write condensed output to stdout.

Keeps: namespace-prefixed progress lines, tool calls, errors/warnings, issue IDs,
markdown structure (#, |, **), and phase markers. Drops raw diffs, codex
boilerplate (Commands run:, tokens used), and bulk verbose content.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString("namespace")
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			result := agent.CondenseOutput(string(data), namespace)
			if result != "" {
				fmt.Fprint(cmd.OutOrStdout(), result)
				if !strings.HasSuffix(result, "\n") {
					fmt.Fprintln(cmd.OutOrStdout())
				}
			}
			return nil
		},
	}
	cmd.Flags().String("namespace", "helix:", "Caller namespace prefix to keep (e.g. helix:)")
	return cmd
}

func (f *CommandFactory) newAgentListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available agent harnesses",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := f.agentRunner()
			statuses := r.Registry.Discover()

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(statuses)
			}

			for _, s := range statuses {
				indicator := "x"
				if s.Available {
					indicator = "ok"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s [%s]  %s\n", s.Name, indicator, s.Binary)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func (f *CommandFactory) newAgentCapabilitiesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capabilities [harness]",
		Short: "Show agent model and reasoning-level capabilities",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := f.agentRunner()

			harness, _ := cmd.Flags().GetString("harness")
			if harness == "" && len(args) > 0 {
				harness = args[0]
			}
			if harness == "" {
				harness = r.Config.Harness
			}

			caps, err := r.Capabilities(harness)
			if err != nil {
				return err
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(caps)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Harness: %s\n", caps.Harness)
			if caps.Path != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Binary: %s (%s)\n", caps.Binary, caps.Path)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Binary: %s\n", caps.Binary)
			}
			if caps.Model != "" {
				modelSource := "default"
				if r.Config.Models[harness] != "" || r.Config.Model != "" {
					modelSource = "config override"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Model: %s (%s)\n", caps.Model, modelSource)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Model: (none configured)")
			}
			if len(caps.Models) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Known models: %s\n", strings.Join(caps.Models, ", "))
			}
			if len(caps.ReasoningLevels) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Reasoning levels: %s\n", strings.Join(caps.ReasoningLevels, ", "))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Reasoning levels: (none configured)")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nConfig example (~/.ddx.yml):\n  agent:\n    models:\n      %s: <model-name>\n    harness: %s\n", harness, harness)
			return nil
		},
	}
	cmd.Flags().String("harness", "", "Harness name (default from config)")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func (f *CommandFactory) newAgentDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check agent harness health",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := f.agentRunner()
			statuses := r.Registry.Discover()

			available := 0
			for _, s := range statuses {
				status := "NOT FOUND"
				if s.Available {
					status = fmt.Sprintf("OK (%s)", s.Path)
					available++
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s %s\n", s.Name, status)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d/%d harnesses available\n", available, len(statuses))

			if available == 0 {
				return fmt.Errorf("no agent harnesses found — install codex, claude, or another supported agent")
			}
			return nil
		},
	}
}

func (f *CommandFactory) newAgentLogCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log [session-id]",
		Short: "Show agent session history",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := f.agentRunner()
			logDir := r.Config.SessionLogDir
			logFile := logDir + "/sessions.jsonl"

			data, err := os.ReadFile(logFile)
			if os.IsNotExist(err) {
				fmt.Fprintln(cmd.OutOrStdout(), "No agent sessions recorded.")
				return nil
			}
			if err != nil {
				return err
			}

			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
				fmt.Fprintln(cmd.OutOrStdout(), "No agent sessions recorded.")
				return nil
			}

			// If session ID specified, show that one
			if len(args) > 0 {
				for _, line := range lines {
					var entry agent.SessionEntry
					if err := json.Unmarshal([]byte(line), &entry); err != nil {
						continue
					}
					if entry.ID == args[0] {
						enc := json.NewEncoder(cmd.OutOrStdout())
						enc.SetIndent("", "  ")
						return enc.Encode(entry)
					}
				}
				return fmt.Errorf("session not found: %s", args[0])
			}

			// Show recent sessions
			limit, _ := cmd.Flags().GetInt("limit")
			start := 0
			if len(lines) > limit {
				start = len(lines) - limit
			}

			for _, line := range lines[start:] {
				var entry agent.SessionEntry
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %-8s  %-10s  %dms  %d tokens  rc=%d\n",
					entry.Timestamp.Format("2006-01-02 15:04:05"),
					entry.ID, entry.Harness, entry.Duration, entry.Tokens, entry.ExitCode)
			}
			return nil
		},
	}
	cmd.Flags().Int("limit", 20, "Number of recent sessions to show")
	return cmd
}

func (f *CommandFactory) newAgentBenchmarkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "benchmark",
		Short: "Run a benchmark suite comparing agent harnesses",
		Long: `Execute a benchmark suite to compare multiple agent harnesses across prompts.

The benchmark suite is defined in JSON format with arms (harness configurations)
and prompts to run. Results include token counts, costs, durations, and can be
saved for later analysis.

Examples:
  ddx agent benchmark --suite benchmarks/coding.json
  ddx agent benchmark --suite benchmarks/coding.json --output results.json
  ddx agent benchmark --suite benchmarks/coding.json --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			suitePath, _ := cmd.Flags().GetString("suite")
			outputPath, _ := cmd.Flags().GetString("output")
			asJSON, _ := cmd.Flags().GetBool("json")

			if suitePath == "" {
				return fmt.Errorf("--suite is required")
			}

			suite, err := agent.LoadBenchmarkSuite(suitePath)
			if err != nil {
				return fmt.Errorf("loading benchmark suite: %w", err)
			}

			runner := f.agentRunner()
			result, err := runner.RunBenchmark(suite)
			if err != nil {
				return fmt.Errorf("running benchmark: %w", err)
			}

			var output []byte
			if asJSON || outputPath != "" {
				output, err = json.MarshalIndent(result, "", "  ")
				if err != nil {
					return fmt.Errorf("marshaling result: %w", err)
				}
			} else {
				// Print summary table
				var sb strings.Builder
				w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)

				fmt.Fprintln(w, "Arm\tCompleted\tFailed\tTokens\tCost\tAvg Duration")
				fmt.Fprintln(w, "---\t---------\t------\t------\t----\t------------")

				for _, arm := range result.Summary.Arms {
					costStr := fmt.Sprintf("$%.4f", arm.TotalCostUSD)
					durationStr := fmt.Sprintf("%dms", arm.AvgDurationMS)
					fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\t%s\n",
						arm.Label, arm.Completed, arm.Failed, arm.TotalTokens, costStr, durationStr)
				}

				w.Flush()
				output = []byte(sb.String())
			}

			if outputPath != "" {
				if err := os.WriteFile(outputPath, output, 0644); err != nil {
					return fmt.Errorf("writing output file: %w", err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Results written to %s\n", outputPath)
			} else {
				cmd.OutOrStdout().Write(output)
			}

			return nil
		},
	}
	cmd.Flags().String("suite", "", "Path to benchmark suite JSON file (required)")
	cmd.Flags().String("output", "", "Path to save results as JSON")
	cmd.Flags().Bool("json", false, "Output results as JSON")
	return cmd
}

// resolveWorktree creates a git worktree at .worktrees/<name> if it does not
// exist, then returns the absolute path to the worktree directory.
func resolveWorktree(repoRoot, name string) (string, error) {
	if repoRoot == "" {
		// Detect from git
		out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			return "", fmt.Errorf("cannot detect repo root: %w", err)
		}
		repoRoot = strings.TrimSpace(string(out))
	}

	wtDir := filepath.Join(repoRoot, ".worktrees", name)

	// Check if worktree already exists
	if _, err := os.Stat(wtDir); err == nil {
		return wtDir, nil
	}

	// Create the worktree with a branch of the same name
	cmd := exec.Command("git", "worktree", "add", wtDir, "-b", name)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		// If branch already exists, try without -b
		cmd2 := exec.Command("git", "worktree", "add", wtDir, name)
		cmd2.Dir = repoRoot
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			return "", fmt.Errorf("git worktree add failed: %s\n%s", string(out), string(out2))
		}
	}

	return wtDir, nil
}
