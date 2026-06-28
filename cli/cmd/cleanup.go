package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
)

type cleanupCommandReport struct {
	DryRun                      bool                                   `json:"dry_run"`
	ProjectRoot                 string                                 `json:"project_root"`
	TempRoot                    string                                 `json:"temp_root"`
	ScannedTempDirs             int                                    `json:"scanned_temp_dirs"`
	ScannedEvidenceDirs         int                                    `json:"scanned_evidence_dirs"`
	CompleteEvidenceDirs        int                                    `json:"complete_evidence_dirs"`
	ScannedScratchDirs          int                                    `json:"scanned_scratch_dirs"`
	RemovedUnregisteredTempDirs int64                                  `json:"removed_unregistered_temp_dirs"`
	RemovedRegisteredWorktrees  int64                                  `json:"removed_registered_worktrees"`
	RemovedRunStateFiles        int64                                  `json:"removed_run_state_files"`
	RemovedScratchDirs          int64                                  `json:"removed_scratch_dirs"`
	PreservedActiveScratchDirs  int64                                  `json:"preserved_active_scratch_dirs"`
	BytesReclaimed              int64                                  `json:"bytes_reclaimed"`
	InodesReclaimed             int64                                  `json:"inodes_reclaimed"`
	ScratchBytesReclaimed       int64                                  `json:"scratch_bytes_reclaimed"`
	ScratchInodesReclaimed      int64                                  `json:"scratch_inodes_reclaimed"`
	Warnings                    []agent.ExecutionCleanupWarning        `json:"warnings"`
	BlockedErrors               []agent.ExecutionCleanupIssue          `json:"blocked_errors"`
	Observations                []agent.ExecutionCleanupObservation    `json:"observations"`
	ProcessFindings             []agent.ExecutionCleanupProcessFinding `json:"process_findings"`
}

func (f *CommandFactory) newCleanupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Inspect and clean stale DDx execution resources",
		Long: `Inspect DDx-owned execution resources and conservatively clean stale state.

By default, cleanup runs in dry-run mode and reports what it would remove.
Use --apply to remove stale DDx temp worktrees, stale run-state files, and
other DDx-owned scratch resources under the configured project roots.

Examples:
  ddx cleanup
  ddx cleanup --json
  ddx cleanup --apply
  ddx cleanup --project /path/to/project --apply`,
		Args: cobra.NoArgs,
		RunE: f.runCleanup,
	}

	cmd.Flags().Bool("apply", false, "Actually remove stale DDx execution resources")
	cmd.Flags().Bool("json", false, "Output the cleanup report as JSON")
	cmd.Flags().String("project", "", "Project root to inspect (default: detected from CWD)")
	return cmd
}

