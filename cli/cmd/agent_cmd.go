package cmd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	tierescalation "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/agent/workerprobe"
	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/DocumentDrivenDX/ddx/internal/serverreg"
	agentlib "github.com/easel/fizeau"
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
  ddx run --harness codex --min-power 10 --prompt task.md
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
		Args: cobra.NoArgs,
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
	cmd.AddCommand(f.newAgentExecutionsCommand())
	cmd.AddCommand(f.newAgentWorkersCommand())
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
			if strings.EqualFold(strings.TrimSpace(harness), "codex") && rcfg.Model() == "" && rcfg.Profile() == "" {
				return fmt.Errorf("agent run --harness codex is under-specified: supply --model or --profile, or use `ddx run --harness codex --min-power <n>` for Fizeau power-based routing")
			}
			if err := agent.ValidateEffortForRunViaService(cmd.Context(), f.WorkingDir, rcfg.Profile(), rcfg.Effort()); err != nil {
				return err
			}
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
			// Load config to detect model overrides. Harness selection is a
			// per-invocation passthrough, not durable project config.
			cfg, _ := config.LoadWithWorkingDir(f.WorkingDir)
			var configModel string
			var configModels map[string]string
			if cfg != nil && cfg.Agent != nil {
				configModel = cfg.Agent.Model
				configModels = cfg.Agent.Models
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
			fmt.Fprintf(cmd.OutOrStdout(), "\nConfig example (~/.ddx.yml):\n  agent:\n    models:\n      %s: <model-name>\n", harness)
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
						if signal.Source.Kind != "" {
							line += fmt.Sprintf("  source: %s/%s", signal.Source.Provider, signal.Source.Kind)
						} else if signal.Source.Provider != "" {
							line += fmt.Sprintf("  source: %s", signal.Source.Provider)
						}
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
		kind := agent.NormalizeSignalSourceKind(info.Quota.Source)
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

func parseExecuteLoopSpec(cmd *cobra.Command, treatPassthroughAsOpaque bool) (executeloop.ExecuteLoopSpec, executeloop.DispatchOptions, error) {
	projectRoot, _ := cmd.Flags().GetString("project")
	fromRev, _ := cmd.Flags().GetString("from")
	harness, _ := cmd.Flags().GetString("harness")
	model, _ := cmd.Flags().GetString("model")
	profile, _ := cmd.Flags().GetString("profile")
	provider, _ := cmd.Flags().GetString("provider")
	modelRef, _ := cmd.Flags().GetString("model-ref")
	effort, _ := cmd.Flags().GetString("effort")
	once, _ := cmd.Flags().GetBool("once")
	watch, _ := cmd.Flags().GetBool("watch")
	rawIdleInterval, _ := cmd.Flags().GetDuration("idle-interval")
	asJSON, _ := cmd.Flags().GetBool("json")
	dispatchJSON := ""
	if asJSON {
		dispatchJSON = "true"
	}
	local, _ := cmd.Flags().GetBool("local")
	noReview, _ := cmd.Flags().GetBool("no-review")
	noReviewAck, _ := cmd.Flags().GetBool("no-review-i-know-what-im-doing")
	reviewHarness, _ := cmd.Flags().GetString("review-harness")
	reviewModel, _ := cmd.Flags().GetString("review-model")
	maxCostUSD, _ := cmd.Flags().GetFloat64("max-cost")
	maxBeadCostUSD, _ := cmd.Flags().GetFloat64("max-bead-cost")
	maxRecoveryCostUSD, _ := cmd.Flags().GetFloat64("max-recovery-cost")
	requestTimeout, _ := cmd.Flags().GetDuration("request-timeout")
	rateLimitMaxWait, _ := cmd.Flags().GetDuration("rate-limit-max-wait")
	minPower, _ := cmd.Flags().GetInt("min-power")
	maxPower, _ := cmd.Flags().GetInt("max-power")

	if noReview && !noReviewAck {
		return executeloop.ExecuteLoopSpec{}, executeloop.DispatchOptions{}, fmt.Errorf("--no-review requires --no-review-i-know-what-im-doing (break-glass acknowledgement)")
	}
	if once && watch {
		return executeloop.ExecuteLoopSpec{}, executeloop.DispatchOptions{}, fmt.Errorf("--once and --watch are mutually exclusive")
	}
	if !watch && cmd.Flags().Changed("idle-interval") {
		return executeloop.ExecuteLoopSpec{}, executeloop.DispatchOptions{}, fmt.Errorf("--idle-interval only applies in watch mode; pass --watch")
	}

	mode := executeloop.ModeDrain
	var idleInterval executeloop.Duration
	if once {
		mode = executeloop.ModeOnce
	} else if watch {
		mode = executeloop.ModeWatch
		idleInterval = executeloop.Duration{Duration: rawIdleInterval}
	}

	spec := executeloop.ExecuteLoopSpec{
		ProjectRoot:        projectRoot,
		Harness:            harness,
		Model:              model,
		Profile:            profile,
		Provider:           provider,
		ModelRef:           modelRef,
		Effort:             effort,
		Mode:               mode,
		IdleInterval:       idleInterval,
		NoReview:           noReview,
		ReviewHarness:      reviewHarness,
		ReviewModel:        reviewModel,
		OpaquePassthrough:  treatPassthroughAsOpaque,
		MaxCostUSD:         maxCostUSD,
		MaxBeadCostUSD:     maxBeadCostUSD,
		MaxRecoveryCostUSD: maxRecoveryCostUSD,
		RequestTimeout:     executeloop.Duration{Duration: requestTimeout},
		RateLimitMaxWait:   executeloop.Duration{Duration: rateLimitMaxWait},
		MinPower:           minPower,
		MaxPower:           maxPower,
		FromRev:            fromRev,
	}
	spec.ApplyDefaults()
	if err := spec.Validate(); err != nil {
		return executeloop.ExecuteLoopSpec{}, executeloop.DispatchOptions{}, err
	}

	return spec, executeloop.DispatchOptions{Local: local, JSON: dispatchJSON}, nil
}

func executeLoopAttemptRuntime(spec executeloop.ExecuteLoopSpec, output io.Writer, events agent.BeadEventAppender, runner agent.AgentRunner, checker agent.ExecutionResourceChecker) agent.ExecuteBeadRuntime {
	return agent.ExecuteBeadRuntime{
		FromRev:          spec.FromRev,
		Output:           output,
		BeadEvents:       events,
		AgentRunner:      runner,
		ResourceChecker:  checker,
		RateLimitMaxWait: spec.RateLimitMaxWait.Duration,
	}
}

// runAgentExecuteLoopImpl is the implementation for runWork. When
// treatPassthroughAsOpaque is true, harness/provider/model are passed to Execute
// unchanged and ValidateForExecuteLoopViaService is not called; DDx does not
// inspect or branch on their values.
func (f *CommandFactory) runAgentExecuteLoopImpl(cmd *cobra.Command, treatPassthroughAsOpaque bool, tryTargetBeadID string) error {
	spec, dispatch, err := parseExecuteLoopSpec(cmd, treatPassthroughAsOpaque)
	if err != nil {
		return err
	}
	projectRoot := resolveProjectRoot(spec.ProjectRoot, f.WorkingDir)
	spec.ProjectRoot = projectRoot
	if tryTargetBeadID != "" {
		// `ddx try <bead-id>` is a single-bead one-shot: target one bead and
		// exit after one attempt regardless of CLI flag state.
		spec.Mode = executeloop.ModeOnce
		spec.IdleInterval = executeloop.Duration{}
	}

	// Zero-config auto-route: when the operator passes no routing flags and
	// the project has no .ddx/config.yaml routing block, dispatch flows
	// through fizeau's provider resolution. Provider configuration and any
	// "no providers configured" error reporting is fizeau's concern; ddx
	// does not read fizeau's config files.
	// ADR-022: queue work always runs inline. The legacy --local flag is
	// a deprecated no-op and the server-submit path is removed from this CLI
	// surface — server-spawned workers exec `ddx work` directly.
	noRoutingFlags := spec.Harness == "" && spec.Model == "" && spec.Provider == "" &&
		spec.ModelRef == "" && spec.Profile == "" && spec.MinPower == 0 &&
		spec.MaxPower == 0 && !cmd.Flags().Changed("harness") &&
		!cmd.Flags().Changed("model") && !cmd.Flags().Changed("provider") &&
		!cmd.Flags().Changed("model-ref") && !cmd.Flags().Changed("profile") &&
		!cmd.Flags().Changed("min-power") && !cmd.Flags().Changed("max-power")
	autoInferTier := noRoutingFlags && !projectHasRoutingConfig(projectRoot)

	// Pre-flight: validate harness availability and model compatibility
	// before claiming any beads. This surfaces errors like "claude binary
	// not found" or "vidar is an agent preset, not a claude model" before
	// any bead status changes hands.
	// Skipped for ddx work (treatPassthroughAsOpaque=true): harness/provider/model
	// are opaque passthrough constraints there; DDx does not validate them.
	if !spec.OpaquePassthrough {
		if err := agent.ValidateForExecuteLoopViaService(cmd.Context(), f.WorkingDir, spec.Harness, spec.Model, spec.Provider, spec.ModelRef); err != nil {
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
		Harness:      spec.Harness,
		Model:        spec.Model,
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

	// Instantiate a process-local LandCoordinator for the inline land path.
	// Stopped on function exit.
	localCoord := serverpkg.NewLocalLandCoordinator(projectRoot, agent.RealLandingGitOps{})
	defer localCoord.Stop()

	// Build post-merge reviewer unless --no-review is set.
	// Reviewer is on-by-default: runs at smart tier after every successful merge.
	var reviewer agent.BeadReviewer
	if !spec.NoReview {
		reviewer = &agent.DefaultBeadReviewer{
			ProjectRoot: projectRoot,
			BeadStore:   bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			BeadEvents:  bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			Harness:     spec.ReviewHarness,
			Model:       spec.ReviewModel,
		}
	}

	spec.Profile = agent.NormalizeRoutingProfile(spec.Profile)

	// SD-024 Stage 1: dispatch flows through LoadAndResolve + RunWithConfig
	// so .ddx/config.yaml's durable knobs (review_max_retries,
	// no_progress_cooldown, max_no_changes_before_close, heartbeat_interval,
	// agent.model/permissions, etc.) reach the running loop without
	// per-knob CLI plumbing. CLI flag values feed CLIOverrides which win
	// over the on-disk config when set.
	overrides := config.CLIOverrides{
		Assignee:          resolveClaimAssignee(),
		Harness:           spec.Harness,
		Model:             spec.Model,
		Provider:          spec.Provider,
		ModelRef:          spec.ModelRef,
		Profile:           spec.Profile,
		Effort:            spec.Effort,
		MinPower:          spec.MinPower,
		MaxPower:          spec.MaxPower,
		OpaquePassthrough: spec.OpaquePassthrough,
	}
	rcfg, err := config.LoadAndResolve(projectRoot, overrides)
	if err != nil {
		// Surface the LoadAndResolve error rather than letting a
		// zero-value ResolvedConfig flow into RunWithConfig — the
		// SD-024 sealed-construction sentinel would otherwise panic
		// at first accessor read instead of producing a clean CLI error.
		return fmt.Errorf("load resolved config: %w", err)
	}

	resourceChecker := buildCLIResourceChecker(projectRoot, f.resourceCheckerOverride)
	if _, err := resourceChecker.Check(cmd.Context()); err != nil {
		var resourceErr *agent.ResourceExhaustedError
		if errors.As(err, &resourceErr) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), agent.ResourceExhaustedStopMessage)
		}
		return err
	}

	var qualityRunner agent.AgentRunner
	if f.AgentRunnerOverride != nil {
		qualityRunner = f.AgentRunnerOverride
	}
	lintHook := agent.NewPreDispatchLintHook(projectRoot, store, rcfg, nil, qualityRunner)
	innerIntakeHook := agent.NewPreClaimIntakeHook(projectRoot, store, rcfg, nil, qualityRunner)
	intakeHook := agent.NewACQualityPreClaimGate(store, rcfg.BeadQualityMode(), rcfg.ACQualityMinScore(), innerIntakeHook)
	triageHook := agent.NewPostAttemptTriageHook(projectRoot, store, rcfg, nil, qualityRunner, nil)
	recoveryHook := agent.NewAutoRecoveryPostLadderExhaustionHook(store, qualityRunner, rcfg, projectRoot, agent.AutoRecoveryConfig{
		MaxRecoveryCostUSD: spec.MaxRecoveryCostUSD,
		MaxBeadCostUSD:     spec.MaxBeadCostUSD,
	})

	// harnessBilledLookup resolves whether a harness contributes to any cost
	// cap. Shared between the global CostCapTracker and per-bead trackers so
	// both use the same billing-class semantics.
	harnessBilledLookup := func(harnessName string) bool {
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
				return escalation.CountsTowardCostCap(h.CostClass == "local", h.Billing == agentlib.BillingModelSubscription, h.CostClass)
			}
		}
		return true
	}

	// Cost-cap state shared across attempts for this loop run.
	// Accumulated billed spend (excluding local and subscription providers)
	// above --max-cost trips the cap and halts further bead claiming.
	// See escalation.DefaultMaxCostUSD / CountsTowardCostCap.
	costCap := escalation.NewCostCapTracker(spec.MaxCostUSD, harnessBilledLookup)
	accumulateBilledCost := func(report agent.ExecuteBeadReport) {
		costCap.Add(report.Harness, report.CostUSD)
	}
	costCapTripped := func() (agent.ExecuteBeadReport, bool) {
		if _, tripped := costCap.Tripped(); !tripped {
			return agent.ExecuteBeadReport{}, false
		}
		spent := costCap.Spent()
		return agent.ExecuteBeadReport{
			Status: agent.ExecuteBeadStatusExecutionFailed,
			Detail: fmt.Sprintf("cost cap reached: $%.2f billed >= $%.2f cap; raise the cap or set 0 to disable. Subscription and local providers do not count.", spent, spec.MaxCostUSD),
		}, true
	}

	// singleTierAttempt runs one execution attempt with an explicit harness
	// and model. The caller controls MinPower per rung; this helper keeps the
	// profile stable and only advances the power floor.
	singleTierAttempt := func(ctx context.Context, beadID string, requestedMinPower int, resolvedHarness, resolvedProvider, resolvedModel string) (agent.ExecuteBeadReport, error) {
		gitOps := &agent.RealGitOps{}
		attemptProvider := spec.Provider
		if resolvedProvider != "" {
			attemptProvider = resolvedProvider
		}
		loopOverrides := config.CLIOverrides{
			Harness:           resolvedHarness,
			Model:             resolvedModel,
			Provider:          attemptProvider,
			ModelRef:          spec.ModelRef,
			Profile:           spec.Profile,
			Effort:            spec.Effort,
			MinPower:          requestedMinPower,
			MaxPower:          spec.MaxPower,
			OpaquePassthrough: spec.OpaquePassthrough,
		}
		requestTimeout := spec.RequestTimeout.Duration
		if requestTimeout > 0 {
			loopOverrides.ProviderRequestTimeout = &requestTimeout
		}
		attemptRcfg, _ := config.LoadAndResolve(projectRoot, loopOverrides)
		res, execErr := agent.ExecuteBeadWithConfig(ctx, projectRoot, beadID, attemptRcfg, executeLoopAttemptRuntime(
			spec,
			cmd.OutOrStdout(),
			bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			f.AgentRunnerOverride,
			resourceChecker,
		), gitOps)
		if execErr != nil && res == nil {
			return agent.ExecuteBeadReport{}, execErr
		}
		if res != nil && agent.IsResourceExhaustedStatus(res.Status) {
			return agent.ReportFromExecuteBeadResult(res, ""), nil
		}
		if execErr != nil {
			agent.MarkResultExecutionError(res, execErr)
			return agent.ReportFromExecuteBeadResult(res, ""), nil
		}
		if res != nil && res.ResultRev != "" && res.ResultRev != res.BaseRev && res.ExitCode == 0 {
			targetBead, _ := store.Get(beadID)
			landRes, _, landErr := agent.SubmitWithPreMergeChecks(
				ctx, projectRoot, targetBead, res,
				func(req agent.LandRequest) (*agent.LandResult, error) { return localCoord.Submit(req) },
				bead.NewStore(filepath.Join(projectRoot, ".ddx")),
				resolveClaimAssignee(), "ddx work",
				nil,
			)
			if landErr == nil {
				agent.ApplyLandResultToExecuteBeadResult(res, landRes)
				_ = agent.WriteExecuteBeadResultArtifact(projectRoot, res)
			} else {
				agent.MarkResultLandError(projectRoot, res, landErr)
				_ = agent.WriteExecuteBeadResultArtifact(projectRoot, res)
				return agent.ReportFromExecuteBeadResult(res, ""), nil
			}
		} else if res != nil && (res.Outcome == agent.ExecuteBeadOutcomeTaskFailed || res.ExitCode != 0) {
			if res.ResultRev != "" && res.ResultRev != res.BaseRev {
				res.Outcome = "preserved"
			} else {
				res.Outcome = "error"
			}
			res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
		} else if res != nil && res.ResultRev == res.BaseRev {
			res.Outcome = "no-changes"
			res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
		}
		return agent.ReportFromExecuteBeadResult(res, ""), nil
	}

	var ladderOnce sync.Once
	var ladder escalationFloorFinder
	loadLadder := func() escalationFloorFinder {
		ladderOnce.Do(func() {
			ladder = tierescalation.NewLadder(nil)
			svc, svcErr := agent.ResolveServiceFromWorkDir(projectRoot)
			if svcErr != nil {
				return
			}
			modelFilter := agentlib.ModelFilter{}
			if spec.Harness != "" {
				modelFilter.Harness = spec.Harness
			}
			modelCtx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
			defer cancel()
			models, listErr := svc.ListModels(modelCtx, modelFilter)
			if listErr != nil {
				return
			}
			ladder = tierescalation.NewLadder(models)
		})
		return ladder
	}

	worker := &agent.ExecuteBeadWorker{
		Store:    store,
		Reviewer: reviewer,
		EscalationNextFloor: func(actualPower int) (int, error) {
			return nextEscalationFloor(loadLadder(), actualPower)
		},
		Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			targetBead, err := store.Get(beadID)
			if err != nil {
				return agent.ExecuteBeadReport{}, err
			}
			initialMinPower, unavailableReport, unavailable := investigationRetryInitialMinPowerWithInference(targetBead, rcfg.MinPower(), spec.MaxPower, loadLadder(), autoInferTier)
			if unavailable {
				return unavailableReport, nil
			}
			// Build a per-bead tracker. A budget:<USD> label on the bead
			// overrides the --max-bead-cost default.
			perBeadBudget := spec.MaxBeadCostUSD
			if override, ok := escalation.ParseBeadBudgetLabel(targetBead.Labels); ok {
				perBeadBudget = override
			}
			perBeadTracker := escalation.NewPerBeadCostTracker(perBeadBudget, harnessBilledLookup)
			report, err := runEscalatingSingleTierAttempts(
				ctx,
				initialMinPower,
				loadLadder(),
				func(ctx context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
					return singleTierAttempt(ctx, beadID, requestedMinPower, spec.Harness, spec.Provider, spec.Model)
				},
				nil,
				perBeadTracker,
			)
			if err == nil {
				accumulateBilledCost(report)
			}
			return report, err
		}),
	}

	cliLandingOps := agent.RealLandingGitOps{}
	progressLog := cmd.OutOrStdout()
	cleanupLog := cmd.ErrOrStderr()
	jsonOutput := dispatch.JSON == "true"
	if jsonOutput {
		progressLog = io.Discard
		cleanupLog = io.Discard
	}
	result, err := worker.Run(cmd.Context(), rcfg, agent.ExecuteBeadLoopRuntime{
		Mode:                     spec.Mode,
		IdleInterval:             spec.IdleInterval.Duration,
		Log:                      progressLog,
		CleanupLog:               cleanupLog,
		EventSink:                loopSink,
		WorkerID:                 resolveClaimAssignee(),
		ProjectRoot:              projectRoot,
		CleanupRunner:            agent.NewExecutionCleanupManager(projectRoot, &agent.RealGitOps{}),
		ResourceChecker:          resourceChecker,
		SessionID:                loopSessionID,
		PreClaimHook:             buildCLIPreClaimHook(projectRoot, cliLandingOps),
		PreClaimIntakeHook:       intakeHook,
		PreDispatchLintHook:      lintHook,
		PostAttemptTriageHook:    triageHook,
		BudgetStop:               costCapTripped,
		NoReview:                 spec.NoReview,
		TargetBeadID:             tryTargetBeadID,
		ReviewCostCap:            costCap,
		OnAttemptFinalized:       buildAttemptMetricsHook(projectRoot, store, spec.Profile),
		PostLadderExhaustionHook: recoveryHook,
	})
	if err != nil && result != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		_ = writeExecuteLoopResult(cmd.OutOrStdout(), projectRoot, result, jsonOutput)
	}
	if err != nil {
		return err
	}
	return writeExecuteLoopResult(cmd.OutOrStdout(), projectRoot, result, jsonOutput)
}

