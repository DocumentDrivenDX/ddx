package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// checkpointPreDispatchDirt captures DDx bookkeeping changes as a commit on
// the current branch without using the caller checkout's real index or commit
// hooks. The checkpoint is intentionally narrow: if the parent checkout also
// contains ordinary implementation files, the attempt fails with an actionable
// error so those changes can be committed in the bead's substantive
// [ddx-<id>] commit instead of being folded into the checkpoint.
const preDispatchCheckpointDirtyRefusalPrefix = "checkpoint refused to absorb implementation changes outside DDx bookkeeping: "
const preDispatchDirtyPreserveRefPrefix = "refs/ddx/pre-dispatch/"

// PreDispatchDirtyPreservation records the recoverable handle DDx created when
// it preserved implementation dirt before a watch-mode redispatch.
type PreDispatchDirtyPreservation struct {
	DirtyPaths     []string
	PreserveRef    string
	RecoverCommand string
}

var preDispatchDirtyPathLister = func(projectRoot string) ([]string, error) {
	return preDispatchCheckpointDirtyPaths(projectRoot)
}

func normalizePreDispatchDirtyPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		normalized = append(normalized, path)
	}
	sort.Strings(normalized)
	return normalized
}

func intersectPreDispatchDirtyPaths(current, want []string) []string {
	if len(current) == 0 || len(want) == 0 {
		return nil
	}
	wantSet := make(map[string]bool, len(want))
	for _, path := range want {
		wantSet[path] = true
	}
	var overlap []string
	for _, path := range current {
		if wantSet[path] {
			overlap = append(overlap, path)
		}
	}
	sort.Strings(overlap)
	return overlap
}

func classifyPreDispatchDirtyPaths(dirtyPaths []string) (allowedPaths, blockedPaths []string) {
	for _, path := range normalizePreDispatchDirtyPaths(dirtyPaths) {
		if preDispatchCheckpointAllowedPath(path) {
			allowedPaths = append(allowedPaths, path)
			continue
		}
		blockedPaths = append(blockedPaths, path)
	}
	return allowedPaths, blockedPaths
}

// stablePreDispatchImplementationDirtyPaths re-reads the project dirt and only
// treats implementation paths as actionable when they survive the immediate
// recheck. This filters transient rename/delete paths that can briefly appear
// in watch mode right after a successful land.
func stablePreDispatchImplementationDirtyPaths(projectRoot string, dirtyPaths []string) ([]string, []string, error) {
	normalized := normalizePreDispatchDirtyPaths(dirtyPaths)
	if projectRoot == "" || len(normalized) == 0 {
		return normalized, normalized, nil
	}

	currentPaths, err := preDispatchDirtyPathLister(projectRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("rechecking pre-dispatch dirt: %w", err)
	}
	return intersectPreDispatchDirtyPaths(currentPaths, normalized), currentPaths, nil
}

func resolveOptionalGitRef(dir, ref string) (string, bool) {
	out, err := internalgit.Command(context.Background(), dir, "rev-parse", "-q", "--verify", ref).Output()
	if err != nil {
		return "", false
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", false
	}
	return sha, true
}

func preDispatchDirtyRecoverCommand(preserveRef string) string {
	if preserveRef == "" {
		return ""
	}
	return "git stash apply " + preserveRef
}

func preservePreDispatchDirtyPaths(projectRoot string, dirtyPaths []string) (*PreDispatchDirtyPreservation, error) {
	if projectRoot == "" {
		return nil, fmt.Errorf("project root is required for pre-dispatch dirty preservation")
	}
	normalized := normalizePreDispatchDirtyPaths(dirtyPaths)
	if len(normalized) == 0 {
		return nil, fmt.Errorf("no dirty implementation paths to preserve")
	}

	var preserved *PreDispatchDirtyPreservation
	if err := withTrackerLock(projectRoot, "preserve_dirt", func() error {
		var err error
		preserved, err = preservePreDispatchDirtyPathsLocked(projectRoot, normalized)
		return err
	}); err != nil {
		return nil, err
	}
	return preserved, nil
}