func (f *CommandFactory) runCleanup(cmd *cobra.Command, args []string) error {
	_ = args
	projectFlag, _ := cmd.Flags().GetString("project")
	apply, _ := cmd.Flags().GetBool("apply")
	asJSON, _ := cmd.Flags().GetBool("json")

	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)
	if projectRoot == "" {
		projectRoot = f.WorkingDir
	}

	mgr := agent.NewExecutionCleanupManager(projectRoot, &agent.RealGitOps{})
	mgr.DryRun = !apply

	summary, err := mgr.Cleanup(cmd.Context())
	if err != nil {
		return err
	}

	report := cleanupCommandReport{
		DryRun:                      !apply,
		ProjectRoot:                 summary.ProjectRoot,
		TempRoot:                    summary.TempRoot,
		ScannedTempDirs:             summary.ScannedTempDirs,
		ScannedEvidenceDirs:         summary.ScannedEvidenceDirs,
		CompleteEvidenceDirs:        summary.CompleteEvidenceDirs,
		ScannedScratchDirs:          summary.ScannedScratchDirs,
		RemovedUnregisteredTempDirs: summary.RemovedUnregisteredTempDirs,
		RemovedRegisteredWorktrees:  summary.RemovedRegisteredWorktrees,
		RemovedRunStateFiles:        summary.RemovedRunStateFiles,
		RemovedScratchDirs:          summary.RemovedScratchDirs,
		PreservedActiveScratchDirs:  summary.PreservedActiveScratchDirs,
		BytesReclaimed:              summary.BytesReclaimed,
		InodesReclaimed:             summary.InodesReclaimed,
		ScratchBytesReclaimed:       summary.ScratchBytesReclaimed,
		ScratchInodesReclaimed:      summary.ScratchInodesReclaimed,
		Warnings:                    append([]agent.ExecutionCleanupWarning(nil), summary.Warnings...),
		Observations:                append([]agent.ExecutionCleanupObservation(nil), summary.Observations...),
		ProcessFindings:             append([]agent.ExecutionCleanupProcessFinding(nil), summary.ProcessFindings...),
	}
	for _, issue := range summary.Issues {
		if issue.Blocking {
			report.BlockedErrors = append(report.BlockedErrors, issue)
		}
	}
	if report.Warnings == nil {
		report.Warnings = []agent.ExecutionCleanupWarning{}
	}
	if report.BlockedErrors == nil {
		report.BlockedErrors = []agent.ExecutionCleanupIssue{}
	}
	if report.Observations == nil {
		report.Observations = []agent.ExecutionCleanupObservation{}
	}
	if report.ProcessFindings == nil {
		report.ProcessFindings = []agent.ExecutionCleanupProcessFinding{}
	}

	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	out := cmd.OutOrStdout()
	verb := "removed"
	if report.DryRun {
		verb = "would remove"
	}
	fmt.Fprintf(out, "cleanup: %s %d stale temp dir(s), %d registered worktree(s), %d run-state file(s), %d scratch dir(s)\n",
		verb,
		report.RemovedUnregisteredTempDirs,
		report.RemovedRegisteredWorktrees,
		report.RemovedRunStateFiles,
		report.RemovedScratchDirs,
	)
	if report.BytesReclaimed > 0 || report.InodesReclaimed > 0 {
		fmt.Fprintf(out, "cleanup: %s %d byte(s), %d inode(s)\n", verb, report.BytesReclaimed, report.InodesReclaimed)
	}
	if report.ScratchBytesReclaimed > 0 || report.ScratchInodesReclaimed > 0 || report.PreservedActiveScratchDirs > 0 {
		fmt.Fprintf(out, "cleanup: scratch scope %s %d byte(s), %d inode(s); preserved %d active scratch dir(s)\n",
			verb,
			report.ScratchBytesReclaimed,
			report.ScratchInodesReclaimed,
			report.PreservedActiveScratchDirs,
		)
	}
	if report.CompleteEvidenceDirs > 0 {
		fmt.Fprintf(out, "cleanup: preserved %d complete evidence bundle(s)\n", report.CompleteEvidenceDirs)
	}
	actionable := 0
	killed := 0
	for _, f := range report.ProcessFindings {
		if f.WouldKill {
			actionable++
		}
		if f.Terminated {
			killed++
		}
	}
	if actionable > 0 {
		if report.DryRun {
			fmt.Fprintf(out, "cleanup: found %d stale process group(s) (would kill: %d)\n", actionable, actionable)
		} else {
			fmt.Fprintf(out, "cleanup: found %d stale process group(s) (killed: %d)\n", actionable, killed)
		}
		for _, f := range report.ProcessFindings {
			if f.PID > 0 {
				fmt.Fprintf(out, "  stale process: pid=%d pgid=%d command=%q worktree=%s reason=%s\n",
					f.PID, f.PGID, f.Command, f.Worktree, f.StaleReason)
			}
		}
	}
	for _, warning := range report.Warnings {
		fmt.Fprintf(out, "warning: %s (%s): %s\n", warning.Path, warning.Class, warning.Message)
	}
	for _, issue := range report.BlockedErrors {
		fmt.Fprintf(out, "blocked: %s (%s): %s\n", issue.Path, issue.Class, issue.Message)
	}
	if report.DryRun {
		fmt.Fprintln(out, "run again with --apply to remove these resources")
	}
	return nil
}
