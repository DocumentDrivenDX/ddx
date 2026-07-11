package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/spf13/cobra"
)

type doctorUnjamReport struct {
	ProjectRoot          string                 `json:"project_root"`
	Clean                bool                   `json:"clean"`
	DDXStateCheckpoint   *doctorUnjamCheckpoint `json:"ddx_state_checkpoint,omitempty"`
	PrunableWorktrees    []doctorUnjamWorktree  `json:"prunable_worktrees"`
	RemovedWorktrees     []doctorUnjamWorktree  `json:"removed_worktrees"`
	ReportOnlyDirtyPaths []doctorUnjamDirtyPath `json:"report_only_dirty_paths,omitempty"`
	PrunedWorktrees      int                    `json:"pruned_worktrees"`
	PruneInvocations     int                    `json:"prune_invocations"`
	Actions              []doctorUnjamAction    `json:"actions"`
	BeadDoctorRepair     *doctorUnjamRepair     `json:"bead_doctor_repair,omitempty"`
	ReleasedClaims       []string               `json:"released_claims,omitempty"`
	PreservedClaims      []string               `json:"preserved_claims,omitempty"`
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

// doctorUnjamDirtyPath records a dirty path that doctor --unjam reports but
// intentionally leaves untouched.
type doctorUnjamDirtyPath struct {
	Path      string `json:"path"`
	Untouched bool   `json:"untouched"`
}

type doctorUnjamAction struct {
	Kind        string `json:"kind"`
	Path        string `json:"path,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Count       int    `json:"count,omitempty"`
	PreserveRef string `json:"preserve_ref,omitempty"`
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

	preserveActions, err := unjamPreserveRefDerivedDirt(cmd.Context(), projectRoot)
	if err != nil {
		return err
	}

	report, err := unjamExecuteBeadWorktrees(cmd.Context(), projectRoot)
	if err != nil {
		return err
	}
	if len(preserveActions) > 0 {
		report.Actions = append(preserveActions, report.Actions...)
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

	reportOnlyDirtyPaths, err := unjamReportOnlyDirtyPaths(cmd.Context(), projectRoot)
	if err != nil {
		return err
	}
	report.ReportOnlyDirtyPaths = reportOnlyDirtyPaths
	if len(reportOnlyDirtyPaths) > 0 {
		report.Clean = false
	}

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

const (
	doctorUnjamPreserveRefStashActionKind    = "preserve_ref_stash"
	doctorUnjamPreserveRefStashMessagePrefix = "ddx doctor --unjam preserve "
)

// unjamPreserveRefDerivedDirt stashes dirty project-root paths that overlap a
// preserved iteration ref so a leaked checkout fragment can be recovered after
// the main unjam pass completes.
func unjamPreserveRefDerivedDirt(ctx context.Context, projectRoot string) ([]doctorUnjamAction, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := gitpkg.Command(ctx, projectRoot, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return nil, nil
	}

	dirtyPaths, err := unjamProjectDirtyPaths(ctx, projectRoot)
	if err != nil {
		return nil, err
	}
	if len(dirtyPaths) == 0 {
		return nil, nil
	}

	preserveRefs, err := listPreserveIterationRefs(ctx, projectRoot)
	if err != nil {
		return nil, err
	}
	if len(preserveRefs) == 0 {
		return nil, nil
	}

	remaining := append([]string(nil), dirtyPaths...)
	actions := make([]doctorUnjamAction, 0, len(preserveRefs))
	for _, preserveRef := range preserveRefs {
		if len(remaining) == 0 {
			break
		}

		treePaths, err := listPreserveRefTreePaths(ctx, projectRoot, preserveRef)
		if err != nil {
			return nil, err
		}

		matchedPaths := matchPreserveRefDirtyPaths(remaining, treePaths)
		if len(matchedPaths) == 0 {
			continue
		}

		if err := stashPreserveRefDirtyPaths(ctx, projectRoot, preserveRef, matchedPaths); err != nil {
			return nil, err
		}

		actions = append(actions, doctorUnjamAction{
			Kind:        doctorUnjamPreserveRefStashActionKind,
			Path:        matchedPaths[0],
			Count:       len(matchedPaths),
			PreserveRef: preserveRef,
		})
		remaining = removeDirtyPaths(remaining, matchedPaths)
	}

	return actions, nil
}

func unjamProjectDirtyPaths(ctx context.Context, projectRoot string) ([]string, error) {
	out, err := gitpkg.Command(ctx, projectRoot, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=matching", "--", ".").Output()
	if err != nil {
		return nil, fmt.Errorf("listing project dirt: %w", err)
	}
	return parseDDXStateCheckpointDirtyPaths(string(out)), nil
}

func unjamReportOnlyDirtyPaths(ctx context.Context, projectRoot string) ([]doctorUnjamDirtyPath, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := gitpkg.Command(ctx, projectRoot, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return nil, nil
	}

	dirtyPaths, err := unjamProjectDirtyPaths(ctx, projectRoot)
	if err != nil {
		return nil, err
	}
	if len(dirtyPaths) == 0 {
		return nil, nil
	}

	reportOnly := make([]doctorUnjamDirtyPath, 0, len(dirtyPaths))
	for _, dirtyPath := range dirtyPaths {
		normalized := normalizeDoctorUnjamPath(dirtyPath)
		if isDoctorUnjamDDXOwnedPath(normalized) {
			continue
		}
		reportOnly = append(reportOnly, doctorUnjamDirtyPath{
			Path:      normalized,
			Untouched: true,
		})
	}
	return reportOnly, nil
}

func isDoctorUnjamDDXOwnedPath(path string) bool {
	path = normalizeDoctorUnjamPath(path)
	if path == "" {
		return false
	}
	if path == ".ddx.yml" || path == ".ddx.yaml" {
		return true
	}
	return path == ".ddx" || strings.HasPrefix(path, ".ddx/")
}

func listPreserveIterationRefs(ctx context.Context, projectRoot string) ([]string, error) {
	out, err := gitpkg.Command(ctx, projectRoot, "for-each-ref", "--sort=-refname", "--format=%(refname)", "refs/ddx/iterations").Output()
	if err != nil {
		return nil, fmt.Errorf("listing preserve iteration refs: %w", err)
	}
	refs := splitNonEmptyLines(string(out))
	filtered := make([]string, 0, len(refs))
	for _, ref := range refs {
		if strings.Contains(filepath.Base(ref), "-") {
			filtered = append(filtered, ref)
		}
	}
	return filtered, nil
}

func listPreserveRefTreePaths(ctx context.Context, projectRoot, preserveRef string) ([]string, error) {
	out, err := gitpkg.Command(ctx, projectRoot, "ls-tree", "--full-tree", "-r", "--name-only", preserveRef).Output()
	if err != nil {
		return nil, fmt.Errorf("listing paths for preserve ref %s: %w", preserveRef, err)
	}
	return splitNonEmptyLines(string(out)), nil
}

func matchPreserveRefDirtyPaths(dirtyPaths, treePaths []string) []string {
	if len(dirtyPaths) == 0 || len(treePaths) == 0 {
		return nil
	}

	matched := make([]string, 0, len(dirtyPaths))
	seen := make(map[string]bool, len(dirtyPaths))
	for _, dirtyPath := range dirtyPaths {
		dirtyPath = normalizeDoctorUnjamPath(dirtyPath)
		if dirtyPath == "" || seen[dirtyPath] {
			continue
		}
		for _, treePath := range treePaths {
			if dirtyPathMatchesTreePath(dirtyPath, treePath) {
				seen[dirtyPath] = true
				matched = append(matched, dirtyPath)
				break
			}
		}
	}
	return matched
}

func dirtyPathMatchesTreePath(dirtyPath, treePath string) bool {
	dirtyPath = normalizeDoctorUnjamPath(dirtyPath)
	treePath = normalizeDoctorUnjamPath(treePath)
	if dirtyPath == "" || treePath == "" || dirtyPath == "." || treePath == "." {
		return false
	}
	if dirtyPath == treePath {
		return true
	}
	if strings.HasPrefix(treePath, dirtyPath+"/") {
		return true
	}
	return strings.HasPrefix(dirtyPath, treePath+"/")
}

func normalizeDoctorUnjamPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func removeDirtyPaths(paths, remove []string) []string {
	if len(paths) == 0 || len(remove) == 0 {
		return append([]string(nil), paths...)
	}
	removeSet := make(map[string]bool, len(remove))
	for _, path := range remove {
		removeSet[normalizeDoctorUnjamPath(path)] = true
	}
	remaining := make([]string, 0, len(paths))
	for _, path := range paths {
		if !removeSet[normalizeDoctorUnjamPath(path)] {
			remaining = append(remaining, path)
		}
	}
	return remaining
}

func stashPreserveRefDirtyPaths(ctx context.Context, projectRoot, preserveRef string, dirtyPaths []string) error {
	if len(dirtyPaths) == 0 {
		return nil
	}

	message := doctorUnjamPreserveRefStashMessagePrefix + preserveRef
	args := []string{"stash", "push", "--all", "--message", message, "--"}
	args = append(args, dirtyPaths...)
	out, err := gitpkg.Command(ctx, projectRoot, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("stashing preserve-derived dirt for %s: %s: %w", preserveRef, strings.TrimSpace(string(out)), err)
	}

	remaining, err := unjamProjectDirtyPathsForPaths(ctx, projectRoot, dirtyPaths)
	if err != nil {
		return err
	}
	if len(remaining) > 0 {
		return fmt.Errorf("stashing preserve-derived dirt for %s left dirty paths: %s", preserveRef, strings.Join(remaining, ", "))
	}
	return nil
}

func unjamProjectDirtyPathsForPaths(ctx context.Context, projectRoot string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	args := []string{"status", "--porcelain=v1", "--untracked-files=all", "--ignored=matching", "--"}
	args = append(args, paths...)
	out, err := gitpkg.Command(ctx, projectRoot, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("checking preserve-derived dirt after stash: %w", err)
	}
	return parseDDXStateCheckpointDirtyPaths(string(out)), nil
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

	orphanDirs, err := listOrphanedExecuteBeadWorktreeDirs(ctx, projectRoot)
	if err != nil {
		return report, err
	}
	for _, orphan := range orphanDirs {
		if err := os.RemoveAll(orphan.Path); err != nil {
			return report, fmt.Errorf("removing orphaned execute-bead worktree dir %s: %w", orphan.Path, err)
		}
		report.RemovedWorktrees = append(report.RemovedWorktrees, orphan)
		report.Actions = append(report.Actions, doctorUnjamAction{
			Kind:   "orphan_worktree_remove",
			Path:   orphan.Path,
			Reason: orphan.Reason,
		})
	}

	// git worktree prune always runs, even when no registered-prunable
	// worktrees matched our execute-bead filter above, so orphaned worktree
	// administrative data never lingers past a successful unjam pass.
	if err := gitpkg.Command(ctx, projectRoot, "worktree", "prune").Run(); err != nil {
		return report, fmt.Errorf("git worktree prune: %w", err)
	}
	report.PruneInvocations = 1

	totalRemoved := len(staleWorktrees) + len(orphanDirs)
	if totalRemoved > 0 {
		report.PrunedWorktrees = totalRemoved
		report.Actions = append(report.Actions, doctorUnjamAction{
			Kind:  "worktree_prune",
			Count: totalRemoved,
		})
	}
	return report, nil
}

// listOrphanedExecuteBeadWorktreeDirs scans the configured execute-bead
// worktree root for directories that git no longer tracks at all (their
// `git worktree list --porcelain` registration is gone, unlike the
// still-registered-but-prunable case listPrunableExecuteBeadWorktrees
// handles). Without this pass such a directory lingers forever: git does not
// know about it, so `git worktree prune` cannot remove it either.
func listOrphanedExecuteBeadWorktreeDirs(ctx context.Context, projectRoot string) ([]doctorUnjamWorktree, error) {
	tempRoot := config.ExecutionTempRoot(projectRoot)
	if tempRoot == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(tempRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading execute-bead worktree root: %w", err)
	}

	registered := map[string]bool{}
	if paths, listErr := (&agent.RealGitOps{}).WorktreeList(projectRoot); listErr == nil {
		for _, path := range paths {
			registered[filepath.Clean(path)] = true
		}
	}

	var orphans []doctorUnjamWorktree
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), agent.ExecuteBeadWtPrefix) {
			continue
		}
		path := filepath.Join(tempRoot, entry.Name())
		if registered[filepath.Clean(path)] {
			continue
		}
		reason := classifyOrphanedExecuteBeadWorktreeDir(path)
		if reason == "" {
			continue
		}
		orphans = append(orphans, doctorUnjamWorktree{Path: path, Reason: reason})
	}
	return orphans, nil
}

// classifyOrphanedExecuteBeadWorktreeDir returns "missing_gitdir" when path
// has no .git file at all, "missing_worktree_git" when the .git file points
// at a gitdir target that no longer exists, or "" when the directory still
// has valid git linkage (and so is left untouched, e.g. a non-worktree
// directory that merely shares the execute-bead prefix by coincidence).
func classifyOrphanedExecuteBeadWorktreeDir(path string) string {
	gitPath := filepath.Join(path, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		return "missing_gitdir"
	}
	if !info.Mode().IsRegular() {
		return ""
	}

	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "missing_gitdir"
	}
	content := strings.TrimSpace(string(data))
	const gitdirPrefix = "gitdir: "
	if !strings.HasPrefix(content, gitdirPrefix) {
		return "missing_gitdir"
	}

	gitdir := strings.TrimSpace(strings.TrimPrefix(content, gitdirPrefix))
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(path, gitdir)
	}
	if _, err := os.Stat(gitdir); err != nil {
		return "missing_worktree_git"
	}
	return ""
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
