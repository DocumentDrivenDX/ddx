package cmd

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/DocumentDrivenDX/ddx/internal/serverreg"
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			serverreg.TryRegisterAsync(f.WorkingDir)
			return nil
		},
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
	cmd.AddCommand(f.newAgentWorkersCommand())
	cmd.AddCommand(f.newAgentCatalogCommand())
	cmd.AddCommand(f.newAgentProvidersCommand())
	cmd.AddCommand(f.newAgentModelsCommand())
	cmd.AddCommand(f.newAgentCheckCommand())
	cmd.AddCommand(f.newAgentRouteStatusCommand())
	cmd.AddCommand(f.newAgentMetricsCommand())

	return cmd
}

func (f *CommandFactory) agentRunner() *agent.Runner {
	cfg, err := config.LoadWithWorkingDir(f.WorkingDir)
	if err != nil || cfg.Agent == nil {
		return agent.NewRunner(agent.Config{
			SessionLogDir: agent.ResolveLogDir(f.WorkingDir, ""),
		})
	}
	return agent.NewRunner(agent.Config{
		Harness:         cfg.Agent.Harness,
		Model:           cfg.Agent.Model,
		Models:          cfg.Agent.Models,
		ReasoningLevels: cfg.Agent.ReasoningLevels,
		TimeoutMS:       cfg.Agent.TimeoutMS,
		SessionLogDir:   agent.ResolveLogDir(f.WorkingDir, cfg.Agent.SessionLogDir),
		Permissions:     cfg.Agent.Permissions,
	})
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
			outputFmt, _ := cmd.Flags().GetString("output")
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

			// Resolve project root (--project flag > DDX_PROJECT_ROOT > CWD git root)
			projectFlag, _ := cmd.Flags().GetString("project")
			workDir := resolveProjectRoot(projectFlag, f.WorkingDir)
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

			// --json is a backward-compatible alias for --output json-result
			if asJSON {
				outputFmt = "json-result"
			}
			switch outputFmt {
			case "json-result":
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(result); err != nil {
					return err
				}
			case "session-jsonl":
				if result.CondensedOutput != "" {
					fmt.Fprint(cmd.OutOrStdout(), result.CondensedOutput)
				} else if result.Output != "" {
					fmt.Fprint(cmd.OutOrStdout(), result.Output)
				}
			case "text":
				text := agent.ExtractOutput(result.Harness, result.Output)
				if text != "" {
					fmt.Fprint(cmd.OutOrStdout(), text)
				}
			default:
				return fmt.Errorf("unknown --output value %q (valid: text, json-result, session-jsonl)", outputFmt)
			}
			if result.ExitCode != 0 {
				return fmt.Errorf("agent exited with code %d", result.ExitCode)
			}
			return nil
		},
	}

	cmd.Flags().String("project", "", "Project root path (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().String("prompt", "", "Path to prompt file")
	cmd.Flags().String("text", "", "Inline prompt text")
	cmd.Flags().String("harness", "", "Harness name (default from config); use 'agent' for the embedded DDx agent")
	cmd.Flags().String("model", "", "Model override; normally omit when using --profile")
	cmd.Flags().String("profile", "", "Routing intent: cheap, fast, smart (selects harness, model, and defaults automatically)")
	cmd.Flags().String("effort", "", "Reasoning effort override; normally omit when using --profile")
	cmd.Flags().String("timeout", "", "Timeout duration (e.g. 30s, 5m)")
	cmd.Flags().String("quorum", "", "Quorum strategy: any, majority, unanimous")
	cmd.Flags().String("harnesses", "", "Comma-separated harnesses for quorum")
	cmd.Flags().Bool("json", false, "Output as JSON (alias for --output json-result)")
	cmd.Flags().String("output", "text", "Output format: text (default), json-result, session-jsonl")
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
				// Refresh quota snapshots via tmux if stale (>15m).
				// This is the slow path — only called from doctor, not from routing.
				now := time.Now()
				_ = r.RefreshClaudeQuotaViaTmux(now, 15*time.Minute)
				_ = r.RefreshCodexQuotaViaTmux(now, 15*time.Minute)

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
					if st.ClaudeQuotaDecision != nil {
						d := st.ClaudeQuotaDecision
						if d.SnapshotPresent {
							ageSec := int(d.Age.Round(time.Second).Seconds())
							line += fmt.Sprintf("  claude-quota-cache: age=%ds fresh=%v", ageSec, d.Fresh)
							if d.Snapshot != nil {
								line += fmt.Sprintf(" 5h=%d/%d weekly=%d/%d source=%s",
									d.Snapshot.FiveHourRemaining, d.Snapshot.FiveHourLimit,
									d.Snapshot.WeeklyRemaining, d.Snapshot.WeeklyLimit,
									d.Snapshot.Source)
							}
						} else {
							line += "  claude-quota-cache: absent"
						}
					}
					if !st.LastChecked.IsZero() {
						line += fmt.Sprintf("  checked: %s", st.LastChecked.UTC().Format(time.RFC3339))
					}
					if st.Error != "" {
						line += "  error: " + st.Error
					}
					fmt.Fprintln(cmd.OutOrStdout(), line)
					// Print non-session quota windows (weekly, extra, credit) as sub-lines.
					if st.RoutingSignal != nil {
						for _, w := range st.RoutingSignal.QuotaWindows {
							if w.LimitID == "session" {
								continue
							}
							wline := fmt.Sprintf("  %-10s  %-32s  %3.0f%%  state=%-8s", w.LimitID, w.Name, w.UsedPercent, w.State)
							if w.ResetsAt != "" {
								wline += "  resets: " + w.ResetsAt
							}
							fmt.Fprintln(cmd.OutOrStdout(), wline)
						}
					}
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
			if !filepath.IsAbs(logDir) {
				logDir = filepath.Join(f.WorkingDir, logDir)
			}
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

			beadID, _ := cmd.Flags().GetString("bead")
			asJSON, _ := cmd.Flags().GetBool("json")

			// --bead filter: show per-bead attempt history
			if beadID != "" {
				var filtered []agent.SessionEntry
				for _, line := range lines {
					var entry agent.SessionEntry
					if err := json.Unmarshal([]byte(line), &entry); err != nil {
						continue
					}
					if entry.Correlation["bead_id"] == beadID {
						filtered = append(filtered, entry)
					}
				}

				if len(filtered) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "no sessions found for bead %s\n", beadID)
					return nil
				}

				// Sort ascending by timestamp (attempt order)
				sort.Slice(filtered, func(i, j int) bool {
					return filtered[i].Timestamp.Before(filtered[j].Timestamp)
				})

				execBaseDir := filepath.Join(f.WorkingDir, agent.ExecuteBeadArtifactDir)
				resolveOutcome := func(entry agent.SessionEntry) string {
					attemptID := entry.Correlation["attempt_id"]
					if attemptID != "" {
						resultPath := filepath.Join(execBaseDir, attemptID, "result.json")
						if rdata, rerr := os.ReadFile(resultPath); rerr == nil {
							var result agent.ExecuteBeadResult
							if jerr := json.Unmarshal(rdata, &result); jerr == nil && result.Outcome != "" {
								return result.Outcome
							}
						}
					}
					if entry.ExitCode == 0 {
						return "success"
					}
					return "error"
				}

				if asJSON {
					type entryWithOutcome struct {
						agent.SessionEntry
						Outcome string `json:"outcome"`
					}
					out := make([]entryWithOutcome, len(filtered))
					for i, e := range filtered {
						out[i] = entryWithOutcome{SessionEntry: e, Outcome: resolveOutcome(e)}
					}
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(out)
				}

				var nMerged, nPreserved, nErrors int
				now := time.Now()
				var lastAttemptTime time.Time

				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "ATTEMPT\tSTARTED\tDURATION\tHARNESS\tMODEL\tOUTCOME\tTOKENS\tCOST\tSESSION")
				for i, entry := range filtered {
					outcome := resolveOutcome(entry)
					switch outcome {
					case "merged":
						nMerged++
					case "preserved":
						nPreserved++
					case "error", "task_failed":
						nErrors++
					}
					if entry.Timestamp.After(lastAttemptTime) {
						lastAttemptTime = entry.Timestamp
					}

					model := entry.Model
					if model == "" {
						model = "-"
					}
					cost := "local"
					if entry.CostUSD > 0 {
						cost = fmt.Sprintf("$%.4f", entry.CostUSD)
					}
					sessionShort := entry.ID
					if len(sessionShort) > 8 {
						sessionShort = sessionShort[:8]
					}

					fmt.Fprintf(tw, "%d\t%s\t%dms\t%s\t%s\t%s\t%d\t%s\t%s\n",
						i+1,
						entry.Timestamp.Format("2006-01-02 15:04:05"),
						entry.Duration,
						entry.Harness,
						model,
						outcome,
						entry.Tokens,
						cost,
						sessionShort,
					)
				}
				_ = tw.Flush()

				elapsed := agentLogFormatElapsed(now.Sub(lastAttemptTime))
				fmt.Fprintf(cmd.OutOrStdout(), "\n%s: %d attempts, %d merged, %d preserved, %d errors. Last attempt: %s ago.\n",
					beadID, len(filtered), nMerged, nPreserved, nErrors, elapsed)
				return nil
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
	cmd.Flags().String("bead", "", "Filter sessions by bead ID and show attempt history")
	cmd.Flags().Bool("json", false, "Output as JSON (with --bead)")
	return cmd
}