func preservePreDispatchDirtyPathsLocked(projectRoot string, dirtyPaths []string) (*PreDispatchDirtyPreservation, error) {
	if err := internalgit.Command(context.Background(), projectRoot, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return nil, fmt.Errorf("verifying project worktree for pre-dispatch preservation: %w", err)
	}
	if out, err := internalgit.Command(context.Background(), projectRoot, "rev-parse", "--verify", "HEAD").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("resolving HEAD for pre-dispatch preservation: %s: %w", strings.TrimSpace(string(out)), err)
	}

	stashBefore, _ := resolveOptionalGitRef(projectRoot, "refs/stash")
	preserveID := GenerateAttemptID()
	preserveRef := preDispatchDirtyPreserveRefPrefix + preserveID
	stashMsg := "ddx pre-dispatch preserve " + preserveID
	stashArgs := []string{"stash", "push", "--include-untracked", "--message", stashMsg, "--"}
	stashArgs = append(stashArgs, dirtyPaths...)
	if out, err := internalgit.Command(context.Background(), projectRoot, stashArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("preserving pre-dispatch dirty implementation paths: %s: %w", strings.TrimSpace(string(out)), err)
	}

	stashAfter, ok := resolveOptionalGitRef(projectRoot, "refs/stash")
	if !ok || stashAfter == "" || stashAfter == stashBefore {
		return nil, fmt.Errorf("preserving pre-dispatch dirty implementation paths did not create a recoverable stash entry")
	}
	if out, err := internalgit.Command(context.Background(), projectRoot, "update-ref", preserveRef, stashAfter).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pinning pre-dispatch dirty preservation ref: %s: %w", strings.TrimSpace(string(out)), err)
	}
	_, _ = internalgit.Command(context.Background(), projectRoot, "stash", "drop", "stash@{0}").CombinedOutput()

	remainingPaths, err := preDispatchDirtyPathLister(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("verifying preserved pre-dispatch dirt: %w", err)
	}
	if overlap := intersectPreDispatchDirtyPaths(remainingPaths, dirtyPaths); len(overlap) > 0 {
		return nil, fmt.Errorf("preserving pre-dispatch dirty implementation paths left the project worktree dirty: %s", strings.Join(overlap, ", "))
	}

	return &PreDispatchDirtyPreservation{
		DirtyPaths:     append([]string(nil), dirtyPaths...),
		PreserveRef:    preserveRef,
		RecoverCommand: preDispatchDirtyRecoverCommand(preserveRef),
	}, nil
}

