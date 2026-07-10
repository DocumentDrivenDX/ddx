package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/spf13/cobra"
)

type doctorUnjamReport struct {
	ProjectRoot       string                `json:"project_root"`
	Clean             bool                  `json:"clean"`
	PrunableWorktrees []doctorUnjamWorktree `json:"prunable_worktrees"`
	RemovedWorktrees  []doctorUnjamWorktree `json:"removed_worktrees"`
	PrunedWorktrees   int                   `json:"pruned_worktrees"`
	Actions           []doctorUnjamAction   `json:"actions"`
	BeadDoctorRepair  *doctorUnjamRepair    `json:"bead_doctor_repair,omitempty"`
	ReleasedClaims    []string              `json:"released_claims,omitempty"`
	PreservedClaims   []string              `json:"preserved_claims,omitempty"`
}

type doctorUnjamRepair struct {
	Path               string   `json:"path"`
	Clean              bool     `json:"clean"`
	FindingsCount      int      `json:"findings_count"`
	FixedFindingsCount int      `json:"fixed_findings_count"`
	FixedBeadIDs       []string `json:"fixed_bead_ids,omitempty"`
	BackupPath         string   `json:"backup_path,omitempty"`
	RepairArtifacts    []string `json:"repair_artifacts,omitempty"`
}

type doctorUnjamWorktree struct {
	Path   string `json:"path"`
	Reason string `json:"reason,omitempty"`
}

type doctorUnjamAction struct {
	Kind  string `json:"kind"`
	Path  string `json:"path,omitempty"`
	Count int    `json:"count,omitempty"`
}

func (f *CommandFactory) runDoctorUnjam(cmd *cobra.Command) error {
	projectRoot := resolveProjectRoot("", f.WorkingDir)
	if projectRoot == "" {
		projectRoot = f.WorkingDir
	}
	if projectRoot == "" {
		return fmt.Errorf("doctor --unjam: unable to resolve project root")
	}

	report, err := unjamExecuteBeadWorktrees(cmd.Context(), projectRoot)
	if err != nil {
		return err
	}
	repairReport, err := unjamExecuteBeadRepair(cmd.Context(), projectRoot)
	if err != nil {
		return err
	}
	report.BeadDoctorRepair = repairReport
	if repairReport != nil && !repairReport.Clean {
		report.Actions = append(report.Actions, doctorUnjamAction{
			Kind:  "bead_doctor_fix",
			Path:  repairReport.Path,
			Count: repairReport.FixedFindingsCount,
		})
	}
	claimReport, err := unjamExecuteBeadClaims(cmd.Context(), projectRoot)
	if err != nil {
		return err
	}
	report.ReleasedClaims = claimReport.ReleasedClaims
	report.PreservedClaims = claimReport.PreservedClaims

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func unjamExecuteBeadClaims(ctx context.Context, projectRoot string) (server.WorkerClaimCleanupReport, error) {
	manager := server.NewWorkerManager(projectRoot)
	return manager.UnjamStaleClaims(ctx)
}

func unjamExecuteBeadRepair(ctx context.Context, projectRoot string) (*doctorUnjamRepair, error) {
	_ = ctx
	path := ddxroot.JoinProject(projectRoot, "beads.jsonl")
	report, err := bead.BeadDoctorFix(path, nil)
	if err != nil {
		return nil, err
	}
	return &doctorUnjamRepair{
		Path:               report.Path,
		Clean:              report.Clean(),
		FindingsCount:      len(report.Findings),
		FixedFindingsCount: report.FixedFindingsCount,
		FixedBeadIDs:       append([]string(nil), report.FixedBeadIDs...),
		BackupPath:         report.BackupPath,
		RepairArtifacts:    append([]string(nil), report.RepairArtifacts...),
	}, nil
}

func unjamExecuteBeadWorktrees(ctx context.Context, projectRoot string) (doctorUnjamReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	report := doctorUnjamReport{
		ProjectRoot:       projectRoot,
		Clean:             true,
		PrunableWorktrees: []doctorUnjamWorktree{},
		RemovedWorktrees:  []doctorUnjamWorktree{},
		Actions:           []doctorUnjamAction{},
	}

	staleWorktrees, err := listPrunableExecuteBeadWorktrees(ctx, projectRoot)
	if err != nil {
		return report, err
	}
	report.PrunableWorktrees = staleWorktrees
	if len(staleWorktrees) == 0 {
		return report, nil
	}

	for _, wt := range staleWorktrees {
		out, err := gitpkg.Command(ctx, projectRoot, "worktree", "remove", "--force", wt.Path).CombinedOutput()
		if err != nil {
			return report, fmt.Errorf("git worktree remove %s: %s: %w", wt.Path, strings.TrimSpace(string(out)), err)
		}
		report.RemovedWorktrees = append(report.RemovedWorktrees, wt)
		report.Actions = append(report.Actions, doctorUnjamAction{
			Kind: "worktree_remove",
			Path: wt.Path,
		})
	}

	if err := gitpkg.Command(ctx, projectRoot, "worktree", "prune").Run(); err != nil {
		return report, fmt.Errorf("git worktree prune: %w", err)
	}
	report.PrunedWorktrees = len(staleWorktrees)
	report.Actions = append(report.Actions, doctorUnjamAction{
		Kind:  "worktree_prune",
		Count: len(staleWorktrees),
	})
	return report, nil
}

func listPrunableExecuteBeadWorktrees(ctx context.Context, projectRoot string) ([]doctorUnjamWorktree, error) {
	out, err := gitpkg.Command(ctx, projectRoot, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	type worktreeEntry struct {
		path   string
		prune  bool
		reason string
	}

	var (
		entries []doctorUnjamWorktree
		current worktreeEntry
	)
	flush := func() {
		if current.path == "" || !current.prune {
			current = worktreeEntry{}
			return
		}
		if filepath.Base(current.path) != "" && strings.HasPrefix(filepath.Base(current.path), agent.ExecuteBeadWtPrefix) {
			entries = append(entries, doctorUnjamWorktree{
				Path:   current.path,
				Reason: current.reason,
			})
		}
		current = worktreeEntry{}
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			current.path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
			continue
		}
		if strings.HasPrefix(line, "prunable ") {
			current.prune = true
			current.reason = strings.TrimSpace(strings.TrimPrefix(line, "prunable "))
			continue
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan git worktree list output: %w", err)
	}

	return entries, nil
}