// agentLogFormatElapsed formats a duration as a human-readable string (e.g. "5s", "3m", "2h", "1d").
func agentLogFormatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
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

func (f *CommandFactory) newAgentCatalogCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Manage the model catalog (tier assignments)",
	}

	// catalog show: print current effective catalog.
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show the current model catalog (tier→surface→model assignments)",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := agent.DefaultModelCatalogPath()
			cat, err := agent.LoadModelCatalogYAML(path)
			if err != nil {
				return fmt.Errorf("load catalog: %w", err)
			}
			if cat == nil {
				cat = agent.DefaultModelCatalogYAML()
				fmt.Fprintf(cmd.OutOrStdout(), "(built-in defaults — no catalog at %s)\n\n", path)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Catalog: %s (updated %s)\n\n", path, cat.UpdatedAt.Format("2006-01-02"))
			}

			// Tiers
			for _, tier := range []string{"smart", "standard", "cheap"} {
				def, ok := cat.Tiers[tier]
				if !ok {
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n", tier, def.Description)
				for surface, model := range def.Surfaces {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-20s %s\n", surface, model)
				}
			}

			// Blocked models
			var blocked []string
			for _, m := range cat.Models {
				if m.Blocked {
					blocked = append(blocked, m.ID)
				}
			}
			if len(blocked) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nBlocked models (routing never selects these):\n")
				for _, id := range blocked {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", id)
				}
			}
			return nil
		},
	}

	cmd.AddCommand(showCmd)
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

