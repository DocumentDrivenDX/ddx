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
	ProjectRoot        string                 `json:"project_root"`
	Clean              bool                   `json:"clean"`
	DDXStateCheckpoint *doctorUnjamCheckpoint `json:"ddx_state_checkpoint,omitempty"`
	PrunableWorktrees  []doctorUnjamWorktree  `json:"prunable_worktrees"`
	RemovedWorktrees   []doctorUnjamWorktree  `json:"removed_worktrees"`
	PrunedWorktrees    int                    `json:"pruned_worktrees"`
	Actions            []doctorUnjamAction    `json:"actions"`
	BeadDoctorRepair   *doctorUnjamRepair     `json:"bead_doctor_repair,omitempty"`
	ReleasedClaims     []string               `json:"released_claims,omitempty"`
	PreservedClaims    []string               `json:"preserved_claims,omitempty"`
}

// doctorUnjamCheckpoint records a checkpoint commit made to absorb dirty
// DDx-owned state (.ddx/executions/, .ddx/metrics/) found at the project root
// before stale worktree pruning runs.
type doctorUnjamCheckpoint struct {
	CommitSHA      string   `json:"commit_sha"`
	CommittedPaths []string `json:"committed_paths"`
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

	checkpoint, err := unjamCheckpointDDXOwnedState(cmd.Context(), projectRoot)
	if err != nil {
		return err
	}

	report, err := unjamExecuteBeadWorktrees(cmd.Context(), projectRoot)
	if err != nil {
		return err
	}
	if checkpoint != nil {
		report.DDXStateCheckpoint = checkpoint
		report.Actions = append([]doctorUnjamAction{{
			Kind:  "ddx_state_checkpoint",
			Count: len(checkpoint.CommittedPaths),
		}}, report.Actions...)
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

// ddxStateCheckpointCommitMessage is the stable commit message used for
// doctor --unjam checkpoints of dirty DDx-owned state. Idempotency comes from
// there being nothing left to stage on a rerun, not from message uniqueness.
const ddxStateCheckpointCommitMessage = "chore: checkpoint ddx-owned state (doctor --unjam)"

// ddxStateCheckpointPathspecs are the DDx-owned project-root paths doctor
// --unjam checkpoints before pruning stale execute-bead worktrees.
var ddxStateCheckpointPathspecs = []string{
	".ddx/executions",
	".ddx/metrics",
}

// unjamCheckpointDDXOwnedState commits any dirty (staged or unstaged, including
// otherwise-gitignored) files under .ddx/executions/ and .ddx/metrics/ at the
// project root into a single checkpoint commit, so that dirt in those
// DDx-owned paths cannot block the rest of the unjam pass. Returns nil when
// there was nothing to checkpoint.
func unjamCheckpointDDXOwnedState(ctx context.Context, projectRoot string) (*doctorUnjamCheckpoint, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := gitpkg.Command(ctx, projectRoot, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return nil, nil
	}

	statusArgs := append([]string{"status", "--porcelain", "--untracked-files=all", "--ignored=matching", "--"}, ddxStateCheckpointPathspecs...)
	statusOut, err := gitpkg.Command(ctx, projectRoot, statusArgs...).Output()
	if err != nil {
		return nil, fmt.Errorf("checking ddx-owned state status: %w", err)
	}
	dirtyPaths := parseDDXStateCheckpointDirtyPaths(string(statusOut))
	if len(dirtyPaths) == 0 {
		return nil, nil
	}

	addArgs := append([]string{"add", "-f", "-A", "--"}, dirtyPaths...)
	if out, err := gitpkg.Command(ctx, projectRoot, addArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("staging ddx-owned state: %s: %w", strings.TrimSpace(string(out)), err)
	}

	cachedArgs := append([]string{"diff", "--cached", "--name-only", "--"}, ddxStateCheckpointPathspecs...)
	cachedOut, err := gitpkg.Command(ctx, projectRoot, cachedArgs...).Output()
	if err != nil {
		return nil, fmt.Errorf("checking staged ddx-owned state: %w", err)
	}
	committedPaths := splitNonEmptyLines(string(cachedOut))
	if len(committedPaths) == 0 {
		return nil, nil
	}

	commitArgs := append([]string{"commit", "--no-verify", "--only", "-m", ddxStateCheckpointCommitMessage, "--"}, committedPaths...)
	if out, err := gitpkg.Command(ctx, projectRoot, commitArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("checkpointing ddx-owned state: %s: %w", strings.TrimSpace(string(out)), err)
	}

	shaOut, err := gitpkg.Command(ctx, projectRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return nil, fmt.Errorf("resolving ddx-owned state checkpoint sha: %w", err)
	}

	return &doctorUnjamCheckpoint{
		CommitSHA:      strings.TrimSpace(string(shaOut)),
		CommittedPaths: committedPaths,
	}, nil
}

func parseDDXStateCheckpointDirtyPaths(output string) []string {
	var paths []string
	seen := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = strings.TrimSpace(path[idx+4:])
		}
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

func splitNonEmptyLines(output string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
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
