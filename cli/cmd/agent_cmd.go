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
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/workerprobe"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/DocumentDrivenDX/ddx/internal/serverreg"
	agentlib "github.com/DocumentDrivenDX/fizeau"
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

Profile routing (--profile default|cheap|fast|smart) selects the best available harness
and model automatically. Workflow tools should prefer --profile over --harness to
stay decoupled from harness installation details.

Examples:
  ddx agent run --profile default --prompt task.md
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
	cmd.AddCommand(f.newAgentExecutionsCommand())
	cmd.AddCommand(f.newAgentWorkersCommand())
	cmd.AddCommand(f.newAgentCatalogCommand())
	cmd.AddCommand(f.newAgentProvidersCommand())
	cmd.AddCommand(f.newAgentModelsCommand())
	cmd.AddCommand(f.newAgentCheckCommand())
	cmd.AddCommand(f.newAgentRouteStatusCommand())
	cmd.AddCommand(f.newAgentMetricsCommand())

	return cmd
}

func (f *CommandFactory) newAgentRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "run",
		Short:        "Invoke an agent with a prompt",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
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

				overrides := config.CLIOverrides{
					Model:       model,
					Effort:      effort,
					Permissions: permissions,
				}
				if timeout > 0 {
					overrides.Timeout = &timeout
				}
				rcfg, _ := config.LoadAndResolve(f.WorkingDir, overrides)
				runtime := agent.CompareRuntime{
					AgentRunRuntime: agent.AgentRunRuntime{
						Prompt:       prompt,
						PromptFile:   promptFile,
						PromptSource: promptSource,
						WorkDir:      workDir,
					},
					Harnesses:   harnessNames,
					ArmModels:   armModels,
					ArmLabels:   armLabels,
					Sandbox:     sandbox,
					KeepSandbox: keepSandbox,
					PostRun:     postRun,
				}
				record, err := agent.RunCompareWithConfigViaService(cmd.Context(), f.WorkingDir, rcfg, runtime)
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
				overrides := config.CLIOverrides{
					Model:       model,
					Effort:      effort,
					Permissions: permissions,
				}
				if timeout > 0 {
					overrides.Timeout = &timeout
				}
				rcfg, _ := config.LoadAndResolve(f.WorkingDir, overrides)
				runtime := agent.QuorumRuntime{
					AgentRunRuntime: agent.AgentRunRuntime{
						Prompt:       prompt,
						PromptFile:   promptFile,
						PromptSource: promptSource,
						WorkDir:      workDir,
					},
					Harnesses: harnessNames,
					Strategy:  quorum,
				}
				results, err := agent.RunQuorumWithConfigViaService(cmd.Context(), f.WorkingDir, rcfg, runtime)
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

			// Pass operator intent directly to Execute; do not pre-resolve
			// the route. The upstream service owns harness+provider+model
			// selection within the caller's passthrough constraints and
			// MinPower/MaxPower bounds (CONTRACT-003 / ddx-da19756a).
			profile = agent.NormalizeRoutingProfile(profile)

			overrides := config.CLIOverrides{
				Harness:     harness,
				Model:       model,
				Profile:     profile,
				Effort:      effort,
				Permissions: permissions,
			}
			if timeout > 0 {
				overrides.Timeout = &timeout
			}
			rcfg, _ := config.LoadAndResolve(f.WorkingDir, overrides)
			runtime := agent.AgentRunRuntime{
				Prompt:       prompt,
				PromptFile:   promptFile,
				PromptSource: promptSource,
				WorkDir:      workDir,
			}
			result, err := agent.RunWithConfigViaService(cmd.Context(), f.WorkingDir, rcfg, runtime)
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
				msg := fmt.Sprintf("agent exited with code %d", result.ExitCode)
				if result.Error != "" {
					msg += "\n  error: " + result.Error
				}
				if result.Stderr != "" && result.Stderr != result.Error {
					// Show at most 5 stderr lines to surface auth/rate-limit diagnostics
					// without flooding the terminal with full agent output.
					lines := strings.SplitN(strings.TrimSpace(result.Stderr), "\n", 6)
					if len(lines) > 5 {
						lines = append(lines[:5], "  ...")
					}
					msg += "\n  stderr:\n    " + strings.Join(lines, "\n    ")
				}
				return fmt.Errorf("%s", msg)
			}
			return nil
		},
	}

	cmd.Flags().String("project", "", "Project root path (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().String("prompt", "", "Path to prompt file")
	cmd.Flags().String("text", "", "Inline prompt text")
	cmd.Flags().String("harness", "", "Harness name (default from config); use 'agent' for the embedded DDx agent")
	cmd.Flags().String("model", "", "Model override; normally omit when using --profile")
	cmd.Flags().String("profile", "", "Routing intent: default, cheap, fast, smart (selects harness, model, and defaults automatically)")
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
			svc, err := agent.NewServiceFromWorkDir(f.WorkingDir)
			if err != nil {
				return fmt.Errorf("constructing agent service: %w", err)
			}

			ctx := context.Background()
			harnesses, err := svc.ListHarnesses(ctx)
			if err != nil {
				return fmt.Errorf("listing harnesses: %w", err)
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				type harnessEntry struct {
					Name      string `json:"name"`
					Type      string `json:"type,omitempty"`
					Available bool   `json:"available"`
					Path      string `json:"path,omitempty"`
					Error     string `json:"error,omitempty"`
				}
				entries := make([]harnessEntry, 0, len(harnesses))
				for _, h := range harnesses {
					entries = append(entries, harnessEntry{
						Name:      h.Name,
						Type:      h.Type,
						Available: h.Available,
						Path:      h.Path,
						Error:     h.Error,
					})
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}

			for _, h := range harnesses {
				indicator := "x"
				if h.Available {
					indicator = "ok"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s [%s]  %s\n", h.Name, indicator, h.Path)
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
			harness, _ := cmd.Flags().GetString("harness")
			if harness == "" && len(args) > 0 {
				harness = args[0]
			}
			// Load config to determine default harness + detect model overrides.
			cfg, _ := config.LoadWithWorkingDir(f.WorkingDir)
			var configHarness, configModel string
			var configModels map[string]string
			if cfg != nil && cfg.Agent != nil {
				configHarness = cfg.Agent.Harness
				configModel = cfg.Agent.Model
				configModels = cfg.Agent.Models
			}
			if harness == "" {
				harness = configHarness
			}

			caps, err := agent.CapabilitiesViaService(cmd.Context(), f.WorkingDir, harness)
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
				if configModels[harness] != "" || configModel != "" {
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
		Use:          "doctor",
		Short:        "Check agent harness health",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			checkConnectivity, _ := cmd.Flags().GetBool("connectivity")
			checkRouting, _ := cmd.Flags().GetBool("routing")
			checkWorkers, _ := cmd.Flags().GetBool("workers")
			timeoutStr, _ := cmd.Flags().GetString("timeout")
			asJSON, _ := cmd.Flags().GetBool("json")

			// ADR-022 step 6: --workers prints the runtime worker registry
			// (server-first, .ddx/workers/ on-disk fallback).
			if checkWorkers {
				return f.runAgentDoctorWorkers(cmd, f.WorkingDir, asJSON)
			}

			// Parse timeout (default 15s for connectivity checks)
			probeTimeout := 15 * time.Second
			if timeoutStr != "" {
				if t, err := time.ParseDuration(timeoutStr); err == nil {
					probeTimeout = t
				}
			}

			// Routing mode: derive full routing state per harness via service.
			if checkRouting {
				svc, svcErr := agent.NewServiceFromWorkDir(f.WorkingDir)
				if svcErr != nil {
					return fmt.Errorf("agent doctor: failed to initialize service: %w", svcErr)
				}
				ctx := cmd.Context()
				harnesses, listErr := svc.ListHarnesses(ctx)
				if listErr != nil {
					return fmt.Errorf("agent doctor: listing harnesses: %w", listErr)
				}

				// Refresh quota caches via HealthCheck, then re-fetch harness list.
				for _, h := range harnesses {
					_ = svc.HealthCheck(ctx, agentlib.HealthTarget{Type: "harness", Name: h.Name})
				}
				harnesses, listErr = svc.ListHarnesses(ctx)
				if listErr != nil {
					return fmt.Errorf("agent doctor: re-listing harnesses after health check: %w", listErr)
				}

				type routingState struct {
					Installed     bool                         `json:"installed"`
					Reachable     bool                         `json:"reachable"`
					Authenticated bool                         `json:"authenticated"`
					QuotaOK       bool                         `json:"quota_ok"`
					QuotaState    string                       `json:"quota_state,omitempty"`
					Degraded      bool                         `json:"degraded"`
					Error         string                       `json:"error,omitempty"`
					Quota         *agent.QuotaInfo             `json:"quota,omitempty"`
					RoutingSignal *agent.RoutingSignalSnapshot `json:"routing_signal,omitempty"`
					LastChecked   time.Time                    `json:"last_checked,omitempty"`
				}
				type routingEntry struct {
					Name  string       `json:"name"`
					State routingState `json:"state"`
				}
				now := time.Now()
				var entries []routingEntry
				for _, hi := range harnesses {
					st := routingState{
						Installed:   hi.Available,
						LastChecked: now,
					}
					// Derive reachable/authenticated: HealthCheck already ran; available == reachable for harnesses.
					st.Reachable = hi.Available
					st.Authenticated = hi.Available
					st.QuotaOK = true
					// Derive quota state from upstream HarnessInfo.Quota.
					if hi.Quota != nil {
						switch hi.Quota.Status {
						case "ok":
							st.QuotaState = "ok"
						case "stale":
							st.QuotaState = "ok"
						}
						if len(hi.Quota.Windows) > 0 {
							// Report worst window as current quota for routing.
							var worstPct int
							var worstReset string
							blocked := false
							for _, w := range hi.Quota.Windows {
								if w.LimitID == "extra" {
									continue
								}
								if w.State == "blocked" {
									blocked = true
									worstReset = w.ResetsAt
								}
								if int(w.UsedPercent+0.5) > worstPct {
									worstPct = int(w.UsedPercent + 0.5)
								}
							}
							if blocked {
								st.QuotaState = "blocked"
								st.QuotaOK = false
							}
							if worstPct > 0 && st.Quota == nil {
								st.Quota = &agent.QuotaInfo{
									PercentUsed: worstPct,
									ResetDate:   worstReset,
								}
							}
						}
						// Translate upstream HarnessInfo into the local
						// RoutingSignalSnapshot shape so the --json consumers
						// (and human text below) keep their existing shape.
						signal := harnessInfoToRoutingSignal(hi, now)
						if signal.Provider != "" {
							st.RoutingSignal = &signal
						}
					}
					if st.QuotaState == "" {
						st.QuotaState = "unknown"
					}
					if hi.Error != "" {
						st.Error = hi.Error
					}
					entries = append(entries, routingEntry{Name: hi.Name, State: st})
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

			svc, err := agent.NewServiceFromWorkDir(f.WorkingDir)
			if err != nil {
				return fmt.Errorf("constructing agent service: %w", err)
			}
			harnesses, err := svc.ListHarnesses(cmd.Context())
			if err != nil {
				return fmt.Errorf("listing harnesses: %w", err)
			}
			available := 0
			functional := 0
			for _, hi := range harnesses {
				statusStr := "NOT FOUND"
				if hi.Available {
					available++
					statusStr = fmt.Sprintf("OK (%s)", hi.Path)

					// Optionally test provider connectivity
					if checkConnectivity {
						providerStatus := agent.TestProviderConnectivityViaService(cmd.Context(), f.WorkingDir, hi.Name, probeTimeout)
						if providerStatus.Reachable && providerStatus.CreditsOK {
							statusStr = fmt.Sprintf("OK (%s) ✓ provider reachable", hi.Path)
							functional++
						} else if providerStatus.Reachable && !providerStatus.CreditsOK {
							statusStr = fmt.Sprintf("⚠️  (%s) provider out of credits/quota", hi.Path)
						} else if providerStatus.Error != "" {
							statusStr = fmt.Sprintf("⚠️  (%s) %s", hi.Path, providerStatus.Error)
						}
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s %s\n", hi.Name, statusStr)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "\n%d/%d harnesses available", available, len(harnesses))
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
	cmd.Flags().Bool("workers", false, "Show runtime worker registry (server-first; .ddx/workers/ fallback)")
	cmd.Flags().String("timeout", "", "Timeout for connectivity checks (default 15s)")
	cmd.Flags().Bool("json", false, "Output as JSON (with --routing or --workers)")
	return cmd
}

// harnessHealthyViaService reports whether the upstream service has an active
// failure cooldown recorded against the given harness. The upstream service's
// RouteStatus is the authoritative health source, so one worker's
// RecordRouteAttempt benefits every other consumer. When RouteStatus is
// unavailable (e.g. fresh service with no routes yet) the harness is
// considered healthy.
func harnessHealthyViaService(ctx context.Context, svc agentlib.FizeauService, harness string) bool {
	if svc == nil || harness == "" {
		return true
	}
	report, err := svc.RouteStatus(ctx)
	if err != nil || report == nil {
		return true
	}
	for _, route := range report.Routes {
		for _, cand := range route.Candidates {
			if cand.Provider != harness {
				continue
			}
			if !cand.Healthy {
				return false
			}
		}
	}
	return true
}

// harnessInfoToRoutingSignal translates an upstream HarnessInfo into the
// RoutingSignalSnapshot shape consumers still emit. Introduced when the
// DDx-side provider-native parsers were retired (ddx-7bc0c8d5): upstream
// quota.Status values ("ok|stale|unavailable") map to the existing
// freshness vocabulary so the JSON output for `ddx agent doctor --routing`
// keeps its previous field layout.
func harnessInfoToRoutingSignal(info agentlib.HarnessInfo, now time.Time) agent.RoutingSignalSnapshot {
	if info.Quota == nil && len(info.UsageWindows) == 0 && info.Account == nil {
		return agent.RoutingSignalSnapshot{}
	}

	snap := agent.RoutingSignalSnapshot{Provider: info.Name}
	if info.Account != nil && (info.Account.Email != "" || info.Account.PlanType != "" || info.Account.OrgName != "") {
		snap.Account = &agent.AccountInfo{
			Email:    info.Account.Email,
			PlanType: info.Account.PlanType,
			OrgName:  info.Account.OrgName,
		}
	}

	if info.Quota != nil {
		freshness := "fresh"
		if !info.Quota.Fresh {
			freshness = "stale"
		}
		if info.Quota.Status == "unavailable" || info.Quota.Status == "unauthenticated" {
			freshness = "unknown"
		}
		kind := info.Quota.Source
		if kind == "" {
			kind = "stats-cache"
		}
		var ageSeconds int64
		if !info.Quota.CapturedAt.IsZero() {
			if age := now.UTC().Sub(info.Quota.CapturedAt.UTC()); age > 0 {
				ageSeconds = int64(age.Seconds())
			}
		}
		meta := agent.SignalSourceMetadata{
			Provider:   info.Name,
			Kind:       kind,
			ObservedAt: info.Quota.CapturedAt.UTC(),
			Freshness:  freshness,
			AgeSeconds: ageSeconds,
		}
		state := "unknown"
		switch info.Quota.Status {
		case "ok":
			state = "ok"
		case "stale":
			state = "ok"
		}
		var usedPercent, windowMinutes int
		var resetsAt string
		for _, w := range info.Quota.Windows {
			snap.QuotaWindows = append(snap.QuotaWindows, agent.QuotaWindow{
				Name:          w.Name,
				LimitID:       w.LimitID,
				WindowMinutes: w.WindowMinutes,
				UsedPercent:   w.UsedPercent,
				ResetsAt:      w.ResetsAt,
				ResetsAtUnix:  w.ResetsAtUnix,
				State:         w.State,
			})
			if w.LimitID == "extra" {
				continue
			}
			if w.State == "blocked" {
				state = "blocked"
				resetsAt = w.ResetsAt
			}
			if int(w.UsedPercent+0.5) > usedPercent {
				usedPercent = int(w.UsedPercent + 0.5)
				windowMinutes = w.WindowMinutes
			}
		}
		snap.Source = meta
		snap.CurrentQuota = agent.QuotaSignal{
			Source:        meta,
			State:         state,
			UsedPercent:   usedPercent,
			WindowMinutes: windowMinutes,
			ResetsAt:      resetsAt,
		}
		snap.HistoricalUsage.Source = meta
	}

	for _, u := range info.UsageWindows {
		snap.HistoricalUsage.InputTokens += u.InputTokens
		snap.HistoricalUsage.OutputTokens += u.OutputTokens
		snap.HistoricalUsage.TotalTokens += u.TotalTokens
	}

	return snap
}

func (f *CommandFactory) newAgentLogCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log [session-id]",
		Short: "Show agent session history",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logDir := agent.SessionLogDirForWorkDir(f.WorkingDir)
			indexEntries, err := agent.ReadSessionIndex(logDir, agent.SessionIndexQuery{})
			if err != nil {
				return err
			}
			if len(indexEntries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No agent sessions recorded.")
				return nil
			}
			sessions := make([]agent.SessionEntry, 0, len(indexEntries))
			for _, entry := range indexEntries {
				sessions = append(sessions, agent.SessionIndexEntryToLegacy(entry))
			}

			// If session ID specified, show that one
			if len(args) > 0 {
				for _, entry := range sessions {
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
				for _, entry := range sessions {
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
			if len(sessions) > limit {
				sessions = sessions[:limit]
			}

			for _, entry := range sessions {
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
	cmd.AddCommand(&cobra.Command{
		Use:   "reindex",
		Short: "Migrate legacy sessions.jsonl into monthly session shards",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			logDir := agent.SessionLogDirForWorkDir(f.WorkingDir)
			count, err := agent.ReindexLegacySessions(f.WorkingDir, logDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "indexed %d legacy sessions\n", count)
			return nil
		},
	})
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

			rcfg, _ := config.LoadAndResolve(f.WorkingDir, config.CLIOverrides{})
			result, err := agent.RunBenchmarkWithConfigViaService(cmd.Context(), f.WorkingDir, rcfg, suite)
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

			// Reachability warnings (ddx-5538aa5b AC#4): for each tier surface,
			// probe the agent service to see whether the catalog model resolves
			// to a healthy route. A tier with zero reachable surfaces would
			// produce "no viable provider" in execute-loop; surface that gap
			// here so users discover it before invoking the queue.
			projectFlag, _ := cmd.Flags().GetString("project")
			projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)
			svc, svcErr := agent.NewServiceFromWorkDir(projectRoot)
			if svcErr == nil && svc != nil {
				ctx := cmd.Context()
				if ctx == nil {
					ctx = context.Background()
				}
				var warnings []string
				for _, tier := range []string{"smart", "standard", "cheap"} {
					def, ok := cat.Tiers[tier]
					if !ok || len(def.Surfaces) == 0 {
						continue
					}
					anyReachable := false
					var perSurface []string
					for surface, model := range def.Surfaces {
						_, err := svc.ResolveRoute(ctx, agentlib.RouteRequest{
							Model: model,
						})
						if err == nil {
							anyReachable = true
							break
						}
						perSurface = append(perSurface, fmt.Sprintf("    %s (%s): %v", surface, model, err))
					}
					if !anyReachable {
						warnings = append(warnings, fmt.Sprintf("  [%s] no surface resolves to a healthy route:\n%s", tier, strings.Join(perSurface, "\n")))
					}
				}
				if len(warnings) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "\nWarning: tiers with no reachable provider:\n")
					for _, w := range warnings {
						fmt.Fprintln(cmd.OutOrStdout(), w)
					}
				}
			}
			return nil
		},
	}
	showCmd.Flags().String("project", "", "Project root (default: current directory)")

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
						out, err := gitpkg.Command(cmd.Context(), "", "rev-parse", shaStr+"^").Output()
						if err == nil {
							baseCommit = strings.TrimSpace(string(out))
							fmt.Fprintf(cmd.OutOrStdout(), "Base commit: %s (parent of %s)\n", baseCommit, shaStr)
						}
					}
				}
			}

			// If no base commit determined, use current HEAD
			if baseCommit == "" {
				out, err := gitpkg.Command(cmd.Context(), "", "rev-parse", "HEAD").Output()
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
					gitCmd := gitpkg.Command(cmd.Context(), workDir, "checkout", baseCommit)
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
			rcfg, _ := config.LoadAndResolve(f.WorkingDir, config.CLIOverrides{
				Harness: harness,
				Model:   model,
			})
			result, err := agent.RunWithConfigViaService(cmd.Context(), f.WorkingDir, rcfg, agent.AgentRunRuntime{
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
				gitCmd := gitpkg.Command(cmd.Context(), workDir, "diff", "--stat")
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

execute-loop runs inline in the current process; per ADR-022 there is no
separate "submit to server" mode. The legacy --local flag is accepted but
ignored (deprecation warning printed) and will be removed in a future release.

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
  ddx agent execute-loop --profile default --once

  # Run continuously as a bounded queue worker
  ddx agent execute-loop --poll-interval 30s

  # Force a specific harness/model for a debugging pass
  ddx agent execute-loop --once --harness codex
  ddx agent execute-loop --once --harness agent --model minimax/minimax-m2.7`,
		Args: cobra.NoArgs,
		RunE: f.runAgentExecuteLoop,
	}
	cmd.Flags().String("project", "", "Target project root path or name (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().String("from", "", "Base git revision to start from (default: HEAD)")
	cmd.Flags().String("harness", "", "Agent harness to use")
	cmd.Flags().String("model", "", "Model override")
	cmd.Flags().String("profile", agent.DefaultRoutingProfile, "Routing profile: default, cheap, fast, or smart")
	cmd.Flags().String("provider", "", "Provider name (e.g. vidar, openrouter); selects a named provider from config")
	cmd.Flags().String("model-ref", "", "Model catalog reference (e.g. code-medium); resolved via the model catalog")
	cmd.Flags().String("effort", "", "Effort level")
	cmd.Flags().Bool("once", false, "Process at most one ready bead")
	cmd.Flags().Duration("poll-interval", 30*time.Second, "Poll interval for continuous scanning; zero drains current ready work and exits (legacy opt-out). Default 30s keeps the worker alive across empty polls.")
	cmd.Flags().Bool("json", false, "Output loop result as JSON")
	cmd.Flags().Bool("local", false, "Deprecated: no-op; execute-loop always runs inline (ADR-022)")
	_ = cmd.Flags().MarkDeprecated("local", "execute-loop always runs inline; the flag is a no-op (ADR-022)")
	cmd.Flags().Bool("no-review", false, "Skip post-merge review (e.g. for doc-only beads or tight iteration loops)")
	cmd.Flags().String("review-harness", "", "Harness to use for the post-merge reviewer (default: same as implementation harness)")
	cmd.Flags().String("review-model", "", "Model override for the post-merge reviewer (default: smart tier)")
	cmd.Flags().Float64("max-cost", escalation.DefaultMaxCostUSD, "Stop the loop when accumulated billed cost exceeds USD; 0 = unlimited; subscription and local providers do not count")
	cmd.Flags().Duration("request-timeout", 0, "Per-request provider wall-clock timeout (e.g. 60m, 2h); overrides project config and model-class defaults. Use when a thinking model needs more than the default 15 min.")
	cmd.Flags().Int("min-power", 0, "Minimum model power required (0 = unconstrained); passed to agent routing unchanged")
	cmd.Flags().Int("max-power", 0, "Maximum model power allowed (0 = unconstrained); passed to agent routing unchanged")
	return cmd
}

func (f *CommandFactory) runAgentExecuteLoop(cmd *cobra.Command, args []string) error {
	return f.runAgentExecuteLoopImpl(cmd, false, "")
}

// hostnameOrEmpty returns os.Hostname() or "" on error. Used to populate
// the workerprobe identity envelope without short-circuiting probe startup
// on a transient hostname-lookup failure.
func hostnameOrEmpty() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

// runAgentExecuteLoopImpl is the shared implementation for runAgentExecuteLoop
// and runWork. When treatPassthroughAsOpaque is true (ddx work path),
// harness/provider/model are passed to Execute unchanged and
// ValidateForExecuteLoopViaService is not called; DDx does not inspect or
// branch on their values. When false (ddx agent execute-loop path), the
// pre-flight validation gate runs first.
func (f *CommandFactory) runAgentExecuteLoopImpl(cmd *cobra.Command, treatPassthroughAsOpaque bool, tryTargetBeadID string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)
	fromRev, _ := cmd.Flags().GetString("from")
	harness, _ := cmd.Flags().GetString("harness")
	model, _ := cmd.Flags().GetString("model")
	profile, _ := cmd.Flags().GetString("profile")
	provider, _ := cmd.Flags().GetString("provider")
	modelRef, _ := cmd.Flags().GetString("model-ref")
	effort, _ := cmd.Flags().GetString("effort")
	once, _ := cmd.Flags().GetBool("once")
	pollInterval, _ := cmd.Flags().GetDuration("poll-interval")
	if tryTargetBeadID != "" {
		// `ddx try <bead-id>` is a single-bead one-shot: target one bead and
		// exit after one attempt regardless of CLI flag state.
		once = true
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	noReview, _ := cmd.Flags().GetBool("no-review")
	reviewHarness, _ := cmd.Flags().GetString("review-harness")
	reviewModel, _ := cmd.Flags().GetString("review-model")
	maxCostUSD, _ := cmd.Flags().GetFloat64("max-cost")
	requestTimeout, _ := cmd.Flags().GetDuration("request-timeout")
	minPower, _ := cmd.Flags().GetInt("min-power")
	maxPower, _ := cmd.Flags().GetInt("max-power")

	// Zero-config auto-route: when the operator passes no routing flags and
	// the project has no .ddx/config.yaml routing block, dispatch flows
	// through fizeau's provider resolution. Provider configuration and any
	// "no providers configured" error reporting is fizeau's concern; ddx
	// does not read fizeau's config files.
	//
	// ddx-b2c9a245: tier is inferred per bead via escalation.InferTier in
	// the executor closure below when no routing flags are present.
	noRoutingFlags := harness == "" && model == "" && provider == "" && modelRef == "" &&
		!cmd.Flags().Changed("profile")
	autoInferTier := noRoutingFlags && !projectHasRoutingConfig(projectRoot)

	// ADR-022: execute-loop always runs inline. The legacy --local flag is
	// a deprecated no-op and the server-submit path is removed from this CLI
	// surface — server-spawned workers exec `ddx work` directly.

	// Pre-flight: validate harness availability and model compatibility
	// before claiming any beads. This surfaces errors like "claude binary
	// not found" or "vidar is an agent preset, not a claude model" before
	// any bead status changes hands.
	// Skipped for ddx work (treatPassthroughAsOpaque=true): harness/provider/model
	// are opaque passthrough constraints there; DDx does not validate them.
	if !treatPassthroughAsOpaque {
		if err := agent.ValidateForExecuteLoopViaService(cmd.Context(), f.WorkingDir, harness, model, provider, modelRef); err != nil {
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

	// ADR-022 step 2: spin up the worker server-probe + best-effort event
	// mirror. The probe is autonomous-friendly: if no ddx-server is
	// reachable, it buffers events into a 200-cap ring; on
	// NotConnected→Connected transitions it registers and flushes the
	// backfill. Per ADR-022 the worker keeps working regardless of probe
	// state, so this code never blocks the loop on network I/O.
	probeIdent := workerprobe.Identity{
		ProjectRoot:  projectRoot,
		Harness:      harness,
		Model:        model,
		ExecutorPID:  os.Getpid(),
		ExecutorHost: hostnameOrEmpty(),
		StartedAt:    time.Now().UTC(),
	}
	probe := workerprobe.New(probeIdent, workerprobe.Config{
		AddrFunc: serverpkg.ReadServerAddr,
	})
	probeCtx, probeCancel := context.WithCancel(context.Background())
	probe.Start(probeCtx)
	defer func() {
		probeCancel()
		probe.Stop()
	}()
	if loopSink != nil {
		loopSink = workerprobe.TeeJSONL(loopSink, probe)
	} else {
		loopSink = workerprobe.TeeJSONL(io.Discard, probe)
	}

	// Start session log tailer so the user sees progress during agent execution
	tailCtx, tailCancel := context.WithCancel(context.Background())
	go agent.TailSessionLogs(tailCtx, projectRoot, cmd.OutOrStdout())

	// Instantiate a process-local LandCoordinator for the inline land path.
	// Stopped on function exit.
	localCoord := serverpkg.NewLocalLandCoordinator(projectRoot, agent.RealLandingGitOps{})
	defer localCoord.Stop()

	// Build post-merge reviewer unless --no-review is set.
	// Reviewer is on-by-default: runs at smart tier after every successful merge.
	var reviewer agent.BeadReviewer
	if !noReview {
		reviewer = &agent.DefaultBeadReviewer{
			ProjectRoot: projectRoot,
			BeadStore:   bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			BeadEvents:  bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			Harness:     reviewHarness,
			Model:       reviewModel,
		}
	}

	profile = agent.NormalizeRoutingProfile(profile)

	// Cost-cap state shared across attempts for this loop run.
	// paths. Accumulated billed spend (excluding local and subscription
	// providers) above --max-cost trips the cap and halts further bead
	// claiming. See escalation.DefaultMaxCostUSD / CountsTowardCostCap.
	costCap := escalation.NewCostCapTracker(maxCostUSD, func(harnessName string) bool {
		// Resolve the harness's billing class via the service. Treat any
		// resolution error as "count by default" (safe — we'd rather cap
		// early than silently overshoot).
		svc, svcErr := agent.NewServiceFromWorkDir(f.WorkingDir)
		if svcErr != nil {
			return true
		}
		infos, err := svc.ListHarnesses(context.Background())
		if err != nil {
			return true
		}
		for _, h := range infos {
			if h.Name == harnessName {
				return escalation.CountsTowardCostCap(h.IsLocal, h.IsSubscription, h.CostClass)
			}
		}
		return true
	})
	accumulateBilledCost := func(report agent.ExecuteBeadReport) {
		costCap.Add(report.Harness, report.CostUSD)
	}
	costCapTripped := func() (agent.ExecuteBeadReport, bool) {
		detail, tripped := costCap.Tripped()
		if !tripped {
			return agent.ExecuteBeadReport{}, false
		}
		return agent.ExecuteBeadReport{
			Status: agent.ExecuteBeadStatusExecutionFailed,
			Detail: detail,
		}, true
	}

	// singleTierAttempt runs one execution attempt with an explicit harness
	// and model. It is called both by the non-escalating path and by each
	// iteration of the tier escalation loop.
	singleTierAttempt := func(ctx context.Context, beadID string, tier escalation.ModelTier, resolvedHarness, resolvedProvider, resolvedModel string) (agent.ExecuteBeadReport, error) {
		gitOps := &agent.RealGitOps{}
		attemptProvider := provider
		if resolvedProvider != "" {
			attemptProvider = resolvedProvider
		}

		// ddx-b2c9a245: when zero-config auto-route is active, derive the
		// per-attempt profile from the inferred tier rather than the static
		// "always cheap" fallback the previous hot-fix used.
		attemptProfile := profile
		if autoInferTier && tier != "" {
			attemptProfile = escalation.TierToProfile(tier)
		}

		loopOverrides := config.CLIOverrides{
			Harness:           resolvedHarness,
			Model:             resolvedModel,
			Provider:          attemptProvider,
			ModelRef:          modelRef,
			Profile:           attemptProfile,
			Effort:            effort,
			MinPower:          minPower,
			MaxPower:          maxPower,
			OpaquePassthrough: treatPassthroughAsOpaque,
		}
		if requestTimeout > 0 {
			loopOverrides.ProviderRequestTimeout = &requestTimeout
		}
		attemptRcfg, _ := config.LoadAndResolve(projectRoot, loopOverrides)
		res, execErr := agent.ExecuteBeadWithConfig(ctx, projectRoot, beadID, attemptRcfg, agent.ExecuteBeadRuntime{
			FromRev:    fromRev,
			BeadEvents: bead.NewStore(filepath.Join(projectRoot, ".ddx")),
		}, gitOps)
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
			ActualPower:        res.ActualPower,
			Tier:               tierStr,
			Status:             res.Status,
			Detail:             res.Detail,
			SessionID:          res.SessionID,
			BaseRev:            res.BaseRev,
			ResultRev:          res.ResultRev,
			PreserveRef:        res.PreserveRef,
			NoChangesRationale: res.NoChangesRationale,
			CostUSD:            res.CostUSD,
			DurationMS:         int64(res.DurationMS),
		}, nil
	}

	worker := &agent.ExecuteBeadWorker{
		Store:    store,
		Reviewer: reviewer,
		Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			// Stop AT THE START of each new bead claim if the cost cap has
			// already tripped — otherwise we'd burn one extra attempt before
			// halting the queue.
			if cappedReport, capped := costCapTripped(); capped {
				cappedReport.BeadID = beadID
				return cappedReport, nil
			}
			// Pass operator intent (harness/provider/model) directly to
			// Execute via singleTierAttempt; do not pre-resolve the route
			// (CONTRACT-003 / ddx-da19756a).
			//
			// ddx-b2c9a245: under zero-config auto-route, infer the tier
			// from bead metadata (labels, kind, scope) and pass it through
			// so singleTierAttempt selects the matching profile.
			var inferredTier escalation.ModelTier
			if autoInferTier {
				if b, getErr := store.Get(beadID); getErr == nil {
					inferredTier = escalation.InferTier(b)
				} else {
					inferredTier = escalation.TierCheap
				}
			}
			report, err := singleTierAttempt(ctx, beadID, inferredTier, harness, provider, model)
			if err == nil {
				accumulateBilledCost(report)
				if cappedReport, capped := costCapTripped(); capped {
					cappedReport.BeadID = beadID
					return cappedReport, nil
				}
			}
			return report, err
		}),
	}

	// SD-024 Stage 1: dispatch flows through LoadAndResolve + RunWithConfig
	// so .ddx/config.yaml's durable knobs (review_max_retries,
	// no_progress_cooldown, max_no_changes_before_close, heartbeat_interval,
	// agent.harness/model/permissions, etc.) reach the running loop without
	// per-knob CLI plumbing. CLI flag values feed CLIOverrides which win
	// over the on-disk config when set.
	overrides := config.CLIOverrides{
		Assignee: resolveClaimAssignee(),
		Harness:  harness,
		Model:    model,
		Provider: provider,
		ModelRef: modelRef,
		Profile:  profile,
		Effort:   effort,
	}
	rcfg, err := config.LoadAndResolve(projectRoot, overrides)
	if err != nil {
		// Surface the LoadAndResolve error rather than letting a
		// zero-value ResolvedConfig flow into RunWithConfig — the
		// SD-024 sealed-construction sentinel would otherwise panic
		// at first accessor read instead of producing a clean CLI error.
		tailCancel()
		return fmt.Errorf("load resolved config: %w", err)
	}

	cliLandingOps := agent.RealLandingGitOps{}
	result, err := worker.Run(cmd.Context(), rcfg, agent.ExecuteBeadLoopRuntime{
		Once:         once,
		PollInterval: pollInterval,
		Log:          cmd.OutOrStdout(),
		EventSink:    loopSink,
		WorkerID:     resolveClaimAssignee(),
		ProjectRoot:  projectRoot,
		SessionID:    loopSessionID,
		PreClaimHook: buildCLIPreClaimHook(projectRoot, cliLandingOps),
		NoReview:     noReview,
		TargetBeadID: tryTargetBeadID,
	})
	tailCancel() // stop session log tailer
	if err != nil {
		return err
	}

	if tryTargetBeadID != "" {
		return formatTryResult(cmd, projectRoot, tryTargetBeadID, result, asJSON)
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
		d := result.NoReadyWorkDetail
		if len(d.SkippedEpics) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "  skipped %d ready epic(s) (epics are structural containers; decompose into tasks): %s\n",
				len(d.SkippedEpics), strings.Join(d.SkippedEpics, ", "))
		}
		if len(d.SkippedOnCooldown) > 0 {
			retryHint := ""
			if d.NextRetryAfter != "" {
				retryHint = " (next retry-after: " + d.NextRetryAfter + ")"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  skipped %d bead(s) on retry cooldown%s: %s\n",
				len(d.SkippedOnCooldown), retryHint, strings.Join(d.SkippedOnCooldown, ", "))
		}
		if len(d.SkippedNotEligible) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "  skipped %d bead(s) with execution-eligible=false: %s\n",
				len(d.SkippedNotEligible), strings.Join(d.SkippedNotEligible, ", "))
		}
		if len(d.SkippedSuperseded) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "  skipped %d superseded bead(s): %s\n",
				len(d.SkippedSuperseded), strings.Join(d.SkippedSuperseded, ", "))
		}
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

func formatTryResult(cmd *cobra.Command, projectRoot, beadID string, result *agent.ExecuteBeadLoopResult, asJSON bool) error {
	if asJSON {
		payload := struct {
			ProjectRoot string `json:"project_root"`
			BeadID      string `json:"bead_id"`
			*agent.ExecuteBeadLoopResult
		}{
			ProjectRoot:           projectRoot,
			BeadID:                beadID,
			ExecuteBeadLoopResult: result,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			return err
		}
	}

	if result == nil || result.NoReadyWork {
		fmt.Fprintf(cmd.ErrOrStderr(), "bead is not execution-ready: %s\n", beadID)
		return &ExitError{Code: tryExitFailed, Message: ""}
	}
	if len(result.Results) == 0 {
		return &ExitError{Code: tryExitFailed, Message: ""}
	}

	report := result.Results[0]
	if !asJSON {
		fmt.Fprintf(cmd.OutOrStdout(), "\nbead: %s\nstatus: %s\n", report.BeadID, report.Status)
		if report.Detail != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "detail: %s\n", report.Detail)
		}
		if report.ResultRev != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "result_rev: %s\n", report.ResultRev)
		}
		if report.PreserveRef != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "preserve_ref: %s\n", report.PreserveRef)
		}
	}
	return tryExitCodeForStatus(report.Status)
}

// executeLoopWithServer submits an execute-loop job to the running ddx server.
// The server starts a background worker and returns its ID.
func (f *CommandFactory) executeLoopWithServer(cmd *cobra.Command, projectRoot, harness, model, profile, provider, modelRef, effort string, once bool, pollInterval time.Duration, asJSON bool, noReview bool, reviewHarness, reviewModel string, opaquePassthrough bool) error {
	serverBase := resolveServerURL(projectRoot)

	workerSpec := map[string]any{
		"once":         once,
		"project_root": projectRoot,
	}
	if opaquePassthrough {
		workerSpec["opaque_passthrough"] = true
	}
	if harness != "" {
		workerSpec["harness"] = harness
	}
	if model != "" {
		workerSpec["model"] = model
	}
	if profile != "" {
		workerSpec["profile"] = profile
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

// buildCLIPreClaimHook returns a PreClaimHook for the inline execute-loop
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
		out, err := gitpkg.Command(context.Background(), "", "rev-parse", "--show-toplevel").Output()
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
	cmd := gitpkg.Command(context.Background(), repoRoot, "worktree", "add", wtDir, "-b", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		// If branch already exists, try without -b
		cmd2 := gitpkg.Command(context.Background(), repoRoot, "worktree", "add", wtDir, name)
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			return "", fmt.Errorf("git worktree add failed: %s\n%s", string(out), string(out2))
		}
	}

	return wtDir, nil
}

// projectHasRoutingConfig reports whether the project at projectRoot has a
// .ddx/config.yaml that supplies routing inputs (agent.harness, agent.model,
// or agent.endpoints). When false, ddx work and ddx agent execute-loop fall
// through to the global agent config defaults instead of demanding per-project
// flags. Tracks ddx-b790449b AC1.
func projectHasRoutingConfig(projectRoot string) bool {
	if projectRoot == "" {
		return false
	}
	cfgPath := filepath.Join(projectRoot, ".ddx", "config.yaml")
	if _, err := os.Stat(cfgPath); err != nil {
		return false
	}
	cfg, err := config.LoadWithWorkingDir(projectRoot)
	if err != nil || cfg == nil || cfg.Agent == nil {
		return false
	}
	a := cfg.Agent
	if strings.TrimSpace(a.Harness) != "" || strings.TrimSpace(a.Model) != "" {
		return true
	}
	if len(a.Endpoints) > 0 {
		return true
	}
	return false
}
