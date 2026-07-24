package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/spf13/cobra"
)

// newWorkerCommand returns the `ddx worker` subcommand tree. It is the
// operator-facing surface for server-managed worker supervision (Phase 1
// of ddx-9d1af129): write and inspect the durable desired-state file that
// the server-side WorkerSupervisor reconcile loop consumes.
//
// `set` and `enable` write `.ddx/workers/desired.json`. They are
// filesystem-only and do NOT require the server to be running — the next
// server tick picks up the file. `status` reads the same file.
func (f *CommandFactory) newWorkerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Manage server-managed worker desired state",
		Long: `Manage the desired-state file that the DDx server's supervisor uses
to keep queue-drain workers running for a project.

Write commands (set, enable) update .ddx/workers/desired.json. The server
picks up changes on its next reconcile tick (default 10s). status reports
what desired state is currently persisted.`,
	}
	cmd.AddCommand(f.newWorkerSetCommand())
	cmd.AddCommand(f.newWorkerStatusCommand())
	cmd.AddCommand(f.newWorkerEnableCommand())
	return cmd
}

func (f *CommandFactory) newWorkerSetCommand() *cobra.Command {
	var (
		projectFlag       string
		count             int
		harness           string
		provider          string
		model             string
		profile           string
		restartEnabled    bool
		noRestartEnabled  bool
		restartMaxPerHour int
	)
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Write desired worker state for a project",
		Long: `Persist .ddx/workers/desired.json for the given project. The server
supervisor consumes this file to keep the requested number of workers
running with the requested default execution spec.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := workerResolveProject(projectFlag, f.WorkingDir)
			sup, err := workerNewSupervisor(projectRoot)
			if err != nil {
				return err
			}
			state := server.DefaultWorkerDesiredState(projectRoot)
			state.DesiredCount = count
			state.DefaultSpec.Harness = harness
			state.DefaultSpec.Provider = provider
			state.DefaultSpec.Model = model
			if profile != "" {
				state.DefaultSpec.Profile = profile
			}
			state.Restart.Enabled = restartEnabled && !noRestartEnabled
			if restartMaxPerHour > 0 {
				state.Restart.MaxRestartsPerHour = restartMaxPerHour
			}
			if err := sup.SaveDesiredState(&state); err != nil {
				return fmt.Errorf("worker set: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (desired_count=%d, restart_enabled=%t)\n",
				workerDesiredStatePath(projectRoot), state.DesiredCount, state.Restart.Enabled)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectFlag, "project", "", "Project root (defaults to current working directory)")
	cmd.Flags().IntVar(&count, "count", 1, "Desired worker count")
	cmd.Flags().StringVar(&harness, "harness", "", "Preferred harness passthrough (empty = server default)")
	cmd.Flags().StringVar(&provider, "provider", "", "Preferred provider passthrough (empty = server default)")
	cmd.Flags().StringVar(&model, "model", "", "Preferred model passthrough (empty = server default)")
	cmd.Flags().StringVar(&profile, "profile", "", "Preferred profile (empty = keep default)")
	cmd.Flags().BoolVar(&restartEnabled, "restart-enabled", true, "Restart workers automatically on unexpected exit (on by default; disable with --no-restart-enabled)")
	cmd.Flags().BoolVar(&noRestartEnabled, "no-restart-enabled", false, "Disable automatic worker restart on unexpected exit (opt-out for --restart-enabled)")
	cmd.Flags().IntVar(&restartMaxPerHour, "restart-max-per-hour", 0, "Cap restarts per hour (0 = keep supervisor default)")
	return cmd
}

func (f *CommandFactory) newWorkerEnableCommand() *cobra.Command {
	var projectFlag string
	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Shortcut: set desired_count=1 with restart enabled",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := workerResolveProject(projectFlag, f.WorkingDir)
			sup, err := workerNewSupervisor(projectRoot)
			if err != nil {
				return err
			}
			state := server.DefaultWorkerDesiredState(projectRoot)
			state.DesiredCount = 1
			state.Restart.Enabled = true
			if err := sup.SaveDesiredState(&state); err != nil {
				return fmt.Errorf("worker enable: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "enabled server-managed workers for %s (desired_count=1, restart enabled)\n", projectRoot)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectFlag, "project", "", "Project root (defaults to current working directory)")
	return cmd
}

func (f *CommandFactory) newWorkerStatusCommand() *cobra.Command {
	var (
		projectFlag string
		asJSON      bool
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show desired worker state for a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := workerResolveProject(projectFlag, f.WorkingDir)
			sup, err := workerNewSupervisor(projectRoot)
			if err != nil {
				return err
			}
			state, err := sup.LoadDesiredState()
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(),
						"no desired state persisted for %s (run `ddx worker enable` to start server-managed workers)\n",
						projectRoot)
					return nil
				}
				return fmt.Errorf("worker status: %w", err)
			}
			presence, err := sup.DiagnoseDesiredWorkerPresence(state, time.Now().UTC())
			if err != nil {
				return fmt.Errorf("worker status: %w", err)
			}
			report := workerStatusReport{
				WorkerDesiredState:    state,
				LiveCount:             presence.LiveCount,
				MissingCount:          presence.MissingCount,
				FDExhaustionDiagnosis: presence.FDExhaustionDiagnosis,
				LastTerminalWorkerID:  presence.LastTerminalWorkerID,
			}
			if asJSON {
				out, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}
			writeWorkerStatusText(cmd.OutOrStdout(), report)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectFlag, "project", "", "Project root (defaults to current working directory)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit machine-readable JSON")
	return cmd
}

// workerStatusReport is the operator-facing payload for `ddx worker status`.
// It preserves every field of the durable desired-state file and adds live
// vs desired presence plus structured diagnosis when workers are missing.
type workerStatusReport struct {
	server.WorkerDesiredState
	LiveCount             int    `json:"live_count"`
	MissingCount          int    `json:"missing_count"`
	FDExhaustionDiagnosis string `json:"fd_exhaustion_diagnosis,omitempty"`
	LastTerminalWorkerID  string `json:"last_terminal_worker_id,omitempty"`
}

func writeWorkerStatusText(out io.Writer, report workerStatusReport) {
	state := report.WorkerDesiredState
	_, _ = fmt.Fprintf(out,
		"project: %s\ndesired_count: %d\nrestart_enabled: %t\ndefault_spec: harness=%q provider=%q model=%q profile=%q\nupdated_at: %s\n",
		state.ProjectRoot, state.DesiredCount, state.Restart.Enabled,
		state.DefaultSpec.Harness, state.DefaultSpec.Provider, state.DefaultSpec.Model, state.DefaultSpec.Profile,
		state.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
	_, _ = fmt.Fprintf(out, "live_count: %d\nmissing_count: %d\n", report.LiveCount, report.MissingCount)
	if report.MissingCount > 0 {
		_, _ = fmt.Fprintf(out, "missing: %d desired worker(s) not running\n", report.MissingCount)
	}
	if report.FDExhaustionDiagnosis != "" {
		_, _ = fmt.Fprintf(out, "fd_exhaustion_diagnosis: %s\n", report.FDExhaustionDiagnosis)
		_, _ = fmt.Fprintln(out, "diagnosis: missing desired worker exited from fd exhaustion (worker-local and restartable)")
		if report.LastTerminalWorkerID != "" {
			_, _ = fmt.Fprintf(out, "last_terminal_worker: %s\n", report.LastTerminalWorkerID)
		}
	}
}

// workerResolveProject returns an absolute project root. Explicit --project
// wins; otherwise fall back to CommandFactory.WorkingDir; otherwise CWD.
func workerResolveProject(flag, workingDir string) string {
	root := flag
	if root == "" {
		root = workingDir
	}
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return root
}

// workerNewSupervisor builds a WorkerSupervisor bound to projectRoot for
// filesystem-only CLI operations (save/load). Reconcile() would require a
// live WorkerManager wired to the server; CLI callers must not invoke it.
func workerNewSupervisor(projectRoot string) (*server.WorkerSupervisor, error) {
	if projectRoot == "" {
		return nil, fmt.Errorf("worker: project root is required")
	}
	m := server.NewWorkerManager(projectRoot)
	return server.NewWorkerSupervisor(m), nil
}

// workerDesiredStatePath returns the on-disk path where SaveDesiredState
// writes, used by user-facing messages. Delegates to ddxroot so convention-
// mode projects (state under XDG rather than in-tree .ddx/) report the
// correct absolute path.
func workerDesiredStatePath(projectRoot string) string {
	return ddxroot.JoinProject(projectRoot, "workers", "desired.json")
}