// buildAttemptMetricsHook returns the OnAttemptFinalized hook that appends a
// row to .ddx/metrics/attempts.jsonl for each finalized attempt. Best-effort:
// store lookups and file I/O errors are silently ignored so a metrics failure
// never disrupts the drain loop.
func buildAttemptMetricsHook(projectRoot string, store *bead.Store, profile string) func(agent.ExecuteBeadReport) {
	return func(report agent.ExecuteBeadReport) {
		if report.AttemptID == "" {
			return
		}
		specID := ""
		if report.BeadID != "" && store != nil {
			if b, err := store.Get(report.BeadID); err == nil {
				specID, _ = b.Extra["spec-id"].(string)
			}
		}
		tsEnd := attemptmetrics.Rfc3339(time.Now().UTC())
		tsStart := ""
		if report.DurationMS > 0 {
			start := time.Now().UTC().Add(-time.Duration(report.DurationMS) * time.Millisecond)
			tsStart = attemptmetrics.Rfc3339(start)
		}
		row := attemptmetrics.AttemptRow{
			SchemaVersion: attemptmetrics.SchemaVersion,
			AttemptID:     report.AttemptID,
			BeadID:        report.BeadID,
			SessionID:     report.SessionID,
			TSStart:       tsStart,
			TSEnd:         tsEnd,
			Model:         report.Model,
			Harness:       report.Harness,
			Profile:       profile,
			Provider:      report.Provider,
			SpecID:        specID,
			Outcome:       report.Status,
			CostUSD:       report.CostUSD,
			DurationMS:    int(report.DurationMS),
			ReviewVerdict: report.ReviewVerdict,
		}
		_ = attemptmetrics.AppendRow(projectRoot, row)
	}
}

