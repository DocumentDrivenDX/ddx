package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/spf13/cobra"
)

// newWorkStatusCommand creates the "ddx work status" subcommand: a
// project-scoped live-worker report.
//
// The default report lists only live `ddx work` / `ddx try` processes whose
// resolved project root matches the requested project. The earlier failure
// mode this command exists to prevent is a global `ps | grep ddx work` scan
// surfacing a worker from another repository while operators are asking
// about a specific project's queue.
func (f *CommandFactory) newWorkStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show live ddx workers scoped to this project",
		Long: `status lists the live ddx worker processes (ddx work / ddx try)
whose project root matches the current project. The default scope is the
git root of the working directory; pass --project to override or
--all-projects to inspect every worker on the host.

Workers from other repositories are intentionally excluded from the
default view. Reporting them as evidence that "the worker is still
running" hides whether the requested project's queue has stalled.

Output columns (terminal):
  PID         — process id of the worker
  AGE         — wall-clock age since the worker process started
  COMMAND     — the worker's command line (truncated for terminals)
  BEAD        — active bead id when it can be inferred from argv or the
                isolated execute-bead worktree
  ATTEMPT     — active attempt id when a fresh worker liveness sidecar
                records one for the live worker
  WORKTREE    — execute-bead worktree path when the worker is inside one

JSON output preserves the full set of fields including project_root,
attempt_id, phase, child_pid, and last_activity_at when available.`,
		Example: `  # Show live workers for the current project
  ddx work status

  # JSON for piping to jq
  ddx work status --json

  # Explicit cross-project view
  ddx work status --all-projects --json`,
		Args: cobra.NoArgs,
		RunE: f.runWorkStatus,
	}
	cmd.Flags().String("project", "", "Target project root path (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().Bool("all-projects", false, "Report workers from every project; opt-in escape hatch")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

// workerStatusScanner is set by tests via the CommandFactory to inject a
// deterministic worker set without spawning real processes.
type workerStatusScanner = workerstatus.Scanner

// WorkStatusReport is the JSON payload emitted by `ddx work status --json`.
type WorkStatusReport struct {
	ProjectRoot string                    `json:"project_root,omitempty"`
	Scope       string                    `json:"scope"`
	Workers     []workerstatus.LiveWorker `json:"workers"`
}

func (f *CommandFactory) runWorkStatus(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	allProjects, _ := cmd.Flags().GetBool("all-projects")
	asJSON, _ := cmd.Flags().GetBool("json")

	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)

	scanner := f.workerScanner()
	all, err := scanner.Scan(cmd.Context())
	if err != nil {
		return fmt.Errorf("scan live workers: %w", err)
	}

	now := time.Now().UTC()
	report := WorkStatusReport{
		ProjectRoot: projectRoot,
		Scope:       "project",
		Workers: enrichWorkersWithRunState(
			workerstatus.EnrichWithFreshLiveness(filterAndSortWorkers(all, projectRoot, allProjects), now),
			now,
		),
	}
	if allProjects {
		report.Scope = "all-projects"
	}

	out := cmd.OutOrStdout()
	if asJSON {
		return writeWorkStatusJSON(out, report)
	}
	writeWorkStatusText(out, report)
	return nil
}

func (f *CommandFactory) workerScanner() workerStatusScanner {
	if f.workerScannerOverride != nil {
		return f.workerScannerOverride
	}
	return workerstatus.New()
}

// enrichWorkersWithRunState fills bead/attempt/worktree from a fresh
// per-attempt run-state record (.ddx/run-state/<attempt>.json) whose PID
// matches a live worker, when liveness enrichment did not supply them — e.g.
// the liveness sidecar was absent, start-time-skewed, or its attempt id was
// stale (ddx-f9b41107). Run-state is read from each worker's own project root,
// so all-projects scope does not leak attempts across projects.
func enrichWorkersWithRunState(workers []workerstatus.LiveWorker, now time.Time) []workerstatus.LiveWorker {
	byProject := make(map[string][]agent.RunState)
	for i := range workers {
		w := &workers[i]
		if w.PID <= 0 || (w.BeadID != "" && w.AttemptID != "" && w.ExecutionWorktree != "") {
			continue
		}
		states, ok := byProject[w.ProjectRoot]
		if !ok {
			states, _ = agent.ReadRunStates(w.ProjectRoot)
			byProject[w.ProjectRoot] = states
		}
		rec, ok := freshestRunStateForPID(states, w.PID, now)
		if !ok {
			continue
		}
		if w.BeadID == "" {
			w.BeadID = rec.BeadID
		}
		if w.AttemptID == "" {
			w.AttemptID = rec.AttemptID
		}
		if w.ExecutionWorktree == "" {
			w.ExecutionWorktree = rec.WorktreePath
		}
	}
	return workers
}

// freshestRunStateForPID returns the most recently refreshed non-expired
// run-state record for pid, if any.
func freshestRunStateForPID(states []agent.RunState, pid int, now time.Time) (agent.RunState, bool) {
	var best agent.RunState
	found := false
	for _, s := range states {
		if s.PID != pid {
			continue
		}
		if !s.ExpiresAt.IsZero() && now.After(s.ExpiresAt) {
			continue
		}
		if !found || s.RefreshedAt.After(best.RefreshedAt) {
			best = s
			found = true
		}
	}
	return best, found
}

func filterAndSortWorkers(all []workerstatus.LiveWorker, projectRoot string, allProjects bool) []workerstatus.LiveWorker {
	var filtered []workerstatus.LiveWorker
	if allProjects {
		filtered = append(filtered, all...)
	} else {
		filtered = workerstatus.FilterByProject(all, projectRoot)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].StartedAt.Equal(filtered[j].StartedAt) {
			return filtered[i].PID < filtered[j].PID
		}
		return filtered[i].StartedAt.Before(filtered[j].StartedAt)
	})
	return filtered
}

func writeWorkStatusJSON(out io.Writer, report WorkStatusReport) error {
	if report.Workers == nil {
		report.Workers = []workerstatus.LiveWorker{}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func writeWorkStatusText(out io.Writer, report WorkStatusReport) {
	if len(report.Workers) == 0 {
		if report.Scope == "all-projects" {
			fmt.Fprintln(out, "No live ddx workers found on this host.")
			return
		}
		fmt.Fprintf(out, "No live ddx workers for project %s.\n", report.ProjectRoot)
		return
	}
	if report.Scope == "all-projects" {
		fmt.Fprintf(out, "live ddx workers (all projects): %d\n", len(report.Workers))
	} else {
		fmt.Fprintf(out, "live ddx workers for %s: %d\n", report.ProjectRoot, len(report.Workers))
	}
	for _, w := range report.Workers {
		bead := w.BeadID
		if bead == "" {
			bead = "-"
		}
		attempt := w.AttemptID
		if attempt == "" {
			attempt = "-"
		}
		worktree := w.ExecutionWorktree
		if worktree == "" {
			worktree = "-"
		}
		fmt.Fprintf(out, "  pid=%d age=%s project=%s bead=%s attempt=%s worktree=%s\n    %s\n",
			w.PID, w.Age, w.ProjectRoot, bead, attempt, worktree, w.Command)
	}
}
