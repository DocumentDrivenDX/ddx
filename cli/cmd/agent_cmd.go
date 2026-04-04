package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/easel/ddx/internal/agent"
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
  ddx agent doctor
  ddx agent log`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newAgentRunCommand())
	cmd.AddCommand(f.newAgentListCommand())
	cmd.AddCommand(f.newAgentDoctorCommand())
	cmd.AddCommand(f.newAgentLogCommand())

	return cmd
}

func (f *CommandFactory) agentRunner() *agent.Runner {
	// TODO: load agent config from .ddx/config.yaml
	return agent.NewRunner(agent.Config{})
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
			quorum, _ := cmd.Flags().GetString("quorum")
			harnesses, _ := cmd.Flags().GetString("harnesses")
			asJSON, _ := cmd.Flags().GetBool("json")

			// Read prompt from stdin if neither file nor text provided
			prompt := promptText
			if prompt == "" && promptFile == "" {
				// Check if stdin has data
				stat, _ := os.Stdin.Stat()
				if (stat.Mode() & os.ModeCharDevice) == 0 {
					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						return fmt.Errorf("reading stdin: %w", err)
					}
					prompt = string(data)
				}
			}

			// Quorum mode
			if quorum != "" || harnesses != "" {
				harnessNames := strings.Split(harnesses, ",")
				if len(harnessNames) == 0 || (len(harnessNames) == 1 && harnessNames[0] == "") {
					return fmt.Errorf("--harnesses required for quorum mode")
				}
				opts := agent.QuorumOptions{
					RunOptions: agent.RunOptions{
						Prompt:     prompt,
						PromptFile: promptFile,
						Model:      model,
						WorkDir:    f.WorkingDir,
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
					if result.Output != "" {
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
				Harness:    harness,
				Prompt:     prompt,
				PromptFile: promptFile,
				Model:      model,
				WorkDir:    f.WorkingDir,
			}
			result, err := r.Run(opts)
			if err != nil {
				return err
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			// Print output
			if result.Output != "" {
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
	cmd.Flags().String("model", "", "Model override")
	cmd.Flags().String("quorum", "", "Quorum strategy: any, majority, unanimous")
	cmd.Flags().String("harnesses", "", "Comma-separated harnesses for quorum")
	cmd.Flags().Bool("json", false, "Output as JSON")

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
			logDir := agent.DefaultLogDir
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