func formatAttemptRouteEconomics(attempt agent.ExecuteBeadReport) string {
	if attempt.Harness == "" && attempt.Provider == "" && attempt.Model == "" {
		return ""
	}
	parts := []string{}
	if attempt.Harness != "" {
		parts = append(parts, "harness="+attempt.Harness)
	}
	if attempt.Provider != "" {
		parts = append(parts, "provider="+attempt.Provider)
	}
	if attempt.Model != "" {
		parts = append(parts, "model="+attempt.Model)
	}
	power := attempt.PredictedPower
	if power == 0 {
		power = attempt.ActualPower
	}
	if power > 0 {
		parts = append(parts, fmt.Sprintf("power=%d", power))
	}
	if attempt.PredictedSpeedTPS > 0 {
		parts = append(parts, fmt.Sprintf("speed=%.1f tok/s", attempt.PredictedSpeedTPS))
	}
	if attempt.PredictedCostUSDPer1kTokens > 0 {
		cost := fmt.Sprintf("cost=$%.6f/1k tok", attempt.PredictedCostUSDPer1kTokens)
		if attempt.PredictedCostSource != "" {
			cost += " source=" + attempt.PredictedCostSource
		}
		parts = append(parts, cost)
	} else if attempt.PredictedCostSource != "" {
		parts = append(parts, "cost_source="+attempt.PredictedCostSource)
	}
	return strings.Join(parts, " ")
}