func (f *CommandFactory) newAgentExecuteLoopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute-loop",
		Short: "Drain the single-project execution-ready bead queue",
		Long: `execute-loop is the primary queue-driven execution surface. It scans the
target project's execution-ready bead queue, claims the next ready bead,
runs "ddx agent execute-bead" on it from the project root, records the
structured result, and continues until no unattempted ready work remains.

Reach for execute-loop by default. Use "ddx agent execute-bead" directly
only as the primitive for debugging or re-running one specific bead.

Planning and document-only beads are valid execution targets — any bead
with unmet acceptance criteria and no blocking deps is eligible.

Close semantics (per execute-bead result status):
  success                      — close bead with session + commit evidence
  already_satisfied            — close bead (after repeated no_changes)
  no_changes                   — unclaim; may cooldown or close after retries
  land_conflict                — unclaim; result preserved under refs/ddx/iterations/
  post_run_check_failed        — unclaim; result preserved
  execution_failed             — unclaim
  structural_validation_failed — unclaim

Only success (and already_satisfied) closes the bead. Every other status
leaves the bead open and unclaimed so a later attempt can try again. Each
attempt is appended to the bead as an execute-bead event (status, detail,
base_rev, result_rev, preserve_ref, retry_after), and the underlying agent
session log is recorded under the execute-bead agent-log path.

By default execute-loop submits to the running ddx server as a background
worker and returns immediately. Use --local to run inline in the current
process.

Project targeting (multi-project servers):
  --project <path>    target a specific project root (absolute path or name)
  DDX_PROJECT_ROOT    env var fallback; used when --project is not set
  (default)           the git root of the current working directory

When submitting to a multi-project server you must ensure the target project
is registered with the server (run "ddx server" from that directory, or use
"ddx server projects register"). The server rejects unrecognised project paths.
`,
		Example: `  # Drain the current execution-ready queue once and exit (normal surface)
  ddx agent execute-loop

  # Pick one ready bead, execute it, and stop
  ddx agent execute-loop --once

  # Run continuously as a bounded queue worker
  ddx agent execute-loop --poll-interval 30s

  # Force a specific harness/model for a debugging pass
  ddx agent execute-loop --once --harness codex
  ddx agent execute-loop --once --harness agent --model minimax/minimax-m2.7

  # Run inline in the current process (not recommended for long runs)
  ddx agent execute-loop --local --once`,
		Args: cobra.NoArgs,
		RunE: f.runAgentExecuteLoop,
	}
	cmd.Flags().String("project", "", "Target project root path or name (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().String("from", "", "Base git revision to start from (default: HEAD)")
	cmd.Flags().String("harness", "", "Agent harness to use")
	cmd.Flags().String("model", "", "Model override")
	cmd.Flags().String("provider", "", "Provider name (e.g. vidar, openrouter); selects a named provider from config")
	cmd.Flags().String("model-ref", "", "Model catalog reference (e.g. code-medium); resolved via the model catalog")
	cmd.Flags().String("effort", "", "Effort level")
	cmd.Flags().Bool("once", false, "Process at most one ready bead")
	cmd.Flags().Duration("poll-interval", 0, "Poll interval for continuous scanning; zero drains current ready work and exits")
	cmd.Flags().Bool("json", false, "Output loop result as JSON")
	cmd.Flags().Bool("local", false, "Run inline in current process instead of server worker (default: submit to server)")
	cmd.Flags().Bool("no-review", false, "Skip post-merge review (e.g. for doc-only beads or tight iteration loops)")
	cmd.Flags().String("review-harness", "", "Harness to use for the post-merge reviewer (default: same as implementation harness)")
	cmd.Flags().String("review-model", "", "Model override for the post-merge reviewer (default: smart tier)")
	cmd.Flags().String("min-tier", "", "Minimum tier for auto-escalation: cheap, standard, or smart (default: cheap)")
	cmd.Flags().String("max-tier", "", "Maximum tier for auto-escalation: cheap, standard, or smart (default: smart)")
	return cmd
}

