package cmd

import (
	"bytes"
	"context"
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
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newAgentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Invoke AI agents with harness dispatch, quorum, and session logging",
		Long: `Unified interface for dispatching work to AI coding agents.

Supports multiple harnesses (codex, claude, gemini, etc.) with output capture,
token tracking, session logging, and multi-agent quorum.

The embedded DDx agent harness is named 'agent' and is always available without
installing external binaries. Use --harness agent or --profile cheap to route to it.

Profile routing (--profile cheap|fast|smart) selects the best available harness
and model automatically. Workflow tools should prefer --profile over --harness to
stay decoupled from harness installation details.

Examples:
  ddx agent run --profile cheap --prompt task.md
  ddx agent run --profile smart --prompt task.md
  ddx agent run --profile smart --model gpt-5.4   # explicit override; avoid by default
  ddx agent run --harness agent --prompt task.md
  ddx agent run --harness codex --prompt task.md
  ddx agent run --quorum majority --harnesses codex,claude --prompt task.md
  ddx agent list
  ddx agent capabilities agent
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
	cmd.AddCommand(f.newAgentDoctorCommand())
	cmd.AddCommand(f.newAgentLogCommand())
	cmd.AddCommand(f.newAgentBenchmarkCommand())
	cmd.AddCommand(f.newAgentUsageCommand())
	cmd.AddCommand(f.newAgentReplayCommand())
	cmd.AddCommand(f.newAgentExecuteBeadCommand())
	cmd.AddCommand(f.newAgentExecuteLoopCommand())

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

	// Wire agent config loader — reads from .ddx/config.yaml on each invocation.
	r.AgentConfigLoader = func() *agent.AgentYAMLConfig {
		c, err := config.LoadWithWorkingDir(f.WorkingDir)
		if err != nil || c.Agent == nil || c.Agent.AgentRunner == nil {
			return nil
		}
		fc := c.Agent.AgentRunner
		yaml := &agent.AgentYAMLConfig{
			Provider:      fc.Provider,
			BaseURL:       fc.BaseURL,
			APIKey:        fc.APIKey,
			Model:         fc.Model,
			Preset:        fc.Preset,
			MaxIterations: fc.MaxIterations,
		}
		if fc.Models != nil {
			yaml.Models = make(map[string]*agent.LLMPresetYAML, len(fc.Models))
			for name, p := range fc.Models {
				yaml.Models[name] = &agent.LLMPresetYAML{
					Model:     p.Model,
					Provider:  p.Provider,
					Endpoints: p.Endpoints,
					APIKey:    p.APIKey,
					Strategy:  p.Strategy,
				}
			}
		}
		return yaml
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
			model, _ := cmd.Flags().GetString("model")
			profile, _ := cmd.Flags().GetString("profile")
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

			// Single harness mode.
			// When no explicit --harness is given but --profile (or --model) is set,
			// route through NormalizeRouteRequest → BuildCandidatePlans → RankCandidates
			// to select the best available harness for the request. This allows workflow
			// tools to pass stage intent (cheap/fast/smart) without choosing a harness.
			resolvedHarness := harness
			resolvedModel := model
			if harness == "" && (profile != "" || model != "") {
				flags := agent.RouteFlags{
					Profile:     profile,
					Model:       model,
					Effort:      effort,
					Permissions: permissions,
					// Harness intentionally empty — routing selects across all candidates.
				}
				// Don't constrain routing to the config-default harness when profile/model
				// drives selection; clear Harness so all registered harnesses are evaluated.
				routingCfg := r.Config
				routingCfg.Harness = ""
				req := agent.NormalizeRouteRequest(flags, routingCfg, r.Catalog)
				plans := r.ProbeAndBuildCandidatePlans(req, timeout)
				ranked := agent.RankCandidates(req.Profile, plans)
				for _, plan := range ranked {
					if plan.Viable {
						resolvedHarness = plan.Harness
						if model == "" && plan.ConcreteModel != "" {
							resolvedModel = plan.ConcreteModel
						}
						break
					}
				}
				if resolvedHarness == "" {
					return fmt.Errorf("agent: no viable harness found for profile %q; install a harness or use --harness to specify one", profile)
				}
			}

			opts := agent.RunOptions{
				Harness:      resolvedHarness,
				Prompt:       prompt,
				PromptFile:   promptFile,
				PromptSource: promptSource,
				Model:        resolvedModel,
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
	cmd.Flags().String("harness", "", "Harness name (default from config); use 'agent' for the embedded DDx agent")
	cmd.Flags().String("model", "", "Model override; normally omit when using --profile")
	cmd.Flags().String("profile", "", "Routing intent: cheap, fast, smart (selects harness, model, and defaults automatically)")
	cmd.Flags().String("effort", "", "Reasoning effort override; normally omit when using --profile")
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
			if caps.Surface != "" {
				localStr := ""
				if caps.IsLocal {
					localStr = " (local)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Surface: %s%s  cost-class: %s  exact-pin: %v\n",
					caps.Surface, localStr, caps.CostClass, caps.ExactPinSupport)
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
			if len(caps.ProfileMappings) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Profile mappings:")
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				// Print in stable order: smart, fast, then others.
				order := []string{"smart", "fast"}
				printed := map[string]bool{}
				for _, p := range order {
					if m, ok := caps.ProfileMappings[p]; ok {
						fmt.Fprintf(tw, "  %s\t→ %s\n", p, m)
						printed[p] = true
					}
				}
				for p, m := range caps.ProfileMappings {
					if !printed[p] {
						fmt.Fprintf(tw, "  %s\t→ %s\n", p, m)
					}
				}
				_ = tw.Flush()
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
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check agent harness health",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := f.agentRunner()
			checkConnectivity, _ := cmd.Flags().GetBool("connectivity")
			checkRouting, _ := cmd.Flags().GetBool("routing")
			timeoutStr, _ := cmd.Flags().GetString("timeout")
			asJSON, _ := cmd.Flags().GetBool("json")
			statuses := r.Registry.Discover()

			// Parse timeout (default 15s for connectivity checks)
			probeTimeout := 15 * time.Second
			if timeoutStr != "" {
				if t, err := time.ParseDuration(timeoutStr); err == nil {
					probeTimeout = t
				}
			}

			// Routing mode: probe full HarnessState per harness.
			if checkRouting {
				type routingEntry struct {
					Name  string             `json:"name"`
					State agent.HarnessState `json:"state"`
				}
				var entries []routingEntry
				for _, s := range statuses {
					st := r.ProbeHarnessState(s.Name, probeTimeout)
					entries = append(entries, routingEntry{Name: s.Name, State: st})
				}
				if asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(entries)
				}
				for _, e := range entries {
					st := e.State
					indicator := "ok"
					if !st.Installed {
						indicator = "not installed"
					} else if !st.Reachable {
						indicator = "not reachable"
					} else if !st.Authenticated {
						indicator = "not authenticated"
					} else if st.QuotaState == "blocked" || (!st.QuotaOK && st.QuotaState != "unknown") {
						indicator = "quota exceeded"
					} else if st.Degraded {
						indicator = "degraded"
					} else if st.QuotaState == "unknown" {
						indicator = "quota unknown"
					}
					quotaState := st.QuotaState
					if quotaState == "" {
						if st.QuotaOK {
							quotaState = "ok"
						} else {
							quotaState = "blocked"
						}
					}
					line := fmt.Sprintf("%-12s  installed=%-5v  reachable=%-5v  auth=%-5v  quota=%-8s  [%s]",
						e.Name, st.Installed, st.Reachable, st.Authenticated, quotaState, indicator)
					if st.Quota != nil {
						quota := st.Quota
						if quota.ResetDate != "" {
							line += fmt.Sprintf("  quota: %d%% of %s (resets %s)", quota.PercentUsed, quota.LimitWindow, quota.ResetDate)
						} else {
							line += fmt.Sprintf("  quota: %d%% of %s", quota.PercentUsed, quota.LimitWindow)
						}
					}
					if st.RoutingSignal != nil {
						signal := st.RoutingSignal
						line += fmt.Sprintf("  source: %s/%s", signal.Source.Provider, signal.Source.Kind)
						if signal.Source.Freshness != "" {
							line += fmt.Sprintf("  freshness: %s", signal.Source.Freshness)
						}
						if signal.HistoricalUsage.TotalTokens > 0 {
							line += fmt.Sprintf("  native-usage: %d tokens", signal.HistoricalUsage.TotalTokens)
						}
					}
					if !st.LastChecked.IsZero() {
						line += fmt.Sprintf("  checked: %s", st.LastChecked.UTC().Format(time.RFC3339))
					}
					if st.Error != "" {
						line += "  error: " + st.Error
					}
					fmt.Fprintln(cmd.OutOrStdout(), line)
				}
				return nil
			}

			available := 0
			functional := 0
			for i, s := range statuses {
				statusStr := "NOT FOUND"
				if s.Available {
					available++
					statusStr = fmt.Sprintf("OK (%s)", s.Path)

					// Optionally test provider connectivity
					if checkConnectivity {
						providerStatus := r.TestProviderConnectivity(s.Name, probeTimeout)
						statuses[i].Provider = &providerStatus

						if providerStatus.Reachable && providerStatus.CreditsOK {
							statusStr = fmt.Sprintf("OK (%s) ✓ provider reachable", s.Path)
							functional++
						} else if providerStatus.Reachable && !providerStatus.CreditsOK {
							statusStr = fmt.Sprintf("⚠️  (%s) provider out of credits/quota", s.Path)
						} else if providerStatus.Error != "" {
							statusStr = fmt.Sprintf("⚠️  (%s) %s", s.Path, providerStatus.Error)
						}
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s %s\n", s.Name, statusStr)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "\n%d/%d harnesses available", available, len(statuses))
			if checkConnectivity && available > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), " (%d functional)", functional)
			}
			fmt.Fprintln(cmd.OutOrStdout())

			if available == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\n⚠️  No agent harnesses found.")
				fmt.Fprintln(cmd.OutOrStdout(), "💡 Install codex, claude, or another supported agent.")
				return nil
			}
			return nil
		},
	}
	cmd.Flags().Bool("connectivity", false, "Test provider connectivity and credit status")
	cmd.Flags().Bool("routing", false, "Probe and report full routing-relevant harness state (installed/reachable/auth/quota/degraded)")
	cmd.Flags().String("timeout", "", "Timeout for connectivity checks (default 15s)")
	cmd.Flags().Bool("json", false, "Output as JSON (with --routing)")
	return cmd
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

				_ = w.Flush()
				output = []byte(sb.String())
			}

			if outputPath != "" {
				if err := os.WriteFile(outputPath, output, 0644); err != nil {
					return fmt.Errorf("writing output file: %w", err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Results written to %s\n", outputPath)
			} else {
				_, _ = cmd.OutOrStdout().Write(output)
			}

			return nil
		},
	}
	cmd.Flags().String("suite", "", "Path to benchmark suite JSON file (required)")
	cmd.Flags().String("output", "", "Path to save results as JSON")
	cmd.Flags().Bool("json", false, "Output results as JSON")
	return cmd
}

func (f *CommandFactory) newAgentReplayCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay <bead-id>",
		Short: "Replay a bead with a different agent harness or model",
		Long: `Reconstructs the prompt from the bead's linked agent session and re-runs it
with a different harness or model for comparison.

Examples:
  ddx agent replay ddx-abc123 --harness claude --model claude-opus-4-6
  ddx agent replay ddx-abc123 --harness agent --at-head
  ddx agent replay ddx-abc123 --sandbox`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beadID := args[0]
			harness, _ := cmd.Flags().GetString("harness")
			model, _ := cmd.Flags().GetString("model")
			atHead, _ := cmd.Flags().GetBool("at-head")
			sandbox, _ := cmd.Flags().GetBool("sandbox")

			// Get the bead
			b, err := f.beadStore().Get(beadID)
			if err != nil {
				return fmt.Errorf("bead not found: %w", err)
			}

			// Extract session ID from bead evidence
			sessionID := ""
			if b.Extra != nil {
				if sid, ok := b.Extra["session_id"]; ok {
					sessionID = fmt.Sprint(sid)
				}
			}

			// Get the prompt - from session or fallback to bead prose
			var prompt string
			var baseCommit string
			if sessionID != "" {
				sess := f.resolveAgentSession(sessionID)
				if sess != nil {
					prompt = sess.Prompt
					// Store original metadata for comparison
					fmt.Fprintf(cmd.OutOrStdout(), "Replaying from session %s\n", sessionID)
					fmt.Fprintf(cmd.OutOrStdout(), "Original: %s with %s\n", sess.Harness, sess.Model)
				}
			}

			// Fallback to bead prose if no session prompt (title → description → acceptance)
			if prompt == "" {
				switch {
				case b.Title != "":
					prompt = b.Title
					fmt.Fprintf(cmd.OutOrStdout(), "Note: Using bead title as prompt (baseline session unknown)\n")
				case b.Description != "":
					prompt = b.Description
					fmt.Fprintf(cmd.OutOrStdout(), "Note: Using bead description as prompt (baseline session unknown)\n")
				case b.Acceptance != "":
					prompt = b.Acceptance
					fmt.Fprintf(cmd.OutOrStdout(), "Note: Using bead acceptance criteria as prompt (baseline session unknown)\n")
				default:
					return fmt.Errorf("no prompt available from session or bead")
				}
			}

			// Determine base commit
			if !atHead {
				if b.Extra != nil {
					if sha, ok := b.Extra["closing_commit_sha"]; ok && sha != "" {
						// Use parent of closing commit
						shaStr := fmt.Sprint(sha)
						out, err := exec.Command("git", "rev-parse", shaStr+"^").Output()
						if err == nil {
							baseCommit = strings.TrimSpace(string(out))
							fmt.Fprintf(cmd.OutOrStdout(), "Base commit: %s (parent of %s)\n", baseCommit, shaStr)
						}
					}
				}
			}

			// If no base commit determined, use current HEAD
			if baseCommit == "" {
				out, err := exec.Command("git", "rev-parse", "HEAD").Output()
				if err == nil {
					baseCommit = strings.TrimSpace(string(out))
					fmt.Fprintf(cmd.OutOrStdout(), "Base commit: %s (current HEAD)\n", baseCommit)
				}
			}

			// Setup workdir for sandbox mode
			workDir := ""
			if sandbox {
				wtName := fmt.Sprintf("replay-%s-%s", beadID, harness)
				wtDir, err := resolveWorktree(f.WorkingDir, wtName)
				if err != nil {
					return fmt.Errorf("sandbox worktree: %w", err)
				}
				workDir = wtDir
				fmt.Fprintf(cmd.OutOrStdout(), "Sandbox: %s\n", workDir)

				// Checkout base commit in worktree
				if baseCommit != "" {
					gitCmd := exec.Command("git", "checkout", baseCommit)
					gitCmd.Dir = workDir
					if out, err := gitCmd.CombinedOutput(); err != nil {
						return fmt.Errorf("checkout %s: %w\n%s", baseCommit, err, string(out))
					}
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "\nReplaying with %s", harness)
			if model != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", model)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 50))

			// Run the agent
			runner := f.agentRunner()
			result, err := runner.Run(agent.RunOptions{
				Harness: harness,
				Model:   model,
				Prompt:  prompt,
				WorkDir: workDir,
			})
			if err != nil {
				return fmt.Errorf("agent run failed: %w", err)
			}

			// Show results
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 50))
			fmt.Fprintf(cmd.OutOrStdout(), "Exit code: %d\n", result.ExitCode)
			fmt.Fprintf(cmd.OutOrStdout(), "Duration: %dms\n", result.DurationMS)
			if result.Tokens > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Tokens: %d\n", result.Tokens)
			}
			if result.CostUSD > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Cost: $%.4f\n", result.CostUSD)
			}

			// Show diff if sandbox mode
			if sandbox && workDir != "" {
				fmt.Fprintln(cmd.OutOrStdout())
				fmt.Fprintln(cmd.OutOrStdout(), "Changes:")
				gitCmd := exec.Command("git", "diff", "--stat")
				gitCmd.Dir = workDir
				out, _ := gitCmd.CombinedOutput()
				if len(out) > 0 {
					_, _ = cmd.OutOrStderr().Write(out)
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "  (no changes)")
				}
			}

			return nil
		},
	}

	cmd.Flags().String("harness", "", "Agent harness to use for replay (required)")
	cmd.Flags().String("model", "", "Model override for the harness")
	cmd.Flags().Bool("at-head", false, "Replay against current HEAD instead of closing commit parent")
	cmd.Flags().Bool("sandbox", false, "Run in an isolated git worktree")

	return cmd
}

type executeLoopCommandOptions struct {
	FromRev string
	Harness string
	Model   string
	Effort  string
}

func (f *CommandFactory) newAgentExecuteLoopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute-loop",
		Short: "Drain the single-project execution-ready bead queue",
		Long: `Scans the current project's execution-ready bead queue, claims the next
ready bead, runs ddx agent execute-bead from the project root, records the
structured result, and continues until no unattempted ready work remains.`,
		Args: cobra.NoArgs,
		RunE: f.runAgentExecuteLoop,
	}
	cmd.Flags().String("from", "", "Base git revision to start from (default: HEAD)")
	cmd.Flags().String("harness", "", "Agent harness to use")
	cmd.Flags().String("model", "", "Model override")
	cmd.Flags().String("effort", "", "Effort level")
	cmd.Flags().Bool("once", false, "Process at most one ready bead")
	cmd.Flags().Duration("poll-interval", 0, "Poll interval for continuous scanning; zero drains current ready work and exits")
	cmd.Flags().Bool("json", false, "Output loop result as JSON")
	return cmd
}

func (f *CommandFactory) runAgentExecuteLoop(cmd *cobra.Command, args []string) error {
	projectRoot := gitpkg.FindProjectRoot(f.WorkingDir)
	fromRev, _ := cmd.Flags().GetString("from")
	harness, _ := cmd.Flags().GetString("harness")
	model, _ := cmd.Flags().GetString("model")
	effort, _ := cmd.Flags().GetString("effort")
	once, _ := cmd.Flags().GetBool("once")
	pollInterval, _ := cmd.Flags().GetDuration("poll-interval")
	asJSON, _ := cmd.Flags().GetBool("json")

	store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
	execFactory := f.withWorkingDir(projectRoot)

	worker := &agent.ExecuteBeadWorker{
		Store: store,
		Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			res, err := execFactory.invokeExecuteBeadFromLoop(ctx, beadID, executeLoopCommandOptions{
				FromRev: fromRev,
				Harness: harness,
				Model:   model,
				Effort:  effort,
			})
			if err != nil {
				return agent.ExecuteBeadReport{}, err
			}
			return agent.ExecuteBeadReport{
				BeadID:      res.BeadID,
				Status:      res.Status,
				Detail:      res.Detail,
				SessionID:   res.SessionID,
				BaseRev:     res.BaseRev,
				ResultRev:   res.ResultRev,
				PreserveRef: res.PreserveRef,
			}, nil
		}),
	}

	result, err := worker.Run(cmd.Context(), agent.ExecuteBeadLoopOptions{
		Assignee:     resolveClaimAssignee(),
		Once:         once,
		PollInterval: pollInterval,
	})
	if err != nil {
		return err
	}

	if asJSON {
		payload := struct {
			ProjectRoot string `json:"project_root"`
			*agent.ExecuteBeadLoopResult
		}{
			ProjectRoot:           projectRoot,
			ExecuteBeadLoopResult: result,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	if result.NoReadyWork {
		fmt.Fprintf(cmd.OutOrStdout(), "project: %s\n", projectRoot)
		fmt.Fprintln(cmd.OutOrStdout(), "No execution-ready beads.")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "project: %s\n", projectRoot)
	for _, attempt := range result.Results {
		fmt.Fprintf(cmd.OutOrStdout(), "%s  %s", attempt.BeadID, attempt.Status)
		if attempt.Detail != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s", attempt.Detail)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
	fmt.Fprintf(cmd.OutOrStdout(), "attempts: %d  successes: %d  failures: %d\n", result.Attempts, result.Successes, result.Failures)
	return nil
}

func (f *CommandFactory) invokeExecuteBeadFromLoop(ctx context.Context, beadID string, opts executeLoopCommandOptions) (ExecuteBeadResult, error) {
	cmd := f.newAgentExecuteBeadCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetContext(ctx)

	args := []string{beadID, "--json"}
	if opts.FromRev != "" {
		args = append(args, "--from", opts.FromRev)
	}
	if opts.Harness != "" {
		args = append(args, "--harness", opts.Harness)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Effort != "" {
		args = append(args, "--effort", opts.Effort)
	}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		return ExecuteBeadResult{}, err
	}

	raw := output.String()
	jsonStart := strings.Index(raw, "{")
	if jsonStart == -1 {
		return ExecuteBeadResult{}, fmt.Errorf("execute-bead returned no JSON result")
	}

	var res ExecuteBeadResult
	dec := json.NewDecoder(strings.NewReader(raw[jsonStart:]))
	if err := dec.Decode(&res); err != nil {
		return ExecuteBeadResult{}, fmt.Errorf("parse execute-bead result: %w", err)
	}
	return res, nil
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