func writeExecuteLoopResult(w io.Writer, projectRoot string, result *agent.ExecuteBeadLoopResult, asJSON bool) error {
	if asJSON {
		payload := struct {
			ProjectRoot string `json:"project_root"`
			*agent.ExecuteBeadLoopResult
		}{
			ProjectRoot:           projectRoot,
			ExecuteBeadLoopResult: result,
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	if result.NoReadyWork && result.Attempts == 0 {
		writeNoReadyWorkSummary(w, projectRoot, result.NoReadyWorkDetail)
		writeQueueSnapshotTerminalSummary(w, result.QueueSnapshot)
		return nil
	}

	fmt.Fprintf(w, "\nproject: %s\n", projectRoot)
	writeWorkTerminalSummary(w, result)
	if result.Failures > 0 {
		fmt.Fprintf(w, "\nfailed:\n")
		for _, attempt := range result.Results {
			if attempt.Status != "success" {
				fmt.Fprintf(w, "  - %s: %s", attempt.BeadID, attempt.Detail)
				if attempt.PreserveRef != "" {
					fmt.Fprintf(w, " (preserved)")
				}
				fmt.Fprintln(w)
			}
		}
	}
	if result.NoReadyWork {
		fmt.Fprintln(w)
		writeNoReadyWorkSummary(w, "", result.NoReadyWorkDetail)
		writeQueueSnapshotTerminalSummary(w, result.QueueSnapshot)
	}
	return nil
}

func writeWorkTerminalSummary(w io.Writer, result *agent.ExecuteBeadLoopResult) {
	closed, changed, alreadySatisfied := countWorkTerminalOutcomes(result)
	fmt.Fprintf(w, "worker exited: %s\n", workExitSummary(result))
	fmt.Fprintf(w, "attempts: %d  |  closed: %d  |  changed: %d  |  already-satisfied: %d  |  failures: %d\n",
		result.Attempts, closed, changed, alreadySatisfied, result.Failures)
}

func countWorkTerminalOutcomes(result *agent.ExecuteBeadLoopResult) (closed, changed, alreadySatisfied int) {
	for _, attempt := range result.Results {
		switch attempt.Status {
		case agent.ExecuteBeadStatusSuccess:
			closed++
			changed++
		case agent.ExecuteBeadStatusAlreadySatisfied:
			closed++
			alreadySatisfied++
		}
	}
	if closed == 0 && result.Successes > 0 {
		closed = result.Successes
	}
	return closed, changed, alreadySatisfied
}

func workExitSummary(result *agent.ExecuteBeadLoopResult) string {
	switch result.ExitReason {
	case "drained":
		return "drained current execution-ready queue"
	case "once_complete":
		return "once complete"
	case "budget":
		return "budget reached"
	case "no_progress":
		return "no-progress policy stopped work"
	case "blocked":
		return "blocked waiting for external action"
	case "sigint", "sigterm", "context_cancelled":
		return "stopped by signal"
	case "fatal_config":
		return "fatal configuration error"
	case "preflight_failed":
		return "preflight failed"
	case "resource_exhausted":
		return "resource exhausted"
	case "routing_unavailable":
		return "routing unavailable"
	}
	if result.StopCondition != "" {
		return strings.ToLower(strings.ReplaceAll(result.StopCondition, "_", " "))
	}
	return "completed"
}

func writeQueueSnapshotTerminalSummary(w io.Writer, snapshot *agent.QueueSnapshot) {
	if snapshot == nil {
		return
	}
	retry := ""
	if snapshot.NextRetryAfter != "" {
		retry = " next-retry=" + snapshot.NextRetryAfter
	}
	fmt.Fprintf(w, "remaining queue: execution-ready=%d blocked=%d operator-attention=%d needs-human/investigation=%d cooldown/deferred=%d%s execution-ineligible=%d superseded=%d epics=%d epic-closure-candidates=%d\n",
		snapshot.ExecutionReadyCount,
		snapshot.BlockedCount,
		snapshot.ProposedOperatorAttentionCount,
		snapshot.HumanReviewBlockerCount,
		snapshot.RetryCooldownCount,
		retry,
		snapshot.ExecutionIneligibleCount,
		snapshot.SupersededCount,
		snapshot.SkippedEpicsCount,
		snapshot.EpicClosureCandidatesCount,
	)
	if snapshot.HumanReviewBlockedTotal <= 0 || snapshot.HumanReviewBlockerCount <= 0 {
		return
	}
	fmt.Fprintf(w, "%d beads blocked behind %d needs-human blockers:\n",
		snapshot.HumanReviewBlockedTotal, snapshot.HumanReviewBlockerCount)
	for i, blocker := range snapshot.HumanReviewBlockers {
		fmt.Fprintf(w, "  %d. %s %s (%d downstream)\n", i+1, blocker.ID, blocker.Title, blocker.DownstreamBlockedCount)
	}
}

func writeNoReadyWorkSummary(w io.Writer, projectRoot string, d agent.NoReadyWorkBreakdown) {
	if projectRoot != "" {
		fmt.Fprintf(w, "project: %s\n", projectRoot)
	}
	fmt.Fprintln(w, "No execution-ready beads.")
	if noReadyWorkBreakdownEmpty(d) {
		fmt.Fprintln(w, "  queue drained: no open work remains in lifecycle queues.")
	}
	if len(d.ProposedOperatorAttention) > 0 {
		fmt.Fprintf(w, "  operator attention: %d proposed bead(s) stop autonomous work and may block downstream dependents: %s\n",
			len(d.ProposedOperatorAttention), strings.Join(d.ProposedOperatorAttention, ", "))
	}
	if len(d.DependencyWaiting) > 0 {
		fmt.Fprintf(w, "  waiting on dependencies: %d open bead(s): %s\n",
			len(d.DependencyWaiting), strings.Join(d.DependencyWaiting, ", "))
	}
	if len(d.ExternalBlocked) > 0 {
		fmt.Fprintf(w, "  external blocked: %d bead(s) with explicit blocked status: %s\n",
			len(d.ExternalBlocked), strings.Join(d.ExternalBlocked, ", "))
	}
	if len(d.RetryCooldown) > 0 {
		retryHint := ""
		if d.NextRetryAfter != "" {
			retryHint = " (next retry-after: " + d.NextRetryAfter + ")"
		}
		fmt.Fprintf(w, "  retry cooldown: %d bead(s)%s: %s\n",
			len(d.RetryCooldown), retryHint, strings.Join(d.RetryCooldown, ", "))
	}
	if len(d.NotEligible) > 0 {
		fmt.Fprintf(w, "  not execution eligible: %d bead(s): %s\n",
			len(d.NotEligible), strings.Join(d.NotEligible, ", "))
	}
	if len(d.Superseded) > 0 {
		fmt.Fprintf(w, "  superseded: %d bead(s): %s\n",
			len(d.Superseded), strings.Join(d.Superseded, ", "))
	}
	if len(d.Epics) > 0 {
		fmt.Fprintf(w, "  epic containers: %d ready epic(s) with open children (decompose into tasks): %s\n",
			len(d.Epics), strings.Join(d.Epics, ", "))
	}
	if len(d.EpicClosureCandidates) > 0 {
		fmt.Fprintf(w, "  completed epic closure candidate(s) (all direct children closed; surfaced for closure evaluation): %s\n",
			strings.Join(d.EpicClosureCandidates, ", "))
	}
}

func noReadyWorkBreakdownEmpty(d agent.NoReadyWorkBreakdown) bool {
	return len(d.ExecutionReady) == 0 &&
		len(d.DependencyWaiting) == 0 &&
		len(d.ProposedOperatorAttention) == 0 &&
		len(d.RetryCooldown) == 0 &&
		len(d.ExternalBlocked) == 0 &&
		len(d.NotEligible) == 0 &&
		len(d.Superseded) == 0 &&
		len(d.Epics) == 0 &&
		len(d.EpicClosureCandidates) == 0 &&
		d.NextRetryAfter == ""
}

// resolveProjectRoot returns the project root to use for work, execute-bead,
// and agent-run commands. Resolution order:
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

type preClaimGitOps interface {
	CurrentBranch(dir string) (string, error)
	FetchOriginAncestryCheck(dir, targetBranch string) (agent.PreClaimResult, error)
}

// buildCLIPreClaimHook returns a PreClaimHook for inline queue work
// that fetches origin and verifies ancestry before each bead claim. Fetch
// failures are logged but do not block the worker (air-gap friendly).
func buildCLIPreClaimHook(projectRoot string, gitOps preClaimGitOps) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		branch, err := gitOps.CurrentBranch(projectRoot)
		if err != nil {
			return nil // can't determine branch — skip
		}
		res, err := gitOps.FetchOriginAncestryCheck(projectRoot, branch)
		if err != nil {
			if !agent.IsIgnorableFetchOriginError(err) {
				return err
			}
			return nil // fetch failure is non-fatal
		}
		if res.Action == "diverged" {
			return fmt.Errorf("local branch %s has diverged from origin (local=%s origin=%s); reconcile manually before claiming",
				branch, res.LocalSHA, res.OriginSHA)
		}
		return nil
	}
}

func buildCLIResourceChecker(projectRoot string, override agent.ExecutionResourceChecker) agent.ExecutionResourceChecker {
	if override != nil {
		return override
	}
	return agent.NewExecutionResourceChecker(projectRoot, &agent.RealGitOps{})
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
// .ddx/config.yaml that supplies routing inputs (agent.model,
// or agent.endpoints). When false, ddx work falls through to the global agent
// config defaults instead of demanding per-project flags. Tracks ddx-b790449b
// AC1.
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
	if strings.TrimSpace(a.Model) != "" {
		return true
	}
	if len(a.Endpoints) > 0 {
		return true
	}
	return false
}