func checkpointPreDispatchDirt(projectRoot, attemptID string) (bool, error) {
	workTreeOut, err := internalgit.Command(context.Background(), projectRoot, "rev-parse", "--is-inside-work-tree").CombinedOutput()
	if err != nil {
		return false, nil
	}
	if strings.TrimSpace(string(workTreeOut)) != "true" {
		return false, fmt.Errorf("verifying project worktree for pre-dispatch checkpoint: git rev-parse --is-inside-work-tree returned %q", strings.TrimSpace(string(workTreeOut)))
	}
	headOut, err := internalgit.Command(context.Background(), projectRoot, "rev-parse", "--verify", "HEAD").CombinedOutput()
	if err != nil {
		return false, nil
	}
	head := strings.TrimSpace(string(headOut))
	if head == "" {
		return false, nil
	}

	skipWorktreePaths, err := checkpointSkipWorktreePaths(projectRoot)
	if err != nil {
		return false, err
	}

	dirtyPaths, err := preDispatchDirtyPathLister(projectRoot)
	if err != nil {
		return false, err
	}
	if len(dirtyPaths) == 0 {
		return false, nil
	}

	allowedPaths, blockedPaths := classifyPreDispatchDirtyPaths(dirtyPaths)
	if len(blockedPaths) > 0 {
		stableBlockedPaths, currentDirtyPaths, err := stablePreDispatchImplementationDirtyPaths(projectRoot, blockedPaths)
		if err != nil {
			return false, err
		}
		if len(stableBlockedPaths) > 0 {
			return false, fmt.Errorf(
				"%s%s; commit or clean those files before rerunning so the bead's [ddx-<id>] substantive commit stays intentional",
				preDispatchCheckpointDirtyRefusalPrefix,
				strings.Join(stableBlockedPaths, ", "),
			)
		}
		allowedPaths, _ = classifyPreDispatchDirtyPaths(currentDirtyPaths)
	}
	if len(allowedPaths) == 0 {
		return false, nil
	}

	indexFile, err := config.CreateExecutionScratch(projectRoot, "ddx-pre-dispatch-index-*")
	if err != nil {
		return false, fmt.Errorf("creating temp checkpoint index: %w", err)
	}
	indexPath := indexFile.Name()
	_ = indexFile.Close()
	_ = os.Remove(indexPath)
	defer func() { _ = os.Remove(indexPath) }()

	gitWithIndex := func(args ...string) ([]byte, error) {
		cmd := internalgit.Command(context.Background(), projectRoot, args...)
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+indexPath)
		return cmd.CombinedOutput()
	}

	if out, err := gitWithIndex("read-tree", "HEAD"); err != nil {
		return false, fmt.Errorf("initializing checkpoint index: %s: %w", strings.TrimSpace(string(out)), err)
	}

	addArgs := []string{"add", "-A", "--force", "--"}
	addArgs = append(addArgs, allowedPaths...)
	// Execution evidence is per-machine and must never be checkpoint-committed,
	// even by this --force add (ddx-d10073a8).
	addArgs = append(addArgs, ":(exclude,glob).ddx/executions/**")
	if out, err := gitWithIndex(addArgs...); err != nil {
		return false, fmt.Errorf("staging checkpoint changes: %s: %w", strings.TrimSpace(string(out)), err)
	}

	changedOut, err := gitWithIndex("diff", "--cached", "--name-only")
	if err != nil {
		return false, fmt.Errorf("checking checkpoint diff: %w", err)
	}
	if len(bytes.TrimSpace(changedOut)) == 0 {
		return false, nil
	}

	treeOut, err := gitWithIndex("write-tree")
	if err != nil {
		return false, fmt.Errorf("writing checkpoint tree: %s: %w", strings.TrimSpace(string(treeOut)), err)
	}
	tree := strings.TrimSpace(string(treeOut))
	msg := "chore: checkpoint pre-execute-bead " + attemptID
	commitOut, err := internalgit.Command(context.Background(), projectRoot,
		"-c", "user.name=ddx-checkpoint",
		"-c", "user.email=checkpoint@ddx.local",
		"commit-tree", tree, "-p", head, "-m", msg,
	).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("creating checkpoint commit: %s: %w", strings.TrimSpace(string(commitOut)), err)
	}
	commit := strings.TrimSpace(string(commitOut))

	refOut, _ := internalgit.Command(context.Background(), projectRoot, "symbolic-ref", "-q", "HEAD").Output()
	ref := strings.TrimSpace(string(refOut))
	if ref == "" {
		ref = "HEAD"
	}
	if out, err := internalgit.Command(context.Background(), projectRoot, "update-ref", ref, commit, head).CombinedOutput(); err != nil {
		return false, fmt.Errorf("advancing checkpoint ref: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if out, err := internalgit.Command(context.Background(), projectRoot, "read-tree", "HEAD").CombinedOutput(); err != nil {
		return false, fmt.Errorf("syncing checkpoint index: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if err := restoreCheckpointSkipWorktreePaths(projectRoot, skipWorktreePaths); err != nil {
		return false, err
	}
	return true, nil
}

func checkpointSkipWorktreePaths(projectRoot string) ([]string, error) {
	out, err := internalgit.Command(context.Background(), projectRoot, "ls-files", "-t", "-z").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("listing skip-worktree paths: %s: %w", strings.TrimSpace(string(out)), err)
	}
	var paths []string
	for len(out) > 0 {
		recordEnd := bytes.IndexByte(out, 0)
		if recordEnd == -1 {
			recordEnd = len(out)
		}
		record := out[:recordEnd]
		if recordEnd < len(out) {
			out = out[recordEnd+1:]
		} else {
			out = nil
		}
		if len(record) < 3 || record[0] != 'S' || record[1] != ' ' {
			continue
		}
		path := filepath.ToSlash(string(record[2:]))
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func restoreCheckpointSkipWorktreePaths(projectRoot string, paths []string) error {
	for start := 0; start < len(paths); start += 100 {
		end := start + 100
		if end > len(paths) {
			end = len(paths)
		}
		args := append([]string{"update-index", "--skip-worktree", "--"}, paths[start:end]...)
		if out, err := internalgit.Command(context.Background(), projectRoot, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("restoring skip-worktree paths after checkpoint index sync: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}
	return nil
}

// isMaterializedSkillSymlink reports whether a path is a skill directory symlink
// that has been materialized into a real directory by an external tool. Skill
// symlinks (.crush/skills/*, .claude/skills/*, .agents/skills/*) tracked in git
// as symlinks (mode 120000) but present in the working tree as directories are
// benign and do not indicate uncommitted implementation changes.
func isMaterializedSkillSymlink(projectRoot, path string) bool {
	// Check if path is in a skill directory root
	skillDirs := []string{".crush/skills", ".claude/skills", ".agents/skills"}
	var isSkillPath bool
	for _, skillDir := range skillDirs {
		if path == skillDir || strings.HasPrefix(path, skillDir+"/") {
			isSkillPath = true
			break
		}
	}
	if !isSkillPath {
		return false
	}

	// Check if it's tracked in git as a symlink
	mode, err := internalgit.Command(context.Background(), projectRoot,
		"ls-files", "--stage", "--", path).Output()
	if err != nil || len(mode) == 0 {
		return false
	}
	// ls-files --stage output is: "<mode> <object> <stage>\t<file>"
	// We need to extract the mode field (first field, 120000 for symlink)
	parts := strings.Fields(string(mode))
	if len(parts) < 1 {
		return false
	}
	gitMode := parts[0]
	if gitMode != "120000" {
		return false
	}

	// Check if the working tree has it as a directory
	fullPath := filepath.Join(projectRoot, filepath.FromSlash(path))
	info, err := os.Stat(fullPath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func preDispatchCheckpointDirtyPaths(projectRoot string) ([]string, error) {
	out, err := internalgit.Command(context.Background(), projectRoot,
		"status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored=matching", "--", ".").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("listing checkpoint dirt: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if len(out) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	paths := make([]string, 0, 8)
	for len(out) > 0 {
		recordEnd := bytes.IndexByte(out, 0)
		if recordEnd == -1 {
			recordEnd = len(out)
		}
		record := out[:recordEnd]
		if recordEnd < len(out) {
			out = out[recordEnd+1:]
		} else {
			out = nil
		}
		if len(record) < 3 {
			continue
		}
		status := string(record[:2])
		path := filepath.ToSlash(string(record[3:]))
		if status == "!!" && !preDispatchCheckpointAllowedPath(path) {
			continue
		}
		if preDispatchCheckpointIgnoredPath(path) {
			continue
		}
		if isMaterializedSkillSymlink(projectRoot, path) {
			continue
		}
		if path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
		if record[0] == 'R' || record[0] == 'C' {
			recordEnd = bytes.IndexByte(out, 0)
			if recordEnd == -1 {
				recordEnd = len(out)
			}
			record = out[:recordEnd]
			if recordEnd < len(out) {
				out = out[recordEnd+1:]
			} else {
				out = nil
			}
			path := filepath.ToSlash(string(record))
			if status == "!!" && !preDispatchCheckpointAllowedPath(path) {
				continue
			}
			if preDispatchCheckpointIgnoredPath(path) {
				continue
			}
			if isMaterializedSkillSymlink(projectRoot, path) {
				continue
			}
			if path != "" && !seen[path] {
				seen[path] = true
				paths = append(paths, path)
			}
		}
	}
	return paths, nil
}

func preDispatchCheckpointAllowedPath(path string) bool {
	if isLockMetricsPath(path) {
		return false
	}
	switch {
	case path == ".ddx/beads.jsonl":
		return true
	case path == ".ddx/beads-archive.jsonl":
		return true
	case strings.HasPrefix(path, ".ddx/executions/"):
		return true
	case strings.HasPrefix(path, ".ddx/metrics/"):
		return true
	case strings.HasPrefix(path, ".ddx/runs/"):
		return true
	case path == ExecutionCleanupMetadataFileName:
		return true
	default:
		return false
	}
}

func preDispatchCheckpointIgnoredPath(path string) bool {
	switch {
	case isLockMetricsPath(path):
		return true
	case path == ".ddx/.git-tracker.lock":
		return true
	case strings.HasPrefix(path, ".ddx/.git-tracker.lock/"):
		return true
	case strings.HasPrefix(path, ".ddx/.git-tracker.lock.") && strings.HasSuffix(path, ".lock"):
		// Main-git-lock stale-break uses a stable sibling advisory-lock sidecar
		// (for example .ddx/.git-tracker.lock.stale-break.lock) and contender-
		// unique tombstones. Runtime coordination only — not parent work.
		return true
	case path == ".ddx/beads.lock":
		return true
	case strings.HasPrefix(path, ".ddx/beads.lock/"):
		return true
	case strings.HasPrefix(path, ".ddx/beads.lock.") && strings.HasSuffix(path, ".lock"):
		// Collection-lock recovery uses a stable sibling advisory-lock sidecar
		// (for example .ddx/beads.lock.stale-break.lock). It is runtime
		// coordination state, never bead implementation work, and must not
		// make the next pre-dispatch checkpoint refuse an otherwise clean tree.
		return true
	case strings.HasPrefix(path, ".ddx/attachments/"):
		return true
	case strings.HasPrefix(path, ".ddx/harness-sessions/"):
		return true
	case path == ".ddx/run-state.json":
		return true
	case path == ".ddx/run-state":
		return true
	case strings.HasPrefix(path, ".ddx/run-state/"):
		return true
	case preDispatchCheckpointIgnoredExecutionEmbeddedPath(path):
		return true
	case strings.HasPrefix(path, ".ddx/beads.jsonl.tmp-"):
		return true
	case path == ".ddx/workers":
		return true
	case strings.HasPrefix(path, ".ddx/workers/"):
		return true
	default:
		return false
	}
}

func preDispatchCheckpointIgnoredExecutionEmbeddedPath(path string) bool {
	const prefix = ".ddx/executions/"
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := strings.TrimPrefix(path, prefix)
	attemptID, remainder, ok := strings.Cut(rest, "/")
	if !ok || attemptID == "" {
		return false
	}
	return remainder == "embedded" || strings.HasPrefix(remainder, "embedded/")
}