func (f *CommandFactory) runAgentExecuteLoop(cmd *cobra.Command, args []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)
	fromRev, _ := cmd.Flags().GetString("from")
	harness, _ := cmd.Flags().GetString("harness")
	model, _ := cmd.Flags().GetString("model")
	provider, _ := cmd.Flags().GetString("provider")
	modelRef, _ := cmd.Flags().GetString("model-ref")
	effort, _ := cmd.Flags().GetString("effort")
	once, _ := cmd.Flags().GetBool("once")
	pollInterval, _ := cmd.Flags().GetDuration("poll-interval")
	asJSON, _ := cmd.Flags().GetBool("json")
	local, _ := cmd.Flags().GetBool("local")
	noReview, _ := cmd.Flags().GetBool("no-review")
	reviewHarness, _ := cmd.Flags().GetString("review-harness")
	reviewModel, _ := cmd.Flags().GetString("review-model")
	minTier, _ := cmd.Flags().GetString("min-tier")
	maxTier, _ := cmd.Flags().GetString("max-tier")

	// If --local, run inline; otherwise submit to running ddx server
	if !local {
		return f.executeLoopWithServer(cmd, projectRoot, harness, model, provider, modelRef, effort, once, pollInterval, asJSON, noReview, reviewHarness, reviewModel, minTier, maxTier)
	}

	// Pre-flight: validate harness availability and model compatibility
	// before claiming any beads. This surfaces errors like "claude binary
	// not found" or "vidar is an agent preset, not a claude model" before
	// any bead status changes hands.
	{
		preflightRunner := f.agentRunner()
		if err := preflightRunner.ValidateForExecuteLoop(harness, model, provider, modelRef); err != nil {
			return fmt.Errorf("execute-loop: %w", err)
		}
	}

	store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))

	// Structured progress sink for this loop run. Events emitted at
	// loop.start, bead.claimed, bead.result, and loop.end land here so
	// log aggregators (FormatSessionLogLines, `ddx server workers log`)
	// can parse the same JSONL envelope used by harness session logs.
	loopSessionID := fmt.Sprintf("agent-loop-%d", time.Now().UnixNano())
	loopLogDir := filepath.Join(projectRoot, agent.DefaultLogDir)
	_ = os.MkdirAll(loopLogDir, 0o755)
	loopLogPath := filepath.Join(loopLogDir, loopSessionID+".jsonl")
	var loopSink io.Writer
	if f, err := os.OpenFile(loopLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		loopSink = f
		defer f.Close() //nolint:errcheck
	}

	// Start session log tailer so the user sees progress during agent execution
	tailCtx, tailCancel := context.WithCancel(context.Background())
	go agent.TailSessionLogs(tailCtx, projectRoot, cmd.OutOrStdout())

	// Instantiate a process-local LandCoordinator so --local uses the same
	// single-writer land path as the server worker. Stopped on function exit.
	localCoord := serverpkg.NewLocalLandCoordinator(projectRoot, agent.RealLandingGitOps{})
	defer localCoord.Stop()

	// Build post-merge reviewer unless --no-review is set.
	// Reviewer is on-by-default: runs at smart tier after every successful merge.
	var reviewer agent.BeadReviewer
	if !noReview {
		reviewer = &agent.DefaultBeadReviewer{
			ProjectRoot: projectRoot,
			BeadStore:   bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			Runner:      f.agentRunner(),
			Harness:     reviewHarness,
			Model:       reviewModel,
		}
	}

	// escalationEnabled is true when neither --harness nor --model is pinned.
	// In that case the executor iterates through tiers (cheap → standard →
	// smart) and escalates on failure. When either flag is set, the executor
	// runs a single attempt with the specified harness/model.
	escalationEnabled := harness == "" && model == ""

	// singleTierAttempt runs one execution attempt with an explicit harness
	// and model. It is called both by the non-escalating path and by each
	// iteration of the tier escalation loop.
	singleTierAttempt := func(ctx context.Context, beadID string, tier agent.ModelTier, resolvedHarness, resolvedModel string) (agent.ExecuteBeadReport, error) {
		runner := f.agentRunner()
		gitOps := &agent.RealGitOps{}

		res, execErr := agent.ExecuteBead(projectRoot, beadID, agent.ExecuteBeadOptions{
			FromRev:    fromRev,
			Harness:    resolvedHarness,
			Model:      resolvedModel,
			Provider:   provider,
			ModelRef:   modelRef,
			Effort:     effort,
			BeadEvents: bead.NewStore(filepath.Join(projectRoot, ".ddx")),
		}, gitOps, runner)
		if execErr != nil && res == nil {
			return agent.ExecuteBeadReport{}, execErr
		}
		if res != nil && res.ResultRev != "" && res.ResultRev != res.BaseRev && res.ExitCode == 0 {
			landReq := agent.BuildLandRequestFromResult(projectRoot, res)
			landRes, landErr := localCoord.Submit(landReq)
			if landErr == nil {
				agent.ApplyLandResultToExecuteBeadResult(res, landRes)
			} else if execErr == nil {
				execErr = landErr
			}
		} else if res != nil && res.ResultRev == res.BaseRev {
			res.Outcome = "no-changes"
			res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
		} else if res != nil && res.ExitCode != 0 {
			res.Outcome = "preserved"
			res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
		}
		if execErr != nil {
			return agent.ExecuteBeadReport{}, execErr
		}
		tierStr := ""
		if tier != "" {
			tierStr = string(tier)
		}
		return agent.ExecuteBeadReport{
			BeadID:             res.BeadID,
			AttemptID:          res.AttemptID,
			WorkerID:           res.WorkerID,
			Harness:            res.Harness,
			Provider:           res.Provider,
			Model:              res.Model,
			Tier:               tierStr,
			Status:             res.Status,
			Detail:             res.Detail,
			SessionID:          res.SessionID,
			BaseRev:            res.BaseRev,
			ResultRev:          res.ResultRev,
			PreserveRef:        res.PreserveRef,
			NoChangesRationale: res.NoChangesRationale,
		}, nil
	}

	worker := &agent.ExecuteBeadWorker{
		Store:    store,
		Reviewer: reviewer,
		Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			if !escalationEnabled {
				// Single-attempt path: --harness or --model was specified.
				// Route dynamically when no harness is pinned (legacy behaviour).
				runner := f.agentRunner()
				resolvedHarness := harness
				if resolvedHarness == "" {
					routeFlags := agent.RouteFlags{Model: model, Provider: provider, ModelRef: modelRef, Effort: effort}
					routingCfg := runner.Config
					routingCfg.Harness = ""
					req := agent.NormalizeRouteRequest(routeFlags, routingCfg, runner.Catalog)
					plans := runner.ProbeAndBuildCandidatePlans(req, 10*time.Second)
					ranked := agent.RankCandidates("", plans)
					if best, err := agent.SelectBestCandidate(ranked); err == nil {
						resolvedHarness = best.Harness
					}
				}
				return singleTierAttempt(ctx, beadID, "", resolvedHarness, model)
			}

			// Tier escalation path: try cheap → standard → smart (bounded by
			// --min-tier / --max-tier). For each tier, probe available harnesses,
			// filter out unhealthy ones, pick the best, and attempt execution.
			// On escalatable failures, mark the harness unhealthy and try the
			// next tier. A successful result or a non-escalatable failure
			// (structural) terminates the loop early.
			tiers := agent.TiersInRange(agent.ModelTier(minTier), agent.ModelTier(maxTier))
			if len(tiers) == 0 {
				return agent.ExecuteBeadReport{
					BeadID: beadID,
					Status: agent.ExecuteBeadStatusExecutionFailed,
					Detail: "execute-loop: no tiers in range (check --min-tier / --max-tier)",
				}, nil
			}

			beadStore := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
			assignee := resolveClaimAssignee()
			runner := f.agentRunner()
			var lastReport agent.ExecuteBeadReport

			for _, tier := range tiers {
				// Build a route request for this tier using the tier name as profile.
				routingCfg := runner.Config
				routingCfg.Harness = ""
				req := agent.NormalizeRouteRequest(
					agent.RouteFlags{Profile: string(tier), Provider: provider, Effort: effort},
					routingCfg, runner.Catalog,
				)
				plans := runner.ProbeAndBuildCandidatePlans(req, 10*time.Second)

				// Filter out harnesses on cooldown from a previous tier attempt.
				for i := range plans {
					if plans[i].Viable && !agent.GlobalProviderHealth.IsHealthy(plans[i].Harness) {
						plans[i].Viable = false
						plans[i].RejectReason = "provider cooldown"
					}
				}

				ranked := agent.RankCandidates(string(tier), plans)
				best, pickErr := agent.SelectBestCandidate(ranked)
				probeResult := "ok"
				if pickErr != nil {
					probeResult = "no viable provider"
					// No viable harness for this tier; skip and escalate.
					_ = beadStore.AppendEvent(beadID, bead.BeadEvent{
						Kind:      "tier-attempt",
						Summary:   "skipped",
						Body:      agent.FormatTierAttemptBody(string(tier), "", "", probeResult, "no viable harness found"),
						Actor:     assignee,
						Source:    "ddx agent execute-loop",
						CreatedAt: time.Now().UTC(),
					})
					continue
				}

				report, attemptErr := singleTierAttempt(ctx, beadID, tier, best.Harness, best.ConcreteModel)
				if attemptErr != nil {
					report = agent.ExecuteBeadReport{
						BeadID:      beadID,
						Tier:        string(tier),
						Harness:     best.Harness,
						Model:       best.ConcreteModel,
						Status:      agent.ExecuteBeadStatusExecutionFailed,
						Detail:      attemptErr.Error(),
						ProbeResult: probeResult,
					}
				} else {
					report.ProbeResult = probeResult
				}
				lastReport = report

				// Record a per-tier attempt event so the escalation trail is visible.
				_ = beadStore.AppendEvent(beadID, bead.BeadEvent{
					Kind:      "tier-attempt",
					Summary:   report.Status,
					Body:      agent.FormatTierAttemptBody(string(tier), report.Harness, report.Model, probeResult, report.Detail),
					Actor:     assignee,
					Source:    "ddx agent execute-loop",
					CreatedAt: time.Now().UTC(),
				})

				if report.Status == agent.ExecuteBeadStatusSuccess {
					return report, nil
				}
				if !agent.ShouldEscalate(report.Status) {
					// Structural failure — escalation cannot help.
					return report, nil
				}

				// Execution-level failure: mark harness unhealthy so the next
				// tier attempt (and parallel workers) skip it during cooldown.
				if report.Status == agent.ExecuteBeadStatusExecutionFailed {
					agent.GlobalProviderHealth.Mark(best.Harness, time.Now().Add(agent.ProviderCooldownDuration))
				}
			}

			if lastReport.BeadID == "" {
				return agent.ExecuteBeadReport{
					BeadID: beadID,
					Status: agent.ExecuteBeadStatusExecutionFailed,
					Detail: "execute-loop: all tiers exhausted — no viable provider found",
				}, nil
			}
			lastReport.Detail = "escalation exhausted: " + lastReport.Detail
			return lastReport, nil
		}),
	}

	cliLandingOps := agent.RealLandingGitOps{}
	result, err := worker.Run(cmd.Context(), agent.ExecuteBeadLoopOptions{
		Assignee:     resolveClaimAssignee(),
		Once:         once,
		PollInterval: pollInterval,
		Log:          cmd.OutOrStdout(),
		EventSink:    loopSink,
		WorkerID:     resolveClaimAssignee(),
		ProjectRoot:  projectRoot,
		Harness:      harness,
		Model:        model,
		Provider:     provider,
		ModelRef:     modelRef,
		SessionID:    loopSessionID,
		PreClaimHook: buildCLIPreClaimHook(projectRoot, cliLandingOps),
		NoReview:     noReview,
		MinTier:      minTier,
		MaxTier:      maxTier,
	})
	tailCancel() // stop session log tailer
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

	fmt.Fprintf(cmd.OutOrStdout(), "\nproject: %s\n", projectRoot)
	fmt.Fprintf(cmd.OutOrStdout(), "completed: %d  |  successes: %d  |  failures: %d\n", result.Attempts, result.Successes, result.Failures)
	if result.Failures > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nfailed:\n")
		for _, attempt := range result.Results {
			if attempt.Status != "success" {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s", attempt.BeadID, attempt.Detail)
				if attempt.PreserveRef != "" {
					fmt.Fprintf(cmd.OutOrStdout(), " (preserved)")
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
		}
	}
	return nil
}

