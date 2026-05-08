package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
)

func (f *CommandFactory) newServerWorkersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workers",
		Short: "Manage agent execution workers",
	}

	cmd.AddCommand(f.newServerWorkersListCommand())
	cmd.AddCommand(f.newServerWorkersShowCommand())
	cmd.AddCommand(f.newServerWorkersLogCommand())

	return cmd
}

func (f *CommandFactory) newServerWorkersListCommand() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agent execution workers",
		RunE: func(cmd *cobra.Command, args []string) error {
			base := resolveServerURL(f.WorkingDir)

			resp, err := newLocalServerClient().Get(base + "/api/agent/workers")
			if err != nil {
				return fmt.Errorf("server request: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("reading response: %w", err)
			}

			if asJSON {
				fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return nil
			}

			var workers []workerRecord
			if err := json.Unmarshal(body, &workers); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			if len(workers) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No workers.")
				return nil
			}

			for _, w := range workers {
				state := w.State
				if state == "exited" {
					if w.LastResult.Status != "" {
						state = w.LastResult.Status
					}
				}
				model := w.Model
				if model == "" && w.LastResult.Model != "" {
					model = w.LastResult.Model
				}
				bead := w.LastResult.BeadID
				if bead == "" {
					bead = "-"
				}
				age := formatDuration(time.Since(w.StartedAt))
				fmt.Fprintf(cmd.OutOrStdout(), "%-36s %-6s %-18s %-24s %s\n",
					w.ID, age, state, model, bead)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return cmd
}

func (f *CommandFactory) newServerWorkersShowCommand() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "show <worker-id>",
		Short: "Show worker details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := resolveServerURL(f.WorkingDir)

			resp, err := newLocalServerClient().Get(base + "/api/agent/workers/" + args[0])
			if err != nil {
				return fmt.Errorf("server request: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("reading response: %w", err)
			}

			if asJSON {
				fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return nil
			}

			var w workerRecord
			if err := json.Unmarshal(body, &w); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			state := w.State
			if state == "exited" && w.LastResult.Status != "" {
				state = w.LastResult.Status
			}
			model := w.Model
			if model == "" && w.LastResult.Model != "" {
				model = w.LastResult.Model
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Worker:    %s\n", w.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "State:     %s\n", state)
			fmt.Fprintf(cmd.OutOrStdout(), "Model:     %s\n", model)
			fmt.Fprintf(cmd.OutOrStdout(), "Harness:   %s\n", w.Harness)
			fmt.Fprintf(cmd.OutOrStdout(), "Started:   %s (%s ago)\n", w.StartedAt.Format(time.RFC3339), formatDuration(time.Since(w.StartedAt)))
			if !w.ExitedAt.IsZero() {
				fmt.Fprintf(cmd.OutOrStdout(), "Exited:    %s (ran %s)\n", w.ExitedAt.Format(time.RFC3339), formatDuration(w.ExitedAt.Sub(w.StartedAt)))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Project:   %s\n", w.ProjectRoot)
			if w.Once {
				fmt.Fprintf(cmd.OutOrStdout(), "Mode:      once\n")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Attempts:  %d  Success: %d  Failed: %d\n", w.Attempts, w.Successes, w.Failures)
			if w.LastResult.BeadID != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Last bead: %s\n", w.LastResult.BeadID)
				fmt.Fprintf(cmd.OutOrStdout(), "Last status: %s\n", w.LastResult.Status)
				if w.LastResult.Detail != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Last detail: %s\n", w.LastResult.Detail)
				}
				if w.LastResult.BaseRev != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Base rev:  %s\n", w.LastResult.BaseRev[:12])
				}
				if w.LastResult.ResultRev != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Result rev: %s\n", w.LastResult.ResultRev[:12])
				}
			}
			if w.LastError != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Error:     %s\n", w.LastError)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return cmd
}

func (f *CommandFactory) newServerWorkersLogCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log <worker-id>",
		Short: "View worker progress and session log",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := resolveServerURL(f.WorkingDir)

			resp, err := newLocalServerClient().Get(base + "/api/agent/workers/" + args[0] + "/log")
			if err != nil {
				return fmt.Errorf("server request: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("reading response: %w", err)
			}

			var logResp struct {
				Stdout string `json:"stdout"`
				Stderr string `json:"stderr"`
			}
			if err := json.Unmarshal(body, &logResp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			if logResp.Stdout != "" {
				fmt.Fprint(cmd.OutOrStdout(), logResp.Stdout)
			}
			if logResp.Stderr != "" {
				fmt.Fprint(os.Stderr, logResp.Stderr)
			}
			return nil
		},
	}

	return cmd
}

type workerResult struct {
	BeadID      string `json:"bead_id"`
	AttemptID   string `json:"attempt_id"`
	WorkerID    string `json:"worker_id"`
	Harness     string `json:"harness"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Status      string `json:"status"`
	Detail      string `json:"detail"`
	SessionID   string `json:"session_id"`
	BaseRev     string `json:"base_rev"`
	ResultRev   string `json:"result_rev"`
	PreserveRef string `json:"preserve_ref"`
}

type workerRecord struct {
	ID          string       `json:"id"`
	State       string       `json:"state"`
	Status      string       `json:"status"`
	ProjectRoot string       `json:"project_root"`
	Harness     string       `json:"harness"`
	Model       string       `json:"model"`
	Effort      string       `json:"effort"`
	Once        bool         `json:"once"`
	StartedAt   time.Time    `json:"started_at"`
	ExitedAt    time.Time    `json:"exited_at"`
	Attempts    int          `json:"attempts"`
	Successes   int          `json:"successes"`
	Failures    int          `json:"failures"`
	LastError   string       `json:"last_error"`
	LastResult  workerResult `json:"last_result"`
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, mins)
}