// executeLoopWithServer submits an execute-loop job to the running ddx server.
// The server starts a background worker and returns its ID.
func (f *CommandFactory) executeLoopWithServer(cmd *cobra.Command, projectRoot, harness, model, provider, modelRef, effort string, once bool, pollInterval time.Duration, asJSON bool, noReview bool, reviewHarness, reviewModel, minTier, maxTier string) error {
	serverBase := resolveServerURL(projectRoot)

	workerSpec := map[string]any{
		"once":         once,
		"project_root": projectRoot,
	}
	if harness != "" {
		workerSpec["harness"] = harness
	}
	if model != "" {
		workerSpec["model"] = model
	}
	if provider != "" {
		workerSpec["provider"] = provider
	}
	if modelRef != "" {
		workerSpec["model_ref"] = modelRef
	}
	if effort != "" {
		workerSpec["effort"] = effort
	}
	if pollInterval > 0 {
		workerSpec["poll_interval"] = pollInterval.String()
	}
	if noReview {
		workerSpec["no_review"] = true
	}
	if reviewHarness != "" {
		workerSpec["review_harness"] = reviewHarness
	}
	if reviewModel != "" {
		workerSpec["review_model"] = reviewModel
	}
	if minTier != "" {
		workerSpec["min_tier"] = minTier
	}
	if maxTier != "" {
		workerSpec["max_tier"] = maxTier
	}

	specData, err := json.Marshal(workerSpec)
	if err != nil {
		return fmt.Errorf("marshal worker spec: %w", err)
	}

	reqURL := serverBase + "/api/agent/workers/execute-loop"
	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, reqURL, bytes.NewReader(specData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := newLocalServerClient()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("submit to server %s: %w\n  Hint: start the server with 'ddx server'", serverBase, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusServiceUnavailable {
		return fmt.Errorf("server is running but no project is loaded for this directory\n  Hint: start the server from the project root")
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var workerRecord struct {
		ID          string `json:"id"`
		State       string `json:"state"`
		ProjectRoot string `json:"project_root"`
		Harness     string `json:"harness,omitempty"`
		Model       string `json:"model,omitempty"`
		Once        bool   `json:"once"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&workerRecord); err != nil {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("parse server response: %w: %s", err, string(body))
	}

	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"worker_id":    workerRecord.ID,
			"state":        workerRecord.State,
			"project_root": workerRecord.ProjectRoot,
			"harness":      workerRecord.Harness,
			"model":        workerRecord.Model,
			"once":         workerRecord.Once,
		})
	}

	fmt.Fprintf(cmd.OutOrStdout(), "worker:   %s\n", workerRecord.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "state:    %s\n", workerRecord.State)
	fmt.Fprintf(cmd.OutOrStdout(), "project:  %s\n", workerRecord.ProjectRoot)
	if workerRecord.Harness != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "harness:  %s\n", workerRecord.Harness)
	}
	if workerRecord.Model != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "model:    %s\n", workerRecord.Model)
	}
	if workerRecord.Once {
		fmt.Fprintln(cmd.OutOrStdout(), "once:     true")
	}
	fmt.Fprintln(cmd.OutOrStdout(), "")
	fmt.Fprintf(cmd.OutOrStdout(), "Monitor progress: ddx server workers show %s\n", workerRecord.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "View logs:        ddx server workers log %s\n", workerRecord.ID)
	return nil
}

// resolveProjectRoot returns the project root to use for execute-loop,
// execute-bead, and agent-run commands. Resolution order:
//  1. --project flag value (if non-empty)
//  2. DDX_PROJECT_ROOT environment variable
//  3. CWD-based git project root detection (gitpkg.FindProjectRoot)
func resolveProjectRoot(projectFlag, workingDir string) string {
	if projectFlag != "" {
		return projectFlag
	}
	if env := os.Getenv("DDX_PROJECT_ROOT"); env != "" {
		return env
	}
	return gitpkg.FindProjectRoot(workingDir)
}

// buildCLIPreClaimHook returns a PreClaimHook for the --local execute-loop
// that fetches origin and verifies ancestry before each bead claim. Fetch
// failures are logged but do not block the worker (air-gap friendly).
func buildCLIPreClaimHook(projectRoot string, gitOps agent.LandingGitOps) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		branch, err := gitOps.CurrentBranch(projectRoot)
		if err != nil {
			return nil // can't determine branch — skip
		}
		res, err := gitOps.FetchOriginAncestryCheck(projectRoot, branch)
		if err != nil {
			return nil // fetch failure is non-fatal
		}
		if res.Action == "diverged" {
			return fmt.Errorf("local branch %s has diverged from origin (local=%s origin=%s); reconcile manually before claiming",
				branch, res.LocalSHA, res.OriginSHA)
		}
		return nil
	}
}

// resolveServerURL determines the base URL for the running DDx server.
//
// Resolution order (matches internal/serverreg):
//  1. DDX_SERVER_URL environment variable (explicit override)
//  2. ~/.local/share/ddx/server.addr (written by `ddx server` on startup)
//  3. https://127.0.0.1:7743 (the canonical default — see FEAT-020)
//
// The addr file may record a bind-address URL like https://0.0.0.0:7743
// (because the server binds to 0.0.0.0 for multi-interface access). Those
// are valid listen addresses but not reachable as client targets on some
// platforms — rewrite them to https://127.0.0.1:<port> for local clients.
//
// The probe-common-ports heuristic that used to live here only worked for
// the legacy plaintext http://127.0.0.1:8080 setup and never honored the
// TLS default that's been in place since alpha13. Clients that skipped
// the addr file would silently get a connection refused and tell the
// user the server wasn't running when it was listening on 7743.
func resolveServerURL(projectRoot string) string {
	if u := os.Getenv("DDX_SERVER_URL"); u != "" {
		return u
	}
	if u := serverpkg.ReadServerAddr(); u != "" {
		return rewriteBindAddrForClient(u)
	}
	return "https://127.0.0.1:7743"
}

// rewriteBindAddrForClient converts a bind-address URL into a client-reachable
// URL. 0.0.0.0 (and [::]) are valid listen addresses but not reachable as
// client destinations on all platforms — substitute 127.0.0.1 so local HTTP
// clients can always connect.
func rewriteBindAddrForClient(u string) string {
	for _, bind := range []string{"//0.0.0.0:", "//[::]:", "//[::0]:"} {
		if idx := strings.Index(u, bind); idx >= 0 {
			return u[:idx] + "//127.0.0.1:" + u[idx+len(bind):]
		}
	}
	return u
}

// newLocalServerClient returns an http.Client configured to talk to the
// local DDx server over the auto-generated self-signed TLS certificate.
// Clients skip verification because the server's cert is a throwaway
// localhost cert stored under ~/.ddx/server/tls/ — there's nothing useful
// to verify, and trusting self-signed certs via the system store would
// require root. Only use this helper for local-loopback requests.
func newLocalServerClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local self-signed cert
		},
	}
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
